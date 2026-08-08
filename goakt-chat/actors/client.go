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

package actors

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/tochemey/goakt/v4/actor"

	"github.com/tochemey/goakt-examples/v2/goakt-chat/wire"
	"github.com/tochemey/goakt-examples/v2/internal/chat"
)

// Client receives messages pushed by the server and prints them to stdout.
type Client struct {
	userName string
	server   *actor.PID
	codec    wire.Codec

	room atomic.Value // string — current room name
}

var _ actor.Actor = (*Client)(nil)

// NewClient returns a client actor that talks to server using codec.
func NewClient(userName, room string, server *actor.PID, codec wire.Codec) *Client {
	c := &Client{
		userName: userName,
		server:   server,
		codec:    codec,
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

// Send encodes a domain message with the client's codec and sends it to the chat
// server. self is the PID the client actor was spawned as; it becomes the sender
// the server replies to.
//
// Routing every outbound message through here keeps encoding in one place — the
// command layer only ever deals in domain types.
func (c *Client) Send(ctx context.Context, self *actor.PID, msg any) error {
	return self.Tell(ctx, c.server, c.codec.Encode(msg))
}

func (c *Client) PreStart(*actor.Context) error {
	return nil
}

func (c *Client) Receive(ctx *actor.ReceiveContext) {
	message, _ := wire.Decode(ctx.Message())

	switch msg := message.(type) {
	case *actor.PostStart:
		ctx.Tell(c.server, c.codec.Encode(&chat.Connect{
			UserName: c.userName,
			Room:     c.CurrentRoom(),
		}))

	case *chat.Broadcast:
		fmt.Printf("\r[%s] [%s] %s: %s\n", formatTime(msg.SentAt), msg.Room, msg.FromUser, msg.Content)
		PrintPrompt(c.userName, c.CurrentRoom())

	case *chat.DirectMessage:
		fmt.Printf("\r[%s] [DM from %s]: %s\n", formatTime(msg.SentAt), msg.FromUser, msg.Content)
		PrintPrompt(c.userName, c.CurrentRoom())

	case *chat.SystemEvent:
		fmt.Printf("\r[%s] *** %s ***\n", formatTime(msg.At), msg.Text)
		PrintPrompt(c.userName, c.CurrentRoom())

	case *chat.ListUsersResponse:
		fmt.Printf("\rOnline in %s: %s\n", c.CurrentRoom(), strings.Join(msg.UserNames, ", "))
		PrintPrompt(c.userName, c.CurrentRoom())

	default:
		ctx.Unhandled()
	}
}

func (c *Client) PostStop(*actor.Context) error {
	return nil
}

// PrintPrompt re-prints the input prompt after output is written.
func PrintPrompt(user, room string) {
	fmt.Printf("[%s @ %s] > ", user, room)
}

// formatTime renders a timestamp as HH:MM:SS, or "?" when unset.
func formatTime(t time.Time) string {
	if t.IsZero() {
		return "?"
	}
	return t.Local().Format("15:04:05")
}
