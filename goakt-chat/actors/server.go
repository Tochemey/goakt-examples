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

// Package actors holds the chat server and client actors. Both are written
// against the domain messages in internal/chat and delegate every wire-format
// concern to the wire package.
package actors

import (
	"fmt"
	"time"

	"github.com/tochemey/goakt/v4/actor"

	"github.com/tochemey/goakt-examples/v2/goakt-chat/wire"
	"github.com/tochemey/goakt-examples/v2/internal/chat"
)

const (
	// DefaultRoom is the room a client joins when it names none.
	DefaultRoom = "general"

	// maxHistorySize caps the per-room replay buffer.
	maxHistorySize = 20
)

// clientInfo tracks a connected client: where to reach it, who it is, which room
// it is in, and which wire format it speaks.
//
// Recording the codec per client is what lets one server serve protobuf and CBOR
// clients side by side — every message the server pushes is encoded with the same
// codec the recipient used to connect.
type clientInfo struct {
	pid      *actor.PID
	userName string
	room     string
	codec    wire.Codec
}

// Server is the central chat hub. It maintains a registry of connected clients
// grouped by room, broadcasts room messages, routes direct messages, replays
// history to new joiners, and notifies peers of join/leave events.
//
// No locking is needed: GoAkt guarantees that Receive is called for one message
// at a time, so all field access is inherently single-threaded.
type Server struct {
	clients map[string]*clientInfo       // key: sender actor path
	history map[string][]*chat.Broadcast // key: room name → ring buffer
}

var _ actor.Actor = (*Server)(nil)

// NewServer returns the chat hub actor.
func NewServer() *Server {
	return &Server{}
}

func (s *Server) PreStart(*actor.Context) error {
	s.clients = make(map[string]*clientInfo)
	s.history = make(map[string][]*chat.Broadcast)
	return nil
}

func (s *Server) Receive(ctx *actor.ReceiveContext) {
	// Normalize the inbound message to the domain model and note the format it
	// arrived in. codec is non-nil for every chat message.
	message, codec := wire.Decode(ctx.Message())

	switch msg := message.(type) {
	case *actor.PostStart:
		fmt.Println("Chat Server started — waiting for clients")

	case *chat.Connect:
		s.handleConnect(ctx, msg, codec)

	case *chat.Disconnect:
		s.handleDisconnect(ctx)

	case *chat.Message:
		s.handleMessage(ctx, msg)

	case *chat.DirectMessage:
		s.handleDirectMessage(ctx, msg)

	case *chat.ListUsersRequest:
		s.handleListUsers(ctx, msg, codec)

	default:
		ctx.Unhandled()
	}
}

func (s *Server) PostStop(*actor.Context) error {
	s.clients = make(map[string]*clientInfo)
	s.history = make(map[string][]*chat.Broadcast)
	fmt.Println("Chat Server stopped")
	return nil
}

// handleConnect registers a client, replays recent history, then notifies peers.
func (s *Server) handleConnect(ctx *actor.ReceiveContext, msg *chat.Connect, codec wire.Codec) {
	sender := ctx.Sender()
	key := sender.ID()

	room := msg.Room
	if room == "" {
		room = DefaultRoom
	}

	if _, exists := s.clients[key]; exists {
		fmt.Printf("client %s already connected\n", key)
		return
	}
	s.clients[key] = &clientInfo{pid: sender, userName: msg.UserName, room: room, codec: codec}

	history := make([]*chat.Broadcast, len(s.history[room]))
	copy(history, s.history[room])

	fmt.Printf("user=%q joined room=%q via %s from %s\n", msg.UserName, room, codec.Name(), key)

	// replay recent history to the newcomer, in the newcomer's own format
	for _, b := range history {
		ctx.Tell(sender, codec.Encode(b))
	}

	// notify everyone else in the room
	s.broadcastToRoom(ctx, room, key, &chat.SystemEvent{
		Text: msg.UserName + " joined " + room,
		At:   time.Now(),
	})
}

// handleDisconnect removes a client and notifies peers.
func (s *Server) handleDisconnect(ctx *actor.ReceiveContext) {
	key := ctx.Sender().ID()

	info, exists := s.clients[key]
	if !exists {
		fmt.Printf("disconnect from unknown client %s\n", key)
		return
	}
	delete(s.clients, key)

	fmt.Printf("user=%q left room=%q\n", info.userName, info.room)

	s.broadcastToRoom(ctx, info.room, key, &chat.SystemEvent{
		Text: info.userName + " left " + info.room,
		At:   time.Now(),
	})
}

// handleMessage fans out a room message to all peers and appends to history.
func (s *Server) handleMessage(ctx *actor.ReceiveContext, msg *chat.Message) {
	key := ctx.Sender().ID()

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

// handleDirectMessage routes a private message to the target user only.
func (s *Server) handleDirectMessage(ctx *actor.ReceiveContext, msg *chat.DirectMessage) {
	target := msg.ToUser

	var recipient *clientInfo
	for _, info := range s.clients {
		if info != nil && info.userName == target {
			recipient = info
			break
		}
	}

	if recipient == nil {
		fmt.Printf("direct message to unknown user %q\n", target)
		return
	}

	dm := &chat.DirectMessage{
		FromUser: msg.FromUser,
		ToUser:   target,
		Content:  msg.Content,
		SentAt:   time.Now(),
	}
	ctx.Tell(recipient.pid, recipient.codec.Encode(dm))
}

// handleListUsers replies with the list of users in the requested room. The reply
// uses the codec the request arrived in, so it works even before the sender has
// connected.
func (s *Server) handleListUsers(ctx *actor.ReceiveContext, msg *chat.ListUsersRequest, codec wire.Codec) {
	room := msg.Room
	if room == "" {
		room = DefaultRoom
	}

	var names []string
	for _, info := range s.clients {
		if info.room == room {
			names = append(names, info.userName)
		}
	}

	ctx.Tell(ctx.Sender(), codec.Encode(&chat.ListUsersResponse{UserNames: names}))
}

// broadcastToRoom sends a domain message to every client in room except the one
// identified by excludeKey, encoding it per recipient.
func (s *Server) broadcastToRoom(ctx *actor.ReceiveContext, room, excludeKey string, msg any) {
	for key, info := range s.clients {
		if key == excludeKey || info.room != room {
			continue
		}
		ctx.Tell(info.pid, info.codec.Encode(msg))
	}
}

// appendHistory adds a broadcast to the room's rolling history, capped at maxHistorySize.
func (s *Server) appendHistory(room string, b *chat.Broadcast) {
	buf := s.history[room]
	buf = append(buf, b)
	if len(buf) > maxHistorySize {
		buf = buf[len(buf)-maxHistorySize:]
	}
	s.history[room] = buf
}
