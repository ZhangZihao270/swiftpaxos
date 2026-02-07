package state

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"testing"
)

// TestComputeResultGET tests that ComputeResult returns value without modifying state
func TestComputeResultGET(t *testing.T) {
	st := InitState()

	// First, put a value using Execute
	putCmd := Command{Op: PUT, K: Key(100), V: Value([]byte("hello"))}
	putCmd.Execute(st)

	// Now test ComputeResult for GET
	getCmd := Command{Op: GET, K: Key(100), V: NIL()}
	result := getCmd.ComputeResult(st)

	if !bytes.Equal(result, []byte("hello")) {
		t.Errorf("ComputeResult(GET) = %v, want 'hello'", result)
	}

	// Verify state is unchanged (still has the value)
	result2 := getCmd.Execute(st)
	if !bytes.Equal(result2, []byte("hello")) {
		t.Errorf("State was modified by ComputeResult")
	}
}

// TestComputeResultPUT tests that ComputeResult does NOT modify state for PUT
func TestComputeResultPUT(t *testing.T) {
	st := InitState()

	// Use ComputeResult for PUT - should NOT modify state
	putCmd := Command{Op: PUT, K: Key(200), V: Value([]byte("world"))}
	result := putCmd.ComputeResult(st)

	// PUT should return NIL during speculation
	if len(result) != 0 {
		t.Errorf("ComputeResult(PUT) = %v, want NIL", result)
	}

	// Verify state was NOT modified
	getCmd := Command{Op: GET, K: Key(200), V: NIL()}
	getResult := getCmd.Execute(st)
	if len(getResult) != 0 {
		t.Errorf("State was modified by ComputeResult(PUT), found %v", getResult)
	}

	// Now actually execute the PUT
	putCmd.Execute(st)

	// Now the value should be present
	getResult = getCmd.Execute(st)
	if !bytes.Equal(getResult, []byte("world")) {
		t.Errorf("Execute(PUT) did not modify state correctly")
	}
}

// TestComputeResultSCAN tests that ComputeResult returns values for SCAN without modifying state
func TestComputeResultSCAN(t *testing.T) {
	st := InitState()

	// Put some values
	for i := int64(0); i < 5; i++ {
		cmd := Command{Op: PUT, K: Key(i), V: Value([]byte{byte(i)})}
		cmd.Execute(st)
	}

	// Create SCAN command (scan 3 keys starting from key 1)
	scanCount := make([]byte, 8)
	binary.LittleEndian.PutUint64(scanCount, 3)
	scanCmd := Command{Op: SCAN, K: Key(1), V: Value(scanCount)}

	// Use ComputeResult - should return values without modifying state
	result := scanCmd.ComputeResult(st)

	// Should have values for keys 1, 2, 3, 4 (within range 1 to 1+3=4)
	// Each value is a single byte
	if len(result) != 4 {
		t.Errorf("ComputeResult(SCAN) returned %d bytes, want 4", len(result))
	}

	// Verify state is unchanged - do a GET to check
	getCmd := Command{Op: GET, K: Key(2), V: NIL()}
	getResult := getCmd.Execute(st)
	if len(getResult) != 1 || getResult[0] != 2 {
		t.Errorf("State was modified by ComputeResult(SCAN)")
	}
}

// TestComputeResultNONE tests that ComputeResult returns NIL for NONE
func TestComputeResultNONE(t *testing.T) {
	st := InitState()

	cmd := Command{Op: NONE, K: Key(0), V: NIL()}
	result := cmd.ComputeResult(st)

	if len(result) != 0 {
		t.Errorf("ComputeResult(NONE) = %v, want NIL", result)
	}
}

// TestComputeResultGetMissing tests GET for non-existent key
func TestComputeResultGetMissing(t *testing.T) {
	st := InitState()

	getCmd := Command{Op: GET, K: Key(999), V: NIL()}
	result := getCmd.ComputeResult(st)

	if len(result) != 0 {
		t.Errorf("ComputeResult(GET missing) = %v, want NIL", result)
	}
}

// TestComputeResultVsExecute tests that ComputeResult and Execute return same result for reads
func TestComputeResultVsExecute(t *testing.T) {
	st := InitState()

	// Setup: put some values
	cmd1 := Command{Op: PUT, K: Key(1), V: Value([]byte("value1"))}
	cmd1.Execute(st)
	cmd2 := Command{Op: PUT, K: Key(2), V: Value([]byte("value2"))}
	cmd2.Execute(st)

	// Test GET - ComputeResult and Execute should return same value
	getCmd := Command{Op: GET, K: Key(1), V: NIL()}
	computeResult := getCmd.ComputeResult(st)
	executeResult := getCmd.Execute(st)

	if !bytes.Equal(computeResult, executeResult) {
		t.Errorf("ComputeResult(GET) = %v, Execute(GET) = %v, should be equal",
			computeResult, executeResult)
	}
}

// TestExecuteModifiesState confirms Execute actually modifies state (baseline test)
func TestExecuteModifiesState(t *testing.T) {
	st := InitState()

	// Before PUT
	getCmd := Command{Op: GET, K: Key(100), V: NIL()}
	result := getCmd.Execute(st)
	if len(result) != 0 {
		t.Error("State should be empty initially")
	}

	// Execute PUT
	putCmd := Command{Op: PUT, K: Key(100), V: Value([]byte("test"))}
	putCmd.Execute(st)

	// After PUT
	result = getCmd.Execute(st)
	if !bytes.Equal(result, []byte("test")) {
		t.Errorf("Execute(PUT) did not modify state, got %v", result)
	}
}

// TestPUTOverwrite tests that PUT overwrites existing values correctly
func TestPUTOverwrite(t *testing.T) {
	st := InitState()

	putCmd := Command{Op: PUT, K: Key(42), V: Value([]byte("first"))}
	putCmd.Execute(st)

	getCmd := Command{Op: GET, K: Key(42), V: NIL()}
	result := getCmd.Execute(st)
	if !bytes.Equal(result, []byte("first")) {
		t.Errorf("First PUT: got %v, want 'first'", result)
	}

	// Overwrite
	putCmd2 := Command{Op: PUT, K: Key(42), V: Value([]byte("second"))}
	putCmd2.Execute(st)

	result = getCmd.Execute(st)
	if !bytes.Equal(result, []byte("second")) {
		t.Errorf("Overwrite PUT: got %v, want 'second'", result)
	}
}

// TestSCANEmptyRange tests SCAN when no keys exist in the range
func TestSCANEmptyRange(t *testing.T) {
	st := InitState()

	// Put values outside the scan range
	cmd := Command{Op: PUT, K: Key(100), V: Value([]byte("far"))}
	cmd.Execute(st)

	// Scan range 0..5 (empty since the key is at 100)
	scanCount := make([]byte, 8)
	binary.LittleEndian.PutUint64(scanCount, 5)
	scanCmd := Command{Op: SCAN, K: Key(0), V: Value(scanCount)}

	result := scanCmd.Execute(st)
	if len(result) != 0 {
		t.Errorf("SCAN empty range: got %d bytes, want 0", len(result))
	}
}

// TestSCANDeterministicOrder tests that SCAN returns values in key order
func TestSCANDeterministicOrder(t *testing.T) {
	st := InitState()

	// Insert keys in reverse order to test sorting
	for i := int64(9); i >= 0; i-- {
		cmd := Command{Op: PUT, K: Key(i), V: Value([]byte{byte(i)})}
		cmd.Execute(st)
	}

	scanCount := make([]byte, 8)
	binary.LittleEndian.PutUint64(scanCount, 9)
	scanCmd := Command{Op: SCAN, K: Key(0), V: Value(scanCount)}

	result := scanCmd.Execute(st)
	if len(result) != 10 {
		t.Fatalf("SCAN: got %d bytes, want 10", len(result))
	}

	// Verify sorted order
	for i := 0; i < 10; i++ {
		if result[i] != byte(i) {
			t.Errorf("SCAN order: result[%d] = %d, want %d", i, result[i], i)
		}
	}
}

// TestMarshalUnmarshalCommand tests serialization round-trip
func TestMarshalUnmarshalCommand(t *testing.T) {
	original := Command{Op: PUT, K: Key(12345), V: Value([]byte("testval"))}

	var buf bytes.Buffer
	original.Marshal(&buf)

	var decoded Command
	err := decoded.Unmarshal(&buf)
	if err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if decoded.Op != original.Op || decoded.K != original.K || !bytes.Equal(decoded.V, original.V) {
		t.Errorf("Round-trip failed: got %v, want %v", decoded, original)
	}
}

// TestConflictBasic tests basic conflict detection
func TestConflictBasic(t *testing.T) {
	putA := &Command{Op: PUT, K: Key(10), V: Value([]byte("a"))}
	putB := &Command{Op: PUT, K: Key(10), V: Value([]byte("b"))}
	getA := &Command{Op: GET, K: Key(10), V: NIL()}
	getB := &Command{Op: GET, K: Key(20), V: NIL()}

	// Two PUTs to same key conflict
	if !Conflict(putA, putB) {
		t.Error("Two PUTs to same key should conflict")
	}

	// PUT and GET to same key conflict
	if !Conflict(putA, getA) {
		t.Error("PUT and GET to same key should conflict")
	}

	// Two GETs to same key don't conflict
	if Conflict(getA, getA) {
		t.Error("Two GETs should not conflict")
	}

	// PUT and GET to different keys don't conflict
	if Conflict(putA, getB) {
		t.Error("PUT(10) and GET(20) should not conflict")
	}
}

// TestConflictBatch tests batch conflict detection
func TestConflictBatch(t *testing.T) {
	batch1 := []Command{{Op: PUT, K: Key(1), V: Value([]byte("a"))}}
	batch2 := []Command{{Op: GET, K: Key(1), V: NIL()}}
	batch3 := []Command{{Op: GET, K: Key(2), V: NIL()}}

	if !ConflictBatch(batch1, batch2) {
		t.Error("PUT(1) batch should conflict with GET(1) batch")
	}
	if ConflictBatch(batch1, batch3) {
		t.Error("PUT(1) batch should not conflict with GET(2) batch")
	}
}

// TestIsRead tests the IsRead helper
func TestIsRead(t *testing.T) {
	get := &Command{Op: GET, K: Key(1), V: NIL()}
	put := &Command{Op: PUT, K: Key(1), V: Value([]byte("x"))}
	none := &Command{Op: NONE, K: Key(0), V: NIL()}

	if !IsRead(get) {
		t.Error("GET should be a read")
	}
	if IsRead(put) {
		t.Error("PUT should not be a read")
	}
	if IsRead(none) {
		t.Error("NONE should not be a read")
	}
}

// TestNOOP tests the NOOP command generator
func TestNOOP(t *testing.T) {
	noop := NOOP()
	if len(noop) != 1 {
		t.Fatalf("NOOP should have 1 command, got %d", len(noop))
	}
	if noop[0].Op != NONE {
		t.Errorf("NOOP op = %d, want NONE(%d)", noop[0].Op, NONE)
	}
}

// --- Benchmarks ---

// BenchmarkExecutePUT benchmarks PUT operations on the map-backed state
func BenchmarkExecutePUT(b *testing.B) {
	st := InitState()
	cmd := Command{Op: PUT, K: Key(0), V: Value([]byte("benchvalue"))}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cmd.K = Key(int64(i))
		cmd.Execute(st)
	}
}

// BenchmarkExecuteGET benchmarks GET operations (with pre-populated state)
func BenchmarkExecuteGET(b *testing.B) {
	st := InitState()
	// Pre-populate
	for i := int64(0); i < 10000; i++ {
		cmd := Command{Op: PUT, K: Key(i), V: Value([]byte("val"))}
		cmd.Execute(st)
	}

	getCmd := Command{Op: GET, K: Key(0), V: NIL()}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		getCmd.K = Key(int64(i % 10000))
		getCmd.Execute(st)
	}
}

// BenchmarkExecuteGETMiss benchmarks GET for non-existent keys
func BenchmarkExecuteGETMiss(b *testing.B) {
	st := InitState()
	// Pre-populate range 0..999
	for i := int64(0); i < 1000; i++ {
		cmd := Command{Op: PUT, K: Key(i), V: Value([]byte("val"))}
		cmd.Execute(st)
	}

	getCmd := Command{Op: GET, K: Key(0), V: NIL()}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		getCmd.K = Key(int64(i + 100000)) // Always miss
		getCmd.Execute(st)
	}
}

// BenchmarkExecuteSCAN benchmarks SCAN operations
func BenchmarkExecuteSCAN(b *testing.B) {
	st := InitState()
	for i := int64(0); i < 1000; i++ {
		cmd := Command{Op: PUT, K: Key(i), V: Value([]byte("val"))}
		cmd.Execute(st)
	}

	scanCount := make([]byte, 8)
	binary.LittleEndian.PutUint64(scanCount, 10)
	scanCmd := Command{Op: SCAN, K: Key(0), V: Value(scanCount)}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		scanCmd.Execute(st)
	}
}

// BenchmarkExecuteMixedWorkload benchmarks a realistic workload (90% GET, 10% PUT)
func BenchmarkExecuteMixedWorkload(b *testing.B) {
	st := InitState()
	// Pre-populate
	for i := int64(0); i < 10000; i++ {
		cmd := Command{Op: PUT, K: Key(i), V: Value([]byte("val"))}
		cmd.Execute(st)
	}

	getCmd := Command{Op: GET, K: Key(0), V: NIL()}
	putCmd := Command{Op: PUT, K: Key(0), V: Value([]byte("newval"))}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if i%10 == 0 {
			putCmd.K = Key(int64(i % 10000))
			putCmd.Execute(st)
		} else {
			getCmd.K = Key(int64(i % 10000))
			getCmd.Execute(st)
		}
	}
}

// BenchmarkComputeResultGET benchmarks speculative GET (ComputeResult)
func BenchmarkComputeResultGET(b *testing.B) {
	st := InitState()
	for i := int64(0); i < 10000; i++ {
		cmd := Command{Op: PUT, K: Key(i), V: Value([]byte("val"))}
		cmd.Execute(st)
	}

	getCmd := Command{Op: GET, K: Key(0), V: NIL()}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		getCmd.K = Key(int64(i % 10000))
		getCmd.ComputeResult(st)
	}
}

// BenchmarkCommandMarshal benchmarks Command serialization (allocation-free)
func BenchmarkCommandMarshal(b *testing.B) {
	cmd := Command{Op: PUT, K: Key(100), V: Value([]byte("benchvalue"))}
	var buf bytes.Buffer

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		cmd.Marshal(&buf)
	}
}

// BenchmarkCommandUnmarshal benchmarks Command deserialization
func BenchmarkCommandUnmarshal(b *testing.B) {
	cmd := Command{Op: PUT, K: Key(100), V: Value([]byte("benchvalue"))}
	var buf bytes.Buffer
	cmd.Marshal(&buf)
	data := buf.Bytes()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var restored Command
		restored.Unmarshal(bytes.NewReader(data))
	}
}

// BenchmarkCommandRoundTrip benchmarks full Marshal+Unmarshal cycle
func BenchmarkCommandRoundTrip(b *testing.B) {
	cmd := Command{Op: PUT, K: Key(100), V: Value([]byte("benchvalue"))}
	var buf bytes.Buffer

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		cmd.Marshal(&buf)
		var restored Command
		restored.Unmarshal(&buf)
	}
}

// BenchmarkStateScaling benchmarks GET/PUT at different state sizes
func BenchmarkStateScaling(b *testing.B) {
	for _, size := range []int{100, 1000, 10000, 100000} {
		b.Run(fmt.Sprintf("GET/size=%d", size), func(b *testing.B) {
			st := InitState()
			for i := int64(0); i < int64(size); i++ {
				cmd := Command{Op: PUT, K: Key(i), V: Value([]byte("v"))}
				cmd.Execute(st)
			}
			getCmd := Command{Op: GET, K: Key(0), V: NIL()}
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				getCmd.K = Key(int64(i % size))
				getCmd.Execute(st)
			}
		})
		b.Run(fmt.Sprintf("PUT/size=%d", size), func(b *testing.B) {
			st := InitState()
			for i := int64(0); i < int64(size); i++ {
				cmd := Command{Op: PUT, K: Key(i), V: Value([]byte("v"))}
				cmd.Execute(st)
			}
			putCmd := Command{Op: PUT, K: Key(0), V: Value([]byte("new"))}
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				putCmd.K = Key(int64(i % size))
				putCmd.Execute(st)
			}
		})
	}
}
