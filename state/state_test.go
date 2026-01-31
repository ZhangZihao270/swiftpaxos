package state

import (
	"bytes"
	"encoding/binary"
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
