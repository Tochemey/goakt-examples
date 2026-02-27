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

package chatv2

import "time"

// ChatMessage is the common interface implemented by all chat v2 message types.
// Register this interface with WithSerializers to use CBOR for all chat messages.
type ChatMessage interface {
	chatMessage()
}

// Connect is sent by a client to join the chat server in a given room.
// If room is empty, the client joins the default "general" room.
type Connect struct {
	UserName string `cbor:"user_name,omitempty"`
	Room     string `cbor:"room,omitempty"`
}

func (*Connect) chatMessage() {}

// Disconnect is sent by a client to leave the chat.
type Disconnect struct{}

func (*Disconnect) chatMessage() {}

// Message is sent by a client to post a message to its current room.
type Message struct {
	UserName string    `cbor:"user_name,omitempty"`
	Content  string    `cbor:"content,omitempty"`
	Room     string    `cbor:"room,omitempty"`
	SentAt   time.Time `cbor:"sent_at,omitempty"`
}

func (*Message) chatMessage() {}

// DirectMessage is sent by a client to privately address another user.
type DirectMessage struct {
	FromUser string    `cbor:"from_user,omitempty"`
	ToUser   string    `cbor:"to_user,omitempty"`
	Content  string    `cbor:"content,omitempty"`
	SentAt   time.Time `cbor:"sent_at,omitempty"`
}

func (*DirectMessage) chatMessage() {}

// ListUsersRequest is sent by a client to retrieve online users in a room.
type ListUsersRequest struct {
	Room string `cbor:"room,omitempty"`
}

func (*ListUsersRequest) chatMessage() {}

// ListUsersResponse is sent by the server back to the requesting client.
type ListUsersResponse struct {
	UserNames []string `cbor:"user_names,omitempty"`
}

func (*ListUsersResponse) chatMessage() {}

// Broadcast is pushed by the server to every client in a room when a new
// message arrives.
type Broadcast struct {
	FromUser string    `cbor:"from_user,omitempty"`
	Content  string    `cbor:"content,omitempty"`
	Room     string    `cbor:"room,omitempty"`
	SentAt   time.Time `cbor:"sent_at,omitempty"`
}

func (*Broadcast) chatMessage() {}

// SystemEvent carries join/leave and other server-generated notifications
// pushed to clients in a room.
type SystemEvent struct {
	Text string    `cbor:"text,omitempty"`
	At   time.Time `cbor:"at,omitempty"`
}

func (*SystemEvent) chatMessage() {}
