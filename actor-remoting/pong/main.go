/*
 * MIT License
 *
 * Copyright (c) 2022-2024 Tochemey
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

	goakt "github.com/tochemey/goakt/v2/actors"
	"github.com/tochemey/goakt/v2/goaktpb"
	"github.com/tochemey/goakt/v2/log"

	samplepb "github.com/tochemey/goakt-examples/v2/samplepb"
)

const (
	port = 50052
	host = "127.0.0.1"
)

func main() {
	ctx := context.Background()

	// use the address default log. real-life implement the log interface`
	logger := log.New(log.DebugLevel, os.Stdout)

	// create the actor system. kindly in real-life application handle the error
	actorSystem, _ := goakt.NewActorSystem("SampleActorSystem",
		goakt.WithPassivationDisabled(), // set big passivation time
		goakt.WithLogger(logger),
		goakt.WithActorInitMaxRetries(3),
		goakt.WithRemoting(host, port))

	// start the actor system
	_ = actorSystem.Start(ctx)

	// create an actor
	_, _ = actorSystem.Spawn(ctx, "Pong", NewPong())

	// capture ctrl+c
	interruptSignal := make(chan os.Signal, 1)
	signal.Notify(interruptSignal, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	<-interruptSignal

	// stop the actor system
	_ = actorSystem.Stop(ctx)
	os.Exit(0)
}

type Pong struct {
	count     int
	startTime time.Time
	logger    log.Logger
}

var _ goakt.Actor = (*Pong)(nil)

func NewPong() *Pong {
	return &Pong{}
}

func (p *Pong) PreStart(ctx context.Context) error {
	p.count = 0
	return nil
}

func (p *Pong) Receive(ctx *goakt.ReceiveContext) {
	switch ctx.Message().(type) {
	case *goaktpb.PostStart:
		p.logger = ctx.Self().Logger()
		p.startTime = time.Now()
	case *samplepb.Ping:
		p.count++
		// reply the sender in case there is a sender
		if ctx.RemoteSender() != nil {
			ctx.RemoteTell(ctx.RemoteSender(), new(samplepb.Pong))
			return
		}

		if !ctx.Sender().Equals(goakt.NoSender) {
			ctx.Tell(ctx.Sender(), new(samplepb.Pong))
		}
	default:
		ctx.Unhandled()
	}
}

func (p *Pong) PostStop(context.Context) error {
	duration := time.Since(p.startTime)
	p.logger.Infof("Ping has processed %d messages per second", int64(p.count)/int64(duration.Seconds()))
	return nil
}
