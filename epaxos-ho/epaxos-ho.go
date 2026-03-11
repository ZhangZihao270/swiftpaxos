package epaxosho

import (
	"encoding/binary"
	"math/rand"
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

// --- Protocol handlers ---

func (r *Replica) handlePropose(propose *defs.GPropose) {
	batchSize := len(r.ProposeChan) + 1

	// Separate proposals into causal and strong batches based on CL field
	causalCmds := make([]state.Command, 0, batchSize)
	causalProposals := make([]*defs.GPropose, 0, batchSize)
	strongCmds := make([]state.Command, 0, batchSize)
	strongProposals := make([]*defs.GPropose, 0, batchSize)

	// Classify the first proposal
	switch propose.Command.CL {
	case state.CAUSAL:
		causalCmds = append(causalCmds, propose.Command)
		causalProposals = append(causalProposals, propose)
	case state.STRONG:
		strongCmds = append(strongCmds, propose.Command)
		strongProposals = append(strongProposals, propose)
	default:
		// Default to strong for safety (unknown CL)
		strongCmds = append(strongCmds, propose.Command)
		strongProposals = append(strongProposals, propose)
	}

	// Drain remaining proposals from the channel
	for i := 1; i < batchSize; i++ {
		prop := <-r.ProposeChan
		switch prop.Command.CL {
		case state.CAUSAL:
			causalCmds = append(causalCmds, prop.Command)
			causalProposals = append(causalProposals, prop)
		case state.STRONG:
			strongCmds = append(strongCmds, prop.Command)
			strongProposals = append(strongProposals, prop)
		default:
			strongCmds = append(strongCmds, prop.Command)
			strongProposals = append(strongProposals, prop)
		}
	}

	// Start causal commit if we have causal commands
	if len(causalCmds) > 0 {
		instNo := r.crtInstance[r.Id]
		r.crtInstance[r.Id]++
		r.startCausalCommit(r.Id, instNo, 0, causalProposals, causalCmds)
	}

	// Start strong commit if we have strong commands
	if len(strongCmds) > 0 {
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

// startStrongCommit initiates the EPaxos-style 2-RTT commit for strong commands.
// Strong commands go through PreAccept → Accept → Commit phases.
func (r *Replica) startStrongCommit(replicaId int32, instance int32, ballot int32, proposals []*defs.GPropose, cmds []state.Command) {
	// TODO: Phase 99.3f — strong dependency computation + PreAccept broadcast
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
	for i := 0; i < len(cmds); i++ {
		r.conflictMutex.Lock()
		if d, present := r.conflicts[replicaId][cmds[i].K]; !present || d < instance {
			r.conflicts[replicaId][cmds[i].K] = instance
		}
		r.conflictMutex.Unlock()

		r.maxSeqPerKeyMu.Lock()
		if s, present := r.maxSeqPerKey[cmds[i].K]; !present || s < seq {
			r.maxSeqPerKey[cmds[i].K] = seq
		}
		r.maxSeqPerKeyMu.Unlock()
	}

	if includeSession {
		for i := 0; i < len(cmds); i++ {
			sid := cmds[i].Sid
			r.sessionConflictsMu.Lock()
			if d, present := r.sessionConflicts[replicaId][sid]; !present || d < instance {
				r.sessionConflicts[replicaId][sid] = instance
			}
			r.sessionConflictsMu.Unlock()
		}
	}
}

// updateCausalAttributes computes causal dependencies for a batch of commands.
// It tracks: (1) session dependencies, (2) read-from dependencies (GET reads from latest PUT),
// (3) max sequence per key for causal ordering.
func (r *Replica) updateCausalAttributes(cmds []state.Command, seq int32, deps []int32, cl []int32, replicaId int32, instance int32) (int32, []int32, []int32) {
	// Track session dependency: find the latest committed command from the same session
	for i := 0; i < len(cmds); i++ {
		r.sessionConflictsMu.RLock()
		d, present := r.sessionConflicts[replicaId][cmds[i].Sid]
		r.sessionConflictsMu.RUnlock()

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

	// Track read-from dependency: GET commands depend on the latest write to that key
	for i := 0; i < len(cmds); i++ {
		if cmds[i].Op == state.GET {
			r.maxWriteInstancePerKeyMu.RLock()
			d, present := r.maxWriteInstancePerKey[cmds[i].K]
			r.maxWriteInstancePerKeyMu.RUnlock()
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

	// Update seq from maxSeqPerKey for all affected keys
	for i := 0; i < len(cmds); i++ {
		r.maxSeqPerKeyMu.RLock()
		s, present := r.maxSeqPerKey[cmds[i].K]
		r.maxSeqPerKeyMu.RUnlock()
		if present && seq <= s {
			seq = s + 1
		}
	}

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
	for q := 0; q < r.N-1; q++ {
		r.SendMsg(r.PreferredPeerOrder[q], r.causalCommitRPC[rand.Intn(r.N*NO_CAUSAL_CHANNEL)], args)
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
	for i := 0; i < len(cmds); i++ {
		r.conflictMutex.Lock()
		if d, present := r.conflicts[replicaId][cmds[i].K]; !present || d < instance {
			r.conflicts[replicaId][cmds[i].K] = instance
		}
		r.conflictMutex.Unlock()

		r.maxSeqPerKeyMu.Lock()
		if s, present := r.maxSeqPerKey[cmds[i].K]; !present || s < seq {
			r.maxSeqPerKey[cmds[i].K] = seq
		}
		r.maxSeqPerKeyMu.Unlock()
	}
}

// updateStrongSessionConflict updates session conflict tracking for strong commands.
func (r *Replica) updateStrongSessionConflict(cmds []state.Command, replicaId int32, instance int32) {
	for i := 0; i < len(cmds); i++ {
		sid := cmds[i].Sid
		r.sessionConflictsMu.Lock()
		if d, present := r.sessionConflicts[replicaId][sid]; !present || d < instance {
			r.sessionConflicts[replicaId][sid] = instance
		}
		r.sessionConflictsMu.Unlock()
	}
}

// updateStrongAttributes1 computes initial dependencies for strong commands.
// It checks per-key conflicts across all replicas, session conflicts, and max seq per key.
// Returns: updated seq, deps, cl, and whether any dependency changed.
func (r *Replica) updateStrongAttributes1(cmds []state.Command, seq int32, deps []int32, cl []int32, replicaId int32, instance int32) (int32, []int32, []int32, bool) {
	changed := false

	// Check per-key conflicts across all replicas
	for q := 0; q < r.N; q++ {
		if r.Id != replicaId && int32(q) == replicaId {
			continue
		}
		for i := 0; i < len(cmds); i++ {
			r.conflictMutex.RLock()
			d, present := r.conflicts[q][cmds[i].K]
			r.conflictMutex.RUnlock()

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

	// Track session dependency
	for i := 0; i < len(cmds); i++ {
		r.sessionConflictsMu.RLock()
		d, present := r.sessionConflicts[replicaId][cmds[i].Sid]
		r.sessionConflictsMu.RUnlock()

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

	// Update seq from maxSeqPerKey for all affected keys
	for i := 0; i < len(cmds); i++ {
		r.maxSeqPerKeyMu.RLock()
		s, present := r.maxSeqPerKey[cmds[i].K]
		r.maxSeqPerKeyMu.RUnlock()
		if present && seq <= s {
			changed = true
			seq = s + 1
		}
	}

	return seq, deps, cl, changed
}

// updateStrongAttributes2 refines dependencies for strong commands on the follower side.
// Similar to updateStrongAttributes1 but without session conflict tracking.
func (r *Replica) updateStrongAttributes2(cmds []state.Command, seq int32, deps []int32, cl []int32, replicaId int32, instance int32) (int32, []int32, []int32, bool) {
	changed := false

	for q := 0; q < r.N; q++ {
		if r.Id != replicaId && int32(q) == replicaId {
			continue
		}
		for i := 0; i < len(cmds); i++ {
			r.conflictMutex.RLock()
			d, present := r.conflicts[q][cmds[i].K]
			r.conflictMutex.RUnlock()

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

	for i := 0; i < len(cmds); i++ {
		r.maxSeqPerKeyMu.RLock()
		s, present := r.maxSeqPerKey[cmds[i].K]
		r.maxSeqPerKeyMu.RUnlock()
		if present && seq <= s {
			changed = true
			seq = s + 1
		}
	}

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

func (r *Replica) handlePrepareReply(reply *PrepareReply) {
	// TODO: Phase 99.3g — recovery path
}

func (r *Replica) handlePreAcceptReply(reply *PreAcceptReply) {
	// TODO: Phase 99.3f-iii — strong commit path
}

func (r *Replica) handlePreAcceptOK(msg *PreAcceptOK) {
	// TODO: Phase 99.3f-iii — fast path acknowledgment
}

func (r *Replica) handleAcceptReply(reply *AcceptReply) {
	// TODO: Phase 99.3f-iv — strong commit path
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
