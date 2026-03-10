package curpht

import (
	"sync"
	"time"
)

type Timer struct {
	c       chan bool
	s       chan int
	wg      sync.WaitGroup
	version int
}

func NewTimer() *Timer {
	return &Timer{
		c:       make(chan bool, 1),
		s:       make(chan int, 1),
		version: 0,
	}
}

func (t *Timer) Start(wait time.Duration) {
	t.wg.Add(1)
	go func(version int, s chan int) {
		first := true
		for {
			if first {
				t.wg.Done()
				first = false
			}
			select {
			case <-s:
				return
			case <-time.After(wait):
				stop := (len(s) != 0)
				if stop {
					return
				}
				// Non-blocking send: skip if channel already has a pending event.
				// This prevents the timer goroutine from blocking when
				// handleStrongMsgs is busy processing replies.
				select {
				case t.c <- true:
				default:
				}
			}
		}
	}(t.version, t.s)
}

func (t *Timer) Reset(wait time.Duration) {
	t.Stop()
	t.Start(wait)
}

func (t *Timer) Stop() {
	t.s <- t.version
	t.wg.Wait()
	t.version++
	t.s = make(chan int, 1)
}
