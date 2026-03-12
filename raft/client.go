package raft

import (
	"log"
	"time"

	"github.com/imdea-software/swiftpaxos/client"
	"github.com/imdea-software/swiftpaxos/replica/defs"
	"github.com/imdea-software/swiftpaxos/state"
)

// Client implements the HybridClient interface for the Raft protocol.
// Since Raft only provides linearizable (strong) consistency,
// weak commands are delegated to the strong path.
type Client struct {
	*client.BufferClient
	numReplicas  int
	leader       int
	deadReplicas map[int]bool // replicas whose reader has exited (EOF/error)
}

// NewClient creates a new Raft client.
// Starts a reply reader with leader failover support.
func NewClient(b *client.BufferClient) *Client {
	c := &Client{
		BufferClient: b,
		numReplicas:  b.NumReplicas(),
		leader:       b.LeaderId,
		deadReplicas: make(map[int]bool),
	}

	// Start reading replies with failover support.
	go c.waitRepliesWithFailover()

	return c
}

// rotateLeader returns the next alive replica ID, skipping dead ones.
func (c *Client) rotateLeader(current int) int {
	for i := 1; i < c.numReplicas; i++ {
		next := (current + i) % c.numReplicas
		if !c.deadReplicas[next] {
			return next
		}
	}
	return (current + 1) % c.numReplicas
}

// waitRepliesWithFailover reads ProposeReplyTS from the current leader.
// On EOF (leader dead) or NOT_LEADER rejection, it rotates to the next replica.
func (c *Client) waitRepliesWithFailover() {
	leader := c.leader
	for {
		r, err := c.GetReplyFrom(leader)
		if err != nil {
			// Reader failed (EOF = leader dead). Mark dead and rotate.
			c.deadReplicas[leader] = true
			oldLeader := leader
			leader = c.rotateLeader(leader)
			c.leader = leader
			c.LeaderId = leader
			log.Printf("Leader %d dead (EOF), rotating to %d", oldLeader, leader)
			// Brief pause before reading from new leader (election may be in progress)
			time.Sleep(500 * time.Millisecond)
			continue
		}
		if r.OK != defs.TRUE {
			// NOT_LEADER rejection. Use leader hint if available and alive.
			if r.LeaderId >= 0 && !c.deadReplicas[int(r.LeaderId)] {
				oldLeader := leader
				leader = int(r.LeaderId)
				c.leader = leader
				c.LeaderId = leader
				log.Printf("NOT_LEADER from %d: hinted leader=%d", oldLeader, leader)
			} else {
				oldLeader := leader
				leader = c.rotateLeader(leader)
				c.leader = leader
				c.LeaderId = leader
				if r.LeaderId >= 0 {
					log.Printf("NOT_LEADER from %d: hint=%d is dead, rotating to %d", oldLeader, r.LeaderId, leader)
				} else {
					log.Printf("NOT_LEADER from %d: no hint, rotating to %d", oldLeader, leader)
				}
			}
			continue
		}
		go func(val state.Value, seqnum int32) {
			time.Sleep(c.BufferClient.WaitDurationForReplica(leader))
			c.RegisterReply(val, seqnum)
		}(r.Value, r.CommandId)
	}
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
