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
	"os"
	"os/signal"
	"syscall"
	"time"

	goakt "github.com/tochemey/goakt/v3/actor"
	"github.com/tochemey/goakt/v3/address"
	"github.com/tochemey/goakt/v3/goaktpb"
	"github.com/tochemey/goakt/v3/log"
	"github.com/tochemey/goakt/v3/remote"

	samplepb "github.com/tochemey/goakt-examples/v2/internal/samplepb"
)

const (
	port = 50051
	host = "127.0.0.1"
)

func main() {
	ctx := context.Background()

	// use the address default log. real-life implement the log interface`
	logger := log.New(log.DebugLevel, os.Stdout)

	// create the actor system. kindly in real-life application handle the error
	actorSystem, _ := goakt.NewActorSystem(
		"Remoting",
		goakt.WithLogger(logger),
		goakt.WithRemote(remote.NewConfig(host, port)),
	)

	// start the actor system
	_ = actorSystem.Start(ctx)

	// wait for the actor system to be ready
	time.Sleep(time.Second)

	// create an actor
	totalScore := 10_000_000

	pid, _ := actorSystem.Spawn(ctx, "Ping", NewPing(totalScore),
		goakt.WithLongLived(),
		goakt.WithSupervisor(
			goakt.NewSupervisor(
				goakt.WithAnyErrorDirective(goakt.ResumeDirective),
			),
		),
	)

	// wait for the actor to be ready
	time.Sleep(time.Second)

	// locate the pong actor
	remoteAddress := address.New("Pong", actorSystem.Name(), host, 50052)

	// send a message to the pong actor
	_ = pid.RemoteTell(ctx, remoteAddress, new(samplepb.Ping))

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

var _ goakt.Actor = (*Ping)(nil)

func NewPing(totalScore int) *Ping {
	return &Ping{
		scores: totalScore,
	}
}

func (act *Ping) PreStart(*goakt.Context) error {
	return nil
}

func (act *Ping) Receive(ctx *goakt.ReceiveContext) {
	switch ctx.Message().(type) {
	case *goaktpb.PostStart:
	case *samplepb.Pong:
		act.count++
		if act.count >= act.scores {
			ctx.RemoteTell(ctx.RemoteSender(), new(samplepb.End))
			return
		}
		ctx.RemoteTell(ctx.RemoteSender(), new(samplepb.Ping))
	default:
		ctx.Unhandled()
	}
}

func (act *Ping) PostStop(*goakt.Context) error {
	return nil
}
