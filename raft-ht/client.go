package raftht

import (
	"sync"

	"github.com/imdea-software/swiftpaxos/client"
	"github.com/imdea-software/swiftpaxos/replica/defs"
	fastrpc "github.com/imdea-software/swiftpaxos/rpc"
	"github.com/imdea-software/swiftpaxos/state"
)

// cacheEntry stores a value and its version (slot number) for the client local cache
type cacheEntry struct {
	value   state.Value
	version int32
}

// Client implements the HybridClient interface for the Raft-HT protocol.
// ALL replies (strong + weak) are received through the RPC table — no WaitReplies.
// Strong ops: leader sends RaftReply via SendToClient (type-prefixed).
// Weak writes: leader sends MWeakReply via SendToClient (1-RTT early reply).
// Weak reads: nearest replica sends MWeakReadReply via SendToClient (local).
type Client struct {
	*client.BufferClient

	cs     CommunicationSupply
	val    state.Value
	leader int32

	// Weak command tracking
	weakPending map[int32]struct{}
	delivered   map[int32]struct{}

	// Per-command key tracking for cache updates
	weakPendingKeys   map[int32]int64      // seqnum → key (for weak writes and reads)
	weakPendingValues map[int32]state.Value // seqnum → value (for weak writes)
	strongPendingKeys map[int32]int64       // seqnum → key (for strong ops)

	// Client local cache: key → (value, version) with slot-based versioning
	localCache map[int64]cacheEntry
	maxVersion int32 // highest version seen

	mu sync.Mutex
}

// NewClient creates a new Raft-HT client.
func NewClient(b *client.BufferClient) *Client {
	c := &Client{
		BufferClient: b,

		val:    nil,
		leader: 0, // Default leader = replica 0

		weakPending: make(map[int32]struct{}),
		delivered:   make(map[int32]struct{}),

		weakPendingKeys:   make(map[int32]int64),
		weakPendingValues: make(map[int32]state.Value),
		strongPendingKeys: make(map[int32]int64),
		localCache:        make(map[int64]cacheEntry),
	}

	// Register ALL message types (strong + weak) with a single RPC table.
	// This ensures a single reader goroutine per replica connection,
	// avoiding the data race from using both WaitReplies and RegisterRPCTable.
	t := fastrpc.NewTableId(defs.RPC_TABLE)
	initCs(&c.cs, t)
	c.RegisterRPCTable(t)

	// Handle ALL replies in a single goroutine
	go c.handleMsgs()

	return c
}

func (c *Client) handleMsgs() {
	for {
		select {
		// Strong op replies (from leader, after commit)
		case m := <-c.cs.raftReplyChan:
			rep := m.(*RaftReply)
			c.handleRaftReply(rep)

		// Weak write replies (from leader, immediate)
		case m := <-c.cs.weakReplyChan:
			rep := m.(*MWeakReply)
			c.handleWeakReply(rep)

		// Weak read replies (from nearest replica)
		case m := <-c.cs.weakReadReplyChan:
			rep := m.(*MWeakReadReply)
			c.handleWeakReadReply(rep)
		}
	}
}

// handleRaftReply handles strong op replies from the leader (after commit).
func (c *Client) handleRaftReply(rep *RaftReply) {
	c.mu.Lock()
	if _, exists := c.delivered[rep.CmdId.SeqNum]; exists {
		c.mu.Unlock()
		return
	}

	c.val = state.Value(rep.Value)
	c.delivered[rep.CmdId.SeqNum] = struct{}{}

	// Update local cache from strong op result
	if key, hasKey := c.strongPendingKeys[rep.CmdId.SeqNum]; hasKey {
		c.maxVersion++
		c.localCache[key] = cacheEntry{value: c.val, version: c.maxVersion}
		delete(c.strongPendingKeys, rep.CmdId.SeqNum)
	}

	c.mu.Unlock()
	c.RegisterReply(c.val, rep.CmdId.SeqNum)
}

// handleWeakReply handles weak write reply from leader (immediate, before commit)
func (c *Client) handleWeakReply(rep *MWeakReply) {
	c.mu.Lock()

	// Update leader hint (even on rejection, so we redirect to the right node)
	if rep.LeaderId >= 0 {
		c.leader = rep.LeaderId
	}

	// Slot == -1 means rejection (non-leader received the proposal).
	// Resend to the updated leader.
	if rep.Slot < 0 {
		leader := c.leader
		c.mu.Unlock()
		p := &MWeakPropose{
			CommandId: rep.CmdId.SeqNum,
			ClientId:  rep.CmdId.ClientId,
		}
		if key, hasKey := c.weakPendingKeys[rep.CmdId.SeqNum]; hasKey {
			p.Command.Op = state.PUT
			p.Command.K = state.Key(key)
			if val, hasVal := c.weakPendingValues[rep.CmdId.SeqNum]; hasVal {
				p.Command.V = val
			}
		}
		c.SendMsg(leader, c.cs.weakProposeRPC, p)
		return
	}

	if _, exists := c.delivered[rep.CmdId.SeqNum]; exists {
		c.mu.Unlock()
		return
	}

	// Update local cache with the write value + slot
	if key, hasKey := c.weakPendingKeys[rep.CmdId.SeqNum]; hasKey {
		if val, hasVal := c.weakPendingValues[rep.CmdId.SeqNum]; hasVal {
			c.localCache[key] = cacheEntry{value: val, version: rep.Slot}
			if rep.Slot > c.maxVersion {
				c.maxVersion = rep.Slot
			}
			delete(c.weakPendingValues, rep.CmdId.SeqNum)
		}
		delete(c.weakPendingKeys, rep.CmdId.SeqNum)
	}

	c.val = nil // weak write reply has no value payload (just confirmation + slot)
	c.delivered[rep.CmdId.SeqNum] = struct{}{}
	delete(c.weakPending, rep.CmdId.SeqNum)
	c.mu.Unlock()
	c.RegisterReply(c.val, rep.CmdId.SeqNum)
}

// handleWeakReadReply handles weak read reply from nearest replica.
// Merges replica response with local cache using max-version rule.
func (c *Client) handleWeakReadReply(rep *MWeakReadReply) {
	c.mu.Lock()
	if _, exists := c.delivered[rep.CmdId.SeqNum]; exists {
		c.mu.Unlock()
		return
	}

	// Merge with local cache
	key, _ := c.weakPendingKeys[rep.CmdId.SeqNum]
	replicaVal := state.Value(rep.Rep)
	replicaVer := rep.Version

	cached, hasCached := c.localCache[key]
	var finalVal state.Value
	var finalVer int32
	if hasCached && cached.version > replicaVer {
		// Cache has fresher value
		finalVal = cached.value
		finalVer = cached.version
	} else {
		// Replica has fresher or equal value
		finalVal = replicaVal
		finalVer = replicaVer
	}
	c.localCache[key] = cacheEntry{value: finalVal, version: finalVer}
	if finalVer > c.maxVersion {
		c.maxVersion = finalVer
	}

	c.val = finalVal
	c.delivered[rep.CmdId.SeqNum] = struct{}{}
	delete(c.weakPending, rep.CmdId.SeqNum)
	delete(c.weakPendingKeys, rep.CmdId.SeqNum)
	c.mu.Unlock()
	c.RegisterReply(c.val, rep.CmdId.SeqNum)
}

// SendWeakWrite sends a weak consistency write to the leader.
// Leader assigns log slot and replies immediately (1 WAN RTT).
func (c *Client) SendWeakWrite(key int64, value []byte) int32 {
	seqnum := c.BufferClient.GetNextSeqnum()

	c.mu.Lock()
	c.weakPending[seqnum] = struct{}{}
	c.weakPendingKeys[seqnum] = key
	c.weakPendingValues[seqnum] = value
	leader := c.leader
	c.mu.Unlock()

	p := &MWeakPropose{
		CommandId: seqnum,
		ClientId:  c.ClientId,
		Command: state.Command{
			Op: state.PUT,
			K:  state.Key(key),
			V:  value,
		},
	}

	if leader != -1 {
		c.SendMsg(leader, c.cs.weakProposeRPC, p)
	}
	return seqnum
}

// SendWeakRead sends a weak consistency read to the nearest replica.
// Returns (value, version), client merges with local cache.
func (c *Client) SendWeakRead(key int64) int32 {
	seqnum := c.BufferClient.GetNextSeqnum()

	c.mu.Lock()
	c.weakPending[seqnum] = struct{}{}
	c.weakPendingKeys[seqnum] = key
	closest := c.ClosestId
	c.mu.Unlock()

	msg := &MWeakRead{
		CommandId: seqnum,
		ClientId:  c.ClientId,
		Key:       state.Key(key),
	}

	if closest != -1 {
		c.SendMsg(int32(closest), c.cs.weakReadRPC, msg)
	}
	return seqnum
}

// SendStrongWrite sends a linearizable write command (delegates to base SendWrite).
// Tracks the key for local cache updates on completion.
func (c *Client) SendStrongWrite(key int64, value []byte) int32 {
	seqnum := c.SendWrite(key, value)
	c.mu.Lock()
	c.strongPendingKeys[seqnum] = key
	c.mu.Unlock()
	return seqnum
}

// SendStrongRead sends a linearizable read command (delegates to base SendRead).
// Tracks the key for local cache updates on completion.
func (c *Client) SendStrongRead(key int64) int32 {
	seqnum := c.SendRead(key)
	c.mu.Lock()
	c.strongPendingKeys[seqnum] = key
	c.mu.Unlock()
	return seqnum
}

// SupportsWeak returns true since Raft-HT supports weak consistency commands.
func (c *Client) SupportsWeak() bool {
	return true
}

// MarkAllSent is a no-op for Raft-HT.
func (c *Client) MarkAllSent() {}
