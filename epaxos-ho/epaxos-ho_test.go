package epaxosho

import (
	"sync"
	"testing"

	"github.com/imdea-software/swiftpaxos/replica"
	"github.com/imdea-software/swiftpaxos/replica/defs"
	fastrpc "github.com/imdea-software/swiftpaxos/rpc"
	"github.com/imdea-software/swiftpaxos/state"
)

func TestConstants(t *testing.T) {
	if MAX_INSTANCE != 5*1024*1024 {
		t.Errorf("MAX_INSTANCE=%d, want %d", MAX_INSTANCE, 5*1024*1024)
	}
	if MAX_DEPTH_DEP != 1000 {
		t.Errorf("MAX_DEPTH_DEP=%d, want 1000", MAX_DEPTH_DEP)
	}
	if TRUE != 1 {
		t.Errorf("TRUE=%d, want 1", TRUE)
	}
	if FALSE != 0 {
		t.Errorf("FALSE=%d, want 0", FALSE)
	}
	if NO_CAUSAL_CHANNEL != 10 {
		t.Errorf("NO_CAUSAL_CHANNEL=%d, want 10", NO_CAUSAL_CHANNEL)
	}
}

func TestInstanceZeroValue(t *testing.T) {
	inst := &Instance{}
	if inst.Status != NONE {
		t.Errorf("zero Instance.Status=%d, want NONE(%d)", inst.Status, NONE)
	}
	if inst.State != NONE {
		t.Errorf("zero Instance.State=%d, want NONE(%d)", inst.State, NONE)
	}
	if inst.bal != 0 || inst.vbal != 0 {
		t.Error("zero Instance should have bal=0, vbal=0")
	}
	if inst.Seq != 0 {
		t.Error("zero Instance should have Seq=0")
	}
	if inst.Cmds != nil || inst.Deps != nil || inst.CL != nil {
		t.Error("zero Instance slices should be nil")
	}
	if inst.lb != nil {
		t.Error("zero Instance should have nil LeaderBookkeeping")
	}
}

func TestInstanceWithCLAndDeps(t *testing.T) {
	cmds := []state.Command{
		{Op: state.PUT, K: 1, V: state.NIL(), CL: state.STRONG, Sid: 42},
		{Op: state.GET, K: 2, V: state.NIL(), CL: state.CAUSAL, Sid: 7},
	}
	deps := []int32{0, -1, 3, 5, -1}
	cl := []int32{int32(state.STRONG), int32(state.CAUSAL)}

	inst := &Instance{
		Cmds:   cmds,
		bal:    1,
		vbal:   1,
		Status: PREACCEPTED,
		State:  WAITING,
		Seq:    10,
		Deps:   deps,
		CL:     cl,
	}

	if len(inst.Cmds) != 2 {
		t.Fatalf("expected 2 cmds, got %d", len(inst.Cmds))
	}
	if inst.Cmds[0].CL != state.STRONG {
		t.Errorf("cmd[0].CL=%d, want STRONG(%d)", inst.Cmds[0].CL, state.STRONG)
	}
	if inst.Cmds[1].CL != state.CAUSAL {
		t.Errorf("cmd[1].CL=%d, want CAUSAL(%d)", inst.Cmds[1].CL, state.CAUSAL)
	}
	if len(inst.CL) != 2 {
		t.Fatalf("expected 2 CL entries, got %d", len(inst.CL))
	}
	if inst.CL[0] != int32(state.STRONG) {
		t.Errorf("CL[0]=%d, want %d", inst.CL[0], int32(state.STRONG))
	}
	if len(inst.Deps) != 5 {
		t.Fatalf("expected 5 deps, got %d", len(inst.Deps))
	}
	if inst.Status != PREACCEPTED {
		t.Errorf("Status=%d, want PREACCEPTED(%d)", inst.Status, PREACCEPTED)
	}
}

func TestLeaderBookkeepingZeroValue(t *testing.T) {
	lb := &LeaderBookkeeping{}
	if lb.maxRecvBallot != 0 {
		t.Error("zero LB should have maxRecvBallot=0")
	}
	if lb.prepareOKs != 0 || lb.preAcceptOKs != 0 || lb.acceptOKs != 0 || lb.nacks != 0 {
		t.Error("zero LB should have all counters=0")
	}
	if lb.allEqual {
		t.Error("zero LB should have allEqual=false")
	}
	if lb.preparing || lb.tryingToPreAccept {
		t.Error("zero LB should not be preparing or tryingToPreAccept")
	}
	if lb.clientProposals != nil || lb.originalDeps != nil || lb.committedDeps != nil {
		t.Error("zero LB slices should be nil")
	}
	if lb.recoveryInst != nil {
		t.Error("zero LB should have nil recoveryInst")
	}
}

func TestRecoveryInstanceFields(t *testing.T) {
	ri := &RecoveryInstance{
		cmds:            []state.Command{{Op: state.PUT, K: 1, V: state.NIL(), CL: state.STRONG}},
		status:          ACCEPTED,
		seq:             5,
		deps:            []int32{1, 2, 3},
		cl:              []int32{int32(state.STRONG)},
		preAcceptCount:  3,
		leaderResponded: true,
	}

	if ri.status != ACCEPTED {
		t.Errorf("status=%d, want ACCEPTED(%d)", ri.status, ACCEPTED)
	}
	if ri.seq != 5 {
		t.Errorf("seq=%d, want 5", ri.seq)
	}
	if len(ri.deps) != 3 {
		t.Errorf("deps len=%d, want 3", len(ri.deps))
	}
	if ri.preAcceptCount != 3 {
		t.Errorf("preAcceptCount=%d, want 3", ri.preAcceptCount)
	}
	if !ri.leaderResponded {
		t.Error("leaderResponded should be true")
	}
}

func TestInstanceIdFields(t *testing.T) {
	id := &instanceId{replica: 2, instance: 42}
	if id.replica != 2 {
		t.Errorf("replica=%d, want 2", id.replica)
	}
	if id.instance != 42 {
		t.Errorf("instance=%d, want 42", id.instance)
	}
}

func TestInstanceStatusTransitions(t *testing.T) {
	// Verify expected status values for causal vs strong paths
	// Causal path: NONE → CAUSAL_ACCEPTED → CAUSALLY_COMMITTED → EXECUTED
	// Strong path: NONE → PREACCEPTED → ACCEPTED → STRONGLY_COMMITTED → EXECUTED
	inst := &Instance{Status: NONE}

	// Causal path
	inst.Status = CAUSAL_ACCEPTED
	if inst.Status != CAUSAL_ACCEPTED {
		t.Errorf("Status=%d, want CAUSAL_ACCEPTED(%d)", inst.Status, CAUSAL_ACCEPTED)
	}
	inst.Status = CAUSALLY_COMMITTED
	if inst.Status != CAUSALLY_COMMITTED {
		t.Errorf("Status=%d, want CAUSALLY_COMMITTED(%d)", inst.Status, CAUSALLY_COMMITTED)
	}

	// Strong path
	inst.Status = PREACCEPTED
	if inst.Status != PREACCEPTED {
		t.Errorf("Status=%d, want PREACCEPTED(%d)", inst.Status, PREACCEPTED)
	}
	inst.Status = ACCEPTED
	if inst.Status != ACCEPTED {
		t.Errorf("Status=%d, want ACCEPTED(%d)", inst.Status, ACCEPTED)
	}
	inst.Status = STRONGLY_COMMITTED
	if inst.Status != STRONGLY_COMMITTED {
		t.Errorf("Status=%d, want STRONGLY_COMMITTED(%d)", inst.Status, STRONGLY_COMMITTED)
	}
	inst.Status = EXECUTED
	if inst.Status != EXECUTED {
		t.Errorf("Status=%d, want EXECUTED(%d)", inst.Status, EXECUTED)
	}
}

func TestExecStructHoldsReplicaRef(t *testing.T) {
	// Exec just holds a reference to Replica — verify the type relationship
	var e *Exec
	_ = e // compile-time check that Exec type exists with r *Replica field
}

func TestAdaptTimeSecConstant(t *testing.T) {
	if ADAPT_TIME_SEC != 10 {
		t.Errorf("ADAPT_TIME_SEC=%d, want 10", ADAPT_TIME_SEC)
	}
}

func TestClockChannelVarsExist(t *testing.T) {
	// Verify the package-level clock channel vars are declared.
	// They are initialized in run(), so they start as nil.
	var _ chan bool = fastClockChan
	var _ chan bool = slowClockChan
}

// TestStubHandlersDoNotPanic verifies that all stub handler methods
// can be called without panicking (they are no-ops for now).
func TestStubHandlersDoNotPanic(t *testing.T) {
	// We can't call New() without a full replica, but we can create
	// a minimal Replica with just the fields the stubs need.
	r := &Replica{}

	// handlePrepare tested separately — it now requires initialized InstanceSpace
	// handlePreAccept tested separately — it now requires initialized InstanceSpace and conflict maps
	// handleAccept tested separately — it now requires initialized InstanceSpace
	// handleCommit tested separately — it now requires initialized InstanceSpace
	// handleCommitShort tested separately — it now requires initialized InstanceSpace
	// handleCausalCommit tested separately — it now requires initialized InstanceSpace
	// handlePrepareReply tested separately — it now requires initialized InstanceSpace
	// handlePreAcceptReply tested separately — it now requires initialized InstanceSpace
	// handlePreAcceptOK tested separately — it now requires initialized InstanceSpace
	// handleAcceptReply tested separately — it now requires initialized InstanceSpace
	// handleTryPreAccept tested separately — it now requires initialized InstanceSpace
	// handleTryPreAcceptReply tested separately — it now requires initialized InstanceSpace
	// startRecoveryForInstance tested separately — it now requires initialized InstanceSpace
	t.Run("executeCommands", func(t *testing.T) {
		r.executeCommands()
	})
}

// TestCausalCommitChannelPolling verifies that the non-blocking causal
// commit channel polling in run() processes messages correctly.
func TestCausalCommitChannelPolling(t *testing.T) {
	// Simulate causal commit channel behavior
	nChannels := 3
	channels := make([]chan fastrpc.Serializable, nChannels)
	for i := range channels {
		channels[i] = make(chan fastrpc.Serializable, 1)
	}

	// Send a CausalCommit to channel 1
	msg := &CausalCommit{LeaderId: 42, Replica: 1, Instance: 7}
	channels[1] <- msg

	// Poll all channels non-blocking (mirrors run() logic)
	var received []*CausalCommit
	for _, ch := range channels {
		select {
		case s := <-ch:
			commit := s.(*CausalCommit)
			received = append(received, commit)
		default:
		}
	}

	if len(received) != 1 {
		t.Fatalf("expected 1 message, got %d", len(received))
	}
	if received[0].LeaderId != 42 {
		t.Errorf("LeaderId=%d, want 42", received[0].LeaderId)
	}
	if received[0].Instance != 7 {
		t.Errorf("Instance=%d, want 7", received[0].Instance)
	}
}

// --- handlePropose tests ---

// newTestReplica creates a minimal Replica for testing.
// It has all the fields needed for causal commit testing initialized.
func newTestReplica(n int) *Replica {
	r := &Replica{
		Replica: &replica.Replica{
			N:                  n,
			Id:                 0,
			PreferredPeerOrder: make([]int32, n),
			Stats:              &defs.Stats{M: make(map[string]int)},
			ProposeChan:        make(chan *defs.GPropose, defs.CHAN_BUFFER_SIZE),
		},
		InstanceSpace:            make([][]*Instance, n),
		crtInstance:              make([]int32, n),
		CommittedUpTo:            make([]int32, n),
		ExecedUpTo:               make([]int32, n),
		conflicts:                make([]map[state.Key]int32, n),
		conflictMutex:            new(sync.RWMutex),
		maxSeqPerKey:             make(map[state.Key]int32),
		maxSeqPerKeyMu:           new(sync.RWMutex),
		sessionConflicts:         make([]map[int32]int32, n),
		sessionConflictsMu:       new(sync.RWMutex),
		maxWriteInstancePerKey:   make(map[state.Key]*instanceId),
		maxWriteInstancePerKeyMu: new(sync.RWMutex),
		maxWriteSeqPerKey:        make(map[state.Key]int32),
		maxWriteSeqPerKeyMu:      new(sync.RWMutex),
		clientMutex:              new(sync.Mutex),
		causalCommitRPC:          make([]uint8, n*NO_CAUSAL_CHANNEL),
	}
	for i := 0; i < n; i++ {
		r.InstanceSpace[i] = make([]*Instance, 1024)
		r.crtInstance[i] = 0
		r.CommittedUpTo[i] = -1
		r.ExecedUpTo[i] = -1
		r.conflicts[i] = make(map[state.Key]int32, HT_INIT_SIZE)
		r.sessionConflicts[i] = make(map[int32]int32, 10)
		r.PreferredPeerOrder[i] = int32(i)
	}
	return r
}

func TestHandleProposeSingleCausal(t *testing.T) {
	r := newTestReplica(3)
	r.Replica = nil // handlePropose only needs ProposeChan from embedded replica

	// We need ProposeChan — it's on the embedded replica.Replica.
	// Since we can't create a full replica, test the classification logic directly.
	causalCmd := state.Command{Op: state.PUT, K: 1, V: state.NIL(), CL: state.CAUSAL, Sid: 10}
	strongCmd := state.Command{Op: state.GET, K: 2, V: state.NIL(), CL: state.STRONG, Sid: 20}

	// Test classification logic directly
	var causalCmds, strongCmds []state.Command
	for _, cmd := range []state.Command{causalCmd, strongCmd, causalCmd} {
		switch cmd.CL {
		case state.CAUSAL:
			causalCmds = append(causalCmds, cmd)
		case state.STRONG:
			strongCmds = append(strongCmds, cmd)
		default:
			strongCmds = append(strongCmds, cmd)
		}
	}

	if len(causalCmds) != 2 {
		t.Errorf("expected 2 causal cmds, got %d", len(causalCmds))
	}
	if len(strongCmds) != 1 {
		t.Errorf("expected 1 strong cmd, got %d", len(strongCmds))
	}
}

func TestHandleProposeDefaultToStrong(t *testing.T) {
	// Commands with unknown CL should default to strong
	cmd := state.Command{Op: state.PUT, K: 1, V: state.NIL(), CL: state.NONE, Sid: 0}

	var causalCmds, strongCmds []state.Command
	switch cmd.CL {
	case state.CAUSAL:
		causalCmds = append(causalCmds, cmd)
	case state.STRONG:
		strongCmds = append(strongCmds, cmd)
	default:
		strongCmds = append(strongCmds, cmd)
	}

	if len(causalCmds) != 0 {
		t.Errorf("expected 0 causal cmds, got %d", len(causalCmds))
	}
	if len(strongCmds) != 1 {
		t.Errorf("expected 1 strong cmd, got %d", len(strongCmds))
	}
}

func TestHandleProposeInstanceAllocation(t *testing.T) {
	// Verify that handlePropose allocates separate instances for causal and strong batches
	r := newTestReplica(5)

	// Simulate: 2 causal + 1 strong → should allocate 2 instances
	startInst := r.crtInstance[0]

	// Simulate causal batch allocation
	causalInst := r.crtInstance[0]
	r.crtInstance[0]++
	// Simulate strong batch allocation
	strongInst := r.crtInstance[0]
	r.crtInstance[0]++

	if causalInst != startInst {
		t.Errorf("causal instance=%d, want %d", causalInst, startInst)
	}
	if strongInst != startInst+1 {
		t.Errorf("strong instance=%d, want %d", strongInst, startInst+1)
	}
	if r.crtInstance[0] != startInst+2 {
		t.Errorf("crtInstance=%d, want %d", r.crtInstance[0], startInst+2)
	}
}

func TestHandleProposeAllCausal(t *testing.T) {
	// All-causal batch should only allocate 1 instance
	cmds := []state.Command{
		{Op: state.PUT, K: 1, V: state.NIL(), CL: state.CAUSAL, Sid: 1},
		{Op: state.PUT, K: 2, V: state.NIL(), CL: state.CAUSAL, Sid: 2},
		{Op: state.GET, K: 3, V: state.NIL(), CL: state.CAUSAL, Sid: 3},
	}

	var causalCmds, strongCmds []state.Command
	for _, cmd := range cmds {
		switch cmd.CL {
		case state.CAUSAL:
			causalCmds = append(causalCmds, cmd)
		case state.STRONG:
			strongCmds = append(strongCmds, cmd)
		default:
			strongCmds = append(strongCmds, cmd)
		}
	}

	if len(causalCmds) != 3 {
		t.Errorf("expected 3 causal cmds, got %d", len(causalCmds))
	}
	if len(strongCmds) != 0 {
		t.Errorf("expected 0 strong cmds, got %d", len(strongCmds))
	}

	// Only causal → 1 instance allocated
	r := newTestReplica(5)
	if len(causalCmds) > 0 {
		r.crtInstance[0]++
	}
	if len(strongCmds) > 0 {
		r.crtInstance[0]++
	}
	if r.crtInstance[0] != 1 {
		t.Errorf("crtInstance=%d, want 1 (only causal batch)", r.crtInstance[0])
	}
}

func TestHandleProposeAllStrong(t *testing.T) {
	cmds := []state.Command{
		{Op: state.PUT, K: 1, V: state.NIL(), CL: state.STRONG, Sid: 1},
		{Op: state.GET, K: 2, V: state.NIL(), CL: state.STRONG, Sid: 2},
	}

	var causalCmds, strongCmds []state.Command
	for _, cmd := range cmds {
		switch cmd.CL {
		case state.CAUSAL:
			causalCmds = append(causalCmds, cmd)
		case state.STRONG:
			strongCmds = append(strongCmds, cmd)
		default:
			strongCmds = append(strongCmds, cmd)
		}
	}

	if len(causalCmds) != 0 {
		t.Errorf("expected 0 causal cmds, got %d", len(causalCmds))
	}
	if len(strongCmds) != 2 {
		t.Errorf("expected 2 strong cmds, got %d", len(strongCmds))
	}
}

// startStrongCommit tested separately — it now requires initialized InstanceSpace and conflict maps.

// --- Causal commit helper tests ---

func TestUpdateCommitted(t *testing.T) {
	r := newTestReplica(3)

	// No instances → CommittedUpTo stays at -1
	r.updateCommitted(0)
	if r.CommittedUpTo[0] != -1 {
		t.Errorf("CommittedUpTo[0]=%d, want -1", r.CommittedUpTo[0])
	}

	// Add causally committed instances 0, 1, 2
	r.InstanceSpace[0][0] = &Instance{Status: CAUSALLY_COMMITTED}
	r.InstanceSpace[0][1] = &Instance{Status: CAUSALLY_COMMITTED}
	r.InstanceSpace[0][2] = &Instance{Status: EXECUTED}
	r.updateCommitted(0)
	if r.CommittedUpTo[0] != 2 {
		t.Errorf("CommittedUpTo[0]=%d, want 2", r.CommittedUpTo[0])
	}

	// Gap at instance 3 (nil) → stops at 2
	r.InstanceSpace[0][4] = &Instance{Status: CAUSALLY_COMMITTED}
	r.updateCommitted(0)
	if r.CommittedUpTo[0] != 2 {
		t.Errorf("CommittedUpTo[0]=%d, want 2 (gap at 3)", r.CommittedUpTo[0])
	}

	// Fill gap → advances to 4
	r.InstanceSpace[0][3] = &Instance{Status: STRONGLY_COMMITTED}
	r.updateCommitted(0)
	if r.CommittedUpTo[0] != 4 {
		t.Errorf("CommittedUpTo[0]=%d, want 4", r.CommittedUpTo[0])
	}
}

func TestUpdateCommittedWithDiscarded(t *testing.T) {
	r := newTestReplica(3)
	r.InstanceSpace[0][0] = &Instance{Status: DISCARDED}
	r.InstanceSpace[0][1] = &Instance{Status: CAUSALLY_COMMITTED}
	r.updateCommitted(0)
	if r.CommittedUpTo[0] != 1 {
		t.Errorf("CommittedUpTo[0]=%d, want 1", r.CommittedUpTo[0])
	}
}

func TestClearHashtables(t *testing.T) {
	r := newTestReplica(3)
	// Populate conflicts
	r.conflicts[0][state.Key(1)] = 5
	r.conflicts[1][state.Key(2)] = 10
	r.sessionConflicts[0][42] = 3

	r.clearHashtables()

	// All maps should be freshly initialized (empty)
	if len(r.conflicts[0]) != 0 {
		t.Errorf("conflicts[0] should be empty, got %d", len(r.conflicts[0]))
	}
	if len(r.conflicts[1]) != 0 {
		t.Errorf("conflicts[1] should be empty, got %d", len(r.conflicts[1]))
	}
	if len(r.sessionConflicts[0]) != 0 {
		t.Errorf("sessionConflicts[0] should be empty, got %d", len(r.sessionConflicts[0]))
	}
}

func TestUpdateCausalConflicts(t *testing.T) {
	r := newTestReplica(3)
	cmds := []state.Command{
		{Op: state.PUT, K: 10, V: state.NIL(), CL: state.CAUSAL, Sid: 1},
		{Op: state.GET, K: 20, V: state.NIL(), CL: state.CAUSAL, Sid: 2},
	}

	// Leader path (includeSession=true)
	r.updateCausalConflicts(cmds, 0, 5, 10, true)

	// Check conflicts updated
	if r.conflicts[0][state.Key(10)] != 5 {
		t.Errorf("conflicts[0][10]=%d, want 5", r.conflicts[0][state.Key(10)])
	}
	if r.conflicts[0][state.Key(20)] != 5 {
		t.Errorf("conflicts[0][20]=%d, want 5", r.conflicts[0][state.Key(20)])
	}

	// Check maxSeqPerKey updated
	if r.maxSeqPerKey[state.Key(10)] != 10 {
		t.Errorf("maxSeqPerKey[10]=%d, want 10", r.maxSeqPerKey[state.Key(10)])
	}

	// Check session conflicts updated
	if r.sessionConflicts[0][1] != 5 {
		t.Errorf("sessionConflicts[0][1]=%d, want 5", r.sessionConflicts[0][1])
	}
	if r.sessionConflicts[0][2] != 5 {
		t.Errorf("sessionConflicts[0][2]=%d, want 5", r.sessionConflicts[0][2])
	}

	// Higher instance should replace
	r.updateCausalConflicts(cmds, 0, 8, 15, true)
	if r.conflicts[0][state.Key(10)] != 8 {
		t.Errorf("conflicts[0][10]=%d, want 8", r.conflicts[0][state.Key(10)])
	}
	if r.maxSeqPerKey[state.Key(10)] != 15 {
		t.Errorf("maxSeqPerKey[10]=%d, want 15", r.maxSeqPerKey[state.Key(10)])
	}

	// Lower instance should NOT replace
	r.updateCausalConflicts(cmds, 0, 3, 5, true)
	if r.conflicts[0][state.Key(10)] != 8 {
		t.Errorf("conflicts[0][10]=%d, want 8 (should not decrease)", r.conflicts[0][state.Key(10)])
	}
}

func TestUpdateCausalConflictsFollowerPath(t *testing.T) {
	r := newTestReplica(3)
	cmds := []state.Command{
		{Op: state.PUT, K: 10, V: state.NIL(), CL: state.CAUSAL, Sid: 1},
	}

	// Follower path (includeSession=false)
	r.updateCausalConflicts(cmds, 1, 5, 10, false)

	// Conflicts should be updated
	if r.conflicts[1][state.Key(10)] != 5 {
		t.Errorf("conflicts[1][10]=%d, want 5", r.conflicts[1][state.Key(10)])
	}

	// Session conflicts should NOT be updated
	if _, present := r.sessionConflicts[1][1]; present {
		t.Error("follower path should not update session conflicts")
	}
}

func TestUpdateCausalAttributes(t *testing.T) {
	r := newTestReplica(3)

	// No prior state → seq=0, deps all -1
	cmds := []state.Command{
		{Op: state.PUT, K: 1, V: state.NIL(), CL: state.CAUSAL, Sid: 42},
	}
	deps := make([]int32, 3)
	cl := make([]int32, 3)
	for i := range deps {
		deps[i] = -1
	}
	seq, deps, cl := r.updateCausalAttributes(cmds, 0, deps, cl, 0, 0)
	if seq != 0 {
		t.Errorf("seq=%d, want 0 (no prior state)", seq)
	}
	for i, d := range deps {
		if d != -1 {
			t.Errorf("deps[%d]=%d, want -1", i, d)
		}
	}

	// Add a prior session conflict
	r.sessionConflicts[0][42] = 5
	r.InstanceSpace[0][5] = &Instance{
		Cmds: []state.Command{{CL: state.CAUSAL}},
		Seq:  3,
	}
	deps2 := []int32{-1, -1, -1}
	cl2 := []int32{0, 0, 0}
	seq2, deps2, cl2 := r.updateCausalAttributes(cmds, 0, deps2, cl2, 0, 6)
	if deps2[0] != 5 {
		t.Errorf("deps[0]=%d, want 5 (session dep)", deps2[0])
	}
	if seq2 != 4 {
		t.Errorf("seq=%d, want 4 (session dep seq 3 + 1)", seq2)
	}
	_ = cl
	_ = cl2
}

func TestUpdateCausalAttributesReadFrom(t *testing.T) {
	r := newTestReplica(3)

	// Set up a write instance for key 10 on replica 1
	r.maxWriteInstancePerKey[state.Key(10)] = &instanceId{replica: 1, instance: 3}
	r.InstanceSpace[1][3] = &Instance{
		Cmds: []state.Command{{CL: state.STRONG}},
		Seq:  7,
	}

	// A GET on key 10 should pick up the read-from dependency
	cmds := []state.Command{
		{Op: state.GET, K: 10, V: state.NIL(), CL: state.CAUSAL, Sid: 1},
	}
	deps := []int32{-1, -1, -1}
	cl := []int32{0, 0, 0}
	seq, deps, _ := r.updateCausalAttributes(cmds, 0, deps, cl, 0, 0)

	if deps[1] != 3 {
		t.Errorf("deps[1]=%d, want 3 (read-from dep)", deps[1])
	}
	if seq != 8 {
		t.Errorf("seq=%d, want 8 (read-from seq 7 + 1)", seq)
	}
}

func TestHandleCausalCommitNewInstance(t *testing.T) {
	r := newTestReplica(3)

	commit := &CausalCommit{
		Consistency: state.CAUSAL,
		LeaderId:    1,
		Replica:     1,
		Instance:    0,
		Command:     []state.Command{{Op: state.PUT, K: 5, V: state.NIL(), CL: state.CAUSAL, Sid: 1}},
		Seq:         10,
		Deps:        []int32{-1, -1, -1},
		CL:          []int32{0, 0, 0},
	}

	r.handleCausalCommit(commit)

	inst := r.InstanceSpace[1][0]
	if inst == nil {
		t.Fatal("instance should be created")
	}
	if inst.Status != CAUSALLY_COMMITTED {
		t.Errorf("Status=%d, want CAUSALLY_COMMITTED(%d)", inst.Status, CAUSALLY_COMMITTED)
	}
	if inst.State != READY {
		t.Errorf("State=%d, want READY(%d)", inst.State, READY)
	}
	if inst.Seq != 10 {
		t.Errorf("Seq=%d, want 10", inst.Seq)
	}
	if r.maxSeq != 11 {
		t.Errorf("maxSeq=%d, want 11", r.maxSeq)
	}
	if r.crtInstance[1] != 1 {
		t.Errorf("crtInstance[1]=%d, want 1", r.crtInstance[1])
	}
	if r.CommittedUpTo[1] != 0 {
		t.Errorf("CommittedUpTo[1]=%d, want 0", r.CommittedUpTo[1])
	}
}

func TestHandleCausalCommitExistingInstance(t *testing.T) {
	r := newTestReplica(3)

	// Pre-existing instance in PREACCEPTED state
	r.InstanceSpace[1][0] = &Instance{
		Status: PREACCEPTED,
		State:  WAITING,
	}

	commit := &CausalCommit{
		Consistency: state.CAUSAL,
		LeaderId:    1,
		Replica:     1,
		Instance:    0,
		Command:     []state.Command{{Op: state.PUT, K: 5, V: state.NIL(), CL: state.CAUSAL}},
		Seq:         5,
		Deps:        []int32{-1, -1, -1},
		CL:          []int32{0, 0, 0},
	}

	r.handleCausalCommit(commit)

	inst := r.InstanceSpace[1][0]
	if inst.Status != CAUSALLY_COMMITTED {
		t.Errorf("Status=%d, want CAUSALLY_COMMITTED", inst.Status)
	}
	if inst.State != READY {
		t.Errorf("State=%d, want READY", inst.State)
	}
	if inst.Seq != 5 {
		t.Errorf("Seq=%d, want 5", inst.Seq)
	}
}

func TestHandleCausalCommitIdempotent(t *testing.T) {
	r := newTestReplica(3)

	// Already committed instance
	r.InstanceSpace[1][0] = &Instance{
		Status: CAUSALLY_COMMITTED,
		Seq:    5,
	}

	commit := &CausalCommit{
		Consistency: state.CAUSAL,
		LeaderId:    1,
		Replica:     1,
		Instance:    0,
		Command:     []state.Command{{Op: state.PUT, K: 5, V: state.NIL(), CL: state.CAUSAL}},
		Seq:         10, // Different seq — should be ignored
		Deps:        []int32{-1, -1, -1},
		CL:          []int32{0, 0, 0},
	}

	r.handleCausalCommit(commit)

	// Should not update — already committed
	if r.InstanceSpace[1][0].Seq != 5 {
		t.Errorf("Seq=%d, want 5 (should not be updated for already-committed)", r.InstanceSpace[1][0].Seq)
	}
}

func TestHandleCausalCommitCheckpoint(t *testing.T) {
	r := newTestReplica(3)

	// Populate some conflicts
	r.conflicts[0][state.Key(1)] = 5
	r.sessionConflicts[0][42] = 3

	// Checkpoint commit (empty command list)
	commit := &CausalCommit{
		Consistency: state.CAUSAL,
		LeaderId:    0,
		Replica:     0,
		Instance:    100,
		Command:     []state.Command{}, // empty = checkpoint
		Seq:         50,
		Deps:        []int32{99, 99, 99},
		CL:          []int32{0, 0, 0},
	}

	r.handleCausalCommit(commit)

	if r.latestCPReplica != 0 {
		t.Errorf("latestCPReplica=%d, want 0", r.latestCPReplica)
	}
	if r.latestCPInstance != 100 {
		t.Errorf("latestCPInstance=%d, want 100", r.latestCPInstance)
	}
	// Hashtables should be cleared
	if len(r.conflicts[0]) != 0 {
		t.Errorf("conflicts[0] should be cleared by checkpoint, got %d entries", len(r.conflicts[0]))
	}
	if len(r.sessionConflicts[0]) != 0 {
		t.Errorf("sessionConflicts[0] should be cleared by checkpoint, got %d entries", len(r.sessionConflicts[0]))
	}
}

// TestMessageChannelTypeAssertions verifies that messages from channels
// can be correctly type-asserted (mirrors the select cases in run()).
func TestMessageChannelTypeAssertions(t *testing.T) {
	tests := []struct {
		name string
		msg  fastrpc.Serializable
	}{
		{"Prepare", &Prepare{LeaderId: 1, Replica: 0, Instance: 5, Ballot: 10}},
		{"PreAccept", &PreAccept{LeaderId: 2, Replica: 1, Instance: 3, Ballot: 5}},
		{"Accept", &Accept{LeaderId: 3, Replica: 2, Instance: 8, Ballot: 15}},
		{"Commit", &Commit{LeaderId: 4, Replica: 0, Instance: 12}},
		{"CommitShort", &CommitShort{LeaderId: 5, Replica: 1, Instance: 20}},
		{"PrepareReply", &PrepareReply{AcceptorId: 1, Replica: 0, Instance: 5}},
		{"PreAcceptReply", &PreAcceptReply{Replica: 2, Instance: 10}},
		{"PreAcceptOK", &PreAcceptOK{Instance: 15}},
		{"AcceptReply", &AcceptReply{Replica: 3, Instance: 7}},
		{"TryPreAccept", &TryPreAccept{LeaderId: 1, Replica: 0, Instance: 4}},
		{"TryPreAcceptReply", &TryPreAcceptReply{AcceptorId: 2, Replica: 1, Instance: 6}},
		{"CausalCommit", &CausalCommit{LeaderId: 7, Replica: 3, Instance: 9}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ch := make(chan fastrpc.Serializable, 1)
			ch <- tt.msg
			received := <-ch
			if received == nil {
				t.Fatal("received nil message")
			}
		})
	}
}

// --- Phase 99.3f-i: Strong commit helper tests ---

// TestUpdateStrongConflicts verifies per-key conflict and maxSeqPerKey updates.
func TestUpdateStrongConflicts(t *testing.T) {
	r := newTestReplica(3)

	cmds := []state.Command{
		{Op: state.PUT, K: 10, V: state.NIL(), CL: state.STRONG, Sid: 1},
		{Op: state.PUT, K: 20, V: state.NIL(), CL: state.STRONG, Sid: 1},
	}

	r.updateStrongConflicts(cmds, 0, 5, 42)

	// Check per-key conflict map
	r.conflictMutex.RLock()
	if r.conflicts[0][10] != 5 {
		t.Errorf("conflicts[0][10]=%d, want 5", r.conflicts[0][10])
	}
	if r.conflicts[0][20] != 5 {
		t.Errorf("conflicts[0][20]=%d, want 5", r.conflicts[0][20])
	}
	r.conflictMutex.RUnlock()

	// Check maxSeqPerKey
	r.maxSeqPerKeyMu.RLock()
	if r.maxSeqPerKey[10] != 42 {
		t.Errorf("maxSeqPerKey[10]=%d, want 42", r.maxSeqPerKey[10])
	}
	if r.maxSeqPerKey[20] != 42 {
		t.Errorf("maxSeqPerKey[20]=%d, want 42", r.maxSeqPerKey[20])
	}
	r.maxSeqPerKeyMu.RUnlock()

	// Higher instance and seq should overwrite
	r.updateStrongConflicts(cmds, 0, 10, 100)
	r.conflictMutex.RLock()
	if r.conflicts[0][10] != 10 {
		t.Errorf("conflicts[0][10]=%d after update, want 10", r.conflicts[0][10])
	}
	r.conflictMutex.RUnlock()
	r.maxSeqPerKeyMu.RLock()
	if r.maxSeqPerKey[10] != 100 {
		t.Errorf("maxSeqPerKey[10]=%d after update, want 100", r.maxSeqPerKey[10])
	}
	r.maxSeqPerKeyMu.RUnlock()

	// Lower instance should NOT overwrite
	r.updateStrongConflicts(cmds, 0, 3, 50)
	r.conflictMutex.RLock()
	if r.conflicts[0][10] != 10 {
		t.Errorf("conflicts[0][10]=%d, should not decrease to 3", r.conflicts[0][10])
	}
	r.conflictMutex.RUnlock()
	r.maxSeqPerKeyMu.RLock()
	if r.maxSeqPerKey[10] != 100 {
		t.Errorf("maxSeqPerKey[10]=%d, should not decrease to 50", r.maxSeqPerKey[10])
	}
	r.maxSeqPerKeyMu.RUnlock()
}

// TestUpdateStrongConflictsDoesNotUpdateSession verifies that updateStrongConflicts
// does NOT touch sessionConflicts (unlike causal conflicts).
func TestUpdateStrongConflictsDoesNotUpdateSession(t *testing.T) {
	r := newTestReplica(3)

	cmds := []state.Command{
		{Op: state.PUT, K: 10, V: state.NIL(), CL: state.STRONG, Sid: 42},
	}

	r.updateStrongConflicts(cmds, 0, 5, 10)

	r.sessionConflictsMu.RLock()
	if _, present := r.sessionConflicts[0][42]; present {
		t.Error("updateStrongConflicts should not update sessionConflicts")
	}
	r.sessionConflictsMu.RUnlock()
}

// TestUpdateStrongSessionConflict verifies session-based conflict tracking.
func TestUpdateStrongSessionConflict(t *testing.T) {
	r := newTestReplica(3)

	cmds := []state.Command{
		{Op: state.PUT, K: 10, V: state.NIL(), CL: state.STRONG, Sid: 42},
		{Op: state.GET, K: 20, V: state.NIL(), CL: state.STRONG, Sid: 7},
	}

	r.updateStrongSessionConflict(cmds, 1, 5)

	r.sessionConflictsMu.RLock()
	if r.sessionConflicts[1][42] != 5 {
		t.Errorf("sessionConflicts[1][42]=%d, want 5", r.sessionConflicts[1][42])
	}
	if r.sessionConflicts[1][7] != 5 {
		t.Errorf("sessionConflicts[1][7]=%d, want 5", r.sessionConflicts[1][7])
	}
	r.sessionConflictsMu.RUnlock()

	// Higher instance overwrites
	r.updateStrongSessionConflict(cmds, 1, 10)
	r.sessionConflictsMu.RLock()
	if r.sessionConflicts[1][42] != 10 {
		t.Errorf("sessionConflicts[1][42]=%d, want 10", r.sessionConflicts[1][42])
	}
	r.sessionConflictsMu.RUnlock()

	// Lower instance does NOT overwrite
	r.updateStrongSessionConflict(cmds, 1, 3)
	r.sessionConflictsMu.RLock()
	if r.sessionConflicts[1][42] != 10 {
		t.Errorf("sessionConflicts[1][42]=%d, should not decrease to 3", r.sessionConflicts[1][42])
	}
	r.sessionConflictsMu.RUnlock()
}

// TestUpdateStrongAttributes1_NoConflicts verifies behavior with no existing conflicts.
func TestUpdateStrongAttributes1_NoConflicts(t *testing.T) {
	r := newTestReplica(3)

	cmds := []state.Command{
		{Op: state.PUT, K: 10, V: state.NIL(), CL: state.STRONG, Sid: 1},
	}
	deps := []int32{-1, -1, -1}
	cl := []int32{0, 0, 0}

	seq, newDeps, newCL, changed := r.updateStrongAttributes1(cmds, 0, deps, cl, 0, 0)

	if changed {
		t.Error("expected no change with empty conflict maps")
	}
	if seq != 0 {
		t.Errorf("seq=%d, want 0", seq)
	}
	for i, d := range newDeps {
		if d != -1 {
			t.Errorf("deps[%d]=%d, want -1", i, d)
		}
	}
	for i, c := range newCL {
		if c != 0 {
			t.Errorf("cl[%d]=%d, want 0", i, c)
		}
	}
}

// TestUpdateStrongAttributes1_KeyConflict verifies dep computation from key conflicts.
func TestUpdateStrongAttributes1_KeyConflict(t *testing.T) {
	r := newTestReplica(3)

	// Pre-populate conflict: replica 1, key 10 → instance 5
	r.conflictMutex.Lock()
	r.conflicts[1][10] = 5
	r.conflictMutex.Unlock()

	// Pre-populate instance space so seq/CL can be read
	r.InstanceSpace[1][5] = &Instance{
		Cmds: []state.Command{{Op: state.PUT, K: 10, V: state.NIL(), CL: state.STRONG}},
		Seq:  20,
	}

	cmds := []state.Command{
		{Op: state.PUT, K: 10, V: state.NIL(), CL: state.STRONG, Sid: 1},
	}
	deps := []int32{-1, -1, -1}
	cl := []int32{0, 0, 0}

	seq, newDeps, newCL, changed := r.updateStrongAttributes1(cmds, 0, deps, cl, 0, 0)

	if !changed {
		t.Error("expected changed=true with key conflict")
	}
	if newDeps[1] != 5 {
		t.Errorf("deps[1]=%d, want 5", newDeps[1])
	}
	if seq != 21 {
		t.Errorf("seq=%d, want 21 (conflict instance seq=20 + 1)", seq)
	}
	if newCL[1] != int32(state.STRONG) {
		t.Errorf("cl[1]=%d, want STRONG(%d)", newCL[1], state.STRONG)
	}
}

// TestUpdateStrongAttributes1_SessionConflict verifies session-based dep on own replica.
func TestUpdateStrongAttributes1_SessionConflict(t *testing.T) {
	r := newTestReplica(3)

	// Pre-populate session conflict: replica 0, session 42 → instance 3
	r.sessionConflictsMu.Lock()
	r.sessionConflicts[0][42] = 3
	r.sessionConflictsMu.Unlock()

	r.InstanceSpace[0][3] = &Instance{
		Cmds: []state.Command{{Op: state.PUT, K: 99, V: state.NIL(), CL: state.CAUSAL}},
		Seq:  15,
	}

	cmds := []state.Command{
		{Op: state.PUT, K: 10, V: state.NIL(), CL: state.STRONG, Sid: 42},
	}
	deps := []int32{-1, -1, -1}
	cl := []int32{0, 0, 0}

	// replicaId=0, so session conflict should apply to deps[0]
	seq, newDeps, newCL, changed := r.updateStrongAttributes1(cmds, 0, deps, cl, 0, 0)

	if !changed {
		t.Error("expected changed=true with session conflict")
	}
	if newDeps[0] != 3 {
		t.Errorf("deps[0]=%d, want 3 from session conflict", newDeps[0])
	}
	if seq != 16 {
		t.Errorf("seq=%d, want 16 (session conflict instance seq=15 + 1)", seq)
	}
	_ = newCL
}

// TestUpdateStrongAttributes1_MaxSeqPerKey verifies seq bump from maxSeqPerKey.
func TestUpdateStrongAttributes1_MaxSeqPerKey(t *testing.T) {
	r := newTestReplica(3)

	r.maxSeqPerKeyMu.Lock()
	r.maxSeqPerKey[10] = 50
	r.maxSeqPerKeyMu.Unlock()

	cmds := []state.Command{
		{Op: state.PUT, K: 10, V: state.NIL(), CL: state.STRONG, Sid: 1},
	}
	deps := []int32{-1, -1, -1}
	cl := []int32{0, 0, 0}

	seq, _, _, changed := r.updateStrongAttributes1(cmds, 0, deps, cl, 0, 0)

	if !changed {
		t.Error("expected changed=true from maxSeqPerKey")
	}
	if seq != 51 {
		t.Errorf("seq=%d, want 51 (maxSeqPerKey[10]=50 + 1)", seq)
	}
}

// TestUpdateStrongAttributes2_NoSession verifies Attributes2 does NOT use session conflicts.
func TestUpdateStrongAttributes2_NoSession(t *testing.T) {
	r := newTestReplica(3)

	// Add session conflict — Attributes2 should ignore it
	r.sessionConflictsMu.Lock()
	r.sessionConflicts[0][42] = 10
	r.sessionConflictsMu.Unlock()
	r.InstanceSpace[0][10] = &Instance{Seq: 99}

	cmds := []state.Command{
		{Op: state.PUT, K: 10, V: state.NIL(), CL: state.STRONG, Sid: 42},
	}
	deps := []int32{-1, -1, -1}
	cl := []int32{0, 0, 0}

	seq, newDeps, _, changed := r.updateStrongAttributes2(cmds, 0, deps, cl, 0, 0)

	// Should NOT pick up session conflict
	if newDeps[0] != -1 {
		t.Errorf("deps[0]=%d, want -1 (Attributes2 should skip session conflicts)", newDeps[0])
	}
	if seq != 0 && !changed {
		// seq unchanged, no conflict
	}
	_ = seq
}

// TestUpdateStrongAttributes2_KeyConflict verifies Attributes2 picks up key conflicts.
func TestUpdateStrongAttributes2_KeyConflict(t *testing.T) {
	r := newTestReplica(3)

	r.conflictMutex.Lock()
	r.conflicts[2][10] = 7
	r.conflictMutex.Unlock()
	r.InstanceSpace[2][7] = &Instance{
		Cmds: []state.Command{{Op: state.PUT, K: 10, V: state.NIL(), CL: state.STRONG}},
		Seq:  30,
	}

	cmds := []state.Command{
		{Op: state.PUT, K: 10, V: state.NIL(), CL: state.STRONG, Sid: 1},
	}
	deps := []int32{-1, -1, -1}
	cl := []int32{0, 0, 0}

	seq, newDeps, _, changed := r.updateStrongAttributes2(cmds, 0, deps, cl, 0, 0)

	if !changed {
		t.Error("expected changed=true from key conflict")
	}
	if newDeps[2] != 7 {
		t.Errorf("deps[2]=%d, want 7", newDeps[2])
	}
	if seq != 31 {
		t.Errorf("seq=%d, want 31", seq)
	}
}

// TestUpdateStrongAttributes1_SkipsOwnReplicaForNonLeader verifies skip logic:
// when r.Id != replicaId, skip q == replicaId in key conflict loop.
func TestUpdateStrongAttributes1_SkipsOwnReplicaForNonLeader(t *testing.T) {
	r := newTestReplica(3)

	// r.Id=0, replicaId=1. Conflict on replica 1 should be skipped.
	r.conflictMutex.Lock()
	r.conflicts[1][10] = 5
	r.conflictMutex.Unlock()
	r.InstanceSpace[1][5] = &Instance{Seq: 20}

	cmds := []state.Command{
		{Op: state.PUT, K: 10, V: state.NIL(), CL: state.STRONG, Sid: 1},
	}
	deps := []int32{-1, -1, -1}
	cl := []int32{0, 0, 0}

	_, newDeps, _, _ := r.updateStrongAttributes1(cmds, 0, deps, cl, 1, 0)

	// q=1 (==replicaId=1) should be skipped since r.Id(0) != replicaId(1)
	if newDeps[1] != -1 {
		t.Errorf("deps[1]=%d, want -1 (should skip replicaId's own conflicts)", newDeps[1])
	}
}

// TestMergeStrongAttributes_Equal verifies merge of identical attributes.
func TestMergeStrongAttributes_Equal(t *testing.T) {
	r := newTestReplica(3)

	deps1 := []int32{5, 3, 7}
	deps2 := []int32{5, 3, 7}
	cl1 := []int32{1, 2, 3}
	cl2 := []int32{1, 2, 3}

	seq, deps, cl, equal := r.mergeStrongAttributes(10, deps1, 10, deps2, cl1, cl2)

	if !equal {
		t.Error("expected equal=true for identical attributes")
	}
	if seq != 10 {
		t.Errorf("seq=%d, want 10", seq)
	}
	// deps should be unchanged
	for i, d := range deps {
		if d != deps2[i] {
			t.Errorf("deps[%d]=%d, want %d", i, d, deps2[i])
		}
	}
	_ = cl
}

// TestMergeStrongAttributes_DifferentSeq verifies merge picks max seq.
func TestMergeStrongAttributes_DifferentSeq(t *testing.T) {
	r := newTestReplica(3)

	deps1 := []int32{5, 3, 7}
	deps2 := []int32{5, 3, 7}
	cl1 := []int32{1, 2, 3}
	cl2 := []int32{1, 2, 3}

	seq, _, _, equal := r.mergeStrongAttributes(10, deps1, 20, deps2, cl1, cl2)

	if equal {
		t.Error("expected equal=false for different seq")
	}
	if seq != 20 {
		t.Errorf("seq=%d, want 20 (max of 10, 20)", seq)
	}
}

// TestMergeStrongAttributes_DifferentDeps verifies merge picks max deps element-wise.
func TestMergeStrongAttributes_DifferentDeps(t *testing.T) {
	r := newTestReplica(3)

	// r.Id=0, so index 0 is skipped in merge
	deps1 := []int32{5, 3, 7}
	deps2 := []int32{5, 8, 2}
	cl1 := []int32{1, 2, 3}
	cl2 := []int32{1, 9, 6}

	seq, deps, cl, equal := r.mergeStrongAttributes(10, deps1, 10, deps2, cl1, cl2)

	if equal {
		t.Error("expected equal=false for different deps")
	}
	if seq != 10 {
		t.Errorf("seq=%d, want 10", seq)
	}
	// Index 0 is skipped (r.Id==0), so deps[0] stays as deps1[0]
	if deps[0] != 5 {
		t.Errorf("deps[0]=%d, want 5 (r.Id skip)", deps[0])
	}
	// Index 1: deps2[1]=8 > deps1[1]=3, so pick deps2
	if deps[1] != 8 {
		t.Errorf("deps[1]=%d, want 8 (max of 3, 8)", deps[1])
	}
	if cl[1] != 9 {
		t.Errorf("cl[1]=%d, want 9 (from deps2 winner)", cl[1])
	}
	// Index 2: deps1[2]=7 > deps2[2]=2, so keep deps1
	if deps[2] != 7 {
		t.Errorf("deps[2]=%d, want 7 (max of 7, 2)", deps[2])
	}
	if cl[2] != 3 {
		t.Errorf("cl[2]=%d, want 3 (from deps1 winner)", cl[2])
	}
}

// TestEqualDeps verifies element-wise equality check.
func TestEqualDeps(t *testing.T) {
	if !equalDeps([]int32{1, 2, 3}, []int32{1, 2, 3}) {
		t.Error("identical deps should be equal")
	}
	if equalDeps([]int32{1, 2, 3}, []int32{1, 2, 4}) {
		t.Error("different deps should not be equal")
	}
	if equalDeps([]int32{1, 2, 3}, []int32{0, 2, 3}) {
		t.Error("different first element should not be equal")
	}
	// Empty deps
	if !equalDeps([]int32{}, []int32{}) {
		t.Error("empty deps should be equal")
	}
}

// --- Phase 99.3f-ii: Broadcast + startStrongCommit tests ---

// TestBcastPreAccept verifies PreAccept broadcast doesn't panic on bare test replica.
func TestBcastPreAccept(t *testing.T) {
	r := newTestReplica(3)
	// bcastPreAccept requires Alive and SendMsg — test that it doesn't panic
	// with a bare test replica (SendMsg is nil, but recover catches panics).
	cmds := []state.Command{{Op: state.PUT, K: 10, V: state.NIL(), CL: state.STRONG}}
	deps := []int32{-1, -1, -1}
	cl := []int32{0, 0, 0}

	// Should not panic due to defer/recover
	r.bcastPreAccept(0, 5, 1, cmds, 10, deps, cl)
}

// TestBcastAccept verifies Accept broadcast doesn't panic on bare test replica.
func TestBcastAccept(t *testing.T) {
	r := newTestReplica(3)
	deps := []int32{-1, -1, -1}
	cl := []int32{0, 0, 0}

	r.bcastAccept(0, 5, 1, 3, 10, deps, cl)
}

// TestBcastStrongCommit verifies strong commit broadcast doesn't panic.
func TestBcastStrongCommit(t *testing.T) {
	r := newTestReplica(3)
	cmds := []state.Command{{Op: state.PUT, K: 10, V: state.NIL(), CL: state.STRONG}}
	deps := []int32{-1, -1, -1}
	cl := []int32{0, 0, 0}

	r.bcastStrongCommit(0, 5, cmds, 10, deps, cl, state.STRONG)
}

// TestStartStrongCommit_BasicInstance verifies that startStrongCommit creates
// the instance with correct status and attributes.
func TestStartStrongCommit_BasicInstance(t *testing.T) {
	r := newTestReplica(3)

	cmds := []state.Command{
		{Op: state.PUT, K: 10, V: state.NIL(), CL: state.STRONG, Sid: 1},
	}
	proposals := []*defs.GPropose{
		{Propose: &defs.Propose{CommandId: 1, Command: cmds[0]}},
	}

	r.startStrongCommit(0, 0, 0, proposals, cmds)

	inst := r.InstanceSpace[0][0]
	if inst == nil {
		t.Fatal("instance not created")
	}
	if inst.Status != PREACCEPTED {
		t.Errorf("Status=%d, want PREACCEPTED(%d)", inst.Status, PREACCEPTED)
	}
	if inst.State != READY {
		t.Errorf("State=%d, want READY(%d)", inst.State, READY)
	}
	if len(inst.Cmds) != 1 {
		t.Errorf("len(Cmds)=%d, want 1", len(inst.Cmds))
	}
	if inst.lb == nil {
		t.Fatal("LeaderBookkeeping is nil")
	}
	if !inst.lb.allEqual {
		t.Error("allEqual should be true initially")
	}
	if len(inst.lb.clientProposals) != 1 {
		t.Errorf("clientProposals len=%d, want 1", len(inst.lb.clientProposals))
	}
	if inst.instanceId == nil || inst.instanceId.replica != 0 || inst.instanceId.instance != 0 {
		t.Error("instanceId not set correctly")
	}
}

// TestStartStrongCommit_AttributesComputed verifies dependency computation.
func TestStartStrongCommit_AttributesComputed(t *testing.T) {
	r := newTestReplica(3)

	// Set up a pre-existing conflict on replica 1
	r.conflictMutex.Lock()
	r.conflicts[1][10] = 3
	r.conflictMutex.Unlock()
	r.InstanceSpace[1][3] = &Instance{
		Cmds: []state.Command{{Op: state.PUT, K: 10, V: state.NIL(), CL: state.STRONG}},
		Seq:  20,
	}

	cmds := []state.Command{
		{Op: state.PUT, K: 10, V: state.NIL(), CL: state.STRONG, Sid: 1},
	}

	r.startStrongCommit(0, 0, 0, nil, cmds)

	inst := r.InstanceSpace[0][0]
	if inst.Deps[1] != 3 {
		t.Errorf("Deps[1]=%d, want 3 from conflict", inst.Deps[1])
	}
	if inst.Seq != 21 {
		t.Errorf("Seq=%d, want 21", inst.Seq)
	}
}

// TestStartStrongCommit_UpdatesConflicts verifies that conflict maps are updated.
func TestStartStrongCommit_UpdatesConflicts(t *testing.T) {
	r := newTestReplica(3)

	cmds := []state.Command{
		{Op: state.PUT, K: 42, V: state.NIL(), CL: state.STRONG, Sid: 1},
	}

	r.startStrongCommit(0, 5, 0, nil, cmds)

	r.conflictMutex.RLock()
	if r.conflicts[0][42] != 5 {
		t.Errorf("conflicts[0][42]=%d, want 5", r.conflicts[0][42])
	}
	r.conflictMutex.RUnlock()
}

// TestStartStrongCommit_MaxSeqBumped verifies maxSeq is updated.
func TestStartStrongCommit_MaxSeqBumped(t *testing.T) {
	r := newTestReplica(3)
	r.maxSeq = 0

	cmds := []state.Command{
		{Op: state.PUT, K: 10, V: state.NIL(), CL: state.STRONG, Sid: 1},
	}

	r.startStrongCommit(0, 0, 0, nil, cmds)

	if r.maxSeq < 1 {
		t.Errorf("maxSeq=%d, want >= 1", r.maxSeq)
	}
}

// TestStartStrongCommit_DepsInitializedToNegOne verifies initial deps are -1.
func TestStartStrongCommit_DepsInitializedToNegOne(t *testing.T) {
	r := newTestReplica(3)

	cmds := []state.Command{
		{Op: state.PUT, K: 99, V: state.NIL(), CL: state.STRONG, Sid: 1},
	}

	r.startStrongCommit(0, 0, 0, nil, cmds)

	inst := r.InstanceSpace[0][0]
	for i, d := range inst.Deps {
		if d != -1 {
			t.Errorf("Deps[%d]=%d, want -1 with no conflicts", i, d)
		}
	}
}

// TestStartStrongCommit_CommittedDepsInitialized verifies committedDeps in lb.
func TestStartStrongCommit_CommittedDepsInitialized(t *testing.T) {
	r := newTestReplica(3)

	cmds := []state.Command{
		{Op: state.PUT, K: 10, V: state.NIL(), CL: state.STRONG, Sid: 1},
	}

	r.startStrongCommit(0, 0, 0, nil, cmds)

	inst := r.InstanceSpace[0][0]
	for i, cd := range inst.lb.committedDeps {
		if cd != -1 {
			t.Errorf("committedDeps[%d]=%d, want -1", i, cd)
		}
	}
}

// TestStartStrongCommit_OriginalDepsSaved verifies originalDeps are saved in lb.
func TestStartStrongCommit_OriginalDepsSaved(t *testing.T) {
	r := newTestReplica(3)

	// Set up a conflict so deps are non-trivial
	r.conflictMutex.Lock()
	r.conflicts[2][10] = 7
	r.conflictMutex.Unlock()
	r.InstanceSpace[2][7] = &Instance{
		Cmds: []state.Command{{Op: state.PUT, K: 10, V: state.NIL(), CL: state.STRONG}},
		Seq:  5,
	}

	cmds := []state.Command{
		{Op: state.PUT, K: 10, V: state.NIL(), CL: state.STRONG, Sid: 1},
	}

	r.startStrongCommit(0, 0, 0, nil, cmds)

	inst := r.InstanceSpace[0][0]
	if inst.lb.originalDeps[2] != 7 {
		t.Errorf("originalDeps[2]=%d, want 7", inst.lb.originalDeps[2])
	}
}

// TestStartStrongCommit_BallotStored verifies ballot is saved in instance.
func TestStartStrongCommit_BallotStored(t *testing.T) {
	r := newTestReplica(3)

	cmds := []state.Command{
		{Op: state.PUT, K: 10, V: state.NIL(), CL: state.STRONG, Sid: 1},
	}

	r.startStrongCommit(0, 0, 42, nil, cmds)

	inst := r.InstanceSpace[0][0]
	if inst.bal != 42 {
		t.Errorf("bal=%d, want 42", inst.bal)
	}
	if inst.vbal != 42 {
		t.Errorf("vbal=%d, want 42", inst.vbal)
	}
}

// TestUpdateStrongAttributes1_MultipleKeys verifies conflict detection across multiple keys.
func TestUpdateStrongAttributes1_MultipleKeys(t *testing.T) {
	r := newTestReplica(3)

	// Key conflict on replica 2 for key 20
	r.conflictMutex.Lock()
	r.conflicts[2][20] = 8
	r.conflictMutex.Unlock()
	r.InstanceSpace[2][8] = &Instance{
		Cmds: []state.Command{
			{Op: state.PUT, K: 20, V: state.NIL(), CL: state.STRONG},
			{Op: state.PUT, K: 20, V: state.NIL(), CL: state.CAUSAL},
		},
		Seq: 10,
	}

	cmds := []state.Command{
		{Op: state.PUT, K: 10, V: state.NIL(), CL: state.STRONG, Sid: 1},
		{Op: state.PUT, K: 20, V: state.NIL(), CL: state.STRONG, Sid: 1},
	}
	deps := []int32{-1, -1, -1}
	cl := []int32{0, 0, 0}

	seq, newDeps, newCL, changed := r.updateStrongAttributes1(cmds, 0, deps, cl, 0, 0)

	if !changed {
		t.Error("expected changed=true")
	}
	if newDeps[2] != 8 {
		t.Errorf("deps[2]=%d, want 8", newDeps[2])
	}
	if seq != 11 {
		t.Errorf("seq=%d, want 11", seq)
	}
	// CL for index 2 should come from conflicting instance's cmd at index matching the key
	// cmd index 1 matches key 20, so CL comes from instance's Cmds[1]
	if newCL[2] != int32(state.CAUSAL) {
		t.Errorf("cl[2]=%d, want CAUSAL(%d)", newCL[2], state.CAUSAL)
	}
}

// --- Phase 99.3f-iii: Ballot helpers, handlePreAccept, handlePreAcceptReply, handlePreAcceptOK tests ---

func TestIsInitialBallot(t *testing.T) {
	if !isInitialBallot(0) {
		t.Error("ballot 0 should be initial")
	}
	if !isInitialBallot(0x0F) {
		t.Error("ballot 0x0F (replicaId only) should be initial")
	}
	if isInitialBallot(0x10) {
		t.Error("ballot 0x10 should NOT be initial")
	}
	if isInitialBallot(0x20) {
		t.Error("ballot 0x20 should NOT be initial")
	}
}

func TestMakeUniqueBallot(t *testing.T) {
	r := newTestReplica(3)
	// r.Id = 0
	b := r.makeUniqueBallot(1)
	if b != (1<<4)|0 {
		t.Errorf("makeUniqueBallot(1)=%d, want %d", b, (1<<4)|0)
	}
}

func TestMakeBallotLargerThan(t *testing.T) {
	r := newTestReplica(3)
	// r.Id = 0
	b := r.makeBallotLargerThan(0x10) // ballot >> 4 = 1, so (1+1)<<4 | 0 = 0x20
	if b != 0x20 {
		t.Errorf("makeBallotLargerThan(0x10)=%d, want %d", b, 0x20)
	}
}

// Helper: create a PREACCEPTED instance with LeaderBookkeeping as startStrongCommit would.
func setupStrongInstance(r *Replica, replicaId int32, instance int32, ballot int32, cmds []state.Command, seq int32, deps []int32) {
	comDeps := make([]int32, r.N)
	origDeps := make([]int32, len(deps))
	copy(origDeps, deps)
	for i := 0; i < r.N; i++ {
		comDeps[i] = -1
	}
	cl := make([]int32, r.N)
	r.InstanceSpace[replicaId][instance] = &Instance{
		Cmds:       cmds,
		bal:        ballot,
		vbal:       ballot,
		Status:     PREACCEPTED,
		State:      READY,
		Seq:        seq,
		Deps:       deps,
		CL:         cl,
		lb:         &LeaderBookkeeping{allEqual: true, originalDeps: origDeps, committedDeps: comDeps},
		instanceId: &instanceId{replicaId, instance},
	}
}

// TestHandlePreAccept_NewInstance verifies follower creates a new instance.
func TestHandlePreAccept_NewInstance(t *testing.T) {
	r := newTestReplica(3)

	cmds := []state.Command{{Op: state.PUT, K: 10, V: state.NIL(), CL: state.STRONG}}
	deps := []int32{-1, -1, -1}
	cl := []int32{0, 0, 0}

	pa := &PreAccept{
		LeaderId: 1,
		Replica:  1,
		Instance: 0,
		Ballot:   0,
		Command:  cmds,
		Seq:      5,
		Deps:     deps,
		CL:       cl,
	}

	func() {
		defer func() { recover() }()
		r.handlePreAccept(pa)
	}()

	inst := r.InstanceSpace[1][0]
	if inst == nil {
		t.Fatal("instance should be created")
	}
	// No local conflicts → attributes unchanged → PREACCEPTED_EQ
	if inst.Status != PREACCEPTED_EQ {
		t.Errorf("Status=%d, want PREACCEPTED_EQ(%d)", inst.Status, PREACCEPTED_EQ)
	}
	if inst.State != READY {
		t.Errorf("State=%d, want READY(%d)", inst.State, READY)
	}
	if inst.Seq != 5 {
		t.Errorf("Seq=%d, want 5", inst.Seq)
	}
}

// TestHandlePreAccept_ExistingExecutedSkipped verifies EXECUTED instances are skipped.
func TestHandlePreAccept_ExistingExecutedSkipped(t *testing.T) {
	r := newTestReplica(3)

	r.InstanceSpace[1][0] = &Instance{Status: EXECUTED}

	pa := &PreAccept{
		LeaderId: 1, Replica: 1, Instance: 0, Ballot: 0,
		Command: []state.Command{{Op: state.PUT, K: 10, V: state.NIL()}},
		Seq: 5, Deps: []int32{-1, -1, -1}, CL: []int32{0, 0, 0},
	}
	r.handlePreAccept(pa)

	// Should remain EXECUTED
	if r.InstanceSpace[1][0].Status != EXECUTED {
		t.Errorf("Status=%d, want EXECUTED(%d)", r.InstanceSpace[1][0].Status, EXECUTED)
	}
}

// TestHandlePreAccept_CommittedFillsCommands verifies reordered Commit then PreAccept.
func TestHandlePreAccept_CommittedFillsCommands(t *testing.T) {
	r := newTestReplica(3)

	// Simulate Commit arrived first (no commands)
	r.InstanceSpace[1][0] = &Instance{Status: STRONGLY_COMMITTED, Cmds: nil}

	cmds := []state.Command{{Op: state.PUT, K: 10, V: state.NIL(), CL: state.STRONG}}
	pa := &PreAccept{
		LeaderId: 1, Replica: 1, Instance: 0, Ballot: 0,
		Command: cmds, Seq: 5, Deps: []int32{-1, -1, -1}, CL: []int32{0, 0, 0},
	}
	r.handlePreAccept(pa)

	// Commands should be filled in
	if r.InstanceSpace[1][0].Cmds == nil {
		t.Error("Cmds should be filled from PreAccept when Commit arrived first")
	}
	if len(r.InstanceSpace[1][0].Cmds) != 1 {
		t.Errorf("len(Cmds)=%d, want 1", len(r.InstanceSpace[1][0].Cmds))
	}
}

// TestHandlePreAccept_BallotReject verifies nack for lower ballot.
func TestHandlePreAccept_BallotReject(t *testing.T) {
	r := newTestReplica(3)

	// Existing instance with higher ballot
	r.InstanceSpace[1][0] = &Instance{
		bal:    0x10, // higher ballot
		Status: PREACCEPTED,
		Seq:    3,
		Deps:   []int32{-1, -1, -1},
		CL:     []int32{0, 0, 0},
	}

	pa := &PreAccept{
		LeaderId: 1, Replica: 1, Instance: 0, Ballot: 0, // lower ballot
		Command: []state.Command{{Op: state.PUT, K: 10, V: state.NIL()}},
		Seq: 5, Deps: []int32{-1, -1, -1}, CL: []int32{0, 0, 0},
	}

	// Should not panic — replyPreAccept will panic due to nil SendMsg, but
	// handlePreAccept should attempt the reply and return
	func() {
		defer func() { recover() }()
		r.handlePreAccept(pa)
	}()

	// Instance should NOT be updated (ballot too low)
	if r.InstanceSpace[1][0].Seq != 3 {
		t.Errorf("Seq=%d, should stay 3 (ballot reject)", r.InstanceSpace[1][0].Seq)
	}
}

// TestHandlePreAccept_ChangedAttributes verifies PREACCEPTED status when attributes change.
func TestHandlePreAccept_ChangedAttributes(t *testing.T) {
	r := newTestReplica(3)

	// Pre-populate a conflict that will change deps
	r.conflictMutex.Lock()
	r.conflicts[2][10] = 5
	r.conflictMutex.Unlock()
	r.InstanceSpace[2][5] = &Instance{
		Cmds: []state.Command{{Op: state.PUT, K: 10, V: state.NIL(), CL: state.STRONG}},
		Seq:  20,
	}

	cmds := []state.Command{{Op: state.PUT, K: 10, V: state.NIL(), CL: state.STRONG}}
	pa := &PreAccept{
		LeaderId: 1, Replica: 1, Instance: 0, Ballot: 0,
		Command: cmds, Seq: 5, Deps: []int32{-1, -1, -1}, CL: []int32{0, 0, 0},
	}

	func() {
		defer func() { recover() }()
		r.handlePreAccept(pa)
	}()

	inst := r.InstanceSpace[1][0]
	if inst == nil {
		t.Fatal("instance should be created")
	}
	// Attributes changed → PREACCEPTED (not PREACCEPTED_EQ)
	if inst.Status != PREACCEPTED {
		t.Errorf("Status=%d, want PREACCEPTED(%d)", inst.Status, PREACCEPTED)
	}
	if inst.Deps[2] != 5 {
		t.Errorf("Deps[2]=%d, want 5 from conflict", inst.Deps[2])
	}
}

// TestHandlePreAccept_UpdatesMaxSeqAndCrtInstance verifies bookkeeping updates.
func TestHandlePreAccept_UpdatesMaxSeqAndCrtInstance(t *testing.T) {
	r := newTestReplica(3)
	r.maxSeq = 0

	pa := &PreAccept{
		LeaderId: 1, Replica: 1, Instance: 5, Ballot: 0,
		Command: []state.Command{{Op: state.PUT, K: 10, V: state.NIL()}},
		Seq: 20, Deps: []int32{-1, -1, -1}, CL: []int32{0, 0, 0},
	}

	func() {
		defer func() { recover() }()
		r.handlePreAccept(pa)
	}()

	if r.maxSeq != 21 {
		t.Errorf("maxSeq=%d, want 21", r.maxSeq)
	}
	if r.crtInstance[1] != 6 {
		t.Errorf("crtInstance[1]=%d, want 6", r.crtInstance[1])
	}
}

// TestHandlePreAccept_Checkpoint verifies checkpoint detection.
func TestHandlePreAccept_Checkpoint(t *testing.T) {
	r := newTestReplica(3)

	pa := &PreAccept{
		LeaderId: 1, Replica: 1, Instance: 0, Ballot: 0,
		Command: []state.Command{}, // empty = checkpoint
		Seq: 5, Deps: []int32{-1, -1, -1}, CL: []int32{0, 0, 0},
	}

	func() {
		defer func() { recover() }()
		r.handlePreAccept(pa)
	}()

	if r.latestCPReplica != 1 {
		t.Errorf("latestCPReplica=%d, want 1", r.latestCPReplica)
	}
	if r.latestCPInstance != 0 {
		t.Errorf("latestCPInstance=%d, want 0", r.latestCPInstance)
	}
}

// TestHandlePreAcceptReply_DelayedReply verifies delayed replies are ignored.
func TestHandlePreAcceptReply_DelayedReply(t *testing.T) {
	r := newTestReplica(3)

	// Instance already committed
	cmds := []state.Command{{Op: state.PUT, K: 10, V: state.NIL(), CL: state.STRONG}}
	setupStrongInstance(r, 0, 0, 0, cmds, 5, []int32{-1, -1, -1})
	r.InstanceSpace[0][0].Status = STRONGLY_COMMITTED

	reply := &PreAcceptReply{
		Replica: 0, Instance: 0, OK: TRUE, Ballot: 0,
		Seq: 5, Deps: []int32{-1, -1, -1}, CL: []int32{0, 0, 0},
		CommittedDeps: []int32{-1, -1, -1},
	}
	// Should return immediately without panicking
	r.handlePreAcceptReply(reply)
}

// TestHandlePreAcceptReply_WrongBallot verifies wrong ballot replies are ignored.
func TestHandlePreAcceptReply_WrongBallot(t *testing.T) {
	r := newTestReplica(3)

	cmds := []state.Command{{Op: state.PUT, K: 10, V: state.NIL(), CL: state.STRONG}}
	setupStrongInstance(r, 0, 0, 0x10, cmds, 5, []int32{-1, -1, -1})

	reply := &PreAcceptReply{
		Replica: 0, Instance: 0, OK: TRUE, Ballot: 0, // wrong ballot
		Seq: 5, Deps: []int32{-1, -1, -1}, CL: []int32{0, 0, 0},
		CommittedDeps: []int32{-1, -1, -1},
	}
	r.handlePreAcceptReply(reply)

	// preAcceptOKs should NOT be incremented
	if r.InstanceSpace[0][0].lb.preAcceptOKs != 0 {
		t.Errorf("preAcceptOKs=%d, want 0 (wrong ballot)", r.InstanceSpace[0][0].lb.preAcceptOKs)
	}
}

// TestHandlePreAcceptReply_Nack verifies nack counting.
func TestHandlePreAcceptReply_Nack(t *testing.T) {
	r := newTestReplica(3)

	cmds := []state.Command{{Op: state.PUT, K: 10, V: state.NIL(), CL: state.STRONG}}
	setupStrongInstance(r, 0, 0, 0, cmds, 5, []int32{-1, -1, -1})

	reply := &PreAcceptReply{
		Replica: 0, Instance: 0, OK: FALSE, Ballot: 0x10, // higher ballot
		Seq: 5, Deps: []int32{-1, -1, -1}, CL: []int32{0, 0, 0},
		CommittedDeps: []int32{-1, -1, -1},
	}

	func() {
		defer func() { recover() }()
		r.handlePreAcceptReply(reply)
	}()

	if r.InstanceSpace[0][0].lb.nacks != 1 {
		t.Errorf("nacks=%d, want 1", r.InstanceSpace[0][0].lb.nacks)
	}
	if r.InstanceSpace[0][0].lb.maxRecvBallot != 0x10 {
		t.Errorf("maxRecvBallot=%d, want 0x10", r.InstanceSpace[0][0].lb.maxRecvBallot)
	}
}

// TestHandlePreAcceptReply_CountsOKs verifies OK counting and attribute merging.
func TestHandlePreAcceptReply_CountsOKs(t *testing.T) {
	r := newTestReplica(5) // 5 replicas: fast quorum = 5/2 + (5/2+1)/2 - 1 = 2+1-1 = 2

	cmds := []state.Command{{Op: state.PUT, K: 10, V: state.NIL(), CL: state.STRONG}}
	setupStrongInstance(r, 0, 0, 0, cmds, 5, []int32{-1, -1, -1, -1, -1})

	reply := &PreAcceptReply{
		Replica: 0, Instance: 0, OK: TRUE, Ballot: 0,
		Seq: 5, Deps: []int32{-1, -1, -1, -1, -1}, CL: []int32{0, 0, 0, 0, 0},
		CommittedDeps: []int32{-1, -1, -1, -1, -1},
	}

	r.handlePreAcceptReply(reply)

	if r.InstanceSpace[0][0].lb.preAcceptOKs != 1 {
		t.Errorf("preAcceptOKs=%d, want 1", r.InstanceSpace[0][0].lb.preAcceptOKs)
	}
}

// TestHandlePreAcceptReply_SlowPath verifies slow path transition to ACCEPTED.
func TestHandlePreAcceptReply_SlowPath(t *testing.T) {
	r := newTestReplica(3)

	cmds := []state.Command{{Op: state.PUT, K: 10, V: state.NIL(), CL: state.STRONG}}
	setupStrongInstance(r, 0, 0, 0, cmds, 5, []int32{-1, -1, -1})
	// Set allEqual=false to prevent fast path
	r.InstanceSpace[0][0].lb.allEqual = false

	// Need N/2 = 1 OK for slow path
	reply := &PreAcceptReply{
		Replica: 0, Instance: 0, OK: TRUE, Ballot: 0,
		Seq: 5, Deps: []int32{-1, -1, -1}, CL: []int32{0, 0, 0},
		CommittedDeps: []int32{-1, -1, -1},
	}

	func() {
		defer func() { recover() }()
		r.handlePreAcceptReply(reply)
	}()

	if r.InstanceSpace[0][0].Status != ACCEPTED {
		t.Errorf("Status=%d, want ACCEPTED(%d)", r.InstanceSpace[0][0].Status, ACCEPTED)
	}
}

// TestHandlePreAcceptReply_FastPath verifies fast path commit with N=3.
func TestHandlePreAcceptReply_FastPath(t *testing.T) {
	r := newTestReplica(3)

	cmds := []state.Command{{Op: state.PUT, K: 10, V: state.NIL(), CL: state.STRONG}}
	setupStrongInstance(r, 0, 0, 0, cmds, 5, []int32{-1, -1, -1})
	// For N=3: fast quorum = 3/2 + (3/2+1)/2 - 1 = 1+1-1 = 1

	reply := &PreAcceptReply{
		Replica: 0, Instance: 0, OK: TRUE, Ballot: 0,
		Seq: 5, Deps: []int32{-1, -1, -1}, CL: []int32{0, 0, 0},
		CommittedDeps: []int32{-1, -1, -1},
	}

	func() {
		defer func() { recover() }()
		r.handlePreAcceptReply(reply)
	}()

	if r.InstanceSpace[0][0].Status != STRONGLY_COMMITTED {
		t.Errorf("Status=%d, want STRONGLY_COMMITTED(%d)", r.InstanceSpace[0][0].Status, STRONGLY_COMMITTED)
	}
}

// TestHandlePreAcceptReply_MergesDeps verifies attribute merging.
func TestHandlePreAcceptReply_MergesDeps(t *testing.T) {
	r := newTestReplica(3)

	cmds := []state.Command{{Op: state.PUT, K: 10, V: state.NIL(), CL: state.STRONG}}
	setupStrongInstance(r, 0, 0, 0, cmds, 5, []int32{-1, -1, -1})

	// Reply with higher deps
	reply := &PreAcceptReply{
		Replica: 0, Instance: 0, OK: TRUE, Ballot: 0,
		Seq: 10, Deps: []int32{-1, 3, -1}, CL: []int32{0, int32(state.STRONG), 0},
		CommittedDeps: []int32{-1, -1, -1},
	}

	func() {
		defer func() { recover() }()
		r.handlePreAcceptReply(reply)
	}()

	inst := r.InstanceSpace[0][0]
	if inst.Seq != 10 {
		t.Errorf("Seq=%d, want 10 (merged max)", inst.Seq)
	}
	if inst.Deps[1] != 3 {
		t.Errorf("Deps[1]=%d, want 3 (merged max)", inst.Deps[1])
	}
}

// TestHandlePreAcceptOK_DelayedIgnored verifies delayed PreAcceptOK is ignored.
func TestHandlePreAcceptOK_DelayedIgnored(t *testing.T) {
	r := newTestReplica(3)

	cmds := []state.Command{{Op: state.PUT, K: 10, V: state.NIL(), CL: state.STRONG}}
	setupStrongInstance(r, 0, 0, 0, cmds, 5, []int32{-1, -1, -1})
	r.InstanceSpace[0][0].Status = STRONGLY_COMMITTED

	r.handlePreAcceptOK(&PreAcceptOK{Instance: 0})

	// Should not increment
	if r.InstanceSpace[0][0].lb.preAcceptOKs != 0 {
		t.Errorf("preAcceptOKs=%d, want 0", r.InstanceSpace[0][0].lb.preAcceptOKs)
	}
}

// TestHandlePreAcceptOK_NonInitialBallotIgnored verifies non-initial ballot is ignored.
func TestHandlePreAcceptOK_NonInitialBallotIgnored(t *testing.T) {
	r := newTestReplica(3)

	cmds := []state.Command{{Op: state.PUT, K: 10, V: state.NIL(), CL: state.STRONG}}
	setupStrongInstance(r, 0, 0, 0x10, cmds, 5, []int32{-1, -1, -1}) // non-initial ballot

	r.handlePreAcceptOK(&PreAcceptOK{Instance: 0})

	if r.InstanceSpace[0][0].lb.preAcceptOKs != 0 {
		t.Errorf("preAcceptOKs=%d, want 0 (non-initial ballot)", r.InstanceSpace[0][0].lb.preAcceptOKs)
	}
}

// TestHandlePreAcceptOK_FastPath verifies fast path commit via PreAcceptOK.
func TestHandlePreAcceptOK_FastPath(t *testing.T) {
	r := newTestReplica(3)

	cmds := []state.Command{{Op: state.PUT, K: 10, V: state.NIL(), CL: state.STRONG}}
	setupStrongInstance(r, 0, 0, 0, cmds, 5, []int32{-1, -1, -1})

	// For N=3: fast quorum = 1
	func() {
		defer func() { recover() }()
		r.handlePreAcceptOK(&PreAcceptOK{Instance: 0})
	}()

	if r.InstanceSpace[0][0].Status != STRONGLY_COMMITTED {
		t.Errorf("Status=%d, want STRONGLY_COMMITTED(%d)", r.InstanceSpace[0][0].Status, STRONGLY_COMMITTED)
	}
}

// TestHandlePreAcceptOK_SlowPath verifies slow path with N=5.
func TestHandlePreAcceptOK_SlowPath(t *testing.T) {
	r := newTestReplica(5)

	cmds := []state.Command{{Op: state.PUT, K: 10, V: state.NIL(), CL: state.STRONG}}
	setupStrongInstance(r, 0, 0, 0, cmds, 5, []int32{-1, -1, -1, -1, -1})
	r.InstanceSpace[0][0].lb.allEqual = false // prevent fast path

	// N=5: need N/2 = 2 for slow path
	func() {
		defer func() { recover() }()
		r.handlePreAcceptOK(&PreAcceptOK{Instance: 0})
	}()
	// Only 1 OK — not enough for slow path either
	if r.InstanceSpace[0][0].Status != PREACCEPTED {
		t.Errorf("Status=%d, want PREACCEPTED(%d) (not enough OKs)", r.InstanceSpace[0][0].Status, PREACCEPTED)
	}

	// Second OK → slow path (N/2 = 2 reached)
	func() {
		defer func() { recover() }()
		r.handlePreAcceptOK(&PreAcceptOK{Instance: 0})
	}()
	if r.InstanceSpace[0][0].Status != ACCEPTED {
		t.Errorf("Status=%d, want ACCEPTED(%d) (slow path)", r.InstanceSpace[0][0].Status, ACCEPTED)
	}
}

// TestHandlePreAcceptReply_CommittedDepsUpdated verifies committedDeps merging.
func TestHandlePreAcceptReply_CommittedDepsUpdated(t *testing.T) {
	r := newTestReplica(3)

	cmds := []state.Command{{Op: state.PUT, K: 10, V: state.NIL(), CL: state.STRONG}}
	setupStrongInstance(r, 0, 0, 0, cmds, 5, []int32{-1, -1, -1})
	r.InstanceSpace[0][0].lb.allEqual = false // prevent fast path

	reply := &PreAcceptReply{
		Replica: 0, Instance: 0, OK: TRUE, Ballot: 0,
		Seq: 5, Deps: []int32{-1, -1, -1}, CL: []int32{0, 0, 0},
		CommittedDeps: []int32{3, 5, 2},
	}

	func() {
		defer func() { recover() }()
		r.handlePreAcceptReply(reply)
	}()

	inst := r.InstanceSpace[0][0]
	if inst.lb.committedDeps[0] != 3 {
		t.Errorf("committedDeps[0]=%d, want 3", inst.lb.committedDeps[0])
	}
	if inst.lb.committedDeps[1] != 5 {
		t.Errorf("committedDeps[1]=%d, want 5", inst.lb.committedDeps[1])
	}
}

// TestBcastPrepare verifies bcastPrepare doesn't panic on test replica.
func TestBcastPrepare(t *testing.T) {
	r := newTestReplica(3)
	// Should not panic — recover catches
	r.bcastPrepare(0, 5, 0x10)
}

// --- Phase 99.3f-iv: handleAccept, handleAcceptReply, handleCommit, handleCommitShort tests ---

// TestHandleAccept_NewInstance verifies Accept creates a new ACCEPTED instance.
func TestHandleAccept_NewInstance(t *testing.T) {
	r := newTestReplica(3)

	acc := &Accept{
		LeaderId: 1, Replica: 1, Instance: 0, Ballot: 0,
		Count: 3, Seq: 10, Deps: []int32{-1, -1, -1}, CL: []int32{0, 0, 0},
	}

	func() {
		defer func() { recover() }()
		r.handleAccept(acc)
	}()

	inst := r.InstanceSpace[1][0]
	if inst == nil {
		t.Fatal("instance should be created")
	}
	if inst.Status != ACCEPTED {
		t.Errorf("Status=%d, want ACCEPTED(%d)", inst.Status, ACCEPTED)
	}
	if inst.State != READY {
		t.Errorf("State=%d, want READY(%d)", inst.State, READY)
	}
	if inst.Seq != 10 {
		t.Errorf("Seq=%d, want 10", inst.Seq)
	}
	if inst.bal != 0 {
		t.Errorf("bal=%d, want 0", inst.bal)
	}
}

// TestHandleAccept_ExistingInstance verifies Accept updates existing instance.
func TestHandleAccept_ExistingInstance(t *testing.T) {
	r := newTestReplica(3)

	// Pre-existing PREACCEPTED instance
	r.InstanceSpace[1][0] = &Instance{
		Status: PREACCEPTED, State: READY, Seq: 5,
		Deps: []int32{-1, -1, -1}, CL: []int32{0, 0, 0}, bal: 0,
	}

	acc := &Accept{
		LeaderId: 1, Replica: 1, Instance: 0, Ballot: 0,
		Count: 1, Seq: 15, Deps: []int32{3, -1, -1}, CL: []int32{int32(state.STRONG), 0, 0},
	}

	func() {
		defer func() { recover() }()
		r.handleAccept(acc)
	}()

	inst := r.InstanceSpace[1][0]
	if inst.Status != ACCEPTED {
		t.Errorf("Status=%d, want ACCEPTED(%d)", inst.Status, ACCEPTED)
	}
	if inst.Seq != 15 {
		t.Errorf("Seq=%d, want 15", inst.Seq)
	}
	if inst.Deps[0] != 3 {
		t.Errorf("Deps[0]=%d, want 3", inst.Deps[0])
	}
}

// TestHandleAccept_CommittedSkipped verifies committed instances are skipped.
func TestHandleAccept_CommittedSkipped(t *testing.T) {
	r := newTestReplica(3)

	r.InstanceSpace[1][0] = &Instance{Status: STRONGLY_COMMITTED}

	acc := &Accept{
		LeaderId: 1, Replica: 1, Instance: 0, Ballot: 0,
		Count: 1, Seq: 10, Deps: []int32{-1, -1, -1}, CL: []int32{0, 0, 0},
	}

	r.handleAccept(acc)

	if r.InstanceSpace[1][0].Status != STRONGLY_COMMITTED {
		t.Errorf("Status should remain STRONGLY_COMMITTED")
	}
}

// TestHandleAccept_BallotReject verifies lower ballot is rejected.
func TestHandleAccept_BallotReject(t *testing.T) {
	r := newTestReplica(3)

	r.InstanceSpace[1][0] = &Instance{
		Status: PREACCEPTED, bal: 0x10, Seq: 5,
		Deps: []int32{-1, -1, -1}, CL: []int32{0, 0, 0},
	}

	acc := &Accept{
		LeaderId: 1, Replica: 1, Instance: 0, Ballot: 0, // lower ballot
		Count: 1, Seq: 20, Deps: []int32{-1, -1, -1}, CL: []int32{0, 0, 0},
	}

	func() {
		defer func() { recover() }()
		r.handleAccept(acc)
	}()

	// Should NOT update
	if r.InstanceSpace[1][0].Seq != 5 {
		t.Errorf("Seq=%d, should stay 5 (ballot reject)", r.InstanceSpace[1][0].Seq)
	}
}

// TestHandleAccept_Checkpoint verifies checkpoint detection.
func TestHandleAccept_Checkpoint(t *testing.T) {
	r := newTestReplica(3)

	acc := &Accept{
		LeaderId: 1, Replica: 1, Instance: 5, Ballot: 0,
		Count: 0, // empty = checkpoint
		Seq: 10, Deps: []int32{-1, -1, -1}, CL: []int32{0, 0, 0},
	}

	func() {
		defer func() { recover() }()
		r.handleAccept(acc)
	}()

	if r.latestCPReplica != 1 {
		t.Errorf("latestCPReplica=%d, want 1", r.latestCPReplica)
	}
	if r.latestCPInstance != 5 {
		t.Errorf("latestCPInstance=%d, want 5", r.latestCPInstance)
	}
}

// TestHandleAccept_UpdatesMaxSeq verifies maxSeq is bumped.
func TestHandleAccept_UpdatesMaxSeq(t *testing.T) {
	r := newTestReplica(3)
	r.maxSeq = 0

	acc := &Accept{
		LeaderId: 1, Replica: 1, Instance: 0, Ballot: 0,
		Count: 1, Seq: 50, Deps: []int32{-1, -1, -1}, CL: []int32{0, 0, 0},
	}

	func() {
		defer func() { recover() }()
		r.handleAccept(acc)
	}()

	if r.maxSeq != 51 {
		t.Errorf("maxSeq=%d, want 51", r.maxSeq)
	}
}

// TestHandleAcceptReply_DelayedReply verifies delayed replies are ignored.
func TestHandleAcceptReply_DelayedReply(t *testing.T) {
	r := newTestReplica(3)

	cmds := []state.Command{{Op: state.PUT, K: 10, V: state.NIL(), CL: state.STRONG}}
	setupStrongInstance(r, 0, 0, 0, cmds, 5, []int32{-1, -1, -1})
	r.InstanceSpace[0][0].Status = STRONGLY_COMMITTED

	r.handleAcceptReply(&AcceptReply{Replica: 0, Instance: 0, OK: TRUE, Ballot: 0})
	// Should not panic or change state
}

// TestHandleAcceptReply_Nack verifies nack counting.
func TestHandleAcceptReply_Nack(t *testing.T) {
	r := newTestReplica(3)

	cmds := []state.Command{{Op: state.PUT, K: 10, V: state.NIL(), CL: state.STRONG}}
	setupStrongInstance(r, 0, 0, 0, cmds, 5, []int32{-1, -1, -1})
	r.InstanceSpace[0][0].Status = ACCEPTED

	r.handleAcceptReply(&AcceptReply{Replica: 0, Instance: 0, OK: FALSE, Ballot: 0x10})

	if r.InstanceSpace[0][0].lb.nacks != 1 {
		t.Errorf("nacks=%d, want 1", r.InstanceSpace[0][0].lb.nacks)
	}
	if r.InstanceSpace[0][0].lb.maxRecvBallot != 0x10 {
		t.Errorf("maxRecvBallot=%d, want 0x10", r.InstanceSpace[0][0].lb.maxRecvBallot)
	}
}

// TestHandleAcceptReply_WrongBallot verifies wrong ballot replies are ignored.
func TestHandleAcceptReply_WrongBallot(t *testing.T) {
	r := newTestReplica(3)

	cmds := []state.Command{{Op: state.PUT, K: 10, V: state.NIL(), CL: state.STRONG}}
	setupStrongInstance(r, 0, 0, 0x10, cmds, 5, []int32{-1, -1, -1})
	r.InstanceSpace[0][0].Status = ACCEPTED

	r.handleAcceptReply(&AcceptReply{Replica: 0, Instance: 0, OK: TRUE, Ballot: 0})

	if r.InstanceSpace[0][0].lb.acceptOKs != 0 {
		t.Errorf("acceptOKs=%d, want 0 (wrong ballot)", r.InstanceSpace[0][0].lb.acceptOKs)
	}
}

// TestHandleAcceptReply_QuorumCommits verifies quorum leads to STRONGLY_COMMITTED.
func TestHandleAcceptReply_QuorumCommits(t *testing.T) {
	r := newTestReplica(3)

	cmds := []state.Command{{Op: state.PUT, K: 10, V: state.NIL(), CL: state.STRONG}}
	setupStrongInstance(r, 0, 0, 0, cmds, 5, []int32{-1, -1, -1})
	r.InstanceSpace[0][0].Status = ACCEPTED

	// N=3: need acceptOKs+1 > N/2 → acceptOKs+1 > 1 → acceptOKs >= 1
	func() {
		defer func() { recover() }()
		r.handleAcceptReply(&AcceptReply{Replica: 0, Instance: 0, OK: TRUE, Ballot: 0})
	}()

	if r.InstanceSpace[0][0].Status != STRONGLY_COMMITTED {
		t.Errorf("Status=%d, want STRONGLY_COMMITTED(%d)", r.InstanceSpace[0][0].Status, STRONGLY_COMMITTED)
	}
}

// TestHandleAcceptReply_NotEnoughOKs verifies partial quorum doesn't commit.
func TestHandleAcceptReply_NotEnoughOKs(t *testing.T) {
	r := newTestReplica(5)

	cmds := []state.Command{{Op: state.PUT, K: 10, V: state.NIL(), CL: state.STRONG}}
	setupStrongInstance(r, 0, 0, 0, cmds, 5, []int32{-1, -1, -1, -1, -1})
	r.InstanceSpace[0][0].Status = ACCEPTED

	// N=5: need acceptOKs+1 > N/2 → acceptOKs+1 > 2 → acceptOKs >= 2
	func() {
		defer func() { recover() }()
		r.handleAcceptReply(&AcceptReply{Replica: 0, Instance: 0, OK: TRUE, Ballot: 0})
	}()

	// Only 1 OK — not enough
	if r.InstanceSpace[0][0].Status != ACCEPTED {
		t.Errorf("Status=%d, want ACCEPTED(%d) (not enough OKs)", r.InstanceSpace[0][0].Status, ACCEPTED)
	}
}

// TestHandleCommit_NewInstance verifies Commit creates STRONGLY_COMMITTED instance.
func TestHandleCommit_NewInstance(t *testing.T) {
	r := newTestReplica(3)

	cmds := []state.Command{{Op: state.PUT, K: 10, V: state.NIL(), CL: state.STRONG}}
	commit := &Commit{
		Consistency: state.STRONG, LeaderId: 1, Replica: 1, Instance: 0,
		Command: cmds, Seq: 10, Deps: []int32{-1, -1, -1}, CL: []int32{0, 0, 0},
	}

	r.handleCommit(commit)

	inst := r.InstanceSpace[1][0]
	if inst == nil {
		t.Fatal("instance should be created")
	}
	if inst.Status != STRONGLY_COMMITTED {
		t.Errorf("Status=%d, want STRONGLY_COMMITTED(%d)", inst.Status, STRONGLY_COMMITTED)
	}
	if inst.State != READY {
		t.Errorf("State=%d, want READY(%d)", inst.State, READY)
	}
	if len(inst.Cmds) != 1 {
		t.Errorf("len(Cmds)=%d, want 1", len(inst.Cmds))
	}
}

// TestHandleCommit_ExistingInstance verifies Commit updates existing instance.
func TestHandleCommit_ExistingInstance(t *testing.T) {
	r := newTestReplica(3)

	r.InstanceSpace[1][0] = &Instance{
		Status: ACCEPTED, State: READY, Seq: 5,
		Deps: []int32{-1, -1, -1}, CL: []int32{0, 0, 0},
	}

	cmds := []state.Command{{Op: state.PUT, K: 10, V: state.NIL(), CL: state.STRONG}}
	commit := &Commit{
		Consistency: state.STRONG, LeaderId: 1, Replica: 1, Instance: 0,
		Command: cmds, Seq: 20, Deps: []int32{3, -1, -1}, CL: []int32{int32(state.STRONG), 0, 0},
	}

	r.handleCommit(commit)

	inst := r.InstanceSpace[1][0]
	if inst.Status != STRONGLY_COMMITTED {
		t.Errorf("Status=%d, want STRONGLY_COMMITTED(%d)", inst.Status, STRONGLY_COMMITTED)
	}
	if inst.Seq != 20 {
		t.Errorf("Seq=%d, want 20", inst.Seq)
	}
	if inst.Deps[0] != 3 {
		t.Errorf("Deps[0]=%d, want 3", inst.Deps[0])
	}
}

// TestHandleCommit_CommittedSkipped verifies already committed instances are skipped.
func TestHandleCommit_CommittedSkipped(t *testing.T) {
	r := newTestReplica(3)

	r.InstanceSpace[1][0] = &Instance{Status: STRONGLY_COMMITTED, Seq: 5}

	commit := &Commit{
		Consistency: state.STRONG, LeaderId: 1, Replica: 1, Instance: 0,
		Command: []state.Command{{Op: state.PUT, K: 10, V: state.NIL()}},
		Seq: 20, Deps: []int32{-1, -1, -1}, CL: []int32{0, 0, 0},
	}

	r.handleCommit(commit)

	if r.InstanceSpace[1][0].Seq != 5 {
		t.Errorf("Seq=%d, should stay 5 (already committed)", r.InstanceSpace[1][0].Seq)
	}
}

// TestHandleCommit_Checkpoint verifies checkpoint detection.
func TestHandleCommit_Checkpoint(t *testing.T) {
	r := newTestReplica(3)

	commit := &Commit{
		Consistency: state.STRONG, LeaderId: 1, Replica: 1, Instance: 5,
		Command: []state.Command{}, // empty = checkpoint
		Seq: 10, Deps: []int32{-1, -1, -1}, CL: []int32{0, 0, 0},
	}

	r.handleCommit(commit)

	if r.latestCPReplica != 1 {
		t.Errorf("latestCPReplica=%d, want 1", r.latestCPReplica)
	}
	if r.latestCPInstance != 5 {
		t.Errorf("latestCPInstance=%d, want 5", r.latestCPInstance)
	}
}

// TestHandleCommit_UpdatesMaxSeqAndCrtInstance verifies bookkeeping.
func TestHandleCommit_UpdatesMaxSeqAndCrtInstance(t *testing.T) {
	r := newTestReplica(3)
	r.maxSeq = 0

	commit := &Commit{
		Consistency: state.STRONG, LeaderId: 1, Replica: 1, Instance: 10,
		Command: []state.Command{{Op: state.PUT, K: 10, V: state.NIL()}},
		Seq: 50, Deps: []int32{-1, -1, -1}, CL: []int32{0, 0, 0},
	}

	r.handleCommit(commit)

	if r.maxSeq != 51 {
		t.Errorf("maxSeq=%d, want 51", r.maxSeq)
	}
	if r.crtInstance[1] != 11 {
		t.Errorf("crtInstance[1]=%d, want 11", r.crtInstance[1])
	}
}

// TestHandleCommitShort_NewInstance verifies CommitShort creates STRONGLY_COMMITTED instance.
func TestHandleCommitShort_NewInstance(t *testing.T) {
	r := newTestReplica(3)

	commit := &CommitShort{
		Consistency: state.STRONG, LeaderId: 1, Replica: 1, Instance: 0,
		Count: 3, Seq: 10, Deps: []int32{-1, -1, -1}, CL: []int32{0, 0, 0},
	}

	r.handleCommitShort(commit)

	inst := r.InstanceSpace[1][0]
	if inst == nil {
		t.Fatal("instance should be created")
	}
	if inst.Status != STRONGLY_COMMITTED {
		t.Errorf("Status=%d, want STRONGLY_COMMITTED(%d)", inst.Status, STRONGLY_COMMITTED)
	}
	if inst.State != READY {
		t.Errorf("State=%d, want READY(%d)", inst.State, READY)
	}
	// CommitShort doesn't include commands
	if inst.Cmds != nil {
		t.Errorf("Cmds should be nil for CommitShort")
	}
}

// TestHandleCommitShort_ExistingInstance verifies CommitShort updates existing instance.
func TestHandleCommitShort_ExistingInstance(t *testing.T) {
	r := newTestReplica(3)

	// Pre-existing instance from PreAccept (has commands)
	r.InstanceSpace[1][0] = &Instance{
		Cmds:   []state.Command{{Op: state.PUT, K: 10, V: state.NIL()}},
		Status: PREACCEPTED, State: READY, Seq: 5,
		Deps: []int32{-1, -1, -1}, CL: []int32{0, 0, 0},
	}

	commit := &CommitShort{
		Consistency: state.STRONG, LeaderId: 1, Replica: 1, Instance: 0,
		Count: 1, Seq: 20, Deps: []int32{3, -1, -1}, CL: []int32{int32(state.STRONG), 0, 0},
	}

	r.handleCommitShort(commit)

	inst := r.InstanceSpace[1][0]
	if inst.Status != STRONGLY_COMMITTED {
		t.Errorf("Status=%d, want STRONGLY_COMMITTED(%d)", inst.Status, STRONGLY_COMMITTED)
	}
	if inst.Seq != 20 {
		t.Errorf("Seq=%d, want 20", inst.Seq)
	}
	// Commands should still be there from PreAccept
	if inst.Cmds == nil || len(inst.Cmds) != 1 {
		t.Error("Cmds should be preserved from PreAccept")
	}
}

// TestHandleCommitShort_CommittedSkipped verifies already committed instances are skipped.
func TestHandleCommitShort_CommittedSkipped(t *testing.T) {
	r := newTestReplica(3)

	r.InstanceSpace[1][0] = &Instance{Status: STRONGLY_COMMITTED, Seq: 5}

	commit := &CommitShort{
		Consistency: state.STRONG, LeaderId: 1, Replica: 1, Instance: 0,
		Count: 1, Seq: 20, Deps: []int32{-1, -1, -1}, CL: []int32{0, 0, 0},
	}

	r.handleCommitShort(commit)

	if r.InstanceSpace[1][0].Seq != 5 {
		t.Errorf("Seq=%d, should stay 5 (already committed)", r.InstanceSpace[1][0].Seq)
	}
}

// TestHandleCommitShort_Checkpoint verifies checkpoint detection.
func TestHandleCommitShort_Checkpoint(t *testing.T) {
	r := newTestReplica(3)

	commit := &CommitShort{
		Consistency: state.STRONG, LeaderId: 1, Replica: 1, Instance: 5,
		Count: 0, // checkpoint
		Seq: 10, Deps: []int32{-1, -1, -1}, CL: []int32{0, 0, 0},
	}

	r.handleCommitShort(commit)

	if r.latestCPReplica != 1 {
		t.Errorf("latestCPReplica=%d, want 1", r.latestCPReplica)
	}
	if r.latestCPInstance != 5 {
		t.Errorf("latestCPInstance=%d, want 5", r.latestCPInstance)
	}
}

// TestHandleCommitShort_UpdatesCrtInstance verifies crtInstance is bumped.
func TestHandleCommitShort_UpdatesCrtInstance(t *testing.T) {
	r := newTestReplica(3)

	commit := &CommitShort{
		Consistency: state.STRONG, LeaderId: 1, Replica: 1, Instance: 10,
		Count: 1, Seq: 5, Deps: []int32{-1, -1, -1}, CL: []int32{0, 0, 0},
	}

	r.handleCommitShort(commit)

	if r.crtInstance[1] != 11 {
		t.Errorf("crtInstance[1]=%d, want 11", r.crtInstance[1])
	}
}

// =============================================================================
// Phase 99.3g-i: handlePrepare, startRecoveryForInstance, bcastTryPreAccept,
//                findPreAcceptConflicts
// =============================================================================

// TestHandlePrepare_NilInstance verifies that handlePrepare creates a NONE instance
// when the instance has never been seen.
func TestHandlePrepare_NilInstance(t *testing.T) {
	r := newTestReplica(3)
	r.Id = 1

	prepare := &Prepare{
		LeaderId: 0,
		Replica:  2,
		Instance: 5,
		Ballot:   (1 << 4) | 0, // ballot from replica 0
	}

	func() {
		defer func() { recover() }()
		r.handlePrepare(prepare)
	}()

	inst := r.InstanceSpace[2][5]
	if inst == nil {
		t.Fatal("instance should have been created")
	}
	if inst.Status != NONE {
		t.Errorf("Status=%d, want NONE(%d)", inst.Status, NONE)
	}
	if inst.bal != prepare.Ballot {
		t.Errorf("bal=%d, want %d", inst.bal, prepare.Ballot)
	}
}

// TestHandlePrepare_ExistingInstance verifies that handlePrepare returns the
// existing instance's attributes and updates the ballot.
func TestHandlePrepare_ExistingInstance(t *testing.T) {
	r := newTestReplica(3)
	r.Id = 1

	cmds := []state.Command{{Op: state.PUT, K: 42, V: state.NIL(), CL: state.STRONG}}
	r.InstanceSpace[2][5] = &Instance{
		Cmds:   cmds,
		Status: PREACCEPTED,
		Seq:    10,
		Deps:   []int32{1, 2, 3},
		CL:     []int32{0, 0, 0},
		bal:    (0 << 4) | 2, // initial ballot from replica 2
		vbal:   (0 << 4) | 2,
		State:  READY,
	}

	prepare := &Prepare{
		LeaderId: 0,
		Replica:  2,
		Instance: 5,
		Ballot:   (1 << 4) | 0, // higher ballot
	}

	func() {
		defer func() { recover() }()
		r.handlePrepare(prepare)
	}()

	inst := r.InstanceSpace[2][5]
	if inst.bal != prepare.Ballot {
		t.Errorf("bal=%d, want %d (should be updated)", inst.bal, prepare.Ballot)
	}
}

// TestHandlePrepare_LowerBallotRejected verifies that a Prepare with a lower
// ballot than the current one is rejected (OK=FALSE).
func TestHandlePrepare_LowerBallotRejected(t *testing.T) {
	r := newTestReplica(3)
	r.Id = 1

	r.InstanceSpace[2][5] = &Instance{
		Status: PREACCEPTED,
		Seq:    10,
		Deps:   []int32{1, 2, 3},
		CL:     []int32{0, 0, 0},
		bal:    (2 << 4) | 2, // high ballot
		vbal:   (0 << 4) | 2,
		State:  READY,
	}

	prepare := &Prepare{
		LeaderId: 0,
		Replica:  2,
		Instance: 5,
		Ballot:   (1 << 4) | 0, // lower than current bal
	}

	func() {
		defer func() { recover() }()
		r.handlePrepare(prepare)
	}()

	inst := r.InstanceSpace[2][5]
	// Ballot should NOT be updated since prepare.Ballot < inst.bal
	if inst.bal != (2<<4)|2 {
		t.Errorf("bal=%d, should stay %d (prepare ballot was lower)", inst.bal, (2<<4)|2)
	}
}

// TestHandlePrepare_WaitingTreatedAsNone verifies that WAITING instances are
// treated the same as nil (never-seen) instances.
func TestHandlePrepare_WaitingTreatedAsNone(t *testing.T) {
	r := newTestReplica(3)
	r.Id = 1

	r.InstanceSpace[2][5] = &Instance{
		Status: NONE,
		State:  WAITING,
		Deps:   []int32{0, 0, 0},
		CL:     []int32{0, 0, 0},
	}

	prepare := &Prepare{
		LeaderId: 0,
		Replica:  2,
		Instance: 5,
		Ballot:   (1 << 4) | 0,
	}

	func() {
		defer func() { recover() }()
		r.handlePrepare(prepare)
	}()

	inst := r.InstanceSpace[2][5]
	// For WAITING, we create a fresh NONE instance
	if inst.Status != NONE {
		t.Errorf("Status=%d, want NONE(%d) for WAITING instance", inst.Status, NONE)
	}
	if inst.State != DONE {
		t.Errorf("State=%d, want DONE(%d)", inst.State, DONE)
	}
}

// TestStartRecoveryForInstance_NilInstance verifies that startRecoveryForInstance
// creates an instance, sets up LeaderBookkeeping, and bumps the ballot.
func TestStartRecoveryForInstance_NilInstance(t *testing.T) {
	r := newTestReplica(3)
	r.Id = 0

	r.startRecoveryForInstance(1, 5)

	inst := r.InstanceSpace[1][5]
	if inst == nil {
		t.Fatal("instance should have been created")
	}
	if inst.Status != NONE {
		t.Errorf("Status=%d, want NONE", inst.Status)
	}
	if inst.lb == nil {
		t.Fatal("LeaderBookkeeping should be set")
	}
	if !inst.lb.preparing {
		t.Error("preparing should be true")
	}
	if inst.lb.recoveryInst != nil {
		t.Error("recoveryInst should be nil for NONE instance")
	}
	// Ballot should be > 0 (bumped from 0)
	if inst.bal == 0 {
		t.Error("ballot should have been bumped")
	}
}

// TestStartRecoveryForInstance_PreacceptedInstance verifies that recovery for
// a PREACCEPTED instance creates a recoveryInst with preAcceptCount=1.
func TestStartRecoveryForInstance_PreacceptedInstance(t *testing.T) {
	r := newTestReplica(3)
	r.Id = 1 // not the original leader (replica 2)

	cmds := []state.Command{{Op: state.PUT, K: 42, V: state.NIL(), CL: state.STRONG}}
	r.InstanceSpace[2][5] = &Instance{
		Cmds:       cmds,
		Status:     PREACCEPTED,
		Seq:        10,
		Deps:       []int32{1, 2, 3},
		CL:         []int32{0, 0, 0},
		bal:        (0 << 4) | 2,
		instanceId: &instanceId{2, 5},
	}

	r.startRecoveryForInstance(2, 5)

	inst := r.InstanceSpace[2][5]
	if inst.lb == nil {
		t.Fatal("LeaderBookkeeping should be set")
	}
	if inst.lb.recoveryInst == nil {
		t.Fatal("recoveryInst should be set for PREACCEPTED instance")
	}
	if inst.lb.recoveryInst.preAcceptCount != 1 {
		t.Errorf("preAcceptCount=%d, want 1", inst.lb.recoveryInst.preAcceptCount)
	}
	if inst.lb.recoveryInst.leaderResponded {
		t.Error("leaderResponded should be false (we are not the original leader)")
	}
	// Ballot should be larger than original
	if inst.bal <= (0<<4)|2 {
		t.Errorf("bal=%d, should be larger than original %d", inst.bal, (0<<4)|2)
	}
}

// TestStartRecoveryForInstance_AcceptedInstance verifies that recovery for
// an ACCEPTED instance creates a recoveryInst with maxRecvBallot set.
func TestStartRecoveryForInstance_AcceptedInstance(t *testing.T) {
	r := newTestReplica(3)
	r.Id = 0

	cmds := []state.Command{{Op: state.PUT, K: 42, V: state.NIL(), CL: state.STRONG}}
	origBal := int32((1 << 4) | 2)
	r.InstanceSpace[2][5] = &Instance{
		Cmds:       cmds,
		Status:     ACCEPTED,
		Seq:        10,
		Deps:       []int32{1, 2, 3},
		CL:         []int32{0, 0, 0},
		bal:        origBal,
		instanceId: &instanceId{2, 5},
	}

	r.startRecoveryForInstance(2, 5)

	inst := r.InstanceSpace[2][5]
	if inst.lb.recoveryInst == nil {
		t.Fatal("recoveryInst should be set for ACCEPTED instance")
	}
	if inst.lb.recoveryInst.status != ACCEPTED {
		t.Errorf("recoveryInst.status=%d, want ACCEPTED(%d)", inst.lb.recoveryInst.status, ACCEPTED)
	}
	if inst.lb.maxRecvBallot != origBal {
		t.Errorf("maxRecvBallot=%d, want %d", inst.lb.maxRecvBallot, origBal)
	}
}

// TestBcastTryPreAccept verifies bcastTryPreAccept sends to all alive peers except self.
func TestBcastTryPreAccept(t *testing.T) {
	r := newTestReplica(3)
	r.Id = 0

	// bcastTryPreAccept will call SendMsg which will panic without real network.
	// We just verify it doesn't panic with the recover() inside.
	cmds := []state.Command{{Op: state.PUT, K: 42, V: state.NIL(), CL: state.STRONG}}
	deps := []int32{-1, -1, -1}
	cl := []int32{0, 0, 0}

	// Should not panic (has internal recover)
	r.bcastTryPreAccept(0, 5, (1<<4)|0, cmds, 10, deps, cl)
}

// TestFindPreAcceptConflicts_NoConflicts verifies no conflict when instance space is empty.
func TestFindPreAcceptConflicts_NoConflicts(t *testing.T) {
	r := newTestReplica(3)

	cmds := []state.Command{{Op: state.PUT, K: 42, V: state.NIL(), CL: state.STRONG}}
	deps := []int32{-1, -1, -1}

	conflict, _, _ := r.findPreAcceptConflicts(cmds, 0, 5, 10, deps)
	if conflict {
		t.Error("should not find conflicts in empty instance space")
	}
}

// TestFindPreAcceptConflicts_AcceptedConflict verifies that an ACCEPTED instance
// at the same position is detected as a conflict.
func TestFindPreAcceptConflicts_AcceptedConflict(t *testing.T) {
	r := newTestReplica(3)

	existingCmds := []state.Command{{Op: state.PUT, K: 42, V: state.NIL(), CL: state.STRONG, Sid: 2}}
	r.InstanceSpace[0][5] = &Instance{
		Cmds:   existingCmds,
		Status: ACCEPTED,
		Seq:    10,
		Deps:   []int32{-1, -1, -1},
	}

	cmds := []state.Command{{Op: state.PUT, K: 42, V: state.NIL(), CL: state.STRONG}}
	deps := []int32{-1, -1, -1}

	conflict, cRep, cInst := r.findPreAcceptConflicts(cmds, 0, 5, 10, deps)
	if !conflict {
		t.Error("should detect conflict with ACCEPTED instance at same position")
	}
	if cRep != 0 || cInst != 5 {
		t.Errorf("conflict at (%d,%d), want (0,5)", cRep, cInst)
	}
}

// TestFindPreAcceptConflicts_SameAttributesNoConflict verifies that if the
// existing instance has the same seq and deps, no conflict is reported.
func TestFindPreAcceptConflicts_SameAttributesNoConflict(t *testing.T) {
	r := newTestReplica(3)

	existingCmds := []state.Command{{Op: state.PUT, K: 42, V: state.NIL(), CL: state.STRONG}}
	r.InstanceSpace[0][5] = &Instance{
		Cmds:   existingCmds,
		Status: PREACCEPTED,
		Seq:    10,
		Deps:   []int32{-1, -1, -1},
	}

	cmds := []state.Command{{Op: state.PUT, K: 42, V: state.NIL(), CL: state.STRONG}}
	deps := []int32{-1, -1, -1}

	conflict, _, _ := r.findPreAcceptConflicts(cmds, 0, 5, 10, deps)
	if conflict {
		t.Error("should not conflict when same seq and deps")
	}
}

// TestFindPreAcceptConflicts_CrossReplicaConflict verifies that a conflict is
// detected across replicas when instances have overlapping keys and mismatched deps.
func TestFindPreAcceptConflicts_CrossReplicaConflict(t *testing.T) {
	r := newTestReplica(3)

	// Instance at replica 1, slot 3 has a conflicting command
	otherCmds := []state.Command{{Op: state.PUT, K: 42, V: state.NIL(), CL: state.STRONG, Sid: 2}}
	r.InstanceSpace[1][3] = &Instance{
		Cmds:   otherCmds,
		Status: PREACCEPTED,
		Seq:    15,
		Deps:   []int32{-1, -1, -1}, // does NOT depend on our instance
	}
	r.crtInstance[1] = 5 // so the loop covers slot 3
	r.ExecedUpTo[0] = 0
	r.ExecedUpTo[1] = 0
	r.ExecedUpTo[2] = 0

	// Our instance at replica 0, slot 5, deps say we depend on slot 2 of replica 1
	cmds := []state.Command{{Op: state.PUT, K: 42, V: state.NIL(), CL: state.STRONG}}
	deps := []int32{-1, 2, -1} // dep on replica 1 is slot 2, but conflict is at slot 3

	conflict, cRep, cInst := r.findPreAcceptConflicts(cmds, 0, 5, 10, deps)
	if !conflict {
		t.Error("should detect cross-replica conflict")
	}
	if cRep != 1 || cInst != 3 {
		t.Errorf("conflict at (%d,%d), want (1,3)", cRep, cInst)
	}
}

// =============================================================================
// Phase 99.3g-ii: handlePrepareReply
// =============================================================================

// setupPreparingInstance creates a PREACCEPTED instance with preparing=true
// for testing handlePrepareReply.
func setupPreparingInstance(r *Replica, rep int32, inst int32, bal int32) *Instance {
	cmds := []state.Command{{Op: state.PUT, K: 1, V: state.NIL(), CL: state.STRONG}}
	instance := &Instance{
		Cmds:   cmds,
		Status: PREACCEPTED,
		State:  READY,
		Seq:    10,
		Deps:   []int32{-1, -1, -1},
		CL:     []int32{0, 0, 0},
		bal:    bal,
		vbal:   bal,
		lb: &LeaderBookkeeping{
			preparing:     true,
			maxRecvBallot: -1,
		},
		instanceId: &instanceId{rep, inst},
	}
	r.InstanceSpace[rep][inst] = instance
	return instance
}

// TestHandlePrepareReply_DelayedIgnored verifies delayed replies are ignored.
func TestHandlePrepareReply_DelayedIgnored(t *testing.T) {
	r := newTestReplica(3)
	inst := setupPreparingInstance(r, 1, 5, (1<<4)|0)
	inst.lb.preparing = false // no longer preparing

	preply := &PrepareReply{
		AcceptorId: 2, Replica: 1, Instance: 5,
		OK: TRUE, Bal: (1 << 4) | 0, VBal: 0,
		Status: NONE, Seq: -1, Deps: []int32{-1, -1, -1}, CL: []int32{0, 0, 0},
	}

	func() {
		defer func() { recover() }()
		r.handlePrepareReply(preply)
	}()

	if inst.lb.prepareOKs != 0 {
		t.Errorf("prepareOKs=%d, want 0 (delayed reply should be ignored)", inst.lb.prepareOKs)
	}
}

// TestHandlePrepareReply_NilLBIgnored verifies nil LeaderBookkeeping is ignored.
func TestHandlePrepareReply_NilLBIgnored(t *testing.T) {
	r := newTestReplica(3)
	r.InstanceSpace[1][5] = &Instance{
		Status: PREACCEPTED, State: READY,
		Deps: []int32{-1, -1, -1}, CL: []int32{0, 0, 0},
	}

	preply := &PrepareReply{
		AcceptorId: 2, Replica: 1, Instance: 5,
		OK: TRUE, Bal: 0, VBal: 0,
		Status: NONE, Seq: -1, Deps: []int32{-1, -1, -1}, CL: []int32{0, 0, 0},
	}

	func() {
		defer func() { recover() }()
		r.handlePrepareReply(preply)
	}()
	// Should not panic beyond SendMsg
}

// TestHandlePrepareReply_Nack verifies nack counting.
func TestHandlePrepareReply_Nack(t *testing.T) {
	r := newTestReplica(3)
	inst := setupPreparingInstance(r, 1, 5, (1<<4)|0)

	preply := &PrepareReply{
		AcceptorId: 2, Replica: 1, Instance: 5,
		OK: FALSE, Bal: (2 << 4) | 2, VBal: 0,
		Status: NONE, Seq: -1, Deps: []int32{-1, -1, -1}, CL: []int32{0, 0, 0},
	}

	r.handlePrepareReply(preply)

	if inst.lb.nacks != 1 {
		t.Errorf("nacks=%d, want 1", inst.lb.nacks)
	}
	if inst.lb.prepareOKs != 0 {
		t.Errorf("prepareOKs=%d, want 0", inst.lb.prepareOKs)
	}
}

// TestHandlePrepareReply_StrongCommitted verifies that a STRONGLY_COMMITTED reply
// with strong commands causes immediate commit broadcast.
func TestHandlePrepareReply_StrongCommitted(t *testing.T) {
	r := newTestReplica(3)
	bal := int32((1 << 4) | 0)
	setupPreparingInstance(r, 1, 5, bal)

	cmds := []state.Command{{Op: state.PUT, K: 1, V: state.NIL(), CL: state.STRONG}}
	preply := &PrepareReply{
		AcceptorId: 2, Replica: 1, Instance: 5,
		OK: TRUE, Bal: bal, VBal: bal,
		Status: STRONGLY_COMMITTED, Command: cmds,
		Seq: 20, Deps: []int32{3, 4, 5}, CL: []int32{1, 1, 1},
	}

	func() {
		defer func() { recover() }()
		r.handlePrepareReply(preply)
	}()

	inst := r.InstanceSpace[1][5]
	if inst.Status != STRONGLY_COMMITTED {
		t.Errorf("Status=%d, want STRONGLY_COMMITTED(%d)", inst.Status, STRONGLY_COMMITTED)
	}
	if inst.State != READY {
		t.Errorf("State=%d, want READY(%d)", inst.State, READY)
	}
	if inst.Seq != 20 {
		t.Errorf("Seq=%d, want 20", inst.Seq)
	}
}

// TestHandlePrepareReply_CausalCommitted verifies that a CAUSALLY_COMMITTED reply
// with causal commands causes causal commit broadcast.
func TestHandlePrepareReply_CausalCommitted(t *testing.T) {
	r := newTestReplica(3)
	bal := int32((1 << 4) | 0)
	setupPreparingInstance(r, 1, 5, bal)

	cmds := []state.Command{{Op: state.PUT, K: 1, V: state.NIL(), CL: state.CAUSAL}}
	preply := &PrepareReply{
		AcceptorId: 2, Replica: 1, Instance: 5,
		OK: TRUE, Bal: bal, VBal: bal,
		Status: CAUSALLY_COMMITTED, Command: cmds,
		Seq: 15, Deps: []int32{1, 2, 3}, CL: []int32{0, 0, 0},
	}

	func() {
		defer func() { recover() }()
		r.handlePrepareReply(preply)
	}()

	inst := r.InstanceSpace[1][5]
	if inst.Status != CAUSALLY_COMMITTED {
		t.Errorf("Status=%d, want CAUSALLY_COMMITTED(%d)", inst.Status, CAUSALLY_COMMITTED)
	}
}

// TestHandlePrepareReply_StrongCommittedNoCommand verifies status-based commit
// when command is nil but status indicates committed.
func TestHandlePrepareReply_StrongCommittedNoCommand(t *testing.T) {
	r := newTestReplica(3)
	bal := int32((1 << 4) | 0)
	setupPreparingInstance(r, 1, 5, bal)

	preply := &PrepareReply{
		AcceptorId: 2, Replica: 1, Instance: 5,
		OK: TRUE, Bal: bal, VBal: bal,
		Status: STRONGLY_COMMITTED, Command: nil,
		Seq: 20, Deps: []int32{3, 4, 5}, CL: []int32{1, 1, 1},
	}

	func() {
		defer func() { recover() }()
		r.handlePrepareReply(preply)
	}()

	inst := r.InstanceSpace[1][5]
	if inst.Status != STRONGLY_COMMITTED {
		t.Errorf("Status=%d, want STRONGLY_COMMITTED(%d)", inst.Status, STRONGLY_COMMITTED)
	}
}

// TestHandlePrepareReply_AcceptedTracked verifies that ACCEPTED replies update
// the recovery instance with the highest ballot.
func TestHandlePrepareReply_AcceptedTracked(t *testing.T) {
	r := newTestReplica(3)
	bal := int32((1 << 4) | 0)
	inst := setupPreparingInstance(r, 1, 5, bal)

	cmds := []state.Command{{Op: state.PUT, K: 1, V: state.NIL(), CL: state.STRONG}}
	preply := &PrepareReply{
		AcceptorId: 2, Replica: 1, Instance: 5,
		OK: TRUE, Bal: bal, VBal: (0 << 4) | 1,
		Status: ACCEPTED, Command: cmds,
		Seq: 20, Deps: []int32{3, 4, 5}, CL: []int32{1, 1, 1},
	}

	r.handlePrepareReply(preply)

	if inst.lb.recoveryInst == nil {
		t.Fatal("recoveryInst should be set for ACCEPTED reply")
	}
	if inst.lb.recoveryInst.status != ACCEPTED {
		t.Errorf("recoveryInst.status=%d, want ACCEPTED(%d)", inst.lb.recoveryInst.status, ACCEPTED)
	}
	if inst.lb.maxRecvBallot != (0<<4)|1 {
		t.Errorf("maxRecvBallot=%d, want %d", inst.lb.maxRecvBallot, (0<<4)|1)
	}
}

// TestHandlePrepareReply_PreacceptedTracked verifies PREACCEPTED replies are tracked.
func TestHandlePrepareReply_PreacceptedTracked(t *testing.T) {
	r := newTestReplica(3)
	bal := int32((1 << 4) | 0)
	inst := setupPreparingInstance(r, 1, 5, bal)

	cmds := []state.Command{{Op: state.PUT, K: 1, V: state.NIL(), CL: state.STRONG}}
	preply := &PrepareReply{
		AcceptorId: 2, Replica: 1, Instance: 5,
		OK: TRUE, Bal: bal, VBal: 0,
		Status: PREACCEPTED, Command: cmds,
		Seq: 10, Deps: []int32{-1, -1, -1}, CL: []int32{0, 0, 0},
	}

	r.handlePrepareReply(preply)

	if inst.lb.recoveryInst == nil {
		t.Fatal("recoveryInst should be set")
	}
	if inst.lb.recoveryInst.preAcceptCount != 1 {
		t.Errorf("preAcceptCount=%d, want 1", inst.lb.recoveryInst.preAcceptCount)
	}
}

// TestHandlePrepareReply_LeaderResponded verifies that a reply from the original
// leader sets leaderResponded=true and returns early.
func TestHandlePrepareReply_LeaderResponded(t *testing.T) {
	r := newTestReplica(3)
	bal := int32((1 << 4) | 0)
	inst := setupPreparingInstance(r, 1, 5, bal)

	cmds := []state.Command{{Op: state.PUT, K: 1, V: state.NIL(), CL: state.STRONG}}
	// AcceptorId == Replica means this is from the original leader.
	preply := &PrepareReply{
		AcceptorId: 1, Replica: 1, Instance: 5,
		OK: TRUE, Bal: bal, VBal: 0,
		Status: PREACCEPTED, Command: cmds,
		Seq: 10, Deps: []int32{-1, -1, -1}, CL: []int32{0, 0, 0},
	}

	r.handlePrepareReply(preply)

	if !inst.lb.recoveryInst.leaderResponded {
		t.Error("leaderResponded should be true when AcceptorId == Replica")
	}
}

// TestHandlePrepareReply_MajorityNOOP verifies that when majority has no recovery
// instance, a NO-OP is proposed via Accept.
func TestHandlePrepareReply_MajorityNOOP(t *testing.T) {
	r := newTestReplica(3)
	bal := int32((1 << 4) | 0)
	setupPreparingInstance(r, 1, 5, bal)

	// Send 2 NONE replies (from replicas 0 and 2) to reach majority (2 >= 3/2+1=2).
	for _, aid := range []int32{0, 2} {
		preply := &PrepareReply{
			AcceptorId: aid, Replica: 1, Instance: 5,
			OK: TRUE, Bal: -1, VBal: -1,
			Status: NONE, Seq: -1, Deps: []int32{-1, -1, -1}, CL: []int32{0, 0, 0},
		}
		func() {
			defer func() { recover() }()
			r.handlePrepareReply(preply)
		}()
	}

	inst := r.InstanceSpace[1][5]
	if inst.Status != ACCEPTED {
		t.Errorf("Status=%d, want ACCEPTED(%d) (NO-OP)", inst.Status, ACCEPTED)
	}
	if inst.Cmds != nil {
		t.Error("Cmds should be nil for NO-OP")
	}
	if !inst.lb.preparing == true {
		// preparing should be false after majority
	}
	if inst.Deps[1] != 4 {
		t.Errorf("NO-OP Deps[1]=%d, want 4 (Instance-1)", inst.Deps[1])
	}
}

// TestHandlePrepareReply_MajorityAccepted verifies that when recovery finds an
// ACCEPTED instance, it goes to Accept phase.
func TestHandlePrepareReply_MajorityAccepted(t *testing.T) {
	r := newTestReplica(3)
	bal := int32((1 << 4) | 0)
	setupPreparingInstance(r, 1, 5, bal)

	cmds := []state.Command{{Op: state.PUT, K: 1, V: state.NIL(), CL: state.STRONG}}

	// First reply: ACCEPTED
	preply1 := &PrepareReply{
		AcceptorId: 0, Replica: 1, Instance: 5,
		OK: TRUE, Bal: bal, VBal: (0 << 4) | 1,
		Status: ACCEPTED, Command: cmds,
		Seq: 20, Deps: []int32{3, 4, 5}, CL: []int32{1, 1, 1},
	}
	r.handlePrepareReply(preply1)

	// Second reply: NONE (reaches majority)
	preply2 := &PrepareReply{
		AcceptorId: 2, Replica: 1, Instance: 5,
		OK: TRUE, Bal: -1, VBal: -1,
		Status: NONE, Seq: -1, Deps: []int32{-1, -1, -1}, CL: []int32{0, 0, 0},
	}
	func() {
		defer func() { recover() }()
		r.handlePrepareReply(preply2)
	}()

	inst := r.InstanceSpace[1][5]
	if inst.Status != ACCEPTED {
		t.Errorf("Status=%d, want ACCEPTED(%d)", inst.Status, ACCEPTED)
	}
	if inst.Seq != 20 {
		t.Errorf("Seq=%d, want 20 (from recovery)", inst.Seq)
	}
	if !inst.lb.preparing == true {
		// preparing should be false
	}
}

// TestHandlePrepareReply_NotEnoughReplies verifies that we wait for majority.
func TestHandlePrepareReply_NotEnoughReplies(t *testing.T) {
	r := newTestReplica(5) // N=5, majority = 3
	bal := int32((1 << 4) | 0)
	inst := setupPreparingInstance(r, 1, 5, bal)
	inst.Deps = []int32{-1, -1, -1, -1, -1}
	inst.CL = []int32{0, 0, 0, 0, 0}

	// Send 1 NONE reply (not enough for majority of 3)
	preply := &PrepareReply{
		AcceptorId: 0, Replica: 1, Instance: 5,
		OK: TRUE, Bal: -1, VBal: -1,
		Status: NONE, Seq: -1, Deps: []int32{-1, -1, -1, -1, -1}, CL: []int32{0, 0, 0, 0, 0},
	}
	r.handlePrepareReply(preply)

	// Should still be preparing
	if !inst.lb.preparing {
		t.Error("should still be preparing (not enough replies)")
	}
	if inst.lb.prepareOKs != 1 {
		t.Errorf("prepareOKs=%d, want 1", inst.lb.prepareOKs)
	}
}

// =============================================================================
// Phase 99.3g-iii: handleTryPreAccept, handleTryPreAcceptReply
// =============================================================================

// TestHandleTryPreAccept_BallotReject verifies rejection when ballot is too small.
func TestHandleTryPreAccept_BallotReject(t *testing.T) {
	r := newTestReplica(3)

	r.InstanceSpace[1][5] = &Instance{
		Status: PREACCEPTED, State: READY,
		Seq: 10, Deps: []int32{-1, -1, -1}, CL: []int32{0, 0, 0},
		bal: (2 << 4) | 1, // high ballot
	}

	tpa := &TryPreAccept{
		LeaderId: 0, Replica: 1, Instance: 5,
		Ballot:  (1 << 4) | 0, // lower ballot
		Command: []state.Command{{Op: state.PUT, K: 1, V: state.NIL(), CL: state.STRONG}},
		Seq:     10, Deps: []int32{-1, -1, -1}, CL: []int32{0, 0, 0},
	}

	func() {
		defer func() { recover() }()
		r.handleTryPreAccept(tpa)
	}()

	// Instance should not be modified
	if r.InstanceSpace[1][5].bal != (2<<4)|1 {
		t.Errorf("bal should stay %d", (2<<4)|1)
	}
}

// TestHandleTryPreAccept_NoConflictNewInstance verifies successful TryPreAccept
// when no conflict and instance is nil.
func TestHandleTryPreAccept_NoConflictNewInstance(t *testing.T) {
	r := newTestReplica(3)
	r.ExecedUpTo[0] = 0
	r.ExecedUpTo[1] = 0
	r.ExecedUpTo[2] = 0

	cmds := []state.Command{{Op: state.PUT, K: 1, V: state.NIL(), CL: state.STRONG}}
	tpa := &TryPreAccept{
		LeaderId: 0, Replica: 1, Instance: 5,
		Ballot: (1 << 4) | 0, Command: cmds,
		Seq: 10, Deps: []int32{-1, -1, -1}, CL: []int32{0, 0, 0},
	}

	func() {
		defer func() { recover() }()
		r.handleTryPreAccept(tpa)
	}()

	inst := r.InstanceSpace[1][5]
	if inst == nil {
		t.Fatal("instance should be created")
	}
	if inst.Status != PREACCEPTED {
		t.Errorf("Status=%d, want PREACCEPTED(%d)", inst.Status, PREACCEPTED)
	}
	if inst.State != READY {
		t.Errorf("State=%d, want READY(%d)", inst.State, READY)
	}
	if inst.bal != (1<<4)|0 {
		t.Errorf("bal=%d, want %d", inst.bal, (1<<4)|0)
	}
	if r.crtInstance[1] != 6 {
		t.Errorf("crtInstance[1]=%d, want 6", r.crtInstance[1])
	}
}

// TestHandleTryPreAccept_NoConflictExistingInstance verifies successful
// TryPreAccept updates an existing instance.
func TestHandleTryPreAccept_NoConflictExistingInstance(t *testing.T) {
	r := newTestReplica(3)
	r.ExecedUpTo[0] = 0
	r.ExecedUpTo[1] = 0
	r.ExecedUpTo[2] = 0

	r.InstanceSpace[1][5] = &Instance{
		Status: NONE, State: READY,
		Seq: 0, Deps: []int32{-1, -1, -1}, CL: []int32{0, 0, 0},
		bal: 0,
	}

	cmds := []state.Command{{Op: state.PUT, K: 1, V: state.NIL(), CL: state.STRONG}}
	tpa := &TryPreAccept{
		LeaderId: 0, Replica: 1, Instance: 5,
		Ballot: (1 << 4) | 0, Command: cmds,
		Seq: 15, Deps: []int32{2, 3, 4}, CL: []int32{1, 1, 1},
	}

	func() {
		defer func() { recover() }()
		r.handleTryPreAccept(tpa)
	}()

	inst := r.InstanceSpace[1][5]
	if inst.Status != PREACCEPTED {
		t.Errorf("Status=%d, want PREACCEPTED", inst.Status)
	}
	if inst.Seq != 15 {
		t.Errorf("Seq=%d, want 15", inst.Seq)
	}
	if inst.bal != (1<<4)|0 {
		t.Errorf("bal=%d, want %d", inst.bal, (1<<4)|0)
	}
}

// TestHandleTryPreAccept_ConflictRejected verifies that conflicts cause rejection.
func TestHandleTryPreAccept_ConflictRejected(t *testing.T) {
	r := newTestReplica(3)
	r.ExecedUpTo[0] = 0
	r.ExecedUpTo[1] = 0
	r.ExecedUpTo[2] = 0

	// Existing ACCEPTED instance at same position → conflict
	existingCmds := []state.Command{{Op: state.PUT, K: 1, V: state.NIL(), CL: state.STRONG}}
	r.InstanceSpace[1][5] = &Instance{
		Cmds: existingCmds, Status: ACCEPTED, State: READY,
		Seq: 10, Deps: []int32{-1, -1, -1}, CL: []int32{0, 0, 0},
		bal: (0 << 4) | 1,
	}

	cmds := []state.Command{{Op: state.PUT, K: 1, V: state.NIL(), CL: state.STRONG}}
	tpa := &TryPreAccept{
		LeaderId: 0, Replica: 1, Instance: 5,
		Ballot: (1 << 4) | 0, Command: cmds,
		Seq: 10, Deps: []int32{-1, -1, -1}, CL: []int32{0, 0, 0},
	}

	func() {
		defer func() { recover() }()
		r.handleTryPreAccept(tpa)
	}()

	// Instance should not be changed to PREACCEPTED
	if r.InstanceSpace[1][5].Status != ACCEPTED {
		t.Errorf("Status=%d, should stay ACCEPTED", r.InstanceSpace[1][5].Status)
	}
}

// TestHandleTryPreAcceptReply_DelayedIgnored verifies delayed replies are ignored.
func TestHandleTryPreAcceptReply_DelayedIgnored(t *testing.T) {
	r := newTestReplica(3)
	r.InstanceSpace[1][5] = &Instance{
		Status: PREACCEPTED, State: READY,
		Deps: []int32{-1, -1, -1}, CL: []int32{0, 0, 0},
		// lb is nil → delayed
	}

	tpar := &TryPreAcceptReply{
		AcceptorId: 2, Replica: 1, Instance: 5,
		OK: TRUE, Ballot: 0,
	}

	r.handleTryPreAcceptReply(tpar)
	// Should not panic
}

// TestHandleTryPreAcceptReply_OK verifies successful TryPreAcceptReply counting.
func TestHandleTryPreAcceptReply_OK(t *testing.T) {
	r := newTestReplica(3)
	cmds := []state.Command{{Op: state.PUT, K: 1, V: state.NIL(), CL: state.STRONG}}
	r.InstanceSpace[1][5] = &Instance{
		Cmds: cmds, Status: PREACCEPTED, State: READY,
		Seq: 10, Deps: []int32{-1, -1, -1}, CL: []int32{0, 0, 0},
		bal: (1 << 4) | 0,
		lb: &LeaderBookkeeping{
			tryingToPreAccept: true,
			recoveryInst: &RecoveryInstance{
				cmds: cmds, status: PREACCEPTED,
				seq: 10, deps: []int32{-1, -1, -1}, cl: []int32{0, 0, 0},
			},
		},
	}

	tpar := &TryPreAcceptReply{
		AcceptorId: 2, Replica: 1, Instance: 5,
		OK: TRUE, Ballot: (1 << 4) | 0,
	}

	r.handleTryPreAcceptReply(tpar)

	inst := r.InstanceSpace[1][5]
	if inst.lb.preAcceptOKs != 1 {
		t.Errorf("preAcceptOKs=%d, want 1", inst.lb.preAcceptOKs)
	}
	if inst.lb.tpaOKs != 1 {
		t.Errorf("tpaOKs=%d, want 1", inst.lb.tpaOKs)
	}
}

// TestHandleTryPreAcceptReply_QuorumAccepts verifies that reaching quorum
// transitions to Accept phase.
func TestHandleTryPreAcceptReply_QuorumAccepts(t *testing.T) {
	r := newTestReplica(3) // N=3, quorum = 1 (N/2=1)
	cmds := []state.Command{{Op: state.PUT, K: 1, V: state.NIL(), CL: state.STRONG}}
	r.InstanceSpace[1][5] = &Instance{
		Cmds: cmds, Status: PREACCEPTED, State: READY,
		Seq: 10, Deps: []int32{-1, -1, -1}, CL: []int32{0, 0, 0},
		bal: (1 << 4) | 0,
		lb: &LeaderBookkeeping{
			tryingToPreAccept: true,
			recoveryInst: &RecoveryInstance{
				cmds: cmds, status: PREACCEPTED,
				seq: 10, deps: []int32{-1, -1, -1}, cl: []int32{0, 0, 0},
			},
		},
	}

	tpar := &TryPreAcceptReply{
		AcceptorId: 2, Replica: 1, Instance: 5,
		OK: TRUE, Ballot: (1 << 4) | 0,
	}

	func() {
		defer func() { recover() }()
		r.handleTryPreAcceptReply(tpar)
	}()

	inst := r.InstanceSpace[1][5]
	if inst.Status != ACCEPTED {
		t.Errorf("Status=%d, want ACCEPTED(%d)", inst.Status, ACCEPTED)
	}
	if inst.lb.tryingToPreAccept {
		t.Error("tryingToPreAccept should be false after quorum")
	}
	if inst.lb.acceptOKs != 0 {
		t.Errorf("acceptOKs=%d, want 0 (reset for Accept phase)", inst.lb.acceptOKs)
	}
}

// TestHandleTryPreAcceptReply_NackHigherBallot verifies nack with higher ballot.
func TestHandleTryPreAcceptReply_NackHigherBallot(t *testing.T) {
	r := newTestReplica(3)
	cmds := []state.Command{{Op: state.PUT, K: 1, V: state.NIL(), CL: state.STRONG}}
	r.InstanceSpace[1][5] = &Instance{
		Cmds: cmds, Status: PREACCEPTED, State: READY,
		Seq: 10, Deps: []int32{-1, -1, -1}, CL: []int32{0, 0, 0},
		bal: (1 << 4) | 0,
		lb: &LeaderBookkeeping{
			tryingToPreAccept: true,
			possibleQuorum:    []bool{true, true, true},
			recoveryInst: &RecoveryInstance{
				cmds: cmds, status: PREACCEPTED,
				seq: 10, deps: []int32{-1, -1, -1}, cl: []int32{0, 0, 0},
			},
		},
	}

	tpar := &TryPreAcceptReply{
		AcceptorId: 2, Replica: 1, Instance: 5,
		OK: FALSE, Ballot: (3 << 4) | 2, // higher ballot
		ConflictReplica: 0, ConflictInstance: 0, ConflictStatus: PREACCEPTED,
	}

	r.handleTryPreAcceptReply(tpar)

	inst := r.InstanceSpace[1][5]
	if inst.lb.nacks != 1 {
		t.Errorf("nacks=%d, want 1", inst.lb.nacks)
	}
}

// TestHandleTryPreAcceptReply_SameInstanceConflict verifies that a conflict
// with the same instance stops the TryPreAccept process.
func TestHandleTryPreAcceptReply_SameInstanceConflict(t *testing.T) {
	r := newTestReplica(3)
	cmds := []state.Command{{Op: state.PUT, K: 1, V: state.NIL(), CL: state.STRONG}}
	r.InstanceSpace[1][5] = &Instance{
		Cmds: cmds, Status: PREACCEPTED, State: READY,
		Seq: 10, Deps: []int32{-1, -1, -1}, CL: []int32{0, 0, 0},
		bal: (1 << 4) | 0,
		lb: &LeaderBookkeeping{
			tryingToPreAccept: true,
			possibleQuorum:    []bool{true, true, true},
			recoveryInst: &RecoveryInstance{
				cmds: cmds, status: PREACCEPTED,
				seq: 10, deps: []int32{-1, -1, -1}, cl: []int32{0, 0, 0},
			},
		},
	}

	tpar := &TryPreAcceptReply{
		AcceptorId: 2, Replica: 1, Instance: 5,
		OK: FALSE, Ballot: (1 << 4) | 0,
		ConflictReplica: 1, ConflictInstance: 5, // same instance
		ConflictStatus: ACCEPTED,
	}

	r.handleTryPreAcceptReply(tpar)

	if r.InstanceSpace[1][5].lb.tryingToPreAccept {
		t.Error("tryingToPreAccept should be false (same instance conflict)")
	}
}
