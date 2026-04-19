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
	"syscall"
	"time"

	"github.com/spf13/cobra"
	goakt "github.com/tochemey/goakt/v4/actor"
	"github.com/tochemey/goakt/v4/discovery/kubernetes"
	goakterrors "github.com/tochemey/goakt/v4/errors"
	"github.com/tochemey/goakt/v4/eventstream"
	"github.com/tochemey/goakt/v4/log"
	"github.com/tochemey/goakt/v4/remote"
	"github.com/tochemey/goakt/v4/supervisor"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
	adksession "google.golang.org/adk/session"

	"github.com/tochemey/goakt-examples/v2/goakt-ai/actors"
	"github.com/tochemey/goakt-examples/v2/goakt-ai/agents"
	"github.com/tochemey/goakt-examples/v2/goakt-ai/llm"
	"github.com/tochemey/goakt-examples/v2/goakt-ai/messages"
	"github.com/tochemey/goakt-examples/v2/goakt-ai/service"
	"github.com/tochemey/goakt-examples/v2/goakt-ai/telemetry"
)

type otelErrorHandler struct{ logger log.Logger }

func (handler otelErrorHandler) Handle(err error) { handler.logger.Errorf("otel SDK error: %v", err) }

type otelRemoteContextPropagator struct {
	propagator propagation.TextMapPropagator
}

func (remotePropagator otelRemoteContextPropagator) Inject(ctx context.Context, headers nethttp.Header) error {
	remotePropagator.propagator.Inject(ctx, propagation.HeaderCarrier(headers))
	return nil
}

func (remotePropagator otelRemoteContextPropagator) Extract(ctx context.Context, headers nethttp.Header) (context.Context, error) {
	return remotePropagator.propagator.Extract(ctx, propagation.HeaderCarrier(headers)), nil
}

const (
	namespace         = "default"
	serviceName       = "goakt-ai"
	actorSystemName   = "GoAktAI"
	discoveryPortName = "discovery-port"
	peersPortName     = "peers-port"
	remotingPortName  = "remoting-port"

	toolPoolSize          = 4
	supervisorMaxRestarts = 3
)

func initTracer(ctx context.Context, logger log.Logger) *sdktrace.TracerProvider {
	otel.SetErrorHandler(otelErrorHandler{logger: logger})

	endpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	if endpoint == "" {
		endpoint = "http://otel-collector:4318"
	}

	traceExporter, err := otlptracehttp.New(ctx, otlptracehttp.WithEndpointURL(endpoint))
	if err != nil {
		logger.Warnf("failed to create OTLP trace exporter: %v (tracing disabled)", err)
		return nil
	}

	otelServiceName := os.Getenv("OTEL_SERVICE_NAME")
	if otelServiceName == "" {
		otelServiceName = "goakt-ai"
	}

	traceResource, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName(otelServiceName),
		),
	)
	if err != nil {
		logger.Warnf("failed to create resource: %v", err)
		traceResource = resource.Default()
	}

	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithSpanProcessor(sdktrace.NewSimpleSpanProcessor(traceExporter)),
		sdktrace.WithResource(traceResource),
	)

	otel.SetTracerProvider(tracerProvider)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))
	logger.Info("OpenTelemetry tracing initialized")

	return tracerProvider
}

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Run the goakt-ai cluster node with Kubernetes discovery",
	Run: func(cmd *cobra.Command, args []string) {
		ctx := cmd.Context()

		logger := log.NewSlog(log.DebugLevel, os.Stdout)

		serviceConfig, err := service.GetConfig()
		if err != nil {
			logger.Fatal(err)
		}

		podLabels := map[string]string{
			"app.kubernetes.io/part-of":   "GoAktAI",
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
			logger.Fatal("failed to get hostname: ", err)
		}
		clusterHost := fmt.Sprintf("%s.%s.%s.svc.cluster.local", hostname, serviceName, namespace)

		tracerProvider := initTracer(ctx, logger)
		if tracerProvider != nil {
			defer func() {
				_ = tracerProvider.Shutdown(context.Background())
			}()
		}

		// Shared ADK runtime. Session state stays in-memory for this
		// example; swap to session/database.NewSessionService to persist
		// conversation history on the already-deployed Postgres.
		llmConfig := llm.ConfigFromEnv()
		eventStream := eventstream.New()

		adkExtension, err := agents.NewADKExtension(ctx, serviceName, llmConfig, adksession.InMemoryService(), eventStream)
		if err != nil {
			logger.Fatalf("build ADK extension: %v", err)
		}

		// Telemetry: subscribe to the shared eventstream and log every
		// emitted event. Unsubscribed on shutdown.
		telemetrySubscriber := telemetry.StartTelemetryLogger(eventStream, logger)
		defer eventStream.RemoveSubscriber(telemetrySubscriber)

		clusterConfig := goakt.
			NewClusterConfig().
			WithDiscovery(discovery).
			WithPartitionCount(20).
			WithMinimumPeersQuorum(1).
			WithReplicaCount(1).
			WithDiscoveryPort(serviceConfig.DiscoveryPort).
			WithPeersPort(serviceConfig.PeersPort).
			WithClusterBalancerInterval(time.Second).
			// AgentActor is the single cluster kind that replaces the
			// three legacy per-role actors. Three instances (one per role)
			// are spawned explicitly below so the cluster has one actor
			// name per role, matching the legacy naming contract.
			WithKinds(actors.NewAgentActor(agents.RoleResearch))

		actorSystem, err := goakt.NewActorSystem(
			serviceConfig.ActorSystemName,
			goakt.WithLogger(logger),
			goakt.WithExtensions(adkExtension),
			goakt.WithActorInitMaxRetries(3),
			goakt.WithRemote(remote.NewConfig(clusterHost, serviceConfig.RemotingPort,
				remote.WithContextPropagator(otelRemoteContextPropagator{
					propagator: otel.GetTextMapPropagator(),
				}),
				remote.WithSerializables(
					(*messages.SubmitQuery)(nil),
					(*messages.QueryResponse)(nil),
					(*messages.ProcessQuery)(nil),
					(*messages.QueryResult)(nil),
					(*messages.ExecuteTool)(nil),
					(*messages.ToolResult)(nil),
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

		// DeadLetter + lifecycle subscription. ActorSystem.Subscribe
		// returns a subscriber bound to the internal events topic which
		// emits Deadletter, ActorStopped and related signals; we forward
		// them to logs for operator visibility.
		systemSubscriber, err := actorSystem.Subscribe()
		if err != nil {
			logger.Fatalf("subscribe to system events: %v", err)
		}
		defer func() {
			_ = actorSystem.Unsubscribe(systemSubscriber)
		}()
		telemetry.StartDeadLetterLogger(systemSubscriber, logger)

		// Register the ConversationGrain kind. GrainIdentity activations
		// pick this factory when the HTTP handler asks for a session-bound
		// grain, and the 10-minute deactivation TTL is applied per-call in
		// service.handleQuery via WithGrainDeactivateAfter.
		if err := actorSystem.RegisterGrainKind(ctx, &actors.ConversationGrain{}); err != nil {
			logger.Fatalf("register conversation grain: %v", err)
		}

		// Spawn one AgentActor per role with an explicit supervisor:
		// three restart attempts with a 2s cooldown cover transient LLM
		// errors while still letting a persistently broken actor surface.
		agentSupervisor := supervisor.NewSupervisor(
			supervisor.WithStrategy(supervisor.OneForOneStrategy),
			supervisor.WithRetry(supervisorMaxRestarts, 2*time.Second),
			supervisor.WithAnyErrorDirective(supervisor.RestartDirective),
		)

		// Role actors and the tool router are cluster-wide singletons by
		// name: one pod wins the spawn, later pods see ErrActorAlreadyExists
		// and route to the existing instances via cluster Ask.
		roles := []agents.Role{agents.RoleResearch, agents.RoleSummarizer, agents.RoleTool}
		for _, role := range roles {
			_, err := actorSystem.Spawn(
				ctx,
				string(role),
				actors.NewAgentActor(role),
				goakt.WithLongLived(),
				goakt.WithSupervisor(agentSupervisor),
			)
			switch {
			case err == nil:
				logger.Infof("spawned %s on this node", role)
			case errors.Is(err, goakterrors.ErrActorAlreadyExists):
				logger.Infof("%s already spawned elsewhere in the cluster; will route via Ask", role)
			default:
				logger.Fatalf("spawn %s: %v", role, err)
			}
		}

		// Router pool for parallel tool fan-out. Round-robin keeps the
		// strategy predictable; if a routee errors the supervisor restarts
		// it with up to three retries.
		_, err = actorSystem.SpawnRouter(
			ctx,
			actors.ToolExecutorRouter,
			toolPoolSize,
			actors.NewToolExecutor(),
			goakt.WithRoutingStrategy(goakt.RoundRobinRouting),
			goakt.WithRestartRouteeOnFailure(supervisorMaxRestarts, 2*time.Second),
		)
		switch {
		case err == nil:
			logger.Info("tool executor router spawned on this node")
		case errors.Is(err, goakterrors.ErrActorAlreadyExists):
			logger.Info("tool executor router already spawned elsewhere in the cluster")
		default:
			logger.Fatalf("spawn tool executor router: %v", err)
		}

		queryService := service.NewQueryService(actorSystem, serviceConfig.Port, hostname, logger, tracerProvider)
		if err := queryService.Start(ctx); err != nil {
			logger.Fatal(err)
		}

		signals := make(chan os.Signal, 1)
		done := make(chan struct{}, 1)
		signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)

		go func() {
			<-signals

			logger.Info("Shutting down...")
			if err := actorSystem.Stop(ctx); err != nil {
				logger.Errorf("error stopping actor system: %v", err)
			}

			shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			if err := queryService.Stop(shutdownCtx); err != nil {
				logger.Errorf("error stopping query service: %v", err)
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
