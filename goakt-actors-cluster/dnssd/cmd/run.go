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
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/cobra"
	goakt "github.com/tochemey/goakt/v3/actor"
	"github.com/tochemey/goakt/v3/discovery/dnssd"
	"github.com/tochemey/goakt/v3/log"
	"github.com/tochemey/goakt/v3/remote"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"

	"github.com/tochemey/goakt-examples/v2/goakt-actors-cluster/dnssd/actors"
	"github.com/tochemey/goakt-examples/v2/goakt-actors-cluster/dnssd/persistence"
	"github.com/tochemey/goakt-examples/v2/goakt-actors-cluster/dnssd/service"
)

func initTracer(ctx context.Context, res *resource.Resource, traceURL string) *sdktrace.TracerProvider {
	exporter, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithInsecure(),
		otlptracegrpc.WithEndpoint(traceURL),
	)
	if err != nil {
		panic(err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithSpanProcessor(sdktrace.NewBatchSpanProcessor(exporter)),
		sdktrace.WithResource(res),
	)

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))
	return tp
}

func initMeter(res *resource.Resource) *metric.MeterProvider {
	// The exporter embeds a default OpenTelemetry Reader and
	// implements prometheus.Collector, allowing it to be used as
	// both a Reader and Collector.
	metricExporter, err := prometheus.New()
	if err != nil {
		panic(err)
	}
	meterProvider := metric.NewMeterProvider(
		metric.WithReader(metricExporter),
		metric.WithResource(res),
	)
	otel.SetMeterProvider(meterProvider)

	http.Handle("/metrics", promhttp.Handler())
	go func() {
		_ = http.ListenAndServe(":9092", nil)
	}()
	fmt.Println("Prometheus server running on :9092")
	return meterProvider
}

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "A brief description of your command",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		// create a background context
		ctx := context.Background()
		// use the address default log. real-life implement the log interface`
		logger := log.New(log.DebugLevel, os.Stdout)

		// get the configuration from the env vars
		config, err := service.GetConfig()
		//  handle the error
		if err != nil {
			logger.Fatal(err)
		}

		res, err := resource.New(ctx,
			resource.WithHost(),
			resource.WithProcess(),
			resource.WithTelemetrySDK(),
			resource.WithAttributes(
				semconv.ServiceNameKey.String("accounts"),
			),
		)
		if err != nil {
			logger.Fatal(err)
		}

		// initialize traces and metric providers
		_ = initTracer(ctx, res, config.TraceURL)
		// define the discovery options
		discoConfig := dnssd.Config{
			DomainName: config.DomainName,
		}
		// instantiate the dnssd discovery provider
		disco := dnssd.NewDiscovery(&discoConfig)

		persistenceStore := persistence.NewPostgresStore(&persistence.PostgresConfig{
			DBHost:     config.DBHost,
			DBPort:     config.DBPort,
			DBName:     config.DBName,
			DBUser:     config.DBUser,
			DBPassword: config.DBPassword,
		})

		// Start the persistence storage
		// This is to demonstrate a proper workflow
		if err := persistenceStore.Start(ctx); err != nil {
			logger.Fatal(err)
		}

		// grab the host
		host, _ := os.Hostname()

		clusterConfig := goakt.
			NewClusterConfig().
			WithDiscovery(disco).
			WithPartitionCount(20).
			WithMinimumPeersQuorum(1).
			WithReplicaCount(1).
			WithDiscoveryPort(config.GossipPort).
			WithPeersPort(config.PeersPort).
			WithClusterBalancerInterval(time.Second).
			WithClusterStateSyncInterval(3 * time.Second).
			WithKinds(new(actors.AccountEntity))

		// create the actor system
		actorSystem, err := goakt.NewActorSystem(
			config.ActorSystemName,
			goakt.WithLogger(logger),
			goakt.WithExtensions(persistenceStore),
			goakt.WithActorInitMaxRetries(3),
			goakt.WithRemote(remote.NewConfig(host, config.RemotingPort)),
			goakt.WithCluster(clusterConfig))

		// handle the error
		if err != nil {
			logger.Panic(err)
		}

		// start the actor system
		if err := actorSystem.Start(ctx); err != nil {
			logger.Panic(err)
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

			// close this after the system
			if err := persistenceStore.Stop(); err != nil {
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
