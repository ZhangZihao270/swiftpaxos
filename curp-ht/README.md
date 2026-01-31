# CURP-HT: Running and Evaluation Guide

This guide explains how to run CURP-HT (CURP with Hybrid Transparency) and evaluate it using the hybrid consistency benchmark.

## Prerequisites

- Go 1.18 or later
- Multiple terminals (for single-machine testing) or multiple machines (for distributed evaluation)

## Building

```bash
cd /path/to/swiftpaxos
go build -o swiftpaxos .
```

## Architecture Overview

CURP-HT requires three types of participants:

| Participant | Description | Minimum Count |
|-------------|-------------|---------------|
| **Master** | Coordinates replica discovery | 1 |
| **Replica** | Stores data, processes commands | 3 (for fault tolerance) |
| **Client** | Sends commands, runs benchmark | 1+ |

## Quick Start: Single Machine Testing

For development and testing, you can run all participants on localhost with different ports.

### Step 1: Create a Local Configuration File

Create `local.conf`:

```
-- Replicas --
replica0 127.0.0.1
replica1 127.0.0.1
replica2 127.0.0.1

-- Clients --
client0 127.0.0.1

-- Master --
master0 127.0.0.1

masterPort: 7087

protocol: curpht

// Replica settings
noop:       false
thrifty:    false
fast:       true

// Client settings
reqs:        100
writes:      50
conflicts:   0
commandSize: 100
clones:      0

// Hybrid benchmark settings
weakRatio:   50
weakWrites:  50

-- Proxy --
server_alias replica0
client0 (local)
---
```

**Note**: The `-- Proxy --` section is required. It maps each client to its designated replica server.

### Step 2: Start the Master

```bash
./swiftpaxos -run master -config local.conf -alias master0
```

### Step 3: Start Replicas (in separate terminals)

Terminal 1:
```bash
./swiftpaxos -run server -config local.conf -alias replica0
```

Terminal 2:
```bash
./swiftpaxos -run server -config local.conf -alias replica1
```

Terminal 3:
```bash
./swiftpaxos -run server -config local.conf -alias replica2
```

### Step 4: Run the Client/Benchmark

```bash
./swiftpaxos -run client -config local.conf -alias client0
```

## Distributed Deployment

For real evaluation across multiple machines:

### Step 1: Create a Distributed Configuration

Example `distributed.conf`:

```
-- Replicas --
replica0 192.168.1.10
replica1 192.168.1.11
replica2 192.168.1.12

-- Clients --
client0 192.168.1.20
client1 192.168.1.21

-- Master --
master0 192.168.1.10

masterPort: 7087

protocol: curpht

// Replica settings
noop:       false
thrifty:    false
fast:       true

// Client settings
reqs:        10000
writes:      100
conflicts:   2
commandSize: 1000
clones:      0

// Hybrid benchmark settings
weakRatio:   50
weakWrites:  50
```

### Step 2: Deploy and Run

1. Copy the binary and config to all machines
2. Start Master on the master machine
3. Start Replicas on each replica machine
4. Start Clients on client machines

## Hybrid Benchmark Configuration

### Key Parameters

| Parameter | Description | Range | Default |
|-----------|-------------|-------|---------|
| `weakRatio` | Percentage of commands using weak consistency | 0-100 | 0 |
| `weakWrites` | Percentage of weak commands that are writes | 0-100 | 50 |
| `writes` | Percentage of strong commands that are writes | 0-100 | 100 |
| `reqs` | Number of requests per client | 1+ | 1000 |
| `conflicts` | Conflict percentage (hot key access) | 0-100 | 2 |
| `commandSize` | Payload size in bytes | 1+ | 1000 |
| `clones` | Number of client clones (parallel clients) | 0+ | 0 |

### Example Workload Configurations

#### All Strong Commands (Baseline)
```
weakRatio:   0
writes:      100
```
All commands are linearizable writes. Use this to compare against standard CURP.

#### All Weak Commands
```
weakRatio:   100
weakWrites:  50
```
All commands use causal consistency. 50% writes, 50% reads.

#### Hybrid 50/50
```
weakRatio:   50
writes:      100
weakWrites:  50
```
Half strong (all writes), half weak (50% writes, 50% reads).

#### Strong Writes, Weak Reads
```
weakRatio:   80
writes:      100
weakWrites:  0
```
20% strong writes (linearizable), 80% weak reads (causal, 1-RTT).

#### Read-Heavy Workload
```
weakRatio:   90
writes:      0
weakWrites:  10
```
10% strong reads, 81% weak reads, 9% weak writes.

## Understanding the Output

### Benchmark Results

When the benchmark completes, you'll see output like:

```
=== Hybrid Benchmark Results ===
Total operations: 10000
Duration: 30.52s
Throughput: 327.65 ops/sec

Strong Operations: 5000 (50.0%)
  Writes: 5000 | Reads: 0
  Median latency: 45.23ms | P99: 89.31ms | P99.9: 112.45ms

Weak Operations: 5000 (50.0%)
  Writes: 2500 | Reads: 2500
  Median latency: 12.15ms | P99: 28.76ms | P99.9: 35.82ms
================================
```

### Metrics Explanation

| Metric | Description |
|--------|-------------|
| **Total operations** | Number of commands completed (excluding warmup) |
| **Duration** | Total benchmark time |
| **Throughput** | Operations per second |
| **Strong Operations** | Count and percentage of linearizable commands |
| **Weak Operations** | Count and percentage of causal commands |
| **Median latency** | 50th percentile latency in milliseconds |
| **P99** | 99th percentile latency |
| **P99.9** | 99.9th percentile latency |

### Expected Latency Characteristics

| Command Type | Expected Latency | Reason |
|--------------|------------------|--------|
| Strong Write | ~2 RTT | Requires replication to majority |
| Strong Read | ~2 RTT | Requires replication to majority |
| Weak Write | ~1 RTT | Leader executes speculatively, replicates async |
| Weak Read | ~1 RTT | Leader returns speculative result immediately |

## Running Evaluation Experiments

### Experiment 1: Varying Weak Ratio

Compare performance as you increase the percentage of weak commands:

```bash
# Create configs with different weakRatio values
for ratio in 0 25 50 75 100; do
    sed "s/weakRatio:.*/weakRatio: $ratio/" local.conf > "config_weak${ratio}.conf"
    ./swiftpaxos -run client -config "config_weak${ratio}.conf" -alias client0
done
```

### Experiment 2: Varying Conflict Rate

Test how conflicts affect performance:

```bash
for conflict in 0 10 25 50; do
    sed "s/conflicts:.*/conflicts: $conflict/" local.conf > "config_conflict${conflict}.conf"
    ./swiftpaxos -run client -config "config_conflict${conflict}.conf" -alias client0
done
```

### Experiment 3: Varying Client Count

Test scalability with multiple clients:

```bash
for clones in 0 4 9 19; do
    sed "s/clones:.*/clones: $clones/" local.conf > "config_clones${clones}.conf"
    ./swiftpaxos -run client -config "config_clones${clones}.conf" -alias client0
done
```

## Troubleshooting

### Common Issues

1. **"connection refused"**: Ensure Master and Replicas are running before starting Client.

2. **"no leader"**: Wait for replicas to elect a leader (usually a few seconds).

3. **Timeout errors**: Check network connectivity between machines.

4. **Low throughput**: Increase `reqs` count for more accurate measurements.

### Debug Logging

Add `-log /path/to/logfile` to enable logging:

```bash
./swiftpaxos -run server -config local.conf -alias replica0 -log replica0.log
```

## Protocol Details

### Strong Commands (Linearizable)
1. Client sends `Propose` to all replicas
2. Leader assigns slot, computes speculative result
3. Leader sends `MReply` with speculative result
4. Leader replicates to majority (Accept/AcceptReply)
5. After commit: Execute in slot order, send `MSyncReply`

### Weak Commands (Causal)
1. Client sends `MWeakPropose` to leader only
2. Leader assigns slot, computes speculative result
3. Leader sends `MWeakReply` immediately (1 RTT complete for client)
4. Leader asynchronously replicates (Accept/AcceptReply)
5. After commit: Execute in slot order (for durability)

### Causal Ordering
- Same-client weak commands track `CausalDep` for session ordering
- Pending writes are tracked for non-blocking read-after-write
- Slot ordering ensures global consistency for state machine

## Files Reference

| File | Description |
|------|-------------|
| `curp-ht/curp-ht.go` | Main replica protocol implementation |
| `curp-ht/client.go` | Client implementation with weak commands |
| `curp-ht/defs.go` | Message definitions and constants |
| `client/hybrid.go` | Hybrid benchmark framework |
| `config/config.go` | Configuration parsing |
| `main.go` | Entry point and client/server initialization |
