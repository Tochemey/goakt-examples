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
	"fmt"

	"github.com/tochemey/goakt/v3/actor"
	"github.com/tochemey/goakt/v3/extension"
	"go.uber.org/atomic"
	"google.golang.org/protobuf/proto"

	"github.com/tochemey/goakt-examples/v2/internal/samplepb"
)

type Grain struct {
	id         string
	created    bool
	stateStore StateStore
	state      *atomic.Pointer[samplepb.Account]
}

var _ actor.Grain = (*Grain)(nil)

func (g *Grain) OnActivate(ctx *actor.GrainContext) error {
	g.state = atomic.NewPointer(new(samplepb.Account))
	g.stateStore = ctx.Extension("MemoryStore").(StateStore)
	g.id = ctx.Self().Name()
	return g.recoverFromStore(ctx.Context())
}

func (g *Grain) OnDeactivate(ctx *actor.GrainContext) error {
	return g.stateStore.WriteState(ctx.Context(), g.id, g.state.Load())
}

func (g *Grain) Dependencies() []extension.Dependency {
	return nil
}

func (g *Grain) ReceiveSync(ctx context.Context, message proto.Message) (proto.Message, error) {
	switch m := message.(type) {
	case *samplepb.CreateAccount:
		if g.created {
			return nil, fmt.Errorf("account %s already created", g.id)
		}

		balance := m.GetAccountBalance()
		g.state.Load().AccountBalance = g.state.Load().GetAccountBalance() + balance
		if err := g.stateStore.WriteState(ctx, g.id, g.state.Load()); err != nil {
			return nil, fmt.Errorf("failed to write state: %w", err)
		}

		return g.state.Load(), nil
	case *samplepb.CreditAccount:
		balance := m.GetBalance()
		g.state.Load().AccountBalance = g.state.Load().GetAccountBalance() + balance
		if err := g.stateStore.WriteState(ctx, g.id, g.state.Load()); err != nil {
			return nil, fmt.Errorf("failed to write state: %w", err)
		}
		return g.state.Load(), nil
	case *samplepb.GetAccount:
		return g.state.Load(), nil
	default:
		return nil, fmt.Errorf("unhandled message type %T", message)
	}
}

func (g *Grain) ReceiveAsync(ctx context.Context, message proto.Message) error {
	return nil
}

func (g *Grain) recoverFromStore(ctx context.Context) error {
	latestState, err := g.stateStore.GetLatestState(ctx, g.id)
	if err != nil {
		return fmt.Errorf("failed to get the latest state: %w", err)
	}

	if latestState != nil {
		g.state.Store(latestState)
	}

	return nil
}
