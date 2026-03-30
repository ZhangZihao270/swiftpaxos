package replica

import (
	"bufio"
	"context"
	"encoding/binary"
	"encoding/json"
	"io"
	"math"
	"net"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/imdea-software/swiftpaxos/config"
	"github.com/imdea-software/swiftpaxos/dlog"
	"github.com/imdea-software/swiftpaxos/replica/defs"
	fastrpc "github.com/imdea-software/swiftpaxos/rpc"
	"github.com/imdea-software/swiftpaxos/state"
)

type ClientSendArg = clientSendArg
type clientSendArg struct {
	code uint8
	msg  fastrpc.Serializable
}

type Replica struct {
	*dlog.Logger

	M     sync.Mutex
	N     int
	F     int
	Id    int32
	Alias string

	PeerAddrList       []string
	Peers              []net.Conn
	PeerReaders        []*bufio.Reader
	PeerWriters        []*bufio.Writer
	ClientWriters      map[int32]*bufio.Writer
	ClientMu           map[int32]*sync.Mutex
	ClientFastChan     map[int32]chan clientSendArg
	ClientAddrs        map[int32]string
	ClientDelay        map[int32]time.Duration
	Config             *config.Config
	Alive              []bool
	PreferredPeerOrder []int32

	State       *state.State
	RPC         *fastrpc.Table
	StableStore *os.File
	Stats       *defs.Stats
	Shutdown    bool
	Listener    net.Listener
	ProposeChan chan *defs.GPropose
	BeaconChan  chan *defs.GBeacon

	Thrifty bool
	Exec    bool
	LRead   bool
	Dreply  bool
	Beacon  bool
	Durable bool

	Ewma      []float64
	Latencies []int64

	Dt             *defs.LatencyTable
	clientDropOnce sync.Once

	// Message drop counter for SendClientMsgFast (Phase 77.1c)
	ClientMsgDrops int64 // atomic: total messages dropped due to full per-client channel
}

func New(alias string, id, f int, addrs []string, thrifty, exec, lread bool, config *config.Config, l *dlog.Logger) *Replica {
	n := len(addrs)
	r := &Replica{
		Logger: l,

		N:     n,
		F:     f,
		Id:    int32(id),
		Alias: alias,

		PeerAddrList:       addrs,
		Peers:              make([]net.Conn, n),
		PeerReaders:        make([]*bufio.Reader, n),
		PeerWriters:        make([]*bufio.Writer, n),
		ClientWriters:      make(map[int32]*bufio.Writer),
		ClientMu:           make(map[int32]*sync.Mutex),
		ClientFastChan:     make(map[int32]chan clientSendArg),
		ClientAddrs:        make(map[int32]string),
		ClientDelay:        make(map[int32]time.Duration),
		Config:             config,
		Alive:              make([]bool, n),
		PreferredPeerOrder: make([]int32, n),

		State:       state.InitState(),
		RPC:         fastrpc.NewTableId(defs.RPC_TABLE),
		StableStore: nil,
		Stats:       &defs.Stats{M: make(map[string]int)},
		Shutdown:    false,
		Listener:    nil,
		ProposeChan: make(chan *defs.GPropose, defs.CHAN_BUFFER_SIZE),
		BeaconChan:  make(chan *defs.GBeacon, defs.CHAN_BUFFER_SIZE),

		Thrifty: thrifty,
		Exec:    exec,
		LRead:   lread,
		Dreply:  true,
		Beacon:  false,
		Durable: false,

		Ewma:      make([]float64, n),
		Latencies: make([]int64, n),

		Dt: defs.NewLatencyTable(defs.LatencyConf, defs.IP(), id, addrs),
	}

	for i := 0; i < r.N; i++ {
		r.PreferredPeerOrder[i] = int32((int(r.Id) + 1 + i) % r.N)
		r.Ewma[i] = 0.0
		r.Latencies[i] = 0
	}

	return r
}

func (r *Replica) Ping(args *defs.PingArgs, reply *defs.PingReply) error {
	return nil
}

func (r *Replica) BeTheLeader(args *defs.BeTheLeaderArgs, reply *defs.BeTheLeaderReply) error {
	return nil
}

func (r *Replica) FastQuorumSize() int {
	return (3*r.N)/4 + 1
}

func (r *Replica) SlowQuorumSize() int {
	return (r.N + 1) / 2
}

func (r *Replica) WriteQuorumSize() int {
	return r.F + 1
}

func (r *Replica) ReadQuorumSize() int {
	return r.N - r.F
}

// setTCPKeepAlive enables TCP keepalive on a connection so the OS detects
// dead peers within ~6-10s (3 probes × 2s interval) and delivers EOF to the reader.
func setTCPKeepAlive(conn net.Conn) {
	if tc, ok := conn.(*net.TCPConn); ok {
		tc.SetKeepAlive(true)
		tc.SetKeepAlivePeriod(2 * time.Second)
	}
}

func (r *Replica) ConnectToPeers() {
	var b [4]byte
	bs := b[:4]
	done := make(chan bool)

	go r.waitForPeerConnections(done)

	for i := 0; i < int(r.Id); i++ {
		for {
			if conn, err := net.Dial("tcp", r.PeerAddrList[i]); err == nil {
				r.Peers[i] = conn
				setTCPKeepAlive(conn)
				break
			}
			time.Sleep(1e9)
		}
		binary.LittleEndian.PutUint32(bs, uint32(r.Id))
		if _, err := r.Peers[i].Write(bs); err != nil {
			r.Println("Write id error:", err)
			continue
		}
		r.Alive[i] = true
		r.PeerReaders[i] = bufio.NewReader(r.Peers[i])
		r.PeerWriters[i] = bufio.NewWriter(r.Peers[i])
		r.Printf("OUT Connected to %d", i)
	}
	<-done
	r.Printf("Replica %d: done connecting to peers", r.Id)
	r.Printf("Node list %v", r.PeerAddrList)

	for rid, reader := range r.PeerReaders {
		if int32(rid) == r.Id {
			continue
		}
		go r.replicaListener(rid, reader)
	}
}

func (r *Replica) ConnectToPeersNoListeners() {
	var b [4]byte
	bs := b[:4]
	done := make(chan bool)

	go r.waitForPeerConnections(done)

	for i := 0; i < int(r.Id); i++ {
		for {
			if conn, err := net.Dial("tcp", r.PeerAddrList[i]); err == nil {
				r.Peers[i] = conn
				setTCPKeepAlive(conn)
				break
			}
			time.Sleep(1e9)
		}
		binary.LittleEndian.PutUint32(bs, uint32(r.Id))
		if _, err := r.Peers[i].Write(bs); err != nil {
			r.Println("Write id error:", err)
			continue
		}
		r.Alive[i] = true
		r.PeerReaders[i] = bufio.NewReader(r.Peers[i])
		r.PeerWriters[i] = bufio.NewWriter(r.Peers[i])
	}
	<-done
	r.Printf("Replica id: %d. Done connecting to peers\n", r.Id)
}

func (r *Replica) WaitForClientConnections() {
	r.Println("Waiting for client connections")

	for !r.Shutdown {
		conn, err := r.Listener.Accept()
		if err != nil {
			r.Println("Accept error:", err)
			continue
		}
		go r.clientListener(conn)
	}
}

// peerWriteDeadline is the maximum time to wait for a peer write/flush to complete.
// If a peer is dead, the kernel TCP stack would block for ~2 minutes; this deadline
// ensures we detect the failure within 1 second and mark the peer as dead.
const peerWriteDeadline = 1 * time.Second

func (r *Replica) SendMsg(peerId int32, code uint8, msg fastrpc.Serializable) {
	r.M.Lock()
	defer r.M.Unlock()

	w := r.PeerWriters[peerId]
	if w == nil {
		r.Printf("Connection to %d lost!", peerId)
		return
	}

	// Set write deadline to avoid blocking for ~2 min on dead peer's TCP connection
	if conn := r.Peers[peerId]; conn != nil {
		conn.SetWriteDeadline(time.Now().Add(peerWriteDeadline))
	}

	w.WriteByte(code)
	msg.Marshal(w)
	if err := w.Flush(); err != nil {
		r.Printf("Peer %d write error: %v — marking dead", peerId, err)
		r.PeerWriters[peerId] = nil
		r.Alive[peerId] = false
		if conn := r.Peers[peerId]; conn != nil {
			conn.Close()
		}
		return
	}

	// Clear write deadline on success
	if conn := r.Peers[peerId]; conn != nil {
		conn.SetWriteDeadline(time.Time{})
	}
}

func (r *Replica) SendClientMsg(id int32, code uint8, msg fastrpc.Serializable) {
	r.M.Lock()
	w := r.ClientWriters[id]
	mu := r.ClientMu[id]
	d := r.ClientDelay[id]
	r.M.Unlock()

	if w == nil || mu == nil {
		r.Printf("Connection to client %d lost!", id)
		return
	}
	// Inject latency for replica→client direction (non-co-located clients).
	// clientDelay is pre-computed at registration: 0 for proxy-local clients.
	// Use a goroutine so we don't block the Sender goroutine.
	if d > 0 {
		go func() {
			time.Sleep(d)
			mu.Lock()
			defer mu.Unlock()
			w.WriteByte(code)
			msg.Marshal(w)
			w.Flush()
		}()
		return
	}
	mu.Lock()
	defer mu.Unlock()
	w.WriteByte(code)
	msg.Marshal(w)
	w.Flush()
}

// SendClientMsgFast sends a message to a client via a dedicated per-client
// goroutine, bypassing the Sender queue. This avoids head-of-line blocking
// where slow remote flushes delay fast local replies.
// Non-blocking: drops the message if the per-client channel is full, to prevent
// the caller (run loop or Sender goroutine) from blocking on a slow/disconnected client.
func (r *Replica) SendClientMsgFast(id int32, code uint8, msg fastrpc.Serializable) {
	r.M.Lock()
	ch := r.ClientFastChan[id]
	r.M.Unlock()
	if ch == nil {
		return
	}
	select {
	case ch <- clientSendArg{code: code, msg: msg}:
	default:
		// Channel full — drop message and count it.
		drops := atomic.AddInt64(&r.ClientMsgDrops, 1)
		r.clientDropOnce.Do(func() {
			r.Printf("WARNING: per-client channel full for client %d, dropping messages (buffer=%d)", id, cap(ch))
		})
		// Log every 1000th drop to track ongoing issues without flooding logs
		if drops%1000 == 0 {
			r.Printf("[MSGDROP] total=%d (client %d, buffer=%d)", drops, id, cap(ch))
		}
	}
}

func (r *Replica) SendMsgNoFlush(peerId int32, code uint8, msg fastrpc.Serializable) {
	r.M.Lock()
	defer r.M.Unlock()

	w := r.PeerWriters[peerId]
	if w == nil {
		r.Printf("Connection to %d lost!", peerId)
		return
	}
	w.WriteByte(code)
	msg.Marshal(w)
}

// FlushPeers flushes buffered writes to all connected peer writers.
// Uses write deadlines to avoid blocking on dead peers.
func (r *Replica) FlushPeers() {
	r.M.Lock()
	defer r.M.Unlock()

	for i, w := range r.PeerWriters {
		if w == nil {
			continue
		}
		if conn := r.Peers[i]; conn != nil {
			conn.SetWriteDeadline(time.Now().Add(peerWriteDeadline))
		}
		if err := w.Flush(); err != nil {
			r.Printf("Peer %d flush error: %v — marking dead", i, err)
			r.PeerWriters[i] = nil
			r.Alive[i] = false
			if conn := r.Peers[i]; conn != nil {
				conn.Close()
			}
			continue
		}
		if conn := r.Peers[i]; conn != nil {
			conn.SetWriteDeadline(time.Time{})
		}
	}
}

func (r *Replica) ReplyProposeTS(reply *defs.ProposeReplyTS, w *bufio.Writer, lock *sync.Mutex) {
	lock.Lock()
	defer lock.Unlock()

	reply.Marshal(w)
	if err := w.Flush(); err != nil {
		r.Printf("ReplyProposeTS flush error for cmd %d: %v", reply.CommandId, err)
	}
}

// ReplyProposeTSDelayed is like ReplyProposeTS but injects replica→client
// delay for WAN simulation parity. Used by Raft so its reply path matches
// the delay injection that other protocols (Raft-HT, CURP, etc.) get via
// SendClientMsg.
func (r *Replica) ReplyProposeTSDelayed(reply *defs.ProposeReplyTS, w *bufio.Writer, lock *sync.Mutex, clientId int32) {
	r.M.Lock()
	d := r.ClientDelay[clientId]
	r.M.Unlock()

	if d > 0 {
		go func() {
			time.Sleep(d)
			lock.Lock()
			defer lock.Unlock()
			reply.Marshal(w)
			w.Flush()
		}()
		return
	}
	lock.Lock()
	defer lock.Unlock()
	reply.Marshal(w)
	w.Flush()
}

func (r *Replica) SendBeacon(peerId int32) {
	r.M.Lock()
	defer r.M.Unlock()

	w := r.PeerWriters[peerId]
	if w == nil {
		r.Printf("Connection to %d lost!", peerId)
		return
	}

	if conn := r.Peers[peerId]; conn != nil {
		conn.SetWriteDeadline(time.Now().Add(peerWriteDeadline))
	}

	w.WriteByte(defs.GENERIC_SMR_BEACON)
	beacon := &defs.Beacon{
		Timestamp: time.Now().UnixNano(),
	}
	beacon.Marshal(w)
	if err := w.Flush(); err != nil {
		r.Printf("Peer %d beacon write error: %v — marking dead", peerId, err)
		r.PeerWriters[peerId] = nil
		r.Alive[peerId] = false
		if conn := r.Peers[peerId]; conn != nil {
			conn.Close()
		}
		return
	}

	if conn := r.Peers[peerId]; conn != nil {
		conn.SetWriteDeadline(time.Time{})
	}
}

func (r *Replica) ReplyBeacon(beacon *defs.GBeacon) {
	r.M.Lock()
	defer r.M.Unlock()

	w := r.PeerWriters[beacon.Rid]
	if w == nil {
		r.Printf("Connection to %d lost!", beacon.Rid)
		return
	}

	if conn := r.Peers[beacon.Rid]; conn != nil {
		conn.SetWriteDeadline(time.Now().Add(peerWriteDeadline))
	}

	w.WriteByte(defs.GENERIC_SMR_BEACON_REPLY)
	rb := &defs.BeaconReply{
		Timestamp: beacon.Timestamp,
	}
	rb.Marshal(w)
	if err := w.Flush(); err != nil {
		r.Printf("Peer %d beacon reply write error: %v — marking dead", beacon.Rid, err)
		r.PeerWriters[beacon.Rid] = nil
		r.Alive[beacon.Rid] = false
		if conn := r.Peers[beacon.Rid]; conn != nil {
			conn.Close()
		}
		return
	}

	if conn := r.Peers[beacon.Rid]; conn != nil {
		conn.SetWriteDeadline(time.Time{})
	}
}

func (r *Replica) UpdatePreferredPeerOrder(quorum []int32) {
	aux := make([]int32, r.N)
	i := 0
	for _, p := range quorum {
		if p == r.Id {
			continue
		}
		aux[i] = p
		i++
	}

	for _, p := range r.PreferredPeerOrder {
		found := false
		for j := 0; j < i; j++ {
			if aux[j] == p {
				found = true
				break
			}
		}
		if !found {
			aux[i] = p
			i++
		}
	}

	r.M.Lock()
	r.PreferredPeerOrder = aux
	r.M.Unlock()
}

func (r *Replica) ComputeClosestPeers() []float64 {
	npings := 20

	for j := 0; j < npings; j++ {
		for i := int32(0); i < int32(r.N); i++ {
			if i == r.Id {
				continue
			}
			r.M.Lock()
			if r.Alive[i] {
				r.M.Unlock()
				r.SendBeacon(i)
			} else {
				r.Latencies[i] = math.MaxInt64
				r.M.Unlock()
			}
		}
		time.Sleep(500 * time.Millisecond)
	}

	quorum := make([]int32, r.N)

	r.M.Lock()
	for i := int32(0); i < int32(r.N); i++ {
		pos := 0
		for j := int32(0); j < int32(r.N); j++ {
			if (r.Latencies[j] < r.Latencies[i]) ||
				((r.Latencies[j] == r.Latencies[i]) && (j < i)) {
				pos++
			}
		}
		quorum[pos] = int32(i)
	}
	r.M.Unlock()

	if r.Dt == nil {
		r.UpdatePreferredPeerOrder(quorum)
	} else {
		sort.Slice(r.PreferredPeerOrder, func(i, j int) bool {
			di := r.Dt.WaitDurationID(int(r.PreferredPeerOrder[i]))
			dj := r.Dt.WaitDurationID(int(r.PreferredPeerOrder[j]))
			return dj == time.Duration(0) || di < dj
		})
	}

	latencies := make([]float64, r.N-1)

	for i := 0; i < r.N-1; i++ {
		node := r.PreferredPeerOrder[i]
		lat := float64(r.Latencies[node]) / float64(npings*1000000)
		r.Println(node, "->", lat, "ms")
		latencies[i] = lat
	}

	return latencies
}

func (r *Replica) waitForPeerConnections(done chan bool) {
	var b [4]byte
	bs := b[:4]

	// Listen on all interfaces (0.0.0.0) with the replica's port.
	// This is required for AWS where instances can only bind to private IPs
	// but peers connect via public IPs.
	// Use SO_REUSEADDR to avoid TIME_WAIT conflicts between consecutive benchmark runs.
	_, port, _ := net.SplitHostPort(r.PeerAddrList[r.Id])
	addr := "0.0.0.0:" + port
	lc := net.ListenConfig{
		Control: func(network, address string, c syscall.RawConn) error {
			return c.Control(func(fd uintptr) {
				syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1)
			})
		},
	}
	l, err := lc.Listen(context.Background(), "tcp", addr)
	if err != nil {
		r.Fatal(r.PeerAddrList[r.Id], err)
	}
	r.Listener = l
	expected := int32(r.N) - r.Id - 1
	var connected int32
	for connected < expected {
		conn, err := r.Listener.Accept()
		if err != nil {
			r.Println("Accept error:", err)
			continue
		}
		if _, err := io.ReadFull(conn, bs); err != nil {
			r.Println("Connection establish error:", err)
			conn.Close()
			continue
		}
		id := int32(binary.LittleEndian.Uint32(bs))
		if id < 0 || id >= int32(r.N) || id == r.Id {
			r.Printf("IN Rejecting invalid peer id %d", id)
			conn.Close()
			continue
		}
		if r.Peers[id] != nil {
			r.Printf("IN Duplicate connection from %d, replacing", id)
			r.Peers[id].Close()
		} else {
			connected++
		}
		setTCPKeepAlive(conn)
		r.Peers[id] = conn
		r.PeerReaders[id] = bufio.NewReader(conn)
		r.PeerWriters[id] = bufio.NewWriter(conn)
		r.Alive[id] = true
		r.Printf("IN Connected to %d", id)
	}

	done <- true
}

func (r *Replica) replicaListener(rid int, reader *bufio.Reader) {
	var (
		msgType      uint8
		err          error = nil
		gbeacon      defs.Beacon
		gbeaconReply defs.BeaconReply
	)

	for err == nil && !r.Shutdown {
		if msgType, err = reader.ReadByte(); err != nil {
			break
		}

		switch uint8(msgType) {

		case defs.GENERIC_SMR_BEACON:
			if err = gbeacon.Unmarshal(reader); err != nil {
				break
			}
			r.ReplyBeacon(&defs.GBeacon{
				Rid:       int32(rid),
				Timestamp: gbeacon.Timestamp,
			})
			break

		case defs.GENERIC_SMR_BEACON_REPLY:
			if err = gbeaconReply.Unmarshal(reader); err != nil {
				break
			}
			r.M.Lock()
			r.Latencies[rid] += time.Now().UnixNano() - gbeaconReply.Timestamp
			r.M.Unlock()
			now := time.Now().UnixNano()
			r.Ewma[rid] = 0.99*r.Ewma[rid] + 0.01*float64(now-gbeaconReply.Timestamp)
			break

		default:
			p, exists := r.RPC.Get(msgType)
			if exists {
				obj := p.Obj.New()
				if err = obj.Unmarshal(reader); err != nil {
					break
				}
				go func(obj fastrpc.Serializable) {
					time.Sleep(r.Dt.WaitDurationID(rid))
					p.Chan <- obj
				}(obj)
			} else {
				r.Println("Warning: received unknown message type", msgType, "from peer", rid, "- closing connection")
				err = io.ErrUnexpectedEOF
				break
			}
		}
	}

	r.Printf("Peer %d reader exited (err=%v), closing connection", rid, err)

	// Close the underlying TCP connection first (outside the lock).
	// This forces any in-progress w.Flush() holding r.M to return
	// immediately with an error, instead of blocking for ~2 min
	// on the kernel TCP timeout. Without this, broadcastAppendEntries
	// holds r.M while Flush() blocks, preventing RequestVote RPCs
	// and stalling leader election.
	if r.Peers[rid] != nil {
		r.Peers[rid].Close()
	}

	r.M.Lock()
	r.Alive[rid] = false
	r.PeerWriters[rid] = nil
	r.M.Unlock()
	r.Printf("Peer %d marked dead", rid)
}

func (r *Replica) clientListener(conn net.Conn) {
	reader := bufio.NewReader(conn)
	writer := bufio.NewWriter(conn)

	var (
		msgType byte
		err     error
	)

	r.M.Lock()
	r.Println("Client up", conn.RemoteAddr(), "(", r.LRead, ")")
	r.M.Unlock()

	addr := strings.Split(conn.RemoteAddr().String(), ":")[0]
	isProxy := r.Config.Proxy.IsProxy(r.Alias, addr)
	isLocal := r.Config.Proxy.IsLocal(r.Alias, addr)

	mutex := &sync.Mutex{}

	// Delay for client messages (simulates geo latency for remote clients).
	// Skip for proxy-local clients: they are co-located with this replica.
	// Uses simple goroutine+sleep instead of DelayProposeChan, which requires
	// consecutive CommandIds and breaks with hybrid weak/strong protocols.
	clientDelay := time.Duration(0)
	if !isLocal {
		clientDelay = r.Dt.WaitDuration(addr)
	}

	for !r.Shutdown && err == nil {
		if msgType, err = reader.ReadByte(); err != nil {
			break
		}

		switch uint8(msgType) {
		case defs.PROPOSE:
			propose := &defs.Propose{}
			if err = propose.Unmarshal(reader); err != nil {
				break
			}
			r.registerClient(propose.ClientId, writer, addr, mutex, clientDelay)
			op := propose.Command.Op
			if r.LRead && (op == state.GET || op == state.SCAN) {
				r.ReplyProposeTSDelayed(&defs.ProposeReplyTS{
					OK:        defs.TRUE,
					CommandId: propose.CommandId,
					Value:     propose.Command.Execute(r.State),
					Timestamp: propose.Timestamp,
				}, writer, mutex, propose.ClientId)
			} else {
				gp := &defs.GPropose{
					Propose: propose,
					Reply:   writer,
					Mutex:   mutex,
					Proxy:   isProxy,
					Addr:    addr,
				}
				if clientDelay > 0 {
					go func(p *defs.GPropose) {
						time.Sleep(clientDelay)
						r.ProposeChan <- p
					}(gp)
				} else {
					r.ProposeChan <- gp
				}
			}
			break

		case defs.READ:
			// TODO: do something with this
			read := &defs.Read{}
			if err = read.Unmarshal(reader); err != nil {
				break
			}
			break

		case defs.PROPOSE_AND_READ:
			// TODO: do something with this
			pr := &defs.ProposeAndRead{}
			if err = pr.Unmarshal(reader); err != nil {
				break
			}
			break

		case defs.STATS:
			r.M.Lock()
			b, _ := json.Marshal(r.Stats)
			r.M.Unlock()
			writer.Write(b)
			writer.Flush()

		default:
			p, exists := r.RPC.Get(msgType)
			if exists {
				obj := p.Obj.New()
				if err = obj.Unmarshal(reader); err != nil {
					break
				}
				// Register client infrastructure if the message carries a ClientId.
				// This ensures ClientWriters/ClientFastChan are initialized even when
				// the first message from a client is not a PROPOSE (e.g., MCausalPropose).
				if cm, ok := obj.(interface{ GetClientId() int32 }); ok {
					r.registerClient(cm.GetClientId(), writer, addr, mutex, clientDelay)
				}
				go func(obj fastrpc.Serializable) {
					time.Sleep(clientDelay)
					p.Chan <- obj
				}(obj)
			} else {
				r.Println("Warning: received unknown client message", msgType, "from", conn.RemoteAddr(), "- closing connection")
				err = io.ErrUnexpectedEOF
				break
			}
		}
	}

	conn.Close()
	r.Println("Client down", conn.RemoteAddr())
}

// registerClient initializes per-client send infrastructure (writer, mutex,
// fast channel) if not already set up. Called on every client message to ensure
// the infrastructure exists regardless of which message type arrives first.
// clientDelay is the pre-computed latency for this client (0 for co-located).
func (r *Replica) registerClient(clientId int32, writer *bufio.Writer, addr string, mutex *sync.Mutex, clientDelay time.Duration) {
	r.M.Lock()
	r.ClientWriters[clientId] = writer
	r.ClientAddrs[clientId] = addr
	r.ClientDelay[clientId] = clientDelay
	if r.ClientMu[clientId] == nil {
		r.ClientMu[clientId] = &sync.Mutex{}
	}
	if r.ClientFastChan[clientId] == nil {
		ch := make(chan clientSendArg, 131072)
		r.ClientFastChan[clientId] = ch
		cid := clientId
		go func() {
			for arg := range ch {
				r.SendClientMsg(cid, arg.code, arg.msg)
			}
		}()
	}
	r.M.Unlock()
}

func Leader(ballot int32, repNum int) int32 {
	return ballot % int32(repNum)
}

func NextBallotOf(rid, oldBallot int32, repNum int) int32 {
	return (oldBallot/int32(repNum)+1)*int32(repNum) + rid
}
