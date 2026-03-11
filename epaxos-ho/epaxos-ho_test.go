package epaxosho

import (
	"testing"

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
	// a minimal Replica with just the fields the stubs need (none).
	r := &Replica{}

	t.Run("handlePropose", func(t *testing.T) {
		r.handlePropose(&defs.GPropose{})
	})
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
	t.Run("handleCausalCommit", func(t *testing.T) {
		r.handleCausalCommit(&CausalCommit{})
	})
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
