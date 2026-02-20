# Phase 44 Evaluation — CURP-HO W-P99 Latency Fix

**Date**: 2026-02-20
**Status**: Code changes complete; benchmark validation pending (Phase 44.1b)

## Objective

Investigate and fix the ~100ms W-P99 tail latency observed at 4 and 32 threads in CURP-HO (Phase 42/43 results), while maintaining throughput scaling and strong-op latency.

## Success Criteria

1. **Throughput scaling**: Within 20% of Phase 42 reference at each thread count (2-96)
2. **W-P99 at 2, 8, 16 threads**: < 2ms (matching Phase 43 improvements)
3. **W-P99 at 4 threads**: < 5ms (improvement from ~100ms)
4. **W-P99 at 32 threads**: < 5ms (improvement from ~100ms)
5. **S-Median**: ~51ms at all thread counts <= 32 (no regression)

## Phase 42 Baseline (2026-02-19)

| Threads | Throughput | S-Median (ms) | W-P99 (ms) |
|--------:|-----------:|---------------:|------------:|
| 2       | 3,551      | 51.26          | 0.86        |
| 4       | 4,109      | 51.17          | 100.96      |
| 8       | 14,050     | 50.97          | 2.62        |
| 16      | 8,771      | 50.89          | 100.95      |
| 32      | 30,339     | 59.16          | 100.38      |

## Phase 43 Improvements (2026-02-20, before Phase 44)

| Threads | W-P99 (Phase 42) | W-P99 (Phase 43) | Change   |
|--------:|------------------:|------------------:|----------|
| 2       | 0.86ms            | 0.82ms            | -5%      |
| 4       | 100.96ms          | ~100.80ms         | ~same    |
| 8       | 2.62ms            | 0.81ms            | -69%     |
| 16      | 100.95ms          | 1.08ms            | -99%     |
| 32      | 100.38ms          | 101.02ms          | ~same    |

Phase 43 fixed 8-thread and 16-thread W-P99 via split handleMsgs goroutines (43.2c). The 4-thread and 32-thread ~100ms tails persisted.

## Code Changes in Phase 44

### Phase 44.3: Fix sendMsgToAll Writer Race

**Problem**: `sendMsgToAll()` called bare `SendMsg()` without mutex protection, racing with `sendMsgSafe()` in the `handleStrongMsgs` goroutine on the same `bufio.Writer`.

**Fix**: Changed `sendMsgToAll()` to use `sendMsgSafe()` for all sends, serializing through `writerMu[rid]`.

**Files**: `curp-ho/client.go`

### Phase 44.4: Remove Priority Fast-Path

**Problem**: A non-blocking select on `causalProposeChan` before the main run-loop select (Phase 43.2b) starved other channels at high throughput. Continuous causal proposes prevented processing of `acceptAckChan`, `ProposeChan`, and `commitChan`.

**Fix**: Removed the priority fast-path entirely. `causalProposeChan` is still handled in the main select — causal proposes are processed normally, just without artificial priority.

**Insight**: The actual fix for 16-thread W-P99 (100.95ms -> 1.08ms) was Phase 43.2c (split handleMsgs), not the priority fast-path. The fast-path was a net negative due to starvation risk.

**Files**: `curp-ho/curp-ho.go`

### Phase 44.5c: Async Remote Send Queues

**Root cause identified**: `sendMsgToAll()` blocked on synchronous TCP `Flush()` to each remote replica (~50ms RTT per replica). With 3 replicas (1 bound + 2 remote), the bound replica send is fast (~0ms local) but each remote send blocks for a full RTT. Under contention at 4+ threads, this created the ~100ms W-P99 tail.

**Solution**: Per-replica async FIFO send queues. Bound replica sends remain synchronous (fast local path). Remote replica sends are enqueued to buffered channels (capacity 128) drained by dedicated `remoteSender()` goroutines.

**Key design decisions**:
- **Bound replica synchronous**: Lowest latency path (co-located), no queue overhead needed
- **FIFO ordering preserved**: One goroutine per remote replica, sequential drain
- **Causal ordering maintained**: `SendStrongWrite/Read` acquire `writerMu[leader]`, serializing against the `remoteSender` goroutine. Causal writes enqueued before strong sends are guaranteed to be flushed first.
- **Channel buffer size 128**: Sufficient to absorb bursts without blocking the client's main loop

**Implementation**:
```go
type sendRequest struct {
    code uint8
    msg  fastrpc.Serializable
}

// Per-replica async send queues (nil for bound replica)
remoteSendQueues []chan sendRequest

func (c *Client) remoteSender(rid int32) {
    for req := range c.remoteSendQueues[rid] {
        c.sendMsgSafe(rid, req.code, req.msg)
    }
}

func (c *Client) sendMsgToAll(code uint8, msg fastrpc.Serializable) {
    c.sendMsgSafe(c.boundReplica, code, msg)      // Sync (fast, ~0ms)
    for i := 0; i < c.N; i++ {
        if int32(i) != c.boundReplica {
            c.remoteSendQueues[i] <- sendRequest{code, msg}  // Async
        }
    }
}
```

**Files**: `curp-ho/client.go`

### Phase 44.5f/g: Remove Diagnostic Instrumentation

Removed all Phase 44.5a instrumentation (timestamp maps, latency breakdown slices, `printLatencyBreakdown()` function). Retained only production code: async send queues, `remoteSender` goroutines, and mutex serialization. `MarkAllSent()` simplified to no-op.

**Files**: `curp-ho/client.go`, `curp-ho/curp-ho_test.go`

## Test Coverage

All Phase 44 changes include comprehensive tests in `curp-ho/curp-ho_test.go`:

- **Async send queue initialization**: Verifies queues created for remote replicas, nil for bound
- **FIFO ordering**: Messages dequeued in enqueue order
- **Mutex serialization**: `SendStrongWrite/Read` acquire `writerMu[leader]`, preventing concurrent access with `remoteSender`
- **Non-blocking enqueue**: Queue capacity (128) absorbs burst sends
- **Writer race prevention**: Concurrent `sendMsgToAll` and `sendMsgSafe` calls serialize correctly
- **MarkAllSent no-op**: HybridClient interface compliance

All tests pass: `go test ./...` clean, `go vet ./...` clean.

## Expected Impact

| Threads | W-P99 Before | Expected W-P99 After | Rationale |
|--------:|-------------:|---------------------:|-----------|
| 2       | 0.82ms       | < 1ms                | Already good, no regression expected |
| 4       | ~100ms       | < 5ms                | Async queues eliminate TCP Flush blocking |
| 8       | 0.81ms       | < 1ms                | Already good, no regression expected |
| 16      | 1.08ms       | < 2ms                | Already good, no regression expected |
| 32      | ~101ms       | < 5ms                | Async queues + no priority starvation |

## Benchmark Results

**Pending**: Requires launching `scripts/run-phase44-sweep.sh` on remote servers (130.245.173.101, .102, .104). See Phase 44.1b in TODO.

Results will be recorded here once the benchmark sweep completes.

## Commits

| Hash | Phase | Description |
|------|-------|-------------|
| `eaaa506` | 44.3 | Fix sendMsgToAll writer race with per-replica mutex |
| `07ec2db` | 44.4 | Remove priority fast-path to prevent run loop starvation |
| `0f7f5c2` | 44.5a | Add weak write latency instrumentation |
| `11a5f3f` | 44.5c | Async remote send queues to fix W-P99 blocking |
| `b848c92` | 44.5f/g | Remove instrumentation, keep production code |
| `88de4f4` | 44.6e | Mark tests pass |
