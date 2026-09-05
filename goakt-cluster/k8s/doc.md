# k8s: GoAkt Cluster with Kubernetes Discovery

This example demonstrates a GoAkt actor cluster on **Kubernetes** with PostgreSQL
persistence and OpenTelemetry tracing. A process-wide `--codec` / `CODEC` switch
selects an **exclusive** stack — never both at once:

| Mode                | Flag / env                      | Client API                  | Actor remoting                          |
|---------------------|---------------------------------|-----------------------------|-----------------------------------------|
| Full CBOR (default) | `--codec cbor` / `CODEC=cbor`   | HTTP/JSON OpenAPI + Swagger | CBOR-encoded Go structs in `messages/`  |
| Full protobuf       | `--codec proto` / `CODEC=proto` | Connect/gRPC only           | `internal/samplepb` via ProtoSerializer |

Actors always speak the domain model in `messages/`. The [`wire`](./wire) package
maps that model to protobuf when the process runs in proto mode. All pods in the
StatefulSet must use the **same** codec.

## Architecture

```
                    ┌──────────────────┐
                    │ Nginx (NodePort) │
                    │  HTTP or gRPC    │
                    └────────┬─────────┘
                             │
         ┌───────────────────┼───────────────────┐
         │                   │                   │
         ▼                   ▼                   ▼
┌────────────────┐  ┌────────────────┐  ┌────────────────┐
│ accounts-0     │  │ accounts-1     │  │ accounts-2     │
│ (StatefulSet)  │  │ (StatefulSet)  │  │ (StatefulSet)  │
│ Actor + API    │  │ Actor + API    │  │ Actor + API    │
└───────┬────────┘  └───────┬────────┘  └───────┬────────┘
        │                   │                   │
        │ OTLP traces       │ OTLP traces       │ OTLP traces
        └───────────────────┼───────────────────┘
                            │
               ┌────────────┴────────────┐
               ▼                         ▼
┌──────────────────┐  ┌──────────────────┐
│  OTEL Collector  │  │    PostgreSQL    │
│ (OTLP → Jaeger)  │  │  (Persistence)   │
└────────┬─────────┘  └──────────────────┘
         │
         ▼
┌──────────────────┐
│      Jaeger      │
│    (Trace UI)    │
└──────────────────┘
```

## Prerequisites

| Tool        | Purpose                              | Installation                                                                     |
|-------------|--------------------------------------|----------------------------------------------------------------------------------|
| **Kind**    | Local Kubernetes cluster             | [kind.sigs.k8s.io](https://kind.sigs.k8s.io/docs/user/quick-start/#installation) |
| **kubectl** | Kubernetes CLI                       | [kubectl install](https://kubernetes.io/docs/tasks/tools/)                       |
| **Docker**  | Container runtime (required by Kind) | [docker.com](https://docs.docker.com/get-docker/)                                |
| **grpcurl** | Proto-mode API tests only            | `go install github.com/fullstorydev/grpcurl/cmd/grpcurl@latest`                  |

## Quick Start (CBOR / HTTP)

```bash
cd goakt-cluster/k8s
make cluster-create         # one-time: create the Kind cluster
make deploy                 # build image, load into Kind, deploy (CODEC=cbor by default)
make port-forward           # blocks — run in another terminal
make test                   # needs port-forward running
make test-resilience        # kill a random accounts pod and re-verify
```

API base URL: `http://localhost:8080`  
Swagger UI: [http://localhost:8080/docs](http://localhost:8080/docs)

### Smoke test (cbor)

```bash
curl -s -X POST http://localhost:8080/accounts \
  -H 'Content-Type: application/json' \
  -d '{"createAccount":{"accountId":"acc-1","accountBalance":100}}'

curl -s -X POST http://localhost:8080/accounts/acc-1/credit \
  -H 'Content-Type: application/json' \
  -d '{"balance":50}'

curl -s http://localhost:8080/accounts/acc-1
```

## Full protobuf mode

Tear down first, then redeploy every pod with the proto stack and the gRPC
nginx config. Running `make deploy CODEC=proto` over a live cbor cluster would
roll the pods one at a time, and pods on different codecs cannot talk to each
other mid-roll:

```bash
make cluster-down
make deploy CODEC=proto
make port-forward                # blocks — run in another terminal
make test CODEC=proto
make test-resilience CODEC=proto
```

`CODEC` selects the nginx config and the test scripts, so pass it to every
target that touches them (`deploy`, `test`, `test-resilience`).

### Smoke test (proto)

```bash
grpcurl -plaintext -proto ../../protos/sample/service.proto \
  -d '{"createAccount":{"accountId":"acc-1","accountBalance":100}}' \
  localhost:8080 samplepb.AccountService/CreateAccount

grpcurl -plaintext -proto ../../protos/sample/service.proto \
  -d '{"creditAccount":{"accountId":"acc-1","balance":50}}' \
  localhost:8080 samplepb.AccountService/CreditAccount

grpcurl -plaintext -proto ../../protos/sample/service.proto \
  -d '{"accountId":"acc-1"}' \
  localhost:8080 samplepb.AccountService/GetAccount
```

## Wire formats

| Codec   | Client edge                      | Remoting types                        | Serializer              |
|---------|----------------------------------|---------------------------------------|-------------------------|
| `cbor`  | OpenAPI HTTP/JSON                | `messages.*` Go structs               | `remote.CBORSerializer` |
| `proto` | Connect/gRPC (`samplepbconnect`) | `internal/samplepb` protobuf messages | default ProtoSerializer |

The CLI flag is `--codec`; in Kubernetes the StatefulSet sets `CODEC`. Mixing
codecs across pods is unsupported.

## Makefile targets

Run `make` (or `make help`) in `goakt-cluster/k8s/` to list them.

| Target                     | Description                                                       |
|----------------------------|-------------------------------------------------------------------|
| `make deploy`              | Build image, load into Kind, deploy — `image` + `cluster-up`      |
| `make cluster-create`      | Create the Kind cluster `goakt-k8s`                               |
| `make cluster-recreate`    | Delete and recreate the cluster (needed after kind-config changes) |
| `make cluster-delete`      | Delete the Kind cluster                                           |
| `make image`               | Build `accounts:dev-k8s` and load it into Kind                    |
| `make cluster-up`          | Deploy PostgreSQL, Jaeger, OTEL Collector, accounts, nginx        |
| `make cluster-down`        | Remove all deployments (keeps the Kind cluster)                   |
| `make status`              | Cluster info, pods, services, and the active `CODEC`              |
| `make test`                | API tests for the selected `CODEC`                                |
| `make test-resilience`     | Create accounts, verify, kill a random pod, re-verify             |
| `make logs`                | Tail logs from the accounts pods                                  |
| `make port-forward`        | nginx → localhost:8080 (blocking)                                 |
| `make port-forward-jaeger` | Jaeger UI → localhost:16686 (blocking)                            |
| `make dashboard`           | Install if needed, print a login token, run `kubectl proxy`       |
| `make dashboard-install`   | Install the Kubernetes dashboard (one-time)                       |

Overridable variables: `CODEC` (`cbor`), `CLUSTER_NAME` (`goakt-k8s`),
`IMAGE_NAME` (`accounts:dev-k8s`).

## Project layout

```
k8s/
├── actors/          # AccountEntity (domain messages + persistence)
├── api/             # OpenAPI spec + generated HTTP types (cbor mode)
├── cmd/             # cobra CLI; --codec on run
├── db/migrations/   # Postgres schema
├── deploy/          # Kind manifests; nginx-config.yaml vs nginx-config-proto.yaml
├── domain/          # Encapsulated account state
├── messages/        # Actor command/reply Go structs
├── persistence/     # Postgres store extension
├── scripts/         # test-api*.sh and test-resilience*.sh, one pair per codec
├── service/         # Exclusive HTTP or Connect façade
└── wire/            # Codec Encode/Decode + remoting registration
```

## Environment variables

| Variable                                          | Default                      | Description                             |
|---------------------------------------------------|------------------------------|-----------------------------------------|
| `CODEC`                                           | `cbor`                       | Exclusive wire mode (`cbor` or `proto`) |
| `PORT`                                            | `50051`                      | Client API listen port                  |
| `DISCOVERY_PORT` / `PEERS_PORT` / `REMOTING_PORT` | (required)                   | Cluster ports                           |
| `DB_*`                                            | (required)                   | Postgres connection                     |
| `OTEL_EXPORTER_OTLP_ENDPOINT`                     | `http://otel-collector:4318` | Trace export                            |
| `OTEL_SERVICE_NAME`                               | `accounts`                   | Trace service name                      |
