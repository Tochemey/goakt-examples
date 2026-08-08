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
| **Earthly** | Reproducible builds                  | [earthly.dev](https://earthly.dev/get-earthly)                                   |
| **Docker**  | Container runtime (required by Kind) | [docker.com](https://docs.docker.com/get-docker/)                                |
| **grpcurl** | Proto-mode API tests only            | `go install github.com/fullstorydev/grpcurl/cmd/grpcurl@latest`                  |

## Quick Start (CBOR / HTTP)

```bash
cd goakt-cluster/k8s
make cluster-create
make deploy                 # CODEC=cbor by default
make port-forward           # in another terminal
make test
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

Redeploy every pod with the proto stack and the gRPC nginx config:

```bash
make cluster-down
make deploy CODEC=proto
make port-forward
make test CODEC=proto
```

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

| Target                                | Description                                            |
|---------------------------------------|--------------------------------------------------------|
| `make deploy`                         | Build image, load into Kind, deploy (respects `CODEC`) |
| `make deploy CODEC=proto`             | Full protobuf stack                                    |
| `make test` / `make test CODEC=proto` | Mode-appropriate API tests                             |
| `make test-resilience`                | Kill a node and re-verify                              |
| `make port-forward`                   | nginx → localhost:8080                                 |
| `make port-forward-jaeger`            | Jaeger UI → localhost:16686                            |
| `make cluster-down`                   | Tear down deployments                                  |

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
├── scripts/         # test-api.sh (cbor) / test-api-proto.sh (proto)
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
