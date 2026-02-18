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
	"os"
	"os/signal"
	"syscall"
	"time"

	actors "github.com/tochemey/goakt/v3/actor"
	"github.com/tochemey/goakt/v3/address"
	"github.com/tochemey/goakt/v3/goaktpb"
	"github.com/tochemey/goakt/v3/log"
	"github.com/tochemey/goakt/v3/remote"
	"github.com/tochemey/goakt/v3/supervisor"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/tochemey/goakt-examples/v2/internal/chatpb"
)

const (
	defaultRoom    = "general"
	maxHistorySize = 20
)

func main() {
	ctx := context.Background()
	host := "127.0.0.1"
	port := 4000

	logger := log.New(log.InfoLevel, os.Stdout)

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

	if _, err := actorSystem.Spawn(
		ctx,
		"ChatServer",
		NewServer(),
		actors.WithSupervisor(
			supervisor.NewSupervisor(
				supervisor.WithStrategy(supervisor.OneForOneStrategy),
				supervisor.WithAnyErrorDirective(supervisor.ResumeDirective),
			))); err != nil {
		logger.Fatal(err)
		os.Exit(1)
	}

	interruptSignal := make(chan os.Signal, 1)
	signal.Notify(interruptSignal, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	<-interruptSignal

	_ = actorSystem.Stop(ctx)
	os.Exit(0)
}

// clientInfo holds the remote address and room for a connected client.
type clientInfo struct {
	addr     *address.Address
	userName string
	room     string
}

// Server is the central chat hub. It maintains a registry of connected clients
// grouped by room, broadcasts room messages, routes direct messages, replays
// history to new joiners, and notifies peers of join/leave events.
//
// No locking is needed: GoAkt guarantees that Receive is called for one message
// at a time, so all field access is inherently single-threaded.
type Server struct {
	logger  log.Logger
	clients map[string]*clientInfo         // key: sender address string
	history map[string][]*chatpb.Broadcast // key: room name → ring buffer
}

var _ actors.Actor = (*Server)(nil)

func NewServer() *Server {
	return &Server{}
}

func (s *Server) PreStart(ctx *actors.Context) error {
	ctx.ActorSystem().Logger().Info("Chat Server starting")
	s.clients = make(map[string]*clientInfo)
	s.history = make(map[string][]*chatpb.Broadcast)
	return nil
}

func (s *Server) Receive(ctx *actors.ReceiveContext) {
	switch msg := ctx.Message().(type) {
	case *goaktpb.PostStart:
		s.logger = ctx.Logger()
		s.logger.Info("Chat Server started — waiting for clients")

	case *chatpb.Connect:
		s.handleConnect(ctx, msg)

	case *chatpb.Disconnect:
		s.handleDisconnect(ctx)

	case *chatpb.Message:
		s.handleMessage(ctx, msg)

	case *chatpb.DirectMessage:
		s.handleDirectMessage(ctx, msg)

	case *chatpb.ListUsersRequest:
		s.handleListUsers(ctx, msg)

	default:
		ctx.Unhandled()
	}
}

func (s *Server) PostStop(*actors.Context) error {
	s.clients = make(map[string]*clientInfo)
	s.history = make(map[string][]*chatpb.Broadcast)
	s.logger.Info("Chat Server stopped")
	return nil
}

// handleConnect registers a client, replays recent history, then notifies peers.
func (s *Server) handleConnect(ctx *actors.ReceiveContext, msg *chatpb.Connect) {
	sender := ctx.RemoteSender()
	key := sender.String()

	room := msg.GetRoom()
	if room == "" {
		room = defaultRoom
	}

	if _, exists := s.clients[key]; exists {
		s.logger.Warnf("client %s already connected", key)
		return
	}
	s.clients[key] = &clientInfo{addr: sender, userName: msg.GetUserName(), room: room}
	history := make([]*chatpb.Broadcast, len(s.history[room]))
	copy(history, s.history[room])

	s.logger.Infof("user=%q joined room=%q from %s", msg.GetUserName(), room, key)

	// replay recent history to the newcomer
	for _, b := range history {
		ctx.RemoteTell(sender, b)
	}

	// notify everyone else in the room
	event := &chatpb.SystemEvent{
		Text: msg.GetUserName() + " joined " + room,
		At:   timestamppb.Now(),
	}
	s.broadcastToRoom(ctx, room, key, event)
}

// handleDisconnect removes a client and notifies peers.
func (s *Server) handleDisconnect(ctx *actors.ReceiveContext) {
	sender := ctx.RemoteSender()
	key := sender.String()

	info, exists := s.clients[key]
	if !exists {
		s.logger.Warnf("disconnect from unknown client %s", key)
		return
	}
	delete(s.clients, key)

	s.logger.Infof("user=%q left room=%q", info.userName, info.room)

	event := &chatpb.SystemEvent{
		Text: info.userName + " left " + info.room,
		At:   timestamppb.Now(),
	}
	s.broadcastToRoom(ctx, info.room, key, event)
}

// handleMessage fans out a room message to all peers and appends to history.
func (s *Server) handleMessage(ctx *actors.ReceiveContext, msg *chatpb.Message) {
	sender := ctx.RemoteSender()
	key := sender.String()

	info, exists := s.clients[key]
	if !exists {
		s.logger.Warnf("message from unknown client %s — ignored", key)
		return
	}

	room := msg.GetRoom()
	if room == "" {
		room = info.room
	}

	broadcast := &chatpb.Broadcast{
		FromUser: info.userName,
		Content:  msg.GetContent(),
		Room:     room,
		SentAt:   timestamppb.Now(),
	}

	s.appendHistory(room, broadcast)
	s.broadcastToRoom(ctx, room, key, broadcast)
}

// handleDirectMessage routes a private message to the target user only.
func (s *Server) handleDirectMessage(ctx *actors.ReceiveContext, msg *chatpb.DirectMessage) {
	target := msg.GetToUser()

	var targetAddr *address.Address
	for _, info := range s.clients {
		if info.userName == target {
			targetAddr = info.addr
			break
		}
	}

	if targetAddr == nil {
		s.logger.Warnf("direct message to unknown user %q", target)
		return
	}

	dm := &chatpb.DirectMessage{
		FromUser: msg.GetFromUser(),
		ToUser:   target,
		Content:  msg.GetContent(),
		SentAt:   timestamppb.Now(),
	}
	ctx.RemoteTell(targetAddr, dm)
}

// handleListUsers replies with the list of users in the requested room.
func (s *Server) handleListUsers(ctx *actors.ReceiveContext, msg *chatpb.ListUsersRequest) {
	room := msg.GetRoom()
	if room == "" {
		room = defaultRoom
	}

	var names []string
	for _, info := range s.clients {
		if info.room == room {
			names = append(names, info.userName)
		}
	}

	ctx.RemoteTell(ctx.RemoteSender(), &chatpb.ListUsersResponse{UserNames: names})
}

// broadcastToRoom sends msg to every client in room except the one identified by excludeKey.
func (s *Server) broadcastToRoom(ctx *actors.ReceiveContext, room, excludeKey string, msg proto.Message) {
	for key, info := range s.clients {
		if key == excludeKey || info.room != room {
			continue
		}
		ctx.RemoteTell(info.addr, msg)
	}
}

// appendHistory adds a broadcast to the room's rolling history, capped at maxHistorySize.
func (s *Server) appendHistory(room string, b *chatpb.Broadcast) {
	buf := s.history[room]
	buf = append(buf, b)
	if len(buf) > maxHistorySize {
		buf = buf[len(buf)-maxHistorySize:]
	}
	s.history[room] = buf
}
