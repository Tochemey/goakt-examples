# GoAkt Reactive Stream Example

This example demonstrates every public API in GoAkt's [`stream`](https://github.com/tochemey/goakt) package through 18 self-contained pipelines.

The `stream` package provides **backpressure-aware, actor-based stream processing** inspired by [Reactive Streams](https://www.reactive-streams.org/). Pipelines are assembled from Sources, Flows, and Sinks, and nothing executes until `RunnableGraph.Run()` is called.

## Running

```bash
go run ./goakt-stream/
```

Press `Ctrl+C` to exit after all pipelines complete.

## Pipelines and Expected Results

### 1. Linear: Of → Map → Filter → ForEach

Doubles integers 1–10, keeps only values above 10, prints each.

**Expected:** prints `12, 14, 16, 18, 20`. Metrics: `in=10, out=5`.

### 2. Fold: Range → Map → Fold

Squares integers 1–10, then reduces to a sum.

**Expected:** `result=385` (1 + 4 + 9 + 16 + 25 + 36 + 49 + 64 + 81 + 100).

### 3. Channel: FromChannel → Map → Collect

Reads temperature floats from a Go channel and formats them as strings.

**Expected:** `items=[22.5°C, 23.1°C, 19.8°C, 25.4°C, 30.2°C]`.

### 4. Broadcast: Of → Broadcast → ForEach + Fold

Fans out `1..5` to two branches. Branch 1 prints each value, branch 2 sums them.

**Expected:** branch-1 prints `1, 2, 3, 4, 5`. Branch-2: `sum=15`.

### 5. Unfold + Scan: Fibonacci → running totals

Generates the first 10 Fibonacci numbers via Unfold (`0, 1, 1, 2, 3, 5, 8, 13, 21, 34`), then computes cumulative sums via Scan.

**Expected:** `items=[0, 1, 2, 4, 7, 12, 20, 33, 54, 88]`.

### 6. Deduplicate + Batch: dedup → batch(3)

Removes consecutive duplicate sensor readings from `20, 20, 21, 21, 21, 22, 23, 23, 24`, then groups into batches of 3.

**Expected:** after dedup `[20, 21, 22, 23, 24]` → batches `[[20, 21, 22], [23, 24]]`.

### 7. FlatMap: sentences → words

Splits sentences into individual words using FlatMap.

**Expected:** `items=[hello, world, GoAkt, streams, reactive, pipelines]`.

### 8. Flatten + Buffer: []int → individual ints

Unwraps `[]int` slices into individual elements, then buffers them through an async stage with backpressure.

**Expected:** `items=[1, 2, 3, 4, 5, 6]`.

### 9. Tick + Throttle + First: heartbeat capture

Emits ticks every 50ms, throttles to 1 per 200ms, captures only the first tick. First cancels upstream after receiving one element.

**Expected:** prints a single timestamp.

### 10. TryMap: resilient processing with Resume

Transforms integers ×10 but returns errors for multiples of 3. The `Resume` error strategy drops failed elements instead of failing the stream.

**Expected:** `items=[10, 20, 40, 50, 70, 80]` (3, 6, 9 skipped).

### 11. Parallel: unordered + ordered

**Part A — ParallelMap:** Squares `1..5` using 3 concurrent workers. Results arrive in completion order (non-deterministic), so they are sorted for display.

**Expected (sorted):** `[1, 4, 9, 16, 25]`.

**Part B — OrderedParallelMap → Chan:** Same computation but output preserves input order. Results are written to a Go channel via the Chan sink.

**Expected:** `[1, 4, 9, 16, 25]` (in order).

### 12. Merge + WithContext + Ignore

**Part A — Merge:** Combines two sources `[1,3,5]` and `[2,4,6]` into one (arrival order is non-deterministic), passes through a WithContext tracing boundary, then collects.

**Expected (sorted):** `[1, 2, 3, 4, 5, 6]`.

**Part B — Ignore:** Runs side-effect tasks through a Map flow. The Ignore sink discards all elements — useful when upstream side effects are the goal.

**Expected:** logs `tasks=[task-a, task-b, task-c]`.

### 13. Combine: zip names + ages

Zips two sources element-by-element using a combine function (zip semantics). Completes when either source is exhausted.

**Expected:** `items=[Alice (age 30), Bob (age 25), Charlie (age 35)]`.

### 14. Balance: round-robin distribution

Distributes `10, 20, 30, 40, 50, 60` across 2 branches. Each element goes to exactly one branch (round-robin with backpressure), unlike Broadcast where all branches receive every element.

**Expected:** 6 elements total split across two branches (exact distribution depends on demand timing). `total=6`.

### 15. Actor Integration: FromActor → Map → ToActor

A source actor implements the pull protocol (`PullRequest` → `PullResponse[string]`) to produce city names. The stream uppercases them via Map, then forwards each result to a sink actor via ToActor.

**Expected:** `items=[TOKYO, PARIS, NEW YORK, LONDON]`.

### 16. Graph: Order Processing (fan-out)

Three e-commerce orders are split into two parallel graph branches — validation and tax calculation — using the Graph builder.

```
orders → validate → validated-sink
       → tax      → tax-sink
```

**Expected — validated orders** (all 3 pass, they have items and positive totals):

| ID        | Customer | Total     |
|-----------|----------|-----------|
| `ORD-001` | Alice    | `$1059.97` |
| `ORD-002` | Bob      | `$149.99`  |
| `ORD-003` | Charlie  | `$538.96`  |

**Expected — tax calculations:**

| ID        | Subtotal   | Tax     | Total      | Region |
|-----------|------------|---------|------------|--------|
| `ORD-001` | `$1059.97` | `$84.80` | `$1144.77` | US 8%  |
| `ORD-002` | `$149.99`  | `$30.00` | `$179.99`  | EU 20% |
| `ORD-003` | `$538.96`  | `$43.12` | `$582.08`  | US 8%  |

### 17. Graph: Log Aggregation (fan-in)

Logs from 3 services are merged via `MergeInto`, filtered to WARN/ERROR, then collected. Sorted by service for deterministic display.

```
api-logs    ─┐
auth-logs   ─┼─ merged → severity-filter → alert-sink
payment-logs─┘
```

**Expected:** 5 alerts (3 INFO entries filtered out):

| Service   | Level   | Message                    |
|-----------|---------|----------------------------|
| `api`     | `ERROR` | handler panic: nil pointer |
| `api`     | `WARN`  | slow query: 2.3s           |
| `auth`    | `ERROR` | invalid token signature    |
| `payment` | `ERROR` | charge declined            |
| `payment` | `WARN`  | retry: gateway timeout     |

### 18. Graph: ETL Diamond (fan-out → merge → sink)

User records fan out to normalize and enrich branches, then merge back into a single sink.

```
users → normalize ─┐
      → enrich    ─┴─ merged → result-sink
```

**Expected:** 6 results (3 normalized + 3 enriched, interleaved in non-deterministic order):

- **Normalized:** trimmed names, lowercase emails, uppercase countries (`US`, `GB`, `DE`), parsed signup dates.
- **Enriched:** extracted email domains (`example.com`, `company.org`, `startup.io`), computed days since signup.
