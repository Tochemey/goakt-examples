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

package protoactor

import (
	"io"
	"log/slog"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/asynkron/protoactor-go/actor"
	"github.com/asynkron/protoactor-go/cluster"
	"github.com/asynkron/protoactor-go/cluster/clusterproviders/test"
	"github.com/asynkron/protoactor-go/cluster/identitylookup/disthash"
	"github.com/asynkron/protoactor-go/remote"

	"github.com/tochemey/goakt-examples/goakt-benchmarks/internal/bench"
)

var system *actor.ActorSystem

// grains is the single-member cluster the grain benchmarks share. It is only
// started when one of them runs, so its remoting and gossip stay out of the
// actor benchmarks.
var (
	grains     *cluster.Cluster
	grainsOnce sync.Once
)

func TestMain(m *testing.M) {
	system = actor.NewActorSystem(actor.WithLoggerFactory(discardLogger))

	code := m.Run()
	system.Shutdown()
	if grains != nil {
		grains.Shutdown(true)
	}
	os.Exit(code)
}

func discardLogger(*actor.ActorSystem) *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// sink counts every Ping it receives.
type sink struct {
	latch *bench.Latch
}

func (x *sink) Receive(ctx actor.Context) {
	switch ctx.Message().(type) {
	case *bench.Ping:
		x.latch.Count()
	}
}

// teller sends n Pings to a target from inside its handler once it receives Start.
type teller struct {
	target *actor.PID
	n      int
}

func (x *teller) Receive(ctx actor.Context) {
	switch ctx.Message().(type) {
	case *bench.Start:
		msg := new(bench.Ping)
		for range x.n {
			ctx.Request(x.target, msg)
		}
	}
}

// responder answers every Ping with a Pong.
type responder struct{}

func (x *responder) Receive(ctx actor.Context) {
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

func (x *pinger) Receive(ctx actor.Context) {
	switch ctx.Message().(type) {
	case *bench.Start:
		ctx.Request(x.ponger, new(bench.Ping))
	case *bench.Pong:
		if !x.latch.Count() {
			ctx.Request(x.ponger, new(bench.Ping))
		}
	}
}

// ponger replies to the sender of every Ping.
type ponger struct{}

func (x *ponger) Receive(ctx actor.Context) {
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

func (x *forkJoinWorker) Receive(ctx actor.Context) {
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

func (x *pairPinger) Receive(ctx actor.Context) {
	switch ctx.Message().(type) {
	case *bench.Start:
		ctx.Request(x.ponger, new(bench.Ping))
	case *bench.Pong:
		x.remaining--
		if x.remaining > 0 {
			ctx.Request(x.ponger, new(bench.Ping))
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

func (x *pipelinedPinger) Receive(ctx actor.Context) {
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
func (x *pipelinedPinger) send(ctx actor.Context) {
	n := min(bench.PipelineDepth, x.remaining)
	x.remaining -= n
	x.outstanding = n
	for range n {
		ctx.Request(x.ponger, x.ping)
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

func (x *ringActor) Receive(ctx actor.Context) {
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

func (x *bigActor) Receive(ctx actor.Context) {
	switch ctx.Message().(type) {
	case *bench.BigRound:
		ctx.Request(x.peers[bench.NextPeer(x.index, x.round, len(x.peers))], x.ping)
	case *bench.Ping:
		ctx.Respond(x.pong)
	case *bench.Pong:
		x.round++
		if x.round < x.rounds {
			ctx.Request(x.peers[bench.NextPeer(x.index, x.round, len(x.peers))], x.ping)
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

var skynetProps = actor.PropsFromProducer(func() actor.Actor { return &skynetNode{} })

func (x *skynetNode) Receive(ctx actor.Context) {
	switch msg := ctx.Message().(type) {
	case *bench.SkynetRange:
		x.parent = ctx.Sender()
		if msg.Size == 1 {
			x.report(ctx, msg.Start)
			return
		}

		for _, child := range msg.Children() {
			ctx.Request(ctx.Spawn(skynetProps), &child)
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
func (x *skynetNode) report(ctx actor.Context, sum int64) {
	if x.total != nil {
		x.total.Complete(sum)
	} else {
		ctx.Send(x.parent, &bench.SkynetResult{Sum: sum})
	}
	ctx.Stop(ctx.Self())
}

// stopper fires a signal once it has stopped.
type stopper struct {
	stopped bench.Signal
}

func (x *stopper) Receive(ctx actor.Context) {
	switch ctx.Message().(type) {
	case *actor.Stopped:
		x.stopped.Fire()
	}
}

// grain answers every Ping with a Pong.
type grain struct{}

func (x *grain) Receive(ctx actor.Context) {
	switch ctx.Message().(type) {
	case *bench.Ping:
		ctx.Respond(new(bench.Pong))
	}
}

// grainKind is the cluster kind every grain is activated as.
const grainKind = "grain"

func spawn(b *testing.B, producer actor.Producer) *actor.PID {
	b.Helper()
	pid := system.Root.Spawn(actor.PropsFromProducer(producer))
	b.Cleanup(func() { _ = system.Root.StopFuture(pid).Wait() })
	return pid
}

func BenchmarkTell(b *testing.B) {
	latch := bench.NewLatch(b.N)
	pid := spawn(b, func() actor.Actor { return &sink{latch: latch} })
	msg := new(bench.Ping)

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		system.Root.Send(pid, msg)
	}
	latch.Wait(b)
	bench.ReportRate(b, b.N, bench.MsgsPerSec)
}

func BenchmarkTellParallel(b *testing.B) {
	latch := bench.NewLatch(b.N)
	pid := spawn(b, func() actor.Actor { return &sink{latch: latch} })
	msg := new(bench.Ping)

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			system.Root.Send(pid, msg)
		}
	})
	latch.Wait(b)
	bench.ReportRate(b, b.N, bench.MsgsPerSec)
}

func BenchmarkTellFromActor(b *testing.B) {
	latch := bench.NewLatch(b.N)
	pid := spawn(b, func() actor.Actor { return &sink{latch: latch} })
	sender := spawn(b, func() actor.Actor { return &teller{target: pid, n: b.N} })

	b.ReportAllocs()
	b.ResetTimer()
	system.Root.Send(sender, new(bench.Start))
	latch.Wait(b)
	bench.ReportRate(b, b.N, bench.MsgsPerSec)
}

func BenchmarkAsk(b *testing.B) {
	pid := spawn(b, func() actor.Actor { return &responder{} })
	msg := new(bench.Ping)

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if _, err := system.Root.RequestFuture(pid, msg, bench.Timeout).Result(); err != nil {
			b.Fatal(err)
		}
	}
	bench.ReportRate(b, b.N, bench.MsgsPerSec)
}

func BenchmarkAskParallel(b *testing.B) {
	pid := spawn(b, func() actor.Actor { return &responder{} })
	msg := new(bench.Ping)

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if _, err := system.Root.RequestFuture(pid, msg, bench.Timeout).Result(); err != nil {
				b.Error(err)
				return
			}
		}
	})
	bench.ReportRate(b, b.N, bench.MsgsPerSec)
}

func BenchmarkPingPong(b *testing.B) {
	latch := bench.NewLatch(b.N)
	pong := spawn(b, func() actor.Actor { return &ponger{} })
	ping := spawn(b, func() actor.Actor { return &pinger{ponger: pong, latch: latch} })

	b.ReportAllocs()
	b.ResetTimer()
	system.Root.Send(ping, new(bench.Start))
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
		pong := spawn(b, func() actor.Actor { return &ponger{} })
		pingers = append(pingers, spawn(b, func() actor.Actor {
			return &pairPinger{ponger: pong, latch: latch, remaining: remaining}
		}))
	}

	b.ReportAllocs()
	b.ResetTimer()
	for _, ping := range pingers {
		system.Root.Send(ping, new(bench.Start))
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
		pong := spawn(b, func() actor.Actor { return &ponger{} })
		pingers = append(pingers, spawn(b, func() actor.Actor {
			return &pipelinedPinger{ponger: pong, latch: latch, remaining: remaining, ping: new(bench.Ping)}
		}))
	}

	b.ReportAllocs()
	b.ResetTimer()
	for _, ping := range pingers {
		system.Root.Send(ping, new(bench.Start))
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
		ring[i] = spawn(b, func() actor.Actor { return &ringActor{next: next, latch: latch} })
	}
	system.Root.Send(ring[len(ring)-1], &setNext{next: ring[0]})

	b.ReportAllocs()
	b.ResetTimer()
	system.Root.Send(ring[0], &bench.Token{Hops: b.N})
	latch.Wait(b)
	bench.ReportRate(b, b.N, bench.MsgsPerSec)
}

func BenchmarkForkJoinThroughput(b *testing.B) {
	workers := make([]*actor.PID, 0, bench.ForkJoinActors)
	latch := bench.NewLatch(min(bench.ForkJoinActors, b.N))
	for i := range bench.ForkJoinActors {
		if target := bench.Share(b.N, i, bench.ForkJoinActors); target > 0 {
			workers = append(workers, spawn(b, func() actor.Actor { return &forkJoinWorker{latch: latch, target: target} }))
		}
	}
	msg := new(bench.Ping)

	b.ReportAllocs()
	b.ResetTimer()
	for i := range b.N {
		system.Root.Send(workers[i%len(workers)], msg)
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
		peers[i] = spawn(b, func() actor.Actor {
			return &bigActor{index: i, peers: peers, rounds: rounds, latch: latch, ping: new(bench.Ping), pong: new(bench.Pong)}
		})
	}

	b.ReportAllocs()
	b.ResetTimer()
	for _, pid := range peers {
		system.Root.Send(pid, new(bench.BigRound))
	}
	latch.Wait(b)
	bench.ReportRate(b, 2*exchanges, bench.MsgsPerSec)
}

func BenchmarkSkynet(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		total := bench.NewSkynetTotal()
		root := system.Root.Spawn(actor.PropsFromProducer(func() actor.Actor { return &skynetNode{total: total} }))
		system.Root.Send(root, &bench.SkynetRange{Start: 0, Size: bench.SkynetLeaves})
		total.Wait(b)
	}
	bench.ReportRate(b, b.N*bench.SkynetActors, bench.ActorsPerSec)
}

func BenchmarkMemoryPerActor(b *testing.B) {
	props := actor.PropsFromProducer(func() actor.Actor { return &sink{} })

	for range b.N {
		before := bench.MeasureHeap()
		pids := make([]*actor.PID, bench.MemoryActors)
		for i := range pids {
			pids[i] = system.Root.Spawn(props)
		}
		after := bench.MeasureHeap()
		bench.ReportHeapPerActor(b, before, after, len(pids))

		b.StopTimer()
		for _, pid := range pids {
			_ = system.Root.StopFuture(pid).Wait()
		}
		b.StartTimer()
	}
}

func BenchmarkSpawnStop(b *testing.B) {
	stopped := bench.NewSignal()
	props := actor.PropsFromProducer(func() actor.Actor { return &stopper{stopped: stopped} })

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		pid := system.Root.Spawn(props)
		system.Root.Stop(pid)
		stopped.Wait(b)
	}
	bench.ReportRate(b, b.N, bench.ActorsPerSec)
}

// startCluster starts a single-member cluster on an actor system of its own.
// The in-memory test provider keeps membership inside the process; remoting
// listens on localhost because a cluster member cannot start without it, but
// every grain is local, so no message crosses it.
func startCluster() *cluster.Cluster {
	sys := actor.NewActorSystem(actor.WithLoggerFactory(discardLogger))
	kind := cluster.NewKind(grainKind, actor.PropsFromProducer(func() actor.Actor { return &grain{} }))
	config := cluster.Configure("bench", test.NewTestProvider(test.NewInMemAgent()), disthash.New(),
		remote.Configure("localhost", 0),
		cluster.WithKinds(kind),
		cluster.WithRequestTimeout(bench.Timeout),
	)
	c := cluster.New(sys, config)
	c.StartMember()
	return c
}

func sharedCluster() *cluster.Cluster {
	grainsOnce.Do(func() { grains = startCluster() })
	return grains
}

// grainCluster starts a cluster of its own for a benchmark that activates a new
// grain per iteration, so those grains are stopped with it instead of
// accumulating in the shared cluster.
func grainCluster(b *testing.B) *cluster.Cluster {
	b.Helper()
	c := startCluster()
	b.Cleanup(func() { c.Shutdown(true) })
	return c
}

func activate(b *testing.B, c *cluster.Cluster) string {
	b.Helper()
	identity := bench.Name("grain")
	if _, err := c.Request(identity, grainKind, new(bench.Ping)); err != nil {
		b.Fatal(err)
	}
	return identity
}

// warmUp activates bench.WarmupGrains grains that the benchmark does not
// measure.
func warmUp(b *testing.B, c *cluster.Cluster) {
	b.Helper()
	for range bench.WarmupGrains {
		activate(b, c)
	}
}

func BenchmarkGrainAsk(b *testing.B) {
	c := sharedCluster()
	identity := activate(b, c)
	msg := new(bench.Ping)

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if _, err := c.Request(identity, grainKind, msg); err != nil {
			b.Fatal(err)
		}
	}
	bench.ReportRate(b, b.N, bench.MsgsPerSec)
}

func BenchmarkGrainAskParallel(b *testing.B) {
	c := sharedCluster()
	identities := make([]string, runtime.GOMAXPROCS(0))
	for i := range identities {
		identities[i] = activate(b, c)
	}
	var next atomic.Int64
	msg := new(bench.Ping)

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		identity := identities[int(next.Add(1)-1)%len(identities)]
		for pb.Next() {
			if _, err := c.Request(identity, grainKind, msg); err != nil {
				b.Error(err)
				return
			}
		}
	})
	bench.ReportRate(b, b.N, bench.MsgsPerSec)
}

func BenchmarkGrainActivate(b *testing.B) {
	c := grainCluster(b)
	warmUp(b, c)

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		activate(b, c)
	}
	bench.ReportRate(b, b.N, bench.GrainsPerSec)
}

func BenchmarkGrainMemoryPerGrain(b *testing.B) {
	c := grainCluster(b)
	warmUp(b, c)

	for range b.N {
		before := bench.MeasureHeap()
		for range bench.MemoryGrains {
			activate(b, c)
		}
		after := bench.MeasureHeap()
		bench.ReportHeapPerGrain(b, before, after, bench.MemoryGrains)
	}
}
