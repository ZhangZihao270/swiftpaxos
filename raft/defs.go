package raft

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
// Sent by candidate to request votes during leader election.
// Fixed size: 4 x int32 = 16 bytes.

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
// Sent by voter in response to RequestVote.
// Fixed size: 3 x int32 = 12 bytes. VoteGranted encoded as int32 (0 or 1).

type RequestVoteReply struct {
	VoterId     int32
	Term        int32
	VoteGranted int32 // 0 = false, 1 = true (encoded as int32 for wire consistency)
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
// Sent by leader for log replication and heartbeats.
// Variable size: fixed header (6 x int32 = 24 bytes) + varint-prefixed Entries + varint-prefixed EntryIds.

type AppendEntries struct {
	LeaderId     int32
	Term         int32
	PrevLogIndex int32
	PrevLogTerm  int32
	LeaderCommit int32
	EntryCnt     int32           // number of entries (redundant with len(Entries), for fixed header)
	Entries      []state.Command // log entries
	EntryIds     []CommandId     // corresponding client command IDs
}

func (t *AppendEntries) New() fastrpc.Serializable {
	return new(AppendEntries)
}

func (t *AppendEntries) BinarySize() (nbytes int, sizeKnown bool) {
	return 0, false // variable length
}

func (t *AppendEntries) Marshal(wire io.Writer) {
	// Fixed header: 6 x int32 = 24 bytes
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

	// Variable-length Entries: varint length + each Command
	var vb [10]byte
	alen := int64(len(t.Entries))
	wlen := binary.PutVarint(vb[:], alen)
	wire.Write(vb[:wlen])
	for i := int64(0); i < alen; i++ {
		t.Entries[i].Marshal(wire)
	}

	// Variable-length EntryIds: varint length + each (ClientId, SeqNum) inline
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
	// Fixed header
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

	// Variable-length Entries
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

	// Variable-length EntryIds
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
// Sent by follower in response to AppendEntries.
// Fixed size: 4 x int32 = 16 bytes. Success encoded as int32 (0 or 1).

type AppendEntriesReply struct {
	FollowerId int32
	Term       int32
	Success    int32 // 0 = false, 1 = true
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
// Sent by leader to client after command is committed and executed.
// Variable size: fixed header (2 x int32 = 8 bytes for CmdId) + varint-prefixed Value.

type RaftReply struct {
	CmdId CommandId
	Value []byte
}

func (t *RaftReply) New() fastrpc.Serializable {
	return new(RaftReply)
}

func (t *RaftReply) BinarySize() (nbytes int, sizeKnown bool) {
	return 0, false // variable length due to Value
}

func (t *RaftReply) Marshal(wire io.Writer) {
	// Fixed header: CmdId (ClientId + SeqNum) = 8 bytes
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

	// Variable-length Value: varint length + raw bytes
	var vb [10]byte
	alen := int64(len(t.Value))
	wlen := binary.PutVarint(vb[:], alen)
	wire.Write(vb[:wlen])
	if alen > 0 {
		wire.Write(t.Value)
	}
}

func (t *RaftReply) Unmarshal(rr io.Reader) error {
	// Fixed header
	var b [8]byte
	var bs []byte
	bs = b[:8]
	if _, err := io.ReadAtLeast(rr, bs, 8); err != nil {
		return err
	}
	t.CmdId.ClientId = int32((uint32(bs[0]) | (uint32(bs[1]) << 8) | (uint32(bs[2]) << 16) | (uint32(bs[3]) << 24)))
	t.CmdId.SeqNum = int32((uint32(bs[4]) | (uint32(bs[5]) << 8) | (uint32(bs[6]) << 16) | (uint32(bs[7]) << 24)))

	// Variable-length Value
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

// --- byteReader interface ---
// Required for Unmarshal methods that use binary.ReadVarint.

type byteReader interface {
	io.Reader
	ReadByte() (c byte, err error)
}

// --- CommunicationSupply ---

type CommunicationSupply struct {
	maxLatency time.Duration

	appendEntriesChan      chan fastrpc.Serializable
	appendEntriesReplyChan chan fastrpc.Serializable
	requestVoteChan        chan fastrpc.Serializable
	requestVoteReplyChan   chan fastrpc.Serializable
	raftReplyChan          chan fastrpc.Serializable

	appendEntriesRPC      uint8
	appendEntriesReplyRPC uint8
	requestVoteRPC        uint8
	requestVoteReplyRPC   uint8
	raftReplyRPC          uint8
}

func initCs(cs *CommunicationSupply, t *fastrpc.Table) {
	cs.maxLatency = 0

	cs.appendEntriesChan = make(chan fastrpc.Serializable, defs.CHAN_BUFFER_SIZE)
	cs.appendEntriesReplyChan = make(chan fastrpc.Serializable, defs.CHAN_BUFFER_SIZE)
	cs.requestVoteChan = make(chan fastrpc.Serializable, defs.CHAN_BUFFER_SIZE)
	cs.requestVoteReplyChan = make(chan fastrpc.Serializable, defs.CHAN_BUFFER_SIZE)
	cs.raftReplyChan = make(chan fastrpc.Serializable, defs.CHAN_BUFFER_SIZE)

	cs.appendEntriesRPC = t.Register(new(AppendEntries), cs.appendEntriesChan)
	cs.appendEntriesReplyRPC = t.Register(new(AppendEntriesReply), cs.appendEntriesReplyChan)
	cs.requestVoteRPC = t.Register(new(RequestVote), cs.requestVoteChan)
	cs.requestVoteReplyRPC = t.Register(new(RequestVoteReply), cs.requestVoteReplyChan)
	cs.raftReplyRPC = t.Register(new(RaftReply), cs.raftReplyChan)
}
