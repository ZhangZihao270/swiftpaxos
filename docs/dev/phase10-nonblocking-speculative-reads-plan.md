# Phase 10 Plan: Non-Blocking Speculative Reads

## Overview

Make same-client dependent reads truly 1-RTT by computing speculative results without blocking on causal dependencies.

## Problem

**Current Behavior**: When a weak read depends on an uncommitted weak write from the same client, the read blocks waiting for the write to be committed and executed before computing the speculative result.

```go
// Current: R1 blocks waiting for W1 to execute
func (r *Replica) handleWeakPropose(propose *MWeakPropose) {
    if propose.CausalDep > 0 {
        r.waitForWeakDep(propose.ClientId, propose.CausalDep)  // BLOCKS!
    }
    desc.val = propose.Command.ComputeResult(r.State)  // Only then compute result
}
```

**Impact**: Same-client read-after-write has latency = write's commit time + read's processing, not 1 RTT.

## Solution

Track pending (uncommitted) writes per client. When computing speculative result for a read, check pending writes first before falling back to committed state.

## Implementation Tasks

### Task 10.1: Add pending writes tracking structure

Add to Replica struct:
```go
// pendingWrites tracks uncommitted writes per client
// Structure: clientId -> key -> pendingWrite
pendingWrites cmap.ConcurrentMap
```

Add pendingWrite struct:
```go
type pendingWrite struct {
    seqNum int32
    value  state.Value
}
```

### Task 10.2: Track pending writes in handleWeakPropose

When weak PUT arrives:
```go
if propose.Command.Op == state.PUT {
    r.addPendingWrite(propose.ClientId, propose.Command.K, propose.CommandId, propose.Command.V)
}
```

### Task 10.3: Clean up pending writes after commit

In asyncReplicateWeak, after Execute():
```go
if desc.cmd.Op == state.PUT {
    r.removePendingWrite(clientId, desc.cmd.K, seqNum)
}
```

### Task 10.4: Implement computeSpeculativeResult

```go
func (r *Replica) computeSpeculativeResult(clientId int32, causalDep int32, cmd state.Command) state.Value {
    if cmd.Op == state.GET {
        // Check pending writes from this client for this key
        if pending := r.getPendingWrite(clientId, cmd.K, causalDep); pending != nil {
            return pending.value
        }
    }
    // Fall back to committed state
    return cmd.ComputeResult(r.State)
}
```

### Task 10.5: Remove blocking waitForWeakDep

Replace in handleWeakPropose:
```go
// OLD (blocking):
if propose.CausalDep > 0 {
    r.waitForWeakDep(propose.ClientId, propose.CausalDep)
}
desc.val = propose.Command.ComputeResult(r.State)

// NEW (non-blocking):
desc.val = r.computeSpeculativeResult(propose.ClientId, propose.CausalDep, propose.Command)
// Keep waitForWeakDep only in asyncReplicateWeak for execution ordering
```

### Task 10.6: Handle SCAN with pending writes

For SCAN operations, need to merge pending writes with committed state:
```go
func (r *Replica) computeSpeculativeResultScan(clientId int32, causalDep int32, cmd state.Command) state.Value {
    // Get committed values
    result := cmd.ComputeResult(r.State)

    // Overlay any pending writes in range
    // This is complex - may need separate implementation
}
```

## Testing

### Task 10.7: Same-client read-after-write
- W1 = PUT(k, "A"), R1 = GET(k) with CausalDep=W1
- R1 should return "A" immediately without blocking

### Task 10.8: Pending writes cleanup
- Verify pendingWrites map is cleared after execution

### Task 10.9: Cross-client isolation
- Client A's pending writes invisible to Client B's reads

## Estimated LOC

- Task 10.1: ~20 lines (struct + initialization)
- Task 10.2: ~15 lines (addPendingWrite)
- Task 10.3: ~15 lines (removePendingWrite)
- Task 10.4: ~25 lines (computeSpeculativeResult)
- Task 10.5: ~10 lines (modify handleWeakPropose)
- Task 10.6: ~40 lines (SCAN handling)
- Task 10.7-10.9: ~100 lines (tests)

Total: ~225 lines (well within 500 LOC limit)
