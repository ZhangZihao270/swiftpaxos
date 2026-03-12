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
