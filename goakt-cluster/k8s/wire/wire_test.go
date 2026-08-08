// MIT License
//
// Copyright (c) 2022-2026 GoAkt Team

package wire

import (
	"testing"

	"github.com/tochemey/goakt-examples/v2/goakt-cluster/k8s/messages"
	"github.com/tochemey/goakt-examples/v2/internal/samplepb"
)

func TestParseCodec(t *testing.T) {
	c, err := ParseCodec("CBOR")
	if err != nil || c.Name() != CBOR {
		t.Fatalf("ParseCodec(cbor): got %#v err=%v", c, err)
	}
	c, err = ParseCodec("proto")
	if err != nil || c.Name() != Proto {
		t.Fatalf("ParseCodec(proto): got %#v err=%v", c, err)
	}
	if _, err := ParseCodec("json"); err == nil {
		t.Fatal("expected error for unknown codec")
	}
}

func TestProtoRoundTrip(t *testing.T) {
	domain := &messages.CreateAccount{AccountID: "a1", AccountBalance: 42.5}
	encoded := ProtoCodec.Encode(domain)
	pb, ok := encoded.(*samplepb.CreateAccount)
	if !ok {
		t.Fatalf("Encode type: %T", encoded)
	}
	decoded, codec := Decode(pb)
	if codec.Name() != Proto {
		t.Fatalf("codec=%s", codec.Name())
	}
	got, ok := decoded.(*messages.CreateAccount)
	if !ok || got.AccountID != "a1" || got.AccountBalance != 42.5 {
		t.Fatalf("Decode: %#v", decoded)
	}
}

func TestCBORIdentity(t *testing.T) {
	domain := &messages.CreditAccount{AccountID: "a1", Balance: 10}
	if CBORCodec.Encode(domain) != domain {
		t.Fatal("CBOR Encode should be identity")
	}
	decoded, codec := Decode(domain)
	if codec.Name() != CBOR {
		t.Fatalf("codec=%s", codec.Name())
	}
	if decoded != domain {
		t.Fatalf("Decode: %#v", decoded)
	}
}

func TestRemoteOptionsExclusive(t *testing.T) {
	if opts := RemoteOptions(ProtoCodec); len(opts) != 0 {
		t.Fatalf("proto RemoteOptions want none, got %d", len(opts))
	}
	if opts := RemoteOptions(CBORCodec); len(opts) == 0 {
		t.Fatal("cbor RemoteOptions want serializable registration")
	}
}

func TestAccountReplyRoundTrip(t *testing.T) {
	acc := &messages.Account{AccountID: "a1", AccountBalance: 99}
	for _, codec := range []Codec{CBORCodec, ProtoCodec} {
		wireMsg := codec.Encode(acc)
		decoded, got := Decode(wireMsg)
		if got.Name() != codec.Name() {
			t.Fatalf("%s: reply codec=%s", codec.Name(), got.Name())
		}
		out, ok := decoded.(*messages.Account)
		if !ok || out.AccountID != "a1" || out.AccountBalance != 99 {
			t.Fatalf("%s: %#v", codec.Name(), decoded)
		}
	}
}
