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

	"github.com/tochemey/goakt/v4/actor"
	"github.com/tochemey/goakt/v4/log"
	"github.com/tochemey/goakt/v4/remote"
	"github.com/tochemey/goakt/v4/supervisor"

	"github.com/tochemey/goakt-examples/v2/internal/samplepb"
)

func main() {
	ctx := context.Background()

	// use the address default log. real-life implement the log interface`
	logger := log.NewSlog(log.DebugLevel, os.Stdout)

	// create the actor system. kindly in real-life application handle the error
	actorSystem, err := actor.NewActorSystem(
		"Remoting",
		actor.WithLogger(logger),
		actor.WithRemote(remote.NewConfig("127.0.0.1", 9000)),
	)

	if err != nil {
		logger.Fatalf("failed to create actor system: %v", err)
	}

	// start the actor system
	if err := actorSystem.Start(ctx); err != nil {
		logger.Fatalf("failed to start actor system: %v", err)
	}

	// create an actor
	totalScore := 1_000

	ping, err := actorSystem.Spawn(ctx, "Ping", NewPing(totalScore),
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

	// locate the pong actor
	pong, err := ping.RemoteLookup(ctx, "127.0.0.1", 9010, "Pong")
	if err != nil {
		logger.Fatalf("failed to lookup remote actor: %v", err)
	}

	// send a message to the pong actor
	if err := ping.Tell(ctx, pong, new(samplepb.Ping)); err != nil {
		logger.Fatalf("failed to send message to remote actor: %v", err)
	}

	// capture ctrl+c
	interruptSignal := make(chan os.Signal, 1)
	signal.Notify(interruptSignal, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	<-interruptSignal

	// stop the actor system
	_ = actorSystem.Stop(ctx)
	os.Exit(0)
}

type Ping struct {
	scores int
	count  int
}

var _ actor.Actor = (*Ping)(nil)

func NewPing(totalScore int) *Ping {
	return &Ping{
		scores: totalScore,
	}
}

func (act *Ping) PreStart(*actor.Context) error {
	return nil
}

func (act *Ping) Receive(ctx *actor.ReceiveContext) {
	switch ctx.Message().(type) {
	case *actor.PostStart:
	case *samplepb.Pong:
		act.count++
		ctx.Logger().Infof("Received pong count: %d", act.count)
		if act.count >= act.scores {
			ctx.Tell(ctx.Sender(), new(samplepb.End))
			return
		}
		ctx.Tell(ctx.Sender(), new(samplepb.Ping))
	default:
		ctx.Unhandled()
	}
}

func (act *Ping) PostStop(*actor.Context) error {
	return nil
}
