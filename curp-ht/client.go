package curpht

import (
	"sync"

	"github.com/imdea-software/swiftpaxos/client"
	"github.com/imdea-software/swiftpaxos/replica"
	"github.com/imdea-software/swiftpaxos/replica/defs"
	fastrpc "github.com/imdea-software/swiftpaxos/rpc"
	"github.com/imdea-software/swiftpaxos/state"
)

// cacheEntry stores a value and its version (slot number) for the client local cache
type cacheEntry struct {
	value   state.Value
	version int32
}

type Client struct {
	*client.BufferClient

	acks  map[CommandId]*replica.MsgSet
	macks map[CommandId]*replica.MsgSet

	N         int
	t         *Timer
	Q         replica.ThreeQuarters
	M         replica.Majority
	cs        CommunicationSupply
	num       int
	val       state.Value
	leader    int32
	ballot    int32
	delivered map[int32]struct{}

	lastCmdId CommandId

	slowPaths   int
	alreadySlow map[CommandId]struct{}

	// Weak command tracking
	weakPending         map[int32]struct{}
	lastWeakWriteSeqNum int32 // Track sequence number of last weak WRITE for causal ordering

	// Per-command key tracking for cache updates
	weakPendingKeys   map[int32]int64       // seqnum → key (for weak writes and reads)
	weakPendingValues map[int32]state.Value  // seqnum → value (for weak writes)
	strongPendingKeys map[int32]int64        // seqnum → key (for strong ops)

	// Client local cache: key → (value, version) with slot-based versioning
	localCache map[int64]cacheEntry
	maxVersion int32 // highest version seen (for strong op cache versioning)

	// Slot from last leader MReply (for strong fast-path cache update)
	lastReplySlot int32

	// Mutex for concurrent map access (needed for pipelining)
	mu sync.Mutex

	// Weak write pipelining: track in-flight weak writes awaiting leader commit
	pendingWeakCommits map[int32]struct{}
	weakCommitCond     *sync.Cond
}

var (
	m         sync.Mutex
	clientNum int
)

// pclients - Number of clients already running on other machines
// This is needed to generate a new key for each new request
func NewClient(b *client.BufferClient, repNum, reqNum, pclients int) *Client {
	m.Lock()
	num := clientNum
	clientNum++
	m.Unlock()

	c := &Client{
		BufferClient: b,

		N:   repNum,
		t:   NewTimer(),
		Q:   replica.NewThreeQuartersOf(repNum),
		M:   replica.NewMajorityOf(repNum),
		num: num,
		val: nil,

		leader:    0, // Default to replica 0 (leader when no quorum file, since ballot=0=Id)
		ballot:    -1,
		delivered: make(map[int32]struct{}),

		acks:  make(map[CommandId]*replica.MsgSet),
		macks: make(map[CommandId]*replica.MsgSet),

		slowPaths:   0,
		alreadySlow: make(map[CommandId]struct{}),

		weakPending:       make(map[int32]struct{}),
		weakPendingKeys:   make(map[int32]int64),
		weakPendingValues: make(map[int32]state.Value),
		strongPendingKeys: make(map[int32]int64),
		localCache:         make(map[int64]cacheEntry),
		pendingWeakCommits: make(map[int32]struct{}),
	}

	c.weakCommitCond = sync.NewCond(&c.mu)

	c.lastCmdId = CommandId{
		ClientId: c.ClientId,
		SeqNum:   0,
	}

	t := fastrpc.NewTableId(defs.RPC_TABLE)
	initCs(&c.cs, t)
	c.RegisterRPCTable(t)

	// Generate a new key for each new request
	if pclients != -1 {
		i := 0
		c.GetClientKey = func() int64 {
			k := 100 + i + (reqNum * (c.num + pclients))
			i++
			return int64(k)
		}
	}

	go c.handleMsgs()

	return c
}

func (c *Client) initMsgSets(cmdId CommandId) {
	m, exists := c.acks[cmdId]
	initAcks := !exists || m == nil
	m, exists = c.macks[cmdId]
	initMacks := !exists || m == nil

	accept := func(_, _ interface{}) bool {
		return true
	}

	if initAcks {
		c.acks[cmdId] = c.acks[cmdId].ReinitMsgSet(c.Q, accept, func(interface{}) {}, c.handleAcks)
	}
	if initMacks {
		c.macks[cmdId] = c.macks[cmdId].ReinitMsgSet(c.M, accept, func(interface{}) {}, c.handleAcks)
	}
}

func (c *Client) handleMsgs() {
	for {
		select {
		case m := <-c.cs.replyChan:
			rep := m.(*MReply)
			c.handleReply(rep)

		case m := <-c.cs.recordAckChan:
			recAck := m.(*MRecordAck)
			c.handleRecordAck(recAck, false)

		case m := <-c.cs.syncReplyChan:
			rep := m.(*MSyncReply)
			c.handleSyncReply(rep)

		case m := <-c.cs.weakReplyChan:
			rep := m.(*MWeakReply)
			c.handleWeakReply(rep)

		case m := <-c.cs.weakReadReplyChan:
			rep := m.(*MWeakReadReply)
			c.handleWeakReadReply(rep)

		case <-c.t.c:
			// Timer-triggered sync intentionally disabled (see CURP paper §4.2).
			// The slow path via SyncReply handles retransmission.
			break
		}
	}
}

func (c *Client) handleReply(r *MReply) {
	c.mu.Lock()
	if _, exists := c.delivered[r.CmdId.SeqNum]; exists {
		c.mu.Unlock()
		return
	}
	c.lastReplySlot = r.Slot
	c.mu.Unlock()

	ack := &MRecordAck{
		Replica: r.Replica,
		Ballot:  r.Ballot,
		CmdId:   r.CmdId,
		Ok:      r.Ok,
	}
	c.val = state.Value(r.Rep)
	c.handleRecordAck(ack, true)
}

func (c *Client) handleRecordAck(r *MRecordAck, fromLeader bool) {
	c.mu.Lock()
	if _, exists := c.delivered[r.CmdId.SeqNum]; exists {
		c.mu.Unlock()
		return
	}
	c.mu.Unlock()

	if c.ballot == -1 {
		c.ballot = r.Ballot
	} else if c.ballot < r.Ballot {
		c.ballot = r.Ballot
	} else if c.ballot > r.Ballot {
		return
	}

	if fromLeader {
		c.leader = r.Replica
	}

	if fromLeader || r.Ok == ORDERED {
		c.initMsgSets(r.CmdId)
		c.macks[r.CmdId].Add(r.Replica, fromLeader, r)
	}

	if r.Ok == TRUE {
		c.initMsgSets(r.CmdId)
		c.acks[r.CmdId].Add(r.Replica, fromLeader, r)
	}
}

func (c *Client) handleSyncReply(rep *MSyncReply) {
	c.mu.Lock()
	if _, exists := c.delivered[rep.CmdId.SeqNum]; exists {
		c.mu.Unlock()
		return
	}

	if c.ballot == -1 {
		c.ballot = rep.Ballot
	} else if c.ballot < rep.Ballot {
		c.ballot = rep.Ballot
	} else if c.ballot > rep.Ballot {
		c.mu.Unlock()
		return
	}

	c.val = state.Value(rep.Rep)
	c.delivered[rep.CmdId.SeqNum] = struct{}{}

	// Update local cache from strong slow-path result
	if key, hasKey := c.strongPendingKeys[rep.CmdId.SeqNum]; hasKey {
		ver := rep.Slot
		if ver == 0 {
			c.maxVersion++
			ver = c.maxVersion
		}
		if ver > c.maxVersion {
			c.maxVersion = ver
		}
		c.localCache[key] = cacheEntry{value: c.val, version: ver}
		delete(c.strongPendingKeys, rep.CmdId.SeqNum)
	}

	c.mu.Unlock()
	c.RegisterReply(c.val, rep.CmdId.SeqNum)
	c.Println("Slow Paths:", c.slowPaths)
}

func (c *Client) handleAcks(leaderMsg interface{}, msgs []interface{}) {
	if leaderMsg == nil {
		return
	}

	seqNum := leaderMsg.(*MRecordAck).CmdId.SeqNum

	c.mu.Lock()
	if _, exists := c.delivered[seqNum]; exists {
		c.mu.Unlock()
		return
	}

	c.delivered[seqNum] = struct{}{}

	// Update local cache from strong fast-path result
	if key, hasKey := c.strongPendingKeys[seqNum]; hasKey {
		ver := c.lastReplySlot
		if ver == 0 {
			c.maxVersion++
			ver = c.maxVersion
		}
		if ver > c.maxVersion {
			c.maxVersion = ver
		}
		c.localCache[key] = cacheEntry{value: c.val, version: ver}
		delete(c.strongPendingKeys, seqNum)
	}

	c.mu.Unlock()
	c.RegisterReply(c.val, seqNum)
	c.Println("Slow Paths:", c.slowPaths)
}

// handleWeakReply handles weak write commit reply from leader (post-commit, includes Slot).
// Since pipelined weak writes already called RegisterReply in SendWeakWrite,
// this handler only updates the cache and signals the barrier condition variable.
func (c *Client) handleWeakReply(rep *MWeakReply) {
	c.mu.Lock()

	// Dedup: if not in pendingWeakCommits, already processed or unknown
	if _, pending := c.pendingWeakCommits[rep.CmdId.SeqNum]; !pending {
		c.mu.Unlock()
		return
	}

	// Update ballot if needed
	if c.ballot == -1 {
		c.ballot = rep.Ballot
	} else if c.ballot < rep.Ballot {
		c.ballot = rep.Ballot
	} else if c.ballot > rep.Ballot {
		c.mu.Unlock()
		return
	}

	// Update leader (reply always comes from leader)
	c.leader = rep.Replica

	// Update local cache with committed write value + real slot version
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

	// Mark as no longer pending commit — signal waitPendingWeakCommits
	delete(c.weakPending, rep.CmdId.SeqNum)
	delete(c.pendingWeakCommits, rep.CmdId.SeqNum)
	c.weakCommitCond.Broadcast()
	c.mu.Unlock()
	// Note: RegisterReply was already called in SendWeakWrite (pipelining)
}

// handleWeakReadReply handles weak read reply from nearest replica
// Merges replica response with local cache using max-version rule
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

// SendWeakWrite sends a weak consistency write operation to leader only.
// The write is pipelined: RegisterReply is called immediately so the benchmark
// loop can proceed to the next command without waiting for the leader's commit.
// The leader commit is tracked in pendingWeakCommits; strong ops barrier-wait
// for all pending weak commits before proceeding (correctness guarantee).
func (c *Client) SendWeakWrite(key int64, value []byte) int32 {
	seqnum := c.getNextSeqnum()

	c.mu.Lock()
	causalDep := c.lastWeakWriteSeqNum
	c.weakPending[seqnum] = struct{}{}
	c.weakPendingKeys[seqnum] = key
	c.weakPendingValues[seqnum] = value
	c.lastWeakWriteSeqNum = seqnum
	c.pendingWeakCommits[seqnum] = struct{}{}
	// Optimistic cache update with provisional version
	c.localCache[key] = cacheEntry{value: value, version: c.maxVersion + 1}
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
		Timestamp: 0,
		CausalDep: causalDep, // Depend on previous weak write
	}

	// Send only to leader
	if leader != -1 {
		c.SendMsg(leader, c.cs.weakProposeRPC, p)
	}

	// Pipeline: complete immediately from the benchmark's perspective
	c.RegisterReply(state.NIL(), seqnum)
	return seqnum
}

// SendWeakRead sends a weak consistency read to the nearest replica
// Returns (value, version), client merges with local cache
func (c *Client) SendWeakRead(key int64) int32 {
	seqnum := c.getNextSeqnum()

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

	// Send to nearest replica (not leader)
	if closest != -1 {
		c.SendMsg(int32(closest), c.cs.weakReadRPC, msg)
	}
	return seqnum
}

// getNextSeqnum returns the next sequence number from the base client.
// This ensures weak commands use the same seqnum space as strong commands,
// preventing conflicts when mixing strong and weak commands in HybridLoop.
func (c *Client) getNextSeqnum() int32 {
	// Use the base client's seqnum to share the same sequence space
	// GetNextSeqnum is promoted through BufferClient -> Client embedding
	seqnum := c.BufferClient.GetNextSeqnum()
	c.lastCmdId.SeqNum = seqnum
	return seqnum
}

// waitPendingWeakCommits blocks until all pipelined weak writes have been
// committed by the leader. Must be called before any strong op to ensure
// session ordering: strong ops must see all prior weak writes from the same session.
func (c *Client) waitPendingWeakCommits() {
	c.mu.Lock()
	for len(c.pendingWeakCommits) > 0 {
		c.weakCommitCond.Wait()
	}
	c.mu.Unlock()
}

// HybridClient interface implementation

// SendStrongWrite sends a linearizable write command (delegates to base SendWrite).
// Tracks the key for local cache updates on completion.
// Barrier-waits for all pipelined weak writes to commit first.
func (c *Client) SendStrongWrite(key int64, value []byte) int32 {
	c.waitPendingWeakCommits()
	seqnum := c.SendWrite(key, value)
	c.mu.Lock()
	c.strongPendingKeys[seqnum] = key
	c.mu.Unlock()
	return seqnum
}

// SendStrongRead sends a linearizable read command (delegates to base SendRead).
// Tracks the key for local cache updates on completion.
// Barrier-waits for all pipelined weak writes to commit first.
func (c *Client) SendStrongRead(key int64) int32 {
	c.waitPendingWeakCommits()
	seqnum := c.SendRead(key)
	c.mu.Lock()
	c.strongPendingKeys[seqnum] = key
	c.mu.Unlock()
	return seqnum
}

// SupportsWeak returns true since curp-ht supports weak consistency commands.
func (c *Client) SupportsWeak() bool {
	return true
}

func (c *Client) MarkAllSent() {}
