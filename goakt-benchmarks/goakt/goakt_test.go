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

package goakt

import (
	"context"
	"os"
	"runtime"
	"testing"

	"github.com/tochemey/goakt/v4/actor"
	"github.com/tochemey/goakt/v4/log"

	"github.com/tochemey/goakt-examples/goakt-benchmarks/internal/bench"
)

var system actor.ActorSystem

func TestMain(m *testing.M) {
	ctx := context.Background()

	var err error
	system, err = actor.NewActorSystem("bench", actor.WithLogger(log.DiscardLogger))
	if err != nil {
		panic(err)
	}

	if err := system.Start(ctx); err != nil {
		panic(err)
	}

	code := m.Run()
	_ = system.Stop(ctx)
	os.Exit(code)
}

// sink counts every Ping it receives.
type sink struct {
	latch *bench.Latch
}

var _ actor.Actor = (*sink)(nil)

func (x *sink) PreStart(*actor.Context) error { return nil }

func (x *sink) Receive(ctx *actor.ReceiveContext) {
	switch ctx.Message().(type) {
	case *bench.Ping:
		x.latch.Count()
	}
}

func (x *sink) PostStop(*actor.Context) error { return nil }

// responder answers every Ping with a Pong.
type responder struct{}

var _ actor.Actor = (*responder)(nil)

func (x *responder) PreStart(*actor.Context) error { return nil }

func (x *responder) Receive(ctx *actor.ReceiveContext) {
	switch ctx.Message().(type) {
	case *bench.Ping:
		ctx.Response(new(bench.Pong))
	}
}

func (x *responder) PostStop(*actor.Context) error { return nil }

// pinger keeps a ping-pong exchange going with a ponger until the latch opens.
type pinger struct {
	ponger *actor.PID
	latch  *bench.Latch
}

var _ actor.Actor = (*pinger)(nil)

func (x *pinger) PreStart(*actor.Context) error { return nil }

func (x *pinger) Receive(ctx *actor.ReceiveContext) {
	switch ctx.Message().(type) {
	case *bench.Start:
		ctx.Tell(x.ponger, new(bench.Ping))
	case *bench.Pong:
		if !x.latch.Count() {
			ctx.Tell(x.ponger, new(bench.Ping))
		}
	}
}

func (x *pinger) PostStop(*actor.Context) error { return nil }

// ponger replies to the sender of every Ping.
type ponger struct{}

var _ actor.Actor = (*ponger)(nil)

func (x *ponger) PreStart(*actor.Context) error { return nil }

func (x *ponger) Receive(ctx *actor.ReceiveContext) {
	switch ctx.Message().(type) {
	case *bench.Ping:
		ctx.Tell(ctx.Sender(), new(bench.Pong))
	}
}

func (x *ponger) PostStop(*actor.Context) error { return nil }

// forkJoinWorker counts the messages it receives and opens its slot of the
// shared latch once it has received its share.
type forkJoinWorker struct {
	latch  *bench.Latch
	target int
	count  int
}

var _ actor.Actor = (*forkJoinWorker)(nil)

func (x *forkJoinWorker) PreStart(*actor.Context) error { return nil }

func (x *forkJoinWorker) Receive(ctx *actor.ReceiveContext) {
	switch ctx.Message().(type) {
	case *bench.Ping:
		x.count++
		if x.count == x.target {
			x.latch.Count()
		}
	}
}

func (x *forkJoinWorker) PostStop(*actor.Context) error { return nil }

// pairPinger is a pinger with its own share of round trips, for the
// parallel ping-pong: it counts its own round trips and opens its slot of the
// shared latch once its share is done.
type pairPinger struct {
	ponger    *actor.PID
	latch     *bench.Latch
	remaining int
}

var _ actor.Actor = (*pairPinger)(nil)

func (x *pairPinger) PreStart(*actor.Context) error { return nil }

func (x *pairPinger) Receive(ctx *actor.ReceiveContext) {
	switch ctx.Message().(type) {
	case *bench.Start:
		ctx.Tell(x.ponger, new(bench.Ping))
	case *bench.Pong:
		x.remaining--
		if x.remaining > 0 {
			ctx.Tell(x.ponger, new(bench.Ping))
		}
		if x.remaining == 0 {
			x.latch.Count()
		}
	}
}

func (x *pairPinger) PostStop(*actor.Context) error { return nil }

// pipelinedPinger keeps PipelineDepth pings in flight to its ponger and sends
// the next batch once every reply of the current one is back, until its share
// of round trips is done; then it opens its slot of the shared latch.
type pipelinedPinger struct {
	ponger      *actor.PID
	latch       *bench.Latch
	remaining   int
	outstanding int
	ping        *bench.Ping
}

var _ actor.Actor = (*pipelinedPinger)(nil)

func (x *pipelinedPinger) PreStart(*actor.Context) error { return nil }

func (x *pipelinedPinger) Receive(ctx *actor.ReceiveContext) {
	switch ctx.Message().(type) {
	case *bench.Start:
		x.send(ctx)
	case *bench.Pong:
		x.outstanding--
		if x.outstanding == 0 && x.remaining > 0 {
			x.send(ctx)
		}
		if x.outstanding == 0 && x.remaining == 0 {
			x.latch.Count()
		}
	}
}

// send sends the next batch, at most the round trips left.
func (x *pipelinedPinger) send(ctx *actor.ReceiveContext) {
	n := min(bench.PipelineDepth, x.remaining)
	x.remaining -= n
	x.outstanding = n
	for range n {
		ctx.Tell(x.ponger, x.ping)
	}
}

func (x *pipelinedPinger) PostStop(*actor.Context) error { return nil }

// setNext closes a thread ring: it tells the last actor which one follows it.
type setNext struct {
	next *actor.PID
}

// ringActor passes the token to the next actor in the ring until its hops run
// out, then opens the latch.
type ringActor struct {
	next  *actor.PID
	latch *bench.Latch
}

var _ actor.Actor = (*ringActor)(nil)

func (x *ringActor) PreStart(*actor.Context) error { return nil }

func (x *ringActor) Receive(ctx *actor.ReceiveContext) {
	switch msg := ctx.Message().(type) {
	case *setNext:
		x.next = msg.next
	case *bench.Token:
		if msg.Hops == 0 {
			x.latch.Count()
			return
		}
		msg.Hops--
		ctx.Tell(x.next, msg)
	}
}

func (x *ringActor) PostStop(*actor.Context) error { return nil }

// bigActor is one of the actors of a Big exchange: it pings a peer per round,
// answers every Ping it receives, and opens its slot of the shared latch after
// its last round.
type bigActor struct {
	index  int
	peers  []*actor.PID
	rounds int
	round  int
	latch  *bench.Latch
	ping   *bench.Ping
	pong   *bench.Pong
}

var _ actor.Actor = (*bigActor)(nil)

func (x *bigActor) PreStart(*actor.Context) error { return nil }

func (x *bigActor) Receive(ctx *actor.ReceiveContext) {
	switch ctx.Message().(type) {
	case *bench.BigRound:
		ctx.Tell(x.peers[bench.NextPeer(x.index, x.round, len(x.peers))], x.ping)
	case *bench.Ping:
		ctx.Tell(ctx.Sender(), x.pong)
	case *bench.Pong:
		x.round++
		if x.round < x.rounds {
			ctx.Tell(x.peers[bench.NextPeer(x.index, x.round, len(x.peers))], x.ping)
		}
		if x.round == x.rounds {
			x.latch.Count()
		}
	}
}

func (x *bigActor) PostStop(*actor.Context) error { return nil }

// skynetNode is one node of a Skynet tree. On its range it either reports its
// leaf number to its parent, or creates its children and sums their results;
// a node reports once and then stops.
type skynetNode struct {
	total   *bench.SkynetTotal
	parent  *actor.PID
	pending int
	sum     int64
}

var _ actor.Actor = (*skynetNode)(nil)

func (x *skynetNode) PreStart(*actor.Context) error { return nil }

func (x *skynetNode) Receive(ctx *actor.ReceiveContext) {
	switch msg := ctx.Message().(type) {
	case *bench.SkynetRange:
		x.parent = ctx.Sender()
		if msg.Size == 1 {
			x.report(ctx, msg.Start)
			return
		}

		for _, child := range msg.Children() {
			ctx.Tell(ctx.Spawn(bench.Name("n"), &skynetNode{}, actor.WithLongLived()), &child)
		}
		x.pending = bench.SkynetFanOut
	case *bench.SkynetResult:
		x.sum += msg.Sum
		x.pending--
		if x.pending == 0 {
			x.report(ctx, x.sum)
		}
	}
}

func (x *skynetNode) report(ctx *actor.ReceiveContext, sum int64) {
	if x.total != nil {
		x.total.Complete(sum)
	} else {
		ctx.Tell(x.parent, &bench.SkynetResult{Sum: sum})
	}
	ctx.Shutdown()
}

func (x *skynetNode) PostStop(*actor.Context) error { return nil }

// stopper fires a signal once it has stopped.
type stopper struct {
	stopped bench.Signal
}

var _ actor.Actor = (*stopper)(nil)

func (x *stopper) PreStart(*actor.Context) error { return nil }

func (x *stopper) Receive(*actor.ReceiveContext) {}

func (x *stopper) PostStop(*actor.Context) error {
	x.stopped.Fire()
	return nil
}

func spawn(b *testing.B, a actor.Actor) *actor.PID {
	b.Helper()
	ctx := context.Background()
	pid, err := system.Spawn(ctx, bench.Name("actor"), a, actor.WithLongLived())
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = pid.Shutdown(ctx) })
	return pid
}

func BenchmarkTell(b *testing.B) {
	ctx := context.Background()
	latch := bench.NewLatch(b.N)
	pid := spawn(b, &sink{latch: latch})
	msg := new(bench.Ping)

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if err := actor.Tell(ctx, pid, msg); err != nil {
			b.Fatal(err)
		}
	}
	latch.Wait(b)
	bench.ReportRate(b, b.N, bench.MsgsPerSec)
}

func BenchmarkTellParallel(b *testing.B) {
	ctx := context.Background()
	latch := bench.NewLatch(b.N)
	pid := spawn(b, &sink{latch: latch})
	msg := new(bench.Ping)

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if err := actor.Tell(ctx, pid, msg); err != nil {
				b.Error(err)
				return
			}
		}
	})
	latch.Wait(b)
	bench.ReportRate(b, b.N, bench.MsgsPerSec)
}

func BenchmarkAsk(b *testing.B) {
	ctx := context.Background()
	pid := spawn(b, &responder{})
	msg := new(bench.Ping)

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if _, err := actor.Ask(ctx, pid, msg, bench.Timeout); err != nil {
			b.Fatal(err)
		}
	}
	bench.ReportRate(b, b.N, bench.MsgsPerSec)
}

func BenchmarkAskParallel(b *testing.B) {
	ctx := context.Background()
	pid := spawn(b, &responder{})
	msg := new(bench.Ping)

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if _, err := actor.Ask(ctx, pid, msg, bench.Timeout); err != nil {
				b.Error(err)
				return
			}
		}
	})
	bench.ReportRate(b, b.N, bench.MsgsPerSec)
}

func BenchmarkPingPong(b *testing.B) {
	ctx := context.Background()
	latch := bench.NewLatch(b.N)
	pong := spawn(b, &ponger{})
	ping := spawn(b, &pinger{ponger: pong, latch: latch})

	b.ReportAllocs()
	b.ResetTimer()
	if err := actor.Tell(ctx, ping, new(bench.Start)); err != nil {
		b.Fatal(err)
	}
	latch.Wait(b)
	bench.ReportRate(b, 2*b.N, bench.MsgsPerSec)
}

func BenchmarkPingPongParallel(b *testing.B) {
	ctx := context.Background()
	pairs := runtime.GOMAXPROCS(0)
	latch := bench.NewLatch(min(pairs, b.N))
	pingers := make([]*actor.PID, 0, pairs)
	for i := range pairs {
		remaining := bench.Share(b.N, i, pairs)
		if remaining == 0 {
			continue
		}
		pong := spawn(b, &ponger{})
		pingers = append(pingers, spawn(b, &pairPinger{ponger: pong, latch: latch, remaining: remaining}))
	}

	b.ReportAllocs()
	b.ResetTimer()
	for _, ping := range pingers {
		if err := actor.Tell(ctx, ping, new(bench.Start)); err != nil {
			b.Fatal(err)
		}
	}
	latch.Wait(b)
	bench.ReportRate(b, 2*b.N, bench.MsgsPerSec)
}

func BenchmarkPingPongPipelined(b *testing.B) {
	ctx := context.Background()
	pairs := runtime.GOMAXPROCS(0)
	latch := bench.NewLatch(min(pairs, b.N))
	pingers := make([]*actor.PID, 0, pairs)
	for i := range pairs {
		remaining := bench.Share(b.N, i, pairs)
		if remaining == 0 {
			continue
		}
		pong := spawn(b, &ponger{})
		pingers = append(pingers, spawn(b, &pipelinedPinger{ponger: pong, latch: latch, remaining: remaining, ping: new(bench.Ping)}))
	}

	b.ReportAllocs()
	b.ResetTimer()
	for _, ping := range pingers {
		if err := actor.Tell(ctx, ping, new(bench.Start)); err != nil {
			b.Fatal(err)
		}
	}
	latch.Wait(b)
	bench.ReportRate(b, 2*b.N, bench.MsgsPerSec)
}

func BenchmarkThreadRing(b *testing.B) {
	ctx := context.Background()
	latch := bench.NewLatch(1)
	ring := make([]*actor.PID, bench.ThreadRingActors)
	for i := len(ring) - 1; i >= 0; i-- {
		var next *actor.PID
		if i < len(ring)-1 {
			next = ring[i+1]
		}
		ring[i] = spawn(b, &ringActor{next: next, latch: latch})
	}
	if err := actor.Tell(ctx, ring[len(ring)-1], &setNext{next: ring[0]}); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	if err := actor.Tell(ctx, ring[0], &bench.Token{Hops: b.N}); err != nil {
		b.Fatal(err)
	}
	latch.Wait(b)
	bench.ReportRate(b, b.N, bench.MsgsPerSec)
}

func BenchmarkForkJoinThroughput(b *testing.B) {
	ctx := context.Background()
	workers := make([]*actor.PID, 0, bench.ForkJoinActors)
	latch := bench.NewLatch(min(bench.ForkJoinActors, b.N))
	for i := range bench.ForkJoinActors {
		if target := bench.Share(b.N, i, bench.ForkJoinActors); target > 0 {
			workers = append(workers, spawn(b, &forkJoinWorker{latch: latch, target: target}))
		}
	}
	msg := new(bench.Ping)

	b.ReportAllocs()
	b.ResetTimer()
	for i := range b.N {
		if err := actor.Tell(ctx, workers[i%len(workers)], msg); err != nil {
			b.Fatal(err)
		}
	}
	latch.Wait(b)
	bench.ReportRate(b, b.N, bench.MsgsPerSec)
}

func BenchmarkBig(b *testing.B) {
	ctx := context.Background()
	rounds := max(b.N/bench.BigActors, 1)
	exchanges := rounds * bench.BigActors
	latch := bench.NewLatch(bench.BigActors)
	peers := make([]*actor.PID, bench.BigActors)
	actors := make([]*bigActor, bench.BigActors)
	for i := range peers {
		actors[i] = &bigActor{index: i, peers: peers, rounds: rounds, latch: latch, ping: new(bench.Ping), pong: new(bench.Pong)}
		peers[i] = spawn(b, actors[i])
	}

	b.ReportAllocs()
	b.ResetTimer()
	for _, pid := range peers {
		if err := actor.Tell(ctx, pid, new(bench.BigRound)); err != nil {
			b.Fatal(err)
		}
	}
	latch.Wait(b)
	bench.ReportRate(b, 2*exchanges, bench.MsgsPerSec)
}

func BenchmarkSkynet(b *testing.B) {
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		total := bench.NewSkynetTotal()
		root, err := system.Spawn(ctx, bench.Name("skynet"), &skynetNode{total: total}, actor.WithLongLived())
		if err != nil {
			b.Fatal(err)
		}
		if err := actor.Tell(ctx, root, &bench.SkynetRange{Start: 0, Size: bench.SkynetLeaves}); err != nil {
			b.Fatal(err)
		}
		total.Wait(b)
	}
	bench.ReportRate(b, b.N*bench.SkynetActors, bench.ActorsPerSec)
}

func BenchmarkMemoryPerActor(b *testing.B) {
	ctx := context.Background()

	for range b.N {
		before := bench.MeasureHeap()
		pids := make([]*actor.PID, bench.MemoryActors)
		for i := range pids {
			pid, err := system.Spawn(ctx, bench.Name("idle"), &sink{}, actor.WithLongLived())
			if err != nil {
				b.Fatal(err)
			}
			pids[i] = pid
		}
		after := bench.MeasureHeap()
		bench.ReportHeapPerActor(b, before, after, len(pids))

		b.StopTimer()
		for _, pid := range pids {
			_ = pid.Shutdown(ctx)
		}
		b.StartTimer()
	}
}

func BenchmarkSpawnStop(b *testing.B) {
	ctx := context.Background()
	stopped := bench.NewSignal()

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		pid, err := system.Spawn(ctx, bench.Name("actor"), &stopper{stopped: stopped})
		if err != nil {
			b.Fatal(err)
		}
		if err := pid.Shutdown(ctx); err != nil {
			b.Fatal(err)
		}
		stopped.Wait(b)
	}
	bench.ReportRate(b, b.N, bench.ActorsPerSec)
}
