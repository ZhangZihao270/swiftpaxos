package curpho

import (
	"sync"

	"github.com/imdea-software/swiftpaxos/client"
	"github.com/imdea-software/swiftpaxos/replica"
	"github.com/imdea-software/swiftpaxos/replica/defs"
	fastrpc "github.com/imdea-software/swiftpaxos/rpc"
	"github.com/imdea-software/swiftpaxos/state"
)

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
	weakPending    map[int32]struct{}
	lastWeakSeqNum int32 // Track sequence number of last weak command for causal ordering

	// CURP-HO: Bound replica for 1-RTT causal op completion.
	// Client binds to closest replica (lowest latency) for fast causal replies.
	// Causal ops are broadcast to all replicas, but client only waits for
	// the bound replica's reply to complete in 1-RTT.
	boundReplica int32

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

		// CURP-HO: Bind to closest replica for 1-RTT causal op completion.
		// ClosestId is computed by base client during Connect() via ping latency measurement.
		boundReplica: int32(b.ClosestId),
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
	c.mu.Unlock()
	c.RegisterReply(c.val, rep.CmdId.SeqNum)
	c.Println("Slow Paths:", c.slowPaths)
}

// handleFastPathAcks handles the fast path (3/4 quorum) with weakDep consistency check.
// If weakDeps are consistent across all non-leader acks, the operation completes.
// If inconsistent, the operation falls back to the slow path (via macks).
func (c *Client) handleFastPathAcks(leaderMsg interface{}, msgs []interface{}) {
	if leaderMsg == nil {
		return
	}

	cmdId := leaderMsg.(*MRecordAck).CmdId

	// Check weakDep consistency among non-leader acks
	if !c.checkWeakDepConsistency(msgs) {
		// Inconsistent weakDeps - cannot complete on fast path
		c.mu.Lock()
		if _, exists := c.alreadySlow[cmdId]; !exists {
			c.alreadySlow[cmdId] = struct{}{}
			c.slowPaths++
		}
		c.mu.Unlock()
		return
	}

	// Consistent weakDeps - deliver on fast path
	c.mu.Lock()
	if _, exists := c.delivered[cmdId.SeqNum]; exists {
		c.mu.Unlock()
		return
	}
	c.delivered[cmdId.SeqNum] = struct{}{}
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
	return c.SendWrite(key, value)
}

// SendStrongRead sends a linearizable read command (delegates to base SendRead).
func (c *Client) SendStrongRead(key int64) int32 {
	return c.SendRead(key)
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
	causalDep := c.lastWeakSeqNum
	c.weakPending[seqnum] = struct{}{}
	c.lastWeakSeqNum = seqnum
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
	causalDep := c.lastWeakSeqNum
	c.weakPending[seqnum] = struct{}{}
	c.lastWeakSeqNum = seqnum
	c.mu.Unlock()

	p := &MCausalPropose{
		CommandId: seqnum,
		ClientId:  c.ClientId,
		Command: state.Command{
			Op: state.GET,
			K:  state.Key(key),
			V:  state.NIL(),
		},
		Timestamp: 0,
		CausalDep: causalDep,
	}

	c.sendMsgToAll(c.cs.causalProposeRPC, p)
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
	c.mu.Unlock()
	c.RegisterReply(c.val, rep.CmdId.SeqNum)
}

// sendMsgToAll broadcasts a message to all replicas.
// Used by CURP-HO for causal op broadcast (all replicas act as witnesses).
func (c *Client) sendMsgToAll(code uint8, msg fastrpc.Serializable) {
	for i := 0; i < c.N; i++ {
		c.SendMsg(int32(i), code, msg)
	}
}

// BoundReplica returns the ID of the replica this client is bound to.
func (c *Client) BoundReplica() int32 {
	return c.boundReplica
}

// weakDepEqual checks if two optional WeakDep pointers are equal.
// Both nil = equal. One nil, one non-nil = not equal. Both non-nil = compare fields.
func weakDepEqual(a, b *CommandId) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return a.ClientId == b.ClientId && a.SeqNum == b.SeqNum
}

// checkWeakDepConsistency checks if all non-leader acks have consistent weakDeps.
// Returns true if all acks agree on the same weakDep (including all nil).
func (c *Client) checkWeakDepConsistency(msgs []interface{}) bool {
	if len(msgs) == 0 {
		return true
	}
	firstAck := msgs[0].(*MRecordAck)
	firstWeakDep := firstAck.WeakDep
	for _, msg := range msgs[1:] {
		ack := msg.(*MRecordAck)
		if !weakDepEqual(firstWeakDep, ack.WeakDep) {
			return false
		}
	}
	return true
}
