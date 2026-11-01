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

// Package bench holds the workload shared by every framework under test, so
// that each framework package runs the exact same scenarios against its own
// actor implementation.
package bench

import (
	"fmt"
	"runtime"
	"sync/atomic"
	"testing"
	"time"
)

// Timeout bounds every request/response call.
const Timeout = 5 * time.Second

// waitTimeout bounds how long a benchmark waits for an actor to drain the
// messages it was sent.
const waitTimeout = time.Minute

// Throughput units reported next to ns/op.
const (
	MsgsPerSec   = "msgs/s"
	ActorsPerSec = "actors/s"
	GrainsPerSec = "grains/s"
	// BytesPerActor is the live heap one idle actor keeps.
	BytesPerActor = "B/actor"
	// BytesPerGrain is the live heap one idle grain keeps.
	BytesPerGrain = "B/grain"
)

// Sizes of the scenarios that run a fixed population of actors. The Savina
// sizes are the suite's defaults (Imam and Sarkar, "Savina - An Actor
// Benchmark Suite", AGERE 2014); Skynet's is the size the original benchmark
// (github.com/atemerev/skynet) is run at.
const (
	// ThreadRingActors is the number of actors a ThreadRing token passes
	// through.
	ThreadRingActors = 100
	// ForkJoinActors is the number of workers a ForkJoin master fans out to.
	ForkJoinActors = 60
	// BigActors is the number of actors in a Big exchange.
	BigActors = 120
	// SkynetFanOut is the number of children every Skynet node above the
	// leaves creates.
	SkynetFanOut = 10
	// SkynetLeaves is the number of leaves of the Skynet tree: a million, as
	// in the original benchmark, over six levels of ten.
	SkynetLeaves = 1_000_000
	// SkynetActors is the number of actors a Skynet tree creates, the leaves
	// and every node above them.
	SkynetActors = SkynetLeaves + SkynetLeaves/10 + SkynetLeaves/100 + SkynetLeaves/1_000 + SkynetLeaves/10_000 + SkynetLeaves/100_000 + 1
	// SkynetSum is the sum every Skynet run must produce: the leaves are
	// numbered 0 to SkynetLeaves-1.
	SkynetSum = SkynetLeaves * (SkynetLeaves - 1) / 2
	// MemoryActors is the number of idle actors MemoryPerActor spawns.
	MemoryActors = 50_000
	// MemoryGrains is the number of idle grains MemoryPerGrain activates.
	MemoryGrains = 50_000
	// WarmupGrains is the number of grains GrainActivate and MemoryPerGrain
	// activate before they measure, so one-off allocations of a fresh
	// cluster, such as its storage tables, are not charged to the grains
	// measured.
	WarmupGrains = 10_000
	// PipelineDepth is the number of messages a PingPongPipelined pinger
	// keeps in flight to its ponger.
	PipelineDepth = 100
)

// ReportRate reports n units of work over the benchmark's measured time as a
// per-second rate. It must be called after the work has completed.
func ReportRate(b *testing.B, n int, unit string) {
	b.ReportMetric(float64(n)/b.Elapsed().Seconds(), unit)
}

// Ping is the message sent to an actor.
type Ping struct{}

// Pong is the reply an actor sends back for a Ping.
type Pong struct{}

// Start kicks off a ping-pong exchange between two actors.
type Start struct{}

// Token travels around a ThreadRing; Hops is the number of passes left.
type Token struct {
	Hops int
}

// SkynetResult carries the sum of a Skynet subtree up to its parent.
type SkynetResult struct {
	Sum int64
}

// SkynetTotal is the sum the root of a Skynet tree reports to the benchmark.
type SkynetTotal struct {
	Sum  int64
	Done chan struct{}
}

// NewSkynetTotal returns the result slot the root of a Skynet tree fills.
func NewSkynetTotal() *SkynetTotal {
	return &SkynetTotal{Done: make(chan struct{})}
}

// Complete records the total and wakes the benchmark.
func (t *SkynetTotal) Complete(sum int64) {
	t.Sum = sum
	close(t.Done)
}

// Wait blocks until the root has reported, then checks the total, failing the
// benchmark after waitTimeout.
func (t *SkynetTotal) Wait(b *testing.B) {
	b.Helper()
	select {
	case <-t.Done:
	case <-time.After(waitTimeout):
		b.Fatal("timed out waiting for the Skynet tree to complete")
	}

	if t.Sum != SkynetSum {
		b.Fatalf("skynet sum is %d, want %d", t.Sum, SkynetSum)
	}
}

// SkynetRange gives a Skynet node the leaves it covers: the leaves numbered
// Start to Start+Size-1. A node with Size one is a leaf and reports Start; any
// other node divides its range among SkynetFanOut children and reports the
// sum of their results.
type SkynetRange struct {
	Start int64
	Size  int64
}

// Children returns the ranges of a node's children, in order.
func (r SkynetRange) Children() []SkynetRange {
	children := make([]SkynetRange, SkynetFanOut)
	size := r.Size / SkynetFanOut
	for i := range children {
		children[i] = SkynetRange{Start: r.Start + int64(i)*size, Size: size}
	}
	return children
}

// BigRound is one round of a Big exchange. Each of the actors sends a Ping to
// a peer it picks at random, waits for the Pong, and repeats until it has
// done Rounds rounds. The Latch counts the Pongs, so it opens after
// BigActors*Rounds exchanges.
type BigRound struct{}

// NextPeer returns a peer for actor i in a Big exchange, never i itself. The
// choice is a cheap deterministic pseudo-random walk, so every framework sends
// to the same peers in the same order and the work is identical.
func NextPeer(i, round, actors int) int {
	peer := (i + 1 + (round*7919+i*104729)%(actors-1)) % actors
	if peer == i {
		peer = (peer + 1) % actors
	}
	return peer
}

// Share returns how many of n units go to the i-th of count recipients when
// the units are dealt round-robin: the first n % count recipients get one more.
func Share(n, i, count int) int {
	share := n / count
	if i < n%count {
		share++
	}
	return share
}

// MeasureHeap returns the live heap in bytes after a full collection.
func MeasureHeap() uint64 {
	runtime.GC()
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	return stats.HeapAlloc
}

// ReportHeapPerActor reports how much live heap each of n actors kept: the
// growth from before to after, divided by n.
func ReportHeapPerActor(b *testing.B, before, after uint64, n int) {
	reportHeapPer(b, before, after, n, BytesPerActor)
}

// ReportHeapPerGrain reports how much live heap each of n grains kept: the
// growth from before to after, divided by n.
func ReportHeapPerGrain(b *testing.B, before, after uint64, n int) {
	reportHeapPer(b, before, after, n, BytesPerGrain)
}

func reportHeapPer(b *testing.B, before, after uint64, n int, unit string) {
	if after < before {
		after = before
	}
	b.ReportMetric(float64(after-before)/float64(n), unit)
}

// Latch is closed once an actor has handled the expected number of messages.
// It lets a benchmark block until all the work it enqueued has been processed,
// instead of only measuring how fast messages are enqueued.
type Latch struct {
	target int64
	count  atomic.Int64
	done   chan struct{}
}

// NewLatch creates a Latch that opens after n calls to Count.
func NewLatch(n int) *Latch {
	return &Latch{target: int64(n), done: make(chan struct{})}
}

// Count records one handled message and reports whether the latch is now open.
func (l *Latch) Count() bool {
	switch l.count.Add(1) {
	case l.target:
		close(l.done)
		return true
	default:
		return false
	}
}

// Wait blocks until the latch opens, failing the benchmark after waitTimeout.
func (l *Latch) Wait(b *testing.B) {
	b.Helper()
	select {
	case <-l.done:
	case <-time.After(waitTimeout):
		b.Fatalf("timed out: handled %d of %d messages", l.count.Load(), l.target)
	}
}

// Signal notifies a benchmark that an actor's stop hook has run.
type Signal chan struct{}

// NewSignal creates a buffered Signal so the actor never blocks on it. A single
// Signal is reused across iterations because each one is waited on before the
// next actor is spawned.
func NewSignal() Signal {
	return make(Signal, 1)
}

// Fire notifies the waiter.
func (s Signal) Fire() {
	s <- struct{}{}
}

// Wait blocks until the signal fires, failing the benchmark after Timeout.
func (s Signal) Wait(b *testing.B) {
	b.Helper()
	select {
	case <-s:
	case <-time.After(Timeout):
		b.Fatal("timed out waiting for the actor to stop")
	}
}

var seq atomic.Uint64

// Name returns a unique actor name for frameworks that require one.
func Name(prefix string) string {
	return fmt.Sprintf("%s-%d", prefix, seq.Add(1))
}
