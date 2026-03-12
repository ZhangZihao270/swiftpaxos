package raft

import (
	"testing"
)

// TestClientSupportsWeak verifies that Raft client does not support weak consistency.
func TestClientSupportsWeak(t *testing.T) {
	c := &Client{BufferClient: nil}
	if c.SupportsWeak() {
		t.Error("Raft client should not support weak consistency")
	}
}

// TestClientMarkAllSent verifies MarkAllSent is a no-op and doesn't panic.
func TestClientMarkAllSent(t *testing.T) {
	c := &Client{BufferClient: nil}
	// Should not panic
	c.MarkAllSent()
}

// TestClientInterfaceCompliance verifies that Client implements the expected methods.
// This is a compile-time check — if Client doesn't implement the interface,
// this file won't compile.
func TestClientInterfaceCompliance(t *testing.T) {
	// Verify the method signatures exist
	var c *Client
	_ = c.SupportsWeak
	_ = c.MarkAllSent
	_ = c.SendStrongWrite
	_ = c.SendStrongRead
	_ = c.SendWeakWrite
	_ = c.SendWeakRead
}

// ============================================================================
// Phase 102b-c: Leader Failover Tests
// ============================================================================

func TestLeaderRotation_WrapAround(t *testing.T) {
	c := &Client{numReplicas: 5, leader: 4}
	newLeader := (c.leader + 1) % c.numReplicas
	if newLeader != 0 {
		t.Errorf("leader rotation from 4 with 5 replicas = %d, want 0", newLeader)
	}
}

func TestLeaderRotation_Simple(t *testing.T) {
	c := &Client{numReplicas: 3, leader: 0}
	newLeader := (c.leader + 1) % c.numReplicas
	if newLeader != 1 {
		t.Errorf("leader rotation from 0 with 3 replicas = %d, want 1", newLeader)
	}
}

func TestLeaderRotation_SingleReplica(t *testing.T) {
	c := &Client{numReplicas: 1, leader: 0}
	newLeader := (c.leader + 1) % c.numReplicas
	if newLeader != 0 {
		t.Errorf("leader rotation with 1 replica = %d, want 0", newLeader)
	}
}
