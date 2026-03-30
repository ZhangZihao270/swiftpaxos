package mongotunable

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

// Client implements the HybridClient interface for MongoDB-Tunable.
// Extends Raft-HT's client with causal tracking: tracks the last weak write's
// log index and sends it as MinIndex in weak read requests.
type Client struct {
	*client.BufferClient

	cs          raftht.CommunicationSupply
	leader      int32
	numReplicas int32

	// Weak command tracking
	weakPending map[int32]struct{}
	delivered   map[int32]struct{}

	// Per-command key tracking
	weakPendingKeys   map[int32]int64
	weakPendingValues map[int32]state.Value
	strongPendingKeys map[int32]int64
	strongPendingCmds map[int32]*defs.Propose

	deadReplicas map[int32]bool

	// Causal tracking: last weak write log index (for MinIndex in weak reads)
	lastWeakWriteSlot int32 // atomic

	mu sync.Mutex
}

// NewClient creates a MongoDB-Tunable client with causal tracking.
func NewClient(b *client.BufferClient) *Client {
	c := &Client{
		BufferClient: b,

		leader:      int32(b.LeaderId),
		numReplicas: int32(b.NumReplicas()),

		weakPending: make(map[int32]struct{}),
		delivered:   make(map[int32]struct{}),

		weakPendingKeys:   make(map[int32]int64),
		weakPendingValues: make(map[int32]state.Value),
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

		case m := <-c.cs.WeakReplyChan:
			rep := m.(*raftht.MWeakReply)
			c.handleWeakReply(rep)

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

func (c *Client) handleWeakReply(rep *raftht.MWeakReply) {
	c.mu.Lock()
	defer c.mu.Unlock()

	seqnum := rep.CmdId.SeqNum

	if rep.LeaderId >= 0 && rep.LeaderId != c.leader && rep.Slot < 0 {
		if !c.deadReplicas[rep.LeaderId] {
			c.leader = rep.LeaderId
			c.LeaderId = int(rep.LeaderId)
		}
		return
	}

	delete(c.weakPending, seqnum)

	// Causal tracking: update last weak write slot
	if rep.Slot >= 0 {
		for {
			old := atomic.LoadInt32(&c.lastWeakWriteSlot)
			if rep.Slot <= old {
				break
			}
			if atomic.CompareAndSwapInt32(&c.lastWeakWriteSlot, old, rep.Slot) {
				break
			}
		}
	}

	val := state.NIL()
	if v, ok := c.weakPendingValues[seqnum]; ok {
		val = v
		delete(c.weakPendingValues, seqnum)
	}
	delete(c.weakPendingKeys, seqnum)

	if _, ok := c.delivered[seqnum]; !ok {
		c.delivered[seqnum] = struct{}{}
		c.RegisterReply(val, seqnum)
	}
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

func (c *Client) SendWeakWrite(key int64, value []byte) int32 {
	seqnum := c.GetNextSeqnum()
	wp := &raftht.MWeakPropose{
		CommandId: seqnum,
		ClientId:  c.ClientId,
		Command:   state.Command{Op: state.PUT, K: state.Key(key), V: value},
	}
	c.mu.Lock()
	c.weakPending[seqnum] = struct{}{}
	c.weakPendingKeys[seqnum] = key
	c.weakPendingValues[seqnum] = value
	c.mu.Unlock()
	c.SendMsg(c.leader, c.cs.WeakProposeRPC, wp)
	return seqnum
}

func (c *Client) SendWeakRead(key int64) int32 {
	seqnum := c.GetNextSeqnum()
	minIdx := atomic.LoadInt32(&c.lastWeakWriteSlot)
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
	minIdx := atomic.LoadInt32(&c.lastWeakWriteSlot)
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
