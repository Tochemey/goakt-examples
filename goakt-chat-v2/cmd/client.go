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

package cmd

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

	"github.com/spf13/cobra"
	"github.com/tochemey/goakt/v4/actor"
	"github.com/tochemey/goakt/v4/log"
	"github.com/tochemey/goakt/v4/remote"
	"github.com/tochemey/goakt/v4/supervisor"
	"github.com/travisjeffery/go-dynaport"

	"github.com/tochemey/goakt-examples/v2/internal/chatv2"
)

const clientHelpText = `Commands:
  /help              show this help
  /users             list online users in the current room
  /join <room>       switch to a different room
  /dm <user> <msg>   send a private message to a user
  /quit              disconnect and exit`

var (
	clientServerHost string
	clientServerPort int
	clientUser       string
	clientRoom       string
)

var clientCmd = &cobra.Command{
	Use:   "client",
	Short: "Connect to the chat server as a client",
	Long: `Connect to the chat server as a chat client.

Interactive mode (default): omit --user and --room to be prompted for username and room.
Non-interactive mode: use --user and --room for scripting or automation.

After connecting, use slash commands: /help, /users, /join <room>, /dm <user> <msg>, /quit`,
	Example: `  chatv2 client
  chatv2 client --user alice --room general
  chatv2 client --host 192.168.1.10 --port 4000`,
	RunE: runClient,
}

func init() {
	rootCmd.AddCommand(clientCmd)
	clientCmd.Flags().StringVar(&clientServerHost, "host", "127.0.0.1", "Server host to connect to")
	clientCmd.Flags().IntVar(&clientServerPort, "port", 4000, "Server port to connect to")
	clientCmd.Flags().StringVar(&clientUser, "user", "", "Username (optional; prompts if not set)")
	clientCmd.Flags().StringVar(&clientRoom, "room", "", "Room name (optional; defaults to 'general')")
}

func runClient(c *cobra.Command, args []string) error {
	ctx := context.Background()
	reader := bufio.NewReader(os.Stdin)

	userName := clientUser
	roomName := clientRoom

	if userName == "" {
		fmt.Print("Enter your username: ")
		line, _ := reader.ReadString('\n')
		userName = strings.TrimSpace(line)
		if userName == "" {
			userName = fmt.Sprintf("guest-%d", time.Now().Unix())
		}
	}

	if roomName == "" {
		fmt.Print("Enter room name (leave blank for 'general'): ")
		line, _ := reader.ReadString('\n')
		roomName = strings.TrimSpace(line)
		if roomName == "" {
			roomName = "general"
		}
	}

	ports := dynaport.Get(1)
	port := ports[0]

	cbor := remote.NewCBORSerializer()
	actorSystem, err := actor.NewActorSystem(
		"ChatSystem",
		actor.WithRemote(remote.NewConfig("127.0.0.1", port,
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
		return fmt.Errorf("failed to create actor system: %w", err)
	}

	if err := actorSystem.Start(ctx); err != nil {
		return fmt.Errorf("failed to start actor system: %w", err)
	}

	server, err := actorSystem.NoSender().RemoteLookup(ctx, clientServerHost, clientServerPort, "ChatServer")
	if err != nil {
		return fmt.Errorf("failed to lookup ChatServer: %w", err)
	}

	clientActor := newChatClient(userName, roomName, server)

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
		return fmt.Errorf("failed to spawn ChatClient: %w", err)
	}

	fmt.Println(clientHelpText)
	printClientPrompt(userName, roomName)

	done := make(chan struct{})

	go func() {
		defer close(done)
		for {
			input, err := reader.ReadString('\n')
			if err != nil {
				return
			}

			input = strings.TrimSpace(input)
			if input == "" {
				printClientPrompt(userName, clientActor.currentRoom())
				continue
			}

			if strings.HasPrefix(input, "/") {
				parts := strings.SplitN(input, " ", 3)
				cmd := strings.ToLower(parts[0])

				switch cmd {
				case "/quit":
					_ = client.Tell(ctx, server, &chatv2.Disconnect{})
					return

				case "/help":
					fmt.Print("\r" + clientHelpText + "\n")

				case "/users":
					_ = client.Tell(ctx, server, &chatv2.ListUsersRequest{
						Room: clientActor.currentRoom(),
					})

				case "/join":
					if len(parts) < 2 || parts[1] == "" {
						fmt.Print("\rUsage: /join <room>\n")
						break
					}
					newRoom := parts[1]
					_ = client.Tell(ctx, server, &chatv2.Disconnect{})

					time.Sleep(200 * time.Millisecond)

					clientActor.setRoom(newRoom)
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

				printClientPrompt(userName, clientActor.currentRoom())
				continue
			}

			_ = client.Tell(ctx, server, &chatv2.Message{
				UserName: userName,
				Content:  input,
				Room:     clientActor.currentRoom(),
				SentAt:   time.Now(),
			})

			printClientPrompt(userName, clientActor.currentRoom())
		}
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	select {
	case <-sig:
		_ = client.Tell(ctx, server, &chatv2.Disconnect{})
	case <-done:
	}

	_ = actorSystem.Stop(ctx)
	return nil
}

func printClientPrompt(user, room string) {
	fmt.Printf("[%s @ %s] > ", user, room)
}

type chatClient struct {
	userName string
	server   *actor.PID
	room     atomic.Value
}

var _ actor.Actor = (*chatClient)(nil)

func newChatClient(userName, room string, server *actor.PID) *chatClient {
	c := &chatClient{
		userName: userName,
		server:   server,
	}
	c.room.Store(room)
	return c
}

func (c *chatClient) currentRoom() string {
	return c.room.Load().(string)
}

func (c *chatClient) setRoom(room string) {
	c.room.Store(room)
}

func (c *chatClient) PreStart(*actor.Context) error {
	return nil
}

func (c *chatClient) Receive(ctx *actor.ReceiveContext) {
	switch msg := ctx.Message().(type) {
	case *actor.PostStart:
		ctx.Tell(c.server, &chatv2.Connect{
			UserName: c.userName,
			Room:     c.currentRoom(),
		})

	case *chatv2.Broadcast:
		ts := formatTime(msg.SentAt)
		fmt.Printf("\r[%s] [%s] %s: %s\n", ts, msg.Room, msg.FromUser, msg.Content)
		printClientPrompt(c.userName, c.currentRoom())

	case *chatv2.DirectMessage:
		ts := formatTime(msg.SentAt)
		fmt.Printf("\r[%s] [DM from %s]: %s\n", ts, msg.FromUser, msg.Content)
		printClientPrompt(c.userName, c.currentRoom())

	case *chatv2.SystemEvent:
		ts := formatTime(msg.At)
		fmt.Printf("\r[%s] *** %s ***\n", ts, msg.Text)
		printClientPrompt(c.userName, c.currentRoom())

	case *chatv2.ListUsersResponse:
		fmt.Printf("\rOnline in %s: %s\n", c.currentRoom(), strings.Join(msg.UserNames, ", "))
		printClientPrompt(c.userName, c.currentRoom())

	default:
		ctx.Unhandled()
	}
}

func (c *chatClient) PostStop(*actor.Context) error {
	return nil
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return "?"
	}
	return t.Local().Format("15:04:05")
}
