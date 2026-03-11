package epaxosho

import (
	"encoding/binary"
	"sync"
	"time"

	"github.com/imdea-software/swiftpaxos/config"
	"github.com/imdea-software/swiftpaxos/dlog"
	"github.com/imdea-software/swiftpaxos/replica"
	"github.com/imdea-software/swiftpaxos/replica/defs"
	fastrpc "github.com/imdea-software/swiftpaxos/rpc"
	"github.com/imdea-software/swiftpaxos/state"
)

const MAX_INSTANCE = 5 * 1024 * 1024

const MAX_DEPTH_DEP = 1000
const TRUE = uint8(1)
const FALSE = uint8(0)

const COMMIT_GRACE_PERIOD = 10 * 1e9 // 10 seconds
const ADAPT_TIME_SEC = 10

const HT_INIT_SIZE = 11000

// NO_CAUSAL_CHANNEL is the number of causal commit channels per replica.
// Multiple channels avoid serialization bottlenecks for causal commits.
const NO_CAUSAL_CHANNEL = 10

var cpMarker []state.Command
var cpcounter = 0

type Replica struct {
	*replica.Replica

	// Message channels
	prepareChan           chan fastrpc.Serializable
	preAcceptChan         chan fastrpc.Serializable
	acceptChan            chan fastrpc.Serializable
	commitChan            chan fastrpc.Serializable
	commitShortChan       chan fastrpc.Serializable
	prepareReplyChan      chan fastrpc.Serializable
	preAcceptReplyChan    chan fastrpc.Serializable
	preAcceptOKChan       chan fastrpc.Serializable
	acceptReplyChan       chan fastrpc.Serializable
	tryPreAcceptChan      chan fastrpc.Serializable
	tryPreAcceptReplyChan chan fastrpc.Serializable
	// Per-replica causal commit channels (N * NO_CAUSAL_CHANNEL channels)
	causalCommitChan []chan fastrpc.Serializable

	// RPC type identifiers
	prepareRPC            uint8
	prepareReplyRPC       uint8
	preAcceptRPC          uint8
	preAcceptReplyRPC     uint8
	preAcceptOKRPC        uint8
	acceptRPC             uint8
	acceptReplyRPC        uint8
	commitRPC             uint8
	commitShortRPC        uint8
	tryPreAcceptRPC       uint8
	tryPreAcceptReplyRPC  uint8
	causalCommitRPC       []uint8

	// Instance management
	InstanceSpace [][]*Instance // the space of all instances (used and not yet used)
	crtInstance   []int32       // highest active instance numbers that this replica knows about
	CommittedUpTo []int32       // highest committed instance per replica that this replica knows about
	ExecedUpTo    []int32       // instance up to which all commands have been executed (including itself)
	exec          *Exec

	// Conflict tracking
	conflicts      []map[state.Key]int32
	conflictMutex  *sync.RWMutex
	maxSeqPerKey   map[state.Key]int32
	maxSeqPerKeyMu *sync.RWMutex
	maxSeq         int32

	// Session-based causal ordering: tracks latest committed instance per session per replica
	sessionConflicts   []map[int32]int32
	sessionConflictsMu *sync.RWMutex

	// Write tracking for last-write-wins execution semantics
	maxWriteInstancePerKey   map[state.Key]*instanceId
	maxWriteInstancePerKeyMu *sync.RWMutex
	maxWriteSeqPerKey        map[state.Key]int32
	maxWriteSeqPerKeyMu      *sync.RWMutex

	// Checkpointing
	latestCPReplica  int32
	latestCPInstance int32

	// Client reply synchronization
	clientMutex *sync.Mutex

	// Recovery
	instancesToRecover chan *instanceId
}

type instanceId struct {
	replica  int32
	instance int32
}

type Instance struct {
	Cmds       []state.Command
	bal, vbal  int32
	Status     int8 // NONE, PREACCEPTED, PREACCEPTED_EQ, CAUSAL_ACCEPTED, ACCEPTED, CAUSALLY_COMMITTED, STRONGLY_COMMITTED, EXECUTED, DISCARDED
	State      int8 // NONE, READY, WAITING, DONE
	Seq        int32
	Deps       []int32
	CL         []int32 // consistency levels per command
	lb         *LeaderBookkeeping
	Index      int // Tarjan SCC algorithm fields
	Lowlink    int
	instanceId *instanceId
}

type RecoveryInstance struct {
	cmds            []state.Command
	status          int8
	seq             int32
	deps            []int32
	cl              []int32
	preAcceptCount  int
	leaderResponded bool
}

type LeaderBookkeeping struct {
	clientProposals   []*defs.GPropose
	maxRecvBallot     int32
	prepareOKs        int
	allEqual          bool
	preAcceptOKs      int
	acceptOKs         int
	nacks             int
	originalDeps      []int32
	committedDeps     []int32
	recoveryInst      *RecoveryInstance
	preparing         bool
	tryingToPreAccept bool
	possibleQuorum    []bool
	tpaOKs            int
}

func New(alias string, id int, peerAddrList []string, exec, beacon, durable bool, failures int, conf *config.Config, logger *dlog.Logger) *Replica {
	r := &Replica{
		Replica: replica.New(alias, id, failures, peerAddrList, true, exec, false, conf, logger),

		prepareChan:           make(chan fastrpc.Serializable, defs.CHAN_BUFFER_SIZE),
		preAcceptChan:         make(chan fastrpc.Serializable, defs.CHAN_BUFFER_SIZE),
		acceptChan:            make(chan fastrpc.Serializable, defs.CHAN_BUFFER_SIZE),
		commitChan:            make(chan fastrpc.Serializable, defs.CHAN_BUFFER_SIZE),
		commitShortChan:       make(chan fastrpc.Serializable, defs.CHAN_BUFFER_SIZE),
		prepareReplyChan:      make(chan fastrpc.Serializable, defs.CHAN_BUFFER_SIZE),
		preAcceptReplyChan:    make(chan fastrpc.Serializable, defs.CHAN_BUFFER_SIZE*3),
		preAcceptOKChan:       make(chan fastrpc.Serializable, defs.CHAN_BUFFER_SIZE*3),
		acceptReplyChan:       make(chan fastrpc.Serializable, defs.CHAN_BUFFER_SIZE*2),
		tryPreAcceptChan:      make(chan fastrpc.Serializable, defs.CHAN_BUFFER_SIZE),
		tryPreAcceptReplyChan: make(chan fastrpc.Serializable, defs.CHAN_BUFFER_SIZE),
		causalCommitChan:      make([]chan fastrpc.Serializable, len(peerAddrList)*NO_CAUSAL_CHANNEL),

		causalCommitRPC: make([]uint8, len(peerAddrList)*NO_CAUSAL_CHANNEL),

		InstanceSpace: make([][]*Instance, len(peerAddrList)),
		crtInstance:   make([]int32, len(peerAddrList)),
		CommittedUpTo: make([]int32, len(peerAddrList)),
		ExecedUpTo:    make([]int32, len(peerAddrList)),

		conflicts:     make([]map[state.Key]int32, len(peerAddrList)),
		conflictMutex: new(sync.RWMutex),
		maxSeqPerKey:  make(map[state.Key]int32),
		maxSeqPerKeyMu: new(sync.RWMutex),
		maxSeq:        0,

		sessionConflicts:   make([]map[int32]int32, len(peerAddrList)),
		sessionConflictsMu: new(sync.RWMutex),

		maxWriteInstancePerKey:   make(map[state.Key]*instanceId),
		maxWriteInstancePerKeyMu: new(sync.RWMutex),
		maxWriteSeqPerKey:        make(map[state.Key]int32),
		maxWriteSeqPerKeyMu:      new(sync.RWMutex),

		latestCPReplica:  0,
		latestCPInstance: -1,

		clientMutex: new(sync.Mutex),

		instancesToRecover: make(chan *instanceId, defs.CHAN_BUFFER_SIZE),
	}

	r.Beacon = beacon
	r.Durable = durable
	// Reply at commit time (not execution time) to avoid stalls from
	// pending dependency resolution in the SCC-based execution engine.
	r.Dreply = false

	for i := 0; i < r.N; i++ {
		r.InstanceSpace[i] = make([]*Instance, MAX_INSTANCE)
		r.crtInstance[i] = 0
		r.ExecedUpTo[i] = -1
		r.CommittedUpTo[i] = -1
		r.conflicts[i] = make(map[state.Key]int32, HT_INIT_SIZE)
		r.sessionConflicts[i] = make(map[int32]int32, 10)
	}

	for i := 0; i < r.N*NO_CAUSAL_CHANNEL; i++ {
		r.causalCommitChan[i] = make(chan fastrpc.Serializable, defs.CHAN_BUFFER_SIZE)
	}

	// Register RPCs
	r.prepareRPC = r.RPC.Register(new(Prepare), r.prepareChan)
	r.prepareReplyRPC = r.RPC.Register(new(PrepareReply), r.prepareReplyChan)
	r.preAcceptRPC = r.RPC.Register(new(PreAccept), r.preAcceptChan)
	r.preAcceptReplyRPC = r.RPC.Register(new(PreAcceptReply), r.preAcceptReplyChan)
	r.preAcceptOKRPC = r.RPC.Register(new(PreAcceptOK), r.preAcceptOKChan)
	r.acceptRPC = r.RPC.Register(new(Accept), r.acceptChan)
	r.acceptReplyRPC = r.RPC.Register(new(AcceptReply), r.acceptReplyChan)
	for i := 0; i < r.N*NO_CAUSAL_CHANNEL; i++ {
		r.causalCommitRPC[i] = r.RPC.Register(new(CausalCommit), r.causalCommitChan[i])
	}
	r.commitRPC = r.RPC.Register(new(Commit), r.commitChan)
	r.commitShortRPC = r.RPC.Register(new(CommitShort), r.commitShortChan)
	r.tryPreAcceptRPC = r.RPC.Register(new(TryPreAccept), r.tryPreAcceptChan)
	r.tryPreAcceptReplyRPC = r.RPC.Register(new(TryPreAcceptReply), r.tryPreAcceptReplyChan)

	r.exec = &Exec{r}

	cpMarker = make([]state.Command, 0)

	r.Stats.M["weird"], r.Stats.M["conflicted"], r.Stats.M["slow"], r.Stats.M["fast"], r.Stats.M["totalCommitTime"] = 0, 0, 0, 0, 0

	go r.run()

	return r
}

// Exec is the execution engine for EPaxos-HO.
// It implements Tarjan's SCC algorithm for strong command ordering
// and direct execution for causal commands.
type Exec struct {
	r *Replica
}

// recordInstanceMetadata appends instance metadata to stable storage.
func (r *Replica) recordInstanceMetadata(inst *Instance) {
	if !r.Durable {
		return
	}

	b := make([]byte, 9+r.N*4)
	binary.LittleEndian.PutUint32(b[0:4], uint32(inst.bal))
	binary.LittleEndian.PutUint32(b[4:8], uint32(inst.vbal))
	b[8] = byte(inst.Status)
	l := 9
	for _, dep := range inst.Deps {
		binary.LittleEndian.PutUint32(b[l:l+4], uint32(dep))
		l += 4
	}
	r.StableStore.Write(b)
}

// recordCommands appends commands to stable storage.
func (r *Replica) recordCommands(cmds []state.Command) {
	if !r.Durable {
		return
	}

	if cmds == nil {
		return
	}
	for i := 0; i < len(cmds); i++ {
		cmds[i].Marshal(r.StableStore)
	}
}

func (r *Replica) sync() {
	if !r.Durable {
		return
	}
	r.StableStore.Sync()
}

var fastClockChan chan bool
var slowClockChan chan bool

func (r *Replica) slowClock() {
	for !r.Shutdown {
		time.Sleep(150 * time.Millisecond)
		slowClockChan <- true
	}
}

func (r *Replica) fastClock() {
	for !r.Shutdown {
		time.Sleep(5 * time.Millisecond)
		fastClockChan <- true
	}
}

func (r *Replica) stopAdapting() {
	time.Sleep(1000 * 1000 * 1000 * ADAPT_TIME_SEC)
	r.Beacon = false
	time.Sleep(1000 * 1000 * 1000)

	for i := 0; i < r.N-1; i++ {
		min := i
		for j := i + 1; j < r.N-1; j++ {
			if r.Ewma[r.PreferredPeerOrder[j]] < r.Ewma[r.PreferredPeerOrder[min]] {
				min = j
			}
		}
		aux := r.PreferredPeerOrder[i]
		r.PreferredPeerOrder[i] = r.PreferredPeerOrder[min]
		r.PreferredPeerOrder[min] = aux
	}

	r.Println(r.PreferredPeerOrder)
}

func (r *Replica) run() {
	r.ConnectToPeers()

	r.ComputeClosestPeers()

	if r.Exec {
		go r.executeCommands()
	}

	slowClockChan = make(chan bool, 1)
	fastClockChan = make(chan bool, 1)
	go r.slowClock()

	if r.Beacon {
		go r.stopAdapting()
	}

	go r.WaitForClientConnections()

	for !r.Shutdown {
		// Poll causal commit channels (non-blocking) before the main select.
		// Multiple channels per replica avoid serialization bottlenecks.
		for _, ch := range r.causalCommitChan {
			select {
			case causalCommitS := <-ch:
				commit := causalCommitS.(*CausalCommit)
				r.handleCausalCommit(commit)
			default:
			}
		}

		select {
		case propose := <-r.ProposeChan:
			r.handlePropose(propose)

		case prepareS := <-r.prepareChan:
			prepare := prepareS.(*Prepare)
			r.handlePrepare(prepare)

		case preAcceptS := <-r.preAcceptChan:
			preAccept := preAcceptS.(*PreAccept)
			r.handlePreAccept(preAccept)

		case acceptS := <-r.acceptChan:
			accept := acceptS.(*Accept)
			r.handleAccept(accept)

		case commitS := <-r.commitChan:
			commit := commitS.(*Commit)
			r.handleCommit(commit)

		case commitS := <-r.commitShortChan:
			commit := commitS.(*CommitShort)
			r.handleCommitShort(commit)

		case prepareReplyS := <-r.prepareReplyChan:
			prepareReply := prepareReplyS.(*PrepareReply)
			r.handlePrepareReply(prepareReply)

		case preAcceptReplyS := <-r.preAcceptReplyChan:
			preAcceptReply := preAcceptReplyS.(*PreAcceptReply)
			r.handlePreAcceptReply(preAcceptReply)

		case preAcceptOKS := <-r.preAcceptOKChan:
			preAcceptOK := preAcceptOKS.(*PreAcceptOK)
			r.handlePreAcceptOK(preAcceptOK)

		case acceptReplyS := <-r.acceptReplyChan:
			acceptReply := acceptReplyS.(*AcceptReply)
			r.handleAcceptReply(acceptReply)

		case tryPreAcceptS := <-r.tryPreAcceptChan:
			tryPreAccept := tryPreAcceptS.(*TryPreAccept)
			r.handleTryPreAccept(tryPreAccept)

		case tryPreAcceptReplyS := <-r.tryPreAcceptReplyChan:
			tryPreAcceptReply := tryPreAcceptReplyS.(*TryPreAcceptReply)
			r.handleTryPreAcceptReply(tryPreAcceptReply)

		case beacon := <-r.BeaconChan:
			r.ReplyBeacon(beacon)

		case <-slowClockChan:
			if r.Beacon {
				r.Printf("weird %d; conflicted %d; slow %d; fast %d\n",
					r.Stats.M["weird"], r.Stats.M["conflicted"], r.Stats.M["slow"], r.Stats.M["fast"])
				for q := int32(0); q < int32(r.N); q++ {
					if q == r.Id {
						continue
					}
					r.SendBeacon(q)
				}
			}

		case iid := <-r.instancesToRecover:
			r.startRecoveryForInstance(iid.replica, iid.instance)
		}
	}
}

// --- Stub handlers (to be implemented in phases 99.3d-g) ---

func (r *Replica) handlePropose(propose *defs.GPropose) {
	// TODO: Phase 99.3d — separate causal/strong batches based on cmd.CL
}

func (r *Replica) handlePrepare(prepare *Prepare) {
	// TODO: Phase 99.3g — recovery path
}

func (r *Replica) handlePreAccept(preAccept *PreAccept) {
	// TODO: Phase 99.3f — strong commit path
}

func (r *Replica) handleAccept(accept *Accept) {
	// TODO: Phase 99.3f — strong commit path
}

func (r *Replica) handleCommit(commit *Commit) {
	// TODO: Phase 99.3f — strong commit handling
}

func (r *Replica) handleCommitShort(commit *CommitShort) {
	// TODO: Phase 99.3f — short commit handling
}

func (r *Replica) handleCausalCommit(commit *CausalCommit) {
	// TODO: Phase 99.3e — causal commit handling
}

func (r *Replica) handlePrepareReply(reply *PrepareReply) {
	// TODO: Phase 99.3g — recovery path
}

func (r *Replica) handlePreAcceptReply(reply *PreAcceptReply) {
	// TODO: Phase 99.3f — strong commit path
}

func (r *Replica) handlePreAcceptOK(msg *PreAcceptOK) {
	// TODO: Phase 99.3f — fast path acknowledgment
}

func (r *Replica) handleAcceptReply(reply *AcceptReply) {
	// TODO: Phase 99.3f — strong commit path
}

func (r *Replica) handleTryPreAccept(msg *TryPreAccept) {
	// TODO: Phase 99.3g — recovery path
}

func (r *Replica) handleTryPreAcceptReply(reply *TryPreAcceptReply) {
	// TODO: Phase 99.3g — recovery path
}

func (r *Replica) startRecoveryForInstance(replicaId int32, instanceId int32) {
	// TODO: Phase 99.3g — initiate recovery for a stalled instance
}

func (r *Replica) executeCommands() {
	// TODO: Phase 99.4 — execution engine with Tarjan SCC
}
