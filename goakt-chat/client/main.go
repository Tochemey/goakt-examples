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

	actors "github.com/tochemey/goakt/v3/actor"
	"github.com/tochemey/goakt/v3/address"
	"github.com/tochemey/goakt/v3/goaktpb"
	"github.com/tochemey/goakt/v3/log"
	"github.com/tochemey/goakt/v3/remote"
	"github.com/tochemey/goakt/v3/supervisor"
	"github.com/travisjeffery/go-dynaport"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/tochemey/goakt-examples/v2/internal/chatpb"
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

	logger := log.New(log.InfoLevel, os.Stdout)

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

	actorSystem, err := actors.NewActorSystem(
		"ChatSystem",
		actors.WithRemote(remote.NewConfig(host, port)),
		actors.WithLogger(logger))

	if err != nil {
		logger.Fatal(err)
		os.Exit(1)
	}

	if err := actorSystem.Start(ctx); err != nil {
		logger.Fatal(err)
		os.Exit(1)
	}

	time.Sleep(time.Second)

	serverAddress := address.New("ChatServer", actorSystem.Name(), serverHost, serverPort)

	clientActor := NewClient(userName, roomName, serverAddress)

	client, err := actorSystem.Spawn(
		ctx,
		"ChatClient",
		clientActor,
		actors.WithSupervisor(
			supervisor.NewSupervisor(
				supervisor.WithStrategy(supervisor.OneForOneStrategy),
				supervisor.WithAnyErrorDirective(supervisor.ResumeDirective),
			)))
	if err != nil {
		logger.Fatal(err)
		os.Exit(1)
	}

	// wait for the client actor to connect to the server
	time.Sleep(time.Second)

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
					_ = client.RemoteTell(ctx, serverAddress, &chatpb.Disconnect{})
					return

				case "/help":
					fmt.Print("\r" + helpText + "\n")

				case "/users":
					_ = client.RemoteTell(ctx, serverAddress, &chatpb.ListUsersRequest{
						Room: clientActor.CurrentRoom(),
					})

				case "/join":
					if len(parts) < 2 || parts[1] == "" {
						fmt.Print("\rUsage: /join <room>\n")
						break
					}
					newRoom := parts[1]
					_ = client.RemoteTell(ctx, serverAddress, &chatpb.Disconnect{})
					time.Sleep(200 * time.Millisecond)
					clientActor.SetRoom(newRoom)
					_ = client.RemoteTell(ctx, serverAddress, &chatpb.Connect{
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
					_ = client.RemoteTell(ctx, serverAddress, &chatpb.DirectMessage{
						FromUser: userName,
						ToUser:   toUser,
						Content:  content,
						SentAt:   timestamppb.Now(),
					})

				default:
					fmt.Printf("\rUnknown command: %s  (type /help)\n", cmd)
				}

				printPrompt(userName, clientActor.CurrentRoom())
				continue
			}

			// plain message → broadcast to room
			_ = client.RemoteTell(ctx, serverAddress, &chatpb.Message{
				UserName: userName,
				Content:  input,
				Room:     clientActor.CurrentRoom(),
				SentAt:   timestamppb.Now(),
			})

			printPrompt(userName, clientActor.CurrentRoom())
		}
	}()

	// wait for Ctrl-C or the input loop to finish
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	select {
	case <-sig:
		_ = client.RemoteTell(ctx, serverAddress, &chatpb.Disconnect{})
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
	userName      string
	serverAddress *address.Address
	logger        log.Logger

	room atomic.Value // string — current room name
}

var _ actors.Actor = (*Client)(nil)

func NewClient(userName, room string, serverAddress *address.Address) *Client {
	c := &Client{
		userName:      userName,
		serverAddress: serverAddress,
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

func (c *Client) PreStart(*actors.Context) error {
	return nil
}

func (c *Client) Receive(ctx *actors.ReceiveContext) {
	switch msg := ctx.Message().(type) {
	case *goaktpb.PostStart:
		c.logger = ctx.Logger()
		ctx.RemoteTell(c.serverAddress, &chatpb.Connect{
			UserName: c.userName,
			Room:     c.CurrentRoom(),
		})

	case *chatpb.Broadcast:
		ts := formatTime(msg.GetSentAt())
		fmt.Printf("\r[%s] [%s] %s: %s\n", ts, msg.GetRoom(), msg.GetFromUser(), msg.GetContent())
		printPrompt(c.userName, c.CurrentRoom())

	case *chatpb.DirectMessage:
		ts := formatTime(msg.GetSentAt())
		fmt.Printf("\r[%s] [DM from %s]: %s\n", ts, msg.GetFromUser(), msg.GetContent())
		printPrompt(c.userName, c.CurrentRoom())

	case *chatpb.SystemEvent:
		ts := formatTime(msg.GetAt())
		fmt.Printf("\r[%s] *** %s ***\n", ts, msg.GetText())
		printPrompt(c.userName, c.CurrentRoom())

	case *chatpb.ListUsersResponse:
		fmt.Printf("\rOnline in %s: %s\n", c.CurrentRoom(), strings.Join(msg.GetUserNames(), ", "))
		printPrompt(c.userName, c.CurrentRoom())

	default:
		ctx.Unhandled()
	}
}

func (c *Client) PostStop(*actors.Context) error {
	c.logger.Info("Chat Client stopped")
	return nil
}

// formatTime renders a protobuf timestamp as HH:MM:SS, or "?" if nil.
func formatTime(ts *timestamppb.Timestamp) string {
	if ts == nil {
		return "?"
	}
	return ts.AsTime().Local().Format("15:04:05")
}
