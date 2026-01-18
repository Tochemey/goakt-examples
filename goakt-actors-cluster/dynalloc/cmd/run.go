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
	"os"
	"time"

	"github.com/spf13/cobra"
	goakt "github.com/tochemey/goakt/v3/actor"
	"github.com/tochemey/goakt/v3/discovery/static"
	"github.com/tochemey/goakt/v3/log"
	"github.com/tochemey/goakt/v3/remote"

	"github.com/tochemey/goakt-examples/v2/goakt-actors-cluster/dynalloc/actors"
	"github.com/tochemey/goakt-examples/v2/goakt-actors-cluster/dynalloc/service"
)

func getLogLevel(level string) log.Level {
	var logLevel log.Level
	switch level {
	case "debug":
		logLevel = log.DebugLevel
	case "info":
		logLevel = log.InfoLevel
	case "warn":
		logLevel = log.WarningLevel
	case "error":
		logLevel = log.ErrorLevel
	default:
		logLevel = log.InfoLevel
	}
	return logLevel
}

// runCmd represents the run command
var runCmd = &cobra.Command{
	Use:   "run",
	Short: "A brief description of your command",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		// create a background context
		ctx := context.Background()

		// get the configuration from the env vars
		config, err := service.GetConfig()
		//  handle the error
		if err != nil {
			panic(err)
		}

		// use the address default log. real-life implement the log interface`
		logger := log.New(getLogLevel(config.LogLevel), os.Stdout)

		// define the discovery options
		hosts := config.DiscoveryHosts
		discoConfig := static.Config{Hosts: hosts}
		// instantiate the dnssd discovery provider
		disco := static.NewDiscovery(&discoConfig)

		// grab the host
		host, _ := os.Hostname()

		clusterConfig := goakt.
			NewClusterConfig().
			WithDiscovery(disco).
			WithPartitionCount(20).
			WithBootstrapTimeout(10 * time.Second).
			WithReadTimeout(3 * time.Second).
			WithWriteTimeout(3 * time.Second).
			WithDiscoveryPort(config.DiscoveryPort).
			WithPeersPort(config.PeersPort).
			WithClusterBalancerInterval(time.Second).
			WithClusterStateSyncInterval(3 * time.Second).
			WithKinds(new(actors.Account))

		// create the actor system
		actorSystem, err := goakt.NewActorSystem(
			config.ActorSystemName,
			goakt.WithLogger(logger),
			goakt.WithActorInitMaxRetries(3),
			goakt.WithRemote(remote.NewConfig(host, config.RemotingPort)),
			goakt.WithCluster(clusterConfig))

		// handle the error
		if err != nil {
			logger.Panic(err)
		}

		remoting := remote.NewRemoting()
		// create the account service
		accountService := service.NewAccountService(actorSystem, remoting, logger, config.Port)

		actorSystem.Run(ctx,
			func(ctx context.Context) error {
				// start the account service
				accountService.Start()
				return nil
			},
			func(ctx context.Context) error {
				remoting.Close()
				newCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
				defer cancel()
				return accountService.Stop(newCtx)
			})
	},
}

func init() {
	rootCmd.AddCommand(runCmd)
}
