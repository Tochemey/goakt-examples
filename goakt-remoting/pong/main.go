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
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/tochemey/goakt/v4/actor"
	"github.com/tochemey/goakt/v4/log"
	"github.com/tochemey/goakt/v4/remote"
	"github.com/tochemey/goakt/v4/supervisor"

	"github.com/tochemey/goakt-examples/v2/internal/samplepb"
)

func main() {
	ctx := context.Background()

	// use the address default log. real-life implement the log interface`
	logger := log.New(log.DebugLevel, os.Stdout)

	// create the actor system. kindly in real-life application handle the error
	actorSystem, err := actor.NewActorSystem("Remoting",
		actor.WithLogger(logger),
		actor.WithActorInitMaxRetries(3),
		actor.WithRemote(remote.NewConfig("127.0.0.1", 9010)))

	if err != nil {
		logger.Fatalf("failed to create actor system: %v", err)
	}

	// start the actor system
	if err := actorSystem.Start(ctx); err != nil {
		logger.Fatalf("failed to start actor system: %v", err)
	}

	// create an actor
	_, err = actorSystem.Spawn(ctx, "Pong",
		NewPong(),
		actor.WithLongLived(),
		actor.WithSupervisor(
			supervisor.NewSupervisor(
				supervisor.WithAnyErrorDirective(supervisor.ResumeDirective),
			),
		),
	)

	if err != nil {
		logger.Fatalf("failed to spawn actor: %v", err)
	}

	// capture ctrl+c
	interruptSignal := make(chan os.Signal, 1)
	signal.Notify(interruptSignal, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	<-interruptSignal

	// stop the actor system
	_ = actorSystem.Stop(ctx)
	os.Exit(0)
}

type Pong struct {
	count int
	start time.Time
}

var _ actor.Actor = (*Pong)(nil)

func NewPong() *Pong {
	return &Pong{}
}

func (act *Pong) PreStart(*actor.Context) error {
	return nil
}

func (act *Pong) Receive(ctx *actor.ReceiveContext) {
	switch ctx.Message().(type) {
	case *actor.PostStart:
		act.start = time.Now()
	case *samplepb.End:
		ctx.Logger().Infof("completed processing message: %d", act.count)
	case *samplepb.Ping:
		act.count++
		ctx.Logger().Infof("Received pong count: %d", act.count)
		ctx.Tell(ctx.Sender(), new(samplepb.Pong))
	default:
		ctx.Unhandled()
	}
}

func (act *Pong) PostStop(*actor.Context) error {
	return nil
}
