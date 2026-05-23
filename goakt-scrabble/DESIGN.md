# goakt-scrabble — design

A browser-playable, real-time multiplayer **Scrabble** game built on
[GoAkt](https://github.com/Tochemey/goakt). English ruleset; human-vs-
human and human-vs-AI. Each game room is a turn-based actor with full
Scrabble rules, an Appel/Jacobson DAWG move generator backing bot
opponents, and the bundled 172,823-word ENABLE dictionary.

This document is the architectural deep dive. See
[README.md](README.md) for game rules, controls, and quick-start usage.

---

## Modes of operation

The same binary runs in two modes — pick whichever matches what you
are doing:

| Mode                         | Command       | Use case                                                                                                                                       |
|------------------------------|---------------|------------------------------------------------------------------------------------------------------------------------------------------------|
| **Compile only**             | `make build`  | Day-to-day dev: type-check + produce the binary. The binary requires a Kubernetes cluster to start (discovery hits the in-cluster API server). |
| **Three-node K8s + ingress** | `make k8s-up` | The full cluster topology: 3 backend pods behind NGINX ingress on a local kind cluster.                                                        |

> **Vercel / Cloudflare Workers / Netlify Functions don't fit** this
> architecture: GoAkt rooms are long-lived stateful actors that hold a
> WebSocket. The natural fits are any platform that runs a long-lived
> container: a self-hosted VPS, Kubernetes, Railway, Render, fly.io, a
> kind cluster — anywhere `docker run` would land. This repo ships a
> Dockerfile + kind/K8s manifests; pushing the image to any of those
> targets is a one-step change to the manifest's `image:` field.

---

## Architecture

### Per-node actor topology

```
Browser (TS, SVG board, drag-and-drop tiles)
    │ WebSocket  /ws?name=...&room=ABCD
    ▼
┌────────────────────────────────────────────────────────────────┐
│ Node (one Go binary per pod / VM)                              │
│                                                                │
│  PlayerSessionActor  ──subscribes──►  Topic: room.<code>       │
│  (one per WS conn)                            ▲                │
│        │ Tell                                 │ Publish        │
│        ▼                                 ┌─────────────────┐   │
│  LobbyActor (cluster singleton)          │ RoomActor       │   │
│        │ SpawnOn(LeastLoad)              │ FSM via Become  │   │
│        ├────────────────────────────────►│ + child BotActor│   │
│        │                                 │ + Bag/Board/DAWG│   │
│        ▼                                 └─────────────────┘   │
│  PlayerProfileGrain (one per player ID)                        │
│                                                                │
│  Leaderboard CRDT — PNCounter keyed by player ID               │
└────────────────────────────────────────────────────────────────┘
```

### Cluster topology (three-node K8s mode)

```
                       Browser
                          │
                          │  http/ws  →  http://localhost
                          ▼
              NGINX Ingress Controller
              (control-plane node, :80)
                          │
                          │  affinity cookie — sends each browser's
                          │  next HTTP request (reconnect, reload)
                          │  back to its previous pod
                          ▼
                  scrabble Service
                  (ClusterIP)
                          │
              ┌───────────┼───────────┐
              ▼           ▼           ▼
        scrabble-0   scrabble-1   scrabble-2     ← StatefulSet
              ╲           │           ╱
               ╲          │          ╱             goakt cluster gossip
                ╲ peers discovered via the          on ports 9000/9001/9002
                  Kubernetes API (pod label         (kubernetes discovery)
                  app=scrabble); remoting uses
                  the headless-service per-pod
                  DNS: scrabble-N.scrabble-
                  headless.scrabble.svc.cluster.local
```

The browser sees one HTTP/WS endpoint. The LobbyActor singleton may be
hosted on any pod; `SpawnOn(LeastLoad)` places new RoomActors on the
least-loaded pod; cross-pod `Tell`s flow through the goakt remoting
gRPC port. Two distinct routing concerns sit side-by-side here:

- **Cross-player (handled by the goakt cluster).** Two players in the
  same room may have terminated their WebSockets on different pods. The
  `RoomActor` lives on exactly one pod; the other player's
  `PlayerSessionActor` sends `PlayerInput` to it via goakt remoting.
  This works regardless of ingress affinity.
- **Same player over time (handled by the affinity cookie).** A single
  WebSocket is one TCP socket, so frames within one connection are
  already pinned to whichever pod accepted the upgrade. What the
  affinity cookie does is route the *next* HTTP request from that
  browser — a reconnect after a network blip, a page reload, the WS
  upgrade for a new tab — back to the same pod, so an existing
  `PlayerSessionActor` can be reused (or its profile cache hit) rather
  than spinning up a fresh one elsewhere.

### Actors

| Actor                | Cardinality                 | Responsibility                                                                    |
|----------------------|-----------------------------|-----------------------------------------------------------------------------------|
| `LobbyActor`         | one per cluster (singleton) | Room directory by code; `JoinOrCreate`; `SpawnOn(LeastLoad)` for new rooms        |
| `RoomActor`          | one per active game         | FSM (waiting → playing → gameOver); owns Engine; turn timer; publishes events     |
| `BotActor`           | one per bot seat in a room  | On `YourTurn`, computes best move via `scrabble.GenerateMove`, sends back `Place` |
| `PlayerSessionActor` | one per WS connection       | Owns `*websocket.Conn`; subscribes to room topic; JSON-encodes events             |
| `PlayerProfileGrain` | one per player ID           | Persistent stats (games, wins, best move) across reconnects                       |

### Room FSM (Become / UnBecome)

Three primary behaviours plus a `paused` sub-state of `playing`:

```
   waiting        accept Join + AddBot/RemoveBot (owner) + Start (owner)
      │
      │ Start (owner, ≥2 players) → startGame
      ▼
   playing        ScheduleOnce(90s, turnTimeout) per turn
      │   ◄─────► paused  (any player can Pause; resume re-arms timer
      │           with the saved remaining duration; bot moves cancelled
      │           on pause + re-scheduled on resume)
      │
      │   accept Place / Exchange / Pass / Pause from currentPlayer
      │   on Place: Move.Validate → apply → publish MoveEvent → advanceTurn
      │   on bot turn: ScheduleOnce(700ms, YourTurn) → bot child
      │
      │ 6 consecutive scoreless turns OR (bag empty AND a rack empty)
      ▼
   gameOver       apply end-of-game rack penalty + out-bonus
                  increment winner's PNCounter via leaderboard PipeTo
                  ScheduleOnce(30s, Shutdown); accept PlayAgain
```

Invalid placements (wrong direction, gap with no existing tile, blank
without a chosen letter, formed word missing from the dictionary, etc.)
are auto-rejected at `Place` time — there is no separate challenge
mechanic. The user-facing tradeoff was discussed up front and is
documented under "Full ruleset (auto-reject)" in the original plan.

---

## Engine package (`./scrabble`)

Pure-Go, no actor dependencies, fully unit-testable. Shared by both the
`RoomActor` (validation) and the `BotActor` (move generation).

| File         | Responsibility                                                                                                                                                                             |
|--------------|--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `lang.go`    | `Language` type — alphabet (rune ↔ LetterID), point values, bag distribution. Currently only `English()` is registered; the type accommodates additional languages without engine changes. |
| `tile.go`    | `Tile` (LetterID + blank flag), helpers; placed blanks carry the chosen letter and still score 0                                                                                           |
| `bag.go`     | `Bag` — shuffled tile pool; `Draw(n)`, `Return(tiles)`, `Remaining()`. Injectable `*rand.Rand` so tests are reproducible                                                                   |
| `rack.go`    | `Rack` — up to 7 tiles; `Refill`, `Remove`, `Exchange`, `Add`                                                                                                                              |
| `board.go`   | `Board` — 15×15; premium-square layout (DL/TL/DW/TW + center); `At`, `IsEmptyBoard`, `Place`, `Clone`                                                                                      |
| `dict.go`    | `Dictionary` interface — `Contains(word []LetterID) bool`                                                                                                                                  |
| `dawg.go`    | `DAWG` — trie-shaped, sorted-edge nodes; implements `Dictionary`; exposes edge traversal for the move generator                                                                            |
| `move.go`    | `Move`, `Validate(board, dict, lang)`, scoring (premium squares + bingo bonus, cross-words), formed-words breakdown                                                                        |
| `endgame.go` | End-of-game detection + rack-penalty / out-bonus scoring                                                                                                                                   |
| `bot.go`     | `BestMove(board, rack, dawg, lang)` — Appel/Jacobson move generator; returns highest-scoring legal `Move` or nil (caller passes)                                                           |

### Tile representation

```go
type LetterID uint8           // 0..N-1 index into a Language.Alphabet
type Tile struct {
    Letter LetterID           // the visible letter (after blank choice)
    Blank  bool                // true if this tile came from a blank
}
```

Blanks score 0 but otherwise behave as their chosen letter. This is
encoded once, in `Square.Score()` (board) and `Tile.Score()` (rack).

### Move-generation algorithm

Appel & Jacobson, *The World's Fastest Scrabble Program* (1988). For
each anchor square (the centre on an empty board; otherwise every
empty square horizontally or vertically adjacent to a placed tile):

1. For each direction (horizontal, vertical):
   1. Compute the left-part limit (distance to the previous tile or
      anchor cluster).
   2. Recurse: extend left with rack tiles (using the DAWG to prune
      impossible prefixes); at each step, extend right through the
      anchor, validating cross-words against the DAWG, until a DAWG
      terminal is reached and the placement is legal.
2. Score every legal placement using `Move.Score`; keep the best.

GADDAG would be slightly faster than DAWG for this; DAWG is enough for
v1 bot strength and is simpler to build.

### Cross-word validation

For every newly-placed tile, walk the perpendicular axis to collect
the cross-word (if any). Each cross-word must be in the dictionary;
each contributes its own score (with the new tile's premium-square
multiplier applied once).

---

## Dictionary

The bundled `dict/en.txt` is the full ENABLE word list (172,823 words,
public domain — used by Words with Friends). Players can drop in a
tournament list (TWL or SOWPODS) and rebuild for tournament-grade play.
See the README for the swap instructions and licensing notes.

---

## Wire protocol (`types.go`)

### Inbound (`browser → session`)

| `type`      | Fields                                    | Notes                                        |
|-------------|-------------------------------------------|----------------------------------------------|
| `start`     | —                                         | Owner only, waiting phase                    |
| `addBot`    | —                                         | Owner only, waiting phase                    |
| `removeBot` | `seat: int`                               | Owner only, waiting phase                    |
| `place`     | `placements: [{row, col, letter, blank}]` | Current player only, playing phase           |
| `exchange`  | `indices: [int]`                          | Current player only; bag must have ≥ 7 tiles |
| `pass`      | —                                         | Current player only                          |
| `pause`     | —                                         | Any player, playing phase                    |
| `resume`    | —                                         | Any player, paused phase                     |
| `chat`      | `text`                                    | Anyone in the room                           |
| `playAgain` | —                                         | gameOver phase                               |

### Outbound (`session → browser`)

| `type`     | Payload (selected fields)                                                                                                           |
|------------|-------------------------------------------------------------------------------------------------------------------------------------|
| `joined`   | `room`, `language`, `playerID`, `owner: bool`, `profile`, `leaderboard`                                                             |
| `state`    | `phase`, `board[15][15]`, `yourRack[]`, `players:[{id,name,score,rackSize,bot}]`, `currentID`, `ownerID`, `bagRemaining`, `timerMs` |
| `move`     | `playerID`, `name`, `placements`, `words:[{word,score}]`, `score`, `newTotal`, `bingo`                                              |
| `chat`     | `from`, `text`                                                                                                                      |
| `error`    | `message`                                                                                                                           |
| `gameOver` | `winnerID`, `winnerName`, `scores:[{playerID,name,score}]`, `leaderboard:[{playerID,name,wins}]`                                    |

`state` is a full snapshot, sent on join and on every phase change /
turn change. `move` is incremental and sent for every successful play
so the move-history side-panel and last-move tile highlight update
without waiting for the next `state`. The browser reconciles against
the next `state` if anything gets out of sync.

Per-player rack content lives in `StateEvent.PerRack` (a `map[playerID][]string`)
on the server side; the session forwards only the recipient's entry as
`yourRack`, so the wire payload to each browser never reveals other
players' racks.

---

## Storage / persistence

- **Active game state** lives entirely in the `RoomActor`'s memory.
  This matches the rest of `goakt-examples` and keeps the demo
  self-contained — losing a node loses its in-flight games. For
  durable games, swap in goakt's `persistence` package as a v2.
- **Leaderboard** uses goakt's CRDT `PNCounter` so wins converge across
  nodes without a database.
- **Player profiles** (display name + cumulative stats) live in a
  `PlayerProfileGrain` (virtual actor) keyed on a per-browser id from
  `sessionStorage`. Backed by one of two stores, selected at startup:
  - `pgProfileStore` (`profile_pg.go`) when `DATABASE_URL` /
    `--database-url` is set. Schema (`player_profiles`) is auto-migrated
    on boot with `CREATE TABLE IF NOT EXISTS`; load/save use
    `INSERT ... ON CONFLICT (id) DO UPDATE`. `make k8s-up` brings up a
    bundled `postgres:18-alpine` StatefulSet and wires the Secret.
  - `memProfileStore` (`profile.go`) as a per-pod fallback when no DSN
    is provided. Useful for tests / demos without Postgres; profile
    data is per-pod and lost on restart.

---

## GoAkt features showcased

| Feature                                                                    | Where it lives                              |
|----------------------------------------------------------------------------|---------------------------------------------|
| `SpawnSingleton`                                                           | `LobbyActor` in `lobby.go`                  |
| `SpawnOn` + `WithPlacement(LeastLoad)`                                     | `LobbyActor.handleJoinOrCreate`             |
| `Become` / `UnBecome` FSM                                                  | `RoomActor` in `room.go`                    |
| `ScheduleOnce` (turn / shutdown / bot-move timers)                         | `RoomActor.schedule`, `scheduleBotMove`     |
| Pub/Sub `TopicActor` — one topic per room                                  | `room.go::publish`, `session.go::PostStart` |
| Grains (virtual actors)                                                    | `PlayerProfileGrain` in `profile.go`        |
| CRDT `PNCounter`                                                           | `leaderboard.go` (keyed per player id)      |
| `Watch` / `*actor.Terminated` for owner-death cleanup                      | `RoomActor.Receive`                         |
| Cluster-aware `ActorOf`                                                    | `gateway.go::requestRoom`                   |
| CBOR-registered cross-node message types                                   | `main.go::buildActorSystem`                 |
| System Extensions for shared per-process state (dictionaries, leaderboard) | `main.go::buildActorSystem`                 |

---

## Phased rollout (all four phases shipped)

### Phase 1 — Engine + bot ✅

- Engine package (`scrabble/`) with full validation + scoring
- DAWG + Appel/Jacobson move generator
- 38 unit tests covering premium-square stacking (TW + DL), cross-word
  validation, bingo bonus, blank-tile scoring, end-game rack penalty +
  out-bonus, and `BestMove` finding the expected best play for fixed
  board + rack + tiny wordlist.

`go test ./scrabble/...` — all green.

### Phase 2 — Single-process multiplayer ✅

- Lobby + Room + Session actors (FSM via `Become`)
- Vanilla-TS browser client with SVG board, click-to-place tiles,
  blank-letter modal picker, in-game move log, unseen-tile tracker
- BotActor wrapping the move generator with a 700ms "thinking" delay
- End-to-end play: 2+ humans, or 1 human + 1 bot (single-process mode
  has since been retired — see "Compile only" / `make k8s-up` modes
  above)

### Phase 3 — UI polish ✅

- Drag-and-drop tile placement via pointer events (mouse + touch),
  with 6 px move threshold so click-to-select still works as fallback
- Ghost tile follows the cursor; drop-target square highlights gold;
  rack outlines when about to recall a pending placement
- Pause / Resume sub-state (any player can pause; turn timer freezes
  with the saved remaining duration; bot moves cancelled + re-scheduled)
- Classic Scrabble board palette (cream cells, blue DL/TL, pink/red
  DW/TW, gold-bordered tentative tiles vs. teal-bordered opponent's
  last move)
- Responsive layout (board scales with `min(100%, calc(100dvh - chrome))`,
  side pane stacks on tablet/mobile)
- `Cache-Control: no-store` middleware so rebuilds reach the browser
  without a hard refresh

### Phase 4 — Container packaging + Kubernetes ✅

- Multi-stage `Dockerfile`: tsc → static Go binary (CGO=0, trimpath,
  ldflags `-s -w`) → `distroless/static-debian12:nonroot` runtime.
- `kind/cluster.yaml`: 1 control-plane (with `ingress-ready=true` label
  + hostPort 80) + 3 workers.
- `k8s/` manifests: namespace, ServiceAccount + namespaced
  Role/RoleBinding granting the kubernetes discovery provider
  `pods: get/list/watch`, headless service for per-pod DNS + remoting,
  ClusterIP service for ingress, 3-replica StatefulSet
  (`podManagementPolicy: Parallel`, downward-API derived bind-host,
  kubernetes discovery via pod label `app=scrabble`, topology spread
  one-per-node, readiness/liveness probes, resource limits), and
  NGINX Ingress with cookie-based session affinity + WebSocket-friendly
  timeouts.
- `Makefile` rewrite with `make help` (sectioned) and end-to-end
  `make k8s-up` / `make k8s-down` targets, plus image build/load,
  individual kind / deploy steps, and `k8s-logs` / `k8s-status` for
  observation.

The browser sees one HTTP/WS endpoint. The 3 backend pods form a
goakt cluster transparently; cross-node room placement and the CRDT
leaderboard converge across all three.

---

## Out of scope

- **Multiple bot difficulty levels** — single greedy DAWG bot only.
  Adding "easy" (random legal move) and "hard" (DAWG + rack-leave
  heuristic) is a follow-up.
- **Durable game persistence** across pod restart / cluster destroy —
  active game state lives in `RoomActor` memory only. Adding goakt's
  `persistence` extension is a v2.
- **Authenticated player accounts** — identity is a per-browser
  `sessionStorage` UUID; passing the same `?id=` across visits resumes
  your `PlayerProfileGrain` and accumulates stats on the same record.
  No password / OAuth layer.
- **Spectator-specific UI** — late joiners receive `StateEvent`s via
  the room's topic and can watch, but there is no dedicated read-only
  view. Pre-game `Add Bot` etc. controls are correctly hidden by role.
- **Tournament-style challenge mechanic** — invalid placements are
  auto-rejected at `Place` time. There is no separate challenge action
  by which a player can dispute a previously-accepted word (because
  one was never accepted).
- **Additional languages** (French, Spanish, …) — the `Language` type
  in `scrabble/lang.go` is generic, and `Registry` can hold many
  bundles, but only English is wired up and shipped with a dictionary.
- **Mobile-optimised layout for very small screens** — the layout is
  responsive and tile placement works on touch, but ≤ 4-inch screens
  will feel cramped for a 15×15 board.
