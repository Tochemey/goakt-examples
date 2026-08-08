# DNS-SD Discovery

An account service clustered across three nodes that find each other through **DNS-SD**. Each node resolves a single
domain name to the addresses of its peers, so no node needs a static peer list.

This example uses:

- **DNS-SD discovery** — nodes join by resolving `DOMAIN_NAME` (CoreDNS serves it in the compose stack)
- **Two client APIs on one port** — a gRPC-compatible Connect RPC service and a REST/JSON service
- **Go structs** for actor messages, serialized with `remote.CBORSerializer`
- **PostgreSQL persistence** — each `AccountEntity` recovers its balance on start through a `Store` extension
- **OpenTelemetry** traces and Prometheus metrics

## Two APIs, one service

Both surfaces listen on the same port and reach the same actors.

### REST / JSON

Generated from `api/openapi.yaml` with `oapi-codegen`.

| Method | Path                    | Description                                                                         |
|--------|-------------------------|-------------------------------------------------------------------------------------|
| POST   | `/accounts`             | Create account. Body: `{"create_account":{"account_id":"x","account_balance":100}}` |
| POST   | `/accounts/{id}/credit` | Credit account. Body: `{"balance":50}`                                              |
| GET    | `/accounts/{id}`        | Get account                                                                         |
| GET    | `/docs` or `/swagger`   | Swagger UI (interactive API docs)                                                   |
| GET    | `/openapi.yaml`         | OpenAPI 3 spec                                                                      |

### gRPC / Connect RPC

Generated from [protos/sample/service.proto](../../protos/sample/service.proto). Reachable with any gRPC client,
a Connect client, or gRPC-Web.

| Procedure                                | Description    |
|------------------------------------------|----------------|
| `/samplepb.AccountService/CreateAccount` | Create account |
| `/samplepb.AccountService/CreditAccount` | Credit account |
| `/samplepb.AccountService/GetAccount`    | Get account    |

### How they share a port

`service/service.go` mounts the Connect handler on its generated `/samplepb.AccountService/` prefix and layers the
REST routes over the same `http.ServeMux`, so the two can never collide. The server enables both HTTP/1.1 and
unencrypted HTTP/2 through `http.Server.Protocols`, because gRPC requires HTTP/2 and there is no TLS here — that
replaces the deprecated `golang.org/x/net/http2/h2c` wrapper.

Neither surface holds business logic. `AccountService` owns the actor calls (`create`, `credit`, `get`), the REST
handlers and the `rpcService` in `service/rpc.go` are thin translation layers over them, and the actors only ever see
the plain Go structs in `messages/`. Protobuf exists at the RPC edge and nowhere else, so the two APIs cannot drift
apart in behaviour.

## Environment Variables

Variables without a default are required.

| Variable                                        | Description             | Default          |
|-------------------------------------------------|-------------------------|------------------|
| PORT                                            | HTTP port               | 50051            |
| DOMAIN_NAME                                     | DNS-SD name to resolve  | required         |
| SYSTEM_NAME                                     | Actor system name       | accounts         |
| DISCOVERY_PORT                                  | Memberlist/gossip port  | required         |
| PEERS_PORT                                      | Olric peers port        | required         |
| REMOTING_PORT                                   | Actor remoting port     | required         |
| TRACE_URL                                       | OTLP trace endpoint     | localhost:4317   |
| DB_HOST, DB_PORT, DB_NAME, DB_USER, DB_PASSWORD | PostgreSQL config       | required         |
| LOG_LEVEL                                       | debug, info, warn, error | debug           |

## How to run it

1. install [Earthly](https://earthly.dev/get-earthly)
2. clone the repository
3. run at the root of the cloned repository `earthly +dnssd-image`
4. `cd goakt-cluster/dnssd`
5. run `docker compose up -d tracer prometheus collector db`
6. run `docker compose up -d coredns lb accounts1 accounts2 accounts3` to start the cluster
7. run `docker compose ps` to list the running instances
8. To stop the cluster run `docker compose down -v --remove-orphans`

Then exercise either API through the load balancer.

REST:

```bash
curl -X POST localhost:8000/accounts \
  -d '{"create_account":{"account_id":"acc-1","account_balance":100}}'

curl -X POST localhost:8000/accounts/acc-1/credit -d '{"balance":50}'

curl localhost:8000/accounts/acc-1
```

gRPC (same port, same accounts):

```bash
grpcurl -plaintext -d '{"create_account":{"account_id":"acc-2","account_balance":100}}' \
  localhost:8000 samplepb.AccountService/CreateAccount

grpcurl -plaintext -d '{"credit_account":{"account_id":"acc-2","balance":50}}' \
  localhost:8000 samplepb.AccountService/CreditAccount

grpcurl -plaintext -d '{"account_id":"acc-2"}' \
  localhost:8000 samplepb.AccountService/GetAccount
```

An account created over one API is readable over the other — they are the same actors.

- **Load balancer**: `localhost:8000` → the three account nodes
- **Swagger UI**: `http://localhost:8000/docs` or `http://localhost:8000/swagger`
- **Prometheus**: `localhost:9090`
- **Jaeger UI**: `localhost:16686`
- **PostgreSQL**: `localhost:5432`

## How discovery works here

`coredns.Corefile` maps `accounts.dnssd.local` to the three container IPs, and each node is configured with
`dns: 172.28.0.10` so it queries CoreDNS. On start, a node resolves the name, gets its peers, and joins the cluster.

nginx does the same thing for client traffic: `nginx.conf` points at CoreDNS with `resolver 172.28.0.10` and declares
the upstream as `server accounts.dnssd.local:50051 resolve`, so it load-balances across whatever addresses the name
resolves to rather than a hardcoded node list. Adding a node means adding a DNS record — nothing else changes.

## Regenerating the API

`api/api.gen.go` is generated from `api/openapi.yaml`:

```bash
earthly +opengen
```

## Build

```bash
earthly +compile-dnssd
earthly +dnssd-image
```
