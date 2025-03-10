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
		goakt.WithPassivationDisabled(),
		goakt.WithActorInitMaxRetries(3))

	// start the actor system
	_ = actorSystem.Start(ctx)

	// wait for system to start properly
	time.Sleep(1 * time.Second)

	// create the actors
	ping := NewPing()
	pong := NewPong()
	pingActor, _ := actorSystem.Spawn(ctx, "Ping", ping)
	pongActor, _ := actorSystem.Spawn(ctx, "Pong", pong)

	// wait for actors to start properly
	time.Sleep(1 * time.Second)

	duration := time.Minute
	if err := pingActor.Tell(ctx, pongActor, new(samplepb.Ping)); err != nil {
		panic(err)
	}

	// Start the timer
	done := make(chan struct{})
	go func() {
		for await := time.After(duration); ; {
			select {
			case <-await:
				done <- struct{}{}
				return
			}
		}
	}()

	<-done

	pingCount := pingActor.ProcessedCount()
	pongCount := pongActor.ProcessedCount()

	logger.Infof("Ping=%s has processed %d messages in %v", pingActor.Address().String(), pingCount, duration)
	logger.Infof("Pong=%s has processed %d messages in %v", pongActor.Address().String(), pongCount, duration)

	logger.Infof("Ping has processed per second: %d", int64(pingCount)/int64(duration.Seconds()))
	logger.Infof("Pong has processed per second: %d", int64(pongCount)/int64(duration.Seconds()))

	// capture ctrl+c
	interruptSignal := make(chan os.Signal, 1)
	signal.Notify(interruptSignal, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	<-interruptSignal

	// stop the actor system
	_ = actorSystem.Stop(ctx)
	os.Exit(0)
}

type Ping struct {
}

var _ goakt.Actor = (*Ping)(nil)

func NewPing() *Ping {
	return &Ping{}
}

func (p *Ping) PreStart(context.Context) error {
	return nil
}

func (p *Ping) Receive(ctx *goakt.ReceiveContext) {
	switch ctx.Message().(type) {
	case *goaktpb.PostStart:
	case *samplepb.Pong:
		ctx.Tell(ctx.Sender(), new(samplepb.Ping))
	default:
		ctx.Unhandled()
	}
}

func (p *Ping) PostStop(context.Context) error {
	return nil
}

type Pong struct {
}

var _ goakt.Actor = (*Pong)(nil)

func NewPong() *Pong {
	return &Pong{}
}

func (p *Pong) PreStart(context.Context) error {
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

func (p *Pong) PostStop(context.Context) error {
	return nil
}
