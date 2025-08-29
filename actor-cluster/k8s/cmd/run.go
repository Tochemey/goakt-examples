/*
 * MIT License
 *
 * Copyright (c) 2022-2025 Arsene Tochemey Gandote
 *
 * Permission is hereby granted, free of charge, to any person obtaining a copy
 * of this software and associated documentation files (the "Software"), to deal
 * in the Software without restriction, including without limitation the rights
 * to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
 * copies of the Software, and to permit persons to whom the Software is
 * furnished to do so, subject to the following conditions:
 *
 * The above copyright notice and this permission notice shall be included in all
 * copies or substantial portions of the Software.
 *
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
 * IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
 * FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
 * AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
 * LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
 * OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
 * SOFTWARE.
 */

package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/caarlos0/env/v11"
	"github.com/spf13/cobra"
	goakt "github.com/tochemey/goakt/v3/actor"
	"github.com/tochemey/goakt/v3/discovery/kubernetes"
	"github.com/tochemey/goakt/v3/log"
	"github.com/tochemey/goakt/v3/remote"

	"github.com/tochemey/goakt-examples/v2/actor-cluster/k8s/actors"
	"github.com/tochemey/goakt-examples/v2/actor-cluster/k8s/service"
)

const (
	namespace         = "default"
	serviceName       = "accounts"
	actorSystemName   = "AccountsSystem"
	discoveryPortName = "discovery-port"
	peersPortName     = "peers-port"
	remotingPortName  = "remoting-port"
)

type config struct {
	GossipPort   int `env:"DISCOVERY_PORT"`
	PeersPort    int `env:"PEERS_PORT"`
	RemotingPort int `env:"REMOTING_PORT"`
	Port         int `env:"PORT" envDefault:"50051"`
}

func getConfig(logger log.Logger) *config {
	// load the host node configuration
	cfg := &config{}
	opts := env.Options{RequiredIfNoDef: true, UseFieldNameByDefault: false}
	if err := env.ParseWithOptions(cfg, opts); err != nil {
		logger.Fatal(err)
	}
	return cfg
}

// runCmd represents the run command
var runCmd = &cobra.Command{
	Use:   "run",
	Short: "A brief description of your command",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		ctx := cmd.Context()

		logger := log.New(log.DebugLevel, os.Stdout)

		podLabels := map[string]string{
			"app.kubernetes.io/part-of":   "Sample",
			"app.kubernetes.io/component": actorSystemName,
			"app.kubernetes.io/name":      serviceName,
		}

		// instantiate the k8 discovery provider
		discovery := kubernetes.NewDiscovery(&kubernetes.Config{
			Namespace:         namespace,
			DiscoveryPortName: discoveryPortName,
			RemotingPortName:  remotingPortName,
			PeersPortName:     peersPortName,
			PodLabels:         podLabels,
		})

		// get the port config
		config := getConfig(logger)

		// grab the host
		hostname, err := os.Hostname()
		if err != nil {
			logger.Fatal("failed to get the host name: ", err)
		}
		host := fmt.Sprintf("%s.%s.%s.svc.cluster.local", hostname, serviceName, namespace)

		clusterConfig := goakt.
			NewClusterConfig().
			WithDiscovery(discovery).
			WithPartitionCount(20).
			WithMinimumPeersQuorum(1).
			WithReplicaCount(1).
			WithDiscoveryPort(config.GossipPort).
			WithPeersPort(config.PeersPort).
			WithKinds(new(actors.AccountEntity))

		// create the actor system
		actorSystem, err := goakt.NewActorSystem(
			actorSystemName,
			goakt.WithLogger(logger),
			goakt.WithActorInitMaxRetries(3),
			goakt.WithRemote(remote.NewConfig(host, config.RemotingPort)),
			goakt.WithCluster(clusterConfig))

		// handle the error
		if err != nil {
			logger.Fatal(err)
		}

		// start the actor system
		if err := actorSystem.Start(ctx); err != nil {
			logger.Fatal(err)
		}

		remoting := remote.NewRemoting()

		// create the account service
		accountService := service.NewAccountService(actorSystem, remoting, logger, config.Port)
		// start the account service
		accountService.Start()

		// wait for interruption/termination
		sigs := make(chan os.Signal, 1)
		done := make(chan struct{}, 1)
		signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
		// wait for a shutdown signal, and then shutdown
		go func() {
			<-sigs
			remoting.Close()

			// stop the actor system
			if err := actorSystem.Stop(ctx); err != nil {
				logger.Fatal(err)
			}

			// stop the account service
			newCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
			defer cancel()
			if err := accountService.Stop(newCtx); err != nil {
				logger.Fatal(err)
			}

			done <- struct{}{}
		}()
		<-done
	},
}

func init() {
	rootCmd.AddCommand(runCmd)
}
