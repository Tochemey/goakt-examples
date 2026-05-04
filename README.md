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

- [goakt-hello-world](./goakt-hello-world) — Minimal actor system: spawn an actor, send messages, graceful shutdown
- [goakt-ping-pong](./goakt-ping-pong) — Actor-to-actor messaging: two actors exchanging messages (Tell pattern)
- [goakt-behaviors](./goakt-behaviors) — Stateful behaviors: actor with multiple states (authenticated, logged-in) and state transitions
- [goakt-parent-child](./goakt-parent-child) — Parent/child hierarchy: spawn children from inside an actor, observe `Terminated`
- [goakt-supervision](./goakt-supervision) — Supervision directives: `Stop`, `Resume`, `Restart` side-by-side with retry budgets
- [goakt-routers](./goakt-routers) — Actor pools/routers: round-robin, fan-out, consistent-hash; runtime pool resize
- [goakt-pubsub](./goakt-pubsub) — Topic-based pub/sub via `WithPubSub()` and the system `TopicActor`
- [goakt-scheduler](./goakt-scheduler) — Scheduling messages: `ScheduleOnce`, recurring `Schedule`, cancellation by reference
- [goakt-dead-letters](./goakt-dead-letters) — Subscribing to the system event stream to observe unhandled messages

### Remoting & Location Transparency

- [goakt-remoting](./goakt-remoting) — Actor remoting: Ping and Pong actors on separate processes, communicating over the network
- [goakt-cluster/dynalloc](goakt-cluster/dynalloc) — Location transparency: actors can live on any node; cluster routes messages automatically

### Clustering & Discovery

- [goakt-cluster/static](goakt-cluster/static) — Static peer discovery: cluster nodes configured via fixed addresses
- [goakt-cluster/dnssd](goakt-cluster/dnssd) — DNS-SD discovery: nodes discover each other via mDNS/DNS (protobuf messages)
- [goakt-cluster/dnssd-v2](goakt-cluster/dnssd-v2) — DNS-SD + Go types: same as dnssd but with standard Go structs and PostgreSQL persistence
- [goakt-cluster/k8s](goakt-cluster/k8s) — Kubernetes discovery: cluster on K8s using the API to discover pods (gRPC, protobuf)
- [goakt-luster/k8s-v2](goakt-cluster/k8s-v2) — **Production-ready K8s cluster**: Go types, HTTP/JSON API, PostgreSQL persistence, OpenTelemetry tracing
- [goakt-cluster/k8s-ebpf](goakt-cluster/k8s-ebpf) — **k8s-v2 + eBPF**: zero-instrumentation actor-level tracing via goakt-ebpf sidecar
- [goakt-cluster/multi-dc](goakt-cluster/multi-dc) — **Multi-datacenter**: two DCs (us-east-1, eu-west-1) with NATS JetStream control plane, cross-DC actor placement via SpawnOn + WithDataCenter, NATS discovery
- [goakt-cluster/multi-dc-isolated](goakt-cluster/multi-dc-isolated) — **Multi-datacenter (network isolation)**: same as multi-dc but with two separate Kind clusters on a shared Docker network, simulating real network boundaries between DCs

### Persistence & Extensions

- [goakt-persistence](./goakt-persistence) — Persistence extension: actor state snapshots to a pluggable store (in-memory example)

### Grains (Virtual Actors)

- [goakt-grains](./goakt-grains) — Grains model: virtual actors with automatic activation and passivation
- [goakt-iot-twin](./goakt-iot-twin) — Device twins as grains: on-demand activation per device, idle passivation, state restored on reactivation
- [goakt-grains-cluster/grains-dnssd](./goakt-grains-cluster/grains-dnssd) — Grains clustering: grains across multiple nodes with DNS-SD discovery

### Reactive Streams

- [goakt-stream](./goakt-stream) — **Reactive streams**: backpressure-aware pipelines with Sources, Flows, Sinks, fan-out/fan-in topologies, parallel processing, and actor integration

### Applications

- [goakt-chat](./goakt-chat) — Multi-room chat: remoting, room-based messaging, message history (protobuf)
- [goakt-chat-v2](./goakt-chat-v2) — Chat with Go types: same chat app using standard Go structs instead of protobuf
- [goakt-saga](./goakt-saga) — **Saga pattern**: production-like money transfer with compensating transactions, Kubernetes/Kind, Go types only
- [goakt-2pc](./goakt-2pc) — **2 phase commit pattern**: The same production-like money transfer with 2 phase commit pattern, Kubernetes/Kind, Go types only
- [goakt-ai](./goakt-ai) — **Distributed AI agents**: multi-agent system with Orchestrator, Research, Summarizer, Tool agents; OpenAI/Anthropic/Google/Mistral; CLI + load balancer; Kubernetes/Kind