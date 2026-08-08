# goakt-chat

A complete multi-room chat application built on GoAkt's remoting, shipped as a single CLI with `server` and `client` subcommands.

It doubles as a serialization example: the same server serves clients speaking **protobuf** and clients speaking **CBOR-encoded Go structs**, at the same time, in the same room.

## Features

- **Room-based messaging** — clients join a named room (default: `general`); messages are broadcast only to peers in the same room.
- **Message history replay** — the server keeps the last 20 messages per room and replays them to new joiners.
- **Join/leave notifications** — a `SystemEvent` is pushed to all room members when someone connects or disconnects.
- **Direct messages** — send a private message to a specific user with `/dm`.
- **User listing** — query who is online in a room with `/users`.
- **Pluggable wire format** — `--codec proto` or `--codec cbor` per client; the server speaks both.
- **Interactive or scripted client** — prompts for username and room, or takes them as flags.

## Running

Build once:

```bash
cd goakt-chat && make build      # outputs bin/chat
# or: go build -o bin/chat .
```

### 1. Start the server

```bash
./bin/chat server
# or bind elsewhere:
./bin/chat server --host 0.0.0.0 --port 5000
```

Wait until you see:

```
Chat Server is running on 127.0.0.1 port 4000 — accepting proto and cbor clients
Chat Server started — waiting for clients
```

### 2. Start one or more clients, each in its own terminal

```bash
./bin/chat client                                    # prompts for username and room
./bin/chat client --user alice --room general        # non-interactive
./bin/chat client --user bob --codec proto           # protobuf on the wire
./bin/chat client --host 192.168.1.10 --bind 192.168.1.20
```

Messages from other clients in the room appear prefixed with a timestamp and the sender's name:

```
[10:42:07] [general] alice: hello bob!
```

### Client commands

| Command            | Description                       |
| ------------------ | --------------------------------- |
| `/help`            | Show command reference            |
| `/users`           | List online users in current room |
| `/join <room>`     | Switch to a different room        |
| `/dm <user> <msg>` | Send a private message            |
| `/quit` or Ctrl-C  | Disconnect and exit               |

### Tips

- **Multiple rooms** — open another terminal, run the client, and type `/join dev`. Only messages from clients in `dev` will appear there.
- **History replay** — join after messages have been sent and the server replays the last 20 so you catch up immediately.
- **Mixed codecs** — run `--codec cbor` and `--codec proto` clients side by side in one room; each sees the other's messages.
- **No port conflicts** — the server is fixed at the `--port` you give it; each client picks a random free port automatically, so you can run as many as you like.
- **Remote servers** — the server pushes messages back to the client, so a client on another machine must set `--bind` to an address the server can reach.

## Wire formats

The actors never mention a serialization format. They are written against one domain model — the plain Go structs in [internal/chat](../internal/chat/messages.go) — and the [wire](./wire) package maps that model onto whatever goes over the network:

| Codec   | Wire type                                      | Serializer                    |
| ------- | ---------------------------------------------- | ----------------------------- |
| `cbor`  | the domain structs themselves                  | `remote.CBORSerializer`       |
| `proto` | generated messages in `internal/chatpb`        | `remote.ProtoSerializer`      |

`wire.Decode` handles the return trip for both, so any node can *receive* either format regardless of what it sends with.

### Why one server can serve both

Two facts about GoAkt's remoting config make this work:

1. `remote.NewConfig` already registers `ProtoSerializer` for every `proto.Message`, so the protobuf path needs no registration at all.
2. The chat structs implement only `chat.ChatMessage` and the generated types implement only `proto.Message`, so the CBOR registrations never overlap with the proto default and serializer dispatch stays unambiguous.

Both the server and the client build their remoting config from the same `wire.RemoteOptions()` call. That is deliberate: a type registered on one end but not the other fails to deserialize at runtime, so there is exactly one place where that list lives. Note that registering the `ChatMessage` interface alone is not enough — the interface entry selects CBOR for the family, but only a *concrete* registration adds the type to GoAkt's global type registry, which is what the receiver uses to resolve the type name carried in each CBOR frame.

Because the server records each client's codec at connect time, every message it pushes — broadcasts, direct messages, system events, history replay — is encoded with the format that particular recipient speaks. There is no codec negotiation and no way for the two ends to disagree.

## Architecture

```
Client A ──Tell(Message)──► Server
                              │
                        Tell(Broadcast)
                              │
                  ┌───────────┴───────────┐
               Client B                Client C
             (same room)             (same room)

Client A ──Tell(DirectMessage)──► Server
                                    │
                            Tell(DirectMessage)
                                    │
                                 Client B only
```

The server actor is the single source of truth for connected clients. All fan-out is done with `ctx.Tell(pid, msg)` inside the server actor, against PIDs resolved through `RemoteLookup`. No locking is needed because GoAkt's mailbox guarantees that `Receive` is called for one message at a time.

## Layout

```
goakt-chat/
  main.go            entrypoint
  cmd/               cobra commands: root, server, client (terminal I/O lives here)
  actors/            the Server and Client actors, written against the domain model
  wire/              codecs, wire↔domain translation, shared serializer registrations
```
