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
	nethttp "net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	goakt "github.com/tochemey/goakt/v4/actor"
	"github.com/tochemey/goakt/v4/datacenter"
	natscp "github.com/tochemey/goakt/v4/datacenter/controlplane/nats"
	natsdisco "github.com/tochemey/goakt/v4/discovery/nats"
	gerrors "github.com/tochemey/goakt/v4/errors"
	"github.com/tochemey/goakt/v4/log"
	"github.com/tochemey/goakt/v4/remote"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"

	"github.com/tochemey/goakt-examples/v2/goakt-cluster/multi-dc/actors"
	"github.com/tochemey/goakt-examples/v2/goakt-cluster/multi-dc/messages"
	"github.com/tochemey/goakt-examples/v2/goakt-cluster/multi-dc/persistence"
	"github.com/tochemey/goakt-examples/v2/goakt-cluster/multi-dc/service"
)

// otelErrorHandler surfaces OTEL SDK export errors in the application logs.
type otelErrorHandler struct{ logger log.Logger }

func (h otelErrorHandler) Handle(err error) { h.logger.Errorf("otel SDK error: %v", err) }

// otelRemoteContextPropagator adapts OTEL TextMap propagation to GoAkt remoting.
type otelRemoteContextPropagator struct {
	propagator propagation.TextMapPropagator
}

func (p otelRemoteContextPropagator) Inject(ctx context.Context, headers nethttp.Header) error {
	p.propagator.Inject(ctx, propagation.HeaderCarrier(headers))
	return nil
}

func (p otelRemoteContextPropagator) Extract(ctx context.Context, headers nethttp.Header) (context.Context, error) {
	return p.propagator.Extract(ctx, propagation.HeaderCarrier(headers)), nil
}

func getLogLevel(level string) log.Level {
	switch level {
	case "debug":
		return log.DebugLevel
	case "info":
		return log.InfoLevel
	case "warn":
		return log.WarningLevel
	case "error":
		return log.ErrorLevel
	default:
		return log.InfoLevel
	}
}

const (
	actorSystemName = "AccountsSystem"
)

// initTracer sets up the standard OTEL SDK TracerProvider for HTTP and custom actor spans.
func initTracer(ctx context.Context, logger log.Logger) *sdktrace.TracerProvider {
	otel.SetErrorHandler(otelErrorHandler{logger: logger})

	endpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	if endpoint == "" {
		endpoint = "http://otel-collector:4318"
	}

	exporter, err := otlptracehttp.New(ctx, otlptracehttp.WithEndpointURL(endpoint))
	if err != nil {
		logger.Warnf("failed to create OTLP trace exporter: %v (tracing disabled)", err)
		return nil
	}

	svcName := os.Getenv("OTEL_SERVICE_NAME")
	if svcName == "" {
		svcName = "accounts"
	}

	res, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName(svcName),
		),
	)
	if err != nil {
		logger.Warnf("failed to create resource: %v", err)
		res = resource.Default()
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithSpanProcessor(sdktrace.NewSimpleSpanProcessor(exporter)),
		sdktrace.WithResource(res),
	)

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))
	logger.Info("OpenTelemetry tracing initialized (HTTP + custom actor spans)")
	return tp
}

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Run the account service with NATS discovery and multi-DC support",
	Run: func(cmd *cobra.Command, args []string) {
		ctx := cmd.Context()

		config, err := service.GetConfig()
		if err != nil {
			log.NewSlog(log.ErrorLevel, os.Stdout).Fatal(err)
		}

		logger := log.NewSlog(getLogLevel(config.LogLevel), os.Stdout)

		hostname, err := os.Hostname()
		if err != nil {
			logger.Fatal("failed to get the host name: ", err)
		}

		// For StatefulSet pods: <hostname>.<service>.<namespace>.svc.cluster.local
		// The headless service name matches the DC name (e.g. accounts-dc1)
		svcName := fmt.Sprintf("accounts-%s", strings.ReplaceAll(config.DCName, "-", ""))
		host := fmt.Sprintf("%s.%s.default.svc.cluster.local", hostname, svcName)

		// NATS discovery for intra-DC peer finding.
		// Each DC uses its own NATS subject so peers only discover nodes in the same DC.
		discovery := natsdisco.NewDiscovery(&natsdisco.Config{
			NatsServer:    config.NatsURL,
			NatsSubject:   fmt.Sprintf("goakt.discovery.%s", config.DCName),
			Host:          host,
			DiscoveryPort: config.DiscoveryPort,
			Timeout:       10 * time.Second,
		})

		// NATS JetStream control plane for cross-DC coordination.
		controlPlaneConfig := &natscp.Config{
			URL:    config.NatsURL,
			Bucket: "goakt_datacenters",
			TTL:    30 * time.Second,
		}
		controlPlaneConfig.Sanitize()
		controlPlane, err := natscp.NewControlPlane(controlPlaneConfig)
		if err != nil {
			logger.Fatal("failed to create NATS control plane: ", err)
		}

		// Datacenter config
		datacenterConfig := datacenter.NewConfig()
		datacenterConfig.ControlPlane = controlPlane
		datacenterConfig.FailOnStaleCache = false
		datacenterConfig.CacheRefreshInterval = 3 * time.Second
		datacenterConfig.MaxCacheStaleness = 15 * time.Second
		datacenterConfig.DataCenter = datacenter.DataCenter{
			Name:   config.DCName,
			Region: config.DCRegion,
			Zone:   config.DCZone,
		}

		// Initialize persistence store
		persistenceStore := persistence.NewPostgresStore(&persistence.PostgresConfig{
			DBHost:     config.DBHost,
			DBPort:     config.DBPort,
			DBName:     config.DBName,
			DBUser:     config.DBUser,
			DBPassword: config.DBPassword,
		})

		if err := persistenceStore.Start(ctx); err != nil {
			logger.Fatal(err)
		}

		clusterConfig := goakt.
			NewClusterConfig().
			WithDiscovery(discovery).
			WithDataCenter(datacenterConfig).
			WithPartitionCount(20).
			WithMinimumPeersQuorum(1).
			WithReplicaCount(1).
			WithDiscoveryPort(config.DiscoveryPort).
			WithPeersPort(config.PeersPort).
			WithClusterBalancerInterval(time.Second).
			WithKinds(new(actors.AccountEntity), new(actors.DataCenterGateway))

		actorSystem, err := goakt.NewActorSystem(
			config.ActorSystemName,
			goakt.WithLogger(logger),
			goakt.WithExtensions(persistenceStore),
			goakt.WithActorInitMaxRetries(3),
			goakt.WithRemote(remote.NewConfig(host, config.RemotingPort,
				remote.WithContextPropagator(otelRemoteContextPropagator{
					propagator: propagation.NewCompositeTextMapPropagator(
						propagation.TraceContext{},
						propagation.Baggage{},
					),
				}),
				remote.WithSerializables(
					(*messages.CreateAccount)(nil),
					(*messages.CreditAccount)(nil),
					(*messages.GetAccount)(nil),
					(*messages.Account)(nil),
					(*messages.ForwardGetAccount)(nil),
					(*messages.ForwardCreditAccount)(nil),
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

		logger.Infof("Actor system started with NATS discovery and multi-DC support (dc=%s, region=%s, zone=%s)",
			config.DCName, config.DCRegion, config.DCZone)

		tp := initTracer(ctx, logger)
		if tp != nil {
			defer func() {
				_ = tp.Shutdown(context.Background())
			}()
		}

		// Spawn the DC gateway as a cluster singleton on the leader/oldest node.
		// The gateway uses SendSync (which calls DiscoverActor) to find actors across DCs.
		// Non-leader nodes reach it via ActorOf("dc-gateway") which returns a remote PID.
		if _, err = actorSystem.SpawnSingleton(ctx, "dc-gateway", new(actors.DataCenterGateway),
			goakt.WithSingletonSpawnTimeout(30*time.Second),
			goakt.WithSingletonSpawnRetries(10),
		); err != nil && !errors.Is(err, gerrors.ErrSingletonAlreadyExists) {
			logger.Fatal("failed to spawn dc-gateway singleton: ", err)
		}

		accountService := service.NewAccountService(actorSystem, config.Port, config.DCName, logger, tp)
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
