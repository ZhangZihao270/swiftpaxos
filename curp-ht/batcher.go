package curpht

import (
	"time"

	"github.com/imdea-software/swiftpaxos/rpc"
)

type Batcher struct {
	acks chan rpc.Serializable
	accs chan rpc.Serializable
}

func NewBatcher(r *Replica, size int) *Batcher {
	b := &Batcher{
		acks: make(chan rpc.Serializable, size),
		accs: make(chan rpc.Serializable, size),
	}

	go func() {
		// Periodic flush timer
		flushTicker := time.NewTicker(100 * time.Microsecond)
		defer flushTicker.Stop()

		// Accumulate messages in slices
		var pendingAcks []MAcceptAck
		var pendingAccs []MAccept

		for !r.Shutdown {
			select {
			case op := <-b.acks:
				pendingAcks = append(pendingAcks, *op.(*MAcceptAck))

				// Try to collect more without blocking
				for len(pendingAcks) < size {
					select {
					case op := <-b.acks:
						pendingAcks = append(pendingAcks, *op.(*MAcceptAck))
					default:
						goto checkFlush // No more messages, check if should flush
					}
				}
				// Buffer full, flush immediately
				goto flush

			case op := <-b.accs:
				pendingAccs = append(pendingAccs, *op.(*MAccept))

				// Try to collect more without blocking
				for len(pendingAccs) < size {
					select {
					case op := <-b.accs:
						pendingAccs = append(pendingAccs, *op.(*MAccept))
					default:
						goto checkFlush
					}
				}
				goto flush

			case <-flushTicker.C:
				// Time-based flush
				goto flush
			}

		checkFlush:
			// Flush if we have enough messages or timer fired
			if len(pendingAcks) > 0 || len(pendingAccs) > 0 {
				goto flush
			}
			continue

		flush:
			if len(pendingAcks) > 0 || len(pendingAccs) > 0 {
				aacks := &MAAcks{
					Acks:    pendingAcks,
					Accepts: pendingAccs,
				}
				r.sender.SendToAll(aacks, r.cs.aacksRPC)

				// Reset slices (reuse backing array)
				pendingAcks = pendingAcks[:0]
				pendingAccs = pendingAccs[:0]
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
