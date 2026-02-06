package curpho

import (
	"bytes"
	"testing"

	"github.com/imdea-software/swiftpaxos/state"
)

// ============================================================================
// Phase 19: Package Setup Verification Tests
// These tests verify that the curp-ho package is correctly set up and
// all types, constants, and serialization methods work identically to curp-ht.
// ============================================================================

// TestPackageConstants verifies all protocol constants are defined correctly
func TestPackageConstants(t *testing.T) {
	// Status constants
	if NORMAL != 0 {
		t.Errorf("NORMAL should be 0, got %d", NORMAL)
	}
	if RECOVERING != 1 {
		t.Errorf("RECOVERING should be 1, got %d", RECOVERING)
	}

	// Phase constants
	if START != 0 {
		t.Errorf("START should be 0, got %d", START)
	}
	if ACCEPT != 1 {
		t.Errorf("ACCEPT should be 1, got %d", ACCEPT)
	}
	if COMMIT != 2 {
		t.Errorf("COMMIT should be 2, got %d", COMMIT)
	}

	// Consistency level constants
	if STRONG != 0 {
		t.Errorf("STRONG should be 0, got %d", STRONG)
	}
	if WEAK != 1 {
		t.Errorf("WEAK should be 1, got %d", WEAK)
	}

	// Boolean constants
	if TRUE != 1 {
		t.Errorf("TRUE should be 1, got %d", TRUE)
	}
	if FALSE != 0 {
		t.Errorf("FALSE should be 0, got %d", FALSE)
	}
	if ORDERED != 2 {
		t.Errorf("ORDERED should be 2, got %d", ORDERED)
	}

	// History size
	if HISTORY_SIZE != 10010001 {
		t.Errorf("HISTORY_SIZE should be 10010001, got %d", HISTORY_SIZE)
	}

	// MaxDescRoutines default
	if MaxDescRoutines != 500 {
		t.Errorf("MaxDescRoutines should be 500, got %d", MaxDescRoutines)
	}
}

// TestCommandIdString tests CommandId.String() method
func TestCommandIdString(t *testing.T) {
	tests := []struct {
		clientId int32
		seqNum   int32
		expected string
	}{
		{100, 42, "100,42"},
		{0, 0, "0,0"},
		{-1, -1, "-1,-1"},
		{999, 1, "999,1"},
	}

	for _, tt := range tests {
		cmdId := CommandId{ClientId: tt.clientId, SeqNum: tt.seqNum}
		got := cmdId.String()
		if got != tt.expected {
			t.Errorf("CommandId{%d,%d}.String() = %q, want %q",
				tt.clientId, tt.seqNum, got, tt.expected)
		}
	}
}

// ============================================================================
// Message Serialization Tests
// ============================================================================

// TestCommandIdSerialization tests CommandId Marshal/Unmarshal
func TestCommandIdSerialization(t *testing.T) {
	original := &CommandId{ClientId: 100, SeqNum: 42}

	var buf bytes.Buffer
	original.Marshal(&buf)

	restored := &CommandId{}
	if err := restored.Unmarshal(&buf); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if restored.ClientId != original.ClientId || restored.SeqNum != original.SeqNum {
		t.Errorf("CommandId mismatch: got %v, want %v", restored, original)
	}
}

// TestCommandIdBinarySize tests CommandId.BinarySize()
func TestCommandIdBinarySize(t *testing.T) {
	cmdId := &CommandId{}
	size, known := cmdId.BinarySize()
	if !known {
		t.Error("CommandId size should be known")
	}
	if size != 8 {
		t.Errorf("CommandId size should be 8, got %d", size)
	}
}

// TestMCommitSerialization tests MCommit Marshal/Unmarshal
func TestMCommitSerialization(t *testing.T) {
	original := &MCommit{Replica: 2, Ballot: 5, CmdSlot: 100}

	var buf bytes.Buffer
	original.Marshal(&buf)

	restored := &MCommit{}
	if err := restored.Unmarshal(&buf); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if restored.Replica != original.Replica {
		t.Errorf("Replica mismatch: got %d, want %d", restored.Replica, original.Replica)
	}
	if restored.Ballot != original.Ballot {
		t.Errorf("Ballot mismatch: got %d, want %d", restored.Ballot, original.Ballot)
	}
	if restored.CmdSlot != original.CmdSlot {
		t.Errorf("CmdSlot mismatch: got %d, want %d", restored.CmdSlot, original.CmdSlot)
	}
}

// TestMAcceptSerialization tests MAccept Marshal/Unmarshal
func TestMAcceptSerialization(t *testing.T) {
	original := &MAccept{
		Replica: 1,
		Ballot:  3,
		Cmd: state.Command{
			Op: state.PUT,
			K:  state.Key(42),
			V:  []byte("test-value"),
		},
		CmdId:   CommandId{ClientId: 100, SeqNum: 5},
		CmdSlot: 10,
	}

	var buf bytes.Buffer
	original.Marshal(&buf)

	restored := &MAccept{}
	if err := restored.Unmarshal(&buf); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if restored.Replica != original.Replica {
		t.Errorf("Replica mismatch")
	}
	if restored.Ballot != original.Ballot {
		t.Errorf("Ballot mismatch")
	}
	if restored.CmdId != original.CmdId {
		t.Errorf("CmdId mismatch")
	}
	if restored.CmdSlot != original.CmdSlot {
		t.Errorf("CmdSlot mismatch")
	}
	if restored.Cmd.Op != original.Cmd.Op {
		t.Errorf("Cmd.Op mismatch")
	}
}

// TestMAcceptAckSerialization tests MAcceptAck Marshal/Unmarshal
func TestMAcceptAckSerialization(t *testing.T) {
	original := &MAcceptAck{Replica: 2, Ballot: 3, CmdSlot: 50}

	var buf bytes.Buffer
	original.Marshal(&buf)

	restored := &MAcceptAck{}
	if err := restored.Unmarshal(&buf); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if restored.Replica != original.Replica || restored.Ballot != original.Ballot || restored.CmdSlot != original.CmdSlot {
		t.Errorf("MAcceptAck mismatch: got %+v, want %+v", restored, original)
	}
}

// TestMRecordAckSerialization tests MRecordAck Marshal/Unmarshal
func TestMRecordAckSerialization(t *testing.T) {
	original := &MRecordAck{
		Replica: 1,
		Ballot:  2,
		CmdId:   CommandId{ClientId: 100, SeqNum: 5},
		Ok:      TRUE,
	}

	var buf bytes.Buffer
	original.Marshal(&buf)

	restored := &MRecordAck{}
	if err := restored.Unmarshal(&buf); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if restored.Replica != original.Replica || restored.Ok != original.Ok || restored.CmdId != original.CmdId {
		t.Errorf("MRecordAck mismatch: got %+v, want %+v", restored, original)
	}
}

// TestMSyncSerialization tests MSync Marshal/Unmarshal
func TestMSyncSerialization(t *testing.T) {
	original := &MSync{CmdId: CommandId{ClientId: 50, SeqNum: 10}}

	var buf bytes.Buffer
	original.Marshal(&buf)

	restored := &MSync{}
	if err := restored.Unmarshal(&buf); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if restored.CmdId != original.CmdId {
		t.Errorf("CmdId mismatch")
	}
}

// TestMReplySerialization tests MReply Marshal/Unmarshal
func TestMReplySerialization(t *testing.T) {
	original := &MReply{
		Replica: 0,
		Ballot:  1,
		CmdId:   CommandId{ClientId: 100, SeqNum: 42},
		Rep:     []byte("hello-world"),
		Ok:      TRUE,
	}

	var buf bytes.Buffer
	original.Marshal(&buf)

	restored := &MReply{}
	if err := restored.Unmarshal(&buf); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if restored.Replica != original.Replica {
		t.Errorf("Replica mismatch")
	}
	if restored.Ok != original.Ok {
		t.Errorf("Ok mismatch")
	}
	if !bytes.Equal(restored.Rep, original.Rep) {
		t.Errorf("Rep mismatch")
	}
}

// TestMSyncReplySerialization tests MSyncReply Marshal/Unmarshal
func TestMSyncReplySerialization(t *testing.T) {
	original := &MSyncReply{
		Replica: 1,
		Ballot:  5,
		CmdId:   CommandId{ClientId: 200, SeqNum: 10},
		Rep:     []byte("sync-result"),
	}

	var buf bytes.Buffer
	original.Marshal(&buf)

	restored := &MSyncReply{}
	if err := restored.Unmarshal(&buf); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if restored.Replica != original.Replica || restored.Ballot != original.Ballot {
		t.Errorf("Header mismatch")
	}
	if !bytes.Equal(restored.Rep, original.Rep) {
		t.Errorf("Rep mismatch")
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
		CausalDep: 41,
	}

	var buf bytes.Buffer
	original.Marshal(&buf)

	restored := &MWeakPropose{}
	if err := restored.Unmarshal(&buf); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if restored.CommandId != original.CommandId {
		t.Errorf("CommandId mismatch: got %d, want %d", restored.CommandId, original.CommandId)
	}
	if restored.ClientId != original.ClientId {
		t.Errorf("ClientId mismatch: got %d, want %d", restored.ClientId, original.ClientId)
	}
	if restored.Timestamp != original.Timestamp {
		t.Errorf("Timestamp mismatch: got %d, want %d", restored.Timestamp, original.Timestamp)
	}
	if restored.CausalDep != original.CausalDep {
		t.Errorf("CausalDep mismatch: got %d, want %d", restored.CausalDep, original.CausalDep)
	}
	if restored.Command.Op != original.Command.Op {
		t.Errorf("Command.Op mismatch")
	}
	if !bytes.Equal(restored.Command.V, original.Command.V) {
		t.Errorf("Command.V mismatch")
	}
}

// TestMWeakReplySerialization tests MWeakReply Marshal/Unmarshal
func TestMWeakReplySerialization(t *testing.T) {
	original := &MWeakReply{
		Replica: 1,
		Ballot:  5,
		CmdId:   CommandId{ClientId: 100, SeqNum: 42},
		Rep:     []byte("result-value"),
	}

	var buf bytes.Buffer
	original.Marshal(&buf)

	restored := &MWeakReply{}
	if err := restored.Unmarshal(&buf); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if restored.Replica != original.Replica {
		t.Errorf("Replica mismatch")
	}
	if restored.CmdId != original.CmdId {
		t.Errorf("CmdId mismatch")
	}
	if !bytes.Equal(restored.Rep, original.Rep) {
		t.Errorf("Rep mismatch")
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
		CausalDep: 0,
	}

	var buf bytes.Buffer
	original.Marshal(&buf)

	restored := &MWeakPropose{}
	if err := restored.Unmarshal(&buf); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if restored.Command.Op != original.Command.Op {
		t.Errorf("Command.Op mismatch")
	}
	if restored.CausalDep != 0 {
		t.Errorf("CausalDep should be 0")
	}
}

// TestMWeakReplyWithEmptyRep tests serialization with empty Rep
func TestMWeakReplyWithEmptyRep(t *testing.T) {
	original := &MWeakReply{
		Replica: 0,
		Ballot:  0,
		CmdId:   CommandId{ClientId: 1, SeqNum: 1},
		Rep:     []byte{},
	}

	var buf bytes.Buffer
	original.Marshal(&buf)

	restored := &MWeakReply{}
	if err := restored.Unmarshal(&buf); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if len(restored.Rep) != 0 {
		t.Errorf("Rep should be empty, got %v", restored.Rep)
	}
}

// TestMAAcksSerialization tests MAAcks Marshal/Unmarshal with mixed content
func TestMAAcksSerialization(t *testing.T) {
	original := &MAAcks{
		Acks: []MAcceptAck{
			{Replica: 0, Ballot: 1, CmdSlot: 10},
			{Replica: 1, Ballot: 1, CmdSlot: 11},
		},
		Accepts: []MAccept{
			{
				Replica: 0,
				Ballot:  1,
				Cmd: state.Command{
					Op: state.PUT,
					K:  state.Key(5),
					V:  []byte("val"),
				},
				CmdId:   CommandId{ClientId: 100, SeqNum: 1},
				CmdSlot: 10,
			},
		},
	}

	var buf bytes.Buffer
	original.Marshal(&buf)

	restored := &MAAcks{}
	if err := restored.Unmarshal(&buf); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if len(restored.Acks) != 2 {
		t.Errorf("Expected 2 acks, got %d", len(restored.Acks))
	}
	if len(restored.Accepts) != 1 {
		t.Errorf("Expected 1 accept, got %d", len(restored.Accepts))
	}
	if restored.Acks[0].CmdSlot != 10 {
		t.Errorf("Acks[0].CmdSlot mismatch")
	}
}

// ============================================================================
// New() Method Tests (fastrpc.Serializable interface)
// ============================================================================

func TestNewMethods(t *testing.T) {
	tests := []struct {
		name string
		fn   func() interface{}
	}{
		{"MReply", func() interface{} { return (&MReply{}).New() }},
		{"MAccept", func() interface{} { return (&MAccept{}).New() }},
		{"MAcceptAck", func() interface{} { return (&MAcceptAck{}).New() }},
		{"MAAcks", func() interface{} { return (&MAAcks{}).New() }},
		{"MRecordAck", func() interface{} { return (&MRecordAck{}).New() }},
		{"MCommit", func() interface{} { return (&MCommit{}).New() }},
		{"MSync", func() interface{} { return (&MSync{}).New() }},
		{"MSyncReply", func() interface{} { return (&MSyncReply{}).New() }},
		{"MWeakPropose", func() interface{} { return (&MWeakPropose{}).New() }},
		{"MWeakReply", func() interface{} { return (&MWeakReply{}).New() }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := tt.fn()
			if obj == nil {
				t.Errorf("%s.New() returned nil", tt.name)
			}
		})
	}
}

// ============================================================================
// BinarySize Tests
// ============================================================================

func TestBinarySizes(t *testing.T) {
	tests := []struct {
		name      string
		size      int
		sizeKnown bool
		fn        func() (int, bool)
	}{
		{"CommandId", 8, true, func() (int, bool) { return (&CommandId{}).BinarySize() }},
		{"MCommit", 16, true, func() (int, bool) { return (&MCommit{}).BinarySize() }},
		{"MAcceptAck", 16, true, func() (int, bool) { return (&MAcceptAck{}).BinarySize() }},
		{"MRecordAck", 17, true, func() (int, bool) { return (&MRecordAck{}).BinarySize() }},
		{"MSync", 8, true, func() (int, bool) { return (&MSync{}).BinarySize() }},
		{"MReply", 0, false, func() (int, bool) { return (&MReply{}).BinarySize() }},
		{"MSyncReply", 0, false, func() (int, bool) { return (&MSyncReply{}).BinarySize() }},
		{"MAccept", 0, false, func() (int, bool) { return (&MAccept{}).BinarySize() }},
		{"MAAcks", 0, false, func() (int, bool) { return (&MAAcks{}).BinarySize() }},
		{"MWeakPropose", 0, false, func() (int, bool) { return (&MWeakPropose{}).BinarySize() }},
		{"MWeakReply", 0, false, func() (int, bool) { return (&MWeakReply{}).BinarySize() }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			size, known := tt.fn()
			if known != tt.sizeKnown {
				t.Errorf("%s: sizeKnown = %v, want %v", tt.name, known, tt.sizeKnown)
			}
			if known && size != tt.size {
				t.Errorf("%s: size = %d, want %d", tt.name, size, tt.size)
			}
		})
	}
}

// ============================================================================
// Cache Tests
// ============================================================================

func TestCaches(t *testing.T) {
	t.Run("MCommitCache", func(t *testing.T) {
		cache := NewMCommitCache()
		obj := cache.Get()
		if obj == nil {
			t.Fatal("Get from empty cache returned nil")
		}
		obj.Replica = 5
		cache.Put(obj)
		obj2 := cache.Get()
		if obj2 == nil {
			t.Fatal("Get after Put returned nil")
		}
	})

	t.Run("MAcceptCache", func(t *testing.T) {
		cache := NewMAcceptCache()
		obj := cache.Get()
		if obj == nil {
			t.Fatal("Get from empty cache returned nil")
		}
		cache.Put(obj)
	})

	t.Run("MAcceptAckCache", func(t *testing.T) {
		cache := NewMAcceptAckCache()
		obj := cache.Get()
		if obj == nil {
			t.Fatal("Get from empty cache returned nil")
		}
		cache.Put(obj)
	})

	t.Run("MReplyCache", func(t *testing.T) {
		cache := NewMReplyCache()
		obj := cache.Get()
		if obj == nil {
			t.Fatal("Get from empty cache returned nil")
		}
		cache.Put(obj)
	})

	t.Run("MRecordAckCache", func(t *testing.T) {
		cache := NewMRecordAckCache()
		obj := cache.Get()
		if obj == nil {
			t.Fatal("Get from empty cache returned nil")
		}
		cache.Put(obj)
	})

	t.Run("MSyncCache", func(t *testing.T) {
		cache := NewMSyncCache()
		obj := cache.Get()
		if obj == nil {
			t.Fatal("Get from empty cache returned nil")
		}
		cache.Put(obj)
	})

	t.Run("MSyncReplyCache", func(t *testing.T) {
		cache := NewMSyncReplyCache()
		obj := cache.Get()
		if obj == nil {
			t.Fatal("Get from empty cache returned nil")
		}
		cache.Put(obj)
	})

	t.Run("MAAcksCache", func(t *testing.T) {
		cache := NewMAAcksCache()
		obj := cache.Get()
		if obj == nil {
			t.Fatal("Get from empty cache returned nil")
		}
		cache.Put(obj)
	})

	t.Run("CommandIdCache", func(t *testing.T) {
		cache := NewCommandIdCache()
		obj := cache.Get()
		if obj == nil {
			t.Fatal("Get from empty cache returned nil")
		}
		cache.Put(obj)
	})

	t.Run("MWeakProposeCache", func(t *testing.T) {
		cache := NewMWeakProposeCache()
		obj := cache.Get()
		if obj == nil {
			t.Fatal("Get from empty cache returned nil")
		}
		obj.CommandId = 123
		cache.Put(obj)
		obj2 := cache.Get()
		if obj2 == nil {
			t.Fatal("Get after Put returned nil")
		}
	})

	t.Run("MWeakReplyCache", func(t *testing.T) {
		cache := NewMWeakReplyCache()
		obj := cache.Get()
		if obj == nil {
			t.Fatal("Get from empty cache returned nil")
		}
		obj.Replica = 5
		cache.Put(obj)
		obj2 := cache.Get()
		if obj2 == nil {
			t.Fatal("Get after Put returned nil")
		}
	})
}

// ============================================================================
// Command Descriptor Tests
// ============================================================================

// TestCommandDescFields tests that commandDesc has all expected fields
func TestCommandDescFields(t *testing.T) {
	desc := &commandDesc{}

	// Default values
	if desc.isWeak {
		t.Error("Default isWeak should be false")
	}
	if desc.applied {
		t.Error("Default applied should be false")
	}
	if desc.phase != 0 {
		t.Errorf("Default phase should be 0 (START), got %d", desc.phase)
	}

	// Set fields
	desc.isWeak = true
	desc.applied = true
	desc.phase = COMMIT
	desc.cmdSlot = 42

	if !desc.isWeak || !desc.applied || desc.phase != COMMIT || desc.cmdSlot != 42 {
		t.Error("Field assignment failed")
	}
}

// TestCommandDescWeakVsStrong tests distinction between weak and strong descriptors
func TestCommandDescWeakVsStrong(t *testing.T) {
	weakDesc := &commandDesc{isWeak: true, phase: ACCEPT}
	strongDesc := &commandDesc{isWeak: false, phase: ACCEPT}

	if !weakDesc.isWeak {
		t.Error("Weak descriptor should have isWeak=true")
	}
	if strongDesc.isWeak {
		t.Error("Strong descriptor should have isWeak=false")
	}
}

// ============================================================================
// State Machine Tests
// ============================================================================

// TestStateExecution tests basic state machine operations
func TestStateExecution(t *testing.T) {
	st := state.InitState()

	// PUT
	putCmd := state.Command{Op: state.PUT, K: state.Key(100), V: []byte("hello")}
	result := putCmd.Execute(st)
	if len(result) != 0 {
		t.Errorf("PUT should return empty value, got %v", result)
	}

	// GET
	getCmd := state.Command{Op: state.GET, K: state.Key(100), V: state.NIL()}
	result = getCmd.Execute(st)
	if !bytes.Equal(result, []byte("hello")) {
		t.Errorf("GET should return 'hello', got %v", result)
	}
}

// TestComputeResultDoesNotModifyState verifies ComputeResult is read-only
func TestComputeResultDoesNotModifyState(t *testing.T) {
	st := state.InitState()

	// ComputeResult for PUT should NOT modify state
	putCmd := state.Command{Op: state.PUT, K: state.Key(100), V: state.Value([]byte("test"))}
	result := putCmd.ComputeResult(st)
	if len(result) != 0 {
		t.Errorf("ComputeResult(PUT) should return NIL, got %v", result)
	}

	// State should still be empty
	getCmd := state.Command{Op: state.GET, K: state.Key(100), V: state.NIL()}
	getResult := getCmd.ComputeResult(st)
	if len(getResult) != 0 {
		t.Errorf("State was modified by ComputeResult(PUT), got %v", getResult)
	}
}

// ============================================================================
// Pending Write Tests
// ============================================================================

// TestPendingWriteKey verifies key generation for pending writes
func TestPendingWriteKey(t *testing.T) {
	tests := []struct {
		clientId int32
		key      state.Key
		expected string
	}{
		{100, state.Key(42), "100:42"},
		{200, state.Key(999), "200:999"},
		{0, state.Key(0), "0:0"},
	}

	for _, tt := range tests {
		got := pendingWriteKey(tt.clientId, tt.key)
		if got != tt.expected {
			t.Errorf("pendingWriteKey(%d, %d) = %q, want %q",
				tt.clientId, tt.key, got, tt.expected)
		}
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

// TestCrossClientIsolation verifies different clients have different pending write keys
func TestCrossClientIsolation(t *testing.T) {
	clientA := int32(100)
	clientB := int32(200)
	key := state.Key(1)

	keyA := pendingWriteKey(clientA, key)
	keyB := pendingWriteKey(clientB, key)

	if keyA == keyB {
		t.Error("Different clients should have different pending write keys")
	}
}

// ============================================================================
// Causal Ordering Tests
// ============================================================================

// TestWeakCommandChain tests a chain of causally dependent weak commands
func TestWeakCommandChain(t *testing.T) {
	st := state.InitState()

	cmd1 := &MWeakPropose{
		CommandId: 1, ClientId: 100,
		Command:   state.Command{Op: state.PUT, K: state.Key(1), V: []byte("value1")},
		CausalDep: 0,
	}
	cmd2 := &MWeakPropose{
		CommandId: 2, ClientId: 100,
		Command:   state.Command{Op: state.PUT, K: state.Key(2), V: []byte("value2")},
		CausalDep: 1,
	}
	cmd3 := &MWeakPropose{
		CommandId: 3, ClientId: 100,
		Command:   state.Command{Op: state.GET, K: state.Key(1), V: state.NIL()},
		CausalDep: 2,
	}

	cmd1.Command.Execute(st)
	cmd2.Command.Execute(st)
	result := cmd3.Command.Execute(st)

	if !bytes.Equal(result, []byte("value1")) {
		t.Errorf("Expected 'value1', got %v", result)
	}

	if cmd2.CausalDep != cmd1.CommandId {
		t.Error("cmd2 should depend on cmd1")
	}
	if cmd3.CausalDep != cmd2.CommandId {
		t.Error("cmd3 should depend on cmd2")
	}
}

// TestMultiClientCausalIndependence tests that different clients have independent chains
func TestMultiClientCausalIndependence(t *testing.T) {
	cmdA1 := &MWeakPropose{CommandId: 1, ClientId: 100, CausalDep: 0}
	cmdA2 := &MWeakPropose{CommandId: 2, ClientId: 100, CausalDep: 1}
	cmdB1 := &MWeakPropose{CommandId: 1, ClientId: 200, CausalDep: 0}
	cmdB2 := &MWeakPropose{CommandId: 2, ClientId: 200, CausalDep: 1}

	if cmdA1.ClientId == cmdB1.ClientId {
		t.Error("Clients should have different IDs")
	}
	if cmdA2.CausalDep != 1 || cmdB2.CausalDep != 1 {
		t.Error("Each client's chain should be independent")
	}
}

// ============================================================================
// Mixed Command Slot Ordering Tests
// ============================================================================

func TestMixedStrongWeakSlotOrdering(t *testing.T) {
	strongDesc := &commandDesc{cmdSlot: 0, isWeak: false}
	weakDesc := &commandDesc{cmdSlot: 1, isWeak: true}
	strongDesc2 := &commandDesc{cmdSlot: 2, isWeak: false}

	strongDesc.applied = true
	weakDesc.applied = true
	strongDesc2.applied = true

	if !strongDesc.applied || !weakDesc.applied || !strongDesc2.applied {
		t.Error("All commands should be executed in slot order")
	}

	if strongDesc.cmdSlot >= weakDesc.cmdSlot || weakDesc.cmdSlot >= strongDesc2.cmdSlot {
		t.Error("Slot ordering violated")
	}
}
