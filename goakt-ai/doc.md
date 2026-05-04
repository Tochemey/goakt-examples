# goakt-ai: Distributed AI Agents Application

A production-like example implementing a **distributed AI agents** application using GoAkt actors. The example runs on **Kubernetes** using **Kind**, uses only **Go types** for actor messages (no protobuf), and demonstrates how multiple specialized AI agents can collaborate across cluster nodes to process user queries.

For implementation details and architecture patterns, see the sections below.

## What We Want to Achieve

### High-Level Goal

Build a **multi-agent AI system** where:

1. **Specialized agents** (research, summarization, tool-use, etc.) run as GoAkt actors
2. **Agents are distributed** across Kubernetes pods—any agent can live on any node
3. **Location transparency**—the orchestrator does not need to know where agents run; the cluster routes messages automatically
4. **Production-ready**—CLI interface, PostgreSQL persistence, OpenTelemetry tracing, Kind-based deployment

### Use Case: Collaborative Query Processing

A user submits a natural language query (e.g., *"Summarize the latest news about AI and calculate 15% of 1000"*). The system:

1. **Orchestrator Agent** receives the query and breaks it into sub-tasks
2. **Research Agent** (if needed) fetches or simulates external data (e.g., news)
3. **Summarizer Agent** condenses long content
4. **Tool Agent** executes computations (calculator, code, etc.)
5. **Orchestrator** aggregates results and returns a coherent response

All of this happens via **actor messaging** (`Tell`/`Ask`) across the cluster. Agents may run on different pods; GoAkt's cluster and remoting handle routing and serialization.

## Architecture Overview

### Component Diagram

```
  User (local machine): goakt-ai query "Summarize..."
                     │
                     │ HTTP (--endpoint or default)
                     ▼
┌────────────────────────────────────────────────────────────────┐
│                      Kubernetes Cluster                        │
│                                                                │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │  Nginx (NodePort) — Load balancer; round-robin to pods   │  │
│  └────────────────────────────┬─────────────────────────────┘  │
│                               │                                │
│             ┌─────────────────┼─────────────────┐              │
│             ▼                 ▼                 ▼              │
│  ┌───────────────┐ ┌───────────────┐ ┌───────────────┐         │
│  │  goakt-ai-0   │ │  goakt-ai-1   │ │  goakt-ai-2   │         │
│  │ (StatefulSet) │ │ (StatefulSet) │ │ (StatefulSet) │         │
│  │               │ │               │ │               │         │
│  │ goakt-ai run  │ │ goakt-ai run  │ │ goakt-ai run  │         │
│  │ + query HTTP  │ │ + query HTTP  │ │ + query HTTP  │         │
│  │   endpoint    │ │   endpoint    │ │   endpoint    │         │
│  │               │ │               │ │               │         │
│  │ Cluster kinds │ │ Cluster kinds │ │ Cluster kinds │         │
│  │ (distributed) │ │ (distributed) │ │ (distributed) │         │
│  └───────┬───────┘ └───────┬───────┘ └───────┬───────┘         │
│          │                 │                 │                 │
│          └─────────────────┼─────────────────┘                 │
│                            │ OTLP traces                       │
│               ┌────────────┴────────────┐                      │
│               ▼                         ▼                      │
│  ┌───────────────────┐  ┌───────────────────┐                  │
│  │  OTEL Collector   │  │    PostgreSQL     │                  │
│  │   (-> Jaeger)     │  │ (Task/Conv state) │                  │
│  └─────────┬─────────┘  └───────────────────┘                  │
│            │                                                   │
│            ▼                                                   │
│  ┌───────────────────┐                                         │
│  │      Jaeger       │                                         │
│  │    (Trace UI)     │                                         │
│  └───────────────────┘                                         │
└────────────────────────────────────────────────────────────────┘
```

- **User runs the CLI locally**—no kubectl. `goakt-ai query "..."` connects to the load balancer endpoint (e.g., `http://localhost:8080` after `make port-forward`).
- **Nginx** load balances requests across goakt-ai pods (round-robin).
- Each pod runs `goakt-ai run` and exposes a **minimal query endpoint** (HTTP) for the CLI—the only client. No public REST API.
- Actor instances are **distributed** across pods by the cluster (location transparent).

### Actor Types

| Actor                 | Role                                                                                                  | Lifecycle                                         |
|-----------------------|-------------------------------------------------------------------------------------------------------|---------------------------------------------------|
| **OrchestratorAgent** | Receives user queries, decomposes into sub-tasks, delegates to specialized agents, aggregates results | One per session/request (actor name = session ID) |
| **ResearchAgent**     | Performs research using LLM APIs (web search, data synthesis)                                         | One per research task or shared pool              |
| **SummarizerAgent**   | Summarizes long text or content                                                                       | One per summarization task or shared pool         |
| **ToolAgent**         | Executes tools (calculator, code execution, etc.)                                                     | One per tool invocation or shared pool            |

Agents are **cluster kinds**—GoAkt distributes them across nodes. The orchestrator uses `ActorOf` to obtain references and `Ask`/`Tell` for communication.

### Agent Relocation When Host Leaves the Cluster

All agents (Orchestrator, Research, Summarizer, Tool) are **relocatable by default**. When a node leaves the cluster:

1. **Graceful shutdown** (e.g. pod receives SIGTERM): GoAkt's relocation system persists the node's actor state to peers and recreates relocatable actors on remaining nodes. The cluster remains available.

2. **Abrupt failure** (e.g. OOMKill, node crash): Relocation cannot run because the node cannot persist state. Actors on that node are lost. The orchestrator's `getOrSpawnAgent` flow handles this: it first tries `ActorOf(kind)`; if the actor does not exist (e.g. its host crashed), it spawns a new instance. Subsequent requests therefore recover automatically.

Agents are spawned with `WithLongLived()` (passivation strategy) and **without** `WithRelocationDisabled()`, so they participate in relocation. The actor system does not use `WithoutRelocation()`.


## Message Flow (Go Types Only)

All actor messages are **Go structs** registered for remoting serialization. No protocol buffers.

### Orchestrator → Specialized Agents

- **`ProcessQuery`** — Orchestrator asks a specialized agent to process a sub-task
- **`QueryResult`** — Agent responds with the result

### User → CLI → Load Balancer → Pod → Orchestrator

- **`SubmitQuery`** — User runs CLI locally; CLI sends HTTP request to load balancer; Nginx forwards to a pod; pod spawns or finds Orchestrator and sends `SubmitQuery`
- **`QueryResponse`** — Orchestrator responds with aggregated result; pod returns HTTP response; CLI prints to stdout

### Internal Coordination

- **`DelegateTask`** — Orchestrator delegates a sub-task to Research/Summarizer/Tool agent
- **`TaskCompleted`** — Sub-agent returns result
- **`TaskFailed`** — Sub-agent returns error

### Persistence (Optional)

- **`SaveConversation`** — Persist conversation/session state to PostgreSQL
- **`LoadConversation`** — Load prior context for a session


## CLI Interface

The application is a **CLI tool**—no public REST API. Users run the CLI on their local machine; it connects to the cluster via the load balancer.

### Commands

| Command                          | Description                                                             |
|----------------------------------|-------------------------------------------------------------------------|
| `goakt-ai run`                   | Start the actor system (cluster node). Used in K8s pods.                |
| `goakt-ai query "your question"` | Submit a query; CLI connects to load balancer, prints result to stdout. |
| `goakt-ai chat`                  | Interactive chat mode (optional).                                       |

### Endpoint Configuration

The CLI connects to the cluster through the load balancer. Configure via:

- **`--endpoint URL`** — e.g., `--endpoint http://localhost:8080`
- **Environment**: `GOAKT_AI_ENDPOINT`
- **Config file**: `endpoint: http://localhost:8080`

After `make port-forward`, the default is `http://localhost:8080`. For direct NodePort access (no port-forward), use `http://<node-ip>:30080` (or the configured NodePort).

### API Key Configuration

Users provide their **real API keys** for LLM providers. Supported methods:

1. **Environment variables** (recommended for security):
   - `OPENAI_API_KEY`
   - `ANTHROPIC_API_KEY`
   - `GOOGLE_API_KEY` (for Gemini)
   - `MISTRAL_API_KEY`

2. **Config file** (`~/.goakt-ai/config.yaml` or `./goakt-ai.yaml`):
   ```yaml
   llm:
     provider: openai  # openai | anthropic | google | mistral
     openai_api_key: sk-...
     anthropic_api_key: sk-ant-...
     google_api_key: ...
     mistral_api_key: ...
   ```

3. **CLI flags** (overrides env/config):
   - `--openai-key`, `--anthropic-key`, `--google-key`, `--mistral-key`
   - `--provider openai|anthropic|google|mistral`

### Supported LLM Providers

| Provider      | Models                             | Env Variable        |
|---------------|------------------------------------|---------------------|
| **OpenAI**    | GPT-4o, GPT-4o-mini, GPT-3.5-turbo | `OPENAI_API_KEY`    |
| **Anthropic** | Claude 3.5 Sonnet, Claude 3 Haiku  | `ANTHROPIC_API_KEY` |
| **Google**    | Gemini 1.5 Pro, Gemini 1.5 Flash   | `GOOGLE_API_KEY`    |
| **Mistral**   | Mistral Large, Mistral Small       | `MISTRAL_API_KEY`   |

Users choose a provider via `--provider` or config; the selected provider's API key must be set.

**Cluster deployment**: When running in Kubernetes, API keys are injected into pods via Kubernetes Secrets (e.g., `llm-api-keys` secret mounted as env vars). Never commit API keys to the repo.


## Deployment (Kind)

### Prerequisites

- [Kind](https://kind.sigs.k8s.io/docs/user/quick-start/#installation)
- [kubectl](https://kubernetes.io/docs/tasks/tools/)
- [Earthly](https://earthly.dev/get-earthly) (for building the Docker image)

### Quick Start

```bash
# All commands run from goakt-ai/

# 1. Create the Kind cluster (one-time)
make cluster-create

# 2. Build the image, load it into Kind, and deploy all components
make deploy

# 3. Set your LLM API key (pick one provider)
make set-llm-keys PROVIDER=openai OPENAI_API_KEY=sk-your-key
# or: make set-llm-keys PROVIDER=google GOOGLE_API_KEY=your-key
# or: make set-llm-keys PROVIDER=anthropic ANTHROPIC_API_KEY=sk-ant-your-key
# or: make set-llm-keys PROVIDER=mistral MISTRAL_API_KEY=your-key

# 4. Expose the load balancer (run in a separate terminal)
make port-forward

# 5. Submit queries from your local machine
make query QUERY="Summarize the key benefits of distributed systems"
make query QUERY="What is 15% of 1000?"
```

Math queries (step 5) work without an API key. All other queries require step 3.

To tear everything down:

```bash
make cluster-down     # Remove deployments (keeps the Kind cluster)
make cluster-delete   # Delete the Kind cluster entirely
```

### Makefile Reference

| Target                     | Description                                                |
|----------------------------|------------------------------------------------------------|
| `make cluster-create`      | Create a Kind cluster (one-time)                           |
| `make deploy`              | Build image, load into Kind, deploy all components         |
| `make set-llm-keys`        | Set LLM API key in the cluster (see examples above)        |
| `make port-forward`        | Forward nginx to `http://localhost:8080`                    |
| `make query QUERY="..."`   | Submit a query to the cluster                              |
| `make status`              | Show cluster and pod status                                |
| `make logs`                | Tail logs from goakt-ai pods                               |
| `make image`               | Build Docker image and load into Kind (without deploying)  |
| `make cluster-up`          | Deploy components (without rebuilding the image)           |
| `make cluster-down`        | Remove all deployments                                     |
| `make cluster-recreate`    | Delete and recreate the Kind cluster                       |
| `make cluster-delete`      | Delete the Kind cluster                                    |
| `make port-forward-jaeger` | Forward Jaeger UI to `http://localhost:16686`               |
| `make test`                | Run CLI integration tests                                  |

## Design Decisions

### 1. Go Types Only

- All actor messages are plain Go structs (e.g., `ProcessQuery`, `QueryResult`)
- Registered with `remote.WithSerializables(...)` for CBOR serialization over the wire
- Aligns with goakt-saga and k8s-v2; no protobuf tooling required

### 2. Load Balancer + Kubernetes Discovery

- **Nginx** load balancer (NodePort) exposes the cluster to users; no kubectl required
- User runs CLI locally; `make port-forward` maps Nginx to localhost:8080
- Pods discover each other via the Kubernetes API (same pattern as goakt-saga, k8s-v2)
- StatefulSet with 3 replicas; each pod runs the full actor system and a minimal query HTTP endpoint for the CLI
- Cluster partitions actors across nodes; location transparent

### 3. Orchestrator Pattern

- One orchestrator per session/request (or long-lived per user)
- Orchestrator decides which agents to invoke and in what order
- Uses `Ask` for request-reply to specialized agents

### 4. AI Integration (Real LLM APIs)

- Agents call **real LLM APIs** using the user's API keys
- Supported providers: **OpenAI**, **Anthropic**, **Google (Gemini)**, **Mistral**
- API keys via environment variables, config file, or CLI flags
- User selects provider via `--provider` or config; no mock/simulated responses

### 5. Persistence

- PostgreSQL stores session state, conversation history, and task results
- Supports recovery and auditing
- Schema: `sessions`, `messages`, `tasks` (or similar)

### 6. Observability

- OpenTelemetry traces for CLI/actor spans
- Jaeger for trace visualization
- Logs for debugging

## Failure Handling

| Scenario                      | Behavior                                                               |
|-------------------------------|------------------------------------------------------------------------|
| Specialized agent timeout     | Orchestrator marks sub-task failed; may retry or return partial result |
| Agent unreachable (node down) | Cluster rebalances; new actor spawned on another node; retry possible  |
| LLM API failure               | Agent returns error; orchestrator can retry or fail the query          |
| Process crash mid-query       | Session state in DB; manual or automated recovery possible             |

## Project Structure (Planned)

```
goakt-ai/
├── actors/              # OrchestratorAgent, ResearchAgent, SummarizerAgent, ToolAgent
├── cmd/                 # CLI entry point (run, query, chat commands)
├── db/
│   └── migrations/      # SQL schema (sessions, messages, tasks)
├── domain/              # Query, Session, Task domain models
├── k8s/                 # Kubernetes manifests
│   ├── kind-config.yaml
│   ├── k8s.yaml         # StatefulSet, Service, RBAC
│   ├── nginx-*.yaml     # Load balancer (NodePort)
│   ├── postgres-*.yaml
│   ├── otel-collector-*.yaml
│   └── jaeger-*.yaml
├── llm/                 # LLM client abstractions (OpenAI, Anthropic, Google, Mistral)
├── messages/             # Go structs for actor messages
├── persistence/          # PostgreSQL store
├── scripts/              # test-cli.sh
├── doc.md                # This file
└── Makefile
```

## Differences from Other Examples

| Feature       | goakt-saga                      | goakt-ai                                 |
|---------------|---------------------------------|------------------------------------------|
| Domain        | Money transfer                  | AI query processing                      |
| Orchestration | Saga (compensation)             | Multi-agent delegation                   |
| Actors        | AccountEntity, SagaOrchestrator | Orchestrator, Research, Summarizer, Tool |
| External deps | None                            | Real LLM APIs (user API keys)            |
| Message flow  | Sequential steps                | DAG of agent delegations                 |

| Feature      | k8s-v2           | goakt-ai                    |
|--------------|------------------|-----------------------------|
| Domain       | Account CRUD     | AI agents                   |
| Actors       | AccountEntity    | Multiple specialized agents |
| Coordination | None (stateless) | Orchestrator coordinates    |

## Summary

**goakt-ai** demonstrates a production-like **distributed AI agents** application:

- **CLI interface**—users run `goakt-ai query "..."` locally; connects via load balancer (no kubectl)
- **Multiple specialized agents** (Orchestrator, Research, Summarizer, Tool) as GoAkt actors
- **Kubernetes + Kind** for deployment; **Go types only** for messages
- **Location transparency**—agents can run on any cluster node
- **Real LLM integration**—OpenAI, Anthropic, Google (Gemini), Mistral with user-provided API keys
- **API key support**—environment variables, config file, or CLI flags
- PostgreSQL persistence, OpenTelemetry tracing

The implementation will follow the patterns established in goakt-saga and goakt-actors-cluster/k8s-v2, adapted for the multi-agent AI domain.

## License

MIT License - see repository root for details.
