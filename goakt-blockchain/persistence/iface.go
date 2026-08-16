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

// Package persistence stores the mined blocks of the local chain replica in
// an embedded key-value store, so a pod restores its copy of the chain from
// its own disk when it restarts.
package persistence

import (
	"context"

	"github.com/tochemey/goakt/v4/extension"

	"github.com/tochemey/goakt-examples/v2/goakt-blockchain/chain"
)

// Store persists the mined blocks of the chain. The genesis block is
// deterministic and is not stored.
type Store interface {
	extension.Extension
	Start(ctx context.Context) error
	// Append persists a mined block under its index.
	Append(ctx context.Context, block chain.Block) error
	// Load returns every persisted block in ascending index order.
	Load(ctx context.Context) ([]chain.Block, error)
	Stop() error
}
