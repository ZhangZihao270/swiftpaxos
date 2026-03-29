package raftht

import (
	"bufio"
	"encoding/binary"
	"io"
	"sync"
	"time"

	"github.com/imdea-software/swiftpaxos/replica/defs"
	fastrpc "github.com/imdea-software/swiftpaxos/rpc"
	"github.com/imdea-software/swiftpaxos/state"
)

// CommandId uniquely identifies a client command.
type CommandId struct {
	ClientId int32
	SeqNum   int32
}

func (c CommandId) String() string {
	return string(rune(c.ClientId)) + "." + string(rune(c.SeqNum))
}

// --- RequestVote ---

type RequestVote struct {
	CandidateId  int32
	Term         int32
	LastLogIndex int32
	LastLogTerm  int32
}

func (t *RequestVote) New() fastrpc.Serializable {
	return new(RequestVote)
}

func (t *RequestVote) BinarySize() (nbytes int, sizeKnown bool) {
	return 16, true
}

func (t *RequestVote) Marshal(wire io.Writer) {
	var b [16]byte
	var bs []byte
	bs = b[:16]
	tmp32 := t.CandidateId
	bs[0] = byte(tmp32)
	bs[1] = byte(tmp32 >> 8)
	bs[2] = byte(tmp32 >> 16)
	bs[3] = byte(tmp32 >> 24)
	tmp32 = t.Term
	bs[4] = byte(tmp32)
	bs[5] = byte(tmp32 >> 8)
	bs[6] = byte(tmp32 >> 16)
	bs[7] = byte(tmp32 >> 24)
	tmp32 = t.LastLogIndex
	bs[8] = byte(tmp32)
	bs[9] = byte(tmp32 >> 8)
	bs[10] = byte(tmp32 >> 16)
	bs[11] = byte(tmp32 >> 24)
	tmp32 = t.LastLogTerm
	bs[12] = byte(tmp32)
	bs[13] = byte(tmp32 >> 8)
	bs[14] = byte(tmp32 >> 16)
	bs[15] = byte(tmp32 >> 24)
	wire.Write(bs)
}

func (t *RequestVote) Unmarshal(wire io.Reader) error {
	var b [16]byte
	var bs []byte
	bs = b[:16]
	if _, err := io.ReadAtLeast(wire, bs, 16); err != nil {
		return err
	}
	t.CandidateId = int32((uint32(bs[0]) | (uint32(bs[1]) << 8) | (uint32(bs[2]) << 16) | (uint32(bs[3]) << 24)))
	t.Term = int32((uint32(bs[4]) | (uint32(bs[5]) << 8) | (uint32(bs[6]) << 16) | (uint32(bs[7]) << 24)))
	t.LastLogIndex = int32((uint32(bs[8]) | (uint32(bs[9]) << 8) | (uint32(bs[10]) << 16) | (uint32(bs[11]) << 24)))
	t.LastLogTerm = int32((uint32(bs[12]) | (uint32(bs[13]) << 8) | (uint32(bs[14]) << 16) | (uint32(bs[15]) << 24)))
	return nil
}

type RequestVoteCache struct {
	mu    sync.Mutex
	cache []*RequestVote
}

func NewRequestVoteCache() *RequestVoteCache {
	c := &RequestVoteCache{}
	c.cache = make([]*RequestVote, 0)
	return c
}

func (p *RequestVoteCache) Get() *RequestVote {
	var t *RequestVote
	p.mu.Lock()
	if len(p.cache) > 0 {
		t = p.cache[len(p.cache)-1]
		p.cache = p.cache[0:(len(p.cache) - 1)]
	}
	p.mu.Unlock()
	if t == nil {
		t = &RequestVote{}
	}
	return t
}

func (p *RequestVoteCache) Put(t *RequestVote) {
	p.mu.Lock()
	p.cache = append(p.cache, t)
	p.mu.Unlock()
}

// --- RequestVoteReply ---

type RequestVoteReply struct {
	VoterId     int32
	Term        int32
	VoteGranted int32
}

func (t *RequestVoteReply) New() fastrpc.Serializable {
	return new(RequestVoteReply)
}

func (t *RequestVoteReply) BinarySize() (nbytes int, sizeKnown bool) {
	return 12, true
}

func (t *RequestVoteReply) Marshal(wire io.Writer) {
	var b [12]byte
	var bs []byte
	bs = b[:12]
	tmp32 := t.VoterId
	bs[0] = byte(tmp32)
	bs[1] = byte(tmp32 >> 8)
	bs[2] = byte(tmp32 >> 16)
	bs[3] = byte(tmp32 >> 24)
	tmp32 = t.Term
	bs[4] = byte(tmp32)
	bs[5] = byte(tmp32 >> 8)
	bs[6] = byte(tmp32 >> 16)
	bs[7] = byte(tmp32 >> 24)
	tmp32 = t.VoteGranted
	bs[8] = byte(tmp32)
	bs[9] = byte(tmp32 >> 8)
	bs[10] = byte(tmp32 >> 16)
	bs[11] = byte(tmp32 >> 24)
	wire.Write(bs)
}

func (t *RequestVoteReply) Unmarshal(wire io.Reader) error {
	var b [12]byte
	var bs []byte
	bs = b[:12]
	if _, err := io.ReadAtLeast(wire, bs, 12); err != nil {
		return err
	}
	t.VoterId = int32((uint32(bs[0]) | (uint32(bs[1]) << 8) | (uint32(bs[2]) << 16) | (uint32(bs[3]) << 24)))
	t.Term = int32((uint32(bs[4]) | (uint32(bs[5]) << 8) | (uint32(bs[6]) << 16) | (uint32(bs[7]) << 24)))
	t.VoteGranted = int32((uint32(bs[8]) | (uint32(bs[9]) << 8) | (uint32(bs[10]) << 16) | (uint32(bs[11]) << 24)))
	return nil
}

type RequestVoteReplyCache struct {
	mu    sync.Mutex
	cache []*RequestVoteReply
}

func NewRequestVoteReplyCache() *RequestVoteReplyCache {
	c := &RequestVoteReplyCache{}
	c.cache = make([]*RequestVoteReply, 0)
	return c
}

func (p *RequestVoteReplyCache) Get() *RequestVoteReply {
	var t *RequestVoteReply
	p.mu.Lock()
	if len(p.cache) > 0 {
		t = p.cache[len(p.cache)-1]
		p.cache = p.cache[0:(len(p.cache) - 1)]
	}
	p.mu.Unlock()
	if t == nil {
		t = &RequestVoteReply{}
	}
	return t
}

func (p *RequestVoteReplyCache) Put(t *RequestVoteReply) {
	p.mu.Lock()
	p.cache = append(p.cache, t)
	p.mu.Unlock()
}

// --- AppendEntries ---

type AppendEntries struct {
	LeaderId     int32
	Term         int32
	PrevLogIndex int32
	PrevLogTerm  int32
	LeaderCommit int32
	EntryCnt     int32
	Entries      []state.Command
	EntryIds     []CommandId
}

func (t *AppendEntries) New() fastrpc.Serializable {
	return new(AppendEntries)
}

func (t *AppendEntries) BinarySize() (nbytes int, sizeKnown bool) {
	return 0, false
}

func (t *AppendEntries) Marshal(wire io.Writer) {
	var b [24]byte
	var bs []byte
	bs = b[:24]
	tmp32 := t.LeaderId
	bs[0] = byte(tmp32)
	bs[1] = byte(tmp32 >> 8)
	bs[2] = byte(tmp32 >> 16)
	bs[3] = byte(tmp32 >> 24)
	tmp32 = t.Term
	bs[4] = byte(tmp32)
	bs[5] = byte(tmp32 >> 8)
	bs[6] = byte(tmp32 >> 16)
	bs[7] = byte(tmp32 >> 24)
	tmp32 = t.PrevLogIndex
	bs[8] = byte(tmp32)
	bs[9] = byte(tmp32 >> 8)
	bs[10] = byte(tmp32 >> 16)
	bs[11] = byte(tmp32 >> 24)
	tmp32 = t.PrevLogTerm
	bs[12] = byte(tmp32)
	bs[13] = byte(tmp32 >> 8)
	bs[14] = byte(tmp32 >> 16)
	bs[15] = byte(tmp32 >> 24)
	tmp32 = t.LeaderCommit
	bs[16] = byte(tmp32)
	bs[17] = byte(tmp32 >> 8)
	bs[18] = byte(tmp32 >> 16)
	bs[19] = byte(tmp32 >> 24)
	tmp32 = t.EntryCnt
	bs[20] = byte(tmp32)
	bs[21] = byte(tmp32 >> 8)
	bs[22] = byte(tmp32 >> 16)
	bs[23] = byte(tmp32 >> 24)
	wire.Write(bs)

	var vb [10]byte
	alen := int64(len(t.Entries))
	wlen := binary.PutVarint(vb[:], alen)
	wire.Write(vb[:wlen])
	for i := int64(0); i < alen; i++ {
		t.Entries[i].Marshal(wire)
	}

	alen2 := int64(len(t.EntryIds))
	wlen = binary.PutVarint(vb[:], alen2)
	wire.Write(vb[:wlen])
	for i := int64(0); i < alen2; i++ {
		var ib [8]byte
		tmp32 = t.EntryIds[i].ClientId
		ib[0] = byte(tmp32)
		ib[1] = byte(tmp32 >> 8)
		ib[2] = byte(tmp32 >> 16)
		ib[3] = byte(tmp32 >> 24)
		tmp32 = t.EntryIds[i].SeqNum
		ib[4] = byte(tmp32)
		ib[5] = byte(tmp32 >> 8)
		ib[6] = byte(tmp32 >> 16)
		ib[7] = byte(tmp32 >> 24)
		wire.Write(ib[:])
	}
}

func (t *AppendEntries) Unmarshal(rr io.Reader) error {
	var b [24]byte
	var bs []byte
	bs = b[:24]
	if _, err := io.ReadAtLeast(rr, bs, 24); err != nil {
		return err
	}
	t.LeaderId = int32((uint32(bs[0]) | (uint32(bs[1]) << 8) | (uint32(bs[2]) << 16) | (uint32(bs[3]) << 24)))
	t.Term = int32((uint32(bs[4]) | (uint32(bs[5]) << 8) | (uint32(bs[6]) << 16) | (uint32(bs[7]) << 24)))
	t.PrevLogIndex = int32((uint32(bs[8]) | (uint32(bs[9]) << 8) | (uint32(bs[10]) << 16) | (uint32(bs[11]) << 24)))
	t.PrevLogTerm = int32((uint32(bs[12]) | (uint32(bs[13]) << 8) | (uint32(bs[14]) << 16) | (uint32(bs[15]) << 24)))
	t.LeaderCommit = int32((uint32(bs[16]) | (uint32(bs[17]) << 8) | (uint32(bs[18]) << 16) | (uint32(bs[19]) << 24)))
	t.EntryCnt = int32((uint32(bs[20]) | (uint32(bs[21]) << 8) | (uint32(bs[22]) << 16) | (uint32(bs[23]) << 24)))

	var wire byteReader
	var ok bool
	if wire, ok = rr.(byteReader); !ok {
		wire = bufio.NewReader(rr)
	}
	alen, err := binary.ReadVarint(wire)
	if err != nil {
		return err
	}
	t.Entries = make([]state.Command, alen)
	for i := int64(0); i < alen; i++ {
		if err := t.Entries[i].Unmarshal(wire); err != nil {
			return err
		}
	}

	alen2, err := binary.ReadVarint(wire)
	if err != nil {
		return err
	}
	t.EntryIds = make([]CommandId, alen2)
	for i := int64(0); i < alen2; i++ {
		var ib [8]byte
		if _, err := io.ReadAtLeast(wire, ib[:], 8); err != nil {
			return err
		}
		t.EntryIds[i].ClientId = int32((uint32(ib[0]) | (uint32(ib[1]) << 8) | (uint32(ib[2]) << 16) | (uint32(ib[3]) << 24)))
		t.EntryIds[i].SeqNum = int32((uint32(ib[4]) | (uint32(ib[5]) << 8) | (uint32(ib[6]) << 16) | (uint32(ib[7]) << 24)))
	}
	return nil
}

type AppendEntriesCache struct {
	mu    sync.Mutex
	cache []*AppendEntries
}

func NewAppendEntriesCache() *AppendEntriesCache {
	c := &AppendEntriesCache{}
	c.cache = make([]*AppendEntries, 0)
	return c
}

func (p *AppendEntriesCache) Get() *AppendEntries {
	var t *AppendEntries
	p.mu.Lock()
	if len(p.cache) > 0 {
		t = p.cache[len(p.cache)-1]
		p.cache = p.cache[0:(len(p.cache) - 1)]
	}
	p.mu.Unlock()
	if t == nil {
		t = &AppendEntries{}
	}
	return t
}

func (p *AppendEntriesCache) Put(t *AppendEntries) {
	p.mu.Lock()
	p.cache = append(p.cache, t)
	p.mu.Unlock()
}

// --- AppendEntriesReply ---

type AppendEntriesReply struct {
	FollowerId int32
	Term       int32
	Success    int32
	MatchIndex int32
}

func (t *AppendEntriesReply) New() fastrpc.Serializable {
	return new(AppendEntriesReply)
}

func (t *AppendEntriesReply) BinarySize() (nbytes int, sizeKnown bool) {
	return 16, true
}

func (t *AppendEntriesReply) Marshal(wire io.Writer) {
	var b [16]byte
	var bs []byte
	bs = b[:16]
	tmp32 := t.FollowerId
	bs[0] = byte(tmp32)
	bs[1] = byte(tmp32 >> 8)
	bs[2] = byte(tmp32 >> 16)
	bs[3] = byte(tmp32 >> 24)
	tmp32 = t.Term
	bs[4] = byte(tmp32)
	bs[5] = byte(tmp32 >> 8)
	bs[6] = byte(tmp32 >> 16)
	bs[7] = byte(tmp32 >> 24)
	tmp32 = t.Success
	bs[8] = byte(tmp32)
	bs[9] = byte(tmp32 >> 8)
	bs[10] = byte(tmp32 >> 16)
	bs[11] = byte(tmp32 >> 24)
	tmp32 = t.MatchIndex
	bs[12] = byte(tmp32)
	bs[13] = byte(tmp32 >> 8)
	bs[14] = byte(tmp32 >> 16)
	bs[15] = byte(tmp32 >> 24)
	wire.Write(bs)
}

func (t *AppendEntriesReply) Unmarshal(wire io.Reader) error {
	var b [16]byte
	var bs []byte
	bs = b[:16]
	if _, err := io.ReadAtLeast(wire, bs, 16); err != nil {
		return err
	}
	t.FollowerId = int32((uint32(bs[0]) | (uint32(bs[1]) << 8) | (uint32(bs[2]) << 16) | (uint32(bs[3]) << 24)))
	t.Term = int32((uint32(bs[4]) | (uint32(bs[5]) << 8) | (uint32(bs[6]) << 16) | (uint32(bs[7]) << 24)))
	t.Success = int32((uint32(bs[8]) | (uint32(bs[9]) << 8) | (uint32(bs[10]) << 16) | (uint32(bs[11]) << 24)))
	t.MatchIndex = int32((uint32(bs[12]) | (uint32(bs[13]) << 8) | (uint32(bs[14]) << 16) | (uint32(bs[15]) << 24)))
	return nil
}

type AppendEntriesReplyCache struct {
	mu    sync.Mutex
	cache []*AppendEntriesReply
}

func NewAppendEntriesReplyCache() *AppendEntriesReplyCache {
	c := &AppendEntriesReplyCache{}
	c.cache = make([]*AppendEntriesReply, 0)
	return c
}

func (p *AppendEntriesReplyCache) Get() *AppendEntriesReply {
	var t *AppendEntriesReply
	p.mu.Lock()
	if len(p.cache) > 0 {
		t = p.cache[len(p.cache)-1]
		p.cache = p.cache[0:(len(p.cache) - 1)]
	}
	p.mu.Unlock()
	if t == nil {
		t = &AppendEntriesReply{}
	}
	return t
}

func (p *AppendEntriesReplyCache) Put(t *AppendEntriesReply) {
	p.mu.Lock()
	p.cache = append(p.cache, t)
	p.mu.Unlock()
}

// --- RaftReply ---

type RaftReply struct {
	CmdId    CommandId
	Value    []byte
	LeaderId int32 // -1 = unknown, >=0 = leader hint for client failover
}

func (t *RaftReply) New() fastrpc.Serializable {
	return new(RaftReply)
}

func (t *RaftReply) BinarySize() (nbytes int, sizeKnown bool) {
	return 0, false
}

func (t *RaftReply) Marshal(wire io.Writer) {
	var b [8]byte
	var bs []byte
	bs = b[:8]
	tmp32 := t.CmdId.ClientId
	bs[0] = byte(tmp32)
	bs[1] = byte(tmp32 >> 8)
	bs[2] = byte(tmp32 >> 16)
	bs[3] = byte(tmp32 >> 24)
	tmp32 = t.CmdId.SeqNum
	bs[4] = byte(tmp32)
	bs[5] = byte(tmp32 >> 8)
	bs[6] = byte(tmp32 >> 16)
	bs[7] = byte(tmp32 >> 24)
	wire.Write(bs)

	var vb [10]byte
	alen := int64(len(t.Value))
	wlen := binary.PutVarint(vb[:], alen)
	wire.Write(vb[:wlen])
	if alen > 0 {
		wire.Write(t.Value)
	}
	bs = b[:4]
	tmp32 = t.LeaderId
	bs[0] = byte(tmp32)
	bs[1] = byte(tmp32 >> 8)
	bs[2] = byte(tmp32 >> 16)
	bs[3] = byte(tmp32 >> 24)
	wire.Write(bs)
}

func (t *RaftReply) Unmarshal(rr io.Reader) error {
	var b [8]byte
	var bs []byte
	bs = b[:8]
	if _, err := io.ReadAtLeast(rr, bs, 8); err != nil {
		return err
	}
	t.CmdId.ClientId = int32((uint32(bs[0]) | (uint32(bs[1]) << 8) | (uint32(bs[2]) << 16) | (uint32(bs[3]) << 24)))
	t.CmdId.SeqNum = int32((uint32(bs[4]) | (uint32(bs[5]) << 8) | (uint32(bs[6]) << 16) | (uint32(bs[7]) << 24)))

	var wire byteReader
	var ok bool
	if wire, ok = rr.(byteReader); !ok {
		wire = bufio.NewReader(rr)
	}
	alen, err := binary.ReadVarint(wire)
	if err != nil {
		return err
	}
	t.Value = make([]byte, alen)
	if alen > 0 {
		if _, err := io.ReadFull(wire, t.Value); err != nil {
			return err
		}
	}
	bs = b[:4]
	if _, err := io.ReadAtLeast(wire, bs, 4); err != nil {
		return err
	}
	t.LeaderId = int32((uint32(bs[0]) | (uint32(bs[1]) << 8) | (uint32(bs[2]) << 16) | (uint32(bs[3]) << 24)))
	return nil
}

type RaftReplyCache struct {
	mu    sync.Mutex
	cache []*RaftReply
}

func NewRaftReplyCache() *RaftReplyCache {
	c := &RaftReplyCache{}
	c.cache = make([]*RaftReply, 0)
	return c
}

func (p *RaftReplyCache) Get() *RaftReply {
	var t *RaftReply
	p.mu.Lock()
	if len(p.cache) > 0 {
		t = p.cache[len(p.cache)-1]
		p.cache = p.cache[0:(len(p.cache) - 1)]
	}
	p.mu.Unlock()
	if t == nil {
		t = &RaftReply{}
	}
	return t
}

func (p *RaftReplyCache) Put(t *RaftReply) {
	p.mu.Lock()
	p.cache = append(p.cache, t)
	p.mu.Unlock()
}

// ============================================================================
// Raft-HT: Weak message types
// ============================================================================

// --- MWeakPropose ---
// Client → Leader: weak write request.
// Fixed size: 2 x int32 (8 bytes) + Command (variable).

type MWeakPropose struct {
	CommandId int32
	ClientId  int32
	Command   state.Command
}

func (t *MWeakPropose) New() fastrpc.Serializable {
	return new(MWeakPropose)
}

func (t *MWeakPropose) BinarySize() (nbytes int, sizeKnown bool) {
	return 0, false // variable due to Command
}

func (t *MWeakPropose) Marshal(wire io.Writer) {
	var b [8]byte
	bs := b[:8]
	tmp32 := t.CommandId
	bs[0] = byte(tmp32)
	bs[1] = byte(tmp32 >> 8)
	bs[2] = byte(tmp32 >> 16)
	bs[3] = byte(tmp32 >> 24)
	tmp32 = t.ClientId
	bs[4] = byte(tmp32)
	bs[5] = byte(tmp32 >> 8)
	bs[6] = byte(tmp32 >> 16)
	bs[7] = byte(tmp32 >> 24)
	wire.Write(bs)
	t.Command.Marshal(wire)
}

func (t *MWeakPropose) Unmarshal(rr io.Reader) error {
	var b [8]byte
	bs := b[:8]
	if _, err := io.ReadAtLeast(rr, bs, 8); err != nil {
		return err
	}
	t.CommandId = int32((uint32(bs[0]) | (uint32(bs[1]) << 8) | (uint32(bs[2]) << 16) | (uint32(bs[3]) << 24)))
	t.ClientId = int32((uint32(bs[4]) | (uint32(bs[5]) << 8) | (uint32(bs[6]) << 16) | (uint32(bs[7]) << 24)))
	if err := t.Command.Unmarshal(rr); err != nil {
		return err
	}
	return nil
}

func (t *MWeakPropose) GetClientId() int32 {
	return t.ClientId
}

type MWeakProposeCache struct {
	mu    sync.Mutex
	cache []*MWeakPropose
}

func NewMWeakProposeCache() *MWeakProposeCache {
	return &MWeakProposeCache{cache: make([]*MWeakPropose, 0)}
}

func (p *MWeakProposeCache) Get() *MWeakPropose {
	var t *MWeakPropose
	p.mu.Lock()
	if len(p.cache) > 0 {
		t = p.cache[len(p.cache)-1]
		p.cache = p.cache[0:(len(p.cache) - 1)]
	}
	p.mu.Unlock()
	if t == nil {
		t = &MWeakPropose{}
	}
	return t
}

func (p *MWeakProposeCache) Put(t *MWeakPropose) {
	p.mu.Lock()
	p.cache = append(p.cache, t)
	p.mu.Unlock()
}

// --- MWeakReply ---
// Leader → Client: immediate reply for weak write (before replication).
// Fixed size: 4 x int32 = 16 bytes.

type MWeakReply struct {
	LeaderId int32
	Term     int32
	CmdId    CommandId // ClientId + SeqNum
	Slot     int32     // Log index (version for client cache)
}

func (t *MWeakReply) New() fastrpc.Serializable {
	return new(MWeakReply)
}

func (t *MWeakReply) BinarySize() (nbytes int, sizeKnown bool) {
	return 20, true // 5 x int32
}

func (t *MWeakReply) Marshal(wire io.Writer) {
	var b [20]byte
	bs := b[:20]
	tmp32 := t.LeaderId
	bs[0] = byte(tmp32)
	bs[1] = byte(tmp32 >> 8)
	bs[2] = byte(tmp32 >> 16)
	bs[3] = byte(tmp32 >> 24)
	tmp32 = t.Term
	bs[4] = byte(tmp32)
	bs[5] = byte(tmp32 >> 8)
	bs[6] = byte(tmp32 >> 16)
	bs[7] = byte(tmp32 >> 24)
	tmp32 = t.CmdId.ClientId
	bs[8] = byte(tmp32)
	bs[9] = byte(tmp32 >> 8)
	bs[10] = byte(tmp32 >> 16)
	bs[11] = byte(tmp32 >> 24)
	tmp32 = t.CmdId.SeqNum
	bs[12] = byte(tmp32)
	bs[13] = byte(tmp32 >> 8)
	bs[14] = byte(tmp32 >> 16)
	bs[15] = byte(tmp32 >> 24)
	tmp32 = t.Slot
	bs[16] = byte(tmp32)
	bs[17] = byte(tmp32 >> 8)
	bs[18] = byte(tmp32 >> 16)
	bs[19] = byte(tmp32 >> 24)
	wire.Write(bs)
}

func (t *MWeakReply) Unmarshal(wire io.Reader) error {
	var b [20]byte
	bs := b[:20]
	if _, err := io.ReadAtLeast(wire, bs, 20); err != nil {
		return err
	}
	t.LeaderId = int32((uint32(bs[0]) | (uint32(bs[1]) << 8) | (uint32(bs[2]) << 16) | (uint32(bs[3]) << 24)))
	t.Term = int32((uint32(bs[4]) | (uint32(bs[5]) << 8) | (uint32(bs[6]) << 16) | (uint32(bs[7]) << 24)))
	t.CmdId.ClientId = int32((uint32(bs[8]) | (uint32(bs[9]) << 8) | (uint32(bs[10]) << 16) | (uint32(bs[11]) << 24)))
	t.CmdId.SeqNum = int32((uint32(bs[12]) | (uint32(bs[13]) << 8) | (uint32(bs[14]) << 16) | (uint32(bs[15]) << 24)))
	t.Slot = int32((uint32(bs[16]) | (uint32(bs[17]) << 8) | (uint32(bs[18]) << 16) | (uint32(bs[19]) << 24)))
	return nil
}

type MWeakReplyCache struct {
	mu    sync.Mutex
	cache []*MWeakReply
}

func NewMWeakReplyCache() *MWeakReplyCache {
	return &MWeakReplyCache{cache: make([]*MWeakReply, 0)}
}

func (p *MWeakReplyCache) Get() *MWeakReply {
	var t *MWeakReply
	p.mu.Lock()
	if len(p.cache) > 0 {
		t = p.cache[len(p.cache)-1]
		p.cache = p.cache[0:(len(p.cache) - 1)]
	}
	p.mu.Unlock()
	if t == nil {
		t = &MWeakReply{}
	}
	return t
}

func (p *MWeakReplyCache) Put(t *MWeakReply) {
	p.mu.Lock()
	p.cache = append(p.cache, t)
	p.mu.Unlock()
}

// --- MWeakRead ---
// Client → Any Replica: weak read request.
// Fixed size: 2 x int32 + Key = variable.

type MWeakRead struct {
	CommandId int32
	ClientId  int32
	Key       state.Key
	MinIndex  int32 // Minimum log index that must be applied before serving this read (causal tracking)
	Op        uint8 // 0 or state.GET = single-key read, state.SCAN = range scan
	Count     int64 // number of keys to scan (only used when Op == SCAN)
}

func (t *MWeakRead) New() fastrpc.Serializable {
	return new(MWeakRead)
}

func (t *MWeakRead) BinarySize() (nbytes int, sizeKnown bool) {
	return 29, true // 3 x int32 + Key (int64) + Op (uint8) + Count (int64)
}

func (t *MWeakRead) Marshal(wire io.Writer) {
	var b [29]byte
	bs := b[:29]
	tmp32 := t.CommandId
	bs[0] = byte(tmp32)
	bs[1] = byte(tmp32 >> 8)
	bs[2] = byte(tmp32 >> 16)
	bs[3] = byte(tmp32 >> 24)
	tmp32 = t.ClientId
	bs[4] = byte(tmp32)
	bs[5] = byte(tmp32 >> 8)
	bs[6] = byte(tmp32 >> 16)
	bs[7] = byte(tmp32 >> 24)
	tmp64 := int64(t.Key)
	bs[8] = byte(tmp64)
	bs[9] = byte(tmp64 >> 8)
	bs[10] = byte(tmp64 >> 16)
	bs[11] = byte(tmp64 >> 24)
	bs[12] = byte(tmp64 >> 32)
	bs[13] = byte(tmp64 >> 40)
	bs[14] = byte(tmp64 >> 48)
	bs[15] = byte(tmp64 >> 56)
	tmp32 = t.MinIndex
	bs[16] = byte(tmp32)
	bs[17] = byte(tmp32 >> 8)
	bs[18] = byte(tmp32 >> 16)
	bs[19] = byte(tmp32 >> 24)
	bs[20] = byte(t.Op)
	tmp64 = t.Count
	bs[21] = byte(tmp64)
	bs[22] = byte(tmp64 >> 8)
	bs[23] = byte(tmp64 >> 16)
	bs[24] = byte(tmp64 >> 24)
	bs[25] = byte(tmp64 >> 32)
	bs[26] = byte(tmp64 >> 40)
	bs[27] = byte(tmp64 >> 48)
	bs[28] = byte(tmp64 >> 56)
	wire.Write(bs)
}

func (t *MWeakRead) Unmarshal(wire io.Reader) error {
	var b [29]byte
	bs := b[:29]
	if _, err := io.ReadAtLeast(wire, bs, 29); err != nil {
		return err
	}
	t.CommandId = int32((uint32(bs[0]) | (uint32(bs[1]) << 8) | (uint32(bs[2]) << 16) | (uint32(bs[3]) << 24)))
	t.ClientId = int32((uint32(bs[4]) | (uint32(bs[5]) << 8) | (uint32(bs[6]) << 16) | (uint32(bs[7]) << 24)))
	t.Key = state.Key(int64(uint64(bs[8]) | (uint64(bs[9]) << 8) | (uint64(bs[10]) << 16) | (uint64(bs[11]) << 24) |
		(uint64(bs[12]) << 32) | (uint64(bs[13]) << 40) | (uint64(bs[14]) << 48) | (uint64(bs[15]) << 56)))
	t.MinIndex = int32((uint32(bs[16]) | (uint32(bs[17]) << 8) | (uint32(bs[18]) << 16) | (uint32(bs[19]) << 24)))
	t.Op = uint8(bs[20])
	t.Count = int64(uint64(bs[21]) | (uint64(bs[22]) << 8) | (uint64(bs[23]) << 16) | (uint64(bs[24]) << 24) | (uint64(bs[25]) << 32) | (uint64(bs[26]) << 40) | (uint64(bs[27]) << 48) | (uint64(bs[28]) << 56))
	return nil
}

func (t *MWeakRead) GetClientId() int32 {
	return t.ClientId
}

type MWeakReadCache struct {
	mu    sync.Mutex
	cache []*MWeakRead
}

func NewMWeakReadCache() *MWeakReadCache {
	return &MWeakReadCache{cache: make([]*MWeakRead, 0)}
}

func (p *MWeakReadCache) Get() *MWeakRead {
	var t *MWeakRead
	p.mu.Lock()
	if len(p.cache) > 0 {
		t = p.cache[len(p.cache)-1]
		p.cache = p.cache[0:(len(p.cache) - 1)]
	}
	p.mu.Unlock()
	if t == nil {
		t = &MWeakRead{}
	}
	return t
}

func (p *MWeakReadCache) Put(t *MWeakRead) {
	p.mu.Lock()
	p.cache = append(p.cache, t)
	p.mu.Unlock()
}

// --- MWeakReadReply ---
// Any Replica → Client: weak read response with value and version.
// Variable size: fixed header (4 x int32 = 16 bytes) + varint-prefixed Value.

type MWeakReadReply struct {
	Replica int32
	Term    int32
	CmdId   CommandId // ClientId + SeqNum
	Rep     []byte    // Value
	Version int32     // Log index of last committed write to this key
}

func (t *MWeakReadReply) New() fastrpc.Serializable {
	return new(MWeakReadReply)
}

func (t *MWeakReadReply) BinarySize() (nbytes int, sizeKnown bool) {
	return 0, false // variable due to Rep
}

func (t *MWeakReadReply) Marshal(wire io.Writer) {
	var b [20]byte
	bs := b[:20]
	tmp32 := t.Replica
	bs[0] = byte(tmp32)
	bs[1] = byte(tmp32 >> 8)
	bs[2] = byte(tmp32 >> 16)
	bs[3] = byte(tmp32 >> 24)
	tmp32 = t.Term
	bs[4] = byte(tmp32)
	bs[5] = byte(tmp32 >> 8)
	bs[6] = byte(tmp32 >> 16)
	bs[7] = byte(tmp32 >> 24)
	tmp32 = t.CmdId.ClientId
	bs[8] = byte(tmp32)
	bs[9] = byte(tmp32 >> 8)
	bs[10] = byte(tmp32 >> 16)
	bs[11] = byte(tmp32 >> 24)
	tmp32 = t.CmdId.SeqNum
	bs[12] = byte(tmp32)
	bs[13] = byte(tmp32 >> 8)
	bs[14] = byte(tmp32 >> 16)
	bs[15] = byte(tmp32 >> 24)
	tmp32 = t.Version
	bs[16] = byte(tmp32)
	bs[17] = byte(tmp32 >> 8)
	bs[18] = byte(tmp32 >> 16)
	bs[19] = byte(tmp32 >> 24)
	wire.Write(bs)

	var vb [10]byte
	alen := int64(len(t.Rep))
	wlen := binary.PutVarint(vb[:], alen)
	wire.Write(vb[:wlen])
	if alen > 0 {
		wire.Write(t.Rep)
	}
}

func (t *MWeakReadReply) Unmarshal(rr io.Reader) error {
	var b [20]byte
	bs := b[:20]
	if _, err := io.ReadAtLeast(rr, bs, 20); err != nil {
		return err
	}
	t.Replica = int32((uint32(bs[0]) | (uint32(bs[1]) << 8) | (uint32(bs[2]) << 16) | (uint32(bs[3]) << 24)))
	t.Term = int32((uint32(bs[4]) | (uint32(bs[5]) << 8) | (uint32(bs[6]) << 16) | (uint32(bs[7]) << 24)))
	t.CmdId.ClientId = int32((uint32(bs[8]) | (uint32(bs[9]) << 8) | (uint32(bs[10]) << 16) | (uint32(bs[11]) << 24)))
	t.CmdId.SeqNum = int32((uint32(bs[12]) | (uint32(bs[13]) << 8) | (uint32(bs[14]) << 16) | (uint32(bs[15]) << 24)))
	t.Version = int32((uint32(bs[16]) | (uint32(bs[17]) << 8) | (uint32(bs[18]) << 16) | (uint32(bs[19]) << 24)))

	var wire byteReader
	var ok bool
	if wire, ok = rr.(byteReader); !ok {
		wire = bufio.NewReader(rr)
	}
	alen, err := binary.ReadVarint(wire)
	if err != nil {
		return err
	}
	t.Rep = make([]byte, alen)
	if alen > 0 {
		if _, err := io.ReadFull(wire, t.Rep); err != nil {
			return err
		}
	}
	return nil
}

type MWeakReadReplyCache struct {
	mu    sync.Mutex
	cache []*MWeakReadReply
}

func NewMWeakReadReplyCache() *MWeakReadReplyCache {
	return &MWeakReadReplyCache{cache: make([]*MWeakReadReply, 0)}
}

func (p *MWeakReadReplyCache) Get() *MWeakReadReply {
	var t *MWeakReadReply
	p.mu.Lock()
	if len(p.cache) > 0 {
		t = p.cache[len(p.cache)-1]
		p.cache = p.cache[0:(len(p.cache) - 1)]
	}
	p.mu.Unlock()
	if t == nil {
		t = &MWeakReadReply{}
	}
	return t
}

func (p *MWeakReadReplyCache) Put(t *MWeakReadReply) {
	p.mu.Lock()
	p.cache = append(p.cache, t)
	p.mu.Unlock()
}

// --- byteReader interface ---

type byteReader interface {
	io.Reader
	ReadByte() (c byte, err error)
}

// --- CommunicationSupply ---

type CommunicationSupply struct {
	MaxLatency time.Duration

	AppendEntriesChan      chan fastrpc.Serializable
	AppendEntriesReplyChan chan fastrpc.Serializable
	RequestVoteChan        chan fastrpc.Serializable
	RequestVoteReplyChan   chan fastrpc.Serializable
	RaftReplyChan          chan fastrpc.Serializable

	// Raft-HT weak channels
	WeakProposeChan   chan fastrpc.Serializable
	WeakReplyChan     chan fastrpc.Serializable
	WeakReadChan      chan fastrpc.Serializable
	WeakReadReplyChan chan fastrpc.Serializable

	AppendEntriesRPC      uint8
	AppendEntriesReplyRPC uint8
	RequestVoteRPC        uint8
	RequestVoteReplyRPC   uint8
	RaftReplyRPC          uint8

	// Raft-HT weak RPCs
	WeakProposeRPC   uint8
	WeakReplyRPC     uint8
	WeakReadRPC      uint8
	WeakReadReplyRPC uint8
}

// InitClientCs initializes a CommunicationSupply for client-side use.
// Registers only the reply types that clients receive (RaftReply, MWeakReply,
// MWeakReadReply) and the request types that clients send (MWeakPropose, MWeakRead).
func InitClientCs(cs *CommunicationSupply, t *fastrpc.Table) {
	initCs(cs, t)
}

func initCs(cs *CommunicationSupply, t *fastrpc.Table) {
	cs.MaxLatency = 0

	cs.AppendEntriesChan = make(chan fastrpc.Serializable, defs.CHAN_BUFFER_SIZE)
	cs.AppendEntriesReplyChan = make(chan fastrpc.Serializable, defs.CHAN_BUFFER_SIZE)
	cs.RequestVoteChan = make(chan fastrpc.Serializable, defs.CHAN_BUFFER_SIZE)
	cs.RequestVoteReplyChan = make(chan fastrpc.Serializable, defs.CHAN_BUFFER_SIZE)
	cs.RaftReplyChan = make(chan fastrpc.Serializable, defs.CHAN_BUFFER_SIZE)

	cs.WeakProposeChan = make(chan fastrpc.Serializable, defs.CHAN_BUFFER_SIZE)
	cs.WeakReplyChan = make(chan fastrpc.Serializable, defs.CHAN_BUFFER_SIZE)
	cs.WeakReadChan = make(chan fastrpc.Serializable, defs.CHAN_BUFFER_SIZE)
	cs.WeakReadReplyChan = make(chan fastrpc.Serializable, defs.CHAN_BUFFER_SIZE)

	cs.AppendEntriesRPC = t.Register(new(AppendEntries), cs.AppendEntriesChan)
	cs.AppendEntriesReplyRPC = t.Register(new(AppendEntriesReply), cs.AppendEntriesReplyChan)
	cs.RequestVoteRPC = t.Register(new(RequestVote), cs.RequestVoteChan)
	cs.RequestVoteReplyRPC = t.Register(new(RequestVoteReply), cs.RequestVoteReplyChan)
	cs.RaftReplyRPC = t.Register(new(RaftReply), cs.RaftReplyChan)

	cs.WeakProposeRPC = t.Register(new(MWeakPropose), cs.WeakProposeChan)
	cs.WeakReplyRPC = t.Register(new(MWeakReply), cs.WeakReplyChan)
	cs.WeakReadRPC = t.Register(new(MWeakRead), cs.WeakReadChan)
	cs.WeakReadReplyRPC = t.Register(new(MWeakReadReply), cs.WeakReadReplyChan)
}
