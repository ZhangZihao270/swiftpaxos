package raft

import (
	"bytes"
	"testing"

	fastrpc "github.com/imdea-software/swiftpaxos/rpc"
	"github.com/imdea-software/swiftpaxos/state"
)

// ============================================================================
// Phase 39.1: Serialization Round-trip Tests
// ============================================================================

// --- RequestVote ---

func TestRequestVoteSerialization(t *testing.T) {
	original := &RequestVote{
		CandidateId:  2,
		Term:         5,
		LastLogIndex: 100,
		LastLogTerm:  4,
	}

	var buf bytes.Buffer
	original.Marshal(&buf)

	restored := &RequestVote{}
	if err := restored.Unmarshal(&buf); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if restored.CandidateId != original.CandidateId {
		t.Errorf("CandidateId mismatch: got %d, want %d", restored.CandidateId, original.CandidateId)
	}
	if restored.Term != original.Term {
		t.Errorf("Term mismatch: got %d, want %d", restored.Term, original.Term)
	}
	if restored.LastLogIndex != original.LastLogIndex {
		t.Errorf("LastLogIndex mismatch: got %d, want %d", restored.LastLogIndex, original.LastLogIndex)
	}
	if restored.LastLogTerm != original.LastLogTerm {
		t.Errorf("LastLogTerm mismatch: got %d, want %d", restored.LastLogTerm, original.LastLogTerm)
	}
}

func TestRequestVoteZeroValues(t *testing.T) {
	original := &RequestVote{}

	var buf bytes.Buffer
	original.Marshal(&buf)

	restored := &RequestVote{}
	if err := restored.Unmarshal(&buf); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if *restored != *original {
		t.Errorf("Zero-value mismatch: got %+v, want %+v", restored, original)
	}
}

func TestRequestVoteNegativeValues(t *testing.T) {
	original := &RequestVote{
		CandidateId:  -1,
		Term:         -100,
		LastLogIndex: -50,
		LastLogTerm:  -1,
	}

	var buf bytes.Buffer
	original.Marshal(&buf)

	restored := &RequestVote{}
	if err := restored.Unmarshal(&buf); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if *restored != *original {
		t.Errorf("Negative-value mismatch: got %+v, want %+v", restored, original)
	}
}

func TestRequestVoteBinarySize(t *testing.T) {
	rv := &RequestVote{CandidateId: 1, Term: 2, LastLogIndex: 3, LastLogTerm: 4}
	nbytes, known := rv.BinarySize()
	if !known {
		t.Error("BinarySize should be known for RequestVote")
	}

	var buf bytes.Buffer
	rv.Marshal(&buf)
	if buf.Len() != nbytes {
		t.Errorf("BinarySize %d != marshalled size %d", nbytes, buf.Len())
	}
}

// --- RequestVoteReply ---

func TestRequestVoteReplySerialization(t *testing.T) {
	original := &RequestVoteReply{
		VoterId:     1,
		Term:        5,
		VoteGranted: 1,
	}

	var buf bytes.Buffer
	original.Marshal(&buf)

	restored := &RequestVoteReply{}
	if err := restored.Unmarshal(&buf); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if restored.VoterId != original.VoterId {
		t.Errorf("VoterId mismatch: got %d, want %d", restored.VoterId, original.VoterId)
	}
	if restored.Term != original.Term {
		t.Errorf("Term mismatch: got %d, want %d", restored.Term, original.Term)
	}
	if restored.VoteGranted != original.VoteGranted {
		t.Errorf("VoteGranted mismatch: got %d, want %d", restored.VoteGranted, original.VoteGranted)
	}
}

func TestRequestVoteReplyNotGranted(t *testing.T) {
	original := &RequestVoteReply{
		VoterId:     0,
		Term:        3,
		VoteGranted: 0,
	}

	var buf bytes.Buffer
	original.Marshal(&buf)

	restored := &RequestVoteReply{}
	if err := restored.Unmarshal(&buf); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if *restored != *original {
		t.Errorf("Mismatch: got %+v, want %+v", restored, original)
	}
}

func TestRequestVoteReplyBinarySize(t *testing.T) {
	rvr := &RequestVoteReply{VoterId: 1, Term: 2, VoteGranted: 1}
	nbytes, known := rvr.BinarySize()
	if !known {
		t.Error("BinarySize should be known for RequestVoteReply")
	}

	var buf bytes.Buffer
	rvr.Marshal(&buf)
	if buf.Len() != nbytes {
		t.Errorf("BinarySize %d != marshalled size %d", nbytes, buf.Len())
	}
}

// --- AppendEntriesReply ---

func TestAppendEntriesReplySerialization(t *testing.T) {
	original := &AppendEntriesReply{
		FollowerId: 2,
		Term:       7,
		Success:    1,
		MatchIndex: 42,
	}

	var buf bytes.Buffer
	original.Marshal(&buf)

	restored := &AppendEntriesReply{}
	if err := restored.Unmarshal(&buf); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if restored.FollowerId != original.FollowerId {
		t.Errorf("FollowerId mismatch: got %d, want %d", restored.FollowerId, original.FollowerId)
	}
	if restored.Term != original.Term {
		t.Errorf("Term mismatch: got %d, want %d", restored.Term, original.Term)
	}
	if restored.Success != original.Success {
		t.Errorf("Success mismatch: got %d, want %d", restored.Success, original.Success)
	}
	if restored.MatchIndex != original.MatchIndex {
		t.Errorf("MatchIndex mismatch: got %d, want %d", restored.MatchIndex, original.MatchIndex)
	}
}

func TestAppendEntriesReplyFailure(t *testing.T) {
	original := &AppendEntriesReply{
		FollowerId: 1,
		Term:       10,
		Success:    0,
		MatchIndex: -1,
	}

	var buf bytes.Buffer
	original.Marshal(&buf)

	restored := &AppendEntriesReply{}
	if err := restored.Unmarshal(&buf); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if *restored != *original {
		t.Errorf("Mismatch: got %+v, want %+v", restored, original)
	}
}

func TestAppendEntriesReplyBinarySize(t *testing.T) {
	aer := &AppendEntriesReply{FollowerId: 1, Term: 2, Success: 1, MatchIndex: 3}
	nbytes, known := aer.BinarySize()
	if !known {
		t.Error("BinarySize should be known for AppendEntriesReply")
	}

	var buf bytes.Buffer
	aer.Marshal(&buf)
	if buf.Len() != nbytes {
		t.Errorf("BinarySize %d != marshalled size %d", nbytes, buf.Len())
	}
}

// --- AppendEntries ---

func TestAppendEntriesWithEntries(t *testing.T) {
	original := &AppendEntries{
		LeaderId:     0,
		Term:         3,
		PrevLogIndex: 5,
		PrevLogTerm:  2,
		LeaderCommit: 4,
		EntryCnt:     2,
		Entries: []state.Command{
			{Op: state.PUT, K: state.Key(10), V: state.Value([]byte("hello"))},
			{Op: state.GET, K: state.Key(20), V: state.Value([]byte{})},
		},
		EntryIds: []CommandId{
			{ClientId: 100, SeqNum: 1},
			{ClientId: 100, SeqNum: 2},
		},
	}

	var buf bytes.Buffer
	original.Marshal(&buf)

	restored := &AppendEntries{}
	if err := restored.Unmarshal(&buf); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if restored.LeaderId != original.LeaderId {
		t.Errorf("LeaderId mismatch: got %d, want %d", restored.LeaderId, original.LeaderId)
	}
	if restored.Term != original.Term {
		t.Errorf("Term mismatch: got %d, want %d", restored.Term, original.Term)
	}
	if restored.PrevLogIndex != original.PrevLogIndex {
		t.Errorf("PrevLogIndex mismatch: got %d, want %d", restored.PrevLogIndex, original.PrevLogIndex)
	}
	if restored.PrevLogTerm != original.PrevLogTerm {
		t.Errorf("PrevLogTerm mismatch: got %d, want %d", restored.PrevLogTerm, original.PrevLogTerm)
	}
	if restored.LeaderCommit != original.LeaderCommit {
		t.Errorf("LeaderCommit mismatch: got %d, want %d", restored.LeaderCommit, original.LeaderCommit)
	}
	if restored.EntryCnt != original.EntryCnt {
		t.Errorf("EntryCnt mismatch: got %d, want %d", restored.EntryCnt, original.EntryCnt)
	}
	if len(restored.Entries) != len(original.Entries) {
		t.Fatalf("Entries length mismatch: got %d, want %d", len(restored.Entries), len(original.Entries))
	}
	for i := range original.Entries {
		if restored.Entries[i].Op != original.Entries[i].Op {
			t.Errorf("Entries[%d].Op mismatch: got %d, want %d", i, restored.Entries[i].Op, original.Entries[i].Op)
		}
		if restored.Entries[i].K != original.Entries[i].K {
			t.Errorf("Entries[%d].K mismatch: got %d, want %d", i, restored.Entries[i].K, original.Entries[i].K)
		}
		if !bytes.Equal(restored.Entries[i].V, original.Entries[i].V) {
			t.Errorf("Entries[%d].V mismatch: got %v, want %v", i, restored.Entries[i].V, original.Entries[i].V)
		}
	}
	if len(restored.EntryIds) != len(original.EntryIds) {
		t.Fatalf("EntryIds length mismatch: got %d, want %d", len(restored.EntryIds), len(original.EntryIds))
	}
	for i := range original.EntryIds {
		if restored.EntryIds[i] != original.EntryIds[i] {
			t.Errorf("EntryIds[%d] mismatch: got %+v, want %+v", i, restored.EntryIds[i], original.EntryIds[i])
		}
	}
}

func TestAppendEntriesEmpty(t *testing.T) {
	// Heartbeat: no entries
	original := &AppendEntries{
		LeaderId:     0,
		Term:         1,
		PrevLogIndex: 0,
		PrevLogTerm:  0,
		LeaderCommit: 0,
		EntryCnt:     0,
		Entries:      []state.Command{},
		EntryIds:     []CommandId{},
	}

	var buf bytes.Buffer
	original.Marshal(&buf)

	restored := &AppendEntries{}
	if err := restored.Unmarshal(&buf); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if restored.LeaderId != original.LeaderId {
		t.Errorf("LeaderId mismatch: got %d, want %d", restored.LeaderId, original.LeaderId)
	}
	if restored.Term != original.Term {
		t.Errorf("Term mismatch: got %d, want %d", restored.Term, original.Term)
	}
	if len(restored.Entries) != 0 {
		t.Errorf("Entries should be empty, got %d", len(restored.Entries))
	}
	if len(restored.EntryIds) != 0 {
		t.Errorf("EntryIds should be empty, got %d", len(restored.EntryIds))
	}
}

func TestAppendEntriesNilSlices(t *testing.T) {
	// nil slices (len=0 when marshalled)
	original := &AppendEntries{
		LeaderId:     1,
		Term:         2,
		PrevLogIndex: 3,
		PrevLogTerm:  1,
		LeaderCommit: 2,
		EntryCnt:     0,
		Entries:      nil,
		EntryIds:     nil,
	}

	var buf bytes.Buffer
	original.Marshal(&buf)

	restored := &AppendEntries{}
	if err := restored.Unmarshal(&buf); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if len(restored.Entries) != 0 {
		t.Errorf("Entries should be empty, got %d", len(restored.Entries))
	}
	if len(restored.EntryIds) != 0 {
		t.Errorf("EntryIds should be empty, got %d", len(restored.EntryIds))
	}
}

func TestAppendEntriesMultipleEntries(t *testing.T) {
	// Batch case: multiple entries
	entries := make([]state.Command, 10)
	ids := make([]CommandId, 10)
	for i := 0; i < 10; i++ {
		entries[i] = state.Command{
			Op: state.PUT,
			K:  state.Key(int64(i)),
			V:  state.Value([]byte("val")),
		}
		ids[i] = CommandId{ClientId: int32(i % 3), SeqNum: int32(i)}
	}

	original := &AppendEntries{
		LeaderId:     0,
		Term:         5,
		PrevLogIndex: 10,
		PrevLogTerm:  4,
		LeaderCommit: 8,
		EntryCnt:     10,
		Entries:      entries,
		EntryIds:     ids,
	}

	var buf bytes.Buffer
	original.Marshal(&buf)

	restored := &AppendEntries{}
	if err := restored.Unmarshal(&buf); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if len(restored.Entries) != 10 {
		t.Fatalf("Expected 10 entries, got %d", len(restored.Entries))
	}
	if len(restored.EntryIds) != 10 {
		t.Fatalf("Expected 10 entry IDs, got %d", len(restored.EntryIds))
	}
	for i := 0; i < 10; i++ {
		if restored.Entries[i].K != state.Key(int64(i)) {
			t.Errorf("Entries[%d].K mismatch: got %d, want %d", i, restored.Entries[i].K, i)
		}
		if restored.EntryIds[i].SeqNum != int32(i) {
			t.Errorf("EntryIds[%d].SeqNum mismatch: got %d, want %d", i, restored.EntryIds[i].SeqNum, i)
		}
	}
}

func TestAppendEntriesBinarySize(t *testing.T) {
	ae := &AppendEntries{Entries: []state.Command{{Op: state.PUT, K: 1, V: []byte("x")}}, EntryIds: []CommandId{{1, 1}}}
	_, known := ae.BinarySize()
	if known {
		t.Error("BinarySize should be unknown for AppendEntries (variable length)")
	}
}

// --- RaftReply ---

func TestRaftReplySerialization(t *testing.T) {
	original := &RaftReply{
		CmdId: CommandId{ClientId: 42, SeqNum: 99},
		Value: []byte("result-data"),
	}

	var buf bytes.Buffer
	original.Marshal(&buf)

	restored := &RaftReply{}
	if err := restored.Unmarshal(&buf); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if restored.CmdId != original.CmdId {
		t.Errorf("CmdId mismatch: got %+v, want %+v", restored.CmdId, original.CmdId)
	}
	if !bytes.Equal(restored.Value, original.Value) {
		t.Errorf("Value mismatch: got %v, want %v", restored.Value, original.Value)
	}
}

func TestRaftReplyEmptyValue(t *testing.T) {
	original := &RaftReply{
		CmdId: CommandId{ClientId: 1, SeqNum: 1},
		Value: []byte{},
	}

	var buf bytes.Buffer
	original.Marshal(&buf)

	restored := &RaftReply{}
	if err := restored.Unmarshal(&buf); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if len(restored.Value) != 0 {
		t.Errorf("Value should be empty, got %v", restored.Value)
	}
}

func TestRaftReplyNilValue(t *testing.T) {
	original := &RaftReply{
		CmdId: CommandId{ClientId: 5, SeqNum: 10},
		Value: nil,
	}

	var buf bytes.Buffer
	original.Marshal(&buf)

	restored := &RaftReply{}
	if err := restored.Unmarshal(&buf); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if len(restored.Value) != 0 {
		t.Errorf("Value should be empty, got %v", restored.Value)
	}
}

func TestRaftReplyBinarySize(t *testing.T) {
	rr := &RaftReply{CmdId: CommandId{1, 1}, Value: []byte("test")}
	_, known := rr.BinarySize()
	if known {
		t.Error("BinarySize should be unknown for RaftReply (variable length)")
	}
}

// ============================================================================
// New() factory method tests
// ============================================================================

func TestNewMethods(t *testing.T) {
	rv := (&RequestVote{}).New()
	if _, ok := rv.(*RequestVote); !ok {
		t.Error("RequestVote.New() should return *RequestVote")
	}

	rvr := (&RequestVoteReply{}).New()
	if _, ok := rvr.(*RequestVoteReply); !ok {
		t.Error("RequestVoteReply.New() should return *RequestVoteReply")
	}

	ae := (&AppendEntries{}).New()
	if _, ok := ae.(*AppendEntries); !ok {
		t.Error("AppendEntries.New() should return *AppendEntries")
	}

	aer := (&AppendEntriesReply{}).New()
	if _, ok := aer.(*AppendEntriesReply); !ok {
		t.Error("AppendEntriesReply.New() should return *AppendEntriesReply")
	}

	rr := (&RaftReply{}).New()
	if _, ok := rr.(*RaftReply); !ok {
		t.Error("RaftReply.New() should return *RaftReply")
	}
}

// ============================================================================
// Cache pool tests
// ============================================================================

func TestRequestVoteCache(t *testing.T) {
	cache := NewRequestVoteCache()

	// Get from empty cache
	rv := cache.Get()
	if rv == nil {
		t.Fatal("Get() from empty cache should return non-nil")
	}

	// Put and get back
	rv.CandidateId = 42
	cache.Put(rv)
	rv2 := cache.Get()
	if rv2.CandidateId != 42 {
		t.Error("Get() after Put() should return the same object")
	}

	// Empty again
	rv3 := cache.Get()
	if rv3 == nil {
		t.Fatal("Get() from empty cache should return non-nil")
	}
}

func TestRequestVoteReplyCache(t *testing.T) {
	cache := NewRequestVoteReplyCache()
	rvr := cache.Get()
	if rvr == nil {
		t.Fatal("Get() should return non-nil")
	}
	rvr.VoterId = 7
	cache.Put(rvr)
	rvr2 := cache.Get()
	if rvr2.VoterId != 7 {
		t.Error("Cache should return put object")
	}
}

func TestAppendEntriesCache(t *testing.T) {
	cache := NewAppendEntriesCache()
	ae := cache.Get()
	if ae == nil {
		t.Fatal("Get() should return non-nil")
	}
	ae.Term = 99
	cache.Put(ae)
	ae2 := cache.Get()
	if ae2.Term != 99 {
		t.Error("Cache should return put object")
	}
}

func TestAppendEntriesReplyCache(t *testing.T) {
	cache := NewAppendEntriesReplyCache()
	aer := cache.Get()
	if aer == nil {
		t.Fatal("Get() should return non-nil")
	}
	aer.FollowerId = 3
	cache.Put(aer)
	aer2 := cache.Get()
	if aer2.FollowerId != 3 {
		t.Error("Cache should return put object")
	}
}

func TestRaftReplyCache(t *testing.T) {
	cache := NewRaftReplyCache()
	rr := cache.Get()
	if rr == nil {
		t.Fatal("Get() should return non-nil")
	}
	rr.CmdId.SeqNum = 55
	cache.Put(rr)
	rr2 := cache.Get()
	if rr2.CmdId.SeqNum != 55 {
		t.Error("Cache should return put object")
	}
}

func TestCacheMultiplePutGet(t *testing.T) {
	cache := NewRequestVoteCache()

	// Put 3 items
	for i := int32(0); i < 3; i++ {
		rv := &RequestVote{CandidateId: i}
		cache.Put(rv)
	}

	// Get 3 items (LIFO order)
	for i := int32(2); i >= 0; i-- {
		rv := cache.Get()
		if rv.CandidateId != i {
			t.Errorf("Expected CandidateId %d, got %d (LIFO order)", i, rv.CandidateId)
		}
	}

	// Empty now, should allocate new
	rv := cache.Get()
	if rv == nil {
		t.Fatal("Get() from empty cache should allocate new")
	}
	if rv.CandidateId != 0 {
		t.Error("Newly allocated should be zero-value")
	}
}

// ============================================================================
// CommunicationSupply + initCs tests
// ============================================================================

func TestInitCs(t *testing.T) {
	cs := &CommunicationSupply{}
	table := fastrpc.NewTable()
	initCs(cs, table)

	// Check all channels are non-nil
	if cs.appendEntriesChan == nil {
		t.Error("appendEntriesChan should be non-nil")
	}
	if cs.appendEntriesReplyChan == nil {
		t.Error("appendEntriesReplyChan should be non-nil")
	}
	if cs.requestVoteChan == nil {
		t.Error("requestVoteChan should be non-nil")
	}
	if cs.requestVoteReplyChan == nil {
		t.Error("requestVoteReplyChan should be non-nil")
	}
	if cs.raftReplyChan == nil {
		t.Error("raftReplyChan should be non-nil")
	}

	// Check all RPC IDs are distinct
	ids := map[uint8]string{
		cs.appendEntriesRPC:      "appendEntries",
		cs.appendEntriesReplyRPC: "appendEntriesReply",
		cs.requestVoteRPC:        "requestVote",
		cs.requestVoteReplyRPC:   "requestVoteReply",
		cs.raftReplyRPC:          "raftReply",
	}
	if len(ids) != 5 {
		t.Errorf("Expected 5 distinct RPC IDs, got %d (some collide)", len(ids))
	}

	// Check table has all 5 registered
	for id, name := range ids {
		pair, ok := table.Get(id)
		if !ok {
			t.Errorf("RPC ID %d (%s) not found in table", id, name)
		}
		if pair.Chan == nil {
			t.Errorf("RPC ID %d (%s) has nil channel", id, name)
		}
		if pair.Obj == nil {
			t.Errorf("RPC ID %d (%s) has nil prototype object", id, name)
		}
	}
}

// ============================================================================
// Large value / edge case tests
// ============================================================================

func TestAppendEntriesLargeValue(t *testing.T) {
	largeVal := make([]byte, 10000)
	for i := range largeVal {
		largeVal[i] = byte(i % 256)
	}

	original := &AppendEntries{
		LeaderId:     0,
		Term:         1,
		PrevLogIndex: 0,
		PrevLogTerm:  0,
		LeaderCommit: 0,
		EntryCnt:     1,
		Entries: []state.Command{
			{Op: state.PUT, K: state.Key(1), V: state.Value(largeVal)},
		},
		EntryIds: []CommandId{
			{ClientId: 1, SeqNum: 1},
		},
	}

	var buf bytes.Buffer
	original.Marshal(&buf)

	restored := &AppendEntries{}
	if err := restored.Unmarshal(&buf); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if !bytes.Equal(restored.Entries[0].V, original.Entries[0].V) {
		t.Errorf("Large value mismatch: lengths got %d, want %d", len(restored.Entries[0].V), len(original.Entries[0].V))
	}
}

func TestRaftReplyLargeValue(t *testing.T) {
	largeVal := make([]byte, 5000)
	for i := range largeVal {
		largeVal[i] = byte(i % 256)
	}

	original := &RaftReply{
		CmdId: CommandId{ClientId: 1, SeqNum: 1},
		Value: largeVal,
	}

	var buf bytes.Buffer
	original.Marshal(&buf)

	restored := &RaftReply{}
	if err := restored.Unmarshal(&buf); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if !bytes.Equal(restored.Value, original.Value) {
		t.Errorf("Large value mismatch: lengths got %d, want %d", len(restored.Value), len(original.Value))
	}
}

func TestMaxInt32Values(t *testing.T) {
	original := &RequestVote{
		CandidateId:  2147483647, // max int32
		Term:         -2147483648, // min int32
		LastLogIndex: 0,
		LastLogTerm:  1,
	}

	var buf bytes.Buffer
	original.Marshal(&buf)

	restored := &RequestVote{}
	if err := restored.Unmarshal(&buf); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if *restored != *original {
		t.Errorf("Max int32 mismatch: got %+v, want %+v", restored, original)
	}
}
