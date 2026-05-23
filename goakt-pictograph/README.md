# goakt-pictograph

A browser-playable, real-time multiplayer **drawing & guessing game** (Skribbl.io-style) built on [GoAkt](https://github.com/Tochemey/goakt). Each match is a room of 2–8 players: every round one player gets a secret word and draws it on a shared canvas while the others race to guess. Best of N rounds wins. Anyone with the room code can also drop in as a **spectator**.

This is the multi-user companion piece to [`goakt-game`](../goakt-game): where Tetris is a tick-driven single-player actor, Pictograph is an event-driven, multi-writer, FSM-shaped actor whose broadcast path runs through GoAkt's pub/sub topic actor.

---

## What GoAkt features it shows

| Feature                                                                        | Where it lives                                                                   |
|--------------------------------------------------------------------------------|----------------------------------------------------------------------------------|
| `SpawnSingleton` — one-instance-per-cluster control plane                      | `LobbyActor` in `lobby.go`                                                       |
| `SpawnOn` + `WithPlacement(LeastLoad)` + `WithRelocationDisabled`              | `LobbyActor.handleJoinOrCreate` in `lobby.go`                                    |
| `Become` / behavior stack as an explicit FSM                                   | `RoomActor` in `room.go` (`waiting → choosing → drawing → roundOver → gameOver`) |
| `Stash` / `UnstashAll` for messages arriving in the wrong phase                | `RoomActor.waitingBehavior`, `choosingBehavior`                                  |
| Recurring `Schedule` + one-shot `ScheduleOnce` for round timers                | `RoomActor.enterDrawing`, `enterChoosing`, `enterRoundOver`                      |
| Pub/Sub (`TopicActor`) — one topic per room, fan-out to N players + spectators | `room.go::publish`, `session.go::PostStart`                                      |
| Grains (virtual actors) for persistent player profiles                         | `PlayerProfileGrain` in `profile.go`                                             |
| CRDT `PNCounter` for the cluster-wide global leaderboard                       | `leaderboard.go`                                                                 |
| `Watch` / `*actor.Terminated` for owner-death cleanup                          | `RoomActor.Receive`                                                              |
| Cluster-aware `ActorOf` for cross-node lookup                                  | `gateway.go::requestRoom`                                                        |
| CBOR serializers registered for cross-node message types                       | `main.go::buildActorSystem`                                                      |
| Static discovery (configurable seed peer list, same shape as `goakt-game`)     | `main.go::peerList`                                                              |

---

## Architecture

```
   ┌─────────────────────────┐         ┌─────────────────────────┐
   │ Node A — :8080  :9000   │         │ Node B — :8081  :9100   │
   │                         │         │                         │
   │  PlayerSessionActor  ◄──┼ Publish─┤    Topic: room.<code>   │
   │  (one per WS conn,      │         │   (system TopicActor)   │
   │   subscribes to topic)  │         │            ▲            │
   │           │             │         │            │ Publish    │
   │           │ Tell        │         │    ┌────────────────┐   │
   │           ▼             │         │    │   RoomActor    │   │
   │     ┌─────────────┐     │         │    │ (cluster-      │   │
   │     │  LobbyActor │◄────┼─ Ask────┼────┤  placed via    │   │
   │     │ (singleton) │     │         │    │  SpawnOn)      │   │
   │     └─────────────┘     │         │    └────────────────┘   │
   │                         │         │                         │
   │     ┌──────────────┐    │         │                         │
   │     │ PlayerProfile│    │         │   CRDT Replicator       │
   │     │   Grain      │    │         │   (one per node, syncs  │
   │     │ (activated   │    │         │    PNCounter leaderbd.) │
   │     │  per userID) │    │         │                         │
   │     └──────────────┘    │         │                         │
   └─────────────────────────┘         └─────────────────────────┘
                ▲
                │ WebSocket
                │
            Browser (2D canvas client, vanilla TypeScript)
```

**Connection flow**

1. Browser opens a WS to `/ws?name=Alice&room=ABCD` (room may be empty → create new).
2. Gateway upgrades and `Ask`s the cluster-singleton `LobbyActor` for a room.
3. Lobby either looks up an existing `RoomActor` by code or `SpawnOn(LeastLoad)`s a new one — possibly on a remote node.
4. Gateway spawns a *local* `PlayerSessionActor`, hands it the room PID and the player's profile (fetched via the `PlayerProfileGrain`).
5. The session subscribes to `room.<code>` via the system `TopicActor` and starts forwarding WS frames to the room.
6. The room publishes state / strokes / chat / score events to the topic; every subscriber's session forwards them to its WS as JSON.

**Match lifecycle (`RoomActor` FSM)**

```
   waiting    accept Join; on ≥2 players → Become(choosing)
      ↓
   choosing   pick drawer; offer 3 word choices; ScheduleOnce(15s, AutoPickFirst)
      ↓       stash guesses here — they belong to the next phase
   drawing    Schedule(1s, TickCountdown); accept Stroke/Guess/Clear; UnstashAll on entry
      ↓
   roundOver  reveal the word; ScheduleOnce(5s, NextRound)
      ↓
   (loop to choosing — or, if rounds ≥ max, Become(gameOver))
      ↓
   gameOver   final scoreboard, commit winner to cluster PNCounter, ScheduleOnce(20s, Shutdown)
```

The four behaviors are real `actor.Behavior` functions registered with `Become`/`UnBecome`. None of them needs a `phase string` field on `RoomActor` — the behavior stack *is* the phase.

---

## Quick start

### Single node

```bash
make run
```

Open <http://localhost:8080>. You'll be assigned a player name and dropped into a fresh room. Share the URL (or the room code shown top-left) for friends to join.

### Two-node cluster (docker compose)

```bash
make cluster-up      # builds the image and brings up node-a + node-b
make cluster-logs    # follow both nodes' logs
make cluster-down    # stop everything
```

Open <http://localhost:8080> *and* <http://localhost:8081> — players from both nodes land in the same room, the room itself is placed on whichever node is least loaded, and the leaderboard converges across both nodes via the CRDT replicator. Watch `make cluster-logs` for `room=room.<code> (local)` vs `(remote@node-b:9000)` annotations.

### Two-node cluster (local processes, no docker)

```bash
make local-node-a    # terminal 1 — node A on default ports
make local-node-b    # terminal 2 — node B on shifted ports
make local-stop      # free any leaked ports between runs
```

---

## Ports

| Purpose                              | Node A default | Node B default | Configurable via   |
|--------------------------------------|----------------|----------------|--------------------|
| HTTP / WebSocket (browser-facing)    | `8080`         | `8081`         | `--http-port`      |
| Remote actor gRPC (inter-node Tells) | `9000`         | `9100`         | `--remoting-port`  |
| Cluster gossip (discovery)           | `9001`         | `9101`         | `--discovery-port` |
| Cluster peer state-sync              | `9002`         | `9102`         | `--peers-port`     |

Discovery is **static**, identical to `goakt-game`: pass `--peers host:9001,host:9101` to each node. Swap in `discovery/kubernetes` or `discovery/dnssd` for production — only `main.go::buildActorSystem` would change.

---

## Game rules

### Goal

Score the most points across the configured number of rounds (3 by
default). At the end of the last round the highest-score player wins
the game and gets a permanent +1 on the cluster-wide leaderboard.

### Players

A room holds between **2 and 8 players**. A player count below 2
keeps the room in the *waiting* phase; a 9th player who tries to join
a full room is rejected with an `error` event and stays disconnected.

### Round structure

Each round runs through four phases, in order:

| Phase       | Duration         | What happens                                                                                                  |
|-------------|------------------|---------------------------------------------------------------------------------------------------------------|
| `waiting`   | until 2+ players | Players gather. Once the minimum is met, a 5-second gather window starts; further joiners reset it.           |
| `choosing`  | up to 15 s       | The drawer is shown 3 random words and picks one. If the drawer doesn't pick in time, the first word is used. |
| `drawing`   | up to 80 s       | The drawer paints. Guessers type into chat. Phase ends early if *every* non-drawer has guessed correctly.     |
| `roundOver` | 5 s              | The word is revealed; the round scoreboard is shown.                                                          |

After the configured number of rounds (`DefaultRounds = 3`) the room
enters `gameOver` for 20 s and then shuts itself down.

### Picking the drawer

Drawers rotate **fairly**: each player draws exactly once before any
player draws a second time. Joiners mid-game are eligible for the
next slot. If the drawer leaves mid-round, the room ends the current
round early and rotates on the next round.

### Scoring

**Per correct guess:** points decay linearly with the time remaining.

```
score = ⌊ 100 × (timeLeft / 80) + 10 × (1 − timeLeft / 80) ⌋    (min 10)
```

Concrete values (the integer truncation matters for the off-by-one
in the middle row):

| Guessed at  | Points |
|-------------|--------|
| t = 80 s    | 100    |
| t = 60 s    | 77     |
| t = 40 s    | 55     |
| t = 20 s    | 32     |
| t =  0 s    | 10     |

**Drawer bonus.** The drawer scores `+50` for *every* guesser who
gets the word — so a popular drawing pays off more than a difficult
one.

**Drawer cannot guess.** Guess messages from the drawer are dropped.

### What other players see when you guess

| Your guess                                | What everyone sees                                  | Your score change |
|-------------------------------------------|-----------------------------------------------------|-------------------|
| Wrong word                                | Shows as chat: `Bob: octopus`                       | nothing           |
| Correct word                              | A system line `✓ Bob guessed the word!` (no text)   | `+score` (see above) |
| Anything after you've already guessed     | Dropped silently                                    | nothing           |

This protects the word from leaking through chat. Spectators (anyone
who joined late) follow the same rules but can never themselves
guess — they appear in the players list but receive zero points.

### Game over and leaderboard

When the last round ends, every connected client receives a
`gameOver` event with the round scoreboard. A moment later a second
`gameOver` event arrives carrying the cluster-wide leaderboard:
**total game wins per player id**, replicated across nodes via the
CRDT `PNCounter` index. Per-game score does **not** carry over —
only "did this player win this game" is persisted.

A player's leaderboard identity is keyed on the `?id=...` URL
parameter (the client persists a per-tab id in `sessionStorage`).
Reusing the same `?id=...` across visits accumulates wins onto the
same row.

### Play again

The game-over overlay has a **▶ Play Again** button. The first player
to click it triggers an immediate restart in the same room:

- All in-game scores reset to 0.
- The drawer rotation resets (everyone is eligible again).
- The 20-second auto-shutdown timer is cancelled.
- A chat message announces who started the new game.
- The room jumps straight into the next round's `choosing` phase
  (skipping the 5-second gather window, since everyone is already
  present).

First click wins — no vote. If nobody clicks within 20 seconds, the
room shuts down and any returning player will get a fresh code.

---

## Controls

| Action                  | How                                                                  |
|-------------------------|----------------------------------------------------------------------|
| Draw (drawer only)      | Click and drag on the canvas                                         |
| Change brush color      | Click a swatch in the toolbar                                        |
| Change brush size       | Drag the **size** slider in the toolbar                              |
| Clear the canvas (drawer only) | Click the **Clear** button in the toolbar                     |
| Pick a word (drawer only, choosing phase) | Click one of the three offered words in the overlay   |
| Send a chat / guess     | Type into the chat input, press **Enter**                            |

---

## Code layout

| File                                 | Responsibility                                                                                                        |
|--------------------------------------|-----------------------------------------------------------------------------------------------------------------------|
| `main.go`                            | Flag parsing, actor system bootstrap (`WithPubSub` + `WithCluster.WithCRDT` + `WithRemote`), HTTP server, lobby spawn |
| `gateway.go`                         | WS upgrade, profile-grain lookup, room request, per-connection session spawn, reader loop                             |
| `lobby.go`                           | `LobbyActor` cluster singleton — room directory by code, `SpawnOn(LeastLoad)` for new rooms                           |
| `room.go`                            | `RoomActor` — FSM via `Become`, scheduling, scoring, publishing to topic, watching the gateway                        |
| `session.go`                         | `PlayerSessionActor` — owns the `*websocket.Conn`, subscribes to room topic, encodes outbound events as JSON          |
| `profile.go`                         | `PlayerProfileGrain` — virtual actor keyed on player id, persists stats across reconnects                             |
| `leaderboard.go`                     | Thin wrapper around the CRDT `Replicator`, manages a per-player `PNCounter` for wins                                  |
| `words.go`                           | Bundled word list (categories: animals / food / objects / sports / fantasy)                                           |
| `types.go`                           | Wire protocol — inbound `WSIn`, outbound `WSOut`, and the cross-node actor messages                                   |
| `web/index.html`                     | Boot HTML; loads `main.js`                                                                                            |
| `web/main.ts`                        | TypeScript source for the browser client (mirrors the `types.go` wire payload)                                        |
| `web/main.js`                        | Build artifact (gitignored). Generated by `make web` or the Docker `web-builder` stage                                |
| `tsconfig.json`                      | TS compiler config (ES2020, strict, in-place compile inside `web/`)                                                   |
| `Dockerfile` + `docker-compose.yaml` | Two-node cluster image + service definition (same layout as `goakt-game`)                                             |
| `Makefile`                           | `run` / `cluster-up` / `cluster-down` / `local-node-a` / `local-node-b` / `web` / `build`                             |

`room.go` and `session.go` are **byte-identical between single-node and cluster mode** — that's the location-transparency story.

---

## Step-by-step demo walkthrough

A guided tour of what makes this example interesting. Throughout the
walkthrough, `CODE` stands for the 4-character room code your first
browser tab will be assigned — copy it from the top-left of that tab's
UI and substitute it everywhere `CODE` appears below.

### Step 1 — start the two-node cluster

```bash
make cluster-up      # builds the image and starts node-a + node-b
make cluster-logs    # in a second terminal — leave this running
```

You should see both nodes finish booting. Watch the logs for:

```
node-a  | actor=GoAktSingletonManager started successfully
node-a  | lobby singleton ready on node-a:9000
node-b  | not the cluster leader; lobby singleton is hosted on another node
```

Only node-a runs the lobby singleton. node-b knows it's not the
leader and skips re-spawning it.

### Step 2 — open the first browser tab on node-a

Visit <http://localhost:8080>. You'll be prompted for a display name
(e.g. *Alice*). The top-left of the UI shows the room code — note it
down as `CODE`.

In `make cluster-logs` you'll see one of:

```
node-a  | lobby: spawned room.<code> (local) for player Alice
node-a  | lobby: spawned room.<code> (remote@node-b:9000) for player Alice
```

- `(local)` — the room actor was placed on node-a (the node Alice
  connected to).
- `(remote@node-b:9000)` — the room actor was placed on node-b, even
  though Alice connected to node-a. `SpawnOn(LeastLoad)` made the
  call; the gateway and session don't care which node hosts the room.

### Step 3 — open three more tabs across both nodes

Each tab gets its own per-tab identity automatically (the client uses
`sessionStorage`, which is scoped per tab), so opening multiple tabs
in the same browser window works fine. Open these three URLs in new
tabs (substitute the code from Step 2):

- <http://localhost:8080/?name=Bob&room=CODE>
- <http://localhost:8081/?name=Carol&room=CODE>
- <http://localhost:8081/?name=Dan&room=CODE>

Use the `?name=` query parameter on each URL — the prompt the client
shows otherwise would slow the demo down and it's easy to mistype.

After all four tabs are connected and the 5-second gather window
elapses, the room transitions to the `choosing` phase. The "Players"
panel in every tab now shows all four names; one player is
highlighted as the drawer.

The point: tabs on `:8081` (node-b) talk to the same `RoomActor` as
tabs on `:8080` (node-a). If the room lives on node-b, the two node-a
tabs reach it via cross-node `Tell`s through the cluster. If the room
lives on node-a, node-b's tabs do the cross-node trip instead. Either
way, no per-tab routing logic exists in the gateway.

**Troubleshooting.** If the Players panel only ever shows one entry
no matter how many tabs you open: an older build of the client stored
the player id in `localStorage`, which is *shared* across tabs in the
same browser profile, causing every tab to be treated as the same
returning player. Run `make build` (or `make cluster-up` again if
you're using docker) to pick up the current client, or hard-reload
each tab so it picks up the new `main.js`. If you want to be sure
each tab is a fresh player, also append `&id=<anything-unique>` to
each URL.

### Step 4 — play a round

The drawer (highlighted in blue) picks a word and starts drawing with
the mouse. Strokes broadcast at ~20 Hz via the room's pub/sub topic;
every tab — whichever node — sees the lines appear in real time.

The other three tabs type guesses in the chat box. A wrong guess
shows up as chat for everyone; the first correct guess hides the word
text and shows `✓ <name> guessed the word!` everywhere, with a score
update.

### Step 5 — drop in a spectator

Open a 5th tab at <http://localhost:8080/?room=CODE> (or `:8081/?room=CODE`)
as *Eve*. Eve appears in the players list and immediately sees the
in-progress drawing, replayed from the room's per-round stroke buffer.

The `RoomActor` has no special spectator code path — Eve is just
another subscriber on the room's pub/sub topic. Nothing in the room
itself needed to change to support an arbitrary number of viewers.

### Step 6 — finish the game

Play through the configured number of rounds (3 by default). When the
final round ends:

- Every tab receives a `gameOver` event with the winner and the round
  scoreboard.
- A moment later, every tab receives an updated `gameOver` event with
  the *cluster-wide leaderboard* — the winner's `PNCounter` was
  incremented via the CRDT replicator, then read back.

### Step 7 — observe leaderboard convergence

Repeat the demo with the same names (`?name=Alice&id=...`), playing
on different nodes for different games. Each `gameOver` event's
`leaderboard` field shows the accumulated wins per player — the same
numbers whether the game that just finished was hosted on node-a or
node-b. The PNCounter deltas gossip across nodes on the
`goakt.crdt.deltas` topic.

### Step 8 — tear down

```bash
make cluster-down
```

---

## License

MIT, same as the rest of the goakt-examples repo.
