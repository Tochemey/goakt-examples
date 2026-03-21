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

// Package main demonstrates every public API in the GoAkt
// [github.com/tochemey/goakt/v4/stream] package through
// self-contained, real-world-inspired pipelines.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"os"
	"os/signal"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/tochemey/goakt/v4/actor"
	"github.com/tochemey/goakt/v4/log"
	"github.com/tochemey/goakt/v4/stream"
)

func main() {
	ctx := context.Background()

	actorSystem, err := actor.NewActorSystem(
		"StreamDemo",
		actor.WithLogger(log.DiscardLogger),
		actor.WithActorInitMaxRetries(3),
	)
	if err != nil {
		slog.Error("failed to create actor system", "error", err)
		os.Exit(1)
	}

	if err := actorSystem.Start(ctx); err != nil {
		slog.Error("failed to start actor system", "error", err)
		os.Exit(1)
	}

	time.Sleep(1 * time.Second)

	// Fluent API pipelines.
	runLinearPipeline(ctx, actorSystem)
	runFoldPipeline(ctx, actorSystem)
	runChannelPipeline(ctx, actorSystem)
	runBroadcastPipeline(ctx, actorSystem)
	runUnfoldScanPipeline(ctx, actorSystem)
	runDedupBatchPipeline(ctx, actorSystem)
	runFlatMapPipeline(ctx, actorSystem)
	runFlattenBufferPipeline(ctx, actorSystem)
	runTickThrottleFirstPipeline(ctx, actorSystem)
	runTryMapPipeline(ctx, actorSystem)
	runParallelPipeline(ctx, actorSystem)
	runMergePipeline(ctx, actorSystem)
	runCombinePipeline(ctx, actorSystem)
	runBalancePipeline(ctx, actorSystem)
	runActorPipeline(ctx, actorSystem)

	// Graph builder pipelines.
	runOrderProcessingGraph(ctx, actorSystem)
	runLogAggregationGraph(ctx, actorSystem)
	runETLDiamondGraph(ctx, actorSystem)

	slog.Info("All pipelines completed. Press Ctrl+C to exit.")

	interruptSignal := make(chan os.Signal, 1)
	signal.Notify(interruptSignal, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	<-interruptSignal

	_ = actorSystem.Stop(ctx)
	os.Exit(0)
}

// ---------------------------------------------------------------------------
// 1. Linear — Of, Map, Filter, ForEach
// ---------------------------------------------------------------------------

// runLinearPipeline doubles each integer and keeps only values above 10.
//
// APIs: Of, Map, Filter, ForEach, From, Via, Metrics
func runLinearPipeline(ctx context.Context, sys actor.ActorSystem) {
	slog.Info("=== 1. Linear: Of → Map → Filter → ForEach ===")

	handle, err := stream.From(stream.Of(1, 2, 3, 4, 5, 6, 7, 8, 9, 10)).
		Via(stream.Map(func(n int) int { return n * 2 })).
		Via(stream.Filter(func(n int) bool { return n > 10 })).
		To(stream.ForEach(func(n int) {
			slog.Info("  received", "value", n)
		})).
		Run(ctx, sys)
	if err != nil {
		slog.Error("pipeline failed", "error", err)
		return
	}

	<-handle.Done()
	m := handle.Metrics()
	slog.Info("  metrics", "in", m.ElementsIn, "out", m.ElementsOut)
}

// ---------------------------------------------------------------------------
// 2. Fold — Range, Fold
// ---------------------------------------------------------------------------

// runFoldPipeline computes the sum of squares from 1 to 10.
//
// APIs: Range, Map, Fold, FoldResult.Value
func runFoldPipeline(ctx context.Context, sys actor.ActorSystem) {
	slog.Info("=== 2. Fold: Range → Map → Fold ===")

	squaredSrc := stream.Via(
		stream.Range(1, 11),
		stream.Map(func(n int64) int64 { return n * n }),
	)

	sumResult, sink := stream.Fold(int64(0), func(acc, elem int64) int64 {
		return acc + elem
	})

	handle, err := squaredSrc.To(sink).Run(ctx, sys)
	if err != nil {
		slog.Error("pipeline failed", "error", err)
		return
	}

	<-handle.Done()
	slog.Info("  sum of squares 1²..10²", "result", sumResult.Value())
}

// ---------------------------------------------------------------------------
// 3. Channel — FromChannel, Collect
// ---------------------------------------------------------------------------

// runChannelPipeline reads temperature values from a Go channel and formats them.
//
// APIs: FromChannel, Map, Collect, Collector.Items
func runChannelPipeline(ctx context.Context, sys actor.ActorSystem) {
	slog.Info("=== 3. Channel: FromChannel → Map → Collect ===")

	ch := make(chan float64, 5)
	go func() {
		defer close(ch)
		for _, r := range []float64{22.5, 23.1, 19.8, 25.4, 30.2} {
			ch <- r
		}
	}()

	formattedSrc := stream.Via(
		stream.FromChannel(ch),
		stream.Map(func(temp float64) string {
			return fmt.Sprintf("%.1f°C", temp)
		}),
	)

	collector, sink := stream.Collect[string]()
	handle, err := formattedSrc.To(sink).Run(ctx, sys)
	if err != nil {
		slog.Error("pipeline failed", "error", err)
		return
	}

	<-handle.Done()
	slog.Info("  collected", "items", collector.Items())
}

// ---------------------------------------------------------------------------
// 4. Broadcast — Broadcast, ForEach, Fold
// ---------------------------------------------------------------------------

// runBroadcastPipeline fans out a source to two branches: one prints, one sums.
//
// APIs: Broadcast, ForEach, Fold
func runBroadcastPipeline(ctx context.Context, sys actor.ActorSystem) {
	slog.Info("=== 4. Broadcast: Of → Broadcast → ForEach + Fold ===")

	branches := stream.Broadcast(stream.Of(1, 2, 3, 4, 5), 2)

	handle1, err := branches[0].
		To(stream.ForEach(func(n int) {
			slog.Info("  branch-1", "value", n)
		})).
		Run(ctx, sys)
	if err != nil {
		slog.Error("branch-1 failed", "error", err)
		return
	}

	sumResult, foldSink := stream.Fold(0, func(acc, elem int) int {
		return acc + elem
	})

	handle2, err := branches[1].To(foldSink).Run(ctx, sys)
	if err != nil {
		slog.Error("branch-2 failed", "error", err)
		return
	}

	<-handle1.Done()
	<-handle2.Done()
	slog.Info("  branch-2 sum", "result", sumResult.Value())
}

// ---------------------------------------------------------------------------
// 5. Unfold + Scan — Unfold, Scan, Collect
// ---------------------------------------------------------------------------

// runUnfoldScanPipeline generates the first 10 Fibonacci numbers using Unfold,
// then computes running totals using Scan.
//
// APIs: Unfold, Scan, Collect
func runUnfoldScanPipeline(ctx context.Context, sys actor.ActorSystem) {
	slog.Info("=== 5. Unfold + Scan: Fibonacci → running totals ===")

	// Unfold generates Fibonacci: 0, 1, 1, 2, 3, 5, 8, 13, 21, 34
	fibSrc := stream.Unfold(
		[3]int{0, 1, 0}, // seed: (a, b, count)
		func(s [3]int) ([3]int, int, bool) {
			a, b, count := s[0], s[1], s[2]
			return [3]int{b, a + b, count + 1}, a, count < 9
		},
	)

	// Scan accumulates running totals: 0, 1, 2, 4, 7, 12, 20, 33, 54, 88
	scanned := stream.Via(fibSrc, stream.Scan(0, func(acc, elem int) int {
		return acc + elem
	}))

	collector, sink := stream.Collect[int]()
	handle, err := scanned.To(sink).Run(ctx, sys)
	if err != nil {
		slog.Error("pipeline failed", "error", err)
		return
	}

	<-handle.Done()
	slog.Info("  running totals", "items", collector.Items())
}

// ---------------------------------------------------------------------------
// 6. Deduplicate + Batch — Deduplicate, Batch, Collect
// ---------------------------------------------------------------------------

// runDedupBatchPipeline removes consecutive duplicate sensor readings,
// then groups the remaining values into batches of 3.
//
// APIs: Deduplicate, Batch, Collect
func runDedupBatchPipeline(ctx context.Context, sys actor.ActorSystem) {
	slog.Info("=== 6. Deduplicate + Batch: dedup → batch(3) ===")

	// Sensor readings with consecutive duplicates.
	src := stream.Of(20, 20, 21, 21, 21, 22, 23, 23, 24)

	batched := stream.Via(
		stream.Via(src, stream.Deduplicate[int]()),
		stream.Batch[int](3, time.Second),
	)

	collector, sink := stream.Collect[[]int]()
	handle, err := batched.To(sink).Run(ctx, sys)
	if err != nil {
		slog.Error("pipeline failed", "error", err)
		return
	}

	<-handle.Done()
	slog.Info("  batches", "items", collector.Items())
}

// ---------------------------------------------------------------------------
// 7. FlatMap — FlatMap, Collect
// ---------------------------------------------------------------------------

// runFlatMapPipeline splits sentences into individual words.
//
// APIs: FlatMap, Collect
func runFlatMapPipeline(ctx context.Context, sys actor.ActorSystem) {
	slog.Info("=== 7. FlatMap: sentences → words ===")

	wordsSrc := stream.Via(
		stream.Of("hello world", "GoAkt streams", "reactive pipelines"),
		stream.FlatMap(func(sentence string) []string {
			return strings.Fields(sentence)
		}),
	)

	collector, sink := stream.Collect[string]()
	handle, err := wordsSrc.To(sink).Run(ctx, sys)
	if err != nil {
		slog.Error("pipeline failed", "error", err)
		return
	}

	<-handle.Done()
	slog.Info("  words", "items", collector.Items())
}

// ---------------------------------------------------------------------------
// 8. Flatten + Buffer — Flatten, Buffer, Collect
// ---------------------------------------------------------------------------

// runFlattenBufferPipeline unwraps slices into individual elements,
// then buffers them through an async stage.
//
// APIs: Flatten, Buffer, Collect, OverflowStrategy(BackpressureSource)
func runFlattenBufferPipeline(ctx context.Context, sys actor.ActorSystem) {
	slog.Info("=== 8. Flatten + Buffer: []int → individual ints ===")

	flattened := stream.Via(
		stream.Of([]int{1, 2, 3}, []int{4, 5}, []int{6}),
		stream.Flatten[int](),
	)

	buffered := stream.Via(flattened, stream.Buffer[int](64, stream.BackpressureSource))

	collector, sink := stream.Collect[int]()
	handle, err := buffered.To(sink).Run(ctx, sys)
	if err != nil {
		slog.Error("pipeline failed", "error", err)
		return
	}

	<-handle.Done()
	slog.Info("  flattened", "items", collector.Items())
}

// ---------------------------------------------------------------------------
// 9. Tick + Throttle + First — Tick, Throttle, First
// ---------------------------------------------------------------------------

// runTickThrottleFirstPipeline emits ticks every 50ms, throttles to 1 per 200ms,
// and captures only the first throttled tick.
//
// APIs: Tick, Throttle, First, FoldResult.Value, StreamHandle.Stop
func runTickThrottleFirstPipeline(ctx context.Context, sys actor.ActorSystem) {
	slog.Info("=== 9. Tick + Throttle + First: heartbeat capture ===")

	throttled := stream.Via(
		stream.Tick(50*time.Millisecond),
		stream.Throttle[time.Time](1, 200*time.Millisecond),
	)

	firstResult, sink := stream.First[time.Time]()
	handle, err := throttled.To(sink).Run(ctx, sys)
	if err != nil {
		slog.Error("pipeline failed", "error", err)
		return
	}

	<-handle.Done()
	slog.Info("  first tick", "time", firstResult.Value().Format(time.RFC3339Nano))
}

// ---------------------------------------------------------------------------
// 10. TryMap — TryMap, Resume error strategy, Collect
// ---------------------------------------------------------------------------

// runTryMapPipeline transforms values while skipping errors using the Resume strategy.
// Multiples of 3 produce errors and are dropped.
//
// APIs: TryMap, WithErrorStrategy(Resume), Collect
func runTryMapPipeline(ctx context.Context, sys actor.ActorSystem) {
	slog.Info("=== 10. TryMap: resilient processing with Resume ===")

	resilient := stream.Via(
		stream.Of(1, 2, 3, 4, 5, 6, 7, 8, 9),
		stream.TryMap(func(n int) (int, error) {
			if n%3 == 0 {
				return 0, fmt.Errorf("skip %d", n)
			}
			return n * 10, nil
		}).WithErrorStrategy(stream.Resume),
	)

	collector, sink := stream.Collect[int]()
	handle, err := resilient.To(sink).Run(ctx, sys)
	if err != nil {
		slog.Error("pipeline failed", "error", err)
		return
	}

	<-handle.Done()
	slog.Info("  results (3,6,9 skipped)", "items", collector.Items())
}

// ---------------------------------------------------------------------------
// 11. Parallel — ParallelMap, OrderedParallelMap, Chan
// ---------------------------------------------------------------------------

// runParallelPipeline demonstrates unordered and ordered parallel processing.
//
// APIs: ParallelMap, OrderedParallelMap, Chan, Collect
func runParallelPipeline(ctx context.Context, sys actor.ActorSystem) {
	slog.Info("=== 11. Parallel: unordered + ordered ===")

	// Part A: ParallelMap (unordered) — results may arrive in any order.
	unordered := stream.Via(
		stream.Range(1, 6),
		stream.ParallelMap(3, func(n int64) int64 { return n * n }),
	)

	col1, sink1 := stream.Collect[int64]()
	h1, err := unordered.To(sink1).Run(ctx, sys)
	if err != nil {
		slog.Error("unordered failed", "error", err)
		return
	}

	<-h1.Done()
	items := col1.Items()
	sorted := make([]int64, len(items))
	copy(sorted, items)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	slog.Info("  ParallelMap (sorted)", "items", sorted)

	// Part B: OrderedParallelMap → Chan sink — results preserve input order.
	ch := make(chan int64, 10)
	ordered := stream.Via(
		stream.Range(1, 6),
		stream.OrderedParallelMap(3, func(n int64) int64 { return n * n }),
	)

	h2, err := ordered.To(stream.Chan(ch)).Run(ctx, sys)
	if err != nil {
		slog.Error("ordered failed", "error", err)
		return
	}

	<-h2.Done()
	var chanResults []int64
	for v := range ch {
		chanResults = append(chanResults, v)
	}
	slog.Info("  OrderedParallelMap → Chan", "items", chanResults)
}

// ---------------------------------------------------------------------------
// 12. Merge + WithContext + Ignore — Merge, WithContext, Ignore
// ---------------------------------------------------------------------------

// runMergePipeline merges two sources and collects sorted results.
// Also demonstrates the Ignore sink for side-effect-only pipelines.
//
// APIs: Merge, WithContext, Collect, Ignore
func runMergePipeline(ctx context.Context, sys actor.ActorSystem) {
	slog.Info("=== 12. Merge + WithContext + Ignore ===")

	// Part A: Merge two sources → WithContext → Collect.
	merged := stream.Via(
		stream.Merge(
			stream.Of(1, 3, 5),
			stream.Of(2, 4, 6),
		),
		stream.WithContext[int]("pipeline", "merge-demo"),
	)

	collector, sink := stream.Collect[int]()
	h1, err := merged.To(sink).Run(ctx, sys)
	if err != nil {
		slog.Error("merge failed", "error", err)
		return
	}

	<-h1.Done()
	items := collector.Items()
	sort.Ints(items)
	slog.Info("  merged (sorted)", "items", items)

	// Part B: Side-effect-only pipeline using Ignore sink.
	var processed []string
	var mu sync.Mutex
	h2, err := stream.Via(
		stream.Of("task-a", "task-b", "task-c"),
		stream.Map(func(task string) string {
			mu.Lock()
			processed = append(processed, task)
			mu.Unlock()
			return task
		}),
	).To(stream.Ignore[string]()).Run(ctx, sys)
	if err != nil {
		slog.Error("ignore pipeline failed", "error", err)
		return
	}

	<-h2.Done()
	slog.Info("  Ignore sink side-effects executed", "tasks", processed)
}

// ---------------------------------------------------------------------------
// 13. Combine — Combine, Collect
// ---------------------------------------------------------------------------

// runCombinePipeline zips names with ages into formatted strings.
//
// APIs: Combine, Collect
func runCombinePipeline(ctx context.Context, sys actor.ActorSystem) {
	slog.Info("=== 13. Combine: zip names + ages ===")

	combined := stream.Combine(
		stream.Of("Alice", "Bob", "Charlie"),
		stream.Of(30, 25, 35),
		func(name string, age int) string {
			return fmt.Sprintf("%s (age %d)", name, age)
		},
	)

	collector, sink := stream.Collect[string]()
	handle, err := combined.To(sink).Run(ctx, sys)
	if err != nil {
		slog.Error("pipeline failed", "error", err)
		return
	}

	<-handle.Done()
	slog.Info("  combined", "items", collector.Items())
}

// ---------------------------------------------------------------------------
// 14. Balance — Balance, ForEach
// ---------------------------------------------------------------------------

// runBalancePipeline distributes elements round-robin across 2 worker branches.
// Each element goes to exactly one branch (unlike Broadcast where all get every element).
//
// APIs: Balance, ForEach, Collect
func runBalancePipeline(ctx context.Context, sys actor.ActorSystem) {
	slog.Info("=== 14. Balance: round-robin distribution ===")

	branches := stream.Balance(stream.Of(10, 20, 30, 40, 50, 60), 2)

	col0, sink0 := stream.Collect[int]()
	h0, err := branches[0].To(sink0).Run(ctx, sys)
	if err != nil {
		slog.Error("balance branch-0 failed", "error", err)
		return
	}

	col1, sink1 := stream.Collect[int]()
	h1, err := branches[1].To(sink1).Run(ctx, sys)
	if err != nil {
		slog.Error("balance branch-1 failed", "error", err)
		return
	}

	<-h0.Done()
	<-h1.Done()
	slog.Info("  branch-0", "items", col0.Items())
	slog.Info("  branch-1", "items", col1.Items())
	slog.Info("  total distributed", "count", len(col0.Items())+len(col1.Items()))
}

// ---------------------------------------------------------------------------
// 15. Actor Integration — FromActor, ToActor
// ---------------------------------------------------------------------------

// runActorPipeline pulls data from a source actor and pushes results to a sink actor.
//
// APIs: FromActor, Map, ToActor
func runActorPipeline(ctx context.Context, sys actor.ActorSystem) {
	slog.Info("=== 15. Actor Integration: FromActor → Map → ToActor ===")

	// Spawn the source actor that implements the pull protocol.
	sourcePID, err := sys.Spawn(ctx, "city-source", &CitySourceActor{
		cities: []string{"Tokyo", "Paris", "New York", "London"},
	})
	if err != nil {
		slog.Error("failed to spawn source actor", "error", err)
		return
	}

	// Spawn a sink actor that collects received messages.
	resultCh := make(chan string, 10)
	sinkPID, err := sys.Spawn(ctx, "city-sink", &ChannelSinkActor{ch: resultCh})
	if err != nil {
		slog.Error("failed to spawn sink actor", "error", err)
		return
	}

	// FromActor → Map(uppercase) → ToActor.
	pipeline := stream.Via(
		stream.FromActor[string](sourcePID),
		stream.Map(func(s string) string { return strings.ToUpper(s) }),
	)

	handle, err := pipeline.To(stream.ToActor[string](sinkPID)).Run(ctx, sys)
	if err != nil {
		slog.Error("actor pipeline failed", "error", err)
		return
	}

	<-handle.Done()

	// Give the sink actor a moment to process, then drain the channel.
	time.Sleep(200 * time.Millisecond)
	close(resultCh)

	var results []string
	for v := range resultCh {
		results = append(results, v)
	}
	slog.Info("  actor pipeline results", "items", results)
}

// CitySourceActor implements the stream pull protocol for [stream.FromActor].
// It responds to [stream.PullRequest] with batches of city names.
type CitySourceActor struct {
	cities []string
	pos    int
}

var _ actor.Actor = (*CitySourceActor)(nil)

func (a *CitySourceActor) PreStart(*actor.Context) error { return nil }
func (a *CitySourceActor) PostStop(*actor.Context) error { return nil }
func (a *CitySourceActor) Receive(ctx *actor.ReceiveContext) {
	switch msg := ctx.Message().(type) {
	case *actor.PostStart:
	case *stream.PullRequest:
		remaining := a.cities[a.pos:]
		take := int(msg.N)
		if take > len(remaining) {
			take = len(remaining)
		}
		batch := remaining[:take]
		a.pos += take
		ctx.Response(&stream.PullResponse[string]{Elements: batch})
	default:
		ctx.Unhandled()
	}
}

// ChannelSinkActor receives string messages and writes them to a channel.
// Used as the target for [stream.ToActor].
type ChannelSinkActor struct {
	ch chan<- string
}

var _ actor.Actor = (*ChannelSinkActor)(nil)

func (a *ChannelSinkActor) PreStart(*actor.Context) error { return nil }
func (a *ChannelSinkActor) PostStop(*actor.Context) error { return nil }
func (a *ChannelSinkActor) Receive(ctx *actor.ReceiveContext) {
	switch msg := ctx.Message().(type) {
	case *actor.PostStart:
	case string:
		a.ch <- msg
	default:
		ctx.Unhandled()
	}
}

// ---------------------------------------------------------------------------
// 16. Order Processing — Graph fan-out
// ---------------------------------------------------------------------------

// Order represents an e-commerce order.
type Order struct {
	ID       string
	Customer string
	Items    []OrderItem
	Region   string
}

// OrderItem is a line item in an order.
type OrderItem struct {
	Product  string
	Quantity int
	Price    float64
}

// Total returns the order subtotal.
func (o Order) Total() float64 {
	var sum float64
	for _, item := range o.Items {
		sum += float64(item.Quantity) * item.Price
	}
	return sum
}

// runOrderProcessingGraph splits orders into validation and tax branches.
//
// APIs: NewGraph, AddSource, AddFlow, AddSink, Build, Filter (graph), Map (graph), Collect (graph)
func runOrderProcessingGraph(ctx context.Context, sys actor.ActorSystem) {
	slog.Info("=== 16. Graph: Order Processing (fan-out) ===")

	orders := []any{
		Order{ID: "ORD-001", Customer: "Alice", Region: "US", Items: []OrderItem{
			{Product: "Laptop", Quantity: 1, Price: 999.99},
			{Product: "Mouse", Quantity: 2, Price: 29.99},
		}},
		Order{ID: "ORD-002", Customer: "Bob", Region: "EU", Items: []OrderItem{
			{Product: "Keyboard", Quantity: 1, Price: 149.99},
		}},
		Order{ID: "ORD-003", Customer: "Charlie", Region: "US", Items: []OrderItem{
			{Product: "Monitor", Quantity: 1, Price: 499.99},
			{Product: "Cable", Quantity: 3, Price: 12.99},
		}},
	}

	validate := stream.Filter(func(v any) bool {
		o := v.(Order)
		return len(o.Items) > 0 && o.Total() > 0
	})

	tax := stream.Map(func(v any) any {
		o := v.(Order)
		rate := 0.0
		switch o.Region {
		case "US":
			rate = 0.08
		case "EU":
			rate = 0.20
		}
		subtotal := o.Total()
		return fmt.Sprintf("%s: subtotal=$%.2f tax=$%.2f total=$%.2f (%s)",
			o.ID, subtotal, subtotal*rate, subtotal*(1+rate), o.Region)
	})

	validatedCol, validatedSink := stream.Collect[any]()
	taxCol, taxSink := stream.Collect[any]()

	rg, err := stream.NewGraph().
		AddSource("orders", stream.Of(orders...)).
		AddFlow("validate", validate, "orders").
		AddFlow("tax", tax, "orders").
		AddSink("validated-sink", validatedSink, "validate").
		AddSink("tax-sink", taxSink, "tax").
		Build()
	if err != nil {
		slog.Error("graph build failed", "error", err)
		return
	}

	handle, err := rg.Run(ctx, sys)
	if err != nil {
		slog.Error("graph run failed", "error", err)
		return
	}

	<-handle.Done()

	for _, v := range validatedCol.Items() {
		o := v.(Order)
		slog.Info("  valid", "id", o.ID, "customer", o.Customer, "total", fmt.Sprintf("$%.2f", o.Total()))
	}
	for _, t := range taxCol.Items() {
		slog.Info("  tax", "detail", t)
	}
}

// ---------------------------------------------------------------------------
// 17. Log Aggregation — Graph fan-in (MergeInto)
// ---------------------------------------------------------------------------

// LogEntry is a structured log record.
type LogEntry struct {
	Service string
	Level   string
	Message string
}

// runLogAggregationGraph merges logs from 3 services and filters to WARN/ERROR.
//
// APIs: NewGraph, AddSource, MergeInto, AddFlow, AddSink, Build
func runLogAggregationGraph(ctx context.Context, sys actor.ActorSystem) {
	slog.Info("=== 17. Graph: Log Aggregation (fan-in) ===")

	apiLogs := []any{
		LogEntry{Service: "api", Level: "INFO", Message: "request received"},
		LogEntry{Service: "api", Level: "ERROR", Message: "handler panic: nil pointer"},
		LogEntry{Service: "api", Level: "WARN", Message: "slow query: 2.3s"},
	}
	authLogs := []any{
		LogEntry{Service: "auth", Level: "INFO", Message: "user login"},
		LogEntry{Service: "auth", Level: "ERROR", Message: "invalid token signature"},
	}
	paymentLogs := []any{
		LogEntry{Service: "payment", Level: "INFO", Message: "charge succeeded"},
		LogEntry{Service: "payment", Level: "WARN", Message: "retry: gateway timeout"},
		LogEntry{Service: "payment", Level: "ERROR", Message: "charge declined"},
	}

	severityFilter := stream.Filter(func(v any) bool {
		e := v.(LogEntry)
		return e.Level == "WARN" || e.Level == "ERROR"
	})

	alertCol, alertSink := stream.Collect[any]()

	rg, err := stream.NewGraph().
		AddSource("api-logs", stream.Of(apiLogs...)).
		AddSource("auth-logs", stream.Of(authLogs...)).
		AddSource("payment-logs", stream.Of(paymentLogs...)).
		MergeInto("merged", "api-logs", "auth-logs", "payment-logs").
		AddFlow("severity-filter", severityFilter, "merged").
		AddSink("alert-sink", alertSink, "severity-filter").
		Build()
	if err != nil {
		slog.Error("graph build failed", "error", err)
		return
	}

	handle, err := rg.Run(ctx, sys)
	if err != nil {
		slog.Error("graph run failed", "error", err)
		return
	}

	<-handle.Done()

	alerts := alertCol.Items()
	sort.Slice(alerts, func(i, j int) bool {
		a, b := alerts[i].(LogEntry), alerts[j].(LogEntry)
		if a.Service != b.Service {
			return a.Service < b.Service
		}
		return a.Level < b.Level
	})

	slog.Info("  alerts collected", "count", len(alerts))
	for _, a := range alerts {
		e := a.(LogEntry)
		slog.Warn("  alert", "service", e.Service, "level", e.Level, "message", e.Message)
	}
}

// ---------------------------------------------------------------------------
// 18. ETL Diamond — Graph fan-out → merge → sink
// ---------------------------------------------------------------------------

// UserRecord is a raw user record.
type UserRecord struct {
	Name    string
	Email   string
	Country string
	SignUp  string // "2006-01-02"
}

// NormalizedUser is the cleaned version of a UserRecord.
type NormalizedUser struct {
	Name    string
	Email   string
	Country string
	SignUp  time.Time
}

// EnrichedUser adds derived fields to a UserRecord.
type EnrichedUser struct {
	Name      string
	Email     string
	Domain    string
	Country   string
	DaysSince int
}

// runETLDiamondGraph fans out user records to normalize and enrich branches,
// then merges them back into a single sink.
//
// APIs: NewGraph, AddSource, AddFlow, MergeInto, AddSink, Build (diamond topology)
func runETLDiamondGraph(ctx context.Context, sys actor.ActorSystem) {
	slog.Info("=== 18. Graph: ETL Diamond (fan-out → merge → sink) ===")

	users := []any{
		UserRecord{Name: "  Alice Johnson ", Email: "alice@example.com", Country: "us", SignUp: "2024-01-15"},
		UserRecord{Name: "Bob Smith", Email: "bob@company.org", Country: "gb", SignUp: "2023-06-20"},
		UserRecord{Name: " Charlie Lee  ", Email: "charlie@startup.io", Country: "de", SignUp: "2025-03-01"},
	}

	normalize := stream.Map(func(v any) any {
		r := v.(UserRecord)
		t, _ := time.Parse("2006-01-02", r.SignUp)
		return NormalizedUser{
			Name:    strings.TrimSpace(r.Name),
			Email:   strings.ToLower(strings.TrimSpace(r.Email)),
			Country: strings.ToUpper(strings.TrimSpace(r.Country)),
			SignUp:  t,
		}
	})

	enrich := stream.Map(func(v any) any {
		r := v.(UserRecord)
		domain := "unknown"
		if parts := strings.SplitN(r.Email, "@", 2); len(parts) == 2 {
			domain = parts[1]
		}
		t, _ := time.Parse("2006-01-02", r.SignUp)
		return EnrichedUser{
			Name:      strings.TrimSpace(r.Name),
			Email:     r.Email,
			Domain:    domain,
			Country:   strings.ToUpper(r.Country),
			DaysSince: int(math.Floor(time.Since(t).Hours() / 24)),
		}
	})

	resultCol, resultSink := stream.Collect[any]()

	rg, err := stream.NewGraph().
		AddSource("users", stream.Of(users...)).
		AddFlow("normalize", normalize, "users").
		AddFlow("enrich", enrich, "users").
		MergeInto("merged", "normalize", "enrich").
		AddSink("result-sink", resultSink, "merged").
		Build()
	if err != nil {
		slog.Error("graph build failed", "error", err)
		return
	}

	handle, err := rg.Run(ctx, sys)
	if err != nil {
		slog.Error("graph run failed", "error", err)
		return
	}

	<-handle.Done()
	slog.Info("  ETL results", "count", len(resultCol.Items()))

	for _, r := range resultCol.Items() {
		switch v := r.(type) {
		case NormalizedUser:
			slog.Info("  normalized", "name", v.Name, "email", v.Email,
				"country", v.Country, "signup", v.SignUp.Format("2006-01-02"))
		case EnrichedUser:
			slog.Info("  enriched", "name", v.Name, "domain", v.Domain,
				"country", v.Country, "days_since_signup", v.DaysSince)
		}
	}
}
