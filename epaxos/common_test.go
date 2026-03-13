package epaxos

import (
	"testing"

	"github.com/imdea-software/swiftpaxos/state"
)

func TestNewInstance(t *testing.T) {
	cmds := []state.Command{{Op: state.PUT, K: 1, V: []byte{42}}}
	deps := []int32{0, 1, 2}
	inst := NewInstance(1, 5, cmds, 10, 9, PREACCEPTED, 3, deps)

	if inst.Bal != 10 {
		t.Errorf("Bal = %d, want 10", inst.Bal)
	}
	if inst.Vbal != 9 {
		t.Errorf("Vbal = %d, want 9", inst.Vbal)
	}
	if inst.Status != PREACCEPTED {
		t.Errorf("Status = %d, want PREACCEPTED", inst.Status)
	}
	if inst.Seq != 3 {
		t.Errorf("Seq = %d, want 3", inst.Seq)
	}
	if inst.Id == nil || inst.Id.Replica != 1 || inst.Id.Instance != 5 {
		t.Errorf("Id = %+v, want {1, 5}", inst.Id)
	}
	if inst.ProposeTime <= 0 {
		t.Error("ProposeTime should be set to current time")
	}
	if len(inst.Cmds) != 1 {
		t.Fatalf("Cmds len = %d, want 1", len(inst.Cmds))
	}
	if len(inst.Deps) != 3 || inst.Deps[2] != 2 {
		t.Errorf("Deps = %v, want [0 1 2]", inst.Deps)
	}
	if inst.Lb != nil {
		t.Error("Lb should be nil for newly created instance")
	}
}

func TestIsInitialBallot(t *testing.T) {
	if !IsInitialBallot(0, 0) {
		t.Error("ballot=0, replica=0 should be initial")
	}
	if !IsInitialBallot(2, 2) {
		t.Error("ballot=2, replica=2 should be initial")
	}
	if IsInitialBallot(3, 0) {
		t.Error("ballot=3, replica=0 should not be initial")
	}
	if IsInitialBallot(0, 1) {
		t.Error("ballot=0, replica=1 should not be initial")
	}
}

func TestMakeBallot_SameReplica(t *testing.T) {
	// When replica owns the instance, ballot starts at replicaId.
	b := MakeBallot(2, 2, 5, 0, false)
	if b != 2 {
		t.Errorf("MakeBallot(2,2,5,0,false) = %d, want 2", b)
	}
}

func TestMakeBallot_DifferentReplica(t *testing.T) {
	// When replica doesn't own the instance, ballot = replicaId + N.
	b := MakeBallot(1, 0, 3, 0, false)
	if b != 4 {
		t.Errorf("MakeBallot(1,0,3,0,false) = %d, want 4", b)
	}
}

func TestMakeBallot_LeaderExceedsMaxRecv(t *testing.T) {
	// Leader keeps incrementing by N until >= maxRecvBallot.
	b := MakeBallot(0, 0, 3, 10, true)
	if b < 10 {
		t.Errorf("MakeBallot leader ballot = %d, should be >= 10", b)
	}
	// Must be congruent to replicaId mod N.
	if b%3 != 0 {
		t.Errorf("ballot %d not congruent to 0 mod 3", b)
	}
}

func TestMakeBallot_LeaderCongruence(t *testing.T) {
	// For replica 2 with N=5, ballot should be ≡ 2 mod 5.
	b := MakeBallot(2, 2, 5, 20, true)
	if b < 20 {
		t.Errorf("ballot = %d, should be >= 20", b)
	}
	if b%5 != 2 {
		t.Errorf("ballot %d not congruent to 2 mod 5", b)
	}
}

func TestSortInstances_BySeq(t *testing.T) {
	instances := []*Instance{
		{Seq: 5, Id: &InstanceId{0, 0}},
		{Seq: 1, Id: &InstanceId{0, 1}},
		{Seq: 3, Id: &InstanceId{0, 2}},
	}
	SortInstances(instances)
	if instances[0].Seq != 1 || instances[1].Seq != 3 || instances[2].Seq != 5 {
		t.Errorf("not sorted by Seq: %d, %d, %d", instances[0].Seq, instances[1].Seq, instances[2].Seq)
	}
}

func TestSortInstances_TiebreakByReplica(t *testing.T) {
	instances := []*Instance{
		{Seq: 3, Id: &InstanceId{2, 0}},
		{Seq: 3, Id: &InstanceId{0, 0}},
		{Seq: 3, Id: &InstanceId{1, 0}},
	}
	SortInstances(instances)
	if instances[0].Id.Replica != 0 || instances[1].Id.Replica != 1 || instances[2].Id.Replica != 2 {
		t.Error("not sorted by replica ID on equal Seq")
	}
}

func TestSortInstances_TiebreakByProposeTime(t *testing.T) {
	instances := []*Instance{
		{Seq: 3, Id: &InstanceId{1, 0}, ProposeTime: 300},
		{Seq: 3, Id: &InstanceId{1, 1}, ProposeTime: 100},
		{Seq: 3, Id: &InstanceId{1, 2}, ProposeTime: 200},
	}
	SortInstances(instances)
	if instances[0].ProposeTime != 100 || instances[1].ProposeTime != 200 || instances[2].ProposeTime != 300 {
		t.Error("not sorted by ProposeTime on equal Seq and Replica")
	}
}

func TestSortInstances_Empty(t *testing.T) {
	var instances []*Instance
	SortInstances(instances) // should not panic
}

func TestNodeArray_Interface(t *testing.T) {
	na := NodeArray{
		{Seq: 2, Id: &InstanceId{0, 0}},
		{Seq: 1, Id: &InstanceId{0, 1}},
	}
	if na.Len() != 2 {
		t.Errorf("Len() = %d, want 2", na.Len())
	}
	if !na.Less(1, 0) {
		t.Error("Less(1,0) should be true (Seq 1 < 2)")
	}
	na.Swap(0, 1)
	if na[0].Seq != 1 || na[1].Seq != 2 {
		t.Error("Swap didn't work")
	}
}

func TestLeaderBookkeeping_Defaults(t *testing.T) {
	lb := &LeaderBookkeeping{
		AllEqual:  true,
		Preparing: true,
		Ballot:    -1,
	}
	if !lb.AllEqual {
		t.Error("AllEqual should default to true")
	}
	if lb.PreAcceptOKs != 0 {
		t.Errorf("PreAcceptOKs = %d, want 0", lb.PreAcceptOKs)
	}
	if lb.AcceptOKs != 0 {
		t.Errorf("AcceptOKs = %d, want 0", lb.AcceptOKs)
	}
}

func TestInstPair(t *testing.T) {
	ip := InstPair{Last: 5, LastWrite: 3}
	if ip.Last != 5 || ip.LastWrite != 3 {
		t.Errorf("InstPair = %+v, want {5, 3}", ip)
	}
}

func TestInstanceId(t *testing.T) {
	id := InstanceId{Replica: 2, Instance: 10}
	if id.Replica != 2 || id.Instance != 10 {
		t.Errorf("InstanceId = %+v, want {2, 10}", id)
	}
}

func TestConstants(t *testing.T) {
	if TRUE != 1 {
		t.Errorf("TRUE = %d, want 1", TRUE)
	}
	if FALSE != 0 {
		t.Errorf("FALSE = %d, want 0", FALSE)
	}
	if MAX_INSTANCE != 10*1024*1024 {
		t.Errorf("MAX_INSTANCE = %d", MAX_INSTANCE)
	}
}
