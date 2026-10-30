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

package hollywood

import (
	"io"
	"log/slog"
	"os"
	"runtime"
	"testing"

	"github.com/anthdm/hollywood/actor"

	"github.com/tochemey/goakt-examples/goakt-benchmarks/internal/bench"
)

var engine *actor.Engine

func TestMain(m *testing.M) {
	// Hollywood logs through the slog default logger.
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))

	var err error
	engine, err = actor.NewEngine(actor.NewEngineConfig())
	if err != nil {
		panic(err)
	}

	os.Exit(m.Run())
}

// sink counts every Ping it receives.
type sink struct {
	latch *bench.Latch
}

func (x *sink) Receive(ctx *actor.Context) {
	switch ctx.Message().(type) {
	case *bench.Ping:
		x.latch.Count()
	}
}

// responder answers every Ping with a Pong.
type responder struct{}

func (x *responder) Receive(ctx *actor.Context) {
	switch ctx.Message().(type) {
	case *bench.Ping:
		ctx.Respond(new(bench.Pong))
	}
}

// pinger keeps a ping-pong exchange going with a ponger until the latch opens.
type pinger struct {
	ponger *actor.PID
	latch  *bench.Latch
}

func (x *pinger) Receive(ctx *actor.Context) {
	switch ctx.Message().(type) {
	case *bench.Start:
		ctx.Send(x.ponger, new(bench.Ping))
	case *bench.Pong:
		if !x.latch.Count() {
			ctx.Send(x.ponger, new(bench.Ping))
		}
	}
}

// ponger replies to the sender of every Ping.
type ponger struct{}

func (x *ponger) Receive(ctx *actor.Context) {
	switch ctx.Message().(type) {
	case *bench.Ping:
		ctx.Respond(new(bench.Pong))
	}
}

// forkJoinWorker counts the messages it receives and opens its slot of the
// shared latch once it has received its share.
type forkJoinWorker struct {
	latch  *bench.Latch
	target int
	count  int
}

func (x *forkJoinWorker) Receive(ctx *actor.Context) {
	switch ctx.Message().(type) {
	case *bench.Ping:
		x.count++
		if x.count == x.target {
			x.latch.Count()
		}
	}
}

// pairPinger is a pinger with its own share of round trips, for the
// parallel ping-pong: it counts its own round trips and opens its slot of the
// shared latch once its share is done.
type pairPinger struct {
	ponger    *actor.PID
	latch     *bench.Latch
	remaining int
}

func (x *pairPinger) Receive(ctx *actor.Context) {
	switch ctx.Message().(type) {
	case *bench.Start:
		ctx.Send(x.ponger, new(bench.Ping))
	case *bench.Pong:
		x.remaining--
		if x.remaining > 0 {
			ctx.Send(x.ponger, new(bench.Ping))
		}
		if x.remaining == 0 {
			x.latch.Count()
		}
	}
}

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

func (x *pipelinedPinger) Receive(ctx *actor.Context) {
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
func (x *pipelinedPinger) send(ctx *actor.Context) {
	n := min(bench.PipelineDepth, x.remaining)
	x.remaining -= n
	x.outstanding = n
	for range n {
		ctx.Send(x.ponger, x.ping)
	}
}

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

func (x *ringActor) Receive(ctx *actor.Context) {
	switch msg := ctx.Message().(type) {
	case *setNext:
		x.next = msg.next
	case *bench.Token:
		if msg.Hops == 0 {
			x.latch.Count()
			return
		}
		msg.Hops--
		ctx.Send(x.next, msg)
	}
}

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

func (x *bigActor) Receive(ctx *actor.Context) {
	switch ctx.Message().(type) {
	case *bench.BigRound:
		ctx.Send(x.peers[bench.NextPeer(x.index, x.round, len(x.peers))], x.ping)
	case *bench.Ping:
		ctx.Respond(x.pong)
	case *bench.Pong:
		x.round++
		if x.round < x.rounds {
			ctx.Send(x.peers[bench.NextPeer(x.index, x.round, len(x.peers))], x.ping)
		}
		if x.round == x.rounds {
			x.latch.Count()
		}
	}
}

// skynetNode is one node of a Skynet tree. On its range it either reports its
// leaf number to its parent, or creates its children and sums their results;
// a node reports once and then stops.
type skynetNode struct {
	total   *bench.SkynetTotal
	parent  *actor.PID
	pending int
	sum     int64
}

func newSkynetNode() actor.Receiver { return &skynetNode{} }

func (x *skynetNode) Receive(ctx *actor.Context) {
	switch msg := ctx.Message().(type) {
	case *bench.SkynetRange:
		x.parent = ctx.Sender()
		if msg.Size == 1 {
			x.report(ctx, msg.Start)
			return
		}

		for _, child := range msg.Children() {
			ctx.Send(ctx.SpawnChild(newSkynetNode, "n"), &child)
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

// report hands the sum up and stops the node.
func (x *skynetNode) report(ctx *actor.Context, sum int64) {
	if x.total != nil {
		x.total.Complete(sum)
	} else {
		ctx.Send(x.parent, &bench.SkynetResult{Sum: sum})
	}
	ctx.Engine().Poison(ctx.PID())
}

// stopper fires a signal once it has stopped.
type stopper struct {
	stopped bench.Signal
}

func (x *stopper) Receive(ctx *actor.Context) {
	switch ctx.Message().(type) {
	case actor.Stopped:
		x.stopped.Fire()
	}
}

func spawn(b *testing.B, producer actor.Producer) *actor.PID {
	b.Helper()
	pid := engine.Spawn(producer, "actor")
	b.Cleanup(func() { <-engine.Poison(pid).Done() })
	return pid
}

func BenchmarkTell(b *testing.B) {
	latch := bench.NewLatch(b.N)
	pid := spawn(b, func() actor.Receiver { return &sink{latch: latch} })
	msg := new(bench.Ping)

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		engine.Send(pid, msg)
	}
	latch.Wait(b)
	bench.ReportRate(b, b.N, bench.MsgsPerSec)
}

func BenchmarkTellParallel(b *testing.B) {
	latch := bench.NewLatch(b.N)
	pid := spawn(b, func() actor.Receiver { return &sink{latch: latch} })
	msg := new(bench.Ping)

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			engine.Send(pid, msg)
		}
	})
	latch.Wait(b)
	bench.ReportRate(b, b.N, bench.MsgsPerSec)
}

func BenchmarkAsk(b *testing.B) {
	pid := spawn(b, func() actor.Receiver { return &responder{} })
	msg := new(bench.Ping)

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if _, err := engine.Request(pid, msg, bench.Timeout).Result(); err != nil {
			b.Fatal(err)
		}
	}
	bench.ReportRate(b, b.N, bench.MsgsPerSec)
}

func BenchmarkAskParallel(b *testing.B) {
	pid := spawn(b, func() actor.Receiver { return &responder{} })
	msg := new(bench.Ping)

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if _, err := engine.Request(pid, msg, bench.Timeout).Result(); err != nil {
				b.Error(err)
				return
			}
		}
	})
	bench.ReportRate(b, b.N, bench.MsgsPerSec)
}

func BenchmarkPingPong(b *testing.B) {
	latch := bench.NewLatch(b.N)
	pong := spawn(b, func() actor.Receiver { return &ponger{} })
	ping := spawn(b, func() actor.Receiver { return &pinger{ponger: pong, latch: latch} })

	b.ReportAllocs()
	b.ResetTimer()
	engine.Send(ping, new(bench.Start))
	latch.Wait(b)
	bench.ReportRate(b, 2*b.N, bench.MsgsPerSec)
}

func BenchmarkPingPongParallel(b *testing.B) {
	pairs := runtime.GOMAXPROCS(0)
	latch := bench.NewLatch(min(pairs, b.N))
	pingers := make([]*actor.PID, 0, pairs)
	for i := range pairs {
		remaining := bench.Share(b.N, i, pairs)
		if remaining == 0 {
			continue
		}
		pong := spawn(b, func() actor.Receiver { return &ponger{} })
		pingers = append(pingers, spawn(b, func() actor.Receiver {
			return &pairPinger{ponger: pong, latch: latch, remaining: remaining}
		}))
	}

	b.ReportAllocs()
	b.ResetTimer()
	for _, ping := range pingers {
		engine.Send(ping, new(bench.Start))
	}
	latch.Wait(b)
	bench.ReportRate(b, 2*b.N, bench.MsgsPerSec)
}

func BenchmarkPingPongPipelined(b *testing.B) {
	pairs := runtime.GOMAXPROCS(0)
	latch := bench.NewLatch(min(pairs, b.N))
	pingers := make([]*actor.PID, 0, pairs)
	for i := range pairs {
		remaining := bench.Share(b.N, i, pairs)
		if remaining == 0 {
			continue
		}
		pong := spawn(b, func() actor.Receiver { return &ponger{} })
		pingers = append(pingers, spawn(b, func() actor.Receiver {
			return &pipelinedPinger{ponger: pong, latch: latch, remaining: remaining, ping: new(bench.Ping)}
		}))
	}

	b.ReportAllocs()
	b.ResetTimer()
	for _, ping := range pingers {
		engine.Send(ping, new(bench.Start))
	}
	latch.Wait(b)
	bench.ReportRate(b, 2*b.N, bench.MsgsPerSec)
}

func BenchmarkThreadRing(b *testing.B) {
	latch := bench.NewLatch(1)
	ring := make([]*actor.PID, bench.ThreadRingActors)
	for i := len(ring) - 1; i >= 0; i-- {
		var next *actor.PID
		if i < len(ring)-1 {
			next = ring[i+1]
		}
		ring[i] = spawn(b, func() actor.Receiver { return &ringActor{next: next, latch: latch} })
	}
	engine.Send(ring[len(ring)-1], &setNext{next: ring[0]})

	b.ReportAllocs()
	b.ResetTimer()
	engine.Send(ring[0], &bench.Token{Hops: b.N})
	latch.Wait(b)
	bench.ReportRate(b, b.N, bench.MsgsPerSec)
}

func BenchmarkForkJoinThroughput(b *testing.B) {
	workers := make([]*actor.PID, 0, bench.ForkJoinActors)
	latch := bench.NewLatch(min(bench.ForkJoinActors, b.N))
	for i := range bench.ForkJoinActors {
		if target := bench.Share(b.N, i, bench.ForkJoinActors); target > 0 {
			workers = append(workers, spawn(b, func() actor.Receiver { return &forkJoinWorker{latch: latch, target: target} }))
		}
	}
	msg := new(bench.Ping)

	b.ReportAllocs()
	b.ResetTimer()
	for i := range b.N {
		engine.Send(workers[i%len(workers)], msg)
	}
	latch.Wait(b)
	bench.ReportRate(b, b.N, bench.MsgsPerSec)
}

func BenchmarkBig(b *testing.B) {
	rounds := max(b.N/bench.BigActors, 1)
	exchanges := rounds * bench.BigActors
	latch := bench.NewLatch(bench.BigActors)
	peers := make([]*actor.PID, bench.BigActors)
	for i := range peers {
		peers[i] = spawn(b, func() actor.Receiver {
			return &bigActor{index: i, peers: peers, rounds: rounds, latch: latch, ping: new(bench.Ping), pong: new(bench.Pong)}
		})
	}

	b.ReportAllocs()
	b.ResetTimer()
	for _, pid := range peers {
		engine.Send(pid, new(bench.BigRound))
	}
	latch.Wait(b)
	bench.ReportRate(b, 2*exchanges, bench.MsgsPerSec)
}

func BenchmarkSkynet(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		total := bench.NewSkynetTotal()
		root := engine.Spawn(func() actor.Receiver { return &skynetNode{total: total} }, "skynet")
		engine.Send(root, &bench.SkynetRange{Start: 0, Size: bench.SkynetLeaves})
		total.Wait(b)
	}
	bench.ReportRate(b, b.N*bench.SkynetActors, bench.ActorsPerSec)
}

func BenchmarkMemoryPerActor(b *testing.B) {
	producer := func() actor.Receiver { return &sink{} }

	for range b.N {
		before := bench.MeasureHeap()
		pids := make([]*actor.PID, bench.MemoryActors)
		for i := range pids {
			pids[i] = engine.Spawn(producer, "idle")
		}
		after := bench.MeasureHeap()
		bench.ReportHeapPerActor(b, before, after, len(pids))

		b.StopTimer()
		for _, pid := range pids {
			<-engine.Poison(pid).Done()
		}
		b.StartTimer()
	}
}

func BenchmarkSpawnStop(b *testing.B) {
	stopped := bench.NewSignal()
	producer := func() actor.Receiver { return &stopper{stopped: stopped} }

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		pid := engine.Spawn(producer, "actor")
		engine.Stop(pid)
		stopped.Wait(b)
	}
	bench.ReportRate(b, b.N, bench.ActorsPerSec)
}
