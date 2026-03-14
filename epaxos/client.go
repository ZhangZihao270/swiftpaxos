package epaxos

import (
	"log"
	"sync"

	"github.com/imdea-software/swiftpaxos/client"
)

// Client implements the client for vanilla EPaxos.
// EPaxos is leaderless — all commands are strong (linearizable).
type Client struct {
	*client.BufferClient

	numReplicas  int
	closestId    int
	deadReplicas map[int]bool
	ping         []float64
	mu           sync.Mutex
}

func NewClient(b *client.BufferClient) *Client {
	c := &Client{
		BufferClient: b,
		numReplicas:  b.NumReplicas(),
		closestId:    b.ClosestId,
		deadReplicas: make(map[int]bool),
		ping:         b.Ping,
	}
	c.WaitReplies(c.closestId)
	go c.watchReaderDead()
	return c
}

// watchReaderDead listens for dead replica notifications and fails over.
func (c *Client) watchReaderDead() {
	for {
		deadReplica := <-c.ReaderDead

		c.mu.Lock()
		c.deadReplicas[deadReplica] = true

		if deadReplica != c.closestId {
			c.mu.Unlock()
			continue
		}

		oldClosest := c.closestId
		newClosest := c.findNextAlive(oldClosest)
		c.closestId = newClosest
		c.ClosestId = newClosest
		c.mu.Unlock()

		log.Printf("EPaxos: closest replica %d dead, failing over to %d", oldClosest, newClosest)
		c.WaitReplies(newClosest)
	}
}

// findNextAlive returns the next alive replica after the given one.
// Uses ping latency to pick the closest alive replica if available.
// Must be called with c.mu held.
func (c *Client) findNextAlive(current int) int {
	if len(c.ping) == c.numReplicas {
		bestId := -1
		bestPing := float64(0)
		for i := 0; i < c.numReplicas; i++ {
			if c.deadReplicas[i] {
				continue
			}
			if bestId == -1 || c.ping[i] < bestPing {
				bestId = i
				bestPing = c.ping[i]
			}
		}
		if bestId >= 0 {
			return bestId
		}
	}

	for i := 1; i < c.numReplicas; i++ {
		next := (current + i) % c.numReplicas
		if !c.deadReplicas[next] {
			return next
		}
	}
	return (current + 1) % c.numReplicas
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
