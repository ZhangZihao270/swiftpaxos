package epaxosho

import (
	"encoding/binary"
	"log"
	"sync"
	"time"

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

	numReplicas  int
	closestId    int
	deadReplicas map[int]bool
	ping         []float64 // copy of base client's Ping for failover selection
	mu           sync.Mutex
}

// NewClient creates a new EPaxos-HO client.
// EPaxos-HO is leaderless — clients send proposals to their closest replica.
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

		log.Printf("EPaxos-HO: closest replica %d dead, failing over to %d", oldClosest, newClosest)
		// Brief backoff to prevent rapid cascading failover if multiple replicas fail
		time.Sleep(100 * time.Millisecond)
		c.WaitReplies(newClosest)
	}
}

// findNextAlive returns the next alive replica after the given one.
// Uses ping latency to pick the closest alive replica if available.
// Must be called with c.mu held.
func (c *Client) findNextAlive(current int) int {
	// If we have ping data, pick the closest alive replica
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

	// Fallback: round-robin to next alive
	for i := 1; i < c.numReplicas; i++ {
		next := (current + i) % c.numReplicas
		if !c.deadReplicas[next] {
			return next
		}
	}
	// All dead (shouldn't happen with majority alive)
	return (current + 1) % c.numReplicas
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

func (c *Client) SendWeakScan(key int64, count int64) int32 {
	seqnum := c.GetNextSeqnum()
	v := make([]byte, 8)
	binary.LittleEndian.PutUint64(v, uint64(count))
	p := defs.Propose{
		CommandId: seqnum,
		ClientId:  c.ClientId,
		Command: state.Command{
			Op: state.SCAN,
			K:  state.Key(key),
			V:  v,
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
