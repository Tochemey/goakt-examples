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

// Package remoting provides production-oriented helpers for configuring GoAkt's
// multiplexed duplex remoting transport in these examples.
//
// Homogeneous clusters should pin [remote.ProtocolPinDuplex] rather than leaving
// the default [remote.ProtocolPinAuto] (which exists for mixed-version rollouts).
// See https://docs.goakt.dev/advanced/remoting.
package remoting

import (
	"time"

	"github.com/tochemey/goakt/v4/remote"
)

// lanOptions are the baseline remoting settings for homogeneous intra-cluster /
// LAN deployments on the duplex transport.
func lanOptions() []remote.Option {
	return []remote.Option{
		remote.WithProtocolPin(remote.ProtocolPinDuplex),
		remote.WithCompression(remote.NoCompression),
		remote.WithOrdinaryLanes(remote.DefaultOrdinaryLanes),
		remote.WithCreditWindow(remote.DefaultCreditWindow),
		remote.WithChunkSize(remote.DefaultChunkSize),
		remote.WithMaxMessageSize(remote.DefaultMaxMessageSize),
		remote.WithMaxConcurrentLargeTransfers(remote.DefaultMaxConcurrentLargeTransfers),
		remote.WithWriteTimeout(10 * time.Second),
		remote.WithReadIdleTimeout(10 * time.Second),
	}
}

// wanOptions are the baseline remoting settings for bandwidth-bound links
// (cross-region / multi-datacenter). Compression must match on every peer.
func wanOptions() []remote.Option {
	return []remote.Option{
		remote.WithProtocolPin(remote.ProtocolPinDuplex),
		remote.WithCompression(remote.ZstdCompression),
		remote.WithOrdinaryLanes(remote.DefaultOrdinaryLanes),
		remote.WithCreditWindow(remote.DefaultCreditWindow),
		remote.WithChunkSize(remote.DefaultChunkSize),
		remote.WithMaxMessageSize(remote.DefaultMaxMessageSize),
		remote.WithMaxConcurrentLargeTransfers(remote.DefaultMaxConcurrentLargeTransfers),
		remote.WithWriteTimeout(10 * time.Second),
		remote.WithReadIdleTimeout(10 * time.Second),
	}
}

// NewConfig returns a [remote.Config] tuned for homogeneous production
// deployments on the duplex remoting transport (intra-cluster / LAN).
//
// Extra options (serializers, TLS, context propagators, lane overrides) are
// applied after the baseline; when the same field is set twice, the later
// option wins.
func NewConfig(bindAddr string, bindPort int, opts ...remote.Option) *remote.Config {
	return remote.NewConfig(bindAddr, bindPort, append(lanOptions(), opts...)...)
}

// NewWANConfig is like [NewConfig] but enables Zstandard compression for
// bandwidth-bound links such as cross-region / multi-datacenter remoting.
// Both ends of every remote connection must use the same compression codec.
func NewWANConfig(bindAddr string, bindPort int, opts ...remote.Option) *remote.Config {
	return remote.NewConfig(bindAddr, bindPort, append(wanOptions(), opts...)...)
}
