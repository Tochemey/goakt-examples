/*
 * MIT License
 *
 * Copyright (c) 2022-2025 Arsene Tochemey Gandote
 *
 * Permission is hereby granted, free of charge, to any person obtaining a copy
 * of this software and associated documentation files (the "Software"), to deal
 * in the Software without restriction, including without limitation the rights
 * to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
 * copies of the Software, and to permit persons to whom the Software is
 * furnished to do so, subject to the following conditions:
 *
 * The above copyright notice and this permission notice shall be included in all
 * copies or substantial portions of the Software.
 *
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
 * IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
 * FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
 * AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
 * LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
 * OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
 * SOFTWARE.
 */

package main

import (
	"context"
	"errors"
	"sync"

	"go.uber.org/atomic"

	"github.com/tochemey/goakt-examples/v2/internal/samplepb"
)

type MemoryStore struct {
	db        *sync.Map
	connected *atomic.Bool
}

var _ StateStore = (*MemoryStore)(nil)

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		db:        &sync.Map{},
		connected: atomic.NewBool(false),
	}
}

func (d *MemoryStore) ID() string {
	return "MemoryStore"
}

// Connect connects the durable store
// nolint
func (d *MemoryStore) Connect(ctx context.Context) error {
	if d.connected.Load() {
		return nil
	}
	d.connected.Store(true)
	return nil
}

func (d *MemoryStore) Ping(ctx context.Context) error {
	if !d.connected.Load() {
		return d.Connect(ctx)
	}
	return nil
}

// Disconnect disconnect the durable store
// nolint
func (d *MemoryStore) Disconnect(ctx context.Context) error {
	if !d.connected.Load() {
		return nil
	}
	d.db.Range(func(key interface{}, value interface{}) bool {
		d.db.Delete(key)
		return true
	})
	d.connected.Store(false)
	return nil
}

func (d *MemoryStore) WriteState(ctx context.Context, persistenceID string, state *samplepb.Account) error {
	if !d.connected.Load() {
		return errors.New("store is not connected")
	}
	d.db.Store(persistenceID, state)
	return nil
}

func (d *MemoryStore) GetLatestState(ctx context.Context, persistenceID string) (*samplepb.Account, error) {
	if !d.connected.Load() {
		return nil, errors.New("store is not connected")
	}
	value, ok := d.db.Load(persistenceID)
	if !ok {
		return new(samplepb.Account), nil
	}
	return value.(*samplepb.Account), nil
}
