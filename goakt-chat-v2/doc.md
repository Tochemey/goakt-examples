# goakt-chat-v2

A variant of goakt-chat that uses **plain Go structs** instead of protocol buffers, with **GoAkt logging disabled** and **printf/println** for server-side user output.

## Differences from goakt-chat

| Aspect         | goakt-chat                  | goakt-chat-v2            |
|----------------|-----------------------------|--------------------------|
| Message format | Protocol buffers (`chatpb`) | Go structs (`chatv2`)    |
| Serialization  | ProtoSerializer             | CBORSerializer           |
| GoAkt logging  | Zap logger (ErrorLevel)     | DiscardLogger (disabled) |
| Server output  | Logger (Info, Warn)         | fmt.Printf / fmt.Println |

## Running

Build and run as a single CLI:

```bash
# Build (outputs to goakt-chat-v2/bin/chatv2)
cd goakt-chat-v2 && make build
# Or: go build -o bin/chatv2 .

# 1. Start the server (in one terminal)
./bin/chatv2 server
# Or with custom host/port:
./bin/chatv2 server --host 127.0.0.1 --port 4000

# 2. Connect clients (in additional terminals)
./bin/chatv2 client
# Or non-interactive mode:
./bin/chatv2 client --user alice --room general
# Or connect to a different server:
./bin/chatv2 client --host 127.0.0.1 --port 4000
```

Client commands: `/help`, `/users`, `/join <room>`, `/dm <user> <msg>`, `/quit`.
