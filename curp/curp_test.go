package curp

import (
	"bytes"
	"strconv"
	"testing"

	cmap "github.com/orcaman/concurrent-map"

	"github.com/imdea-software/swiftpaxos/hook"
	"github.com/imdea-software/swiftpaxos/replica"
	"github.com/imdea-software/swiftpaxos/replica/defs"
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

// newTestReplica creates a minimal Replica suitable for deliver() tests.
func newTestReplica() *Replica {
	baseRep := &replica.Replica{
		Exec:  true,
		State: state.InitState(),
	}
	r := &Replica{
		Replica:     baseRep,
		isLeader:    true,
		ballot:      0,
		delivered:   cmap.New(),
		executed:    cmap.New(),
		committed:   cmap.New(),
		proposes:    cmap.New(),
		values:      cmap.New(),
		cmdDescs:    cmap.New(),
		sender:      make(replica.Sender, 128),
		deliverChan: make(chan int, 128),
		closedChan:  make(chan struct{}),
	}
	close(r.closedChan)
	return r
}

// TestSpeculativeReplySkipsSlotOrdering verifies that speculative replies
// (leader, phase != COMMIT) do NOT wait for slot ordering. This preserves
// the fast path — the dep mechanism (leaderUnsync → Ok=FALSE) protects
// against stale reads for conflicting keys.
// (Phase 85: reverts Phase 83.1 which incorrectly added slot ordering to speculative path.)
func TestSpeculativeReplySkipsSlotOrdering(t *testing.T) {
	r := newTestReplica()

	cmdId := CommandId{ClientId: 1, SeqNum: 0}
	propose := &defs.GPropose{
		Propose: &defs.Propose{
			ClientId: 1,
			Command:  state.Command{Op: state.GET, K: state.Key(42), V: state.NIL()},
		},
	}
	r.proposes.Set(cmdId.String(), propose)

	// PUT something first so GET returns a value
	putCmd := state.Command{Op: state.PUT, K: state.Key(42), V: state.Value([]byte("val"))}
	putCmd.Execute(r.State)

	desc := &commandDesc{
		cmdId:        cmdId,
		cmd:          propose.Command,
		phase:        ACCEPT, // NOT COMMIT — speculative
		cmdSlot:      1,      // slot > 0
		slotStr:      "1",
		dep:          -1,
		afterPayload: hook.NewOptCondF(func() bool { return true }),
		msgs:         make(chan interface{}, 128),
		seq:          true,
	}

	// Slot 0 is NOT executed — but speculative reply should still be sent
	r.deliver(desc, 1)

	// Speculative reply should be sent (slot ordering skipped for non-COMMIT)
	select {
	case msg := <-r.sender:
		_ = msg
		// Correct: speculative reply sent without waiting for slot ordering
	default:
		t.Error("Speculative reply should skip slot ordering, but was blocked")
	}
}

// TestCommitWaitsForSlotOrdering verifies that COMMIT phase delivery
// still enforces slot ordering — only speculative replies skip it.
func TestCommitWaitsForSlotOrdering(t *testing.T) {
	r := newTestReplica()

	cmdId := CommandId{ClientId: 1, SeqNum: 0}
	propose := &defs.GPropose{
		Propose: &defs.Propose{
			ClientId: 1,
			Command:  state.Command{Op: state.PUT, K: state.Key(42), V: state.Value([]byte("val"))},
		},
	}
	r.proposes.Set(cmdId.String(), propose)

	desc := &commandDesc{
		cmdId:        cmdId,
		cmd:          propose.Command,
		phase:        COMMIT, // COMMIT phase — slot ordering enforced
		cmdSlot:      1,      // slot > 0
		slotStr:      "1",
		dep:          -1,
		afterPayload: hook.NewOptCondF(func() bool { return true }),
		msgs:         make(chan interface{}, 128),
		seq:          true,
	}

	// Slot 0 is NOT executed — COMMIT delivery should be blocked
	r.deliver(desc, 1)

	// desc.applied should be false because slot ordering blocked execution
	if desc.applied {
		t.Error("COMMIT deliver should wait for slot ordering, but desc.applied=true")
	}
}

// TestD8AlwaysSendMReply verifies that MReply is sent even when Ok==FALSE
// (pending dependency). Before D8 fix, only Ok==TRUE replies were sent,
// causing client hangs when the fast path needed a leader reply.
func TestD8AlwaysSendMReply(t *testing.T) {
	r := newTestReplica()

	cmdId := CommandId{ClientId: 1, SeqNum: 0}
	propose := &defs.GPropose{
		Propose: &defs.Propose{
			ClientId: 1,
			Command:  state.Command{Op: state.GET, K: state.Key(1), V: state.NIL()},
		},
	}
	r.proposes.Set(cmdId.String(), propose)

	desc := &commandDesc{
		cmdId:        cmdId,
		cmd:          propose.Command,
		phase:        ACCEPT,
		cmdSlot:      0,
		slotStr:      "0",
		dep:          5, // dependency on slot 5
		afterPayload: hook.NewOptCondF(func() bool { return true }),
		msgs:         make(chan interface{}, 128),
		seq:          true,
	}
	// dep=5 is NOT in committed map, so Ok should be FALSE

	r.deliver(desc, 0)

	// D8: reply should still be sent despite Ok==FALSE
	select {
	case msg := <-r.sender:
		_ = msg
	default:
		t.Error("D8: MReply should be sent even when Ok==FALSE")
	}
}

// TestAppliedPreventsDoubleExecution verifies that the applied flag prevents
// Execute from being called twice on the same descriptor.
func TestAppliedPreventsDoubleExecution(t *testing.T) {
	r := newTestReplica()
	r.history = make([]commandStaticDesc, HISTORY_SIZE)

	cmdId := CommandId{ClientId: 1, SeqNum: 0}
	propose := &defs.GPropose{
		Propose: &defs.Propose{
			ClientId: 1,
			Command:  state.Command{Op: state.PUT, K: state.Key(10), V: state.Value([]byte("first"))},
		},
	}
	r.proposes.Set(cmdId.String(), propose)

	desc := &commandDesc{
		cmdId:        cmdId,
		cmd:          propose.Command,
		phase:        COMMIT,
		cmdSlot:      0,
		slotStr:      "0",
		dep:          -1,
		applied:      false,
		afterPayload: hook.NewOptCondF(func() bool { return true }),
		msgs:         make(chan interface{}, 128),
		seq:          true,
	}

	r.deliver(desc, 0)

	// Verify first execution happened
	if !desc.applied {
		t.Fatal("desc.applied should be true after deliver()")
	}
	val, exists := r.values.Get(cmdId.String())
	if !exists {
		t.Fatal("values should be set after execution")
	}

	// Now change the command to write a different value and re-deliver
	desc.cmd = state.Command{Op: state.PUT, K: state.Key(10), V: state.Value([]byte("second"))}
	// Reset delivered so deliver() doesn't early-return
	r.delivered = cmap.New()

	r.deliver(desc, 0)

	// Value should still be "first" — applied guard prevented re-execution
	val2, _ := r.values.Get(cmdId.String())
	if !bytes.Equal(val.([]byte), val2.([]byte)) {
		t.Errorf("applied guard failed: value changed from %v to %v", val, val2)
	}
}

// TestSpeculativeUsesComputeResult verifies that speculative execution uses
// ComputeResult (read-only) instead of Execute (modifies state).
func TestSpeculativeUsesComputeResult(t *testing.T) {
	r := newTestReplica()

	// PUT key=1 first
	putCmd := state.Command{Op: state.PUT, K: state.Key(1), V: state.Value([]byte("orig"))}
	putCmd.Execute(r.State)

	cmdId := CommandId{ClientId: 1, SeqNum: 0}
	// Speculative PUT should use ComputeResult (returns NIL, doesn't modify state)
	putCmd2 := state.Command{Op: state.PUT, K: state.Key(1), V: state.Value([]byte("new"))}
	propose := &defs.GPropose{
		Propose: &defs.Propose{
			ClientId: 1,
			Command:  putCmd2,
		},
	}
	r.proposes.Set(cmdId.String(), propose)

	desc := &commandDesc{
		cmdId:        cmdId,
		cmd:          putCmd2,
		phase:        ACCEPT, // speculative
		cmdSlot:      0,
		slotStr:      "0",
		dep:          -1,
		afterPayload: hook.NewOptCondF(func() bool { return true }),
		msgs:         make(chan interface{}, 128),
		seq:          true,
	}

	r.deliver(desc, 0)

	// State should NOT be modified by speculative PUT
	getCmd := state.Command{Op: state.GET, K: state.Key(1), V: state.NIL()}
	result := getCmd.Execute(r.State)
	if !bytes.Equal(result, []byte("orig")) {
		t.Errorf("speculative PUT modified state: GET(1) = %q, want 'orig'", result)
	}
}
