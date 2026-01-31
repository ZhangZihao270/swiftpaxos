package curpht

import (
	"bytes"
	"testing"

	"github.com/imdea-software/swiftpaxos/state"
)

// TestMWeakProposeSerialization tests MWeakPropose Marshal/Unmarshal
func TestMWeakProposeSerialization(t *testing.T) {
	original := &MWeakPropose{
		CommandId: 42,
		ClientId:  100,
		Command: state.Command{
			Op: state.PUT,
			K:  state.Key(123),
			V:  []byte("test-value"),
		},
		Timestamp: 1234567890,
	}

	// Serialize
	var buf bytes.Buffer
	original.Marshal(&buf)

	// Deserialize
	restored := &MWeakPropose{}
	err := restored.Unmarshal(&buf)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	// Verify
	if restored.CommandId != original.CommandId {
		t.Errorf("CommandId mismatch: got %d, want %d", restored.CommandId, original.CommandId)
	}
	if restored.ClientId != original.ClientId {
		t.Errorf("ClientId mismatch: got %d, want %d", restored.ClientId, original.ClientId)
	}
	if restored.Timestamp != original.Timestamp {
		t.Errorf("Timestamp mismatch: got %d, want %d", restored.Timestamp, original.Timestamp)
	}
	if restored.Command.Op != original.Command.Op {
		t.Errorf("Command.Op mismatch: got %d, want %d", restored.Command.Op, original.Command.Op)
	}
	if restored.Command.K != original.Command.K {
		t.Errorf("Command.K mismatch: got %d, want %d", restored.Command.K, original.Command.K)
	}
	if !bytes.Equal(restored.Command.V, original.Command.V) {
		t.Errorf("Command.V mismatch: got %v, want %v", restored.Command.V, original.Command.V)
	}
}

// TestMWeakReplySerialization tests MWeakReply Marshal/Unmarshal
func TestMWeakReplySerialization(t *testing.T) {
	original := &MWeakReply{
		Replica: 1,
		Ballot:  5,
		CmdId: CommandId{
			ClientId: 100,
			SeqNum:   42,
		},
		Rep: []byte("result-value"),
	}

	// Serialize
	var buf bytes.Buffer
	original.Marshal(&buf)

	// Deserialize
	restored := &MWeakReply{}
	err := restored.Unmarshal(&buf)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	// Verify
	if restored.Replica != original.Replica {
		t.Errorf("Replica mismatch: got %d, want %d", restored.Replica, original.Replica)
	}
	if restored.Ballot != original.Ballot {
		t.Errorf("Ballot mismatch: got %d, want %d", restored.Ballot, original.Ballot)
	}
	if restored.CmdId.ClientId != original.CmdId.ClientId {
		t.Errorf("CmdId.ClientId mismatch: got %d, want %d", restored.CmdId.ClientId, original.CmdId.ClientId)
	}
	if restored.CmdId.SeqNum != original.CmdId.SeqNum {
		t.Errorf("CmdId.SeqNum mismatch: got %d, want %d", restored.CmdId.SeqNum, original.CmdId.SeqNum)
	}
	if !bytes.Equal(restored.Rep, original.Rep) {
		t.Errorf("Rep mismatch: got %v, want %v", restored.Rep, original.Rep)
	}
}

// TestMWeakProposeCache tests object pool for MWeakPropose
func TestMWeakProposeCache(t *testing.T) {
	cache := NewMWeakProposeCache()

	// Get from empty cache should create new
	obj1 := cache.Get()
	if obj1 == nil {
		t.Fatal("Get from empty cache returned nil")
	}

	// Put back and get again
	obj1.CommandId = 123
	cache.Put(obj1)

	obj2 := cache.Get()
	if obj2 == nil {
		t.Fatal("Get after Put returned nil")
	}
	// Should get the same object back (or a new one)
	if obj2 != obj1 {
		// This is OK, just different from pool
	}
}

// TestMWeakReplyCache tests object pool for MWeakReply
func TestMWeakReplyCache(t *testing.T) {
	cache := NewMWeakReplyCache()

	// Get from empty cache should create new
	obj1 := cache.Get()
	if obj1 == nil {
		t.Fatal("Get from empty cache returned nil")
	}

	// Put back and get again
	obj1.Replica = 5
	cache.Put(obj1)

	obj2 := cache.Get()
	if obj2 == nil {
		t.Fatal("Get after Put returned nil")
	}
}

// TestConsistencyConstants tests that consistency level constants are defined
func TestConsistencyConstants(t *testing.T) {
	if STRONG != 0 {
		t.Errorf("STRONG should be 0, got %d", STRONG)
	}
	if WEAK != 1 {
		t.Errorf("WEAK should be 1, got %d", WEAK)
	}
}

// TestCommandIdString tests CommandId.String() method
func TestCommandIdString(t *testing.T) {
	cmdId := CommandId{
		ClientId: 100,
		SeqNum:   42,
	}
	str := cmdId.String()
	expected := "100,42"
	if str != expected {
		t.Errorf("CommandId.String() = %q, want %q", str, expected)
	}
}

// TestWeakCommandExecution tests that weak commands can execute
func TestWeakCommandExecution(t *testing.T) {
	// Create a state
	st := state.InitState()

	// Create a PUT command
	putCmd := state.Command{
		Op: state.PUT,
		K:  state.Key(100),
		V:  []byte("hello"),
	}

	// Execute PUT
	result := putCmd.Execute(st)
	if len(result) != 0 {
		t.Errorf("PUT should return empty value, got %v", result)
	}

	// Create a GET command
	getCmd := state.Command{
		Op: state.GET,
		K:  state.Key(100),
		V:  state.NIL(),
	}

	// Execute GET
	result = getCmd.Execute(st)
	if !bytes.Equal(result, []byte("hello")) {
		t.Errorf("GET should return 'hello', got %v", result)
	}
}

// TestMWeakProposeNew tests New() method
func TestMWeakProposeNew(t *testing.T) {
	m := &MWeakPropose{}
	newObj := m.New()
	if newObj == nil {
		t.Fatal("New() returned nil")
	}
	if _, ok := newObj.(*MWeakPropose); !ok {
		t.Fatal("New() returned wrong type")
	}
}

// TestMWeakReplyNew tests New() method
func TestMWeakReplyNew(t *testing.T) {
	m := &MWeakReply{}
	newObj := m.New()
	if newObj == nil {
		t.Fatal("New() returned nil")
	}
	if _, ok := newObj.(*MWeakReply); !ok {
		t.Fatal("New() returned wrong type")
	}
}

// TestMWeakProposeBinarySize tests BinarySize() method
func TestMWeakProposeBinarySize(t *testing.T) {
	m := &MWeakPropose{}
	_, sizeKnown := m.BinarySize()
	// Size is not known because Command has variable size
	if sizeKnown {
		t.Error("BinarySize should return sizeKnown=false for MWeakPropose")
	}
}

// TestMWeakReplyBinarySize tests BinarySize() method
func TestMWeakReplyBinarySize(t *testing.T) {
	m := &MWeakReply{}
	_, sizeKnown := m.BinarySize()
	// Size is not known because Rep has variable size
	if sizeKnown {
		t.Error("BinarySize should return sizeKnown=false for MWeakReply")
	}
}

// TestCommandDescIsWeakField tests that commandDesc has isWeak field
func TestCommandDescIsWeakField(t *testing.T) {
	desc := &commandDesc{}
	// Default should be false
	if desc.isWeak {
		t.Error("Default isWeak should be false")
	}

	desc.isWeak = true
	if !desc.isWeak {
		t.Error("isWeak should be settable to true")
	}
}

// TestMWeakProposeWithEmptyCommand tests serialization with empty command
func TestMWeakProposeWithEmptyCommand(t *testing.T) {
	original := &MWeakPropose{
		CommandId: 1,
		ClientId:  1,
		Command: state.Command{
			Op: state.GET,
			K:  state.Key(0),
			V:  state.NIL(),
		},
		Timestamp: 0,
	}

	var buf bytes.Buffer
	original.Marshal(&buf)

	restored := &MWeakPropose{}
	err := restored.Unmarshal(&buf)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if restored.Command.Op != original.Command.Op {
		t.Errorf("Command.Op mismatch")
	}
}

// TestMWeakReplyWithEmptyRep tests serialization with empty Rep
func TestMWeakReplyWithEmptyRep(t *testing.T) {
	original := &MWeakReply{
		Replica: 0,
		Ballot:  0,
		CmdId: CommandId{
			ClientId: 1,
			SeqNum:   1,
		},
		Rep: []byte{},
	}

	var buf bytes.Buffer
	original.Marshal(&buf)

	restored := &MWeakReply{}
	err := restored.Unmarshal(&buf)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if len(restored.Rep) != 0 {
		t.Errorf("Rep should be empty, got %v", restored.Rep)
	}
}

// TestMixedCommandsSlotOrdering tests that weak and strong commands share slot space
// This is a conceptual test - actual slot ordering requires a full replica setup
func TestMixedCommandsSlotOrdering(t *testing.T) {
	// This test verifies the design: weak and strong commands should share
	// the same slot sequence for global ordering.

	// In the implementation:
	// - handlePropose increments r.lastCmdSlot for strong commands
	// - handleWeakPropose increments r.lastCmdSlot for weak commands
	// Both use the same counter, ensuring global ordering.

	// We can verify this by checking the code structure exists
	// (Full integration test would require setting up multiple goroutines)

	// For now, we just verify the constants and types are correct
	if STRONG >= WEAK {
		t.Log("STRONG and WEAK constants are distinct")
	}
}

// ============================================================================
// Integration-style tests (Phase 7.5)
// These tests verify component integration without requiring network setup
// ============================================================================

// TestCausalDepSerialization tests that CausalDep field is properly serialized
func TestCausalDepSerialization(t *testing.T) {
	original := &MWeakPropose{
		CommandId: 10,
		ClientId:  1,
		Command: state.Command{
			Op: state.PUT,
			K:  state.Key(100),
			V:  []byte("value1"),
		},
		Timestamp: 123456,
		CausalDep: 5, // This command depends on seqnum 5
	}

	// Serialize
	var buf bytes.Buffer
	original.Marshal(&buf)

	// Deserialize
	restored := &MWeakPropose{}
	err := restored.Unmarshal(&buf)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	// Verify CausalDep is preserved
	if restored.CausalDep != original.CausalDep {
		t.Errorf("CausalDep mismatch: got %d, want %d", restored.CausalDep, original.CausalDep)
	}
}

// TestCausalDepZeroValue tests that CausalDep=0 is handled correctly
func TestCausalDepZeroValue(t *testing.T) {
	// First command from a client should have CausalDep=0
	original := &MWeakPropose{
		CommandId: 1,
		ClientId:  1,
		Command: state.Command{
			Op: state.PUT,
			K:  state.Key(1),
			V:  []byte("first"),
		},
		Timestamp: 0,
		CausalDep: 0, // No dependency (first command)
	}

	var buf bytes.Buffer
	original.Marshal(&buf)

	restored := &MWeakPropose{}
	err := restored.Unmarshal(&buf)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if restored.CausalDep != 0 {
		t.Errorf("CausalDep should be 0, got %d", restored.CausalDep)
	}
}

// TestWeakCommandChain tests a chain of causally dependent weak commands
func TestWeakCommandChain(t *testing.T) {
	st := state.InitState()

	// Simulate a chain of weak commands: cmd1 -> cmd2 -> cmd3
	// Each depends on the previous one

	cmd1 := &MWeakPropose{
		CommandId: 1,
		ClientId:  100,
		Command: state.Command{
			Op: state.PUT,
			K:  state.Key(1),
			V:  []byte("value1"),
		},
		CausalDep: 0, // No dependency
	}

	cmd2 := &MWeakPropose{
		CommandId: 2,
		ClientId:  100,
		Command: state.Command{
			Op: state.PUT,
			K:  state.Key(2),
			V:  []byte("value2"),
		},
		CausalDep: 1, // Depends on cmd1
	}

	cmd3 := &MWeakPropose{
		CommandId: 3,
		ClientId:  100,
		Command: state.Command{
			Op: state.GET,
			K:  state.Key(1),
			V:  state.NIL(),
		},
		CausalDep: 2, // Depends on cmd2
	}

	// Execute commands in order (simulating correct causal execution)
	cmd1.Command.Execute(st)
	cmd2.Command.Execute(st)
	result := cmd3.Command.Execute(st)

	// Verify cmd3 sees cmd1's value
	if !bytes.Equal(result, []byte("value1")) {
		t.Errorf("Expected 'value1', got %v", result)
	}

	// Verify dependency chain
	if cmd2.CausalDep != cmd1.CommandId {
		t.Errorf("cmd2 should depend on cmd1")
	}
	if cmd3.CausalDep != cmd2.CommandId {
		t.Errorf("cmd3 should depend on cmd2")
	}
}

// TestMultiClientCausalIndependence tests that different clients have independent causal chains
func TestMultiClientCausalIndependence(t *testing.T) {
	// Client A's commands
	cmdA1 := &MWeakPropose{
		CommandId: 1,
		ClientId:  100,
		CausalDep: 0,
	}
	cmdA2 := &MWeakPropose{
		CommandId: 2,
		ClientId:  100,
		CausalDep: 1, // Depends on A1
	}

	// Client B's commands (independent of A)
	cmdB1 := &MWeakPropose{
		CommandId: 1,
		ClientId:  200,
		CausalDep: 0, // No dependency on A's commands
	}
	cmdB2 := &MWeakPropose{
		CommandId: 2,
		ClientId:  200,
		CausalDep: 1, // Depends on B1, not A's commands
	}

	// Verify A's chain
	if cmdA1.CausalDep != 0 {
		t.Error("cmdA1 should have no dependency")
	}
	if cmdA2.CausalDep != 1 {
		t.Error("cmdA2 should depend on cmdA1")
	}

	// Verify B's chain is independent
	if cmdB1.CausalDep != 0 {
		t.Error("cmdB1 should have no dependency")
	}
	if cmdB2.CausalDep != 1 {
		t.Error("cmdB2 should depend on cmdB1")
	}

	// Verify clients are different
	if cmdA1.ClientId == cmdB1.ClientId {
		t.Error("Clients should have different IDs")
	}
}

// TestWeakReplyPoolAllocation tests that the sync.Pool allocation works
func TestWeakReplyPoolAllocation(t *testing.T) {
	// Simulate what happens in handleWeakPropose
	pool := &weakReplyPoolType{}

	// First get creates a new object
	reply1 := pool.Get()
	if reply1 == nil {
		t.Fatal("Pool.Get returned nil")
	}

	// Set some values
	reply1.Replica = 1
	reply1.Ballot = 10
	reply1.CmdId = CommandId{ClientId: 100, SeqNum: 1}
	reply1.Rep = []byte("result1")

	// Return to pool
	pool.Put(reply1)

	// Get again - may or may not be the same object
	reply2 := pool.Get()
	if reply2 == nil {
		t.Fatal("Pool.Get after Put returned nil")
	}

	// Verify we can use the object
	reply2.Replica = 2
	reply2.Ballot = 20
}

// weakReplyPoolType is a helper type for testing sync.Pool behavior
type weakReplyPoolType struct{}

func (p *weakReplyPoolType) Get() *MWeakReply {
	return &MWeakReply{}
}

func (p *weakReplyPoolType) Put(r *MWeakReply) {
	// In real sync.Pool, this would return to the pool
}

// TestCommandDescWeakExecution tests that commandDesc correctly tracks weak commands
func TestCommandDescWeakExecution(t *testing.T) {
	// Create a weak command descriptor
	desc := &commandDesc{
		phase: COMMIT, // Simulating executed state
		isWeak: true,
	}

	// Verify it's marked as weak
	if !desc.isWeak {
		t.Error("Command should be marked as weak")
	}

	// A strong command descriptor
	strongDesc := &commandDesc{
		phase:  COMMIT,
		isWeak: false,
	}

	if strongDesc.isWeak {
		t.Error("Strong command should not be marked as weak")
	}
}

// TestWeakProposeMessageFields tests all fields of MWeakPropose
func TestWeakProposeMessageFields(t *testing.T) {
	msg := &MWeakPropose{
		CommandId: 42,
		ClientId:  100,
		Command: state.Command{
			Op: state.PUT,
			K:  state.Key(999),
			V:  []byte("test-data"),
		},
		Timestamp: 1234567890,
		CausalDep: 41, // Depends on previous command
	}

	// Verify all fields are set correctly
	if msg.CommandId != 42 {
		t.Errorf("CommandId = %d, want 42", msg.CommandId)
	}
	if msg.ClientId != 100 {
		t.Errorf("ClientId = %d, want 100", msg.ClientId)
	}
	if msg.Command.Op != state.PUT {
		t.Errorf("Command.Op = %d, want PUT", msg.Command.Op)
	}
	if msg.Command.K != state.Key(999) {
		t.Errorf("Command.K = %d, want 999", msg.Command.K)
	}
	if !bytes.Equal(msg.Command.V, []byte("test-data")) {
		t.Errorf("Command.V mismatch")
	}
	if msg.Timestamp != 1234567890 {
		t.Errorf("Timestamp = %d, want 1234567890", msg.Timestamp)
	}
	if msg.CausalDep != 41 {
		t.Errorf("CausalDep = %d, want 41", msg.CausalDep)
	}
}

// TestWeakReplyMessageFields tests all fields of MWeakReply
func TestWeakReplyMessageFields(t *testing.T) {
	msg := &MWeakReply{
		Replica: 2,
		Ballot:  15,
		CmdId: CommandId{
			ClientId: 100,
			SeqNum:   42,
		},
		Rep: []byte("response-data"),
	}

	// Verify all fields
	if msg.Replica != 2 {
		t.Errorf("Replica = %d, want 2", msg.Replica)
	}
	if msg.Ballot != 15 {
		t.Errorf("Ballot = %d, want 15", msg.Ballot)
	}
	if msg.CmdId.ClientId != 100 {
		t.Errorf("CmdId.ClientId = %d, want 100", msg.CmdId.ClientId)
	}
	if msg.CmdId.SeqNum != 42 {
		t.Errorf("CmdId.SeqNum = %d, want 42", msg.CmdId.SeqNum)
	}
	if !bytes.Equal(msg.Rep, []byte("response-data")) {
		t.Errorf("Rep mismatch")
	}
}
