# k8s: GoAkt Cluster with Kubernetes Discovery (gRPC/Connect)

This example demonstrates a GoAkt actor cluster running on **Kubernetes** with:

- **Protocol buffers** for actor messages
- **gRPC/Connect** API for client communication
- **Kind** (Kubernetes in Docker) for local development

## Prerequisites

| Tool        | Purpose                  | Installation                                                                     |
|-------------|--------------------------|----------------------------------------------------------------------------------|
| **Kind**    | Local Kubernetes cluster | [kind.sigs.k8s.io](https://kind.sigs.k8s.io/docs/user/quick-start/#installation) |
| **kubectl** | Kubernetes CLI           | [kubectl install](https://kubernetes.io/docs/tasks/tools/)                       |
| **Earthly** | Reproducible builds      | [earthly.dev](https://earthly.dev/get-earthly)                                   |
| **grpcurl** | gRPC testing (optional)  | `go install github.com/fullstorydev/grpcurl/cmd/grpcurl@latest`                  |

## Quick Start

### 1. Create the Kind Cluster

```bash
cd goakt-actors-cluster/k8s
make cluster-create
```

### 2. Build and Deploy

```bash
make deploy
```

### 3. Expose the API

In another terminal:

```bash
make port-forward
```

gRPC API is available at `localhost:8080`.

## Testing the Service

### Step 1: Expose the API

```bash
make port-forward
```

Leave running. Use additional terminals for the following steps.

### Step 2: Verify Deployment

```bash
make status
```

Optional: open the Kubernetes dashboard:

```bash
make dashboard
```

### Step 3: Smoke Test (Manual)

Using grpcurl (from repo root or with correct proto path):

```bash
# Create account
grpcurl -plaintext -import-path ../../protos -proto sample/service.proto \
  -d '{"create_account":{"account_id":"acc-001","account_balance":100}}' \
  localhost:8080 samplepb.AccountService/CreateAccount

# Get account
grpcurl -plaintext -import-path ../../protos -proto sample/service.proto \
  -d '{"account_id":"acc-001"}' \
  localhost:8080 samplepb.AccountService/GetAccount

# Credit account
grpcurl -plaintext -import-path ../../protos -proto sample/service.proto \
  -d '{"credit_account":{"account_id":"acc-001","balance":50}}' \
  localhost:8080 samplepb.AccountService/CreditAccount
```

### Step 4: Load Test (Automated)

```bash
make test
```

Creates 100 accounts, credits each, and verifies a sample. Customize:

```bash
NUM_ACCOUNTS=50 VERIFY_SAMPLE=10 make test
```

### Step 5: Inspect Logs

```bash
make logs
```

## Makefile Reference

| Target                   | Description                         |
|--------------------------|-------------------------------------|
| `make deploy`            | Build image, load into Kind, deploy |
| `make cluster-create`    | Create Kind cluster                 |
| `make cluster-delete`    | Delete Kind cluster                 |
| `make image`             | Build and load Docker image         |
| `make cluster-up`        | Deploy accounts and nginx           |
| `make cluster-down`      | Remove deployments                  |
| `make status`            | Show cluster and pod status         |
| `make port-forward`      | Forward nginx to localhost:8080     |
| `make dashboard`         | Access Kubernetes dashboard         |
| `make dashboard-install` | Install dashboard (one-time)        |
| `make logs`              | Tail logs from accounts pods        |
| `make test`              | Run gRPC integration tests          |

## Workflow

### First-Time Setup

```bash
make cluster-create
make deploy
```

Then follow [Testing the Service](#testing-the-service).

### Cleanup

```bash
make cluster-down              # Remove deployments
make cluster-down cluster-delete   # Remove everything
```

## Project Structure

```
k8s/
├── actors/           # Account actor (protobuf messages)
├── cmd/              # CLI entry point
├── k8s/              # Kubernetes manifests
│   ├── k8s.yaml            # StatefulSet, Service, RBAC
│   └── nginx-*.yaml        # Load balancer (gRPC)
├── scripts/          # Test scripts
│   └── test-api.sh        # grpcurl-based tests
├── service/          # gRPC/Connect API service
├── doc.md
└── Makefile
```

## Differences from k8s-v2

| Feature        | k8s (this)       | k8s-v2              |
|----------------|------------------|---------------------|
| Actor messages | Protocol buffers | Standard Go structs |
| API            | gRPC/Connect     | HTTP/JSON REST      |
| Persistence    | None             | PostgreSQL          |
| Nginx          | gRPC proxy       | HTTP proxy          |

## License

MIT License - see repository root for details.
