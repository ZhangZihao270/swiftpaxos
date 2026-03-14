package epaxos

import (
	"testing"
)

// TestEpaxosClientSupportsWeak verifies vanilla EPaxos does not support weak.
func TestEpaxosClientSupportsWeak(t *testing.T) {
	c := &Client{deadReplicas: make(map[int]bool)}
	if c.SupportsWeak() {
		t.Error("EPaxos client should not support weak consistency")
	}
}

// TestEpaxosFindNextAlive_RoundRobin tests round-robin failover.
func TestEpaxosFindNextAlive_RoundRobin(t *testing.T) {
	c := &Client{
		numReplicas:  5,
		closestId:    0,
		deadReplicas: make(map[int]bool),
	}

	c.deadReplicas[0] = true
	next := c.findNextAlive(0)
	if next != 1 {
		t.Errorf("expected next=1, got %d", next)
	}

	c.deadReplicas[1] = true
	next = c.findNextAlive(0)
	if next != 2 {
		t.Errorf("expected next=2, got %d", next)
	}
}

// TestEpaxosFindNextAlive_WithPing tests ping-based failover.
func TestEpaxosFindNextAlive_WithPing(t *testing.T) {
	c := &Client{
		numReplicas:  5,
		closestId:    0,
		deadReplicas: make(map[int]bool),
	}
	c.ping = []float64{1.0, 10.0, 3.0, 5.0, 8.0}

	c.deadReplicas[0] = true
	next := c.findNextAlive(0)
	if next != 2 {
		t.Errorf("expected next=2 (ping=3.0), got %d", next)
	}
}

// TestEpaxosFindNextAlive_AllDead tests all-dead fallback.
func TestEpaxosFindNextAlive_AllDead(t *testing.T) {
	c := &Client{
		numReplicas:  3,
		closestId:    0,
		deadReplicas: make(map[int]bool),
	}
	c.deadReplicas[0] = true
	c.deadReplicas[1] = true
	c.deadReplicas[2] = true

	next := c.findNextAlive(0)
	if next != 1 {
		t.Errorf("expected fallback next=1, got %d", next)
	}
}
