package defs

import (
	"bytes"
	"testing"

	"github.com/imdea-software/swiftpaxos/state"
)

func TestProposeReplyTSSerializationWithLeaderId(t *testing.T) {
	original := &ProposeReplyTS{
		OK:        FALSE,
		CommandId: 42,
		Value:     state.NIL(),
		Timestamp: 12345678,
		LeaderId:  3,
	}

	var buf bytes.Buffer
	original.Marshal(&buf)

	decoded := &ProposeReplyTS{}
	if err := decoded.Unmarshal(&buf); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if decoded.OK != original.OK {
		t.Errorf("OK = %d, want %d", decoded.OK, original.OK)
	}
	if decoded.CommandId != original.CommandId {
		t.Errorf("CommandId = %d, want %d", decoded.CommandId, original.CommandId)
	}
	if decoded.Timestamp != original.Timestamp {
		t.Errorf("Timestamp = %d, want %d", decoded.Timestamp, original.Timestamp)
	}
	if decoded.LeaderId != original.LeaderId {
		t.Errorf("LeaderId = %d, want %d", decoded.LeaderId, original.LeaderId)
	}
}

func TestProposeReplyTSLeaderIdNegativeOne(t *testing.T) {
	original := &ProposeReplyTS{
		OK:        TRUE,
		CommandId: 99,
		Value:     state.NIL(),
		Timestamp: 0,
		LeaderId:  -1,
	}

	var buf bytes.Buffer
	original.Marshal(&buf)

	decoded := &ProposeReplyTS{}
	if err := decoded.Unmarshal(&buf); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if decoded.LeaderId != -1 {
		t.Errorf("LeaderId = %d, want -1", decoded.LeaderId)
	}
}

func TestProposeReplyTSLeaderIdZero(t *testing.T) {
	// LeaderId=0 means "leader is replica 0" — must not be confused with "no hint"
	original := &ProposeReplyTS{
		OK:        FALSE,
		CommandId: 1,
		Value:     state.NIL(),
		Timestamp: 0,
		LeaderId:  0,
	}

	var buf bytes.Buffer
	original.Marshal(&buf)

	decoded := &ProposeReplyTS{}
	if err := decoded.Unmarshal(&buf); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if decoded.LeaderId != 0 {
		t.Errorf("LeaderId = %d, want 0", decoded.LeaderId)
	}
}
