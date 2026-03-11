package epaxosho

import (
	"github.com/imdea-software/swiftpaxos/client"
	"github.com/imdea-software/swiftpaxos/replica/defs"
	"github.com/imdea-software/swiftpaxos/state"
)

// Client implements the HybridClient interface for EPaxos-HO.
// EPaxos-HO supports both strong (linearizable) and weak (causal) commands.
// Strong commands go through EPaxos 2-RTT consensus.
// Weak commands are tagged with CL=CAUSAL for 1-RTT fast commit.
type Client struct {
	*client.BufferClient
}

// NewClient creates a new EPaxos-HO client.
// EPaxos-HO is leaderless — clients send proposals to their closest replica.
func NewClient(b *client.BufferClient) *Client {
	c := &Client{
		BufferClient: b,
	}
	c.WaitReplies(int(b.ClosestId))
	return c
}

// SendStrongWrite sends a linearizable write command.
// Uses default CL (NONE → treated as strong by replica handlePropose).
func (c *Client) SendStrongWrite(key int64, value []byte) int32 {
	return c.SendWrite(key, value)
}

// SendStrongRead sends a linearizable read command.
func (c *Client) SendStrongRead(key int64) int32 {
	return c.SendRead(key)
}

// SendWeakWrite sends a causal write command with CL=CAUSAL.
func (c *Client) SendWeakWrite(key int64, value []byte) int32 {
	seqnum := c.GetNextSeqnum()
	p := defs.Propose{
		CommandId: seqnum,
		ClientId:  c.ClientId,
		Command: state.Command{
			Op: state.PUT,
			K:  state.Key(key),
			V:  value,
			CL: state.CAUSAL,
		},
		Timestamp: 0,
	}
	c.SendProposal(p)
	return seqnum
}

// SendWeakRead sends a causal read command with CL=CAUSAL.
func (c *Client) SendWeakRead(key int64) int32 {
	seqnum := c.GetNextSeqnum()
	p := defs.Propose{
		CommandId: seqnum,
		ClientId:  c.ClientId,
		Command: state.Command{
			Op: state.GET,
			K:  state.Key(key),
			V:  state.NIL(),
			CL: state.CAUSAL,
		},
		Timestamp: 0,
	}
	c.SendProposal(p)
	return seqnum
}

// SupportsWeak returns true since EPaxos-HO supports weak (causal) consistency.
func (c *Client) SupportsWeak() bool {
	return true
}

// MarkAllSent is a no-op for EPaxos-HO (no MSync retry mechanism).
func (c *Client) MarkAllSent() {}
