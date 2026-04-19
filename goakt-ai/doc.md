# goakt-ai — Distributed AI Agents on GoAkt + Google ADK

A production-shaped example that runs a multi-agent AI system on
[GoAkt](https://github.com/tochemey/goakt) with
[Google's Agent Development Kit for Go](https://pkg.go.dev/google.golang.org/adk).
GoAkt handles clustering, remoting, grains, routers, streams, eventstream,
and supervision; ADK handles LLM tool calling, agent trees, and session
memory. Deploys to Kubernetes via Kind.

## What it does

A user submits a natural-language query over HTTP. The system routes the
query to a session-scoped grain, runs one turn through an ADK agent tree
(orchestrator + research / summarizer / tool sub-agents), and returns
either a JSON response or a Server-Sent Events token stream.

Two request shapes are exposed:

- `POST /query` — blocking JSON, one turn, returns aggregated text.
- `GET /query/stream` — SSE, one turn, streams partial LLM tokens as
  they arrive.
- `GET /health` — liveness probe.

Sessions are identified by `session_id` (generated if the caller does
not supply one). Conversation history is stored by ADK's session
service; providing the same `session_id` on subsequent turns continues
the conversation.

## Architecture

```
      HTTP request                       Cluster (any node)
           │                     ┌───────────────────────────────────┐
           │                     │                                   │
           ▼                     ▼                                   │
   ┌──────────────┐      ┌────────────────────┐                      │
   │ QueryService │──────▶ ConversationGrain  │  (per SessionID,     │
   │  /query      │      │  single-writer     │   auto-passivates    │
   │  /stream     │      │  per cluster)      │   after idle TTL)    │
   └──────┬───────┘      └──────────┬─────────┘                      │
          │                         │                                │
          │                         │ runs                           │
          │                         ▼                                │
          │               ┌────────────────────┐                     │
          │               │   ADK Runner       │                     │
          │               │   root LlmAgent    │                     │
          │               │   ├─ research      │                     │
          │               │   ├─ summarizer    │                     │
          │               │   └─ tool          │                     │
          │               └────────────────────┘                     │
          │                                                          │
          ▼                                                          │
   ┌──────────────┐                                                  │
   │ stream.From  │     goakt/v4/stream pipeline                     │
   │   Channel →  │     (producer goroutine feeds an SSE sink)       │
   │   ForEach    │                                                  │
   └──────────────┘                                                  │
                                                                     │
   ┌─ Cluster-kind actors ──────────────────────────────────────┐    │
   │                                                            │    │
   │  AgentActor (kind) × 3  — one per Role; PipeTo(Self)       │    │
   │    RoleResearch / RoleSummarizer / RoleTool                │    │
   │    Idle → Thinking (Stash) → Idle state machine            │    │
   │    Serves legacy *messages.ProcessQuery callers            │    │
   │                                                            │    │
   │  ToolExecutor router (pool size 4, round-robin)            │    │
   │    Serves *messages.ExecuteTool for parallel tool dispatch │    │
   └────────────────────────────────────────────────────────────┘    │
                                                                     │
   eventstream ◀─ telemetry subscribers (turn-finished, llm-error,   │
                  tool-called, grain-passivated) + actor deadletter  │
                                                                     │
   ADKExtension (model.LLM + session.Service + eventstream)          │
   registered once via goakt.WithExtensions at bootstrap ────────────┘
```

### Dual routing paths

Only Gemini (via adk-go's native `gemini.NewModel`) supports function
calling. OpenAI / Anthropic / Mistral are adapted as a `model.LLM` by
`actors/llm_adapter.go`; that adapter strips tool declarations, so the
root LlmAgent cannot delegate to its sub-agent tools on those providers.

The grain resolves this at request time:

- `LLM_PROVIDER=google` → run the **root** runner; the LLM picks which
  sub-agent to invoke via ADK tool calls.
- Anything else → run the **role** runner picked by `routeByKeyword`
  (the same regex-style rules the legacy orchestrator used: `%-of`,
  `+ - * /`, `summar`, default research).

Both paths share the same `session.Service`, so conversation history
does not depend on which route a turn took.

## Package layout

| Path                           | Responsibility                                                                         |
|--------------------------------|----------------------------------------------------------------------------------------|
| `actors/`                      | ADK extension, agent factories, tools, grain, cluster kinds, router, telemetry         |
| `actors/agents.go`             | `buildRootAgent`, `buildSingleRoleAgent` — ADK LlmAgent wiring                         |
| `actors/agent_actor.go`        | `AgentActor` cluster-kind with Behaviors + PipeTo state machine                        |
| `actors/conversation_grain.go` | `ConversationGrain` — per-session single-writer virtual actor                          |
| `actors/constants.go`          | Canonical string constants (user IDs, agent names, tool names, operators)              |
| `actors/llm_adapter.go`        | Adapter wrapping `llm.Client` as `model.LLM` for non-Gemini providers                  |
| `actors/tools.go`              | `arithmetic` and `percent_of` tools registered with the Tool sub-agent                 |
| `actors/tool_executor.go`      | Router routee for legacy `ExecuteTool` messages                                        |
| `actors/routing.go`            | `routeByKeyword` + prompt-prefix helpers for the non-Gemini path                       |
| `actors/telemetry.go`          | Eventstream topics, `StartTelemetryLogger`, `StartDeadLetterLogger`                    |
| `actors/adk_extension.go`      | `ADKExtension` — the shared ADK runtime handle                                         |
| `cmd/`                         | Cobra CLI, cluster bootstrap, supervisor wiring                                        |
| `llm/`                         | Legacy LLM clients (OpenAI, Anthropic, Google, Mistral) used by the adapter            |
| `messages/`                    | Plain-Go message types (`SubmitQuery`, `ProcessQuery`, `ExecuteTool`, `StreamToken` …) |
| `service/`                     | HTTP handlers; `service/stream.go` hosts the SSE endpoint                              |
| `k8s/`                         | Kubernetes manifests (StatefulSet, Nginx, Jaeger, OTEL collector, Postgres)            |

## GoAkt capabilities used

| Capability                    | Where                                                                                          |
|-------------------------------|------------------------------------------------------------------------------------------------|
| Plain actors                  | `AgentActor`, `ToolExecutor` routees                                                           |
| Cluster kinds                 | `AgentActor` registered via `WithKinds`; spawned once per role at bootstrap                    |
| Grains (virtual actors)       | `ConversationGrain` keyed by SessionID; auto-passivates after 10 min idle                      |
| Remoting (CBOR)               | `remote.WithSerializables(...)` for every message type that may cross a node                   |
| Behaviors + Stash             | `AgentActor` Idle → Thinking state machine; stash buffers concurrent ProcessQueries            |
| Routers                       | `SpawnRouter` pool (4 × `ToolExecutor`, round-robin) for parallel tool fan-out                 |
| Supervision                   | `supervisor.NewSupervisor(WithRetry(3, 2s), WithAnyErrorDirective(RestartDirective))`          |
| Scheduled passivation         | `WithGrainDeactivateAfter(10 * time.Minute)`                                                   |
| Reactive streams              | `goakt/v4/stream.FromChannel → ForEach` powers the SSE producer pipeline                       |
| Eventstream pubsub            | Telemetry topics (`ai.turn.finished`, `ai.llm.error`, `ai.tool.called`, `ai.grain.passivated`) |
| Deadletter + lifecycle events | `ActorSystem.Subscribe()` wired to `StartDeadLetterLogger`                                     |
| Extensions                    | `ADKExtension` registered via `goakt.WithExtensions`, read by every actor and grain            |
| Kubernetes discovery          | `goakt/v4/discovery/kubernetes` + headless service for FQDN peer addressing                    |
| OTel remoting propagator      | Custom `otelRemoteContextPropagator` injects/extracts trace context into actor RPCs            |

## ADK surface used

| Symbol                         | Purpose                                                                                      |
|--------------------------------|----------------------------------------------------------------------------------------------|
| `agent.Agent` / `llmagent.New` | LlmAgent with `Instruction`, `SubAgents`, `Tools`                                            |
| `agenttool.New(...)`           | Lets the root orchestrator call each sub-agent as a tool                                     |
| `functiontool.New`             | Registers `arithmetic` and `percent_of` tools with the tool sub-agent                        |
| `runner.New` / `Runner.Run`    | One runner per role + one root runner, reused across turns                                   |
| `session.InMemoryService`      | In-memory conversation history. Swap to `session/database` to persist on Postgres            |
| `model.LLM`                    | Provider abstraction — implemented directly for Gemini, adapted from `llm.Client` for others |
| `genai.NewContentFromText`     | Builds the user-role `genai.Content` the runner expects                                      |

## Message flow

1. **`POST /query`** arrives at any pod's `QueryService`.
2. `QueryService.handleQuery` resolves a `ConversationGrain` identity
   keyed by `session_id` via `ActorSystem.GrainIdentity(...)`. GoAkt
   places the grain on exactly one cluster node.
3. `AskGrain(identity, SubmitQuery, 60s)` delivers the query to the
   grain's single-writer mailbox.
4. `ConversationGrain.handleSubmit` picks the appropriate runner
   (root for Gemini, role runner otherwise) and calls
   `runner.Run(ctx, userID, sessionID, genaiContent, RunConfig{...})`.
5. The ADK runner drives one turn, emits events, appends to the
   session, and returns. The grain collects the final text, publishes
   a telemetry event on the eventstream, and `Response`s the caller.
6. `QueryService` marshals the response as JSON.

`GET /query/stream` follows the same path but:

- Builds a per-request runner (cheap; reuses the shared model and
  session service).
- Runs the producer in a goroutine that feeds a channel,
  `select`ing on `r.Context().Done()` so a client disconnect cleans
  up the goroutine and the ADK turn.
- Wires the channel to a `stream.FromChannel[StreamToken] → ForEach`
  pipeline that writes SSE frames.

## HTTP reference

| Method | Path            | Body / Query                                | Response                                         |
|--------|-----------------|---------------------------------------------|--------------------------------------------------|
| POST   | `/query`        | JSON: `{"query": "...", "session_id": "…"}` | JSON: `{"session_id", "result" | "error"}`       |
| GET    | `/query/stream` | `?q=<query>&session_id=<id>`                | `text/event-stream` of `StreamToken` JSON frames |
| GET    | `/health`       | —                                           | `200 OK` / `ok`                                  |

## CLI

The example ships a Cobra CLI (`main.go` → `cmd/`) with two commands:

- `goakt-ai run` — start a cluster node. Reads env vars (ports, DB
  config, OTel endpoint, LLM provider + key).
- `goakt-ai query "…"` — submit a query to the load balancer
  endpoint. Prints the aggregated text response.

## Configuration (env vars)

| Variable                      | Default                      | Notes                                            |
|-------------------------------|------------------------------|--------------------------------------------------|
| `PORT`                        | `50051`                      | HTTP port for query service                      |
| `REMOTING_PORT`               | `50052`                      | Actor-to-actor RPC                               |
| `DISCOVERY_PORT`              | `3322`                       | Kubernetes discovery                             |
| `PEERS_PORT`                  | `3320`                       | Cluster peer communication                       |
| `SYSTEM_NAME`                 | `goakt-ai`                   | Actor system name                                |
| `LLM_PROVIDER`                | `openai`                     | `openai` \| `anthropic` \| `google` \| `mistral` |
| `LLM_MODEL`                   | provider default             | e.g. `gpt-4o-mini`, `gemini-2.5-flash-lite`      |
| `OPENAI_API_KEY` etc.         | —                            | Matched to `LLM_PROVIDER`                        |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | `http://otel-collector:4318` | OTLP HTTP endpoint                               |
| `OTEL_SERVICE_NAME`           | `goakt-ai`                   | Service name reported to tracers                 |

Only Gemini (`LLM_PROVIDER=google`) exercises the full ADK tool-calling
path. Other providers work single-turn through the keyword fallback.

## Deployment (Kind)

### Prerequisites

- [Kind](https://kind.sigs.k8s.io/docs/user/quick-start/#installation)
- [kubectl](https://kubernetes.io/docs/tasks/tools/)
- [Earthly](https://earthly.dev/get-earthly) (for the Docker image build)

### Quick start

```bash
# From goakt-ai/

make cluster-create                                     # once
make deploy                                             # image + manifests
make set-llm-keys PROVIDER=google GOOGLE_API_KEY=...    # or openai / anthropic / mistral
make port-forward                                       # in another terminal
make query QUERY="Summarize the benefits of distributed systems"
make query QUERY="What is 15% of 1000?"
```

Teardown:

```bash
make cluster-down      # remove deployments, keep the Kind cluster
make cluster-delete    # remove the Kind cluster entirely
```

### Makefile reference

| Target                      | Description                                        |
|-----------------------------|----------------------------------------------------|
| `make cluster-create`       | Create a Kind cluster (one-time)                   |
| `make deploy`               | Build image, load into Kind, deploy all components |
| `make set-llm-keys`         | Set an LLM API key secret in the cluster           |
| `make port-forward`         | Forward nginx to `http://localhost:8080`           |
| `make query QUERY="..."`    | Submit a query via the CLI                         |
| `make status` / `make logs` | Cluster inspection                                 |
| `make image`                | Build + load the image only                        |
| `make cluster-up`           | Deploy (no image rebuild)                          |
| `make cluster-down`         | Remove deployments                                 |
| `make cluster-recreate`     | Recreate the Kind cluster                          |
| `make cluster-delete`       | Delete the Kind cluster                            |
| `make port-forward-jaeger`  | Forward Jaeger UI to `http://localhost:16686`      |
| `make test`                 | Run CLI integration tests                          |

### Cluster topology (per pod)

- 1 × Go binary running the actor system + query service.
- `ADKExtension` registered at startup (model + session service +
  eventstream).
- 3 × `AgentActor` spawned by role, supervised with retry-backoff.
- 1 × `ToolExecutor` router with a 4-routee pool.
- 1 × Postgres init-container wait (Postgres itself deployed separately;
  swap `session.InMemoryService()` for `session/database` to persist
  conversation history there).

Supporting infrastructure in `k8s/`:

- `nginx-*` — round-robin load balancer exposed as a NodePort.
- `jaeger-*` and `otel-collector-*` — distributed tracing.
- `postgres-deployment.yaml` — optional state store.

## Observability

- OpenTelemetry traces on all HTTP handlers and actor remoting
  (custom `otelRemoteContextPropagator`).
- Application eventstream topics — see `actors/telemetry.go`.
  `StartTelemetryLogger` logs every event at Info.
- Actor system events (deadletters, actor stops, restarts) surfaced
  via `ActorSystem.Subscribe()` and `StartDeadLetterLogger`.

## Extending the example

- **Persist sessions on Postgres.** Replace
  `adksession.InMemoryService()` in `cmd/run.go` with
  `database.NewSessionService(postgres.Open(dsn))`. Run
  `database.AutoMigrate(svc)` once on startup.
- **Add a new role.** Append a constant to the `Role` enum, extend
  `buildSingleRoleAgent`, and include the new role in the
  `cmd/run.go` spawn loop. The grain picks it up automatically if
  you update `routeByKeyword` (non-Gemini) or the orchestrator
  description (Gemini).
- **Add a tool.** Define `<Name>Args` / `<Name>Result` types and a
  `functiontool.New` wrapper in `actors/tools.go`. Append it to
  `builtinTools()`; it becomes available to the Tool sub-agent on
  next startup.
- **Stream via the router.** The Tool router pool currently serves
  only legacy `ExecuteTool` messages. To fan out tool calls from ADK
  in parallel, add a `BeforeToolCallback` on the ADK LlmAgent that
  dispatches through `ToolExecutorRouter` instead of running in
  process.

## License

MIT — see the `LICENSE` file at the repository root.
