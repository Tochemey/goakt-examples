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

Same as goakt-chat:

1. **Server** (in one terminal):
   ```bash
   cd goakt-chat-v2/server
   go run .
   ```

2. **Clients** (in additional terminals):
   ```bash
   cd goakt-chat-v2/client
   go run .
   ```

Client commands: `/help`, `/users`, `/join <room>`, `/dm <user> <msg>`, `/quit`.
