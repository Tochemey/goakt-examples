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

// Package wire isolates the account actors from the format their messages take
// on the network.
//
// Actors are written against the plain Go structs in the messages package. A
// [Codec] turns those domain messages into the concrete types that travel
// between nodes:
//
//   - [CBORCodec] sends the domain structs as-is; GoAkt CBOR-encodes them.
//   - [ProtoCodec] translates them to the generated protobuf messages in
//     internal/samplepb, which GoAkt encodes with its default proto serializer.
//
// Unlike goakt-chat, a k8s process runs in exactly one mode for its whole life —
// either full CBOR or full protobuf — selected by --codec / CODEC. Remoting
// registers only the serializers for that mode.
package wire

import (
	"fmt"
	"strings"

	"github.com/tochemey/goakt/v4/remote"

	"github.com/tochemey/goakt-examples/v2/goakt-cluster/k8s/messages"
	"github.com/tochemey/goakt-examples/v2/internal/samplepb"
)

// Codec names accepted by [ParseCodec].
const (
	CBOR  = "cbor"
	Proto = "proto"
)

// Codec converts a domain message from messages into the concrete type sent
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

// ParseCodec resolves a codec by name, as supplied on the command line or via CODEC.
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

// RemoteOptions returns the remoting options for the selected codec.
//
// CBOR mode registers the domain structs with the CBOR serializer. Proto mode
// registers nothing — remote.NewConfig already handles every proto.Message —
// so the process never advertises CBOR types.
func RemoteOptions(codec Codec) []remote.Option {
	if codec == nil || codec.Name() == Proto {
		return nil
	}
	return []remote.Option{
		remote.WithSerializables(
			(*messages.CreateAccount)(nil),
			(*messages.CreditAccount)(nil),
			(*messages.GetAccount)(nil),
			(*messages.Account)(nil),
		),
	}
}

// Decode converts a message received off the wire into its domain equivalent and
// reports the codec family it arrived in.
//
// Messages that belong to neither family — actor lifecycle signals such as
// *actor.PostStart — are returned unchanged with a nil codec.
func Decode(msg any) (any, Codec) {
	switch m := msg.(type) {
	case *samplepb.CreateAccount:
		return &messages.CreateAccount{
			AccountID:      m.GetAccountId(),
			AccountBalance: m.GetAccountBalance(),
		}, ProtoCodec

	case *samplepb.CreditAccount:
		return &messages.CreditAccount{
			AccountID: m.GetAccountId(),
			Balance:   m.GetBalance(),
		}, ProtoCodec

	case *samplepb.GetAccount:
		return &messages.GetAccount{AccountID: m.GetAccountId()}, ProtoCodec

	case *samplepb.Account:
		return &messages.Account{
			AccountID:      m.GetAccountId(),
			AccountBalance: m.GetAccountBalance(),
		}, ProtoCodec

	case *messages.CreateAccount, *messages.CreditAccount, *messages.GetAccount, *messages.Account:
		return m, CBORCodec

	default:
		return msg, nil
	}
}

type cborCodec struct{}

func (cborCodec) Name() string { return CBOR }

func (cborCodec) Encode(msg any) any { return msg }

type protoCodec struct{}

func (protoCodec) Name() string { return Proto }

func (protoCodec) Encode(msg any) any {
	switch m := msg.(type) {
	case *messages.CreateAccount:
		return &samplepb.CreateAccount{
			AccountId:      m.AccountID,
			AccountBalance: m.AccountBalance,
		}
	case *messages.CreditAccount:
		return &samplepb.CreditAccount{
			AccountId: m.AccountID,
			Balance:   m.Balance,
		}
	case *messages.GetAccount:
		return &samplepb.GetAccount{AccountId: m.AccountID}
	case *messages.Account:
		return &samplepb.Account{
			AccountId:      m.AccountID,
			AccountBalance: m.AccountBalance,
		}
	default:
		return msg
	}
}
