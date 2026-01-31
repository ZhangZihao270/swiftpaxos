# Phase 4 Plan: Replica-Side Modifications

## Overview
Modify the curp-ht replica to handle weak consistency commands.

## Task 4.1: Add isWeak Field to commandDesc

### Location
curp-ht/curp-ht.go - commandDesc struct

### Changes
```go
type commandDesc struct {
    // ... existing fields ...
    isWeak bool  // Mark if this is a weak command
}
```

---

## Task 4.2: Update run() Loop

### Location
curp-ht/curp-ht.go - run() function

### Changes
Add new case in the select statement:
```go
case m := <-r.cs.weakProposeChan:
    if r.isLeader {
        weakPropose := m.(*MWeakPropose)
        r.handleWeakPropose(weakPropose)
    }
```

---

## Task 4.3: Implement handleWeakPropose

### Signature
```go
func (r *Replica) handleWeakPropose(propose *MWeakPropose)
```

### Implementation Steps
1. Assign slot (share slot space with strong commands)
2. Record dependency using leaderUnsync
3. Create weak command descriptor
4. Execute speculatively (immediately)
5. Reply to client immediately
6. Start async replication in background

---

## Task 4.4: Implement getWeakCmdDesc

### Signature
```go
func (r *Replica) getWeakCmdDesc(slot int, propose *MWeakPropose, dep int) *commandDesc
```

### Implementation
1. Create new descriptor using newDesc()
2. Set isWeak = true
3. Set cmdSlot, dep, cmdId, cmd
4. Return descriptor

---

## Task 4.5: Implement asyncReplicateWeak

### Signature
```go
func (r *Replica) asyncReplicateWeak(desc *commandDesc, slot int)
```

### Implementation
1. Send Accept to other replicas via batcher
2. Handle accept locally
3. Wait for majority ack then commit (reuse existing flow)

---

## Design Notes

### Slot Space Sharing
Weak and strong commands share the same slot sequence to maintain global ordering.
This is critical for correctness.

### Speculative Execution
Weak commands are executed immediately without waiting for replication.
The client gets the result faster (1 RTT instead of 2 RTT).

### Async Replication
Replication happens in the background. If leader fails before replication
completes, weak commands may need to be re-executed after leader recovery.

---

## Estimated LOC
- Task 4.1: ~2 lines
- Task 4.2: ~5 lines
- Task 4.3: ~30 lines
- Task 4.4: ~15 lines
- Task 4.5: ~15 lines

Total: ~67 lines
