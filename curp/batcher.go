package curp

import (
	"sync/atomic"
	"time"

	"github.com/imdea-software/swiftpaxos/rpc"
)

type Batcher struct {
	acks chan rpc.Serializable
	accs chan rpc.Serializable

	// batchDelayNs is the time to wait for additional messages before sending
	// 0 = immediate send (default), 150000 = 150μs delay (optimal for throughput)
	batchDelayNs int64
}

func NewBatcher(r *Replica, size int) *Batcher {
	b := &Batcher{
		acks:         make(chan rpc.Serializable, size),
		accs:         make(chan rpc.Serializable, size),
		batchDelayNs: 0, // Default: immediate send (backward compatible)
	}

	go func() {
		for !r.Shutdown {
			select {
			case op := <-b.acks:
				// Apply batching delay if configured
				if delay := atomic.LoadInt64(&b.batchDelayNs); delay > 0 {
					time.Sleep(time.Duration(delay))
				}

				l1 := len(b.acks) + 1
				l2 := len(b.accs)
				aacks := &MAAcks{
					Acks:    make([]MAcceptAck, l1),
					Accepts: make([]MAccept, l2),
				}
				for i := 0; i < l1; i++ {
					aacks.Acks[i] = *op.(*MAcceptAck)
					if i < l1-1 {
						op = <-b.acks
					}
				}
				for i := 0; i < l2; i++ {
					op = <-b.accs
					aacks.Accepts[i] = *op.(*MAccept)
				}
				r.sender.SendToAll(aacks, r.cs.aacksRPC)

			case op := <-b.accs:
				// Apply batching delay if configured
				if delay := atomic.LoadInt64(&b.batchDelayNs); delay > 0 {
					time.Sleep(time.Duration(delay))
				}

				l1 := len(b.acks)
				l2 := len(b.accs) + 1
				aacks := &MAAcks{
					Acks:    make([]MAcceptAck, l1),
					Accepts: make([]MAccept, l2),
				}
				for i := 0; i < l2; i++ {
					aacks.Accepts[i] = *op.(*MAccept)
					if i < l2-1 {
						op = <-b.accs
					}
				}
				for i := 0; i < l1; i++ {
					op = <-b.acks
					aacks.Acks[i] = *op.(*MAcceptAck)
				}
				r.sender.SendToAll(aacks, r.cs.aacksRPC)
			}
		}
	}()

	return b
}

func (b *Batcher) SendAccept(a *MAccept) {
	b.accs <- a
}

func (b *Batcher) SendAcceptAck(a *MAcceptAck) {
	b.acks <- a
}

// SetBatchDelay sets the batching delay in nanoseconds
// 0 = immediate send (default), 150000 = 150μs delay (optimal for throughput)
func (b *Batcher) SetBatchDelay(delayNs int64) {
	atomic.StoreInt64(&b.batchDelayNs, delayNs)
}
