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
	"github.com/google/uuid"
	"github.com/tochemey/goakt/v4/actor"
)

// MatchFactory is a cluster-singleton actor that mints a fresh MatchActor
// per request and places it on whichever cluster node currently has the
// lightest load. Single-node setups still go through this path so the
// 2.1 → 2.2 transition (one node → many) requires no code change here.
//
// The factory holds *no* per-match state — it's a stateless dispatcher.
// One singleton per cluster is enough because spawning a new match is a
// cheap, infrequent operation (once per WS connection).
type MatchFactory struct{}

var _ actor.Actor = (*MatchFactory)(nil)

func (*MatchFactory) PreStart(*actor.Context) error { return nil }
func (*MatchFactory) PostStop(*actor.Context) error { return nil }

func (f *MatchFactory) Receive(ctx *actor.ReceiveContext) {
	switch ctx.Message().(type) {
	case *actor.PostStart:
		ctx.Logger().Infof("matchmaker ready on %s", ctx.Self().Path().HostPort())

	case *CreateMatch:
		name := matchActorPrefix + uuid.NewString()
		// LeastLoad places new matches on the least-loaded peer; in single-
		// node mode this trivially picks the local node. WithRelocationDisabled
		// keeps a match pinned to its origin node — relocating an in-flight
		// game's state to another node would just drop it on the floor, so
		// we prefer to let the match die with its host.
		pid, err := ctx.ActorSystem().SpawnOn(ctx.Context(), name, &MatchActor{},
			actor.WithLongLived(),
			actor.WithPlacement(actor.LeastLoad),
			actor.WithRelocationDisabled())
		if err != nil {
			ctx.Logger().Errorf("matchmaker: SpawnOn(%s) failed: %v", name, err)
			ctx.Err(err)
			return
		}
		placement := "local"
		if pid != nil && pid.IsRemote() {
			placement = "remote@" + pid.Path().HostPort()
		}
		ctx.Logger().Infof("matchmaker: spawned %s (%s)", name, placement)
		ctx.Response(&MatchCreated{MatchName: name})

	default:
		ctx.Unhandled()
	}
}
