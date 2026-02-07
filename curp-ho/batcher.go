package curpho

import (
	"sync/atomic"
	"time"

	"github.com/imdea-software/swiftpaxos/rpc"
)

// Batcher batches Accept and AcceptAck messages for efficient network transmission.
//
// Design: Adaptive event-driven batching with configurable delay
//   - Immediately processes messages when they arrive (optimal latency)
//   - Optional batching delay to accumulate more messages (higher throughput)
//   - Drains all pending messages using len(channel) (natural batching)
//   - Adapts automatically to workload
//
// Performance: < 10μs processing time without delay, < 100μs with delay
//
// Batching delay trade-off:
//   - 0μs:    Lowest latency, smaller batches, more syscalls
//   - 50μs:   Balanced, 2-3x larger batches, moderate latency increase
//   - 100μs:  Highest throughput, largest batches, higher latency
//
// See docs/phase-31.4-network-batching.md for analysis.
type Batcher struct {
	acks chan rpc.Serializable
	accs chan rpc.Serializable

	// BatchDelay is the time to wait for additional messages before sending
	// 0 = immediate send (zero-delay batching)
	// 50-100μs = wait for more messages (adaptive batching)
	batchDelayNs int64

	// Statistics for monitoring batch performance
	totalBatches   uint64
	totalAcks      uint64
	totalAccepts   uint64
	minBatchSize   uint64
	maxBatchSize   uint64
}

func NewBatcher(r *Replica, size int) *Batcher {
	b := &Batcher{
		acks:         make(chan rpc.Serializable, size),
		accs:         make(chan rpc.Serializable, size),
		batchDelayNs: 0, // Default: zero-delay batching (Phase 18.5 design)
		minBatchSize: ^uint64(0), // Max uint64
		maxBatchSize: 0,
	}

	go func() {
		for !r.Shutdown {
			select {
			case op := <-b.acks:
				// Apply batching delay if configured
				if b.batchDelayNs > 0 {
					time.Sleep(time.Duration(atomic.LoadInt64(&b.batchDelayNs)))
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

				// Update statistics
				batchSize := uint64(l1 + l2)
				atomic.AddUint64(&b.totalBatches, 1)
				atomic.AddUint64(&b.totalAcks, uint64(l1))
				atomic.AddUint64(&b.totalAccepts, uint64(l2))

				// Update min/max atomically
				for {
					oldMin := atomic.LoadUint64(&b.minBatchSize)
					if batchSize >= oldMin || atomic.CompareAndSwapUint64(&b.minBatchSize, oldMin, batchSize) {
						break
					}
				}
				for {
					oldMax := atomic.LoadUint64(&b.maxBatchSize)
					if batchSize <= oldMax || atomic.CompareAndSwapUint64(&b.maxBatchSize, oldMax, batchSize) {
						break
					}
				}

				r.sender.SendToAll(aacks, r.cs.aacksRPC)

			case op := <-b.accs:
				// Apply batching delay if configured
				if b.batchDelayNs > 0 {
					time.Sleep(time.Duration(atomic.LoadInt64(&b.batchDelayNs)))
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

				// Update statistics
				batchSize := uint64(l1 + l2)
				atomic.AddUint64(&b.totalBatches, 1)
				atomic.AddUint64(&b.totalAcks, uint64(l1))
				atomic.AddUint64(&b.totalAccepts, uint64(l2))

				// Update min/max atomically
				for {
					oldMin := atomic.LoadUint64(&b.minBatchSize)
					if batchSize >= oldMin || atomic.CompareAndSwapUint64(&b.minBatchSize, oldMin, batchSize) {
						break
					}
				}
				for {
					oldMax := atomic.LoadUint64(&b.maxBatchSize)
					if batchSize <= oldMax || atomic.CompareAndSwapUint64(&b.maxBatchSize, oldMax, batchSize) {
						break
					}
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
// 0 = immediate send, 50000 = 50μs delay, 100000 = 100μs delay
func (b *Batcher) SetBatchDelay(delayNs int64) {
	atomic.StoreInt64(&b.batchDelayNs, delayNs)
}

// GetStats returns batching statistics
func (b *Batcher) GetStats() (batches, acks, accepts, minSize, maxSize uint64) {
	return atomic.LoadUint64(&b.totalBatches),
		atomic.LoadUint64(&b.totalAcks),
		atomic.LoadUint64(&b.totalAccepts),
		atomic.LoadUint64(&b.minBatchSize),
		atomic.LoadUint64(&b.maxBatchSize)
}

// GetAvgBatchSize returns the average batch size
func (b *Batcher) GetAvgBatchSize() float64 {
	batches := atomic.LoadUint64(&b.totalBatches)
	if batches == 0 {
		return 0
	}
	total := atomic.LoadUint64(&b.totalAcks) + atomic.LoadUint64(&b.totalAccepts)
	return float64(total) / float64(batches)
}
