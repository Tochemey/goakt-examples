# multi-dc-isolated: GoAkt Multi-Datacenter with Network Isolation

This example demonstrates GoAkt's **multi-datacenter** support using **two separate Kind clusters** to simulate real network isolation between datacenters. Unlike the `multi-dc` example (which uses a single Kind cluster with two StatefulSets), this setup creates a genuine network boundary between DCs.

- **Two Kind clusters** (`kind-dc1`, `kind-dc2`) representing separate datacenters
- **Standalone NATS container** on a shared Docker network for cross-DC coordination
- **NATS JetStream control plane** for datacenter registration and discovery
- **NATS discovery** for intra-DC peer finding
- **DataCenterGateway singleton** for cross-DC actor discovery via `DiscoverActor`
- **PostgreSQL persistence** per datacenter (isolated stores)
- **HTTP/JSON REST API** with Swagger UI

## Architecture

```
                       ┌────────────────────────┐
                       │  nats-shared (Docker)  │
                       │  JetStream + KV Store  │
                       └───────────┬────────────┘
                                   │
                 ┌─────────────────┼─────────────────┐
                 │   goakt-multi-dc-net (Docker)     │
                 │                                   │
   ┌─────────────▼───────────────┐   ┌───────────────▼─────────────┐
   │  kind-dc1 (172.20.0.3)      │   │  kind-dc2 (172.20.0.4)      │
   │                             │   │                             │
   │  ┌───────────────────────┐  │   │  ┌───────────────────────┐  │
   │  │ Leader (pod-0)        │  │   │  │ Leader (pod-0)        │  │
   │  │ - DC Controller       │  │   │  │ - DC Controller       │  │
   │  │ - DC Gateway          │◄─┼───┼─►│ - DC Gateway          │  │
   │  │   (singleton)         │  │   │  │   (singleton)         │  │
   │  │ :50051 / :50052       │  │   │  │ :50051 / :50052       │  │
   │  └───────────────────────┘  │   │  └───────────────────────┘  │
   │  ┌──────────┐ ┌──────────┐  │   │  ┌──────────┐ ┌──────────┐  │
   │  │ pod-1    │ │ pod-2    │  │   │  │ pod-1    │ │ pod-2    │  │
   │  │ :50151   │ │ :50251   │  │   │  │ :50151   │ │ :50251   │  │
   │  └──────────┘ └──────────┘  │   │  └──────────┘ └──────────┘  │
   │                             │   │                             │
   │  ┌───────────────────────┐  │   │  ┌───────────────────────┐  │
   │  │ PostgreSQL (dc1)      │  │   │  │ PostgreSQL (dc2)      │  │
   │  └───────────────────────┘  │   │  └───────────────────────┘  │
   │  ┌───────────────────────┐  │   │  ┌───────────────────────┐  │
   │  │ nginx (:8080)         │  │   │  │ nginx (:8080)         │  │
   │  └───────────────────────┘  │   │  └───────────────────────┘  │
   └─────────────────────────────┘   └─────────────────────────────┘
```

Each DC runs in its own Kind cluster (separate Docker container). The clusters are connected via a shared Docker network (`goakt-multi-dc-net`). A standalone NATS container provides both peer discovery (per-DC) and JetStream-backed control plane (cross-DC coordination). All pods use `hostNetwork: true` and bind to the Kind node's shared network IP.

### Cross-DC Request Flow

When DC-2 receives a request for an account that exists in DC-1:

```
Client → docker exec dc2-control-plane curl :8080/accounts/acc-001
           │
           ▼
    nginx (:8080) → round-robin across pod-0/pod-1/pod-2
           │
           ▼
    DC-2 pod (any)
           │
    1. ActorOf("acc-001") → not in DC-2's Olric cluster
           │
    2. ActorOf("dc-gateway") → remote PID on DC-2 leader
           │
           ▼
    DC-2 Leader (dc-gateway singleton on 172.20.0.4:50052)
           │
    3. SendSync("acc-001", GetAccount)
       └─ ActorOf → not found locally
       └─ DiscoverActor → queries NATS JetStream KV
          └─ Finds DC-1 endpoints: [172.20.0.3:50052, :50152, :50252]
          └─ RemoteLookup to DC-1 via shared Docker network
                    │
                    ▼
             DC-1 cluster (172.20.0.3)
                    │
    4. Finds acc-001 in DC-1's Olric cluster → returns Account
                    │
                    ▼
         Response flows back through gateway → service → nginx → client
```

## Key Differences from `multi-dc`

| Aspect            | multi-dc           | multi-dc-isolated              |
|-------------------|--------------------|--------------------------------|
| Kind clusters     | 1 shared           | 2 separate                     |
| Network isolation | Simulated (labels) | Real (separate K8s networks)   |
| NATS              | K8s Deployment     | Standalone Docker container    |
| Pod networking    | Normal pod IPs     | `hostNetwork: true`            |
| Remoting address  | Pod FQDN (K8s DNS) | Kind node Docker IP            |
| Port allocation   | Fixed per pod      | Ordinal-based (unique per pod) |

## Prerequisites

| Tool        | Purpose                              | Installation                                                                     |
|-------------|--------------------------------------|----------------------------------------------------------------------------------|
| **Docker**  | Container runtime (required by Kind) | [docker.com](https://docs.docker.com/get-docker/)                                |
| **Kind**    | Local Kubernetes clusters            | [kind.sigs.k8s.io](https://kind.sigs.k8s.io/docs/user/quick-start/#installation) |
| **kubectl** | Kubernetes CLI                       | [kubectl install](https://kubernetes.io/docs/tasks/tools/)                       |
| **Earthly** | Reproducible builds                  | [earthly.dev](https://earthly.dev/get-earthly)                                   |

## Quick Start

### 1. Setup Infrastructure

Creates the shared Docker network, starts a standalone NATS container with JetStream, and creates two Kind clusters (`kind-dc1` and `kind-dc2`).

```bash
cd goakt-cluster/multi-dc-isolated
make setup
```

This outputs the discovered IPs:
```
NATS IP:  172.20.0.2
DC-1 IP:  172.20.0.3
DC-2 IP:  172.20.0.4
```

### 2. Build and Deploy

Builds the Docker image, loads it into both Kind clusters, and deploys PostgreSQL, the accounts StatefulSet (3 replicas), and nginx to each DC.

```bash
make deploy
```

You can also deploy to individual DCs:
```bash
make deploy-dc1
make deploy-dc2
```

### 3. Verify Deployment

```bash
make status
```

You should see 3 account pods, 1 postgres pod, and 1 nginx pod in each DC:
```
=== DC-1 (kind-dc1) ===
NAME               READY   STATUS
accounts-dc1-0     1/1     Running
accounts-dc1-1     1/1     Running
accounts-dc1-2     1/1     Running
postgres-dc1-...   1/1     Running
nginx-...          1/1     Running

=== DC-2 (kind-dc2) ===
NAME               READY   STATUS
accounts-dc2-0     1/1     Running
accounts-dc2-1     1/1     Running
accounts-dc2-2     1/1     Running
postgres-dc2-...   1/1     Running
nginx-...          1/1     Running
```

### 4. Smoke Test (Manual)

Since the Docker network IPs are not reachable from the host, use `docker exec` to run curl commands from inside the Kind nodes:

```bash
# Check DC status
docker exec dc1-control-plane curl -s http://127.0.0.1:8080/dc/status
docker exec dc2-control-plane curl -s http://127.0.0.1:8080/dc/status

# Create account in DC-1
docker exec dc1-control-plane curl -s -X POST http://127.0.0.1:8080/accounts \
  -H "Content-Type: application/json" \
  -d '{"create_account":{"account_id":"acc-001","account_balance":100.00}}'

# Query account from DC-2 (cross-DC lookup across separate Kind clusters)
docker exec dc2-control-plane curl -s http://127.0.0.1:8080/accounts/acc-001

# Credit account via DC-2
docker exec dc2-control-plane curl -s -X POST http://127.0.0.1:8080/accounts/acc-001/credit \
  -H "Content-Type: application/json" \
  -d '{"balance":50.00}'

# Verify final balance from DC-1 (should be 150)
docker exec dc1-control-plane curl -s http://127.0.0.1:8080/accounts/acc-001
```

### 5. Automated Test

```bash
make test
```

Runs the full cross-DC integration test: creates 50 accounts in DC-1, queries all from DC-2, credits via DC-2, and verifies final balances from DC-1.

### 6. View Logs

```bash
make logs-dc1    # Tail DC-1 pod logs
make logs-dc2    # Tail DC-2 pod logs
```

Or use k9s for an interactive terminal dashboard:

```bash
make k9s-dc1     # k9s for DC-1
make k9s-dc2     # k9s for DC-2
```

### 7. Clean Up

```bash
make teardown
```

Deletes both Kind clusters, the NATS container, and the shared Docker network.

## How It Works

### hostNetwork and Ordinal-Based Ports

With `hostNetwork: true`, each pod shares the Kind node's network namespace. Since all 3 pods in a DC are on the same Kind node, each needs unique ports. The `POD_ORDINAL` (extracted from the pod name) offsets the base ports by `ordinal * 100`:

| Pod   | HTTP  | Remoting | Discovery | Peers |
|-------|-------|----------|-----------|-------|
| pod-0 | 50051 | 50052    | 3322      | 3320  |
| pod-1 | 50151 | 50152    | 3422      | 3420  |
| pod-2 | 50251 | 50252    | 3522      | 3520  |

### Cross-DC Communication

Each pod's `NODE_IP` is set to the Kind node's IP on the shared Docker network (e.g., `172.20.0.3` for DC-1). GoAkt uses this IP as the remoting bind address, making pods directly reachable from the other Kind cluster via the shared Docker network.

### DC Gateway Singleton

A `DataCenterGateway` cluster singleton runs on each DC's leader node. It handles cross-DC actor discovery via `SendSync` (which uses `DiscoverActor` through the NATS control plane). Non-leader nodes route cross-DC requests to the gateway via the cluster.

### NATS Discovery (Intra-DC)

Each datacenter uses a separate NATS subject for peer discovery (`goakt.discovery.dc-1`, `goakt.discovery.dc-2`). This ensures nodes only discover peers within the same DC.

### NATS JetStream Control Plane (Cross-DC)

The control plane manages datacenter registration, heartbeats, and cache synchronization. Each DC's leader registers its cluster members' remoting addresses with the control plane, enabling `DiscoverActor` to locate actors across DCs.

## Makefile Reference

| Target            | Description                                                |
|-------------------|------------------------------------------------------------|
| `make setup`      | Create Docker network, NATS container, and 2 Kind clusters |
| `make deploy`     | Build image and deploy to both DCs                         |
| `make deploy-dc1` | Deploy to DC-1 only                                        |
| `make deploy-dc2` | Deploy to DC-2 only                                        |
| `make teardown`   | Delete everything                                          |
| `make image`      | Build the Docker image only                                |
| `make status`     | Show pod status in both DCs                                |
| `make test`       | Run cross-DC API integration tests                         |
| `make logs-dc1`   | Tail DC-1 pod logs                                         |
| `make logs-dc2`   | Tail DC-2 pod logs                                         |
| `make k9s-dc1`    | Launch k9s for DC-1                                        |
| `make k9s-dc2`    | Launch k9s for DC-2                                        |

## Configuration

### Environment Variables (Pods)

| Variable         | Description                 | DC-1 value             | DC-2 value             |
|------------------|-----------------------------|------------------------|------------------------|
| `PORT`           | Base HTTP API port          | 50051                  | 50051                  |
| `DISCOVERY_PORT` | Base discovery port         | 3322                   | 3322                   |
| `PEERS_PORT`     | Base gossip port            | 3320                   | 3320                   |
| `REMOTING_PORT`  | Base remoting port          | 50052                  | 50052                  |
| `POD_ORDINAL`    | StatefulSet pod index       | 0, 1, or 2             | 0, 1, or 2             |
| `NODE_IP`        | Kind node shared network IP | 172.20.0.3             | 172.20.0.4             |
| `NATS_URL`       | NATS server URL             | nats://172.20.0.2:4222 | nats://172.20.0.2:4222 |
| `DC_NAME`        | Datacenter name             | dc-1                   | dc-2                   |
| `DC_REGION`      | Datacenter region           | us-east-1              | eu-west-1              |
| `DC_ZONE`        | Datacenter zone             | az-a                   | az-a                   |
| `DB_HOST`        | PostgreSQL host             | postgres-dc1           | postgres-dc2           |
| `LOG_LEVEL`      | Log level                   | info                   | info                   |

Actual ports are computed as: `base_port + (POD_ORDINAL * 100)`.

## Troubleshooting

### Pods can't reach NATS
Check that the Kind nodes are connected to the shared Docker network:
```bash
docker network inspect goakt-multi-dc-net
```

### Cross-DC lookups fail
Verify the NATS container is running and the JetStream KV has records:
```bash
docker exec nats-shared wget -qO- http://127.0.0.1:8222/jsz?streams=true
```
The `KV_goakt_datacenters` stream should have 2 messages (one per DC).

### Port conflicts
With `hostNetwork: true`, ensure no other processes use ports in the 50051-50252 or 3320-3522 ranges on the Kind nodes.

### DC status shows `ready: false`
The DC controller's cache takes a few seconds to sync after startup. Wait 15-30 seconds and retry. Only the cluster leader node reports `ready: true`.
