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
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/tochemey/goakt/v4/actor"
	"github.com/tochemey/goakt/v4/remote"
	"github.com/tochemey/goakt/v4/supervisor"
	"github.com/travisjeffery/go-dynaport"

	"github.com/tochemey/goakt-examples/v2/goakt-chat/actors"
	"github.com/tochemey/goakt-examples/v2/goakt-chat/wire"
	"github.com/tochemey/goakt-examples/v2/internal/chat"
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
	clientBindHost   string
	clientUser       string
	clientRoom       string
	clientCodec      string
)

var clientCmd = &cobra.Command{
	Use:   "client",
	Short: "Connect to the chat server as a client",
	Long: `Connect to the chat server as a chat client.

Interactive mode (default): omit --user and --room to be prompted for username and room.
Non-interactive mode: use --user and --room for scripting or automation.

--codec selects the wire format this client speaks: "cbor" sends the plain Go
structs from internal/chat, "proto" sends the generated protobuf messages from
internal/chatpb. The server accepts both, so clients using different codecs can
talk to each other in the same room.

The client binds its own remoting listener so the server can push messages back to
it. When connecting to a server on another machine, set --bind to an address that
server can reach.

After connecting, use slash commands: /help, /users, /join <room>, /dm <user> <msg>, /quit`,
	Example: `  chat client
  chat client --user alice --room general
  chat client --user bob --codec proto
  chat client --host 192.168.1.10 --port 4000 --bind 192.168.1.20`,
	RunE: runClient,
}

func init() {
	rootCmd.AddCommand(clientCmd)
	clientCmd.Flags().StringVar(&clientServerHost, "host", "127.0.0.1", "Server host to connect to")
	clientCmd.Flags().IntVar(&clientServerPort, "port", 4000, "Server port to connect to")
	clientCmd.Flags().StringVar(&clientBindHost, "bind", "127.0.0.1", "Host this client binds its own listener to")
	clientCmd.Flags().StringVar(&clientUser, "user", "", "Username (optional; prompts if not set)")
	clientCmd.Flags().StringVar(&clientRoom, "room", "", "Room name (optional; defaults to 'general')")
	clientCmd.Flags().StringVar(&clientCodec, "codec", wire.CBOR,
		fmt.Sprintf("Wire format to send with: %q or %q", wire.CBOR, wire.Proto))
}

func runClient(*cobra.Command, []string) error {
	ctx := context.Background()
	reader := bufio.NewReader(os.Stdin)

	codec, err := wire.ParseCodec(clientCodec)
	if err != nil {
		return err
	}

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
			roomName = actors.DefaultRoom
		}
	}

	ports := dynaport.Get(1)
	port := ports[0]

	// Same registrations as the server — see wire.RemoteOptions.
	actorSystem, err := actor.NewActorSystem(
		"ChatSystem",
		actor.WithRemote(remote.NewConfig(clientBindHost, port, wire.RemoteOptions()...)),
		actor.WithLoggingDisabled())

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

	clientActor := actors.NewClient(userName, roomName, server, codec)

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

	fmt.Printf("Connected to %s:%d using the %s codec\n", clientServerHost, clientServerPort, codec.Name())
	fmt.Println(clientHelpText)
	actors.PrintPrompt(userName, roomName)

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
				actors.PrintPrompt(userName, clientActor.CurrentRoom())
				continue
			}

			// slash-command dispatch
			if strings.HasPrefix(input, "/") {
				parts := strings.SplitN(input, " ", 3)
				command := strings.ToLower(parts[0])

				switch command {
				case "/quit":
					_ = clientActor.Send(ctx, client, &chat.Disconnect{})
					return

				case "/help":
					fmt.Print("\r" + clientHelpText + "\n")

				case "/users":
					_ = clientActor.Send(ctx, client, &chat.ListUsersRequest{
						Room: clientActor.CurrentRoom(),
					})

				case "/join":
					if len(parts) < 2 || parts[1] == "" {
						fmt.Print("\rUsage: /join <room>\n")
						break
					}
					newRoom := parts[1]
					_ = clientActor.Send(ctx, client, &chat.Disconnect{})

					time.Sleep(200 * time.Millisecond)

					clientActor.SetRoom(newRoom)
					_ = clientActor.Send(ctx, client, &chat.Connect{
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
					_ = clientActor.Send(ctx, client, &chat.DirectMessage{
						FromUser: userName,
						ToUser:   parts[1],
						Content:  parts[2],
						SentAt:   time.Now(),
					})

				default:
					fmt.Printf("\rUnknown command: %s  (type /help)\n", command)
				}

				actors.PrintPrompt(userName, clientActor.CurrentRoom())
				continue
			}

			// plain message → broadcast to room
			_ = clientActor.Send(ctx, client, &chat.Message{
				UserName: userName,
				Content:  input,
				Room:     clientActor.CurrentRoom(),
				SentAt:   time.Now(),
			})

			actors.PrintPrompt(userName, clientActor.CurrentRoom())
		}
	}()

	// wait for Ctrl-C or the input loop to finish
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	select {
	case <-sig:
		_ = clientActor.Send(ctx, client, &chat.Disconnect{})
	case <-done:
	}

	_ = actorSystem.Stop(ctx)
	return nil
}
