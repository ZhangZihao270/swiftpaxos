# Phase 7 Plan: Testing

## Overview
Add comprehensive tests for the curp-ht package to verify weak command handling,
strong command regression, and mixed workload correctness.

## Task 7.1: Unit Tests for Weak Command Execution

### Test: TestWeakCommandExecution
Verify that weak commands execute correctly with immediate response.

```go
func TestWeakCommandExecution(t *testing.T) {
    // Setup: Create mock replica as leader
    // Send: Weak write command
    // Assert: Reply is received immediately
    // Assert: Value is stored correctly
}
```

## Task 7.2: Unit Tests for Strong Command (Regression)

### Test: TestStrongCommandUnchanged
Verify that strong command behavior is unchanged after adding weak support.

```go
func TestStrongCommandUnchanged(t *testing.T) {
    // Setup: Create mock replica
    // Send: Strong write command (normal Propose)
    // Assert: Normal accept/commit flow is followed
    // Assert: Reply matches expected behavior
}
```

## Task 7.3: Unit Tests for Mixed Commands

### Test: TestMixedCommands
Verify correctness when mixing weak and strong commands.

```go
func TestMixedCommands(t *testing.T) {
    // Setup: Create mock replica as leader
    // Send: Interleaved weak and strong commands
    // Assert: Slot ordering is maintained
    // Assert: All commands complete correctly
}
```

## Task 7.4: Unit Tests for Causal Ordering

### Test: TestCausalOrdering
Verify that weak commands maintain causal order.

```go
func TestCausalOrdering(t *testing.T) {
    // Setup: Create mock replica as leader
    // Send: Weak command A (write key K)
    // Send: Weak command B (read key K, depends on A)
    // Assert: B sees A's value
}
```

## Task 7.5: Integration Tests

### Test: TestMultiReplicaWeakFlow
Verify weak command flow across multiple replicas.

```go
func TestMultiReplicaWeakFlow(t *testing.T) {
    // Setup: Create multiple replicas, one as leader
    // Send: Weak command to leader
    // Assert: Leader replies immediately
    // Wait: For async replication
    // Assert: All replicas have the command
}
```

## Test File Structure

```
curp-ht/
├── curp-ht_test.go      # Main test file
├── test_helpers.go      # Test utilities (mock setup, etc.)
```

## Implementation Notes

Since the project has no existing test infrastructure, we'll need to:
1. Create minimal mock structures for testing
2. Test at the function level where possible
3. Focus on testing the new weak command logic

## Estimated LOC
- Task 7.1: ~50 lines
- Task 7.2: ~50 lines
- Task 7.3: ~60 lines
- Task 7.4: ~40 lines
- Task 7.5: ~100 lines

Total: ~300 lines
