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
- **Completion is signalled once per actor.** In the scenarios with many actors (`PingPongParallel`, `PingPongPipelined`, `ForkJoinThroughput`, `Big`, `ThreadRing`, `Skynet`) each actor counts its own messages and touches the shared latch once, when its share is done. A shared counter incremented per message would put a contended cache line into every framework's hot path and compress the differences between them; with it, GoAkt's pipelined ping-pong measured 35M msgs/s instead of 83M on an Apple M1.
- **Skynet and MemoryPerActor are expensive per iteration.** One Skynet iteration creates over a million actors and one MemoryPerActor iteration spawns 50,000, so with `BENCHTIME=1s` each runs once per round. Hollywood's Skynet peaks at about 4 GB of resident memory, because every actor owns a 1,024-slot inbox; the other three stay under 1 GB.

Results depend on the machine, Go version and load, so publish them along with the hardware they ran on. Run with `COUNT` of at least 10, so that benchstat can report confidence intervals.

## Results

The benchmarks ran on two sizes of the same AWS instance family, so the core count is the only difference between the runs. 8 cores shows each framework on a small node; 64 cores matches a typical production server and shows which frameworks keep scaling as cores are added. Each run is one `make bench COUNT=10 BENCHTIME=1s` on a dedicated instance with nothing else running.

| | 8 cores | 64 cores |
|---|---|---|
| Instance | AWS `c7a.2xlarge` | AWS `c7a.16xlarge` |
| CPU | AMD EPYC 9R14, 8 physical cores (one thread per core) | AMD EPYC 9R14, 64 physical cores (one thread per core) |
| Memory | 16 GiB | 128 GiB |
| `GOMAXPROCS` | 8 | 64 |
| Date | 2026-11-01 | pending |
| Full output | [RESULTS-2026-11-01-c7a-8.md](./RESULTS-2026-11-01-c7a-8.md) | pending |

Both runs share the rest of the setup:

| | |
|---|---|
| OS | Ubuntu 26.04.1 LTS, Linux 7.0, x86_64 |
| Go | 1.27.3 |
| GoAkt | v4.6.6 |
| Ergo | v3 (`ergo.services/ergo` v1.999.330) |
| Proto.Actor | `v0.0.0-20260118094027-288962e52f3f` |
| Hollywood | v1.0.5 |

Each cell is the median of 10 runs. Throughput is higher-is-better; the other tables are lower-is-better. The best value in each row is in bold, and the factor in parentheses is relative to GoAkt. The `-8` and `-64` suffixes are `GOMAXPROCS`. `Skynet` and `MemoryPerActor` ran once per round, so their medians are over 10 iterations. The full benchstat output for each run, with the spread of every row, is in its results file. Earlier versions of this README reported runs on an Apple M1, whose numbers are not comparable with these.

### 8 cores (`c7a.2xlarge`)

#### Throughput

Higher is better.

| Benchmark | goakt | ergo | protoactor | hollywood |
|---|---:|---:|---:|---:|
| Tell-8 | 14.57M msgs/s | 5.66M msgs/s (0.39×) | 17.63M msgs/s (1.21×) | **22.38M msgs/s (1.54×)** |
| TellParallel-8 | 8.15M msgs/s | 4.64M msgs/s (0.57×) | 10.60M msgs/s (1.30×) | **16.48M msgs/s (2.02×)** |
| Ask-8 | **1.28M msgs/s** | 217K msgs/s (0.17×) | 315K msgs/s (0.25×) | 685K msgs/s (0.54×) |
| AskParallel-8 | **2.03M msgs/s** | 846K msgs/s (0.42×) | 812K msgs/s (0.40×) | 659K msgs/s (0.33×) |
| PingPong-8 | **7.91M msgs/s** | 246K msgs/s (0.03×) | 465K msgs/s (0.06×) | 4.10M msgs/s (0.52×) |
| PingPongParallel-8 | **48.17M msgs/s** | 2.37M msgs/s (0.05×) | 4.37M msgs/s (0.09×) | 17.52M msgs/s (0.36×) |
| PingPongPipelined-8 | **143.14M msgs/s** | 20.63M msgs/s (0.14×) | 83.16M msgs/s (0.58×) | 20.17M msgs/s (0.14×) |
| ThreadRing-8 | **7.81M msgs/s** | 213K msgs/s (0.03×) | 456K msgs/s (0.06×) | 3.77M msgs/s (0.48×) |
| ForkJoinThroughput-8 | **17.38M msgs/s** | 3.71M msgs/s (0.21×) | 11.65M msgs/s (0.67×) | 6.38M msgs/s (0.37×) |
| Big-8 | **35.52M msgs/s** | 3.11M msgs/s (0.09×) | 5.76M msgs/s (0.16×) | 16.40M msgs/s (0.46×) |
| Skynet-8 | 289K actors/s | 582K actors/s (2.01×) | **1.30M actors/s (4.51×)** | 86K actors/s (0.30×) |
| SpawnStop-8 | 162K actors/s | 165K actors/s (1.02×) | **230K actors/s (1.42×)** | 160K actors/s (0.99×) |

#### Time per operation

Lower is better.

| Benchmark | goakt | ergo | protoactor | hollywood |
|---|---:|---:|---:|---:|
| Tell-8 | 68.6 ns | 176.8 ns (2.58×) | 56.7 ns (0.83×) | **44.7 ns (0.65×)** |
| TellParallel-8 | 122.7 ns | 215.6 ns (1.76×) | 94.4 ns (0.77×) | **60.7 ns (0.49×)** |
| Ask-8 | **783.5 ns** | 4.62 µs (5.89×) | 3.18 µs (4.05×) | 1.46 µs (1.86×) |
| AskParallel-8 | **493.2 ns** | 1.18 µs (2.40×) | 1.23 µs (2.50×) | 1.52 µs (3.08×) |
| PingPong-8 | **252.8 ns** | 8.13 µs (32.15×) | 4.30 µs (17.01×) | 487.8 ns (1.93×) |
| PingPongParallel-8 | **41.5 ns** | 845.2 ns (20.35×) | 457.6 ns (11.02×) | 114.2 ns (2.75×) |
| PingPongPipelined-8 | **14.0 ns** | 96.9 ns (6.94×) | 24.1 ns (1.72×) | 99.1 ns (7.09×) |
| ThreadRing-8 | **128.1 ns** | 4.70 µs (36.71×) | 2.19 µs (17.11×) | 265.1 ns (2.07×) |
| ForkJoinThroughput-8 | **57.5 ns** | 269.9 ns (4.69×) | 85.8 ns (1.49×) | 156.8 ns (2.72×) |
| Big-8 | **56.3 ns** | 643.5 ns (11.43×) | 347.3 ns (6.17×) | 121.9 ns (2.17×) |
| Skynet-8 | 3845.69 ms | 1909.71 ms (0.50×) | **853.59 ms (0.22×)** | 12867.84 ms (3.35×) |
| SpawnStop-8 | 6.17 µs | 6.06 µs (0.98×) | **4.34 µs (0.70×)** | 6.25 µs (1.01×) |

#### Bytes allocated per operation

Lower is better.

| Benchmark | goakt | ergo | protoactor | hollywood |
|---|---:|---:|---:|---:|
| Tell-8 | **8 B** | 91 B (10.71×) | 26 B (3.06×) | 25 B (2.94×) |
| TellParallel-8 | **25 B** | 136 B (5.44×) | 55 B (2.20×) | 27 B (1.08×) |
| Ask-8 | **128 B** | 168 B (1.31×) | 512 B (4.00×) | 703 B (5.49×) |
| AskParallel-8 | **128 B** | 194 B (1.52×) | 500 B (3.91×) | 699 B (5.46×) |
| PingPong-8 | **0 B** | 208 B | 64 B | 79 B |
| PingPongParallel-8 | **0 B** | 176 B | 63 B | 79 B |
| PingPongPipelined-8 | **0 B** | 147 B | 32 B | 54 B |
| ThreadRing-8 | **0 B** | 72 B | 16 B | 40 B |
| ForkJoinThroughput-8 | **0 B** | 93 B | 4 B | 32 B |
| Big-8 | **0 B** | 133 B | 53 B | 70 B |
| Skynet-8 | 4958298528 B | 4598699212 B (0.93×) | **1189525806 B (0.24×)** | 32129975508 B (6.48×) |
| SpawnStop-8 | 2056 B | 4324 B (2.10×) | **1000 B (0.49×)** | 28734 B (13.98×) |

#### Allocations per operation

Lower is better.

| Benchmark | goakt | ergo | protoactor | hollywood |
|---|---:|---:|---:|---:|
| Tell-8 | **0** | 3 | **0** | **0** |
| TellParallel-8 | **0** | 3 | **0** | **0** |
| Ask-8 | **2** | 5 (2.50×) | 9 (4.50×) | 13 (6.50×) |
| AskParallel-8 | **2** | 4 (2.00×) | 9 (4.50×) | 12 (6.00×) |
| PingPong-8 | **0** | 8 | 2 | 3 |
| PingPongParallel-8 | **0** | 7 | 2 | 3 |
| PingPongPipelined-8 | **0** | 5 | 1 | **0** |
| ThreadRing-8 | **0** | 3 | 1 | 2 |
| ForkJoinThroughput-8 | **0** | 3 | **0** | 1 |
| Big-8 | **0** | 5 | 2 | 2 |
| Skynet-8 | 35165381 | 43517212 (1.24×) | **25836248 (0.73×)** | 38086798 (1.08×) |
| SpawnStop-8 | 27 | 41 (1.52×) | **20 (0.74×)** | 30 (1.11×) |

#### Live heap per idle actor

Lower is better.

| Benchmark | goakt | ergo | protoactor | hollywood |
|---|---:|---:|---:|---:|
| MemoryPerActor-8 | 1065 B | 2879 B (2.70×) | **616 B (0.58×)** | 27880 B (26.18×) |

#### What the numbers show

- **Messaging between actors is where GoAkt leads most.** With one pair per processor and one message in flight (`PingPongParallel`), GoAkt moves 48.2M msgs/s, against 17.5M for Hollywood, 4.4M for Proto.Actor and 2.4M for Ergo. With 100 in flight (`PingPongPipelined`) it moves 143.1M, against 83.2M for Proto.Actor, 20.6M for Ergo and 20.2M for Hollywood; Proto.Actor's figure has the widest spread of the run, ±25%. The many-to-many `Big` exchange is similar: 35.5M msgs/s, 2.2× Hollywood and 6.2× Proto.Actor. Spreading one sender over 60 actors (`ForkJoinThroughput`) also favours GoAkt, at 17.4M msgs/s against 11.7M for Proto.Actor, 6.4M for Hollywood and 3.7M for Ergo. In all of these GoAkt is the only framework that allocates no bytes per message.
- **Single runnable actor.** When only one actor can run at a time, GoAkt is fastest: 7.9M msgs/s on the single-pair `PingPong` against 4.1M for Hollywood, and 7.8M on `ThreadRing` against 3.8M. Both measure how fast a framework hands the turn from one actor to the next, since no two actors are ever runnable together, and GoAkt runs the actor a turn readies on the same worker without waking another. Ergo and Proto.Actor are 17 to 37 times behind.
- **Request/response.** A sequential `Ask` runs at 1.28M msgs/s, against 685K for Hollywood, 315K for Proto.Actor and 217K for Ergo. With 8 concurrent callers GoAkt stays ahead at 2.03M msgs/s, against 846K for Ergo, 812K for Proto.Actor and 659K for Hollywood. GoAkt allocates 128 B and 2 objects per `Ask` from outside the system; its `PID.Ask` from inside an actor allocates nothing, the difference is in the package-level `Ask`.
- **Tell from outside the system is GoAkt's weakest messaging scenario.** For a single sender, Hollywood is fastest at 22.4M msgs/s (44.7 ns per message), then Proto.Actor at 17.6M, GoAkt at 14.6M (68.6 ns) and Ergo at 5.7M. With 8 senders contending for one mailbox (`TellParallel`), Hollywood leads at 16.5M msgs/s, 2.0× GoAkt's 8.2M, with Proto.Actor at 10.6M and Ergo at 4.6M. GoAkt's `Tell` rounds ranged from 63 to 85 ns, 8 of 10 between 66 and 72 ns, so the result is not one slow outlier. Only Ergo allocates an object per `Tell`.
- **Actor lifecycle favours Proto.Actor.** `SpawnStop` runs at 162K actors/s, level with Ergo's 165K and Hollywood's 160K, and behind Proto.Actor's 230K. `Skynet` creates and stops its 1.1M actors at 289K actors/s, against 1.30M for Proto.Actor and 582K for Ergo; GoAkt's and Ergo's Skynet figures spread by ±8% and ±14%, wider than the rest of the run. A GoAkt spawn costs 27 allocations including the stop, a Proto.Actor spawn 20, an Ergo spawn 41. Hollywood is slower than GoAkt at Skynet (86K actors/s) because each of its actors allocates a 1,024-slot inbox, 30 GiB in all over one tree.
- **Memory.** An idle GoAkt actor keeps 1,065 B of live heap, an Ergo process 2,879 B, a Proto.Actor actor 616 B and a Hollywood actor 27,880 B, the inbox again.
- **Hollywood can lose replies under concurrent load.** This run completed cleanly. In an earlier run with the same settings, on an Apple M1, one of the 10 Hollywood `AskParallel` runs failed with `context deadline exceeded`, because a reply never arrived within the 5s timeout. `Engine.Request` names its temporary response process `response/<rand.Intn(MaxInt32)>`; when two in-flight requests draw the same number, one registration overwrites the other and that reply is lost.

### 64 cores (`c7a.16xlarge`)

Pending: the run on `c7a.16xlarge` has not been made yet.

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
