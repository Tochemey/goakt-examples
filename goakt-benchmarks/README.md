# GoAkt Benchmarks

Benchmarks that compare [GoAkt](https://github.com/Tochemey/goakt) with other Go actor frameworks:

| Package                      | Framework                                                 |
|------------------------------|-----------------------------------------------------------|
| [goakt](./goakt)             | [GoAkt](https://github.com/Tochemey/goakt) v4             |
| [ergo](./ergo)               | [Ergo Framework](https://github.com/ergo-services/ergo) v3 |
| [protoactor](./protoactor)   | [Proto.Actor Go](https://github.com/asynkron/protoactor-go) |
| [hollywood](./hollywood)     | [Hollywood](https://github.com/anthdm/hollywood)          |

This directory is its own Go module, with its own `go.mod`. The competing frameworks and their dependencies stay out of the root examples module, and every framework is pinned to an exact version, so results can be reproduced.

## Scenarios

Every framework package implements the same thirteen benchmarks under the same names, with the same sizes from the shared package, so benchstat can line them up.

The first six measure one operation each, driven from outside the system or between two actors:

| Benchmark            | What it measures                                                                                          |
|----------------------|-----------------------------------------------------------------------------------------------------------|
| `BenchmarkTell`         | Fire-and-forget messages from outside the system to one actor, until the actor has handled all of them. |
| `BenchmarkTellParallel` | Same as `Tell`, with `GOMAXPROCS` goroutines sending to the same actor at once (mailbox contention).    |
| `BenchmarkAsk`          | Request/response round trips from outside the system to one actor, one at a time.                       |
| `BenchmarkAskParallel`  | Same as `Ask`, with `GOMAXPROCS` goroutines making requests at once.                                    |
| `BenchmarkPingPong`     | Round trips between two actors, both inside the system, with each actor replying to the sender.          |
| `BenchmarkSpawnStop`    | Spawning an actor, stopping it, and waiting until its stop hook has run.                                 |

The other seven are the scenarios actor frameworks are usually compared on, taken from the sources named, at those sources' sizes:

| Benchmark                   | Origin | What it measures |
|-----------------------------|--------|------------------|
| `BenchmarkPingPongParallel`    | Savina PingPong run as one pair per processor | `GOMAXPROCS` independent pinger/ponger pairs, each with one message in flight: a ping, its pong, the next ping. Total messages across all pairs. |
| `BenchmarkPingPongPipelined`   | Akka's PingPong, which batches; GoAkt's own `BenchmarkTellPingPong` | The same pairs, each pinger keeping 100 messages in flight and sending the next 100 once all their pongs are back. Total messages across all pairs. |
| `BenchmarkThreadRing`          | Savina ThreadRing, the Computer Language Benchmarks Game thread-ring | One token passed around a ring of 100 actors, one hop per message, until it has made `b.N` hops. Message latency through a long chain of actors, with only one actor runnable at a time. |
| `BenchmarkForkJoinThroughput`  | Savina Fork Join (throughput) | One sender spreading `b.N` messages round-robin over 60 actors, until every actor has handled its share. Scheduling many runnable actors. |
| `BenchmarkBig`                 | Savina Big, from BenchErl | 120 actors each sending a Ping to a peer and answering every Ping with a Pong, for `b.N / 120` rounds each. Many-to-many contention. |
| `BenchmarkSkynet`              | Skynet (github.com/atemerev/skynet) | A root actor creating 10 children, each creating 10, down to a million leaves; every leaf reports its number, every node sums its children and stops. Creating and stopping 1,111,111 actors with a message tree over them. Reported as actors per second. |
| `BenchmarkMemoryPerActor`      | Ergo's memusage; Akka's actors-per-gigabyte figure | The live heap after a full collection, divided by 50,000 idle actors spawned into the system. Reported in bytes per actor and shown in its own table. |

The shared workload lives in [internal/bench](./internal/bench). It defines the messages, the sizes above, a `Latch` that opens once the expected number of actors or messages has been counted, a `Signal` that an actor fires from its stop hook, and the Skynet ranges and total.

### Three ping-pongs, three questions

The ping-pong benchmarks differ in one parameter, how many messages are in flight, and that parameter decides what the number means:

| Benchmark | Pairs | In flight per pair | What the number is |
|---|---|---|---|
| `PingPong` | 1 | 1 | The round-trip latency between two actors when nothing else runs: how fast the framework hands the turn from one idle actor to the other and back. |
| `PingPongParallel` | `GOMAXPROCS` | 1 | The same hand-off with every core busy. Each message still wakes an idle actor, so this is a scheduling number: how cheaply the framework makes an actor runnable and gets a worker to it. |
| `PingPongPipelined` | `GOMAXPROCS` | 100 | Sustained throughput. One wake-up carries a hundred messages, so this is a mailbox number: how fast a producer can fill a mailbox while its consumer drains it. |

A framework's pipelined figure is several times its one-in-flight figure, for every framework, and the gap is the cost of waking an actor per message. Quote the one-in-flight figures when the question is latency or scheduling, and the pipelined one when it is how many messages per second a busy system moves. Never compare a pipelined figure from one framework with a one-in-flight figure from another. GoAkt's own `BenchmarkTellPingPong` is the pipelined shape, at 100 and 1,000 in flight, and its numbers are comparable with `PingPongPipelined` here.

## Methodology

- **Same work everywhere.** Each package uses the same message types: pointers to empty structs from `internal/bench`. Any allocations reported therefore come from the framework, not from the payload.
- **End-to-end timing.** The `Tell` and `PingPong` benchmarks stop the clock only after the receiving actor has handled every message, so a framework can't look fast by queueing quickly and processing later. `SpawnStop` waits for the actor's stop hook (`PostStop`, `Terminate`, `*actor.Stopped`, `actor.Stopped`).
- **Throughput.** Every benchmark also reports a rate over the measured time. `msgs/s` counts the messages handled by actors: one per `Tell`, one request per `Ask`, and two per `PingPong` round trip (the ping and the pong). `actors/s` counts actors spawned and stopped. In the parallel benchmarks, the rate is the total across all goroutines.
- **Default settings.** Every framework runs locally with its default mailbox and dispatcher. Logging is turned off, and networking is turned off where the framework has it. One actor system per package is started in `TestMain`, outside the measurements.
- **Framework-native idioms.** Each actor uses the framework's own reply mechanism: `ReceiveContext.Response` / `Tell(Sender())` in GoAkt, `HandleCall` / `Send(from)` in Ergo, `Context.Respond` in Proto.Actor and Hollywood.
- **Actor names.** GoAkt requires a unique name for every actor, so `SpawnStop` and `Skynet` build one per actor with `bench.Name`, inside the timed loop. The other frameworks generate their identifiers internally, so that cost is in their numbers too.
- **ThreadRing passes one token.** The token is a single message whose hop count is decremented in place by each actor before it is forwarded. Only one actor holds it at a time, so no synchronization is needed, and no framework allocates a message per hop.
- **Big picks peers deterministically.** `bench.NextPeer` is a fixed pseudo-random walk over the actor indexes, so every framework sends the same messages to the same peers in the same order.
- **Skynet reports to the parent.** Each node learns its parent from the sender of its range message, so the frameworks' own sender tracking is used: `Sender()` in GoAkt and Hollywood, `from` in Ergo, `Request` and `Sender()` in Proto.Actor. A node stops itself once it has reported.
- **Completion is signalled once per actor.** In the scenarios with many actors (`PingPongParallel`, `PingPongPipelined`, `ForkJoinThroughput`, `Big`, `ThreadRing`, `Skynet`) each actor counts its own messages and touches the shared latch once, when its share is done. A shared counter incremented per message would put a contended cache line into every framework's hot path and compress the differences between them; with it, GoAkt's pipelined ping-pong measured 35M msgs/s instead of 83M.
- **Skynet and MemoryPerActor are expensive per iteration.** One Skynet iteration creates over a million actors and one MemoryPerActor iteration spawns 50,000, so with `BENCHTIME=1s` each runs once per round. Hollywood's Skynet peaks at about 4 GB of resident memory, because every actor owns a 1,024-slot inbox; the other three stay under 1 GB.

Results depend on the machine, Go version and load, so publish them along with the hardware they ran on. Run with `COUNT` of at least 10, so that benchstat can report confidence intervals.

## Results

These results come from one run of `make bench COUNT=10 BENCHTIME=1s` on 2026-10-30:

| | |
|---|---|
| Machine | Apple M1, 8 cores, 16 GB, macOS 27.0.1 |
| Go | 1.27.3, `GOMAXPROCS=8` |
| GoAkt | v4.6.5 |
| Ergo | v3 (`ergo.services/ergo` v1.999.330) |
| Proto.Actor | `v0.0.0-20260118094027-288962e52f3f` |
| Hollywood | v1.0.5 |

Each cell is the median of 10 runs. Throughput is higher-is-better; the other tables are lower-is-better. The best value in each row is in bold, and the factor in parentheses is relative to GoAkt. The `-8` suffix is `GOMAXPROCS`. `Skynet` and `MemoryPerActor` ran once per round, so their medians are over 10 iterations.

### Throughput

Higher is better.

| Benchmark | goakt | ergo | protoactor | hollywood |
|---|---:|---:|---:|---:|
| Tell-8 | **17.29M msgs/s** | 5.26M msgs/s (0.30×) | 4.05M msgs/s (0.23×) | 11.84M msgs/s (0.68×) |
| TellParallel-8 | 6.01M msgs/s | 4.90M msgs/s (0.81×) | **6.10M msgs/s (1.02×)** | 5.45M msgs/s (0.91×) |
| Ask-8 | **1.37M msgs/s** | 283K msgs/s (0.21×) | 337K msgs/s (0.25×) | 301K msgs/s (0.22×) |
| AskParallel-8 | **2.21M msgs/s** | 1.20M msgs/s (0.54×) | 1.32M msgs/s (0.60×) | 648K msgs/s (0.29×) |
| PingPong-8 | 2.33M msgs/s | 302K msgs/s (0.13×) | 598K msgs/s (0.26×) | **3.62M msgs/s (1.55×)** |
| ThreadRing-8 | 2.38M msgs/s | 296K msgs/s (0.12×) | 604K msgs/s (0.25×) | **3.35M msgs/s (1.41×)** |
| Skynet-8 | 144K actors/s | 499K actors/s (3.46×) | **867K actors/s (6.01×)** | 73K actors/s (0.51×) |
| SpawnStop-8 | 121K actors/s | 192K actors/s (1.58×) | **297K actors/s (2.45×)** | 113K actors/s (0.93×) |
| PingPongParallel-8 | **23.79M msgs/s** | 1.80M msgs/s (0.08×) | 3.20M msgs/s (0.13×) | 7.25M msgs/s (0.30×) |
| PingPongPipelined-8 | **83.46M msgs/s** | 13.34M msgs/s (0.16×) | 45.19M msgs/s (0.54×) | 8.09M msgs/s (0.10×) |
| ForkJoinThroughput-8 | 3.84M msgs/s | 2.07M msgs/s (0.54×) | **5.65M msgs/s (1.47×)** | 4.90M msgs/s (1.28×) |
| Big-8 | **17.01M msgs/s** | 2.38M msgs/s (0.14×) | 4.30M msgs/s (0.25×) | 6.86M msgs/s (0.40×) |

### Time per operation

Lower is better.

| Benchmark | goakt | ergo | protoactor | hollywood |
|---|---:|---:|---:|---:|
| Tell-8 | **57.9 ns** | 190.0 ns (3.28×) | 247.2 ns (4.27×) | 84.5 ns (1.46×) |
| TellParallel-8 | 166.3 ns | 204.2 ns (1.23×) | **163.8 ns (0.98×)** | 183.4 ns (1.10×) |
| Ask-8 | **729.5 ns** | 3.53 µs (4.84×) | 2.96 µs (4.06×) | 3.32 µs (4.55×) |
| AskParallel-8 | **453.4 ns** | 832.5 ns (1.84×) | 759.4 ns (1.67×) | 1.54 µs (3.40×) |
| PingPong-8 | 859.0 ns | 6.62 µs (7.71×) | 3.35 µs (3.90×) | **552.8 ns (0.64×)** |
| ThreadRing-8 | 420.1 ns | 3.38 µs (8.05×) | 1.66 µs (3.94×) | **298.1 ns (0.71×)** |
| Skynet-8 | 7707.33 ms | 2226.74 ms (0.29×) | **1281.53 ms (0.17×)** | 15139.83 ms (1.96×) |
| SpawnStop-8 | 8.24 µs | 5.22 µs (0.63×) | **3.37 µs (0.41×)** | 8.85 µs (1.07×) |
| PingPongParallel-8 | **84.1 ns** | 1.11 µs (13.22×) | 625.8 ns (7.44×) | 276.1 ns (3.28×) |
| PingPongPipelined-8 | **24.0 ns** | 149.9 ns (6.25×) | 44.3 ns (1.85×) | 247.2 ns (10.32×) |
| ForkJoinThroughput-8 | 260.1 ns | 483.5 ns (1.86×) | **176.9 ns (0.68×)** | 203.8 ns (0.78×) |
| Big-8 | **117.6 ns** | 841.5 ns (7.16×) | 465.5 ns (3.96×) | 291.6 ns (2.48×) |

### Bytes allocated per operation

Lower is better.

| Benchmark | goakt | ergo | protoactor | hollywood |
|---|---:|---:|---:|---:|
| Tell-8 | **0 B** | 90 B | 9 B | 26 B |
| TellParallel-8 | **22 B** | 172 B (7.98×) | 46 B (2.16×) | 25 B (1.16×) |
| Ask-8 | **128 B** | 168 B (1.31×) | 512 B (4.00×) | 703 B (5.49×) |
| AskParallel-8 | **128 B** | 194 B (1.52×) | 497 B (3.88×) | 692 B (5.41×) |
| PingPong-8 | **0 B** | 208 B | 64 B | 79 B |
| ThreadRing-8 | **0 B** | 72 B | 16 B | 40 B |
| Skynet-8 | 6068303788 B | 4600226440 B (0.76×) | **1188861212 B (0.20×)** | 32129133704 B (5.29×) |
| SpawnStop-8 | 3208 B | 4324 B (1.35×) | **1000 B (0.31×)** | 28736 B (8.96×) |
| PingPongParallel-8 | **0 B** | 176 B | 64 B | 79 B |
| PingPongPipelined-8 | **0 B** | 147 B | 32 B | 54 B |
| ForkJoinThroughput-8 | **0 B** | 104 B | 15 B | 39 B |
| Big-8 | **0 B** | 133 B | 53 B | 70 B |

### Allocations per operation

Lower is better.

| Benchmark | goakt | ergo | protoactor | hollywood |
|---|---:|---:|---:|---:|
| Tell-8 | **0** | 3 | **0** | **0** |
| TellParallel-8 | **0** | 3 | **0** | **0** |
| Ask-8 | **2** | 5 (2.50×) | 9 (4.50×) | 12 (6.00×) |
| AskParallel-8 | **2** | 4 (2.00×) | 9 (4.50×) | 11 (5.50×) |
| PingPong-8 | **0** | 8 | 2 | 3 |
| ThreadRing-8 | **0** | 3 | 1 | 2 |
| Skynet-8 | 83893922 | 43534470 (0.52×) | **25834778 (0.31×)** | 38399720 (0.46×) |
| SpawnStop-8 | 67 | 41 (0.61×) | **20 (0.30×)** | 30 (0.45×) |
| PingPongParallel-8 | **0** | 7 | 2 | 3 |
| PingPongPipelined-8 | **0** | 5 | 1 | **0** |
| ForkJoinThroughput-8 | **0** | 3 | **0** | 1 |
| Big-8 | **0** | 5 | 2 | 2 |

### Live heap per idle actor

Lower is better.

| Benchmark | goakt | ergo | protoactor | hollywood |
|---|---:|---:|---:|---:|
| MemoryPerActor-8 | 1088 B | 2879 B (2.64×) | **616 B (0.57×)** | 27880 B (25.61×) |

### What the numbers show

- **Messaging between actors is where GoAkt leads most.** With one pair per processor and one message in flight (`PingPongParallel`), GoAkt moves 23.8M msgs/s, against 7.3M for Hollywood, 3.2M for Proto.Actor and 1.8M for Ergo. With 100 in flight (`PingPongPipelined`) it moves 83.5M, against 45.2M for Proto.Actor, 13.3M for Ergo and 8.1M for Hollywood. The many-to-many `Big` exchange is similar: 17.0M msgs/s, 2.5× Hollywood and 4× Proto.Actor. In all of these GoAkt is the only framework that allocates nothing per message.
- **Request/response.** A sequential `Ask` runs at 1.37M msgs/s, while the other three manage 280K to 340K. With 8 concurrent callers GoAkt stays ahead at 2.21M msgs/s, against 1.32M for Proto.Actor. GoAkt allocates 128 B and 2 objects per `Ask` from outside the system; its `PID.Ask` from inside an actor allocates nothing, the difference is in the package-level `Ask`.
- **Single runnable actor.** When only one actor can run at a time, Hollywood is fastest: it wins the single-pair `PingPong` (3.6M against 2.3M msgs/s) and `ThreadRing` (3.4M against 2.4M). Both measure how fast a framework hands the turn from one actor to the next, since no two actors are ever runnable together. Ergo and Proto.Actor are 4 to 8 times behind GoAkt here.
- **Tell.** GoAkt is fastest for a single sender at 17.3M msgs/s; Hollywood is next at 11.8M. With 8 senders contending for one mailbox (`TellParallel`), GoAkt and Proto.Actor are tied within noise. Spreading one sender over 60 actors (`ForkJoinThroughput`) favours Proto.Actor at 5.7M msgs/s and Hollywood at 4.9M over GoAkt's 3.8M, even though GoAkt is the only one that allocates nothing per message there.
- **Actor lifecycle is GoAkt's weakest scenario.** `SpawnStop` runs at 121K actors/s against 297K for Proto.Actor, and `Skynet` creates and stops its 1.1M actors at 144K actors/s against 867K for Proto.Actor and 499K for Ergo. A GoAkt spawn costs 67 allocations including the stop, a Proto.Actor spawn 20. Hollywood is slower than GoAkt at Skynet (73K actors/s) because each of its actors allocates a 1,024-slot inbox, 32 GB in all over one tree.
- **Memory.** An idle GoAkt actor keeps 1,088 B of live heap, an Ergo process 2,879 B, a Proto.Actor actor 616 B and a Hollywood actor 27,880 B, the inbox again.
- **Hollywood can lose replies under concurrent load.** This run completed cleanly. In an earlier run with the same settings, one of the 10 Hollywood `AskParallel` runs failed with `context deadline exceeded`, because a reply never arrived within the 5s timeout. `Engine.Request` names its temporary response process `response/<rand.Intn(MaxInt32)>`; when two in-flight requests draw the same number, one registration overwrites the other and that reply is lost.

## Running

All commands run from this directory. The only requirement is Go.

```bash
make bench                  # benchmark every framework, then compare them
make bench-goakt            # benchmark a single framework (goakt, ergo, protoactor, hollywood)
make compare                # re-print the benchstat comparison from saved results
make table                  # print the saved results as Markdown comparison tables
make smoke                  # run every benchmark once, quickly, to check they work
```

The tuning variables can be overridden:

```bash
make bench BENCH='Ask' COUNT=20 BENCHTIME=2s CPU=1,4,8
```

Raw results are saved under `results/<framework>.txt`, which is git-ignored. `make compare` passes them to benchstat, which is declared as a tool in `go.mod`, with GoAkt as the baseline column:

```bash
go tool benchstat -ignore pkg goakt=results/goakt.txt ergo=results/ergo.txt ...
```

`make table` renders the same results as the Markdown tables in [Results](#results), using [cmd/table](./cmd/table).

You can also run a package directly with `go test`:

```bash
go test -run '^$' -bench . -benchmem ./protoactor
```

## Adding a framework

1. `go get` the framework at a pinned version, then `make vendor`.
2. Create a package named after it, holding a single `<framework>_test.go` file.
3. Start the framework once in `TestMain`, and implement the thirteen benchmarks above under the same names, using the messages, sizes, `Latch`, `Signal` and Skynet helpers from `internal/bench`.
4. Add the package name to `FRAMEWORKS` in the [Makefile](./Makefile).
