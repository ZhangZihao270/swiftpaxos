package pileusht

import (
	"log"
	"sync"

	"github.com/imdea-software/swiftpaxos/client"
	"github.com/imdea-software/swiftpaxos/replica/defs"
	raftht "github.com/imdea-software/swiftpaxos/raft-ht"
	fastrpc "github.com/imdea-software/swiftpaxos/rpc"
	"github.com/imdea-software/swiftpaxos/state"
)

// cacheEntry holds a recently written value for read-your-writes merging.
type cacheEntry struct {
	Value    state.Value
	LogIndex int32 // log index assigned by leader
}

// Client implements the HybridClient interface for Pileus-HT v2.
// Weak writes get fast leader reply (like Raft-HT). Weak reads use
// client-side cache merge for read-your-writes guarantee (~0ms penalty)
// instead of the MinIndex follower wait approach (~50ms penalty).
//
// Cache merge: on weak write reply, client caches {key → (value, logIndex)}.
// On weak read reply, client compares follower's Version with cache's LogIndex;
// if cache is newer, returns cached value (read-your-writes). Otherwise returns
// follower's reply (follower has caught up).
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

	// Per-command key tracking for weak reads (for cache merge)
	weakReadKeys map[int32]int64

	deadReplicas map[int32]bool

	// Client-side write cache for read-your-writes (key → most recent write)
	writeCache map[int64]cacheEntry

	mu sync.Mutex
}

// NewClient creates a Pileus-HT v2 client with cache-based causal tracking.
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
		weakReadKeys:      make(map[int32]int64),
		deadReplicas:      make(map[int32]bool),
		writeCache:        make(map[int64]cacheEntry),
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

	// Cache the write result for read-your-writes merge
	if rep.Slot >= 0 {
		if key, ok := c.weakPendingKeys[seqnum]; ok {
			if val, vok := c.weakPendingValues[seqnum]; vok {
				existing, exists := c.writeCache[key]
				if !exists || rep.Slot > existing.LogIndex {
					c.writeCache[key] = cacheEntry{
						Value:    val,
						LogIndex: rep.Slot,
					}
				}
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

	// Cache merge: if client has a newer cached write for this key, use it
	result := rep.Rep
	if key, ok := c.weakReadKeys[seqnum]; ok {
		if cached, cok := c.writeCache[key]; cok {
			if cached.LogIndex > rep.Version {
				// Client's own write is newer than follower's state
				result = cached.Value
			} else {
				// Follower has caught up — evict cache entry
				delete(c.writeCache, key)
			}
		}
		delete(c.weakReadKeys, seqnum)
	}
	delete(c.weakPendingKeys, seqnum)

	if _, ok := c.delivered[seqnum]; !ok {
		c.delivered[seqnum] = struct{}{}
		c.RegisterReply(result, seqnum)
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

// SendWeakWrite sends a weak write to the leader for fast reply.
// The reply includes a log index (Slot) which is cached for read-your-writes.
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

// SendWeakRead sends a weak read to the closest replica without MinIndex.
// Read-your-writes is guaranteed via client-side cache merge, not follower wait.
func (c *Client) SendWeakRead(key int64) int32 {
	seqnum := c.GetNextSeqnum()
	wr := &raftht.MWeakRead{
		CommandId: seqnum,
		ClientId:  c.ClientId,
		Key:       state.Key(key),
		MinIndex:  0, // No follower wait — use cache merge instead
	}
	c.mu.Lock()
	c.weakPending[seqnum] = struct{}{}
	c.weakPendingKeys[seqnum] = key
	c.weakReadKeys[seqnum] = key
	c.mu.Unlock()
	closest := int32(c.ClosestId)
	c.SendMsg(closest, c.cs.WeakReadRPC, wr)
	return seqnum
}

func (c *Client) SendWeakScan(key int64, count int64) int32 {
	// TODO: implement proper weak scan in Phase 126 Step 3
	return c.SendWeakRead(key)
}

func (c *Client) SupportsWeak() bool { return true }
func (c *Client) MarkAllSent()       {}
