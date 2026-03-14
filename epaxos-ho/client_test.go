package epaxosho

import (
	"testing"
)

// TestClientSupportsWeak verifies EPaxos-HO supports weak consistency.
func TestClientSupportsWeak(t *testing.T) {
	c := &Client{deadReplicas: make(map[int]bool)}
	if !c.SupportsWeak() {
		t.Error("EPaxos-HO client should support weak consistency")
	}
}

// TestClientMarkAllSent verifies MarkAllSent is a no-op.
func TestClientMarkAllSent(t *testing.T) {
	c := &Client{deadReplicas: make(map[int]bool)}
	c.MarkAllSent()
}

// TestFindNextAlive_RoundRobin tests round-robin failover without ping data.
func TestFindNextAlive_RoundRobin(t *testing.T) {
	c := &Client{
		numReplicas:  5,
		closestId:    0,
		deadReplicas: make(map[int]bool),
	}

	// No ping data, dead replica 0 → should pick 1
	c.deadReplicas[0] = true
	next := c.findNextAlive(0)
	if next != 1 {
		t.Errorf("expected next=1, got %d", next)
	}

	// Dead 0 and 1 → should pick 2
	c.deadReplicas[1] = true
	next = c.findNextAlive(0)
	if next != 2 {
		t.Errorf("expected next=2, got %d", next)
	}

	// Dead 0, 1, 2, 3 → should pick 4
	c.deadReplicas[2] = true
	c.deadReplicas[3] = true
	next = c.findNextAlive(0)
	if next != 4 {
		t.Errorf("expected next=4, got %d", next)
	}
}

// TestFindNextAlive_WrapAround tests round-robin wrap-around.
func TestFindNextAlive_WrapAround(t *testing.T) {
	c := &Client{
		numReplicas:  5,
		closestId:    3,
		deadReplicas: make(map[int]bool),
	}

	// Dead 3, 4 → should wrap to 0
	c.deadReplicas[3] = true
	c.deadReplicas[4] = true
	next := c.findNextAlive(3)
	if next != 0 {
		t.Errorf("expected next=0, got %d", next)
	}
}

// TestFindNextAlive_WithPing tests failover using ping latency.
func TestFindNextAlive_WithPing(t *testing.T) {
	c := &Client{
		numReplicas:  5,
		closestId:    0,
		deadReplicas: make(map[int]bool),
	}
	// Set ping: replica 2 is closest after replica 0
	c.ping = []float64{1.0, 10.0, 3.0, 5.0, 8.0}

	// Dead replica 0 → should pick replica 2 (lowest ping among alive)
	c.deadReplicas[0] = true
	next := c.findNextAlive(0)
	if next != 2 {
		t.Errorf("expected next=2 (ping=3.0), got %d", next)
	}

	// Dead 0 and 2 → should pick 3 (next lowest)
	c.deadReplicas[2] = true
	next = c.findNextAlive(0)
	if next != 3 {
		t.Errorf("expected next=3 (ping=5.0), got %d", next)
	}
}

// TestFindNextAlive_AllDead tests fallback when all replicas are dead.
func TestFindNextAlive_AllDead(t *testing.T) {
	c := &Client{
		numReplicas:  3,
		closestId:    0,
		deadReplicas: make(map[int]bool),
	}
	c.deadReplicas[0] = true
	c.deadReplicas[1] = true
	c.deadReplicas[2] = true

	// All dead → should still return (current+1)%N = 1
	next := c.findNextAlive(0)
	if next != 1 {
		t.Errorf("expected fallback next=1, got %d", next)
	}
}

// TestDeadReplicaTracking tests that dead replicas are properly tracked.
func TestDeadReplicaTracking(t *testing.T) {
	c := &Client{
		numReplicas:  5,
		closestId:    2,
		deadReplicas: make(map[int]bool),
	}

	if c.deadReplicas[0] {
		t.Error("replica 0 should be alive initially")
	}

	c.deadReplicas[0] = true
	c.deadReplicas[3] = true

	if !c.deadReplicas[0] {
		t.Error("replica 0 should be dead")
	}
	if !c.deadReplicas[3] {
		t.Error("replica 3 should be dead")
	}
	if c.deadReplicas[2] {
		t.Error("replica 2 should still be alive")
	}
}
