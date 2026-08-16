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

package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	goakt "github.com/tochemey/goakt/v4/actor"
	"github.com/tochemey/goakt/v4/discovery/kubernetes"
	gerrors "github.com/tochemey/goakt/v4/errors"
	"github.com/tochemey/goakt/v4/log"
	"github.com/tochemey/goakt/v4/remote"

	"github.com/tochemey/goakt-examples/v2/goakt-blockchain/actors"
	"github.com/tochemey/goakt-examples/v2/goakt-blockchain/messages"
	"github.com/tochemey/goakt-examples/v2/goakt-blockchain/persistence"
	"github.com/tochemey/goakt-examples/v2/goakt-blockchain/service"
	"github.com/tochemey/goakt-examples/v2/internal/remoting"
)

const (
	namespace         = "default"
	serviceName       = "blockchain"
	discoveryPortName = "discovery-port"
	peersPortName     = "peers-port"
	remotingPortName  = "remoting-port"
)

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Run the blockchain node with Kubernetes discovery",
	Run: func(cmd *cobra.Command, _ []string) {
		ctx := cmd.Context()

		logger := log.NewSlog(log.InfoLevel, os.Stdout)

		config, err := service.GetConfig()
		if err != nil {
			logger.Fatal(err)
		}

		// the pod labels the Kubernetes discovery provider filters on; they
		// must match the labels of the StatefulSet pods in deploy/k8s.yaml
		podLabels := map[string]string{
			"app.kubernetes.io/part-of":   "BlockChain",
			"app.kubernetes.io/component": config.ActorSystemName,
			"app.kubernetes.io/name":      serviceName,
		}

		discovery := kubernetes.NewDiscovery(&kubernetes.Config{
			Namespace:         namespace,
			DiscoveryPortName: discoveryPortName,
			RemotingPortName:  remotingPortName,
			PeersPortName:     peersPortName,
			PodLabels:         podLabels,
		})

		hostname, err := os.Hostname()
		if err != nil {
			logger.Fatal("failed to get the host name: ", err)
		}
		host := fmt.Sprintf("%s.%s.%s.svc.cluster.local", hostname, serviceName, namespace)

		// this pod's copy of the chain lives in a local Pebble store exposed
		// to the actors as a system extension
		store := persistence.NewPebbleStore(config.DataDir)
		if err := store.Start(ctx); err != nil {
			logger.Fatal(err)
		}

		clusterConfig := goakt.
			NewClusterConfig().
			WithDiscovery(discovery).
			WithPartitionCount(20).
			WithMinimumPeersQuorum(1).
			WithReplicaCount(1).
			WithDiscoveryPort(config.DiscoveryPort).
			WithPeersPort(config.PeersPort).
			WithClusterBalancerInterval(time.Second).
			// only kinds that may be spawned cluster-wide go here; broker,
			// miner and blockchain are local children of the Node singleton
			WithKinds(new(actors.Node))

		actorSystem, err := goakt.NewActorSystem(
			config.ActorSystemName,
			goakt.WithLogger(logger),
			goakt.WithActorInitMaxRetries(3),
			goakt.WithPubSub(),
			goakt.WithExtensions(store),
			goakt.WithRemote(remoting.NewConfig(host, config.RemotingPort,
				// the messages that may travel between pods, encoded with CBOR
				remote.WithSerializables(
					(*messages.SubmitTransaction)(nil),
					(*messages.TransactionSubmitted)(nil),
					(*messages.MineBlock)(nil),
					(*messages.CheckPowSolution)(nil),
					(*messages.ProofValidity)(nil),
					(*messages.ChainState)(nil),
					(*messages.GetChain)(nil),
					(*messages.GetPendingTransactions)(nil),
					(*messages.PendingTransactions)(nil),
					(*messages.BlockMinted)(nil),
					(*messages.GetBlocksFrom)(nil),
					(*messages.BlocksFrom)(nil),
					(*messages.SyncChain)(nil),
					(*messages.SyncDone)(nil),
					(*messages.GetLastHash)(nil),
					(*messages.LastHash)(nil),
					(*messages.GetLastIndex)(nil),
					(*messages.LastIndex)(nil),
					(*messages.GetLastBlock)(nil),
					(*messages.LastBlock)(nil),
				),
			)),
			goakt.WithCluster(clusterConfig),
		)
		if err != nil {
			logger.Fatal(err)
		}

		if err := actorSystem.Start(ctx); err != nil {
			logger.Fatal(err)
		}

		logger.Info("Actor system started with Kubernetes discovery")

		// every pod runs its own chain replica, named after the pod so the
		// name is cluster-unique. Its peers are the replicas of the other
		// pods of the StatefulSet. Relocation is disabled: a replica is bound
		// to its pod's Pebble volume, so when the pod dies unexpectedly it
		// must not be redeployed on another pod; the pod's replacement
		// respawns it from the local store instead.
		var peers []string
		for i := 0; i < config.Replicas; i++ {
			pod := fmt.Sprintf("%s-%d", serviceName, i)
			if pod != hostname {
				peers = append(peers, actors.ReplicaName(pod))
			}
		}

		if _, err := actorSystem.Spawn(ctx, actors.ReplicaName(hostname),
			actors.NewReplica(peers),
			goakt.WithLongLived(),
			goakt.WithRelocationDisabled()); err != nil {
			logger.Fatal(err)
		}

		// every pod tries to spawn the Node singleton on boot; only one wins
		// and the others share it through ActorOf
		if _, err := actorSystem.SpawnSingleton(ctx, actors.NodeName, &actors.Node{}); err != nil {
			if !errors.Is(err, gerrors.ErrActorAlreadyExists) {
				logger.Fatal(err)
			}
			logger.Infof("%s singleton already running elsewhere in the cluster", actors.NodeName)
		}

		blockchainService := service.NewBlockchainService(actorSystem, config.Port, logger)
		blockchainService.Start()

		sigs := make(chan os.Signal, 1)
		done := make(chan struct{}, 1)
		signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
		go func() {
			<-sigs

			logger.Info("Shutting down...")
			if err := actorSystem.Stop(ctx); err != nil {
				logger.Errorf("error stopping actor system: %v", err)
			}

			if err := store.Stop(); err != nil {
				logger.Errorf("error stopping the chain store: %v", err)
			}

			newCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
			defer cancel()
			if err := blockchainService.Stop(newCtx); err != nil {
				logger.Errorf("error stopping blockchain service: %v", err)
			}

			done <- struct{}{}
		}()
		<-done
		logger.Info("Shutdown complete")
	},
}

func init() {
	rootCmd.AddCommand(runCmd)
}
