package pileus

import (
	"log"
	"sync"
	"sync/atomic"

	"github.com/imdea-software/swiftpaxos/client"
	"github.com/imdea-software/swiftpaxos/replica/defs"
	raftht "github.com/imdea-software/swiftpaxos/raft-ht"
	fastrpc "github.com/imdea-software/swiftpaxos/rpc"
	"github.com/imdea-software/swiftpaxos/state"
)

// Client implements the HybridClient interface for Pileus.
// All writes are forced to the strong (Raft consensus) path.
// Weak reads use causal tracking: the client tracks the last strong write's
// log index and sends it as MinIndex in weak read requests.
type Client struct {
	*client.BufferClient

	cs          raftht.CommunicationSupply
	leader      int32
	numReplicas int32

	// Weak command tracking (for weak reads only)
	weakPending map[int32]struct{}
	delivered   map[int32]struct{}

	// Per-command key tracking
	weakPendingKeys   map[int32]int64
	strongPendingKeys map[int32]int64
	strongPendingCmds map[int32]*defs.Propose

	deadReplicas map[int32]bool

	// Causal tracking: last strong write log index (for MinIndex in weak reads)
	lastWriteSlot int32 // atomic

	mu sync.Mutex
}

// NewClient creates a Pileus client.
func NewClient(b *client.BufferClient) *Client {
	c := &Client{
		BufferClient: b,

		leader:      int32(b.LeaderId),
		numReplicas: int32(b.NumReplicas()),

		weakPending: make(map[int32]struct{}),
		delivered:   make(map[int32]struct{}),

		weakPendingKeys:   make(map[int32]int64),
		strongPendingKeys: make(map[int32]int64),
		strongPendingCmds: make(map[int32]*defs.Propose),
		deadReplicas:      make(map[int32]bool),
	}

	t := fastrpc.NewTableId(defs.RPC_TABLE)
	raftht.InitClientCs(&c.cs, t)
	c.RegisterRPCTable(t)

	go c.handleMsgs()

	return c
}

func (c *Client) handleMsgs() {
	for {
		select {
		case m := <-c.cs.RaftReplyChan:
			rep := m.(*raftht.RaftReply)
			c.handleRaftReply(rep)

		case m := <-c.cs.WeakReadReplyChan:
			rep := m.(*raftht.MWeakReadReply)
			c.handleWeakReadReply(rep)

		case deadReplica := <-c.ReaderDead:
			c.handleReaderDead(int32(deadReplica))
		}
	}
}

func (c *Client) handleRaftReply(rep *raftht.RaftReply) {
	c.mu.Lock()
	defer c.mu.Unlock()

	seqnum := rep.CmdId.SeqNum

	if rep.LeaderId >= 0 && rep.LeaderId != c.leader {
		if !c.deadReplicas[rep.LeaderId] {
			c.leader = rep.LeaderId
			c.LeaderId = int(rep.LeaderId)
		}
		if cmd, ok := c.strongPendingCmds[seqnum]; ok {
			c.SendProposal(*cmd)
		}
		return
	}

	delete(c.strongPendingCmds, seqnum)
	delete(c.strongPendingKeys, seqnum)
	c.RegisterReply(rep.Value, seqnum)
}

func (c *Client) handleWeakReadReply(rep *raftht.MWeakReadReply) {
	c.mu.Lock()
	defer c.mu.Unlock()

	seqnum := rep.CmdId.SeqNum
	delete(c.weakPending, seqnum)
	delete(c.weakPendingKeys, seqnum)

	if _, ok := c.delivered[seqnum]; !ok {
		c.delivered[seqnum] = struct{}{}
		c.RegisterReply(rep.Rep, seqnum)
	}
}

func (c *Client) handleReaderDead(deadReplica int32) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.deadReplicas[deadReplica] = true

	if deadReplica == c.leader {
		oldLeader := c.leader
		c.leader = c.rotateLeader(c.leader)
		c.LeaderId = int(c.leader)
		log.Printf("Leader %d dead (EOF), rotating to %d", oldLeader, c.leader)
	}
}

func (c *Client) rotateLeader(current int32) int32 {
	for i := int32(1); i < c.numReplicas; i++ {
		next := (current + i) % c.numReplicas
		if !c.deadReplicas[next] {
			return next
		}
	}
	return (current + 1) % c.numReplicas
}

// --- HybridClient interface ---

func (c *Client) SendStrongWrite(key int64, value []byte) int32 {
	seqnum := c.GetNextSeqnum()
	p := defs.Propose{
		CommandId: seqnum,
		ClientId:  c.ClientId,
		Command:   state.Command{Op: state.PUT, K: state.Key(key), V: value},
		Timestamp: 0,
	}
	c.mu.Lock()
	c.strongPendingCmds[seqnum] = &p
	c.strongPendingKeys[seqnum] = key
	c.mu.Unlock()
	c.SendProposal(p)
	return seqnum
}

func (c *Client) SendStrongRead(key int64) int32 {
	seqnum := c.GetNextSeqnum()
	p := defs.Propose{
		CommandId: seqnum,
		ClientId:  c.ClientId,
		Command:   state.Command{Op: state.GET, K: state.Key(key), V: state.NIL()},
		Timestamp: 0,
	}
	c.mu.Lock()
	c.strongPendingCmds[seqnum] = &p
	c.strongPendingKeys[seqnum] = key
	c.mu.Unlock()
	c.SendProposal(p)
	return seqnum
}

// SendWeakWrite forces all writes to the strong path (Raft consensus).
// This is the key Pileus difference from MongoDB-Tunable.
func (c *Client) SendWeakWrite(key int64, value []byte) int32 {
	return c.SendStrongWrite(key, value)
}

func (c *Client) SendWeakRead(key int64) int32 {
	seqnum := c.GetNextSeqnum()
	minIdx := atomic.LoadInt32(&c.lastWriteSlot)
	wr := &raftht.MWeakRead{
		CommandId: seqnum,
		ClientId:  c.ClientId,
		Key:       state.Key(key),
		MinIndex:  minIdx,
	}
	c.mu.Lock()
	c.weakPending[seqnum] = struct{}{}
	c.weakPendingKeys[seqnum] = key
	c.mu.Unlock()
	closest := int32(c.ClosestId)
	c.SendMsg(closest, c.cs.WeakReadRPC, wr)
	return seqnum
}

func (c *Client) SendWeakScan(key int64, count int64) int32 {
	seqnum := c.GetNextSeqnum()
	minIdx := atomic.LoadInt32(&c.lastWriteSlot)
	wr := &raftht.MWeakRead{
		CommandId: seqnum,
		ClientId:  c.ClientId,
		Key:       state.Key(key),
		MinIndex:  minIdx,
		Op:        uint8(state.SCAN),
		Count:     count,
	}
	c.mu.Lock()
	c.weakPending[seqnum] = struct{}{}
	c.weakPendingKeys[seqnum] = key
	c.mu.Unlock()
	closest := int32(c.ClosestId)
	c.SendMsg(closest, c.cs.WeakReadRPC, wr)
	return seqnum
}

func (c *Client) SupportsWeak() bool { return true }
func (c *Client) MarkAllSent()       {}
