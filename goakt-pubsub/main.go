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
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/tochemey/goakt/v4/actor"
	"github.com/tochemey/goakt/v4/log"
)

const newsTopic = "news"

// News is the user-defined payload published on the topic.
type News struct {
	Headline string
}

// Subscriber subscribes to newsTopic on start and prints whatever
// the topic actor delivers to it.
type Subscriber struct{}

var _ actor.Actor = (*Subscriber)(nil)

func (*Subscriber) PreStart(*actor.Context) error { return nil }
func (*Subscriber) PostStop(*actor.Context) error { return nil }

func (*Subscriber) Receive(ctx *actor.ReceiveContext) {
	switch msg := ctx.Message().(type) {
	case *actor.PostStart:
		// Subscribe must be sent from inside an actor so the topic
		// actor can identify the subscriber via ctx.Sender().
		ctx.Tell(ctx.ActorSystem().TopicActor(), actor.NewSubscribe(newsTopic))
	case *actor.SubscribeAck:
		ctx.Logger().Infof("[%s] subscribed to %q", ctx.Self().Name(), msg.Topic())
	case *actor.UnsubscribeAck:
		ctx.Logger().Infof("[%s] unsubscribed from %q", ctx.Self().Name(), msg.Topic())
	case *News:
		ctx.Logger().Infof("[%s] got news: %s", ctx.Self().Name(), msg.Headline)
	default:
		ctx.Unhandled()
	}
}

func main() {
	ctx := context.Background()
	logger := log.DefaultLogger

	// WithPubSub() enables the system's TopicActor.
	system, err := actor.NewActorSystem("PubSub",
		actor.WithLogger(logger),
		actor.WithPubSub())
	if err != nil {
		logger.Fatal(err)
	}

	if err := system.Start(ctx); err != nil {
		logger.Fatal(err)
	}

	defer func() { _ = system.Stop(ctx) }()

	alice, _ := system.Spawn(ctx, "alice", &Subscriber{})
	bob, _ := system.Spawn(ctx, "bob", &Subscriber{})

	// Wait for both subscriptions to register before publishing.
	time.Sleep(200 * time.Millisecond)

	publish := func(headline string) {
		// Publish is itself a message addressed to the topic actor.
		// The id field is used by the topic actor to deduplicate
		// messages that arrive over both local and cluster paths.
		_ = actor.Tell(ctx, system.TopicActor(),
			actor.NewPublish(uuid.NewString(), newsTopic, &News{Headline: headline}))
	}

	publish("goakt v4 released")
	publish("actors are still cool")
	time.Sleep(300 * time.Millisecond)

	// alice unsubscribes; bob remains.
	logger.Info("--- alice unsubscribes ---")
	_ = alice.Tell(ctx, system.TopicActor(), actor.NewUnsubscribe(newsTopic))
	time.Sleep(150 * time.Millisecond)

	publish("bob is the only listener now")
	time.Sleep(300 * time.Millisecond)

	_ = bob // keep bob alive until shutdown
	fmt.Println()
}
