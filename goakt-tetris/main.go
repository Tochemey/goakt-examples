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
	"syscall"
	"time"

	"github.com/tochemey/goakt/v4/actor"
	"github.com/tochemey/goakt/v4/discovery/static"
	gerrors "github.com/tochemey/goakt/v4/errors"
	"github.com/tochemey/goakt/v4/log"
	"github.com/tochemey/goakt/v4/remote"

	"github.com/tochemey/goakt-examples/v2/internal/remoting"
)

const (
	systemName  = "tetris"
	readTimeout = 10 * time.Second

	// Default bind host for cluster traffic. Override with --bind-host for
	// multi-machine setups; "127.0.0.1" keeps single-machine multi-node
	// dev (Phase 2.2) self-contained.
	defaultBindHost = "127.0.0.1"
)

// Embed only the runtime assets — the .ts source lives in web/ alongside
// its compiled .js output so editors get a single working directory, but
// the binary doesn't need to carry the source.
//
//go:embed web/index.html web/main.js
var webFS embed.FS

// CLI flags. Each node in a multi-node cluster needs unique values for
// http/remoting/discovery/peers ports; --peers points every node at every
// other node's discovery port (including itself).
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

	system, err := buildActorSystem(logger)
	if err != nil {
		logger.Fatal(err)
	}
	if err := system.Start(ctx); err != nil {
		logger.Fatal(err)
	}

	// Singleton matchmaker. One per cluster — even when scaling to N
	// pods, exactly one MatchFactory runs across them. Every node calls
	// SpawnSingleton on boot; only the first wins, the rest get
	// ErrSingletonAlreadyExists, which is fine — they share the existing
	// singleton via ActorOf.
	if _, err := system.SpawnSingleton(ctx, MatchmakerActorName, &MatchFactory{}); err != nil {
		if !errors.Is(err, gerrors.ErrSingletonAlreadyExists) {
			logger.Fatal(err)
		}
		logger.Infof("matchmaker singleton already running elsewhere in the cluster")
	}

	web, err := fs.Sub(webFS, "web")
	if err != nil {
		logger.Fatal(err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", wsHandler(system, logger))
	mux.Handle("/", http.FileServer(http.FS(web)))

	addr := fmt.Sprintf(":%d", *httpPort)
	srv := &http.Server{
		Addr:        addr,
		Handler:     mux,
		ReadTimeout: readTimeout,
	}

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
	_ = srv.Shutdown(shutCtx)
	_ = system.Stop(shutCtx)
}

// buildActorSystem assembles the cluster-aware ActorSystem: remote
// (with the serializers our cross-node message types need), discovery,
// and the cluster config that registers our actor kinds.
func buildActorSystem(logger log.Logger) (actor.ActorSystem, error) {
	cbor := remote.NewCBORSerializer()
	remoteCfg := remoting.NewConfig(*bindHost, *remotingPort,
		// Messages that cross the wire when a match lands on a remote
		// node. Register the pointer-typed nil sentinel — that's the form
		// GoAkt's serializer registry expects.
		remote.WithSerializers((*PlayerInput)(nil), cbor),
		remote.WithSerializers((*Subscribe)(nil), cbor),
		remote.WithSerializers((*Unsubscribe)(nil), cbor),
		remote.WithSerializers((*Snapshot)(nil), cbor),
		remote.WithSerializers((*CreateMatch)(nil), cbor),
		remote.WithSerializers((*MatchCreated)(nil), cbor),
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
		// Only kinds that may be spawned via SpawnOn / SpawnSingleton go
		// here. PlayerSessionActor stays node-local and is *not* listed.
		WithKinds(new(MatchActor), new(MatchFactory))

	return actor.NewActorSystem(systemName,
		actor.WithLogger(logger),
		actor.WithRemote(remoteCfg),
		actor.WithCluster(clusterCfg),
	)
}

// peerList parses --peers ("host:port,host:port,…"), or — if empty —
// returns just this node's own discovery endpoint, which is enough for
// single-node Phase 2.1 to bootstrap into a cluster of size 1.
func peerList(raw, selfHost string, selfPort int) []string {
	if strings.TrimSpace(raw) == "" {
		return []string{fmt.Sprintf("%s:%d", selfHost, selfPort)}
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
