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

package persistence

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"

	"github.com/cockroachdb/pebble/v2"

	"github.com/tochemey/goakt-examples/v2/goakt-blockchain/chain"
)

// PebbleStoreID identifies the store in the actor system extensions.
const PebbleStoreID = "PebbleStore"

// PebbleStore keeps the blocks in a Pebble database, the LSM key-value store
// go-ethereum uses for its chain data. Keys are the big-endian block indexes,
// so iterating the keyspace returns the chain in order; values are the JSON
// representation of the blocks.
type PebbleStore struct {
	dir string
	db  *pebble.DB
}

var _ Store = (*PebbleStore)(nil)

func NewPebbleStore(dir string) *PebbleStore {
	return &PebbleStore{dir: dir}
}

func (x *PebbleStore) ID() string {
	return PebbleStoreID
}

func (x *PebbleStore) Start(context.Context) error {
	db, err := pebble.Open(x.dir, &pebble.Options{})
	if err != nil {
		return fmt.Errorf("failed to open the pebble database at %s: %w", x.dir, err)
	}
	x.db = db
	return nil
}

func (x *PebbleStore) Stop() error {
	if x.db == nil {
		return nil
	}
	return x.db.Close()
}

func (x *PebbleStore) Append(_ context.Context, block chain.Block) error {
	value, err := json.Marshal(block)
	if err != nil {
		return fmt.Errorf("failed to encode block %d: %w", block.Index, err)
	}
	if err := x.db.Set(blockKey(block.Index), value, pebble.Sync); err != nil {
		return fmt.Errorf("failed to persist block %d: %w", block.Index, err)
	}
	return nil
}

func (x *PebbleStore) Load(context.Context) ([]chain.Block, error) {
	iter, err := x.db.NewIter(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to iterate the pebble database: %w", err)
	}
	defer func() {
		_ = iter.Close()
	}()

	var blocks []chain.Block
	for iter.First(); iter.Valid(); iter.Next() {
		var block chain.Block
		if err := json.Unmarshal(iter.Value(), &block); err != nil {
			return nil, fmt.Errorf("failed to decode a persisted block: %w", err)
		}
		blocks = append(blocks, block)
	}
	return blocks, iter.Error()
}

func blockKey(index int64) []byte {
	key := make([]byte, 8)
	binary.BigEndian.PutUint64(key, uint64(index))
	return key
}
