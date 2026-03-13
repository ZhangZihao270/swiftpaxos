package epaxos

import (
	"github.com/imdea-software/swiftpaxos/client"
)

type Client struct {
	*client.BufferClient
}

func NewClient(b *client.BufferClient) *Client {
	c := &Client{
		BufferClient: b,
	}
	c.WaitReplies(int(b.ClosestId))
	return c
}

func (c *Client) SendStrongWrite(key int64, value []byte) int32 {
	return c.SendWrite(key, value)
}

func (c *Client) SendStrongRead(key int64) int32 {
	return c.SendRead(key)
}

func (c *Client) SendWeakWrite(key int64, value []byte) int32 {
	return c.SendStrongWrite(key, value)
}

func (c *Client) SendWeakRead(key int64) int32 {
	return c.SendStrongRead(key)
}

func (c *Client) SupportsWeak() bool {
	return false
}

func (c *Client) MarkAllSent() {}
