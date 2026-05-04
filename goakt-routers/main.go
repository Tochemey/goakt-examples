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

	"github.com/tochemey/goakt/v4/actor"
	"github.com/tochemey/goakt/v4/log"
)

// Job is the unit of work distributed by the routers.
type Job struct {
	ID  int
	Key string
}

// Worker is a routee. A router spawns N copies via reflection,
// so the type must be usable without constructor-injected state.
type Worker struct{}

var _ actor.Actor = (*Worker)(nil)

func (*Worker) PreStart(*actor.Context) error { return nil }
func (*Worker) PostStop(*actor.Context) error { return nil }

func (*Worker) Receive(ctx *actor.ReceiveContext) {
	switch msg := ctx.Message().(type) {
	case *actor.PostStart:
	case *Job:
		ctx.Logger().Infof("[%s] processed job=%d key=%q", ctx.Self().Name(), msg.ID, msg.Key)
	default:
		ctx.Unhandled()
	}
}

func main() {
	ctx := context.Background()
	logger := log.DefaultLogger

	system, err := actor.NewActorSystem("Routers", actor.WithLogger(logger))
	if err != nil {
		logger.Fatal(err)
	}

	if err := system.Start(ctx); err != nil {
		logger.Fatal(err)
	}
	defer func() { _ = system.Stop(ctx) }()

	roundRobinDemo(ctx, system)
	fanOutDemo(ctx, system)
	consistentHashDemo(ctx, system)
}

// roundRobinDemo: every job goes to the next routee in order.
// Useful for evenly distributing uniform, stateless work.
func roundRobinDemo(ctx context.Context, system actor.ActorSystem) {
	logger := system.Logger()
	logger.Info("=== round-robin: 3 workers, 6 jobs ===")

	router, err := system.SpawnRouter(ctx, "rr", 3, &Worker{},
		actor.WithRoutingStrategy(actor.RoundRobinRouting))
	if err != nil {
		logger.Fatal(err)
	}

	sender := system.NoSender()
	for i := 1; i <= 6; i++ {
		_ = sender.Tell(ctx, router, actor.NewBroadcast(&Job{ID: i}))
	}

	time.Sleep(300 * time.Millisecond)

	// Inspect the live pool by Asking the router for its routees.
	resp, err := sender.Ask(ctx, router, &actor.GetRoutees{}, time.Second)
	if err == nil {
		logger.Infof("active routees: %v", resp.(*actor.Routees).Names())
	}
	_ = system.Kill(ctx, "rr")
}

// fanOutDemo: every routee receives every message. Useful for
// cache invalidation, pub-sub-style delivery, or multi-sink processing.
func fanOutDemo(ctx context.Context, system actor.ActorSystem) {
	logger := system.Logger()
	logger.Info("=== fan-out: 3 workers, 2 broadcasts (each worker sees both) ===")

	router, err := system.SpawnRouter(ctx, "fan", 3, &Worker{},
		actor.WithRoutingStrategy(actor.FanOutRouting))
	if err != nil {
		logger.Fatal(err)
	}

	sender := system.NoSender()
	_ = sender.Tell(ctx, router, actor.NewBroadcast(&Job{ID: 100, Key: "invalidate-A"}))
	_ = sender.Tell(ctx, router, actor.NewBroadcast(&Job{ID: 101, Key: "invalidate-B"}))

	time.Sleep(300 * time.Millisecond)

	_ = system.Kill(ctx, "fan")
}

// consistentHashDemo: messages with the same key always land on
// the same routee. Useful for sticky sessions or per-entity locality.
func consistentHashDemo(ctx context.Context, system actor.ActorSystem) {
	logger := system.Logger()
	logger.Info("=== consistent-hash: same key → same worker ===")

	extractor := func(msg any) string {
		if j, ok := msg.(*Job); ok {
			return j.Key
		}
		return ""
	}

	router, err := system.SpawnRouter(ctx, "ch", 3, &Worker{},
		actor.WithConsistentHashRouter(extractor))
	if err != nil {
		logger.Fatal(err)
	}

	sender := system.NoSender()
	keys := []string{"alice", "bob", "alice", "carol", "bob", "alice"}
	for i, k := range keys {
		_ = sender.Tell(ctx, router, actor.NewBroadcast(&Job{ID: i + 1, Key: k}))
	}
	time.Sleep(300 * time.Millisecond)

	// Resize the pool at runtime: keys belonging to an unchanged
	// routee remain stable; only the moved virtual nodes remap.
	_ = sender.Tell(ctx, router, actor.NewAdjustRouterPoolSize(2))
	time.Sleep(100 * time.Millisecond)
	resp, _ := sender.Ask(ctx, router, &actor.GetRoutees{}, time.Second)
	if r, ok := resp.(*actor.Routees); ok {
		logger.Infof("after +2 scale-up, routees: %v", r.Names())
	}

	for i, k := range keys {
		_ = sender.Tell(ctx, router, actor.NewBroadcast(&Job{ID: i + 100, Key: k}))
	}

	time.Sleep(300 * time.Millisecond)

	_ = system.Kill(ctx, "ch")
	fmt.Println()
}
