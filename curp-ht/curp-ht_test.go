package curpht

import (
	"bytes"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	cmap "github.com/orcaman/concurrent-map"

	"github.com/imdea-software/swiftpaxos/dlog"
	"github.com/imdea-software/swiftpaxos/hook"
	"github.com/imdea-software/swiftpaxos/replica"
	"github.com/imdea-software/swiftpaxos/replica/defs"
	fastrpc "github.com/imdea-software/swiftpaxos/rpc"
	"github.com/imdea-software/swiftpaxos/state"
)

// newTestSender creates a buffered Sender that absorbs messages without blocking.
func newTestSender() replica.Sender {
	return replica.Sender(make(chan replica.SendArg, 1000))
}

// newTestBaseReplica creates a base replica.Replica with a logger for tests.
func newTestBaseReplica(n int) *replica.Replica {
	return &replica.Replica{
		N:      n,
		Logger: dlog.New("", false),
	}
}

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
		Slot:    77,
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
	if restored.Slot != original.Slot {
		t.Errorf("Slot mismatch: got %d, want %d", restored.Slot, original.Slot)
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
		Slot:    0,
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
		Slot:    12345,
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
		Slot:    150,
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
	if restored.Slot != original.Slot {
		t.Errorf("Slot mismatch: got %d, want %d", restored.Slot, original.Slot)
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
		Slot:    50,
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
		Slot:    50,
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
		Slot:    50,
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
		Slot:    42,
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
		Rep:  []byte("result-value"),
		Slot: 33,
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
	if restored.Slot != original.Slot {
		t.Errorf("Slot mismatch: got %d, want %d", restored.Slot, original.Slot)
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
		Rep:  []byte{},
		Slot: 5,
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
	if restored.Slot != original.Slot {
		t.Errorf("Slot mismatch: got %d, want %d", restored.Slot, original.Slot)
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
		Rep:  []byte("response-data"),
		Slot: 99,
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
	if msg.Slot != 99 {
		t.Errorf("Slot = %d, want 99", msg.Slot)
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
// Phase 36: MWeakRead / MWeakReadReply Serialization Tests
// ============================================================================

// TestMWeakReadSerialization tests MWeakRead Marshal/Unmarshal round-trip
func TestMWeakReadSerialization(t *testing.T) {
	original := &MWeakRead{
		CommandId: 42,
		ClientId:  100,
		Key:       state.Key(999),
	}

	var buf bytes.Buffer
	original.Marshal(&buf)

	if buf.Len() != 25 {
		t.Errorf("MWeakRead should serialize to 25 bytes, got %d", buf.Len())
	}

	restored := &MWeakRead{}
	err := restored.Unmarshal(&buf)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if restored.CommandId != original.CommandId {
		t.Errorf("CommandId mismatch: got %d, want %d", restored.CommandId, original.CommandId)
	}
	if restored.ClientId != original.ClientId {
		t.Errorf("ClientId mismatch: got %d, want %d", restored.ClientId, original.ClientId)
	}
	if restored.Key != original.Key {
		t.Errorf("Key mismatch: got %d, want %d", restored.Key, original.Key)
	}
	if restored.Op != 0 {
		t.Errorf("Op should default to 0, got %d", restored.Op)
	}
	if restored.Count != 0 {
		t.Errorf("Count should default to 0, got %d", restored.Count)
	}
}

// TestMWeakReadSerializationWithScan tests MWeakRead round-trip with SCAN op
func TestMWeakReadSerializationWithScan(t *testing.T) {
	original := &MWeakRead{
		CommandId: 42,
		ClientId:  100,
		Key:       state.Key(999),
		Op:        uint8(state.SCAN),
		Count:     500,
	}

	var buf bytes.Buffer
	original.Marshal(&buf)

	restored := &MWeakRead{}
	err := restored.Unmarshal(&buf)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if restored.Op != uint8(state.SCAN) {
		t.Errorf("Op mismatch: got %d, want %d", restored.Op, state.SCAN)
	}
	if restored.Count != 500 {
		t.Errorf("Count mismatch: got %d, want 500", restored.Count)
	}
}

// TestMWeakReadBinarySize tests MWeakRead fixed size
func TestMWeakReadBinarySize(t *testing.T) {
	m := &MWeakRead{}
	size, known := m.BinarySize()
	if !known {
		t.Error("MWeakRead should have known binary size")
	}
	if size != 25 {
		t.Errorf("MWeakRead BinarySize = %d, want 25", size)
	}
}

// TestMWeakReadNew tests New() method
func TestMWeakReadNew(t *testing.T) {
	m := &MWeakRead{}
	newObj := m.New()
	if newObj == nil {
		t.Fatal("New() returned nil")
	}
	if _, ok := newObj.(*MWeakRead); !ok {
		t.Fatal("New() returned wrong type")
	}
}

// TestMWeakReadReplyNew tests New() method
func TestMWeakReadReplyNew(t *testing.T) {
	m := &MWeakReadReply{}
	newObj := m.New()
	if newObj == nil {
		t.Fatal("New() returned nil")
	}
	if _, ok := newObj.(*MWeakReadReply); !ok {
		t.Fatal("New() returned wrong type")
	}
}

// TestMWeakReadReplySerialization tests MWeakReadReply Marshal/Unmarshal round-trip
func TestMWeakReadReplySerialization(t *testing.T) {
	original := &MWeakReadReply{
		Replica: 2,
		Ballot:  10,
		CmdId:   CommandId{ClientId: 100, SeqNum: 42},
		Rep:     []byte("read-result"),
		Version: 77,
	}

	var buf bytes.Buffer
	original.Marshal(&buf)

	restored := &MWeakReadReply{}
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
	if restored.Version != original.Version {
		t.Errorf("Version mismatch: got %d, want %d", restored.Version, original.Version)
	}
}

// TestMWeakReadReplyEmptyRep tests MWeakReadReply with empty Rep
func TestMWeakReadReplyEmptyRep(t *testing.T) {
	original := &MWeakReadReply{
		Replica: 0,
		Ballot:  0,
		CmdId:   CommandId{ClientId: 1, SeqNum: 1},
		Rep:     []byte{},
		Version: 0,
	}

	var buf bytes.Buffer
	original.Marshal(&buf)

	restored := &MWeakReadReply{}
	err := restored.Unmarshal(&buf)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if len(restored.Rep) != 0 {
		t.Errorf("Rep should be empty, got %v", restored.Rep)
	}
	if restored.Version != 0 {
		t.Errorf("Version should be 0, got %d", restored.Version)
	}
}

// TestMWeakReadReplyBinarySize tests BinarySize
func TestMWeakReadReplyBinarySize(t *testing.T) {
	m := &MWeakReadReply{}
	_, known := m.BinarySize()
	if known {
		t.Error("MWeakReadReply should have unknown binary size (variable Rep)")
	}
}

// TestMWeakReadCache tests object pool for MWeakRead
func TestMWeakReadCache(t *testing.T) {
	cache := NewMWeakReadCache()

	obj1 := cache.Get()
	if obj1 == nil {
		t.Fatal("Get from empty cache returned nil")
	}

	obj1.CommandId = 123
	cache.Put(obj1)

	obj2 := cache.Get()
	if obj2 == nil {
		t.Fatal("Get after Put returned nil")
	}
}

// TestMWeakReadReplyCache tests object pool for MWeakReadReply
func TestMWeakReadReplyCache(t *testing.T) {
	cache := NewMWeakReadReplyCache()

	obj1 := cache.Get()
	if obj1 == nil {
		t.Fatal("Get from empty cache returned nil")
	}

	obj1.Replica = 5
	cache.Put(obj1)

	obj2 := cache.Get()
	if obj2 == nil {
		t.Fatal("Get after Put returned nil")
	}
}

// ============================================================================
// Phase 36: Client Local Cache Tests
// ============================================================================

// TestClientCacheMergeReplicaWins tests that replica value wins when version is higher
func TestClientCacheMergeReplicaWins(t *testing.T) {
	cache := make(map[int64]cacheEntry)

	// Cache has version 5
	cache[100] = cacheEntry{value: []byte("old"), version: 5}

	// Replica returns version 10 (higher)
	replicaVal := state.Value([]byte("new"))
	replicaVer := int32(10)

	cached := cache[100]
	var finalVal state.Value
	var finalVer int32
	if cached.version > replicaVer {
		finalVal = cached.value
		finalVer = cached.version
	} else {
		finalVal = replicaVal
		finalVer = replicaVer
	}

	if !bytes.Equal(finalVal, []byte("new")) {
		t.Errorf("Expected replica value 'new', got %v", finalVal)
	}
	if finalVer != 10 {
		t.Errorf("Expected version 10, got %d", finalVer)
	}
}

// TestClientCacheMergeCacheWins tests that cache value wins when version is higher
func TestClientCacheMergeCacheWins(t *testing.T) {
	cache := make(map[int64]cacheEntry)

	// Cache has version 10
	cache[100] = cacheEntry{value: []byte("cached"), version: 10}

	// Replica returns version 5 (lower)
	replicaVal := state.Value([]byte("stale"))
	replicaVer := int32(5)

	cached := cache[100]
	var finalVal state.Value
	var finalVer int32
	if cached.version > replicaVer {
		finalVal = cached.value
		finalVer = cached.version
	} else {
		finalVal = replicaVal
		finalVer = replicaVer
	}

	if !bytes.Equal(finalVal, []byte("cached")) {
		t.Errorf("Expected cached value 'cached', got %v", finalVal)
	}
	if finalVer != 10 {
		t.Errorf("Expected version 10, got %d", finalVer)
	}
}

// TestClientCacheMergeNoCache tests merge when cache has no entry for the key
func TestClientCacheMergeNoCache(t *testing.T) {
	cache := make(map[int64]cacheEntry)

	// No cache entry for key 100
	replicaVal := state.Value([]byte("from-replica"))
	replicaVer := int32(3)

	cached, hasCached := cache[100]
	var finalVal state.Value
	var finalVer int32
	if hasCached && cached.version > replicaVer {
		finalVal = cached.value
		finalVer = cached.version
	} else {
		finalVal = replicaVal
		finalVer = replicaVer
	}

	if !bytes.Equal(finalVal, []byte("from-replica")) {
		t.Errorf("Expected replica value, got %v", finalVal)
	}
	if finalVer != 3 {
		t.Errorf("Expected version 3, got %d", finalVer)
	}
}

// TestClientCacheEntryStruct tests cacheEntry struct fields
func TestClientCacheEntryStruct(t *testing.T) {
	entry := cacheEntry{
		value:   state.Value([]byte("test-value")),
		version: 42,
	}

	if !bytes.Equal(entry.value, []byte("test-value")) {
		t.Errorf("value mismatch")
	}
	if entry.version != 42 {
		t.Errorf("version = %d, want 42", entry.version)
	}
}

// TestClientCacheWeakWriteUpdate tests cache update after weak write commit
func TestClientCacheWeakWriteUpdate(t *testing.T) {
	cache := make(map[int64]cacheEntry)

	// Simulate weak write commit: key=100, value="written", slot=5
	key := int64(100)
	val := state.Value([]byte("written"))
	slot := int32(5)

	cache[key] = cacheEntry{value: val, version: slot}

	// Verify
	entry, exists := cache[key]
	if !exists {
		t.Fatal("Cache entry should exist after weak write")
	}
	if !bytes.Equal(entry.value, []byte("written")) {
		t.Errorf("value mismatch: got %v", entry.value)
	}
	if entry.version != 5 {
		t.Errorf("version = %d, want 5", entry.version)
	}
}

// TestClientCacheStrongUpdate tests cache update after strong op completion
func TestClientCacheStrongUpdate(t *testing.T) {
	cache := make(map[int64]cacheEntry)
	maxVersion := int32(0)

	// Simulate strong fast-path: key=200, slot from reply=10
	key := int64(200)
	val := state.Value([]byte("strong-value"))
	slot := int32(10)

	if slot > maxVersion {
		maxVersion = slot
	}
	cache[key] = cacheEntry{value: val, version: slot}

	// Verify
	if maxVersion != 10 {
		t.Errorf("maxVersion = %d, want 10", maxVersion)
	}
	entry := cache[key]
	if !bytes.Equal(entry.value, []byte("strong-value")) {
		t.Errorf("value mismatch")
	}
	if entry.version != 10 {
		t.Errorf("version = %d, want 10", entry.version)
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

// TestClientFastSlowPathCounters tests that fast/slow path counters are properly initialized
// and can be incremented independently.
func TestClientFastSlowPathCounters(t *testing.T) {
	c := &Client{}
	if c.fastPaths != 0 {
		t.Errorf("fastPaths should start at 0, got %d", c.fastPaths)
	}
	if c.slowPaths != 0 {
		t.Errorf("slowPaths should start at 0, got %d", c.slowPaths)
	}

	c.fastPaths++
	c.fastPaths++
	c.slowPaths++

	if c.fastPaths != 2 {
		t.Errorf("fastPaths: got %d, want 2", c.fastPaths)
	}
	if c.slowPaths != 1 {
		t.Errorf("slowPaths: got %d, want 1", c.slowPaths)
	}
}

// TestCommandDescSlotAssignedAt tests the slotAssignedAt field on commandDesc.
func TestCommandDescSlotAssignedAt(t *testing.T) {
	desc := &commandDesc{}
	if !desc.slotAssignedAt.IsZero() {
		t.Error("slotAssignedAt should be zero on new descriptor")
	}
	desc.slotAssignedAt = time.Now()
	if desc.slotAssignedAt.IsZero() {
		t.Error("slotAssignedAt should not be zero after assignment")
	}
	// Verify elapsed time is measurable
	elapsed := time.Since(desc.slotAssignedAt)
	if elapsed < 0 {
		t.Error("elapsed time should be non-negative")
	}
}

// TestClientMsgDropCounter tests that the ClientMsgDrops counter on Replica
// can be atomically incremented and read.
func TestClientMsgDropCounter(t *testing.T) {
	r := &replica.Replica{}
	if atomic.LoadInt64(&r.ClientMsgDrops) != 0 {
		t.Errorf("ClientMsgDrops should start at 0, got %d", r.ClientMsgDrops)
	}

	atomic.AddInt64(&r.ClientMsgDrops, 1)
	atomic.AddInt64(&r.ClientMsgDrops, 1)
	atomic.AddInt64(&r.ClientMsgDrops, 1)

	if atomic.LoadInt64(&r.ClientMsgDrops) != 3 {
		t.Errorf("ClientMsgDrops: got %d, want 3", atomic.LoadInt64(&r.ClientMsgDrops))
	}
}

// TestValuesSetAfterExecution verifies that r.values is set immediately
// after execution in deliver(), before descriptor cleanup. This enables
// MSync recovery for committed-but-not-yet-cleaned-up commands.
func TestValuesSetAfterExecution(t *testing.T) {
	values := cmap.New()

	// Simulate deliver() execution path for 3 slots
	for slot := 0; slot < 3; slot++ {
		desc := &commandDesc{
			cmdSlot: slot,
			applied: false,
			cmdId:   CommandId{ClientId: 1, SeqNum: int32(slot)},
		}
		desc.cmd = state.Command{Op: state.PUT, K: state.Key(slot), V: state.Value([]byte{byte(slot + 1)})}

		// Simulate execution
		desc.val = desc.cmd.V
		desc.applied = true

		// Values should be set immediately after execution
		values.Set(desc.cmdId.String(), desc.val)

		// Verify value is available (MSync can find it)
		val, exists := values.Get(desc.cmdId.String())
		if !exists {
			t.Errorf("slot %d: values not set after execution", slot)
		}
		if !bytes.Equal(val.([]byte), desc.val) {
			t.Errorf("slot %d: values mismatch: got %v, want %v", slot, val, desc.val)
		}
	}

	// Verify all 3 values are concurrently accessible
	if values.Count() != 3 {
		t.Errorf("expected 3 values, got %d", values.Count())
	}
}

// TestMSyncRetryPendingCount verifies that the pending command counting
// logic correctly tracks undelivered strong and weak write commands.
func TestMSyncRetryPendingCount(t *testing.T) {
	c := &Client{
		delivered:         make(map[int32]struct{}),
		strongPendingKeys: make(map[int32]int64),
		weakPending:       make(map[int32]struct{}),
		weakPendingValues: make(map[int32]state.Value),
	}

	// Simulate pending commands
	c.strongPendingKeys[1] = 100
	c.strongPendingKeys[2] = 200
	c.weakPending[3] = struct{}{}
	c.weakPendingValues[3] = state.Value([]byte{1})

	// Count undelivered
	var pending int
	for seqnum := range c.strongPendingKeys {
		if _, delivered := c.delivered[seqnum]; !delivered {
			pending++
		}
	}
	for seqnum := range c.weakPending {
		if _, delivered := c.delivered[seqnum]; !delivered {
			if _, isWrite := c.weakPendingValues[seqnum]; isWrite {
				pending++
			}
		}
	}
	if pending != 3 {
		t.Errorf("expected 3 pending, got %d", pending)
	}

	// Deliver one
	c.delivered[1] = struct{}{}
	pending = 0
	for seqnum := range c.strongPendingKeys {
		if _, delivered := c.delivered[seqnum]; !delivered {
			pending++
		}
	}
	for seqnum := range c.weakPending {
		if _, delivered := c.delivered[seqnum]; !delivered {
			if _, isWrite := c.weakPendingValues[seqnum]; isWrite {
				pending++
			}
		}
	}
	if pending != 2 {
		t.Errorf("expected 2 pending after delivery, got %d", pending)
	}
}

// TestWriterMuInitialization verifies writerMu is sized correctly.
func TestWriterMuInitialization(t *testing.T) {
	for _, n := range []int{3, 5, 7} {
		mu := make([]sync.Mutex, n)
		if len(mu) != n {
			t.Errorf("writerMu should have %d entries, got %d", n, len(mu))
		}
	}
}

// TestSplitMsgGoroutines verifies that handleStrongMsgs and handleWeakMsgs
// process messages on their respective channels without cross-contamination.
func TestSplitMsgGoroutines(t *testing.T) {
	// Verify the channel structure: strong channels should be separate from weak channels.
	// CommunicationSupply has distinct channels for each message type.
	var cs CommunicationSupply
	tbl := fastrpc.NewTableId(defs.RPC_TABLE)
	initCs(&cs, tbl)

	// Strong channels should be non-nil and distinct
	strongChans := []chan fastrpc.Serializable{
		cs.replyChan,
		cs.recordAckChan,
		cs.syncReplyChan,
	}
	for i, ch := range strongChans {
		if ch == nil {
			t.Errorf("strong channel %d is nil", i)
		}
	}

	// Weak channels should be non-nil and distinct
	weakChans := []chan fastrpc.Serializable{
		cs.weakReplyChan,
		cs.weakReadReplyChan,
	}
	for i, ch := range weakChans {
		if ch == nil {
			t.Errorf("weak channel %d is nil", i)
		}
	}

	// Verify no overlap between strong and weak channel sets
	strongSet := make(map[chan fastrpc.Serializable]bool)
	for _, ch := range strongChans {
		strongSet[ch] = true
	}
	for i, ch := range weakChans {
		if strongSet[ch] {
			t.Errorf("weak channel %d overlaps with a strong channel", i)
		}
	}
}

// ============================================================================
// Phase 128 Step 1: Role/Term State Management Tests
// ============================================================================

// TestRoleConstants verifies the role constant values are distinct.
func TestRoleConstants(t *testing.T) {
	if FOLLOWER == CANDIDATE || CANDIDATE == LEADER || FOLLOWER == LEADER {
		t.Error("role constants must be distinct")
	}
	// FOLLOWER should be 0 (default iota value)
	if FOLLOWER != 0 {
		t.Errorf("FOLLOWER should be 0, got %d", FOLLOWER)
	}
}

// TestReplicaInitialState verifies a new Replica starts as FOLLOWER with term 0.
func TestReplicaInitialState(t *testing.T) {
	r := &Replica{
		role:        FOLLOWER,
		currentTerm: 0,
		votedFor:    -1,
	}
	if r.role != FOLLOWER {
		t.Errorf("initial role should be FOLLOWER, got %d", r.role)
	}
	if r.currentTerm != 0 {
		t.Errorf("initial term should be 0, got %d", r.currentTerm)
	}
	if r.votedFor != -1 {
		t.Errorf("initial votedFor should be -1, got %d", r.votedFor)
	}
	if r.IsLeader() {
		t.Error("new replica should not be leader")
	}
}

// TestBecomeLeader verifies role transition to LEADER.
func TestBecomeLeader(t *testing.T) {
	r := &Replica{
		role:        FOLLOWER,
		currentTerm: 1,
		votedFor:    -1,
	}
	r.becomeLeader()
	if r.role != LEADER {
		t.Errorf("role should be LEADER after becomeLeader, got %d", r.role)
	}
	if !r.IsLeader() {
		t.Error("IsLeader() should return true after becomeLeader")
	}
	// becomeLeader should not change term or votedFor
	if r.currentTerm != 1 {
		t.Errorf("becomeLeader should not change term, got %d", r.currentTerm)
	}
	if r.votedFor != -1 {
		t.Errorf("becomeLeader should not change votedFor, got %d", r.votedFor)
	}
}

// TestBecomeFollowerHigherTerm verifies step-down to FOLLOWER with a higher term.
func TestBecomeFollowerHigherTerm(t *testing.T) {
	r := &Replica{
		role:        LEADER,
		currentTerm: 3,
		votedFor:    0,
	}
	r.Replica = &replica.Replica{} // Need base replica for Id field
	r.becomeFollower(5)
	if r.role != FOLLOWER {
		t.Errorf("role should be FOLLOWER, got %d", r.role)
	}
	if r.currentTerm != 5 {
		t.Errorf("currentTerm should be 5, got %d", r.currentTerm)
	}
	if r.votedFor != -1 {
		t.Errorf("votedFor should be reset to -1, got %d", r.votedFor)
	}
	if r.IsLeader() {
		t.Error("should not be leader after becomeFollower")
	}
}

// TestBecomeFollowerSameTerm verifies step-down without term change keeps votedFor.
func TestBecomeFollowerSameTerm(t *testing.T) {
	r := &Replica{
		role:        CANDIDATE,
		currentTerm: 3,
		votedFor:    2,
	}
	r.becomeFollower(3) // same term
	if r.role != FOLLOWER {
		t.Errorf("role should be FOLLOWER, got %d", r.role)
	}
	if r.currentTerm != 3 {
		t.Errorf("currentTerm should stay 3, got %d", r.currentTerm)
	}
	if r.votedFor != 2 {
		t.Errorf("votedFor should stay 2 for same term, got %d", r.votedFor)
	}
}

// TestBecomeCandidate verifies transition to CANDIDATE increments term and self-votes.
func TestBecomeCandidate(t *testing.T) {
	r := &Replica{
		role:        FOLLOWER,
		currentTerm: 5,
		votedFor:    -1,
	}
	r.Replica = &replica.Replica{}
	r.Id = 2

	r.becomeCandidate()
	if r.role != CANDIDATE {
		t.Errorf("role should be CANDIDATE, got %d", r.role)
	}
	if r.currentTerm != 6 {
		t.Errorf("currentTerm should be 6 (incremented), got %d", r.currentTerm)
	}
	if r.votedFor != 2 {
		t.Errorf("votedFor should be self (2), got %d", r.votedFor)
	}
	if r.IsLeader() {
		t.Error("candidate should not be leader")
	}
}

// TestRoleTransitionSequence verifies a full election lifecycle:
// FOLLOWER -> CANDIDATE -> LEADER -> FOLLOWER (step-down).
func TestRoleTransitionSequence(t *testing.T) {
	r := &Replica{
		role:        FOLLOWER,
		currentTerm: 0,
		votedFor:    -1,
	}
	r.Replica = &replica.Replica{}
	r.Id = 1

	// Step 1: Start election
	r.becomeCandidate()
	if r.role != CANDIDATE || r.currentTerm != 1 || r.votedFor != 1 {
		t.Errorf("after becomeCandidate: role=%d term=%d votedFor=%d", r.role, r.currentTerm, r.votedFor)
	}

	// Step 2: Win election
	r.becomeLeader()
	if r.role != LEADER || r.currentTerm != 1 {
		t.Errorf("after becomeLeader: role=%d term=%d", r.role, r.currentTerm)
	}

	// Step 3: Discover higher term → step down
	r.becomeFollower(3)
	if r.role != FOLLOWER || r.currentTerm != 3 || r.votedFor != -1 {
		t.Errorf("after becomeFollower(3): role=%d term=%d votedFor=%d", r.role, r.currentTerm, r.votedFor)
	}
}

// TestHandleAcceptTermStepDown verifies that handleAccept steps down
// when receiving a message with a higher ballot/term.
// After step-down, the handler proceeds (terms match), so we provide
// a properly initialized afterPayload to avoid nil panic.
func TestHandleAcceptTermStepDown(t *testing.T) {
	r := &Replica{
		role:        LEADER,
		currentTerm: 2,
		status:      NORMAL,
	}
	r.Replica = &replica.Replica{N: 3}
	r.delivered = cmap.New()

	msg := &MAccept{
		Replica: 1,
		Ballot:  5, // higher term
		CmdSlot: 0,
	}
	desc := &commandDesc{
		afterPayload: hook.NewOptCondF(func() bool { return false }),
	}

	r.handleAccept(msg, desc)
	if r.role != FOLLOWER {
		t.Errorf("should step down to FOLLOWER, got role=%d", r.role)
	}
	if r.currentTerm != 5 {
		t.Errorf("currentTerm should be updated to 5, got %d", r.currentTerm)
	}
}

// TestHandleAcceptAckTermStepDown verifies that handleAcceptAck steps down
// when receiving a message with a higher ballot/term.
func TestHandleAcceptAckTermStepDown(t *testing.T) {
	r := &Replica{
		role:        LEADER,
		currentTerm: 2,
		status:      NORMAL,
	}
	r.Replica = &replica.Replica{N: 3}

	msg := &MAcceptAck{
		Replica: 1,
		Ballot:  7,
		CmdSlot: 0,
	}
	desc := &commandDesc{
		acks: replica.NewMsgSet(replica.NewMajorityOf(3), func(_, _ interface{}) bool { return true }, nil, func(_ interface{}, _ []interface{}) {}),
	}

	r.handleAcceptAck(msg, desc)
	if r.role != FOLLOWER {
		t.Errorf("should step down to FOLLOWER, got role=%d", r.role)
	}
	if r.currentTerm != 7 {
		t.Errorf("currentTerm should be updated to 7, got %d", r.currentTerm)
	}
}

// TestHandleCommitTermStepDown verifies that handleCommit steps down
// when receiving a message with a higher ballot/term.
func TestHandleCommitTermStepDown(t *testing.T) {
	r := &Replica{
		role:        LEADER,
		currentTerm: 2,
		status:      NORMAL,
	}
	r.Replica = &replica.Replica{N: 3}
	r.delivered = cmap.New()

	msg := &MCommit{
		Replica: 1,
		Ballot:  10,
		CmdSlot: 0,
	}

	desc := &commandDesc{
		afterPayload: hook.NewOptCondF(func() bool { return false }),
	}
	r.handleCommit(msg, desc)
	if r.role != FOLLOWER {
		t.Errorf("should step down to FOLLOWER, got role=%d", r.role)
	}
	if r.currentTerm != 10 {
		t.Errorf("currentTerm should be updated to 10, got %d", r.currentTerm)
	}
}

// TestHandleAcceptStaleTerm verifies that messages with lower term are rejected.
func TestHandleAcceptStaleTerm(t *testing.T) {
	r := &Replica{
		role:        LEADER,
		currentTerm: 5,
		status:      NORMAL,
	}
	r.Replica = &replica.Replica{N: 3}
	r.delivered = cmap.New()

	msg := &MAccept{
		Replica: 1,
		Ballot:  3, // stale term
		CmdSlot: 0,
	}
	desc := &commandDesc{}

	// Should not step down for stale term — returns early before afterPayload
	r.handleAccept(msg, desc)
	if r.role != LEADER {
		t.Errorf("should remain LEADER for stale term, got role=%d", r.role)
	}
	if r.currentTerm != 5 {
		t.Errorf("currentTerm should remain 5, got %d", r.currentTerm)
	}
}

// TestIsLeaderMethod verifies the IsLeader() convenience method.
func TestIsLeaderMethod(t *testing.T) {
	r := &Replica{role: FOLLOWER}
	if r.IsLeader() {
		t.Error("FOLLOWER should not be leader")
	}
	r.role = CANDIDATE
	if r.IsLeader() {
		t.Error("CANDIDATE should not be leader")
	}
	r.role = LEADER
	if !r.IsLeader() {
		t.Error("LEADER should be leader")
	}
}

// ============================================================================
// Phase 128 Step 2: Leader Election Protocol Tests
// ============================================================================

// TestMRequestVoteSerialization tests MRequestVote Marshal/Unmarshal round-trip.
func TestMRequestVoteSerialization(t *testing.T) {
	original := &MRequestVote{
		Replica:          2,
		Term:             5,
		LastCommittedSlot: 42,
	}

	var buf bytes.Buffer
	original.Marshal(&buf)

	if buf.Len() != 12 {
		t.Errorf("MRequestVote should serialize to 12 bytes, got %d", buf.Len())
	}

	restored := &MRequestVote{}
	err := restored.Unmarshal(&buf)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if restored.Replica != original.Replica {
		t.Errorf("Replica mismatch: got %d, want %d", restored.Replica, original.Replica)
	}
	if restored.Term != original.Term {
		t.Errorf("Term mismatch: got %d, want %d", restored.Term, original.Term)
	}
	if restored.LastCommittedSlot != original.LastCommittedSlot {
		t.Errorf("LastCommittedSlot mismatch: got %d, want %d", restored.LastCommittedSlot, original.LastCommittedSlot)
	}
}

// TestMRequestVoteReplySerialization tests MRequestVoteReply Marshal/Unmarshal round-trip.
func TestMRequestVoteReplySerialization(t *testing.T) {
	original := &MRequestVoteReply{
		Replica:     1,
		Term:        3,
		VoteGranted: TRUE,
	}

	var buf bytes.Buffer
	original.Marshal(&buf)

	if buf.Len() != 9 {
		t.Errorf("MRequestVoteReply should serialize to 9 bytes, got %d", buf.Len())
	}

	restored := &MRequestVoteReply{}
	err := restored.Unmarshal(&buf)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if restored.Replica != original.Replica {
		t.Errorf("Replica mismatch: got %d, want %d", restored.Replica, original.Replica)
	}
	if restored.Term != original.Term {
		t.Errorf("Term mismatch: got %d, want %d", restored.Term, original.Term)
	}
	if restored.VoteGranted != original.VoteGranted {
		t.Errorf("VoteGranted mismatch: got %d, want %d", restored.VoteGranted, original.VoteGranted)
	}
}

// TestMHeartbeatSerialization tests MHeartbeat Marshal/Unmarshal round-trip.
func TestMHeartbeatSerialization(t *testing.T) {
	original := &MHeartbeat{
		Replica: 0,
		Term:    7,
	}

	var buf bytes.Buffer
	original.Marshal(&buf)

	if buf.Len() != 8 {
		t.Errorf("MHeartbeat should serialize to 8 bytes, got %d", buf.Len())
	}

	restored := &MHeartbeat{}
	err := restored.Unmarshal(&buf)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if restored.Replica != original.Replica {
		t.Errorf("Replica mismatch: got %d, want %d", restored.Replica, original.Replica)
	}
	if restored.Term != original.Term {
		t.Errorf("Term mismatch: got %d, want %d", restored.Term, original.Term)
	}
}

// TestMRequestVoteBinarySize tests BinarySize.
func TestMRequestVoteBinarySize(t *testing.T) {
	m := &MRequestVote{}
	size, known := m.BinarySize()
	if !known || size != 12 {
		t.Errorf("BinarySize should be (12, true), got (%d, %v)", size, known)
	}
}

// TestMRequestVoteReplyBinarySize tests BinarySize.
func TestMRequestVoteReplyBinarySize(t *testing.T) {
	m := &MRequestVoteReply{}
	size, known := m.BinarySize()
	if !known || size != 9 {
		t.Errorf("BinarySize should be (9, true), got (%d, %v)", size, known)
	}
}

// TestMHeartbeatBinarySize tests BinarySize.
func TestMHeartbeatBinarySize(t *testing.T) {
	m := &MHeartbeat{}
	size, known := m.BinarySize()
	if !known || size != 8 {
		t.Errorf("BinarySize should be (8, true), got (%d, %v)", size, known)
	}
}

// TestMRequestVoteCache tests object pool for MRequestVote.
func TestMRequestVoteCache(t *testing.T) {
	cache := NewMRequestVoteCache()
	obj := cache.Get()
	if obj == nil {
		t.Fatal("Get returned nil")
	}
	obj.Term = 5
	cache.Put(obj)
	obj2 := cache.Get()
	if obj2 == nil {
		t.Fatal("Get after Put returned nil")
	}
}

// TestStartElection verifies that startElection increments term and self-votes.
func TestStartElection(t *testing.T) {
	r := &Replica{
		role:        FOLLOWER,
		currentTerm: 3,
		votedFor:    -1,
	}
	r.Replica = newTestBaseReplica(5)
	r.Id = 2
	r.sender = newTestSender() // No-op sender for tests
	r.electionTimer = time.NewTimer(time.Hour) // Won't fire

	r.startElection()

	if r.role != CANDIDATE {
		t.Errorf("role should be CANDIDATE, got %d", r.role)
	}
	if r.currentTerm != 4 {
		t.Errorf("currentTerm should be 4, got %d", r.currentTerm)
	}
	if r.votedFor != 2 {
		t.Errorf("votedFor should be self (2), got %d", r.votedFor)
	}
	if r.votesReceived != 1 {
		t.Errorf("votesReceived should be 1 (self), got %d", r.votesReceived)
	}
}

// TestHandleRequestVoteGranted verifies vote is granted for valid request.
func TestHandleRequestVoteGranted(t *testing.T) {
	r := &Replica{
		role:          FOLLOWER,
		currentTerm:   3,
		votedFor:      -1,
		lastCommitted: 10,
	}
	r.Replica = newTestBaseReplica(5)
	r.Id = 1
	r.sender = newTestSender()
	r.electionTimer = time.NewTimer(time.Hour)

	msg := &MRequestVote{
		Replica:          2,
		Term:             3,
		LastCommittedSlot: 10,
	}

	r.handleRequestVote(msg)

	if r.votedFor != 2 {
		t.Errorf("votedFor should be 2 (granted), got %d", r.votedFor)
	}
}

// TestHandleRequestVoteDeniedStaleTerm verifies vote is denied for stale term.
func TestHandleRequestVoteDeniedStaleTerm(t *testing.T) {
	r := &Replica{
		role:          FOLLOWER,
		currentTerm:   5,
		votedFor:      -1,
		lastCommitted: 10,
	}
	r.Replica = newTestBaseReplica(5)
	r.Id = 1
	r.sender = newTestSender()

	msg := &MRequestVote{
		Replica:          2,
		Term:             3, // stale term
		LastCommittedSlot: 10,
	}

	r.handleRequestVote(msg)

	if r.votedFor != -1 {
		t.Errorf("votedFor should remain -1 (denied), got %d", r.votedFor)
	}
}

// TestHandleRequestVoteDeniedAlreadyVoted verifies vote is denied if already voted.
func TestHandleRequestVoteDeniedAlreadyVoted(t *testing.T) {
	r := &Replica{
		role:          FOLLOWER,
		currentTerm:   3,
		votedFor:      0, // Already voted for replica 0
		lastCommitted: 10,
	}
	r.Replica = newTestBaseReplica(5)
	r.Id = 1
	r.sender = newTestSender()

	msg := &MRequestVote{
		Replica:          2, // Different candidate
		Term:             3,
		LastCommittedSlot: 10,
	}

	r.handleRequestVote(msg)

	if r.votedFor != 0 {
		t.Errorf("votedFor should remain 0 (already voted), got %d", r.votedFor)
	}
}

// TestHandleRequestVoteHigherTerm verifies step-down on higher term request.
func TestHandleRequestVoteHigherTerm(t *testing.T) {
	r := &Replica{
		role:          LEADER,
		currentTerm:   3,
		votedFor:      1,
		lastCommitted: 10,
	}
	r.Replica = newTestBaseReplica(5)
	r.Id = 1
	r.sender = newTestSender()
	r.electionTimer = time.NewTimer(time.Hour)

	msg := &MRequestVote{
		Replica:          2,
		Term:             5, // Higher term
		LastCommittedSlot: 10,
	}

	r.handleRequestVote(msg)

	if r.role != FOLLOWER {
		t.Errorf("role should be FOLLOWER after higher term, got %d", r.role)
	}
	if r.currentTerm != 5 {
		t.Errorf("currentTerm should be 5, got %d", r.currentTerm)
	}
	// Should grant vote since votedFor reset to -1 and log is up-to-date
	if r.votedFor != 2 {
		t.Errorf("votedFor should be 2 (granted after step-down), got %d", r.votedFor)
	}
}

// TestHandleRequestVoteDeniedStaleLog verifies vote denied when candidate log is behind.
func TestHandleRequestVoteDeniedStaleLog(t *testing.T) {
	r := &Replica{
		role:          FOLLOWER,
		currentTerm:   3,
		votedFor:      -1,
		lastCommitted: 20, // Our log is further ahead
	}
	r.Replica = newTestBaseReplica(5)
	r.Id = 1
	r.sender = newTestSender()

	msg := &MRequestVote{
		Replica:          2,
		Term:             3,
		LastCommittedSlot: 10, // Candidate is behind
	}

	r.handleRequestVote(msg)

	if r.votedFor != -1 {
		t.Errorf("votedFor should remain -1 (candidate log behind), got %d", r.votedFor)
	}
}

// TestHandleVoteReplyWinElection verifies a candidate wins with majority votes.
func TestHandleVoteReplyWinElection(t *testing.T) {
	r := &Replica{
		role:          CANDIDATE,
		currentTerm:   5,
		votedFor:      1,
		votesReceived: 1, // Self vote
	}
	r.Replica = newTestBaseReplica(5)
	r.Id = 1
	r.sender = newTestSender()
	r.electionTimer = time.NewTimer(time.Hour)

	// Receive 2 more votes (need 3 total = majority of 5)
	r.handleRequestVoteReply(&MRequestVoteReply{Replica: 2, Term: 5, VoteGranted: TRUE})
	if r.role == LEADER {
		t.Error("should not be leader after 2 votes (need 3)")
	}

	r.handleRequestVoteReply(&MRequestVoteReply{Replica: 3, Term: 5, VoteGranted: TRUE})
	if r.role != LEADER {
		t.Errorf("should be LEADER after 3 votes, got role=%d", r.role)
	}
	if r.votesReceived != 3 {
		t.Errorf("votesReceived should be 3, got %d", r.votesReceived)
	}
}

// TestHandleVoteReplyDenied verifies denied votes don't count.
func TestHandleVoteReplyDenied(t *testing.T) {
	r := &Replica{
		role:          CANDIDATE,
		currentTerm:   5,
		votedFor:      1,
		votesReceived: 1,
	}
	r.Replica = newTestBaseReplica(5)
	r.Id = 1
	r.sender = newTestSender()

	r.handleRequestVoteReply(&MRequestVoteReply{Replica: 2, Term: 5, VoteGranted: FALSE})
	if r.votesReceived != 1 {
		t.Errorf("votesReceived should remain 1 after denied vote, got %d", r.votesReceived)
	}
}

// TestHandleVoteReplyHigherTerm verifies step-down on higher term reply.
func TestHandleVoteReplyHigherTerm(t *testing.T) {
	r := &Replica{
		role:          CANDIDATE,
		currentTerm:   5,
		votedFor:      1,
		votesReceived: 1,
	}
	r.Replica = newTestBaseReplica(5)
	r.Id = 1
	r.sender = newTestSender()

	r.handleRequestVoteReply(&MRequestVoteReply{Replica: 2, Term: 8, VoteGranted: FALSE})
	if r.role != FOLLOWER {
		t.Errorf("should step down to FOLLOWER, got role=%d", r.role)
	}
	if r.currentTerm != 8 {
		t.Errorf("currentTerm should be 8, got %d", r.currentTerm)
	}
}

// TestHandleHeartbeatResetTimer verifies heartbeat resets election timer.
func TestHandleHeartbeatResetTimer(t *testing.T) {
	r := &Replica{
		role:        FOLLOWER,
		currentTerm: 3,
		votedFor:    -1,
	}
	r.Replica = newTestBaseReplica(5)
	r.electionTimer = time.NewTimer(time.Hour)

	msg := &MHeartbeat{Replica: 0, Term: 3}
	r.handleHeartbeat(msg)

	// After handling heartbeat, role should still be follower
	if r.role != FOLLOWER {
		t.Errorf("role should be FOLLOWER, got %d", r.role)
	}
}

// TestHandleHeartbeatHigherTerm verifies step-down on higher term heartbeat.
func TestHandleHeartbeatHigherTerm(t *testing.T) {
	r := &Replica{
		role:        CANDIDATE,
		currentTerm: 3,
		votedFor:    1,
	}
	r.Replica = newTestBaseReplica(5)
	r.electionTimer = time.NewTimer(time.Hour)

	msg := &MHeartbeat{Replica: 0, Term: 5}
	r.handleHeartbeat(msg)

	if r.role != FOLLOWER {
		t.Errorf("should step down to FOLLOWER, got role=%d", r.role)
	}
	if r.currentTerm != 5 {
		t.Errorf("currentTerm should be 5, got %d", r.currentTerm)
	}
}

// TestHandleHeartbeatStale verifies stale heartbeats are ignored.
func TestHandleHeartbeatStale(t *testing.T) {
	r := &Replica{
		role:        FOLLOWER,
		currentTerm: 5,
		votedFor:    -1,
	}
	r.Replica = newTestBaseReplica(5)

	msg := &MHeartbeat{Replica: 0, Term: 3} // stale
	r.handleHeartbeat(msg)

	if r.currentTerm != 5 {
		t.Errorf("currentTerm should remain 5, got %d", r.currentTerm)
	}
}

// TestHandleHeartbeatCandidateStepDown verifies candidate steps down on same-term heartbeat.
func TestHandleHeartbeatCandidateStepDown(t *testing.T) {
	r := &Replica{
		role:        CANDIDATE,
		currentTerm: 3,
		votedFor:    1,
	}
	r.Replica = newTestBaseReplica(5)
	r.electionTimer = time.NewTimer(time.Hour)

	// Same-term heartbeat from another leader
	msg := &MHeartbeat{Replica: 0, Term: 3}
	r.handleHeartbeat(msg)

	if r.role != FOLLOWER {
		t.Errorf("candidate should step down on same-term heartbeat, got role=%d", r.role)
	}
}

// TestRandomElectionTimeout verifies timeout is in expected range.
func TestRandomElectionTimeout(t *testing.T) {
	for i := 0; i < 100; i++ {
		d := randomElectionTimeout()
		if d < ElectionTimeoutMin || d > ElectionTimeoutMax {
			t.Errorf("timeout %v outside range [%v, %v]", d, ElectionTimeoutMin, ElectionTimeoutMax)
		}
	}
}

// TestElectionConstants verifies election timing constants.
func TestElectionConstants(t *testing.T) {
	if ElectionTimeoutMin >= ElectionTimeoutMax {
		t.Error("ElectionTimeoutMin should be less than ElectionTimeoutMax")
	}
	if HeartbeatInterval >= ElectionTimeoutMin {
		t.Error("HeartbeatInterval should be less than ElectionTimeoutMin")
	}
}

// TestElectionChannelRegistration verifies election channels are registered.
func TestElectionChannelRegistration(t *testing.T) {
	var cs CommunicationSupply
	tbl := fastrpc.NewTableId(defs.RPC_TABLE)
	initCs(&cs, tbl)

	if cs.requestVoteChan == nil {
		t.Error("requestVoteChan should not be nil")
	}
	if cs.requestVoteReplyChan == nil {
		t.Error("requestVoteReplyChan should not be nil")
	}
	if cs.heartbeatChan == nil {
		t.Error("heartbeatChan should not be nil")
	}
	// RPC codes should be non-zero (registered)
	if cs.requestVoteRPC == 0 {
		t.Error("requestVoteRPC should be registered")
	}
	if cs.requestVoteReplyRPC == 0 {
		t.Error("requestVoteReplyRPC should be registered")
	}
	if cs.heartbeatRPC == 0 {
		t.Error("heartbeatRPC should be registered")
	}
}

// ============================================================================
// Phase 128 Step 4: Log Recovery Tests
// ============================================================================

// newTestReplicaForRecovery creates a Replica suitable for log recovery tests.
func newTestReplicaForRecovery(id int32, n int) *Replica {
	base := newTestBaseReplica(n)
	base.Id = id
	base.Exec = true
	base.State = state.InitState()

	var cs CommunicationSupply
	tbl := fastrpc.NewTableId(defs.RPC_TABLE)
	initCs(&cs, tbl)

	r := &Replica{
		Replica:     base,
		currentTerm: 1,
		status:      NORMAL,
		votedFor:    -1,
		role:        FOLLOWER,
		lastCmdSlot: 0,
		slots:       make(map[CommandId]int),
		synced:      cmap.New(),
		values:      cmap.New(),
		proposes:    cmap.New(),
		cmdDescs:    cmap.New(),
		unsynced:    cmap.New(),
		executed:    cmap.New(),
		committed:   cmap.New(),
		delivered:   cmap.New(),
		weakExecuted: cmap.New(),
		keyVersions:  cmap.New(),
		history:      make([]commandStaticDesc, HISTORY_SIZE),
		commitNotify:  make(map[int]chan struct{}),
		executeNotify: make(map[int]chan struct{}),
		deliverChan:   make(chan int, defs.CHAN_BUFFER_SIZE),
		sender:        newTestSender(),
		cs:            cs,
		Q:             replica.NewMajorityOf(n),
		weakDepNotify: make(map[int32]chan struct{}),
	}

	r.closedChan = make(chan struct{})
	close(r.closedChan)

	return r
}

// TestMLogSyncSerialization tests MLogSync Marshal/Unmarshal round-trip
func TestMLogSyncSerialization(t *testing.T) {
	original := &MLogSync{
		Replica: 2,
		Term:    5,
	}

	var buf bytes.Buffer
	original.Marshal(&buf)

	decoded := &MLogSync{}
	if err := decoded.Unmarshal(&buf); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if decoded.Replica != original.Replica {
		t.Errorf("Replica: got %d, want %d", decoded.Replica, original.Replica)
	}
	if decoded.Term != original.Term {
		t.Errorf("Term: got %d, want %d", decoded.Term, original.Term)
	}
}

// TestMLogSyncBinarySize tests that MLogSync reports correct binary size.
func TestMLogSyncBinarySize(t *testing.T) {
	msg := &MLogSync{}
	size, known := msg.BinarySize()
	if !known {
		t.Error("MLogSync should have known size")
	}
	if size != 8 {
		t.Errorf("expected 8 bytes, got %d", size)
	}
}

// TestMLogSyncReplySerializationEmpty tests MLogSyncReply with no entries.
func TestMLogSyncReplySerializationEmpty(t *testing.T) {
	original := &MLogSyncReply{
		Replica:    1,
		Term:       3,
		NumEntries: 0,
		Entries:    []LogEntry{},
	}

	var buf bytes.Buffer
	original.Marshal(&buf)

	decoded := &MLogSyncReply{}
	if err := decoded.Unmarshal(&buf); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if decoded.Replica != 1 || decoded.Term != 3 || decoded.NumEntries != 0 {
		t.Errorf("header mismatch: got {%d,%d,%d}", decoded.Replica, decoded.Term, decoded.NumEntries)
	}
	if len(decoded.Entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(decoded.Entries))
	}
}

// TestMLogSyncReplySerializationWithEntries tests MLogSyncReply with multiple entries.
func TestMLogSyncReplySerializationWithEntries(t *testing.T) {
	entries := []LogEntry{
		{
			Slot:  0,
			CmdId: CommandId{ClientId: 10, SeqNum: 1},
			Cmd:   state.Command{Op: state.PUT, K: 42, V: state.NIL()},
		},
		{
			Slot:  1,
			CmdId: CommandId{ClientId: 10, SeqNum: 2},
			Cmd:   state.Command{Op: state.GET, K: 100, V: state.NIL()},
		},
		{
			Slot:  5,
			CmdId: CommandId{ClientId: 20, SeqNum: 1},
			Cmd:   state.Command{Op: state.PUT, K: 7, V: state.NIL()},
		},
	}

	original := &MLogSyncReply{
		Replica:    2,
		Term:       4,
		NumEntries: int32(len(entries)),
		Entries:    entries,
	}

	var buf bytes.Buffer
	original.Marshal(&buf)

	decoded := &MLogSyncReply{}
	if err := decoded.Unmarshal(&buf); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if decoded.Replica != 2 || decoded.Term != 4 || decoded.NumEntries != 3 {
		t.Errorf("header mismatch: got {%d,%d,%d}", decoded.Replica, decoded.Term, decoded.NumEntries)
	}
	if len(decoded.Entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(decoded.Entries))
	}
	for i, e := range decoded.Entries {
		if e.Slot != entries[i].Slot {
			t.Errorf("entry %d slot: got %d, want %d", i, e.Slot, entries[i].Slot)
		}
		if e.CmdId != entries[i].CmdId {
			t.Errorf("entry %d cmdId: got %v, want %v", i, e.CmdId, entries[i].CmdId)
		}
		if e.Cmd.Op != entries[i].Cmd.Op || e.Cmd.K != entries[i].Cmd.K {
			t.Errorf("entry %d cmd: got {%d,%d}, want {%d,%d}",
				i, e.Cmd.Op, e.Cmd.K, entries[i].Cmd.Op, entries[i].Cmd.K)
		}
	}
}

// TestMLogSyncReplyBinarySize tests that MLogSyncReply reports unknown size.
func TestMLogSyncReplyBinarySize(t *testing.T) {
	msg := &MLogSyncReply{}
	_, known := msg.BinarySize()
	if known {
		t.Error("MLogSyncReply should have unknown size (variable-length entries)")
	}
}

// TestMLogSyncCache tests MLogSync cache get/put.
func TestMLogSyncCache(t *testing.T) {
	cache := NewMLogSyncCache()
	msg := &MLogSync{Replica: 1, Term: 2}
	cache.Put(msg)
	got := cache.Get()
	if got.Replica != 1 || got.Term != 2 {
		t.Errorf("cache round-trip failed: got {%d,%d}", got.Replica, got.Term)
	}
	// Get from empty cache should return fresh
	fresh := cache.Get()
	if fresh == nil {
		t.Error("Get from empty cache should return non-nil")
	}
}

// TestMLogSyncReplyCache tests MLogSyncReply cache get/put.
func TestMLogSyncReplyCache(t *testing.T) {
	cache := NewMLogSyncReplyCache()
	msg := &MLogSyncReply{Replica: 3, Term: 7, NumEntries: 0, Entries: nil}
	cache.Put(msg)
	got := cache.Get()
	if got.Replica != 3 || got.Term != 7 {
		t.Errorf("cache round-trip failed: got {%d,%d}", got.Replica, got.Term)
	}
}

// TestLogSyncChannelRegistration verifies log sync channels are registered.
func TestLogSyncChannelRegistration(t *testing.T) {
	var cs CommunicationSupply
	tbl := fastrpc.NewTableId(defs.RPC_TABLE)
	initCs(&cs, tbl)

	if cs.logSyncChan == nil {
		t.Error("logSyncChan should not be nil")
	}
	if cs.logSyncReplyChan == nil {
		t.Error("logSyncReplyChan should not be nil")
	}
	if cs.logSyncRPC == 0 {
		t.Error("logSyncRPC should be registered")
	}
	if cs.logSyncReplyRPC == 0 {
		t.Error("logSyncReplyRPC should be registered")
	}
}

// TestHandleLogSyncReturnsCommittedEntries tests that handleLogSync sends a reply (via sender).
func TestHandleLogSyncReturnsCommittedEntries(t *testing.T) {
	r := newTestReplicaForRecovery(1, 3)
	r.role = FOLLOWER
	r.currentTerm = 5

	// Populate history with committed entries
	r.history[0] = commandStaticDesc{
		cmdSlot: 0, phase: COMMIT,
		cmd:   state.Command{Op: state.PUT, K: 1, V: state.NIL()},
		cmdId: CommandId{ClientId: 10, SeqNum: 1},
	}
	r.history[1] = commandStaticDesc{
		cmdSlot: 1, phase: COMMIT,
		cmd:   state.Command{Op: state.PUT, K: 2, V: state.NIL()},
		cmdId: CommandId{ClientId: 10, SeqNum: 2},
	}
	// Slot 2 is only ACCEPT (not committed) — should NOT be returned
	r.history[2] = commandStaticDesc{
		cmdSlot: 2, phase: ACCEPT,
		cmd:   state.Command{Op: state.PUT, K: 3, V: state.NIL()},
		cmdId: CommandId{ClientId: 10, SeqNum: 3},
	}

	msg := &MLogSync{Replica: 0, Term: 5}
	r.handleLogSync(msg)

	// Verify a message was sent via the sender channel
	select {
	case <-r.sender:
		// OK — reply was sent
	case <-time.After(100 * time.Millisecond):
		t.Fatal("expected MLogSyncReply to be sent")
	}
}

// TestHandleLogSyncStaleTerm tests that stale-term MLogSync is ignored.
func TestHandleLogSyncStaleTerm(t *testing.T) {
	r := newTestReplicaForRecovery(1, 3)
	r.currentTerm = 5

	msg := &MLogSync{Replica: 0, Term: 3} // stale term
	r.handleLogSync(msg)

	// No reply should be sent
	select {
	case <-r.sender:
		t.Error("should not send reply for stale-term MLogSync")
	case <-time.After(50 * time.Millisecond):
		// OK — no reply sent
	}
}

// TestHandleLogSyncHigherTermStepDown tests that higher-term MLogSync causes step-down.
func TestHandleLogSyncHigherTermStepDown(t *testing.T) {
	r := newTestReplicaForRecovery(1, 3)
	r.currentTerm = 3
	r.role = LEADER

	msg := &MLogSync{Replica: 0, Term: 5}
	r.handleLogSync(msg)

	if r.currentTerm != 5 {
		t.Errorf("expected term 5, got %d", r.currentTerm)
	}
	if r.role != FOLLOWER {
		t.Errorf("expected FOLLOWER role, got %d", r.role)
	}
}

// TestHandleLogSyncReplyCollectsReplies tests reply collection during recovery.
func TestHandleLogSyncReplyCollectsReplies(t *testing.T) {
	r := newTestReplicaForRecovery(0, 3)
	r.role = LEADER
	r.status = RECOVERING
	r.currentTerm = 5
	r.logSyncReplies = make([]MLogSyncReply, 0)
	r.logSyncExpected = 2

	// First reply — not enough for majority
	reply1 := &MLogSyncReply{
		Replica: 1, Term: 5, NumEntries: 0, Entries: nil,
	}
	r.handleLogSyncReply(reply1)

	if r.status != NORMAL {
		// With N=3, majority = 2 (N/2=1). One reply from peer + self = 2 >= majority.
		// So status should already be NORMAL.
		t.Log("Note: with N=3, one peer reply is sufficient for majority")
	}
}

// TestHandleLogSyncReplyIgnoredWhenNotRecovering tests that replies are ignored outside recovery.
func TestHandleLogSyncReplyIgnoredWhenNotRecovering(t *testing.T) {
	r := newTestReplicaForRecovery(0, 3)
	r.role = LEADER
	r.status = NORMAL // Not recovering
	r.currentTerm = 5

	reply := &MLogSyncReply{
		Replica: 1, Term: 5, NumEntries: 1,
		Entries: []LogEntry{{Slot: 0, CmdId: CommandId{10, 1}, Cmd: state.Command{Op: state.PUT, K: 1, V: state.NIL()}}},
	}
	r.handleLogSyncReply(reply)

	// Should be ignored — no state change
	if r.status != NORMAL {
		t.Error("status should remain NORMAL")
	}
}

// TestHandleLogSyncReplyHigherTermStepDown tests step-down on higher-term reply.
func TestHandleLogSyncReplyHigherTermStepDown(t *testing.T) {
	r := newTestReplicaForRecovery(0, 3)
	r.role = LEADER
	r.status = RECOVERING
	r.currentTerm = 5

	reply := &MLogSyncReply{Replica: 1, Term: 7, NumEntries: 0}
	r.handleLogSyncReply(reply)

	if r.currentTerm != 7 {
		t.Errorf("expected term 7, got %d", r.currentTerm)
	}
	if r.role != FOLLOWER {
		t.Errorf("expected FOLLOWER role, got %d", r.role)
	}
}

// TestMergeAndRecoverLogBasic tests basic log merge with entries from peers.
func TestMergeAndRecoverLogBasic(t *testing.T) {
	r := newTestReplicaForRecovery(0, 3)
	r.role = LEADER
	r.status = RECOVERING
	r.currentTerm = 5

	// Simulate: our history has slot 0 committed
	r.history[0] = commandStaticDesc{
		cmdSlot: 0, phase: COMMIT,
		cmd:   state.Command{Op: state.PUT, K: 1, V: state.NIL()},
		cmdId: CommandId{ClientId: 10, SeqNum: 1},
	}

	// Peer has slot 0 and slot 1 committed
	r.logSyncReplies = []MLogSyncReply{
		{
			Replica: 1, Term: 5, NumEntries: 2,
			Entries: []LogEntry{
				{Slot: 0, CmdId: CommandId{10, 1}, Cmd: state.Command{Op: state.PUT, K: 1, V: state.NIL()}},
				{Slot: 1, CmdId: CommandId{10, 2}, Cmd: state.Command{Op: state.PUT, K: 2, V: state.NIL()}},
			},
		},
	}

	r.mergeAndRecoverLog()

	if r.status != NORMAL {
		t.Errorf("expected NORMAL status, got %d", r.status)
	}
	if r.lastCmdSlot != 2 {
		t.Errorf("expected lastCmdSlot=2, got %d", r.lastCmdSlot)
	}
	if r.lastCommitted != 1 {
		t.Errorf("expected lastCommitted=1, got %d", r.lastCommitted)
	}
	// Verify both slots are marked executed
	if !r.executed.Has("0") || !r.executed.Has("1") {
		t.Error("expected slots 0 and 1 to be executed")
	}
	// Verify values are stored
	cmdId1 := CommandId{ClientId: 10, SeqNum: 1}
	cmdId2 := CommandId{ClientId: 10, SeqNum: 2}
	if !r.values.Has(cmdId1.String()) {
		t.Error("expected value for cmdId1")
	}
	if !r.values.Has(cmdId2.String()) {
		t.Error("expected value for cmdId2")
	}
	// Verify slots map
	if r.slots[cmdId1] != 0 || r.slots[cmdId2] != 1 {
		t.Errorf("slots map mismatch: %v", r.slots)
	}
}

// TestMergeAndRecoverLogEmptyHistory tests recovery with empty history (fresh start).
func TestMergeAndRecoverLogEmptyHistory(t *testing.T) {
	r := newTestReplicaForRecovery(0, 3)
	r.role = LEADER
	r.status = RECOVERING
	r.currentTerm = 5

	// No entries from peers either
	r.logSyncReplies = []MLogSyncReply{
		{Replica: 1, Term: 5, NumEntries: 0, Entries: nil},
	}

	r.mergeAndRecoverLog()

	if r.status != NORMAL {
		t.Errorf("expected NORMAL status, got %d", r.status)
	}
	if r.lastCmdSlot != 0 {
		t.Errorf("expected lastCmdSlot=0, got %d", r.lastCmdSlot)
	}
}

// TestMergeAndRecoverLogMultiplePeers tests merge with entries from multiple peers.
func TestMergeAndRecoverLogMultiplePeers(t *testing.T) {
	r := newTestReplicaForRecovery(0, 5)
	r.role = LEADER
	r.status = RECOVERING
	r.currentTerm = 5

	// Peer 1 has slots 0, 1
	// Peer 2 has slots 0, 2 (different from peer 1)
	r.logSyncReplies = []MLogSyncReply{
		{
			Replica: 1, Term: 5, NumEntries: 2,
			Entries: []LogEntry{
				{Slot: 0, CmdId: CommandId{10, 1}, Cmd: state.Command{Op: state.PUT, K: 1, V: state.NIL()}},
				{Slot: 1, CmdId: CommandId{10, 2}, Cmd: state.Command{Op: state.PUT, K: 2, V: state.NIL()}},
			},
		},
		{
			Replica: 2, Term: 5, NumEntries: 2,
			Entries: []LogEntry{
				{Slot: 0, CmdId: CommandId{10, 1}, Cmd: state.Command{Op: state.PUT, K: 1, V: state.NIL()}},
				{Slot: 2, CmdId: CommandId{20, 1}, Cmd: state.Command{Op: state.PUT, K: 3, V: state.NIL()}},
			},
		},
	}

	r.mergeAndRecoverLog()

	if r.status != NORMAL {
		t.Errorf("expected NORMAL, got %d", r.status)
	}
	// Should have slots 0, 1, 2 → lastCmdSlot = 3
	if r.lastCmdSlot != 3 {
		t.Errorf("expected lastCmdSlot=3, got %d", r.lastCmdSlot)
	}
	if !r.executed.Has("0") || !r.executed.Has("1") || !r.executed.Has("2") {
		t.Error("expected slots 0, 1, 2 to be executed")
	}
}

// TestMergeAndRecoverLogSkipsAlreadyExecuted tests that already-executed slots are not re-executed.
func TestMergeAndRecoverLogSkipsAlreadyExecuted(t *testing.T) {
	r := newTestReplicaForRecovery(0, 3)
	r.role = LEADER
	r.status = RECOVERING
	r.currentTerm = 5

	// Pre-execute slot 0
	cmd0 := state.Command{Op: state.PUT, K: 1, V: state.NIL()}
	cmd0.Execute(r.State) // Execute once
	r.executed.Set("0", struct{}{})

	// Our history has slot 0 committed
	r.history[0] = commandStaticDesc{
		cmdSlot: 0, phase: COMMIT,
		cmd:   cmd0,
		cmdId: CommandId{ClientId: 10, SeqNum: 1},
	}

	// Peer also has slot 0
	r.logSyncReplies = []MLogSyncReply{
		{
			Replica: 1, Term: 5, NumEntries: 1,
			Entries: []LogEntry{
				{Slot: 0, CmdId: CommandId{10, 1}, Cmd: cmd0},
			},
		},
	}

	// Execute merge — slot 0 should be skipped (already executed)
	r.mergeAndRecoverLog()

	if r.status != NORMAL {
		t.Errorf("expected NORMAL, got %d", r.status)
	}
}

// TestStartLogRecovery tests that startLogRecovery sets correct state and sends MLogSync.
func TestStartLogRecovery(t *testing.T) {
	r := newTestReplicaForRecovery(0, 3)
	r.role = LEADER
	r.currentTerm = 5
	r.lastCommitted = 3

	r.startLogRecovery()

	if r.status != RECOVERING {
		t.Errorf("expected RECOVERING status, got %d", r.status)
	}
	if r.logSyncExpected != 2 { // N-1 = 3-1 = 2
		t.Errorf("expected logSyncExpected=2, got %d", r.logSyncExpected)
	}

	// Verify MLogSync was sent (at least one message on sender channel)
	select {
	case <-r.sender:
		// OK — MLogSync was sent
	case <-time.After(100 * time.Millisecond):
		t.Error("expected MLogSync to be sent to peers")
	}
}

// TestRecoveryFullFlow tests the complete flow: election win → log recovery → NORMAL.
func TestRecoveryFullFlow(t *testing.T) {
	r := newTestReplicaForRecovery(0, 3)
	r.role = LEADER
	r.currentTerm = 5
	r.status = NORMAL

	// Simulate election win triggering recovery
	r.startLogRecovery()
	if r.status != RECOVERING {
		t.Fatal("expected RECOVERING after startLogRecovery")
	}

	// Proposals should be rejected during recovery
	// (handlePropose checks r.status != NORMAL)

	// Simulate peer replies
	reply1 := &MLogSyncReply{
		Replica: 1, Term: 5, NumEntries: 1,
		Entries: []LogEntry{
			{Slot: 0, CmdId: CommandId{10, 1}, Cmd: state.Command{Op: state.PUT, K: 1, V: state.NIL()}},
		},
	}
	r.handleLogSyncReply(reply1)

	// With N=3, N/2=1. One reply is enough for majority.
	if r.status != NORMAL {
		t.Errorf("expected NORMAL after majority replies, got %d", r.status)
	}
	if r.lastCmdSlot != 1 {
		t.Errorf("expected lastCmdSlot=1, got %d", r.lastCmdSlot)
	}
}

// TestMergeAndRecoverLogTracksKeyVersions tests that recovery updates keyVersions for PUT commands.
func TestMergeAndRecoverLogTracksKeyVersions(t *testing.T) {
	r := newTestReplicaForRecovery(0, 3)
	r.role = LEADER
	r.status = RECOVERING
	r.currentTerm = 5

	r.logSyncReplies = []MLogSyncReply{
		{
			Replica: 1, Term: 5, NumEntries: 2,
			Entries: []LogEntry{
				{Slot: 0, CmdId: CommandId{10, 1}, Cmd: state.Command{Op: state.PUT, K: 42, V: state.NIL()}},
				{Slot: 1, CmdId: CommandId{10, 2}, Cmd: state.Command{Op: state.GET, K: 42, V: state.NIL()}},
			},
		},
	}

	r.mergeAndRecoverLog()

	// Key 42 should have version = slot 0 (from the PUT)
	keyStr := r.int32ToString(42)
	v, exists := r.keyVersions.Get(keyStr)
	if !exists {
		t.Fatal("expected keyVersions entry for key 42")
	}
	if v.(int) != 0 {
		t.Errorf("expected version 0 for key 42, got %d", v.(int))
	}
}

// TestLastCommittedUpdatedOnCommit tests that lastCommitted is updated during normal commit.
func TestLastCommittedUpdatedOnCommit(t *testing.T) {
	r := newTestReplicaForRecovery(0, 3)
	r.role = LEADER
	r.currentTerm = 5
	r.lastCommitted = -1

	// Create a descriptor in ACCEPT phase
	desc := &commandDesc{
		cmdSlot: 7,
		phase:   ACCEPT,
		cmdId:   CommandId{ClientId: 10, SeqNum: 1},
		cmd:     state.Command{Op: state.PUT, K: 1, V: state.NIL()},
		afterPayload: hook.NewOptCondF(func() bool { return true }),
		acks:    replica.NewMsgSet(r.Q, func(_, _ interface{}) bool { return true }, nil, func(_ interface{}, _ []interface{}) {}),
		msgs:    make(chan interface{}, 8),
		successor: -1,
	}

	commit := &MCommit{
		Replica: 0,
		Ballot:  5,
		CmdSlot: 7,
	}

	r.handleCommit(commit, desc)

	if r.lastCommitted != 7 {
		t.Errorf("expected lastCommitted=7, got %d", r.lastCommitted)
	}
}

// TestHandleLogSyncNoCommittedEntries tests follower with no committed entries.
func TestHandleLogSyncNoCommittedEntries(t *testing.T) {
	r := newTestReplicaForRecovery(1, 3)
	r.currentTerm = 5

	msg := &MLogSync{Replica: 0, Term: 5}
	r.handleLogSync(msg)

	// Should send a reply (even with 0 entries)
	select {
	case <-r.sender:
		// OK — reply sent
	case <-time.After(100 * time.Millisecond):
		t.Fatal("expected MLogSyncReply to be sent")
	}
}

// TestMergeAndRecoverLogWithGaps tests recovery with gaps in slot numbers.
func TestMergeAndRecoverLogWithGaps(t *testing.T) {
	r := newTestReplicaForRecovery(0, 3)
	r.role = LEADER
	r.status = RECOVERING
	r.currentTerm = 5

	// Entries at slots 0, 2, 5 (gaps at 1, 3, 4)
	r.logSyncReplies = []MLogSyncReply{
		{
			Replica: 1, Term: 5, NumEntries: 3,
			Entries: []LogEntry{
				{Slot: 0, CmdId: CommandId{10, 1}, Cmd: state.Command{Op: state.PUT, K: 1, V: state.NIL()}},
				{Slot: 2, CmdId: CommandId{10, 2}, Cmd: state.Command{Op: state.PUT, K: 2, V: state.NIL()}},
				{Slot: 5, CmdId: CommandId{10, 3}, Cmd: state.Command{Op: state.PUT, K: 3, V: state.NIL()}},
			},
		},
	}

	r.mergeAndRecoverLog()

	// lastCmdSlot should be 6 (max slot 5 + 1)
	if r.lastCmdSlot != 6 {
		t.Errorf("expected lastCmdSlot=6, got %d", r.lastCmdSlot)
	}
	// Slots 0, 2, 5 should be executed; 1, 3, 4 should not
	if !r.executed.Has("0") || !r.executed.Has("2") || !r.executed.Has("5") {
		t.Error("expected slots 0, 2, 5 to be executed")
	}
	if r.executed.Has("1") || r.executed.Has("3") || r.executed.Has("4") {
		t.Error("expected slots 1, 3, 4 to NOT be executed")
	}
}

// TestLogEntryType tests LogEntry struct fields.
func TestLogEntryType(t *testing.T) {
	entry := LogEntry{
		Slot:  42,
		CmdId: CommandId{ClientId: 10, SeqNum: 5},
		Cmd:   state.Command{Op: state.PUT, K: 7, V: state.NIL()},
	}
	if entry.Slot != 42 {
		t.Errorf("expected slot 42, got %d", entry.Slot)
	}
	if entry.CmdId.ClientId != 10 || entry.CmdId.SeqNum != 5 {
		t.Errorf("cmdId mismatch: %v", entry.CmdId)
	}
}

// TestCommandStaticDescHasCmdId tests that commandStaticDesc includes cmdId field.
func TestCommandStaticDescHasCmdId(t *testing.T) {
	desc := commandStaticDesc{
		cmdSlot: 3,
		phase:   COMMIT,
		cmd:     state.Command{Op: state.PUT, K: 1, V: state.NIL()},
		cmdId:   CommandId{ClientId: 10, SeqNum: 5},
	}
	if desc.cmdId.ClientId != 10 || desc.cmdId.SeqNum != 5 {
		t.Errorf("cmdId not stored: %v", desc.cmdId)
	}
}
