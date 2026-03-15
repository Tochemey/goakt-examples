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
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/tochemey/goakt/v4/actor"
	"github.com/tochemey/goakt/v4/remote"
	"github.com/tochemey/goakt/v4/supervisor"

	"github.com/tochemey/goakt-examples/v2/internal/chat"
)

const (
	defaultRoom    = "general"
	maxHistorySize = 20
)

var (
	serverHost string
	serverPort int
)

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Start the chat server",
	Long: `Start the chat server. Clients connect to this server to chat in rooms.

The server binds to the specified host and port. Start the server before
connecting any clients. Use --host 0.0.0.0 to accept connections from other machines.`,
	Example: `  chat server
  chatv2 server --host 0.0.0.0 --port 5000`,
	RunE: runServer,
}

func init() {
	rootCmd.AddCommand(serverCmd)
	serverCmd.Flags().StringVar(&serverHost, "host", "127.0.0.1", "Host to bind the server to")
	serverCmd.Flags().IntVar(&serverPort, "port", 4000, "Port to listen on")
}

func runServer(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	cbor := remote.NewCBORSerializer()
	actorSystem, err := actor.NewActorSystem(
		"ChatSystem",
		actor.WithRemote(remote.NewConfig(serverHost, serverPort,
			remote.WithSerializers((*chat.ChatMessage)(nil), cbor),
			remote.WithSerializers((*chat.Connect)(nil), cbor),
			remote.WithSerializers((*chat.Disconnect)(nil), cbor),
			remote.WithSerializers((*chat.Message)(nil), cbor),
			remote.WithSerializers((*chat.DirectMessage)(nil), cbor),
			remote.WithSerializers((*chat.ListUsersRequest)(nil), cbor),
			remote.WithSerializers((*chat.ListUsersResponse)(nil), cbor),
			remote.WithSerializers((*chat.Broadcast)(nil), cbor),
			remote.WithSerializers((*chat.SystemEvent)(nil), cbor),
		)),
		actor.WithLoggingDisabled())

	if err != nil {
		return fmt.Errorf("failed to create actor system: %w", err)
	}

	if err := actorSystem.Start(ctx); err != nil {
		return fmt.Errorf("failed to start actor system: %w", err)
	}

	if _, err := actorSystem.Spawn(
		ctx,
		"ChatServer",
		newChatServer(),
		actor.WithSupervisor(
			supervisor.NewSupervisor(
				supervisor.WithStrategy(supervisor.OneForOneStrategy),
				supervisor.WithAnyErrorDirective(supervisor.ResumeDirective),
			))); err != nil {
		return fmt.Errorf("failed to spawn ChatServer: %w", err)
	}

	fmt.Println("Chat Server is running on", serverHost, "port", serverPort)

	interruptSignal := make(chan os.Signal, 1)
	signal.Notify(interruptSignal, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	<-interruptSignal

	_ = actorSystem.Stop(ctx)
	return nil
}

// clientInfo holds the remote address and room for a connected client.
type clientInfo struct {
	pid      *actor.PID
	userName string
	room     string
}

// chatServer is the central chat hub. It maintains a registry of connected clients
// grouped by room, broadcasts room messages, routes direct messages, replays
// history to new joiners, and notifies peers of join/leave events.
type chatServer struct {
	clients map[string]*clientInfo
	history map[string][]*chat.Broadcast
}

var _ actor.Actor = (*chatServer)(nil)

func newChatServer() *chatServer {
	return &chatServer{}
}

func (s *chatServer) PreStart(ctx *actor.Context) error {
	s.clients = make(map[string]*clientInfo)
	s.history = make(map[string][]*chat.Broadcast)
	return nil
}

func (s *chatServer) Receive(ctx *actor.ReceiveContext) {
	switch msg := ctx.Message().(type) {
	case *actor.PostStart:
		s.handlePostStart(ctx)

	case *chat.Connect:
		s.handleConnect(ctx, msg)

	case *chat.Disconnect:
		s.handleDisconnect(ctx)

	case *chat.Message:
		s.handleMessage(ctx, msg)

	case *chat.DirectMessage:
		s.handleDirectMessage(ctx, msg)

	case *chat.ListUsersRequest:
		s.handleListUsers(ctx, msg)

	default:
		ctx.Unhandled()
	}
}

func (s *chatServer) handlePostStart(ctx *actor.ReceiveContext) {
	fmt.Println("Chat Server started — waiting for clients")
}

func (s *chatServer) PostStop(*actor.Context) error {
	s.clients = make(map[string]*clientInfo)
	s.history = make(map[string][]*chat.Broadcast)
	fmt.Println("Chat Server stopped")
	return nil
}

func (s *chatServer) handleConnect(ctx *actor.ReceiveContext, msg *chat.Connect) {
	sender := ctx.Sender()
	key := sender.ID()

	room := msg.Room
	if room == "" {
		room = defaultRoom
	}

	if _, exists := s.clients[key]; exists {
		fmt.Printf("client %s already connected\n", key)
		return
	}
	s.clients[key] = &clientInfo{pid: sender, userName: msg.UserName, room: room}
	history := make([]*chat.Broadcast, len(s.history[room]))
	copy(history, s.history[room])

	fmt.Printf("user=%q joined room=%q from %s\n", msg.UserName, room, key)

	for _, b := range history {
		ctx.Tell(sender, b)
	}

	event := &chat.SystemEvent{
		Text: msg.UserName + " joined " + room,
		At:   time.Now(),
	}
	s.broadcastToRoom(ctx, room, key, event)
}

func (s *chatServer) handleDisconnect(ctx *actor.ReceiveContext) {
	sender := ctx.Sender()
	key := sender.ID()

	info, exists := s.clients[key]
	if !exists {
		fmt.Printf("disconnect from unknown client %s\n", key)
		return
	}
	delete(s.clients, key)

	fmt.Printf("user=%q left room=%q\n", info.userName, info.room)

	event := &chat.SystemEvent{
		Text: info.userName + " left " + info.room,
		At:   time.Now(),
	}
	s.broadcastToRoom(ctx, info.room, key, event)
}

func (s *chatServer) handleMessage(ctx *actor.ReceiveContext, msg *chat.Message) {
	sender := ctx.Sender()
	key := sender.ID()

	info, exists := s.clients[key]
	if !exists {
		fmt.Printf("message from unknown client %s — ignored\n", key)
		return
	}

	room := msg.Room
	if room == "" {
		room = info.room
	}

	broadcast := &chat.Broadcast{
		FromUser: info.userName,
		Content:  msg.Content,
		Room:     room,
		SentAt:   time.Now(),
	}

	s.appendHistory(room, broadcast)
	s.broadcastToRoom(ctx, room, key, broadcast)
}

func (s *chatServer) handleDirectMessage(ctx *actor.ReceiveContext, msg *chat.DirectMessage) {
	target := msg.ToUser

	var targetPID *actor.PID
	for _, info := range s.clients {
		if info != nil && info.userName == target {
			targetPID = info.pid
			break
		}
	}

	if targetPID == nil {
		fmt.Printf("direct message to unknown user %q\n", target)
		return
	}

	dm := &chat.DirectMessage{
		FromUser: msg.FromUser,
		ToUser:   target,
		Content:  msg.Content,
		SentAt:   time.Now(),
	}
	ctx.Tell(targetPID, dm)
}

func (s *chatServer) handleListUsers(ctx *actor.ReceiveContext, msg *chat.ListUsersRequest) {
	room := msg.Room
	if room == "" {
		room = defaultRoom
	}

	var names []string
	for _, info := range s.clients {
		if info.room == room {
			names = append(names, info.userName)
		}
	}

	ctx.Tell(ctx.Sender(), &chat.ListUsersResponse{UserNames: names})
}

func (s *chatServer) broadcastToRoom(ctx *actor.ReceiveContext, room, excludeKey string, msg any) {
	for key, info := range s.clients {
		if key == excludeKey || info.room != room {
			continue
		}
		ctx.Tell(info.pid, msg)
	}
}

func (s *chatServer) appendHistory(room string, b *chat.Broadcast) {
	buf := s.history[room]
	buf = append(buf, b)
	if len(buf) > maxHistorySize {
		buf = buf[len(buf)-maxHistorySize:]
	}
	s.history[room] = buf
}
