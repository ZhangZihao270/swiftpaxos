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
