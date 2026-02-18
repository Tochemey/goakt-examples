# goakt-chat

A complete multi-room chat application built on top of GoAkt's remoting and `RemoteForward` API.

## Features

- **Room-based messaging** — clients join a named room (default: `general`); messages are broadcast only to peers in the same room.
- **Message history replay** — the server keeps the last 20 messages per room and replays them to new joiners.
- **Join/leave notifications** — a `SystemEvent` is pushed to all room members when someone connects or disconnects.
- **Direct messages** — send a private message to a specific user with `/dm`.
- **User listing** — query who is online in a room with `/users`.
- **Interactive client** — prompts for username and room at startup; supports slash-commands at runtime.

## Client commands

| Command            | Description                       |
| ------------------ | --------------------------------- |
| `/help`            | Show command reference            |
| `/users`           | List online users in current room |
| `/join <room>`     | Switch to a different room        |
| `/dm <user> <msg>` | Send a private message            |
| `/quit` or Ctrl-C  | Disconnect and exit               |

## Running

You need three terminals — one for the server and one per chat client.

### 1. Start the server first

```bash
cd server
go run .
```

Wait until you see:

```
INFO  Chat Server started — waiting for clients
```

### 2. Start one or more clients (each in its own terminal)

```bash
cd client
go run .
```

Each client will prompt for a username and room name, then connect to the server:

```
Enter your username: alice
Enter room name (leave blank for 'general'):
```

Once two or more clients are in the same room, messages typed by one appear in the others, prefixed with a timestamp and the sender's name:

```
[10:42:07] [general] alice: hello bob!
```

### Tips

- **Multiple rooms** — open another terminal, run the client, and type `/join dev`. Only messages from clients in `dev` will appear there.
- **History replay** — if you join after messages have been sent, the server automatically replays the last 20 messages so you catch up immediately.
- **No server restarts needed** — the server is long-lived; clients can connect and disconnect freely.
- **No port conflicts** — the server is fixed at `127.0.0.1:4000`; each client picks a random free port automatically, so you can run as many clients as you like.

## Architecture

```
Client A ──RemoteTell(Message)──► Server
                                    │
                           RemoteTell(Broadcast)
                                    │
                       ┌────────────┴────────────┐
                    Client B                   Client C
                  (same room)               (same room)

Client A ──RemoteTell(DirectMessage)──► Server
                                          │
                                 RemoteTell(DirectMessage)
                                          │
                                       Client B only
```

The server actor is the single source of truth for connected clients. All fan-out is done via `ctx.RemoteTell(addr, msg)` inside the server actor, which sends a protobuf message to any remote actor address without requiring the target to be local. No locking is needed because GoAkt's mailbox guarantees that `Receive` is called for one message at a time.
