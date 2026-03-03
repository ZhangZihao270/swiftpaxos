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
// Strong ops use the base WaitReplies mechanism (same as vanilla Raft).
// Weak writes go to leader (1-RTT early reply). Weak reads go to nearest replica.
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
	strongPendingKeys map[int32]int64      // seqnum → key (for strong ops)

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

	// Register weak message types with RPC table
	t := fastrpc.NewTableId(defs.RPC_TABLE)
	initCs(&c.cs, t)
	c.RegisterRPCTable(t)

	// Start reading strong replies from leader (ProposeReplyTS via base writer)
	c.WaitReplies(int(b.LeaderId))

	// Start handling weak messages
	go c.handleMsgs()

	return c
}

func (c *Client) handleMsgs() {
	for {
		select {
		case m := <-c.cs.weakReplyChan:
			rep := m.(*MWeakReply)
			c.handleWeakReply(rep)

		case m := <-c.cs.weakReadReplyChan:
			rep := m.(*MWeakReadReply)
			c.handleWeakReadReply(rep)
		}
	}
}

// handleWeakReply handles weak write reply from leader (immediate, before commit)
func (c *Client) handleWeakReply(rep *MWeakReply) {
	c.mu.Lock()
	if _, exists := c.delivered[rep.CmdId.SeqNum]; exists {
		c.mu.Unlock()
		return
	}

	// Update leader
	c.leader = rep.LeaderId

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
