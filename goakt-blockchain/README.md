# GoAkt Blockchain

A GoAkt port of [scalachain](https://github.com/elleFlorio/scalachain), the actor-based blockchain from the freeCodeCamp article [How to build a simple actor-based blockchain](https://www.freecodecamp.org/news/how-to-build-a-simple-actor-based-blockchain-aac1e996c177/), running as a GoAkt cluster on Kubernetes with the Kubernetes discovery provider.

## Architecture

Three pods form one GoAkt cluster. Like a real blockchain network, **every pod holds a full copy of the ledger**, stored in its own embedded [Pebble](https://github.com/cockroachdb/pebble) database — the LSM key-value store go-ethereum uses for its chain data — on a per-pod volume:

```
                      curl
                       │
                       ▼
                   ┌───────┐
                   │ nginx │  round-robins the REST API across the pods:
                   └───┬───┘  reads are answered by the pod's local replica,
                       │      writes are routed to the node0 singleton
                       ▼
 ┌─ blockchain-0 ─────────┐  ┌─ blockchain-1 ─────────┐  ┌─ blockchain-2 ─────────┐
 │  ┌──────────────────┐  │  │                        │  │                        │
 │  │ node0  singleton │  │  │                        │  │                        │
 │  │ ┌──────┐ ┌─────┐ │  │  │                        │  │                        │
 │  │ │broker│ │miner│ │  │  │                        │  │                        │
 │  │ └──────┘ └─────┘ │  │  │                        │  │                        │
 │  └────────┬─────────┘  │  │                        │  │                        │
 │           │ publishes  │  │                        │  │                        │
 │  ┌────────▼─────────┐  │  │  ┌──────────────────┐  │  │  ┌──────────────────┐  │
 │  │ replica (pebble) │  │  │  │ replica (pebble) │  │  │  │ replica (pebble) │  │
 │  └────────▲─────────┘  │  │  └────────▲─────────┘  │  │  └────────▲─────────┘  │
 └───────────┼────────────┘  └───────────┼────────────┘  └───────────┼────────────┘
             │                           │                           │
             └──────────── blocks.minted pub/sub topic ──────────────┘
```

- **Node** (`node0`) is a **cluster singleton**: exactly one lives in the whole cluster, and when its host pod dies it is respawned on a surviving pod. It supervises the broker and the miner, mines on top of its host pod's chain replica, and — being the only miner — is the source of truth for new blocks, so no forks can occur.
- **Broker** keeps the transactions waiting to be included in a block.
- **Miner** computes the proof-of-work. It is a two-state machine built with `Become`/`UnBecome`: while busy it rejects new mining requests but still validates proofs. The computation runs off the mailbox through `PipeTo`, which delivers the `ProofFound` result back to the node.
- **Replica** (one per pod, `chain-<pod>`) maintains the pod's full copy of the chain in its local Pebble store. Mined blocks fan out on a **pub/sub topic** (`blocks.minted`); each replica validates an announced block against its tip (linkage, proof-of-work, and hash are all checked) before appending and persisting it. When a replica starts — pod restart or fresh pod — it **catches up** on missed blocks from its peer replicas, and the singleton resynchronizes its local replica before mining resumes after a failover. Replicas are spawned with `WithRelocationDisabled()`: a replica is bound to its pod's volume, so it must never be redeployed on another pod when its host dies — the pod's replacement rebuilds it from the local store instead.

The chain itself matches the article: a genesis block (index 0, hash `"1"`, proof 100), block hashes computed as the SHA-256 of the block's JSON representation, and a proof-of-work that brute-forces a proof `p` such that `SHA-256(lastHash + p)` starts with four zeros. Mining a block rewards the node with a coinbase transaction of 100 coins.

The cluster pieces:

- **Kubernetes discovery**: pods find each other through the Kubernetes API, filtering on the pod labels and reading the named container ports (`discovery-port`, `peers-port`, `remoting-port`). RBAC grants the pods `get/watch/list` on pods.
- **Cluster singleton**: every pod calls `SpawnSingleton` on boot; one wins and the others share the running `node0`.
- **Location transparency**: the REST API runs on every pod behind nginx. Chain reads are answered by the pod's local replica without leaving the pod; writes resolve `node0` by name with `ActorOf` and reach it over remoting.
- **Serialization**: messages are plain Go structs; the ones that may travel between pods are registered with `remote.WithSerializables` and encoded with CBOR.

## Run it

Prerequisites: [Kind](https://kind.sigs.k8s.io/docs/user/quick-start/#installation), [kubectl](https://kubernetes.io/docs/tasks/tools/), and [Docker](https://docs.docker.com/get-docker/).

```bash
cd goakt-blockchain
make cluster-create
make deploy
make port-forward     # in another terminal
make test
```

The REST API is then reachable on `http://localhost:8080`:

```bash
# submit a couple of transactions
curl -X POST localhost:8080/transactions -d '{"sender":"alice","recipient":"bob","value":40}'
curl -X POST localhost:8080/transactions -d '{"sender":"bob","recipient":"carol","value":15}'

# see them pending
curl localhost:8080/transactions

# mine a block (returns immediately; watch the logs with `make logs`)
curl localhost:8080/mine

# the chain now has a new block containing both transactions plus the mining reward
curl localhost:8080/status

# check a proof-of-work solution against the current last block
curl "localhost:8080/validate?proof=72608"
```

## Failover

`make test-resilience` mines a block, finds the pod hosting the `node0` singleton, deletes it, and verifies that the singleton is respawned on a surviving pod, that **the chain survives the failover** — the new host pod already holds the full ledger in its local Pebble store — and that mining keeps working.
