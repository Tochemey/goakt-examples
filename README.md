# GoAkt Examples

[![GitHub go.mod Go version](https://badges.chse.dev/github/go-mod/go-version/Tochemey/goakt-examples)](https://go.dev/doc/install)

Examples for [GoAkt](https://github.com/Tochemey/goakt) v4. For the stable v3.14 examples, see the [v3 branch](https://github.com/Tochemey/goakt-examples/tree/release/v3.14).

## Getting Started

```bash
git clone https://github.com/Tochemey/goakt-examples
cd goakt-examples
```

**Build all examples** (requires [Earthly](https://earthly.dev/get-earthly)):

```bash
earthly +all
```

## Examples Overview

### Core Concepts

- [goakt-actor-hello-world](./goakt-actor-hello-world) — Minimal actor system: spawn an actor, send messages, graceful shutdown
- [goakt-ping-pong](./goakt-ping-pong) — Actor-to-actor messaging: two actors exchanging messages (Tell pattern)
- [goakt-actor-behaviors](./goakt-actor-behaviors) — Stateful behaviors: actor with multiple states (authenticated, logged-in) and state transitions

### Remoting & Location Transparency

- [goakt-remoting](./goakt-remoting) — Actor remoting: Ping and Pong actors on separate processes, communicating over the network
- [goakt-actors-cluster/dynalloc](./goakt-actors-cluster/dynalloc) — Location transparency: actors can live on any node; cluster routes messages automatically

### Clustering & Discovery

- [goakt-actors-cluster/static](./goakt-actors-cluster/static) — Static peer discovery: cluster nodes configured via fixed addresses
- [goakt-actors-cluster/dnssd](./goakt-actors-cluster/dnssd) — DNS-SD discovery: nodes discover each other via mDNS/DNS (protobuf messages)
- [goakt-actors-cluster/dnssd-v2](./goakt-actors-cluster/dnssd-v2) — DNS-SD + Go types: same as dnssd but with standard Go structs and PostgreSQL persistence
- [goakt-actors-cluster/k8s](./goakt-actors-cluster/k8s) — Kubernetes discovery: cluster on K8s using the API to discover pods (gRPC, protobuf)
- [goakt-actors-cluster/k8s-v2](./goakt-actors-cluster/k8s-v2) — **Production-ready K8s cluster**: Go types, HTTP/JSON API, PostgreSQL persistence, OpenTelemetry tracing
- [goakt-actors-cluster/k8s-ebpf](./goakt-actors-cluster/k8s-ebpf) — **k8s-v2 + eBPF**: zero-instrumentation actor-level tracing via goakt-ebpf sidecar

### Persistence & Extensions

- [goakt-actor-persistence](./goakt-actor-persistence) — Persistence extension: actor state snapshots to a pluggable store (in-memory example)

### Grains (Virtual Actors)

- [goakt-grains](./goakt-grains) — Grains model: virtual actors with automatic activation and passivation
- [goakt-grains-cluster/grains-dnssd](./goakt-grains-cluster/grains-dnssd) — Grains clustering: grains across multiple nodes with DNS-SD discovery

### Applications

- [goakt-chat](./goakt-chat) — Multi-room chat: remoting, room-based messaging, message history (protobuf)
- **[goakt-chat-v2](./goakt-chat-v2) — Chat with Go types: same chat app using standard Go structs instead of protobuf.

## Kubernetes Cluster (k8s-v2)

The **k8s-v2** example is the most comprehensive cluster setup. It demonstrates a production-style GoAkt actor cluster on Kubernetes with:

- **Standard Go types** for actor messages (no protocol buffers)
- **PostgreSQL persistence** for actor state
- **HTTP/JSON REST API** with Swagger UI
- **OpenTelemetry tracing** (HTTP spans + custom actor spans → Jaeger)
- **Kind** (Kubernetes in Docker) for local development

### Architecture

```
                    ┌─────────────────┐
                    │ Nginx (NodePort)│
                    │ Load Balancer   │
                    └────────┬────────┘
                             │
         ┌───────────────────┼───────────────────┐
         │                   │                   │
         ▼                   ▼                   ▼
┌────────────────┐  ┌────────────────┐  ┌────────────────┐
│ accounts-0     │  │ accounts-1     │  │ accounts-2     │
│ (StatefulSet)  │  │ (StatefulSet)  │  │ (StatefulSet)  │
│ Actor + HTTP   │  │ Actor + HTTP   │  │ Actor + HTTP   │
└───────┬────────┘  └───────┬────────┘  └───────┬────────┘
        │                   │                   │
        │ OTLP traces       │ OTLP traces       │ OTLP traces
        └───────────────────┼───────────────────┘
                            │
         ┌──────────────────┼──────────────────┐
         │                  │                  │
         ▼                  ▼                  ▼
┌────────────────┐  ┌──────────────────┐
│ OTEL Collector │  │    PostgreSQL    │
│ (OTLP → Jaeger)│  │   (Persistence)  │
└───────┬────────┘  └──────────────────┘
        │
        ▼
┌────────────────┐
│     Jaeger     │
│ (Trace UI)     │
└────────────────┘
```

### Quick Start

```bash
cd goakt-actors-cluster/k8s-v2
make cluster-create    # Create Kind cluster (one-time)
make deploy            # Build, load image, deploy all components
make port-forward      # Expose API at http://localhost:8080
```

**Prerequisites:** Kind, kubectl, Earthly, Docker. See [k8s-v2/doc.md](./goakt-actors-cluster/k8s-v2/doc.md) for installation.

### Testing the API

With `make port-forward` running:

- **API base:** http://localhost:8080
- **Swagger UI:** http://localhost:8080/docs
- **Jaeger traces:** `make port-forward-jaeger` → http://localhost:16686

```bash
# Create an account
curl -X POST http://localhost:8080/accounts \
  -H "Content-Type: application/json" \
  -d '{"create_account":{"account_id":"acc-001","account_balance":100.00}}'

# Run integration tests (1000 accounts)
make test
```

### Key Make Targets

- **`make deploy`** — Build image, load into Kind, deploy all components
- **`make cluster-create`** — Create Kind cluster
- **`make cluster-delete`** — Delete Kind cluster
- **`make port-forward`** — Forward API to localhost:8080
- **`make port-forward-jaeger`** — Forward Jaeger UI to localhost:16686
- **`make test`** — Run API integration tests
- **`make logs`** — Tail accounts pod logs

For full documentation, troubleshooting, and configuration, see **[goakt-actors-cluster/k8s-v2/doc.md](./goakt-actors-cluster/k8s-v2/doc.md)**.

## Kubernetes Cluster with eBPF (k8s-ebpf)

The **k8s-ebpf** example extends k8s-v2 with **goakt-ebpf** as a sidecar for zero-instrumentation eBPF tracing. Each pod runs the accounts app plus an eBPF agent that captures actor-level spans (`doReceive`, `process`, remote messaging) via uprobes.

- **goakt-ebpf sidecar** in each pod for automatic actor-level tracing
- **Shared PID namespace** so the eBPF agent can attach to the accounts process
- **Standard Go types**, PostgreSQL persistence, HTTP/JSON API (same as k8s-v2)

### Architecture

```
                    ┌─────────────────┐
                    │ Nginx (NodePort)│
                    │ Load Balancer   │
                    └────────┬────────┘
                             │
         ┌───────────────────┼───────────────────┐
         │                   │                   │
         ▼                   ▼                   ▼
┌────────────────┐  ┌────────────────┐  ┌────────────────┐
│ accounts-0     │  │ accounts-1     │  │ accounts-2     │
│ ┌────────────┐ │  │ ┌────────────┐ │  │ ┌────────────┐ │
│ │  accounts  │ │  │ │  accounts  │ │  │ │  accounts  │ │
│ │ (GoAkt app)│ │  │ │ (GoAkt app)│ │  │ │ (GoAkt app)│ │
│ └─────┬──────┘ │  │ └─────┬──────┘ │  │ └─────┬──────┘ │
│       │ uprobe │  │       │ uprobe │  │       │ uprobe │
│ ┌─────┴──────┐ │  │ ┌─────┴──────┐ │  │ ┌─────┴──────┐ │
│ │ goakt-ebpf │ │  │ │ goakt-ebpf │ │  │ │ goakt-ebpf │ │
│ │ (sidecar)  │ │  │ │ (sidecar)  │ │  │ │ (sidecar)  │ │
│ └────────────┘ │  │ └────────────┘ │  │ └────────────┘ │
└───────┬────────┘  └───────┬────────┘  └───────┬────────┘
        │                   │                   │
        │ OTLP traces       │ OTLP traces       │ OTLP traces
        └───────────────────┼───────────────────┘
                            │
         ┌──────────────────┼──────────────────┐
         │                  │                  │
         ▼                  ▼                  ▼
┌────────────────┐  ┌──────────────────┐
│ OTEL Collector │  │    PostgreSQL     │
│ (OTLP → Jaeger)│  │   (Persistence)   │
└───────┬────────┘  └──────────────────┘
        │
        ▼
┌────────────────┐
│     Jaeger     │
│ (Trace UI)     │
└────────────────┘
```

### Quick Start

**Prerequisites:** Kind, kubectl, Earthly, Docker, and the sibling **goakt-ebpf** repository.

```bash
cd goakt-actors-cluster/k8s-ebpf
make cluster-create    # Create Kind cluster (one-time)
make deploy           # Build accounts + goakt-ebpf images, load, deploy
make port-forward     # Expose API at http://localhost:8080
```

### Testing the API

With `make port-forward` running:

- **API base:** http://localhost:8080
- **Swagger UI:** http://localhost:8080/docs
- **Jaeger traces:** `make port-forward-jaeger` → http://localhost:16686 (select `goakt-ebpf` for actor spans, `accounts` for HTTP spans)

```bash
# Create an account
curl -X POST http://localhost:8080/accounts \
  -H "Content-Type: application/json" \
  -d '{"create_account":{"account_id":"acc-001","account_balance":100.00}}'

# Run integration tests
make test
```

### Key Make Targets

- **`make deploy`** — Build both images (accounts + goakt-ebpf), load into Kind, deploy all components
- **`make cluster-create`** — Create Kind cluster
- **`make cluster-delete`** — Delete Kind cluster
- **`make port-forward`** — Forward API to localhost:8080
- **`make port-forward-jaeger`** — Forward Jaeger UI to localhost:16686
- **`make logs-ebpf`** — Tail goakt-ebpf sidecar logs
- **`make test`** — Run API integration tests

For full documentation, prerequisites, and troubleshooting, see **[goakt-actors-cluster/k8s-ebpf/doc.md](./goakt-actors-cluster/k8s-ebpf/doc.md)**.

## Quick Reference

**Single-process** — `go run .` or run the built binary (hello-world, ping-pong, actor-behaviors, remoting, actor-persistence, grains)

**Docker Compose** — `docker-compose up` (static, dnssd, dynalloc, grains-dnssd)

**Kubernetes (Kind)** — `make cluster-create && make deploy` (k8s, k8s-v2, k8s-ebpf)

**API & discovery by example:**

- **gRPC + protobuf** — dynalloc, static, dnssd, k8s, grains-dnssd, chat
- **HTTP/JSON + Go types** — dnssd-v2, k8s-v2, k8s-ebpf
- **PostgreSQL persistence** — dnssd-v2, k8s-v2, k8s-ebpf
- **eBPF actor tracing** — k8s-ebpf

See the `doc.md` in each example directory for detailed run instructions.
