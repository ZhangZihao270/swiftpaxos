package curpho

import (
	"sync"
	"time"

	"github.com/imdea-software/swiftpaxos/client"
	"github.com/imdea-software/swiftpaxos/replica"
	"github.com/imdea-software/swiftpaxos/replica/defs"
	fastrpc "github.com/imdea-software/swiftpaxos/rpc"
	"github.com/imdea-software/swiftpaxos/state"
)

// cacheEntry stores a cached value and its version (slot number).
// Higher version = fresher data. Used for max-version merge on weak reads.
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

	// CURP-HO: Client write set — tracks uncommitted weak writes.
	// Entries added on SendCausalWrite, cleared on SyncReply (leader commit) or fast-path delivery.
	writeSet map[CommandId]struct{}

	// CURP-HO: Bound replica for 1-RTT causal op completion.
	// Client binds to closest replica (lowest latency) for fast causal replies.
	// Causal ops are broadcast to all replicas, but client only waits for
	// the bound replica's reply to complete in 1-RTT.
	boundReplica int32

	// Client local cache: key → (value, version) with slot-based versioning
	localCache        map[int64]cacheEntry
	weakPendingKeys   map[int32]int64       // seqNum → key (for weak writes and reads)
	weakPendingValues map[int32]state.Value  // seqNum → value (for weak writes)
	strongPendingKeys map[int32]int64       // seqNum → key (for strong ops)
	lastReplySlot     int32                 // slot from last leader MReply
	maxVersion        int32                 // highest version seen

	// Mutex for concurrent map access (needed for pipelining)
	mu sync.Mutex
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

		weakPending: make(map[int32]struct{}),
		writeSet:    make(map[CommandId]struct{}),

		// CURP-HO: Bind to closest replica for 1-RTT causal op completion.
		// ClosestId is computed by base client during Connect() via ping latency measurement.
		boundReplica: int32(b.ClosestId),

		// Client local cache
		localCache:        make(map[int64]cacheEntry),
		weakPendingKeys:   make(map[int32]int64),
		weakPendingValues: make(map[int32]state.Value),
		strongPendingKeys: make(map[int32]int64),
	}

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

	// Start MSync retry timer: periodically retransmit MSync for pending
	// strong commands whose replies may have been dropped by the non-blocking
	// SendClientMsgFast on the replica side.
	c.t.Start(2 * time.Second)

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
		c.acks[cmdId] = c.acks[cmdId].ReinitMsgSet(c.Q, accept, func(interface{}) {}, c.handleFastPathAcks)
	}
	if initMacks {
		c.macks[cmdId] = c.macks[cmdId].ReinitMsgSet(c.M, accept, func(interface{}) {}, c.handleSlowPathAcks)
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

		case m := <-c.cs.causalReplyChan:
			rep := m.(*MCausalReply)
			c.handleCausalReply(rep)

		case m := <-c.cs.weakReadReplyChan:
			rep := m.(*MWeakReadReply)
			c.handleWeakReadReply(rep)

		case <-c.t.c:
			// Retry MSync for pending commands whose replies may have
			// been dropped by the non-blocking SendClientMsgFast, or
			// whose delivery is stuck in slot ordering on the proxy.
			// Send to ALL replicas so any replica that has executed the
			// command can reply.
			c.mu.Lock()
			var pendingSeqnums []int32
			// Strong commands pending delivery
			for seqnum := range c.strongPendingKeys {
				if _, delivered := c.delivered[seqnum]; !delivered {
					pendingSeqnums = append(pendingSeqnums, seqnum)
				}
			}
			// Weak/causal commands pending delivery
			for seqnum := range c.weakPending {
				if _, delivered := c.delivered[seqnum]; !delivered {
					pendingSeqnums = append(pendingSeqnums, seqnum)
				}
			}
			clientId := c.ClientId
			n := c.N
			c.mu.Unlock()

			if len(pendingSeqnums) > 0 {
				c.Println("MSync retry:", len(pendingSeqnums), "pending commands")
			}
			for _, seqnum := range pendingSeqnums {
				sync := &MSync{
					CmdId: CommandId{ClientId: clientId, SeqNum: seqnum},
				}
				for r := int32(0); r < int32(n); r++ {
					c.SendMsg(r, c.cs.syncRPC, sync)
				}
			}
		}
	}
}

func (c *Client) handleReply(r *MReply) {
	c.mu.Lock()
	if _, exists := c.delivered[r.CmdId.SeqNum]; exists {
		c.mu.Unlock()
		return
	}
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
	// Clear write set entries up to this committed seqnum
	for ws := range c.writeSet {
		if ws.ClientId == c.ClientId && ws.SeqNum <= rep.CmdId.SeqNum {
			delete(c.writeSet, ws)
		}
	}
	// Update local cache from strong slow-path result
	if key, hasKey := c.strongPendingKeys[rep.CmdId.SeqNum]; hasKey {
		c.maxVersion++
		c.localCache[key] = cacheEntry{value: c.val, version: c.maxVersion}
		delete(c.strongPendingKeys, rep.CmdId.SeqNum)
	}
	// Clean up weak command tracking (MSync retry may deliver weak commands here)
	delete(c.weakPending, rep.CmdId.SeqNum)
	delete(c.weakPendingKeys, rep.CmdId.SeqNum)
	delete(c.weakPendingValues, rep.CmdId.SeqNum)
	c.mu.Unlock()
	c.RegisterReply(c.val, rep.CmdId.SeqNum)
	c.Println("Slow Paths:", c.slowPaths)
}

// handleFastPathAcks handles the fast path (super quorum) with two checks:
// 1. Causal dep check: all weak writes in client's write set must appear in
//    CausalDeps of every witness in the quorum.
// 2. ReadDep consistency: all witnesses must agree on the same ReadDep.
// If either check fails, fall back to the slow path (via macks).
func (c *Client) handleFastPathAcks(leaderMsg interface{}, msgs []interface{}) {
	if leaderMsg == nil {
		return
	}

	cmdId := leaderMsg.(*MRecordAck).CmdId

	// Check 1: Causal dependency check
	if !c.checkCausalDeps(msgs) {
		c.mu.Lock()
		if _, exists := c.alreadySlow[cmdId]; !exists {
			c.alreadySlow[cmdId] = struct{}{}
			c.slowPaths++
		}
		c.mu.Unlock()
		return
	}

	// Check 2: ReadDep consistency check
	if !c.checkReadDepConsistency(msgs) {
		c.mu.Lock()
		if _, exists := c.alreadySlow[cmdId]; !exists {
			c.alreadySlow[cmdId] = struct{}{}
			c.slowPaths++
		}
		c.mu.Unlock()
		return
	}

	// Both checks passed - deliver on fast path
	c.mu.Lock()
	if _, exists := c.delivered[cmdId.SeqNum]; exists {
		c.mu.Unlock()
		return
	}
	c.delivered[cmdId.SeqNum] = struct{}{}
	// Clear write set entries up to this delivered seqnum
	for ws := range c.writeSet {
		if ws.ClientId == c.ClientId && ws.SeqNum < cmdId.SeqNum {
			delete(c.writeSet, ws)
		}
	}
	// Update local cache from strong fast-path result
	if key, hasKey := c.strongPendingKeys[cmdId.SeqNum]; hasKey {
		c.maxVersion++
		c.localCache[key] = cacheEntry{value: c.val, version: c.maxVersion}
		delete(c.strongPendingKeys, cmdId.SeqNum)
	}
	c.mu.Unlock()
	c.RegisterReply(c.val, cmdId.SeqNum)
	c.Println("Slow Paths:", c.slowPaths)
}

// handleSlowPathAcks handles the slow path (majority quorum).
// No weakDep consistency check needed - the leader has ordered the command.
func (c *Client) handleSlowPathAcks(leaderMsg interface{}, msgs []interface{}) {
	if leaderMsg == nil {
		return
	}

	c.mu.Lock()
	if _, exists := c.delivered[leaderMsg.(*MRecordAck).CmdId.SeqNum]; exists {
		c.mu.Unlock()
		return
	}
	c.delivered[leaderMsg.(*MRecordAck).CmdId.SeqNum] = struct{}{}
	c.mu.Unlock()
	c.RegisterReply(c.val, leaderMsg.(*MRecordAck).CmdId.SeqNum)
	c.Println("Slow Paths:", c.slowPaths)
}

// handleWeakReply handles weak command reply from leader
func (c *Client) handleWeakReply(rep *MWeakReply) {
	c.mu.Lock()
	if _, exists := c.delivered[rep.CmdId.SeqNum]; exists {
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

	// Weak command completes immediately upon receiving leader's reply
	c.val = state.Value(rep.Rep)
	c.delivered[rep.CmdId.SeqNum] = struct{}{}
	delete(c.weakPending, rep.CmdId.SeqNum)

	// Update local cache with committed write value + slot (version)
	if key, hasKey := c.weakPendingKeys[rep.CmdId.SeqNum]; hasKey {
		if val, hasVal := c.weakPendingValues[rep.CmdId.SeqNum]; hasVal {
			c.maxVersion++
			c.localCache[key] = cacheEntry{value: val, version: c.maxVersion}
			delete(c.weakPendingValues, rep.CmdId.SeqNum)
		}
		delete(c.weakPendingKeys, rep.CmdId.SeqNum)
	}

	c.mu.Unlock()
	c.RegisterReply(c.val, rep.CmdId.SeqNum)
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

// HybridClient interface implementation

// SendStrongWrite sends a linearizable write command (delegates to base SendWrite).
func (c *Client) SendStrongWrite(key int64, value []byte) int32 {
	seqnum := c.SendWrite(key, value)
	c.mu.Lock()
	c.strongPendingKeys[seqnum] = key
	c.mu.Unlock()
	return seqnum
}

// SendStrongRead sends a linearizable read command (delegates to base SendRead).
func (c *Client) SendStrongRead(key int64) int32 {
	seqnum := c.SendRead(key)
	c.mu.Lock()
	c.strongPendingKeys[seqnum] = key
	c.mu.Unlock()
	return seqnum
}

// SupportsWeak returns true since curp-ho supports weak consistency commands.
func (c *Client) SupportsWeak() bool {
	return true
}

// SendWeakWrite for CURP-HO broadcasts MCausalPropose to ALL replicas.
// Overrides the CURP-HT version which only sends to leader.
// Client completes when it receives MCausalReply from boundReplica (1-RTT).
func (c *Client) SendWeakWrite(key int64, value []byte) int32 {
	return c.SendCausalWrite(key, value)
}

// SendWeakRead for CURP-HO broadcasts MCausalPropose to ALL replicas.
// Overrides the CURP-HT version which only sends to leader.
func (c *Client) SendWeakRead(key int64) int32 {
	return c.SendCausalRead(key)
}

// SendCausalWrite broadcasts a causal write to ALL replicas.
// All replicas add to their witness pool. The bound replica replies immediately
// with speculative result (1-RTT). Leader coordinates replication separately.
func (c *Client) SendCausalWrite(key int64, value []byte) int32 {
	seqnum := c.getNextSeqnum()

	c.mu.Lock()
	causalDep := c.lastWeakWriteSeqNum
	c.weakPending[seqnum] = struct{}{}
	c.weakPendingKeys[seqnum] = key
	c.weakPendingValues[seqnum] = value
	c.lastWeakWriteSeqNum = seqnum
	c.writeSet[CommandId{ClientId: c.ClientId, SeqNum: seqnum}] = struct{}{}
	c.mu.Unlock()

	p := &MCausalPropose{
		CommandId: seqnum,
		ClientId:  c.ClientId,
		Command: state.Command{
			Op: state.PUT,
			K:  state.Key(key),
			V:  value,
		},
		Timestamp: 0,
		CausalDep: causalDep,
	}

	c.sendMsgToAll(c.cs.causalProposeRPC, p)
	return seqnum
}

// SendCausalRead broadcasts a causal read to ALL replicas.
// Similar to SendCausalWrite but with GET operation.
func (c *Client) SendCausalRead(key int64) int32 {
	seqnum := c.getNextSeqnum()

	c.mu.Lock()
	c.weakPending[seqnum] = struct{}{}
	c.weakPendingKeys[seqnum] = key
	closest := c.ClosestId
	c.mu.Unlock()

	// Weak reads go to nearest replica only (not broadcast)
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

// handleCausalReply processes a causal reply from any replica.
// Only the bound replica's reply completes the operation (1-RTT).
// Replies from non-bound replicas are silently ignored.
func (c *Client) handleCausalReply(rep *MCausalReply) {
	// Only accept replies from bound replica
	if rep.Replica != c.boundReplica {
		return
	}

	c.mu.Lock()
	if _, exists := c.delivered[rep.CmdId.SeqNum]; exists {
		c.mu.Unlock()
		return
	}

	c.val = state.Value(rep.Rep)
	c.delivered[rep.CmdId.SeqNum] = struct{}{}
	delete(c.weakPending, rep.CmdId.SeqNum)

	// Update local cache for causal writes (speculative value from bound replica)
	if key, hasKey := c.weakPendingKeys[rep.CmdId.SeqNum]; hasKey {
		if val, hasVal := c.weakPendingValues[rep.CmdId.SeqNum]; hasVal {
			c.maxVersion++
			c.localCache[key] = cacheEntry{value: val, version: c.maxVersion}
			delete(c.weakPendingValues, rep.CmdId.SeqNum)
		}
		delete(c.weakPendingKeys, rep.CmdId.SeqNum)
	}

	c.mu.Unlock()
	c.RegisterReply(c.val, rep.CmdId.SeqNum)
}

// handleWeakReadReply merges the replica's response with the local cache
// using the max-version rule: whoever has the higher version wins.
func (c *Client) handleWeakReadReply(rep *MWeakReadReply) {
	c.mu.Lock()
	if _, exists := c.delivered[rep.CmdId.SeqNum]; exists {
		c.mu.Unlock()
		return
	}

	key, _ := c.weakPendingKeys[rep.CmdId.SeqNum]
	replicaVal := state.Value(rep.Rep)
	replicaVer := rep.Version

	// Merge: max-version wins
	cached, hasCached := c.localCache[key]
	var finalVal state.Value
	var finalVer int32
	if hasCached && cached.version > replicaVer {
		finalVal = cached.value
		finalVer = cached.version
	} else {
		finalVal = replicaVal
		finalVer = replicaVer
	}

	// Update cache with merged result
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

// sendMsgToAll broadcasts a message to all replicas.
// Used by CURP-HO for causal op broadcast (all replicas act as witnesses).
// Sends to bound replica first so it receives the message without waiting
// for remote TCP flushes to other replicas.
func (c *Client) sendMsgToAll(code uint8, msg fastrpc.Serializable) {
	c.SendMsg(c.boundReplica, code, msg)
	for i := 0; i < c.N; i++ {
		if int32(i) != c.boundReplica {
			c.SendMsg(int32(i), code, msg)
		}
	}
}

// BoundReplica returns the ID of the replica this client is bound to.
func (c *Client) BoundReplica() int32 {
	return c.boundReplica
}

// readDepEqual checks if two optional ReadDep pointers are equal.
// Both nil = equal. One nil, one non-nil = not equal. Both non-nil = compare fields.
func readDepEqual(a, b *CommandId) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return a.ClientId == b.ClientId && a.SeqNum == b.SeqNum
}

// checkCausalDeps verifies that every weak write in the client's write set
// appears in the CausalDeps of ALL witnesses in the quorum.
// If write set is empty, trivially passes.
func (c *Client) checkCausalDeps(msgs []interface{}) bool {
	c.mu.Lock()
	if len(c.writeSet) == 0 {
		c.mu.Unlock()
		return true
	}
	// Copy write set under lock
	wsCopy := make(map[CommandId]struct{}, len(c.writeSet))
	for k, v := range c.writeSet {
		wsCopy[k] = v
	}
	c.mu.Unlock()

	for _, msg := range msgs {
		ack := msg.(*MRecordAck)
		// Build set of reported causal deps for fast lookup
		depSet := make(map[CommandId]struct{}, len(ack.CausalDeps))
		for _, dep := range ack.CausalDeps {
			depSet[dep] = struct{}{}
		}
		// Every write set entry must appear in this witness's causal deps
		for wsCmdId := range wsCopy {
			if _, found := depSet[wsCmdId]; !found {
				return false
			}
		}
	}
	return true
}

// checkReadDepConsistency checks if all non-leader acks have consistent ReadDeps.
// Returns true if all acks agree on the same ReadDep (including all nil).
func (c *Client) checkReadDepConsistency(msgs []interface{}) bool {
	if len(msgs) == 0 {
		return true
	}
	firstAck := msgs[0].(*MRecordAck)
	firstReadDep := firstAck.ReadDep
	for _, msg := range msgs[1:] {
		ack := msg.(*MRecordAck)
		if !readDepEqual(firstReadDep, ack.ReadDep) {
			return false
		}
	}
	return true
}
