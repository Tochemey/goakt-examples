/*
 * MIT License
 *
 * Copyright (c) 2022-2025 Arsene Tochemey Gandote
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

package benchmark

import (
	"context"
	"fmt"
	"runtime"
	"testing"
	"time"

	. "github.com/klauspost/cpuid/v2" //nolint
	require "github.com/stretchr/testify/require"
	actors "github.com/tochemey/goakt/v3/actor"
	"github.com/tochemey/goakt/v3/log"

	"github.com/tochemey/goakt-examples/v2/internal/benchpb"
)

func BenchmarkActor(b *testing.B) {
	b.Run("tell", func(b *testing.B) {
		ctx := context.TODO()

		// create the actor system
		actorSystem, _ := actors.NewActorSystem("bench",
			actors.WithLogger(log.DiscardLogger),
			actors.WithActorInitMaxRetries(1))

		// start the actor system
		_ = actorSystem.Start(ctx)

		// define the benchmark actor
		actor := &Benchmarker{}

		// create the actor ref
		pid, _ := actorSystem.Spawn(ctx, "test", actor)

		runParallel(b, func(pb *testing.PB) {
			for pb.Next() {
				// send a message to the actor
				_ = actors.Tell(ctx, pid, new(benchpb.BenchTell))
			}
		})

		_ = pid.Shutdown(ctx)
		_ = actorSystem.Stop(ctx)
	})
	b.Run("tell without passivation", func(b *testing.B) {
		ctx := context.TODO()

		// create the actor system
		actorSystem, _ := actors.NewActorSystem("bench",
			actors.WithLogger(log.DiscardLogger),
			actors.WithActorInitMaxRetries(1),
			actors.WithPassivationDisabled())

		// start the actor system
		_ = actorSystem.Start(ctx)

		// define the benchmark actor
		actor := &Benchmarker{}

		// create the actor ref
		pid, _ := actorSystem.Spawn(ctx, "test", actor)

		runParallel(b, func(pb *testing.PB) {
			for pb.Next() {
				// send a message to the actor
				_ = actors.Tell(ctx, pid, new(benchpb.BenchTell))
			}
		})

		_ = pid.Shutdown(ctx)
		_ = actorSystem.Stop(ctx)
	})
	b.Run("ask", func(b *testing.B) {
		ctx := context.TODO()
		// create the actor system
		actorSystem, _ := actors.NewActorSystem("bench",
			actors.WithLogger(log.DiscardLogger),
			actors.WithActorInitMaxRetries(1),
			actors.WithPassivation(5*time.Second))

		// start the actor system
		_ = actorSystem.Start(ctx)

		// define the benchmark actor
		actor := &Benchmarker{}

		// create the actor ref
		pid, _ := actorSystem.Spawn(ctx, "test", actor)
		runParallel(b, func(pb *testing.PB) {
			for pb.Next() {
				// send a message to the actor
				_, _ = actors.Ask(ctx, pid, new(benchpb.BenchRequest), receivingTimeout)
			}
		})

		_ = pid.Shutdown(ctx)
		_ = actorSystem.Stop(ctx)
	})
	b.Run("ask without passivation", func(b *testing.B) {
		ctx := context.TODO()
		// create the actor system
		actorSystem, _ := actors.NewActorSystem("bench",
			actors.WithLogger(log.DiscardLogger),
			actors.WithActorInitMaxRetries(1),
			actors.WithPassivationDisabled())

		// start the actor system
		_ = actorSystem.Start(ctx)

		// define the benchmark actor
		actor := &Benchmarker{}

		// create the actor ref
		pid, _ := actorSystem.Spawn(ctx, "test", actor)
		runParallel(b, func(pb *testing.PB) {
			for pb.Next() {
				// send a message to the actor
				_, _ = actors.Ask(ctx, pid, new(benchpb.BenchRequest), receivingTimeout)
			}
		})

		_ = pid.Shutdown(ctx)
		_ = actorSystem.Stop(ctx)
	})
}

func runParallel(b *testing.B, benchFn func(pb *testing.PB)) {
	b.ReportAllocs()
	b.ResetTimer()
	start := time.Now()
	b.RunParallel(benchFn)
	b.StopTimer()
	opsPerSec := float64(b.N) / time.Since(start).Seconds()
	b.ReportMetric(opsPerSec, "ops/s")
}

func TestBenchmark_BenchTell(t *testing.T) {
	ctx := context.TODO()

	actorsCount := 2000
	workersCount := 20
	duration := 10 * time.Second

	benchmark := NewBenchmark(actorsCount, workersCount, duration)
	require.NoError(t, benchmark.Start(ctx))

	fmt.Printf("Starting benchmark for (%v): num workers:(%d)\n", duration, workersCount)
	if err := benchmark.BenchTell(ctx); err != nil {
		t.Fatal(err)
	}

	fmt.Printf("Go Version: %s\n", runtime.Version())
	fmt.Printf("cpu: %s (Physical Cores: %d)\n", CPU.BrandName, CPU.PhysicalCores)
	fmt.Printf("Runtime CPUs: %d\n", runtime.NumCPU())
	fmt.Printf("Total actors spawned: (%d)\n", actorsCount)
	fmt.Printf("Total workers: (%d), total messages sent: (%d), total messages received: (%d) - duration: (%v)\n", workersCount, totalSent.Load(), totalRecv.Load(), duration)
	fmt.Printf("Messages per second: (%d)\n", totalRecv.Load()/int64(duration.Seconds()))
	t.Cleanup(func() {
		require.NoError(t, benchmark.Stop(ctx))
	})
}

func TestBenchmark_BenchAsk(t *testing.T) {
	ctx := context.TODO()

	actorsCount := 2000
	workersCount := 20
	duration := 10 * time.Second

	benchmark := NewBenchmark(actorsCount, workersCount, duration)
	require.NoError(t, benchmark.Start(ctx))

	fmt.Printf("Starting benchmark for (%v): num workers:(%d)\n", duration, workersCount)
	if err := benchmark.BenchAsk(ctx); err != nil {
		t.Fatal(err)
	}

	fmt.Printf("Go Version: %s\n", runtime.Version())
	fmt.Printf("cpu: %s (Physical Cores: %d)\n", CPU.BrandName, CPU.PhysicalCores)
	fmt.Printf("Runtime CPUs: %d\n", runtime.NumCPU())
	fmt.Printf("Total actors spawned: (%d)\n", actorsCount)
	fmt.Printf("Total workers: (%d), total messages sent: (%d), total messages received: (%d) - duration: (%v)\n", workersCount, totalSent.Load(), totalRecv.Load(), duration)
	fmt.Printf("Messages per second: (%d)\n", totalRecv.Load()/int64(duration.Seconds()))
	t.Cleanup(func() {
		require.NoError(t, benchmark.Stop(ctx))
	})
}
