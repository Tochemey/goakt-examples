# k8s-v2: GoAkt Cluster with Kubernetes Discovery and Persistence

This example demonstrates a GoAkt actor cluster running on **Kubernetes** with:

- **Standard Go types** for actor messages (no protocol buffers)
- **PostgreSQL persistence** for actor state
- **HTTP/JSON REST API** for client communication
- **Kind** (Kubernetes in Docker) for local development

## Architecture

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
│ OTEL Collector │  │    PostgreSQL     │
│ (OTLP → Jaeger)│  │   (Persistence)  │
└───────┬────────┘  └──────────────────┘
        │
        ▼
┌────────────────┐
│     Jaeger     │
│ (Trace UI)     │
└────────────────┘
```

## Prerequisites

### Required Tools

| Tool        | Purpose                              | Installation                                                                     |
|-------------|--------------------------------------|----------------------------------------------------------------------------------|
| **Kind**    | Local Kubernetes cluster             | [kind.sigs.k8s.io](https://kind.sigs.k8s.io/docs/user/quick-start/#installation) |
| **kubectl** | Kubernetes CLI                       | [kubectl install](https://kubernetes.io/docs/tasks/tools/)                       |
| **Earthly** | Reproducible builds                  | [earthly.dev](https://earthly.dev/get-earthly)                                   |
| **Docker**  | Container runtime (required by Kind) | [docker.com](https://docs.docker.com/get-docker/)                                |

### Install Kind

```bash
# macOS (Homebrew)
brew install kind

# Linux
curl -Lo ./kind https://kind.sigs.k8s.io/dl/v0.24.0/kind-linux-amd64
chmod +x ./kind
sudo mv ./kind /usr/local/bin/kind

# Verify
kind version
```

### Install Earthly

```bash
# macOS (Homebrew)
brew install earthly

# Or use the official installer
curl -fsSL https://raw.githubusercontent.com/earthly/earthly/main/earthly-cli/install.sh | sudo bash
```

## Quick Start

### 1. Create the Kind Cluster

```bash
cd goakt-actors-cluster/k8s-v2
make cluster-create
```

Creates a Kubernetes cluster named `goakt-k8s-v2`. Wait for it to be ready.

### 2. Build and Deploy

```bash
make deploy
```

Builds the image, loads it into Kind, and deploys PostgreSQL, the accounts StatefulSet (3 replicas), and Nginx.

---

## Testing the Service

Follow these steps in order to verify the deployment and exercise the cluster.

### Step 1: Expose the API

In a terminal, start port-forwarding so the API is reachable from your machine:

```bash
make port-forward
```

API base URL: `http://localhost:8080`  
Swagger UI: [http://localhost:8080/docs](http://localhost:8080/docs)

Leave this running. Use additional terminals for the following steps.

### Step 2: Verify Deployment

Check that pods and services are running:

```bash
make status
```

Optional: open the Kubernetes dashboard to inspect workloads and pods:

```bash
make dashboard
```

Use the printed token to log in at the URL shown.

### Step 3: Smoke Test (Manual)

Verify the API with a single account:

```bash
# Create
curl -X POST http://localhost:8080/accounts \
  -H "Content-Type: application/json" \
  -d '{"create_account":{"account_id":"acc-001","account_balance":100.00}}'

# Get
curl http://localhost:8080/accounts/acc-001

# Credit
curl -X POST http://localhost:8080/accounts/acc-001/credit \
  -H "Content-Type: application/json" \
  -d '{"balance":50.00}'

# Verify final balance (should be 150)
curl http://localhost:8080/accounts/acc-001
```

### Step 4: Load Test (Automated)

Run the integration test that creates 1000 accounts, credits each, and verifies a sample across the cluster:

```bash
make test
```

Customize the test:

```bash
NUM_ACCOUNTS=500 VERIFY_SAMPLE=100 make test   # Fewer accounts
VERIFY_SAMPLE=1000 make test                    # Verify all 1000 accounts
```

### Step 5: Inspect Logs (Optional)

Tail logs from the accounts pods:

```bash
make logs
```

### Step 6: View Traces (Optional)

The accounts service emits OpenTelemetry traces (HTTP spans and custom actor spans) to the OTEL Collector. To view traces in Jaeger:

```bash
# In a separate terminal
make port-forward-jaeger
```

Then open [http://localhost:16686](http://localhost:16686), select the `accounts` service, and click "Find Traces". You'll see HTTP request spans with child spans for actor.Spawn, actor.ActorOf, and actor.Ask.

## Makefile Reference

| Target                   | Description                                            |
|--------------------------|--------------------------------------------------------|
| `make deploy`            | Build image, load into Kind, and deploy all components |
| `make cluster-create`    | Create a new Kind cluster                              |
| `make cluster-delete`    | Delete the Kind cluster                                |
| `make image`             | Build Docker image and load into Kind                  |
| `make cluster-up`        | Deploy PostgreSQL, accounts, nginx, and tracing stack  |
| `make cluster-down`      | Remove all deployments                                 |
| `make status`            | Show cluster and pod status                            |
| `make port-forward`      | Forward nginx to localhost:8080                        |
| `make port-forward-jaeger` | Forward Jaeger UI to localhost:16686 (view traces)    |
| `make dashboard`         | Access Kubernetes dashboard (workloads, pods)          |
| `make dashboard-install` | Install Kubernetes dashboard (one-time)                |
| `make logs`              | Tail logs from accounts pods                           |
| `make test`              | Run API integration tests (1000 accounts)              |

## Workflow

### First-Time Setup

```bash
make cluster-create
make deploy
```

Then follow [Testing the Service](#testing-the-service) (Steps 1–5).

### Iterative Development

```bash
make deploy
# Or faster: make image && kubectl rollout restart statefulset/accounts
```

### Cleanup

```bash
make cluster-down              # Remove deployments, keep cluster
make cluster-down cluster-delete   # Remove everything
```

## Configuration

### Environment Variables (Pods)

The accounts pods receive configuration via environment variables:

| Variable         | Description            | Default       |
|------------------|------------------------|---------------|
| `PORT`           | HTTP API port          | 50051         |
| `DISCOVERY_PORT` | Cluster discovery port | 3322          |
| `PEERS_PORT`     | Cluster gossip port    | 3320          |
| `REMOTING_PORT`  | Actor remoting port    | 50052         |
| `DB_HOST`        | PostgreSQL host        | postgres      |
| `DB_PORT`        | PostgreSQL port        | 5432          |
| `DB_NAME`        | Database name          | accounts      |
| `DB_USER`        | Database user          | (from secret) |
| `DB_PASSWORD`    | Database password      | (from secret) |

### Test Script (test-api.sh)

| Variable          | Description                   | Default               |
|-------------------|-------------------------------|-----------------------|
| `NUM_ACCOUNTS`    | Number of accounts to create  | 1000                  |
| `INITIAL_BALANCE` | Initial balance per account   | 100                   |
| `CREDIT_AMOUNT`   | Amount to credit each account | 50                    |
| `VERIFY_SAMPLE`   | Number of accounts to verify  | 100                   |
| `BASE_URL`        | API base URL                  | http://localhost:8080 |

### PostgreSQL Credentials

Default credentials are in `k8s/postgres-secret.yaml`:

- **User:** accounts
- **Password:** accounts
- **Database:** accounts

For production, use a proper secrets management solution.

## Differences from k8s (v1)

| Feature        | k8s (v1)         | k8s-v2              |
|----------------|------------------|---------------------|
| Actor messages | Protocol buffers | Standard Go structs |
| API            | gRPC/Connect     | HTTP/JSON REST      |
| Persistence    | None             | PostgreSQL          |
| Serialization  | Protobuf         | CBOR (for remoting) |
| Nginx          | gRPC proxy       | HTTP proxy          |
| Tracing        | None             | OpenTelemetry (HTTP + custom actor spans → Jaeger) |

## Troubleshooting

### Pods not starting

```bash
# Check pod status
kubectl get pods
kubectl describe pod <pod-name>

# Check logs
make logs
# or
kubectl logs -l app.kubernetes.io/name=accounts -f
```

### PostgreSQL connection issues

The accounts pods use an init container to wait for PostgreSQL. If pods are stuck in `Init:0/1`:

```bash
# Verify PostgreSQL is running
kubectl get pods -l app=postgres
kubectl logs deployment/postgres

# Check if init container completed
kubectl describe pod accounts-0
```

### Image not found

Ensure the image is loaded into Kind:

```bash
make image
# Verify
docker exec goakt-k8s-v2-control-plane crictl images | grep accounts
```

### Port 8080 already in use

Use a different port:

```bash
kubectl port-forward service/nginx 9080:80
# API at http://localhost:9080
```

### Dashboard token not showing

If `make dashboard` does not display a token:

```bash
kubectl -n kubernetes-dashboard create token dashboard-admin
```

### Kind cluster issues

```bash
# Delete and recreate
make cluster-delete
make cluster-create
make deploy
```

### No traces in Jaeger

- **Check OTEL Collector** — `kubectl logs deployment/otel-collector` should show trace batches being received.
- **Verify OTEL env vars** — Accounts pods use `OTEL_EXPORTER_OTLP_ENDPOINT` and `OTEL_SERVICE_NAME`; ensure the collector is reachable.

## Project Structure

```
k8s-v2/
├── actors/          # Account entity actor (with persistence)
├── api/             # OpenAPI spec and generated HTTP handlers
├── cmd/             # CLI entry point
├── db/
│   └── migrations/  # SQL schema
├── domain/          # Account domain model
├── k8s/             # Kubernetes manifests
│   ├── k8s.yaml            # StatefulSet, Service, RBAC
│   ├── postgres-*.yaml     # PostgreSQL deployment
│   ├── otel-collector-deployment.yaml  # OTEL Collector (OTLP → Jaeger)
│   ├── jaeger-deployment.yaml         # Jaeger (trace backend)
│   └── nginx-*.yaml        # Load balancer
├── messages/        # Go structs for actor messages
├── persistence/     # PostgreSQL store interface and implementation
├── scripts/         # Test scripts
├── service/         # HTTP API service
├── doc.md           # This file
└── Makefile
```

## License

MIT License - see repository root for details.
