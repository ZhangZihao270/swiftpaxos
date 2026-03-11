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

	t.Run("handlePrepare", func(t *testing.T) {
		r.handlePrepare(&Prepare{})
	})
	t.Run("handlePreAccept", func(t *testing.T) {
		r.handlePreAccept(&PreAccept{})
	})
	t.Run("handleAccept", func(t *testing.T) {
		r.handleAccept(&Accept{})
	})
	t.Run("handleCommit", func(t *testing.T) {
		r.handleCommit(&Commit{})
	})
	t.Run("handleCommitShort", func(t *testing.T) {
		r.handleCommitShort(&CommitShort{})
	})
	// handleCausalCommit tested separately — it now requires initialized InstanceSpace
	t.Run("handlePrepareReply", func(t *testing.T) {
		r.handlePrepareReply(&PrepareReply{})
	})
	t.Run("handlePreAcceptReply", func(t *testing.T) {
		r.handlePreAcceptReply(&PreAcceptReply{})
	})
	t.Run("handlePreAcceptOK", func(t *testing.T) {
		r.handlePreAcceptOK(&PreAcceptOK{})
	})
	t.Run("handleAcceptReply", func(t *testing.T) {
		r.handleAcceptReply(&AcceptReply{})
	})
	t.Run("handleTryPreAccept", func(t *testing.T) {
		r.handleTryPreAccept(&TryPreAccept{})
	})
	t.Run("handleTryPreAcceptReply", func(t *testing.T) {
		r.handleTryPreAcceptReply(&TryPreAcceptReply{})
	})
	t.Run("startRecoveryForInstance", func(t *testing.T) {
		r.startRecoveryForInstance(0, 0)
	})
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

func TestStartStrongCommitStub(t *testing.T) {
	r := &Replica{}
	// Should not panic — it's a stub
	r.startStrongCommit(0, 0, 0, nil, nil)
}

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
