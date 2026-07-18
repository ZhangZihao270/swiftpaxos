Hybrid-Consistency State-Machine Replication
============================================

This repository is a research prototype for **hybrid-consistency** state-machine
replication: protocols that let a single system serve both *strong* (linearizable)
and *weak* (causal) commands, so applications pay the coordination cost of strong
consistency only for the operations that require it.

It contains a family of hybrid protocols (CURP-HT/HO, EPaxos-HO, Raft-HT,
Pileus/Pileus-HT, MongoDB-Tunable), plus the vanilla consensus protocols (CURP,
EPaxos, Raft) they are compared against.

> **Provenance.** This project is built on top of
> [SwiftPaxos](https://github.com/imdea-software/swiftpaxos) (NSDI '24), whose code
> base in turn derives from [Egalitarian Paxos](https://github.com/otrack/epaxos).
> The Go module path is still `github.com/imdea-software/swiftpaxos`, and the
> original SwiftPaxos participant/config/master machinery is reused as the harness
> for all protocols here. Our contribution is the set of hybrid-consistency
> protocols and the benchmark/evaluation tooling around them.

Hybrid consistency in one paragraph
-----------------------------------

A history is *hybrid-consistent* when there exist two orderings over the operations:
a partial causal order `≺_P` over **all** operations, and a total linearizable order
`≺_T` over a subset `O_T` that contains **every strong operation** (weak operations
are pulled into `O_T` only when a strong op reads from them). Weak commands commit
fast on the causal path; strong commands go through full consensus and respect
real-time order. See `docs/` and the design notes for the full formal model and the
per-protocol implementation conditions.

Implemented protocols
---------------------

Selected with the `protocol:` field in the config (or the `-protocol` flag). Each
protocol lives in its own package.

### Hybrid-consistency protocols (this project)

| `protocol` value | Directory     | Description                                                                 |
|------------------|---------------|-----------------------------------------------------------------------------|
| `curpht`         | `curp-ht/`    | **CURP-HT** — CURP with Hybrid Transparency |
| `curpho`         | `curp-ho/`    | **CURP-HO** — Hybrid-Optimal CURP|
| `epaxosho`       | `epaxos-ho/`  | **EPaxos-HO** — leaderless Hybrid-Optimal EPaxos |
| `raftht`         | `raft-ht/`    | **Raft-HT** — Raft with Hybrid Transparency              |
| `pileus`         | `pileus/`     | **Pileus** — Weak-read only   |
| `pileusht`       | `pileusht/`   | **Pileus-HT** — Pileus with fast weak writes.                               |
| `mongotunable`   | `mongotunable/` | **MongoDB-Tunable** — tunable read/write concern over Raft-HT.             |

Each protocol package follows the same layout: `<name>.go` (replica logic),
`client.go`, `defs.go` (message types + marshalling), and where relevant
`batcher.go`, `timer.go`, `exec.go`.

Code structure
--------------

```
main.go            Client entry point + protocol → client dispatch
run.go             Replica entry point + protocol → replica dispatch
config/            Deployment config parsing
replica/           Base replica (replica.New) + MsgSet (mset.go) shared by all protocols
client/            Base client (BufferClient); hybrid clients wrap HybridBufferClient
master/            Master: holds/serves the cluster configuration
rpc/               RPC registration
state/             Key-value state machine executed by replicas
dlog/  hook/       Logging and hooks

<protocol dirs>    One package per protocol (see tables above)

configs/           Base config files for the evaluation experiments (exp1.1 … exp-tao)
scripts/           Experiment driver scripts (eval-*.sh) + plotting (plot-*.py)
evaluation/        Result data, analysis notes, and generated plots
docs/              Protocol flows, verification notes, and phase-by-phase design docs
results/           Raw benchmark output
tla/               TLA+ specifications
```

The two dispatch points are the source of truth for which protocols exist:
`run.go` wires each `protocol:` value to a replica, and `main.go` wires it to a client.

Building
--------

Requires Go 1.20+.

```bash
git clone <this-repo>
cd swiftpaxos
go build -o swiftpaxos .
```

Running
-------

There are three participant roles: a **master** (holds the cluster configuration),
**servers/replicas** (run the protocol), and **clients** (issue the workload). Every
participant reads the same deployment config file and identifies itself with an alias.

```bash
# Master
./swiftpaxos -run master -config local-5r.conf

# Each replica (alias must match a name under "-- Replicas --")
./swiftpaxos -run server -config local-5r.conf -alias replica0

# Each client (alias must match a name under "-- Clients --")
./swiftpaxos -run client -config local-5r.conf -alias client0
```

Common flags:

| Flag           | Meaning                                                      |
|----------------|-------------------------------------------------------------|
| `-run`         | `master` \| `server` \| `client`                            |
| `-config`      | Deployment config file (**required**)                       |
| `-alias`       | This participant's name (must appear in the config)         |
| `-protocol`    | Override the config's `protocol:` field                     |
| `-log`         | Path to the log file                                        |
| `-latency`     | Latency config file                                         |
| `-quorum`      | Quorum config file                                          |

For local single-machine runs there are helper scripts (`scripts/run-local.sh`,
`scripts/run-local-multi.sh`) and ready-made configs (`local-5r.conf`,
`eval-local.conf`). Run them from the repo root, e.g. `bash scripts/run-local.sh`.
All shell scripts (runners, benchmarks, and per-experiment drivers) live under
`scripts/`.

Configuration
-------------

A config file lists the replicas, clients, and master (with IPs), then a set of
key/value parameters. Example (`local-5r.conf`):

```
-- Replicas --
replica0 127.0.1.1
...
-- Clients --
client0 127.0.1.6
-- Master --
master0 127.0.1.1
masterPort: 7287

protocol: curpht

// Replica settings
noop:    false
thrifty: false
fast:    true

// Client / workload settings
reqs:        5000     // requests per client thread
writes:      10       // % of strong ops that are writes
commandSize: 100      // command payload bytes
conflicts:   0        // % conflicting commands

// Hybrid workload
weakRatio:   50       // % of commands issued as weak (causal); 0 = pure strong baseline
weakWrites:  10       // % of weak commands that are writes (rest are reads)
```

### Key workload parameters

| Parameter        | Meaning                                                                 | Default |
|------------------|-------------------------------------------------------------------------|---------|
| `weakRatio`      | Percentage of commands issued as weak/causal (0–100). `0` = strong-only baseline. | 0 |
| `weakWrites`     | Percentage of weak commands that are writes (rest reads).               | 50      |
| `writes`         | Percentage of strong commands that are writes.                          | —       |
| `reqs`           | Requests per client thread.                                             | —       |
| `commandSize`    | Command payload size in bytes.                                          | 100     |
| `conflicts`      | Percentage of conflicting commands (for leaderless protocols).          | 0       |
| `clientThreads`  | Client threads per process (overrides `clones` when > 0).               | 0       |
| `pipeline`/`pendings` | Enable client pipelining and its depth.                            | —       |
| `keySpace`       | Number of unique keys (> 0 enables a shared key space).                 | 0       |
| `zipfSkew`       | Zipf skew for key selection (values ≤ 1 clamp to 1.01; 0 = uniform).    | 0       |
| `networkDelay`   | Simulated one-way network delay in ms (RTT = 2×).                       | 0       |
| `batchDelayUs`   | Server-side batching delay (µs).                                        | —       |
| `maxDescRoutines`| Descriptor-processing goroutine pool size.                             | —       |

The benchmark reports latency (median / P99 / P99.9) and throughput broken down
**separately for strong and weak operations**, plus totals.

License
-------

See [LICENSE](LICENSE). SwiftPaxos and EPaxos retain their original licenses.
