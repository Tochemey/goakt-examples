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
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/tochemey/goakt/v4/actor"
	"github.com/tochemey/goakt/v4/remote"
	"github.com/tochemey/goakt/v4/supervisor"

	"github.com/tochemey/goakt-examples/v2/internal/chatv2"
)

const (
	defaultRoom    = "general"
	maxHistorySize = 20
)

func main() {
	ctx := context.Background()
	host := "127.0.0.1"
	port := 4000

	// Use DiscardLogger to disable GoAkt logging
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
		actor.WithLoggingDisabled())

	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to create actor system:", err)
		os.Exit(1)
	}

	if err := actorSystem.Start(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "failed to start actor system:", err)
		os.Exit(1)
	}

	if _, err := actorSystem.Spawn(
		ctx,
		"ChatServer",
		NewServer(),
		actor.WithSupervisor(
			supervisor.NewSupervisor(
				supervisor.WithStrategy(supervisor.OneForOneStrategy),
				supervisor.WithAnyErrorDirective(supervisor.ResumeDirective),
			))); err != nil {
		fmt.Fprintln(os.Stderr, "failed to spawn ChatServer:", err)
		os.Exit(1)
	}

	fmt.Println("Chat Server is running on", host, "port", port)

	interruptSignal := make(chan os.Signal, 1)
	signal.Notify(interruptSignal, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	<-interruptSignal

	_ = actorSystem.Stop(ctx)
	os.Exit(0)
}

// clientInfo holds the remote address and room for a connected client.
type clientInfo struct {
	pid      *actor.PID
	userName string
	room     string
}

// Server is the central chat hub. It maintains a registry of connected clients
// grouped by room, broadcasts room messages, routes direct messages, replays
// history to new joiners, and notifies peers of join/leave events.
type Server struct {
	clients map[string]*clientInfo
	history map[string][]*chatv2.Broadcast
}

var _ actor.Actor = (*Server)(nil)

func NewServer() *Server {
	return &Server{}
}

func (s *Server) PreStart(ctx *actor.Context) error {
	s.clients = make(map[string]*clientInfo)
	s.history = make(map[string][]*chatv2.Broadcast)
	return nil
}

func (s *Server) Receive(ctx *actor.ReceiveContext) {
	switch msg := ctx.Message().(type) {
	case *actor.PostStart:
		s.handlePostStart(ctx)

	case *chatv2.Connect:
		s.handleConnect(ctx, msg)

	case *chatv2.Disconnect:
		s.handleDisconnect(ctx)

	case *chatv2.Message:
		s.handleMessage(ctx, msg)

	case *chatv2.DirectMessage:
		s.handleDirectMessage(ctx, msg)

	case *chatv2.ListUsersRequest:
		s.handleListUsers(ctx, msg)

	default:
		ctx.Unhandled()
	}
}

func (s *Server) handlePostStart(ctx *actor.ReceiveContext) {
	fmt.Println("Chat Server started — waiting for clients")
}

func (s *Server) PostStop(*actor.Context) error {
	s.clients = make(map[string]*clientInfo)
	s.history = make(map[string][]*chatv2.Broadcast)
	fmt.Println("Chat Server stopped")
	return nil
}

// handleConnect registers a client, replays recent history, then notifies peers.
func (s *Server) handleConnect(ctx *actor.ReceiveContext, msg *chatv2.Connect) {
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
	history := make([]*chatv2.Broadcast, len(s.history[room]))
	copy(history, s.history[room])

	fmt.Printf("user=%q joined room=%q from %s\n", msg.UserName, room, key)

	// replay recent history to the newcomer
	for _, b := range history {
		ctx.Tell(sender, b)
	}

	// notify everyone else in the room
	event := &chatv2.SystemEvent{
		Text: msg.UserName + " joined " + room,
		At:   time.Now(),
	}
	s.broadcastToRoom(ctx, room, key, event)
}

// handleDisconnect removes a client and notifies peers.
func (s *Server) handleDisconnect(ctx *actor.ReceiveContext) {
	sender := ctx.Sender()
	key := sender.ID()

	info, exists := s.clients[key]
	if !exists {
		fmt.Printf("disconnect from unknown client %s\n", key)
		return
	}
	delete(s.clients, key)

	fmt.Printf("user=%q left room=%q\n", info.userName, info.room)

	event := &chatv2.SystemEvent{
		Text: info.userName + " left " + info.room,
		At:   time.Now(),
	}
	s.broadcastToRoom(ctx, info.room, key, event)
}

// handleMessage fans out a room message to all peers and appends to history.
func (s *Server) handleMessage(ctx *actor.ReceiveContext, msg *chatv2.Message) {
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

	broadcast := &chatv2.Broadcast{
		FromUser: info.userName,
		Content:  msg.Content,
		Room:     room,
		SentAt:   time.Now(),
	}

	s.appendHistory(room, broadcast)
	s.broadcastToRoom(ctx, room, key, broadcast)
}

// handleDirectMessage routes a private message to the target user only.
func (s *Server) handleDirectMessage(ctx *actor.ReceiveContext, msg *chatv2.DirectMessage) {
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

	dm := &chatv2.DirectMessage{
		FromUser: msg.FromUser,
		ToUser:   target,
		Content:  msg.Content,
		SentAt:   time.Now(),
	}
	ctx.Tell(targetPID, dm)
}

// handleListUsers replies with the list of users in the requested room.
func (s *Server) handleListUsers(ctx *actor.ReceiveContext, msg *chatv2.ListUsersRequest) {
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

	ctx.Tell(ctx.Sender(), &chatv2.ListUsersResponse{UserNames: names})
}

// broadcastToRoom sends msg to every client in room except the one identified by excludeKey.
func (s *Server) broadcastToRoom(ctx *actor.ReceiveContext, room, excludeKey string, msg any) {
	for key, info := range s.clients {
		if key == excludeKey || info.room != room {
			continue
		}
		ctx.Tell(info.pid, msg)
	}
}

// appendHistory adds a broadcast to the room's rolling history, capped at maxHistorySize.
func (s *Server) appendHistory(room string, b *chatv2.Broadcast) {
	buf := s.history[room]
	buf = append(buf, b)
	if len(buf) > maxHistorySize {
		buf = buf[len(buf)-maxHistorySize:]
	}
	s.history[room] = buf
}
