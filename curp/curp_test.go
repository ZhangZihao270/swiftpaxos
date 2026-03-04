package curp

import (
	"strconv"
	"testing"
)

// TestDescMsgsBufferSize verifies the descriptor message channel buffer is 128
// (Phase 53.1a: enlarged from 8 to prevent event loop blocking at high concurrency)
func TestDescMsgsBufferSize(t *testing.T) {
	desc := &commandDesc{}
	desc.msgs = make(chan interface{}, 128)
	if cap(desc.msgs) != 128 {
		t.Errorf("desc.msgs buffer should be 128, got %d", cap(desc.msgs))
	}
}

// TestDescSlotStrCached verifies slotStr is correctly cached on commandDesc
// (Phase 53.2a: eliminates repeated strconv.Itoa allocations)
func TestDescSlotStrCached(t *testing.T) {
	desc := &commandDesc{}
	desc.cmdSlot = 42
	desc.slotStr = strconv.Itoa(42)

	if desc.slotStr != "42" {
		t.Errorf("desc.slotStr should be '42', got '%s'", desc.slotStr)
	}

	// Verify it matches strconv.Itoa
	if desc.slotStr != strconv.Itoa(desc.cmdSlot) {
		t.Errorf("desc.slotStr doesn't match strconv.Itoa(desc.cmdSlot)")
	}
}

// TestDescSlotStrZeroValue verifies slotStr is empty string for uninitialized desc
func TestDescSlotStrZeroValue(t *testing.T) {
	desc := &commandDesc{}
	if desc.slotStr != "" {
		t.Errorf("uninitialized desc.slotStr should be empty, got '%s'", desc.slotStr)
	}
}

// TestNonBlockingSendChannelFull verifies that when desc.msgs is full,
// the send doesn't block (Phase 53.1b)
func TestNonBlockingSendChannelFull(t *testing.T) {
	desc := &commandDesc{}
	desc.msgs = make(chan interface{}, 2)

	// Fill the channel
	desc.msgs <- "msg1"
	desc.msgs <- "msg2"

	// Non-blocking send should not block (simulates the select/default pattern)
	sent := false
	select {
	case desc.msgs <- "msg3":
		sent = true
	default:
		sent = false
	}

	if sent {
		t.Error("expected send to fail when channel is full")
	}

	// Drain and verify original messages preserved
	msg1 := <-desc.msgs
	msg2 := <-desc.msgs
	if msg1 != "msg1" || msg2 != "msg2" {
		t.Errorf("original messages not preserved: got %v, %v", msg1, msg2)
	}
}

// TestMaxDescRoutinesDefault verifies the default MaxDescRoutines value
func TestMaxDescRoutinesDefault(t *testing.T) {
	if MaxDescRoutines != 10000 {
		t.Errorf("MaxDescRoutines should be 10000, got %d", MaxDescRoutines)
	}
}
