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
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/cobra"
	goakt "github.com/tochemey/goakt/v4/actor"
	"github.com/tochemey/goakt/v4/discovery/dnssd"
	"github.com/tochemey/goakt/v4/log"
	"github.com/tochemey/goakt/v4/remote"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"

	"github.com/tochemey/goakt-examples/v2/goakt-actors-cluster/dnssd-v2/actors"
	"github.com/tochemey/goakt-examples/v2/goakt-actors-cluster/dnssd-v2/messages"
	"github.com/tochemey/goakt-examples/v2/goakt-actors-cluster/dnssd-v2/persistence"
	"github.com/tochemey/goakt-examples/v2/goakt-actors-cluster/dnssd-v2/service"
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

func initMeter(res *resource.Resource, logger log.Logger) *metric.MeterProvider {
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
	logger.Info("Prometheus server running on :9092")
	return meterProvider
}

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Run the account service with DNSSD discovery",
	Run: func(cmd *cobra.Command, args []string) {
		ctx := context.Background()

		// use the address default log. real-life implement the log interface`
		logger := log.NewSlog(log.DebugLevel, os.Stdout)

		config, err := service.GetConfig()
		if err != nil {
			logger.Fatal(err)
			os.Exit(1)
		}

		res, err := resource.New(ctx,
			resource.WithHost(),
			resource.WithProcess(),
			resource.WithTelemetrySDK(),
			resource.WithAttributes(
				semconv.ServiceNameKey.String("accounts-selfmanaged"),
			),
		)
		if err != nil {
			logger.Fatal(err)
			os.Exit(1)
		}

		_ = initTracer(ctx, res, config.TraceURL)
		_ = initMeter(res, logger)

		host, _ := os.Hostname()

		discoConfig := &dnssd.Config{
			DomainName: config.DomainName,
		}
		disco := dnssd.NewDiscovery(discoConfig)

		persistenceStore := persistence.NewPostgresStore(&persistence.PostgresConfig{
			DBHost:     config.DBHost,
			DBPort:     config.DBPort,
			DBName:     config.DBName,
			DBUser:     config.DBUser,
			DBPassword: config.DBPassword,
		})

		if err := persistenceStore.Start(ctx); err != nil {
			logger.Fatal(err)
			os.Exit(1)
		}

		cbor := remote.NewCBORSerializer()
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
			WithKinds(new(actors.AccountEntity))

		actorSystem, err := goakt.NewActorSystem(
			config.ActorSystemName,
			goakt.WithLogger(logger),
			goakt.WithExtensions(persistenceStore),
			goakt.WithActorInitMaxRetries(3),
			goakt.WithRemote(remote.NewConfig(host, config.RemotingPort,
				remote.WithSerializers((*messages.CreateAccount)(nil), cbor),
				remote.WithSerializers((*messages.CreditAccount)(nil), cbor),
				remote.WithSerializers((*messages.GetAccount)(nil), cbor),
				remote.WithSerializers((*messages.Account)(nil), cbor),
			)),
			goakt.WithCluster(clusterConfig),
		)
		if err != nil {
			logger.Fatal(err)
			os.Exit(1)
		}

		if err := actorSystem.Start(ctx); err != nil {
			logger.Fatal(err)
			os.Exit(1)
		}

		logger.Info("Actor system started with DNSSD discovery")
		logger.Infof("Domain name: %s", config.DomainName)

		accountService := service.NewAccountService(actorSystem, config.Port, logger)
		accountService.Start()

		sigs := make(chan os.Signal, 1)
		done := make(chan struct{}, 1)
		signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
		go func() {
			<-sigs

			logger.Info("Shutting down...")
			if err := actorSystem.Stop(ctx); err != nil {
				logger.Errorf("error stopping actor system: %v", err)
			}

			if err := persistenceStore.Stop(); err != nil {
				logger.Errorf("error stopping persistence: %v", err)
			}

			newCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
			defer cancel()
			if err := accountService.Stop(newCtx); err != nil {
				logger.Errorf("error stopping account service: %v", err)
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
