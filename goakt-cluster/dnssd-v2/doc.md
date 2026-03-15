# Selfmanaged Discovery - Go Structs Implementation

This package provides an account service implementation using:

- **Go structs** instead of protocol buffers for messages and API
- **Selfmanaged discovery** provider (UDP broadcast on LAN; no peer config)
- **GoAkt logging disabled** (`log.DiscardLogger`) with `fmt.Printf`/`fmt.Println` for server-side user output
- **HTTP JSON API** instead of Connect RPC

## API Endpoints

| Method | Path                    | Description                                                                         |
|--------|-------------------------|-------------------------------------------------------------------------------------|
| POST   | `/accounts`             | Create account. Body: `{"create_account":{"account_id":"x","account_balance":100}}` |
| POST   | `/accounts/{id}/credit` | Credit account. Body: `{"balance":50}`                                              |
| GET    | `/accounts/{id}`        | Get account                                                                         |
| GET    | `/docs` or `/swagger`   | Swagger UI (interactive API docs)                                                   |
| GET    | `/openapi.yaml`         | OpenAPI 3 spec                                                                      |

## Environment Variables

| Variable                                        | Description            | Default          |
|-------------------------------------------------|------------------------|------------------|
| PORT                                            | HTTP port              | 50051            |
| CLUSTER_NAME                                    | Cluster identifier     | accounts-cluster |
| SYSTEM_NAME                                     | Actor system name      | accounts         |
| DISCOVERY_PORT                                  | Memberlist/gossip port | required         |
| PEERS_PORT                                      | Olric peers port       | required         |
| REMOTING_PORT                                   | Actor remoting port    | required         |
| BROADCAST_PORT                                  | UDP broadcast port     | 7947             |
| TRACE_URL                                       | OTLP trace endpoint    | localhost:4317   |
| DB_HOST, DB_PORT, DB_NAME, DB_USER, DB_PASSWORD | PostgreSQL config      | required         |

## Run

```bash
# Set required env vars (see .env.example or dnssd docker-compose)
go run ./goakt-actors-cluster/selfmanaged run
```

## Docker Compose

Build the image and run the full stack (3 account nodes, load balancer, PostgreSQL, Prometheus, Jaeger, OTLP collector):

```bash
earth selfmanaged-image
cd goakt-actors-cluster/selfmanaged
docker compose up -d tracer prometheus collector db
docker compose up -d lb accounts1 accounts2 accounts3
```

- **Load balancer**: `localhost:8000` → round-robins to accounts1/2/3
- **Swagger UI**: `http://localhost:8000/docs` or `http://localhost:8000/swagger`
- **Prometheus**: `localhost:9090`
- **Jaeger UI**: `localhost:16686`
- **PostgreSQL**: `localhost:5432`

## Build

```bash
earth compile-selfmanaged
earth selfmanaged-image
```
