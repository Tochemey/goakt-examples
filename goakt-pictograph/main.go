// MIT License
//
// Copyright (c) 2022-2026 GoAkt Team
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package main

import (
	"context"
	"embed"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/tochemey/goakt/v4/actor"
	"github.com/tochemey/goakt/v4/discovery/static"
	gerrors "github.com/tochemey/goakt/v4/errors"
	"github.com/tochemey/goakt/v4/log"
	"github.com/tochemey/goakt/v4/remote"
)

const (
	systemName  = "pictograph"
	readTimeout = 10 * time.Second

	// defaultBindHost — same shape as goakt-game: 127.0.0.1 keeps
	// single-machine multi-node dev self-contained; override with
	// --bind-host for multi-machine setups.
	defaultBindHost = "127.0.0.1"
)

//go:embed web/index.html web/main.js
var webFS embed.FS

// Same CLI shape as goakt-game so the docker-compose / Makefile
// recipes from that example translate one-to-one.
var (
	httpPort      = flag.Int("http-port", 8080, "HTTP/WebSocket port for browser clients")
	bindHost      = flag.String("bind-host", defaultBindHost, "Host this node advertises for cluster traffic")
	remotingPort  = flag.Int("remoting-port", 9000, "gRPC port for inter-node actor messaging")
	discoveryPort = flag.Int("discovery-port", 9001, "Gossip port used by the static discovery provider")
	peersPort     = flag.Int("peers-port", 9002, "Cluster peer state-sync port")
	peers         = flag.String("peers", "", "Comma-separated host:discoveryPort list of cluster bootstrap peers; defaults to this node only")
)

func main() {
	flag.Parse()
	ctx := context.Background()
	logger := log.DefaultLogger

	store := newProfileStore()
	leaderboard := NewLeaderboard()

	// Both the profile store and the leaderboard are registered as
	// system Extensions: cluster-spawned actors (RoomActor, LobbyActor)
	// are re-instantiated via the kind registry on whichever node hosts
	// them — that path bypasses constructor-injected deps, so anything
	// load-bearing must be reachable from the system itself.
	system, err := buildActorSystem(logger, store, leaderboard)
	if err != nil {
		logger.Fatal(err)
	}

	if err := system.Start(ctx); err != nil {
		logger.Fatal(err)
	}
	// Bind the leaderboard to the started system; this is what gives it
	// access to the Replicator and a stable per-node author id.
	leaderboard.Bind(system)

	// Singleton lobby — gated on IsLeader so that only the cluster's
	// oldest node attempts the spawn. If a late-joining node also
	// called SpawnSingleton, the library would route the request to the
	// leader and log an error-level "singleton already exists" on the
	// receiving side even though our local handler treats that as
	// benign. GoAkt's singleton manager relocates the lobby to the new
	// oldest node automatically on failover, so latecomers don't need
	// to respawn it themselves.
	//
	// On an IsLeader error (e.g. leader not yet elected) fall back to
	// the unconditional SpawnSingleton so we don't fail-closed.
	isLeader, leaderErr := system.IsLeader(ctx)
	if leaderErr != nil || isLeader {
		if _, err := system.SpawnSingleton(ctx, LobbyActorName, new(LobbyActor)); err != nil {
			if !errors.Is(err, gerrors.ErrSingletonAlreadyExists) {
				logger.Fatal(err)
			}
			logger.Infof("lobby singleton already running elsewhere in the cluster")
		}
	} else {
		logger.Info("not the cluster leader; lobby singleton is hosted on another node")
	}

	web, err := fs.Sub(webFS, "web")
	if err != nil {
		logger.Fatal(err)
	}

	// drainCtx signals every WebSocket reader to exit when the server
	// is shutting down. http.Server.Shutdown does NOT terminate
	// hijacked connections (which WS becomes after Accept), so without
	// this signal the reader loops would block on conn.Read forever
	// past the shutdown deadline. wsHandlers tracks each in-flight WS
	// handler so main can wait for their teardown (GoodbyePlayer Tell
	// to the room, conn.Close) to finish before stopping the actor
	// system.
	drainCtx, cancelDrain := context.WithCancel(context.Background())
	var wsHandlers sync.WaitGroup

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", wsHandler(system, leaderboard, drainCtx, &wsHandlers, logger))
	mux.Handle("/", http.FileServer(http.FS(web)))

	addr := fmt.Sprintf(":%d", *httpPort)
	srv := &http.Server{
		Addr:        addr,
		Handler:     mux,
		ReadTimeout: readTimeout,
	}
	// Auto-fire the drain when Shutdown is called so callers don't
	// have to remember to cancel it manually.
	srv.RegisterOnShutdown(cancelDrain)

	go func() {
		logger.Infof("listening on http://localhost%s (open in browser)", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal(err)
		}
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	logger.Info("shutting down")

	shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// srv.Shutdown closes the listener (refuses new connections),
	// waits for non-hijacked requests, and fires the drainCtx via the
	// RegisterOnShutdown hook above.
	if err := srv.Shutdown(shutCtx); err != nil {
		logger.Warnf("http shutdown: %v", err)
	}

	// Now wait for every WebSocket handler to finish its teardown
	// (GoodbyePlayer Tell to room, conn.Close, sessionPID shutdown).
	// We can't use http.Server's wait here because hijacked
	// connections are explicitly excluded from Shutdown's accounting.
	drained := make(chan struct{})
	go func() {
		wsHandlers.Wait()
		close(drained)
	}()
	select {
	case <-drained:
		logger.Info("all websocket sessions drained")
	case <-shutCtx.Done():
		logger.Warnf("websocket drain exceeded %s; some sessions may be cut mid-teardown", shutCtx.Err())
	}

	if err := system.Stop(shutCtx); err != nil {
		logger.Warnf("actor system stop: %v", err)
	}
}

// buildActorSystem assembles the cluster-aware ActorSystem: remoting
// (with CBOR serializers for every cross-node message type), pub/sub,
// the profile-store and leaderboard extensions, and a cluster config
// that registers our actor kinds and turns on CRDT replication.
func buildActorSystem(logger log.Logger, store *profileStore, leaderboard *Leaderboard) (actor.ActorSystem, error) {
	cbor := remote.NewCBORSerializer()
	// Every type a remote actor might receive needs a registered
	// serializer. Lobby↔Gateway (JoinOrCreate/JoinOrCreateResult),
	// Room↔Session (PlayerHello/GoodbyePlayer/PlayerInput) and every
	// outbound event type (which travels via pub/sub when the topic
	// fans out across nodes) — register all of them.
	remoteCfg := remote.NewConfig(*bindHost, *remotingPort,
		remote.WithSerializers((*JoinOrCreate)(nil), cbor),
		remote.WithSerializers((*JoinOrCreateResult)(nil), cbor),
		remote.WithSerializers((*PlayerHello)(nil), cbor),
		remote.WithSerializers((*GoodbyePlayer)(nil), cbor),
		remote.WithSerializers((*PlayerInput)(nil), cbor),
		remote.WithSerializers((*JoinedEvent)(nil), cbor),
		remote.WithSerializers((*StateEvent)(nil), cbor),
		remote.WithSerializers((*StrokeEvent)(nil), cbor),
		remote.WithSerializers((*ClearEvent)(nil), cbor),
		remote.WithSerializers((*ChatEvent)(nil), cbor),
		remote.WithSerializers((*GuessedEvent)(nil), cbor),
		remote.WithSerializers((*ScoreEvent)(nil), cbor),
		remote.WithSerializers((*WordChoicesEvent)(nil), cbor),
		remote.WithSerializers((*SecretWordEvent)(nil), cbor),
		remote.WithSerializers((*RoundOverEvent)(nil), cbor),
		remote.WithSerializers((*GameOverEvent)(nil), cbor),
		remote.WithSerializers((*ErrorEvent)(nil), cbor),
		// Grain wire payloads.
		remote.WithSerializers((*GetProfile)(nil), cbor),
		remote.WithSerializers((*RecordGame)(nil), cbor),
		remote.WithSerializers((*SetName)(nil), cbor),
		remote.WithSerializers((*ProfileView)(nil), cbor),
	)

	discoConfig := &static.Config{Hosts: peerList(*peers, *bindHost, *discoveryPort)}
	disco := static.NewDiscovery(discoConfig)

	clusterCfg := actor.NewClusterConfig().
		WithDiscovery(disco).
		WithDiscoveryPort(*discoveryPort).
		WithPeersPort(*peersPort).
		WithPartitionCount(20).
		WithBootstrapTimeout(10*time.Second).
		WithReadTimeout(3*time.Second).
		WithWriteTimeout(3*time.Second).
		// Only kinds that may be spawned via SpawnOn / SpawnSingleton
		// need to be registered — local-only PlayerSessionActor is
		// intentionally absent.
		WithKinds(new(RoomActor), new(LobbyActor)).
		// CRDT replication backs Leaderboard.RecordWin / Top. Defaults
		// are fine for a demo (30s anti-entropy, 5m prune).
		WithCRDT()

	return actor.NewActorSystem(systemName,
		actor.WithLogger(logger),
		actor.WithRemote(remoteCfg),
		actor.WithCluster(clusterCfg),
		actor.WithPubSub(),
		actor.WithExtensions(store, leaderboard),
	)
}

// peerList parses --peers ("host:port,host:port,…"), or — if empty —
// returns just this node's own discovery endpoint, which is enough for
// single-node bootstrap into a cluster of size 1.
func peerList(raw, selfHost string, selfPort int) []string {
	if strings.TrimSpace(raw) == "" {
		return []string{fmt.Sprintf("%s:%d", selfHost, selfPort)}
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, peer := range parts {
		peer = strings.TrimSpace(peer)
		if peer != "" {
			out = append(out, peer)
		}
	}
	return out
}
