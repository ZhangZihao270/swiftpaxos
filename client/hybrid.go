package client

import (
	"fmt"
	"log"
	"sort"
	"sync"
	"time"

	"github.com/imdea-software/swiftpaxos/state"
)

// replyTimeout is the maximum time to wait for a single reply before
// declaring a hang. This prevents clients from blocking forever when
// a reply is lost due to protocol bugs or network issues.
const replyTimeout = 120 * time.Second

// ConsistencyLevel represents the consistency guarantee for a command
type ConsistencyLevel int

const (
	// Strong provides linearizable consistency (2-RTT for writes)
	Strong ConsistencyLevel = iota
	// Weak provides causal consistency (1-RTT)
	Weak
)

// String returns the string representation of ConsistencyLevel
func (c ConsistencyLevel) String() string {
	switch c {
	case Strong:
		return "Strong"
	case Weak:
		return "Weak"
	default:
		return "Unknown"
	}
}

// HybridClient defines the interface for clients that support both strong and weak consistency.
// Protocols that support hybrid consistency should implement this interface.
// Protocols that only support strong consistency can use the default BufferClient methods.
type HybridClient interface {
	// SendStrongWrite sends a linearizable write command.
	// Returns the command sequence number.
	SendStrongWrite(key int64, value []byte) int32

	// SendStrongRead sends a linearizable read command.
	// Returns the command sequence number.
	SendStrongRead(key int64) int32

	// SendWeakWrite sends a causally consistent write command.
	// Returns the command sequence number.
	SendWeakWrite(key int64, value []byte) int32

	// SendWeakRead sends a causally consistent read command.
	// Returns the command sequence number.
	SendWeakRead(key int64) int32

	// SupportsWeak returns true if the client supports weak consistency commands.
	SupportsWeak() bool

	// MarkAllSent signals that all commands have been sent (drain mode).
	// Used by protocols with MSync retry to force-deliver permanently stuck commands.
	MarkAllSent()
}

// HybridMetrics tracks per-consistency-level metrics for the hybrid benchmark.
type HybridMetrics struct {
	// Strong command metrics
	StrongWriteCount    int
	StrongReadCount     int
	StrongWriteLatency  []float64 // in milliseconds
	StrongReadLatency   []float64 // in milliseconds

	// Weak command metrics
	WeakWriteCount    int
	WeakReadCount     int
	WeakWriteLatency  []float64 // in milliseconds
	WeakReadLatency   []float64 // in milliseconds
}

// NewHybridMetrics creates a new HybridMetrics with pre-allocated slices.
func NewHybridMetrics(expectedOps int) *HybridMetrics {
	return &HybridMetrics{
		StrongWriteLatency: make([]float64, 0, expectedOps/4),
		StrongReadLatency:  make([]float64, 0, expectedOps/4),
		WeakWriteLatency:   make([]float64, 0, expectedOps/4),
		WeakReadLatency:    make([]float64, 0, expectedOps/4),
	}
}

// CommandType represents the type of command (for metrics tracking)
type CommandType int

const (
	StrongWrite CommandType = iota
	StrongRead
	WeakWrite
	WeakRead
)

// String returns the string representation of CommandType
func (c CommandType) String() string {
	switch c {
	case StrongWrite:
		return "StrongWrite"
	case StrongRead:
		return "StrongRead"
	case WeakWrite:
		return "WeakWrite"
	case WeakRead:
		return "WeakRead"
	default:
		return "Unknown"
	}
}

// HybridReqReply extends ReqReply with consistency level information.
type HybridReqReply struct {
	*ReqReply
	CmdType CommandType
}

// HybridBufferClient extends BufferClient with hybrid consistency support.
type HybridBufferClient struct {
	*BufferClient

	// Hybrid benchmark configuration
	weakRatio   int // Percentage of commands that use weak consistency (0-100)
	weakWrites  int // Percentage of weak commands that are writes (0-100)

	// HybridClient implementation (set by protocol-specific client)
	hybrid HybridClient

	// Metrics tracking
	Metrics *HybridMetrics

	// Reply channel with command type information
	HybridReply chan *HybridReqReply

	// Duration of the benchmark (for aggregation)
	duration time.Duration
}

// NewHybridBufferClient creates a new HybridBufferClient wrapping an existing BufferClient.
func NewHybridBufferClient(bc *BufferClient, weakRatio, weakWrites int) *HybridBufferClient {
	hbc := &HybridBufferClient{
		BufferClient: bc,
		weakRatio:    weakRatio,
		weakWrites:   weakWrites,
		Metrics:      NewHybridMetrics(bc.reqNum),
		HybridReply:  make(chan *HybridReqReply, bc.reqNum+1),
	}
	return hbc
}

// SetHybridClient sets the underlying HybridClient implementation.
// This should be called by protocol-specific clients after initialization.
func (c *HybridBufferClient) SetHybridClient(h HybridClient) {
	c.hybrid = h
}

// SupportsHybrid returns true if the client has a HybridClient implementation.
// A HybridClient that does not support weak consistency (e.g., Raft) still
// benefits from the hybrid loop for metrics tracking and pipelining;
// DecideCommandType with weakRatio=0 ensures all commands are strong.
func (c *HybridBufferClient) SupportsHybrid() bool {
	return c.hybrid != nil
}

// RegisterHybridReply records a command completion with timing and type information.
func (c *HybridBufferClient) RegisterHybridReply(val state.Value, seqnum int32, cmdType CommandType) {
	rr := &ReqReply{
		Val:    val,
		Seqnum: int(seqnum),
	}
	c.HybridReply <- &HybridReqReply{
		ReqReply: rr,
		CmdType:  cmdType,
	}
}

// DecideCommandType determines whether the next command should be strong/weak and read/write.
// Returns (isWeak, isWrite)
func (c *HybridBufferClient) DecideCommandType() (bool, bool) {
	isWeak := c.randomTrue(c.weakRatio)

	var isWrite bool
	if isWeak {
		isWrite = c.randomTrue(c.weakWrites)
	} else {
		isWrite = c.randomTrue(c.writes)
	}

	return isWeak, isWrite
}

// GetCommandType returns the CommandType based on isWeak and isWrite flags.
func GetCommandType(isWeak, isWrite bool) CommandType {
	if isWeak {
		if isWrite {
			return WeakWrite
		}
		return WeakRead
	}
	if isWrite {
		return StrongWrite
	}
	return StrongRead
}

// HybridLoop runs the hybrid consistency benchmark.
// It uses weakRatio to decide between strong and weak commands,
// and writes/weakWrites to decide between reads and writes.
func (c *HybridBufferClient) HybridLoop() {
	if !c.SupportsHybrid() {
		c.Println("Warning: HybridClient not set or does not support weak consistency, falling back to Loop()")
		c.Loop()
		return
	}

	getKey := c.genGetKey()
	val := make([]byte, c.psize)
	c.rand.Read(val)

	// Track command types for metrics
	cmdTypes := make([]CommandType, c.reqNum+1)

	var cmdM sync.Mutex
	cmdNum := int32(0)
	wait := make(chan struct{}, 0)

	// Reply processing goroutine
	timedOut := make(chan struct{})
	go func() {
		for i := 0; i <= c.reqNum; i++ {
			var r *ReqReply
			select {
			case r = <-c.Reply:
			case <-time.After(replyTimeout):
				sw, sr, ww, wr := c.Metrics.StrongWriteCount, c.Metrics.StrongReadCount,
					c.Metrics.WeakWriteCount, c.Metrics.WeakReadCount
				log.Printf("REPLY TIMEOUT: waited %v for reply %d/%d (received: StrongW=%d StrongR=%d WeakW=%d WeakR=%d total=%d)",
					replyTimeout, i, c.reqNum+1, sw, sr, ww, wr, sw+sr+ww+wr)
				close(timedOut)
				return
			}
			// Ignore first request (warmup)
			if i != 0 {
				d := r.Time.Sub(c.reqTime[r.Seqnum])
				latencyMs := float64(d.Nanoseconds()) / float64(time.Millisecond)
				cmdType := cmdTypes[r.Seqnum]
				c.recordLatency(cmdType, latencyMs)
				c.Println("Returning:", r.Val.String())
				c.Printf("latency %v (%s)\n", latencyMs, cmdType.String())
			}
			if c.window > 0 {
				cmdM.Lock()
				if cmdNum == c.window {
					cmdNum--
					cmdM.Unlock()
					wait <- struct{}{}
				} else {
					cmdNum--
					cmdM.Unlock()
				}
			}
			if c.seq || (c.syncFreq > 0 && i%c.syncFreq == 0) {
				wait <- struct{}{}
			}
		}
		if !c.seq {
			wait <- struct{}{}
		}
	}()

	// Command generation loop
	for i := 0; i <= c.reqNum; i++ {
		key := getKey()

		// First command (i=0, warmup) MUST be a strong command to set up ClientWriters.
		// Weak commands use custom RPC messages that don't register ClientWriters,
		// so we need at least one strong PROPOSE to establish the connection properly.
		var isWeak, isWrite bool
		if i == 0 {
			isWeak = false // Force strong for warmup
			isWrite = false
		} else {
			isWeak, isWrite = c.DecideCommandType()
		}
		cmdType := GetCommandType(isWeak, isWrite)
		cmdTypes[i] = cmdType

		c.reqTime[i] = time.Now()

		// Ignore first request (warmup)
		if i == 1 {
			c.launchTime = c.reqTime[i]
		}

		// Send command based on type
		switch cmdType {
		case StrongWrite:
			c.hybrid.SendStrongWrite(key, state.Value(val))
		case StrongRead:
			c.hybrid.SendStrongRead(key)
		case WeakWrite:
			c.hybrid.SendWeakWrite(key, state.Value(val))
		case WeakRead:
			c.hybrid.SendWeakRead(key)
		}

		// Pipelining window management
		if c.window > 0 {
			cmdM.Lock()
			if cmdNum == c.window-1 {
				cmdNum++
				cmdM.Unlock()
				select {
				case <-wait:
				case <-timedOut:
					c.hybrid.MarkAllSent()
					return
				}
			} else {
				cmdNum++
				cmdM.Unlock()
			}
		}
		if c.seq || (c.syncFreq > 0 && i%c.syncFreq == 0) {
			select {
			case <-wait:
			case <-timedOut:
				c.hybrid.MarkAllSent()
				return
			}
		}
	}

	// Signal drain mode: all commands sent, waiting for final replies
	c.hybrid.MarkAllSent()

	if !c.seq {
		select {
		case <-wait:
		case <-timedOut:
		}
	}

	duration := time.Now().Sub(c.launchTime)
	c.Printf("Test took %v\n", duration)
	c.PrintMetrics(duration)
	c.Disconnect()
}

// recordLatency records a latency measurement for the given command type.
func (c *HybridBufferClient) recordLatency(cmdType CommandType, latencyMs float64) {
	switch cmdType {
	case StrongWrite:
		c.Metrics.StrongWriteCount++
		c.Metrics.StrongWriteLatency = append(c.Metrics.StrongWriteLatency, latencyMs)
	case StrongRead:
		c.Metrics.StrongReadCount++
		c.Metrics.StrongReadLatency = append(c.Metrics.StrongReadLatency, latencyMs)
	case WeakWrite:
		c.Metrics.WeakWriteCount++
		c.Metrics.WeakWriteLatency = append(c.Metrics.WeakWriteLatency, latencyMs)
	case WeakRead:
		c.Metrics.WeakReadCount++
		c.Metrics.WeakReadLatency = append(c.Metrics.WeakReadLatency, latencyMs)
	}
}

// PrintMetrics outputs the hybrid benchmark metrics summary.
func (c *HybridBufferClient) PrintMetrics(duration time.Duration) {
	totalOps := c.reqNum // Exclude warmup request
	throughput := float64(totalOps) / duration.Seconds()

	strongOps := c.Metrics.StrongWriteCount + c.Metrics.StrongReadCount
	weakOps := c.Metrics.WeakWriteCount + c.Metrics.WeakReadCount

	c.Println("\n=== Hybrid Benchmark Results ===")
	c.Printf("Total operations: %d\n", totalOps)
	c.Printf("Duration: %.2fs\n", duration.Seconds())
	c.Printf("Throughput: %.2f ops/sec\n", throughput)

	if strongOps > 0 {
		strongPct := float64(strongOps) * 100 / float64(totalOps)
		c.Printf("\nStrong Operations: %d (%.1f%%)\n", strongOps, strongPct)
		c.Printf("  Writes: %d | Reads: %d\n", c.Metrics.StrongWriteCount, c.Metrics.StrongReadCount)

		allStrongLatencies := append(c.Metrics.StrongWriteLatency, c.Metrics.StrongReadLatency...)
		if len(allStrongLatencies) > 0 {
			avg, median, p99, p999 := computePercentiles(allStrongLatencies)
			c.Printf("  Avg: %.2fms | Median: %.2fms | P99: %.2fms | P99.9: %.2fms\n", avg, median, p99, p999)
		}
		if len(c.Metrics.StrongWriteLatency) > 0 {
			avg, median, p99, p999 := computePercentiles(c.Metrics.StrongWriteLatency)
			c.Printf("  Strong Write: Avg: %.2fms | Median: %.2fms | P99: %.2fms | P99.9: %.2fms\n", avg, median, p99, p999)
		}
		if len(c.Metrics.StrongReadLatency) > 0 {
			avg, median, p99, p999 := computePercentiles(c.Metrics.StrongReadLatency)
			c.Printf("  Strong Read:  Avg: %.2fms | Median: %.2fms | P99: %.2fms | P99.9: %.2fms\n", avg, median, p99, p999)
		}
	}

	if weakOps > 0 {
		weakPct := float64(weakOps) * 100 / float64(totalOps)
		c.Printf("\nWeak Operations: %d (%.1f%%)\n", weakOps, weakPct)
		c.Printf("  Writes: %d | Reads: %d\n", c.Metrics.WeakWriteCount, c.Metrics.WeakReadCount)

		allWeakLatencies := append(c.Metrics.WeakWriteLatency, c.Metrics.WeakReadLatency...)
		if len(allWeakLatencies) > 0 {
			avg, median, p99, p999 := computePercentiles(allWeakLatencies)
			c.Printf("  Avg: %.2fms | Median: %.2fms | P99: %.2fms | P99.9: %.2fms\n", avg, median, p99, p999)
		}
		if len(c.Metrics.WeakWriteLatency) > 0 {
			avg, median, p99, p999 := computePercentiles(c.Metrics.WeakWriteLatency)
			c.Printf("  Weak Write:  Avg: %.2fms | Median: %.2fms | P99: %.2fms | P99.9: %.2fms\n", avg, median, p99, p999)
		}
		if len(c.Metrics.WeakReadLatency) > 0 {
			avg, median, p99, p999 := computePercentiles(c.Metrics.WeakReadLatency)
			c.Printf("  Weak Read:   Avg: %.2fms | Median: %.2fms | P99: %.2fms | P99.9: %.2fms\n", avg, median, p99, p999)
		}
	}

	c.Println("================================")
}

// computePercentiles computes median, P99, and P99.9 from a slice of latencies.
func computePercentiles(latencies []float64) (avg, median, p99, p999 float64) {
	if len(latencies) == 0 {
		return 0, 0, 0, 0
	}

	sorted := make([]float64, len(latencies))
	copy(sorted, latencies)
	sort.Float64s(sorted)

	n := len(sorted)
	sum := 0.0
	for _, v := range sorted {
		sum += v
	}
	avg = sum / float64(n)
	median = sorted[n/2]
	p99 = sorted[int(float64(n)*0.99)]
	p999Idx := int(float64(n) * 0.999)
	if p999Idx >= n {
		p999Idx = n - 1
	}
	p999 = sorted[p999Idx]

	return avg, median, p99, p999
}

// MetricsString returns a formatted string of the metrics (for logging/testing).
func (c *HybridBufferClient) MetricsString() string {
	return fmt.Sprintf("StrongW:%d StrongR:%d WeakW:%d WeakR:%d",
		c.Metrics.StrongWriteCount, c.Metrics.StrongReadCount,
		c.Metrics.WeakWriteCount, c.Metrics.WeakReadCount)
}

// HybridLoopWithOptions runs the hybrid benchmark with options.
// If printResults is true, prints metrics at the end.
// If false, metrics can be retrieved via GetMetrics() for aggregation.
func (c *HybridBufferClient) HybridLoopWithOptions(printResults bool) {
	if !c.SupportsHybrid() {
		c.Println("Warning: HybridClient not set or does not support weak consistency, falling back to Loop()")
		c.Loop()
		return
	}

	getKey := c.genGetKey()
	val := make([]byte, c.psize)
	c.rand.Read(val)

	// Track command types for metrics
	cmdTypes := make([]CommandType, c.reqNum+1)

	var cmdM sync.Mutex
	cmdNum := int32(0)
	wait := make(chan struct{}, 0)

	// Reply processing goroutine
	timedOut := make(chan struct{})
	go func() {
		for i := 0; i <= c.reqNum; i++ {
			var r *ReqReply
			select {
			case r = <-c.Reply:
			case <-time.After(replyTimeout):
				// Count received replies by type for diagnostics
				sw, sr, ww, wr := c.Metrics.StrongWriteCount, c.Metrics.StrongReadCount,
					c.Metrics.WeakWriteCount, c.Metrics.WeakReadCount
				log.Printf("REPLY TIMEOUT: waited %v for reply %d/%d (received: StrongW=%d StrongR=%d WeakW=%d WeakR=%d total=%d)",
					replyTimeout, i, c.reqNum+1, sw, sr, ww, wr, sw+sr+ww+wr)
				close(timedOut)
				return
			}
			// Ignore first request (warmup)
			if i != 0 {
				d := r.Time.Sub(c.reqTime[r.Seqnum])
				latencyMs := float64(d.Nanoseconds()) / float64(time.Millisecond)
				cmdType := cmdTypes[r.Seqnum]
				c.recordLatency(cmdType, latencyMs)
				if printResults {
					c.Println("Returning:", r.Val.String())
					c.Printf("latency %v (%s)\n", latencyMs, cmdType.String())
				}
			}
			if c.window > 0 {
				cmdM.Lock()
				if cmdNum == c.window {
					cmdNum--
					cmdM.Unlock()
					wait <- struct{}{}
				} else {
					cmdNum--
					cmdM.Unlock()
				}
			}
			if c.seq || (c.syncFreq > 0 && i%c.syncFreq == 0) {
				wait <- struct{}{}
			}
		}
		if !c.seq {
			wait <- struct{}{}
		}
	}()

	// Command generation loop
	for i := 0; i <= c.reqNum; i++ {
		key := getKey()

		var isWeak, isWrite bool
		if i == 0 {
			isWeak = false // Force strong for warmup
			isWrite = false
		} else {
			isWeak, isWrite = c.DecideCommandType()
		}
		cmdType := GetCommandType(isWeak, isWrite)
		cmdTypes[i] = cmdType

		c.reqTime[i] = time.Now()

		if i == 1 {
			c.launchTime = c.reqTime[i]
		}

		switch cmdType {
		case StrongWrite:
			c.hybrid.SendStrongWrite(key, state.Value(val))
		case StrongRead:
			c.hybrid.SendStrongRead(key)
		case WeakWrite:
			c.hybrid.SendWeakWrite(key, state.Value(val))
		case WeakRead:
			c.hybrid.SendWeakRead(key)
		}

		if c.window > 0 {
			cmdM.Lock()
			if cmdNum == c.window-1 {
				cmdNum++
				cmdM.Unlock()
				select {
				case <-wait:
				case <-timedOut:
					c.hybrid.MarkAllSent()
					return
				}
			} else {
				cmdNum++
				cmdM.Unlock()
			}
		}
		if c.seq || (c.syncFreq > 0 && i%c.syncFreq == 0) {
			select {
			case <-wait:
			case <-timedOut:
				c.hybrid.MarkAllSent()
				return
			}
		}
	}

	// Signal drain mode: all commands sent, waiting for final replies
	c.hybrid.MarkAllSent()

	if !c.seq {
		select {
		case <-wait:
		case <-timedOut:
		}
	}

	c.duration = time.Now().Sub(c.launchTime)
	if printResults {
		c.Printf("Test took %v\n", c.duration)
		c.PrintMetrics(c.duration)
	}
	c.Disconnect()
}

// GetMetrics returns a copy of the metrics for aggregation.
func (c *HybridBufferClient) GetMetrics() *HybridMetrics {
	return c.Metrics
}

// GetDuration returns the benchmark duration.
func (c *HybridBufferClient) GetDuration() time.Duration {
	return c.duration
}

// AggregateMetrics combines metrics from multiple threads into one.
func AggregateMetrics(metrics []*HybridMetrics) *HybridMetrics {
	result := &HybridMetrics{
		StrongWriteLatency: make([]float64, 0),
		StrongReadLatency:  make([]float64, 0),
		WeakWriteLatency:   make([]float64, 0),
		WeakReadLatency:    make([]float64, 0),
	}

	for _, m := range metrics {
		if m == nil {
			continue
		}
		result.StrongWriteCount += m.StrongWriteCount
		result.StrongReadCount += m.StrongReadCount
		result.WeakWriteCount += m.WeakWriteCount
		result.WeakReadCount += m.WeakReadCount
		result.StrongWriteLatency = append(result.StrongWriteLatency, m.StrongWriteLatency...)
		result.StrongReadLatency = append(result.StrongReadLatency, m.StrongReadLatency...)
		result.WeakWriteLatency = append(result.WeakWriteLatency, m.WeakWriteLatency...)
		result.WeakReadLatency = append(result.WeakReadLatency, m.WeakReadLatency...)
	}

	return result
}

// Printer is an interface for logging output.
type Printer interface {
	Println(v ...interface{})
	Printf(format string, v ...interface{})
}

// Print outputs the aggregated metrics summary.
func (m *HybridMetrics) Print(p Printer, totalOps int, duration time.Duration) {
	strongOps := m.StrongWriteCount + m.StrongReadCount
	weakOps := m.WeakWriteCount + m.WeakReadCount
	actualTotalOps := strongOps + weakOps
	throughput := float64(actualTotalOps) / duration.Seconds()

	p.Println("\n=== Hybrid Benchmark Results ===")
	p.Printf("Total operations: %d\n", actualTotalOps)
	p.Printf("Duration: %.2fs\n", duration.Seconds())
	p.Printf("Throughput: %.2f ops/sec\n", throughput)

	if strongOps > 0 {
		strongPct := float64(strongOps) * 100 / float64(actualTotalOps)
		p.Printf("\nStrong Operations: %d (%.1f%%)\n", strongOps, strongPct)
		p.Printf("  Writes: %d | Reads: %d\n", m.StrongWriteCount, m.StrongReadCount)

		allStrongLatencies := append(m.StrongWriteLatency, m.StrongReadLatency...)
		if len(allStrongLatencies) > 0 {
			avg, median, p99, p999 := computePercentiles(allStrongLatencies)
			p.Printf("  Avg: %.2fms | Median: %.2fms | P99: %.2fms | P99.9: %.2fms\n", avg, median, p99, p999)
		}
		if len(m.StrongWriteLatency) > 0 {
			avg, median, p99, p999 := computePercentiles(m.StrongWriteLatency)
			p.Printf("  Strong Write: Avg: %.2fms | Median: %.2fms | P99: %.2fms | P99.9: %.2fms\n", avg, median, p99, p999)
		}
		if len(m.StrongReadLatency) > 0 {
			avg, median, p99, p999 := computePercentiles(m.StrongReadLatency)
			p.Printf("  Strong Read:  Avg: %.2fms | Median: %.2fms | P99: %.2fms | P99.9: %.2fms\n", avg, median, p99, p999)
		}
	}

	if weakOps > 0 {
		weakPct := float64(weakOps) * 100 / float64(actualTotalOps)
		p.Printf("\nWeak Operations: %d (%.1f%%)\n", weakOps, weakPct)
		p.Printf("  Writes: %d | Reads: %d\n", m.WeakWriteCount, m.WeakReadCount)

		allWeakLatencies := append(m.WeakWriteLatency, m.WeakReadLatency...)
		if len(allWeakLatencies) > 0 {
			avg, median, p99, p999 := computePercentiles(allWeakLatencies)
			p.Printf("  Avg: %.2fms | Median: %.2fms | P99: %.2fms | P99.9: %.2fms\n", avg, median, p99, p999)
		}
		if len(m.WeakWriteLatency) > 0 {
			avg, median, p99, p999 := computePercentiles(m.WeakWriteLatency)
			p.Printf("  Weak Write:  Avg: %.2fms | Median: %.2fms | P99: %.2fms | P99.9: %.2fms\n", avg, median, p99, p999)
		}
		if len(m.WeakReadLatency) > 0 {
			avg, median, p99, p999 := computePercentiles(m.WeakReadLatency)
			p.Printf("  Weak Read:   Avg: %.2fms | Median: %.2fms | P99: %.2fms | P99.9: %.2fms\n", avg, median, p99, p999)
		}
	}

	p.Println("================================")
}
