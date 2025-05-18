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

package persistence

import (
	"context"

	"github.com/tochemey/goakt/v3/extension"

	"github.com/tochemey/goakt-examples/v2/actor-cluster/dnssd/domain"
)

const MemoryStateStoreID = "MemoryStore"

type Store interface {
	extension.Extension
	Start()
	WriteState(ctx context.Context, actorID string, state *domain.Account)
	GetState(ctx context.Context, actorID string) *domain.Account
	Stop()
}

type MemoryStore struct {
	db *Map[string, *domain.Account]
}

var _ Store = (*MemoryStore)(nil)

func NewMemoryStore() Store {
	return &MemoryStore{
		db: New[string, *domain.Account](),
	}
}

func (x *MemoryStore) ID() string {
	return MemoryStateStoreID
}

func (x *MemoryStore) Start() {
}

func (x *MemoryStore) WriteState(_ context.Context, actorID string, state *domain.Account) {
	x.db.Set(actorID, state)
}

func (x *MemoryStore) GetState(_ context.Context, actorID string) *domain.Account {
	value, ok := x.db.Get(actorID)
	if !ok {
		return &domain.Account{}
	}
	return value
}

func (x *MemoryStore) Stop() {
	x.db.Reset()
}
