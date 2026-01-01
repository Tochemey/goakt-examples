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

	goakt "github.com/tochemey/goakt/v3/actor"
	"github.com/tochemey/goakt/v3/goaktpb"
	"github.com/tochemey/goakt/v3/log"

	samplepb "github.com/tochemey/goakt-examples/v2/internal/samplepb"
)

func main() {
	ctx := context.Background()

	// use the address default log. real-life implement the log interface`
	logger := log.DefaultLogger

	// create the actor system. kindly in real-life application handle the error
	actorSystem, _ := goakt.NewActorSystem("SampleActorSystem",
		goakt.WithLogger(logger),
		goakt.WithActorInitMaxRetries(3))

	// start the actor system
	_ = actorSystem.Start(ctx)

	// wait for system to start properly
	time.Sleep(1 * time.Second)

	// create the actors
	pong, _ := actorSystem.Spawn(ctx, "Pong", NewPong(), goakt.WithLongLived())
	ping, _ := actorSystem.Spawn(ctx, "Ping", NewPing(pong), goakt.WithLongLived())

	// wait for actors to start properly
	time.Sleep(1 * time.Second)

	duration := time.Minute
	_ = goakt.Tell(ctx, ping, new(samplepb.Begin))

	// Wait for one minute to pass
	<-time.After(duration)

	// wait for the actors to process the messages
	time.Sleep(1 * time.Second)
	m1, m2 := ping.Metric(ctx), pong.Metric(ctx)

	pingCount := m1.ProcessedCount()
	pongCount := m2.ProcessedCount()

	logger.Infof("Ping has processed %d messages in %v. (per/sec: %d)", pingCount, duration, int64(pingCount)/int64(duration.Seconds()))
	logger.Infof("Pong has processed %d messages in %v. (per/sec: %d)", pongCount, duration, int64(pongCount)/int64(duration.Seconds()))

	// capture ctrl+c
	interruptSignal := make(chan os.Signal, 1)
	signal.Notify(interruptSignal, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	<-interruptSignal

	// stop the actor system
	_ = actorSystem.Stop(ctx)
	os.Exit(0)
}

type Ping struct {
	// pong actor reference
	pong *goakt.PID
}

var _ goakt.Actor = (*Ping)(nil)

func NewPing(pong *goakt.PID) *Ping {
	return &Ping{
		pong: pong,
	}
}

func (p *Ping) PreStart(*goakt.Context) error {
	return nil
}

func (p *Ping) Receive(ctx *goakt.ReceiveContext) {
	switch ctx.Message().(type) {
	case *goaktpb.PostStart:
	case *samplepb.Begin:
		ctx.Tell(p.pong, new(samplepb.Ping))
	case *samplepb.Pong:
		ctx.Tell(ctx.Sender(), new(samplepb.Ping))
	default:
		ctx.Unhandled()
	}
}

func (p *Ping) PostStop(*goakt.Context) error {
	return nil
}

type Pong struct {
}

var _ goakt.Actor = (*Pong)(nil)

func NewPong() *Pong {
	return &Pong{}
}

func (p *Pong) PreStart(*goakt.Context) error {
	return nil
}

func (p *Pong) Receive(ctx *goakt.ReceiveContext) {
	switch ctx.Message().(type) {
	case *goaktpb.PostStart:
	case *samplepb.Ping:
		ctx.Tell(ctx.Sender(), new(samplepb.Pong))
	default:
		ctx.Unhandled()
	}
}

func (p *Pong) PostStop(*goakt.Context) error {
	return nil
}
