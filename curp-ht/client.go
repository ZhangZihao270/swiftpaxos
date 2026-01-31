package curpht

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

		leader:    -1,
		ballot:    -1,
		delivered: make(map[int32]struct{}),

		acks:  make(map[CommandId]*replica.MsgSet),
		macks: make(map[CommandId]*replica.MsgSet),

		slowPaths:   0,
		alreadySlow: make(map[CommandId]struct{}),

		weakPending: make(map[int32]struct{}),
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

		case needSync := <-c.t.c:
			// FIXME
			break
			if needSync && c.leader != -1 {
				if _, exists := c.delivered[c.lastCmdId.SeqNum]; !exists {
					sync := &MSync{
						CmdId: c.lastCmdId,
					}
					// TODO: send to closest?
					c.SendMsg(c.leader, c.cs.syncRPC, sync)
				}
			}
		}
	}
}

func (c *Client) handleReply(r *MReply) {
	if _, exists := c.delivered[r.CmdId.SeqNum]; exists {
		return
	}

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
	if _, exists := c.delivered[r.CmdId.SeqNum]; exists {
		return
	}

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
	if _, exists := c.delivered[rep.CmdId.SeqNum]; exists {
		return
	}

	if c.ballot == -1 {
		c.ballot = rep.Ballot
	} else if c.ballot < rep.Ballot {
		c.ballot = rep.Ballot
	} else if c.ballot > rep.Ballot {
		return
	}

	c.val = state.Value(rep.Rep)
	c.delivered[rep.CmdId.SeqNum] = struct{}{}
	c.RegisterReply(c.val, rep.CmdId.SeqNum)
	c.Println("Slow Paths:", c.slowPaths)
}

func (c *Client) handleAcks(leaderMsg interface{}, msgs []interface{}) {
	if leaderMsg == nil {
		return
	}

	if _, exists := c.delivered[leaderMsg.(*MRecordAck).CmdId.SeqNum]; exists {
		return
	}

	c.delivered[leaderMsg.(*MRecordAck).CmdId.SeqNum] = struct{}{}
	c.RegisterReply(c.val, leaderMsg.(*MRecordAck).CmdId.SeqNum)
	c.Println("Slow Paths:", c.slowPaths)
}

// handleWeakReply handles weak command reply from leader
func (c *Client) handleWeakReply(rep *MWeakReply) {
	if _, exists := c.delivered[rep.CmdId.SeqNum]; exists {
		return
	}

	// Update ballot if needed
	if c.ballot == -1 {
		c.ballot = rep.Ballot
	} else if c.ballot < rep.Ballot {
		c.ballot = rep.Ballot
	} else if c.ballot > rep.Ballot {
		return
	}

	// Update leader (reply always comes from leader)
	c.leader = rep.Replica

	// Weak command completes immediately upon receiving leader's reply
	c.val = state.Value(rep.Rep)
	c.delivered[rep.CmdId.SeqNum] = struct{}{}
	delete(c.weakPending, rep.CmdId.SeqNum)
	c.RegisterReply(c.val, rep.CmdId.SeqNum)
}

// SendWeakWrite sends a weak consistency write operation to leader only
func (c *Client) SendWeakWrite(key int64, value []byte) int32 {
	seqnum := c.getNextSeqnum()
	p := &MWeakPropose{
		CommandId: seqnum,
		ClientId:  c.ClientId,
		Command: state.Command{
			Op: state.PUT,
			K:  state.Key(key),
			V:  value,
		},
		Timestamp: 0,
		CausalDep: c.lastWeakSeqNum, // Depend on previous weak command
	}

	// Track as pending and update lastWeakSeqNum for causal ordering
	c.weakPending[seqnum] = struct{}{}
	c.lastWeakSeqNum = seqnum

	// Send only to leader
	if c.leader != -1 {
		c.SendMsg(c.leader, c.cs.weakProposeRPC, p)
	}
	return seqnum
}

// SendWeakRead sends a weak consistency read operation to leader only
func (c *Client) SendWeakRead(key int64) int32 {
	seqnum := c.getNextSeqnum()
	p := &MWeakPropose{
		CommandId: seqnum,
		ClientId:  c.ClientId,
		Command: state.Command{
			Op: state.GET,
			K:  state.Key(key),
			V:  state.NIL(),
		},
		Timestamp: 0,
		CausalDep: c.lastWeakSeqNum, // Depend on previous weak command
	}

	// Track as pending and update lastWeakSeqNum for causal ordering
	c.weakPending[seqnum] = struct{}{}
	c.lastWeakSeqNum = seqnum

	// Send only to leader
	if c.leader != -1 {
		c.SendMsg(c.leader, c.cs.weakProposeRPC, p)
	}
	return seqnum
}

// getNextSeqnum returns the next sequence number
// Note: This accesses the embedded Client's seqnum field
func (c *Client) getNextSeqnum() int32 {
	// Access the base client's seqnum through SendWrite which increments it
	// For now, we track our own seqnum for weak commands
	c.lastCmdId.SeqNum++
	return c.lastCmdId.SeqNum
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

// SupportsWeak returns true since curp-ht supports weak consistency commands.
func (c *Client) SupportsWeak() bool {
	return true
}
