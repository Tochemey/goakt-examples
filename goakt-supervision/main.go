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
	"github.com/tochemey/goakt/v4/supervisor"
)

type (
	Hello struct{}
	Boom  struct{}
)

// Worker increments a counter on every Hello and panics on Boom.
// The counter lets us see whether internal state survived after a directive.
type Worker struct {
	counter int
}

var _ actor.Actor = (*Worker)(nil)

func (*Worker) PreStart(ctx *actor.Context) error {
	ctx.ActorSystem().Logger().Infof("  PreStart  %s", ctx.ActorName())
	return nil
}

func (*Worker) PostStop(ctx *actor.Context) error {
	ctx.ActorSystem().Logger().Infof("  PostStop  %s", ctx.ActorName())
	return nil
}

func (w *Worker) Receive(ctx *actor.ReceiveContext) {
	switch ctx.Message().(type) {
	case *actor.PostStart:
	case *Hello:
		w.counter++
		ctx.Logger().Infof("  Hello -> %s counter=%d", ctx.Self().Name(), w.counter)
	case *Boom:
		ctx.Logger().Warnf("  Boom! %s is about to panic", ctx.Self().Name())
		panic("worker exploded")
	default:
		ctx.Unhandled()
	}
}

func main() {
	ctx := context.Background()
	logger := log.DefaultLogger

	system, err := actor.NewActorSystem("Supervision", actor.WithLogger(logger))
	if err != nil {
		logger.Fatal(err)
	}
	if err := system.Start(ctx); err != nil {
		logger.Fatal(err)
	}
	defer func() { _ = system.Stop(ctx) }()

	// Three workers, three directives. After each Boom, send a Hello
	// and observe whether the counter survived (Resume), reset to 0 (Restart),
	// or whether the actor is gone (Stop).
	stopSup := supervisor.NewSupervisor(supervisor.WithAnyErrorDirective(supervisor.StopDirective))
	resumeSup := supervisor.NewSupervisor(supervisor.WithAnyErrorDirective(supervisor.ResumeDirective))
	restartSup := supervisor.NewSupervisor(
		supervisor.WithAnyErrorDirective(supervisor.RestartDirective),
		supervisor.WithRetry(3, 200*time.Millisecond),
	)

	stopper, _ := system.Spawn(ctx, "stopper", &Worker{}, actor.WithSupervisor(stopSup))
	resumer, _ := system.Spawn(ctx, "resumer", &Worker{}, actor.WithSupervisor(resumeSup))
	restarter, _ := system.Spawn(ctx, "restarter", &Worker{}, actor.WithSupervisor(restartSup))

	// Build up some state, then crash, then probe.
	for _, pid := range []*actor.PID{stopper, resumer, restarter} {
		_ = actor.Tell(ctx, pid, &Hello{})
		_ = actor.Tell(ctx, pid, &Hello{})
	}
	time.Sleep(200 * time.Millisecond)

	logger.Info("--- triggering Boom on all three ---")
	for _, pid := range []*actor.PID{stopper, resumer, restarter} {
		_ = actor.Tell(ctx, pid, &Boom{})
	}
	time.Sleep(500 * time.Millisecond)

	logger.Info("--- post-failure probe ---")
	for _, pid := range []*actor.PID{stopper, resumer, restarter} {
		_ = actor.Tell(ctx, pid, &Hello{})
	}
	time.Sleep(300 * time.Millisecond)

	// stopper is gone; resumer kept its counter; restarter reset to 1.
	logger.Infof("stopper running?   %v", stopper.IsRunning())
	logger.Infof("resumer running?   %v", resumer.IsRunning())
	logger.Infof("restarter running? %v", restarter.IsRunning())
}
