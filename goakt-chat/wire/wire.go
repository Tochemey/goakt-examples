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

// Package wire isolates the chat actors from the format their messages take on
// the network.
//
// The actors are written against a single domain model — the plain Go structs in
// internal/chat. A [Codec] turns those domain messages into the concrete types
// that actually travel between nodes:
//
//   - [CBORCodec] sends the domain structs as-is; they are CBOR-encoded by GoAkt.
//   - [ProtoCodec] translates them to the generated protobuf messages in
//     internal/chatpb, which GoAkt encodes with its default proto serializer.
//
// [Decode] handles the return trip for both families, so a node can receive
// either format regardless of the codec it sends with.
package wire

import (
	"fmt"
	"strings"
	"time"

	"github.com/tochemey/goakt/v4/remote"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/tochemey/goakt-examples/v2/internal/chat"
	"github.com/tochemey/goakt-examples/v2/internal/chatpb"
)

// Codec names accepted by [ParseCodec].
const (
	CBOR  = "cbor"
	Proto = "proto"
)

// Codec converts a domain message from internal/chat into the concrete type sent
// over the network.
type Codec interface {
	// Name returns the codec's flag value: "cbor" or "proto".
	Name() string
	// Encode converts a domain message into its wire representation. Messages it
	// does not recognize are returned unchanged.
	Encode(msg any) any
}

// The two supported wire families.
var (
	CBORCodec  Codec = cborCodec{}
	ProtoCodec Codec = protoCodec{}
)

// ParseCodec resolves a codec by name, as supplied on the command line.
func ParseCodec(name string) (Codec, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case CBOR:
		return CBORCodec, nil
	case Proto:
		return ProtoCodec, nil
	default:
		return nil, fmt.Errorf("unknown codec %q: want %q or %q", name, Proto, CBOR)
	}
}

// RemoteOptions returns the serializer registrations that every chat node — server
// and client alike — must apply to its remoting config.
//
// Both cmd/server.go and cmd/client.go call this single function so the two sides
// can never drift apart: a type registered on one end but not the other would fail
// to deserialize at runtime.
//
// Registering the concrete types matters as much as registering the ChatMessage
// interface. The interface entry is what selects CBOR for the whole family, but
// only a concrete registration adds the type to GoAkt's global type registry, and
// the CBOR frame carries a type name that the receiver resolves through it.
//
// The protobuf messages need no entry here: remote.NewConfig already registers the
// proto serializer for every proto.Message. Because the chat structs implement only
// chat.ChatMessage and the generated types implement only proto.Message, the two
// registrations never overlap — one node speaks both formats at once.
func RemoteOptions() []remote.Option {
	return []remote.Option{
		remote.WithSerializables(
			(*chat.ChatMessage)(nil),
			(*chat.Connect)(nil),
			(*chat.Disconnect)(nil),
			(*chat.Message)(nil),
			(*chat.DirectMessage)(nil),
			(*chat.ListUsersRequest)(nil),
			(*chat.ListUsersResponse)(nil),
			(*chat.Broadcast)(nil),
			(*chat.SystemEvent)(nil),
		),
	}
}

// Decode converts a message received off the wire into its domain equivalent and
// reports the codec family it arrived in.
//
// Messages that belong to neither family — actor lifecycle signals such as
// *actor.PostStart — are returned unchanged with a nil codec. Callers therefore
// only ever see a nil codec alongside a non-chat message.
func Decode(msg any) (any, Codec) {
	switch m := msg.(type) {
	case *chatpb.Connect:
		return &chat.Connect{UserName: m.GetUserName(), Room: m.GetRoom()}, ProtoCodec

	case *chatpb.Disconnect:
		return &chat.Disconnect{}, ProtoCodec

	case *chatpb.Message:
		return &chat.Message{
			UserName: m.GetUserName(),
			Content:  m.GetContent(),
			Room:     m.GetRoom(),
			SentAt:   goTime(m.GetSentAt()),
		}, ProtoCodec

	case *chatpb.DirectMessage:
		return &chat.DirectMessage{
			FromUser: m.GetFromUser(),
			ToUser:   m.GetToUser(),
			Content:  m.GetContent(),
			SentAt:   goTime(m.GetSentAt()),
		}, ProtoCodec

	case *chatpb.ListUsersRequest:
		return &chat.ListUsersRequest{Room: m.GetRoom()}, ProtoCodec

	case *chatpb.ListUsersResponse:
		return &chat.ListUsersResponse{UserNames: m.GetUserNames()}, ProtoCodec

	case *chatpb.Broadcast:
		return &chat.Broadcast{
			FromUser: m.GetFromUser(),
			Content:  m.GetContent(),
			Room:     m.GetRoom(),
			SentAt:   goTime(m.GetSentAt()),
		}, ProtoCodec

	case *chatpb.SystemEvent:
		return &chat.SystemEvent{Text: m.GetText(), At: goTime(m.GetAt())}, ProtoCodec

	case chat.ChatMessage:
		// Already a domain message: CBOR carries the structs unchanged.
		return m, CBORCodec

	default:
		return msg, nil
	}
}

// cborCodec sends the domain structs unchanged; GoAkt's CBOR serializer encodes
// them directly, so no translation is needed.
type cborCodec struct{}

func (cborCodec) Name() string { return CBOR }

func (cborCodec) Encode(msg any) any { return msg }

// protoCodec translates domain messages into the generated protobuf types.
type protoCodec struct{}

func (protoCodec) Name() string { return Proto }

func (protoCodec) Encode(msg any) any {
	switch m := msg.(type) {
	case *chat.Connect:
		return &chatpb.Connect{UserName: m.UserName, Room: m.Room}

	case *chat.Disconnect:
		return &chatpb.Disconnect{}

	case *chat.Message:
		return &chatpb.Message{
			UserName: m.UserName,
			Content:  m.Content,
			Room:     m.Room,
			SentAt:   protoTime(m.SentAt),
		}

	case *chat.DirectMessage:
		return &chatpb.DirectMessage{
			FromUser: m.FromUser,
			ToUser:   m.ToUser,
			Content:  m.Content,
			SentAt:   protoTime(m.SentAt),
		}

	case *chat.ListUsersRequest:
		return &chatpb.ListUsersRequest{Room: m.Room}

	case *chat.ListUsersResponse:
		return &chatpb.ListUsersResponse{UserNames: m.UserNames}

	case *chat.Broadcast:
		return &chatpb.Broadcast{
			FromUser: m.FromUser,
			Content:  m.Content,
			Room:     m.Room,
			SentAt:   protoTime(m.SentAt),
		}

	case *chat.SystemEvent:
		return &chatpb.SystemEvent{Text: m.Text, At: protoTime(m.At)}

	default:
		return msg
	}
}

// protoTime maps a zero time.Time to a nil timestamp so the two formats agree on
// what "unset" looks like.
func protoTime(t time.Time) *timestamppb.Timestamp {
	if t.IsZero() {
		return nil
	}
	return timestamppb.New(t)
}

// goTime is the inverse of protoTime.
func goTime(ts *timestamppb.Timestamp) time.Time {
	if ts == nil {
		return time.Time{}
	}
	return ts.AsTime()
}
