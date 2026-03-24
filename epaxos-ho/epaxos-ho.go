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

const DO_CHECKPOINTING = true
const CHECKPOINT_PERIOD = 10000


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
	// Causal commit channel
	causalCommitChan chan fastrpc.Serializable

	// Diagnostic counter (incremented in event loop whenever a client reply is sent)
	clientReplyCount int64

	// Outstanding strong commands awaiting quorum (for diagnostics)
	outstandingStrong int64

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
	causalCommitRPC       uint8

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
	lastStuckRetry     time.Time

	// Batching
	batchWait int
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

func (r *Replica) BatchingEnabled() bool {
	return r.batchWait > 0
}

func New(alias string, id int, peerAddrList []string, exec, beacon, durable bool, batchWait int, failures int, conf *config.Config, logger *dlog.Logger) *Replica {
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
		causalCommitChan: make(chan fastrpc.Serializable, defs.CHAN_BUFFER_SIZE),

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

		batchWait: batchWait,
	}

	r.Beacon = beacon
	r.Durable = durable
	r.Dreply = true

	for i := 0; i < r.N; i++ {
		r.InstanceSpace[i] = make([]*Instance, MAX_INSTANCE)
		r.crtInstance[i] = 0
		r.ExecedUpTo[i] = -1
		r.CommittedUpTo[i] = -1
		r.conflicts[i] = make(map[state.Key]int32, HT_INIT_SIZE)
		r.sessionConflicts[i] = make(map[int32]int32, 10)
	}

	// Register RPCs
	r.prepareRPC = r.RPC.Register(new(Prepare), r.prepareChan)
	r.prepareReplyRPC = r.RPC.Register(new(PrepareReply), r.prepareReplyChan)
	r.preAcceptRPC = r.RPC.Register(new(PreAccept), r.preAcceptChan)
	r.preAcceptReplyRPC = r.RPC.Register(new(PreAcceptReply), r.preAcceptReplyChan)
	r.preAcceptOKRPC = r.RPC.Register(new(PreAcceptOK), r.preAcceptOKChan)
	r.acceptRPC = r.RPC.Register(new(Accept), r.acceptChan)
	r.acceptReplyRPC = r.RPC.Register(new(AcceptReply), r.acceptReplyChan)
	r.causalCommitRPC = r.RPC.Register(new(CausalCommit), r.causalCommitChan)
	r.commitRPC = r.RPC.Register(new(Commit), r.commitChan)
	r.commitShortRPC = r.RPC.Register(new(CommitShort), r.commitShortChan)
	r.tryPreAcceptRPC = r.RPC.Register(new(TryPreAccept), r.tryPreAcceptChan)
	r.tryPreAcceptReplyRPC = r.RPC.Register(new(TryPreAcceptReply), r.tryPreAcceptReplyChan)

	r.exec = &Exec{r: r}

	cpMarker = make([]state.Command, 0)

	r.Stats.M["weird"], r.Stats.M["conflicted"], r.Stats.M["slow"], r.Stats.M["fast"], r.Stats.M["totalCommitTime"] = 0, 0, 0, 0, 0

	go r.run()

	return r
}

// Exec is the execution engine for EPaxos-HO.
// It implements Tarjan's SCC algorithm for strong command ordering
// and direct execution for causal commands.
type Exec struct {
	r           *Replica
	skippedDeps int64 // count of causal deps skipped during execute (atomic)
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
		time.Sleep(time.Duration(r.batchWait) * time.Millisecond)
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

	if r.BatchingEnabled() {
		go r.fastClock()
	}

	if r.Beacon {
		go r.stopAdapting()
	}

	onOffProposeChan := r.ProposeChan

	go r.WaitForClientConnections()

	// Periodic heartbeat to detect event loop stalls
	heartbeatCount := int64(0)
	proposeCount := int64(0)
	causalCommitCount := int64(0)
	preAcceptCount := int64(0)
	preAcceptReplyCount := int64(0)
	preAcceptOKCount := int64(0)
	commitCount := int64(0)
	acceptReplyCount := int64(0)
	go func() {
		for !r.Shutdown {
			time.Sleep(5 * time.Second)
			r.Printf("HEARTBEAT: iters=%d alive=%v propose=%d causalCommit=%d preAccept=%d paReply=%d paOK=%d commit=%d accReply=%d clientReply=%d proposeChanLen=%d outStrong=%d",
				heartbeatCount, r.Alive, proposeCount, causalCommitCount, preAcceptCount,
				preAcceptReplyCount, preAcceptOKCount, commitCount, acceptReplyCount, r.clientReplyCount, len(r.ProposeChan), r.outstandingStrong)
		}
	}()

	for !r.Shutdown {
		heartbeatCount++

		select {
		case causalCommitS := <-r.causalCommitChan:
			commit := causalCommitS.(*CausalCommit)
			causalCommitCount++
			r.handleCausalCommit(commit)

		case propose := <-onOffProposeChan:
			proposeCount++
			r.handlePropose(propose)
			if r.BatchingEnabled() {
				onOffProposeChan = nil
			}

		case prepareS := <-r.prepareChan:
			prepare := prepareS.(*Prepare)
			r.handlePrepare(prepare)

		case preAcceptS := <-r.preAcceptChan:
			preAccept := preAcceptS.(*PreAccept)
			preAcceptCount++
			r.handlePreAccept(preAccept)

		case acceptS := <-r.acceptChan:
			accept := acceptS.(*Accept)
			r.handleAccept(accept)

		case commitS := <-r.commitChan:
			commit := commitS.(*Commit)
			commitCount++
			r.handleCommit(commit)

		case commitS := <-r.commitShortChan:
			commit := commitS.(*CommitShort)
			commitCount++
			r.handleCommitShort(commit)

		case prepareReplyS := <-r.prepareReplyChan:
			prepareReply := prepareReplyS.(*PrepareReply)
			r.handlePrepareReply(prepareReply)

		case preAcceptReplyS := <-r.preAcceptReplyChan:
			preAcceptReply := preAcceptReplyS.(*PreAcceptReply)
			preAcceptReplyCount++
			r.handlePreAcceptReply(preAcceptReply)

		case preAcceptOKS := <-r.preAcceptOKChan:
			preAcceptOK := preAcceptOKS.(*PreAcceptOK)
			preAcceptOKCount++
			r.handlePreAcceptOK(preAcceptOK)

		case acceptReplyS := <-r.acceptReplyChan:
			acceptReply := acceptReplyS.(*AcceptReply)
			acceptReplyCount++
			r.handleAcceptReply(acceptReply)

		case tryPreAcceptS := <-r.tryPreAcceptChan:
			tryPreAccept := tryPreAcceptS.(*TryPreAccept)
			r.handleTryPreAccept(tryPreAccept)

		case tryPreAcceptReplyS := <-r.tryPreAcceptReplyChan:
			tryPreAcceptReply := tryPreAcceptReplyS.(*TryPreAcceptReply)
			r.handleTryPreAcceptReply(tryPreAcceptReply)

		case <-fastClockChan:
			onOffProposeChan = r.ProposeChan

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
			// Retry stuck PREACCEPTED/ACCEPTED instances (leader-side liveness)
			if r.outstandingStrong > 0 && time.Since(r.lastStuckRetry) > 1*time.Second {
				r.retryStuckInstances()
				r.lastStuckRetry = time.Now()
			}

		case iid := <-r.instancesToRecover:
			r.startRecoveryForInstance(iid.replica, iid.instance)
		}
	}
}

// --- Protocol handlers ---

func (r *Replica) handlePropose(propose *defs.GPropose) {
	batchSize := len(r.ProposeChan) + 1

	// Classify proposals by consistency level into separate batches.
	// Matching Orca design: causal and strong commands get SEPARATE instances.
	causalCmds := make([]state.Command, 0, batchSize)
	causalProposals := make([]*defs.GPropose, 0, batchSize)
	strongCmds := make([]state.Command, 0, batchSize)
	strongProposals := make([]*defs.GPropose, 0, batchSize)

	// Classify first proposal
	if propose.Command.CL == state.CAUSAL {
		causalCmds = append(causalCmds, propose.Command)
		causalProposals = append(causalProposals, propose)
	} else {
		strongCmds = append(strongCmds, propose.Command)
		strongProposals = append(strongProposals, propose)
	}

	// Drain remaining proposals
	for i := 1; i < batchSize; i++ {
		prop := <-r.ProposeChan
		if prop.Command.CL == state.CAUSAL {
			causalCmds = append(causalCmds, prop.Command)
			causalProposals = append(causalProposals, prop)
		} else {
			strongCmds = append(strongCmds, prop.Command)
			strongProposals = append(strongProposals, prop)
		}
	}

	// Create separate instances for causal and strong (matching Orca design).
	// This ensures causal instances follow 1-RTT path and strong instances
	// follow PreAccept consensus path independently.
	if len(causalProposals) > 0 {
		instNo := r.crtInstance[r.Id]
		r.crtInstance[r.Id]++
		r.startCausalCommit(r.Id, instNo, 0, causalProposals, causalCmds)
	}
	if len(strongProposals) > 0 {
		instNo := r.crtInstance[r.Id]
		r.crtInstance[r.Id]++
		r.startStrongCommit(r.Id, instNo, 0, strongProposals, strongCmds)
	}
}

// startCausalCommit initiates a 1-RTT causal commit for the given commands.
// Causal commands are committed immediately and broadcast to all replicas.
func (r *Replica) startCausalCommit(replicaId int32, instance int32, ballot int32, proposals []*defs.GPropose, cmds []state.Command) {
	seq := int32(0)
	deps := make([]int32, r.N)
	cl := make([]int32, r.N)
	for q := 0; q < r.N; q++ {
		deps[q] = -1
		cl[q] = 0
	}

	seq, deps, cl = r.updateCausalAttributes(cmds, seq, deps, cl, replicaId, instance)

	comDeps := make([]int32, r.N)
	for i := 0; i < r.N; i++ {
		comDeps[i] = -1
	}

	r.InstanceSpace[replicaId][instance] = &Instance{
		Cmds:       cmds,
		bal:        ballot,
		vbal:       ballot,
		Status:     CAUSALLY_COMMITTED,
		State:      READY,
		Seq:        seq,
		Deps:       deps,
		CL:         cl,
		lb:         &LeaderBookkeeping{clientProposals: proposals, allEqual: true, originalDeps: deps, committedDeps: comDeps},
		instanceId: &instanceId{replicaId, instance},
	}

	if seq >= r.maxSeq {
		r.maxSeq = seq + 1
	}

	r.updateCommitted(replicaId)

	// Reply to clients at commit time for causal ops (1-RTT fast path)
	if r.InstanceSpace[replicaId][instance].lb.clientProposals != nil && !r.Dreply {
		for i := 0; i < len(r.InstanceSpace[replicaId][instance].lb.clientProposals); i++ {
			prop := r.InstanceSpace[replicaId][instance].lb.clientProposals[i]
			r.clientReplyCount++
			r.ReplyProposeTS(
				&defs.ProposeReplyTS{
					OK:        TRUE,
					CommandId: prop.CommandId,
					Value:     state.NIL(),
					Timestamp: prop.Timestamp,
				},
				prop.Reply,
				prop.Mutex)
		}
	}

	r.updateCausalConflicts(cmds, replicaId, instance, seq, true)

	r.recordInstanceMetadata(r.InstanceSpace[replicaId][instance])
	r.recordCommands(cmds)
	r.sync()
	r.bcastCausalCommit(replicaId, instance, cmds, seq, deps, cl, state.CAUSAL)

	cpcounter += len(cmds)

	if replicaId == r.Id && DO_CHECKPOINTING && cpcounter >= CHECKPOINT_PERIOD {
		cpcounter = 0

		r.crtInstance[r.Id]++
		instance++

		r.maxSeq++
		for q := 0; q < r.N; q++ {
			deps[q] = r.crtInstance[int32(q)] - 1
			cl[q] = 0
		}

		r.InstanceSpace[replicaId][instance] = &Instance{
			Cmds:       cpMarker,
			bal:        0,
			vbal:       0,
			Status:     CAUSALLY_COMMITTED,
			State:      READY,
			Seq:        r.maxSeq,
			Deps:       deps,
			CL:         cl,
			lb:         &LeaderBookkeeping{allEqual: true, originalDeps: deps, committedDeps: comDeps},
			instanceId: &instanceId{replicaId, instance},
		}

		r.latestCPReplica = replicaId
		r.latestCPInstance = instance

		r.clearHashtables()

		r.recordInstanceMetadata(r.InstanceSpace[replicaId][instance])
		r.sync()

		r.bcastCausalCommit(replicaId, instance, cpMarker, r.maxSeq, deps, cl, state.CAUSAL)
	}
}

// bcastPreAccept broadcasts a PreAccept message to peers.
// With Thrifty mode, sends to a fast-path quorum instead of all peers.
func (r *Replica) bcastPreAccept(replicaId int32, instance int32, ballot int32, cmds []state.Command, seq int32, deps []int32, cl []int32) {
	defer func() {
		if err := recover(); err != nil {
			dlog.Println("PreAccept bcast failed:", err)
		}
	}()

	args := &PreAccept{
		LeaderId: r.Id,
		Replica:  replicaId,
		Instance: instance,
		Ballot:   ballot,
		Command:  cmds,
		Seq:      seq,
		Deps:     deps,
		CL:       cl,
	}

	n := r.N - 1
	if r.Thrifty {
		n = r.N/2 + (r.N/2+1)/2 - 1
	}

	sent := 0
	for q := 0; q < r.N; q++ {
		peer := r.PreferredPeerOrder[q]
		if peer == r.Id || !r.Alive[peer] {
			continue
		}
		r.SendMsg(peer, r.preAcceptRPC, args)
		sent++
		if sent >= n {
			break
		}
	}
}

// bcastAccept broadcasts an Accept message to peers.
// With Thrifty mode, sends to a simple majority instead of all peers.
func (r *Replica) bcastAccept(replicaId int32, instance int32, ballot int32, count int32, seq int32, deps []int32, cl []int32) {
	defer func() {
		if err := recover(); err != nil {
			dlog.Println("Accept bcast failed:", err)
		}
	}()

	args := &Accept{
		LeaderId: r.Id,
		Replica:  replicaId,
		Instance: instance,
		Ballot:   ballot,
		Count:    count,
		Seq:      seq,
		Deps:     deps,
		CL:       cl,
	}

	n := r.N - 1
	if r.Thrifty {
		n = r.N / 2
	}

	sent := 0
	for q := 0; q < r.N; q++ {
		peer := r.PreferredPeerOrder[q]
		if peer == r.Id || !r.Alive[peer] {
			continue
		}
		r.SendMsg(peer, r.acceptRPC, args)
		sent++
		if sent >= n {
			break
		}
	}
}

// bcastStrongCommit broadcasts a strong Commit to all peers.
// Sends CommitShort (without commands) to the first half and full Commit to the rest,
// unless Thrifty mode is active (switches to full Commit after quorum).
func (r *Replica) bcastStrongCommit(replicaId int32, instance int32, cmds []state.Command, seq int32, deps []int32, cl []int32, consistency state.Operation) {
	defer func() {
		if err := recover(); err != nil {
			dlog.Println("Commit bcast failed:", err)
		}
	}()

	args := &Commit{
		Consistency: consistency,
		LeaderId:    r.Id,
		Replica:     replicaId,
		Instance:    instance,
		Command:     cmds,
		Seq:         seq,
		Deps:        deps,
		CL:          cl,
	}

	argsShort := &CommitShort{
		Consistency: consistency,
		LeaderId:    r.Id,
		Replica:     replicaId,
		Instance:    instance,
		Count:       int32(len(cmds)),
		Seq:         seq,
		Deps:        deps,
		CL:          cl,
	}

	sent := 0
	for q := 0; q < r.N; q++ {
		peer := r.PreferredPeerOrder[q]
		if peer == r.Id || !r.Alive[peer] {
			continue
		}
		if r.Thrifty && sent >= r.N/2 {
			r.SendMsg(peer, r.commitRPC, args)
		} else {
			r.SendMsg(peer, r.commitShortRPC, argsShort)
			sent++
		}
	}
}

// startStrongCommit initiates the EPaxos-style 2-RTT commit for strong commands.
// Strong commands go through PreAccept → Accept → Commit phases.
// This function computes initial attributes, creates the instance, and broadcasts PreAccept.
func (r *Replica) startStrongCommit(replicaId int32, instance int32, ballot int32, proposals []*defs.GPropose, cmds []state.Command) {
	seq := int32(0)
	deps := make([]int32, r.N)
	cl := make([]int32, r.N)
	for q := 0; q < r.N; q++ {
		deps[q] = -1
		cl[q] = 0
	}

	seq, deps, cl, _ = r.updateStrongAttributes1(cmds, seq, deps, cl, replicaId, instance)

	comDeps := make([]int32, r.N)
	for i := 0; i < r.N; i++ {
		comDeps[i] = -1
	}

	r.InstanceSpace[replicaId][instance] = &Instance{
		Cmds:       cmds,
		bal:        ballot,
		vbal:       ballot,
		Status:     PREACCEPTED,
		State:      READY,
		Seq:        seq,
		Deps:       deps,
		CL:         cl,
		lb:         &LeaderBookkeeping{clientProposals: proposals, allEqual: true, originalDeps: deps, committedDeps: comDeps},
		instanceId: &instanceId{replicaId, instance},
	}

	if seq >= r.maxSeq {
		r.maxSeq = seq + 1
	}

	r.outstandingStrong += int64(len(proposals))

	r.updateStrongConflicts(cmds, replicaId, instance, seq)

	r.recordInstanceMetadata(r.InstanceSpace[replicaId][instance])
	r.recordCommands(cmds)
	r.sync()

	r.bcastPreAccept(replicaId, instance, ballot, cmds, seq, deps, cl)

	cpcounter += len(cmds)

	if replicaId == r.Id && DO_CHECKPOINTING && cpcounter >= CHECKPOINT_PERIOD {
		cpcounter = 0

		r.crtInstance[r.Id]++
		instance++

		r.maxSeq++
		for q := 0; q < r.N; q++ {
			deps[q] = r.crtInstance[int32(q)] - 1
			cl[q] = 0
		}

		r.InstanceSpace[r.Id][instance] = &Instance{
			Cmds:       cpMarker,
			bal:        0,
			vbal:       0,
			Status:     PREACCEPTED,
			State:      READY,
			Seq:        r.maxSeq,
			Deps:       deps,
			CL:         cl,
			lb:         &LeaderBookkeeping{allEqual: true, originalDeps: deps, committedDeps: comDeps},
			instanceId: &instanceId{r.Id, instance},
		}

		r.latestCPReplica = r.Id
		r.latestCPInstance = instance

		r.clearHashtables()

		r.recordInstanceMetadata(r.InstanceSpace[r.Id][instance])
		r.sync()

		r.bcastPreAccept(r.Id, instance, 0, cpMarker, r.maxSeq, deps, cl)
	}
}

// --- Ballot helpers ---

func (r *Replica) makeUniqueBallot(ballot int32) int32 {
	return (ballot << 4) | r.Id
}

func (r *Replica) makeBallotLargerThan(ballot int32) int32 {
	return r.makeUniqueBallot((ballot >> 4) + 1)
}

func isInitialBallot(ballot int32) bool {
	return (ballot >> 4) == 0
}

// bcastPrepare broadcasts a Prepare message to all peers.
// Used during recovery when a higher ballot is needed.
func (r *Replica) bcastPrepare(replicaId int32, instance int32, ballot int32) {
	defer func() {
		if err := recover(); err != nil {
			dlog.Println("Prepare bcast failed:", err)
		}
	}()

	args := &Prepare{
		LeaderId: r.Id,
		Replica:  replicaId,
		Instance: instance,
		Ballot:   ballot,
	}

	for q := 0; q < r.N; q++ {
		peer := r.PreferredPeerOrder[q]
		if peer == r.Id || !r.Alive[peer] {
			continue
		}
		r.SendMsg(peer, r.prepareRPC, args)
	}
}

// handlePrepare processes a Prepare message during recovery (responder side).
// Returns the acceptor's current state for this instance so the recovery leader
// can reconstruct the committed/pre-accepted state.
func (r *Replica) handlePrepare(prepare *Prepare) {
	inst := r.InstanceSpace[prepare.Replica][prepare.Instance]
	nildeps := make([]int32, r.N)
	nilcl := make([]int32, r.N)

	var preply *PrepareReply

	if inst == nil {
		// Never seen this instance — reply with NONE status
		r.InstanceSpace[prepare.Replica][prepare.Instance] = &Instance{
			bal:        prepare.Ballot,
			vbal:       prepare.Ballot,
			Status:     NONE,
			State:      DONE,
			Deps:       nildeps,
			CL:         nilcl,
			instanceId: &instanceId{prepare.Replica, prepare.Instance},
		}
		preply = &PrepareReply{
			AcceptorId: r.Id,
			Replica:    prepare.Replica,
			Instance:   prepare.Instance,
			OK:         TRUE,
			Bal:        -1,
			VBal:       -1,
			Status:     NONE,
			Seq:        -1,
			Deps:       nildeps,
			CL:         nilcl,
		}
	} else if inst.State == WAITING {
		// WAITING means causal dependency not reached — treat as NONE
		r.InstanceSpace[prepare.Replica][prepare.Instance] = &Instance{
			bal:        prepare.Ballot,
			vbal:       prepare.Ballot,
			Status:     NONE,
			State:      DONE,
			Deps:       nildeps,
			CL:         nilcl,
			instanceId: &instanceId{prepare.Replica, prepare.Instance},
		}
		preply = &PrepareReply{
			AcceptorId: r.Id,
			Replica:    prepare.Replica,
			Instance:   prepare.Instance,
			OK:         TRUE,
			Bal:        -1,
			VBal:       -1,
			Status:     NONE,
			Seq:        -1,
			Deps:       nildeps,
			CL:         nilcl,
		}
	} else {
		ok := uint8(TRUE)
		if prepare.Ballot < inst.bal {
			ok = FALSE
		} else {
			inst.bal = prepare.Ballot
		}
		preply = &PrepareReply{
			AcceptorId: r.Id,
			Replica:    prepare.Replica,
			Instance:   prepare.Instance,
			OK:         ok,
			Bal:        inst.bal,
			VBal:       inst.vbal,
			Status:     inst.Status,
			Command:    inst.Cmds,
			Seq:        inst.Seq,
			Deps:       inst.Deps,
			CL:         inst.CL,
		}
	}

	r.replyPrepare(prepare.LeaderId, preply)
}

// handlePreAccept processes a PreAccept message from the leader (follower side).
// It updates attributes with local conflict information and replies with either
// a PreAcceptOK (fast path, no changes) or PreAcceptReply (with updated attributes).
func (r *Replica) handlePreAccept(preAccept *PreAccept) {
	inst := r.InstanceSpace[preAccept.Replica][preAccept.Instance]

	if preAccept.Seq >= r.maxSeq {
		r.maxSeq = preAccept.Seq + 1
	}

	if preAccept.Instance >= r.crtInstance[preAccept.Replica] {
		r.crtInstance[preAccept.Replica] = preAccept.Instance + 1
	}

	// Already executed or discarded — send PreAcceptOK so leader can complete quorum
	if inst != nil && (inst.Status == EXECUTED || inst.Status == DISCARDED) {
		r.SendMsg(preAccept.LeaderId, r.preAcceptOKRPC, &PreAcceptOK{Instance: preAccept.Instance})
		return
	}

	// Reordered: we already received Commit/Accept before PreAccept.
	// Still send PreAcceptOK so the leader can reach quorum and commit.
	if inst != nil && (inst.Status == STRONGLY_COMMITTED || inst.Status == ACCEPTED) {
		if inst.Cmds == nil {
			r.InstanceSpace[preAccept.Replica][preAccept.Instance].Cmds = preAccept.Command
			r.updateStrongConflicts(preAccept.Command, preAccept.Replica, preAccept.Instance, preAccept.Seq)
		}
		r.recordCommands(preAccept.Command)
		r.sync()
		r.SendMsg(preAccept.LeaderId, r.preAcceptOKRPC, &PreAcceptOK{Instance: preAccept.Instance})
		return
	}

	// Compute local attributes
	seq, deps, cl, changed := r.updateStrongAttributes2(preAccept.Command, preAccept.Seq, preAccept.Deps, preAccept.CL, preAccept.Replica, preAccept.Instance)

	// Check for uncommitted strong dependencies
	uncommittedDeps := false
	for q := 0; q < r.N; q++ {
		if cl[q] == int32(state.STRONG) && deps[q] > r.CommittedUpTo[q] {
			uncommittedDeps = true
			break
		}
	}

	status := int8(PREACCEPTED_EQ)
	if changed {
		status = PREACCEPTED
	}

	// Ballot check and instance update
	if inst != nil {
		if preAccept.Ballot < inst.bal {
			r.replyPreAccept(preAccept.LeaderId, &PreAcceptReply{
				Replica:       preAccept.Replica,
				Instance:      preAccept.Instance,
				OK:            FALSE,
				Ballot:        inst.bal,
				Seq:           inst.Seq,
				Deps:          inst.Deps,
				CL:            inst.CL,
				CommittedDeps: r.CommittedUpTo,
			})
			return
		}
		inst.Cmds = preAccept.Command
		inst.Seq = seq
		inst.Deps = deps
		inst.CL = cl
		inst.bal = preAccept.Ballot
		inst.vbal = preAccept.Ballot
		inst.Status = status
		inst.State = READY
	} else {
		r.InstanceSpace[preAccept.Replica][preAccept.Instance] = &Instance{
			Cmds:       preAccept.Command,
			bal:        preAccept.Ballot,
			vbal:       preAccept.Ballot,
			Status:     status,
			State:      READY,
			Seq:        seq,
			Deps:       deps,
			CL:         cl,
			instanceId: &instanceId{preAccept.Replica, preAccept.Instance},
		}
	}

	r.updateStrongConflicts(preAccept.Command, preAccept.Replica, preAccept.Instance, preAccept.Seq)

	r.recordInstanceMetadata(r.InstanceSpace[preAccept.Replica][preAccept.Instance])
	r.recordCommands(preAccept.Command)
	r.sync()

	if len(preAccept.Command) == 0 {
		// Checkpoint
		r.latestCPReplica = preAccept.Replica
		r.latestCPInstance = preAccept.Instance
		r.clearHashtables()
	}

	if changed || uncommittedDeps || preAccept.Replica != preAccept.LeaderId || !isInitialBallot(preAccept.Ballot) {
		// Slow path reply — include updated attributes
		r.replyPreAccept(preAccept.LeaderId, &PreAcceptReply{
			Replica:       preAccept.Replica,
			Instance:      preAccept.Instance,
			OK:            TRUE,
			Ballot:        preAccept.Ballot,
			Seq:           seq,
			Deps:          deps,
			CL:            cl,
			CommittedDeps: r.CommittedUpTo,
		})
	} else {
		// Fast path reply — attributes unchanged
		r.SendMsg(preAccept.LeaderId, r.preAcceptOKRPC, &PreAcceptOK{Instance: preAccept.Instance})
	}
}

// handleAccept processes an Accept message from the leader (follower side).
// Sets instance to ACCEPTED with the leader's finalized attributes and replies.
func (r *Replica) handleAccept(accept *Accept) {
	inst := r.InstanceSpace[accept.Replica][accept.Instance]

	if accept.Seq >= r.maxSeq {
		r.maxSeq = accept.Seq + 1
	}

	if inst != nil && (inst.Status == STRONGLY_COMMITTED || inst.Status == CAUSALLY_COMMITTED || inst.Status == EXECUTED || inst.Status == DISCARDED) {
		// Already committed/executed — still send AcceptReply so leader can reach quorum
		r.replyAccept(accept.LeaderId, &AcceptReply{
			Replica:  accept.Replica,
			Instance: accept.Instance,
			OK:       TRUE,
			Ballot:   accept.Ballot,
		})
		return
	}

	if accept.Instance >= r.crtInstance[accept.Replica] {
		r.crtInstance[accept.Replica] = accept.Instance + 1
	}

	if inst != nil {
		if accept.Ballot < inst.bal {
			r.replyAccept(accept.LeaderId, &AcceptReply{
				Replica:  accept.Replica,
				Instance: accept.Instance,
				OK:       FALSE,
				Ballot:   inst.bal,
			})
			return
		}
		inst.Status = ACCEPTED
		inst.State = READY
		inst.Seq = accept.Seq
		inst.Deps = accept.Deps
		inst.CL = accept.CL
		inst.bal = accept.Ballot
		inst.vbal = accept.Ballot
	} else {
		r.InstanceSpace[accept.Replica][accept.Instance] = &Instance{
			bal:        accept.Ballot,
			vbal:       accept.Ballot,
			Status:     ACCEPTED,
			State:      READY,
			Seq:        accept.Seq,
			Deps:       accept.Deps,
			CL:         accept.CL,
			instanceId: &instanceId{accept.Replica, accept.Instance},
		}

		if accept.Count == 0 {
			// Checkpoint
			r.latestCPReplica = accept.Replica
			r.latestCPInstance = accept.Instance
			r.clearHashtables()
		}
	}

	r.recordInstanceMetadata(r.InstanceSpace[accept.Replica][accept.Instance])
	r.sync()

	r.replyAccept(accept.LeaderId, &AcceptReply{
		Replica:  accept.Replica,
		Instance: accept.Instance,
		OK:       TRUE,
		Ballot:   accept.Ballot,
	})
}

// handleCommit processes a full Commit message (follower side, strong path).
// Contains full command payload. Sets instance to STRONGLY_COMMITTED.
func (r *Replica) handleCommit(commit *Commit) {
	inst := r.InstanceSpace[commit.Replica][commit.Instance]

	if commit.Seq >= r.maxSeq {
		r.maxSeq = commit.Seq + 1
	}

	if commit.Instance >= r.crtInstance[commit.Replica] {
		r.crtInstance[commit.Replica] = commit.Instance + 1
	}

	if inst != nil && (inst.Status == STRONGLY_COMMITTED || inst.Status == CAUSALLY_COMMITTED || inst.Status == EXECUTED || inst.Status == DISCARDED) {
		return
	}

	if inst != nil {
		if inst.lb != nil && inst.lb.clientProposals != nil && len(commit.Command) == 0 {
			// NO-OP committed — re-propose our pending proposals
			for _, p := range inst.lb.clientProposals {
				r.ProposeChan <- p
			}
			inst.lb = nil
		}
		inst.Cmds = commit.Command
		inst.Seq = commit.Seq
		inst.Deps = commit.Deps
		inst.CL = commit.CL
		inst.Status = STRONGLY_COMMITTED
		inst.State = READY
	} else {
		r.InstanceSpace[commit.Replica][commit.Instance] = &Instance{
			Cmds:       commit.Command,
			Status:     STRONGLY_COMMITTED,
			State:      READY,
			Seq:        commit.Seq,
			Deps:       commit.Deps,
			CL:         commit.CL,
			instanceId: &instanceId{commit.Replica, commit.Instance},
		}

		if len(commit.Command) == 0 {
			// Checkpoint
			r.latestCPReplica = commit.Replica
			r.latestCPInstance = commit.Instance
			r.clearHashtables()
		}
	}

	r.updateCommitted(commit.Replica)
	r.recordInstanceMetadata(r.InstanceSpace[commit.Replica][commit.Instance])
	r.recordCommands(commit.Command)
}

// handleCommitShort processes a short Commit message (follower side, strong path).
// Commands were already sent via PreAccept/Accept — only metadata here.
func (r *Replica) handleCommitShort(commit *CommitShort) {
	inst := r.InstanceSpace[commit.Replica][commit.Instance]

	if commit.Instance >= r.crtInstance[commit.Replica] {
		r.crtInstance[commit.Replica] = commit.Instance + 1
	}

	if inst != nil && (inst.Status == STRONGLY_COMMITTED || inst.Status == CAUSALLY_COMMITTED || inst.Status == EXECUTED || inst.Status == DISCARDED) {
		return
	}

	if inst != nil {
		if inst.lb != nil && inst.lb.clientProposals != nil {
			// Re-propose pending proposals in a different instance
			for _, p := range inst.lb.clientProposals {
				r.ProposeChan <- p
			}
			inst.lb = nil
		}
		inst.Seq = commit.Seq
		inst.Deps = commit.Deps
		inst.CL = commit.CL
		inst.Status = STRONGLY_COMMITTED
		inst.State = READY
	} else {
		r.InstanceSpace[commit.Replica][commit.Instance] = &Instance{
			Status:     STRONGLY_COMMITTED,
			State:      READY,
			Seq:        commit.Seq,
			Deps:       commit.Deps,
			CL:         commit.CL,
			instanceId: &instanceId{commit.Replica, commit.Instance},
		}

		if commit.Count == 0 {
			// Checkpoint
			r.latestCPReplica = commit.Replica
			r.latestCPInstance = commit.Instance
			r.clearHashtables()
		}
	}

	r.updateCommitted(commit.Replica)
	r.recordInstanceMetadata(r.InstanceSpace[commit.Replica][commit.Instance])
}

func (r *Replica) handleCausalCommit(commit *CausalCommit) {
	inst := r.InstanceSpace[commit.Replica][commit.Instance]

	if commit.Seq >= r.maxSeq {
		r.maxSeq = commit.Seq + 1
	}

	if commit.Instance >= r.crtInstance[commit.Replica] {
		r.crtInstance[commit.Replica] = commit.Instance + 1
	}

	if inst != nil && (inst.Status == CAUSALLY_COMMITTED || inst.Status == EXECUTED || inst.Status == DISCARDED) {
		return
	}

	if inst != nil {
		if inst.lb != nil && inst.lb.clientProposals != nil && len(commit.Command) == 0 {
			// Someone committed a NO-OP but we have proposals — re-propose them
			for _, p := range inst.lb.clientProposals {
				r.ProposeChan <- p
			}
			inst.lb = nil
		}

		inst.Cmds = commit.Command
		inst.State = READY
		inst.Seq = commit.Seq
		inst.Deps = commit.Deps
		inst.CL = commit.CL
		inst.Status = CAUSALLY_COMMITTED
	} else {
		r.InstanceSpace[commit.Replica][commit.Instance] = &Instance{
			Cmds:       commit.Command,
			Status:     CAUSALLY_COMMITTED,
			State:      READY,
			Seq:        commit.Seq,
			Deps:       commit.Deps,
			CL:         commit.CL,
			instanceId: &instanceId{commit.Replica, commit.Instance},
		}

		if len(commit.Command) == 0 {
			// Checkpoint
			r.latestCPReplica = commit.Replica
			r.latestCPInstance = commit.Instance
			r.clearHashtables()
		}
	}

	r.updateCommitted(commit.Replica)
	r.updateCausalConflicts(commit.Command, commit.Replica, commit.Instance, commit.Seq, false)

	r.recordInstanceMetadata(r.InstanceSpace[commit.Replica][commit.Instance])
	r.recordCommands(commit.Command)
}

// --- Causal commit helpers ---

// updateCommitted advances CommittedUpTo[q] by scanning forward while
// the next instance is committed (causal or strong), executed, or discarded.
func (r *Replica) updateCommitted(q int32) {
	for r.InstanceSpace[q][r.CommittedUpTo[q]+1] != nil &&
		(r.InstanceSpace[q][r.CommittedUpTo[q]+1].Status == STRONGLY_COMMITTED ||
			r.InstanceSpace[q][r.CommittedUpTo[q]+1].Status == CAUSALLY_COMMITTED ||
			r.InstanceSpace[q][r.CommittedUpTo[q]+1].Status == EXECUTED ||
			r.InstanceSpace[q][r.CommittedUpTo[q]+1].Status == DISCARDED) {
		r.CommittedUpTo[q]++
	}
}

// clearHashtables reinitializes all per-replica conflict and session conflict maps
// after a checkpoint, discarding stale dependency information.
func (r *Replica) clearHashtables() {
	for q := 0; q < r.N; q++ {
		r.conflicts[q] = make(map[state.Key]int32, HT_INIT_SIZE)
		r.sessionConflicts[q] = make(map[int32]int32, HT_INIT_SIZE)
	}
}

// updateCausalConflicts updates conflict tracking maps after a causal commit.
// For each command, it updates the per-key conflict map and maxSeqPerKey.
// If includeSession is true (leader path), it also updates session conflict maps.
func (r *Replica) updateCausalConflicts(cmds []state.Command, replicaId int32, instance int32, seq int32, includeSession bool) {
	r.conflictMutex.Lock()
	for i := 0; i < len(cmds); i++ {
		if d, present := r.conflicts[replicaId][cmds[i].K]; !present || d < instance {
			r.conflicts[replicaId][cmds[i].K] = instance
		}
	}
	r.conflictMutex.Unlock()

	r.maxSeqPerKeyMu.Lock()
	for i := 0; i < len(cmds); i++ {
		if s, present := r.maxSeqPerKey[cmds[i].K]; !present || s < seq {
			r.maxSeqPerKey[cmds[i].K] = seq
		}
	}
	r.maxSeqPerKeyMu.Unlock()

	if includeSession {
		r.sessionConflictsMu.Lock()
		for i := 0; i < len(cmds); i++ {
			sid := cmds[i].Sid
			if d, present := r.sessionConflicts[replicaId][sid]; !present || d < instance {
				r.sessionConflicts[replicaId][sid] = instance
			}
		}
		r.sessionConflictsMu.Unlock()
	}
}

// updateCausalAttributes computes causal dependencies for a batch of commands.
// It tracks: (1) session dependencies, (2) read-from dependencies (GET reads from latest PUT),
// (3) max sequence per key for causal ordering.
func (r *Replica) updateCausalAttributes(cmds []state.Command, seq int32, deps []int32, cl []int32, replicaId int32, instance int32) (int32, []int32, []int32) {
	// Track session dependency: find the latest committed command from the same session
	r.sessionConflictsMu.RLock()
	for i := 0; i < len(cmds); i++ {
		d, present := r.sessionConflicts[replicaId][cmds[i].Sid]
		if present && d > deps[replicaId] {
			deps[replicaId] = d
			if r.InstanceSpace[replicaId][d] != nil && len(r.InstanceSpace[replicaId][d].Cmds) > 0 {
				cl[replicaId] = int32(r.InstanceSpace[replicaId][d].Cmds[0].CL)
			}
			if r.InstanceSpace[replicaId][d] != nil && seq <= r.InstanceSpace[replicaId][d].Seq {
				seq = r.InstanceSpace[replicaId][d].Seq + 1
			}
			break
		}
	}
	r.sessionConflictsMu.RUnlock()

	// Track read-from dependency: GET commands depend on the latest write to that key
	r.maxWriteInstancePerKeyMu.RLock()
	for i := 0; i < len(cmds); i++ {
		if cmds[i].Op == state.GET {
			d, present := r.maxWriteInstancePerKey[cmds[i].K]
			if present && d.instance > deps[d.replica] {
				deps[d.replica] = d.instance
				if r.InstanceSpace[d.replica][d.instance] != nil && len(r.InstanceSpace[d.replica][d.instance].Cmds) > 0 {
					cl[d.replica] = int32(r.InstanceSpace[d.replica][d.instance].Cmds[0].CL)
				}
				if r.InstanceSpace[d.replica][d.instance] != nil && seq <= r.InstanceSpace[d.replica][d.instance].Seq {
					seq = r.InstanceSpace[d.replica][d.instance].Seq + 1
				}
				break
			}
		}
	}
	r.maxWriteInstancePerKeyMu.RUnlock()

	// Update seq from maxSeqPerKey for all affected keys
	r.maxSeqPerKeyMu.RLock()
	for i := 0; i < len(cmds); i++ {
		s, present := r.maxSeqPerKey[cmds[i].K]
		if present && seq <= s {
			seq = s + 1
		}
	}
	r.maxSeqPerKeyMu.RUnlock()

	return seq, deps, cl
}

// bcastCausalCommit broadcasts a CausalCommit message to all peer replicas.
// Uses a random causal commit RPC channel to balance load across channels.
func (r *Replica) bcastCausalCommit(replicaId int32, instance int32, cmds []state.Command, seq int32, deps []int32, cl []int32, consistency state.Operation) {
	defer func() {
		if err := recover(); err != nil {
			dlog.Println("Causal commit bcast failed:", err)
		}
	}()

	args := &CausalCommit{
		Consistency: consistency,
		LeaderId:    r.Id,
		Replica:     replicaId,
		Instance:    instance,
		Command:     cmds,
		Seq:         seq,
		Deps:        deps,
		CL:          cl,
	}
	for q := 0; q < r.N; q++ {
		peer := r.PreferredPeerOrder[q]
		if peer == r.Id || !r.Alive[peer] {
			continue
		}
		r.SendMsg(peer, r.causalCommitRPC, args)
	}
}

// --- Strong commit helpers (Phase 99.3f-i) ---

// Reply helpers — thin wrappers around SendMsg for each reply type.
func (r *Replica) replyPrepare(replicaId int32, reply *PrepareReply) {
	r.SendMsg(replicaId, r.prepareReplyRPC, reply)
}

func (r *Replica) replyPreAccept(replicaId int32, reply *PreAcceptReply) {
	r.SendMsg(replicaId, r.preAcceptReplyRPC, reply)
}

func (r *Replica) replyAccept(replicaId int32, reply *AcceptReply) {
	r.SendMsg(replicaId, r.acceptReplyRPC, reply)
}

func (r *Replica) replyTryPreAccept(replicaId int32, reply *TryPreAcceptReply) {
	r.SendMsg(replicaId, r.tryPreAcceptReplyRPC, reply)
}

// updateStrongConflicts updates conflict tracking maps after a strong commit.
// Unlike causal conflicts, this does NOT update session conflicts — strong ops
// use key-based conflict tracking only.
func (r *Replica) updateStrongConflicts(cmds []state.Command, replicaId int32, instance int32, seq int32) {
	r.conflictMutex.Lock()
	for i := 0; i < len(cmds); i++ {
		if d, present := r.conflicts[replicaId][cmds[i].K]; !present || d < instance {
			r.conflicts[replicaId][cmds[i].K] = instance
		}
	}
	r.conflictMutex.Unlock()

	r.maxSeqPerKeyMu.Lock()
	for i := 0; i < len(cmds); i++ {
		if s, present := r.maxSeqPerKey[cmds[i].K]; !present || s < seq {
			r.maxSeqPerKey[cmds[i].K] = seq
		}
	}
	r.maxSeqPerKeyMu.Unlock()
}

// updateStrongSessionConflict updates session conflict tracking for strong commands.
func (r *Replica) updateStrongSessionConflict(cmds []state.Command, replicaId int32, instance int32) {
	r.sessionConflictsMu.Lock()
	for i := 0; i < len(cmds); i++ {
		sid := cmds[i].Sid
		if d, present := r.sessionConflicts[replicaId][sid]; !present || d < instance {
			r.sessionConflicts[replicaId][sid] = instance
		}
	}
	r.sessionConflictsMu.Unlock()
}

// updateStrongAttributes1 computes initial dependencies for strong commands.
// It checks per-key conflicts across all replicas, session conflicts, and max seq per key.
// Returns: updated seq, deps, cl, and whether any dependency changed.
func (r *Replica) updateStrongAttributes1(cmds []state.Command, seq int32, deps []int32, cl []int32, replicaId int32, instance int32) (int32, []int32, []int32, bool) {
	changed := false

	// Check per-key conflicts across all replicas
	r.conflictMutex.RLock()
	for q := 0; q < r.N; q++ {
		if r.Id != replicaId && int32(q) == replicaId {
			continue
		}
		for i := 0; i < len(cmds); i++ {
			d, present := r.conflicts[q][cmds[i].K]
			if present && d > deps[q] {
				deps[q] = d
				if r.InstanceSpace[q][d] != nil && len(r.InstanceSpace[q][d].Cmds) > i {
					cl[q] = int32(r.InstanceSpace[q][d].Cmds[i].CL)
				}
				if r.InstanceSpace[q][d] != nil && seq <= r.InstanceSpace[q][d].Seq {
					seq = r.InstanceSpace[q][d].Seq + 1
				}
				changed = true
				break
			}
		}
	}
	r.conflictMutex.RUnlock()

	// Track session dependency
	r.sessionConflictsMu.RLock()
	for i := 0; i < len(cmds); i++ {
		d, present := r.sessionConflicts[replicaId][cmds[i].Sid]
		if present && d > deps[replicaId] {
			deps[replicaId] = d
			if r.InstanceSpace[replicaId][d] != nil && len(r.InstanceSpace[replicaId][d].Cmds) > 0 {
				cl[replicaId] = int32(r.InstanceSpace[replicaId][d].Cmds[0].CL)
			}
			if r.InstanceSpace[replicaId][d] != nil && seq <= r.InstanceSpace[replicaId][d].Seq {
				seq = r.InstanceSpace[replicaId][d].Seq + 1
			}
			changed = true
			break
		}
	}
	r.sessionConflictsMu.RUnlock()

	// Update seq from maxSeqPerKey for all affected keys
	r.maxSeqPerKeyMu.RLock()
	for i := 0; i < len(cmds); i++ {
		s, present := r.maxSeqPerKey[cmds[i].K]
		if present && seq <= s {
			changed = true
			seq = s + 1
		}
	}
	r.maxSeqPerKeyMu.RUnlock()

	return seq, deps, cl, changed
}

// updateStrongAttributes2 refines dependencies for strong commands on the follower side.
// Similar to updateStrongAttributes1 but without session conflict tracking.
func (r *Replica) updateStrongAttributes2(cmds []state.Command, seq int32, deps []int32, cl []int32, replicaId int32, instance int32) (int32, []int32, []int32, bool) {
	changed := false

	r.conflictMutex.RLock()
	for q := 0; q < r.N; q++ {
		if r.Id != replicaId && int32(q) == replicaId {
			continue
		}
		for i := 0; i < len(cmds); i++ {
			d, present := r.conflicts[q][cmds[i].K]
			if present && d > deps[q] {
				deps[q] = d
				if r.InstanceSpace[q][d] != nil && len(r.InstanceSpace[q][d].Cmds) > i {
					cl[q] = int32(r.InstanceSpace[q][d].Cmds[i].CL)
				}
				if r.InstanceSpace[q][d] != nil && seq <= r.InstanceSpace[q][d].Seq {
					seq = r.InstanceSpace[q][d].Seq + 1
				}
				changed = true
				break
			}
		}
	}
	r.conflictMutex.RUnlock()

	r.maxSeqPerKeyMu.RLock()
	for i := 0; i < len(cmds); i++ {
		s, present := r.maxSeqPerKey[cmds[i].K]
		if present && seq <= s {
			changed = true
			seq = s + 1
		}
	}
	r.maxSeqPerKeyMu.RUnlock()

	return seq, deps, cl, changed
}

// mergeStrongAttributes merges seq and deps from two sources, picking the max for each.
// Returns the merged seq, deps, cl, and whether they were equal.
func (r *Replica) mergeStrongAttributes(seq1 int32, deps1 []int32, seq2 int32, deps2 []int32, cl1 []int32, cl2 []int32) (int32, []int32, []int32, bool) {
	equal := true
	if seq1 != seq2 {
		equal = false
		if seq2 > seq1 {
			seq1 = seq2
		}
	}
	for q := 0; q < r.N; q++ {
		if int32(q) == r.Id {
			continue
		}
		if deps1[q] != deps2[q] {
			equal = false
			if deps2[q] > deps1[q] {
				deps1[q] = deps2[q]
				cl1[q] = cl2[q]
			}
		}
	}
	return seq1, deps1, cl1, equal
}

// equalDeps checks if two dependency arrays are equal.
func equalDeps(deps1 []int32, deps2 []int32) bool {
	for i := 0; i < len(deps1); i++ {
		if deps1[i] != deps2[i] {
			return false
		}
	}
	return true
}

func (r *Replica) handlePrepareReply(preply *PrepareReply) {
	inst := r.InstanceSpace[preply.Replica][preply.Instance]

	if inst.lb == nil || !inst.lb.preparing {
		// Delayed reply — we've moved on.
		return
	}

	if preply.OK == FALSE {
		inst.lb.nacks++
		return
	}

	inst.lb.prepareOKs++

	// If a replica already committed/executed this instance, we can commit directly.
	if preply.Command != nil && len(preply.Command) > 0 {
		if preply.Command[0].CL == state.STRONG &&
			(preply.Status == STRONGLY_COMMITTED || preply.Status == EXECUTED || preply.Status == DISCARDED) {
			r.InstanceSpace[preply.Replica][preply.Instance] = &Instance{
				Cmds:       preply.Command,
				bal:        inst.bal,
				vbal:       inst.bal,
				Status:     STRONGLY_COMMITTED,
				State:      READY,
				Seq:        preply.Seq,
				Deps:       preply.Deps,
				CL:         preply.CL,
				instanceId: &instanceId{preply.Replica, preply.Instance},
			}
			r.bcastStrongCommit(preply.Replica, preply.Instance, preply.Command, preply.Seq, preply.Deps, preply.CL, state.STRONG)
			return
		} else if preply.Command[0].CL == state.CAUSAL &&
			(preply.Status == CAUSALLY_COMMITTED || preply.Status == EXECUTED || preply.Status == DISCARDED) {
			r.InstanceSpace[preply.Replica][preply.Instance] = &Instance{
				Cmds:       preply.Command,
				bal:        inst.bal,
				vbal:       inst.bal,
				Status:     CAUSALLY_COMMITTED,
				State:      READY,
				Seq:        preply.Seq,
				Deps:       preply.Deps,
				CL:         preply.CL,
				instanceId: &instanceId{preply.Replica, preply.Instance},
			}
			r.bcastCausalCommit(preply.Replica, preply.Instance, preply.Command, preply.Seq, preply.Deps, preply.CL, state.CAUSAL)
			return
		}
	} else if preply.Status == STRONGLY_COMMITTED || preply.Status == EXECUTED || preply.Status == DISCARDED {
		r.InstanceSpace[preply.Replica][preply.Instance] = &Instance{
			Cmds:       preply.Command,
			bal:        inst.bal,
			vbal:       inst.bal,
			Status:     STRONGLY_COMMITTED,
			State:      READY,
			Seq:        preply.Seq,
			Deps:       preply.Deps,
			CL:         preply.CL,
			instanceId: &instanceId{preply.Replica, preply.Instance},
		}
		r.bcastStrongCommit(preply.Replica, preply.Instance, preply.Command, preply.Seq, preply.Deps, preply.CL, state.STRONG)
		return
	} else if preply.Status == CAUSALLY_COMMITTED {
		r.InstanceSpace[preply.Replica][preply.Instance] = &Instance{
			Cmds:       preply.Command,
			bal:        inst.bal,
			vbal:       inst.bal,
			Status:     CAUSALLY_COMMITTED,
			State:      READY,
			Seq:        preply.Seq,
			Deps:       preply.Deps,
			CL:         preply.CL,
			instanceId: &instanceId{preply.Replica, preply.Instance},
		}
		r.bcastCausalCommit(preply.Replica, preply.Instance, preply.Command, preply.Seq, preply.Deps, preply.CL, state.CAUSAL)
		return
	}

	// Track ACCEPTED replies — take the one with the highest ballot.
	if preply.Status == ACCEPTED {
		if inst.lb.recoveryInst == nil || inst.lb.maxRecvBallot < preply.VBal {
			inst.lb.recoveryInst = &RecoveryInstance{
				cmds:   preply.Command,
				status: preply.Status,
				seq:    preply.Seq,
				deps:   preply.Deps,
				cl:     preply.CL,
			}
			inst.lb.maxRecvBallot = preply.VBal
		}
	}

	// Track PREACCEPTED replies — count matching attributes.
	if (preply.Status == PREACCEPTED || preply.Status == PREACCEPTED_EQ) &&
		(inst.lb.recoveryInst == nil || inst.lb.recoveryInst.status < ACCEPTED) {
		if inst.lb.recoveryInst == nil {
			inst.lb.recoveryInst = &RecoveryInstance{
				cmds:           preply.Command,
				status:         preply.Status,
				seq:            preply.Seq,
				deps:           preply.Deps,
				cl:             preply.CL,
				preAcceptCount: 1,
			}
		} else if preply.Seq == inst.Seq && equalDeps(preply.Deps, inst.Deps) {
			inst.lb.recoveryInst.preAcceptCount++
		} else if preply.Status == PREACCEPTED_EQ {
			// Different attributes but PREACCEPTED_EQ: take these (they agreed with original leader).
			inst.lb.recoveryInst = &RecoveryInstance{
				cmds:           preply.Command,
				status:         preply.Status,
				seq:            preply.Seq,
				deps:           preply.Deps,
				cl:             preply.CL,
				preAcceptCount: 1,
			}
		}
		if preply.AcceptorId == preply.Replica {
			// Reply from the initial command leader — safe to restart phase 1.
			inst.lb.recoveryInst.leaderResponded = true
			return
		}
	}

	if inst.lb.prepareOKs < r.N/2+1 {
		return
	}

	// Received Prepare replies from a majority.
	ir := inst.lb.recoveryInst

	if ir != nil {
		// At least one replica has (pre-)accepted this instance.
		if ir.status == ACCEPTED ||
			(!ir.leaderResponded && ir.preAcceptCount >= r.N/2 && (r.Thrifty || ir.status == PREACCEPTED_EQ)) {
			// Safe to go to Accept phase.
			inst.Cmds = ir.cmds
			inst.Seq = ir.seq
			inst.Deps = ir.deps
			inst.CL = ir.cl
			inst.vbal = inst.bal
			inst.Status = ACCEPTED
			inst.State = READY
			inst.lb.preparing = false
			r.bcastAccept(preply.Replica, preply.Instance, inst.bal, int32(len(inst.Cmds)), inst.Seq, inst.Deps, inst.CL)
		} else if !ir.leaderResponded && ir.preAcceptCount >= (r.N/2+1)/2 {
			// Send TryPreAccepts — but first try to pre-accept locally.
			inst.lb.preAcceptOKs = 0
			inst.lb.nacks = 0
			inst.lb.possibleQuorum = make([]bool, r.N)
			for q := 0; q < r.N; q++ {
				inst.lb.possibleQuorum[q] = true
			}
			if conf, q, i := r.findPreAcceptConflicts(ir.cmds, preply.Replica, preply.Instance, ir.seq, ir.deps); conf {
				if r.InstanceSpace[q][i].Status > CAUSALLY_COMMITTED {
					// Restart Phase 1.
					inst.lb.preparing = false
					r.startStrongCommit(preply.Replica, preply.Instance, inst.bal, inst.lb.clientProposals, ir.cmds)
					return
				} else {
					inst.lb.nacks = 1
					inst.lb.possibleQuorum[r.Id] = false
				}
			} else {
				inst.Cmds = ir.cmds
				inst.Seq = ir.seq
				inst.Deps = ir.deps
				inst.vbal = inst.bal
				inst.CL = ir.cl
				inst.Status = PREACCEPTED
				inst.State = READY
				inst.lb.preAcceptOKs = 1
			}
			inst.lb.preparing = false
			inst.lb.tryingToPreAccept = true
			r.bcastTryPreAccept(preply.Replica, preply.Instance, inst.bal, inst.Cmds, inst.Seq, inst.Deps, inst.CL)
		} else {
			// Restart Phase 1.
			inst.lb.preparing = false
			r.startStrongCommit(preply.Replica, preply.Instance, inst.bal, inst.lb.clientProposals, ir.cmds)
		}
	} else {
		// No recovery instance — propose NO-OP.
		noopDeps := make([]int32, r.N)
		noopCL := make([]int32, r.N)
		noopDeps[preply.Replica] = preply.Instance - 1
		inst.lb.preparing = false
		r.InstanceSpace[preply.Replica][preply.Instance] = &Instance{
			Cmds:       nil,
			bal:        inst.bal,
			vbal:       inst.bal,
			Status:     ACCEPTED,
			State:      READY,
			Seq:        0,
			Deps:       noopDeps,
			CL:         noopCL,
			lb:         inst.lb,
			instanceId: &instanceId{preply.Replica, preply.Instance},
		}
		r.bcastAccept(preply.Replica, preply.Instance, inst.bal, 0, 0, noopDeps, noopCL)
	}
}

// handlePreAcceptReply processes a PreAcceptReply from a follower (leader side).
// Merges attributes and determines whether to commit on the fast path or
// fall through to the slow path (Accept phase).
func (r *Replica) handlePreAcceptReply(pareply *PreAcceptReply) {
	inst := r.InstanceSpace[pareply.Replica][pareply.Instance]

	if inst.Status != PREACCEPTED {
		// Already moved past PreAccept phase — delayed reply
		return
	}

	if pareply.OK == FALSE {
		// Another leader is active — nack handling
		inst.lb.nacks++
		if pareply.Ballot > inst.lb.maxRecvBallot {
			inst.lb.maxRecvBallot = pareply.Ballot
		}
		if inst.lb.nacks >= r.N/2 {
			inst.bal = r.makeBallotLargerThan(pareply.Ballot)
			r.bcastPrepare(pareply.Replica, pareply.Instance, inst.bal)
		}
		return
	}

	if inst.bal != pareply.Ballot {
		return
	}

	inst.lb.preAcceptOKs++

	var equal bool
	inst.Seq, inst.Deps, inst.CL, equal = r.mergeStrongAttributes(inst.Seq, inst.Deps, pareply.Seq, pareply.Deps, inst.CL, pareply.CL)

	if (r.N <= 3 && !r.Thrifty) || inst.lb.preAcceptOKs > 1 {
		inst.lb.allEqual = inst.lb.allEqual && equal
		if !equal {
			r.Stats.M["conflicted"]++
		}
	}

	allCommitted := true
	for q := 0; q < r.N; q++ {
		if inst.lb.committedDeps[q] < pareply.CommittedDeps[q] {
			inst.lb.committedDeps[q] = pareply.CommittedDeps[q]
		}
		if inst.lb.committedDeps[q] < r.CommittedUpTo[q] {
			inst.lb.committedDeps[q] = r.CommittedUpTo[q]
		}
		if inst.lb.committedDeps[q] < inst.Deps[q] {
			allCommitted = false
		}
	}

	// Fast path: all peers agreed on same attributes, all deps committed, initial ballot
	if inst.lb.preAcceptOKs >= r.FastQuorumSize()-1 && inst.lb.allEqual && allCommitted && isInitialBallot(inst.bal) {
		r.Stats.M["fast"]++

		r.InstanceSpace[pareply.Replica][pareply.Instance].Status = STRONGLY_COMMITTED
		r.updateCommitted(pareply.Replica)
		r.updateStrongSessionConflict(inst.Cmds, pareply.Replica, pareply.Instance)

		if inst.lb.clientProposals != nil && !r.Dreply {
			r.outstandingStrong -= int64(len(inst.lb.clientProposals))
			for i := 0; i < len(inst.lb.clientProposals); i++ {
				prop := inst.lb.clientProposals[i]
				r.clientReplyCount++
				r.ReplyProposeTS(
					&defs.ProposeReplyTS{
						OK:        TRUE,
						CommandId: prop.CommandId,
						Value:     state.NIL(),
						Timestamp: prop.Timestamp,
					},
					prop.Reply,
					prop.Mutex)
			}
		}

		r.recordInstanceMetadata(inst)
		r.sync()

		r.InstanceSpace[pareply.Replica][pareply.Instance].State = READY
		r.bcastStrongCommit(pareply.Replica, pareply.Instance, inst.Cmds, inst.Seq, inst.Deps, inst.CL, state.STRONG)

	} else if inst.lb.preAcceptOKs >= r.N/2 && (!inst.lb.allEqual || !allCommitted || !isInitialBallot(inst.bal)) {
		// Slow path: move to Accept phase (only when fast path is provably impossible)
		r.Stats.M["slow"]++
		if !allCommitted {
			r.Stats.M["weird"]++
		}

		inst.Status = ACCEPTED
		r.InstanceSpace[pareply.Replica][pareply.Instance].State = READY
		r.bcastAccept(pareply.Replica, pareply.Instance, inst.bal, int32(len(inst.Cmds)), inst.Seq, inst.Deps, inst.CL)
	}
}

// handlePreAcceptOK processes a fast-path PreAcceptOK from a follower (leader side).
// PreAcceptOK indicates the follower's attributes matched exactly — no merge needed.
func (r *Replica) handlePreAcceptOK(pareply *PreAcceptOK) {
	inst := r.InstanceSpace[r.Id][pareply.Instance]

	if inst.Status != PREACCEPTED {
		return
	}

	if !isInitialBallot(inst.bal) {
		return
	}

	inst.lb.preAcceptOKs++

	allCommitted := true
	for q := 0; q < r.N; q++ {
		if inst.lb.committedDeps[q] < inst.lb.originalDeps[q] {
			inst.lb.committedDeps[q] = inst.lb.originalDeps[q]
		}
		if inst.lb.committedDeps[q] < r.CommittedUpTo[q] {
			inst.lb.committedDeps[q] = r.CommittedUpTo[q]
		}
		if inst.lb.committedDeps[q] < inst.Deps[q] {
			allCommitted = false
		}
	}

	// Fast path commit check
	if inst.lb.preAcceptOKs >= r.FastQuorumSize()-1 && inst.lb.allEqual && allCommitted && isInitialBallot(inst.bal) {
		r.Stats.M["fast"]++

		r.InstanceSpace[r.Id][pareply.Instance].Status = STRONGLY_COMMITTED
		r.updateCommitted(r.Id)
		r.updateStrongSessionConflict(inst.Cmds, r.Id, pareply.Instance)

		if inst.lb.clientProposals != nil && !r.Dreply {
			r.outstandingStrong -= int64(len(inst.lb.clientProposals))
			for i := 0; i < len(inst.lb.clientProposals); i++ {
				prop := inst.lb.clientProposals[i]
				r.clientReplyCount++
				r.ReplyProposeTS(
					&defs.ProposeReplyTS{
						OK:        TRUE,
						CommandId: prop.CommandId,
						Value:     state.NIL(),
						Timestamp: prop.Timestamp,
					},
					prop.Reply,
					prop.Mutex)
			}
		}

		r.recordInstanceMetadata(inst)
		r.sync()

		r.bcastStrongCommit(r.Id, pareply.Instance, inst.Cmds, inst.Seq, inst.Deps, inst.CL, state.STRONG)

	} else if inst.lb.preAcceptOKs >= r.N/2 && (!inst.lb.allEqual || !allCommitted || !isInitialBallot(inst.bal)) {
		// Slow path (only when fast path is provably impossible)
		r.Stats.M["slow"]++
		inst.Status = ACCEPTED
		r.bcastAccept(r.Id, pareply.Instance, inst.bal, int32(len(inst.Cmds)), inst.Seq, inst.Deps, inst.CL)
	}
}

// handleAcceptReply processes an AcceptReply from a follower (leader side).
// When quorum reached, commits the instance and broadcasts strong commit.
func (r *Replica) handleAcceptReply(areply *AcceptReply) {
	inst := r.InstanceSpace[areply.Replica][areply.Instance]

	if inst.Status != ACCEPTED {
		// Already moved past Accept phase — delayed reply
		return
	}

	if areply.OK == FALSE {
		inst.lb.nacks++
		if areply.Ballot > inst.lb.maxRecvBallot {
			inst.lb.maxRecvBallot = areply.Ballot
		}
		return
	}

	if inst.bal != areply.Ballot {
		return
	}

	inst.lb.acceptOKs++

	if inst.lb.acceptOKs+1 > r.N/2 {
		r.InstanceSpace[areply.Replica][areply.Instance].Status = STRONGLY_COMMITTED

		r.updateCommitted(areply.Replica)
		r.updateStrongSessionConflict(inst.Cmds, areply.Replica, areply.Instance)

		if inst.lb.clientProposals != nil && !r.Dreply {
			r.outstandingStrong -= int64(len(inst.lb.clientProposals))
			for i := 0; i < len(inst.lb.clientProposals); i++ {
				prop := inst.lb.clientProposals[i]
				r.clientReplyCount++
				r.ReplyProposeTS(
					&defs.ProposeReplyTS{
						OK:        TRUE,
						CommandId: prop.CommandId,
						Value:     state.NIL(),
						Timestamp: prop.Timestamp,
					},
					prop.Reply,
					prop.Mutex)
			}
		}

		r.recordInstanceMetadata(inst)
		r.sync()

		r.bcastStrongCommit(areply.Replica, areply.Instance, inst.Cmds, inst.Seq, inst.Deps, inst.CL, state.STRONG)
	}
}

func (r *Replica) handleTryPreAccept(tpa *TryPreAccept) {
	inst := r.InstanceSpace[tpa.Replica][tpa.Instance]

	if inst != nil && inst.bal > tpa.Ballot {
		// Ballot number too small.
		r.replyTryPreAccept(tpa.LeaderId, &TryPreAcceptReply{
			AcceptorId:       r.Id,
			Replica:          tpa.Replica,
			Instance:         tpa.Instance,
			OK:               FALSE,
			Ballot:           inst.bal,
			ConflictReplica:  tpa.Replica,
			ConflictInstance: tpa.Instance,
			ConflictStatus:   inst.Status,
		})
		return
	}

	if conflict, confRep, confInst := r.findPreAcceptConflicts(tpa.Command, tpa.Replica, tpa.Instance, tpa.Seq, tpa.Deps); conflict {
		// Conflict found — reject.
		conflictBal := int32(0)
		if inst != nil {
			conflictBal = inst.bal
		}
		r.replyTryPreAccept(tpa.LeaderId, &TryPreAcceptReply{
			AcceptorId:       r.Id,
			Replica:          tpa.Replica,
			Instance:         tpa.Instance,
			OK:               FALSE,
			Ballot:           conflictBal,
			ConflictReplica:  confRep,
			ConflictInstance: confInst,
			ConflictStatus:   r.InstanceSpace[confRep][confInst].Status,
		})
	} else {
		// No conflict — can pre-accept.
		if tpa.Instance >= r.crtInstance[tpa.Replica] {
			r.crtInstance[tpa.Replica] = tpa.Instance + 1
		}
		if inst != nil {
			inst.Cmds = tpa.Command
			inst.Deps = tpa.Deps
			inst.CL = tpa.CL
			inst.Seq = tpa.Seq
			inst.Status = PREACCEPTED
			inst.State = READY
			inst.bal = tpa.Ballot
			inst.vbal = tpa.Ballot
		} else {
			r.InstanceSpace[tpa.Replica][tpa.Instance] = &Instance{
				Cmds:       tpa.Command,
				bal:        tpa.Ballot,
				vbal:       tpa.Ballot,
				Status:     PREACCEPTED,
				State:      READY,
				Seq:        tpa.Seq,
				Deps:       tpa.Deps,
				CL:         tpa.CL,
				instanceId: &instanceId{tpa.Replica, tpa.Instance},
			}
			inst = r.InstanceSpace[tpa.Replica][tpa.Instance]
		}
		r.replyTryPreAccept(tpa.LeaderId, &TryPreAcceptReply{
			AcceptorId: r.Id,
			Replica:    tpa.Replica,
			Instance:   tpa.Instance,
			OK:         TRUE,
			Ballot:     inst.bal,
		})
	}
}

func (r *Replica) handleTryPreAcceptReply(tpar *TryPreAcceptReply) {
	inst := r.InstanceSpace[tpar.Replica][tpar.Instance]
	if inst == nil || inst.lb == nil || !inst.lb.tryingToPreAccept || inst.lb.recoveryInst == nil {
		return
	}

	ir := inst.lb.recoveryInst

	if tpar.OK == TRUE {
		inst.lb.preAcceptOKs++
		inst.lb.tpaOKs++
		if inst.lb.preAcceptOKs >= r.N/2 {
			// Safe to start Accept phase.
			inst.Cmds = ir.cmds
			inst.Seq = ir.seq
			inst.Deps = ir.deps
			inst.CL = ir.cl
			inst.Status = ACCEPTED
			inst.State = READY
			inst.lb.tryingToPreAccept = false
			inst.lb.acceptOKs = 0
			r.bcastAccept(tpar.Replica, tpar.Instance, inst.bal, int32(len(inst.Cmds)), inst.Seq, inst.Deps, inst.CL)
			return
		}
	} else {
		inst.lb.nacks++
		if tpar.Ballot > inst.bal {
			// Higher ballot seen — should retry later.
			return
		}
		inst.lb.tpaOKs++
		if tpar.ConflictReplica == tpar.Replica && tpar.ConflictInstance == tpar.Instance {
			// Conflict with the same instance — re-run prepare.
			inst.lb.tryingToPreAccept = false
			return
		}
		inst.lb.possibleQuorum[tpar.AcceptorId] = false
		inst.lb.possibleQuorum[tpar.ConflictReplica] = false
		notInQuorum := 0
		for q := 0; q < r.N; q++ {
			if !inst.lb.possibleQuorum[q] {
				notInQuorum++
			}
		}
		if tpar.ConflictStatus >= STRONGLY_COMMITTED || notInQuorum > r.N/2 {
			// Abandon recovery, restart from Phase 1.
			inst.lb.tryingToPreAccept = false
			r.startStrongCommit(tpar.Replica, tpar.Instance, inst.bal, inst.lb.clientProposals, ir.cmds)
		}
		if inst.lb.tpaOKs >= r.N/2 {
			// Defer recovery.
			inst.lb.tryingToPreAccept = false
		}
	}
}

// retryStuckInstances scans this replica's own instance space for instances
// stuck in PREACCEPTED or ACCEPTED state and re-broadcasts the corresponding
// message. This provides liveness when the original broadcast's responses
// were lost or when peers silently dropped the message (e.g., because they
// already had a later status for the instance due to message reordering).
func (r *Replica) retryStuckInstances() {
	retried := 0
	from := r.CommittedUpTo[r.Id] + 1
	to := r.crtInstance[r.Id]
	for i := from; i < to; i++ {
		inst := r.InstanceSpace[r.Id][i]
		if inst == nil || inst.lb == nil {
			continue
		}
		if inst.Status == PREACCEPTED || inst.Status == PREACCEPTED_EQ {
			r.bcastPreAccept(r.Id, i, inst.bal, inst.Cmds, inst.Seq, inst.Deps, inst.CL)
			retried++
		} else if inst.Status == ACCEPTED {
			r.bcastAccept(r.Id, i, inst.bal, int32(len(inst.Cmds)), inst.Seq, inst.Deps, inst.CL)
			retried++
		}
	}
	if retried > 0 {
		r.Printf("retryStuckInstances: retried %d instances (range %d-%d)", retried, from, to-1)
	}
}

// startRecoveryForInstance initiates recovery for a stalled instance by
// incrementing the ballot and broadcasting Prepare messages.
func (r *Replica) startRecoveryForInstance(replicaId int32, instance int32) {
	nildeps := make([]int32, r.N)
	nilcl := make([]int32, r.N)

	if r.InstanceSpace[replicaId][instance] == nil {
		r.InstanceSpace[replicaId][instance] = &Instance{
			Status:     NONE,
			State:      READY,
			Deps:       nildeps,
			CL:         nilcl,
			instanceId: &instanceId{replicaId, instance},
		}
	}

	inst := r.InstanceSpace[replicaId][instance]

	if inst.lb == nil {
		inst.lb = &LeaderBookkeeping{
			maxRecvBallot: -1,
			preparing:     true,
		}
	} else {
		inst.lb = &LeaderBookkeeping{
			clientProposals: inst.lb.clientProposals,
			maxRecvBallot:   -1,
			preparing:       true,
		}
	}

	if inst.Status == ACCEPTED {
		inst.lb.recoveryInst = &RecoveryInstance{
			cmds:   inst.Cmds,
			status: inst.Status,
			seq:    inst.Seq,
			deps:   inst.Deps,
			cl:     inst.CL,
		}
		inst.lb.maxRecvBallot = inst.bal
	} else if inst.Status >= PREACCEPTED {
		inst.lb.recoveryInst = &RecoveryInstance{
			cmds:            inst.Cmds,
			status:          inst.Status,
			seq:             inst.Seq,
			deps:            inst.Deps,
			cl:              inst.CL,
			preAcceptCount:  1,
			leaderResponded: (r.Id == replicaId),
		}
	}

	inst.bal = r.makeBallotLargerThan(inst.bal)

	r.bcastPrepare(replicaId, instance, inst.bal)
}

// bcastTryPreAccept broadcasts a TryPreAccept message to all alive peers.
func (r *Replica) bcastTryPreAccept(replicaId int32, instance int32, ballot int32, cmds []state.Command, seq int32, deps []int32, cl []int32) {
	defer func() {
		if err := recover(); err != nil {
			dlog.Println("TryPreAccept bcast failed:", err)
		}
	}()

	args := &TryPreAccept{
		LeaderId: r.Id,
		Replica:  replicaId,
		Instance: instance,
		Ballot:   ballot,
		Command:  cmds,
		Seq:      seq,
		CL:       cl,
		Deps:     deps,
	}

	for q := int32(0); q < int32(r.N); q++ {
		if q == r.Id {
			continue
		}
		if !r.Alive[q] {
			continue
		}
		r.SendMsg(q, r.tryPreAcceptRPC, args)
	}
}

// findPreAcceptConflicts checks if the given instance's attributes conflict with
// any known instance. Used during recovery (TryPreAccept phase).
// Returns: (conflict found, conflicting replica, conflicting instance)
func (r *Replica) findPreAcceptConflicts(cmds []state.Command, replicaId int32, instance int32, seq int32, deps []int32) (bool, int32, int32) {
	inst := r.InstanceSpace[replicaId][instance]
	if inst != nil && len(inst.Cmds) > 0 {
		if inst.Status >= ACCEPTED {
			return true, replicaId, instance
		}
		if inst.Seq == seq && equalDeps(inst.Deps, deps) {
			return false, replicaId, instance
		}
	}
	for q := int32(0); q < int32(r.N); q++ {
		for i := r.ExecedUpTo[q]; i < r.crtInstance[q]; i++ {
			if replicaId == q && instance == i {
				break
			}
			if i == deps[q] {
				continue
			}
			other := r.InstanceSpace[q][i]
			if other == nil || other.Cmds == nil || len(other.Cmds) == 0 {
				continue
			}
			if other.Deps[replicaId] >= instance {
				continue
			}
			if state.ConflictBatch(other.Cmds, cmds) {
				if i > deps[q] ||
					(i < deps[q] && other.Seq >= seq && (q != replicaId || other.Status > PREACCEPTED_EQ)) {
					return true, q, i
				}
			}
		}
	}
	return false, -1, -1
}

// executeCommands is implemented in exec.go
