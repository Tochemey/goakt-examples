# GoAkt Benchmarks

Benchmarks that compare [GoAkt](https://github.com/Tochemey/goakt) with other Go actor frameworks:

| Package | Framework | Version |
|---|---|---|
| [goakt](./goakt) | [GoAkt](https://github.com/Tochemey/goakt) | v4.6.7 |
| [ergo](./ergo) | [Ergo Framework](https://github.com/ergo-services/ergo) | v3 (`ergo.services/ergo` v1.999.330) |
| [protoactor](./protoactor) | [Proto.Actor Go](https://github.com/asynkron/protoactor-go) | `v0.0.0-20260118094027-288962e52f3f` |
| [hollywood](./hollywood) | [Hollywood](https://github.com/anthdm/hollywood) | v1.0.5 |

This directory is its own Go module, with its own `go.mod`. The competing frameworks and their dependencies stay out of the root examples module, and every framework is pinned to an exact version, so results can be reproduced.

## At a glance

GoAkt v4.6.7 on AWS machines with 8, 32 and 64 cores; see [Results](#results) for the tables and [Methodology](#methodology) for how they were measured.

- **GoAkt leads at every core count on:**
  - messaging between actors: `PingPong`, `PingPongParallel`, `PingPongPipelined` and `ThreadRing`
  - fan-out and many-to-many: `ForkJoinThroughput` and `Big`
  - request/response: `Ask` and `AskParallel`
  - concurrent sends to one actor: `TellParallel`
  - grains: `GrainAsk` and the memory an idle grain keeps

  It allocates nothing per message in any of the ping-pong benchmarks. Its lead on `PingPongParallel` grows with the core count, from 3.1× the next framework on 8 cores to 10.7× on 64.
- **GoAkt trails at every core count on:**

  | Benchmark | Leader | 8 cores | 32 cores | 64 cores |
  |---|---|---|---|---|
  | `TellFromActor` | Hollywood | 1.19× | 1.24× | 1.73× |
  | `Tell` | Hollywood | 1.07× | 1.26× | 1.21× |
  | `Skynet` | Proto.Actor | 2.86× | 5.87× | 5.10× |
  | `SpawnStop` | Proto.Actor | 1.17× | 1.32× | 1.32× |

  Proto.Actor also keeps less memory per idle actor: 616 B against GoAkt's 1,064 to 1,065 B.
- **It depends on the core count:**
  - `GrainAskParallel`: GoAkt leads on 8 cores, 2.1× Proto.Actor, but Proto.Actor leads on 32 cores (1.15×) and 64 cores (1.46×).
  - `GrainActivate`: GoAkt and Proto.Actor are within 4% of each other at every core count.

## Scenarios

Every framework package implements the same fourteen benchmarks under the same names, with the same sizes from the shared package, so benchstat can line them up.

The first seven measure one operation each, driven from outside the system or between two actors:

| Benchmark            | What it measures                                                                                          |
|----------------------|-----------------------------------------------------------------------------------------------------------|
| `BenchmarkTell`         | Fire-and-forget messages from outside the system to one actor, until the actor has handled all of them. |
| `BenchmarkTellParallel` | Same as `Tell`, with `GOMAXPROCS` goroutines sending to the same actor at once (mailbox contention).    |
| `BenchmarkTellFromActor` | Same as `Tell`, but sent by another actor from inside its message handler, so every message carries a sender. |
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

### Grains

Grains are virtual actors: they are addressed by name and the framework activates one on its first message. Only GoAkt and Proto.Actor have them, so these four benchmarks exist in the `goakt` and `protoactor` packages only, and their results are compared in a table of their own. Ergo has no virtual actors. Hollywood's cluster package spawns named actors on other nodes only when `Activate` is called, over a network, so nothing is activated on a first message.

| Benchmark | What it measures | Actor counterpart |
|---|---|---|
| `BenchmarkGrainAsk` | Request/response round trips to one active grain, one at a time: `AskGrain` in GoAkt, `Cluster.Request` in Proto.Actor. Both run in a single-node cluster; see [Methodology](#methodology). | `Ask` |
| `BenchmarkGrainAskParallel` | Same as `GrainAsk`, with `GOMAXPROCS` goroutines each asking a grain of its own, so callers contend only on the name lookup. | `AskParallel` |
| `BenchmarkGrainActivate` | Activating a grain under a new name and completing its first request/response round trip. Reported as grains per second. | `SpawnStop` |
| `BenchmarkGrainMemoryPerGrain` | The live heap after a full collection, divided by 50,000 idle grains activated into the system. Reported in bytes per grain and shown in its own table. | `MemoryPerActor` |

A grain `Tell` is not compared: Proto.Actor's grains only take requests, and GoAkt's own repository already has `BenchmarkGrainTell`.

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
- **TellFromActor sends from a handler.** Ergo only lets a process send while it is running one of its own handlers, so the sending actor receives one `Start` and sends all `b.N` messages from inside that handler, in every framework. Each uses its own call for a send that carries the sender: `ReceiveContext.Tell` in GoAkt, `Send` in Ergo, `Context.Request` in Proto.Actor and `Context.Send` in Hollywood.
- **Grains run in a single-node cluster in both frameworks.** Grains are meant for clusters, and Proto.Actor's need one, so GoAkt's run in one too and both register every grain they activate. GoAkt uses `static` discovery listing only itself, with one replica, and activates grains with `GrainOf` and `WithLongLivedGrain`, so none is passivated during a run. Proto.Actor uses the in-memory `test` provider and the `disthash` identity lookup. Both listen on localhost because a cluster member can't start without remoting, but every grain is local, so no message crosses the network. The cluster starts only when a grain benchmark runs, so its gossip stays out of the actor benchmarks.
- **Grain benchmarks that create grains get a fresh cluster and a warm-up.** `GrainActivate` and `GrainMemoryPerGrain` start a cluster of their own, so the grains they create don't accumulate, and activate 10,000 grains before measuring. A fresh GoAkt cluster allocates a 4 MB table on the first write to each of its 271 partitions, about 1.1 GB in all; without the warm-up, that one-off cost would be charged to the first grains measured. Grain records written into those tables afterwards don't grow the heap, so `B/grain` is the in-process cost of a grain. Setup, warm-up and shutdown are outside the measured time.
- **Actor names.** GoAkt requires a unique name for every actor, so `SpawnStop` and `Skynet` build one per actor with `bench.Name`, inside the timed loop. The other frameworks generate their identifiers internally, so that cost is in their numbers too.
- **ThreadRing passes one token.** The token is a single message whose hop count is decremented in place by each actor before it is forwarded. Only one actor holds it at a time, so no synchronization is needed, and no framework allocates a message per hop.
- **Big picks peers deterministically.** `bench.NextPeer` is a fixed pseudo-random walk over the actor indexes, so every framework sends the same messages to the same peers in the same order.
- **Skynet reports to the parent.** Each node learns its parent from the sender of its range message, so the frameworks' own sender tracking is used: `Sender()` in GoAkt and Hollywood, `from` in Ergo, `Request` and `Sender()` in Proto.Actor. A node stops itself once it has reported.
- **Completion is signalled once per actor.** In the scenarios with many actors (`PingPongParallel`, `PingPongPipelined`, `ForkJoinThroughput`, `Big`, `ThreadRing`, `Skynet`) each actor counts its own messages and touches the shared latch once, when its share is done. A shared counter incremented per message would put a contended cache line into every framework's hot path and compress the differences between them; with it, GoAkt's pipelined ping-pong measured 35M msgs/s instead of 83M on an Apple M1.
- **Skynet and MemoryPerActor are expensive per iteration.** One Skynet iteration creates over a million actors and one MemoryPerActor iteration spawns 50,000, so with `BENCHTIME=1s` each runs once per round. Hollywood's Skynet peaks at about 4 GB of resident memory, because every actor owns a 1,024-slot inbox; the other three stay under 1 GB.

Results depend on the machine, Go version and load, so publish them along with the hardware they ran on. Run with `COUNT` of at least 10, so that benchstat can report confidence intervals.

## Results

The benchmarks ran on 2026-11-01 on three sizes of the same AWS instance family, each a dedicated instance with nothing else running:

| | 8 cores | 32 cores | 64 cores |
|---|---|---|---|
| Instance | `c7a.2xlarge` | `c7a.8xlarge` | `c7a.16xlarge` |
| Memory | 16 GiB | 64 GiB | 128 GiB |
| `GOMAXPROCS` | 8 | 32 | 64 |
| Full output | [RESULTS-2026-11-01-c7a-8.md](./RESULTS-2026-11-01-c7a-8.md) | [RESULTS-2026-11-01-c7a-32.md](./RESULTS-2026-11-01-c7a-32.md) | [RESULTS-2026-11-01-c7a-64.md](./RESULTS-2026-11-01-c7a-64.md) |

All three share the rest of the setup:

| | |
|---|---|
| Region | us-east-1 |
| CPU | AMD EPYC 9R14, one thread per core, so every vCPU is a physical core |
| OS | Ubuntu 26.04.1 LTS, Linux 7.0, x86_64 |
| Go | 1.27.3 |
| Framework versions | as in the table at the top |

**How each core count was measured:**

1. `make bench COUNT=10 BENCHTIME=1s` ran all four frameworks.
2. GoAkt was then rerun on its own with `make bench-goakt COUNT=10 BENCHTIME=1s` at commit `f7ee4c6`. That commit has the same code as the v4.6.7 release, whose tag adds only changelog files. GoAkt's rows come from this rerun, and the other three frameworks' rows from step 1.
3. On 32 cores, both runs used the same instance without a restart. On 8 and 64 cores, the instance was restarted between them, so the two runs may have been on different physical hosts.

`vmstat` reported 0% CPU steal throughout every run, and every round completed. Each results file lists the rows whose rounds varied widely, with the full benchstat output.

**How to read the tables:**
- Each cell is the median of 10 runs. The best value in each row is in bold, and the factor in parentheses is relative to GoAkt.
- The suffix after each name is `GOMAXPROCS`.
- `Skynet`, `MemoryPerActor` and `GrainMemoryPerGrain` run once per round, so their medians are over 10 iterations.
- A bold value is not always a significant lead: where two medians are close, check the benchstat p-value in the results file.

Earlier versions of this README reported other runs, including some on an Apple M1. Those numbers are not comparable with these.

### 8 cores (`c7a.2xlarge`)

Full output, with the spread and p-value of every row: [RESULTS-2026-11-01-c7a-8.md](./RESULTS-2026-11-01-c7a-8.md).

#### Actors

##### Throughput

Higher is better.

| Benchmark | goakt | ergo | protoactor | hollywood |
|---|---:|---:|---:|---:|
| Tell-8 | 21.07M msgs/s | 5.66M msgs/s (0.27×) | 18.44M msgs/s (0.88×) | **22.46M msgs/s (1.07×)** |
| TellParallel-8 | **18.23M msgs/s** | 4.68M msgs/s (0.26×) | 10.48M msgs/s (0.57×) | 16.55M msgs/s (0.91×) |
| TellFromActor-8 | 18.48M msgs/s | 4.76M msgs/s (0.26×) | 13.46M msgs/s (0.73×) | **21.94M msgs/s (1.19×)** |
| Ask-8 | **1.30M msgs/s** | 217K msgs/s (0.17×) | 309K msgs/s (0.24×) | 666K msgs/s (0.51×) |
| AskParallel-8 | **2.11M msgs/s** | 843K msgs/s (0.40×) | 821K msgs/s (0.39×) | 663K msgs/s (0.31×) |
| PingPong-8 | **7.86M msgs/s** | 249K msgs/s (0.03×) | 473K msgs/s (0.06×) | 4.24M msgs/s (0.54×) |
| PingPongParallel-8 | **47.71M msgs/s** | 2.37M msgs/s (0.05×) | 4.38M msgs/s (0.09×) | 15.41M msgs/s (0.32×) |
| PingPongPipelined-8 | **142.88M msgs/s** | 20.40M msgs/s (0.14×) | 96.48M msgs/s (0.68×) | 20.21M msgs/s (0.14×) |
| ThreadRing-8 | **7.92M msgs/s** | 210K msgs/s (0.03×) | 459K msgs/s (0.06×) | 3.81M msgs/s (0.48×) |
| ForkJoinThroughput-8 | **26.97M msgs/s** | 3.57M msgs/s (0.13×) | 10.68M msgs/s (0.40×) | 6.22M msgs/s (0.23×) |
| Big-8 | **35.54M msgs/s** | 3.10M msgs/s (0.09×) | 5.73M msgs/s (0.16×) | 15.52M msgs/s (0.44×) |
| Skynet-8 | 456K actors/s | 572K actors/s (1.25×) | **1.30M actors/s (2.86×)** | 88K actors/s (0.19×) |
| SpawnStop-8 | 199K actors/s | 165K actors/s (0.83×) | **233K actors/s (1.17×)** | 160K actors/s (0.80×) |

<details>
<summary>Time, bytes and allocations per operation, and live heap per idle actor</summary>

##### Time per operation

Lower is better.

| Benchmark | goakt | ergo | protoactor | hollywood |
|---|---:|---:|---:|---:|
| Tell-8 | 47.5 ns | 176.6 ns (3.72×) | 54.3 ns (1.14×) | **44.5 ns (0.94×)** |
| TellParallel-8 | **54.9 ns** | 213.7 ns (3.90×) | 95.5 ns (1.74×) | 60.4 ns (1.10×) |
| TellFromActor-8 | 54.1 ns | 210.2 ns (3.88×) | 76.6 ns (1.42×) | **45.6 ns (0.84×)** |
| Ask-8 | **771.1 ns** | 4.61 µs (5.98×) | 3.23 µs (4.20×) | 1.50 µs (1.95×) |
| AskParallel-8 | **472.9 ns** | 1.19 µs (2.51×) | 1.22 µs (2.58×) | 1.51 µs (3.19×) |
| PingPong-8 | **254.4 ns** | 8.05 µs (31.63×) | 4.23 µs (16.63×) | 472.2 ns (1.86×) |
| PingPongParallel-8 | **41.9 ns** | 843.7 ns (20.12×) | 456.1 ns (10.88×) | 129.9 ns (3.10×) |
| PingPongPipelined-8 | **14.0 ns** | 98.1 ns (7.00×) | 20.7 ns (1.48×) | 98.9 ns (7.07×) |
| ThreadRing-8 | **126.3 ns** | 4.76 µs (37.66×) | 2.18 µs (17.23×) | 262.2 ns (2.08×) |
| ForkJoinThroughput-8 | **37.1 ns** | 279.9 ns (7.55×) | 93.7 ns (2.53×) | 160.8 ns (4.34×) |
| Big-8 | **56.3 ns** | 644.8 ns (11.46×) | 349.1 ns (6.20×) | 128.9 ns (2.29×) |
| Skynet-8 | 2435.84 ms | 1942.10 ms (0.80×) | **852.68 ms (0.35×)** | 12653.01 ms (5.19×) |
| SpawnStop-8 | 5.02 µs | 6.07 µs (1.21×) | **4.29 µs (0.86×)** | 6.25 µs (1.25×) |

##### Bytes allocated per operation

Lower is better.

| Benchmark | goakt | ergo | protoactor | hollywood |
|---|---:|---:|---:|---:|
| Tell-8 | 23 B | 91 B (3.96×) | **19 B (0.83×)** | 25 B (1.09×) |
| TellParallel-8 | **11 B** | 136 B (12.36×) | 54 B (4.91×) | 27 B (2.45×) |
| TellFromActor-8 | 29 B | 90 B (3.10×) | 36 B (1.24×) | **25 B (0.86×)** |
| Ask-8 | **128 B** | 168 B (1.31×) | 512 B (4.00×) | 703 B (5.49×) |
| AskParallel-8 | **128 B** | 194 B (1.52×) | 500 B (3.91×) | 699 B (5.46×) |
| PingPong-8 | **0 B** | 208 B | 64 B | 79 B |
| PingPongParallel-8 | **0 B** | 176 B | 63 B | 79 B |
| PingPongPipelined-8 | **0 B** | 147 B | 32 B | 54 B |
| ThreadRing-8 | **0 B** | 72 B | 16 B | 40 B |
| ForkJoinThroughput-8 | **0 B** | 94 B | 5 B | 33 B |
| Big-8 | **0 B** | 133 B | 53 B | 70 B |
| Skynet-8 | 2875590640 B | 4598390052 B (1.60×) | **1188899060 B (0.41×)** | 32129577888 B (11.17×) |
| SpawnStop-8 | 1928 B | 4324 B (2.24×) | **1000 B (0.52×)** | 28734 B (14.90×) |

##### Allocations per operation

Lower is better.

| Benchmark | goakt | ergo | protoactor | hollywood |
|---|---:|---:|---:|---:|
| Tell-8 | **0** | 3 | **0** | **0** |
| TellParallel-8 | **0** | 3 | **0** | **0** |
| TellFromActor-8 | **0** | 3 | 1 | **0** |
| Ask-8 | **2** | 5 (2.50×) | 9 (4.50×) | 12 (6.25×) |
| AskParallel-8 | **2** | 4 (2.00×) | 9 (4.50×) | 12 (6.00×) |
| PingPong-8 | **0** | 8 | 3 | 3 |
| PingPongParallel-8 | **0** | 7 | 2 | 3 |
| PingPongPipelined-8 | **0** | 5 | 1 | **0** |
| ThreadRing-8 | **0** | 3 | 1 | 2 |
| ForkJoinThroughput-8 | **0** | 3 | **0** | 1 |
| Big-8 | **0** | 5 | 2 | 2 |
| Skynet-8 | 30253489 | 43504252 (1.44×) | **25835933 (0.85×)** | 38074758 (1.26×) |
| SpawnStop-8 | 25 | 41 (1.64×) | **20 (0.80×)** | 30 (1.20×) |

##### Live heap per idle actor

Lower is better.

| Benchmark | goakt | ergo | protoactor | hollywood |
|---|---:|---:|---:|---:|
| MemoryPerActor-8 | 1065 B | 2879 B (2.70×) | **616 B (0.58×)** | 27880 B (26.18×) |

</details>

#### Grains

GoAkt and Proto.Actor only, each in a single-node cluster.

##### Throughput

Higher is better.

| Benchmark | goakt | protoactor |
|---|---:|---:|
| GrainAsk-8 | **1.31M msgs/s** | 188K msgs/s (0.14×) |
| GrainAskParallel-8 | **4.34M msgs/s** | 2.02M msgs/s (0.47×) |
| GrainActivate-8 | 59K grains/s | **61K grains/s (1.04×)** |

<details>
<summary>Time, bytes and allocations per operation, and live heap per idle grain</summary>

##### Time per operation

Lower is better.

| Benchmark | goakt | protoactor |
|---|---:|---:|
| GrainAsk-8 | **760.5 ns** | 5.32 µs (6.99×) |
| GrainAskParallel-8 | **230.5 ns** | 494.6 ns (2.15×) |
| GrainActivate-8 | 16.94 µs | **16.34 µs (0.96×)** |

##### Bytes allocated per operation

Lower is better.

| Benchmark | goakt | protoactor |
|---|---:|---:|
| GrainAsk-8 | **0 B** | 1144 B |
| GrainAskParallel-8 | **0 B** | 1144 B |
| GrainActivate-8 | **8834 B** | 17485 B (1.98×) |

##### Allocations per operation

Lower is better.

| Benchmark | goakt | protoactor |
|---|---:|---:|
| GrainAsk-8 | **0** | 21 |
| GrainAskParallel-8 | **0** | 21 |
| GrainActivate-8 | **123** | 921 (7.49×) |

##### Live heap per idle grain

Lower is better.

| Benchmark | goakt | protoactor |
|---|---:|---:|
| GrainMemoryPerGrain-8 | **398 B** | 1624 B (4.08×) |

</details>

### 32 cores (`c7a.8xlarge`)

Full output, with the spread and p-value of every row: [RESULTS-2026-11-01-c7a-32.md](./RESULTS-2026-11-01-c7a-32.md).

#### Actors

##### Throughput

Higher is better.

| Benchmark | goakt | ergo | protoactor | hollywood |
|---|---:|---:|---:|---:|
| Tell-32 | 15.41M msgs/s | 2.77M msgs/s (0.18×) | 13.08M msgs/s (0.85×) | **19.36M msgs/s (1.26×)** |
| TellParallel-32 | **8.38M msgs/s** | 1.89M msgs/s (0.22×) | 5.67M msgs/s (0.68×) | 5.41M msgs/s (0.65×) |
| TellFromActor-32 | 14.27M msgs/s | 2.54M msgs/s (0.18×) | 7.81M msgs/s (0.55×) | **17.67M msgs/s (1.24×)** |
| Ask-32 | **1.07M msgs/s** | 208K msgs/s (0.19×) | 255K msgs/s (0.24×) | 545K msgs/s (0.51×) |
| AskParallel-32 | **1.19M msgs/s** | 625K msgs/s (0.53×) | 539K msgs/s (0.45×) | 369K msgs/s (0.31×) |
| PingPong-32 | **7.88M msgs/s** | 249K msgs/s (0.03×) | 395K msgs/s (0.05×) | 3.70M msgs/s (0.47×) |
| PingPongParallel-32 | **94.21M msgs/s** | 5.61M msgs/s (0.06×) | 12.28M msgs/s (0.13×) | 11.76M msgs/s (0.12×) |
| PingPongPipelined-32 | **330.73M msgs/s** | 18.50M msgs/s (0.06×) | 162.59M msgs/s (0.49×) | 12.53M msgs/s (0.04×) |
| ThreadRing-32 | **7.84M msgs/s** | 242K msgs/s (0.03×) | 400K msgs/s (0.05×) | 3.02M msgs/s (0.39×) |
| ForkJoinThroughput-32 | **9.13M msgs/s** | 880K msgs/s (0.10×) | 4.83M msgs/s (0.53×) | 2.89M msgs/s (0.32×) |
| Big-32 | **21.58M msgs/s** | 6.89M msgs/s (0.32×) | 13.99M msgs/s (0.65×) | 10.78M msgs/s (0.50×) |
| Skynet-32 | 422K actors/s | 1.08M actors/s (2.55×) | **2.48M actors/s (5.87×)** | 56K actors/s (0.13×) |
| SpawnStop-32 | 147K actors/s | 140K actors/s (0.95×) | **194K actors/s (1.32×)** | 139K actors/s (0.94×) |

<details>
<summary>Time, bytes and allocations per operation, and live heap per idle actor</summary>

##### Time per operation

Lower is better.

| Benchmark | goakt | ergo | protoactor | hollywood |
|---|---:|---:|---:|---:|
| Tell-32 | 64.9 ns | 361.1 ns (5.56×) | 76.5 ns (1.18×) | **51.7 ns (0.80×)** |
| TellParallel-32 | **119.3 ns** | 530.5 ns (4.45×) | 176.4 ns (1.48×) | 184.8 ns (1.55×) |
| TellFromActor-32 | 70.1 ns | 393.9 ns (5.62×) | 128.1 ns (1.83×) | **56.6 ns (0.81×)** |
| Ask-32 | **935.0 ns** | 4.80 µs (5.14×) | 3.93 µs (4.20×) | 1.84 µs (1.96×) |
| AskParallel-32 | **842.0 ns** | 1.60 µs (1.90×) | 1.86 µs (2.20×) | 2.71 µs (3.22×) |
| PingPong-32 | **253.7 ns** | 8.03 µs (31.66×) | 5.07 µs (19.96×) | 541.2 ns (2.13×) |
| PingPongParallel-32 | **21.2 ns** | 356.4 ns (16.79×) | 162.9 ns (7.67×) | 170.1 ns (8.01×) |
| PingPongPipelined-32 | **6.0 ns** | 108.1 ns (17.88×) | 12.3 ns (2.03×) | 159.6 ns (26.38×) |
| ThreadRing-32 | **127.6 ns** | 4.12 µs (32.34×) | 2.50 µs (19.61×) | 330.8 ns (2.59×) |
| ForkJoinThroughput-32 | **109.5 ns** | 1.14 µs (10.37×) | 206.9 ns (1.89×) | 346.1 ns (3.16×) |
| Big-32 | **92.7 ns** | 290.1 ns (3.13×) | 142.9 ns (1.54×) | 185.6 ns (2.00×) |
| Skynet-32 | 2630.58 ms | 1030.56 ms (0.39×) | **448.14 ms (0.17×)** | 19819.43 ms (7.53×) |
| SpawnStop-32 | 6.78 µs | 7.15 µs (1.05×) | **5.15 µs (0.76×)** | 7.19 µs (1.06×) |

##### Bytes allocated per operation

Lower is better.

| Benchmark | goakt | ergo | protoactor | hollywood |
|---|---:|---:|---:|---:|
| Tell-32 | **20 B** | 90 B (4.39×) | 26 B (1.29×) | 25 B (1.22×) |
| TellParallel-32 | **2 B** | 150 B (75.25×) | 48 B (24.25×) | 25 B (12.50×) |
| TellFromActor-32 | **24 B** | 90 B (3.75×) | 32 B (1.33×) | 25 B (1.04×) |
| Ask-32 | **128 B** | 169 B (1.32×) | 512 B (4.00×) | 703 B (5.49×) |
| AskParallel-32 | **128 B** | 192 B (1.50×) | 497 B (3.88×) | 700 B (5.47×) |
| PingPong-32 | **0 B** | 208 B | 64 B | 79 B |
| PingPongParallel-32 | **0 B** | 176 B | 63 B | 80 B |
| PingPongPipelined-32 | **0 B** | 150 B | 32 B | 53 B |
| ThreadRing-32 | **0 B** | 72 B | 16 B | 40 B |
| ForkJoinThroughput-32 | **0 B** | 102 B | 4 B | 31 B |
| Big-32 | **0 B** | 131 B | 50 B | 66 B |
| Skynet-32 | 2868980676 B | 4585508384 B (1.60×) | **1187409946 B (0.41×)** | 32132664692 B (11.20×) |
| SpawnStop-32 | 1928 B | 4328 B (2.24×) | **1000 B (0.52×)** | 28736 B (14.90×) |

##### Allocations per operation

Lower is better.

| Benchmark | goakt | ergo | protoactor | hollywood |
|---|---:|---:|---:|---:|
| Tell-32 | **0** | 3 | **0** | **0** |
| TellParallel-32 | **0** | 3 | **0** | **0** |
| TellFromActor-32 | **0** | 3 | 1 | **0** |
| Ask-32 | **2** | 5 (2.50×) | 10 (4.75×) | 13 (6.50×) |
| AskParallel-32 | **2** | 4 (2.00×) | 9 (4.50×) | 12 (6.00×) |
| PingPong-32 | **0** | 8 | 2 | 3 |
| PingPongParallel-32 | **0** | 7 | 2 | 3 |
| PingPongPipelined-32 | **0** | 5 | 1 | **0** |
| ThreadRing-32 | **0** | 3 | 1 | 2 |
| ForkJoinThroughput-32 | **0** | 3 | **0** | **0** |
| Big-32 | **0** | 5 | 2 | 2 |
| Skynet-32 | 30200780 | 43295274 (1.43×) | **25814042 (0.85×)** | 38483276 (1.27×) |
| SpawnStop-32 | 25 | 41 (1.64×) | **20 (0.80×)** | 30 (1.20×) |

##### Live heap per idle actor

Lower is better.

| Benchmark | goakt | ergo | protoactor | hollywood |
|---|---:|---:|---:|---:|
| MemoryPerActor-32 | 1064 B | 2879 B (2.71×) | **616 B (0.58×)** | 27880 B (26.20×) |

</details>

#### Grains

GoAkt and Proto.Actor only, each in a single-node cluster.

##### Throughput

Higher is better.

| Benchmark | goakt | protoactor |
|---|---:|---:|
| GrainAsk-32 | **1.10M msgs/s** | 169K msgs/s (0.15×) |
| GrainAskParallel-32 | 2.25M msgs/s | **2.60M msgs/s (1.15×)** |
| GrainActivate-32 | 56K grains/s | **58K grains/s (1.02×)** |

<details>
<summary>Time, bytes and allocations per operation, and live heap per idle grain</summary>

##### Time per operation

Lower is better.

| Benchmark | goakt | protoactor |
|---|---:|---:|
| GrainAsk-32 | **910.5 ns** | 5.92 µs (6.51×) |
| GrainAskParallel-32 | 443.4 ns | **384.1 ns (0.87×)** |
| GrainActivate-32 | 17.79 µs | **17.37 µs (0.98×)** |

##### Bytes allocated per operation

Lower is better.

| Benchmark | goakt | protoactor |
|---|---:|---:|
| GrainAsk-32 | **0 B** | 1144 B |
| GrainAskParallel-32 | **0 B** | 1143 B |
| GrainActivate-32 | 8864 B | **3853 B (0.43×)** |

##### Allocations per operation

Lower is better.

| Benchmark | goakt | protoactor |
|---|---:|---:|
| GrainAsk-32 | **0** | 21 |
| GrainAskParallel-32 | **0** | 21 |
| GrainActivate-32 | 123 | **66 (0.54×)** |

##### Live heap per idle grain

Lower is better.

| Benchmark | goakt | protoactor |
|---|---:|---:|
| GrainMemoryPerGrain-32 | **399 B** | 1624 B (4.07×) |

</details>

### 64 cores (`c7a.16xlarge`)

Full output, with the spread and p-value of every row: [RESULTS-2026-11-01-c7a-64.md](./RESULTS-2026-11-01-c7a-64.md).

#### Actors

##### Throughput

Higher is better.

| Benchmark | goakt | ergo | protoactor | hollywood |
|---|---:|---:|---:|---:|
| Tell-64 | 14.39M msgs/s | 2.45M msgs/s (0.17×) | 11.95M msgs/s (0.83×) | **17.38M msgs/s (1.21×)** |
| TellParallel-64 | **5.89M msgs/s** | 2.15M msgs/s (0.37×) | 5.58M msgs/s (0.95×) | 4.20M msgs/s (0.71×) |
| TellFromActor-64 | 10.35M msgs/s | 2.29M msgs/s (0.22×) | 7.33M msgs/s (0.71×) | **17.91M msgs/s (1.73×)** |
| Ask-64 | **960K msgs/s** | 218K msgs/s (0.23×) | 248K msgs/s (0.26×) | 515K msgs/s (0.54×) |
| AskParallel-64 | **1.32M msgs/s** | 764K msgs/s (0.58×) | 571K msgs/s (0.43×) | 322K msgs/s (0.24×) |
| PingPong-64 | **7.88M msgs/s** | 244K msgs/s (0.03×) | 415K msgs/s (0.05×) | 3.70M msgs/s (0.47×) |
| PingPongParallel-64 | **180.10M msgs/s** | 7.65M msgs/s (0.04×) | 16.80M msgs/s (0.09×) | 11.65M msgs/s (0.06×) |
| PingPongPipelined-64 | **602.00M msgs/s** | 17.72M msgs/s (0.03×) | 166.17M msgs/s (0.28×) | 12.23M msgs/s (0.02×) |
| ThreadRing-64 | **7.85M msgs/s** | 231K msgs/s (0.03×) | 417K msgs/s (0.05×) | 3.00M msgs/s (0.38×) |
| ForkJoinThroughput-64 | **8.02M msgs/s** | 759K msgs/s (0.09×) | 3.76M msgs/s (0.47×) | 2.41M msgs/s (0.30×) |
| Big-64 | **22.25M msgs/s** | 8.84M msgs/s (0.40×) | 16.43M msgs/s (0.74×) | 10.90M msgs/s (0.49×) |
| Skynet-64 | 436K actors/s | 893K actors/s (2.05×) | **2.22M actors/s (5.10×)** | 48K actors/s (0.11×) |
| SpawnStop-64 | 141K actors/s | 130K actors/s (0.92×) | **186K actors/s (1.32×)** | 138K actors/s (0.98×) |

<details>
<summary>Time, bytes and allocations per operation, and live heap per idle actor</summary>

##### Time per operation

Lower is better.

| Benchmark | goakt | ergo | protoactor | hollywood |
|---|---:|---:|---:|---:|
| Tell-64 | 69.5 ns | 408.9 ns (5.89×) | 83.7 ns (1.20×) | **57.6 ns (0.83×)** |
| TellParallel-64 | **169.8 ns** | 465.4 ns (2.74×) | 179.3 ns (1.06×) | 238.1 ns (1.40×) |
| TellFromActor-64 | 96.6 ns | 436.4 ns (4.52×) | 136.3 ns (1.41×) | **55.9 ns (0.58×)** |
| Ask-64 | **1.04 µs** | 4.59 µs (4.41×) | 4.03 µs (3.87×) | 1.94 µs (1.86×) |
| AskParallel-64 | **758.1 ns** | 1.31 µs (1.73×) | 1.75 µs (2.31×) | 3.11 µs (4.10×) |
| PingPong-64 | **253.8 ns** | 8.21 µs (32.35×) | 4.81 µs (18.97×) | 540.5 ns (2.13×) |
| PingPongParallel-64 | **11.1 ns** | 261.6 ns (23.55×) | 119.1 ns (10.72×) | 171.7 ns (15.46×) |
| PingPongPipelined-64 | **3.3 ns** | 112.9 ns (33.99×) | 12.0 ns (3.62×) | 163.6 ns (49.23×) |
| ThreadRing-64 | **127.3 ns** | 4.33 µs (34.05×) | 2.40 µs (18.86×) | 333.2 ns (2.62×) |
| ForkJoinThroughput-64 | **124.7 ns** | 1.32 µs (10.58×) | 265.7 ns (2.13×) | 415.4 ns (3.33×) |
| Big-64 | **89.9 ns** | 226.1 ns (2.52×) | 121.8 ns (1.36×) | 183.4 ns (2.04×) |
| Skynet-64 | 2551.00 ms | 1244.44 ms (0.49×) | **502.18 ms (0.20×)** | 22972.01 ms (9.01×) |
| SpawnStop-64 | 7.11 µs | 7.71 µs (1.09×) | **5.38 µs (0.76×)** | 7.27 µs (1.02×) |

##### Bytes allocated per operation

Lower is better.

| Benchmark | goakt | ergo | protoactor | hollywood |
|---|---:|---:|---:|---:|
| Tell-64 | **16 B** | 90 B (5.62×) | 40 B (2.50×) | 25 B (1.56×) |
| TellParallel-64 | **1 B** | 150 B (150.00×) | 50 B (50.00×) | 25 B (25.00×) |
| TellFromActor-64 | 32 B | 89 B (2.74×) | 32 B (0.98×) | **25 B (0.77×)** |
| Ask-64 | **128 B** | 169 B (1.32×) | 512 B (4.00×) | 703 B (5.49×) |
| AskParallel-64 | **128 B** | 190 B (1.48×) | 496 B (3.88×) | 699 B (5.46×) |
| PingPong-64 | **0 B** | 208 B | 64 B | 79 B |
| PingPongParallel-64 | **0 B** | 176 B | 63 B | 80 B |
| PingPongPipelined-64 | **0 B** | 151 B | 32 B | 53 B |
| ThreadRing-64 | **0 B** | 72 B | 16 B | 40 B |
| ForkJoinThroughput-64 | **0 B** | 104 B | 5 B | 32 B |
| Big-64 | **0 B** | 129 B | 47 B | 64 B |
| Skynet-64 | 2864590332 B | 4585201388 B (1.60×) | **1187611518 B (0.41×)** | 32134681804 B (11.22×) |
| SpawnStop-64 | 1928 B | 4331 B (2.25×) | **1000 B (0.52×)** | 28735 B (14.90×) |

##### Allocations per operation

Lower is better.

| Benchmark | goakt | ergo | protoactor | hollywood |
|---|---:|---:|---:|---:|
| Tell-64 | **0** | 3 | **0** | **0** |
| TellParallel-64 | **0** | 3 | **0** | **0** |
| TellFromActor-64 | **0** | 3 | 1 | **0** |
| Ask-64 | **2** | 5 (2.50×) | 10 (4.75×) | 12 (6.00×) |
| AskParallel-64 | **2** | 4 (2.00×) | 9 (4.50×) | 12 (6.00×) |
| PingPong-64 | **0** | 8 | 3 | 3 |
| PingPongParallel-64 | **0** | 7 | 2 | 3 |
| PingPongPipelined-64 | **0** | 5 | 1 | **0** |
| ThreadRing-64 | **0** | 3 | 1 | 2 |
| ForkJoinThroughput-64 | **0** | 3 | **0** | 1 |
| Big-64 | **0** | 5 | 1 | 2 |
| Skynet-64 | 30169812 | 43234178 (1.43×) | **25791806 (0.85×)** | 38646518 (1.28×) |
| SpawnStop-64 | 25 | 41 (1.64×) | **20 (0.80×)** | 30 (1.20×) |

##### Live heap per idle actor

Lower is better.

| Benchmark | goakt | ergo | protoactor | hollywood |
|---|---:|---:|---:|---:|
| MemoryPerActor-64 | 1065 B | 2879 B (2.70×) | **616 B (0.58×)** | 27880 B (26.18×) |

</details>

#### Grains

GoAkt and Proto.Actor only, each in a single-node cluster.

##### Throughput

Higher is better.

| Benchmark | goakt | protoactor |
|---|---:|---:|
| GrainAsk-64 | **961K msgs/s** | 170K msgs/s (0.18×) |
| GrainAskParallel-64 | 1.81M msgs/s | **2.63M msgs/s (1.46×)** |
| GrainActivate-64 | 54K grains/s | **56K grains/s (1.03×)** |

<details>
<summary>Time, bytes and allocations per operation, and live heap per idle grain</summary>

##### Time per operation

Lower is better.

| Benchmark | goakt | protoactor |
|---|---:|---:|
| GrainAsk-64 | **1.04 µs** | 5.90 µs (5.67×) |
| GrainAskParallel-64 | 553.8 ns | **379.8 ns (0.69×)** |
| GrainActivate-64 | 18.50 µs | **17.89 µs (0.97×)** |

##### Bytes allocated per operation

Lower is better.

| Benchmark | goakt | protoactor |
|---|---:|---:|
| GrainAsk-64 | **0 B** | 1144 B |
| GrainAskParallel-64 | **0 B** | 1143 B |
| GrainActivate-64 | 8880 B | **3854 B (0.43×)** |

##### Allocations per operation

Lower is better.

| Benchmark | goakt | protoactor |
|---|---:|---:|
| GrainAsk-64 | **0** | 21 |
| GrainAskParallel-64 | **0** | 21 |
| GrainActivate-64 | 123 | **66 (0.54×)** |

##### Live heap per idle grain

Lower is better.

| Benchmark | goakt | protoactor |
|---|---:|---:|
| GrainMemoryPerGrain-64 | **398 B** | 1620 B (4.08×) |

</details>

## Running

All commands run from this directory. The only requirement is Go.

```bash
make bench                  # benchmark every framework, then compare them
make bench-goakt            # benchmark a single framework (goakt, ergo, protoactor, hollywood)
make compare                # re-print the benchstat comparison from saved results
make table                  # print the saved actor results as Markdown comparison tables
make table-grains           # print the saved grain results (goakt, protoactor) as Markdown tables
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

`make table` renders the same results as the Markdown tables in [Results](#results), using [cmd/table](./cmd/table). It leaves out the grain benchmarks; `make table-grains` renders those for GoAkt and Proto.Actor only.

You can also run a package directly with `go test`:

```bash
go test -run '^$' -bench . -benchmem ./protoactor
```

## Adding a framework

1. `go get` the framework at a pinned version, then `make vendor`.
2. Create a package named after it, holding a single `<framework>_test.go` file.
3. Start the framework once in `TestMain`, and implement the fourteen benchmarks above under the same names, using the messages, sizes, `Latch`, `Signal` and Skynet helpers from `internal/bench`.
4. Add the package name to `FRAMEWORKS` in the [Makefile](./Makefile).
5. If the framework has grains (virtual actors), implement the four grain benchmarks as well, and add the package name to `GRAIN_FRAMEWORKS`.
