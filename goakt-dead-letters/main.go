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
	"sync"
	"time"

	"github.com/tochemey/goakt/v4/actor"
	"github.com/tochemey/goakt/v4/eventstream"
	"github.com/tochemey/goakt/v4/log"
)

type (
	Hello   struct{}
	Mystery struct{}
)

// Picky handles Hello and dead-letters anything else via Unhandled.
type Picky struct{}

var _ actor.Actor = (*Picky)(nil)

func (*Picky) PreStart(*actor.Context) error { return nil }
func (*Picky) PostStop(*actor.Context) error { return nil }

func (*Picky) Receive(ctx *actor.ReceiveContext) {
	switch ctx.Message().(type) {
	case *actor.PostStart:
	case *Hello:
		ctx.Logger().Infof("[%s] hello", ctx.Self().Name())
	default:
		ctx.Unhandled()
	}
}

func main() {
	ctx := context.Background()
	logger := log.DefaultLogger

	system, err := actor.NewActorSystem("DeadLetters", actor.WithLogger(logger))
	if err != nil {
		logger.Fatal(err)
	}
	if err := system.Start(ctx); err != nil {
		logger.Fatal(err)
	}
	defer func() { _ = system.Stop(ctx) }()

	// The actor system event stream multiplexes lifecycle events
	// (ActorStarted, ActorStopped, NodeJoined, ...). Dead letters
	// are published as *actor.Deadletter values on the same stream.
	subscriber, err := system.Subscribe()
	if err != nil {
		logger.Fatal(err)
	}
	defer func() { _ = system.Unsubscribe(subscriber) }()

	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go drainDeadLetters(subscriber, logger, stop, &wg)

	picky, err := system.Spawn(ctx, "picky", &Picky{})
	if err != nil {
		logger.Fatal(err)
	}

	// Handled normally.
	_ = actor.Tell(ctx, picky, &Hello{})

	// Unhandled message types → each is published to the event
	// stream as a *actor.Deadletter with reason "unhandled message".
	_ = actor.Tell(ctx, picky, &Mystery{})
	_ = actor.Tell(ctx, picky, &Mystery{})
	_ = actor.Tell(ctx, picky, &Mystery{})

	// Note: sending to a stopped/non-existent actor does *not*
	// produce an event-stream dead letter — the sender gets
	// ErrDead / ErrActorNotFound back synchronously instead.
	// The dead-letter stream is for messages the receiver did
	// accept but couldn't process.

	time.Sleep(500 * time.Millisecond)
	close(stop)
	wg.Wait()
}

// drainDeadLetters polls the subscriber and logs any Deadletter
// it sees. Iterator() returns a snapshot of currently buffered
// messages, so we re-poll on a small interval to catch new ones.
func drainDeadLetters(sub eventstream.Subscriber, logger log.Logger, stop <-chan struct{}, wg *sync.WaitGroup) {
	defer wg.Done()
	tick := time.NewTicker(50 * time.Millisecond)
	defer tick.Stop()

	for {
		for msg := range sub.Iterator() {
			dl, ok := msg.Payload().(*actor.Deadletter)
			if !ok {
				continue
			}
			logger.Warnf("dead letter: from=%s to=%s msg=%T reason=%s",
				dl.Sender(), dl.Receiver(), dl.Message(), dl.Reason())
		}
		select {
		case <-stop:
			return
		case <-tick.C:
		}
	}
}
