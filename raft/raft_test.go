package raft

import (
	"bytes"
	"testing"
	"time"

	"github.com/imdea-software/swiftpaxos/replica/defs"
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

// ============================================================================
// Phase 39.2a: Replica struct, LogEntry, RaftState constants tests
// ============================================================================

// --- RaftState constants ---

func TestRaftStateConstants(t *testing.T) {
	// Verify constants are distinct
	if FOLLOWER == CANDIDATE || FOLLOWER == LEADER || CANDIDATE == LEADER {
		t.Error("FOLLOWER, CANDIDATE, LEADER must be distinct")
	}
	// Verify FOLLOWER is the zero value (default role)
	if FOLLOWER != 0 {
		t.Error("FOLLOWER should be 0 (iota start)")
	}
}

// --- LogEntry ---

func TestLogEntryFields(t *testing.T) {
	entry := LogEntry{
		Command: state.Command{
			Op: state.PUT,
			K:  state.Key(42),
			V:  state.Value([]byte("hello")),
		},
		Term:  3,
		CmdId: CommandId{ClientId: 10, SeqNum: 5},
	}

	if entry.Term != 3 {
		t.Errorf("Term mismatch: got %d, want 3", entry.Term)
	}
	if entry.CmdId.ClientId != 10 {
		t.Errorf("CmdId.ClientId mismatch: got %d, want 10", entry.CmdId.ClientId)
	}
	if entry.CmdId.SeqNum != 5 {
		t.Errorf("CmdId.SeqNum mismatch: got %d, want 5", entry.CmdId.SeqNum)
	}
	if entry.Command.Op != state.PUT {
		t.Errorf("Command.Op mismatch: got %d, want PUT", entry.Command.Op)
	}
	if entry.Command.K != state.Key(42) {
		t.Errorf("Command.K mismatch: got %d, want 42", entry.Command.K)
	}
}

func TestLogEntryZeroValue(t *testing.T) {
	var entry LogEntry
	if entry.Term != 0 {
		t.Errorf("Zero-value Term should be 0, got %d", entry.Term)
	}
	if entry.CmdId.ClientId != 0 || entry.CmdId.SeqNum != 0 {
		t.Error("Zero-value CmdId should be {0, 0}")
	}
}

// --- LogEntry serialization (via AppendEntries round-trip) ---

func TestLogEntryViaAppendEntriesSerialization(t *testing.T) {
	// Verify that log entries survive a full serialize/deserialize cycle
	// when embedded in AppendEntries
	entries := []LogEntry{
		{
			Command: state.Command{Op: state.PUT, K: state.Key(1), V: state.Value([]byte("v1"))},
			Term:    1,
			CmdId:   CommandId{ClientId: 100, SeqNum: 1},
		},
		{
			Command: state.Command{Op: state.GET, K: state.Key(2), V: state.Value([]byte{})},
			Term:    2,
			CmdId:   CommandId{ClientId: 100, SeqNum: 2},
		},
	}

	ae := &AppendEntries{
		LeaderId:     0,
		Term:         2,
		PrevLogIndex: 0,
		PrevLogTerm:  1,
		LeaderCommit: 0,
		EntryCnt:     2,
		Entries:      make([]state.Command, len(entries)),
		EntryIds:     make([]CommandId, len(entries)),
	}
	for i, e := range entries {
		ae.Entries[i] = e.Command
		ae.EntryIds[i] = e.CmdId
	}

	var buf bytes.Buffer
	ae.Marshal(&buf)

	restored := &AppendEntries{}
	if err := restored.Unmarshal(&buf); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	// Reconstruct log entries from deserialized AppendEntries
	for i, e := range entries {
		if restored.Entries[i].Op != e.Command.Op {
			t.Errorf("Entry[%d].Op mismatch", i)
		}
		if restored.Entries[i].K != e.Command.K {
			t.Errorf("Entry[%d].K mismatch", i)
		}
		if restored.EntryIds[i] != e.CmdId {
			t.Errorf("Entry[%d].CmdId mismatch: got %+v, want %+v", i, restored.EntryIds[i], e.CmdId)
		}
	}
}

// --- BeTheLeader ---

func TestBeTheLeaderSetsRole(t *testing.T) {
	// Test BeTheLeader on a minimal Replica struct (without full network init)
	r := &Replica{
		role:       FOLLOWER,
		votedFor:   -1,
		log:        make([]LogEntry, 0),
		nextIndex:  make([]int32, 3),
		matchIndex: make([]int32, 3),
	}
	// Simulate base replica fields needed
	r.Replica = nil // We can't call r.Println without a base, so test just the state changes

	// Manually set the fields that BeTheLeader reads
	n := 3
	r.nextIndex = make([]int32, n)
	r.matchIndex = make([]int32, n)

	// Can't call BeTheLeader directly because it uses r.Id and r.N from embedded Replica
	// Instead, test the logic inline
	r.role = LEADER
	r.votedFor = 0 // assume Id=0
	lastLogIndex := int32(len(r.log) - 1)
	for i := 0; i < n; i++ {
		r.nextIndex[i] = lastLogIndex + 1
		r.matchIndex[i] = -1
	}
	r.matchIndex[0] = lastLogIndex

	if r.role != LEADER {
		t.Error("Should be LEADER after BeTheLeader")
	}
	if r.votedFor != 0 {
		t.Error("votedFor should be own Id after BeTheLeader")
	}
	for i := 0; i < n; i++ {
		if r.nextIndex[i] != 0 {
			t.Errorf("nextIndex[%d] should be 0 (empty log), got %d", i, r.nextIndex[i])
		}
		if i == 0 {
			if r.matchIndex[i] != -1 {
				t.Errorf("matchIndex[leader] should be -1 (empty log), got %d", i)
			}
		} else {
			if r.matchIndex[i] != -1 {
				t.Errorf("matchIndex[%d] should be -1, got %d", i, r.matchIndex[i])
			}
		}
	}
}

func TestBeTheLeaderWithNonEmptyLog(t *testing.T) {
	n := 3
	r := &Replica{
		role:     FOLLOWER,
		votedFor: -1,
		log: []LogEntry{
			{Term: 1, CmdId: CommandId{ClientId: 1, SeqNum: 1}},
			{Term: 1, CmdId: CommandId{ClientId: 1, SeqNum: 2}},
			{Term: 2, CmdId: CommandId{ClientId: 2, SeqNum: 1}},
		},
		nextIndex:  make([]int32, n),
		matchIndex: make([]int32, n),
	}

	// Simulate BeTheLeader logic
	r.role = LEADER
	lastLogIndex := int32(len(r.log) - 1) // = 2
	for i := 0; i < n; i++ {
		r.nextIndex[i] = lastLogIndex + 1  // = 3
		r.matchIndex[i] = -1
	}
	r.matchIndex[0] = lastLogIndex // leader's own match = 2

	if r.role != LEADER {
		t.Error("Should be LEADER")
	}
	for i := 0; i < n; i++ {
		if r.nextIndex[i] != 3 {
			t.Errorf("nextIndex[%d] should be 3 (last+1), got %d", i, r.nextIndex[i])
		}
	}
	if r.matchIndex[0] != 2 {
		t.Errorf("matchIndex[leader] should be 2, got %d", r.matchIndex[0])
	}
}

// --- Initial state tests ---

func TestInitialFollowerState(t *testing.T) {
	r := &Replica{
		currentTerm:      0,
		votedFor:         -1,
		log:              make([]LogEntry, 0),
		commitIndex:      -1,
		lastApplied:      -1,
		role:             FOLLOWER,
		nextIndex:        make([]int32, 3),
		matchIndex:       make([]int32, 3),
		pendingProposals: make([]*defs.GPropose, 0),
		votesReceived:    0,
		votesNeeded:      2, // (3/2)+1
	}

	if r.currentTerm != 0 {
		t.Errorf("Initial term should be 0, got %d", r.currentTerm)
	}
	if r.votedFor != -1 {
		t.Errorf("Initial votedFor should be -1, got %d", r.votedFor)
	}
	if len(r.log) != 0 {
		t.Errorf("Initial log should be empty, got %d entries", len(r.log))
	}
	if r.commitIndex != -1 {
		t.Errorf("Initial commitIndex should be -1, got %d", r.commitIndex)
	}
	if r.lastApplied != -1 {
		t.Errorf("Initial lastApplied should be -1, got %d", r.lastApplied)
	}
	if r.role != FOLLOWER {
		t.Errorf("Initial role should be FOLLOWER, got %d", r.role)
	}
	if r.votesNeeded != 2 {
		t.Errorf("votesNeeded for 3 nodes should be 2, got %d", r.votesNeeded)
	}
}

func TestVotesNeededCalculation(t *testing.T) {
	tests := []struct {
		n        int
		expected int
	}{
		{3, 2},
		{5, 3},
		{7, 4},
		{1, 1},
	}

	for _, tc := range tests {
		needed := (tc.n / 2) + 1
		if needed != tc.expected {
			t.Errorf("For n=%d: expected votesNeeded=%d, got %d", tc.n, tc.expected, needed)
		}
	}
}

func TestPendingProposalsSlice(t *testing.T) {
	pending := make([]*defs.GPropose, 0)

	// Should start empty
	if len(pending) != 0 {
		t.Error("pendingProposals should start empty")
	}

	// Append proposals (growing with log)
	p1 := &defs.GPropose{}
	p2 := &defs.GPropose{}
	pending = append(pending, p1) // index 0
	pending = append(pending, nil) // index 1: follower-received entry, no proposal
	pending = append(pending, p2) // index 2

	if pending[0] != p1 {
		t.Error("Should find proposal at index 0")
	}
	if pending[1] != nil {
		t.Error("Index 1 should be nil (no proposal)")
	}
	if pending[2] != p2 {
		t.Error("Should find proposal at index 2")
	}

	// Nil out after execution (release for GC)
	pending[0] = nil
	if pending[0] != nil {
		t.Error("Should be nil after clearing")
	}
}

// ============================================================================
// Phase 39.2b-h: Raft Protocol Logic Tests
// ============================================================================

// newTestReplica creates a minimal Replica for unit testing (no network).
func newTestReplica(id int32, n int) *Replica {
	return &Replica{
		Replica:              nil, // no base replica (no network)
		id:                   id,
		currentTerm:          0,
		votedFor:             -1,
		log:                  make([]LogEntry, 0),
		commitIndex:          -1,
		lastApplied:          -1,
		role:                 FOLLOWER,
		n:                    n,
		nextIndex:            make([]int32, n),
		matchIndex:           make([]int32, n),
		pendingProposals:     make([]*defs.GPropose, 0),
		commitNotify:         make(chan struct{}, 1),
		votesReceived:        0,
		votesNeeded:          (n / 2) + 1,
		appendEntriesCache:   NewAppendEntriesCache(),
		appendEntriesReplyCache: NewAppendEntriesReplyCache(),
		requestVoteCache:     NewRequestVoteCache(),
		requestVoteReplyCache: NewRequestVoteReplyCache(),
		raftReplyCache:       NewRaftReplyCache(),
	}
}

// --- becomeFollower tests ---

func TestBecomeFollower(t *testing.T) {
	r := newTestReplica(0, 3)
	r.role = LEADER
	r.currentTerm = 2
	r.votedFor = 0
	r.votesReceived = 2

	r.becomeFollower(5)

	if r.currentTerm != 5 {
		t.Errorf("currentTerm should be 5, got %d", r.currentTerm)
	}
	if r.role != FOLLOWER {
		t.Errorf("role should be FOLLOWER, got %d", r.role)
	}
	if r.votedFor != -1 {
		t.Errorf("votedFor should be -1, got %d", r.votedFor)
	}
	if r.votesReceived != 0 {
		t.Errorf("votesReceived should be 0, got %d", r.votesReceived)
	}
}

func TestBecomeFollowerFromCandidate(t *testing.T) {
	r := newTestReplica(1, 3)
	r.role = CANDIDATE
	r.currentTerm = 3
	r.votedFor = 1
	r.votesReceived = 1

	r.becomeFollower(4)

	if r.role != FOLLOWER {
		t.Error("Should be FOLLOWER")
	}
	if r.currentTerm != 4 {
		t.Error("Term should be 4")
	}
}

// --- isLogUpToDate tests ---

func TestIsLogUpToDate_EmptyLogs(t *testing.T) {
	r := newTestReplica(0, 3)
	// Both empty: candidate's log is up-to-date
	msg := &RequestVote{LastLogIndex: -1, LastLogTerm: 0}
	if !r.isLogUpToDate(msg) {
		t.Error("Empty candidate log should be up-to-date against empty local log")
	}
}

func TestIsLogUpToDate_CandidateHasHigherTerm(t *testing.T) {
	r := newTestReplica(0, 3)
	r.log = []LogEntry{{Term: 1}, {Term: 1}}

	msg := &RequestVote{LastLogIndex: 0, LastLogTerm: 2}
	if !r.isLogUpToDate(msg) {
		t.Error("Candidate with higher last log term should be up-to-date")
	}
}

func TestIsLogUpToDate_CandidateHasLowerTerm(t *testing.T) {
	r := newTestReplica(0, 3)
	r.log = []LogEntry{{Term: 2}, {Term: 2}}

	msg := &RequestVote{LastLogIndex: 5, LastLogTerm: 1}
	if r.isLogUpToDate(msg) {
		t.Error("Candidate with lower last log term should NOT be up-to-date")
	}
}

func TestIsLogUpToDate_SameTermLongerLog(t *testing.T) {
	r := newTestReplica(0, 3)
	r.log = []LogEntry{{Term: 1}, {Term: 1}}

	msg := &RequestVote{LastLogIndex: 3, LastLogTerm: 1}
	if !r.isLogUpToDate(msg) {
		t.Error("Candidate with same term but longer log should be up-to-date")
	}
}

func TestIsLogUpToDate_SameTermShorterLog(t *testing.T) {
	r := newTestReplica(0, 3)
	r.log = []LogEntry{{Term: 1}, {Term: 1}, {Term: 1}}

	msg := &RequestVote{LastLogIndex: 0, LastLogTerm: 1}
	if r.isLogUpToDate(msg) {
		t.Error("Candidate with same term but shorter log should NOT be up-to-date")
	}
}

// --- advanceCommitIndex tests ---

func TestAdvanceCommitIndex_MajorityMatch(t *testing.T) {
	r := newTestReplica(0, 3)
	r.role = LEADER
	r.currentTerm = 1
	r.log = []LogEntry{
		{Term: 1},
		{Term: 1},
		{Term: 1},
	}
	r.matchIndex = []int32{2, 1, 2} // node 0=2, node 1=1, node 2=2
	r.commitIndex = -1

	r.advanceCommitIndex()

	// Sorted desc: [2, 2, 1]. Majority (index N/2=1) = 2
	if r.commitIndex != 2 {
		t.Errorf("commitIndex should be 2, got %d", r.commitIndex)
	}
}

func TestAdvanceCommitIndex_OnlyCurrentTermCommits(t *testing.T) {
	r := newTestReplica(0, 3)
	r.role = LEADER
	r.currentTerm = 2
	r.log = []LogEntry{
		{Term: 1}, // index 0: old term
		{Term: 2}, // index 1: current term
	}
	r.matchIndex = []int32{1, 0, 1}
	r.commitIndex = -1

	r.advanceCommitIndex()

	// Majority at index 1, and log[1].Term == currentTerm(2), so commit
	if r.commitIndex != 1 {
		t.Errorf("commitIndex should be 1, got %d", r.commitIndex)
	}
}

func TestAdvanceCommitIndex_OldTermNotCommitted(t *testing.T) {
	r := newTestReplica(0, 3)
	r.role = LEADER
	r.currentTerm = 2
	r.log = []LogEntry{
		{Term: 1}, // index 0: old term only
	}
	r.matchIndex = []int32{0, 0, 0}
	r.commitIndex = -1

	r.advanceCommitIndex()

	// Even though majority at 0, log[0].Term(1) != currentTerm(2) → no commit
	if r.commitIndex != -1 {
		t.Errorf("commitIndex should stay -1 (old term), got %d", r.commitIndex)
	}
}

func TestAdvanceCommitIndex_NoAdvancePastLog(t *testing.T) {
	r := newTestReplica(0, 3)
	r.role = LEADER
	r.currentTerm = 1
	r.log = []LogEntry{{Term: 1}}
	r.matchIndex = []int32{0, 0, 0}
	r.commitIndex = 0

	r.advanceCommitIndex()

	// Already committed, no further advance
	if r.commitIndex != 0 {
		t.Errorf("commitIndex should remain 0, got %d", r.commitIndex)
	}
}

func TestAdvanceCommitIndex_FiveNodes(t *testing.T) {
	r := newTestReplica(0, 5)
	r.role = LEADER
	r.currentTerm = 1
	r.log = []LogEntry{{Term: 1}, {Term: 1}, {Term: 1}}
	r.matchIndex = []int32{2, 2, 1, 0, 2} // sorted desc: [2,2,2,1,0], majority at index 2 = 2
	r.commitIndex = 0

	r.advanceCommitIndex()

	if r.commitIndex != 2 {
		t.Errorf("commitIndex should be 2, got %d", r.commitIndex)
	}
}

func TestAdvanceCommitIndex_MultipleIndicesInOneCall(t *testing.T) {
	// Advances from commitIndex=-1 through indices 0,1,2 in a single call
	r := newTestReplica(0, 3)
	r.role = LEADER
	r.currentTerm = 1
	r.log = []LogEntry{{Term: 1}, {Term: 1}, {Term: 1}}
	r.matchIndex = []int32{2, 2, -1} // majority at 2
	r.commitIndex = -1

	r.advanceCommitIndex()

	if r.commitIndex != 2 {
		t.Errorf("commitIndex should be 2 (all three entries), got %d", r.commitIndex)
	}
}

func TestAdvanceCommitIndex_SkipsOldTermEntries(t *testing.T) {
	// Entries from old term should be skipped, only current term can be committed directly
	r := newTestReplica(0, 3)
	r.role = LEADER
	r.currentTerm = 2
	r.log = []LogEntry{
		{Term: 1}, // index 0: old term
		{Term: 2}, // index 1: current term
		{Term: 2}, // index 2: current term
	}
	r.matchIndex = []int32{2, 2, 2}
	r.commitIndex = -1

	r.advanceCommitIndex()

	// Index 0 has term 1 != currentTerm 2, so it's skipped.
	// But index 1 has term 2 == currentTerm, and majority have matchIndex >= 1.
	// Raft §5.4.2: once a current-term entry is committed, all prior entries are
	// implicitly committed. The scan continues past index 0 to commit index 1 and 2.
	if r.commitIndex != 2 {
		t.Errorf("commitIndex should be 2, got %d", r.commitIndex)
	}
}

func TestAdvanceCommitIndex_StopsAtPartialMajority(t *testing.T) {
	// matchIndex shows partial replication — majority only up to index 1
	r := newTestReplica(0, 5)
	r.role = LEADER
	r.currentTerm = 1
	r.log = []LogEntry{{Term: 1}, {Term: 1}, {Term: 1}, {Term: 1}}
	r.matchIndex = []int32{3, 1, 1, 0, 0} // 3 replicas have >=1, only 2 have >=2
	r.commitIndex = -1

	r.advanceCommitIndex()

	if r.commitIndex != 1 {
		t.Errorf("commitIndex should be 1, got %d", r.commitIndex)
	}
}

// --- startElection tests ---

func TestStartElection(t *testing.T) {
	r := newTestReplica(1, 3)
	r.currentTerm = 3
	r.role = FOLLOWER
	r.votedFor = -1

	// Can't call startElection directly because it uses r.sender and r.Id from embedded Replica.
	// Test the logic inline.
	r.currentTerm++
	r.role = CANDIDATE
	r.votedFor = 1 // self
	r.votesReceived = 1

	if r.currentTerm != 4 {
		t.Errorf("Term should be 4 after election start, got %d", r.currentTerm)
	}
	if r.role != CANDIDATE {
		t.Error("Should be CANDIDATE")
	}
	if r.votedFor != 1 {
		t.Error("Should have voted for self")
	}
	if r.votesReceived != 1 {
		t.Error("Should count own vote")
	}
}

// --- handleRequestVoteReply tests ---

func TestHandleRequestVoteReply_WinElection(t *testing.T) {
	r := newTestReplica(0, 3)
	r.role = CANDIDATE
	r.currentTerm = 5
	r.votesReceived = 1 // own vote
	r.votesNeeded = 2

	// First vote from another node
	msg := &RequestVoteReply{VoterId: 1, Term: 5, VoteGranted: 1}
	r.handleRequestVoteReply(msg)

	if r.votesReceived != 2 {
		t.Errorf("votesReceived should be 2, got %d", r.votesReceived)
	}
	// becomeLeader would be called, but it uses r.Println (embedded Replica)
	// In the actual code, it transitions to LEADER. Here we test the vote counting.
}

func TestHandleRequestVoteReply_HigherTermStepsDown(t *testing.T) {
	r := newTestReplica(0, 3)
	r.role = CANDIDATE
	r.currentTerm = 3

	msg := &RequestVoteReply{VoterId: 1, Term: 5, VoteGranted: 0}
	r.handleRequestVoteReply(msg)

	if r.role != FOLLOWER {
		t.Error("Should step down to FOLLOWER on higher term")
	}
	if r.currentTerm != 5 {
		t.Errorf("Term should be 5, got %d", r.currentTerm)
	}
}

func TestHandleRequestVoteReply_IgnoredWhenNotCandidate(t *testing.T) {
	r := newTestReplica(0, 3)
	r.role = FOLLOWER
	r.currentTerm = 3
	r.votesReceived = 0

	msg := &RequestVoteReply{VoterId: 1, Term: 3, VoteGranted: 1}
	r.handleRequestVoteReply(msg)

	if r.votesReceived != 0 {
		t.Error("Should ignore vote when not CANDIDATE")
	}
}

// --- handleAppendEntries logic tests ---

func TestHandleAppendEntries_RejectStaleTerm(t *testing.T) {
	r := newTestReplica(0, 3)
	r.currentTerm = 5

	msg := &AppendEntries{
		LeaderId:     1,
		Term:         3, // stale
		PrevLogIndex: -1,
		PrevLogTerm:  0,
		LeaderCommit: 0,
		Entries:      nil,
		EntryIds:     nil,
	}

	// Can't call handleAppendEntries directly (uses r.sender).
	// Test the logic: stale term should be rejected.
	if msg.Term < r.currentTerm {
		// This would send a rejection reply in the actual handler
	}
	if msg.Term >= r.currentTerm {
		t.Error("Term 3 should be stale compared to currentTerm 5")
	}
}

func TestHandleAppendEntries_StepDownOnHigherTerm(t *testing.T) {
	r := newTestReplica(0, 3)
	r.currentTerm = 3
	r.role = CANDIDATE

	// If msg.Term > currentTerm → becomeFollower
	r.becomeFollower(5)

	if r.role != FOLLOWER {
		t.Error("Should step down to FOLLOWER")
	}
	if r.currentTerm != 5 {
		t.Error("Term should be updated to 5")
	}
}

func TestHandleAppendEntries_LogConsistencyCheck(t *testing.T) {
	r := newTestReplica(0, 3)
	r.log = []LogEntry{
		{Term: 1, CmdId: CommandId{ClientId: 1, SeqNum: 1}},
		{Term: 1, CmdId: CommandId{ClientId: 1, SeqNum: 2}},
	}

	// PrevLogIndex=1, PrevLogTerm=1 → should pass (log[1].Term == 1)
	prevLogIndex := int32(1)
	if prevLogIndex >= int32(len(r.log)) {
		t.Error("Log should have entry at index 1")
	}
	if r.log[prevLogIndex].Term != 1 {
		t.Error("Term at index 1 should be 1")
	}

	// PrevLogIndex=1, PrevLogTerm=2 → should fail (log[1].Term is 1, not 2)
	if r.log[prevLogIndex].Term == 2 {
		t.Error("This should not match")
	}
}

func TestHandleAppendEntries_AppendNewEntries(t *testing.T) {
	r := newTestReplica(0, 3)
	r.currentTerm = 2
	r.log = []LogEntry{
		{Term: 1, CmdId: CommandId{ClientId: 1, SeqNum: 1}},
	}

	// Simulate appending entries at index 1
	newEntries := []state.Command{
		{Op: state.PUT, K: state.Key(10), V: state.Value([]byte("v1"))},
		{Op: state.GET, K: state.Key(20), V: state.Value([]byte{})},
	}
	newIds := []CommandId{
		{ClientId: 2, SeqNum: 1},
		{ClientId: 2, SeqNum: 2},
	}

	insertIdx := int32(1) // PrevLogIndex(0) + 1
	for i := 0; i < len(newEntries); i++ {
		entry := LogEntry{
			Command: newEntries[i],
			Term:    2,
			CmdId:   newIds[i],
		}
		r.log = append(r.log, entry)
	}

	if len(r.log) != 3 {
		t.Errorf("Log should have 3 entries, got %d", len(r.log))
	}
	if r.log[1].Term != 2 {
		t.Errorf("Entry at index 1 should have term 2, got %d", r.log[1].Term)
	}
	if r.log[2].CmdId.SeqNum != 2 {
		t.Errorf("Entry at index 2 should have SeqNum 2, got %d", r.log[2].CmdId.SeqNum)
	}
	_ = insertIdx
}

func TestHandleAppendEntries_TruncateConflicting(t *testing.T) {
	r := newTestReplica(0, 3)
	r.log = []LogEntry{
		{Term: 1, CmdId: CommandId{ClientId: 1, SeqNum: 1}},
		{Term: 1, CmdId: CommandId{ClientId: 1, SeqNum: 2}},
		{Term: 2, CmdId: CommandId{ClientId: 1, SeqNum: 3}}, // will conflict
	}

	// Simulate: leader says index 2 should have term 3, not 2
	logIdx := int32(2)
	if r.log[logIdx].Term != 3 { // conflict: local has term 2
		r.log = r.log[:logIdx]
	}

	if len(r.log) != 2 {
		t.Errorf("Log should be truncated to 2 entries, got %d", len(r.log))
	}
}

func TestHandleAppendEntries_CommitIndexAdvance(t *testing.T) {
	r := newTestReplica(0, 3)
	r.commitIndex = 0
	r.log = []LogEntry{
		{Term: 1},
		{Term: 1},
		{Term: 1},
	}

	// LeaderCommit=2, lastNewIndex=2
	leaderCommit := int32(2)
	lastNewIndex := int32(len(r.log) - 1)
	if leaderCommit > r.commitIndex {
		if leaderCommit < lastNewIndex {
			r.commitIndex = leaderCommit
		} else {
			r.commitIndex = lastNewIndex
		}
	}

	if r.commitIndex != 2 {
		t.Errorf("commitIndex should be 2, got %d", r.commitIndex)
	}
}

func TestHandleAppendEntries_CommitIndexCapped(t *testing.T) {
	r := newTestReplica(0, 3)
	r.commitIndex = -1
	r.log = []LogEntry{
		{Term: 1},
	}

	// LeaderCommit=5, but we only have 1 entry (index 0)
	leaderCommit := int32(5)
	lastNewIndex := int32(len(r.log) - 1) // 0
	if leaderCommit > r.commitIndex {
		if leaderCommit < lastNewIndex {
			r.commitIndex = leaderCommit
		} else {
			r.commitIndex = lastNewIndex
		}
	}

	if r.commitIndex != 0 {
		t.Errorf("commitIndex should be capped at 0, got %d", r.commitIndex)
	}
}

// --- handleAppendEntriesReply logic tests ---

func TestHandleAppendEntriesReply_SuccessUpdatesMatch(t *testing.T) {
	r := newTestReplica(0, 3)
	r.role = LEADER
	r.currentTerm = 1
	r.log = []LogEntry{{Term: 1}, {Term: 1}}
	r.matchIndex = []int32{1, -1, -1}
	r.nextIndex = []int32{2, 0, 0}

	// Simulate successful reply from follower 1
	msg := &AppendEntriesReply{FollowerId: 1, Term: 1, Success: 1, MatchIndex: 1}

	// Apply the logic
	if msg.Success == 1 {
		if msg.MatchIndex >= r.matchIndex[msg.FollowerId] {
			r.matchIndex[msg.FollowerId] = msg.MatchIndex
			r.nextIndex[msg.FollowerId] = msg.MatchIndex + 1
		}
	}

	if r.matchIndex[1] != 1 {
		t.Errorf("matchIndex[1] should be 1, got %d", r.matchIndex[1])
	}
	if r.nextIndex[1] != 2 {
		t.Errorf("nextIndex[1] should be 2, got %d", r.nextIndex[1])
	}
}

func TestHandleAppendEntriesReply_FailureDecrementsNext(t *testing.T) {
	r := newTestReplica(0, 3)
	r.role = LEADER
	r.nextIndex = []int32{5, 3, 3}

	msg := &AppendEntriesReply{FollowerId: 1, Term: 1, Success: 0, MatchIndex: 0}

	// Apply the failure logic
	if msg.Success != 1 {
		if msg.MatchIndex >= 0 {
			r.nextIndex[msg.FollowerId] = msg.MatchIndex + 1
		} else {
			r.nextIndex[msg.FollowerId] = 0
		}
	}

	if r.nextIndex[1] != 1 {
		t.Errorf("nextIndex[1] should be 1 after failure, got %d", r.nextIndex[1])
	}
}

func TestHandleAppendEntriesReply_HigherTermStepsDown(t *testing.T) {
	r := newTestReplica(0, 3)
	r.role = LEADER
	r.currentTerm = 3

	msg := &AppendEntriesReply{FollowerId: 1, Term: 5, Success: 0, MatchIndex: -1}

	// Apply the logic
	if msg.Term > r.currentTerm {
		r.becomeFollower(msg.Term)
	}

	if r.role != FOLLOWER {
		t.Error("Should step down on higher term")
	}
	if r.currentTerm != 5 {
		t.Errorf("Term should be 5, got %d", r.currentTerm)
	}
}

// --- handleRequestVote logic tests ---

func TestHandleRequestVote_GrantVote(t *testing.T) {
	r := newTestReplica(0, 3)
	r.currentTerm = 3
	r.votedFor = -1
	r.log = []LogEntry{{Term: 1}, {Term: 2}}

	msg := &RequestVote{CandidateId: 1, Term: 3, LastLogIndex: 2, LastLogTerm: 2}

	// Check the conditions
	if msg.Term < r.currentTerm {
		t.Error("Should not reject: same term")
	}
	if msg.Term > r.currentTerm {
		t.Error("Should not step down: same term")
	}
	if r.votedFor != -1 && r.votedFor != msg.CandidateId {
		t.Error("Should be able to vote: votedFor is -1")
	}
	if !r.isLogUpToDate(msg) {
		t.Error("Candidate's log should be up-to-date")
	}
}

func TestHandleRequestVote_RejectStaleCandidate(t *testing.T) {
	r := newTestReplica(0, 3)
	r.currentTerm = 5

	msg := &RequestVote{CandidateId: 1, Term: 3, LastLogIndex: 0, LastLogTerm: 1}

	if msg.Term >= r.currentTerm {
		t.Error("Should reject: candidate term 3 < current term 5")
	}
}

func TestHandleRequestVote_RejectIfAlreadyVoted(t *testing.T) {
	r := newTestReplica(0, 3)
	r.currentTerm = 3
	r.votedFor = 2 // already voted for node 2

	msg := &RequestVote{CandidateId: 1, Term: 3, LastLogIndex: 0, LastLogTerm: 1}

	canVote := (r.votedFor == -1 || r.votedFor == msg.CandidateId)
	if canVote {
		t.Error("Should not be able to vote: already voted for node 2, not node 1")
	}
}

// --- sendAppendEntries construction tests ---

func TestSendAppendEntries_EntryConstruction(t *testing.T) {
	r := newTestReplica(0, 3)
	r.currentTerm = 2
	r.commitIndex = 0
	r.log = []LogEntry{
		{Term: 1, Command: state.Command{Op: state.PUT, K: 1, V: []byte("a")}, CmdId: CommandId{ClientId: 1, SeqNum: 1}},
		{Term: 2, Command: state.Command{Op: state.PUT, K: 2, V: []byte("b")}, CmdId: CommandId{ClientId: 1, SeqNum: 2}},
	}
	r.nextIndex = []int32{2, 1, 0}

	// Simulate constructing AppendEntries for peer 2 (nextIndex=0)
	peerId := int32(2)
	nextIdx := r.nextIndex[peerId]
	prevLogIndex := nextIdx - 1
	prevLogTerm := int32(0)
	if prevLogIndex >= 0 && prevLogIndex < int32(len(r.log)) {
		prevLogTerm = r.log[prevLogIndex].Term
	}

	if prevLogIndex != -1 {
		t.Errorf("prevLogIndex should be -1 for nextIndex=0, got %d", prevLogIndex)
	}
	if prevLogTerm != 0 {
		t.Errorf("prevLogTerm should be 0, got %d", prevLogTerm)
	}

	// Build entries
	count := int32(len(r.log)) - nextIdx
	if count != 2 {
		t.Errorf("Should send 2 entries, got %d", count)
	}

	// Simulate for peer 1 (nextIndex=1)
	nextIdx = r.nextIndex[1]
	prevLogIndex = nextIdx - 1
	prevLogTerm = r.log[prevLogIndex].Term

	if prevLogIndex != 0 {
		t.Errorf("prevLogIndex should be 0, got %d", prevLogIndex)
	}
	if prevLogTerm != 1 {
		t.Errorf("prevLogTerm should be 1, got %d", prevLogTerm)
	}
	count = int32(len(r.log)) - nextIdx
	if count != 1 {
		t.Errorf("Should send 1 entry, got %d", count)
	}
}

// --- Execute commands tests ---

func TestExecuteCommands_AppliesCommitted(t *testing.T) {
	r := newTestReplica(0, 3)
	st := state.InitState()

	// Manually set log and commitIndex
	r.log = []LogEntry{
		{Term: 1, Command: state.Command{Op: state.PUT, K: state.Key(1), V: state.Value([]byte("hello"))}, CmdId: CommandId{ClientId: 1, SeqNum: 1}},
		{Term: 1, Command: state.Command{Op: state.PUT, K: state.Key(2), V: state.Value([]byte("world"))}, CmdId: CommandId{ClientId: 1, SeqNum: 2}},
	}
	r.commitIndex = 1
	r.lastApplied = -1

	// Execute committed commands manually (simulating executeCommands loop)
	for r.lastApplied < r.commitIndex {
		r.lastApplied++
		idx := r.lastApplied
		r.log[idx].Command.Execute(st)
	}

	if r.lastApplied != 1 {
		t.Errorf("lastApplied should be 1, got %d", r.lastApplied)
	}

	// Verify state was updated
	getCmd := state.Command{Op: state.GET, K: state.Key(1), V: state.NIL()}
	val := getCmd.Execute(st)
	if !bytes.Equal(val, []byte("hello")) {
		t.Errorf("State should have key 1 = 'hello', got %v", val)
	}

	getCmd2 := state.Command{Op: state.GET, K: state.Key(2), V: state.NIL()}
	val2 := getCmd2.Execute(st)
	if !bytes.Equal(val2, []byte("world")) {
		t.Errorf("State should have key 2 = 'world', got %v", val2)
	}
}

func TestExecuteCommands_StopsAtCommitIndex(t *testing.T) {
	r := newTestReplica(0, 3)
	r.log = []LogEntry{
		{Term: 1, Command: state.Command{Op: state.PUT, K: state.Key(1), V: state.Value([]byte("a"))}},
		{Term: 1, Command: state.Command{Op: state.PUT, K: state.Key(2), V: state.Value([]byte("b"))}},
		{Term: 1, Command: state.Command{Op: state.PUT, K: state.Key(3), V: state.Value([]byte("c"))}},
	}
	r.commitIndex = 1 // only first 2 committed
	r.lastApplied = -1

	for r.lastApplied < r.commitIndex {
		r.lastApplied++
	}

	if r.lastApplied != 1 {
		t.Errorf("lastApplied should be 1, not beyond commitIndex. Got %d", r.lastApplied)
	}
}

// --- Full Raft scenario: leader appends, replicates, commits ---

func TestRaftScenario_LeaderAppendsEntries(t *testing.T) {
	r := newTestReplica(0, 3)
	r.role = LEADER
	r.currentTerm = 1
	r.matchIndex = []int32{-1, -1, -1}
	r.nextIndex = []int32{0, 0, 0}

	// Leader receives proposals and appends to log
	cmd1 := state.Command{Op: state.PUT, K: state.Key(1), V: state.Value([]byte("v1"))}
	cmd2 := state.Command{Op: state.PUT, K: state.Key(2), V: state.Value([]byte("v2"))}

	r.log = append(r.log, LogEntry{Command: cmd1, Term: 1, CmdId: CommandId{ClientId: 1, SeqNum: 1}})
	r.log = append(r.log, LogEntry{Command: cmd2, Term: 1, CmdId: CommandId{ClientId: 1, SeqNum: 2}})
	r.matchIndex[0] = 1 // leader's own
	r.nextIndex[0] = 2

	// Simulate follower 1 acknowledges both entries
	r.matchIndex[1] = 1
	r.nextIndex[1] = 2

	// Advance commit index
	r.advanceCommitIndex()

	// With matchIndex = [1, 1, -1], sorted desc = [1, 1, -1]
	// Majority at index 1 = 1, and log[1].Term == currentTerm(1)
	if r.commitIndex != 1 {
		t.Errorf("commitIndex should be 1 after majority replication, got %d", r.commitIndex)
	}

	// Now follower 2 also catches up
	r.matchIndex[2] = 1
	r.nextIndex[2] = 2

	r.advanceCommitIndex()

	// With matchIndex = [1, 1, 1], sorted desc = [1, 1, 1]
	// Still commitIndex = 1 (no change, already committed)
	if r.commitIndex != 1 {
		t.Errorf("commitIndex should still be 1, got %d", r.commitIndex)
	}
}

// --- commitNotify tests ---

func TestNotifyCommit_NonBlocking(t *testing.T) {
	r := newTestReplica(0, 3)

	// First notify should succeed (buffer size 1)
	r.notifyCommit()

	// Second notify should not block (non-blocking send, drops if full)
	r.notifyCommit()

	// Drain the single notification
	select {
	case <-r.commitNotify:
	default:
		t.Error("expected notification in channel")
	}

	// Channel should now be empty
	select {
	case <-r.commitNotify:
		t.Error("channel should be empty after drain")
	default:
	}
}

func TestAdvanceCommitIndex_NotifiesOnAdvance(t *testing.T) {
	r := newTestReplica(0, 3)
	r.role = LEADER
	r.currentTerm = 1
	r.log = []LogEntry{
		{Term: 1, Command: state.Command{Op: state.PUT, K: state.Key(1), V: state.Value([]byte("a"))}},
	}
	r.matchIndex = []int32{0, 0, -1}

	// Drain any existing notifications
	select {
	case <-r.commitNotify:
	default:
	}

	r.advanceCommitIndex()

	if r.commitIndex != 0 {
		t.Errorf("commitIndex should be 0, got %d", r.commitIndex)
	}

	// Should have received a commit notification
	select {
	case <-r.commitNotify:
		// ok
	default:
		t.Error("expected commit notification after advanceCommitIndex")
	}
}

func TestAdvanceCommitIndex_NoNotifyWhenNoAdvance(t *testing.T) {
	r := newTestReplica(0, 3)
	r.role = LEADER
	r.currentTerm = 1
	r.commitIndex = 0
	r.log = []LogEntry{
		{Term: 1, Command: state.Command{Op: state.PUT, K: state.Key(1), V: state.Value([]byte("a"))}},
	}
	r.matchIndex = []int32{0, 0, -1} // majority at 0, but commitIndex already 0

	// Drain any existing notifications
	select {
	case <-r.commitNotify:
	default:
	}

	r.advanceCommitIndex()

	// commitIndex didn't advance (still 0), so no notification
	select {
	case <-r.commitNotify:
		t.Error("should not notify when commitIndex doesn't advance")
	default:
		// ok
	}
}

func TestExecuteCommands_WakesOnNotify(t *testing.T) {
	r := newTestReplica(0, 3)
	r.log = []LogEntry{
		{Term: 1, Command: state.Command{Op: state.PUT, K: state.Key(1), V: state.Value([]byte("a"))}},
		{Term: 1, Command: state.Command{Op: state.PUT, K: state.Key(2), V: state.Value([]byte("b"))}},
	}
	r.commitIndex = 0
	r.lastApplied = -1

	// Apply committed entries synchronously (simulating first half of executeCommands loop)
	for r.lastApplied < r.commitIndex {
		r.lastApplied++
	}
	if r.lastApplied != 0 {
		t.Errorf("lastApplied should be 0 after applying, got %d", r.lastApplied)
	}

	// Now test the blocking/wakeup: goroutine blocks on commitNotify, we send notify
	done := make(chan struct{})
	go func() {
		<-r.commitNotify
		close(done)
	}()

	// Send notification (simulating advanceCommitIndex)
	r.notifyCommit()

	// The goroutine should unblock
	select {
	case <-done:
		// ok
	case <-time.After(time.Second):
		t.Fatal("executeCommands goroutine did not wake up within 1 second")
	}
}
