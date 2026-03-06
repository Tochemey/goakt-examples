# k8s-ebpf: GoAkt Cluster with eBPF Tracing on Kubernetes

This example demonstrates a GoAkt actor cluster running on **Kubernetes** with **goakt-ebpf** as a sidecar for zero-instrumentation eBPF tracing. It extends the k8s-v2 example with:

- **goakt-ebpf sidecar** in each pod for automatic actor-level tracing
- **Shared PID namespace** so the eBPF agent can attach to the accounts process
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
│ (OTLP → Jaeger)│  │   (Persistence)  │
└───────┬────────┘  └──────────────────┘
        │
        ▼
┌────────────────┐
│     Jaeger     │
│ (Trace UI)     │
└────────────────┘
```

Each pod runs two containers with `shareProcessNamespace: true`:
- **accounts** — the GoAkt application
- **goakt-ebpf** — the eBPF tracing agent, attached via `-exe /app/accounts`

The agent uses eBPF uprobes to capture actor spans (`doReceive`, `process`, remote messaging, etc.) and exports them via OTLP to the collector.

## Prerequisites

### Required Tools

| Tool        | Purpose                              | Installation                                                                     |
|-------------|--------------------------------------|----------------------------------------------------------------------------------|
| **Kind**    | Local Kubernetes cluster             | [kind.sigs.k8s.io](https://kind.sigs.k8s.io/docs/user/quick-start/#installation) |
| **kubectl** | Kubernetes CLI                       | [kubectl install](https://kubernetes.io/docs/tasks/tools/)                       |
| **Earthly** | Reproducible builds                  | [earthly.dev](https://earthly.dev/get-earthly)                                   |
| **Docker**  | Container runtime (required by Kind) | [docker.com](https://docs.docker.com/get-docker/)                                |

### Required Repositories

This example requires two repositories side by side:

```
goakt-ebpf/        # eBPF tracing agent
goakt-examples/    # This repository
```

The Makefile builds the goakt-ebpf Docker image from the sibling `goakt-ebpf` directory. Override the path with `EBPF_REPO`:

```bash
make image EBPF_REPO=/path/to/goakt-ebpf
```

## Quick Start

### 1. Create the Kind Cluster

```bash
cd goakt-actors-cluster/k8s-ebpf
make cluster-create
```

Creates a Kubernetes cluster named `goakt-k8s-ebpf`.

### 2. Build and Deploy

```bash
make deploy
```

Builds both images (accounts + goakt-ebpf), loads them into Kind, and deploys PostgreSQL, the accounts StatefulSet (3 replicas with eBPF sidecars), tracing stack, and Nginx.

---

## Testing the Service

### Step 1: Expose the API

```bash
make port-forward
```

API base URL: `http://localhost:8080`
Swagger UI: [http://localhost:8080/docs](http://localhost:8080/docs)

### Step 2: Verify Deployment

```bash
make status
```

Each pod should show `2/2` containers ready (accounts + goakt-ebpf).

### Step 3: Smoke Test

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

### Step 4: Load Test

```bash
make test
```

### Step 5: View Traces

```bash
make port-forward-jaeger
```

Open [http://localhost:16686](http://localhost:16686). Select the `goakt-ebpf` service to see actor-level spans (`actor.doReceive`, `actor.process`, remote messaging). Select `accounts` to see application-level HTTP spans.

### Step 6: Inspect eBPF Agent Logs

```bash
make logs-ebpf
```

## Makefile Reference

| Target                   | Description                                                      |
|--------------------------|------------------------------------------------------------------|
| `make deploy`            | Build images, load into Kind, and deploy all components          |
| `make cluster-create`    | Create a new Kind cluster                                        |
| `make cluster-delete`    | Delete the Kind cluster                                          |
| `make image`             | Build accounts + goakt-ebpf images and load into Kind            |
| `make image-accounts`    | Build accounts image only                                        |
| `make image-ebpf`        | Build goakt-ebpf image only                                      |
| `make cluster-up`        | Deploy PostgreSQL, accounts (with eBPF sidecar), tracing, nginx  |
| `make cluster-down`      | Remove all deployments                                           |
| `make status`            | Show cluster and pod status                                      |
| `make port-forward`      | Forward nginx to localhost:8080                                  |
| `make port-forward-jaeger` | Forward Jaeger UI to localhost:16686 (view traces)             |
| `make logs`              | Tail logs from accounts containers                               |
| `make logs-ebpf`         | Tail logs from goakt-ebpf sidecar containers                     |
| `make test`              | Run API integration tests (1000 accounts)                        |
| `make test-resilience`   | Create accounts, verify, kill node, re-verify                    |

## How the Sidecar Works

The pod spec enables shared PID namespace and runs goakt-ebpf alongside the accounts app:

```yaml
spec:
  shareProcessNamespace: true
  containers:
    - name: accounts
      image: accounts:dev-k8s-v2
    - name: goakt-ebpf
      image: goakt-ebpf:dev
      args: ["-exe", "/app/accounts"]
      securityContext:
        capabilities:
          add: [SYS_PTRACE, SYS_ADMIN, BPF, PERFMON]
      env:
        - name: OTEL_EXPORTER_OTLP_ENDPOINT
          value: "http://otel-collector:4318"
```

Key details:
- `shareProcessNamespace: true` lets the sidecar see the accounts process
- `-exe /app/accounts` tells the agent to find the target process by executable path (with shared PID namespace, PID is not fixed at 1)
- The agent needs `SYS_PTRACE` (process memory reading), `SYS_ADMIN` + `BPF` + `PERFMON` (eBPF uprobe attachment)
- The accounts binary must retain DWARF debug info (no `-ldflags="-s -w"`)

## Troubleshooting

### goakt-ebpf sidecar not starting

```bash
# Check pod events
kubectl describe pod accounts-0

# Check sidecar logs
make logs-ebpf
```

Common issues:
- **Missing capabilities** — Kind supports eBPF; check that the manifest has the correct `securityContext`
- **Binary stripped** — If the accounts binary was built with `-ldflags="-s -w"`, the agent cannot find struct field offsets

### No traces from goakt-ebpf

```bash
# Check sidecar logs for errors
kubectl logs accounts-0 -c goakt-ebpf

# Verify OTEL Collector is receiving
kubectl logs deployment/otel-collector
```

### Pods showing 1/2 ready

The goakt-ebpf sidecar has no readiness probe. If the accounts container isn't ready, the pod won't be fully ready. Check the accounts container specifically:

```bash
kubectl logs accounts-0 -c accounts
```

## Differences from k8s-v2

| Feature        | k8s-v2              | k8s-ebpf                          |
|----------------|---------------------|------------------------------------|
| Tracing        | App-level OTEL SDK  | App + eBPF actor-level tracing     |
| Pod containers | 1 (accounts)        | 2 (accounts + goakt-ebpf sidecar)  |
| PID namespace  | Default (isolated)  | Shared (`shareProcessNamespace`)   |
| Images         | accounts only       | accounts + goakt-ebpf              |
| Cluster name   | goakt-k8s-v2        | goakt-k8s-ebpf                     |

## License

MIT License - see repository root for details.
