# GoAkt Examples

[![GitHub go.mod Go version](https://badges.chse.dev/github/go-mod/go-version/Tochemey/goakt-examples)](https://go.dev/doc/install)

Examples for [GoAkt](https://github.com/Tochemey/goakt) v4. For the v3.14 examples, see the [v3 branch](https://github.com/Tochemey/goakt-examples/tree/release/v3.14).

## Getting Started

```bash
git clone https://github.com/Tochemey/goakt-examples
cd goakt-examples
```

To build all examples:

```bash
make build
```

Code generation (protobuf, Connect, OpenAPI) and the Docker images for the cluster examples are driven by the root [Makefile](./Makefile) and only need [Docker](https://docs.docker.com/get-docker/):

```bash
make all      # regenerate the protobuf and OpenAPI code
make images   # build every example Docker image
make help     # list all targets
```

## Examples

### Core Concepts

- [goakt-hello-world](./goakt-hello-world): a minimal actor system. Spawns an actor, sends it messages, and shuts down gracefully.
- [goakt-ping-pong](./goakt-ping-pong): two actors exchanging messages using the Tell pattern.
- [goakt-behaviors](./goakt-behaviors): an actor that moves between states (authenticated, logged in) using behaviors.
- [goakt-parent-child](./goakt-parent-child): spawning child actors from inside an actor and observing `Terminated` events.
- [goakt-supervision](./goakt-supervision): the `Stop`, `Resume`, and `Restart` supervision directives, each with retry limits.
- [goakt-routers](./goakt-routers): actor pools with round-robin, fan-out, and consistent-hash routing, including resizing a pool at runtime.
- [goakt-pubsub](./goakt-pubsub): topic-based publish/subscribe using `WithPubSub()` and the system `TopicActor`.
- [goakt-scheduler](./goakt-scheduler): scheduling messages with `ScheduleOnce`, recurring schedules, and cancellation by reference.
- [goakt-dead-letters](./goakt-dead-letters): subscribing to the system event stream to observe unhandled messages.

### Remoting and Location Transparency

- [goakt-remoting](./goakt-remoting): Ping and Pong actors running in separate processes and communicating over the network.
- [goakt-cluster/dynalloc](goakt-cluster/dynalloc): location transparency in a cluster. Actors can live on any node and the cluster routes messages to them.

### Clustering and Discovery

- [goakt-cluster/static](goakt-cluster/static): a cluster whose nodes are configured with fixed peer addresses.
- [goakt-cluster/dnssd](goakt-cluster/dnssd): nodes that discover each other through DNS-SD/mDNS. Exposes the same account actors over two client APIs on one port — a gRPC-compatible Connect RPC service and an OpenAPI-generated REST/JSON service with a Swagger UI — while the actors themselves use plain Go structs with CBOR serialization, backed by PostgreSQL persistence.
- [goakt-cluster/k8s](goakt-cluster/k8s): a Kubernetes cluster with PostgreSQL persistence and OpenTelemetry tracing. An exclusive `--codec` / `CODEC` switch selects the full stack: `cbor` (HTTP/JSON OpenAPI + CBOR Go-struct remoting, default) or `proto` (Connect/gRPC + protobuf remoting).
- [goakt-cluster/k8s-ebpf](goakt-cluster/k8s-ebpf): the k8s example with actor-level tracing collected by a goakt-ebpf sidecar, without instrumenting the application code.
- [goakt-cluster/multi-dc](goakt-cluster/multi-dc): two datacenters (us-east-1 and eu-west-1) with a NATS JetStream control plane, NATS discovery, and cross-datacenter actor placement via `SpawnOn` and `WithDataCenter`.
- [goakt-cluster/multi-dc-isolated](goakt-cluster/multi-dc-isolated): the same as multi-dc but with two separate Kind clusters on a shared Docker network, to simulate real network boundaries between datacenters.

### Persistence and Extensions

- [goakt-persistence](./goakt-persistence): the persistence extension. Snapshots actor state to a pluggable store, with an in-memory store as the example implementation.

### Grains (Virtual Actors)

- [goakt-grains](./goakt-grains): the grains model. Virtual actors with automatic activation and passivation.
- [goakt-iot-twin](./goakt-iot-twin): device twins as grains. Each device activates on demand, passivates when idle, and restores its state on reactivation.
- [goakt-grains-cluster/grains-dnssd](./goakt-grains-cluster/grains-dnssd): grains spread across multiple nodes with DNS-SD discovery.

### Reactive Streams

- [goakt-stream](./goakt-stream): backpressure-aware pipelines built from Sources, Flows, and Sinks, with fan-out and fan-in topologies, parallel processing, and actor integration.

### Applications

- [goakt-chat](./goakt-chat): a multi-room chat application with remoting, room-based messaging, direct messages, and message history, shipped as a single CLI with `server` and `client` subcommands. The actors are written against one domain model and a `wire` package maps it onto either protobuf or CBOR-encoded Go structs, selected per client with `--codec`; a single server serves both formats at once.
- [goakt-blockchain](./goakt-blockchain): a GoAkt port of scalachain, the actor-based blockchain from the freeCodeCamp article [How to build a simple actor-based blockchain](https://www.freecodecamp.org/news/how-to-build-a-simple-actor-based-blockchain-aac1e996c177/), running as a GoAkt cluster on Kubernetes/Kind with the Kubernetes discovery provider. The `Node` actor is a cluster singleton supervising a transaction `Broker` and a proof-of-work `Miner` (a `Become`/`UnBecome` state machine that pipes the mined proof back with `PipeTo`); like a real blockchain network, every pod keeps a full replica of the ledger in an embedded Pebble store, mined blocks fan out over pub/sub, replicas catch up from their peers on start, and the chain survives singleton failover.
- [goakt-saga](./goakt-saga): a money transfer service that uses the saga pattern with compensating transactions. Runs on Kubernetes/Kind and uses plain Go types.
- [goakt-2pc](./goakt-2pc): the same money transfer service implemented with two-phase commit instead of a saga. Runs on Kubernetes/Kind and uses plain Go types.
- [goakt-ai](./goakt-ai): a distributed multi-agent system with Orchestrator, Research, Summarizer, and Tool agents. Supports OpenAI, Anthropic, Google, and Mistral models, and ships with a CLI and a load balancer. Runs on Kubernetes/Kind.
- [goakt-tetris](./goakt-tetris): a browser-playable Tetris game where each match is an actor tied to a WebSocket connection. Covers a scheduled-tick game loop, `Watch`/`Terminated` lifecycle cleanup, a `SpawnSingleton` matchmaker, cluster-aware placement with `SpawnOn`, and CBOR serializers, with a TypeScript canvas client and a two-node `docker compose` setup.
- [goakt-pictograph](./goakt-pictograph): a browser-playable multiplayer drawing and guessing game in the style of Skribbl.io. Each room is a `RoomActor` with a `Become`-driven state machine (waiting, choosing, drawing, round over, game over), `Stash` for early guesses, a pub/sub topic per room for stroke and chat fan-out (spectators included), a `PlayerProfileGrain` for cross-session stats, and a cluster-wide CRDT `PNCounter` leaderboard. Uses the same two-node `docker compose` setup as goakt-tetris.
- [goakt-scrabble](./goakt-scrabble): browser-playable multiplayer Scrabble for 2 to 4 players, humans or bots, in English. Each room is a `RoomActor` with a turn-based `Become` state machine (waiting, playing, game over) around a pure-Go Scrabble engine (DAWG dictionary, premium-square scoring, Appel/Jacobson move generation). Each bot seat is a child `BotActor` that uses `ScheduleOnce` to simulate thinking time. Shared per-language dictionaries live in system Extensions, and a CRDT `PNCounter` tracks a per-player, per-language leaderboard. The client is a vanilla TypeScript SVG board.
- [skybound-runner](https://github.com/Tochemey/skybound-runner): a browser-playable co-op platformer for up to 4 players, in its own repository. A `GameActor` per match runs an authoritative 60 Hz physics simulation, a `MatchFactory` singleton matches players into games with `SpawnOn` and least-load placement, and a `PlayerSessionActor` bridges each WebSocket connection to its game. Uses CBOR serializers for remoting and a TypeScript canvas client.
