/*
 * MIT License
 *
 * Copyright (c) 2022-2024 Arsene Tochemey Gandote
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

	"github.com/tochemey/goakt-examples/v2/internal/samplepb"
)

func main() {
	ctx := context.Background()

	// use the address default log. real-life implement the log interface`
	logger := log.DefaultLogger

	// create the actor system. kindly in real-life application handle the error
	actorSystem, _ := goakt.NewActorSystem("SampleActorSystem",
		goakt.WithPassivationDisabled(),
		goakt.WithLogger(logger))

	// start the actor system
	_ = actorSystem.Start(ctx)

	// wait for system to start properly
	time.Sleep(1 * time.Second)

	// create an actor
	actor := NewActor()
	actorRef, _ := actorSystem.Spawn(ctx, "Actor", actor)

	// wait for actor to start properly
	time.Sleep(1 * time.Second)

	// Start the timer
	duration := time.Minute
	done := make(chan struct{})
	go func() {
		for await := time.After(duration); ; {
			select {
			case <-await:
				done <- struct{}{}
				return
			default:
				_ = goakt.Tell(ctx, actorRef, new(samplepb.Ping))
			}
		}
	}()

	<-done

	count := actorRef.ProcessedCount()
	logger.Infof("%s has processed %d messages in %v", actorRef.ID(), count, duration)

	// capture ctrl+c
	interruptSignal := make(chan os.Signal, 1)
	signal.Notify(interruptSignal, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	<-interruptSignal

	// stop the actor system
	_ = actorSystem.Stop(ctx)
	os.Exit(0)
}

// Actor implements goakt.Actor
type Actor struct {
}

var _ goakt.Actor = (*Actor)(nil)

func NewActor() *Actor {
	return &Actor{}
}

func (p *Actor) PreStart(context.Context) error {
	return nil
}

func (p *Actor) Receive(ctx *goakt.ReceiveContext) {
	switch ctx.Message().(type) {
	case *goaktpb.PostStart:
	case *samplepb.Ping:
	default:
		ctx.Unhandled()
	}
}

func (p *Actor) PostStop(context.Context) error {
	return nil
}
