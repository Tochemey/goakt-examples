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
	"fmt"
	nethttp "net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	goakt "github.com/tochemey/goakt/v4/actor"
	"github.com/tochemey/goakt/v4/discovery/kubernetes"
	"github.com/tochemey/goakt/v4/log"
	"github.com/tochemey/goakt/v4/remote"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"

	"github.com/tochemey/goakt-examples/v2/goakt-2pc/actors"
	"github.com/tochemey/goakt-examples/v2/goakt-2pc/messages"
	"github.com/tochemey/goakt-examples/v2/goakt-2pc/persistence"
	"github.com/tochemey/goakt-examples/v2/goakt-2pc/service"
)

type otelErrorHandler struct{ logger log.Logger }

func (h otelErrorHandler) Handle(err error) { h.logger.Errorf("otel SDK error: %v", err) }

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

const (
	namespace         = "default"
	serviceName       = "two-pc-transfer"
	actorSystemName   = "two-pc-TransferSystem"
	discoveryPortName = "discovery-port"
	peersPortName     = "peers-port"
	remotingPortName  = "remoting-port"
)

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
		svcName = "two-pc-transfer"
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
	logger.Info("OpenTelemetry tracing initialized")
	return tp
}

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Run the 2PC transfer service with Kubernetes discovery",
	Run: func(cmd *cobra.Command, args []string) {
		ctx := cmd.Context()

		logger := log.NewSlog(log.DebugLevel, os.Stdout)

		config, err := service.GetConfig()
		if err != nil {
			logger.Fatal(err)
		}

		podLabels := map[string]string{
			"app.kubernetes.io/part-of":   "two-pc",
			"app.kubernetes.io/component": actorSystemName,
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

		tp := initTracer(ctx, logger)
		if tp != nil {
			defer func() {
				_ = tp.Shutdown(context.Background())
			}()
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
			WithKinds(new(actors.AccountEntity), new(actors.Coordinator))

		actorSystem, err := goakt.NewActorSystem(
			config.ActorSystemName,
			goakt.WithLogger(logger),
			goakt.WithExtensions(persistenceStore),
			goakt.WithActorInitMaxRetries(3),
			goakt.WithRemote(remote.NewConfig(host, config.RemotingPort,
				remote.WithContextPropagator(otelRemoteContextPropagator{
					propagator: otel.GetTextMapPropagator(),
				}),
				remote.WithSerializables(
					(*messages.CreateAccount)(nil),
					(*messages.DebitAccount)(nil),
					(*messages.CreditAccount)(nil),
					(*messages.GetAccount)(nil),
					(*messages.Account)(nil),
					(*messages.StartTransfer)(nil),
					(*messages.TransferCompleted)(nil),
					(*messages.TransferFailed)(nil),
					(*messages.GetTransferStatus)(nil),
					(*messages.TransferStatus)(nil),
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

		logger.Info("Actor system started with Kubernetes discovery and 2PC pattern")

		transferService := service.NewTransferService(actorSystem, config.Port, logger, tp)
		transferService.Start()

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
			if err := transferService.Stop(newCtx); err != nil {
				logger.Errorf("error stopping service: %v", err)
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
