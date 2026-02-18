package raft

import (
	"github.com/imdea-software/swiftpaxos/client"
)

// Client implements the HybridClient interface for the Raft protocol.
// Since Raft only provides linearizable (strong) consistency,
// weak commands are delegated to the strong path.
type Client struct {
	*client.BufferClient
}

// NewClient creates a new Raft client.
// The client uses the base BufferClient's WaitReplies mechanism
// to receive ProposeReplyTS responses from the Raft leader.
func NewClient(b *client.BufferClient) *Client {
	c := &Client{
		BufferClient: b,
	}

	// Start reading replies from the leader.
	// Raft sends ProposeReplyTS directly via the client writer,
	// so we use the base WaitReplies mechanism (no fastrpc table needed).
	c.WaitReplies(int(b.LeaderId))

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

// SendWeakWrite delegates to strong write (Raft has no weak consistency).
func (c *Client) SendWeakWrite(key int64, value []byte) int32 {
	return c.SendStrongWrite(key, value)
}

// SendWeakRead delegates to strong read (Raft has no weak consistency).
func (c *Client) SendWeakRead(key int64) int32 {
	return c.SendStrongRead(key)
}

// SupportsWeak returns false since Raft only provides linearizable consistency.
func (c *Client) SupportsWeak() bool {
	return false
}

// MarkAllSent is a no-op for Raft (no MSync retry mechanism).
func (c *Client) MarkAllSent() {}
