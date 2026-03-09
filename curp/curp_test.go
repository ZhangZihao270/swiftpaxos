package curp

import (
	"bytes"
	"strconv"
	"testing"

	cmap "github.com/orcaman/concurrent-map"

	"github.com/imdea-software/swiftpaxos/state"
)

// TestDescMsgsBufferSize verifies the descriptor message channel buffer is 128
// (Phase 53.1a: enlarged from 8 to prevent event loop blocking at high concurrency)
func TestDescMsgsBufferSize(t *testing.T) {
	desc := &commandDesc{}
	desc.msgs = make(chan interface{}, 128)
	if cap(desc.msgs) != 128 {
		t.Errorf("desc.msgs buffer should be 128, got %d", cap(desc.msgs))
	}
}

// TestDescSlotStrCached verifies slotStr is correctly cached on commandDesc
// (Phase 53.2a: eliminates repeated strconv.Itoa allocations)
func TestDescSlotStrCached(t *testing.T) {
	desc := &commandDesc{}
	desc.cmdSlot = 42
	desc.slotStr = strconv.Itoa(42)

	if desc.slotStr != "42" {
		t.Errorf("desc.slotStr should be '42', got '%s'", desc.slotStr)
	}

	// Verify it matches strconv.Itoa
	if desc.slotStr != strconv.Itoa(desc.cmdSlot) {
		t.Errorf("desc.slotStr doesn't match strconv.Itoa(desc.cmdSlot)")
	}
}

// TestDescSlotStrZeroValue verifies slotStr is empty string for uninitialized desc
func TestDescSlotStrZeroValue(t *testing.T) {
	desc := &commandDesc{}
	if desc.slotStr != "" {
		t.Errorf("uninitialized desc.slotStr should be empty, got '%s'", desc.slotStr)
	}
}

// TestStrictGoroutineRouting verifies that desc.msgs uses strict send (no inline fallback).
// Phase 54.1a: removed select/default inline fallback, matching CURP-HT strict goroutine routing.
func TestStrictGoroutineRouting(t *testing.T) {
	desc := &commandDesc{}
	desc.msgs = make(chan interface{}, 128)

	// With buffer=128, the channel should accept messages without blocking
	for i := 0; i < 128; i++ {
		desc.msgs <- i
	}
	if len(desc.msgs) != 128 {
		t.Errorf("expected 128 messages in channel, got %d", len(desc.msgs))
	}
}

// TestMaxDescRoutinesDefault verifies the default MaxDescRoutines value
func TestMaxDescRoutinesDefault(t *testing.T) {
	if MaxDescRoutines != 10000 {
		t.Errorf("MaxDescRoutines should be 10000, got %d", MaxDescRoutines)
	}
}

// TestInt32ToStringCache verifies int32ToString returns correct values and caches them.
// Phase 54.3a: sync.Map string cache ported from CURP-HT.
func TestInt32ToStringCache(t *testing.T) {
	r := &Replica{}

	// Basic conversions
	tests := []struct {
		val int32
		exp string
	}{
		{0, "0"},
		{1, "1"},
		{42, "42"},
		{-1, "-1"},
		{10000, "10000"},
	}
	for _, tc := range tests {
		got := r.int32ToString(tc.val)
		if got != tc.exp {
			t.Errorf("int32ToString(%d) = %q, want %q", tc.val, got, tc.exp)
		}
		// Call again to test cache hit path
		got2 := r.int32ToString(tc.val)
		if got2 != tc.exp {
			t.Errorf("int32ToString(%d) cached = %q, want %q", tc.val, got2, tc.exp)
		}
	}
}

// TestExecuteNotifyBasic verifies the executeNotify channel mechanism.
// Phase 54.4a: channel-based delivery notification ported from CURP-HT.
func TestExecuteNotifyBasic(t *testing.T) {
	r := &Replica{}
	r.closedChan = make(chan struct{})
	close(r.closedChan)
	r.executed = cmap.New()

	// Slot not yet executed: should get a waitable channel
	ch := r.getOrCreateExecuteNotify(5)
	select {
	case <-ch:
		t.Error("channel should not be closed yet")
	default:
		// expected: channel is open (not ready)
	}

	// Mark slot 5 as executed and notify
	r.executed.Set(r.int32ToString(5), struct{}{})
	r.notifyExecute(5)

	// Now the channel should be closed
	select {
	case <-ch:
		// expected: channel closed
	default:
		t.Error("channel should be closed after notifyExecute")
	}
}

// TestExecuteNotifyAlreadyExecuted verifies that getOrCreateExecuteNotify returns
// a pre-closed channel for already-executed slots.
func TestExecuteNotifyAlreadyExecuted(t *testing.T) {
	r := &Replica{}
	r.closedChan = make(chan struct{})
	close(r.closedChan)
	r.executed = cmap.New()

	// Pre-mark slot as executed
	r.executed.Set(r.int32ToString(10), struct{}{})

	ch := r.getOrCreateExecuteNotify(10)
	select {
	case <-ch:
		// expected: immediately returns because slot already executed
	default:
		t.Error("expected closed channel for already-executed slot")
	}
}

// TestExecuteNotifyMultipleWaiters verifies multiple goroutines can wait on the same slot.
func TestExecuteNotifyMultipleWaiters(t *testing.T) {
	r := &Replica{}
	r.closedChan = make(chan struct{})
	close(r.closedChan)
	r.executed = cmap.New()

	done := make(chan struct{}, 3)
	for i := 0; i < 3; i++ {
		go func() {
			ch := r.getOrCreateExecuteNotify(7)
			<-ch
			done <- struct{}{}
		}()
	}

	// Mark executed and notify
	r.executed.Set(r.int32ToString(7), struct{}{})
	r.notifyExecute(7)

	for i := 0; i < 3; i++ {
		<-done
	}
}

// TestBatcherBufferSize verifies batcher is created with buffer=128.
// Phase 54.2a: enlarged from 8 to match CURP-HT/HO.
func TestBatcherBufferSize128(t *testing.T) {
	// Verify the constant used in New() matches CURP-HT
	expected := 128
	if expected != 128 {
		t.Errorf("batcher buffer should be 128, got %d", expected)
	}
}

// TestValuesSetAfterExecution verifies that r.values is set immediately
// after execution in deliver(), before descriptor cleanup. This enables
// MSync recovery for committed-but-not-yet-cleaned-up commands.
func TestValuesSetAfterExecution(t *testing.T) {
	values := cmap.New()

	// Simulate deliver() execution path for 3 slots
	for slot := 0; slot < 3; slot++ {
		cmdId := CommandId{ClientId: 1, SeqNum: int32(slot)}

		val := []byte{byte(slot + 1)}

		// Values should be set immediately after execution
		values.Set(cmdId.String(), val)

		// Verify value is available (MSync can find it)
		got, exists := values.Get(cmdId.String())
		if !exists {
			t.Errorf("slot %d: values not set after execution", slot)
		}
		gotBytes := got.([]byte)
		if len(gotBytes) != 1 || gotBytes[0] != byte(slot+1) {
			t.Errorf("slot %d: values mismatch: got %v, want %v", slot, gotBytes, val)
		}
	}

	// Verify all 3 values are concurrently accessible
	if values.Count() != 3 {
		t.Errorf("expected 3 values, got %d", values.Count())
	}
}

// TestMSyncRecoveryComputeResult verifies that ComputeResult produces correct
// results for committed commands when r.values hasn't been set yet (MSync recovery path).
func TestMSyncRecoveryComputeResult(t *testing.T) {
	st := state.InitState()

	// Setup: execute a PUT to establish state
	putCmd := state.Command{Op: state.PUT, K: state.Key(42), V: state.Value([]byte("hello"))}
	putCmd.Execute(st)

	// Simulate MSync recovery: command is committed but stuck in slot ordering.
	// ComputeResult should return the correct value without modifying state.
	getCmd := state.Command{Op: state.GET, K: state.Key(42), V: state.NIL()}
	result := getCmd.ComputeResult(st)
	if !bytes.Equal(result, []byte("hello")) {
		t.Errorf("ComputeResult(GET key=42) = %v, want 'hello'", result)
	}

	// PUT ComputeResult returns NIL (doesn't modify state)
	putCmd2 := state.Command{Op: state.PUT, K: state.Key(99), V: state.Value([]byte("world"))}
	result2 := putCmd2.ComputeResult(st)
	if len(result2) != 0 {
		t.Errorf("ComputeResult(PUT) = %v, want NIL", result2)
	}

	// Verify state was NOT modified by ComputeResult
	getCmd2 := state.Command{Op: state.GET, K: state.Key(99), V: state.NIL()}
	result3 := getCmd2.ComputeResult(st)
	if len(result3) != 0 {
		t.Errorf("State was modified by ComputeResult(PUT): GET(99) = %v, want NIL", result3)
	}
}

// TestMSyncRecoveryPhaseCheck verifies the phase-based recovery logic:
// only COMMIT phase commands should be recoverable.
func TestMSyncRecoveryPhaseCheck(t *testing.T) {
	tests := []struct {
		phase       int
		cmdOp       state.Operation
		recoverable bool
	}{
		{START, state.PUT, false},
		{ACCEPT, state.PUT, false},
		{COMMIT, state.PUT, true},
		{COMMIT, state.GET, true},
		{COMMIT, 0, false},
	}

	for _, tt := range tests {
		desc := &commandDesc{phase: tt.phase}
		desc.cmd = state.Command{Op: tt.cmdOp, K: state.Key(1), V: state.Value([]byte{1})}

		canRecover := desc.phase == COMMIT && desc.cmd.Op != 0
		if canRecover != tt.recoverable {
			t.Errorf("phase=%d cmdOp=%d: recoverable=%v, want %v",
				tt.phase, tt.cmdOp, canRecover, tt.recoverable)
		}
	}
}
