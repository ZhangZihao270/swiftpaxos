package client

import (
	"math/rand"
	"testing"
	"time"
)

// TestConsistencyLevelString tests ConsistencyLevel String method
func TestConsistencyLevelString(t *testing.T) {
	tests := []struct {
		level    ConsistencyLevel
		expected string
	}{
		{Strong, "Strong"},
		{Weak, "Weak"},
		{ConsistencyLevel(99), "Unknown"},
	}

	for _, tc := range tests {
		if got := tc.level.String(); got != tc.expected {
			t.Errorf("ConsistencyLevel(%d).String() = %s, want %s", tc.level, got, tc.expected)
		}
	}
}

// TestCommandTypeString tests CommandType String method
func TestCommandTypeString(t *testing.T) {
	tests := []struct {
		cmdType  CommandType
		expected string
	}{
		{StrongWrite, "StrongWrite"},
		{StrongRead, "StrongRead"},
		{WeakWrite, "WeakWrite"},
		{WeakRead, "WeakRead"},
		{CommandType(99), "Unknown"},
	}

	for _, tc := range tests {
		if got := tc.cmdType.String(); got != tc.expected {
			t.Errorf("CommandType(%d).String() = %s, want %s", tc.cmdType, got, tc.expected)
		}
	}
}

// TestGetCommandType tests GetCommandType function
func TestGetCommandType(t *testing.T) {
	tests := []struct {
		isWeak   bool
		isWrite  bool
		expected CommandType
	}{
		{false, false, StrongRead},
		{false, true, StrongWrite},
		{true, false, WeakRead},
		{true, true, WeakWrite},
	}

	for _, tc := range tests {
		if got := GetCommandType(tc.isWeak, tc.isWrite); got != tc.expected {
			t.Errorf("GetCommandType(%v, %v) = %v, want %v", tc.isWeak, tc.isWrite, got, tc.expected)
		}
	}
}

// TestNewHybridMetrics tests HybridMetrics initialization
func TestNewHybridMetrics(t *testing.T) {
	m := NewHybridMetrics(1000)

	if m == nil {
		t.Fatal("NewHybridMetrics returned nil")
	}

	if m.StrongWriteCount != 0 || m.StrongReadCount != 0 ||
		m.WeakWriteCount != 0 || m.WeakReadCount != 0 {
		t.Error("Counts should be initialized to 0")
	}

	if cap(m.StrongWriteLatency) < 250 || cap(m.StrongReadLatency) < 250 ||
		cap(m.WeakWriteLatency) < 250 || cap(m.WeakReadLatency) < 250 {
		t.Error("Latency slices should be pre-allocated")
	}
}

// TestComputePercentiles tests percentile computation
func TestComputePercentiles(t *testing.T) {
	// Empty slice
	avg, median, p99, p999 := computePercentiles([]float64{})
	if avg != 0 || median != 0 || p99 != 0 || p999 != 0 {
		t.Error("Empty slice should return zeros")
	}

	// Single element
	avg, median, p99, p999 = computePercentiles([]float64{5.0})
	if avg != 5.0 || median != 5.0 || p99 != 5.0 || p999 != 5.0 {
		t.Errorf("Single element: got avg=%v, median=%v, p99=%v, p999=%v", avg, median, p99, p999)
	}

	// Known distribution
	data := make([]float64, 100)
	for i := 0; i < 100; i++ {
		data[i] = float64(i + 1) // 1 to 100
	}
	avg, median, p99, p999 = computePercentiles(data)

	// Avg should be 50.5
	if avg < 50.4 || avg > 50.6 {
		t.Errorf("Avg should be 50.5, got %v", avg)
	}

	// Median should be around 50-51
	if median < 50 || median > 51 {
		t.Errorf("Median should be around 50-51, got %v", median)
	}

	// P99 should be around 99-100
	if p99 < 99 || p99 > 100 {
		t.Errorf("P99 should be around 99-100, got %v", p99)
	}

	// P999 should be around 99-100
	if p999 < 99 || p999 > 100 {
		t.Errorf("P999 should be around 99-100, got %v", p999)
	}
}

// mockHybridClient implements HybridClient for testing
type mockHybridClient struct {
	strongWriteCalled bool
	strongReadCalled  bool
	weakWriteCalled   bool
	weakReadCalled    bool
	supportsWeak      bool
}

func (m *mockHybridClient) SendStrongWrite(key int64, value []byte) int32 {
	m.strongWriteCalled = true
	return 1
}

func (m *mockHybridClient) SendStrongRead(key int64) int32 {
	m.strongReadCalled = true
	return 2
}

func (m *mockHybridClient) SendWeakWrite(key int64, value []byte) int32 {
	m.weakWriteCalled = true
	return 3
}

func (m *mockHybridClient) SendWeakRead(key int64) int32 {
	m.weakReadCalled = true
	return 4
}

func (m *mockHybridClient) SupportsWeak() bool {
	return m.supportsWeak
}

// TestHybridClientInterface tests the HybridClient interface
func TestHybridClientInterface(t *testing.T) {
	mock := &mockHybridClient{supportsWeak: true}

	// Verify interface compliance
	var _ HybridClient = mock

	// Test method calls
	mock.SendStrongWrite(1, []byte("test"))
	if !mock.strongWriteCalled {
		t.Error("SendStrongWrite not called")
	}

	mock.SendStrongRead(1)
	if !mock.strongReadCalled {
		t.Error("SendStrongRead not called")
	}

	mock.SendWeakWrite(1, []byte("test"))
	if !mock.weakWriteCalled {
		t.Error("SendWeakWrite not called")
	}

	mock.SendWeakRead(1)
	if !mock.weakReadCalled {
		t.Error("SendWeakRead not called")
	}

	if !mock.SupportsWeak() {
		t.Error("SupportsWeak should return true")
	}
}

// TestDecideCommandTypeAllStrong tests DecideCommandType with weakRatio=0
func TestDecideCommandTypeAllStrong(t *testing.T) {
	// Create a minimal BufferClient for testing
	source := rand.NewSource(time.Now().UnixNano())
	bc := &BufferClient{
		writes: 50,
		rand:   rand.New(source),
	}
	hbc := &HybridBufferClient{
		BufferClient: bc,
		weakRatio:    0, // All strong commands
		weakWrites:   50,
	}

	// With weakRatio=0, should always return isWeak=false
	for i := 0; i < 100; i++ {
		isWeak, _ := hbc.DecideCommandType()
		if isWeak {
			t.Error("With weakRatio=0, isWeak should always be false")
		}
	}
}

// TestDecideCommandTypeAllWeak tests DecideCommandType with weakRatio=100
func TestDecideCommandTypeAllWeak(t *testing.T) {
	source := rand.NewSource(time.Now().UnixNano())
	bc := &BufferClient{
		writes: 50,
		rand:   rand.New(source),
	}
	hbc := &HybridBufferClient{
		BufferClient: bc,
		weakRatio:    100, // All weak commands
		weakWrites:   50,
	}

	// With weakRatio=100, should always return isWeak=true
	for i := 0; i < 100; i++ {
		isWeak, _ := hbc.DecideCommandType()
		if !isWeak {
			t.Error("With weakRatio=100, isWeak should always be true")
		}
	}
}

// TestRecordLatency tests recordLatency method
func TestRecordLatency(t *testing.T) {
	bc := &BufferClient{}
	hbc := &HybridBufferClient{
		BufferClient: bc,
		Metrics:      NewHybridMetrics(100),
	}

	// Record latencies for each type
	hbc.recordLatency(StrongWrite, 10.0)
	hbc.recordLatency(StrongWrite, 20.0)
	hbc.recordLatency(StrongRead, 5.0)
	hbc.recordLatency(WeakWrite, 2.0)
	hbc.recordLatency(WeakRead, 1.0)
	hbc.recordLatency(WeakRead, 1.5)

	// Verify counts
	if hbc.Metrics.StrongWriteCount != 2 {
		t.Errorf("StrongWriteCount = %d, want 2", hbc.Metrics.StrongWriteCount)
	}
	if hbc.Metrics.StrongReadCount != 1 {
		t.Errorf("StrongReadCount = %d, want 1", hbc.Metrics.StrongReadCount)
	}
	if hbc.Metrics.WeakWriteCount != 1 {
		t.Errorf("WeakWriteCount = %d, want 1", hbc.Metrics.WeakWriteCount)
	}
	if hbc.Metrics.WeakReadCount != 2 {
		t.Errorf("WeakReadCount = %d, want 2", hbc.Metrics.WeakReadCount)
	}

	// Verify latencies recorded
	if len(hbc.Metrics.StrongWriteLatency) != 2 {
		t.Errorf("StrongWriteLatency length = %d, want 2", len(hbc.Metrics.StrongWriteLatency))
	}
	if hbc.Metrics.StrongWriteLatency[0] != 10.0 || hbc.Metrics.StrongWriteLatency[1] != 20.0 {
		t.Error("StrongWriteLatency values incorrect")
	}
}

// TestMetricsString tests MetricsString method
func TestMetricsString(t *testing.T) {
	bc := &BufferClient{}
	hbc := &HybridBufferClient{
		BufferClient: bc,
		Metrics:      NewHybridMetrics(100),
	}

	hbc.Metrics.StrongWriteCount = 10
	hbc.Metrics.StrongReadCount = 20
	hbc.Metrics.WeakWriteCount = 5
	hbc.Metrics.WeakReadCount = 15

	expected := "StrongW:10 StrongR:20 WeakW:5 WeakR:15"
	if got := hbc.MetricsString(); got != expected {
		t.Errorf("MetricsString() = %s, want %s", got, expected)
	}
}

// TestSupportsHybrid tests SupportsHybrid method
func TestSupportsHybrid(t *testing.T) {
	bc := &BufferClient{}
	hbc := &HybridBufferClient{
		BufferClient: bc,
	}

	// Without hybrid client set
	if hbc.SupportsHybrid() {
		t.Error("SupportsHybrid should return false when hybrid is nil")
	}

	// With hybrid client that doesn't support weak
	hbc.SetHybridClient(&mockHybridClient{supportsWeak: false})
	if hbc.SupportsHybrid() {
		t.Error("SupportsHybrid should return false when SupportsWeak is false")
	}

	// With hybrid client that supports weak
	hbc.SetHybridClient(&mockHybridClient{supportsWeak: true})
	if !hbc.SupportsHybrid() {
		t.Error("SupportsHybrid should return true when hybrid supports weak")
	}
}
