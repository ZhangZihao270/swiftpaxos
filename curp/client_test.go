package curp

import (
	"testing"

	"github.com/imdea-software/swiftpaxos/client"
)

// TestClientImplementsHybridClient verifies that Client implements the HybridClient interface
func TestClientImplementsHybridClient(t *testing.T) {
	// This will fail to compile if Client doesn't implement HybridClient
	var _ client.HybridClient = (*Client)(nil)
}

// TestClientSupportsWeak verifies that SupportsWeak returns false for CURP
func TestClientSupportsWeak(t *testing.T) {
	c := &Client{}
	if c.SupportsWeak() {
		t.Error("CURP client should not support weak consistency")
	}
}

// TestClientStrongMethods verifies that strong methods delegate to BufferClient
func TestClientStrongMethods(t *testing.T) {
	// Create a minimal client for testing
	b := &client.BufferClient{}
	c := &Client{
		BufferClient: b,
	}

	// These should not panic - they delegate to BufferClient methods
	// We can't actually call them without full setup, but we verify the methods exist
	_ = c.SendStrongWrite
	_ = c.SendStrongRead
}

// TestClientWeakMethodsPanic verifies that weak methods panic when called
func TestClientWeakMethodsPanic(t *testing.T) {
	c := &Client{}

	// Test SendWeakWrite panics
	defer func() {
		if r := recover(); r == nil {
			t.Error("SendWeakWrite should panic")
		}
	}()
	c.SendWeakWrite(1, []byte("test"))
}

// TestClientWeakReadPanics verifies that SendWeakRead panics when called
func TestClientWeakReadPanics(t *testing.T) {
	c := &Client{}

	// Test SendWeakRead panics
	defer func() {
		if r := recover(); r == nil {
			t.Error("SendWeakRead should panic")
		}
	}()
	c.SendWeakRead(1)
}

// TestClientMarkAllSent verifies that MarkAllSent is a no-op (doesn't panic)
func TestClientMarkAllSent(t *testing.T) {
	c := &Client{}
	// Should not panic
	c.MarkAllSent()
}
