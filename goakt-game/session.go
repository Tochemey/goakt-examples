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
	"encoding/json"
	"time"

	"github.com/coder/websocket"
	"github.com/tochemey/goakt/v4/actor"
)

// writeBudget caps how long a single snapshot write may take. A slow client
// trips this, the actor errors, and supervision tears the session down —
// that's our backpressure. Per-tick budget is 33ms, so 50ms gives one tick
// of head-room before we declare the client unhealthy.
const writeBudget = 50 * time.Millisecond

// PlayerSessionActor bridges one WebSocket connection to the MatchActor. It
// owns the *websocket.Conn directly: Receive serializes all I/O, so writes
// happen inline — no side channel, no separate goroutine. The reader side
// (which has to block in conn.Read) lives in the gateway and Tells this
// actor a *closed{} when the socket ends.
type PlayerSessionActor struct {
	match *actor.PID
	conn  *websocket.Conn
}

var _ actor.Actor = (*PlayerSessionActor)(nil)

func (*PlayerSessionActor) PreStart(*actor.Context) error { return nil }
func (*PlayerSessionActor) PostStop(*actor.Context) error { return nil }

func (p *PlayerSessionActor) Receive(ctx *actor.ReceiveContext) {
	switch ctx.Message().(type) {
	case *actor.PostStart:
		ctx.Tell(p.match, &Subscribe{})

	case *PlayerInput:
		ctx.Tell(p.match, ctx.Message())

	case *Snapshot:
		data, err := json.Marshal(ctx.Message())
		if err != nil {
			ctx.Err(err)
			return
		}

		wctx, cancel := context.WithTimeout(ctx.Context(), writeBudget)
		err = p.conn.Write(wctx, websocket.MessageText, data)
		cancel()
		if err != nil {
			ctx.Logger().Infof("ws write failed for %s: %v", ctx.Self().Name(), err)
			// Self-shutdown only. The gateway emits the Unsubscribe to
			// the match before sending us *closed — see gateway.go.
			ctx.Shutdown()
		}

	case *closed:
		ctx.Shutdown()

	default:
		ctx.Unhandled()
	}
}

// closed is sent by the gateway reader goroutine when the WS connection ends.
type closed struct{}
