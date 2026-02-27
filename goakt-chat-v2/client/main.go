// MIT License
//
// Copyright (c) 2022-2026 GoAkt Team
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/tochemey/goakt/v4/actor"
	"github.com/tochemey/goakt/v4/log"
	"github.com/tochemey/goakt/v4/remote"
	"github.com/tochemey/goakt/v4/supervisor"
	"github.com/travisjeffery/go-dynaport"

	"github.com/tochemey/goakt-examples/v2/internal/chatv2"
)

const (
	serverHost = "127.0.0.1"
	serverPort = 4000

	helpText = `Commands:
  /help              show this help
  /users             list online users in the current room
  /join <room>       switch to a different room
  /dm <user> <msg>   send a private message to a user
  /quit              disconnect and exit`
)

func main() {
	ctx := context.Background()

	// --- interactive startup ---
	reader := bufio.NewReader(os.Stdin)

	fmt.Print("Enter your username: ")
	userName, _ := reader.ReadString('\n')
	userName = strings.TrimSpace(userName)
	if userName == "" {
		userName = fmt.Sprintf("guest-%d", time.Now().Unix())
	}

	fmt.Print("Enter room name (leave blank for 'general'): ")
	roomName, _ := reader.ReadString('\n')
	roomName = strings.TrimSpace(roomName)
	if roomName == "" {
		roomName = "general"
	}

	// --- actor system ---
	host := serverHost
	ports := dynaport.Get(1)
	port := ports[0]

	cbor := remote.NewCBORSerializer()
	actorSystem, err := actor.NewActorSystem(
		"ChatSystem",
		actor.WithRemote(remote.NewConfig(host, port,
			remote.WithSerializers((*chatv2.ChatMessage)(nil), cbor),
			remote.WithSerializers((*chatv2.Connect)(nil), cbor),
			remote.WithSerializers((*chatv2.Disconnect)(nil), cbor),
			remote.WithSerializers((*chatv2.Message)(nil), cbor),
			remote.WithSerializers((*chatv2.DirectMessage)(nil), cbor),
			remote.WithSerializers((*chatv2.ListUsersRequest)(nil), cbor),
			remote.WithSerializers((*chatv2.ListUsersResponse)(nil), cbor),
			remote.WithSerializers((*chatv2.Broadcast)(nil), cbor),
			remote.WithSerializers((*chatv2.SystemEvent)(nil), cbor),
		)),
		actor.WithLogger(log.DiscardLogger))

	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to create actor system:", err)
		os.Exit(1)
	}

	if err := actorSystem.Start(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "failed to start actor system:", err)
		os.Exit(1)
	}

	server, err := actorSystem.NoSender().RemoteLookup(ctx, serverHost, serverPort, "ChatServer")
	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to lookup ChatServer:", err)
		os.Exit(1)
	}

	clientActor := NewClient(userName, roomName, server)

	client, err := actorSystem.Spawn(
		ctx,
		"ChatClient",
		clientActor,
		actor.WithSupervisor(
			supervisor.NewSupervisor(
				supervisor.WithStrategy(supervisor.OneForOneStrategy),
				supervisor.WithAnyErrorDirective(supervisor.ResumeDirective),
			)))
	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to spawn ChatClient:", err)
		os.Exit(1)
	}

	printHelp()
	printPrompt(userName, roomName)

	done := make(chan struct{})

	// input loop
	go func() {
		defer close(done)
		for {
			input, err := reader.ReadString('\n')
			if err != nil {
				return
			}

			input = strings.TrimSpace(input)
			if input == "" {
				printPrompt(userName, clientActor.CurrentRoom())
				continue
			}

			// slash-command dispatch
			if strings.HasPrefix(input, "/") {
				parts := strings.SplitN(input, " ", 3)
				cmd := strings.ToLower(parts[0])

				switch cmd {
				case "/quit":
					_ = client.Tell(ctx, server, &chatv2.Disconnect{})
					return

				case "/help":
					fmt.Print("\r" + helpText + "\n")

				case "/users":
					_ = client.Tell(ctx, server, &chatv2.ListUsersRequest{
						Room: clientActor.CurrentRoom(),
					})

				case "/join":
					if len(parts) < 2 || parts[1] == "" {
						fmt.Print("\rUsage: /join <room>\n")
						break
					}
					newRoom := parts[1]
					_ = client.Tell(ctx, server, &chatv2.Disconnect{})

					time.Sleep(200 * time.Millisecond)

					clientActor.SetRoom(newRoom)
					_ = client.Tell(ctx, server, &chatv2.Connect{
						UserName: userName,
						Room:     newRoom,
					})

					roomName = newRoom
					fmt.Printf("\rJoined room: %s\n", newRoom)

				case "/dm":
					if len(parts) < 3 {
						fmt.Print("\rUsage: /dm <user> <message>\n")
						break
					}
					toUser := parts[1]
					content := parts[2]
					_ = client.Tell(ctx, server, &chatv2.DirectMessage{
						FromUser: userName,
						ToUser:   toUser,
						Content:  content,
						SentAt:   time.Now(),
					})

				default:
					fmt.Printf("\rUnknown command: %s  (type /help)\n", cmd)
				}

				printPrompt(userName, clientActor.CurrentRoom())
				continue
			}

			// plain message → broadcast to room
			_ = client.Tell(ctx, server, &chatv2.Message{
				UserName: userName,
				Content:  input,
				Room:     clientActor.CurrentRoom(),
				SentAt:   time.Now(),
			})

			printPrompt(userName, clientActor.CurrentRoom())
		}
	}()

	// wait for Ctrl-C or the input loop to finish
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	select {
	case <-sig:
		_ = client.Tell(ctx, server, &chatv2.Disconnect{})
	case <-done:
	}

	_ = actorSystem.Stop(ctx)
	os.Exit(0)
}

// printHelp prints the command reference once at startup.
func printHelp() {
	fmt.Println(helpText)
}

// printPrompt re-prints the prompt after output is written.
func printPrompt(user, room string) {
	fmt.Printf("[%s @ %s] > ", user, room)
}

// Client receives messages pushed by the server and prints them to stdout.
type Client struct {
	userName string
	server   *actor.PID

	room atomic.Value // string — current room name
}

var _ actor.Actor = (*Client)(nil)

func NewClient(userName, room string, server *actor.PID) *Client {
	c := &Client{
		userName: userName,
		server:   server,
	}
	c.room.Store(room)
	return c
}

// CurrentRoom returns the client's active room, safe for concurrent access.
func (c *Client) CurrentRoom() string {
	return c.room.Load().(string)
}

// SetRoom updates the active room.
func (c *Client) SetRoom(room string) {
	c.room.Store(room)
}

func (c *Client) PreStart(*actor.Context) error {
	return nil
}

func (c *Client) Receive(ctx *actor.ReceiveContext) {
	switch msg := ctx.Message().(type) {
	case *actor.PostStart:
		ctx.Tell(c.server, &chatv2.Connect{
			UserName: c.userName,
			Room:     c.CurrentRoom(),
		})

	case *chatv2.Broadcast:
		ts := formatTime(msg.SentAt)
		fmt.Printf("\r[%s] [%s] %s: %s\n", ts, msg.Room, msg.FromUser, msg.Content)
		printPrompt(c.userName, c.CurrentRoom())

	case *chatv2.DirectMessage:
		ts := formatTime(msg.SentAt)
		fmt.Printf("\r[%s] [DM from %s]: %s\n", ts, msg.FromUser, msg.Content)
		printPrompt(c.userName, c.CurrentRoom())

	case *chatv2.SystemEvent:
		ts := formatTime(msg.At)
		fmt.Printf("\r[%s] *** %s ***\n", ts, msg.Text)
		printPrompt(c.userName, c.CurrentRoom())

	case *chatv2.ListUsersResponse:
		fmt.Printf("\rOnline in %s: %s\n", c.CurrentRoom(), strings.Join(msg.UserNames, ", "))
		printPrompt(c.userName, c.CurrentRoom())

	default:
		ctx.Unhandled()
	}
}

func (c *Client) PostStop(*actor.Context) error {
	return nil
}

// formatTime renders a time as HH:MM:SS, or "?" if zero.
func formatTime(t time.Time) string {
	if t.IsZero() {
		return "?"
	}
	return t.Local().Format("15:04:05")
}
