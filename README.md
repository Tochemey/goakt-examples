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

---

## Examples Overview

### Core Concepts

| Example                                                  | Demonstrates                                                                                    |
|----------------------------------------------------------|-------------------------------------------------------------------------------------------------|
| [**goakt-actor-hello-world**](./goakt-actor-hello-world) | Minimal actor system: spawn an actor, send messages, graceful shutdown                          |
| [**goakt-ping-pong**](./goakt-ping-pong)                 | Actor-to-actor messaging: two actors exchanging messages (Tell pattern)                         |
| [**goakt-actor-behaviors**](./goakt-actor-behaviors)     | Stateful behaviors: actor with multiple states (authenticated, logged-in) and state transitions |

### Remoting & Location Transparency

| Example                                                              | Demonstrates                                                                               |
|----------------------------------------------------------------------|--------------------------------------------------------------------------------------------|
| [**goakt-remoting**](./goakt-remoting)                               | Actor remoting: Ping and Pong actors on separate processes, communicating over the network |
| [**goakt-actors-cluster/dynalloc**](./goakt-actors-cluster/dynalloc) | Location transparency: actors can live on any node; cluster routes messages automatically  |

### Clustering & Discovery

| Example                                                              | Demonstrates                                                                             |
|----------------------------------------------------------------------|------------------------------------------------------------------------------------------|
| [**goakt-actors-cluster/static**](./goakt-actors-cluster/static)     | Static peer discovery: cluster nodes configured via fixed addresses                      |
| [**goakt-actors-cluster/dnssd**](./goakt-actors-cluster/dnssd)       | DNS-SD discovery: nodes discover each other via mDNS/DNS (protobuf messages)             |
| [**goakt-actors-cluster/dnssd-v2**](./goakt-actors-cluster/dnssd-v2) | DNS-SD + Go types: same as dnssd but with standard Go structs and PostgreSQL persistence |
| [**goakt-actors-cluster/k8s**](./goakt-actors-cluster/k8s)           | Kubernetes discovery: cluster on K8s using the API to discover pods (gRPC, protobuf)     |
| [**goakt-actors-cluster/k8s-v2**](./goakt-actors-cluster/k8s-v2)     | Kubernetes + persistence: K8s discovery with Go types, HTTP/JSON API, and PostgreSQL     |

### Persistence & Extensions

| Example                                                  | Demonstrates                                                                          |
|----------------------------------------------------------|---------------------------------------------------------------------------------------|
| [**goakt-actor-persistence**](./goakt-actor-persistence) | Persistence extension: actor state snapshots to a pluggable store (in-memory example) |

### Grains (Virtual Actors)

| Example                                                                      | Demonstrates                                                           |
|------------------------------------------------------------------------------|------------------------------------------------------------------------|
| [**goakt-grains**](./goakt-grains)                                           | Grains model: virtual actors with automatic activation and passivation |
| [**goakt-grains-cluster/grains-dnssd**](./goakt-grains-cluster/grains-dnssd) | Grains clustering: grains across multiple nodes with DNS-SD discovery  |

### Applications

| Example                              | Demonstrates                                                                    |
|--------------------------------------|---------------------------------------------------------------------------------|
| [**goakt-chat**](./goakt-chat)       | Multi-room chat: remoting, room-based messaging, message history (protobuf)     |
| [**goakt-chat-v2**](./goakt-chat-v2) | Chat with Go types: same chat app using standard Go structs instead of protobuf |

---

## Quick Reference

| Example           | API       | Messages | Discovery  | Persistence |
|-------------------|-----------|----------|------------|-------------|
| hello-world       | —         | —        | —          | —           |
| ping-pong         | —         | protobuf | —          | —           |
| actor-behaviors   | —         | protobuf | —          | —           |
| remoting          | —         | protobuf | —          | —           |
| dynalloc          | gRPC      | protobuf | static     | —           |
| static            | gRPC      | protobuf | static     | —           |
| dnssd             | gRPC      | protobuf | DNS-SD     | —           |
| dnssd-v2          | HTTP/JSON | Go types | DNS-SD     | PostgreSQL  |
| k8s               | gRPC      | protobuf | Kubernetes | —           |
| k8s-v2            | HTTP/JSON | Go types | Kubernetes | PostgreSQL  |
| actor-persistence | —         | protobuf | —          | extension   |
| grains            | —         | protobuf | —          | —           |
| grains-dnssd      | gRPC      | protobuf | DNS-SD     | —           |
| chat              | gRPC      | protobuf | —          | —           |
| chat-v2           | gRPC      | Go types | —          | —           |

---

## Running the Examples

Each example has its own run instructions. Common patterns:

- **Single-process**: `go run .` or run the built binary
- **Docker Compose**: `docker-compose up` (static, dnssd, dynalloc, grains-dnssd)
- **Kubernetes (Kind)**: `make cluster-create && make deploy` (k8s, k8s-v2)

See the `doc.md` in each example directory for detailed steps.
