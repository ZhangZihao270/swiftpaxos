package mongotunable

import (
	"log"
	"time"

	"github.com/imdea-software/swiftpaxos/client"
	"github.com/imdea-software/swiftpaxos/replica/defs"
	"github.com/imdea-software/swiftpaxos/state"
)

// Client implements the HybridClient interface for the MongoDB-Tunable protocol.
// All replies (strong + weak) come through the standard ProposeReplyTS path.
type Client struct {
	*client.BufferClient
	numReplicas  int
	leader       int
	deadReplicas map[int]bool
}

// NewClient creates a new MongoDB-Tunable client.
func NewClient(b *client.BufferClient) *Client {
	c := &Client{
		BufferClient: b,
		numReplicas:  b.NumReplicas(),
		leader:       b.LeaderId,
		deadReplicas: make(map[int]bool),
	}

	go c.waitRepliesWithFailover()

	return c
}

func (c *Client) rotateLeader(current int) int {
	for i := 1; i < c.numReplicas; i++ {
		next := (current + i) % c.numReplicas
		if !c.deadReplicas[next] {
			return next
		}
	}
	return (current + 1) % c.numReplicas
}

func (c *Client) waitRepliesWithFailover() {
	leader := c.leader
	for {
		r, err := c.GetReplyFrom(leader)
		if err != nil {
			c.deadReplicas[leader] = true
			oldLeader := leader
			leader = c.rotateLeader(leader)
			c.leader = leader
			c.LeaderId = leader
			log.Printf("Leader %d dead (EOF), rotating to %d", oldLeader, leader)
			time.Sleep(500 * time.Millisecond)
			continue
		}
		if r.OK != defs.TRUE {
			if r.LeaderId >= 0 && !c.deadReplicas[int(r.LeaderId)] {
				leader = int(r.LeaderId)
			} else {
				leader = c.rotateLeader(leader)
			}
			c.leader = leader
			c.LeaderId = leader
			continue
		}
		go func(val state.Value, seqnum int32) {
			time.Sleep(c.BufferClient.WaitDurationForReplica(leader))
			c.RegisterReply(val, seqnum)
		}(r.Value, r.CommandId)
	}
}

// sendCommand sends a propose with the given op, key, value, and consistency level.
func (c *Client) sendCommand(op state.Operation, key int64, value []byte, cl state.Operation) int32 {
	seqnum := c.GetNextSeqnum()
	p := defs.Propose{
		CommandId: seqnum,
		ClientId:  c.ClientId,
		Command: state.Command{
			Op: op,
			K:  state.Key(key),
			V:  value,
			CL: cl,
		},
		Timestamp: 0,
	}
	c.SendProposal(p)
	return seqnum
}

// SendStrongWrite sends a linearizable write command (CL=STRONG).
func (c *Client) SendStrongWrite(key int64, value []byte) int32 {
	return c.sendCommand(state.PUT, key, value, state.STRONG)
}

// SendStrongRead sends a linearizable read command (CL=STRONG).
func (c *Client) SendStrongRead(key int64) int32 {
	return c.sendCommand(state.GET, key, state.NIL(), state.STRONG)
}

// SendWeakWrite sends a causal write command (CL=CAUSAL).
func (c *Client) SendWeakWrite(key int64, value []byte) int32 {
	return c.sendCommand(state.PUT, key, value, state.CAUSAL)
}

// SendWeakRead sends a causal read command (CL=CAUSAL).
func (c *Client) SendWeakRead(key int64) int32 {
	return c.sendCommand(state.GET, key, state.NIL(), state.CAUSAL)
}

// SupportsWeak returns true since MongoDB-Tunable supports causal consistency.
func (c *Client) SupportsWeak() bool {
	return true
}

// MarkAllSent is a no-op for MongoDB-Tunable.
func (c *Client) MarkAllSent() {}
