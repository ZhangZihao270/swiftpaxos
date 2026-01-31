# Phase 6 Plan: Causal Ordering

## Overview
Implement causal ordering for weak commands to ensure that operations from the
same client are executed in order.

## Design

### Causal Consistency Guarantees
1. **Session Order**: Operations from the same client are executed in order
2. **Read-Your-Writes**: A read sees all prior writes from the same client
3. **Monotonic Reads**: Successive reads see monotonically increasing state

### Implementation Approach
- Each weak command carries a dependency on the previous weak command from the same client
- The leader waits for the dependency to be executed before processing the new command

## Task 6.1: Add CausalDep Field to MWeakPropose

### Changes to defs.go
```go
type MWeakPropose struct {
    CommandId int32
    ClientId  int32
    Command   state.Command
    Timestamp int64
    CausalDep int32  // Sequence number of the previous weak command from this client
}
```

### Serialization Updates
- Update Marshal/Unmarshal to handle CausalDep field

## Task 6.2: Add lastWeakSeqNum Tracking in Client

### Changes to client.go
```go
type Client struct {
    // ... existing fields ...
    lastWeakSeqNum int32  // Track sequence number of last weak command
}
```

### Changes to SendWeakWrite/SendWeakRead
- Set CausalDep to lastWeakSeqNum
- Update lastWeakSeqNum after sending

## Task 6.3: Implement waitForExecution in Replica

### New Method
```go
func (r *Replica) waitForWeakDep(clientId int32, depSeqNum int32) {
    // Check if the dependent command has been executed
    // If not, wait (with timeout) for it to complete
}
```

### Update handleWeakPropose
```go
func (r *Replica) handleWeakPropose(propose *MWeakPropose) {
    // Wait for causal dependency if present
    if propose.CausalDep > 0 {
        r.waitForWeakDep(propose.ClientId, propose.CausalDep)
    }
    // ... rest of handling
}
```

## Considerations

### Timeout Handling
- Need a timeout to prevent deadlocks if the dependent command is lost
- On timeout, either proceed anyway or return error to client

### Tracking Weak Command Execution
- Need to track which weak commands have been executed per-client
- Use a map: clientId -> lastExecutedWeakSeqNum

## Estimated LOC
- Task 6.1: ~20 lines (serialization)
- Task 6.2: ~10 lines
- Task 6.3: ~30 lines

Total: ~60 lines
