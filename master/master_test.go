package master

import (
	"testing"

	"github.com/imdea-software/swiftpaxos/dlog"
	"github.com/imdea-software/swiftpaxos/replica/defs"
)

func newTestMaster(n int) *Master {
	l := dlog.New("", false)
	return New(n, 7087, l)
}

// TestRegister_DeterministicPlacement verifies that replicas are placed at
// their requested ReplicaId, regardless of registration order.
func TestRegister_DeterministicPlacement(t *testing.T) {
	m := newTestMaster(3)

	// Register in reverse order: 2, 1, 0.
	addrs := []string{"127.0.0.1", "127.0.0.2", "127.0.0.3"}
	for _, id := range []int{2, 1, 0} {
		reply := &defs.RegisterReply{}
		err := m.Register(&defs.RegisterArgs{
			Addr:      addrs[id],
			Port:      7070 + id,
			ReplicaId: id,
		}, reply)
		if err != nil {
			t.Fatalf("Register(id=%d) error: %v", id, err)
		}
	}

	// Verify placement.
	for i := 0; i < 3; i++ {
		expected := addrs[i]
		if m.addrList[i] != expected {
			t.Errorf("addrList[%d] = %q, want %q", i, m.addrList[i], expected)
		}
		if m.portList[i] != 7070+i {
			t.Errorf("portList[%d] = %d, want %d", i, m.portList[i], 7070+i)
		}
	}
}

// TestRegister_Replica0IsLeader verifies that replica 0 is always the leader.
func TestRegister_Replica0IsLeader(t *testing.T) {
	m := newTestMaster(3)

	// Register in order 2, 0, 1 (replica 0 registers second).
	for _, id := range []int{2, 0, 1} {
		reply := &defs.RegisterReply{}
		err := m.Register(&defs.RegisterArgs{
			Addr:      "127.0.0.1",
			Port:      7070 + id,
			ReplicaId: id,
		}, reply)
		if err != nil {
			t.Fatalf("Register(id=%d) error: %v", id, err)
		}

		if reply.Ready {
			if id == 0 && !reply.IsLeader {
				t.Error("replica 0 should be leader when all registered")
			}
			if id != 0 && reply.IsLeader {
				t.Errorf("replica %d should not be leader", id)
			}
		}
	}

	// Verify leader array.
	if !m.leader[0] {
		t.Error("master.leader[0] should be true")
	}
	for i := 1; i < 3; i++ {
		if m.leader[i] {
			t.Errorf("master.leader[%d] should be false", i)
		}
	}
}

// TestRegister_InvalidReplicaId verifies that out-of-range IDs are rejected.
func TestRegister_InvalidReplicaId(t *testing.T) {
	m := newTestMaster(3)

	reply := &defs.RegisterReply{}
	err := m.Register(&defs.RegisterArgs{
		Addr:      "127.0.0.1",
		Port:      7070,
		ReplicaId: 5, // out of range
	}, reply)
	if err == nil {
		t.Error("expected error for out-of-range ReplicaId, got nil")
	}

	err = m.Register(&defs.RegisterArgs{
		Addr:      "127.0.0.1",
		Port:      7070,
		ReplicaId: -1,
	}, reply)
	if err == nil {
		t.Error("expected error for negative ReplicaId, got nil")
	}
}

// TestRegister_DuplicateIdempotent verifies that re-registering the same ID
// is idempotent and doesn't increment the count.
func TestRegister_DuplicateIdempotent(t *testing.T) {
	m := newTestMaster(2)

	reply := &defs.RegisterReply{}
	// Register replica 0 twice.
	for i := 0; i < 2; i++ {
		err := m.Register(&defs.RegisterArgs{
			Addr:      "127.0.0.1",
			Port:      7070,
			ReplicaId: 0,
		}, reply)
		if err != nil {
			t.Fatalf("Register attempt %d error: %v", i, err)
		}
	}

	if m.numRegistered != 1 {
		t.Errorf("numRegistered = %d, want 1 (duplicate should not increment)", m.numRegistered)
	}
	if reply.Ready {
		t.Error("should not be ready with only 1 of 2 replicas registered")
	}
}

// TestRegister_ReadyOnlyWhenAllRegistered verifies that Ready is false
// until all N replicas have registered.
func TestRegister_ReadyOnlyWhenAllRegistered(t *testing.T) {
	m := newTestMaster(3)

	for id := 0; id < 3; id++ {
		reply := &defs.RegisterReply{}
		err := m.Register(&defs.RegisterArgs{
			Addr:      "127.0.0.1",
			Port:      7070 + id,
			ReplicaId: id,
		}, reply)
		if err != nil {
			t.Fatalf("Register(id=%d) error: %v", id, err)
		}

		if id < 2 && reply.Ready {
			t.Errorf("should not be ready after %d registrations", id+1)
		}
		if id == 2 && !reply.Ready {
			t.Error("should be ready after all 3 registrations")
		}
	}
}
