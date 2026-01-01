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

package main

import (
	"context"
	"fmt"

	"github.com/tochemey/goakt/v3/actor"
	"go.uber.org/atomic"

	"github.com/tochemey/goakt-examples/v2/internal/samplepb"
)

type Grain struct {
	id         string
	created    bool
	stateStore StateStore
	state      *atomic.Pointer[samplepb.Account]
}

var _ actor.Grain = (*Grain)(nil)

func (g *Grain) OnActivate(ctx context.Context, props *actor.GrainProps) error {
	g.state = atomic.NewPointer(new(samplepb.Account))
	g.stateStore = props.ActorSystem().Extension("MemoryStore").(StateStore)
	g.id = props.Identity().Name()
	return g.recoverFromStore(ctx)
}

func (g *Grain) OnDeactivate(ctx context.Context, props *actor.GrainProps) error {
	return g.stateStore.WriteState(ctx, g.id, g.state.Load())
}

func (g *Grain) OnReceive(ctx *actor.GrainContext) {
	switch m := ctx.Message().(type) {
	case *samplepb.CreateAccount:
		if g.created {
			ctx.Err(fmt.Errorf("account %s already created", g.id))
			return
		}

		balance := m.GetAccountBalance()
		g.state.Load().AccountBalance = g.state.Load().GetAccountBalance() + balance
		if err := g.stateStore.WriteState(ctx.Context(), g.id, g.state.Load()); err != nil {
			ctx.Err(fmt.Errorf("failed to write state: %w", err))
			return
		}

		ctx.Response(g.state.Load())
	case *samplepb.CreditAccount:
		balance := m.GetBalance()
		g.state.Load().AccountBalance = g.state.Load().GetAccountBalance() + balance
		if err := g.stateStore.WriteState(ctx.Context(), g.id, g.state.Load()); err != nil {
			ctx.Err(fmt.Errorf("failed to write state: %w", err))
			return
		}
		ctx.Response(g.state.Load())
	case *samplepb.GetAccount:
		ctx.Response(g.state.Load())
	default:
		ctx.Unhandled()
	}
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
