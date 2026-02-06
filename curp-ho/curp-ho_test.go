package curpho

import (
	"bytes"
	"testing"

	"github.com/imdea-software/swiftpaxos/state"
	cmap "github.com/orcaman/concurrent-map"
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

// ============================================================================
// Phase 20: Unsynced Entry / Witness Pool Tests
// These tests verify the UnsyncedEntry structure and the updated unsynced
// operations (unsyncStrong, unsyncCausal, leaderUnsyncStrong, leaderUnsyncCausal,
// sync, syncLeader, ok, okWithWeakDep) and conflict checking functions.
// ============================================================================

// newTestReplica creates a minimal Replica for testing unsynced operations.
// It only initializes fields needed for unsynced/conflict logic.
func newTestReplica(isLeader bool) *Replica {
	return &Replica{
		isLeader:     isLeader,
		unsynced:     cmap.New(),
		synced:       cmap.New(),
		boundClients: make(map[int32]bool),
	}
}

// --- UnsyncedEntry struct tests ---

func TestUnsyncedEntryCreation(t *testing.T) {
	entry := &UnsyncedEntry{
		Slot:     5,
		IsStrong: true,
		Op:       state.PUT,
		Value:    state.Value([]byte("hello")),
		ClientId: 42,
		SeqNum:   7,
		CmdId:    CommandId{ClientId: 42, SeqNum: 7},
	}

	if entry.Slot != 5 {
		t.Errorf("Slot = %d, want 5", entry.Slot)
	}
	if !entry.IsStrong {
		t.Error("IsStrong should be true")
	}
	if entry.Op != state.PUT {
		t.Errorf("Op = %d, want PUT", entry.Op)
	}
	if string(entry.Value) != "hello" {
		t.Errorf("Value = %q, want hello", entry.Value)
	}
	if entry.ClientId != 42 || entry.SeqNum != 7 {
		t.Errorf("ClientId/SeqNum = %d/%d, want 42/7", entry.ClientId, entry.SeqNum)
	}
	if entry.CmdId.ClientId != 42 || entry.CmdId.SeqNum != 7 {
		t.Errorf("CmdId = %v, want {42,7}", entry.CmdId)
	}
}

func TestUnsyncedEntryCausal(t *testing.T) {
	entry := &UnsyncedEntry{
		Slot:     1,
		IsStrong: false,
		Op:       state.GET,
		Value:    state.NIL(),
		ClientId: 10,
		SeqNum:   3,
		CmdId:    CommandId{ClientId: 10, SeqNum: 3},
	}
	if entry.IsStrong {
		t.Error("IsStrong should be false for causal entry")
	}
	if entry.Op != state.GET {
		t.Errorf("Op = %d, want GET", entry.Op)
	}
}

// --- unsyncStrong tests (non-leader) ---

func TestUnsyncStrongFirstEntry(t *testing.T) {
	r := newTestReplica(false)
	cmd := state.Command{Op: state.PUT, K: state.Key(100), V: state.Value([]byte("v1"))}
	cmdId := CommandId{ClientId: 1, SeqNum: 1}

	r.unsyncStrong(cmd, cmdId)

	v, exists := r.unsynced.Get("100")
	if !exists {
		t.Fatal("Expected entry for key 100")
	}
	entry := v.(*UnsyncedEntry)
	if entry.Slot != 1 {
		t.Errorf("Slot = %d, want 1 (count)", entry.Slot)
	}
	if !entry.IsStrong {
		t.Error("IsStrong should be true")
	}
	if entry.Op != state.PUT {
		t.Errorf("Op = %d, want PUT", entry.Op)
	}
	if entry.CmdId != cmdId {
		t.Errorf("CmdId = %v, want %v", entry.CmdId, cmdId)
	}
}

func TestUnsyncStrongMultiple(t *testing.T) {
	r := newTestReplica(false)
	cmd1 := state.Command{Op: state.PUT, K: state.Key(100), V: state.Value([]byte("v1"))}
	cmd2 := state.Command{Op: state.PUT, K: state.Key(100), V: state.Value([]byte("v2"))}
	cmdId1 := CommandId{ClientId: 1, SeqNum: 1}
	cmdId2 := CommandId{ClientId: 2, SeqNum: 2}

	r.unsyncStrong(cmd1, cmdId1)
	r.unsyncStrong(cmd2, cmdId2)

	v, _ := r.unsynced.Get("100")
	entry := v.(*UnsyncedEntry)
	// Count should be 2 (two pending ops)
	if entry.Slot != 2 {
		t.Errorf("Slot = %d, want 2 (count of pending ops)", entry.Slot)
	}
	// Metadata should reflect latest entry
	if entry.CmdId != cmdId2 {
		t.Errorf("CmdId = %v, want %v (latest)", entry.CmdId, cmdId2)
	}
}

func TestUnsyncStrongDifferentKeys(t *testing.T) {
	r := newTestReplica(false)
	cmd1 := state.Command{Op: state.PUT, K: state.Key(100), V: state.Value([]byte("v1"))}
	cmd2 := state.Command{Op: state.PUT, K: state.Key(200), V: state.Value([]byte("v2"))}
	cmdId1 := CommandId{ClientId: 1, SeqNum: 1}
	cmdId2 := CommandId{ClientId: 1, SeqNum: 2}

	r.unsyncStrong(cmd1, cmdId1)
	r.unsyncStrong(cmd2, cmdId2)

	v1, _ := r.unsynced.Get("100")
	v2, _ := r.unsynced.Get("200")
	if v1.(*UnsyncedEntry).Slot != 1 || v2.(*UnsyncedEntry).Slot != 1 {
		t.Error("Each key should have count 1")
	}
}

// --- unsyncCausal tests (all replicas) ---

func TestUnsyncCausalFirstEntry(t *testing.T) {
	r := newTestReplica(false)
	cmd := state.Command{Op: state.PUT, K: state.Key(50), V: state.Value([]byte("weak"))}
	cmdId := CommandId{ClientId: 10, SeqNum: 5}

	r.unsyncCausal(cmd, cmdId)

	v, exists := r.unsynced.Get("50")
	if !exists {
		t.Fatal("Expected entry for key 50")
	}
	entry := v.(*UnsyncedEntry)
	if entry.Slot != 1 {
		t.Errorf("Slot = %d, want 1", entry.Slot)
	}
	if entry.IsStrong {
		t.Error("IsStrong should be false for causal entry")
	}
	if entry.CmdId != cmdId {
		t.Errorf("CmdId = %v, want %v", entry.CmdId, cmdId)
	}
}

func TestUnsyncCausalMultiple(t *testing.T) {
	r := newTestReplica(false)
	cmd1 := state.Command{Op: state.PUT, K: state.Key(50), V: state.Value([]byte("w1"))}
	cmd2 := state.Command{Op: state.PUT, K: state.Key(50), V: state.Value([]byte("w2"))}
	cmdId1 := CommandId{ClientId: 10, SeqNum: 1}
	cmdId2 := CommandId{ClientId: 10, SeqNum: 2}

	r.unsyncCausal(cmd1, cmdId1)
	r.unsyncCausal(cmd2, cmdId2)

	v, _ := r.unsynced.Get("50")
	entry := v.(*UnsyncedEntry)
	if entry.Slot != 2 {
		t.Errorf("Slot = %d, want 2", entry.Slot)
	}
	// Latest entry metadata
	if entry.CmdId != cmdId2 {
		t.Errorf("CmdId = %v, want %v", entry.CmdId, cmdId2)
	}
	if string(entry.Value) != "w2" {
		t.Errorf("Value = %q, want w2", entry.Value)
	}
}

func TestUnsyncMixedStrongAndCausal(t *testing.T) {
	r := newTestReplica(false)
	cmdStrong := state.Command{Op: state.PUT, K: state.Key(100), V: state.Value([]byte("strong"))}
	cmdWeak := state.Command{Op: state.PUT, K: state.Key(100), V: state.Value([]byte("weak"))}
	cmdIdStrong := CommandId{ClientId: 1, SeqNum: 1}
	cmdIdWeak := CommandId{ClientId: 2, SeqNum: 2}

	r.unsyncStrong(cmdStrong, cmdIdStrong)
	r.unsyncCausal(cmdWeak, cmdIdWeak)

	v, _ := r.unsynced.Get("100")
	entry := v.(*UnsyncedEntry)
	if entry.Slot != 2 {
		t.Errorf("Slot = %d, want 2 (strong + causal)", entry.Slot)
	}
	// Latest entry should be causal
	if entry.IsStrong {
		t.Error("Latest entry should be causal")
	}
	if entry.CmdId != cmdIdWeak {
		t.Errorf("CmdId = %v, want %v", entry.CmdId, cmdIdWeak)
	}
}

// --- leaderUnsyncStrong tests (leader) ---

func TestLeaderUnsyncStrongFirstEntry(t *testing.T) {
	r := newTestReplica(true)
	cmd := state.Command{Op: state.PUT, K: state.Key(100), V: state.Value([]byte("v1"))}
	cmdId := CommandId{ClientId: 1, SeqNum: 1}

	dep := r.leaderUnsyncStrong(cmd, 0, cmdId)
	if dep != -1 {
		t.Errorf("dep = %d, want -1 (no previous)", dep)
	}

	v, _ := r.unsynced.Get("100")
	entry := v.(*UnsyncedEntry)
	if entry.Slot != 0 {
		t.Errorf("Slot = %d, want 0", entry.Slot)
	}
	if !entry.IsStrong {
		t.Error("IsStrong should be true")
	}
}

func TestLeaderUnsyncStrongDependency(t *testing.T) {
	r := newTestReplica(true)
	cmd1 := state.Command{Op: state.PUT, K: state.Key(100), V: state.Value([]byte("v1"))}
	cmd2 := state.Command{Op: state.PUT, K: state.Key(100), V: state.Value([]byte("v2"))}
	cmdId1 := CommandId{ClientId: 1, SeqNum: 1}
	cmdId2 := CommandId{ClientId: 2, SeqNum: 2}

	r.leaderUnsyncStrong(cmd1, 0, cmdId1)
	dep := r.leaderUnsyncStrong(cmd2, 5, cmdId2)

	// Second op should depend on first (slot 0)
	if dep != 0 {
		t.Errorf("dep = %d, want 0 (slot of first op)", dep)
	}

	v, _ := r.unsynced.Get("100")
	entry := v.(*UnsyncedEntry)
	if entry.Slot != 5 {
		t.Errorf("Slot = %d, want 5 (latest slot)", entry.Slot)
	}
	if entry.CmdId != cmdId2 {
		t.Errorf("CmdId = %v, want %v", entry.CmdId, cmdId2)
	}
}

func TestLeaderUnsyncStrongNoDep(t *testing.T) {
	r := newTestReplica(true)
	cmd1 := state.Command{Op: state.PUT, K: state.Key(100), V: state.Value([]byte("v1"))}
	cmd2 := state.Command{Op: state.PUT, K: state.Key(200), V: state.Value([]byte("v2"))}
	cmdId1 := CommandId{ClientId: 1, SeqNum: 1}
	cmdId2 := CommandId{ClientId: 1, SeqNum: 2}

	r.leaderUnsyncStrong(cmd1, 0, cmdId1)
	dep := r.leaderUnsyncStrong(cmd2, 1, cmdId2)

	// Different keys → no dependency
	if dep != -1 {
		t.Errorf("dep = %d, want -1 (different keys)", dep)
	}
}

// --- leaderUnsyncCausal tests (leader) ---

func TestLeaderUnsyncCausalEntry(t *testing.T) {
	r := newTestReplica(true)
	cmd := state.Command{Op: state.PUT, K: state.Key(50), V: state.Value([]byte("weak"))}
	cmdId := CommandId{ClientId: 10, SeqNum: 5}

	dep := r.leaderUnsyncCausal(cmd, 3, cmdId)
	if dep != -1 {
		t.Errorf("dep = %d, want -1 (first entry)", dep)
	}

	v, _ := r.unsynced.Get("50")
	entry := v.(*UnsyncedEntry)
	if entry.IsStrong {
		t.Error("IsStrong should be false for causal")
	}
	if entry.Slot != 3 {
		t.Errorf("Slot = %d, want 3", entry.Slot)
	}
}

func TestLeaderUnsyncCausalDependency(t *testing.T) {
	r := newTestReplica(true)
	cmd1 := state.Command{Op: state.PUT, K: state.Key(50), V: state.Value([]byte("w1"))}
	cmd2 := state.Command{Op: state.PUT, K: state.Key(50), V: state.Value([]byte("w2"))}
	cmdId1 := CommandId{ClientId: 10, SeqNum: 1}
	cmdId2 := CommandId{ClientId: 10, SeqNum: 2}

	r.leaderUnsyncCausal(cmd1, 0, cmdId1)
	dep := r.leaderUnsyncCausal(cmd2, 5, cmdId2)

	if dep != 0 {
		t.Errorf("dep = %d, want 0", dep)
	}
}

// --- sync tests (non-leader cleanup) ---

func TestSyncDecrementsCount(t *testing.T) {
	r := newTestReplica(false)
	cmd := state.Command{Op: state.PUT, K: state.Key(100), V: state.Value([]byte("v1"))}
	cmdId1 := CommandId{ClientId: 1, SeqNum: 1}
	cmdId2 := CommandId{ClientId: 2, SeqNum: 2}

	// Add two strong entries for key 100
	r.unsyncStrong(cmd, cmdId1)
	r.unsyncStrong(cmd, cmdId2)

	v, _ := r.unsynced.Get("100")
	if v.(*UnsyncedEntry).Slot != 2 {
		t.Fatalf("Count before sync = %d, want 2", v.(*UnsyncedEntry).Slot)
	}

	// Sync first → count should be 1
	r.sync(cmdId1, cmd)
	v, _ = r.unsynced.Get("100")
	if v.(*UnsyncedEntry).Slot != 1 {
		t.Errorf("Count after first sync = %d, want 1", v.(*UnsyncedEntry).Slot)
	}

	// Sync second → count should be 0
	r.sync(cmdId2, cmd)
	v, _ = r.unsynced.Get("100")
	if v.(*UnsyncedEntry).Slot != 0 {
		t.Errorf("Count after second sync = %d, want 0", v.(*UnsyncedEntry).Slot)
	}
}

func TestSyncIdempotent(t *testing.T) {
	r := newTestReplica(false)
	cmd := state.Command{Op: state.PUT, K: state.Key(100), V: state.Value([]byte("v1"))}
	cmdId := CommandId{ClientId: 1, SeqNum: 1}

	r.unsyncStrong(cmd, cmdId)

	// Sync twice with same cmdId → should only decrement once
	r.sync(cmdId, cmd)
	r.sync(cmdId, cmd)

	v, _ := r.unsynced.Get("100")
	if v.(*UnsyncedEntry).Slot != 0 {
		t.Errorf("Count = %d, want 0 (idempotent sync)", v.(*UnsyncedEntry).Slot)
	}
}

func TestSyncSkipsLeader(t *testing.T) {
	r := newTestReplica(true) // Leader
	cmd := state.Command{Op: state.PUT, K: state.Key(100), V: state.Value([]byte("v1"))}
	cmdId := CommandId{ClientId: 1, SeqNum: 1}

	r.leaderUnsyncStrong(cmd, 0, cmdId)

	// sync() on leader should be a no-op
	r.sync(cmdId, cmd)

	v, exists := r.unsynced.Get("100")
	if !exists {
		t.Fatal("Leader's unsynced should not be modified by sync()")
	}
	entry := v.(*UnsyncedEntry)
	if entry.Slot != 0 {
		t.Errorf("Leader slot should remain 0, got %d", entry.Slot)
	}
}

// --- syncLeader tests ---

func TestSyncLeaderRemovesMatchingEntry(t *testing.T) {
	r := newTestReplica(true)
	cmd := state.Command{Op: state.PUT, K: state.Key(100), V: state.Value([]byte("v1"))}
	cmdId := CommandId{ClientId: 1, SeqNum: 1}

	r.leaderUnsyncStrong(cmd, 0, cmdId)
	r.syncLeader(cmdId, cmd)

	_, exists := r.unsynced.Get("100")
	if exists {
		t.Error("syncLeader should remove the entry when CmdId matches")
	}
}

func TestSyncLeaderPreservesNewerEntry(t *testing.T) {
	r := newTestReplica(true)
	cmd := state.Command{Op: state.PUT, K: state.Key(100), V: state.Value([]byte("v1"))}
	cmdId1 := CommandId{ClientId: 1, SeqNum: 1}
	cmdId2 := CommandId{ClientId: 2, SeqNum: 2}

	r.leaderUnsyncStrong(cmd, 0, cmdId1)
	r.leaderUnsyncStrong(cmd, 5, cmdId2)

	// Sync the first (older) cmdId → should NOT remove because the entry was overwritten
	r.syncLeader(cmdId1, cmd)

	v, exists := r.unsynced.Get("100")
	if !exists {
		t.Fatal("syncLeader should preserve entry with newer CmdId")
	}
	if v.(*UnsyncedEntry).CmdId != cmdId2 {
		t.Errorf("Entry should still be cmdId2, got %v", v.(*UnsyncedEntry).CmdId)
	}
}

// --- ok tests ---

func TestOkNoEntry(t *testing.T) {
	r := newTestReplica(false)
	cmd := state.Command{Op: state.PUT, K: state.Key(100), V: state.NIL()}

	result := r.ok(cmd)
	if result != TRUE {
		t.Errorf("ok = %d, want TRUE (no entry)", result)
	}
}

func TestOkStrongWriteConflict(t *testing.T) {
	r := newTestReplica(false)
	cmd := state.Command{Op: state.PUT, K: state.Key(100), V: state.Value([]byte("v1"))}
	cmdId := CommandId{ClientId: 1, SeqNum: 1}

	r.unsyncStrong(cmd, cmdId)

	result := r.ok(cmd)
	if result != FALSE {
		t.Errorf("ok = %d, want FALSE (strong write conflict)", result)
	}
}

func TestOkCausalWriteNoConflict(t *testing.T) {
	r := newTestReplica(false)
	cmd := state.Command{Op: state.PUT, K: state.Key(100), V: state.Value([]byte("v1"))}
	cmdId := CommandId{ClientId: 10, SeqNum: 1}

	r.unsyncCausal(cmd, cmdId)

	// Causal (weak) entries should NOT cause conflicts for strong ops
	result := r.ok(cmd)
	if result != TRUE {
		t.Errorf("ok = %d, want TRUE (causal write should not conflict)", result)
	}
}

func TestOkZeroCount(t *testing.T) {
	r := newTestReplica(false)
	cmd := state.Command{Op: state.PUT, K: state.Key(100), V: state.Value([]byte("v1"))}
	cmdId := CommandId{ClientId: 1, SeqNum: 1}

	r.unsyncStrong(cmd, cmdId)
	r.sync(cmdId, cmd) // Count drops to 0

	result := r.ok(cmd)
	if result != TRUE {
		t.Errorf("ok = %d, want TRUE (count is 0 after sync)", result)
	}
}

// --- okWithWeakDep tests ---

func TestOkWithWeakDepNoEntry(t *testing.T) {
	r := newTestReplica(false)
	cmd := state.Command{Op: state.GET, K: state.Key(100), V: state.NIL()}

	ok, dep := r.okWithWeakDep(cmd)
	if ok != TRUE {
		t.Errorf("ok = %d, want TRUE", ok)
	}
	if dep != nil {
		t.Errorf("dep = %v, want nil", dep)
	}
}

func TestOkWithWeakDepStrongConflict(t *testing.T) {
	r := newTestReplica(false)
	cmd := state.Command{Op: state.PUT, K: state.Key(100), V: state.Value([]byte("v1"))}
	cmdId := CommandId{ClientId: 1, SeqNum: 1}

	r.unsyncStrong(cmd, cmdId)

	ok, dep := r.okWithWeakDep(cmd)
	if ok != FALSE {
		t.Errorf("ok = %d, want FALSE (strong write conflict)", ok)
	}
	if dep != nil {
		t.Errorf("dep = %v, want nil (conflict = no dep)", dep)
	}
}

func TestOkWithWeakDepCausalWrite(t *testing.T) {
	r := newTestReplica(false)
	writecmd := state.Command{Op: state.PUT, K: state.Key(100), V: state.Value([]byte("val"))}
	readcmd := state.Command{Op: state.GET, K: state.Key(100), V: state.NIL()}
	weakCmdId := CommandId{ClientId: 10, SeqNum: 5}

	// Add a causal write to key 100
	r.unsyncCausal(writecmd, weakCmdId)

	// A strong read for the same key should see the weakDep
	ok, dep := r.okWithWeakDep(readcmd)
	if ok != TRUE {
		t.Errorf("ok = %d, want TRUE", ok)
	}
	if dep == nil {
		t.Fatal("dep = nil, want weakDep pointing to the causal write")
	}
	if *dep != weakCmdId {
		t.Errorf("dep = %v, want %v", *dep, weakCmdId)
	}
}

func TestOkWithWeakDepCausalRead(t *testing.T) {
	r := newTestReplica(false)
	readCmd := state.Command{Op: state.GET, K: state.Key(100), V: state.NIL()}
	weakCmdId := CommandId{ClientId: 10, SeqNum: 5}

	// Add a causal READ (not write) to key 100
	r.unsyncCausal(readCmd, weakCmdId)

	// A strong read should NOT get a weakDep for a causal READ
	ok, dep := r.okWithWeakDep(readCmd)
	if ok != TRUE {
		t.Errorf("ok = %d, want TRUE", ok)
	}
	if dep != nil {
		t.Errorf("dep = %v, want nil (causal read should not create weakDep)", dep)
	}
}

// --- checkStrongWriteConflict tests ---

func TestCheckStrongWriteConflictNoEntry(t *testing.T) {
	r := newTestReplica(false)
	if r.checkStrongWriteConflict(state.Key(100)) {
		t.Error("Expected no conflict for empty unsynced")
	}
}

func TestCheckStrongWriteConflictPresent(t *testing.T) {
	r := newTestReplica(false)
	cmd := state.Command{Op: state.PUT, K: state.Key(100), V: state.Value([]byte("v"))}
	cmdId := CommandId{ClientId: 1, SeqNum: 1}

	r.unsyncStrong(cmd, cmdId)

	if !r.checkStrongWriteConflict(state.Key(100)) {
		t.Error("Expected strong write conflict")
	}
}

func TestCheckStrongWriteConflictStrongRead(t *testing.T) {
	r := newTestReplica(false)
	cmd := state.Command{Op: state.GET, K: state.Key(100), V: state.NIL()}
	cmdId := CommandId{ClientId: 1, SeqNum: 1}

	r.unsyncStrong(cmd, cmdId)

	// Strong READ should not be detected as a strong WRITE conflict
	if r.checkStrongWriteConflict(state.Key(100)) {
		t.Error("Strong READ should not be a write conflict")
	}
}

func TestCheckStrongWriteConflictCausalWrite(t *testing.T) {
	r := newTestReplica(false)
	cmd := state.Command{Op: state.PUT, K: state.Key(100), V: state.Value([]byte("v"))}
	cmdId := CommandId{ClientId: 10, SeqNum: 1}

	r.unsyncCausal(cmd, cmdId)

	// Causal write should NOT be detected as a strong write conflict
	if r.checkStrongWriteConflict(state.Key(100)) {
		t.Error("Causal write should not be a strong write conflict")
	}
}

// --- getWeakWriteDep tests ---

func TestGetWeakWriteDepNoEntry(t *testing.T) {
	r := newTestReplica(false)
	dep := r.getWeakWriteDep(state.Key(100))
	if dep != nil {
		t.Errorf("Expected nil dep, got %v", dep)
	}
}

func TestGetWeakWriteDepPresent(t *testing.T) {
	r := newTestReplica(false)
	cmd := state.Command{Op: state.PUT, K: state.Key(100), V: state.Value([]byte("val"))}
	cmdId := CommandId{ClientId: 10, SeqNum: 5}

	r.unsyncCausal(cmd, cmdId)

	dep := r.getWeakWriteDep(state.Key(100))
	if dep == nil {
		t.Fatal("Expected non-nil dep for causal write")
	}
	if *dep != cmdId {
		t.Errorf("dep = %v, want %v", *dep, cmdId)
	}
}

func TestGetWeakWriteDepCausalRead(t *testing.T) {
	r := newTestReplica(false)
	cmd := state.Command{Op: state.GET, K: state.Key(100), V: state.NIL()}
	cmdId := CommandId{ClientId: 10, SeqNum: 5}

	r.unsyncCausal(cmd, cmdId)

	// Causal READ should not return a dep
	dep := r.getWeakWriteDep(state.Key(100))
	if dep != nil {
		t.Errorf("Expected nil dep for causal read, got %v", dep)
	}
}

func TestGetWeakWriteDepStrongWrite(t *testing.T) {
	r := newTestReplica(false)
	cmd := state.Command{Op: state.PUT, K: state.Key(100), V: state.Value([]byte("v"))}
	cmdId := CommandId{ClientId: 1, SeqNum: 1}

	r.unsyncStrong(cmd, cmdId)

	// Strong write should NOT return a weakDep
	dep := r.getWeakWriteDep(state.Key(100))
	if dep != nil {
		t.Errorf("Expected nil dep for strong write, got %v", dep)
	}
}

// --- getWeakWriteValue tests ---

func TestGetWeakWriteValueNoEntry(t *testing.T) {
	r := newTestReplica(false)
	val, found := r.getWeakWriteValue(state.Key(100))
	if found {
		t.Errorf("Expected not found, got %v", val)
	}
}

func TestGetWeakWriteValuePresent(t *testing.T) {
	r := newTestReplica(false)
	cmd := state.Command{Op: state.PUT, K: state.Key(100), V: state.Value([]byte("weakval"))}
	cmdId := CommandId{ClientId: 10, SeqNum: 5}

	r.unsyncCausal(cmd, cmdId)

	val, found := r.getWeakWriteValue(state.Key(100))
	if !found {
		t.Fatal("Expected to find weak write value")
	}
	if string(val) != "weakval" {
		t.Errorf("Value = %q, want weakval", val)
	}
}

func TestGetWeakWriteValueStrongWrite(t *testing.T) {
	r := newTestReplica(false)
	cmd := state.Command{Op: state.PUT, K: state.Key(100), V: state.Value([]byte("strong"))}
	cmdId := CommandId{ClientId: 1, SeqNum: 1}

	r.unsyncStrong(cmd, cmdId)

	// Strong write should NOT be returned as a weak write value
	_, found := r.getWeakWriteValue(state.Key(100))
	if found {
		t.Error("Strong write should not be returned as weak write value")
	}
}

// --- boundClients tests ---

func TestBoundClientTracking(t *testing.T) {
	r := newTestReplica(false)

	// Initially no clients are bound
	if r.isBoundReplicaFor(42) {
		t.Error("Client 42 should not be bound initially")
	}

	// Register client
	r.registerBoundClient(42)
	if !r.isBoundReplicaFor(42) {
		t.Error("Client 42 should be bound after registration")
	}

	// Other clients should not be affected
	if r.isBoundReplicaFor(99) {
		t.Error("Client 99 should not be bound")
	}
}

func TestMultipleBoundClients(t *testing.T) {
	r := newTestReplica(false)

	r.registerBoundClient(1)
	r.registerBoundClient(2)
	r.registerBoundClient(3)

	if !r.isBoundReplicaFor(1) || !r.isBoundReplicaFor(2) || !r.isBoundReplicaFor(3) {
		t.Error("All registered clients should be bound")
	}
	if r.isBoundReplicaFor(4) {
		t.Error("Unregistered client should not be bound")
	}
}

// --- Integration: unsync + sync + ok flow ---

func TestUnsyncSyncOkIntegration(t *testing.T) {
	r := newTestReplica(false)
	cmd := state.Command{Op: state.PUT, K: state.Key(100), V: state.Value([]byte("v"))}
	cmdId := CommandId{ClientId: 1, SeqNum: 1}

	// Initially ok
	if r.ok(cmd) != TRUE {
		t.Fatal("ok should be TRUE initially")
	}

	// After unsync, not ok (strong write conflict)
	r.unsyncStrong(cmd, cmdId)
	if r.ok(cmd) != FALSE {
		t.Fatal("ok should be FALSE after strong unsync")
	}

	// After sync, ok again
	r.sync(cmdId, cmd)
	if r.ok(cmd) != TRUE {
		t.Fatal("ok should be TRUE after sync")
	}
}

func TestLeaderUnsyncSyncLeaderIntegration(t *testing.T) {
	r := newTestReplica(true)
	cmd1 := state.Command{Op: state.PUT, K: state.Key(100), V: state.Value([]byte("v1"))}
	cmd2 := state.Command{Op: state.PUT, K: state.Key(100), V: state.Value([]byte("v2"))}
	cmdId1 := CommandId{ClientId: 1, SeqNum: 1}
	cmdId2 := CommandId{ClientId: 2, SeqNum: 2}

	// First op → no dep
	dep1 := r.leaderUnsyncStrong(cmd1, 0, cmdId1)
	if dep1 != -1 {
		t.Errorf("dep1 = %d, want -1", dep1)
	}

	// Second op → depends on first
	dep2 := r.leaderUnsyncStrong(cmd2, 5, cmdId2)
	if dep2 != 0 {
		t.Errorf("dep2 = %d, want 0", dep2)
	}

	// Sync first (stale) → should NOT remove because cmdId2 overwrote
	r.syncLeader(cmdId1, cmd1)
	_, exists := r.unsynced.Get("100")
	if !exists {
		t.Fatal("Entry should still exist after syncing stale cmdId")
	}

	// Sync second → should remove
	r.syncLeader(cmdId2, cmd2)
	_, exists = r.unsynced.Get("100")
	if exists {
		t.Fatal("Entry should be removed after syncing current cmdId")
	}
}

func TestCausalUnsyncOkNoConflict(t *testing.T) {
	r := newTestReplica(false)
	weakWrite := state.Command{Op: state.PUT, K: state.Key(100), V: state.Value([]byte("w"))}
	strongWrite := state.Command{Op: state.PUT, K: state.Key(100), V: state.Value([]byte("s"))}
	weakCmdId := CommandId{ClientId: 10, SeqNum: 1}

	// Add a causal write
	r.unsyncCausal(weakWrite, weakCmdId)

	// Strong write should pass ok (causal writes don't cause strong conflicts)
	result := r.ok(strongWrite)
	if result != TRUE {
		t.Errorf("ok = %d, want TRUE (causal write should not block strong write)", result)
	}

	// okWithWeakDep should return the causal write as a dependency
	ok, dep := r.okWithWeakDep(strongWrite)
	if ok != TRUE {
		t.Errorf("okWithWeakDep ok = %d, want TRUE", ok)
	}
	if dep == nil || *dep != weakCmdId {
		t.Errorf("okWithWeakDep dep = %v, want %v", dep, weakCmdId)
	}
}

// ============================================================================
// Phase 21: Client-Replica Binding Tests
// These tests verify the client-side boundReplica field, the sendMsgToAll
// helper, and the integration between client binding and replica-side
// boundClients tracking.
// ============================================================================

// --- Client struct boundReplica field tests ---

func TestClientBoundReplicaField(t *testing.T) {
	// Verify the Client struct has the boundReplica field with correct default
	c := &Client{
		boundReplica: 2,
	}
	if c.boundReplica != 2 {
		t.Errorf("boundReplica = %d, want 2", c.boundReplica)
	}
}

func TestClientBoundReplicaAccessor(t *testing.T) {
	c := &Client{
		boundReplica: 1,
	}
	if c.BoundReplica() != 1 {
		t.Errorf("BoundReplica() = %d, want 1", c.BoundReplica())
	}
}

func TestClientBoundReplicaZero(t *testing.T) {
	// boundReplica=0 means bound to replica 0 (co-located or lowest latency)
	c := &Client{
		boundReplica: 0,
	}
	if c.BoundReplica() != 0 {
		t.Errorf("BoundReplica() = %d, want 0", c.BoundReplica())
	}
}

func TestClientBoundReplicaDefaultZero(t *testing.T) {
	// Zero-value Client should have boundReplica=0 (default)
	c := &Client{}
	if c.boundReplica != 0 {
		t.Errorf("Default boundReplica = %d, want 0", c.boundReplica)
	}
}

// --- Binding model: client-replica correspondence tests ---

func TestBindingModel3Replicas(t *testing.T) {
	// Simulate a 3-replica setup where client binds to replica 1 (closest)
	c := &Client{
		N:            3,
		boundReplica: 1,
	}

	// Verify binding
	if c.boundReplica != 1 {
		t.Errorf("Expected bound to replica 1, got %d", c.boundReplica)
	}

	// Replica 1 should be the one that tracks this client
	r0 := newTestReplica(true) // leader
	r1 := newTestReplica(false)
	r2 := newTestReplica(false)

	// Only replica 1 (the bound replica) registers this client
	clientId := int32(100)
	r1.registerBoundClient(clientId)

	if r0.isBoundReplicaFor(clientId) {
		t.Error("Replica 0 (leader) should NOT be bound for client 100")
	}
	if !r1.isBoundReplicaFor(clientId) {
		t.Error("Replica 1 (bound) should be bound for client 100")
	}
	if r2.isBoundReplicaFor(clientId) {
		t.Error("Replica 2 should NOT be bound for client 100")
	}
}

func TestBindingModelLeaderIsBound(t *testing.T) {
	// Special case: client's closest replica is the leader
	c := &Client{
		N:            3,
		boundReplica: 0, // Bound to replica 0 = leader
		leader:       0,
	}

	// When bound replica == leader, the leader does both:
	// 1. Reply to client for causal ops (1-RTT)
	// 2. Coordinate replication in background
	if c.boundReplica != c.leader {
		t.Errorf("boundReplica=%d != leader=%d, expected equal", c.boundReplica, c.leader)
	}

	// Replica 0 is both leader and bound
	r0 := newTestReplica(true)
	r0.registerBoundClient(100)

	if !r0.isLeader {
		t.Error("Replica 0 should be leader")
	}
	if !r0.isBoundReplicaFor(100) {
		t.Error("Replica 0 should be bound for client 100")
	}
}

func TestBindingModelMultipleClients(t *testing.T) {
	// Multiple clients can bind to the same replica
	r := newTestReplica(false)

	r.registerBoundClient(100)
	r.registerBoundClient(200)
	r.registerBoundClient(300)

	for _, clientId := range []int32{100, 200, 300} {
		if !r.isBoundReplicaFor(clientId) {
			t.Errorf("Client %d should be bound to this replica", clientId)
		}
	}
}

func TestBindingModelDifferentReplicasForDifferentClients(t *testing.T) {
	// Different clients can bind to different replicas based on latency
	r0 := newTestReplica(true)
	r1 := newTestReplica(false)
	r2 := newTestReplica(false)

	// Client 100 is closest to replica 0
	r0.registerBoundClient(100)
	// Client 200 is closest to replica 1
	r1.registerBoundClient(200)
	// Client 300 is closest to replica 2
	r2.registerBoundClient(300)

	if !r0.isBoundReplicaFor(100) || r0.isBoundReplicaFor(200) || r0.isBoundReplicaFor(300) {
		t.Error("Replica 0 should only be bound for client 100")
	}
	if r1.isBoundReplicaFor(100) || !r1.isBoundReplicaFor(200) || r1.isBoundReplicaFor(300) {
		t.Error("Replica 1 should only be bound for client 200")
	}
	if r2.isBoundReplicaFor(100) || r2.isBoundReplicaFor(200) || !r2.isBoundReplicaFor(300) {
		t.Error("Replica 2 should only be bound for client 300")
	}
}

// --- Integration: Binding with witness pool ---

func TestBoundReplicaWitnessIntegration(t *testing.T) {
	// Verify that bound replica can act as witness (add to unsynced) AND reply
	r := newTestReplica(false)
	r.registerBoundClient(100)

	// Bound replica receives causal propose, adds to witness pool
	cmd := state.Command{Op: state.PUT, K: state.Key(42), V: state.Value([]byte("causal-val"))}
	cmdId := CommandId{ClientId: 100, SeqNum: 1}
	r.unsyncCausal(cmd, cmdId)

	// Verify entry is in witness pool
	v, exists := r.unsynced.Get("42")
	if !exists {
		t.Fatal("Expected witness entry for key 42")
	}
	entry := v.(*UnsyncedEntry)
	if entry.IsStrong {
		t.Error("Causal entry should have IsStrong=false")
	}
	if entry.CmdId != cmdId {
		t.Errorf("CmdId = %v, want %v", entry.CmdId, cmdId)
	}

	// Verify the replica is bound for this client
	if !r.isBoundReplicaFor(100) {
		t.Error("Replica should be bound for client 100")
	}
}

func TestNonBoundReplicaWitnessOnly(t *testing.T) {
	// Non-bound replicas also add to witness pool but do NOT reply to client
	r := newTestReplica(false)
	// NOT registered as bound for client 100

	cmd := state.Command{Op: state.PUT, K: state.Key(42), V: state.Value([]byte("causal-val"))}
	cmdId := CommandId{ClientId: 100, SeqNum: 1}
	r.unsyncCausal(cmd, cmdId)

	// Witness entry should still be there
	_, exists := r.unsynced.Get("42")
	if !exists {
		t.Fatal("Non-bound replica should still add to witness pool")
	}

	// But this replica is NOT bound → should NOT reply
	if r.isBoundReplicaFor(100) {
		t.Error("Non-bound replica should NOT be bound for client 100")
	}
}

func TestLeaderWitnessAndReplication(t *testing.T) {
	// Leader adds to witness pool AND coordinates replication
	r := newTestReplica(true)

	cmd := state.Command{Op: state.PUT, K: state.Key(42), V: state.Value([]byte("causal-val"))}
	cmdId := CommandId{ClientId: 100, SeqNum: 1}

	// Leader uses leaderUnsyncCausal (with slot)
	dep := r.leaderUnsyncCausal(cmd, 10, cmdId)
	if dep != -1 {
		t.Errorf("dep = %d, want -1 (first entry)", dep)
	}

	v, _ := r.unsynced.Get("42")
	entry := v.(*UnsyncedEntry)
	if entry.Slot != 10 {
		t.Errorf("Slot = %d, want 10", entry.Slot)
	}
}

// --- Auto-detect binding from first causal propose ---

func TestAutoDetectBinding(t *testing.T) {
	// Test the auto-detect pattern: first causal propose registers the client
	r := newTestReplica(false)

	// Before registration
	if r.isBoundReplicaFor(42) {
		t.Error("Client 42 should not be bound before auto-detect")
	}

	// Simulate receiving first causal propose and auto-detecting
	r.registerBoundClient(42)

	if !r.isBoundReplicaFor(42) {
		t.Error("Client 42 should be bound after auto-detect")
	}

	// Second registration is idempotent
	r.registerBoundClient(42)
	if !r.isBoundReplicaFor(42) {
		t.Error("Client 42 should still be bound after re-registration")
	}
}

// --- Strong op sees causal witness from bound replica's client ---

func TestStrongOpSeesWitnessFromBoundClient(t *testing.T) {
	// End-to-end: causal op from bound client is visible to strong op on same replica
	r := newTestReplica(false)
	r.registerBoundClient(100)

	// Client 100 sends causal PUT to key 42
	causalCmd := state.Command{Op: state.PUT, K: state.Key(42), V: state.Value([]byte("causal-val"))}
	causalCmdId := CommandId{ClientId: 100, SeqNum: 1}
	r.unsyncCausal(causalCmd, causalCmdId)

	// Another client sends strong GET for key 42 → should see weakDep
	strongRead := state.Command{Op: state.GET, K: state.Key(42), V: state.NIL()}
	ok, dep := r.okWithWeakDep(strongRead)
	if ok != TRUE {
		t.Errorf("ok = %d, want TRUE", ok)
	}
	if dep == nil || *dep != causalCmdId {
		t.Errorf("dep = %v, want %v", dep, causalCmdId)
	}

	// The strong op can also read the speculative value
	val, found := r.getWeakWriteValue(state.Key(42))
	if !found {
		t.Fatal("Expected to find weak write value")
	}
	if string(val) != "causal-val" {
		t.Errorf("val = %q, want causal-val", val)
	}
}
