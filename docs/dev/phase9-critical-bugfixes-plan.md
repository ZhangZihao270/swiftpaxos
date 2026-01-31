# Phase 9 Plan: Critical Bug Fixes

## Overview

The current CURP-HT implementation has critical correctness issues that violate the protocol's safety guarantees. This document outlines the fixes needed.

## Issue 1: Speculative Execution Modifies State Machine

### Problem
Currently, both strong and weak commands call `Execute()` during speculative execution, which modifies the state machine before commit. This is incorrect because:
1. If a command is never committed (e.g., leader failure), the state modification cannot be rolled back
2. Concurrent speculative executions may see each other's uncommitted state

### Solution: Task 9.1 - Add ComputeResult()

Add a new method `ComputeResult(st *State) Value` that:
- **GET**: Returns the value from state (read-only)
- **SCAN**: Returns concatenated values (read-only)
- **PUT**: Returns NIL() without modifying state
- **NONE**: Returns NIL()

```go
func (c *Command) ComputeResult(st *State) Value {
    st.mutex.Lock()
    defer st.mutex.Unlock()

    switch c.Op {
    case GET:
        if value, present := st.Store.Get(c.K); present {
            return value.(Value)
        }
    case SCAN:
        // Same as Execute but read-only
        found := make([]Value, 0)
        count := binary.LittleEndian.Uint64(c.V)
        it := st.Store.Select(func(index interface{}, value interface{}) bool {
            keyAsserted := index.(Key)
            return keyAsserted >= c.K && keyAsserted <= c.K+Key(count)
        }).Iterator()
        for it.Next() {
            found = append(found, it.Value().(Value))
        }
        return concat(found)
    case PUT:
        // For PUT, we return NIL during speculation
        // The actual value will be written on commit
        return NIL()
    }
    return NIL()
}
```

### Task 9.2 - Add applied field to commandDesc

Add `applied bool` field to track whether command has been applied to state machine.

```go
type commandDesc struct {
    // ... existing fields
    applied bool  // Whether command has been applied to state machine
}
```

### Task 9.3 - Modify deliver() for strong commands

Change the execution flow:
1. Speculative phase: Call `ComputeResult()` instead of `Execute()`
2. After COMMIT: Call `Execute()` to apply to state machine
3. Only mark `r.executed` after actual execution

### Task 9.4 - Modify handleWeakPropose() for weak commands

Same pattern as strong commands:
1. Use `ComputeResult()` for speculative result
2. Don't modify state during speculative execution

## Issue 2: Weak Commands Don't Follow Slot Ordering

### Problem
Weak commands only check `CausalDep` but don't check if `slot-1` has been executed. This can cause out-of-order state machine modifications.

### Task 9.5 - Add slot ordering for weak commands

After commit, check `slot-1` is executed before executing current slot.

### Task 9.6 - Unify execution path

Create `executeInOrder()` helper:
```go
func (r *Replica) executeInOrder(slot int, cmd state.Command, desc *commandDesc) {
    // Wait for slot-1 to be executed
    r.waitSlotExecuted(slot - 1)

    // Execute command
    desc.val = cmd.Execute(r.State)
    desc.applied = true

    // Mark this slot as executed
    r.executed.Set(strconv.Itoa(slot), struct{}{})
}
```

### Task 9.7 - Update asyncReplicateWeak()

After majority ack:
1. Mark as committed
2. Trigger `executeInOrder()` for proper execution

## Testing Tasks

### Task 9.8 - State not modified during speculation
Verify state unchanged after speculative execution, changed only after commit.

### Task 9.9 - Execution follows slot order
Create interleaved strong/weak commands, verify execution order matches slot order.

### Task 9.10 - Correct result after commit
Verify speculative result matches final result for both command types.

## Estimated LOC
- Task 9.1: ~30 lines
- Task 9.2: ~5 lines
- Task 9.3: ~50 lines
- Task 9.4: ~30 lines
- Task 9.5: ~30 lines
- Task 9.6: ~40 lines
- Task 9.7: ~20 lines
- Task 9.8-9.10: ~150 lines tests

Total: ~355 lines (within 500 LOC limit)
