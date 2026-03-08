package epaxos

import (
	"github.com/imdea-software/swiftpaxos/client"
)

// Client implements the HybridClient interface for the EPaxos protocol.
// Since EPaxos only provides linearizable (strong) consistency,
// weak commands are delegated to the strong path.
type Client struct {
	*client.BufferClient
}

// NewClient creates a new EPaxos client.
// EPaxos is leaderless: clients send proposals to and wait for replies
// from their closest replica.
func NewClient(b *client.BufferClient) *Client {
	c := &Client{
		BufferClient: b,
	}

	// EPaxos is leaderless — wait for replies from the closest replica.
	c.WaitReplies(int(b.ClosestId))

	return c
}

// SendStrongWrite sends a linearizable write command.
func (c *Client) SendStrongWrite(key int64, value []byte) int32 {
	return c.SendWrite(key, value)
}

// SendStrongRead sends a linearizable read command.
func (c *Client) SendStrongRead(key int64) int32 {
	return c.SendRead(key)
}

// SendWeakWrite delegates to strong write (EPaxos has no weak consistency).
func (c *Client) SendWeakWrite(key int64, value []byte) int32 {
	return c.SendStrongWrite(key, value)
}

// SendWeakRead delegates to strong read (EPaxos has no weak consistency).
func (c *Client) SendWeakRead(key int64) int32 {
	return c.SendStrongRead(key)
}

// SupportsWeak returns false since EPaxos only provides linearizable consistency.
func (c *Client) SupportsWeak() bool {
	return false
}

// MarkAllSent is a no-op for EPaxos (no MSync retry mechanism).
func (c *Client) MarkAllSent() {}
