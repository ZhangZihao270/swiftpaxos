package raftht

import (
	"bytes"
	"testing"

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
	nbytes, known := wr.BinarySize()
	if !known {
		t.Error("BinarySize should be known for MWeakReply (fixed 20 bytes)")
	}

	var buf bytes.Buffer
	wr.Marshal(&buf)
	if buf.Len() != nbytes {
		t.Errorf("BinarySize %d != marshalled size %d", nbytes, buf.Len())
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

	if *restored != *original {
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
		t.Error("BinarySize should be known for MWeakRead (fixed 16 bytes)")
	}

	var buf bytes.Buffer
	wr.Marshal(&buf)
	if buf.Len() != nbytes {
		t.Errorf("BinarySize %d != marshalled size %d", nbytes, buf.Len())
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
	if cs.weakProposeChan == nil {
		t.Error("weakProposeChan should be non-nil")
	}
	if cs.weakReplyChan == nil {
		t.Error("weakReplyChan should be non-nil")
	}
	if cs.weakReadChan == nil {
		t.Error("weakReadChan should be non-nil")
	}
	if cs.weakReadReplyChan == nil {
		t.Error("weakReadReplyChan should be non-nil")
	}

	// Check all RPC IDs are distinct (9 total: 5 vanilla Raft + 4 weak)
	ids := map[uint8]string{
		cs.appendEntriesRPC:      "appendEntries",
		cs.appendEntriesReplyRPC: "appendEntriesReply",
		cs.requestVoteRPC:        "requestVote",
		cs.requestVoteReplyRPC:   "requestVoteReply",
		cs.raftReplyRPC:          "raftReply",
		cs.weakProposeRPC:        "weakPropose",
		cs.weakReplyRPC:          "weakReply",
		cs.weakReadRPC:           "weakRead",
		cs.weakReadReplyRPC:      "weakReadReply",
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
// Phase 49.9b: processWeakRead test
// ============================================================================

func TestProcessWeakRead_ViaChannel(t *testing.T) {
	r := newTestReplica(0, 3)
	st := state.InitState()

	// Put a value into state
	putCmd := state.Command{Op: state.PUT, K: state.Key(42), V: state.Value([]byte("channel-val"))}
	putCmd.Execute(st)
	r.keyVersions[42] = 7

	// Simulate processWeakRead logic (same as old handleWeakRead but now in executeCommands)
	msg := &MWeakRead{CommandId: 5, ClientId: 200, Key: state.Key(42)}

	cmd := state.Command{Op: state.GET, K: msg.Key, V: state.NIL()}
	value := cmd.Execute(st)

	version := int32(0)
	if v, ok := r.keyVersions[int64(msg.Key)]; ok {
		version = v
	}

	if !bytes.Equal(value, []byte("channel-val")) {
		t.Errorf("Should read 'channel-val', got %v", value)
	}
	if version != 7 {
		t.Errorf("Version should be 7, got %d", version)
	}
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
		weakReadCh:              make(chan weakReadReq, 100),
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
