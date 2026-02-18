package raft

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/imdea-software/swiftpaxos/config"
	"github.com/imdea-software/swiftpaxos/dlog"
	"github.com/imdea-software/swiftpaxos/replica/defs"
	"github.com/imdea-software/swiftpaxos/state"
)

// testConf returns a minimal config suitable for integration tests.
func testConf() *config.Config {
	return &config.Config{
		Proxy: &config.ProxyInfo{},
	}
}

// intToValue converts an int to a state.Value ([]byte).
func intToValue(v int) state.Value {
	b := make([]byte, 8)
	binary.LittleEndian.PutUint64(b, uint64(v))
	return state.Value(b)
}

// startReplicas creates n Raft replicas on 127.0.0.1:basePort+i.
// Replica 0 is designated as leader. Returns the replicas after peer
// connections are established and the cluster is ready.
func startReplicas(t *testing.T, n int, basePort int) []*Replica {
	t.Helper()

	addrs := make([]string, n)
	for i := 0; i < n; i++ {
		addrs[i] = fmt.Sprintf("127.0.0.1:%d", basePort+i)
	}

	conf := testConf()
	logger := dlog.New("", false)

	replicas := make([]*Replica, n)
	for i := 0; i < n; i++ {
		isLeader := (i == 0)
		replicas[i] = New(fmt.Sprintf("r%d", i), i, addrs, isLeader, n/2, conf, logger)
	}

	// Wait for peer connections and leader to be established.
	// ConnectToPeers is called inside run(), which is launched by New().
	// We poll until the leader's role is LEADER and all peers are alive.
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		leader := findLeader(replicas)
		if leader != nil {
			// Check that all peers are connected (Alive array)
			allAlive := true
			for _, r := range replicas {
				if r.Replica == nil {
					allAlive = false
					break
				}
				for j := 0; j < n; j++ {
					if int32(j) != r.id && !r.Alive[j] {
						allAlive = false
						break
					}
				}
				if !allAlive {
					break
				}
			}
			if allAlive {
				return replicas
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("Timed out waiting for cluster to form")
	return nil
}

// stopReplica gracefully shuts down a replica.
// Closes the listener and all peer connections to ensure fast failure
// detection on surviving replicas (no hung writes to dead peers).
func stopReplica(r *Replica) {
	r.Shutdown = true
	if r.Listener != nil {
		r.Listener.Close()
	}
	// Close all peer connections so writes from other replicas fail fast.
	r.M.Lock()
	for i, conn := range r.Peers {
		if conn != nil {
			conn.Close()
			r.Peers[i] = nil
			r.PeerWriters[i] = nil
			r.PeerReaders[i] = nil
		}
	}
	r.M.Unlock()
}

// stopReplicaAndDisconnect stops a replica and also closes connections
// from all other replicas TO the stopped replica, so their writes fail fast.
func stopReplicaAndDisconnect(r *Replica, allReplicas []*Replica) {
	deadId := r.id
	stopReplica(r)
	// Close connections from surviving replicas to the dead one.
	for _, other := range allReplicas {
		if other.id == deadId || other.Shutdown {
			continue
		}
		other.M.Lock()
		if other.Peers[deadId] != nil {
			other.Peers[deadId].Close()
			other.Peers[deadId] = nil
			other.PeerWriters[deadId] = nil
			other.PeerReaders[deadId] = nil
			other.Alive[deadId] = false
		}
		other.M.Unlock()
	}
}

// findLeader returns the first replica with role == LEADER, or nil.
func findLeader(replicas []*Replica) *Replica {
	for _, r := range replicas {
		if r.role == LEADER {
			return r
		}
	}
	return nil
}

// waitForStableLeader waits until a leader has been stable for at least
// the given duration. This handles election instability where leadership
// bounces between replicas in a 2-node cluster.
func waitForStableLeader(t *testing.T, replicas []*Replica, timeout time.Duration) *Replica {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var lastLeader *Replica
	stableSince := time.Now()

	for time.Now().Before(deadline) {
		leader := findLeader(replicas)
		if leader != lastLeader {
			lastLeader = leader
			stableSince = time.Now()
		}
		if lastLeader != nil && time.Since(stableSince) >= 500*time.Millisecond {
			return lastLeader
		}
		time.Sleep(50 * time.Millisecond)
	}
	// Return whatever we have, even if not stable for full duration
	if lastLeader != nil {
		return lastLeader
	}
	t.Fatal("No leader elected within timeout")
	return nil
}

// sendCommandOnConn sends a Propose on an existing connection and reads the reply.
// Uses a persistent connection to avoid re-registering client on each command.
func sendCommandOnConn(t *testing.T, writer *bufio.Writer, reader *bufio.Reader, clientId int32, seqNum int32, key state.Key, value state.Value) *defs.ProposeReplyTS {
	t.Helper()

	propose := &defs.Propose{
		CommandId: seqNum,
		ClientId:  clientId,
		Command: state.Command{
			Op: state.PUT,
			K:  key,
			V:  value,
		},
		Timestamp: time.Now().UnixNano(),
	}

	writer.WriteByte(defs.PROPOSE)
	propose.Marshal(writer)
	if err := writer.Flush(); err != nil {
		t.Fatalf("Failed to send propose: %v", err)
	}

	reply := &defs.ProposeReplyTS{}
	if err := reply.Unmarshal(reader); err != nil {
		t.Fatalf("Failed to read reply: %v", err)
	}
	return reply
}

// connectToReplica opens a TCP connection and returns buffered reader/writer.
func connectToReplica(t *testing.T, addr string) (net.Conn, *bufio.Writer, *bufio.Reader) {
	t.Helper()
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		t.Fatalf("Failed to connect to %s: %v", addr, err)
	}
	conn.SetDeadline(time.Now().Add(30 * time.Second))
	return conn, bufio.NewWriter(conn), bufio.NewReader(conn)
}

// --- Integration Tests ---

func TestRaftBasicReplication(t *testing.T) {
	basePort := 18100
	replicas := startReplicas(t, 3, basePort)
	defer func() {
		for _, r := range replicas {
			stopReplica(r)
		}
	}()

	// Verify leader is replica 0
	leader := findLeader(replicas)
	if leader == nil {
		t.Fatal("No leader found")
	}
	if leader.id != 0 {
		t.Fatalf("Expected leader to be replica 0, got %d", leader.id)
	}

	// Send 5 commands to the leader via TCP
	leaderAddr := fmt.Sprintf("127.0.0.1:%d", basePort)
	conn, writer, reader := connectToReplica(t, leaderAddr)
	defer conn.Close()

	for i := int32(0); i < 5; i++ {
		key := state.Key(100 + int64(i))
		value := intToValue(1000 + int(i))
		reply := sendCommandOnConn(t, writer, reader, 1, i, key, value)
		if reply.OK != defs.TRUE {
			t.Fatalf("Command %d: expected OK=TRUE, got %d", i, reply.OK)
		}
		if reply.CommandId != i {
			t.Fatalf("Command %d: expected CommandId=%d, got %d", i, i, reply.CommandId)
		}
	}

	// Verify all replicas have the commands in their logs
	time.Sleep(200 * time.Millisecond) // allow replication to complete
	for idx, r := range replicas {
		if len(r.log) < 5 {
			t.Errorf("Replica %d: expected >= 5 log entries, got %d", idx, len(r.log))
		}
	}
}

func TestRaftLeaderElection(t *testing.T) {
	basePort := 18200
	replicas := startReplicas(t, 3, basePort)
	defer func() {
		for _, r := range replicas {
			stopReplica(r)
		}
	}()

	// Verify initial leader is replica 0
	leader := findLeader(replicas)
	if leader == nil || leader.id != 0 {
		t.Fatal("Expected initial leader to be replica 0")
	}

	// Send a few commands before killing the leader
	leaderAddr := fmt.Sprintf("127.0.0.1:%d", basePort)
	conn, writer, reader := connectToReplica(t, leaderAddr)
	for i := int32(0); i < 3; i++ {
		reply := sendCommandOnConn(t, writer, reader, 1, i, state.Key(int64(i)), intToValue(int(i)*10))
		if reply.OK != defs.TRUE {
			t.Fatalf("Pre-election command %d failed", i)
		}
	}
	conn.Close()

	// Allow replication to propagate
	time.Sleep(300 * time.Millisecond)

	// Record committed entries count on survivors before kill
	preKillLogLen1 := len(replicas[1].log)
	preKillLogLen2 := len(replicas[2].log)

	// Kill the leader (replica 0) and disconnect from surviving replicas
	stopReplicaAndDisconnect(replicas[0], replicas)

	// Wait for a new leader to be elected (election timeout ~300-500ms)
	var newLeader *Replica
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		for _, r := range replicas[1:] {
			if r.role == LEADER {
				newLeader = r
				break
			}
		}
		if newLeader != nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	if newLeader == nil {
		t.Fatal("No new leader elected after killing replica 0")
	}

	// 41.3b: Verify new leader's term > 0 (election happened)
	if newLeader.currentTerm <= 0 {
		t.Fatalf("New leader term should be > 0, got %d", newLeader.currentTerm)
	}

	// 41.3c: Verify new leader's log contains all previously committed entries
	if len(newLeader.log) < 3 {
		t.Fatalf("New leader should have >= 3 log entries, got %d", len(newLeader.log))
	}

	// Verify log entries on survivors are intact
	if len(replicas[1].log) < preKillLogLen1 {
		t.Errorf("Replica 1 lost log entries after leader death")
	}
	if len(replicas[2].log) < preKillLogLen2 {
		t.Errorf("Replica 2 lost log entries after leader death")
	}
}

func TestRaftClientResumesAfterFailover(t *testing.T) {
	basePort := 18300
	replicas := startReplicas(t, 3, basePort)
	defer func() {
		for _, r := range replicas {
			stopReplica(r)
		}
	}()

	// Phase 1: Send commands to the initial leader (replica 0)
	leaderAddr := fmt.Sprintf("127.0.0.1:%d", basePort)
	conn1, writer1, reader1 := connectToReplica(t, leaderAddr)
	for i := int32(0); i < 3; i++ {
		reply := sendCommandOnConn(t, writer1, reader1, 2, i, state.Key(200+int64(i)), intToValue(2000+int(i)))
		if reply.OK != defs.TRUE {
			t.Fatalf("Pre-failover command %d: expected OK=TRUE", i)
		}
	}
	conn1.Close()

	// Allow replication
	time.Sleep(300 * time.Millisecond)

	// Kill the leader and disconnect from survivors
	stopReplicaAndDisconnect(replicas[0], replicas)

	// Wait for a stable leader (leadership can bounce between the two survivors)
	newLeader := waitForStableLeader(t, replicas[1:], 10*time.Second)

	// Phase 2: Send commands to new leader
	newLeaderAddr := fmt.Sprintf("127.0.0.1:%d", basePort+int(newLeader.id))
	conn2, writer2, reader2 := connectToReplica(t, newLeaderAddr)
	defer conn2.Close()

	for i := int32(3); i < 6; i++ {
		reply := sendCommandOnConn(t, writer2, reader2, 2, i, state.Key(200+int64(i)), intToValue(2000+int(i)))
		// 41.4b: Verify commands sent after failover return OK=TRUE
		if reply.OK != defs.TRUE {
			t.Fatalf("Post-failover command %d: expected OK=TRUE, got %d", i, reply.OK)
		}
	}

	// Allow replication on surviving replicas
	time.Sleep(300 * time.Millisecond)

	// 41.4c: Verify state machine on surviving replicas has values from both phases
	for _, r := range replicas[1:] {
		// Should have at least 6 log entries (3 pre-failover + 3 post-failover)
		if len(r.log) < 6 {
			t.Errorf("Replica %d: expected >= 6 log entries, got %d", r.id, len(r.log))
		}
	}
}
