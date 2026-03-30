# multi-dc: GoAkt Multi-Datacenter Cluster Example

This example demonstrates GoAkt's **multi-datacenter** support with:

- **Two datacenters** (DC-1 in us-east-1, DC-2 in eu-west-1) running separate clusters
- **NATS JetStream control plane** for cross-DC coordination
- **NATS discovery** for intra-DC peer finding
- **DataCenterGateway singleton** for cross-DC actor discovery via `DiscoverActor`
- **Cross-DC actor placement** via `SpawnOn` with `WithDataCenter`
- **Location-transparent messaging** across datacenters
- **PostgreSQL persistence** per datacenter (isolated stores)
- **HTTP/JSON REST API** with Swagger UI
- **OpenTelemetry tracing** (Jaeger)

## Architecture

```
                          ┌──────────────────────┐
                          │  NATS Server         │
                          │  (JetStream enabled) │
                          │  - Control Plane     │
                          │  - Discovery (both)  │
                          └──────────┬───────────┘
                                     │
                   ┌─────────────────┼─────────────────┐
                   │                                   │
   ┌───────────────▼───────────────┐   ┌───────────────▼───────────────┐
   │          DC-1                 │   │          DC-2                 │
   │      (us-east-1/az-a)         │   │      (eu-west-1/az-a)         │
   │                               │   │                               │
   │  ┌─────────────────────────┐  │   │  ┌─────────────────────────┐  │
   │  │   accounts-dc1 cluster  │  │   │  │   accounts-dc2 cluster  │  │
   │  │   (3 replicas)          │  │   │  │   (3 replicas)          │  │
   │  │                         │  │   │  │                         │  │
   │  │  ┌───────────────────┐  │  │   │  │  ┌───────────────────┐  │  │
   │  │  │ Leader (pod-0)    │  │  │   │  │  │ Leader (pod-0)    │  │  │
   │  │  │ - DC Controller   │  │◄─┼───┼──┤  │ - DC Controller   │  │  │
   │  │  │ - DC Gateway      │  │  │   │  │  │ - DC Gateway      │  │  │
   │  │  │   (singleton)     │  │  │   │  │  │   (singleton)     │  │  │
   │  │  └───────────────────┘  │  │   │  │  └───────────────────┘  │  │
   │  │  ┌────────┐ ┌────────┐  │  │   │  │  ┌────────┐ ┌────────┐  │  │
   │  │  │ pod-1  │ │ pod-2  │  │  │   │  │  │ pod-1  │ │ pod-2  │  │  │
   │  │  └────────┘ └────────┘  │  │   │  │  └────────┘ └────────┘  │  │
   │  └─────────────────────────┘  │   │  └─────────────────────────┘  │
   │              │                │   │              │                │
   │  ┌───────────▼─────────────┐  │   │  ┌───────────▼─────────────┐  │
   │  │  PostgreSQL (dc1 store) │  │   │  │  PostgreSQL (dc2 store) │  │
   │  └─────────────────────────┘  │   │  └─────────────────────────┘  │
   └───────────────────────────────┘   └───────────────────────────────┘
                   │                                   │
                   └───────────────┬───────────────────┘
                                   │
                          ┌────────▼────────┐
                          │  Nginx LB       │
                          │  /dc1/* → DC-1  │
                          │  /dc2/* → DC-2  │
                          └─────────────────┘
```

### Cross-DC Request Flow

When DC-2 receives a request for an account that exists in DC-1:

```
Client → Nginx (/dc2/accounts/acc-001)
           │
           ▼
    DC-2 pod (any)
           │
    1. ActorOf("acc-001") → not in DC-2 cluster
           │
    2. ActorOf("dc-gateway") → remote PID on leader
           │
           ▼
    DC-2 Leader (dc-gateway singleton)
           │
    3. SendSync("acc-001", GetAccount)
       └─ ActorOf → not found locally
       └─ DiscoverActor → queries NATS control plane
          └─ RemoteLookup to DC-1 endpoints
                    │
                    ▼
             DC-1 cluster
                    │
    4. Finds acc-001 actor → returns Account response
                    │
                    ▼
         Response flows back through gateway → service → client
```

## Prerequisites

| Tool        | Purpose                              | Installation                                                                     |
|-------------|--------------------------------------|----------------------------------------------------------------------------------|
| **Kind**    | Local Kubernetes cluster             | [kind.sigs.k8s.io](https://kind.sigs.k8s.io/docs/user/quick-start/#installation) |
| **kubectl** | Kubernetes CLI                       | [kubectl install](https://kubernetes.io/docs/tasks/tools/)                       |
| **Earthly** | Reproducible builds                  | [earthly.dev](https://earthly.dev/get-earthly)                                   |
| **Docker**  | Container runtime (required by Kind) | [docker.com](https://docs.docker.com/get-docker/)                                |

## Quick Start

### 1. Create the Kind Cluster

```bash
cd goakt-cluster/multi-dc
make cluster-create
```

### 2. Build and Deploy

```bash
make deploy
```

Builds the image, loads it into Kind, and deploys: NATS, PostgreSQL (x2), accounts DC-1 (3 replicas), accounts DC-2 (3 replicas), OTEL Collector, Jaeger, and Nginx.

## Testing the Service

### Step 1: Expose the API

```bash
make port-forward
```

- DC-1 API: `http://localhost:8080/dc1/`
- DC-2 API: `http://localhost:8080/dc2/`
- Default (DC-1): `http://localhost:8080/`
- Swagger UI: `http://localhost:8080/docs`

### Step 2: Verify Deployment

```bash
make status
```

### Step 3: Smoke Test (Manual)

```bash
# Check DC status
curl http://localhost:8080/dc1/dc/status
curl http://localhost:8080/dc2/dc/status

# Create account in DC-1
curl -X POST http://localhost:8080/dc1/accounts \
  -H "Content-Type: application/json" \
  -d '{"create_account":{"account_id":"acc-001","account_balance":100.00}}'

# Query account from DC-2 (cross-DC lookup)
curl http://localhost:8080/dc2/accounts/acc-001

# Credit account via DC-2
curl -X POST http://localhost:8080/dc2/accounts/acc-001/credit \
  -H "Content-Type: application/json" \
  -d '{"balance":50.00}'

# Verify final balance from DC-1 (should be 150)
curl http://localhost:8080/dc1/accounts/acc-001

# Spawn account remotely in DC-2 from DC-1
curl -X POST http://localhost:8080/dc1/accounts/spawn-remote \
  -H "Content-Type: application/json" \
  -d '{"account_id":"acc-remote","account_balance":200.00,"target_dc":"dc-2"}'

# Verify remote account from DC-2
curl http://localhost:8080/dc2/accounts/acc-remote
```

### Step 4: Automated Test

```bash
make test
```

Runs the full cross-DC integration test: creates accounts in DC-1, queries from DC-2, credits via DC-2, verifies from DC-1, and tests remote spawn.

### Step 5: View Traces

```bash
make port-forward-jaeger
```

Open [http://localhost:16686](http://localhost:16686), select `accounts-dc1` or `accounts-dc2` service.

## Key Concepts

### NATS Discovery (Intra-DC)

Each datacenter uses a separate NATS subject for peer discovery (`goakt.discovery.dc-1`, `goakt.discovery.dc-2`). This ensures nodes only discover peers within the same DC.

### NATS JetStream Control Plane (Cross-DC)

The control plane manages datacenter registration, heartbeats, state transitions, and event watching. Each DC's leader registers its cluster members' remoting addresses with the control plane.

### Cross-DC Actor Placement

Use `SpawnOn` with `WithDataCenter` to spawn actors in a specific datacenter:

```go
targetDC := &datacenter.DataCenter{Name: "dc-2"}
pid, err := system.SpawnOn(ctx, "worker", NewWorker(), actor.WithDataCenter(targetDC))
```

### Location-Transparent Messaging

Once spawned, actors are addressed the same way regardless of datacenter. `Tell`, `Ask`, and `ActorOf` route automatically.

## Makefile Reference

| Target                     | Description                                            |
|----------------------------|--------------------------------------------------------|
| `make deploy`              | Build image, load into Kind, and deploy all components |
| `make cluster-create`      | Create a new Kind cluster                              |
| `make cluster-delete`      | Delete the Kind cluster                                |
| `make cluster-up`          | Deploy all components (NATS, PG x2, DC-1, DC-2, Nginx) |
| `make cluster-down`        | Remove all deployments                                 |
| `make status`              | Show cluster and pod status                            |
| `make port-forward`        | Forward nginx to localhost:8080                        |
| `make port-forward-jaeger` | Forward Jaeger UI to localhost:16686                   |
| `make test`                | Run cross-DC integration tests                         |
| `make logs-dc1`            | Tail logs from DC-1 pods                               |
| `make logs-dc2`            | Tail logs from DC-2 pods                               |

## Configuration

### Environment Variables (Pods)

| Variable         | Description            | DC-1 value       | DC-2 value       |
|------------------|------------------------|------------------|------------------|
| `PORT`           | HTTP API port          | 50051            | 50051            |
| `DISCOVERY_PORT` | Cluster discovery port | 3322             | 3322             |
| `PEERS_PORT`     | Cluster gossip port    | 3320             | 3320             |
| `REMOTING_PORT`  | Actor remoting port    | 50052            | 50052            |
| `NATS_URL`       | NATS server URL        | nats://nats:4222 | nats://nats:4222 |
| `DC_NAME`        | Datacenter name        | dc-1             | dc-2             |
| `DC_REGION`      | Datacenter region      | us-east-1        | eu-west-1        |
| `DC_ZONE`        | Datacenter zone        | az-a             | az-a             |
| `DB_HOST`        | PostgreSQL host        | postgres-dc1     | postgres-dc2     |

## Project Structure

```
multi-dc/
├── actors/          # Account entity actor (with persistence)
├── api/             # OpenAPI spec and generated HTTP handlers
├── cmd/             # CLI entry point (NATS discovery + DC config)
├── db/
│   └── migrations/  # SQL schema
├── deploy/
│   ├── kind-config.yaml
│   ├── nats-deployment.yaml           # NATS with JetStream
│   ├── dc1.yaml                       # DC-1 StatefulSet + Services
│   ├── dc2.yaml                       # DC-2 StatefulSet + Services
│   ├── postgres-dc1-deployment.yaml   # PostgreSQL for DC-1
│   ├── postgres-dc2-deployment.yaml   # PostgreSQL for DC-2
│   ├── postgres-*.yaml                # Shared PG config/secret
│   ├── nginx-*.yaml                   # Load balancer
│   ├── otel-collector-*.yaml          # OTEL Collector
│   └── jaeger-deployment.yaml         # Jaeger (trace backend)
├── domain/          # Account domain model
├── messages/        # Go structs for actor messages
├── persistence/     # PostgreSQL store
├── scripts/         # Integration test scripts
├── service/         # HTTP API service (with cross-DC endpoints)
├── doc.md           # This file
└── Makefile
```

## Differences from k8s-v2 (Single DC)

| Feature        | k8s-v2 (Single DC) | multi-dc                       |
|----------------|--------------------|--------------------------------|
| Discovery      | Kubernetes API     | NATS                           |
| Datacenters    | 1                  | 2 (DC-1 + DC-2)                |
| Control plane  | None               | NATS JetStream                 |
| PostgreSQL     | 1 instance         | 2 instances (per DC)           |
| StatefulSets   | 1 (accounts)       | 2 (accounts-dc1, accounts-dc2) |
| Cross-DC spawn | N/A                | SpawnOn + WithDataCenter       |
| DC status API  | N/A                | GET /dc/status                 |
| Nginx routing  | / → accounts       | /dc1/* → DC-1, /dc2/* → DC-2   |

## Troubleshooting

### NATS not starting

```bash
kubectl get pods -l app=nats
kubectl logs deployment/nats
```

### Pods stuck in Init

Init containers wait for both PostgreSQL and NATS. Check both:

```bash
kubectl get pods -l app=postgres
kubectl get pods -l app=nats
kubectl describe pod accounts-dc1-0
```

### Cross-DC queries failing

Check DC readiness:

```bash
curl http://localhost:8080/dc1/dc/status
curl http://localhost:8080/dc2/dc/status
```

Both should show `"ready":true`. If not, the control plane may still be initializing.

### Cleanup

```bash
make cluster-down              # Remove deployments, keep cluster
make cluster-down cluster-delete   # Remove everything
```

## License

MIT License - see repository root for details.
