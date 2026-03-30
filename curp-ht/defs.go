package curpht

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/imdea-software/swiftpaxos/replica/defs"
	fastrpc "github.com/imdea-software/swiftpaxos/rpc"
	"github.com/imdea-software/swiftpaxos/state"
)

// status
const (
	NORMAL = iota
	RECOVERING
)

// role
const (
	FOLLOWER = iota
	CANDIDATE
	LEADER
)

// phase
const (
	START = iota
	ACCEPT
	COMMIT
)

// Consistency Level
const (
	STRONG uint8 = 0 // Linearizable
	WEAK   uint8 = 1 // Causal
)

const (
	HISTORY_SIZE = 10010001
	TRUE         = uint8(1)
	FALSE        = uint8(0)
	ORDERED      = uint8(2)
)

var MaxDescRoutines = 10000 // Default concurrent goroutine limit; configurable via config

type CommandId struct {
	ClientId int32
	SeqNum   int32
}

func (cmdId CommandId) String() string {
	return fmt.Sprintf("%v,%v", cmdId.ClientId, cmdId.SeqNum)
}

type MReply struct {
	Replica int32
	Ballot  int32
	CmdId   CommandId
	Rep     []byte
	Ok      uint8
	Slot    int32
}

type MAccept struct {
	Replica int32
	Ballot  int32
	Cmd     state.Command
	CmdId   CommandId
	CmdSlot int
}

type MAcceptAck struct {
	Replica int32
	Ballot  int32
	CmdSlot int
}

type MAAcks struct {
	Acks    []MAcceptAck
	Accepts []MAccept
}

type MRecordAck struct {
	Replica int32
	Ballot  int32
	CmdId   CommandId
	Ok      uint8
}

type MCommit struct {
	Replica int32
	Ballot  int32
	CmdSlot int
}

type MSync struct {
	CmdId CommandId
}

type MSyncReply struct {
	Replica int32
	Ballot  int32
	CmdId   CommandId
	Rep     []byte
	Slot    int32
}

// MWeakPropose - Weak command propose (sent only to Leader)
type MWeakPropose struct {
	CommandId int32
	ClientId  int32
	Command   state.Command
	Timestamp int64
	CausalDep int32 // Sequence number of the previous weak command from this client (0 if none)
}

func (m *MWeakPropose) GetClientId() int32 { return m.ClientId }

// MWeakReply - Weak command reply (Leader replies after commit with slot)
type MWeakReply struct {
	Replica int32
	Ballot  int32
	CmdId   CommandId
	Rep     []byte
	Slot    int32
}

// MWeakRead - Weak read request (sent to nearest replica)
// Op: 0 or state.GET = single-key read, state.SCAN = range scan
// Count: number of keys to scan (only used when Op == SCAN)
type MWeakRead struct {
	CommandId int32
	ClientId  int32
	Key       state.Key
	Op        uint8
	Count     int64
}

func (m *MWeakRead) GetClientId() int32 { return m.ClientId }

// MWeakReadReply - Weak read reply from nearest replica (value + version)
type MWeakReadReply struct {
	Replica int32
	Ballot  int32
	CmdId   CommandId
	Rep     []byte
	Version int32
}

// MRequestVote - Candidate requests vote from peers during leader election
type MRequestVote struct {
	Replica          int32 // Candidate's replica ID
	Term             int32 // Candidate's term
	LastCommittedSlot int32 // Candidate's highest committed slot (for log comparison)
}

// MRequestVoteReply - Reply to vote request
type MRequestVoteReply struct {
	Replica     int32 // Voter's replica ID
	Term        int32 // Voter's current term (may be updated)
	VoteGranted uint8 // 1 = granted, 0 = denied
}

// MHeartbeat - Leader sends periodic heartbeats to prevent elections
type MHeartbeat struct {
	Replica int32 // Leader's replica ID
	Term    int32 // Leader's term
}

// LogEntry represents a committed log entry for recovery
type LogEntry struct {
	Slot  int32
	CmdId CommandId
	Cmd   state.Command
}

// MLogSync - New leader requests committed log entries from followers
type MLogSync struct {
	Replica int32 // New leader's ID
	Term    int32 // New leader's term
}

// MLogSyncReply - Follower sends committed log entries to new leader
type MLogSyncReply struct {
	Replica    int32      // Follower's ID
	Term       int32      // Follower's term
	NumEntries int32      // Number of entries
	Entries    []LogEntry // Committed log entries
}

// MSlotSync - New leader asks peers for their lastCommitted slot.
// Lightweight recovery: no log entries transferred, just the slot number.
type MSlotSync struct {
	Replica int32 // New leader's ID
	Term    int32 // New leader's term
}

// MSlotSyncReply - Peer replies with its lastCommitted slot.
type MSlotSyncReply struct {
	Replica       int32 // Peer's ID
	Term          int32 // Peer's term
	LastCommitted int32 // Peer's highest committed slot
}

// MForwardPropose - Follower forwards a client proposal to the actual leader.
// Used when a client sends a Propose to a non-leader replica (e.g., after failover
// when the client doesn't know who the real leader is).
type MForwardPropose struct {
	Replica   int32         // Forwarding replica ID
	ClientId  int32         // Original client ID
	CommandId int32         // Original command sequence number
	Command   state.Command // The actual command
	Timestamp int64         // Original timestamp
}

func (m *MReply) New() fastrpc.Serializable {
	return new(MReply)
}

func (m *MAccept) New() fastrpc.Serializable {
	return new(MAccept)
}

func (m *MAcceptAck) New() fastrpc.Serializable {
	return new(MAcceptAck)
}

func (m *MAAcks) New() fastrpc.Serializable {
	return new(MAAcks)
}

func (m *MRecordAck) New() fastrpc.Serializable {
	return new(MRecordAck)
}

func (m *MCommit) New() fastrpc.Serializable {
	return new(MCommit)
}

func (m *MSync) New() fastrpc.Serializable {
	return new(MSync)
}

func (m *MSyncReply) New() fastrpc.Serializable {
	return new(MSyncReply)
}

func (m *MWeakPropose) New() fastrpc.Serializable {
	return new(MWeakPropose)
}

func (m *MWeakReply) New() fastrpc.Serializable {
	return new(MWeakReply)
}

func (m *MWeakRead) New() fastrpc.Serializable {
	return new(MWeakRead)
}

func (m *MWeakReadReply) New() fastrpc.Serializable {
	return new(MWeakReadReply)
}

func (m *MRequestVote) New() fastrpc.Serializable {
	return new(MRequestVote)
}

func (m *MRequestVoteReply) New() fastrpc.Serializable {
	return new(MRequestVoteReply)
}

func (m *MHeartbeat) New() fastrpc.Serializable {
	return new(MHeartbeat)
}

func (m *MLogSync) New() fastrpc.Serializable {
	return new(MLogSync)
}

func (m *MLogSyncReply) New() fastrpc.Serializable {
	return new(MLogSyncReply)
}

func (m *MSlotSync) New() fastrpc.Serializable {
	return new(MSlotSync)
}

func (m *MSlotSyncReply) New() fastrpc.Serializable {
	return new(MSlotSyncReply)
}

func (m *MForwardPropose) New() fastrpc.Serializable {
	return new(MForwardPropose)
}

type CommunicationSupply struct {
	maxLatency time.Duration

	replyChan     chan fastrpc.Serializable
	acceptChan    chan fastrpc.Serializable
	acceptAckChan chan fastrpc.Serializable
	aacksChan     chan fastrpc.Serializable
	recordAckChan chan fastrpc.Serializable
	commitChan    chan fastrpc.Serializable
	syncChan      chan fastrpc.Serializable
	syncReplyChan chan fastrpc.Serializable

	// Weak command channels
	weakProposeChan   chan fastrpc.Serializable
	weakReplyChan     chan fastrpc.Serializable
	weakReadChan      chan fastrpc.Serializable
	weakReadReplyChan chan fastrpc.Serializable

	// Election channels
	requestVoteChan      chan fastrpc.Serializable
	requestVoteReplyChan chan fastrpc.Serializable
	heartbeatChan        chan fastrpc.Serializable

	// Log recovery channels
	logSyncChan      chan fastrpc.Serializable
	logSyncReplyChan chan fastrpc.Serializable

	// Slot sync channels (lightweight recovery)
	slotSyncChan      chan fastrpc.Serializable
	slotSyncReplyChan chan fastrpc.Serializable

	// Proposal forwarding channel
	forwardProposeChan chan fastrpc.Serializable

	replyRPC     uint8
	acceptRPC    uint8
	acceptAckRPC uint8
	aacksRPC     uint8
	recordAckRPC uint8
	commitRPC    uint8
	syncRPC      uint8
	syncReplyRPC uint8

	// Weak command RPCs
	weakProposeRPC   uint8
	weakReplyRPC     uint8
	weakReadRPC      uint8
	weakReadReplyRPC uint8

	// Election RPCs
	requestVoteRPC      uint8
	requestVoteReplyRPC uint8
	heartbeatRPC        uint8

	// Log recovery RPCs
	logSyncRPC      uint8
	logSyncReplyRPC uint8

	// Slot sync RPCs (lightweight recovery)
	slotSyncRPC      uint8
	slotSyncReplyRPC uint8

	// Proposal forwarding RPC
	forwardProposeRPC uint8
}

func initCs(cs *CommunicationSupply, t *fastrpc.Table) {
	cs.maxLatency = 0

	cs.replyChan = make(chan fastrpc.Serializable, defs.CHAN_BUFFER_SIZE)
	cs.acceptChan = make(chan fastrpc.Serializable, defs.CHAN_BUFFER_SIZE)
	cs.acceptAckChan = make(chan fastrpc.Serializable, defs.CHAN_BUFFER_SIZE)
	cs.aacksChan = make(chan fastrpc.Serializable, defs.CHAN_BUFFER_SIZE)
	cs.recordAckChan = make(chan fastrpc.Serializable, defs.CHAN_BUFFER_SIZE)
	cs.commitChan = make(chan fastrpc.Serializable, defs.CHAN_BUFFER_SIZE)
	cs.syncChan = make(chan fastrpc.Serializable, defs.CHAN_BUFFER_SIZE)
	cs.syncReplyChan = make(chan fastrpc.Serializable, defs.CHAN_BUFFER_SIZE)

	cs.replyRPC = t.Register(new(MReply), cs.replyChan)
	cs.acceptRPC = t.Register(new(MAccept), cs.acceptChan)
	cs.acceptAckRPC = t.Register(new(MAcceptAck), cs.acceptAckChan)
	cs.aacksRPC = t.Register(new(MAAcks), cs.aacksChan)
	cs.recordAckRPC = t.Register(new(MRecordAck), cs.recordAckChan)
	cs.commitRPC = t.Register(new(MCommit), cs.commitChan)
	cs.syncRPC = t.Register(new(MSync), cs.syncChan)
	cs.syncReplyRPC = t.Register(new(MSyncReply), cs.syncReplyChan)

	// Initialize weak command channels
	cs.weakProposeChan = make(chan fastrpc.Serializable, defs.CHAN_BUFFER_SIZE)
	cs.weakReplyChan = make(chan fastrpc.Serializable, defs.CHAN_BUFFER_SIZE)
	cs.weakReadChan = make(chan fastrpc.Serializable, defs.CHAN_BUFFER_SIZE)
	cs.weakReadReplyChan = make(chan fastrpc.Serializable, defs.CHAN_BUFFER_SIZE)

	// Register weak command RPCs
	cs.weakProposeRPC = t.Register(new(MWeakPropose), cs.weakProposeChan)
	cs.weakReplyRPC = t.Register(new(MWeakReply), cs.weakReplyChan)
	cs.weakReadRPC = t.Register(new(MWeakRead), cs.weakReadChan)
	cs.weakReadReplyRPC = t.Register(new(MWeakReadReply), cs.weakReadReplyChan)

	// Initialize election channels
	cs.requestVoteChan = make(chan fastrpc.Serializable, defs.CHAN_BUFFER_SIZE)
	cs.requestVoteReplyChan = make(chan fastrpc.Serializable, defs.CHAN_BUFFER_SIZE)
	cs.heartbeatChan = make(chan fastrpc.Serializable, defs.CHAN_BUFFER_SIZE)

	// Register election RPCs
	cs.requestVoteRPC = t.Register(new(MRequestVote), cs.requestVoteChan)
	cs.requestVoteReplyRPC = t.Register(new(MRequestVoteReply), cs.requestVoteReplyChan)
	cs.heartbeatRPC = t.Register(new(MHeartbeat), cs.heartbeatChan)

	// Initialize log recovery channels
	cs.logSyncChan = make(chan fastrpc.Serializable, defs.CHAN_BUFFER_SIZE)
	cs.logSyncReplyChan = make(chan fastrpc.Serializable, defs.CHAN_BUFFER_SIZE)

	// Register log recovery RPCs
	cs.logSyncRPC = t.Register(new(MLogSync), cs.logSyncChan)
	cs.logSyncReplyRPC = t.Register(new(MLogSyncReply), cs.logSyncReplyChan)

	// Initialize and register slot sync (lightweight recovery)
	cs.slotSyncChan = make(chan fastrpc.Serializable, defs.CHAN_BUFFER_SIZE)
	cs.slotSyncReplyChan = make(chan fastrpc.Serializable, defs.CHAN_BUFFER_SIZE)
	cs.slotSyncRPC = t.Register(new(MSlotSync), cs.slotSyncChan)
	cs.slotSyncReplyRPC = t.Register(new(MSlotSyncReply), cs.slotSyncReplyChan)

	// Initialize and register proposal forwarding
	cs.forwardProposeChan = make(chan fastrpc.Serializable, defs.CHAN_BUFFER_SIZE)
	cs.forwardProposeRPC = t.Register(new(MForwardPropose), cs.forwardProposeChan)
}

type byteReader interface {
	io.Reader
	ReadByte() (c byte, err error)
}

func (t *MCommit) BinarySize() (nbytes int, sizeKnown bool) {
	return 16, true
}

type MCommitCache struct {
	mu    sync.Mutex
	cache []*MCommit
}

func NewMCommitCache() *MCommitCache {
	c := &MCommitCache{}
	c.cache = make([]*MCommit, 0)
	return c
}

func (p *MCommitCache) Get() *MCommit {
	var t *MCommit
	p.mu.Lock()
	if len(p.cache) > 0 {
		t = p.cache[len(p.cache)-1]
		p.cache = p.cache[0:(len(p.cache) - 1)]
	}
	p.mu.Unlock()
	if t == nil {
		t = &MCommit{}
	}
	return t
}
func (p *MCommitCache) Put(t *MCommit) {
	p.mu.Lock()
	p.cache = append(p.cache, t)
	p.mu.Unlock()
}
func (t *MCommit) Marshal(wire io.Writer) {
	var b [16]byte
	var bs []byte
	bs = b[:16]
	tmp32 := t.Replica
	bs[0] = byte(tmp32)
	bs[1] = byte(tmp32 >> 8)
	bs[2] = byte(tmp32 >> 16)
	bs[3] = byte(tmp32 >> 24)
	tmp32 = t.Ballot
	bs[4] = byte(tmp32)
	bs[5] = byte(tmp32 >> 8)
	bs[6] = byte(tmp32 >> 16)
	bs[7] = byte(tmp32 >> 24)
	tmp64 := t.CmdSlot
	bs[8] = byte(tmp64)
	bs[9] = byte(tmp64 >> 8)
	bs[10] = byte(tmp64 >> 16)
	bs[11] = byte(tmp64 >> 24)
	bs[12] = byte(tmp64 >> 32)
	bs[13] = byte(tmp64 >> 40)
	bs[14] = byte(tmp64 >> 48)
	bs[15] = byte(tmp64 >> 56)
	wire.Write(bs)
}

func (t *MCommit) Unmarshal(wire io.Reader) error {
	var b [16]byte
	var bs []byte
	bs = b[:16]
	if _, err := io.ReadAtLeast(wire, bs, 16); err != nil {
		return err
	}
	t.Replica = int32((uint32(bs[0]) | (uint32(bs[1]) << 8) | (uint32(bs[2]) << 16) | (uint32(bs[3]) << 24)))
	t.Ballot = int32((uint32(bs[4]) | (uint32(bs[5]) << 8) | (uint32(bs[6]) << 16) | (uint32(bs[7]) << 24)))
	t.CmdSlot = int((uint64(bs[8]) | (uint64(bs[9]) << 8) | (uint64(bs[10]) << 16) | (uint64(bs[11]) << 24) | (uint64(bs[12]) << 32) | (uint64(bs[13]) << 40) | (uint64(bs[14]) << 48) | (uint64(bs[15]) << 56)))
	return nil
}

func (t *MSyncReply) BinarySize() (nbytes int, sizeKnown bool) {
	return 0, false
}

type MSyncReplyCache struct {
	mu    sync.Mutex
	cache []*MSyncReply
}

func NewMSyncReplyCache() *MSyncReplyCache {
	c := &MSyncReplyCache{}
	c.cache = make([]*MSyncReply, 0)
	return c
}

func (p *MSyncReplyCache) Get() *MSyncReply {
	var t *MSyncReply
	p.mu.Lock()
	if len(p.cache) > 0 {
		t = p.cache[len(p.cache)-1]
		p.cache = p.cache[0:(len(p.cache) - 1)]
	}
	p.mu.Unlock()
	if t == nil {
		t = &MSyncReply{}
	}
	return t
}
func (p *MSyncReplyCache) Put(t *MSyncReply) {
	p.mu.Lock()
	p.cache = append(p.cache, t)
	p.mu.Unlock()
}
func (t *MSyncReply) Marshal(wire io.Writer) {
	var b [16]byte
	var bs []byte
	bs = b[:16]
	tmp32 := t.Replica
	bs[0] = byte(tmp32)
	bs[1] = byte(tmp32 >> 8)
	bs[2] = byte(tmp32 >> 16)
	bs[3] = byte(tmp32 >> 24)
	tmp32 = t.Ballot
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
	wire.Write(bs)
	bs = b[:]
	alen1 := int64(len(t.Rep))
	if wlen := binary.PutVarint(bs, alen1); wlen >= 0 {
		wire.Write(b[0:wlen])
	}
	wire.Write(t.Rep)
	bs = b[:4]
	tmp32 = t.Slot
	bs[0] = byte(tmp32)
	bs[1] = byte(tmp32 >> 8)
	bs[2] = byte(tmp32 >> 16)
	bs[3] = byte(tmp32 >> 24)
	wire.Write(bs)
}

func (t *MSyncReply) Unmarshal(rr io.Reader) error {
	var wire byteReader
	var ok bool
	if wire, ok = rr.(byteReader); !ok {
		wire = bufio.NewReader(rr)
	}
	var b [16]byte
	var bs []byte
	bs = b[:16]
	if _, err := io.ReadAtLeast(wire, bs, 16); err != nil {
		return err
	}
	t.Replica = int32((uint32(bs[0]) | (uint32(bs[1]) << 8) | (uint32(bs[2]) << 16) | (uint32(bs[3]) << 24)))
	t.Ballot = int32((uint32(bs[4]) | (uint32(bs[5]) << 8) | (uint32(bs[6]) << 16) | (uint32(bs[7]) << 24)))
	t.CmdId.ClientId = int32((uint32(bs[8]) | (uint32(bs[9]) << 8) | (uint32(bs[10]) << 16) | (uint32(bs[11]) << 24)))
	t.CmdId.SeqNum = int32((uint32(bs[12]) | (uint32(bs[13]) << 8) | (uint32(bs[14]) << 16) | (uint32(bs[15]) << 24)))
	alen1, err := binary.ReadVarint(wire)
	if err != nil {
		return err
	}
	t.Rep = make([]byte, alen1)
	if alen1 > 0 {
		if _, err := io.ReadFull(wire, t.Rep); err != nil {
			return err
		}
	}
	bs = b[:4]
	if _, err := io.ReadAtLeast(wire, bs, 4); err != nil {
		return err
	}
	t.Slot = int32((uint32(bs[0]) | (uint32(bs[1]) << 8) | (uint32(bs[2]) << 16) | (uint32(bs[3]) << 24)))
	return nil
}

func (t *MAccept) BinarySize() (nbytes int, sizeKnown bool) {
	return 0, false
}

type MAcceptCache struct {
	mu    sync.Mutex
	cache []*MAccept
}

func NewMAcceptCache() *MAcceptCache {
	c := &MAcceptCache{}
	c.cache = make([]*MAccept, 0)
	return c
}

func (p *MAcceptCache) Get() *MAccept {
	var t *MAccept
	p.mu.Lock()
	if len(p.cache) > 0 {
		t = p.cache[len(p.cache)-1]
		p.cache = p.cache[0:(len(p.cache) - 1)]
	}
	p.mu.Unlock()
	if t == nil {
		t = &MAccept{}
	}
	return t
}
func (p *MAcceptCache) Put(t *MAccept) {
	p.mu.Lock()
	p.cache = append(p.cache, t)
	p.mu.Unlock()
}
func (t *MAccept) Marshal(wire io.Writer) {
	var b [16]byte
	var bs []byte
	bs = b[:8]
	tmp32 := t.Replica
	bs[0] = byte(tmp32)
	bs[1] = byte(tmp32 >> 8)
	bs[2] = byte(tmp32 >> 16)
	bs[3] = byte(tmp32 >> 24)
	tmp32 = t.Ballot
	bs[4] = byte(tmp32)
	bs[5] = byte(tmp32 >> 8)
	bs[6] = byte(tmp32 >> 16)
	bs[7] = byte(tmp32 >> 24)
	wire.Write(bs)
	t.Cmd.Marshal(wire)
	bs = b[:16]
	tmp32 = t.CmdId.ClientId
	bs[0] = byte(tmp32)
	bs[1] = byte(tmp32 >> 8)
	bs[2] = byte(tmp32 >> 16)
	bs[3] = byte(tmp32 >> 24)
	tmp32 = t.CmdId.SeqNum
	bs[4] = byte(tmp32)
	bs[5] = byte(tmp32 >> 8)
	bs[6] = byte(tmp32 >> 16)
	bs[7] = byte(tmp32 >> 24)
	tmp64 := t.CmdSlot
	bs[8] = byte(tmp64)
	bs[9] = byte(tmp64 >> 8)
	bs[10] = byte(tmp64 >> 16)
	bs[11] = byte(tmp64 >> 24)
	bs[12] = byte(tmp64 >> 32)
	bs[13] = byte(tmp64 >> 40)
	bs[14] = byte(tmp64 >> 48)
	bs[15] = byte(tmp64 >> 56)
	wire.Write(bs)
}

func (t *MAccept) Unmarshal(wire io.Reader) error {
	var b [16]byte
	var bs []byte
	bs = b[:8]
	if _, err := io.ReadAtLeast(wire, bs, 8); err != nil {
		return err
	}
	t.Replica = int32((uint32(bs[0]) | (uint32(bs[1]) << 8) | (uint32(bs[2]) << 16) | (uint32(bs[3]) << 24)))
	t.Ballot = int32((uint32(bs[4]) | (uint32(bs[5]) << 8) | (uint32(bs[6]) << 16) | (uint32(bs[7]) << 24)))
	t.Cmd.Unmarshal(wire)
	bs = b[:16]
	if _, err := io.ReadAtLeast(wire, bs, 16); err != nil {
		return err
	}
	t.CmdId.ClientId = int32((uint32(bs[0]) | (uint32(bs[1]) << 8) | (uint32(bs[2]) << 16) | (uint32(bs[3]) << 24)))
	t.CmdId.SeqNum = int32((uint32(bs[4]) | (uint32(bs[5]) << 8) | (uint32(bs[6]) << 16) | (uint32(bs[7]) << 24)))
	t.CmdSlot = int((uint64(bs[8]) | (uint64(bs[9]) << 8) | (uint64(bs[10]) << 16) | (uint64(bs[11]) << 24) | (uint64(bs[12]) << 32) | (uint64(bs[13]) << 40) | (uint64(bs[14]) << 48) | (uint64(bs[15]) << 56)))
	return nil
}

func (t *MReply) BinarySize() (nbytes int, sizeKnown bool) {
	return 0, false
}

type MReplyCache struct {
	mu    sync.Mutex
	cache []*MReply
}

func NewMReplyCache() *MReplyCache {
	c := &MReplyCache{}
	c.cache = make([]*MReply, 0)
	return c
}

func (p *MReplyCache) Get() *MReply {
	var t *MReply
	p.mu.Lock()
	if len(p.cache) > 0 {
		t = p.cache[len(p.cache)-1]
		p.cache = p.cache[0:(len(p.cache) - 1)]
	}
	p.mu.Unlock()
	if t == nil {
		t = &MReply{}
	}
	return t
}
func (p *MReplyCache) Put(t *MReply) {
	p.mu.Lock()
	p.cache = append(p.cache, t)
	p.mu.Unlock()
}
func (t *MReply) Marshal(wire io.Writer) {
	var b [16]byte
	var bs []byte
	bs = b[:16]
	tmp32 := t.Replica
	bs[0] = byte(tmp32)
	bs[1] = byte(tmp32 >> 8)
	bs[2] = byte(tmp32 >> 16)
	bs[3] = byte(tmp32 >> 24)
	tmp32 = t.Ballot
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
	wire.Write(bs)
	bs = b[:]
	alen1 := int64(len(t.Rep))
	if wlen := binary.PutVarint(bs, alen1); wlen >= 0 {
		wire.Write(b[0:wlen])
	}
	wire.Write(t.Rep)
	bs = b[:1]
	bs[0] = byte(t.Ok)
	wire.Write(bs)
	bs = b[:4]
	tmp32 = t.Slot
	bs[0] = byte(tmp32)
	bs[1] = byte(tmp32 >> 8)
	bs[2] = byte(tmp32 >> 16)
	bs[3] = byte(tmp32 >> 24)
	wire.Write(bs)
}

func (t *MReply) Unmarshal(rr io.Reader) error {
	var wire byteReader
	var ok bool
	if wire, ok = rr.(byteReader); !ok {
		wire = bufio.NewReader(rr)
	}
	var b [16]byte
	var bs []byte
	bs = b[:16]
	if _, err := io.ReadAtLeast(wire, bs, 16); err != nil {
		return err
	}
	t.Replica = int32((uint32(bs[0]) | (uint32(bs[1]) << 8) | (uint32(bs[2]) << 16) | (uint32(bs[3]) << 24)))
	t.Ballot = int32((uint32(bs[4]) | (uint32(bs[5]) << 8) | (uint32(bs[6]) << 16) | (uint32(bs[7]) << 24)))
	t.CmdId.ClientId = int32((uint32(bs[8]) | (uint32(bs[9]) << 8) | (uint32(bs[10]) << 16) | (uint32(bs[11]) << 24)))
	t.CmdId.SeqNum = int32((uint32(bs[12]) | (uint32(bs[13]) << 8) | (uint32(bs[14]) << 16) | (uint32(bs[15]) << 24)))
	alen1, err := binary.ReadVarint(wire)
	if err != nil {
		return err
	}
	t.Rep = make([]byte, alen1)
	if alen1 > 0 {
		if _, err := io.ReadFull(wire, t.Rep); err != nil {
			return err
		}
	}
	bs = b[:1]
	if _, err := io.ReadAtLeast(wire, bs, 1); err != nil {
		return err
	}
	t.Ok = uint8(bs[0])
	bs = b[:4]
	if _, err := io.ReadAtLeast(wire, bs, 4); err != nil {
		return err
	}
	t.Slot = int32((uint32(bs[0]) | (uint32(bs[1]) << 8) | (uint32(bs[2]) << 16) | (uint32(bs[3]) << 24)))
	return nil
}

func (t *MAcceptAck) BinarySize() (nbytes int, sizeKnown bool) {
	return 16, true
}

type MAcceptAckCache struct {
	mu    sync.Mutex
	cache []*MAcceptAck
}

func NewMAcceptAckCache() *MAcceptAckCache {
	c := &MAcceptAckCache{}
	c.cache = make([]*MAcceptAck, 0)
	return c
}

func (p *MAcceptAckCache) Get() *MAcceptAck {
	var t *MAcceptAck
	p.mu.Lock()
	if len(p.cache) > 0 {
		t = p.cache[len(p.cache)-1]
		p.cache = p.cache[0:(len(p.cache) - 1)]
	}
	p.mu.Unlock()
	if t == nil {
		t = &MAcceptAck{}
	}
	return t
}
func (p *MAcceptAckCache) Put(t *MAcceptAck) {
	p.mu.Lock()
	p.cache = append(p.cache, t)
	p.mu.Unlock()
}
func (t *MAcceptAck) Marshal(wire io.Writer) {
	var b [16]byte
	var bs []byte
	bs = b[:16]
	tmp32 := t.Replica
	bs[0] = byte(tmp32)
	bs[1] = byte(tmp32 >> 8)
	bs[2] = byte(tmp32 >> 16)
	bs[3] = byte(tmp32 >> 24)
	tmp32 = t.Ballot
	bs[4] = byte(tmp32)
	bs[5] = byte(tmp32 >> 8)
	bs[6] = byte(tmp32 >> 16)
	bs[7] = byte(tmp32 >> 24)
	tmp64 := t.CmdSlot
	bs[8] = byte(tmp64)
	bs[9] = byte(tmp64 >> 8)
	bs[10] = byte(tmp64 >> 16)
	bs[11] = byte(tmp64 >> 24)
	bs[12] = byte(tmp64 >> 32)
	bs[13] = byte(tmp64 >> 40)
	bs[14] = byte(tmp64 >> 48)
	bs[15] = byte(tmp64 >> 56)
	wire.Write(bs)
}

func (t *MAcceptAck) Unmarshal(wire io.Reader) error {
	var b [16]byte
	var bs []byte
	bs = b[:16]
	if _, err := io.ReadAtLeast(wire, bs, 16); err != nil {
		return err
	}
	t.Replica = int32((uint32(bs[0]) | (uint32(bs[1]) << 8) | (uint32(bs[2]) << 16) | (uint32(bs[3]) << 24)))
	t.Ballot = int32((uint32(bs[4]) | (uint32(bs[5]) << 8) | (uint32(bs[6]) << 16) | (uint32(bs[7]) << 24)))
	t.CmdSlot = int((uint64(bs[8]) | (uint64(bs[9]) << 8) | (uint64(bs[10]) << 16) | (uint64(bs[11]) << 24) | (uint64(bs[12]) << 32) | (uint64(bs[13]) << 40) | (uint64(bs[14]) << 48) | (uint64(bs[15]) << 56)))
	return nil
}

func (t *MAAcks) BinarySize() (nbytes int, sizeKnown bool) {
	return 0, false
}

type MAAcksCache struct {
	mu    sync.Mutex
	cache []*MAAcks
}

func NewMAAcksCache() *MAAcksCache {
	c := &MAAcksCache{}
	c.cache = make([]*MAAcks, 0)
	return c
}

func (p *MAAcksCache) Get() *MAAcks {
	var t *MAAcks
	p.mu.Lock()
	if len(p.cache) > 0 {
		t = p.cache[len(p.cache)-1]
		p.cache = p.cache[0:(len(p.cache) - 1)]
	}
	p.mu.Unlock()
	if t == nil {
		t = &MAAcks{}
	}
	return t
}
func (p *MAAcksCache) Put(t *MAAcks) {
	p.mu.Lock()
	p.cache = append(p.cache, t)
	p.mu.Unlock()
}
func (t *MAAcks) Marshal(wire io.Writer) {
	var b [10]byte
	var bs []byte
	bs = b[:]
	alen1 := int64(len(t.Acks))
	if wlen := binary.PutVarint(bs, alen1); wlen >= 0 {
		wire.Write(b[0:wlen])
	}
	for i := int64(0); i < alen1; i++ {
		bs = b[:4]
		tmp32 := t.Acks[i].Replica
		bs[0] = byte(tmp32)
		bs[1] = byte(tmp32 >> 8)
		bs[2] = byte(tmp32 >> 16)
		bs[3] = byte(tmp32 >> 24)
		wire.Write(bs)
		tmp32 = t.Acks[i].Ballot
		bs[0] = byte(tmp32)
		bs[1] = byte(tmp32 >> 8)
		bs[2] = byte(tmp32 >> 16)
		bs[3] = byte(tmp32 >> 24)
		wire.Write(bs)
		bs = b[:8]
		tmp64 := t.Acks[i].CmdSlot
		bs[0] = byte(tmp64)
		bs[1] = byte(tmp64 >> 8)
		bs[2] = byte(tmp64 >> 16)
		bs[3] = byte(tmp64 >> 24)
		bs[4] = byte(tmp64 >> 32)
		bs[5] = byte(tmp64 >> 40)
		bs[6] = byte(tmp64 >> 48)
		bs[7] = byte(tmp64 >> 56)
		wire.Write(bs)
	}
	bs = b[:]
	alen2 := int64(len(t.Accepts))
	if wlen := binary.PutVarint(bs, alen2); wlen >= 0 {
		wire.Write(b[0:wlen])
	}
	for i := int64(0); i < alen2; i++ {
		t.Accepts[i].Marshal(wire)
	}
}

func (t *MAAcks) Unmarshal(rr io.Reader) error {
	var wire byteReader
	var ok bool
	if wire, ok = rr.(byteReader); !ok {
		wire = bufio.NewReader(rr)
	}
	var b [10]byte
	var bs []byte
	alen1, err := binary.ReadVarint(wire)
	if err != nil {
		return err
	}
	t.Acks = make([]MAcceptAck, alen1)
	for i := int64(0); i < alen1; i++ {
		bs = b[:4]
		if _, err := io.ReadAtLeast(wire, bs, 4); err != nil {
			return err
		}
		t.Acks[i].Replica = int32((uint32(bs[0]) | (uint32(bs[1]) << 8) | (uint32(bs[2]) << 16) | (uint32(bs[3]) << 24)))
		if _, err := io.ReadAtLeast(wire, bs, 4); err != nil {
			return err
		}
		t.Acks[i].Ballot = int32((uint32(bs[0]) | (uint32(bs[1]) << 8) | (uint32(bs[2]) << 16) | (uint32(bs[3]) << 24)))
		bs = b[:8]
		if _, err := io.ReadAtLeast(wire, bs, 8); err != nil {
			return err
		}
		t.Acks[i].CmdSlot = int((uint64(bs[0]) | (uint64(bs[1]) << 8) | (uint64(bs[2]) << 16) | (uint64(bs[3]) << 24) | (uint64(bs[4]) << 32) | (uint64(bs[5]) << 40) | (uint64(bs[6]) << 48) | (uint64(bs[7]) << 56)))
	}
	alen2, err := binary.ReadVarint(wire)
	if err != nil {
		return err
	}
	t.Accepts = make([]MAccept, alen2)
	for i := int64(0); i < alen2; i++ {
		t.Accepts[i].Unmarshal(wire)
	}
	return nil
}

func (t *MRecordAck) BinarySize() (nbytes int, sizeKnown bool) {
	return 17, true
}

type MRecordAckCache struct {
	mu    sync.Mutex
	cache []*MRecordAck
}

func NewMRecordAckCache() *MRecordAckCache {
	c := &MRecordAckCache{}
	c.cache = make([]*MRecordAck, 0)
	return c
}

func (p *MRecordAckCache) Get() *MRecordAck {
	var t *MRecordAck
	p.mu.Lock()
	if len(p.cache) > 0 {
		t = p.cache[len(p.cache)-1]
		p.cache = p.cache[0:(len(p.cache) - 1)]
	}
	p.mu.Unlock()
	if t == nil {
		t = &MRecordAck{}
	}
	return t
}
func (p *MRecordAckCache) Put(t *MRecordAck) {
	p.mu.Lock()
	p.cache = append(p.cache, t)
	p.mu.Unlock()
}
func (t *MRecordAck) Marshal(wire io.Writer) {
	var b [17]byte
	var bs []byte
	bs = b[:17]
	tmp32 := t.Replica
	bs[0] = byte(tmp32)
	bs[1] = byte(tmp32 >> 8)
	bs[2] = byte(tmp32 >> 16)
	bs[3] = byte(tmp32 >> 24)
	tmp32 = t.Ballot
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
	bs[16] = byte(t.Ok)
	wire.Write(bs)
}

func (t *MRecordAck) Unmarshal(wire io.Reader) error {
	var b [17]byte
	var bs []byte
	bs = b[:17]
	if _, err := io.ReadAtLeast(wire, bs, 17); err != nil {
		return err
	}
	t.Replica = int32((uint32(bs[0]) | (uint32(bs[1]) << 8) | (uint32(bs[2]) << 16) | (uint32(bs[3]) << 24)))
	t.Ballot = int32((uint32(bs[4]) | (uint32(bs[5]) << 8) | (uint32(bs[6]) << 16) | (uint32(bs[7]) << 24)))
	t.CmdId.ClientId = int32((uint32(bs[8]) | (uint32(bs[9]) << 8) | (uint32(bs[10]) << 16) | (uint32(bs[11]) << 24)))
	t.CmdId.SeqNum = int32((uint32(bs[12]) | (uint32(bs[13]) << 8) | (uint32(bs[14]) << 16) | (uint32(bs[15]) << 24)))
	t.Ok = uint8(bs[16])
	return nil
}

func (t *MSync) BinarySize() (nbytes int, sizeKnown bool) {
	return 8, true
}

type MSyncCache struct {
	mu    sync.Mutex
	cache []*MSync
}

func NewMSyncCache() *MSyncCache {
	c := &MSyncCache{}
	c.cache = make([]*MSync, 0)
	return c
}

func (p *MSyncCache) Get() *MSync {
	var t *MSync
	p.mu.Lock()
	if len(p.cache) > 0 {
		t = p.cache[len(p.cache)-1]
		p.cache = p.cache[0:(len(p.cache) - 1)]
	}
	p.mu.Unlock()
	if t == nil {
		t = &MSync{}
	}
	return t
}
func (p *MSyncCache) Put(t *MSync) {
	p.mu.Lock()
	p.cache = append(p.cache, t)
	p.mu.Unlock()
}
func (t *MSync) Marshal(wire io.Writer) {
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
}

func (t *MSync) Unmarshal(wire io.Reader) error {
	var b [8]byte
	var bs []byte
	bs = b[:8]
	if _, err := io.ReadAtLeast(wire, bs, 8); err != nil {
		return err
	}
	t.CmdId.ClientId = int32((uint32(bs[0]) | (uint32(bs[1]) << 8) | (uint32(bs[2]) << 16) | (uint32(bs[3]) << 24)))
	t.CmdId.SeqNum = int32((uint32(bs[4]) | (uint32(bs[5]) << 8) | (uint32(bs[6]) << 16) | (uint32(bs[7]) << 24)))
	return nil
}

func (t *CommandId) BinarySize() (nbytes int, sizeKnown bool) {
	return 8, true
}

type CommandIdCache struct {
	mu    sync.Mutex
	cache []*CommandId
}

func NewCommandIdCache() *CommandIdCache {
	c := &CommandIdCache{}
	c.cache = make([]*CommandId, 0)
	return c
}

func (p *CommandIdCache) Get() *CommandId {
	var t *CommandId
	p.mu.Lock()
	if len(p.cache) > 0 {
		t = p.cache[len(p.cache)-1]
		p.cache = p.cache[0:(len(p.cache) - 1)]
	}
	p.mu.Unlock()
	if t == nil {
		t = &CommandId{}
	}
	return t
}
func (p *CommandIdCache) Put(t *CommandId) {
	p.mu.Lock()
	p.cache = append(p.cache, t)
	p.mu.Unlock()
}
func (t *CommandId) Marshal(wire io.Writer) {
	var b [8]byte
	var bs []byte
	bs = b[:8]
	tmp32 := t.ClientId
	bs[0] = byte(tmp32)
	bs[1] = byte(tmp32 >> 8)
	bs[2] = byte(tmp32 >> 16)
	bs[3] = byte(tmp32 >> 24)
	tmp32 = t.SeqNum
	bs[4] = byte(tmp32)
	bs[5] = byte(tmp32 >> 8)
	bs[6] = byte(tmp32 >> 16)
	bs[7] = byte(tmp32 >> 24)
	wire.Write(bs)
}

func (t *CommandId) Unmarshal(wire io.Reader) error {
	var b [8]byte
	var bs []byte
	bs = b[:8]
	if _, err := io.ReadAtLeast(wire, bs, 8); err != nil {
		return err
	}
	t.ClientId = int32((uint32(bs[0]) | (uint32(bs[1]) << 8) | (uint32(bs[2]) << 16) | (uint32(bs[3]) << 24)))
	t.SeqNum = int32((uint32(bs[4]) | (uint32(bs[5]) << 8) | (uint32(bs[6]) << 16) | (uint32(bs[7]) << 24)))
	return nil
}

// MWeakPropose serialization
func (t *MWeakPropose) BinarySize() (nbytes int, sizeKnown bool) {
	return 0, false // Variable size due to Command
}

type MWeakProposeCache struct {
	mu    sync.Mutex
	cache []*MWeakPropose
}

func NewMWeakProposeCache() *MWeakProposeCache {
	c := &MWeakProposeCache{}
	c.cache = make([]*MWeakPropose, 0)
	return c
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

func (t *MWeakPropose) Marshal(wire io.Writer) {
	var b [16]byte
	var bs []byte
	bs = b[:8]
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
	bs = b[:12]
	tmp64 := t.Timestamp
	bs[0] = byte(tmp64)
	bs[1] = byte(tmp64 >> 8)
	bs[2] = byte(tmp64 >> 16)
	bs[3] = byte(tmp64 >> 24)
	bs[4] = byte(tmp64 >> 32)
	bs[5] = byte(tmp64 >> 40)
	bs[6] = byte(tmp64 >> 48)
	bs[7] = byte(tmp64 >> 56)
	tmp32 = t.CausalDep
	bs[8] = byte(tmp32)
	bs[9] = byte(tmp32 >> 8)
	bs[10] = byte(tmp32 >> 16)
	bs[11] = byte(tmp32 >> 24)
	wire.Write(bs)
}

func (t *MWeakPropose) Unmarshal(wire io.Reader) error {
	var b [16]byte
	var bs []byte
	bs = b[:8]
	if _, err := io.ReadAtLeast(wire, bs, 8); err != nil {
		return err
	}
	t.CommandId = int32((uint32(bs[0]) | (uint32(bs[1]) << 8) | (uint32(bs[2]) << 16) | (uint32(bs[3]) << 24)))
	t.ClientId = int32((uint32(bs[4]) | (uint32(bs[5]) << 8) | (uint32(bs[6]) << 16) | (uint32(bs[7]) << 24)))
	t.Command.Unmarshal(wire)
	bs = b[:12]
	if _, err := io.ReadAtLeast(wire, bs, 12); err != nil {
		return err
	}
	t.Timestamp = int64((uint64(bs[0]) | (uint64(bs[1]) << 8) | (uint64(bs[2]) << 16) | (uint64(bs[3]) << 24) | (uint64(bs[4]) << 32) | (uint64(bs[5]) << 40) | (uint64(bs[6]) << 48) | (uint64(bs[7]) << 56)))
	t.CausalDep = int32((uint32(bs[8]) | (uint32(bs[9]) << 8) | (uint32(bs[10]) << 16) | (uint32(bs[11]) << 24)))
	return nil
}

// MWeakReply serialization
func (t *MWeakReply) BinarySize() (nbytes int, sizeKnown bool) {
	return 0, false // Variable size due to Rep []byte
}

type MWeakReplyCache struct {
	mu    sync.Mutex
	cache []*MWeakReply
}

func NewMWeakReplyCache() *MWeakReplyCache {
	c := &MWeakReplyCache{}
	c.cache = make([]*MWeakReply, 0)
	return c
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

func (t *MWeakReply) Marshal(wire io.Writer) {
	var b [16]byte
	var bs []byte
	bs = b[:16]
	tmp32 := t.Replica
	bs[0] = byte(tmp32)
	bs[1] = byte(tmp32 >> 8)
	bs[2] = byte(tmp32 >> 16)
	bs[3] = byte(tmp32 >> 24)
	tmp32 = t.Ballot
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
	wire.Write(bs)
	bs = b[:]
	alen1 := int64(len(t.Rep))
	if wlen := binary.PutVarint(bs, alen1); wlen >= 0 {
		wire.Write(b[0:wlen])
	}
	wire.Write(t.Rep)
	bs = b[:4]
	tmp32 = t.Slot
	bs[0] = byte(tmp32)
	bs[1] = byte(tmp32 >> 8)
	bs[2] = byte(tmp32 >> 16)
	bs[3] = byte(tmp32 >> 24)
	wire.Write(bs)
}

func (t *MWeakReply) Unmarshal(rr io.Reader) error {
	var wire byteReader
	var ok bool
	if wire, ok = rr.(byteReader); !ok {
		wire = bufio.NewReader(rr)
	}
	var b [16]byte
	var bs []byte
	bs = b[:16]
	if _, err := io.ReadAtLeast(wire, bs, 16); err != nil {
		return err
	}
	t.Replica = int32((uint32(bs[0]) | (uint32(bs[1]) << 8) | (uint32(bs[2]) << 16) | (uint32(bs[3]) << 24)))
	t.Ballot = int32((uint32(bs[4]) | (uint32(bs[5]) << 8) | (uint32(bs[6]) << 16) | (uint32(bs[7]) << 24)))
	t.CmdId.ClientId = int32((uint32(bs[8]) | (uint32(bs[9]) << 8) | (uint32(bs[10]) << 16) | (uint32(bs[11]) << 24)))
	t.CmdId.SeqNum = int32((uint32(bs[12]) | (uint32(bs[13]) << 8) | (uint32(bs[14]) << 16) | (uint32(bs[15]) << 24)))
	alen1, err := binary.ReadVarint(wire)
	if err != nil {
		return err
	}
	t.Rep = make([]byte, alen1)
	if alen1 > 0 {
		if _, err := io.ReadFull(wire, t.Rep); err != nil {
			return err
		}
	}
	bs = b[:4]
	if _, err := io.ReadAtLeast(wire, bs, 4); err != nil {
		return err
	}
	t.Slot = int32((uint32(bs[0]) | (uint32(bs[1]) << 8) | (uint32(bs[2]) << 16) | (uint32(bs[3]) << 24)))
	return nil
}

// MWeakRead serialization

func (t *MWeakRead) BinarySize() (nbytes int, sizeKnown bool) {
	return 25, true // 4 + 4 + 8 + 1 + 8
}

type MWeakReadCache struct {
	mu    sync.Mutex
	cache []*MWeakRead
}

func NewMWeakReadCache() *MWeakReadCache {
	c := &MWeakReadCache{}
	c.cache = make([]*MWeakRead, 0)
	return c
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

func (t *MWeakRead) Marshal(wire io.Writer) {
	var b [25]byte
	var bs []byte
	bs = b[:25]
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
	bs[16] = byte(t.Op)
	tmp64 = t.Count
	bs[17] = byte(tmp64)
	bs[18] = byte(tmp64 >> 8)
	bs[19] = byte(tmp64 >> 16)
	bs[20] = byte(tmp64 >> 24)
	bs[21] = byte(tmp64 >> 32)
	bs[22] = byte(tmp64 >> 40)
	bs[23] = byte(tmp64 >> 48)
	bs[24] = byte(tmp64 >> 56)
	wire.Write(bs)
}

func (t *MWeakRead) Unmarshal(wire io.Reader) error {
	var b [25]byte
	var bs []byte
	bs = b[:25]
	if _, err := io.ReadAtLeast(wire, bs, 25); err != nil {
		return err
	}
	t.CommandId = int32((uint32(bs[0]) | (uint32(bs[1]) << 8) | (uint32(bs[2]) << 16) | (uint32(bs[3]) << 24)))
	t.ClientId = int32((uint32(bs[4]) | (uint32(bs[5]) << 8) | (uint32(bs[6]) << 16) | (uint32(bs[7]) << 24)))
	t.Key = state.Key(uint64(bs[8]) | (uint64(bs[9]) << 8) | (uint64(bs[10]) << 16) | (uint64(bs[11]) << 24) | (uint64(bs[12]) << 32) | (uint64(bs[13]) << 40) | (uint64(bs[14]) << 48) | (uint64(bs[15]) << 56))
	t.Op = uint8(bs[16])
	t.Count = int64(uint64(bs[17]) | (uint64(bs[18]) << 8) | (uint64(bs[19]) << 16) | (uint64(bs[20]) << 24) | (uint64(bs[21]) << 32) | (uint64(bs[22]) << 40) | (uint64(bs[23]) << 48) | (uint64(bs[24]) << 56))
	return nil
}

// MWeakReadReply serialization

func (t *MWeakReadReply) BinarySize() (nbytes int, sizeKnown bool) {
	return 0, false // Variable size due to Rep
}

type MWeakReadReplyCache struct {
	mu    sync.Mutex
	cache []*MWeakReadReply
}

func NewMWeakReadReplyCache() *MWeakReadReplyCache {
	c := &MWeakReadReplyCache{}
	c.cache = make([]*MWeakReadReply, 0)
	return c
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

func (t *MWeakReadReply) Marshal(wire io.Writer) {
	var b [16]byte
	var bs []byte
	bs = b[:16]
	tmp32 := t.Replica
	bs[0] = byte(tmp32)
	bs[1] = byte(tmp32 >> 8)
	bs[2] = byte(tmp32 >> 16)
	bs[3] = byte(tmp32 >> 24)
	tmp32 = t.Ballot
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
	wire.Write(bs)
	bs = b[:]
	alen1 := int64(len(t.Rep))
	if wlen := binary.PutVarint(bs, alen1); wlen >= 0 {
		wire.Write(b[0:wlen])
	}
	wire.Write(t.Rep)
	bs = b[:4]
	tmp32 = t.Version
	bs[0] = byte(tmp32)
	bs[1] = byte(tmp32 >> 8)
	bs[2] = byte(tmp32 >> 16)
	bs[3] = byte(tmp32 >> 24)
	wire.Write(bs)
}

func (t *MWeakReadReply) Unmarshal(rr io.Reader) error {
	var wire byteReader
	var ok bool
	if wire, ok = rr.(byteReader); !ok {
		wire = bufio.NewReader(rr)
	}
	var b [16]byte
	var bs []byte
	bs = b[:16]
	if _, err := io.ReadAtLeast(wire, bs, 16); err != nil {
		return err
	}
	t.Replica = int32((uint32(bs[0]) | (uint32(bs[1]) << 8) | (uint32(bs[2]) << 16) | (uint32(bs[3]) << 24)))
	t.Ballot = int32((uint32(bs[4]) | (uint32(bs[5]) << 8) | (uint32(bs[6]) << 16) | (uint32(bs[7]) << 24)))
	t.CmdId.ClientId = int32((uint32(bs[8]) | (uint32(bs[9]) << 8) | (uint32(bs[10]) << 16) | (uint32(bs[11]) << 24)))
	t.CmdId.SeqNum = int32((uint32(bs[12]) | (uint32(bs[13]) << 8) | (uint32(bs[14]) << 16) | (uint32(bs[15]) << 24)))
	alen1, err := binary.ReadVarint(wire)
	if err != nil {
		return err
	}
	t.Rep = make([]byte, alen1)
	if alen1 > 0 {
		if _, err := io.ReadFull(wire, t.Rep); err != nil {
			return err
		}
	}
	bs = b[:4]
	if _, err := io.ReadAtLeast(wire, bs, 4); err != nil {
		return err
	}
	t.Version = int32((uint32(bs[0]) | (uint32(bs[1]) << 8) | (uint32(bs[2]) << 16) | (uint32(bs[3]) << 24)))
	return nil
}

// MRequestVote serialization (fixed size: 12 bytes)

func (t *MRequestVote) BinarySize() (nbytes int, sizeKnown bool) {
	return 12, true
}

type MRequestVoteCache struct {
	mu    sync.Mutex
	cache []*MRequestVote
}

func NewMRequestVoteCache() *MRequestVoteCache {
	c := &MRequestVoteCache{}
	c.cache = make([]*MRequestVote, 0)
	return c
}

func (p *MRequestVoteCache) Get() *MRequestVote {
	var t *MRequestVote
	p.mu.Lock()
	if len(p.cache) > 0 {
		t = p.cache[len(p.cache)-1]
		p.cache = p.cache[0:(len(p.cache) - 1)]
	}
	p.mu.Unlock()
	if t == nil {
		t = &MRequestVote{}
	}
	return t
}

func (p *MRequestVoteCache) Put(t *MRequestVote) {
	p.mu.Lock()
	p.cache = append(p.cache, t)
	p.mu.Unlock()
}

func (t *MRequestVote) Marshal(wire io.Writer) {
	var b [12]byte
	var bs []byte
	bs = b[:12]
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
	tmp32 = t.LastCommittedSlot
	bs[8] = byte(tmp32)
	bs[9] = byte(tmp32 >> 8)
	bs[10] = byte(tmp32 >> 16)
	bs[11] = byte(tmp32 >> 24)
	wire.Write(bs)
}

func (t *MRequestVote) Unmarshal(wire io.Reader) error {
	var b [12]byte
	var bs []byte
	bs = b[:12]
	if _, err := io.ReadAtLeast(wire, bs, 12); err != nil {
		return err
	}
	t.Replica = int32((uint32(bs[0]) | (uint32(bs[1]) << 8) | (uint32(bs[2]) << 16) | (uint32(bs[3]) << 24)))
	t.Term = int32((uint32(bs[4]) | (uint32(bs[5]) << 8) | (uint32(bs[6]) << 16) | (uint32(bs[7]) << 24)))
	t.LastCommittedSlot = int32((uint32(bs[8]) | (uint32(bs[9]) << 8) | (uint32(bs[10]) << 16) | (uint32(bs[11]) << 24)))
	return nil
}

// MRequestVoteReply serialization (fixed size: 9 bytes)

func (t *MRequestVoteReply) BinarySize() (nbytes int, sizeKnown bool) {
	return 9, true
}

type MRequestVoteReplyCache struct {
	mu    sync.Mutex
	cache []*MRequestVoteReply
}

func NewMRequestVoteReplyCache() *MRequestVoteReplyCache {
	c := &MRequestVoteReplyCache{}
	c.cache = make([]*MRequestVoteReply, 0)
	return c
}

func (p *MRequestVoteReplyCache) Get() *MRequestVoteReply {
	var t *MRequestVoteReply
	p.mu.Lock()
	if len(p.cache) > 0 {
		t = p.cache[len(p.cache)-1]
		p.cache = p.cache[0:(len(p.cache) - 1)]
	}
	p.mu.Unlock()
	if t == nil {
		t = &MRequestVoteReply{}
	}
	return t
}

func (p *MRequestVoteReplyCache) Put(t *MRequestVoteReply) {
	p.mu.Lock()
	p.cache = append(p.cache, t)
	p.mu.Unlock()
}

func (t *MRequestVoteReply) Marshal(wire io.Writer) {
	var b [9]byte
	var bs []byte
	bs = b[:9]
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
	bs[8] = byte(t.VoteGranted)
	wire.Write(bs)
}

func (t *MRequestVoteReply) Unmarshal(wire io.Reader) error {
	var b [9]byte
	var bs []byte
	bs = b[:9]
	if _, err := io.ReadAtLeast(wire, bs, 9); err != nil {
		return err
	}
	t.Replica = int32((uint32(bs[0]) | (uint32(bs[1]) << 8) | (uint32(bs[2]) << 16) | (uint32(bs[3]) << 24)))
	t.Term = int32((uint32(bs[4]) | (uint32(bs[5]) << 8) | (uint32(bs[6]) << 16) | (uint32(bs[7]) << 24)))
	t.VoteGranted = uint8(bs[8])
	return nil
}

// MHeartbeat serialization (fixed size: 8 bytes)

func (t *MHeartbeat) BinarySize() (nbytes int, sizeKnown bool) {
	return 8, true
}

type MHeartbeatCache struct {
	mu    sync.Mutex
	cache []*MHeartbeat
}

func NewMHeartbeatCache() *MHeartbeatCache {
	c := &MHeartbeatCache{}
	c.cache = make([]*MHeartbeat, 0)
	return c
}

func (p *MHeartbeatCache) Get() *MHeartbeat {
	var t *MHeartbeat
	p.mu.Lock()
	if len(p.cache) > 0 {
		t = p.cache[len(p.cache)-1]
		p.cache = p.cache[0:(len(p.cache) - 1)]
	}
	p.mu.Unlock()
	if t == nil {
		t = &MHeartbeat{}
	}
	return t
}

func (p *MHeartbeatCache) Put(t *MHeartbeat) {
	p.mu.Lock()
	p.cache = append(p.cache, t)
	p.mu.Unlock()
}

func (t *MHeartbeat) Marshal(wire io.Writer) {
	var b [8]byte
	var bs []byte
	bs = b[:8]
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
	wire.Write(bs)
}

func (t *MHeartbeat) Unmarshal(wire io.Reader) error {
	var b [8]byte
	var bs []byte
	bs = b[:8]
	if _, err := io.ReadAtLeast(wire, bs, 8); err != nil {
		return err
	}
	t.Replica = int32((uint32(bs[0]) | (uint32(bs[1]) << 8) | (uint32(bs[2]) << 16) | (uint32(bs[3]) << 24)))
	t.Term = int32((uint32(bs[4]) | (uint32(bs[5]) << 8) | (uint32(bs[6]) << 16) | (uint32(bs[7]) << 24)))
	return nil
}

// MLogSync serialization — fixed 8 bytes: Replica(4) + Term(4)

func (t *MLogSync) BinarySize() (nbytes int, sizeKnown bool) {
	return 8, true
}

type MLogSyncCache struct {
	mu    sync.Mutex
	cache []*MLogSync
}

func NewMLogSyncCache() *MLogSyncCache {
	c := &MLogSyncCache{}
	c.cache = make([]*MLogSync, 0)
	return c
}

func (p *MLogSyncCache) Get() *MLogSync {
	var t *MLogSync
	p.mu.Lock()
	if len(p.cache) > 0 {
		t = p.cache[len(p.cache)-1]
		p.cache = p.cache[0:(len(p.cache) - 1)]
	}
	p.mu.Unlock()
	if t == nil {
		t = &MLogSync{}
	}
	return t
}
func (p *MLogSyncCache) Put(t *MLogSync) {
	p.mu.Lock()
	p.cache = append(p.cache, t)
	p.mu.Unlock()
}
func (t *MLogSync) Marshal(wire io.Writer) {
	var b [8]byte
	var bs []byte
	bs = b[:8]
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
	wire.Write(bs)
}

func (t *MLogSync) Unmarshal(wire io.Reader) error {
	var b [8]byte
	var bs []byte
	bs = b[:8]
	if _, err := io.ReadAtLeast(wire, bs, 8); err != nil {
		return err
	}
	t.Replica = int32((uint32(bs[0]) | (uint32(bs[1]) << 8) | (uint32(bs[2]) << 16) | (uint32(bs[3]) << 24)))
	t.Term = int32((uint32(bs[4]) | (uint32(bs[5]) << 8) | (uint32(bs[6]) << 16) | (uint32(bs[7]) << 24)))
	return nil
}

// MLogSyncReply serialization — variable length: header(12) + entries (each: Slot(4) + CmdId(8) + Cmd(var))

func (t *MLogSyncReply) BinarySize() (nbytes int, sizeKnown bool) {
	return 0, false
}

type MLogSyncReplyCache struct {
	mu    sync.Mutex
	cache []*MLogSyncReply
}

func NewMLogSyncReplyCache() *MLogSyncReplyCache {
	c := &MLogSyncReplyCache{}
	c.cache = make([]*MLogSyncReply, 0)
	return c
}

func (p *MLogSyncReplyCache) Get() *MLogSyncReply {
	var t *MLogSyncReply
	p.mu.Lock()
	if len(p.cache) > 0 {
		t = p.cache[len(p.cache)-1]
		p.cache = p.cache[0:(len(p.cache) - 1)]
	}
	p.mu.Unlock()
	if t == nil {
		t = &MLogSyncReply{}
	}
	return t
}
func (p *MLogSyncReplyCache) Put(t *MLogSyncReply) {
	p.mu.Lock()
	p.cache = append(p.cache, t)
	p.mu.Unlock()
}
func (t *MLogSyncReply) Marshal(wire io.Writer) {
	var b [12]byte
	var bs []byte
	bs = b[:12]
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
	tmp32 = t.NumEntries
	bs[8] = byte(tmp32)
	bs[9] = byte(tmp32 >> 8)
	bs[10] = byte(tmp32 >> 16)
	bs[11] = byte(tmp32 >> 24)
	wire.Write(bs)
	for i := int32(0); i < t.NumEntries; i++ {
		bs = b[:12]
		tmp32 = t.Entries[i].Slot
		bs[0] = byte(tmp32)
		bs[1] = byte(tmp32 >> 8)
		bs[2] = byte(tmp32 >> 16)
		bs[3] = byte(tmp32 >> 24)
		tmp32 = t.Entries[i].CmdId.ClientId
		bs[4] = byte(tmp32)
		bs[5] = byte(tmp32 >> 8)
		bs[6] = byte(tmp32 >> 16)
		bs[7] = byte(tmp32 >> 24)
		tmp32 = t.Entries[i].CmdId.SeqNum
		bs[8] = byte(tmp32)
		bs[9] = byte(tmp32 >> 8)
		bs[10] = byte(tmp32 >> 16)
		bs[11] = byte(tmp32 >> 24)
		wire.Write(bs)
		t.Entries[i].Cmd.Marshal(wire)
	}
}

func (t *MLogSyncReply) Unmarshal(rr io.Reader) error {
	var wire byteReader
	var ok bool
	if wire, ok = rr.(byteReader); !ok {
		wire = bufio.NewReader(rr)
	}
	var b [12]byte
	var bs []byte
	bs = b[:12]
	if _, err := io.ReadAtLeast(wire, bs, 12); err != nil {
		return err
	}
	t.Replica = int32((uint32(bs[0]) | (uint32(bs[1]) << 8) | (uint32(bs[2]) << 16) | (uint32(bs[3]) << 24)))
	t.Term = int32((uint32(bs[4]) | (uint32(bs[5]) << 8) | (uint32(bs[6]) << 16) | (uint32(bs[7]) << 24)))
	t.NumEntries = int32((uint32(bs[8]) | (uint32(bs[9]) << 8) | (uint32(bs[10]) << 16) | (uint32(bs[11]) << 24)))
	t.Entries = make([]LogEntry, t.NumEntries)
	for i := int32(0); i < t.NumEntries; i++ {
		bs = b[:12]
		if _, err := io.ReadAtLeast(wire, bs, 12); err != nil {
			return err
		}
		t.Entries[i].Slot = int32((uint32(bs[0]) | (uint32(bs[1]) << 8) | (uint32(bs[2]) << 16) | (uint32(bs[3]) << 24)))
		t.Entries[i].CmdId.ClientId = int32((uint32(bs[4]) | (uint32(bs[5]) << 8) | (uint32(bs[6]) << 16) | (uint32(bs[7]) << 24)))
		t.Entries[i].CmdId.SeqNum = int32((uint32(bs[8]) | (uint32(bs[9]) << 8) | (uint32(bs[10]) << 16) | (uint32(bs[11]) << 24)))
		t.Entries[i].Cmd.Unmarshal(wire)
	}
	return nil
}

// MForwardPropose serialization — variable size: Replica(4) + ClientId(4) + CommandId(4) + Command(var) + Timestamp(8)

func (t *MForwardPropose) BinarySize() (nbytes int, sizeKnown bool) {
	return 0, false
}

type MForwardProposeCache struct {
	mu    sync.Mutex
	cache []*MForwardPropose
}

func NewMForwardProposeCache() *MForwardProposeCache {
	c := &MForwardProposeCache{}
	c.cache = make([]*MForwardPropose, 0)
	return c
}

func (p *MForwardProposeCache) Get() *MForwardPropose {
	var t *MForwardPropose
	p.mu.Lock()
	if len(p.cache) > 0 {
		t = p.cache[len(p.cache)-1]
		p.cache = p.cache[0:(len(p.cache) - 1)]
	}
	p.mu.Unlock()
	if t == nil {
		t = &MForwardPropose{}
	}
	return t
}

func (p *MForwardProposeCache) Put(t *MForwardPropose) {
	p.mu.Lock()
	p.cache = append(p.cache, t)
	p.mu.Unlock()
}

func (t *MForwardPropose) Marshal(wire io.Writer) {
	var b [20]byte
	var bs []byte
	bs = b[:12]
	tmp32 := t.Replica
	bs[0] = byte(tmp32)
	bs[1] = byte(tmp32 >> 8)
	bs[2] = byte(tmp32 >> 16)
	bs[3] = byte(tmp32 >> 24)
	tmp32 = t.ClientId
	bs[4] = byte(tmp32)
	bs[5] = byte(tmp32 >> 8)
	bs[6] = byte(tmp32 >> 16)
	bs[7] = byte(tmp32 >> 24)
	tmp32 = t.CommandId
	bs[8] = byte(tmp32)
	bs[9] = byte(tmp32 >> 8)
	bs[10] = byte(tmp32 >> 16)
	bs[11] = byte(tmp32 >> 24)
	wire.Write(bs)
	t.Command.Marshal(wire)
	bs = b[:8]
	tmp64 := t.Timestamp
	bs[0] = byte(tmp64)
	bs[1] = byte(tmp64 >> 8)
	bs[2] = byte(tmp64 >> 16)
	bs[3] = byte(tmp64 >> 24)
	bs[4] = byte(tmp64 >> 32)
	bs[5] = byte(tmp64 >> 40)
	bs[6] = byte(tmp64 >> 48)
	bs[7] = byte(tmp64 >> 56)
	wire.Write(bs)
}

func (t *MForwardPropose) Unmarshal(wire io.Reader) error {
	var b [20]byte
	var bs []byte
	bs = b[:12]
	if _, err := io.ReadAtLeast(wire, bs, 12); err != nil {
		return err
	}
	t.Replica = int32((uint32(bs[0]) | (uint32(bs[1]) << 8) | (uint32(bs[2]) << 16) | (uint32(bs[3]) << 24)))
	t.ClientId = int32((uint32(bs[4]) | (uint32(bs[5]) << 8) | (uint32(bs[6]) << 16) | (uint32(bs[7]) << 24)))
	t.CommandId = int32((uint32(bs[8]) | (uint32(bs[9]) << 8) | (uint32(bs[10]) << 16) | (uint32(bs[11]) << 24)))
	t.Command.Unmarshal(wire)
	bs = b[:8]
	if _, err := io.ReadAtLeast(wire, bs, 8); err != nil {
		return err
	}
	t.Timestamp = int64((uint64(bs[0]) | (uint64(bs[1]) << 8) | (uint64(bs[2]) << 16) | (uint64(bs[3]) << 24) | (uint64(bs[4]) << 32) | (uint64(bs[5]) << 40) | (uint64(bs[6]) << 48) | (uint64(bs[7]) << 56)))
	return nil
}

// MSlotSync serialization — fixed 8 bytes: Replica(4) + Term(4)

func (t *MSlotSync) BinarySize() (nbytes int, sizeKnown bool) {
	return 8, true
}

type MSlotSyncCache struct {
	mu    sync.Mutex
	cache []*MSlotSync
}

func NewMSlotSyncCache() *MSlotSyncCache {
	c := &MSlotSyncCache{}
	c.cache = make([]*MSlotSync, 0)
	return c
}

func (p *MSlotSyncCache) Get() *MSlotSync {
	var t *MSlotSync
	p.mu.Lock()
	if len(p.cache) > 0 {
		t = p.cache[len(p.cache)-1]
		p.cache = p.cache[0:(len(p.cache) - 1)]
	}
	p.mu.Unlock()
	if t == nil {
		t = &MSlotSync{}
	}
	return t
}

func (p *MSlotSyncCache) Put(t *MSlotSync) {
	p.mu.Lock()
	p.cache = append(p.cache, t)
	p.mu.Unlock()
}

func (t *MSlotSync) Marshal(wire io.Writer) {
	var b [8]byte
	var bs []byte
	bs = b[:8]
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
	wire.Write(bs)
}

func (t *MSlotSync) Unmarshal(wire io.Reader) error {
	var b [8]byte
	var bs []byte
	bs = b[:8]
	if _, err := io.ReadAtLeast(wire, bs, 8); err != nil {
		return err
	}
	t.Replica = int32((uint32(bs[0]) | (uint32(bs[1]) << 8) | (uint32(bs[2]) << 16) | (uint32(bs[3]) << 24)))
	t.Term = int32((uint32(bs[4]) | (uint32(bs[5]) << 8) | (uint32(bs[6]) << 16) | (uint32(bs[7]) << 24)))
	return nil
}

// MSlotSyncReply serialization — fixed 12 bytes: Replica(4) + Term(4) + LastCommitted(4)

func (t *MSlotSyncReply) BinarySize() (nbytes int, sizeKnown bool) {
	return 12, true
}

type MSlotSyncReplyCache struct {
	mu    sync.Mutex
	cache []*MSlotSyncReply
}

func NewMSlotSyncReplyCache() *MSlotSyncReplyCache {
	c := &MSlotSyncReplyCache{}
	c.cache = make([]*MSlotSyncReply, 0)
	return c
}

func (p *MSlotSyncReplyCache) Get() *MSlotSyncReply {
	var t *MSlotSyncReply
	p.mu.Lock()
	if len(p.cache) > 0 {
		t = p.cache[len(p.cache)-1]
		p.cache = p.cache[0:(len(p.cache) - 1)]
	}
	p.mu.Unlock()
	if t == nil {
		t = &MSlotSyncReply{}
	}
	return t
}

func (p *MSlotSyncReplyCache) Put(t *MSlotSyncReply) {
	p.mu.Lock()
	p.cache = append(p.cache, t)
	p.mu.Unlock()
}

func (t *MSlotSyncReply) Marshal(wire io.Writer) {
	var b [12]byte
	var bs []byte
	bs = b[:12]
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
	tmp32 = t.LastCommitted
	bs[8] = byte(tmp32)
	bs[9] = byte(tmp32 >> 8)
	bs[10] = byte(tmp32 >> 16)
	bs[11] = byte(tmp32 >> 24)
	wire.Write(bs)
}

func (t *MSlotSyncReply) Unmarshal(wire io.Reader) error {
	var b [12]byte
	var bs []byte
	bs = b[:12]
	if _, err := io.ReadAtLeast(wire, bs, 12); err != nil {
		return err
	}
	t.Replica = int32((uint32(bs[0]) | (uint32(bs[1]) << 8) | (uint32(bs[2]) << 16) | (uint32(bs[3]) << 24)))
	t.Term = int32((uint32(bs[4]) | (uint32(bs[5]) << 8) | (uint32(bs[6]) << 16) | (uint32(bs[7]) << 24)))
	t.LastCommitted = int32((uint32(bs[8]) | (uint32(bs[9]) << 8) | (uint32(bs[10]) << 16) | (uint32(bs[11]) << 24)))
	return nil
}
