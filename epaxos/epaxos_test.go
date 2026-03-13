package epaxos

import (
	"testing"
)

func TestDepsEqual(t *testing.T) {
	if !depsEqual([]int32{1, 2, 3}, []int32{1, 2, 3}) {
		t.Error("equal deps should return true")
	}
	if depsEqual([]int32{1, 2, 3}, []int32{1, 2, 4}) {
		t.Error("different deps should return false")
	}
	if !depsEqual([]int32{}, []int32{}) {
		t.Error("empty deps should be equal")
	}
}

func TestUpdateDeferred(t *testing.T) {
	// Reset deferMap for test isolation.
	deferMap = make(map[uint64]uint64)

	updateDeferred(0, 5, 1, 10)

	present, dq, di := deferredByInstance(1, 10)
	if !present {
		t.Fatal("expected deferred entry to be present")
	}
	if dq != 0 || di != 5 {
		t.Errorf("deferredByInstance(1,10) = (%d,%d), want (0,5)", dq, di)
	}
}

func TestDeferredByInstance_Missing(t *testing.T) {
	deferMap = make(map[uint64]uint64)

	present, _, _ := deferredByInstance(99, 99)
	if present {
		t.Error("expected no deferred entry")
	}
}

func TestUpdateDeferred_Overwrite(t *testing.T) {
	deferMap = make(map[uint64]uint64)

	updateDeferred(0, 5, 1, 10)
	updateDeferred(2, 7, 1, 10)

	present, dq, di := deferredByInstance(1, 10)
	if !present {
		t.Fatal("expected deferred entry")
	}
	if dq != 2 || di != 7 {
		t.Errorf("expected overwritten value (2,7), got (%d,%d)", dq, di)
	}
}

func TestBatchingEnabled(t *testing.T) {
	r := &Replica{batchWait: 0}
	if r.BatchingEnabled() {
		t.Error("batchWait=0 should not enable batching")
	}
	r.batchWait = 5
	if !r.BatchingEnabled() {
		t.Error("batchWait=5 should enable batching")
	}
}

func TestClientSupportsWeak(t *testing.T) {
	c := &Client{BufferClient: nil}
	if c.SupportsWeak() {
		t.Error("EPaxos client should not support weak consistency")
	}
}

func TestClientMarkAllSent(t *testing.T) {
	c := &Client{BufferClient: nil}
	c.MarkAllSent() // should not panic
}
