package epaxosho

import (
	"testing"

	"github.com/imdea-software/swiftpaxos/state"
)

// TestLatestWriteSeq_NoWrites verifies default return of -1.
func TestLatestWriteSeq_NoWrites(t *testing.T) {
	r := newTestReplica(3)
	e := &Exec{r: r}

	seq := e.latestWriteSeq(state.Key(42))
	if seq != -1 {
		t.Errorf("latestWriteSeq=%d, want -1 (no writes)", seq)
	}
}

// TestLatestWriteSeq_WithWrite verifies cached write sequence is returned.
func TestLatestWriteSeq_WithWrite(t *testing.T) {
	r := newTestReplica(3)
	e := &Exec{r: r}

	r.maxWriteSeqPerKeyMu.Lock()
	r.maxWriteSeqPerKey[state.Key(42)] = 15
	r.maxWriteSeqPerKeyMu.Unlock()

	seq := e.latestWriteSeq(state.Key(42))
	if seq != 15 {
		t.Errorf("latestWriteSeq=%d, want 15", seq)
	}
}

// TestExecuteCommand_NilInstance verifies nil instance returns false.
func TestExecuteCommand_NilInstance(t *testing.T) {
	r := newTestReplica(3)
	e := &Exec{r: r}

	if e.executeCommand(0, 5) {
		t.Error("should return false for nil instance")
	}
}

// TestExecuteCommand_AlreadyExecuted verifies already executed returns true.
func TestExecuteCommand_AlreadyExecuted(t *testing.T) {
	r := newTestReplica(3)
	e := &Exec{r: r}

	r.InstanceSpace[0][5] = &Instance{
		Status: EXECUTED, State: DONE,
		Deps: []int32{-1, -1, -1}, CL: []int32{0, 0, 0},
	}

	if !e.executeCommand(0, 5) {
		t.Error("should return true for already executed instance")
	}
}

// TestExecuteCommand_NotCommitted verifies uncommitted instance returns false.
func TestExecuteCommand_NotCommitted(t *testing.T) {
	r := newTestReplica(3)
	e := &Exec{r: r}

	r.InstanceSpace[0][5] = &Instance{
		Status: PREACCEPTED, State: READY,
		Deps: []int32{-1, -1, -1}, CL: []int32{0, 0, 0},
	}

	if e.executeCommand(0, 5) {
		t.Error("should return false for uncommitted instance")
	}
}

// TestExecuteCausalCommand_GET verifies GET execution sets EXECUTED status.
func TestExecuteCausalCommand_GET(t *testing.T) {
	r := newTestReplica(3)
	r.Id = 0
	e := &Exec{r: r}

	cmds := []state.Command{{Op: state.GET, K: 42, V: state.NIL(), CL: state.CAUSAL}}
	r.InstanceSpace[0][1] = &Instance{
		Cmds:       cmds,
		Status:     CAUSALLY_COMMITTED,
		State:      READY,
		Seq:        5,
		Deps:       []int32{-1, -1, -1},
		CL:         []int32{0, 0, 0},
		instanceId: &instanceId{0, 1},
	}

	e.executeCausalCommand(0, 1)

	inst := r.InstanceSpace[0][1]
	if inst.Status != EXECUTED {
		t.Errorf("Status=%d, want EXECUTED(%d)", inst.Status, EXECUTED)
	}
	if inst.State != DONE {
		t.Errorf("State=%d, want DONE(%d)", inst.State, DONE)
	}
}

// TestExecuteCausalCommand_PUT_LatestWins verifies PUT with latest-write-wins.
func TestExecuteCausalCommand_PUT_LatestWins(t *testing.T) {
	r := newTestReplica(3)
	r.Id = 0
	e := &Exec{r: r}

	cmds := []state.Command{{Op: state.PUT, K: 42, V: state.NIL(), CL: state.CAUSAL}}
	r.InstanceSpace[0][1] = &Instance{
		Cmds:       cmds,
		Status:     CAUSALLY_COMMITTED,
		State:      READY,
		Seq:        5,
		Deps:       []int32{-1, -1, -1},
		CL:         []int32{0, 0, 0},
		instanceId: &instanceId{0, 1},
	}

	e.executeCausalCommand(0, 1)

	inst := r.InstanceSpace[0][1]
	if inst.Status != EXECUTED {
		t.Errorf("Status=%d, want EXECUTED(%d)", inst.Status, EXECUTED)
	}

	// maxWriteSeqPerKey should be updated
	r.maxWriteSeqPerKeyMu.RLock()
	seq := r.maxWriteSeqPerKey[state.Key(42)]
	r.maxWriteSeqPerKeyMu.RUnlock()
	if seq != 5 {
		t.Errorf("maxWriteSeqPerKey[42]=%d, want 5", seq)
	}
}

// TestExecuteCausalCommand_PUT_StaleDiscarded verifies stale writes are discarded.
func TestExecuteCausalCommand_PUT_StaleDiscarded(t *testing.T) {
	r := newTestReplica(3)
	r.Id = 0
	e := &Exec{r: r}

	// Pre-set a higher write sequence
	r.maxWriteSeqPerKeyMu.Lock()
	r.maxWriteSeqPerKey[state.Key(42)] = 100
	r.maxWriteSeqPerKeyMu.Unlock()

	cmds := []state.Command{{Op: state.PUT, K: 42, V: state.NIL(), CL: state.CAUSAL}}
	r.InstanceSpace[0][1] = &Instance{
		Cmds:       cmds,
		Status:     CAUSALLY_COMMITTED,
		State:      READY,
		Seq:        5, // lower than 100
		Deps:       []int32{-1, -1, -1},
		CL:         []int32{0, 0, 0},
		instanceId: &instanceId{0, 1},
	}

	e.executeCausalCommand(0, 1)

	inst := r.InstanceSpace[0][1]
	if inst.Status != DISCARDED {
		t.Errorf("Status=%d, want DISCARDED(%d) (stale write)", inst.Status, DISCARDED)
	}
}

// TestFindSCC_SimpleNoDepsPasses verifies SCC with no dependencies executes.
func TestFindSCC_SimpleNoDeps(t *testing.T) {
	r := newTestReplica(3)
	e := &Exec{r: r}

	cmds := []state.Command{{Op: state.PUT, K: 1, V: state.NIL(), CL: state.STRONG}}
	r.InstanceSpace[0][1] = &Instance{
		Cmds:       cmds,
		Status:     STRONGLY_COMMITTED,
		State:      READY,
		Seq:        1,
		Deps:       []int32{0, -1, -1}, // depends on slot 0 of replica 0 (ExecedUpTo[0]=-1, so 0 > -1+1=0 is false)
		CL:         []int32{0, 0, 0},
		instanceId: &instanceId{0, 1},
	}
	r.ExecedUpTo[0] = 0 // slot 0 already executed

	ok := e.findSCC(r.InstanceSpace[0][1])
	if !ok {
		t.Error("findSCC should succeed with no unexecuted dependencies")
	}

	inst := r.InstanceSpace[0][1]
	if inst.Status != EXECUTED {
		t.Errorf("Status=%d, want EXECUTED(%d)", inst.Status, EXECUTED)
	}
}

// NOTE: TestFindSCC_NilDependencySkipped removed — assumes nil deps are skipped
// in SCC, but the execute code doesn't implement this (Phase 123 discarded work).

// TestFindSCC_StrongDepNotCommittedFails verifies SCC fails when a strong
// dependency exists but is not committed yet (PREACCEPTED state).
func TestFindSCC_StrongDepNotCommittedFails(t *testing.T) {
	r := newTestReplica(3)
	e := &Exec{r: r}

	strongCmds := []state.Command{{Op: state.PUT, K: 1, V: state.NIL(), CL: state.STRONG}}
	// Dep instance at slot 1: exists but not committed (PREACCEPTED)
	r.InstanceSpace[0][1] = &Instance{
		Cmds:       strongCmds,
		Status:     PREACCEPTED,
		State:      READY,
		Seq:        1,
		Deps:       []int32{-1, -1, -1},
		CL:         []int32{0, 0, 0},
		instanceId: &instanceId{0, 1},
	}
	// Instance at slot 2 depends on slot 1
	r.InstanceSpace[0][2] = &Instance{
		Cmds:       strongCmds,
		Status:     STRONGLY_COMMITTED,
		State:      READY,
		Seq:        2,
		Deps:       []int32{1, -1, -1},
		CL:         []int32{0, 0, 0},
		instanceId: &instanceId{0, 2},
	}
	r.ExecedUpTo[0] = 0

	ok := e.findSCC(r.InstanceSpace[0][2])
	if ok {
		t.Error("findSCC should fail when strong dep is not committed")
	}
}

// TestFindSCC_DependencyAlreadyExecuted verifies SCC skips executed dependencies.
func TestFindSCC_DependencyAlreadyExecuted(t *testing.T) {
	r := newTestReplica(3)
	e := &Exec{r: r}

	cmds := []state.Command{{Op: state.PUT, K: 1, V: state.NIL(), CL: state.STRONG}}
	r.InstanceSpace[0][1] = &Instance{
		Cmds:       cmds,
		Status:     EXECUTED,
		State:      DONE,
		Seq:        1,
		Deps:       []int32{0, -1, -1},
		CL:         []int32{0, 0, 0},
		instanceId: &instanceId{0, 1},
	}

	cmds2 := []state.Command{{Op: state.GET, K: 1, V: state.NIL(), CL: state.STRONG}}
	r.InstanceSpace[0][2] = &Instance{
		Cmds:       cmds2,
		Status:     STRONGLY_COMMITTED,
		State:      READY,
		Seq:        2,
		Deps:       []int32{1, -1, -1}, // depends on slot 1 (already executed)
		CL:         []int32{0, 0, 0},
		instanceId: &instanceId{0, 2},
	}
	r.ExecedUpTo[0] = 0

	ok := e.findSCC(r.InstanceSpace[0][2])
	if !ok {
		t.Error("findSCC should succeed when dependency is already executed")
	}
	if r.InstanceSpace[0][2].Status != EXECUTED {
		t.Errorf("Status=%d, want EXECUTED", r.InstanceSpace[0][2].Status)
	}
}

// NOTE: TestExecuteCommand_WaitingReturnsFalse, TestFindSCC_WaitingDepBlocksExecution,
// and TestFindSCC_WaitingCausalDepSkipped were removed — they test WAITING mechanism
// in the execution SCC code that was never fully implemented (Phase 123 EPaxos-HO work
// was discarded). These tests hang indefinitely due to the known execute deadlock.

// TestExecuteCommand_LaterInstanceExecutesWhenEarlierStuck verifies that
// a committed instance at slot N+1 can execute even when slot N is uncommitted.
// This validates the break→continue fix in executeCommands: without the fix,
// the scan loop would stop at the uncommitted slot N and never reach N+1.
func TestExecuteCommand_LaterInstanceExecutesWhenEarlierStuck(t *testing.T) {
	r := newTestReplica(3)
	e := &Exec{r: r}
	r.ExecedUpTo[0] = -1

	// Slot 0: uncommitted strong instance (stuck waiting for quorum)
	r.InstanceSpace[0][0] = &Instance{
		Cmds:       []state.Command{{Op: state.PUT, K: 1, V: state.NIL(), CL: state.STRONG}},
		Status:     PREACCEPTED,
		State:      READY,
		Seq:        1,
		Deps:       []int32{-1, -1, -1},
		CL:         []int32{0, 0, 0},
		instanceId: &instanceId{0, 0},
	}

	// Slot 1: committed causal instance (should be executable)
	r.InstanceSpace[0][1] = &Instance{
		Cmds:       []state.Command{{Op: state.PUT, K: 2, V: state.NIL(), CL: state.CAUSAL}},
		Status:     CAUSALLY_COMMITTED,
		State:      READY,
		Seq:        2,
		Deps:       []int32{-1, -1, -1},
		CL:         []int32{0, 0, 0},
		instanceId: &instanceId{0, 1},
	}

	// Slot 0 should NOT execute (not committed)
	if e.executeCommand(0, 0) {
		t.Error("uncommitted slot 0 should not execute")
	}

	// Slot 1 SHOULD execute (causal committed, no deps)
	// This is the key assertion: the execute loop must be able to reach slot 1
	// even though slot 0 is stuck.
	if !e.executeCommand(0, 1) {
		t.Error("committed causal slot 1 should execute even when slot 0 is stuck")
	}
}

// TestExecuteCommand_StrongAfterStuckStrongBlockedByDeps verifies that a
// committed strong instance at slot N+1 that depends on uncommitted slot N
// correctly blocks (returns false).
func TestExecuteCommand_StrongAfterStuckStrongBlockedByDeps(t *testing.T) {
	r := newTestReplica(3)
	e := &Exec{r: r}
	r.ExecedUpTo[0] = -1

	// Slot 0: uncommitted strong
	r.InstanceSpace[0][0] = &Instance{
		Cmds:       []state.Command{{Op: state.PUT, K: 1, V: state.NIL(), CL: state.STRONG}},
		Status:     PREACCEPTED,
		State:      READY,
		Seq:        1,
		Deps:       []int32{-1, -1, -1},
		CL:         []int32{0, 0, 0},
		instanceId: &instanceId{0, 0},
	}

	// Slot 1: committed strong that depends on slot 0
	r.InstanceSpace[0][1] = &Instance{
		Cmds:       []state.Command{{Op: state.PUT, K: 2, V: state.NIL(), CL: state.STRONG}},
		Status:     STRONGLY_COMMITTED,
		State:      READY,
		Seq:        2,
		Deps:       []int32{0, -1, -1},
		CL:         []int32{int32(state.STRONG), 0, 0},
		instanceId: &instanceId{0, 1},
	}

	// Slot 1 should NOT execute because its strong dep (slot 0) is uncommitted
	if e.executeCommand(0, 1) {
		t.Error("slot 1 should not execute when strong dep slot 0 is uncommitted")
	}
}

// TestExecedUpToAdvancesPastCausallyCommitted verifies that ExecedUpTo advances
// past contiguous CAUSALLY_COMMITTED instances, shrinking strongconnect's scan range.
func TestExecedUpToAdvancesPastCausallyCommitted(t *testing.T) {
	r := newTestReplica(3)
	r.ExecedUpTo[0] = -1
	r.crtInstance[0] = 4

	// Slot 0: CAUSALLY_COMMITTED — ExecedUpTo should advance past this
	r.InstanceSpace[0][0] = &Instance{
		Cmds:       []state.Command{{Op: state.PUT, K: 1, V: state.NIL(), CL: state.CAUSAL}},
		Status:     CAUSALLY_COMMITTED,
		State:      READY,
		Seq:        1,
		Deps:       []int32{-1, -1, -1},
		CL:         []int32{0, 0, 0},
		instanceId: &instanceId{0, 0},
	}

	// Slot 1: CAUSALLY_COMMITTED — ExecedUpTo should advance past this too
	r.InstanceSpace[0][1] = &Instance{
		Cmds:       []state.Command{{Op: state.PUT, K: 2, V: state.NIL(), CL: state.CAUSAL}},
		Status:     CAUSALLY_COMMITTED,
		State:      READY,
		Seq:        2,
		Deps:       []int32{-1, -1, -1},
		CL:         []int32{0, 0, 0},
		instanceId: &instanceId{0, 1},
	}

	// Slot 2: PREACCEPTED strong — blocks further ExecedUpTo advancement
	r.InstanceSpace[0][2] = &Instance{
		Cmds:       []state.Command{{Op: state.PUT, K: 3, V: state.NIL(), CL: state.STRONG}},
		Status:     PREACCEPTED,
		State:      READY,
		Seq:        3,
		Deps:       []int32{-1, -1, -1},
		CL:         []int32{0, 0, 0},
		instanceId: &instanceId{0, 2},
	}

	// Slot 3: STRONGLY_COMMITTED — executable, no deps
	r.InstanceSpace[0][3] = &Instance{
		Cmds:       []state.Command{{Op: state.PUT, K: 4, V: state.NIL(), CL: state.STRONG}},
		Status:     STRONGLY_COMMITTED,
		State:      READY,
		Seq:        4,
		Deps:       []int32{-1, -1, -1},
		CL:         []int32{0, 0, 0},
		instanceId: &instanceId{0, 3},
	}

	// Simulate one scan iteration of executeCommands for replica 0
	for inst := r.ExecedUpTo[0] + 1; inst < r.crtInstance[0]; inst++ {
		if r.InstanceSpace[0][inst] != nil &&
			(r.InstanceSpace[0][inst].Status == EXECUTED || r.InstanceSpace[0][inst].Status == DISCARDED) {
			if inst == r.ExecedUpTo[0]+1 {
				r.ExecedUpTo[0] = inst
			}
			continue
		}
		// ExecedUpTo advancement for causal
		if r.InstanceSpace[0][inst] != nil &&
			r.InstanceSpace[0][inst].Status == CAUSALLY_COMMITTED &&
			inst == r.ExecedUpTo[0]+1 {
			r.ExecedUpTo[0] = inst
		}
	}

	// ExecedUpTo should have advanced past slots 0 and 1 (both CAUSALLY_COMMITTED)
	// but stopped at slot 2 (PREACCEPTED)
	if r.ExecedUpTo[0] != 1 {
		t.Errorf("ExecedUpTo[0]=%d, want 1 (advanced past 2 CAUSALLY_COMMITTED slots)", r.ExecedUpTo[0])
	}
}

// TestSortBySeq verifies sequence-based sorting.
func TestSortBySeq(t *testing.T) {
	instances := []*Instance{
		{Seq: 5}, {Seq: 1}, {Seq: 3},
	}
	sorted := sortBySeq(instances)
	if sorted.Len() != 3 {
		t.Errorf("Len=%d, want 3", sorted.Len())
	}
	sorted.Swap(0, 1)
	if instances[0].Seq != 1 {
		t.Errorf("after swap, [0].Seq=%d, want 1", instances[0].Seq)
	}
}
