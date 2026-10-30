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

package ergo

import (
	"os"
	"runtime"
	"testing"

	"ergo.services/ergo"
	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"

	"github.com/tochemey/goakt-examples/goakt-benchmarks/internal/bench"
)

var node gen.Node

func TestMain(m *testing.M) {
	var options gen.NodeOptions
	options.Network.Mode = gen.NetworkModeDisabled
	options.Log.Level = gen.LogLevelDisabled
	options.Log.DefaultLogger.Disable = true

	var err error
	node, err = ergo.StartNode("bench@localhost", options)
	if err != nil {
		panic(err)
	}

	code := m.Run()
	node.Stop()
	os.Exit(code)
}

// sink counts every Ping it receives.
type sink struct {
	act.Actor
	latch *bench.Latch
}

func (x *sink) HandleMessage(_ gen.PID, message any) error {
	switch message.(type) {
	case *bench.Ping:
		x.latch.Count()
	}
	return nil
}

// responder answers every Ping with a Pong.
type responder struct {
	act.Actor
}

func (x *responder) HandleCall(_ gen.PID, _ gen.Ref, request any) (any, error) {
	switch request.(type) {
	case *bench.Ping:
		return new(bench.Pong), nil
	}
	return nil, nil
}

// pinger keeps a ping-pong exchange going with a ponger until the latch opens.
type pinger struct {
	act.Actor
	ponger gen.PID
	latch  *bench.Latch
}

func (x *pinger) HandleMessage(_ gen.PID, message any) error {
	switch message.(type) {
	case *bench.Start:
		return x.Send(x.ponger, new(bench.Ping))
	case *bench.Pong:
		if !x.latch.Count() {
			return x.Send(x.ponger, new(bench.Ping))
		}
	}
	return nil
}

// ponger replies to the sender of every Ping.
type ponger struct {
	act.Actor
}

func (x *ponger) HandleMessage(from gen.PID, message any) error {
	switch message.(type) {
	case *bench.Ping:
		return x.Send(from, new(bench.Pong))
	}
	return nil
}

// forkJoinWorker counts the messages it receives and opens its slot of the
// shared latch once it has received its share.
type forkJoinWorker struct {
	act.Actor
	latch  *bench.Latch
	target int
	count  int
}

func (x *forkJoinWorker) HandleMessage(_ gen.PID, message any) error {
	switch message.(type) {
	case *bench.Ping:
		x.count++
		if x.count == x.target {
			x.latch.Count()
		}
	}
	return nil
}

// pairPinger is a pinger with its own share of round trips, for the
// parallel ping-pong: it counts its own round trips and opens its slot of the
// shared latch once its share is done.
type pairPinger struct {
	act.Actor
	ponger    gen.PID
	latch     *bench.Latch
	remaining int
}

func (x *pairPinger) HandleMessage(_ gen.PID, message any) error {
	switch message.(type) {
	case *bench.Start:
		return x.SendPID(x.ponger, new(bench.Ping))
	case *bench.Pong:
		x.remaining--
		if x.remaining > 0 {
			return x.SendPID(x.ponger, new(bench.Ping))
		}
		if x.remaining == 0 {
			x.latch.Count()
		}
	}
	return nil
}

// pipelinedPinger keeps PipelineDepth pings in flight to its ponger and sends
// the next batch once every reply of the current one is back, until its share
// of round trips is done; then it opens its slot of the shared latch.
type pipelinedPinger struct {
	act.Actor
	ponger      gen.PID
	latch       *bench.Latch
	remaining   int
	outstanding int
	ping        *bench.Ping
}

func (x *pipelinedPinger) HandleMessage(_ gen.PID, message any) error {
	switch message.(type) {
	case *bench.Start:
		return x.send()
	case *bench.Pong:
		x.outstanding--
		if x.outstanding == 0 && x.remaining > 0 {
			return x.send()
		}
		if x.outstanding == 0 && x.remaining == 0 {
			x.latch.Count()
		}
	}
	return nil
}

// send sends the next batch, at most the round trips left.
func (x *pipelinedPinger) send() error {
	n := min(bench.PipelineDepth, x.remaining)
	x.remaining -= n
	x.outstanding = n
	for range n {
		if err := x.SendPID(x.ponger, x.ping); err != nil {
			return err
		}
	}
	return nil
}

// setNext closes a thread ring: it tells the last actor which one follows it.
type setNext struct {
	next gen.PID
}

// ringActor passes the token to the next actor in the ring until its hops run
// out, then opens the latch.
type ringActor struct {
	act.Actor
	next  gen.PID
	latch *bench.Latch
}

func (x *ringActor) HandleMessage(_ gen.PID, message any) error {
	switch msg := message.(type) {
	case *setNext:
		x.next = msg.next
	case *bench.Token:
		if msg.Hops == 0 {
			x.latch.Count()
			return nil
		}
		msg.Hops--
		return x.SendPID(x.next, msg)
	}
	return nil
}

// bigActor is one of the actors of a Big exchange: it pings a peer per round,
// answers every Ping it receives, and opens its slot of the shared latch after
// its last round.
type bigActor struct {
	act.Actor
	index  int
	peers  []gen.PID
	rounds int
	round  int
	latch  *bench.Latch
	ping   *bench.Ping
	pong   *bench.Pong
}

func (x *bigActor) HandleMessage(from gen.PID, message any) error {
	switch message.(type) {
	case *bench.BigRound:
		return x.SendPID(x.peers[bench.NextPeer(x.index, x.round, len(x.peers))], x.ping)
	case *bench.Ping:
		return x.SendPID(from, x.pong)
	case *bench.Pong:
		x.round++
		if x.round < x.rounds {
			return x.SendPID(x.peers[bench.NextPeer(x.index, x.round, len(x.peers))], x.ping)
		}
		if x.round == x.rounds {
			x.latch.Count()
		}
	}
	return nil
}

// skynetNode is one node of a Skynet tree. On its range it either reports its
// leaf number to its parent, or creates its children and sums their results;
// a node reports once and then stops.
type skynetNode struct {
	act.Actor
	total   *bench.SkynetTotal
	parent  gen.PID
	pending int
	sum     int64
}

func newSkynetNode() gen.ProcessBehavior { return &skynetNode{} }

func (x *skynetNode) HandleMessage(from gen.PID, message any) error {
	switch msg := message.(type) {
	case *bench.SkynetRange:
		x.parent = from
		if msg.Size == 1 {
			return x.report(msg.Start)
		}

		for _, child := range msg.Children() {
			pid, err := x.Spawn(newSkynetNode, gen.ProcessOptions{})
			if err != nil {
				return err
			}
			if err := x.SendPID(pid, &child); err != nil {
				return err
			}
		}
		x.pending = bench.SkynetFanOut
	case *bench.SkynetResult:
		x.sum += msg.Sum
		x.pending--
		if x.pending == 0 {
			return x.report(x.sum)
		}
	}
	return nil
}

// report hands the sum up and stops the node.
func (x *skynetNode) report(sum int64) error {
	if x.total != nil {
		x.total.Complete(sum)
		return gen.TerminateReasonNormal
	}
	if err := x.SendPID(x.parent, &bench.SkynetResult{Sum: sum}); err != nil {
		return err
	}
	return gen.TerminateReasonNormal
}

// stopper fires a signal once it has stopped.
type stopper struct {
	act.Actor
	stopped bench.Signal
}

func (x *stopper) Terminate(error) {
	x.stopped.Fire()
}

func spawn(b *testing.B, factory gen.ProcessFactory) gen.PID {
	b.Helper()
	pid, err := node.Spawn(factory, gen.ProcessOptions{})
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = node.Kill(pid) })
	return pid
}

func BenchmarkTell(b *testing.B) {
	latch := bench.NewLatch(b.N)
	pid := spawn(b, func() gen.ProcessBehavior { return &sink{latch: latch} })
	msg := new(bench.Ping)

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if err := node.Send(pid, msg); err != nil {
			b.Fatal(err)
		}
	}
	latch.Wait(b)
	bench.ReportRate(b, b.N, bench.MsgsPerSec)
}

func BenchmarkTellParallel(b *testing.B) {
	latch := bench.NewLatch(b.N)
	pid := spawn(b, func() gen.ProcessBehavior { return &sink{latch: latch} })
	msg := new(bench.Ping)

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if err := node.Send(pid, msg); err != nil {
				b.Error(err)
				return
			}
		}
	})
	latch.Wait(b)
	bench.ReportRate(b, b.N, bench.MsgsPerSec)
}

func BenchmarkAsk(b *testing.B) {
	pid := spawn(b, func() gen.ProcessBehavior { return &responder{} })
	msg := new(bench.Ping)
	timeout := int(bench.Timeout.Seconds())

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if _, err := node.CallWithTimeout(pid, msg, timeout); err != nil {
			b.Fatal(err)
		}
	}
	bench.ReportRate(b, b.N, bench.MsgsPerSec)
}

func BenchmarkAskParallel(b *testing.B) {
	pid := spawn(b, func() gen.ProcessBehavior { return &responder{} })
	msg := new(bench.Ping)
	timeout := int(bench.Timeout.Seconds())

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if _, err := node.CallWithTimeout(pid, msg, timeout); err != nil {
				b.Error(err)
				return
			}
		}
	})
	bench.ReportRate(b, b.N, bench.MsgsPerSec)
}

func BenchmarkPingPong(b *testing.B) {
	latch := bench.NewLatch(b.N)
	pong := spawn(b, func() gen.ProcessBehavior { return &ponger{} })
	ping := spawn(b, func() gen.ProcessBehavior { return &pinger{ponger: pong, latch: latch} })

	b.ReportAllocs()
	b.ResetTimer()
	if err := node.Send(ping, new(bench.Start)); err != nil {
		b.Fatal(err)
	}
	latch.Wait(b)
	bench.ReportRate(b, 2*b.N, bench.MsgsPerSec)
}

func BenchmarkPingPongParallel(b *testing.B) {
	pairs := runtime.GOMAXPROCS(0)
	latch := bench.NewLatch(min(pairs, b.N))
	pingers := make([]gen.PID, 0, pairs)
	for i := range pairs {
		remaining := bench.Share(b.N, i, pairs)
		if remaining == 0 {
			continue
		}
		pong := spawn(b, func() gen.ProcessBehavior { return &ponger{} })
		pingers = append(pingers, spawn(b, func() gen.ProcessBehavior {
			return &pairPinger{ponger: pong, latch: latch, remaining: remaining}
		}))
	}

	b.ReportAllocs()
	b.ResetTimer()
	for _, ping := range pingers {
		if err := node.Send(ping, new(bench.Start)); err != nil {
			b.Fatal(err)
		}
	}
	latch.Wait(b)
	bench.ReportRate(b, 2*b.N, bench.MsgsPerSec)
}

func BenchmarkPingPongPipelined(b *testing.B) {
	pairs := runtime.GOMAXPROCS(0)
	latch := bench.NewLatch(min(pairs, b.N))
	pingers := make([]gen.PID, 0, pairs)
	for i := range pairs {
		remaining := bench.Share(b.N, i, pairs)
		if remaining == 0 {
			continue
		}
		pong := spawn(b, func() gen.ProcessBehavior { return &ponger{} })
		pingers = append(pingers, spawn(b, func() gen.ProcessBehavior {
			return &pipelinedPinger{ponger: pong, latch: latch, remaining: remaining, ping: new(bench.Ping)}
		}))
	}

	b.ReportAllocs()
	b.ResetTimer()
	for _, ping := range pingers {
		if err := node.Send(ping, new(bench.Start)); err != nil {
			b.Fatal(err)
		}
	}
	latch.Wait(b)
	bench.ReportRate(b, 2*b.N, bench.MsgsPerSec)
}

func BenchmarkThreadRing(b *testing.B) {
	latch := bench.NewLatch(1)
	ring := make([]gen.PID, bench.ThreadRingActors)
	for i := len(ring) - 1; i >= 0; i-- {
		var next gen.PID
		if i < len(ring)-1 {
			next = ring[i+1]
		}
		ring[i] = spawn(b, func() gen.ProcessBehavior { return &ringActor{next: next, latch: latch} })
	}
	if err := node.Send(ring[len(ring)-1], &setNext{next: ring[0]}); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	if err := node.Send(ring[0], &bench.Token{Hops: b.N}); err != nil {
		b.Fatal(err)
	}
	latch.Wait(b)
	bench.ReportRate(b, b.N, bench.MsgsPerSec)
}

func BenchmarkForkJoinThroughput(b *testing.B) {
	workers := make([]gen.PID, 0, bench.ForkJoinActors)
	latch := bench.NewLatch(min(bench.ForkJoinActors, b.N))
	for i := range bench.ForkJoinActors {
		if target := bench.Share(b.N, i, bench.ForkJoinActors); target > 0 {
			workers = append(workers, spawn(b, func() gen.ProcessBehavior { return &forkJoinWorker{latch: latch, target: target} }))
		}
	}
	msg := new(bench.Ping)

	b.ReportAllocs()
	b.ResetTimer()
	for i := range b.N {
		if err := node.Send(workers[i%len(workers)], msg); err != nil {
			b.Fatal(err)
		}
	}
	latch.Wait(b)
	bench.ReportRate(b, b.N, bench.MsgsPerSec)
}

func BenchmarkBig(b *testing.B) {
	rounds := max(b.N/bench.BigActors, 1)
	exchanges := rounds * bench.BigActors
	latch := bench.NewLatch(bench.BigActors)
	peers := make([]gen.PID, bench.BigActors)
	for i := range peers {
		peers[i] = spawn(b, func() gen.ProcessBehavior {
			return &bigActor{index: i, peers: peers, rounds: rounds, latch: latch, ping: new(bench.Ping), pong: new(bench.Pong)}
		})
	}

	b.ReportAllocs()
	b.ResetTimer()
	for _, pid := range peers {
		if err := node.Send(pid, new(bench.BigRound)); err != nil {
			b.Fatal(err)
		}
	}
	latch.Wait(b)
	bench.ReportRate(b, 2*exchanges, bench.MsgsPerSec)
}

func BenchmarkSkynet(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		total := bench.NewSkynetTotal()
		root, err := node.Spawn(func() gen.ProcessBehavior { return &skynetNode{total: total} }, gen.ProcessOptions{})
		if err != nil {
			b.Fatal(err)
		}
		if err := node.Send(root, &bench.SkynetRange{Start: 0, Size: bench.SkynetLeaves}); err != nil {
			b.Fatal(err)
		}
		total.Wait(b)
	}
	bench.ReportRate(b, b.N*bench.SkynetActors, bench.ActorsPerSec)
}

func BenchmarkMemoryPerActor(b *testing.B) {
	factory := func() gen.ProcessBehavior { return &sink{} }

	for range b.N {
		before := bench.MeasureHeap()
		pids := make([]gen.PID, bench.MemoryActors)
		for i := range pids {
			pid, err := node.Spawn(factory, gen.ProcessOptions{})
			if err != nil {
				b.Fatal(err)
			}
			pids[i] = pid
		}
		after := bench.MeasureHeap()
		bench.ReportHeapPerActor(b, before, after, len(pids))

		b.StopTimer()
		for _, pid := range pids {
			_ = node.Kill(pid)
		}
		b.StartTimer()
	}
}

func BenchmarkSpawnStop(b *testing.B) {
	stopped := bench.NewSignal()
	factory := func() gen.ProcessBehavior { return &stopper{stopped: stopped} }

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		pid, err := node.Spawn(factory, gen.ProcessOptions{})
		if err != nil {
			b.Fatal(err)
		}
		if err := node.SendExit(pid, gen.TerminateReasonNormal); err != nil {
			b.Fatal(err)
		}
		stopped.Wait(b)
	}
	bench.ReportRate(b, b.N, bench.ActorsPerSec)
}
