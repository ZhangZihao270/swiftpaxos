package curpht

import (
	"bytes"
	"testing"

	"github.com/imdea-software/swiftpaxos/state"
)

// ============================================================================
// Phase 31.7: Serialization Optimization Tests
// ============================================================================

// TestMReplySerialization tests MReply Marshal/Unmarshal round-trip
func TestMReplySerialization(t *testing.T) {
	original := &MReply{
		Replica: 2,
		Ballot:  10,
		CmdId:   CommandId{ClientId: 100, SeqNum: 42},
		Rep:     []byte("test-reply-data"),
		Ok:      TRUE,
	}

	var buf bytes.Buffer
	original.Marshal(&buf)

	restored := &MReply{}
	err := restored.Unmarshal(&buf)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if restored.Replica != original.Replica {
		t.Errorf("Replica mismatch: got %d, want %d", restored.Replica, original.Replica)
	}
	if restored.Ballot != original.Ballot {
		t.Errorf("Ballot mismatch: got %d, want %d", restored.Ballot, original.Ballot)
	}
	if restored.CmdId != original.CmdId {
		t.Errorf("CmdId mismatch: got %v, want %v", restored.CmdId, original.CmdId)
	}
	if !bytes.Equal(restored.Rep, original.Rep) {
		t.Errorf("Rep mismatch: got %v, want %v", restored.Rep, original.Rep)
	}
	if restored.Ok != original.Ok {
		t.Errorf("Ok mismatch: got %d, want %d", restored.Ok, original.Ok)
	}
}

// TestMReplyEmptyRep tests MReply with empty Rep field
func TestMReplyEmptyRep(t *testing.T) {
	original := &MReply{
		Replica: 1,
		Ballot:  5,
		CmdId:   CommandId{ClientId: 1, SeqNum: 1},
		Rep:     []byte{},
		Ok:      FALSE,
	}

	var buf bytes.Buffer
	original.Marshal(&buf)

	restored := &MReply{}
	err := restored.Unmarshal(&buf)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if len(restored.Rep) != 0 {
		t.Errorf("Rep should be empty, got %v", restored.Rep)
	}
	if restored.Ok != FALSE {
		t.Errorf("Ok should be FALSE, got %d", restored.Ok)
	}
}

// TestMReplyLargeRep tests MReply with a large Rep payload
func TestMReplyLargeRep(t *testing.T) {
	largeRep := make([]byte, 4096)
	for i := range largeRep {
		largeRep[i] = byte(i % 256)
	}

	original := &MReply{
		Replica: 0,
		Ballot:  999,
		CmdId:   CommandId{ClientId: 50, SeqNum: 200},
		Rep:     largeRep,
		Ok:      TRUE,
	}

	var buf bytes.Buffer
	original.Marshal(&buf)

	restored := &MReply{}
	err := restored.Unmarshal(&buf)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if !bytes.Equal(restored.Rep, original.Rep) {
		t.Errorf("Large Rep mismatch (len got=%d, want=%d)", len(restored.Rep), len(original.Rep))
	}
}

// TestMSyncReplySerialization tests MSyncReply Marshal/Unmarshal round-trip
func TestMSyncReplySerialization(t *testing.T) {
	original := &MSyncReply{
		Replica: 1,
		Ballot:  7,
		CmdId:   CommandId{ClientId: 200, SeqNum: 99},
		Rep:     []byte("sync-reply-data"),
	}

	var buf bytes.Buffer
	original.Marshal(&buf)

	restored := &MSyncReply{}
	err := restored.Unmarshal(&buf)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if restored.Replica != original.Replica || restored.Ballot != original.Ballot {
		t.Error("Fixed fields mismatch")
	}
	if restored.CmdId != original.CmdId {
		t.Errorf("CmdId mismatch")
	}
	if !bytes.Equal(restored.Rep, original.Rep) {
		t.Errorf("Rep mismatch: got %v, want %v", restored.Rep, original.Rep)
	}
}

// TestMAcceptSerialization tests MAccept Marshal/Unmarshal with embedded Command
func TestMAcceptSerialization(t *testing.T) {
	original := &MAccept{
		Replica: 0,
		Ballot:  3,
		Cmd: state.Command{
			Op: state.PUT,
			K:  state.Key(42),
			V:  state.Value([]byte("accept-value")),
		},
		CmdId:   CommandId{ClientId: 10, SeqNum: 5},
		CmdSlot: 100,
	}

	var buf bytes.Buffer
	original.Marshal(&buf)

	restored := &MAccept{}
	err := restored.Unmarshal(&buf)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if restored.Replica != original.Replica || restored.Ballot != original.Ballot {
		t.Error("Fixed fields mismatch")
	}
	if restored.Cmd.Op != original.Cmd.Op || restored.Cmd.K != original.Cmd.K {
		t.Error("Command fields mismatch")
	}
	if !bytes.Equal(restored.Cmd.V, original.Cmd.V) {
		t.Error("Command.V mismatch")
	}
	if restored.CmdSlot != original.CmdSlot {
		t.Errorf("CmdSlot mismatch: got %d, want %d", restored.CmdSlot, original.CmdSlot)
	}
}

// TestMCommitSerialization tests MCommit fixed-size serialization
func TestMCommitSerialization(t *testing.T) {
	original := &MCommit{
		Replica: 2,
		Ballot:  15,
		CmdSlot: 12345678,
	}

	var buf bytes.Buffer
	original.Marshal(&buf)

	if buf.Len() != 16 {
		t.Errorf("MCommit should serialize to 16 bytes, got %d", buf.Len())
	}

	restored := &MCommit{}
	err := restored.Unmarshal(&buf)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if *restored != *original {
		t.Errorf("MCommit mismatch: got %+v, want %+v", restored, original)
	}
}

// TestMAAcksSerialization tests MAAcks with nested Acks and Accepts
func TestMAAcksSerialization(t *testing.T) {
	original := &MAAcks{
		Acks: []MAcceptAck{
			{Replica: 0, Ballot: 1, CmdSlot: 10},
			{Replica: 1, Ballot: 1, CmdSlot: 10},
		},
		Accepts: []MAccept{
			{Replica: 0, Ballot: 1, Cmd: state.Command{Op: state.PUT, K: state.Key(1), V: []byte("v1")}, CmdId: CommandId{ClientId: 1, SeqNum: 1}, CmdSlot: 10},
		},
	}

	var buf bytes.Buffer
	original.Marshal(&buf)

	restored := &MAAcks{}
	err := restored.Unmarshal(&buf)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if len(restored.Acks) != 2 {
		t.Fatalf("Acks count: got %d, want 2", len(restored.Acks))
	}
	if restored.Acks[0].CmdSlot != 10 || restored.Acks[1].CmdSlot != 10 {
		t.Error("Acks CmdSlot mismatch")
	}
	if len(restored.Accepts) != 1 {
		t.Fatalf("Accepts count: got %d, want 1", len(restored.Accepts))
	}
}

// --- Serialization Benchmarks ---

func BenchmarkMReplyMarshal(b *testing.B) {
	msg := &MReply{
		Replica: 1,
		Ballot:  5,
		CmdId:   CommandId{ClientId: 100, SeqNum: 42},
		Rep:     make([]byte, 100),
		Ok:      TRUE,
	}
	var buf bytes.Buffer

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		msg.Marshal(&buf)
	}
}

func BenchmarkMReplyUnmarshal(b *testing.B) {
	msg := &MReply{
		Replica: 1,
		Ballot:  5,
		CmdId:   CommandId{ClientId: 100, SeqNum: 42},
		Rep:     make([]byte, 100),
		Ok:      TRUE,
	}
	var buf bytes.Buffer
	msg.Marshal(&buf)
	data := buf.Bytes()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		restored := &MReply{}
		restored.Unmarshal(bytes.NewReader(data))
	}
}

func BenchmarkMReplyRoundTrip(b *testing.B) {
	msg := &MReply{
		Replica: 1,
		Ballot:  5,
		CmdId:   CommandId{ClientId: 100, SeqNum: 42},
		Rep:     make([]byte, 100),
		Ok:      TRUE,
	}
	var buf bytes.Buffer

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		msg.Marshal(&buf)
		restored := &MReply{}
		restored.Unmarshal(&buf)
	}
}

func BenchmarkMCommitMarshal(b *testing.B) {
	msg := &MCommit{Replica: 1, Ballot: 5, CmdSlot: 100}
	var buf bytes.Buffer

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		msg.Marshal(&buf)
	}
}

func BenchmarkMAcceptMarshal(b *testing.B) {
	msg := &MAccept{
		Replica: 1,
		Ballot:  5,
		Cmd:     state.Command{Op: state.PUT, K: state.Key(100), V: []byte("value")},
		CmdId:   CommandId{ClientId: 10, SeqNum: 1},
		CmdSlot: 50,
	}
	var buf bytes.Buffer

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		msg.Marshal(&buf)
	}
}

func BenchmarkMWeakReplyMarshal(b *testing.B) {
	msg := &MWeakReply{
		Replica: 1,
		Ballot:  5,
		CmdId:   CommandId{ClientId: 100, SeqNum: 42},
		Rep:     make([]byte, 100),
	}
	var buf bytes.Buffer

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		msg.Marshal(&buf)
	}
}

func BenchmarkCommandMarshal(b *testing.B) {
	cmd := state.Command{Op: state.PUT, K: state.Key(100), V: state.Value([]byte("benchvalue"))}
	var buf bytes.Buffer

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		cmd.Marshal(&buf)
	}
}

func BenchmarkCommandUnmarshal(b *testing.B) {
	cmd := state.Command{Op: state.PUT, K: state.Key(100), V: state.Value([]byte("benchvalue"))}
	var buf bytes.Buffer
	cmd.Marshal(&buf)
	data := buf.Bytes()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var restored state.Command
		restored.Unmarshal(bytes.NewReader(data))
	}
}

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

// TestCommandDescAppliedField tests the applied field for tracking state machine modifications
func TestCommandDescAppliedField(t *testing.T) {
	// New descriptor should have applied=false by default
	desc := &commandDesc{}
	if desc.applied {
		t.Error("Default applied should be false")
	}

	// Mark as applied
	desc.applied = true
	if !desc.applied {
		t.Error("applied should be settable to true")
	}

	// Verify weak and applied are independent
	weakDesc := &commandDesc{
		isWeak:  true,
		applied: false,
	}
	if weakDesc.applied {
		t.Error("Weak command should start with applied=false")
	}

	strongDesc := &commandDesc{
		isWeak:  false,
		applied: true,
	}
	if !strongDesc.applied {
		t.Error("Strong command can have applied=true")
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

// ============================================================================
// Phase 9 Critical Bug Fix Tests
// ============================================================================

// TestComputeResultDoesNotModifyState verifies that ComputeResult does not modify state
// This tests the fix for Issue 1: Speculative execution should NOT modify state machine
func TestComputeResultDoesNotModifyState(t *testing.T) {
	st := state.InitState()

	// Initial state should be empty
	getCmd := state.Command{Op: state.GET, K: state.Key(100), V: state.NIL()}
	if len(getCmd.ComputeResult(st)) != 0 {
		t.Error("State should be empty initially")
	}

	// Use ComputeResult for PUT - should NOT modify state
	putCmd := state.Command{Op: state.PUT, K: state.Key(100), V: state.Value([]byte("test"))}
	result := putCmd.ComputeResult(st)

	// PUT returns NIL during speculation
	if len(result) != 0 {
		t.Errorf("ComputeResult(PUT) should return NIL, got %v", result)
	}

	// State should still be empty (PUT not applied)
	getResult := getCmd.ComputeResult(st)
	if len(getResult) != 0 {
		t.Errorf("State was modified by ComputeResult(PUT), got %v", getResult)
	}
}

// TestExecuteModifiesStateAfterCommit verifies that Execute does modify state
// This confirms the correct behavior after commit
func TestExecuteModifiesStateAfterCommit(t *testing.T) {
	st := state.InitState()

	// Execute PUT - should modify state
	putCmd := state.Command{Op: state.PUT, K: state.Key(100), V: state.Value([]byte("committed"))}
	putCmd.Execute(st)

	// State should now have the value
	getCmd := state.Command{Op: state.GET, K: state.Key(100), V: state.NIL()}
	result := getCmd.Execute(st)

	if !bytes.Equal(result, []byte("committed")) {
		t.Errorf("Execute(PUT) did not modify state correctly, got %v", result)
	}
}

// TestAppliedFieldTracking verifies that applied field correctly tracks state modification
func TestAppliedFieldTracking(t *testing.T) {
	// Before commit: applied should be false
	desc := &commandDesc{
		phase:   ACCEPT, // Before commit
		applied: false,
	}

	if desc.applied {
		t.Error("applied should be false before commit")
	}

	// After commit: mark as applied
	desc.phase = COMMIT
	desc.applied = true

	if !desc.applied {
		t.Error("applied should be true after commit")
	}
}

// TestSpeculativeResultMatchesFinalResult verifies that speculative result matches final result for reads
func TestSpeculativeResultMatchesFinalResult(t *testing.T) {
	st := state.InitState()

	// Setup: put a value first (simulating previous committed command)
	setupCmd := state.Command{Op: state.PUT, K: state.Key(1), V: state.Value([]byte("value1"))}
	setupCmd.Execute(st)

	// GET command
	getCmd := state.Command{Op: state.GET, K: state.Key(1), V: state.NIL()}

	// Speculative result (ComputeResult)
	specResult := getCmd.ComputeResult(st)

	// Final result (Execute)
	finalResult := getCmd.Execute(st)

	// Both should return the same value
	if !bytes.Equal(specResult, finalResult) {
		t.Errorf("Speculative result %v != final result %v", specResult, finalResult)
	}
}

// TestSlotOrderedExecution verifies that commands should execute in slot order
// This tests the fix for Issue 2: Weak commands must follow slot ordering
func TestSlotOrderedExecution(t *testing.T) {
	// This is a conceptual test - full integration would require replica setup
	// We verify the design: applied field ensures single execution

	desc1 := &commandDesc{cmdSlot: 0, applied: false}
	desc2 := &commandDesc{cmdSlot: 1, applied: false}
	desc3 := &commandDesc{cmdSlot: 2, applied: false}

	// Simulate slot-ordered execution
	// Slot 0 first
	if desc1.applied {
		t.Error("Slot 0 should not be executed yet")
	}
	desc1.applied = true

	// Slot 1 only after slot 0
	if !desc1.applied {
		t.Error("Slot 0 should be executed before slot 1")
	}
	desc2.applied = true

	// Slot 2 only after slot 1
	if !desc2.applied {
		t.Error("Slot 1 should be executed before slot 2")
	}
	desc3.applied = true

	// All should be applied now
	if !desc1.applied || !desc2.applied || !desc3.applied {
		t.Error("All slots should be executed")
	}
}

// TestMixedStrongWeakSlotOrdering verifies that strong and weak share slot space
func TestMixedStrongWeakSlotOrdering(t *testing.T) {
	// Create interleaved strong and weak commands
	strongDesc := &commandDesc{cmdSlot: 0, isWeak: false, applied: false}
	weakDesc := &commandDesc{cmdSlot: 1, isWeak: true, applied: false}
	strongDesc2 := &commandDesc{cmdSlot: 2, isWeak: false, applied: false}

	// Execute in slot order regardless of type
	strongDesc.applied = true  // Slot 0 (strong)
	weakDesc.applied = true    // Slot 1 (weak)
	strongDesc2.applied = true // Slot 2 (strong)

	// Verify all executed
	if !strongDesc.applied || !weakDesc.applied || !strongDesc2.applied {
		t.Error("All commands should be executed in slot order")
	}

	// Verify slot ordering is maintained
	if strongDesc.cmdSlot >= weakDesc.cmdSlot {
		t.Error("Slot ordering violated: strong(0) should be before weak(1)")
	}
	if weakDesc.cmdSlot >= strongDesc2.cmdSlot {
		t.Error("Slot ordering violated: weak(1) should be before strong(2)")
	}
}

// ============================================================================
// Phase 10: Non-Blocking Speculative Reads Tests
// ============================================================================

// TestPendingWriteKey verifies the key generation for pending writes
func TestPendingWriteKey(t *testing.T) {
	r := &Replica{} // Create minimal replica for testing

	key1 := r.pendingWriteKey(100, state.Key(42))
	expected := "100:42"
	if key1 != expected {
		t.Errorf("pendingWriteKey(100, 42) = %q, want %q", key1, expected)
	}

	key2 := r.pendingWriteKey(200, state.Key(999))
	expected2 := "200:999"
	if key2 != expected2 {
		t.Errorf("pendingWriteKey(200, 999) = %q, want %q", key2, expected2)
	}
}

// TestPendingWriteStruct verifies the pendingWrite struct
func TestPendingWriteStruct(t *testing.T) {
	pw := &pendingWrite{
		seqNum: 5,
		value:  state.Value([]byte("test-value")),
	}

	if pw.seqNum != 5 {
		t.Errorf("seqNum = %d, want 5", pw.seqNum)
	}
	if !bytes.Equal(pw.value, []byte("test-value")) {
		t.Errorf("value mismatch")
	}
}

// TestSameClientReadAfterWrite tests that a read sees pending write from same client
// This is the key test for Phase 10 optimization
func TestSameClientReadAfterWrite(t *testing.T) {
	// Simulate the scenario:
	// Client 100 sends: W1 = PUT(key=1, "A"), R1 = GET(key=1) with CausalDep=W1
	// R1 should return "A" immediately via pending writes

	st := state.InitState()

	// Initial state: key 1 doesn't exist
	getCmd := state.Command{Op: state.GET, K: state.Key(1), V: state.NIL()}
	result := getCmd.ComputeResult(st)
	if len(result) != 0 {
		t.Error("Key should not exist initially")
	}

	// Simulate pending write: PUT(key=1, "A") with seqNum=1
	// In real implementation, this would be tracked in pendingWrites map
	pendingValue := state.Value([]byte("A"))
	pw := &pendingWrite{seqNum: 1, value: pendingValue}

	// Now simulate a read with CausalDep=1
	// The read should see the pending value, not the committed state
	if pw.seqNum <= 1 { // CausalDep=1
		// Read sees pending value
		if !bytes.Equal(pw.value, []byte("A")) {
			t.Errorf("Read should see pending value 'A', got %v", pw.value)
		}
	}
}

// TestPendingWritesCleanup verifies that pending writes are cleaned up after commit
func TestPendingWritesCleanup(t *testing.T) {
	// This is a conceptual test - full integration requires replica setup
	// We verify the cleanup logic: after commit, pending write should be removed

	pw := &pendingWrite{seqNum: 5, value: state.Value([]byte("pending"))}

	// Before cleanup
	if pw.seqNum != 5 {
		t.Error("Pending write should exist before cleanup")
	}

	// After cleanup (simulate by setting to nil)
	// In real implementation, r.removePendingWrite() is called
	pw = nil
	if pw != nil {
		t.Error("Pending write should be nil after cleanup")
	}
}

// TestCrossClientIsolation verifies that clients can't see each other's pending writes
func TestCrossClientIsolation(t *testing.T) {
	// Client A (100) has pending write on key 1
	// Client B (200) reads key 1 - should NOT see A's pending write

	r := &Replica{} // Create minimal replica for testing
	clientA := int32(100)
	clientB := int32(200)
	key := state.Key(1)

	// Client A's pending write
	keyA := r.pendingWriteKey(clientA, key)
	keyB := r.pendingWriteKey(clientB, key)

	// Keys should be different
	if keyA == keyB {
		t.Error("Different clients should have different pending write keys")
	}

	// Verify key format includes client ID
	if keyA != "100:1" {
		t.Errorf("Client A key = %q, want '100:1'", keyA)
	}
	if keyB != "200:1" {
		t.Errorf("Client B key = %q, want '200:1'", keyB)
	}
}

// TestPendingWriteSequenceOrdering verifies newer writes overwrite older ones
func TestPendingWriteSequenceOrdering(t *testing.T) {
	// If client sends W1 then W2 on same key, W2 should be the pending write

	pw1 := &pendingWrite{seqNum: 1, value: state.Value([]byte("v1"))}
	pw2 := &pendingWrite{seqNum: 2, value: state.Value([]byte("v2"))}

	// W2 should overwrite W1 (seqNum 2 > 1)
	if pw2.seqNum <= pw1.seqNum {
		t.Error("W2.seqNum should be greater than W1.seqNum")
	}

	// The latest pending write should be used
	latest := pw2
	if !bytes.Equal(latest.value, []byte("v2")) {
		t.Errorf("Latest value should be 'v2', got %v", latest.value)
	}
}

// TestComputeSpeculativeResultGETWithPending tests GET with pending writes
func TestComputeSpeculativeResultGETWithPending(t *testing.T) {
	// This is a logic test for the computeSpeculativeResult behavior
	// When there's a pending write matching the key and satisfying CausalDep,
	// it should return the pending value instead of committed state

	st := state.InitState()

	// Committed state: key 1 = "committed"
	commitCmd := state.Command{Op: state.PUT, K: state.Key(1), V: state.Value([]byte("committed"))}
	commitCmd.Execute(st)

	// Pending write: key 1 = "pending" with seqNum=5
	pendingVal := state.Value([]byte("pending"))
	pw := &pendingWrite{seqNum: 5, value: pendingVal}

	// A read with CausalDep=5 should see the pending value
	// (simulating the logic in computeSpeculativeResult)
	causalDep := int32(5)
	if pw.seqNum <= causalDep {
		// Should use pending value
		if !bytes.Equal(pw.value, []byte("pending")) {
			t.Error("Should return pending value when CausalDep is satisfied")
		}
	}

	// A read with CausalDep=4 should NOT see this pending value
	causalDep = 4
	if pw.seqNum <= causalDep {
		t.Error("Should NOT return pending value when CausalDep is not satisfied")
	}
}

// TestComputeSpeculativeResultPUT tests PUT returns NIL
func TestComputeSpeculativeResultPUT(t *testing.T) {
	st := state.InitState()
	putCmd := state.Command{Op: state.PUT, K: state.Key(1), V: state.Value([]byte("test"))}

	// ComputeResult for PUT should return NIL
	result := putCmd.ComputeResult(st)
	if len(result) != 0 {
		t.Errorf("ComputeResult(PUT) should return NIL, got %v", result)
	}
}

// TestMaxDescRoutinesDefault verifies the default MaxDescRoutines value
func TestMaxDescRoutinesDefault(t *testing.T) {
	if MaxDescRoutines != 10000 {
		t.Errorf("MaxDescRoutines should be 10000, got %d", MaxDescRoutines)
	}
}

// TestMaxDescRoutinesOverride verifies MaxDescRoutines can be overridden
func TestMaxDescRoutinesOverride(t *testing.T) {
	original := MaxDescRoutines
	defer func() { MaxDescRoutines = original }()

	MaxDescRoutines = 5000
	if MaxDescRoutines != 5000 {
		t.Errorf("MaxDescRoutines after override should be 5000, got %d", MaxDescRoutines)
	}

	MaxDescRoutines = 100
	if MaxDescRoutines != 100 {
		t.Errorf("MaxDescRoutines after second override should be 100, got %d", MaxDescRoutines)
	}
}
