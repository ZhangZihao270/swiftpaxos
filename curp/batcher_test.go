package curp

import (
	"sync/atomic"
	"testing"
	"time"
)

// TestBatcherSetBatchDelay verifies that SetBatchDelay correctly updates the delay
func TestBatcherSetBatchDelay(t *testing.T) {
	b := &Batcher{
		batchDelayNs: 0,
	}

	// Test setting delay
	b.SetBatchDelay(150000) // 150μs
	delay := atomic.LoadInt64(&b.batchDelayNs)
	if delay != 150000 {
		t.Errorf("Expected delay 150000, got %d", delay)
	}

	// Test updating delay
	b.SetBatchDelay(50000) // 50μs
	delay = atomic.LoadInt64(&b.batchDelayNs)
	if delay != 50000 {
		t.Errorf("Expected delay 50000, got %d", delay)
	}

	// Test zero delay (immediate send)
	b.SetBatchDelay(0)
	delay = atomic.LoadInt64(&b.batchDelayNs)
	if delay != 0 {
		t.Errorf("Expected delay 0, got %d", delay)
	}
}

// TestBatcherDefaultDelay verifies that the default delay is 0 (immediate send)
func TestBatcherDefaultDelay(t *testing.T) {
	b := &Batcher{
		batchDelayNs: 0,
	}

	delay := atomic.LoadInt64(&b.batchDelayNs)
	if delay != 0 {
		t.Errorf("Expected default delay 0, got %d", delay)
	}
}

// TestBatcherDelayApplied verifies that the delay is actually applied
func TestBatcherDelayApplied(t *testing.T) {
	b := &Batcher{
		batchDelayNs: 0,
	}

	// Measure time with no delay
	start := time.Now()
	if delay := atomic.LoadInt64(&b.batchDelayNs); delay > 0 {
		time.Sleep(time.Duration(delay))
	}
	elapsed := time.Since(start)
	if elapsed > 1*time.Millisecond {
		t.Errorf("Expected near-zero delay, got %v", elapsed)
	}

	// Set 10ms delay and measure
	b.SetBatchDelay(10_000_000) // 10ms
	start = time.Now()
	if delay := atomic.LoadInt64(&b.batchDelayNs); delay > 0 {
		time.Sleep(time.Duration(delay))
	}
	elapsed = time.Since(start)
	if elapsed < 9*time.Millisecond || elapsed > 15*time.Millisecond {
		t.Errorf("Expected ~10ms delay, got %v", elapsed)
	}
}
