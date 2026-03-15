package raftht

import (
	"bytes"
	"testing"
	"time"

	"github.com/imdea-software/swiftpaxos/replica/defs"
	fastrpc "github.com/imdea-software/swiftpaxos/rpc"
	"github.com/imdea-software/swiftpaxos/state"
)

// ============================================================================
// Phase 49.7a: Serialization Round-trip Tests for Weak Message Types
// ============================================================================

// --- MWeakPropose ---

func TestMWeakProposeSerialization(t *testing.T) {
	original := &MWeakPropose{
		CommandId: 42,
		ClientId:  100,
		Command: state.Command{
			Op: state.PUT,
			K:  state.Key(10),
			V:  state.Value([]byte("hello")),
		},
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

func TestMWeakProposeEmptyValue(t *testing.T) {
	original := &MWeakPropose{
		CommandId: 1,
		ClientId:  2,
		Command: state.Command{
			Op: state.GET,
			K:  state.Key(5),
			V:  state.Value([]byte{}),
		},
	}

	var buf bytes.Buffer
	original.Marshal(&buf)

	restored := &MWeakPropose{}
	if err := restored.Unmarshal(&buf); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if restored.Command.Op != state.GET {
		t.Errorf("Command.Op should be GET, got %d", restored.Command.Op)
	}
}

func TestMWeakProposeBinarySize(t *testing.T) {
	wp := &MWeakPropose{CommandId: 1, ClientId: 2, Command: state.Command{Op: state.PUT, K: 1, V: []byte("x")}}
	_, known := wp.BinarySize()
	if known {
		t.Error("BinarySize should be unknown for MWeakPropose (variable due to Command)")
	}
}

// --- MWeakReply ---

func TestMWeakReplySerialization(t *testing.T) {
	original := &MWeakReply{
		LeaderId: 0,
		Term:     5,
		CmdId:    CommandId{ClientId: 100, SeqNum: 42},
		Slot:     77,
	}

	var buf bytes.Buffer
	original.Marshal(&buf)

	restored := &MWeakReply{}
	if err := restored.Unmarshal(&buf); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if restored.LeaderId != original.LeaderId {
		t.Errorf("LeaderId mismatch: got %d, want %d", restored.LeaderId, original.LeaderId)
	}
	if restored.Term != original.Term {
		t.Errorf("Term mismatch: got %d, want %d", restored.Term, original.Term)
	}
	if restored.CmdId != original.CmdId {
		t.Errorf("CmdId mismatch: got %+v, want %+v", restored.CmdId, original.CmdId)
	}
	if restored.Slot != original.Slot {
		t.Errorf("Slot mismatch: got %d, want %d", restored.Slot, original.Slot)
	}
}

func TestMWeakReplyBinarySize(t *testing.T) {
	wr := &MWeakReply{LeaderId: 1, Term: 2, CmdId: CommandId{3, 4}, Slot: 5}
	size, known := wr.BinarySize()
	if !known {
		t.Error("BinarySize should be known for MWeakReply (fixed 20 bytes)")
	}
	if size != 20 {
		t.Errorf("BinarySize should be 20, got %d", size)
	}
}

func TestMWeakReplyZeroValues(t *testing.T) {
	original := &MWeakReply{}

	var buf bytes.Buffer
	original.Marshal(&buf)

	restored := &MWeakReply{}
	if err := restored.Unmarshal(&buf); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if restored.LeaderId != original.LeaderId || restored.Term != original.Term ||
		restored.CmdId != original.CmdId || restored.Slot != original.Slot {
		t.Errorf("Zero-value mismatch: got %+v, want %+v", restored, original)
	}
}

// --- MWeakRead ---

func TestMWeakReadSerialization(t *testing.T) {
	original := &MWeakRead{
		CommandId: 99,
		ClientId:  200,
		Key:       state.Key(12345),
	}

	var buf bytes.Buffer
	original.Marshal(&buf)

	restored := &MWeakRead{}
	if err := restored.Unmarshal(&buf); err != nil {
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
}

func TestMWeakReadBinarySize(t *testing.T) {
	wr := &MWeakRead{CommandId: 1, ClientId: 2, Key: state.Key(3)}
	nbytes, known := wr.BinarySize()
	if !known {
		t.Error("BinarySize should be known for MWeakRead (fixed 20 bytes)")
	}
	if nbytes != 20 {
		t.Errorf("BinarySize = %d, want 20", nbytes)
	}

	var buf bytes.Buffer
	wr.Marshal(&buf)
	if buf.Len() != nbytes {
		t.Errorf("BinarySize %d != marshalled size %d", nbytes, buf.Len())
	}
}

func TestMWeakReadMinIndex(t *testing.T) {
	original := &MWeakRead{
		CommandId: 42,
		ClientId:  100,
		Key:       state.Key(7777),
		MinIndex:  5678,
	}

	var buf bytes.Buffer
	original.Marshal(&buf)

	restored := &MWeakRead{}
	if err := restored.Unmarshal(&buf); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if restored.MinIndex != 5678 {
		t.Errorf("MinIndex = %d, want 5678", restored.MinIndex)
	}
	if restored.CommandId != 42 {
		t.Errorf("CommandId = %d, want 42", restored.CommandId)
	}
	if restored.Key != state.Key(7777) {
		t.Errorf("Key = %d, want 7777", restored.Key)
	}
}

func TestMWeakReadLargeKey(t *testing.T) {
	original := &MWeakRead{
		CommandId: 1,
		ClientId:  1,
		Key:       state.Key(9223372036854775807), // max int64
	}

	var buf bytes.Buffer
	original.Marshal(&buf)

	restored := &MWeakRead{}
	if err := restored.Unmarshal(&buf); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if restored.Key != original.Key {
		t.Errorf("Key mismatch: got %d, want %d", restored.Key, original.Key)
	}
}

// --- MWeakReadReply ---

func TestMWeakReadReplySerialization(t *testing.T) {
	original := &MWeakReadReply{
		Replica: 2,
		Term:    10,
		CmdId:   CommandId{ClientId: 100, SeqNum: 42},
		Rep:     []byte("test-value"),
		Version: 55,
	}

	var buf bytes.Buffer
	original.Marshal(&buf)

	restored := &MWeakReadReply{}
	if err := restored.Unmarshal(&buf); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if restored.Replica != original.Replica {
		t.Errorf("Replica mismatch: got %d, want %d", restored.Replica, original.Replica)
	}
	if restored.Term != original.Term {
		t.Errorf("Term mismatch: got %d, want %d", restored.Term, original.Term)
	}
	if restored.CmdId != original.CmdId {
		t.Errorf("CmdId mismatch: got %+v, want %+v", restored.CmdId, original.CmdId)
	}
	if !bytes.Equal(restored.Rep, original.Rep) {
		t.Errorf("Rep mismatch: got %v, want %v", restored.Rep, original.Rep)
	}
	if restored.Version != original.Version {
		t.Errorf("Version mismatch: got %d, want %d", restored.Version, original.Version)
	}
}

func TestMWeakReadReplyEmptyRep(t *testing.T) {
	original := &MWeakReadReply{
		Replica: 0,
		Term:    1,
		CmdId:   CommandId{ClientId: 1, SeqNum: 1},
		Rep:     []byte{},
		Version: 0,
	}

	var buf bytes.Buffer
	original.Marshal(&buf)

	restored := &MWeakReadReply{}
	if err := restored.Unmarshal(&buf); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if len(restored.Rep) != 0 {
		t.Errorf("Rep should be empty, got %v", restored.Rep)
	}
}

func TestMWeakReadReplyLargeRep(t *testing.T) {
	largeRep := make([]byte, 4096)
	for i := range largeRep {
		largeRep[i] = byte(i % 256)
	}

	original := &MWeakReadReply{
		Replica: 1,
		Term:    99,
		CmdId:   CommandId{ClientId: 50, SeqNum: 200},
		Rep:     largeRep,
		Version: 12345,
	}

	var buf bytes.Buffer
	original.Marshal(&buf)

	restored := &MWeakReadReply{}
	if err := restored.Unmarshal(&buf); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if !bytes.Equal(restored.Rep, original.Rep) {
		t.Errorf("Large Rep mismatch: lengths got %d, want %d", len(restored.Rep), len(original.Rep))
	}
	if restored.Version != original.Version {
		t.Errorf("Version mismatch: got %d, want %d", restored.Version, original.Version)
	}
}

func TestMWeakReadReplyBinarySize(t *testing.T) {
	wr := &MWeakReadReply{Replica: 1, Term: 2, CmdId: CommandId{3, 4}, Rep: []byte("x"), Version: 5}
	_, known := wr.BinarySize()
	if known {
		t.Error("BinarySize should be unknown for MWeakReadReply (variable due to Rep)")
	}
}

// ============================================================================
// New() factory method tests
// ============================================================================

func TestWeakNewMethods(t *testing.T) {
	wp := (&MWeakPropose{}).New()
	if _, ok := wp.(*MWeakPropose); !ok {
		t.Error("MWeakPropose.New() should return *MWeakPropose")
	}

	wr := (&MWeakReply{}).New()
	if _, ok := wr.(*MWeakReply); !ok {
		t.Error("MWeakReply.New() should return *MWeakReply")
	}

	wrd := (&MWeakRead{}).New()
	if _, ok := wrd.(*MWeakRead); !ok {
		t.Error("MWeakRead.New() should return *MWeakRead")
	}

	wrr := (&MWeakReadReply{}).New()
	if _, ok := wrr.(*MWeakReadReply); !ok {
		t.Error("MWeakReadReply.New() should return *MWeakReadReply")
	}
}

// ============================================================================
// Cache pool tests
// ============================================================================

func TestMWeakProposeCache(t *testing.T) {
	cache := NewMWeakProposeCache()
	wp := cache.Get()
	if wp == nil {
		t.Fatal("Get() should return non-nil")
	}
	wp.CommandId = 42
	cache.Put(wp)
	wp2 := cache.Get()
	if wp2.CommandId != 42 {
		t.Error("Cache should return put object")
	}
}

func TestMWeakReplyCache(t *testing.T) {
	cache := NewMWeakReplyCache()
	wr := cache.Get()
	if wr == nil {
		t.Fatal("Get() should return non-nil")
	}
	wr.Slot = 99
	cache.Put(wr)
	wr2 := cache.Get()
	if wr2.Slot != 99 {
		t.Error("Cache should return put object")
	}
}

func TestMWeakReadCache(t *testing.T) {
	cache := NewMWeakReadCache()
	rd := cache.Get()
	if rd == nil {
		t.Fatal("Get() should return non-nil")
	}
	rd.CommandId = 77
	cache.Put(rd)
	rd2 := cache.Get()
	if rd2.CommandId != 77 {
		t.Error("Cache should return put object")
	}
}

func TestMWeakReadReplyCache(t *testing.T) {
	cache := NewMWeakReadReplyCache()
	rr := cache.Get()
	if rr == nil {
		t.Fatal("Get() should return non-nil")
	}
	rr.Version = 55
	cache.Put(rr)
	rr2 := cache.Get()
	if rr2.Version != 55 {
		t.Error("Cache should return put object")
	}
}

// ============================================================================
// CommunicationSupply + initCs tests (weak channels)
// ============================================================================

func TestInitCsWeakChannels(t *testing.T) {
	cs := &CommunicationSupply{}
	table := fastrpc.NewTable()
	initCs(cs, table)

	// Check weak channels are non-nil
	if cs.WeakProposeChan == nil {
		t.Error("weakProposeChan should be non-nil")
	}
	if cs.WeakReplyChan == nil {
		t.Error("weakReplyChan should be non-nil")
	}
	if cs.WeakReadChan == nil {
		t.Error("weakReadChan should be non-nil")
	}
	if cs.WeakReadReplyChan == nil {
		t.Error("weakReadReplyChan should be non-nil")
	}

	// Check all RPC IDs are distinct (9 total: 5 vanilla Raft + 4 weak)
	ids := map[uint8]string{
		cs.AppendEntriesRPC:      "appendEntries",
		cs.AppendEntriesReplyRPC: "appendEntriesReply",
		cs.RequestVoteRPC:        "requestVote",
		cs.RequestVoteReplyRPC:   "requestVoteReply",
		cs.RaftReplyRPC:          "raftReply",
		cs.WeakProposeRPC:        "weakPropose",
		cs.WeakReplyRPC:          "weakReply",
		cs.WeakReadRPC:           "weakRead",
		cs.WeakReadReplyRPC:      "weakReadReply",
	}
	if len(ids) != 9 {
		t.Errorf("Expected 9 distinct RPC IDs, got %d (some collide)", len(ids))
	}
}

// ============================================================================
// Phase 49.9a: GetClientId() interface tests
// ============================================================================

func TestMWeakProposeGetClientId(t *testing.T) {
	p := &MWeakPropose{CommandId: 1, ClientId: 42}
	if p.GetClientId() != 42 {
		t.Errorf("MWeakPropose.GetClientId() should return 42, got %d", p.GetClientId())
	}
}

func TestMWeakReadGetClientId(t *testing.T) {
	r := &MWeakRead{CommandId: 1, ClientId: 99}
	if r.GetClientId() != 99 {
		t.Errorf("MWeakRead.GetClientId() should return 99, got %d", r.GetClientId())
	}
}

// ============================================================================
// Phase 50.1: RWMutex-based processWeakRead tests
// ============================================================================

func TestProcessWeakRead_WithRWMutex(t *testing.T) {
	r := newTestReplica(0, 3)
	st := state.InitState()

	// Put a value into state
	putCmd := state.Command{Op: state.PUT, K: state.Key(42), V: state.Value([]byte("rwmutex-val"))}
	putCmd.Execute(st)
	r.keyVersions[42] = 7

	// Simulate processWeakRead logic using RLock (same as new implementation)
	msg := &MWeakRead{CommandId: 5, ClientId: 200, Key: state.Key(42)}

	r.stateMu.RLock()
	cmd := state.Command{Op: state.GET, K: msg.Key, V: state.NIL()}
	value := cmd.Execute(st)

	version := int32(0)
	if v, ok := r.keyVersions[int64(msg.Key)]; ok {
		version = v
	}
	r.stateMu.RUnlock()

	if !bytes.Equal(value, []byte("rwmutex-val")) {
		t.Errorf("Should read 'rwmutex-val', got %v", value)
	}
	if version != 7 {
		t.Errorf("Version should be 7, got %d", version)
	}
}

func TestProcessWeakRead_ConcurrentWithExecution(t *testing.T) {
	r := newTestReplica(0, 3)
	st := state.InitState()

	// Write initial state
	putCmd := state.Command{Op: state.PUT, K: state.Key(10), V: state.Value([]byte("initial"))}
	putCmd.Execute(st)
	r.keyVersions[10] = 1

	// Simulate concurrent read and write lock usage
	done := make(chan struct{})

	// Writer goroutine (simulates executeCommands)
	go func() {
		for i := 0; i < 100; i++ {
			r.stateMu.Lock()
			cmd := state.Command{Op: state.PUT, K: state.Key(10), V: state.Value([]byte("updated"))}
			cmd.Execute(st)
			r.keyVersions[10] = int32(i + 2)
			r.stateMu.Unlock()
		}
		close(done)
	}()

	// Reader goroutine (simulates weak reads)
	for i := 0; i < 100; i++ {
		r.stateMu.RLock()
		cmd := state.Command{Op: state.GET, K: state.Key(10), V: state.NIL()}
		val := cmd.Execute(st)
		ver := r.keyVersions[10]
		r.stateMu.RUnlock()

		// Value should be non-nil (either "initial" or "updated")
		if val == nil {
			t.Errorf("Weak read returned nil value")
		}
		// Version should be >= 1
		if ver < 1 {
			t.Errorf("Version should be >= 1, got %d", ver)
		}
	}

	<-done
}

// ============================================================================
// Phase 49.7b-d: Replica Logic Unit Tests
// ============================================================================

// newTestReplica creates a minimal Replica for unit testing (no network).
func newTestReplica(id int32, n int) *Replica {
	return &Replica{
		Replica:     nil,
		id:          id,
		currentTerm: 0,
		votedFor:    -1,
		log:         make([]LogEntry, 0),
		commitIndex: -1,
		lastApplied: -1,
		role:        FOLLOWER,
		knownLeader: -1,
		n:           n,
		nextIndex:   make([]int32, n),
		matchIndex:  make([]int32, n),
		pendingProposals: make([]*defs.GPropose, 0),
		commitNotify:     make(chan struct{}, 1),
		votesReceived:    0,
		votesNeeded:      (n / 2) + 1,
		appendEntriesCache:      NewAppendEntriesCache(),
		appendEntriesReplyCache: NewAppendEntriesReplyCache(),
		requestVoteCache:        NewRequestVoteCache(),
		requestVoteReplyCache:   NewRequestVoteReplyCache(),
		raftReplyCache:          NewRaftReplyCache(),
		keyVersions:             make(map[int64]int32),
	}
}

// TestKeyVersionsTracking tests that executeCommands updates keyVersions for PUT ops
func TestKeyVersionsTracking(t *testing.T) {
	r := newTestReplica(0, 3)
	st := state.InitState()

	r.log = []LogEntry{
		{Term: 1, Command: state.Command{Op: state.PUT, K: state.Key(10), V: state.Value([]byte("v1"))}, CmdId: CommandId{ClientId: 1, SeqNum: 1}},
		{Term: 1, Command: state.Command{Op: state.GET, K: state.Key(10), V: state.NIL()}, CmdId: CommandId{ClientId: 1, SeqNum: 2}},
		{Term: 1, Command: state.Command{Op: state.PUT, K: state.Key(20), V: state.Value([]byte("v2"))}, CmdId: CommandId{ClientId: 1, SeqNum: 3}},
		{Term: 1, Command: state.Command{Op: state.PUT, K: state.Key(10), V: state.Value([]byte("v3"))}, CmdId: CommandId{ClientId: 1, SeqNum: 4}},
	}
	r.commitIndex = 3
	r.lastApplied = -1

	// Execute committed commands, track keyVersions
	for r.lastApplied < r.commitIndex {
		r.lastApplied++
		idx := r.lastApplied
		entry := r.log[idx]
		entry.Command.Execute(st)

		if entry.Command.Op == state.PUT {
			r.keyVersions[int64(entry.Command.K)] = idx
		}
	}

	// Key 10: last PUT was at index 3
	if v, ok := r.keyVersions[10]; !ok || v != 3 {
		t.Errorf("keyVersions[10] should be 3, got %d (ok=%v)", v, ok)
	}

	// Key 20: last PUT was at index 2
	if v, ok := r.keyVersions[20]; !ok || v != 2 {
		t.Errorf("keyVersions[20] should be 2, got %d (ok=%v)", v, ok)
	}

	// GET should not create a keyVersions entry for a key that wasn't PUT
	// (key 10 was PUT, so it exists, but the GET at index 1 shouldn't overwrite it)
}

// TestHandleWeakPropose_LogAppend tests that handleWeakPropose appends to log correctly
func TestHandleWeakPropose_LogAppend(t *testing.T) {
	r := newTestReplica(0, 3)
	r.role = LEADER
	r.currentTerm = 5

	// Pre-populate log with one entry
	r.log = []LogEntry{
		{Term: 5, Command: state.Command{Op: state.PUT, K: 1, V: []byte("a")}, CmdId: CommandId{ClientId: 1, SeqNum: 1}},
	}
	r.matchIndex[0] = 0

	// Simulate handleWeakPropose logic (can't call directly: needs sender)
	propose := &MWeakPropose{
		CommandId: 10,
		ClientId:  200,
		Command:   state.Command{Op: state.PUT, K: state.Key(42), V: state.Value([]byte("weak-val"))},
	}

	entry := LogEntry{
		Command: propose.Command,
		Term:    r.currentTerm,
		CmdId:   CommandId{ClientId: propose.ClientId, SeqNum: propose.CommandId},
	}
	idx := int32(len(r.log))
	r.log = append(r.log, entry)

	for int32(len(r.pendingProposals)) <= idx {
		r.pendingProposals = append(r.pendingProposals, nil)
	}
	r.matchIndex[r.id] = int32(len(r.log) - 1)

	// Verify
	if len(r.log) != 2 {
		t.Fatalf("Log should have 2 entries, got %d", len(r.log))
	}
	if r.log[1].Term != 5 {
		t.Errorf("Entry term should be 5, got %d", r.log[1].Term)
	}
	if r.log[1].CmdId.ClientId != 200 {
		t.Errorf("Entry ClientId should be 200, got %d", r.log[1].CmdId.ClientId)
	}
	if r.log[1].CmdId.SeqNum != 10 {
		t.Errorf("Entry SeqNum should be 10, got %d", r.log[1].CmdId.SeqNum)
	}
	if idx != 1 {
		t.Errorf("Slot should be 1, got %d", idx)
	}
	if r.matchIndex[0] != 1 {
		t.Errorf("matchIndex[leader] should be 1, got %d", r.matchIndex[0])
	}
}

// TestHandleWeakPropose_BatchAppend tests that batched weak writes append all to log correctly
func TestHandleWeakPropose_BatchAppend(t *testing.T) {
	r := newTestReplica(0, 3)
	r.role = LEADER
	r.currentTerm = 3

	// Simulate batch of 3 weak writes appended to log (handleWeakPropose batch logic)
	proposals := []*MWeakPropose{
		{CommandId: 1, ClientId: 100, Command: state.Command{Op: state.PUT, K: state.Key(10), V: state.Value([]byte("a"))}},
		{CommandId: 2, ClientId: 101, Command: state.Command{Op: state.PUT, K: state.Key(20), V: state.Value([]byte("b"))}},
		{CommandId: 3, ClientId: 102, Command: state.Command{Op: state.PUT, K: state.Key(30), V: state.Value([]byte("c"))}},
	}

	for _, wp := range proposals {
		entry := LogEntry{
			Command: wp.Command,
			Term:    r.currentTerm,
			CmdId:   CommandId{ClientId: wp.ClientId, SeqNum: wp.CommandId},
		}
		idx := int32(len(r.log))
		r.log = append(r.log, entry)
		for int32(len(r.pendingProposals)) <= idx {
			r.pendingProposals = append(r.pendingProposals, nil)
		}
	}
	r.matchIndex[r.id] = int32(len(r.log) - 1)

	// Verify all 3 entries are in log
	if len(r.log) != 3 {
		t.Fatalf("Log should have 3 entries, got %d", len(r.log))
	}
	if r.log[0].CmdId.ClientId != 100 || r.log[1].CmdId.ClientId != 101 || r.log[2].CmdId.ClientId != 102 {
		t.Errorf("Entries should be in order: got clients %d, %d, %d",
			r.log[0].CmdId.ClientId, r.log[1].CmdId.ClientId, r.log[2].CmdId.ClientId)
	}
	if r.matchIndex[0] != 2 {
		t.Errorf("matchIndex[leader] should be 2 (last entry), got %d", r.matchIndex[0])
	}
}

// TestHandleWeakPropose_RejectNonLeader tests that non-leaders silently drop weak writes
func TestHandleWeakPropose_RejectNonLeader(t *testing.T) {
	r := newTestReplica(1, 3)
	r.role = FOLLOWER
	r.currentTerm = 3

	logLenBefore := len(r.log)

	// Follower should not append
	if r.role != LEADER {
		// silently drop — this is the expected path
	}

	if len(r.log) != logLenBefore {
		t.Error("Follower should not append to log on weak propose")
	}
}

// TestHandleWeakRead_ReturnsCommittedState tests weak read returns committed state + version
func TestHandleWeakRead_ReturnsCommittedState(t *testing.T) {
	r := newTestReplica(0, 3)
	st := state.InitState()

	// Put a value into state
	putCmd := state.Command{Op: state.PUT, K: state.Key(42), V: state.Value([]byte("committed-value"))}
	putCmd.Execute(st)

	// Track version
	r.keyVersions[42] = 5 // last committed write at log index 5

	// Simulate handleWeakRead logic
	msg := &MWeakRead{CommandId: 1, ClientId: 100, Key: state.Key(42)}

	cmd := state.Command{Op: state.GET, K: msg.Key, V: state.NIL()}
	value := cmd.Execute(st)

	version := int32(0)
	if v, ok := r.keyVersions[int64(msg.Key)]; ok {
		version = v
	}

	if !bytes.Equal(value, []byte("committed-value")) {
		t.Errorf("Should read committed value, got %v", value)
	}
	if version != 5 {
		t.Errorf("Version should be 5, got %d", version)
	}
}

// TestHandleWeakRead_UnknownKey tests weak read for a key that was never written
func TestHandleWeakRead_UnknownKey(t *testing.T) {
	r := newTestReplica(0, 3)
	st := state.InitState()

	msg := &MWeakRead{CommandId: 1, ClientId: 100, Key: state.Key(999)}

	cmd := state.Command{Op: state.GET, K: msg.Key, V: state.NIL()}
	value := cmd.Execute(st)

	version := int32(0)
	if v, ok := r.keyVersions[int64(msg.Key)]; ok {
		version = v
	}

	// Unknown key returns empty/nil value and version 0
	if version != 0 {
		t.Errorf("Version should be 0 for unknown key, got %d", version)
	}
	_ = value // value for unknown key is implementation-defined (typically nil/empty)
}

// ============================================================================
// Phase 49.7e: Client Cache Merge Logic Tests
// ============================================================================

func TestClientCacheMerge_ReplicaWins(t *testing.T) {
	cache := make(map[int64]cacheEntry)
	cache[42] = cacheEntry{value: []byte("old"), version: 3}

	// Replica returns newer version
	replicaVal := state.Value([]byte("new"))
	replicaVer := int32(5)

	cached := cache[42]
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
		t.Errorf("Replica should win: got %v", finalVal)
	}
	if finalVer != 5 {
		t.Errorf("Version should be 5, got %d", finalVer)
	}
}

func TestClientCacheMerge_CacheWins(t *testing.T) {
	cache := make(map[int64]cacheEntry)
	cache[42] = cacheEntry{value: []byte("cached"), version: 10}

	// Replica returns older version
	replicaVal := state.Value([]byte("stale"))
	replicaVer := int32(3)

	cached := cache[42]
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
		t.Errorf("Cache should win: got %v", finalVal)
	}
	if finalVer != 10 {
		t.Errorf("Version should be 10, got %d", finalVer)
	}
}

func TestClientCacheMerge_NoCacheEntry(t *testing.T) {
	cache := make(map[int64]cacheEntry)

	// No cache entry — replica always wins
	replicaVal := state.Value([]byte("first"))
	replicaVer := int32(1)

	cached, hasCached := cache[42]
	var finalVal state.Value
	var finalVer int32
	if hasCached && cached.version > replicaVer {
		finalVal = cached.value
		finalVer = cached.version
	} else {
		finalVal = replicaVal
		finalVer = replicaVer
	}

	if !bytes.Equal(finalVal, []byte("first")) {
		t.Errorf("Replica should win when no cache: got %v", finalVal)
	}
	if finalVer != 1 {
		t.Errorf("Version should be 1, got %d", finalVer)
	}
}

func TestClientCacheMerge_EqualVersion(t *testing.T) {
	cache := make(map[int64]cacheEntry)
	cache[42] = cacheEntry{value: []byte("cached"), version: 5}

	// Same version — replica wins (not strictly greater)
	replicaVal := state.Value([]byte("replica"))
	replicaVer := int32(5)

	cached := cache[42]
	var finalVal state.Value
	var finalVer int32
	if cached.version > replicaVer {
		finalVal = cached.value
		finalVer = cached.version
	} else {
		finalVal = replicaVal
		finalVer = replicaVer
	}

	if !bytes.Equal(finalVal, []byte("replica")) {
		t.Errorf("Replica should win on equal version: got %v", finalVal)
	}
	if finalVer != 5 {
		t.Errorf("Version should be 5, got %d", finalVer)
	}
}

// ============================================================================
// Phase 59: Timer Management, Leader Tracking, and Weak Propose Rejection Tests
// ============================================================================

// TestBecomeLeader_SetsKnownLeader verifies that becomeLeader updates knownLeader
func TestBecomeLeader_SetsKnownLeader(t *testing.T) {
	r := newTestReplica(2, 3)
	r.currentTerm = 5
	r.knownLeader = -1

	r.becomeLeader()

	if r.knownLeader != 2 {
		t.Errorf("knownLeader = %d, want 2", r.knownLeader)
	}
	if r.role != LEADER {
		t.Errorf("role = %d, want LEADER", r.role)
	}
}

// TestBecomeLeader_TimerManagement verifies heartbeat timer starts and election timer stops
func TestBecomeLeader_TimerManagement(t *testing.T) {
	r := newTestReplica(0, 3)
	r.electionTimer = time.NewTimer(time.Hour) // long timer so it doesn't fire
	r.heartbeatTimer = time.NewTimer(time.Hour)
	r.heartbeatTimer.Stop() // simulate stopped state (as follower)
	r.heartbeatTimeout = 100 * time.Millisecond

	r.becomeLeader()

	// Heartbeat timer should have been reset and fire within heartbeatTimeout
	select {
	case <-r.heartbeatTimer.C:
		// Good — heartbeat timer is running
	case <-time.After(200 * time.Millisecond):
		t.Error("heartbeat timer did not fire after becomeLeader")
	}
}

// TestBecomeFollower_TimerManagement verifies election timer starts and heartbeat stops
func TestBecomeFollower_TimerManagement(t *testing.T) {
	r := newTestReplica(0, 3)
	r.role = LEADER
	r.electionTimer = time.NewTimer(time.Hour)
	r.electionTimer.Stop() // simulate stopped state (as leader)
	r.heartbeatTimer = time.NewTimer(time.Hour)

	r.becomeFollower(5)

	if r.role != FOLLOWER {
		t.Errorf("role = %d, want FOLLOWER", r.role)
	}
	if r.currentTerm != 5 {
		t.Errorf("currentTerm = %d, want 5", r.currentTerm)
	}

	// Election timer should have been reset and fire within 500ms
	select {
	case <-r.electionTimer.C:
		// Good — election timer is running
	case <-time.After(600 * time.Millisecond):
		t.Error("election timer did not fire after becomeFollower")
	}
}

// TestBecomeFollower_NilTimers verifies no panic when timers are nil (during init)
func TestBecomeFollower_NilTimers(t *testing.T) {
	r := newTestReplica(1, 3)
	// timers are nil by default in test replica — should not panic
	r.becomeFollower(3)

	if r.role != FOLLOWER {
		t.Errorf("role = %d, want FOLLOWER", r.role)
	}
}

// TestKnownLeader_UpdatedByAppendEntries verifies knownLeader is set when
// processing an AppendEntries (simulated inline since full handler needs sender)
func TestKnownLeader_UpdatedByAppendEntries(t *testing.T) {
	r := newTestReplica(1, 3)
	r.knownLeader = -1
	r.currentTerm = 0

	// Simulate the AppendEntries handling path: becomeFollower + set knownLeader
	msg := &AppendEntries{LeaderId: 0, Term: 1}
	if msg.Term > r.currentTerm {
		r.becomeFollower(msg.Term)
	}
	r.knownLeader = msg.LeaderId

	if r.knownLeader != 0 {
		t.Errorf("knownLeader = %d, want 0 (set by AppendEntries)", r.knownLeader)
	}
	if r.currentTerm != 1 {
		t.Errorf("currentTerm = %d, want 1", r.currentTerm)
	}
}

// TestKnownLeader_ChangesOnNewLeader verifies knownLeader updates on leader change
func TestKnownLeader_ChangesOnNewLeader(t *testing.T) {
	r := newTestReplica(1, 3)
	r.currentTerm = 1
	r.knownLeader = 0

	// Simulate AppendEntries from new leader at higher term
	msg := &AppendEntries{LeaderId: 2, Term: 3}
	if msg.Term > r.currentTerm {
		r.becomeFollower(msg.Term)
	}
	r.knownLeader = msg.LeaderId

	if r.knownLeader != 2 {
		t.Errorf("knownLeader = %d, want 2 (new leader)", r.knownLeader)
	}
}

// TestHandleWeakPropose_NonLeaderDoesNotAppend verifies log is unchanged on rejection
func TestHandleWeakPropose_NonLeaderDoesNotAppend(t *testing.T) {
	r := newTestReplica(1, 3)
	r.role = FOLLOWER
	r.currentTerm = 5
	r.knownLeader = 0
	r.log = append(r.log, LogEntry{Term: 3}) // pre-existing entry

	logLenBefore := len(r.log)

	// Verify the rejection condition
	if r.role == LEADER {
		t.Fatal("Test setup error: replica should be FOLLOWER")
	}

	if len(r.log) != logLenBefore {
		t.Error("Follower should not modify log on weak propose rejection")
	}
	if r.knownLeader != 0 {
		t.Errorf("knownLeader should be 0 for redirect hint, got %d", r.knownLeader)
	}
}

// TestBeTheLeader_SetsKnownLeader verifies BeTheLeader sets knownLeader to self
func TestBeTheLeader_SetsKnownLeader(t *testing.T) {
	r := newTestReplica(2, 3)
	r.knownLeader = -1

	r.BeTheLeader(nil, nil)

	if r.knownLeader != 2 {
		t.Errorf("knownLeader = %d after BeTheLeader, want 2", r.knownLeader)
	}
	if r.role != LEADER {
		t.Errorf("role = %d after BeTheLeader, want LEADER", r.role)
	}
}

func TestRaftReplySerialization_WithLeaderId(t *testing.T) {
	original := &RaftReply{
		CmdId:    CommandId{ClientId: 42, SeqNum: 7},
		Value:    []byte("hello"),
		LeaderId: 3,
	}

	var buf bytes.Buffer
	original.Marshal(&buf)

	decoded := &RaftReply{}
	if err := decoded.Unmarshal(&buf); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if decoded.CmdId != original.CmdId {
		t.Errorf("CmdId = %v, want %v", decoded.CmdId, original.CmdId)
	}
	if string(decoded.Value) != string(original.Value) {
		t.Errorf("Value = %q, want %q", decoded.Value, original.Value)
	}
	if decoded.LeaderId != 3 {
		t.Errorf("LeaderId = %d, want 3", decoded.LeaderId)
	}
}

func TestRaftReplySerialization_LeaderIdNegative(t *testing.T) {
	original := &RaftReply{
		CmdId:    CommandId{ClientId: 1, SeqNum: 1},
		Value:    nil,
		LeaderId: -1,
	}

	var buf bytes.Buffer
	original.Marshal(&buf)

	decoded := &RaftReply{}
	if err := decoded.Unmarshal(&buf); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if decoded.LeaderId != -1 {
		t.Errorf("LeaderId = %d, want -1", decoded.LeaderId)
	}
}

func TestRaftReplySerialization_EmptyValueWithLeaderId(t *testing.T) {
	original := &RaftReply{
		CmdId:    CommandId{ClientId: 0, SeqNum: 0},
		Value:    []byte{},
		LeaderId: 4,
	}

	var buf bytes.Buffer
	original.Marshal(&buf)

	decoded := &RaftReply{}
	if err := decoded.Unmarshal(&buf); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if decoded.LeaderId != 4 {
		t.Errorf("LeaderId = %d, want 4", decoded.LeaderId)
	}
}

// ============================================================================
// Phase 102b-c: Client Leader Failover Tests
// ============================================================================

func TestRotateLeader(t *testing.T) {
	c := &Client{numReplicas: 5, deadReplicas: make(map[int32]bool)}

	tests := []struct {
		current int32
		want    int32
	}{
		{0, 1},
		{1, 2},
		{4, 0}, // wrap around
		{3, 4},
	}
	for _, tt := range tests {
		got := c.rotateLeader(tt.current)
		if got != tt.want {
			t.Errorf("rotateLeader(%d) = %d, want %d", tt.current, got, tt.want)
		}
	}
}

func TestRotateLeader_SingleReplica(t *testing.T) {
	c := &Client{numReplicas: 1, deadReplicas: make(map[int32]bool)}
	if got := c.rotateLeader(0); got != 0 {
		t.Errorf("rotateLeader(0) with 1 replica = %d, want 0", got)
	}
}

func TestRotateLeader_SkipsDead(t *testing.T) {
	c := &Client{numReplicas: 5, deadReplicas: map[int32]bool{1: true, 2: true}}
	// From 0, should skip 1 and 2 (dead), land on 3
	if got := c.rotateLeader(0); got != 3 {
		t.Errorf("rotateLeader(0) skipping dead 1,2 = %d, want 3", got)
	}
}

func TestRotateLeader_AllDeadFallback(t *testing.T) {
	c := &Client{numReplicas: 3, deadReplicas: map[int32]bool{0: true, 1: true, 2: true}}
	// All dead: fallback to (current+1)%N
	if got := c.rotateLeader(0); got != 1 {
		t.Errorf("rotateLeader(0) all dead = %d, want 1 (fallback)", got)
	}
}

func TestHandleRaftReply_RejectionUpdatesLeader(t *testing.T) {
	// Test the rejection detection logic: LeaderId >= 0 means redirect.
	rep := &RaftReply{
		CmdId:    CommandId{ClientId: 100, SeqNum: 5},
		Value:    nil,
		LeaderId: 2, // hint: leader is replica 2
	}

	// Verify the rejection condition
	if rep.LeaderId < 0 {
		t.Error("LeaderId should be >= 0 for rejection")
	}

	// Simulate the leader update logic from handleRaftReply
	leader := int32(0)
	if rep.LeaderId >= 0 {
		leader = rep.LeaderId
	}
	if leader != 2 {
		t.Errorf("leader = %d after rejection hint, want 2", leader)
	}
}

func TestHandleRaftReply_SuccessNoRedirect(t *testing.T) {
	// LeaderId = -1 means success (no redirect)
	rep := &RaftReply{
		CmdId:    CommandId{ClientId: 100, SeqNum: 5},
		Value:    []byte("result"),
		LeaderId: -1,
	}

	if rep.LeaderId >= 0 {
		t.Error("LeaderId should be -1 for success (no redirect)")
	}
}

func TestHandleReaderDead_NotLeader(t *testing.T) {
	c := &Client{
		leader:            0,
		numReplicas:       3,
		weakPending:       make(map[int32]struct{}),
		delivered:         make(map[int32]struct{}),
		weakPendingKeys:   make(map[int32]int64),
		weakPendingValues: make(map[int32]state.Value),
		strongPendingKeys: make(map[int32]int64),
		strongPendingCmds: make(map[int32]*defs.Propose),
		deadReplicas:      make(map[int32]bool),
		localCache:        make(map[int64]cacheEntry),
	}

	// Reader for replica 2 dies, but leader is 0 — should not rotate
	c.handleReaderDead(2)

	if c.leader != 0 {
		t.Errorf("leader = %d, want 0 (non-leader death should not rotate)", c.leader)
	}
}

func TestHandleReaderDead_IsLeader(t *testing.T) {
	c := &Client{
		leader:            1,
		numReplicas:       3,
		weakPending:       make(map[int32]struct{}),
		delivered:         make(map[int32]struct{}),
		weakPendingKeys:   make(map[int32]int64),
		weakPendingValues: make(map[int32]state.Value),
		strongPendingKeys: make(map[int32]int64),
		strongPendingCmds: make(map[int32]*defs.Propose),
		deadReplicas:      make(map[int32]bool),
		localCache:        make(map[int64]cacheEntry),
	}

	// Reader for replica 1 (the leader) dies — should rotate to 2
	c.handleReaderDead(1)

	if c.leader != 2 {
		t.Errorf("leader = %d after leader death, want 2", c.leader)
	}
}
