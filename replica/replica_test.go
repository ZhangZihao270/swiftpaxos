package replica

import (
	"bufio"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/imdea-software/swiftpaxos/dlog"
	"github.com/imdea-software/swiftpaxos/replica/defs"
	fastrpc "github.com/imdea-software/swiftpaxos/rpc"
)

// mockMsg implements fastrpc.Serializable for testing.
type mockMsg struct{}

func (m *mockMsg) Marshal(w io.Writer)          {}
func (m *mockMsg) Unmarshal(r io.Reader) error   { return nil }
func (m *mockMsg) New() fastrpc.Serializable     { return &mockMsg{} }

// newTestReplica creates a minimal Replica for testing without network setup.
func newTestReplica(n int, id int32) *Replica {
	l := dlog.New("", false)
	return &Replica{
		Logger:        l,
		N:             n,
		F:             (n - 1) / 2,
		Id:            id,
		Peers:         make([]net.Conn, n),
		PeerReaders:   make([]*bufio.Reader, n),
		PeerWriters:   make([]*bufio.Writer, n),
		Alive:         make([]bool, n),
		Ewma:          make([]float64, n),
		Latencies:     make([]int64, n),
		Dt:            defs.NewLatencyTable("", "", int(id), nil),
		RPC:           fastrpc.NewTable(),
		ClientWriters: make(map[int32]*bufio.Writer),
		ClientMu:      make(map[int32]*sync.Mutex),
	}
}

// TestReplicaListenerEOF_NilsWriter verifies that when a peer connection
// is closed (EOF), replicaListener sets Alive[rid]=false and PeerWriters[rid]=nil.
func TestReplicaListenerEOF_NilsWriter(t *testing.T) {
	r := newTestReplica(3, 0)

	// Create a pipe to simulate a peer connection.
	serverConn, clientConn := net.Pipe()
	rid := 1
	r.Peers[rid] = serverConn
	r.PeerReaders[rid] = bufio.NewReader(serverConn)
	r.PeerWriters[rid] = bufio.NewWriter(serverConn)
	r.Alive[rid] = true

	done := make(chan struct{})
	go func() {
		r.replicaListener(rid, r.PeerReaders[rid])
		close(done)
	}()

	// Close the client side to cause EOF on the reader.
	clientConn.Close()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("replicaListener did not exit within 2 seconds after EOF")
	}

	r.M.Lock()
	defer r.M.Unlock()

	if r.Alive[rid] {
		t.Error("expected Alive[rid] to be false after EOF")
	}
	if r.PeerWriters[rid] != nil {
		t.Error("expected PeerWriters[rid] to be nil after EOF")
	}
}

// TestReplicaListenerEOF_ClosesConn verifies that replicaListener closes
// the underlying TCP connection, which would unblock any in-progress Flush().
func TestReplicaListenerEOF_ClosesConn(t *testing.T) {
	r := newTestReplica(3, 0)

	serverConn, clientConn := net.Pipe()
	rid := 2
	r.Peers[rid] = serverConn
	r.PeerReaders[rid] = bufio.NewReader(serverConn)
	r.PeerWriters[rid] = bufio.NewWriter(serverConn)
	r.Alive[rid] = true

	done := make(chan struct{})
	go func() {
		r.replicaListener(rid, r.PeerReaders[rid])
		close(done)
	}()

	clientConn.Close()
	<-done

	// Verify the connection is closed: writing to it should fail.
	_, err := serverConn.Write([]byte("test"))
	if err == nil {
		t.Error("expected error writing to closed connection, got nil")
	}
}

// TestSendMsg_NilWriter verifies SendMsg returns early when PeerWriters[id] is nil.
func TestSendMsg_NilWriter(t *testing.T) {
	r := newTestReplica(3, 0)

	// PeerWriters[1] is nil by default (not connected).
	// SendMsg should not panic.
	r.SendMsg(1, 0, &mockMsg{})
}

// TestSendMsg_AfterPeerDeath verifies that SendMsg skips a dead peer
// whose writer has been nil'd.
func TestSendMsg_AfterPeerDeath(t *testing.T) {
	r := newTestReplica(3, 0)

	serverConn, clientConn := net.Pipe()
	rid := int32(1)
	r.Peers[rid] = serverConn
	r.PeerReaders[rid] = bufio.NewReader(serverConn)
	r.PeerWriters[rid] = bufio.NewWriter(serverConn)
	r.Alive[rid] = true

	// Simulate peer death.
	clientConn.Close()
	done := make(chan struct{})
	go func() {
		r.replicaListener(int(rid), r.PeerReaders[rid])
		close(done)
	}()
	<-done

	// SendMsg should return early without panicking.
	r.SendMsg(rid, 0, &mockMsg{})

	r.M.Lock()
	defer r.M.Unlock()
	if r.PeerWriters[rid] != nil {
		t.Error("expected PeerWriters to be nil after peer death")
	}
}

// TestFlushPeers_SkipsNilWriters verifies FlushPeers doesn't panic on nil writers.
func TestFlushPeers_SkipsNilWriters(t *testing.T) {
	r := newTestReplica(3, 0)

	// All writers are nil — should not panic.
	r.FlushPeers()
}

// TestSendMsg_WriteDeadline verifies that SendMsg completes quickly when a peer
// connection is stalled (simulated by closing the read side), instead of blocking
// for ~2 minutes on the kernel TCP timeout.
func TestSendMsg_WriteDeadline(t *testing.T) {
	r := newTestReplica(3, 0)

	// Create a pipe. We'll close the read side but keep the write side open.
	// On a real TCP connection, this would cause Flush to block; with a pipe,
	// closing one side causes immediate error — so we test the error handling path.
	serverConn, clientConn := net.Pipe()
	rid := int32(1)
	r.Peers[rid] = serverConn
	r.PeerWriters[rid] = bufio.NewWriter(serverConn)
	r.Alive[rid] = true

	// Close the read side to cause writes to fail.
	clientConn.Close()

	// SendMsg should complete quickly (not block for 2 min)
	done := make(chan struct{})
	go func() {
		r.SendMsg(rid, 0, &mockMsg{})
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("SendMsg blocked for more than 3 seconds on dead peer")
	}

	// Verify the peer is marked dead
	r.M.Lock()
	defer r.M.Unlock()
	if r.PeerWriters[rid] != nil {
		t.Error("expected PeerWriters[rid] to be nil after write error")
	}
	if r.Alive[rid] {
		t.Error("expected Alive[rid] to be false after write error")
	}
}

// TestSendMsg_WriteDeadlineSuccess verifies that SendMsg works normally
// for alive peers (deadline is set and cleared without error).
func TestSendMsg_WriteDeadlineSuccess(t *testing.T) {
	r := newTestReplica(3, 0)

	serverConn, clientConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	rid := int32(1)
	r.Peers[rid] = serverConn
	r.PeerWriters[rid] = bufio.NewWriter(serverConn)
	r.Alive[rid] = true

	// Read from the other side in background to prevent blocking
	go func() {
		buf := make([]byte, 4096)
		for {
			_, err := clientConn.Read(buf)
			if err != nil {
				return
			}
		}
	}()

	// SendMsg should succeed
	r.SendMsg(rid, 0, &mockMsg{})

	r.M.Lock()
	defer r.M.Unlock()
	if r.PeerWriters[rid] == nil {
		t.Error("expected PeerWriters[rid] to remain non-nil after successful write")
	}
	if !r.Alive[rid] {
		t.Error("expected Alive[rid] to remain true after successful write")
	}
}

// TestFlushPeers_WriteDeadline verifies that FlushPeers marks dead peers
// when Flush fails due to write deadline.
func TestFlushPeers_WriteDeadline(t *testing.T) {
	r := newTestReplica(3, 0)

	// Set up peer 1 with a closed connection (will fail on flush)
	serverConn1, clientConn1 := net.Pipe()
	r.Peers[1] = serverConn1
	r.PeerWriters[1] = bufio.NewWriter(serverConn1)
	r.Alive[1] = true
	clientConn1.Close() // Close read side to cause flush error

	// Set up peer 2 as alive (will succeed)
	serverConn2, clientConn2 := net.Pipe()
	defer clientConn2.Close()
	defer serverConn2.Close()
	r.Peers[2] = serverConn2
	r.PeerWriters[2] = bufio.NewWriter(serverConn2)
	r.Alive[2] = true

	// Write something to peer 1's buffer so Flush has data to send
	r.PeerWriters[1].WriteByte(42)

	// Read from peer 2 in background
	go func() {
		buf := make([]byte, 4096)
		for {
			_, err := clientConn2.Read(buf)
			if err != nil {
				return
			}
		}
	}()

	done := make(chan struct{})
	go func() {
		r.FlushPeers()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("FlushPeers blocked for more than 3 seconds")
	}

	r.M.Lock()
	defer r.M.Unlock()

	// Peer 1 should be marked dead
	if r.PeerWriters[1] != nil {
		t.Error("expected PeerWriters[1] to be nil after flush error")
	}
	if r.Alive[1] {
		t.Error("expected Alive[1] to be false after flush error")
	}

	// Peer 2 should remain alive
	if r.PeerWriters[2] == nil {
		t.Error("expected PeerWriters[2] to remain non-nil")
	}
	if !r.Alive[2] {
		t.Error("expected Alive[2] to remain true")
	}
}

// TestPeerWriteDeadlineConstant verifies the deadline constant is reasonable.
func TestPeerWriteDeadlineConstant(t *testing.T) {
	if peerWriteDeadline < 100*time.Millisecond {
		t.Errorf("peerWriteDeadline too small: %v", peerWriteDeadline)
	}
	if peerWriteDeadline > 10*time.Second {
		t.Errorf("peerWriteDeadline too large: %v", peerWriteDeadline)
	}
}
