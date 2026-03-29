package curp

import (
	"sync"
	"time"

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

	mu       sync.Mutex
	writerMu []sync.Mutex
	pending  map[int32]struct{} // seqnums of pending (undelivered) commands
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

		writerMu: make([]sync.Mutex, repNum),
		pending:  make(map[int32]struct{}),
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
	// commands whose replies may have been dropped by SendClientMsgFast.
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

		case <-c.t.c:
			// Retry pending commands whose replies may have been dropped
			// by the non-blocking SendClientMsgFast.
			c.mu.Lock()
			var syncSeqnums []int32
			for seqnum := range c.pending {
				if _, delivered := c.delivered[seqnum]; !delivered {
					syncSeqnums = append(syncSeqnums, seqnum)
				}
			}
			clientId := c.ClientId
			n := c.N
			c.mu.Unlock()

			if len(syncSeqnums) > 0 {
				c.Println("MSync retry:", len(syncSeqnums), "pending")
			}
			for _, seqnum := range syncSeqnums {
				sync := &MSync{
					CmdId: CommandId{ClientId: clientId, SeqNum: seqnum},
				}
				for r := int32(0); r < int32(n); r++ {
					c.writerMu[r].Lock()
					c.SendMsg(r, c.cs.syncRPC, sync)
					c.writerMu[r].Unlock()
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
	c.mu.Lock()
	delete(c.pending, rep.CmdId.SeqNum)
	c.mu.Unlock()
	c.RegisterReply(c.val, rep.CmdId.SeqNum)
	c.Println("Slow Paths:", c.slowPaths)
}

func (c *Client) handleAcks(leaderMsg interface{}, msgs []interface{}) {
	if leaderMsg == nil {
		return
	}

	seqNum := leaderMsg.(*MRecordAck).CmdId.SeqNum
	if _, exists := c.delivered[seqNum]; exists {
		return
	}

	c.delivered[seqNum] = struct{}{}
	c.mu.Lock()
	delete(c.pending, seqNum)
	c.mu.Unlock()
	c.RegisterReply(c.val, seqNum)
	c.Println("Slow Paths:", c.slowPaths)
}

// HybridClient interface implementation (Phase 52.4)
// CURP only supports strong consistency, so weak methods are stubs.

func (c *Client) SendStrongWrite(key int64, value []byte) int32 {
	c.writerMu[c.LeaderId].Lock()
	seqnum := c.SendWrite(key, value)
	c.writerMu[c.LeaderId].Unlock()
	c.mu.Lock()
	c.pending[seqnum] = struct{}{}
	c.mu.Unlock()
	return seqnum
}

func (c *Client) SendStrongRead(key int64) int32 {
	c.writerMu[c.LeaderId].Lock()
	seqnum := c.SendRead(key)
	c.writerMu[c.LeaderId].Unlock()
	c.mu.Lock()
	c.pending[seqnum] = struct{}{}
	c.mu.Unlock()
	return seqnum
}

func (c *Client) SendWeakWrite(key int64, value []byte) int32 {
	// CURP doesn't support weak writes - should never be called when weakRatio=0
	panic("CURP does not support weak writes")
}

func (c *Client) SendWeakRead(key int64) int32 {
	// CURP doesn't support weak reads - should never be called when weakRatio=0
	panic("CURP does not support weak reads")
}

func (c *Client) SendWeakScan(key int64, count int64) int32 {
	return c.SendScan(key, count)
}

func (c *Client) SupportsWeak() bool {
	return false
}

func (c *Client) MarkAllSent() {
	// CURP doesn't have MSync retry mechanism, no-op
}
