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
	"bytes"
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
	"github.com/tochemey/goakt/v4/discovery/kubernetes"
	gerrors "github.com/tochemey/goakt/v4/errors"
	"github.com/tochemey/goakt/v4/log"
	"github.com/tochemey/goakt/v4/remote"

	"github.com/tochemey/goakt-examples/v2/goakt-scrabble/scrabble"
)

const (
	systemName      = "scrabble"
	readTimeout     = 10 * time.Second
	defaultBindHost = "127.0.0.1"

	// Port names exposed by the StatefulSet's container ports; the
	// kubernetes discovery provider reads these from the pod spec.
	discoveryPortName = "discovery"
	remotingPortName  = "remoting"
	peersPortName     = "peers"
)

//go:embed web/index.html web/main.js
var webFS embed.FS

//go:embed dict/en.txt
var dictFS embed.FS

var (
	httpPort      = flag.Int("http-port", 8080, "HTTP/WebSocket port for browser clients")
	bindHost      = flag.String("bind-host", defaultBindHost, "Host this node advertises for cluster traffic")
	remotingPort  = flag.Int("remoting-port", 9000, "gRPC port for inter-node actor messaging")
	discoveryPort = flag.Int("discovery-port", 9001, "Gossip port used by the kubernetes discovery provider")
	peersPort     = flag.Int("peers-port", 9002, "Cluster peer state-sync port")
	namespace     = flag.String("namespace", "", "Kubernetes namespace this pod runs in (defaults to $POD_NAMESPACE)")
	appLabel      = flag.String("app-label", "scrabble", "Value of the 'app' pod label used to match cluster peers")
	databaseURL   = flag.String("database-url", "", "Postgres DSN for the profile store (defaults to $DATABASE_URL; in-memory fallback if unset)")
)

const profileStoreInitTimeout = 10 * time.Second

func main() {
	flag.Parse()
	ctx := context.Background()
	logger := log.DefaultLogger

	registry, err := buildRegistry(logger)
	if err != nil {
		logger.Fatal(err)
	}

	store, closeStore, err := buildProfileStore(ctx, logger)
	if err != nil {
		logger.Fatal(err)
	}
	defer closeStore()

	leaderboard := NewLeaderboard()

	system, err := buildActorSystem(logger, registry, store, leaderboard)
	if err != nil {
		logger.Fatal(err)
	}

	if err := system.Start(ctx); err != nil {
		logger.Fatal(err)
	}
	leaderboard.Bind(system)

	isLeader, leaderErr := system.IsLeader(ctx)
	if leaderErr != nil || isLeader {
		if _, err := system.SpawnSingleton(ctx, LobbyActorName, new(LobbyActor)); err != nil {
			if !errors.Is(err, gerrors.ErrSingletonAlreadyExists) {
				logger.Fatal(err)
			}
			logger.Info("lobby singleton already running elsewhere in the cluster")
		}
	} else {
		logger.Info("not the cluster leader; lobby singleton is hosted on another node")
	}

	web, err := fs.Sub(webFS, "web")
	if err != nil {
		logger.Fatal(err)
	}

	drainCtx, cancelDrain := context.WithCancel(context.Background())
	var wsHandlers sync.WaitGroup

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", wsHandler(system, leaderboard, drainCtx, &wsHandlers, logger))
	mux.Handle("/", noStore(http.FileServer(http.FS(web))))

	addr := fmt.Sprintf(":%d", *httpPort)
	srv := &http.Server{
		Addr:        addr,
		Handler:     mux,
		ReadTimeout: readTimeout,
	}
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

	if err := srv.Shutdown(shutCtx); err != nil {
		logger.Warnf("http shutdown: %v", err)
	}

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

// buildRegistry loads each bundled wordlist into a per-language DAWG.
// New languages are added by dropping `dict/<code>.txt` and registering
// here.
func buildRegistry(logger log.Logger) (*Registry, error) {
	registry := NewRegistry()

	english, err := loadBundle(scrabble.English(), "dict/en.txt")
	if err != nil {
		return nil, fmt.Errorf("english: %w", err)
	}
	registry.Add(english)
	logger.Infof("loaded language en (%d words)", english.Dawg.Size())

	return registry, nil
}

func loadBundle(lang *scrabble.Language, path string) (*LangBundle, error) {
	data, err := dictFS.ReadFile(path)
	if err != nil {
		return nil, err
	}

	dawg, err := scrabble.BuildDAWG(lang, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}

	return &LangBundle{Lang: lang, Dawg: dawg}, nil
}

// buildProfileStore picks a profile-store backend. If --database-url
// (or $DATABASE_URL) is set, it connects to Postgres, runs the schema
// migration, and returns a pgProfileStore + its Close function.
// Otherwise it returns the in-memory store. A non-empty DSN that fails
// to connect is a hard error; we don't silently degrade to in-memory.
func buildProfileStore(ctx context.Context, logger log.Logger) (profileStore, func(), error) {
	dsn := strings.TrimSpace(*databaseURL)
	if dsn == "" {
		dsn = os.Getenv("DATABASE_URL")
	}

	if dsn == "" {
		logger.Info("profile store: using in-memory backend (set DATABASE_URL for Postgres)")
		return newMemProfileStore(), func() {}, nil
	}

	initCtx, cancel := context.WithTimeout(ctx, profileStoreInitTimeout)
	defer cancel()

	pg, err := newPgProfileStore(initCtx, dsn)
	if err != nil {
		return nil, nil, fmt.Errorf("postgres profile store: %w", err)
	}

	logger.Info("profile store: using Postgres backend")

	return pg, pg.Close, nil
}

func buildActorSystem(logger log.Logger, registry *Registry, store profileStore, leaderboard *Leaderboard) (actor.ActorSystem, error) {
	cbor := remote.NewCBORSerializer()

	remoteCfg := remote.NewConfig(*bindHost, *remotingPort,
		remote.WithSerializers((*JoinOrCreate)(nil), cbor),
		remote.WithSerializers((*JoinOrCreateResult)(nil), cbor),
		remote.WithSerializers((*PlayerHello)(nil), cbor),
		remote.WithSerializers((*GoodbyePlayer)(nil), cbor),
		remote.WithSerializers((*PlayerInput)(nil), cbor),
		remote.WithSerializers((*BotPlay)(nil), cbor),
		remote.WithSerializers((*YourTurn)(nil), cbor),
		remote.WithSerializers((*JoinedEvent)(nil), cbor),
		remote.WithSerializers((*StateEvent)(nil), cbor),
		remote.WithSerializers((*MoveEvent)(nil), cbor),
		remote.WithSerializers((*ChatEvent)(nil), cbor),
		remote.WithSerializers((*ErrorEvent)(nil), cbor),
		remote.WithSerializers((*GameOverEvent)(nil), cbor),
		remote.WithSerializers((*GetProfile)(nil), cbor),
		remote.WithSerializers((*RecordGame)(nil), cbor),
		remote.WithSerializers((*SetName)(nil), cbor),
		remote.WithSerializers((*ProfileView)(nil), cbor),
	)

	ns := strings.TrimSpace(*namespace)
	if ns == "" {
		ns = os.Getenv("POD_NAMESPACE")
	}
	if ns == "" {
		return nil, fmt.Errorf("namespace is required (set --namespace or POD_NAMESPACE)")
	}

	disco := kubernetes.NewDiscovery(&kubernetes.Config{
		Namespace:         ns,
		DiscoveryPortName: discoveryPortName,
		RemotingPortName:  remotingPortName,
		PeersPortName:     peersPortName,
		PodLabels:         map[string]string{"app": *appLabel},
	})

	clusterCfg := actor.NewClusterConfig().
		WithDiscovery(disco).
		WithDiscoveryPort(*discoveryPort).
		WithPeersPort(*peersPort).
		WithPartitionCount(20).
		WithBootstrapTimeout(10 * time.Second).
		WithReadTimeout(3 * time.Second).
		WithWriteTimeout(3 * time.Second).
		WithKinds(new(RoomActor), new(LobbyActor)).
		WithCRDT()

	return actor.NewActorSystem(systemName,
		actor.WithLogger(logger),
		actor.WithRemote(remoteCfg),
		actor.WithCluster(clusterCfg),
		actor.WithPubSub(),
		actor.WithExtensions(registry, store, leaderboard),
	)
}

// noStore wraps a handler so the browser will not cache its responses.
// The bundled web assets are rebuilt and re-embedded on every `make build`;
// caching them on the client masks the new bytes behind a stale fetch.
func noStore(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		h.ServeHTTP(w, r)
	})
}

