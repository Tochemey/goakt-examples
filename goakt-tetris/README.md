# goakt-tetris

A browser-playable single-player **Tetris** built on [GoAkt](https://github.com/Tochemey/goakt) — every running game is a per-connection actor, snapshots are pushed to the browser over a WebSocket, and matches can be transparently distributed across a multi-node cluster.

This example is the demo case for "what does a real-time, stateful, multi-user app look like when you let GoAkt own concurrency and placement?"

---

## What GoAkt features it shows

| Feature                                                                     | Where it lives                       |
|-----------------------------------------------------------------------------|--------------------------------------|
| Tick-driven actor (`system.Schedule`)                                       | `MatchActor` in `match.go`           |
| Per-connection actor with bounded I/O lifetime                              | `PlayerSessionActor` in `session.go` |
| Watch / `*actor.Terminated` for owner-death cleanup                         | `MatchActor.Receive` + helpers       |
| `SpawnSingleton` for one-instance-per-cluster control plane                 | `MatchFactory` in `matchmaker.go`    |
| `SpawnOn` + `WithPlacement(LeastLoad)` for cluster-aware actor placement    | `MatchFactory`                       |
| `WithRelocationDisabled` for state actors that can't be migrated mid-flight | `MatchFactory`                       |
| Cluster-aware `ActorOf` for cross-node lookup                               | `gateway.go::requestMatch`           |
| CBOR serializers registered for cross-node message types                    | `main.go::buildActorSystem`          |
| Static discovery (configurable seed peer list)                              | `main.go::peerList`                  |

---

## Architecture

```
   ┌────────────────────────┐         ┌────────────────────────┐
   │ Node A — :8080  :9000  │         │ Node B — :8081  :9100  │
   │                        │         │                        │
   │  PlayerSessionActor    │── Tell ─┤   MatchActor           │
   │  (one per WS conn)     │ ←Snap── │   (may live on either) │
   │      ↑                 │         │                        │
   │      │ subscribe       │ ←Ask──  │   MatchFactory         │
   │ HTTP/WS gateway        │  ──────►│   (cluster singleton)  │
   └────────────────────────┘         └────────────────────────┘
                ▲
                │ WebSocket
                │
            Browser (three.js-free, plain 2D canvas)
```

- A WS connect → gateway asks the cluster's `matchmaker` singleton for a fresh match → matchmaker `SpawnOn`s a `MatchActor` somewhere in the cluster → gateway spawns a *local* `PlayerSessionActor` and hands it the match PID.
- The session subscribes to the match (cross-node-safe via `ctx.Sender()`).
- The match's 60 Hz tick advances physics; each tick `Tell`s a `*Snapshot` to every subscriber.
- When the WS closes → session shuts down → match observes the `*Terminated` → match self-stops and cancels its tick schedule.

Open ideas the example doesn't yet cover but maps cleanly to GoAkt: a persistent `PlayerProfileGrain` keyed on userID for MMR/stats across sessions, a `/watch` endpoint that subscribes a session to an existing match for spectator fan-out, and client-side prediction + server reconciliation for input-latency hiding.

---

## Quick start

### Single node

```bash
make run
```

Then open <http://localhost:8080>.

### Two-node cluster (docker compose)

```bash
make cluster-up      # builds the image and brings up node-a + node-b
make cluster-logs    # follow both nodes' logs in another terminal
make cluster-down    # stop everything and remove the network
```

Open <http://localhost:8080> *or* <http://localhost:8081> — the matchmaker singleton will place each game on whichever node is currently least-loaded, regardless of which one your browser connects to. Watch the server logs (`make cluster-logs`) for `match=match.<uuid> (local)` vs `(remote@node-b:9000)` annotations.

The two containers share a private docker network and resolve each other by hostname (`node-a`, `node-b`). Internally they use a uniform port layout (HTTP 8080, remoting 9000, discovery 9001, peers 9002); only the HTTP port is remapped to a distinct host port per node.

### Two-node cluster (local processes, no docker)

Faster than docker for an iterative dev loop. Run each in its own terminal:

```bash
make local-node-a    # terminal 1 — node A on default ports
make local-node-b    # terminal 2 — node B on shifted ports
make local-stop      # free any leaked ports between runs
```

This path mirrors the docker setup port-for-port, just with the cluster traffic carried over `127.0.0.1` instead of a docker network.

---

## Ports

| Purpose                              | Node A default | Node B default | Configurable via   |
|--------------------------------------|----------------|----------------|--------------------|
| HTTP / WebSocket (browser-facing)    | `8080`         | `8081`         | `--http-port`      |
| Remote actor gRPC (inter-node Tells) | `9000`         | `9100`         | `--remoting-port`  |
| Cluster gossip (discovery)           | `9001`         | `9101`         | `--discovery-port` |
| Cluster peer state-sync              | `9002`         | `9102`         | `--peers-port`     |

Discovery is **static** for this example. Each node is launched with `--peers "host:9001,host:9101"` listing every gossip endpoint in the cluster (including its own). Production setups would swap in `discovery/kubernetes` or `discovery/dnssd` — `main.go::buildActorSystem` is the one function that'd need to change.

---

## Controls

| Key   | Action                    |
|-------|---------------------------|
| ← / → | Move piece left/right     |
| ↑     | Rotate                    |
| ↓     | Soft drop (hold)          |
| Space | Hard drop                 |
| P     | Pause / resume            |
| R     | Restart (after game over) |

---

## Code layout

| File                             | Responsibility                                                                                                                |
|----------------------------------|-------------------------------------------------------------------------------------------------------------------------------|
| `main.go`                        | Flag parsing, actor system bootstrap (remote + cluster + serializers), HTTP server, singleton matchmaker spawn                |
| `gateway.go`                     | WebSocket upgrade, per-connection actor lifecycle, matchmaker request, reader loop that bridges WS frames into actor messages |
| `matchmaker.go`                  | `MatchFactory` cluster singleton; spawns `MatchActor`s with `SpawnOn`                                                         |
| `match.go`                       | `MatchActor` — the game itself (grid, gravity, line-clearing, scoring, subscriber broadcast, owner-death cleanup)             |
| `session.go`                     | `PlayerSessionActor` — owns the `*websocket.Conn`; forwards `PlayerInput`; writes `Snapshot` JSON to the WS inline            |
| `types.go`                       | Wire-protocol constants, message types, board dimensions                                                                      |
| `web/main.ts`                    | **TypeScript source** for the browser client — types mirror `types.go`                                                        |
| `web/main.js`                    | Build artifact (gitignored). Generated by `make web` or the Docker `web-builder` stage; embedded into the Go binary           |
| `web/index.html`                 | Boot HTML; loads `main.js`                                                                                                    |
| `tsconfig.json`                  | TS compiler config (target ES2020, strict, in-place compile inside `web/`)                                                    |
| `Dockerfile` + `docker-compose.yaml` | Two-node cluster image + service definition                                                                                   |

`session.go` and `match.go` are **byte-identical between single-node and cluster mode** — that's the location-transparency story.

---

## Editing the web client

The browser code is **TypeScript** (`web/main.ts`) and compiles in place to **JavaScript** (`web/main.js`) which is what Go embeds into the binary. Source + output share a single folder; the Go `//go:embed` line in `main.go` is scoped to `web/index.html` and `web/main.js` so the `.ts` file isn't carried into the binary.

`web/main.js` is a **build artifact** (gitignored), produced two ways:

| Path | How `main.js` is built |
|---|---|
| `make build`, `make run`, `make local-node-*` | The `build` target depends on `web`, which calls `npx --package=typescript@5.6 -y -- tsc -p .`. Requires Node.js (≥ 18) on your machine. |
| `make cluster-up` (Docker) | The Dockerfile has a dedicated `web-builder` stage on `node:22-alpine` that runs the same `tsc` command, then copies the result into the Go build stage. No Node.js needed on the host. |

When iterating on the client locally, `make web` re-runs the compile on its own. TS types for the wire payload — `Piece`, `Snapshot`, `Action` — mirror the Go structs in `types.go`; keeping them in sync is a manual step (the protocol is small enough that codegen isn't worth it).

---

## License

MIT, same as the rest of the goakt-examples repo.
