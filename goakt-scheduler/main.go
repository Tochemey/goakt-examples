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
	"time"

	"github.com/tochemey/goakt/v4/actor"
	"github.com/tochemey/goakt/v4/log"
)

type (
	OneShot struct{}
	Tick    struct{}
)

type Worker struct{}

var _ actor.Actor = (*Worker)(nil)

func (*Worker) PreStart(*actor.Context) error { return nil }
func (*Worker) PostStop(*actor.Context) error { return nil }

func (*Worker) Receive(ctx *actor.ReceiveContext) {
	switch ctx.Message().(type) {
	case *actor.PostStart:
	case *OneShot:
		ctx.Logger().Infof("one-shot fired at %s", time.Now().Format("15:04:05.000"))
	case *Tick:
		ctx.Logger().Infof("tick at %s", time.Now().Format("15:04:05.000"))
	default:
		ctx.Unhandled()
	}
}

func main() {
	ctx := context.Background()
	logger := log.DefaultLogger

	system, err := actor.NewActorSystem("Scheduler", actor.WithLogger(logger))
	if err != nil {
		logger.Fatal(err)
	}
	if err := system.Start(ctx); err != nil {
		logger.Fatal(err)
	}
	defer func() { _ = system.Stop(ctx) }()

	worker, err := system.Spawn(ctx, "worker", &Worker{})
	if err != nil {
		logger.Fatal(err)
	}

	// One-shot: fired exactly once after 500ms.
	if err := system.ScheduleOnce(ctx, &OneShot{}, worker, 500*time.Millisecond); err != nil {
		logger.Fatal(err)
	}

	// Recurring: every 300ms. Tag it with a reference so we can cancel it later.
	const tickRef = "ticker"
	if err := system.Schedule(ctx, &Tick{}, worker, 300*time.Millisecond,
		actor.WithReference(tickRef)); err != nil {
		logger.Fatal(err)
	}

	// Let a few ticks fire, then cancel the recurring schedule.
	time.Sleep(time.Second)
	logger.Info("--- cancelling recurring tick ---")
	if err := system.CancelSchedule(tickRef); err != nil {
		logger.Errorf("cancel failed: %v", err)
	}

	// Confirm no further ticks arrive.
	time.Sleep(700 * time.Millisecond)
	logger.Info("done")
}
