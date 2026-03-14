package epaxosswift

import (
	"testing"
)

// TestClientSupportsWeak verifies that EPaxos-Swift client does not support weak consistency.
func TestClientSupportsWeak(t *testing.T) {
	c := &Client{deadReplicas: make(map[int]bool)}
	if c.SupportsWeak() {
		t.Error("EPaxos-Swift client should not support weak consistency")
	}
}

// TestClientMarkAllSent verifies MarkAllSent is a no-op and doesn't panic.
func TestClientMarkAllSent(t *testing.T) {
	c := &Client{deadReplicas: make(map[int]bool)}
	c.MarkAllSent()
}

// TestClientInterfaceCompliance verifies that Client implements the expected methods.
func TestClientInterfaceCompliance(t *testing.T) {
	var c *Client
	_ = c.SupportsWeak
	_ = c.MarkAllSent
	_ = c.SendStrongWrite
	_ = c.SendStrongRead
	_ = c.SendWeakWrite
	_ = c.SendWeakRead
}

// TestSwiftFindNextAlive_RoundRobin tests round-robin failover.
func TestSwiftFindNextAlive_RoundRobin(t *testing.T) {
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
}

// TestSwiftFindNextAlive_WithPing tests ping-based failover.
func TestSwiftFindNextAlive_WithPing(t *testing.T) {
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
