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

// TestFindSCC_NilDependencySkipped verifies SCC skips nil dependencies
// (they may be causal instances not yet received).
func TestFindSCC_NilDependencySkipped(t *testing.T) {
	r := newTestReplica(3)
	e := &Exec{r: r}

	cmds := []state.Command{{Op: state.PUT, K: 1, V: state.NIL(), CL: state.STRONG}}
	r.InstanceSpace[0][2] = &Instance{
		Cmds:       cmds,
		Status:     STRONGLY_COMMITTED,
		State:      READY,
		Seq:        2,
		Deps:       []int32{1, -1, -1}, // depends on slot 1 of replica 0
		CL:         []int32{0, 0, 0},
		instanceId: &instanceId{0, 2},
	}
	// Slot 1 is nil (causal dep not yet received — should be skipped)
	r.ExecedUpTo[0] = 0

	ok := e.findSCC(r.InstanceSpace[0][2])
	if !ok {
		t.Error("findSCC should succeed (nil deps are skipped as potentially causal)")
	}
}

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
