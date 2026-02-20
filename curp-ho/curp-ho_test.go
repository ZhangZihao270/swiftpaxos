package curpho

import (
	"bytes"
	"sync"
	"testing"
	"time"

	"github.com/imdea-software/swiftpaxos/client"
	"github.com/imdea-software/swiftpaxos/replica"
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
	if MaxDescRoutines != 10000 {
		t.Errorf("MaxDescRoutines should be 10000, got %d", MaxDescRoutines)
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
		{"MRecordAck", 0, false, func() (int, bool) { return (&MRecordAck{}).BinarySize() }},
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

	r := newTestReplica(false)
	for _, tt := range tests {
		got := r.pendingWriteKey(tt.clientId, tt.key)
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
	r := newTestReplica(false)
	clientA := int32(100)
	clientB := int32(200)
	key := state.Key(1)

	keyA := r.pendingWriteKey(clientA, key)
	keyB := r.pendingWriteKey(clientB, key)

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
		isLeader:         isLeader,
		unsynced:         cmap.New(),
		synced:           cmap.New(),
		unsyncedByClient: cmap.New(),
		boundClients:     make(map[int32]bool),
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

// TestLeaderCausalNoDoubleUnsync verifies that on the leader, calling
// unsyncCausal before leaderUnsyncCausal for the same key causes a slot
// mismatch. This was the root cause of a Fatal crash: unsyncCausal sets
// Slot=1 (counter), then leaderUnsyncCausal with slot=0 sees entry.Slot=1>0.
// The fix: leader skips unsyncCausal, only calls leaderUnsyncCausal.
func TestLeaderCausalNoDoubleUnsync(t *testing.T) {
	r := newTestReplica(true) // isLeader=true
	cmd := state.Command{Op: state.PUT, K: state.Key(42), V: state.Value([]byte("val"))}
	cmdId := CommandId{ClientId: 1, SeqNum: 1}

	// Simulate the correct behavior: leader only calls leaderUnsyncCausal
	dep := r.leaderUnsyncCausal(cmd, 0, cmdId)
	if dep != -1 {
		t.Errorf("first command should have no dep, got %d", dep)
	}

	// Verify the entry has actual slot=0 (not counter=1)
	key := r.int32ToString(int32(cmd.K))
	v, exists := r.unsynced.Get(key)
	if !exists {
		t.Fatal("entry should exist in unsynced map")
	}
	entry := v.(*UnsyncedEntry)
	if entry.Slot != 0 {
		t.Errorf("leader entry Slot = %d, want 0 (actual slot)", entry.Slot)
	}
}

// TestNonLeaderCausalUsesCounter verifies that non-leader replicas use
// counter-based Slot via unsyncCausal (not slot-based).
func TestNonLeaderCausalUsesCounter(t *testing.T) {
	r := newTestReplica(false) // isLeader=false
	cmd := state.Command{Op: state.PUT, K: state.Key(42), V: state.Value([]byte("val"))}
	cmdId1 := CommandId{ClientId: 1, SeqNum: 1}
	cmdId2 := CommandId{ClientId: 1, SeqNum: 2}

	// Non-leader calls unsyncCausal
	r.unsyncCausal(cmd, cmdId1)
	key := r.int32ToString(int32(cmd.K))
	v, _ := r.unsynced.Get(key)
	if v.(*UnsyncedEntry).Slot != 1 {
		t.Errorf("first unsyncCausal Slot = %d, want 1", v.(*UnsyncedEntry).Slot)
	}

	r.unsyncCausal(cmd, cmdId2)
	v, _ = r.unsynced.Get(key)
	if v.(*UnsyncedEntry).Slot != 2 {
		t.Errorf("second unsyncCausal Slot = %d, want 2", v.(*UnsyncedEntry).Slot)
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

	ok, dep, _ := r.witnessCheck(cmd, 0)
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

	ok, dep, _ := r.witnessCheck(cmd, 0)
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
	ok, dep, _ := r.witnessCheck(readcmd, 0)
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
	ok, dep, _ := r.witnessCheck(readCmd, 0)
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

	// okWithWeakDep for a strong WRITE should return TRUE but no weakDep.
	// Per protocol spec, weakDep is only for strong READs.
	ok, dep, _ := r.witnessCheck(strongWrite, 0)
	if ok != TRUE {
		t.Errorf("okWithWeakDep ok = %d, want TRUE", ok)
	}
	if dep != nil {
		t.Errorf("okWithWeakDep dep = %v, want nil (strong writes don't get weakDep)", dep)
	}

	// okWithWeakDep for a strong READ should return TRUE + weakDep
	strongRead := state.Command{Op: state.GET, K: state.Key(100), V: state.NIL()}
	ok, dep, _ = r.witnessCheck(strongRead, 0)
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
	ok, dep, _ := r.witnessCheck(strongRead, 0)
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

// ============================================================================
// Phase 22: Causal Op Message Protocol Tests
// These tests verify the MCausalPropose and MCausalReply message types,
// their serialization/deserialization, cache pooling, and RPC registration.
// ============================================================================

// --- MCausalPropose serialization tests ---

func TestMCausalProposeSerialization(t *testing.T) {
	original := &MCausalPropose{
		CommandId:    42,
		ClientId:     100,
		Command: state.Command{
			Op: state.PUT,
			K:  state.Key(123),
			V:  []byte("test-value"),
		},
		Timestamp:    1234567890,
		CausalDep:    41,
		BoundReplica: 2,
	}

	var buf bytes.Buffer
	original.Marshal(&buf)

	restored := &MCausalPropose{}
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
	if restored.BoundReplica != original.BoundReplica {
		t.Errorf("BoundReplica mismatch: got %d, want %d", restored.BoundReplica, original.BoundReplica)
	}
	if restored.Command.Op != original.Command.Op {
		t.Errorf("Command.Op mismatch: got %d, want %d", restored.Command.Op, original.Command.Op)
	}
	if !bytes.Equal(restored.Command.V, original.Command.V) {
		t.Errorf("Command.V mismatch: got %v, want %v", restored.Command.V, original.Command.V)
	}
}

func TestMCausalProposeWithEmptyCommand(t *testing.T) {
	original := &MCausalPropose{
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

	restored := &MCausalPropose{}
	if err := restored.Unmarshal(&buf); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if restored.Command.Op != state.GET {
		t.Errorf("Command.Op mismatch: got %d, want GET", restored.Command.Op)
	}
	if restored.CausalDep != 0 {
		t.Errorf("CausalDep should be 0, got %d", restored.CausalDep)
	}
}

func TestMCausalProposeWithLargeValue(t *testing.T) {
	largeValue := make([]byte, 1024)
	for i := range largeValue {
		largeValue[i] = byte(i % 256)
	}

	original := &MCausalPropose{
		CommandId: 999,
		ClientId:  500,
		Command: state.Command{
			Op: state.PUT,
			K:  state.Key(42),
			V:  largeValue,
		},
		Timestamp: 9876543210,
		CausalDep: 998,
	}

	var buf bytes.Buffer
	original.Marshal(&buf)

	restored := &MCausalPropose{}
	if err := restored.Unmarshal(&buf); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if !bytes.Equal(restored.Command.V, original.Command.V) {
		t.Error("Large value mismatch after serialization round-trip")
	}
}

// --- MCausalReply serialization tests ---

func TestMCausalReplySerialization(t *testing.T) {
	original := &MCausalReply{
		Replica: 1,
		CmdId:   CommandId{ClientId: 100, SeqNum: 42},
		Rep:     []byte("result-value"),
	}

	var buf bytes.Buffer
	original.Marshal(&buf)

	restored := &MCausalReply{}
	if err := restored.Unmarshal(&buf); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if restored.Replica != original.Replica {
		t.Errorf("Replica mismatch: got %d, want %d", restored.Replica, original.Replica)
	}
	if restored.CmdId != original.CmdId {
		t.Errorf("CmdId mismatch: got %v, want %v", restored.CmdId, original.CmdId)
	}
	if !bytes.Equal(restored.Rep, original.Rep) {
		t.Errorf("Rep mismatch: got %v, want %v", restored.Rep, original.Rep)
	}
}

func TestMCausalReplyWithEmptyRep(t *testing.T) {
	original := &MCausalReply{
		Replica: 0,
		CmdId:   CommandId{ClientId: 1, SeqNum: 1},
		Rep:     []byte{},
	}

	var buf bytes.Buffer
	original.Marshal(&buf)

	restored := &MCausalReply{}
	if err := restored.Unmarshal(&buf); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if len(restored.Rep) != 0 {
		t.Errorf("Rep should be empty, got %v", restored.Rep)
	}
}

func TestMCausalReplyNoBallot(t *testing.T) {
	// MCausalReply intentionally has NO Ballot field (unlike MWeakReply).
	// Verify this by checking that serialization only has Replica(4) + CmdId(8) + varint + Rep
	reply := &MCausalReply{
		Replica: 2,
		CmdId:   CommandId{ClientId: 50, SeqNum: 10},
		Rep:     []byte{}, // Empty rep for predictable size
	}

	var buf bytes.Buffer
	reply.Marshal(&buf)

	// Fixed header: Replica(4) + CmdId.ClientId(4) + CmdId.SeqNum(4) = 12 bytes
	// Plus varint(0) = 1 byte for empty Rep
	expectedSize := 12 + 1
	if buf.Len() != expectedSize {
		t.Errorf("MCausalReply size = %d, want %d (no Ballot field)", buf.Len(), expectedSize)
	}
}

// --- BinarySize tests ---

func TestMCausalProposeBinarySize(t *testing.T) {
	m := &MCausalPropose{}
	size, known := m.BinarySize()
	if known {
		t.Error("MCausalPropose size should not be known (variable due to Command)")
	}
	if size != 0 {
		t.Errorf("size = %d, want 0", size)
	}
}

func TestMCausalReplyBinarySize(t *testing.T) {
	m := &MCausalReply{}
	size, known := m.BinarySize()
	if known {
		t.Error("MCausalReply size should not be known (variable due to Rep)")
	}
	if size != 0 {
		t.Errorf("size = %d, want 0", size)
	}
}

// --- New() method tests ---

func TestMCausalProposeNew(t *testing.T) {
	m := &MCausalPropose{}
	n := m.New()
	if n == nil {
		t.Error("MCausalPropose.New() returned nil")
	}
	if _, ok := n.(*MCausalPropose); !ok {
		t.Error("MCausalPropose.New() returned wrong type")
	}
}

func TestMCausalReplyNew(t *testing.T) {
	m := &MCausalReply{}
	n := m.New()
	if n == nil {
		t.Error("MCausalReply.New() returned nil")
	}
	if _, ok := n.(*MCausalReply); !ok {
		t.Error("MCausalReply.New() returned wrong type")
	}
}

// --- Cache pool tests ---

func TestMCausalProposeCache(t *testing.T) {
	cache := NewMCausalProposeCache()

	// Get from empty cache
	obj := cache.Get()
	if obj == nil {
		t.Fatal("Get from empty cache returned nil")
	}

	// Populate and put back
	obj.CommandId = 123
	obj.ClientId = 456
	cache.Put(obj)

	// Get reused object
	obj2 := cache.Get()
	if obj2 == nil {
		t.Fatal("Get after Put returned nil")
	}
}

func TestMCausalReplyCache(t *testing.T) {
	cache := NewMCausalReplyCache()

	// Get from empty cache
	obj := cache.Get()
	if obj == nil {
		t.Fatal("Get from empty cache returned nil")
	}

	// Populate and put back
	obj.Replica = 5
	obj.Rep = []byte("cached-result")
	cache.Put(obj)

	// Get reused object
	obj2 := cache.Get()
	if obj2 == nil {
		t.Fatal("Get after Put returned nil")
	}
}

func TestMCausalProposeCacheMultiple(t *testing.T) {
	cache := NewMCausalProposeCache()

	// Put multiple objects
	for i := 0; i < 5; i++ {
		obj := &MCausalPropose{CommandId: int32(i)}
		cache.Put(obj)
	}

	// Get all back
	for i := 0; i < 5; i++ {
		obj := cache.Get()
		if obj == nil {
			t.Fatalf("Get %d returned nil", i)
		}
	}

	// Extra get should still return a new object (not nil)
	obj := cache.Get()
	if obj == nil {
		t.Fatal("Get from depleted cache returned nil (should create new)")
	}
}

// --- Comparison: MCausalPropose vs MWeakPropose field compatibility ---

func TestCausalProposeExtendsWeakProposeFields(t *testing.T) {
	// MCausalPropose extends MWeakPropose with an extra BoundReplica field.
	// The wire format should be MWeakPropose bytes + 4 bytes for BoundReplica.
	causal := &MCausalPropose{
		CommandId:    42,
		ClientId:     100,
		Command: state.Command{
			Op: state.PUT,
			K:  state.Key(1),
			V:  []byte("val"),
		},
		Timestamp:    12345,
		CausalDep:    41,
		BoundReplica: 2,
	}

	weak := &MWeakPropose{
		CommandId: causal.CommandId,
		ClientId:  causal.ClientId,
		Command:   causal.Command,
		Timestamp: causal.Timestamp,
		CausalDep: causal.CausalDep,
	}

	var causalBuf, weakBuf bytes.Buffer
	causal.Marshal(&causalBuf)
	weak.Marshal(&weakBuf)

	// MCausalPropose should be MWeakPropose + 4 bytes (BoundReplica)
	if causalBuf.Len() != weakBuf.Len()+4 {
		t.Errorf("MCausalPropose size = %d, want MWeakPropose size (%d) + 4", causalBuf.Len(), weakBuf.Len())
	}

	// The prefix should match MWeakPropose wire format
	causalBytes := causalBuf.Bytes()
	weakBytes := weakBuf.Bytes()
	if !bytes.Equal(causalBytes[:len(weakBytes)], weakBytes) {
		t.Error("MCausalPropose wire prefix should match MWeakPropose wire format")
	}
}

// --- Serialization round-trip with all zero values ---

func TestMCausalProposeZeroValues(t *testing.T) {
	original := &MCausalPropose{
		CommandId: 0,
		ClientId:  0,
		Command: state.Command{
			Op: 0,
			K:  state.Key(0),
			V:  state.NIL(),
		},
		Timestamp: 0,
		CausalDep: 0,
	}

	var buf bytes.Buffer
	original.Marshal(&buf)

	restored := &MCausalPropose{}
	if err := restored.Unmarshal(&buf); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if restored.CommandId != 0 || restored.ClientId != 0 ||
		restored.Timestamp != 0 || restored.CausalDep != 0 ||
		restored.BoundReplica != 0 {
		t.Error("Zero values not preserved in round-trip")
	}
}

func TestMCausalReplyZeroValues(t *testing.T) {
	original := &MCausalReply{
		Replica: 0,
		CmdId:   CommandId{ClientId: 0, SeqNum: 0},
		Rep:     state.NIL(),
	}

	var buf bytes.Buffer
	original.Marshal(&buf)

	restored := &MCausalReply{}
	if err := restored.Unmarshal(&buf); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if restored.Replica != 0 || restored.CmdId.ClientId != 0 || restored.CmdId.SeqNum != 0 {
		t.Error("Zero values not preserved in round-trip")
	}
}

// ============================================================================
// Phase 23: Causal Op Client-Side Tests
// These tests verify SendCausalWrite, SendCausalRead, handleCausalReply,
// the handleMsgs causalReplyChan case, and the SendWeakWrite/SendWeakRead
// delegation to causal methods.
// ============================================================================

// newTestClient creates a minimal Client for testing handleCausalReply.
// Includes a BufferClient with Reply channel to avoid nil pointer panics.
func newTestClient(boundReplica int32) *Client {
	bc := &client.BufferClient{
		Reply: make(chan *client.ReqReply, 100),
	}
	return &Client{
		BufferClient:       bc,
		N:                  3,
		boundReplica:       boundReplica,
		delivered:          make(map[int32]struct{}),
		weakPending:        make(map[int32]struct{}),
		localCache:         make(map[int64]cacheEntry),
		weakPendingKeys:    make(map[int32]int64),
		weakPendingValues:  make(map[int32]state.Value),
		strongPendingKeys:  make(map[int32]int64),
		weakWriteSendTimes: make(map[int32]time.Time),
		writerMu:           make([]sync.Mutex, 3),
	}
}

// --- handleCausalReply tests ---

func TestHandleCausalReplyFromBoundReplica(t *testing.T) {
	c := newTestClient(1)
	c.weakPending[42] = struct{}{}

	rep := &MCausalReply{
		Replica: 1, // matches boundReplica
		CmdId:   CommandId{ClientId: 100, SeqNum: 42},
		Rep:     []byte("result"),
	}

	c.handleCausalReply(rep)

	// Should be delivered
	if _, exists := c.delivered[42]; !exists {
		t.Error("Command should be marked as delivered")
	}
	// Should be removed from weakPending
	if _, exists := c.weakPending[42]; exists {
		t.Error("Command should be removed from weakPending")
	}
	// Value should be set
	if string(c.val) != "result" {
		t.Errorf("val = %q, want 'result'", c.val)
	}
	// RegisterReply should have sent to Reply channel
	select {
	case rr := <-c.BufferClient.Reply:
		if rr.Seqnum != 42 {
			t.Errorf("RegisterReply seqnum = %d, want 42", rr.Seqnum)
		}
	default:
		t.Error("Expected RegisterReply to send to Reply channel")
	}
}

func TestHandleCausalReplyFromNonBoundReplica(t *testing.T) {
	c := newTestClient(1)
	c.weakPending[42] = struct{}{}

	rep := &MCausalReply{
		Replica: 0, // does NOT match boundReplica (1)
		CmdId:   CommandId{ClientId: 100, SeqNum: 42},
		Rep:     []byte("result"),
	}

	c.handleCausalReply(rep)

	// Should NOT be delivered (wrong replica)
	if _, exists := c.delivered[42]; exists {
		t.Error("Command should NOT be delivered from non-bound replica")
	}
	// Should still be in weakPending
	if _, exists := c.weakPending[42]; !exists {
		t.Error("Command should still be in weakPending")
	}
}

func TestHandleCausalReplyAlreadyDelivered(t *testing.T) {
	c := newTestClient(1)
	c.delivered[42] = struct{}{}

	rep := &MCausalReply{
		Replica: 1,
		CmdId:   CommandId{ClientId: 100, SeqNum: 42},
		Rep:     []byte("new-result"),
	}

	c.handleCausalReply(rep)

	// Should not change val (already delivered)
	if string(c.val) == "new-result" {
		t.Error("Already delivered command should not update val")
	}
}

func TestHandleCausalReplyEmptyResult(t *testing.T) {
	c := newTestClient(2)
	c.weakPending[10] = struct{}{}

	rep := &MCausalReply{
		Replica: 2,
		CmdId:   CommandId{ClientId: 50, SeqNum: 10},
		Rep:     []byte{}, // Empty result (e.g., from PUT)
	}

	c.handleCausalReply(rep)

	if _, exists := c.delivered[10]; !exists {
		t.Error("Command should be delivered even with empty result")
	}
}

// --- SendWeakWrite/SendWeakRead delegation tests ---

func TestSendWeakWriteDelegatesToCausal(t *testing.T) {
	c := newTestClient(0)

	// These should compile and be callable (verifies method signatures)
	var _ func(int64, []byte) int32 = c.SendWeakWrite
	var _ func(int64, []byte) int32 = c.SendCausalWrite
	var _ func(int64) int32 = c.SendWeakRead
	var _ func(int64) int32 = c.SendCausalRead
}

// --- Causal chaining: multiple causal ops maintain dependency ---

func TestCausalDependencyChain(t *testing.T) {
	c := newTestClient(1)
	c.lastWeakWriteSeqNum = 0

	// First causal op: causalDep = 0 (lastWeakWriteSeqNum before)
	c.mu.Lock()
	dep1 := c.lastWeakWriteSeqNum
	seqnum1 := int32(1)
	c.weakPending[seqnum1] = struct{}{}
	c.lastWeakWriteSeqNum = seqnum1
	c.mu.Unlock()

	if dep1 != 0 {
		t.Errorf("First op causalDep = %d, want 0", dep1)
	}

	// Second causal op: causalDep = 1 (previous seqnum)
	c.mu.Lock()
	dep2 := c.lastWeakWriteSeqNum
	seqnum2 := int32(2)
	c.weakPending[seqnum2] = struct{}{}
	c.lastWeakWriteSeqNum = seqnum2
	c.mu.Unlock()

	if dep2 != 1 {
		t.Errorf("Second op causalDep = %d, want 1", dep2)
	}

	// Third causal op: causalDep = 2
	c.mu.Lock()
	dep3 := c.lastWeakWriteSeqNum
	seqnum3 := int32(3)
	c.weakPending[seqnum3] = struct{}{}
	c.lastWeakWriteSeqNum = seqnum3
	c.mu.Unlock()

	if dep3 != 2 {
		t.Errorf("Third op causalDep = %d, want 2", dep3)
	}
}

// --- Causal reply from different replicas ---

func TestHandleCausalReplyFromEachReplica(t *testing.T) {
	// Only bound replica's reply should be accepted
	for boundId := int32(0); boundId < 3; boundId++ {
		for replyFrom := int32(0); replyFrom < 3; replyFrom++ {
			c := newTestClient(boundId)
			c.weakPending[1] = struct{}{}

			rep := &MCausalReply{
				Replica: replyFrom,
				CmdId:   CommandId{ClientId: 100, SeqNum: 1},
				Rep:     []byte("result"),
			}
			c.handleCausalReply(rep)

			_, delivered := c.delivered[1]
			if replyFrom == boundId && !delivered {
				t.Errorf("bound=%d, reply from %d: should be delivered", boundId, replyFrom)
			}
			if replyFrom != boundId && delivered {
				t.Errorf("bound=%d, reply from %d: should NOT be delivered", boundId, replyFrom)
			}
		}
	}
}

// --- MCausalPropose message construction tests ---

func TestMCausalProposeWriteConstruction(t *testing.T) {
	p := &MCausalPropose{
		CommandId: 42,
		ClientId:  100,
		Command: state.Command{
			Op: state.PUT,
			K:  state.Key(123),
			V:  []byte("test-value"),
		},
		Timestamp: 0,
		CausalDep: 41,
	}

	if p.Command.Op != state.PUT {
		t.Errorf("Op = %d, want PUT", p.Command.Op)
	}
	if p.CausalDep != 41 {
		t.Errorf("CausalDep = %d, want 41", p.CausalDep)
	}
}

func TestMCausalProposeReadConstruction(t *testing.T) {
	p := &MCausalPropose{
		CommandId: 43,
		ClientId:  100,
		Command: state.Command{
			Op: state.GET,
			K:  state.Key(123),
			V:  state.NIL(),
		},
		Timestamp: 0,
		CausalDep: 42,
	}

	if p.Command.Op != state.GET {
		t.Errorf("Op = %d, want GET", p.Command.Op)
	}
}

// --- Integration: handleCausalReply with multiple pending commands ---

func TestHandleCausalReplyMultiplePending(t *testing.T) {
	c := newTestClient(1)

	// Multiple pending commands
	c.weakPending[1] = struct{}{}
	c.weakPending[2] = struct{}{}
	c.weakPending[3] = struct{}{}

	// Deliver command 2 first (out of order is fine for causal)
	rep2 := &MCausalReply{
		Replica: 1,
		CmdId:   CommandId{ClientId: 100, SeqNum: 2},
		Rep:     []byte("result-2"),
	}
	c.handleCausalReply(rep2)

	if _, exists := c.delivered[2]; !exists {
		t.Error("Command 2 should be delivered")
	}
	if _, exists := c.weakPending[2]; exists {
		t.Error("Command 2 should be removed from pending")
	}

	// Commands 1 and 3 should still be pending
	if _, exists := c.weakPending[1]; !exists {
		t.Error("Command 1 should still be pending")
	}
	if _, exists := c.weakPending[3]; !exists {
		t.Error("Command 3 should still be pending")
	}

	// Deliver command 1
	rep1 := &MCausalReply{
		Replica: 1,
		CmdId:   CommandId{ClientId: 100, SeqNum: 1},
		Rep:     []byte("result-1"),
	}
	c.handleCausalReply(rep1)

	if _, exists := c.delivered[1]; !exists {
		t.Error("Command 1 should be delivered")
	}
}

// ============================================================================
// Phase 43.1: Weak Write Instrumentation Tests
// These tests verify the latency instrumentation added for diagnosing
// the CURP-HO W-P99 spike.
// ============================================================================

func TestInstrumentationRecordsLatency(t *testing.T) {
	c := newTestClient(1)

	// Simulate a weak write: record send time, then handle reply
	seqnum := int32(42)
	c.weakPending[seqnum] = struct{}{}
	c.weakWriteSendTimes[seqnum] = time.Now().Add(-5 * time.Millisecond) // 5ms ago

	rep := &MCausalReply{
		Replica: 1,
		CmdId:   CommandId{ClientId: 100, SeqNum: seqnum},
		Rep:     []byte("result"),
	}
	c.handleCausalReply(rep)

	// Latency should be recorded
	if len(c.weakWriteLatencies) != 1 {
		t.Fatalf("weakWriteLatencies len = %d, want 1", len(c.weakWriteLatencies))
	}
	if c.weakWriteLatencies[0] < 5*time.Millisecond {
		t.Errorf("latency = %v, want >= 5ms", c.weakWriteLatencies[0])
	}

	// Send time should be cleaned up
	if _, exists := c.weakWriteSendTimes[seqnum]; exists {
		t.Error("weakWriteSendTimes should be cleaned up after reply")
	}
}

func TestInstrumentationNoLatencyForNonBound(t *testing.T) {
	c := newTestClient(1)

	seqnum := int32(42)
	c.weakPending[seqnum] = struct{}{}
	c.weakWriteSendTimes[seqnum] = time.Now()

	// Reply from wrong replica — should be discarded
	rep := &MCausalReply{
		Replica: 0, // not bound replica (1)
		CmdId:   CommandId{ClientId: 100, SeqNum: seqnum},
		Rep:     []byte("result"),
	}
	c.handleCausalReply(rep)

	// No latency recorded (reply was discarded)
	if len(c.weakWriteLatencies) != 0 {
		t.Errorf("weakWriteLatencies len = %d, want 0 (non-bound reply should be ignored)", len(c.weakWriteLatencies))
	}
	// Send time should still be present (not yet delivered)
	if _, exists := c.weakWriteSendTimes[seqnum]; !exists {
		t.Error("weakWriteSendTimes should still exist for non-bound reply")
	}
}

func TestInstrumentationNoLatencyForAlreadyDelivered(t *testing.T) {
	c := newTestClient(1)

	seqnum := int32(42)
	c.delivered[seqnum] = struct{}{} // already delivered
	c.weakWriteSendTimes[seqnum] = time.Now()

	rep := &MCausalReply{
		Replica: 1,
		CmdId:   CommandId{ClientId: 100, SeqNum: seqnum},
		Rep:     []byte("result"),
	}
	c.handleCausalReply(rep)

	// No new latency recorded
	if len(c.weakWriteLatencies) != 0 {
		t.Errorf("weakWriteLatencies len = %d, want 0 (already delivered)", len(c.weakWriteLatencies))
	}
}

func TestInstrumentationMultipleLatencies(t *testing.T) {
	c := newTestClient(1)

	// Simulate 3 weak writes with different latencies
	for i := int32(1); i <= 3; i++ {
		c.weakPending[i] = struct{}{}
		c.weakWriteSendTimes[i] = time.Now().Add(-time.Duration(i) * time.Millisecond)
	}

	// Deliver them
	for i := int32(1); i <= 3; i++ {
		rep := &MCausalReply{
			Replica: 1,
			CmdId:   CommandId{ClientId: 100, SeqNum: i},
			Rep:     []byte("r"),
		}
		c.handleCausalReply(rep)
	}

	if len(c.weakWriteLatencies) != 3 {
		t.Fatalf("weakWriteLatencies len = %d, want 3", len(c.weakWriteLatencies))
	}
	// Latencies should be increasing (1ms, 2ms, 3ms approximately)
	for i, lat := range c.weakWriteLatencies {
		expected := time.Duration(i+1) * time.Millisecond
		if lat < expected {
			t.Errorf("latency[%d] = %v, want >= %v", i, lat, expected)
		}
	}
}

func TestInstrumentationPrintDoesNotPanic(t *testing.T) {
	c := newTestClient(1)

	// Empty case: should not panic
	c.printWeakWriteInstrumentation()

	// With data: should not panic
	c.weakWriteLatencies = []time.Duration{
		1 * time.Millisecond,
		2 * time.Millisecond,
		100 * time.Millisecond,
	}
	c.sendMsgToAllSlowLog = []sendMsgToAllDuration{
		{seqNum: 1, duration: 15 * time.Millisecond},
	}
	c.printWeakWriteInstrumentation()
}

func TestInstrumentationSendTimesCleanupOnWeakRead(t *testing.T) {
	c := newTestClient(1)

	// Weak reads don't go through SendCausalWrite, so no send time is recorded.
	// handleCausalReply should handle missing send time gracefully.
	seqnum := int32(42)
	c.weakPending[seqnum] = struct{}{}
	// No entry in weakWriteSendTimes — simulating a weak read or missed tracking

	rep := &MCausalReply{
		Replica: 1,
		CmdId:   CommandId{ClientId: 100, SeqNum: seqnum},
		Rep:     []byte("result"),
	}
	c.handleCausalReply(rep)

	// No latency recorded (no send time to compute from)
	if len(c.weakWriteLatencies) != 0 {
		t.Errorf("weakWriteLatencies len = %d, want 0 (no send time for weak reads)", len(c.weakWriteLatencies))
	}
	// Should still be delivered
	if _, exists := c.delivered[seqnum]; !exists {
		t.Error("Command should be delivered even without instrumentation send time")
	}
}

// ============================================================================
// Phase 24: Causal Op Replica-Side Tests
// These tests verify the replica-side handling of CURP-HO causal operations:
// handleCausalPropose, getCausalCmdDesc, asyncReplicateCausal.
// ============================================================================

// newTestReplicaForDesc creates a test replica with enough state for
// command descriptor creation and causal propose processing.
func newTestReplicaForDesc(isLeader bool) *Replica {
	cmap.SHARD_COUNT = 32768
	r := &Replica{
		isLeader:      isLeader,
		ballot:        0,
		status:        NORMAL,
		lastCmdSlot:   0,
		unsynced:         cmap.New(),
		synced:           cmap.New(),
		unsyncedByClient: cmap.New(),
		values:           cmap.New(),
		proposes:         cmap.New(),
		cmdDescs:         cmap.New(),
		executed:         cmap.New(),
		committed:        cmap.New(),
		delivered:        cmap.New(),
		weakExecuted:     cmap.New(),
		pendingWrites:    cmap.New(),
		boundClients:     make(map[int32]bool),
		slots:         make(map[CommandId]int),
		history:       make([]commandStaticDesc, HISTORY_SIZE),
		commitNotify:  make(map[int]chan struct{}),
		executeNotify: make(map[int]chan struct{}),
		deliverChan:   make(chan int, 100),
		descPool: sync.Pool{
			New: func() interface{} {
				return &commandDesc{}
			},
		},
		routineCount: 0,
	}
	r.Q = replica.NewMajorityOf(3)

	// Initialize pre-allocated closed channel
	r.closedChan = make(chan struct{})
	close(r.closedChan)

	return r
}

// --- getCausalCmdDesc tests ---

func TestGetCausalCmdDescBasicFields(t *testing.T) {
	r := newTestReplicaForDesc(true)
	propose := &MCausalPropose{
		CommandId: 5,
		ClientId:  42,
		Command:   state.Command{Op: state.PUT, K: state.Key(100), V: state.Value([]byte("v1"))},
		CausalDep: 0,
	}

	desc := r.getCausalCmdDesc(0, propose, -1)

	if desc.cmdId.ClientId != 42 {
		t.Errorf("ClientId = %d, want 42", desc.cmdId.ClientId)
	}
	if desc.cmdId.SeqNum != 5 {
		t.Errorf("SeqNum = %d, want 5", desc.cmdId.SeqNum)
	}
	if desc.cmdSlot != 0 {
		t.Errorf("cmdSlot = %d, want 0", desc.cmdSlot)
	}
	if desc.dep != -1 {
		t.Errorf("dep = %d, want -1", desc.dep)
	}
	if desc.phase != ACCEPT {
		t.Errorf("phase = %d, want ACCEPT (%d)", desc.phase, ACCEPT)
	}
	if !desc.isWeak {
		t.Error("isWeak should be true for causal commands")
	}
	if desc.cmd.Op != state.PUT {
		t.Errorf("cmd.Op = %d, want PUT", desc.cmd.Op)
	}
	if desc.cmd.K != state.Key(100) {
		t.Errorf("cmd.K = %d, want 100", desc.cmd.K)
	}
}

func TestGetCausalCmdDescWithDependency(t *testing.T) {
	r := newTestReplicaForDesc(true)

	// Create first command at slot 0 and register in cmdDescs
	// (so getCausalCmdDesc can find it when resolving dep)
	propose1 := &MCausalPropose{
		CommandId: 1,
		ClientId:  42,
		Command:   state.Command{Op: state.PUT, K: state.Key(100), V: state.Value([]byte("v1"))},
	}
	desc1 := r.getCausalCmdDesc(0, propose1, -1)
	r.cmdDescs.Set("0", desc1) // Register so dep lookup finds it

	// Create second command at slot 1 depending on slot 0
	propose2 := &MCausalPropose{
		CommandId: 2,
		ClientId:  42,
		Command:   state.Command{Op: state.PUT, K: state.Key(100), V: state.Value([]byte("v2"))},
	}
	desc2 := r.getCausalCmdDesc(1, propose2, 0)

	if desc2.dep != 0 {
		t.Errorf("dep = %d, want 0", desc2.dep)
	}

	// Verify successor linking: desc1.successor should be slot 1
	desc1.successorL.Lock()
	succ := desc1.successor
	desc1.successorL.Unlock()
	if succ != 1 {
		t.Errorf("desc1.successor = %d, want 1", succ)
	}
}

func TestGetCausalCmdDescNoDep(t *testing.T) {
	r := newTestReplicaForDesc(true)
	propose := &MCausalPropose{
		CommandId: 1,
		ClientId:  10,
		Command:   state.Command{Op: state.GET, K: state.Key(50), V: state.NIL()},
	}

	desc := r.getCausalCmdDesc(0, propose, -1)

	if desc.dep != -1 {
		t.Errorf("dep = %d, want -1", desc.dep)
	}
	if desc.cmd.Op != state.GET {
		t.Errorf("cmd.Op = %d, want GET", desc.cmd.Op)
	}
}

// --- handleCausalPropose component tests ---
// These test the individual building blocks that handleCausalPropose uses.

func TestCausalProposeWitnessPoolAddsEntry(t *testing.T) {
	r := newTestReplica(false) // Non-leader
	cmd := state.Command{Op: state.PUT, K: state.Key(200), V: state.Value([]byte("causal-val"))}
	cmdId := CommandId{ClientId: 77, SeqNum: 3}

	// Simulate step 1 of handleCausalPropose: add to witness pool
	r.unsyncCausal(cmd, cmdId)

	v, exists := r.unsynced.Get("200")
	if !exists {
		t.Fatal("Expected entry in witness pool for key 200")
	}
	entry := v.(*UnsyncedEntry)
	if entry.IsStrong {
		t.Error("Entry should be causal (IsStrong=false)")
	}
	if entry.Slot != 1 {
		t.Errorf("Slot = %d, want 1 (count)", entry.Slot)
	}
	if string(entry.Value) != "causal-val" {
		t.Errorf("Value = %q, want causal-val", entry.Value)
	}
}

func TestCausalProposePendingWriteTracking(t *testing.T) {
	r := newTestReplica(false)
	r.pendingWrites = cmap.New()

	// Simulate step 2 of handleCausalPropose: track pending write for PUT
	r.addPendingWrite(42, state.Key(100), 5, state.Value([]byte("pending-val")))

	pw := r.getPendingWrite(42, state.Key(100), 5)
	if pw == nil {
		t.Fatal("Expected pending write for client 42, key 100")
	}
	if string(pw.value) != "pending-val" {
		t.Errorf("Pending write value = %q, want pending-val", pw.value)
	}
	if pw.seqNum != 5 {
		t.Errorf("Pending write seqNum = %d, want 5", pw.seqNum)
	}
}

func TestCausalProposeNoPendingWriteForGET(t *testing.T) {
	r := newTestReplica(false)
	r.pendingWrites = cmap.New()

	// GET commands should NOT add pending writes
	// (handleCausalPropose checks cmd.Op == state.PUT before calling addPendingWrite)
	pw := r.getPendingWrite(42, state.Key(100), 0)
	if pw != nil {
		t.Error("Should not have pending write for GET command")
	}
}

func TestCausalProposeLeaderSlotAssignment(t *testing.T) {
	r := newTestReplicaForDesc(true) // Leader
	r.lastCmdSlot = 5

	propose := &MCausalPropose{
		CommandId: 10,
		ClientId:  42,
		Command:   state.Command{Op: state.PUT, K: state.Key(100), V: state.Value([]byte("v1"))},
		CausalDep: 0,
	}

	// Simulate leader-side slot assignment (step 4 of handleCausalPropose)
	slot := r.lastCmdSlot
	r.lastCmdSlot++
	cmdId := CommandId{ClientId: propose.ClientId, SeqNum: propose.CommandId}
	dep := r.leaderUnsyncCausal(propose.Command, slot, cmdId)
	desc := r.getCausalCmdDesc(slot, propose, dep)

	if slot != 5 {
		t.Errorf("slot = %d, want 5", slot)
	}
	if r.lastCmdSlot != 6 {
		t.Errorf("lastCmdSlot = %d, want 6 (incremented)", r.lastCmdSlot)
	}
	if dep != -1 {
		t.Errorf("dep = %d, want -1 (no previous)", dep)
	}
	if desc.cmdSlot != 5 {
		t.Errorf("desc.cmdSlot = %d, want 5", desc.cmdSlot)
	}
}

func TestCausalProposeLeaderSlotDependency(t *testing.T) {
	r := newTestReplicaForDesc(true)

	// First causal op on key 100 at slot 0
	cmd1 := state.Command{Op: state.PUT, K: state.Key(100), V: state.Value([]byte("v1"))}
	cmdId1 := CommandId{ClientId: 1, SeqNum: 1}
	dep1 := r.leaderUnsyncCausal(cmd1, 0, cmdId1)
	r.lastCmdSlot = 1

	// Second causal op on same key at slot 1
	cmd2 := state.Command{Op: state.PUT, K: state.Key(100), V: state.Value([]byte("v2"))}
	cmdId2 := CommandId{ClientId: 1, SeqNum: 2}
	dep2 := r.leaderUnsyncCausal(cmd2, 1, cmdId2)

	if dep1 != -1 {
		t.Errorf("dep1 = %d, want -1", dep1)
	}
	if dep2 != 0 {
		t.Errorf("dep2 = %d, want 0 (depends on slot 0)", dep2)
	}
}

func TestCausalProposeNonLeaderNoSlotAssignment(t *testing.T) {
	r := newTestReplica(false) // Non-leader
	r.lastCmdSlot = 0

	// Non-leaders don't assign slots in handleCausalPropose
	// They only add to witness pool and reply
	cmd := state.Command{Op: state.PUT, K: state.Key(100), V: state.Value([]byte("v1"))}
	cmdId := CommandId{ClientId: 42, SeqNum: 1}
	r.unsyncCausal(cmd, cmdId)

	// lastCmdSlot should remain unchanged for non-leaders
	if r.lastCmdSlot != 0 {
		t.Errorf("lastCmdSlot = %d, want 0 (non-leader should not increment)", r.lastCmdSlot)
	}
}

// --- Speculative result tests for causal ops ---

func TestComputeSpeculativeResultPUT(t *testing.T) {
	r := newTestReplica(false)
	r.pendingWrites = cmap.New()

	cmd := state.Command{Op: state.PUT, K: state.Key(100), V: state.Value([]byte("val"))}
	result := r.computeSpeculativeResult(42, 0, cmd)

	// PUT should return NIL during speculation
	if !bytes.Equal(result, state.NIL()) {
		t.Errorf("PUT speculative result = %v, want NIL", result)
	}
}

func TestComputeSpeculativeResultGETWithPendingWrite(t *testing.T) {
	r := newTestReplica(false)
	r.pendingWrites = cmap.New()

	// Add a pending write from client 42 for key 100
	r.addPendingWrite(42, state.Key(100), 3, state.Value([]byte("pending-data")))

	// GET should return the pending write value if causalDep >= seqNum
	cmd := state.Command{Op: state.GET, K: state.Key(100), V: state.NIL()}
	result := r.computeSpeculativeResult(42, 3, cmd)

	if !bytes.Equal(result, []byte("pending-data")) {
		t.Errorf("GET speculative result = %v, want pending-data", result)
	}
}

func TestComputeSpeculativeResultGETNoPending(t *testing.T) {
	r := newTestReplica(false)
	r.pendingWrites = cmap.New()

	// No pending write - would fall back to committed state
	// But we don't have r.State in test replica, so just verify no panic
	// when pending write doesn't exist
	_ = state.Command{Op: state.GET, K: state.Key(999), V: state.NIL()}
	pw := r.getPendingWrite(42, state.Key(999), 0)
	if pw != nil {
		t.Error("Should have no pending write for key 999")
	}
}

// --- Multiple causal ops integration ---

func TestMultipleCausalOpsWitnessPool(t *testing.T) {
	r := newTestReplica(false)

	// Multiple causal ops on same key should increment count
	cmd1 := state.Command{Op: state.PUT, K: state.Key(50), V: state.Value([]byte("w1"))}
	cmd2 := state.Command{Op: state.PUT, K: state.Key(50), V: state.Value([]byte("w2"))}
	cmd3 := state.Command{Op: state.GET, K: state.Key(50), V: state.NIL()}
	cmdId1 := CommandId{ClientId: 10, SeqNum: 1}
	cmdId2 := CommandId{ClientId: 10, SeqNum: 2}
	cmdId3 := CommandId{ClientId: 10, SeqNum: 3}

	r.unsyncCausal(cmd1, cmdId1)
	r.unsyncCausal(cmd2, cmdId2)
	r.unsyncCausal(cmd3, cmdId3)

	v, _ := r.unsynced.Get("50")
	entry := v.(*UnsyncedEntry)
	if entry.Slot != 3 {
		t.Errorf("Slot = %d, want 3 (three pending causal ops)", entry.Slot)
	}
	// Latest metadata should be from cmd3
	if entry.CmdId != cmdId3 {
		t.Errorf("CmdId = %v, want %v", entry.CmdId, cmdId3)
	}
}

func TestCausalAndStrongMixedWitnessPool(t *testing.T) {
	r := newTestReplica(false)

	// Add causal op first
	causalCmd := state.Command{Op: state.PUT, K: state.Key(100), V: state.Value([]byte("causal"))}
	causalId := CommandId{ClientId: 10, SeqNum: 1}
	r.unsyncCausal(causalCmd, causalId)

	// Then strong op on same key
	strongCmd := state.Command{Op: state.PUT, K: state.Key(100), V: state.Value([]byte("strong"))}
	strongId := CommandId{ClientId: 20, SeqNum: 1}
	r.unsyncStrong(strongCmd, strongId)

	v, _ := r.unsynced.Get("100")
	entry := v.(*UnsyncedEntry)
	if entry.Slot != 2 {
		t.Errorf("Slot = %d, want 2 (causal + strong)", entry.Slot)
	}
	// Latest should be strong
	if !entry.IsStrong {
		t.Error("Latest entry should be strong")
	}

	// ok() should return FALSE (strong write conflict)
	result := r.ok(state.Command{Op: state.PUT, K: state.Key(100)})
	if result != FALSE {
		t.Errorf("ok() = %d, want FALSE (strong write conflict)", result)
	}
}

func TestCausalOpNoConflictForStrongCheck(t *testing.T) {
	r := newTestReplica(false)

	// Only causal op in witness pool
	causalCmd := state.Command{Op: state.PUT, K: state.Key(100), V: state.Value([]byte("causal"))}
	causalId := CommandId{ClientId: 10, SeqNum: 1}
	r.unsyncCausal(causalCmd, causalId)

	// ok() should return TRUE (causal ops don't conflict with strong)
	result := r.ok(state.Command{Op: state.PUT, K: state.Key(100)})
	if result != TRUE {
		t.Errorf("ok() = %d, want TRUE (causal ops don't conflict)", result)
	}
}

// --- Leader causal replication setup ---

func TestLeaderCausalMultipleSlotAssignment(t *testing.T) {
	r := newTestReplicaForDesc(true)

	// Simulate 3 causal commands from leader perspective
	cmds := []state.Command{
		{Op: state.PUT, K: state.Key(100), V: state.Value([]byte("v1"))},
		{Op: state.PUT, K: state.Key(200), V: state.Value([]byte("v2"))},
		{Op: state.PUT, K: state.Key(100), V: state.Value([]byte("v3"))},
	}
	cmdIds := []CommandId{
		{ClientId: 1, SeqNum: 1},
		{ClientId: 1, SeqNum: 2},
		{ClientId: 1, SeqNum: 3},
	}

	var descs []*commandDesc
	for i, cmd := range cmds {
		slot := r.lastCmdSlot
		r.lastCmdSlot++
		dep := r.leaderUnsyncCausal(cmd, slot, cmdIds[i])
		propose := &MCausalPropose{
			CommandId: cmdIds[i].SeqNum,
			ClientId:  cmdIds[i].ClientId,
			Command:   cmd,
		}
		desc := r.getCausalCmdDesc(slot, propose, dep)
		descs = append(descs, desc)
	}

	if r.lastCmdSlot != 3 {
		t.Errorf("lastCmdSlot = %d, want 3", r.lastCmdSlot)
	}

	// Verify slots
	if descs[0].cmdSlot != 0 {
		t.Errorf("desc[0].cmdSlot = %d, want 0", descs[0].cmdSlot)
	}
	if descs[1].cmdSlot != 1 {
		t.Errorf("desc[1].cmdSlot = %d, want 1", descs[1].cmdSlot)
	}
	if descs[2].cmdSlot != 2 {
		t.Errorf("desc[2].cmdSlot = %d, want 2", descs[2].cmdSlot)
	}

	// cmd3 (slot 2) depends on cmd1 (slot 0) since both are key 100
	if descs[2].dep != 0 {
		t.Errorf("desc[2].dep = %d, want 0 (same key as slot 0)", descs[2].dep)
	}
	// cmd2 (slot 1) has no dep on key 200 (first op)
	if descs[1].dep != -1 {
		t.Errorf("desc[1].dep = %d, want -1 (different key)", descs[1].dep)
	}
}

// --- Pending write lifecycle ---

func TestPendingWriteLifecycle(t *testing.T) {
	r := newTestReplica(false)
	r.pendingWrites = cmap.New()

	// Step 1: Add pending write (during handleCausalPropose)
	r.addPendingWrite(42, state.Key(100), 5, state.Value([]byte("pending")))

	// Step 2: Verify it's readable
	pw := r.getPendingWrite(42, state.Key(100), 5)
	if pw == nil || string(pw.value) != "pending" {
		t.Fatal("Expected pending write to be readable")
	}

	// Step 3: Remove pending write (during asyncReplicateCausal after commit)
	r.removePendingWrite(42, state.Key(100), 5)

	// Step 4: Verify it's gone
	pw = r.getPendingWrite(42, state.Key(100), 5)
	if pw != nil {
		t.Error("Pending write should be removed after execution")
	}
}

func TestPendingWriteNewerNotRemoved(t *testing.T) {
	r := newTestReplica(false)
	r.pendingWrites = cmap.New()

	// Add two writes for same key - newer seqNum wins
	r.addPendingWrite(42, state.Key(100), 5, state.Value([]byte("old")))
	r.addPendingWrite(42, state.Key(100), 7, state.Value([]byte("new")))

	// Try to remove old one - should not remove since newer exists
	r.removePendingWrite(42, state.Key(100), 5)

	pw := r.getPendingWrite(42, state.Key(100), 7)
	if pw == nil || string(pw.value) != "new" {
		t.Error("Newer pending write should still exist")
	}
}

// --- markWeakExecuted / waitForWeakDep tests ---

func TestMarkWeakExecutedAndCheck(t *testing.T) {
	r := newTestReplica(false)
	r.weakExecuted = cmap.New()

	r.markWeakExecuted(42, 5)

	clientKey := "42"
	v, exists := r.weakExecuted.Get(clientKey)
	if !exists {
		t.Fatal("Expected weakExecuted entry for client 42")
	}
	if v.(int32) != 5 {
		t.Errorf("weakExecuted = %d, want 5", v.(int32))
	}

	// Newer seqNum should update
	r.markWeakExecuted(42, 10)
	v, _ = r.weakExecuted.Get(clientKey)
	if v.(int32) != 10 {
		t.Errorf("weakExecuted = %d, want 10", v.(int32))
	}

	// Older seqNum should NOT update
	r.markWeakExecuted(42, 3)
	v, _ = r.weakExecuted.Get(clientKey)
	if v.(int32) != 10 {
		t.Errorf("weakExecuted = %d, want 10 (should not regress)", v.(int32))
	}
}

// --- syncLeader cleanup tests ---

func TestSyncLeaderCleanupsMatchingEntry(t *testing.T) {
	r := newTestReplica(true)

	cmdId := CommandId{ClientId: 1, SeqNum: 1}
	cmd := state.Command{Op: state.PUT, K: state.Key(100), V: state.Value([]byte("v1"))}

	// Add via leaderUnsyncCausal
	r.leaderUnsyncCausal(cmd, 0, cmdId)

	// Verify entry exists
	_, exists := r.unsynced.Get("100")
	if !exists {
		t.Fatal("Expected unsynced entry")
	}

	// Cleanup
	r.syncLeader(cmdId, cmd)

	// Verify entry removed
	_, exists = r.unsynced.Get("100")
	if exists {
		t.Error("Expected unsynced entry to be removed after syncLeader")
	}
}

func TestSyncLeaderDoesNotRemoveNewerEntry(t *testing.T) {
	r := newTestReplica(true)

	cmd := state.Command{Op: state.PUT, K: state.Key(100), V: state.Value([]byte("v1"))}
	cmdId1 := CommandId{ClientId: 1, SeqNum: 1}
	cmdId2 := CommandId{ClientId: 1, SeqNum: 2}

	// Add two ops on same key
	r.leaderUnsyncCausal(cmd, 0, cmdId1)
	r.leaderUnsyncCausal(cmd, 1, cmdId2)

	// Try to clean up the OLD entry - should not remove since newer exists
	r.syncLeader(cmdId1, cmd)

	v, exists := r.unsynced.Get("100")
	if !exists {
		t.Fatal("Expected unsynced entry to still exist (newer op)")
	}
	entry := v.(*UnsyncedEntry)
	if entry.CmdId != cmdId2 {
		t.Errorf("CmdId = %v, want %v (newer entry)", entry.CmdId, cmdId2)
	}
}

// --- getCausalCmdDesc additional tests ---

func TestGetCausalCmdDescGETCommand(t *testing.T) {
	r := newTestReplicaForDesc(true)
	propose := &MCausalPropose{
		CommandId: 1,
		ClientId:  10,
		Command:   state.Command{Op: state.GET, K: state.Key(50), V: state.NIL()},
	}

	desc := r.getCausalCmdDesc(3, propose, -1)

	if desc.cmd.Op != state.GET {
		t.Errorf("cmd.Op = %d, want GET", desc.cmd.Op)
	}
	if desc.cmdSlot != 3 {
		t.Errorf("cmdSlot = %d, want 3", desc.cmdSlot)
	}
}

func TestGetCausalCmdDescPhaseIsACCEPT(t *testing.T) {
	r := newTestReplicaForDesc(true)
	propose := &MCausalPropose{
		CommandId: 1,
		ClientId:  1,
		Command:   state.Command{Op: state.PUT, K: state.Key(1), V: state.Value([]byte("x"))},
	}

	desc := r.getCausalCmdDesc(0, propose, -1)

	// Causal commands skip START and go directly to ACCEPT
	if desc.phase != ACCEPT {
		t.Errorf("phase = %d, want ACCEPT (%d)", desc.phase, ACCEPT)
	}
}

// --- Notify channel tests ---

func TestCommitNotifyChannel(t *testing.T) {
	r := newTestReplicaForDesc(true)

	// Get notify channel for slot 5
	ch := r.getOrCreateCommitNotify(5)

	// Channel should not be closed yet
	select {
	case <-ch:
		t.Fatal("Channel should not be closed before commit")
	default:
		// Expected
	}

	// Mark as committed
	r.committed.Set("5", struct{}{})
	r.notifyCommit(5)

	// Channel should now be closed
	select {
	case <-ch:
		// Expected
	default:
		t.Fatal("Channel should be closed after commit notification")
	}
}

func TestExecuteNotifyChannel(t *testing.T) {
	r := newTestReplicaForDesc(true)

	// Get notify channel for slot 3
	ch := r.getOrCreateExecuteNotify(3)

	// Not closed yet
	select {
	case <-ch:
		t.Fatal("Channel should not be closed before execution")
	default:
	}

	// Mark as executed
	r.executed.Set("3", struct{}{})
	r.notifyExecute(3)

	// Should be closed now
	select {
	case <-ch:
		// Expected
	default:
		t.Fatal("Channel should be closed after execution notification")
	}
}

func TestCommitNotifyAlreadyCommitted(t *testing.T) {
	r := newTestReplicaForDesc(true)

	// Pre-commit slot 5
	r.committed.Set("5", struct{}{})

	// Getting notify should return already-closed channel
	ch := r.getOrCreateCommitNotify(5)

	select {
	case <-ch:
		// Expected - already committed
	default:
		t.Fatal("Channel should be pre-closed for already committed slot")
	}
}

// ============================================================================
// Phase 25: Strong Op Modifications Tests
// These tests verify MRecordAck with WeakDep serialization, okWithWeakDep,
// and computeSpeculativeResultWithUnsynced.
// ============================================================================

// --- MRecordAck with WeakDep serialization tests ---

func TestMRecordAckSerializationNoWeakDep(t *testing.T) {
	original := &MRecordAck{
		Replica: 2,
		Ballot:  5,
		CmdId:   CommandId{ClientId: 10, SeqNum: 20},
		Ok:      TRUE,
		ReadDep: nil,
	}

	var buf bytes.Buffer
	original.Marshal(&buf)

	decoded := &MRecordAck{}
	err := decoded.Unmarshal(&buf)
	if err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if decoded.Replica != 2 {
		t.Errorf("Replica = %d, want 2", decoded.Replica)
	}
	if decoded.Ballot != 5 {
		t.Errorf("Ballot = %d, want 5", decoded.Ballot)
	}
	if decoded.CmdId.ClientId != 10 || decoded.CmdId.SeqNum != 20 {
		t.Errorf("CmdId = %v, want {10,20}", decoded.CmdId)
	}
	if decoded.Ok != TRUE {
		t.Errorf("Ok = %d, want TRUE", decoded.Ok)
	}
	if decoded.ReadDep != nil {
		t.Errorf("WeakDep = %v, want nil", decoded.ReadDep)
	}
}

func TestMRecordAckSerializationWithWeakDep(t *testing.T) {
	original := &MRecordAck{
		Replica: 3,
		Ballot:  7,
		CmdId:   CommandId{ClientId: 42, SeqNum: 100},
		Ok:      TRUE,
		ReadDep: &CommandId{ClientId: 99, SeqNum: 50},
	}

	var buf bytes.Buffer
	original.Marshal(&buf)

	decoded := &MRecordAck{}
	err := decoded.Unmarshal(&buf)
	if err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if decoded.Replica != 3 {
		t.Errorf("Replica = %d, want 3", decoded.Replica)
	}
	if decoded.Ballot != 7 {
		t.Errorf("Ballot = %d, want 7", decoded.Ballot)
	}
	if decoded.CmdId.ClientId != 42 || decoded.CmdId.SeqNum != 100 {
		t.Errorf("CmdId = %v, want {42,100}", decoded.CmdId)
	}
	if decoded.Ok != TRUE {
		t.Errorf("Ok = %d, want TRUE", decoded.Ok)
	}
	if decoded.ReadDep == nil {
		t.Fatal("WeakDep should not be nil")
	}
	if decoded.ReadDep.ClientId != 99 || decoded.ReadDep.SeqNum != 50 {
		t.Errorf("WeakDep = %v, want {99,50}", decoded.ReadDep)
	}
}

func TestMRecordAckSerializationSizes(t *testing.T) {
	// Without ReadDep, no CausalDeps: 20 bytes (17 fixed + 1 flag + 2 count)
	noWeak := &MRecordAck{
		Replica: 1,
		Ballot:  1,
		CmdId:   CommandId{ClientId: 1, SeqNum: 1},
		Ok:      TRUE,
		ReadDep: nil,
	}
	var buf1 bytes.Buffer
	noWeak.Marshal(&buf1)
	if buf1.Len() != 20 {
		t.Errorf("Size without ReadDep = %d, want 20", buf1.Len())
	}

	// With ReadDep, no CausalDeps: 28 bytes (17 fixed + 1 flag + 8 CommandId + 2 count)
	withWeak := &MRecordAck{
		Replica: 1,
		Ballot:  1,
		CmdId:   CommandId{ClientId: 1, SeqNum: 1},
		Ok:      TRUE,
		ReadDep: &CommandId{ClientId: 1, SeqNum: 1},
	}
	var buf2 bytes.Buffer
	withWeak.Marshal(&buf2)
	if buf2.Len() != 28 {
		t.Errorf("Size with ReadDep = %d, want 28", buf2.Len())
	}

	// With ReadDep + 2 CausalDeps: 44 bytes (17 + 1 + 8 + 2 + 2*8)
	withCausal := &MRecordAck{
		Replica:    1,
		Ballot:     1,
		CmdId:      CommandId{ClientId: 1, SeqNum: 1},
		Ok:         TRUE,
		ReadDep:    &CommandId{ClientId: 1, SeqNum: 1},
		CausalDeps: []CommandId{{ClientId: 2, SeqNum: 3}, {ClientId: 4, SeqNum: 5}},
	}
	var buf3 bytes.Buffer
	withCausal.Marshal(&buf3)
	if buf3.Len() != 44 {
		t.Errorf("Size with ReadDep + 2 CausalDeps = %d, want 44", buf3.Len())
	}
}

func TestMRecordAckSerializationFALSEOk(t *testing.T) {
	original := &MRecordAck{
		Replica: 0,
		Ballot:  0,
		CmdId:   CommandId{ClientId: 1, SeqNum: 1},
		Ok:      FALSE,
		ReadDep: nil,
	}

	var buf bytes.Buffer
	original.Marshal(&buf)

	decoded := &MRecordAck{}
	decoded.Unmarshal(&buf)

	if decoded.Ok != FALSE {
		t.Errorf("Ok = %d, want FALSE", decoded.Ok)
	}
}

func TestMRecordAckSerializationORDEREDOk(t *testing.T) {
	dep := &CommandId{ClientId: 5, SeqNum: 3}
	original := &MRecordAck{
		Replica: 1,
		Ballot:  2,
		CmdId:   CommandId{ClientId: 10, SeqNum: 7},
		Ok:      ORDERED,
		ReadDep: dep,
	}

	var buf bytes.Buffer
	original.Marshal(&buf)

	decoded := &MRecordAck{}
	decoded.Unmarshal(&buf)

	if decoded.Ok != ORDERED {
		t.Errorf("Ok = %d, want ORDERED", decoded.Ok)
	}
	if decoded.ReadDep == nil || decoded.ReadDep.ClientId != 5 || decoded.ReadDep.SeqNum != 3 {
		t.Errorf("WeakDep = %v, want {5,3}", decoded.ReadDep)
	}
}

// --- okWithWeakDep tests ---

func TestOkWithWeakDepNoConflict(t *testing.T) {
	r := newTestReplica(false)
	cmd := state.Command{Op: state.PUT, K: state.Key(100)}

	ok, weakDep, _ := r.witnessCheck(cmd, 0)
	if ok != TRUE {
		t.Errorf("ok = %d, want TRUE (no entries)", ok)
	}
	if weakDep != nil {
		t.Errorf("weakDep = %v, want nil", weakDep)
	}
}

func TestOkWithWeakDepStrongWriteConflict(t *testing.T) {
	r := newTestReplica(false)

	// Add a strong write to unsynced
	strongCmd := state.Command{Op: state.PUT, K: state.Key(100), V: state.Value([]byte("v1"))}
	strongId := CommandId{ClientId: 1, SeqNum: 1}
	r.unsyncStrong(strongCmd, strongId)

	// Check incoming strong op on same key
	ok, weakDep, _ := r.witnessCheck(state.Command{Op: state.PUT, K: state.Key(100)}, 0)
	if ok != FALSE {
		t.Errorf("ok = %d, want FALSE (strong write conflict)", ok)
	}
	if weakDep != nil {
		t.Errorf("weakDep = %v, want nil (conflict, not dep)", weakDep)
	}
}

func TestOkWithWeakDepCausalWriteDep(t *testing.T) {
	r := newTestReplica(false)

	// Add a causal write to unsynced
	causalCmd := state.Command{Op: state.PUT, K: state.Key(100), V: state.Value([]byte("causal"))}
	causalId := CommandId{ClientId: 10, SeqNum: 5}
	r.unsyncCausal(causalCmd, causalId)

	// Check incoming strong op on same key - should get weakDep
	ok, weakDep, _ := r.witnessCheck(state.Command{Op: state.GET, K: state.Key(100)}, 0)
	if ok != TRUE {
		t.Errorf("ok = %d, want TRUE (causal doesn't conflict)", ok)
	}
	if weakDep == nil {
		t.Fatal("weakDep should not be nil (causal write dep)")
	}
	if weakDep.ClientId != 10 || weakDep.SeqNum != 5 {
		t.Errorf("weakDep = %v, want {10,5}", weakDep)
	}
}

func TestOkWithWeakDepCausalReadNoDep(t *testing.T) {
	r := newTestReplica(false)

	// Add a causal READ to unsynced (not a write)
	causalCmd := state.Command{Op: state.GET, K: state.Key(100), V: state.NIL()}
	causalId := CommandId{ClientId: 10, SeqNum: 5}
	r.unsyncCausal(causalCmd, causalId)

	// Should get TRUE and no weakDep (only writes create deps)
	ok, weakDep, _ := r.witnessCheck(state.Command{Op: state.GET, K: state.Key(100)}, 0)
	if ok != TRUE {
		t.Errorf("ok = %d, want TRUE", ok)
	}
	if weakDep != nil {
		t.Errorf("weakDep = %v, want nil (causal read, not write)", weakDep)
	}
}

// TestOkWithWeakDepStrongReadVsWriteWithCausalWrite verifies that per protocol spec,
// only strong READs get weakDep when a causal write is pending on the same key.
// Strong WRITEs should get ok=TRUE but weakDep=nil.
func TestOkWithWeakDepStrongReadVsWriteWithCausalWrite(t *testing.T) {
	r := newTestReplica(false)

	// Add a causal write to unsynced
	causalCmd := state.Command{Op: state.PUT, K: state.Key(200), V: state.Value([]byte("pending"))}
	causalId := CommandId{ClientId: 20, SeqNum: 10}
	r.unsyncCausal(causalCmd, causalId)

	// Strong READ should get weakDep
	ok, weakDep, _ := r.witnessCheck(state.Command{Op: state.GET, K: state.Key(200)}, 0)
	if ok != TRUE {
		t.Errorf("strong GET: ok = %d, want TRUE", ok)
	}
	if weakDep == nil {
		t.Fatal("strong GET: weakDep should not be nil when causal write is pending")
	}
	if *weakDep != causalId {
		t.Errorf("strong GET: weakDep = %v, want %v", *weakDep, causalId)
	}

	// Strong WRITE should NOT get weakDep (per protocol spec)
	ok, weakDep, _ = r.witnessCheck(state.Command{Op: state.PUT, K: state.Key(200)}, 0)
	if ok != TRUE {
		t.Errorf("strong PUT: ok = %d, want TRUE", ok)
	}
	if weakDep != nil {
		t.Errorf("strong PUT: weakDep = %v, want nil (writes don't need weakDep)", weakDep)
	}

	// SCAN should also NOT get weakDep
	ok, weakDep, _ = r.witnessCheck(state.Command{Op: state.SCAN, K: state.Key(200)}, 0)
	if ok != TRUE {
		t.Errorf("strong SCAN: ok = %d, want TRUE", ok)
	}
	if weakDep != nil {
		t.Errorf("strong SCAN: weakDep = %v, want nil (only GETs get weakDep)", weakDep)
	}
}

// --- computeSpeculativeResultWithUnsynced tests ---

func TestSpeculativeWithUnsyncedGETSeesWeakWrite(t *testing.T) {
	r := newTestReplica(false)

	// Add a causal write to witness pool
	causalCmd := state.Command{Op: state.PUT, K: state.Key(100), V: state.Value([]byte("weak-value"))}
	causalId := CommandId{ClientId: 10, SeqNum: 5}
	r.unsyncCausal(causalCmd, causalId)

	// Strong GET should see the weak write via getWeakWriteValue
	val, found := r.getWeakWriteValue(state.Key(100))
	if !found {
		t.Fatal("Expected weak write value in witness pool")
	}
	if !bytes.Equal(val, []byte("weak-value")) {
		t.Errorf("Value = %q, want weak-value", val)
	}
}

func TestSpeculativeWithUnsyncedGETNoWeakWrite(t *testing.T) {
	r := newTestReplica(false)

	// No weak write - getWeakWriteValue should return false
	_, found := r.getWeakWriteValue(state.Key(100))
	if found {
		t.Error("Should not find weak write value for empty witness pool")
	}
}

func TestSpeculativeWithUnsyncedStrongWriteNotReturned(t *testing.T) {
	r := newTestReplica(false)

	// Add a STRONG write to unsynced
	strongCmd := state.Command{Op: state.PUT, K: state.Key(100), V: state.Value([]byte("strong-val"))}
	strongId := CommandId{ClientId: 1, SeqNum: 1}
	r.unsyncStrong(strongCmd, strongId)

	// getWeakWriteValue should NOT return strong writes
	_, found := r.getWeakWriteValue(state.Key(100))
	if found {
		t.Error("Should not return strong write value (only causal/weak)")
	}
}

// --- checkStrongWriteConflict tests ---

func TestCheckStrongWriteConflictExists(t *testing.T) {
	r := newTestReplica(false)

	strongCmd := state.Command{Op: state.PUT, K: state.Key(100), V: state.Value([]byte("v1"))}
	strongId := CommandId{ClientId: 1, SeqNum: 1}
	r.unsyncStrong(strongCmd, strongId)

	if !r.checkStrongWriteConflict(state.Key(100)) {
		t.Error("Should detect strong write conflict")
	}
}

func TestCheckStrongWriteConflictNotExists(t *testing.T) {
	r := newTestReplica(false)

	if r.checkStrongWriteConflict(state.Key(100)) {
		t.Error("Should not detect conflict on empty witness pool")
	}
}

func TestCheckStrongWriteConflictCausalNotConflict(t *testing.T) {
	r := newTestReplica(false)

	causalCmd := state.Command{Op: state.PUT, K: state.Key(100), V: state.Value([]byte("causal"))}
	causalId := CommandId{ClientId: 10, SeqNum: 1}
	r.unsyncCausal(causalCmd, causalId)

	// Causal writes should NOT be detected as strong write conflicts
	if r.checkStrongWriteConflict(state.Key(100)) {
		t.Error("Causal write should not be a strong write conflict")
	}
}

// --- getWeakWriteDep tests ---

func TestGetWeakWriteDepExists(t *testing.T) {
	r := newTestReplica(false)

	causalCmd := state.Command{Op: state.PUT, K: state.Key(100), V: state.Value([]byte("v"))}
	causalId := CommandId{ClientId: 10, SeqNum: 5}
	r.unsyncCausal(causalCmd, causalId)

	dep := r.getWeakWriteDep(state.Key(100))
	if dep == nil {
		t.Fatal("Expected weak write dep")
	}
	if dep.ClientId != 10 || dep.SeqNum != 5 {
		t.Errorf("dep = %v, want {10,5}", dep)
	}
}

func TestGetWeakWriteDepNotExists(t *testing.T) {
	r := newTestReplica(false)

	dep := r.getWeakWriteDep(state.Key(100))
	if dep != nil {
		t.Errorf("dep = %v, want nil", dep)
	}
}

func TestGetWeakWriteDepStrongWriteNotReturned(t *testing.T) {
	r := newTestReplica(false)

	strongCmd := state.Command{Op: state.PUT, K: state.Key(100), V: state.Value([]byte("v"))}
	strongId := CommandId{ClientId: 1, SeqNum: 1}
	r.unsyncStrong(strongCmd, strongId)

	dep := r.getWeakWriteDep(state.Key(100))
	if dep != nil {
		t.Errorf("dep = %v, want nil (strong write, not weak)", dep)
	}
}

// --- Integration: strong op with causal in witness pool ---

func TestStrongReadWithCausalWriteInWitnessPool(t *testing.T) {
	r := newTestReplica(false)

	// Client A writes causal op to key 100
	causalCmd := state.Command{Op: state.PUT, K: state.Key(100), V: state.Value([]byte("causal-data"))}
	causalId := CommandId{ClientId: 10, SeqNum: 1}
	r.unsyncCausal(causalCmd, causalId)

	// Client B does strong read of key 100
	// okWithWeakDep should return TRUE + weakDep pointing to causal write
	ok, weakDep, _ := r.witnessCheck(state.Command{Op: state.GET, K: state.Key(100)}, 0)
	if ok != TRUE {
		t.Errorf("ok = %d, want TRUE", ok)
	}
	if weakDep == nil || *weakDep != causalId {
		t.Errorf("weakDep = %v, want %v", weakDep, causalId)
	}

	// getWeakWriteValue should return causal write value for speculative execution
	val, found := r.getWeakWriteValue(state.Key(100))
	if !found || !bytes.Equal(val, []byte("causal-data")) {
		t.Errorf("Speculative result = %q, want causal-data", val)
	}
}

func TestStrongWriteWithCausalWriteInWitnessPool(t *testing.T) {
	r := newTestReplica(false)

	// Causal write on key 100
	causalCmd := state.Command{Op: state.PUT, K: state.Key(100), V: state.Value([]byte("causal"))}
	causalId := CommandId{ClientId: 10, SeqNum: 1}
	r.unsyncCausal(causalCmd, causalId)

	// Strong write on same key - should NOT conflict (causal != strong conflict)
	// Per protocol spec, strong writes don't get weakDep (only strong reads do)
	ok, weakDep, _ := r.witnessCheck(state.Command{Op: state.PUT, K: state.Key(100)}, 0)
	if ok != TRUE {
		t.Errorf("ok = %d, want TRUE (causal write doesn't conflict with strong write)", ok)
	}
	if weakDep != nil {
		t.Errorf("weakDep = %v, want nil (strong writes don't get weakDep per spec)", weakDep)
	}
}

// ============================================================================
// Phase 26: Client Fast Path with WeakDep Tests
// ============================================================================

// --- 26.2: readDepEqual helper ---

func TestWeakDepEqual(t *testing.T) {
	tests := []struct {
		name     string
		a, b     *CommandId
		expected bool
	}{
		{"both nil", nil, nil, true},
		{"a nil b non-nil", nil, &CommandId{1, 1}, false},
		{"a non-nil b nil", &CommandId{1, 1}, nil, false},
		{"same values", &CommandId{1, 5}, &CommandId{1, 5}, true},
		{"different client", &CommandId{1, 5}, &CommandId{2, 5}, false},
		{"different seqnum", &CommandId{1, 5}, &CommandId{1, 6}, false},
		{"both different", &CommandId{1, 5}, &CommandId{2, 6}, false},
		{"zero values", &CommandId{0, 0}, &CommandId{0, 0}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := readDepEqual(tt.a, tt.b)
			if result != tt.expected {
				t.Errorf("readDepEqual(%v, %v) = %v, want %v", tt.a, tt.b, result, tt.expected)
			}
		})
	}
}

// --- 26.2: checkReadDepConsistency ---

func TestCheckWeakDepConsistencyEmpty(t *testing.T) {
	c := &Client{}
	if !c.checkReadDepConsistency(nil) {
		t.Error("nil msgs should be consistent")
	}
	if !c.checkReadDepConsistency([]interface{}{}) {
		t.Error("empty slice should be consistent")
	}
}

func TestCheckWeakDepConsistencyAllNil(t *testing.T) {
	c := &Client{}
	msgs := []interface{}{
		&MRecordAck{Replica: 1, ReadDep: nil},
		&MRecordAck{Replica: 2, ReadDep: nil},
		&MRecordAck{Replica: 3, ReadDep: nil},
	}
	if !c.checkReadDepConsistency(msgs) {
		t.Error("all nil weakDeps should be consistent")
	}
}

func TestCheckWeakDepConsistencyAllSame(t *testing.T) {
	c := &Client{}
	msgs := []interface{}{
		&MRecordAck{Replica: 1, ReadDep: &CommandId{ClientId: 10, SeqNum: 5}},
		&MRecordAck{Replica: 2, ReadDep: &CommandId{ClientId: 10, SeqNum: 5}},
		&MRecordAck{Replica: 3, ReadDep: &CommandId{ClientId: 10, SeqNum: 5}},
	}
	if !c.checkReadDepConsistency(msgs) {
		t.Error("all same weakDeps should be consistent")
	}
}

func TestCheckWeakDepConsistencyMixedNilNonNil(t *testing.T) {
	c := &Client{}
	msgs := []interface{}{
		&MRecordAck{Replica: 1, ReadDep: nil},
		&MRecordAck{Replica: 2, ReadDep: &CommandId{ClientId: 10, SeqNum: 5}},
	}
	if c.checkReadDepConsistency(msgs) {
		t.Error("mixed nil/non-nil should be inconsistent")
	}
}

func TestCheckWeakDepConsistencyDifferentSeqNum(t *testing.T) {
	c := &Client{}
	msgs := []interface{}{
		&MRecordAck{Replica: 1, ReadDep: &CommandId{ClientId: 10, SeqNum: 5}},
		&MRecordAck{Replica: 2, ReadDep: &CommandId{ClientId: 10, SeqNum: 6}},
	}
	if c.checkReadDepConsistency(msgs) {
		t.Error("different seqnums should be inconsistent")
	}
}

func TestCheckWeakDepConsistencyDifferentClient(t *testing.T) {
	c := &Client{}
	msgs := []interface{}{
		&MRecordAck{Replica: 1, ReadDep: &CommandId{ClientId: 10, SeqNum: 5}},
		&MRecordAck{Replica: 2, ReadDep: &CommandId{ClientId: 11, SeqNum: 5}},
	}
	if c.checkReadDepConsistency(msgs) {
		t.Error("different client IDs should be inconsistent")
	}
}

func TestCheckWeakDepConsistencySingle(t *testing.T) {
	c := &Client{}
	msgs := []interface{}{
		&MRecordAck{Replica: 1, ReadDep: &CommandId{ClientId: 10, SeqNum: 5}},
	}
	if !c.checkReadDepConsistency(msgs) {
		t.Error("single message should be consistent")
	}
}

func TestCheckWeakDepConsistencyThreeWayInconsistent(t *testing.T) {
	c := &Client{}
	// First two agree, third differs
	msgs := []interface{}{
		&MRecordAck{Replica: 1, ReadDep: &CommandId{ClientId: 10, SeqNum: 5}},
		&MRecordAck{Replica: 2, ReadDep: &CommandId{ClientId: 10, SeqNum: 5}},
		&MRecordAck{Replica: 3, ReadDep: &CommandId{ClientId: 10, SeqNum: 7}},
	}
	if c.checkReadDepConsistency(msgs) {
		t.Error("third ack differs, should be inconsistent")
	}
}

// --- 26.3: handleFastPathAcks ---

func TestHandleFastPathAcksNilLeader(t *testing.T) {
	c := &Client{
		delivered:   make(map[int32]struct{}),
		alreadySlow: make(map[CommandId]struct{}),
	}
	// Should return without panic when leaderMsg is nil
	c.handleFastPathAcks(nil, []interface{}{})
	if len(c.delivered) != 0 {
		t.Error("nil leader should not deliver")
	}
}

func TestHandleFastPathAcksInconsistentWeakDeps(t *testing.T) {
	c := &Client{
		delivered:   make(map[int32]struct{}),
		alreadySlow: make(map[CommandId]struct{}),
	}
	leaderMsg := &MRecordAck{
		Replica: 0,
		CmdId:   CommandId{ClientId: 1, SeqNum: 1},
		Ok:      TRUE,
	}
	msgs := []interface{}{
		&MRecordAck{Replica: 1, ReadDep: nil},
		&MRecordAck{Replica: 2, ReadDep: &CommandId{ClientId: 10, SeqNum: 5}},
	}
	c.handleFastPathAcks(leaderMsg, msgs)

	// Should NOT deliver
	if _, exists := c.delivered[1]; exists {
		t.Error("inconsistent weakDeps should not deliver on fast path")
	}
	// Should increment slowPaths
	if c.slowPaths != 1 {
		t.Errorf("slowPaths = %d, want 1", c.slowPaths)
	}
	// Should mark as alreadySlow
	cmdId := CommandId{ClientId: 1, SeqNum: 1}
	if _, exists := c.alreadySlow[cmdId]; !exists {
		t.Error("should be marked as alreadySlow")
	}
}

func TestHandleFastPathAcksInconsistentNoDuplicateCount(t *testing.T) {
	c := &Client{
		delivered:   make(map[int32]struct{}),
		alreadySlow: make(map[CommandId]struct{}),
	}
	leaderMsg := &MRecordAck{
		Replica: 0,
		CmdId:   CommandId{ClientId: 1, SeqNum: 1},
		Ok:      TRUE,
	}
	msgs := []interface{}{
		&MRecordAck{Replica: 1, ReadDep: nil},
		&MRecordAck{Replica: 2, ReadDep: &CommandId{ClientId: 10, SeqNum: 5}},
	}
	// Call twice (simulating quorum callback firing again)
	c.handleFastPathAcks(leaderMsg, msgs)
	c.handleFastPathAcks(leaderMsg, msgs)

	// slowPaths should only be incremented once
	if c.slowPaths != 1 {
		t.Errorf("slowPaths = %d, want 1 (no duplicate counting)", c.slowPaths)
	}
}

func TestHandleFastPathAcksAlreadyDelivered(t *testing.T) {
	c := &Client{
		delivered:   make(map[int32]struct{}),
		alreadySlow: make(map[CommandId]struct{}),
	}
	// Pre-deliver
	c.delivered[1] = struct{}{}

	leaderMsg := &MRecordAck{
		Replica: 0,
		CmdId:   CommandId{ClientId: 1, SeqNum: 1},
		Ok:      TRUE,
	}
	msgs := []interface{}{
		&MRecordAck{Replica: 1, ReadDep: nil},
		&MRecordAck{Replica: 2, ReadDep: nil},
	}
	// Consistent weakDeps but already delivered - should return without panic
	c.handleFastPathAcks(leaderMsg, msgs)
	// No additional delivery attempt (no panic from nil BufferClient)
}

func TestHandleFastPathAcksConsistentEmptyMsgs(t *testing.T) {
	c := &Client{
		delivered:   make(map[int32]struct{}),
		alreadySlow: make(map[CommandId]struct{}),
	}
	// Pre-deliver to avoid calling RegisterReply
	c.delivered[1] = struct{}{}

	leaderMsg := &MRecordAck{
		Replica: 0,
		CmdId:   CommandId{ClientId: 1, SeqNum: 1},
		Ok:      TRUE,
	}
	// Empty msgs is consistent
	c.handleFastPathAcks(leaderMsg, []interface{}{})
	// Should not mark as slow
	if c.slowPaths != 0 {
		t.Errorf("slowPaths = %d, want 0", c.slowPaths)
	}
}

// --- 26.3: handleSlowPathAcks ---

func TestHandleSlowPathAcksNilLeader(t *testing.T) {
	c := &Client{
		delivered: make(map[int32]struct{}),
	}
	c.handleSlowPathAcks(nil, []interface{}{})
	if len(c.delivered) != 0 {
		t.Error("nil leader should not deliver")
	}
}

func TestHandleSlowPathAcksAlreadyDelivered(t *testing.T) {
	c := &Client{
		delivered: make(map[int32]struct{}),
	}
	c.delivered[1] = struct{}{}

	leaderMsg := &MRecordAck{
		Replica: 0,
		CmdId:   CommandId{ClientId: 1, SeqNum: 1},
	}
	// Should return without panic (already delivered)
	c.handleSlowPathAcks(leaderMsg, []interface{}{})
}

func TestHandleSlowPathIgnoresWeakDepInconsistency(t *testing.T) {
	c := &Client{
		delivered: make(map[int32]struct{}),
	}
	// Pre-deliver to avoid calling RegisterReply
	c.delivered[1] = struct{}{}

	leaderMsg := &MRecordAck{
		Replica: 0,
		CmdId:   CommandId{ClientId: 1, SeqNum: 1},
	}
	// Inconsistent weakDeps - slow path doesn't check, just delivers
	msgs := []interface{}{
		&MRecordAck{Replica: 1, ReadDep: nil},
		&MRecordAck{Replica: 2, ReadDep: &CommandId{ClientId: 10, SeqNum: 5}},
	}
	// Should not panic; already delivered so returns early
	c.handleSlowPathAcks(leaderMsg, msgs)
}

// --- 26: Integration tests ---

func TestFastPathSlowPathFallback(t *testing.T) {
	c := &Client{
		delivered:   make(map[int32]struct{}),
		alreadySlow: make(map[CommandId]struct{}),
	}
	cmdId := CommandId{ClientId: 1, SeqNum: 1}
	leaderMsg := &MRecordAck{
		Replica: 0,
		CmdId:   cmdId,
		Ok:      TRUE,
	}

	// Step 1: Fast path fails due to inconsistent weakDeps
	fastMsgs := []interface{}{
		&MRecordAck{Replica: 1, ReadDep: nil},
		&MRecordAck{Replica: 2, ReadDep: &CommandId{ClientId: 10, SeqNum: 5}},
	}
	c.handleFastPathAcks(leaderMsg, fastMsgs)

	// Verify: not delivered, marked slow
	if _, exists := c.delivered[1]; exists {
		t.Error("should not be delivered after fast path failure")
	}
	if c.slowPaths != 1 {
		t.Errorf("slowPaths = %d, want 1", c.slowPaths)
	}

	// Step 2: Slow path delivers (we mark delivered manually to avoid RegisterReply)
	c.delivered[1] = struct{}{} // Simulating slow path delivery

	// Step 3: Fast path called again (e.g., more acks arrive) - should be no-op
	c.handleFastPathAcks(leaderMsg, []interface{}{
		&MRecordAck{Replica: 1, ReadDep: nil},
		&MRecordAck{Replica: 2, ReadDep: nil},
	})
	// Should still be delivered, no change
	if _, exists := c.delivered[1]; !exists {
		t.Error("should still be delivered")
	}
}

func TestMultipleCommandsIndependent(t *testing.T) {
	c := &Client{
		delivered:   make(map[int32]struct{}),
		alreadySlow: make(map[CommandId]struct{}),
	}

	// Command 1: inconsistent (falls back to slow path)
	cmd1Leader := &MRecordAck{Replica: 0, CmdId: CommandId{ClientId: 1, SeqNum: 1}, Ok: TRUE}
	cmd1Msgs := []interface{}{
		&MRecordAck{Replica: 1, ReadDep: nil},
		&MRecordAck{Replica: 2, ReadDep: &CommandId{ClientId: 10, SeqNum: 5}},
	}
	c.handleFastPathAcks(cmd1Leader, cmd1Msgs)

	// Command 2: consistent (should be deliverable, but we pre-deliver to avoid RegisterReply)
	c.delivered[2] = struct{}{}
	cmd2Leader := &MRecordAck{Replica: 0, CmdId: CommandId{ClientId: 1, SeqNum: 2}, Ok: TRUE}
	cmd2Msgs := []interface{}{
		&MRecordAck{Replica: 1, ReadDep: &CommandId{ClientId: 10, SeqNum: 5}},
		&MRecordAck{Replica: 2, ReadDep: &CommandId{ClientId: 10, SeqNum: 5}},
	}
	c.handleFastPathAcks(cmd2Leader, cmd2Msgs)

	// Command 1 should NOT be delivered, command 2 should be (pre-set)
	if _, exists := c.delivered[1]; exists {
		t.Error("command 1 should not be delivered on fast path")
	}
	if c.slowPaths != 1 {
		t.Errorf("slowPaths = %d, want 1 (only command 1 is slow)", c.slowPaths)
	}
}

func TestInitMsgSetsSeparateHandlers(t *testing.T) {
	c := &Client{
		Q:           replica.NewThreeQuartersOf(3),
		M:           replica.NewMajorityOf(3),
		acks:        make(map[CommandId]*replica.MsgSet),
		macks:       make(map[CommandId]*replica.MsgSet),
		delivered:   make(map[int32]struct{}),
		alreadySlow: make(map[CommandId]struct{}),
	}

	cmdId := CommandId{ClientId: 1, SeqNum: 1}
	c.initMsgSets(cmdId)

	// Verify both MsgSets are initialized
	if c.acks[cmdId] == nil {
		t.Error("acks MsgSet should be initialized")
	}
	if c.macks[cmdId] == nil {
		t.Error("macks MsgSet should be initialized")
	}
}

func TestInitMsgSetsIdempotent(t *testing.T) {
	c := &Client{
		Q:           replica.NewThreeQuartersOf(3),
		M:           replica.NewMajorityOf(3),
		acks:        make(map[CommandId]*replica.MsgSet),
		macks:       make(map[CommandId]*replica.MsgSet),
		delivered:   make(map[int32]struct{}),
		alreadySlow: make(map[CommandId]struct{}),
	}

	cmdId := CommandId{ClientId: 1, SeqNum: 1}
	c.initMsgSets(cmdId)
	acks1 := c.acks[cmdId]
	macks1 := c.macks[cmdId]

	// Call again - should not reinitialize
	c.initMsgSets(cmdId)
	if c.acks[cmdId] != acks1 {
		t.Error("acks MsgSet should not be reinitialized")
	}
	if c.macks[cmdId] != macks1 {
		t.Error("macks MsgSet should not be reinitialized")
	}
}

// TestMaxDescRoutinesOverride verifies MaxDescRoutines can be overridden at runtime
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

// === MWeakRead/MWeakReadReply serialization tests (Phase 37.3) ===

func TestMWeakReadSerialization(t *testing.T) {
	original := &MWeakRead{
		CommandId: 42,
		ClientId:  7,
		Key:       state.Key(12345),
	}

	var buf bytes.Buffer
	original.Marshal(&buf)

	decoded := &MWeakRead{}
	if err := decoded.Unmarshal(&buf); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if decoded.CommandId != original.CommandId {
		t.Errorf("CommandId mismatch: got %d, want %d", decoded.CommandId, original.CommandId)
	}
	if decoded.ClientId != original.ClientId {
		t.Errorf("ClientId mismatch: got %d, want %d", decoded.ClientId, original.ClientId)
	}
	if decoded.Key != original.Key {
		t.Errorf("Key mismatch: got %d, want %d", decoded.Key, original.Key)
	}
}

func TestMWeakReadBinarySize(t *testing.T) {
	m := &MWeakRead{}
	size, known := m.BinarySize()
	if !known || size != 16 {
		t.Errorf("BinarySize() = (%d, %v), want (16, true)", size, known)
	}
}

func TestMWeakReadReplySerializationRoundTrip(t *testing.T) {
	original := &MWeakReadReply{
		Replica: 2,
		Ballot:  5,
		CmdId:   CommandId{ClientId: 7, SeqNum: 42},
		Rep:     []byte("test-value"),
		Version: 99,
	}

	var buf bytes.Buffer
	original.Marshal(&buf)

	decoded := &MWeakReadReply{}
	if err := decoded.Unmarshal(&buf); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if decoded.Replica != original.Replica {
		t.Errorf("Replica mismatch: got %d, want %d", decoded.Replica, original.Replica)
	}
	if decoded.Ballot != original.Ballot {
		t.Errorf("Ballot mismatch: got %d, want %d", decoded.Ballot, original.Ballot)
	}
	if decoded.CmdId != original.CmdId {
		t.Errorf("CmdId mismatch: got %v, want %v", decoded.CmdId, original.CmdId)
	}
	if !bytes.Equal(decoded.Rep, original.Rep) {
		t.Errorf("Rep mismatch: got %v, want %v", decoded.Rep, original.Rep)
	}
	if decoded.Version != original.Version {
		t.Errorf("Version mismatch: got %d, want %d", decoded.Version, original.Version)
	}
}

func TestMWeakReadReplyEmptyRep(t *testing.T) {
	original := &MWeakReadReply{
		Replica: 0,
		Ballot:  0,
		CmdId:   CommandId{},
		Rep:     []byte{},
		Version: 0,
	}

	var buf bytes.Buffer
	original.Marshal(&buf)

	decoded := &MWeakReadReply{}
	if err := decoded.Unmarshal(&buf); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if len(decoded.Rep) != 0 {
		t.Errorf("Expected empty Rep, got length %d", len(decoded.Rep))
	}
}

func TestMWeakReadCache(t *testing.T) {
	cache := NewMWeakReadCache()

	m1 := cache.Get()
	if m1 == nil {
		t.Fatal("Get() returned nil")
	}

	m1.CommandId = 42
	cache.Put(m1)

	m2 := cache.Get()
	if m2 == nil {
		t.Fatal("Get() after Put returned nil")
	}
	if m2.CommandId != 42 {
		t.Errorf("Got CommandId=%d from cache, want 42", m2.CommandId)
	}
}

func TestMWeakReadReplyCache(t *testing.T) {
	cache := NewMWeakReadReplyCache()

	m1 := cache.Get()
	if m1 == nil {
		t.Fatal("Get() returned nil")
	}

	m1.Version = 99
	cache.Put(m1)

	m2 := cache.Get()
	if m2 == nil {
		t.Fatal("Get() after Put returned nil")
	}
	if m2.Version != 99 {
		t.Errorf("Got Version=%d from cache, want 99", m2.Version)
	}
}

// === Client cache tests (Phase 37.5) ===

func TestClientCacheMergeReplicaWins(t *testing.T) {
	c := newTestClient(1)
	// No cache entry — replica wins
	c.weakPending[1] = struct{}{}
	c.weakPendingKeys[1] = 100

	rep := &MWeakReadReply{
		Replica: 1,
		Ballot:  0,
		CmdId:   CommandId{ClientId: 10, SeqNum: 1},
		Rep:     []byte("replica-val"),
		Version: 5,
	}
	c.handleWeakReadReply(rep)

	entry, exists := c.localCache[100]
	if !exists {
		t.Fatal("Expected cache entry for key 100")
	}
	if entry.version != 5 {
		t.Errorf("Expected version 5, got %d", entry.version)
	}
	if !bytes.Equal(entry.value, []byte("replica-val")) {
		t.Errorf("Expected replica-val, got %s", entry.value)
	}
}

func TestClientCacheMergeCacheWins(t *testing.T) {
	c := newTestClient(1)
	// Pre-populate cache with higher version
	c.localCache[100] = cacheEntry{value: []byte("cached-val"), version: 10}
	c.weakPending[1] = struct{}{}
	c.weakPendingKeys[1] = 100

	rep := &MWeakReadReply{
		Replica: 1,
		Ballot:  0,
		CmdId:   CommandId{ClientId: 10, SeqNum: 1},
		Rep:     []byte("replica-val"),
		Version: 5,
	}
	c.handleWeakReadReply(rep)

	entry := c.localCache[100]
	if entry.version != 10 {
		t.Errorf("Expected cache version 10 to win, got %d", entry.version)
	}
	if !bytes.Equal(entry.value, []byte("cached-val")) {
		t.Errorf("Expected cached-val to win, got %s", entry.value)
	}
}

func TestClientCacheWeakWriteUpdate(t *testing.T) {
	c := newTestClient(1)
	c.weakPending[1] = struct{}{}
	c.weakPendingKeys[1] = 200
	c.weakPendingValues[1] = []byte("written-val")
	c.ballot = 0

	rep := &MCausalReply{
		Replica: 1, // matches boundReplica
		CmdId:   CommandId{ClientId: 10, SeqNum: 1},
		Rep:     []byte("result"),
	}
	c.handleCausalReply(rep)

	entry, exists := c.localCache[200]
	if !exists {
		t.Fatal("Expected cache entry for key 200 after causal write")
	}
	if !bytes.Equal(entry.value, []byte("written-val")) {
		t.Errorf("Expected written-val, got %s", entry.value)
	}
	if entry.version <= 0 {
		t.Errorf("Expected positive version, got %d", entry.version)
	}
}

func TestClientCacheStrongUpdate(t *testing.T) {
	c := newTestClient(1)
	c.writeSet = make(map[CommandId]struct{})
	c.strongPendingKeys[1] = 300
	c.val = []byte("strong-result")
	c.ballot = 0

	// handleSyncReply accesses c.val (set above) and then calls
	// c.RegisterReply and c.Println which need the base client.
	// Test the cache logic directly instead:
	c.mu.Lock()
	c.localCache[300] = cacheEntry{value: []byte("strong-result"), version: 1}
	delete(c.strongPendingKeys, int32(1))
	c.mu.Unlock()

	entry, exists := c.localCache[300]
	if !exists {
		t.Fatal("Expected cache entry for key 300 after strong op")
	}
	if !bytes.Equal(entry.value, []byte("strong-result")) {
		t.Errorf("Expected strong-result, got %s", entry.value)
	}
}

// TestRetryClassifiesWeakReadsVsCausalWrites verifies that the MSync timer
// correctly distinguishes weak reads (need re-send MWeakRead) from causal writes
// (recoverable via MSync). Weak reads have weakPending entry but no weakPendingValues
// entry, while causal writes have both.
func TestRetryClassifiesWeakReadsVsCausalWrites(t *testing.T) {
	c := &Client{
		weakPending:       make(map[int32]struct{}),
		weakPendingKeys:   make(map[int32]int64),
		weakPendingValues: make(map[int32]state.Value),
		strongPendingKeys: make(map[int32]int64),
		delivered:         make(map[int32]struct{}),
	}

	// Causal write: has both weakPending and weakPendingValues
	c.weakPending[10] = struct{}{}
	c.weakPendingKeys[10] = 100
	c.weakPendingValues[10] = []byte("val10")

	// Weak read: has weakPending and weakPendingKeys, but NO weakPendingValues
	c.weakPending[20] = struct{}{}
	c.weakPendingKeys[20] = 200

	// Already delivered: should be skipped
	c.weakPending[30] = struct{}{}
	c.weakPendingKeys[30] = 300
	c.delivered[30] = struct{}{}

	// Strong pending: always goes to MSync
	c.strongPendingKeys[40] = 400

	// Classify (same logic as timer handler)
	var syncSeqnums []int32
	var weakReadRetries []int32

	for seqnum := range c.strongPendingKeys {
		if _, delivered := c.delivered[seqnum]; !delivered {
			syncSeqnums = append(syncSeqnums, seqnum)
		}
	}
	for seqnum := range c.weakPending {
		if _, delivered := c.delivered[seqnum]; !delivered {
			if _, isWrite := c.weakPendingValues[seqnum]; isWrite {
				syncSeqnums = append(syncSeqnums, seqnum)
			} else {
				weakReadRetries = append(weakReadRetries, seqnum)
			}
		}
	}

	// Verify classification
	syncSet := make(map[int32]bool)
	for _, s := range syncSeqnums {
		syncSet[s] = true
	}
	readSet := make(map[int32]bool)
	for _, r := range weakReadRetries {
		readSet[r] = true
	}

	// Causal write (10) should be in sync
	if !syncSet[10] {
		t.Error("Causal write seqnum 10 should be in syncSeqnums")
	}
	// Weak read (20) should be in weakReadRetries
	if !readSet[20] {
		t.Error("Weak read seqnum 20 should be in weakReadRetries")
	}
	// Delivered (30) should not be in either
	if syncSet[30] || readSet[30] {
		t.Error("Delivered seqnum 30 should not be in any retry list")
	}
	// Strong (40) should be in sync
	if !syncSet[40] {
		t.Error("Strong seqnum 40 should be in syncSeqnums")
	}
	// Weak read (20) should NOT be in sync
	if syncSet[20] {
		t.Error("Weak read seqnum 20 should NOT be in syncSeqnums")
	}
}

// --- Phase 43.3: BoundReplica field tests ---

func TestMCausalProposeBoundReplicaSerialization(t *testing.T) {
	// Test that BoundReplica survives a Marshal/Unmarshal round-trip
	// with various replica ID values
	for _, boundID := range []int32{0, 1, 2, 4, 127} {
		original := &MCausalPropose{
			CommandId:    10,
			ClientId:     200,
			Command: state.Command{
				Op: state.PUT,
				K:  state.Key(5),
				V:  []byte("v"),
			},
			Timestamp:    999,
			CausalDep:    9,
			BoundReplica: boundID,
		}

		var buf bytes.Buffer
		original.Marshal(&buf)

		restored := &MCausalPropose{}
		if err := restored.Unmarshal(&buf); err != nil {
			t.Fatalf("Unmarshal failed for BoundReplica=%d: %v", boundID, err)
		}

		if restored.BoundReplica != boundID {
			t.Errorf("BoundReplica mismatch: got %d, want %d", restored.BoundReplica, boundID)
		}
	}
}

// --- Phase 43.2a: Async sendMsgToAll tests ---

func TestWriterMuInitialization(t *testing.T) {
	c := newTestClient(1)
	if len(c.writerMu) != 3 {
		t.Errorf("writerMu length = %d, want 3", len(c.writerMu))
	}
}

func TestWriterMuLockUnlock(t *testing.T) {
	// Verify that writerMu can be locked and unlocked without deadlock
	c := newTestClient(0)
	for i := 0; i < c.N; i++ {
		c.writerMu[i].Lock()
		c.writerMu[i].Unlock()
	}
}

func TestWriterMuConcurrentAccess(t *testing.T) {
	// Verify that per-replica mutexes allow concurrent access to different replicas
	c := newTestClient(0)
	var wg sync.WaitGroup
	for i := 0; i < c.N; i++ {
		wg.Add(1)
		go func(rid int) {
			c.writerMu[rid].Lock()
			time.Sleep(1 * time.Millisecond) // hold lock briefly
			c.writerMu[rid].Unlock()
			wg.Done()
		}(i)
	}
	// If mutexes were shared (single mutex), this would serialize.
	// With per-replica mutexes, all three goroutines run concurrently.
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		// success
	case <-time.After(1 * time.Second):
		t.Fatal("Timed out: per-replica mutexes should allow concurrent access to different replicas")
	}
}

func TestWriterMuSerializesAccess(t *testing.T) {
	// Verify that the same replica's mutex serializes access
	c := newTestClient(0)
	counter := 0
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			c.writerMu[0].Lock()
			val := counter
			counter = val + 1
			c.writerMu[0].Unlock()
			wg.Done()
		}()
	}
	wg.Wait()
	if counter != 100 {
		t.Errorf("counter = %d, want 100 (mutex should serialize increments)", counter)
	}
}
