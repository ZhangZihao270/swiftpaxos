package epaxos

import (
	"encoding/binary"
	"io"
	"sync"
	"time"

	"github.com/imdea-software/swiftpaxos/config"
	"github.com/imdea-software/swiftpaxos/dlog"
	"github.com/imdea-software/swiftpaxos/replica"
	"github.com/imdea-software/swiftpaxos/replica/defs"
	fastrpc "github.com/imdea-software/swiftpaxos/rpc"
	"github.com/imdea-software/swiftpaxos/state"
)

var cpMarker []state.Command
var cpcounter = 0

type Replica struct {
	*replica.Replica
	prepareChan           chan fastrpc.Serializable
	preAcceptChan         chan fastrpc.Serializable
	acceptChan            chan fastrpc.Serializable
	commitChan            chan fastrpc.Serializable
	prepareReplyChan      chan fastrpc.Serializable
	preAcceptReplyChan    chan fastrpc.Serializable
	preAcceptOKChan       chan fastrpc.Serializable
	acceptReplyChan       chan fastrpc.Serializable
	tryPreAcceptChan      chan fastrpc.Serializable
	tryPreAcceptReplyChan chan fastrpc.Serializable
	prepareRPC            uint8
	prepareReplyRPC       uint8
	preAcceptRPC          uint8
	preAcceptReplyRPC     uint8
	acceptRPC             uint8
	acceptReplyRPC        uint8
	commitRPC             uint8
	tryPreAcceptRPC       uint8
	tryPreAcceptReplyRPC  uint8
	InstanceSpace         [][]*Instance
	crtInstance           []int32
	CommittedUpTo         []int32
	ExecedUpTo            []int32
	exec                  *Exec
	conflicts             []map[state.Key]*InstPair
	maxSeqPerKey          map[state.Key]int32
	maxSeq                int32
	latestCPReplica       int32
	latestCPInstance      int32
	clientMutex           *sync.Mutex
	instancesToRecover    chan *InstanceId
	IsLeader              bool
	maxRecvBallot         int32
	batchWait             int
	transconf             bool
}

func New(alias string, id int, peerAddrList []string, exec, beacon, durable bool, batchWait int, transconf bool, failures int, conf *config.Config, logger *dlog.Logger) *Replica {
	r := &Replica{
		Replica:               replica.New(alias, id, failures, peerAddrList, true, exec, false, conf, logger),
		prepareChan:           make(chan fastrpc.Serializable, defs.CHAN_BUFFER_SIZE),
		preAcceptChan:         make(chan fastrpc.Serializable, defs.CHAN_BUFFER_SIZE),
		acceptChan:            make(chan fastrpc.Serializable, defs.CHAN_BUFFER_SIZE),
		commitChan:            make(chan fastrpc.Serializable, defs.CHAN_BUFFER_SIZE),
		prepareReplyChan:      make(chan fastrpc.Serializable, defs.CHAN_BUFFER_SIZE),
		preAcceptReplyChan:    make(chan fastrpc.Serializable, defs.CHAN_BUFFER_SIZE*3),
		preAcceptOKChan:       make(chan fastrpc.Serializable, defs.CHAN_BUFFER_SIZE*3),
		acceptReplyChan:       make(chan fastrpc.Serializable, defs.CHAN_BUFFER_SIZE*2),
		tryPreAcceptChan:      make(chan fastrpc.Serializable, defs.CHAN_BUFFER_SIZE),
		tryPreAcceptReplyChan: make(chan fastrpc.Serializable, defs.CHAN_BUFFER_SIZE),
		InstanceSpace:         make([][]*Instance, len(peerAddrList)),
		crtInstance:           make([]int32, len(peerAddrList)),
		CommittedUpTo:         make([]int32, len(peerAddrList)),
		ExecedUpTo:            make([]int32, len(peerAddrList)),
		conflicts:             make([]map[state.Key]*InstPair, len(peerAddrList)),
		maxSeqPerKey:          make(map[state.Key]int32),
		maxSeq:                0,
		latestCPReplica:       0,
		latestCPInstance:      -1,
		clientMutex:           new(sync.Mutex),
		instancesToRecover:    make(chan *InstanceId, defs.CHAN_BUFFER_SIZE),
		IsLeader:              false,
		maxRecvBallot:         -1,
		batchWait:             batchWait,
		transconf:             transconf,
	}

	r.Beacon = beacon
	r.Durable = durable
	r.Dreply = false

	for i := 0; i < r.N; i++ {
		r.InstanceSpace[i] = make([]*Instance, MAX_INSTANCE)
		r.crtInstance[i] = -1
		r.ExecedUpTo[i] = -1
		r.CommittedUpTo[i] = -1
		r.conflicts[i] = make(map[state.Key]*InstPair, HT_INIT_SIZE)
	}

	r.exec = &Exec{r}

	cpMarker = make([]state.Command, 0)

	r.prepareRPC = r.RPC.Register(new(Prepare), r.prepareChan)
	r.prepareReplyRPC = r.RPC.Register(new(PrepareReply), r.prepareReplyChan)
	r.preAcceptRPC = r.RPC.Register(new(PreAccept), r.preAcceptChan)
	r.preAcceptReplyRPC = r.RPC.Register(new(PreAcceptReply), r.preAcceptReplyChan)
	r.acceptRPC = r.RPC.Register(new(Accept), r.acceptChan)
	r.acceptReplyRPC = r.RPC.Register(new(AcceptReply), r.acceptReplyChan)
	r.commitRPC = r.RPC.Register(new(Commit), r.commitChan)
	r.tryPreAcceptRPC = r.RPC.Register(new(TryPreAccept), r.tryPreAcceptChan)
	r.tryPreAcceptReplyRPC = r.RPC.Register(new(TryPreAcceptReply), r.tryPreAcceptReplyChan)

	r.Stats.M["weird"], r.Stats.M["conflicted"], r.Stats.M["slow"], r.Stats.M["fast"], r.Stats.M["totalCommitTime"], r.Stats.M["totalBatching"], r.Stats.M["totalBatchingSize"] = 0, 0, 0, 0, 0, 0, 0

	go r.run()

	return r
}

func (r *Replica) recordInstanceMetadata(inst *Instance) {
	if !r.Durable {
		return
	}

	b := make([]byte, 9+r.N*4)
	binary.LittleEndian.PutUint32(b[0:4], uint32(inst.Bal))
	binary.LittleEndian.PutUint32(b[0:4], uint32(inst.Vbal))
	b[4] = byte(inst.Status)
	binary.LittleEndian.PutUint32(b[5:9], uint32(inst.Seq))
	l := 9
	for _, dep := range inst.Deps {
		binary.LittleEndian.PutUint32(b[l:l+4], uint32(dep))
		l += 4
	}
	r.StableStore.Write(b[:])
}

func (r *Replica) recordCommands(cmds []state.Command) {
	if !r.Durable {
		return
	}

	if cmds == nil {
		return
	}
	for i := 0; i < len(cmds); i++ {
		cmds[i].Marshal(io.Writer(r.StableStore))
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

func (r *Replica) fastClock() {
	for !r.Shutdown {
		time.Sleep(time.Duration(r.batchWait) * time.Millisecond)
		fastClockChan <- true
	}
}
func (r *Replica) slowClock() {
	for !r.Shutdown {
		time.Sleep(150 * time.Millisecond)
		slowClockChan <- true
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

func (r *Replica) BatchingEnabled() bool {
	return r.batchWait > 0
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

	for !r.Shutdown {

		select {

		case propose := <-onOffProposeChan:
			r.handlePropose(propose)
			if r.BatchingEnabled() {
				onOffProposeChan = nil
			}
			break

		case <-fastClockChan:
			onOffProposeChan = r.ProposeChan
			break

		case prepareS := <-r.prepareChan:
			prepare := prepareS.(*Prepare)
			r.handlePrepare(prepare)
			break

		case preAcceptS := <-r.preAcceptChan:
			preAccept := preAcceptS.(*PreAccept)
			r.handlePreAccept(preAccept)
			break

		case acceptS := <-r.acceptChan:
			accept := acceptS.(*Accept)
			r.handleAccept(accept)
			break

		case commitS := <-r.commitChan:
			commit := commitS.(*Commit)
			r.handleCommit(commit)
			break

		case prepareReplyS := <-r.prepareReplyChan:
			prepareReply := prepareReplyS.(*PrepareReply)
			r.handlePrepareReply(prepareReply)
			break

		case preAcceptReplyS := <-r.preAcceptReplyChan:
			preAcceptReply := preAcceptReplyS.(*PreAcceptReply)
			r.handlePreAcceptReply(preAcceptReply)
			break

		case acceptReplyS := <-r.acceptReplyChan:
			acceptReply := acceptReplyS.(*AcceptReply)
			r.handleAcceptReply(acceptReply)
			break

		case tryPreAcceptS := <-r.tryPreAcceptChan:
			tryPreAccept := tryPreAcceptS.(*TryPreAccept)
			r.handleTryPreAccept(tryPreAccept)
			break

		case tryPreAcceptReplyS := <-r.tryPreAcceptReplyChan:
			tryPreAcceptReply := tryPreAcceptReplyS.(*TryPreAcceptReply)
			r.handleTryPreAcceptReply(tryPreAcceptReply)
			break

		case beacon := <-r.BeaconChan:
			r.ReplyBeacon(beacon)
			break

		case <-slowClockChan:
			if r.Beacon {
				r.Printf("weird %d; conflicted %d; slow %d; fast %d\n", r.Stats.M["weird"], r.Stats.M["conflicted"], r.Stats.M["slow"], r.Stats.M["fast"])
				for q := int32(0); q < int32(r.N); q++ {
					if q == r.Id {
						continue
					}
					r.SendBeacon(q)
				}
			}
			break

		case iid := <-r.instancesToRecover:
			r.startRecoveryForInstance(iid.Replica, iid.Instance)
		}
	}
}

func (r *Replica) executeCommands() {
	const SLEEP_TIME_NS = 1e6
	problemInstance := make([]int32, r.N)
	timeout := make([]uint64, r.N)
	for q := 0; q < r.N; q++ {
		problemInstance[q] = -1
		timeout[q] = 0
	}

	for !r.Shutdown {
		executed := false
		for q := int32(0); q < int32(r.N); q++ {
			for inst := r.ExecedUpTo[q] + 1; inst <= r.crtInstance[q]; inst++ {
				if r.InstanceSpace[q][inst] != nil && r.InstanceSpace[q][inst].Status == EXECUTED {
					if inst == r.ExecedUpTo[q]+1 {
						r.ExecedUpTo[q] = inst
					}
					continue
				}
				if r.InstanceSpace[q][inst] == nil || r.InstanceSpace[q][inst].Status < COMMITTED || r.InstanceSpace[q][inst].Cmds == nil {
					if inst == problemInstance[q] {
						timeout[q] += SLEEP_TIME_NS
						if timeout[q] >= COMMIT_GRACE_PERIOD {
							for k := problemInstance[q]; k <= r.crtInstance[q]; k++ {
								r.instancesToRecover <- &InstanceId{q, k}
							}
							timeout[q] = 0
						}
					} else {
						problemInstance[q] = inst
						timeout[q] = 0
					}
					break
				}
				if ok := r.exec.executeCommand(int32(q), inst); ok {
					executed = true
					if inst == r.ExecedUpTo[q]+1 {
						r.ExecedUpTo[q] = inst
					}
				}
			}
		}
		if !executed {
			r.M.Lock()
			r.M.Unlock()
			time.Sleep(SLEEP_TIME_NS)
		}
	}
}

// Message sending helpers

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

func (r *Replica) bcastPrepare(replicaId int32, instance int32) {
	defer func() {
		if err := recover(); err != nil {
			r.Println("Prepare bcast failed:", err)
		}
	}()
	lb := r.InstanceSpace[replicaId][instance].Lb
	args := &Prepare{r.Id, replicaId, instance, lb.LastTriedBallot}

	n := r.N - 1
	q := r.Id
	for sent := 0; sent < n; {
		q = (q + 1) % int32(r.N)
		if q == r.Id {
			break
		}
		if !r.Alive[q] {
			continue
		}
		r.SendMsg(q, r.prepareRPC, args)
		sent++
	}
}

func (r *Replica) bcastPreAccept(replicaId int32, instance int32) {
	defer func() {
		if err := recover(); err != nil {
			r.Println("PreAccept bcast failed:", err)
		}
	}()
	lb := r.InstanceSpace[replicaId][instance].Lb
	pa := new(PreAccept)
	pa.LeaderId = r.Id
	pa.Replica = replicaId
	pa.Instance = instance
	pa.Ballot = lb.LastTriedBallot
	pa.Command = lb.Cmds
	pa.Seq = lb.Seq
	pa.Deps = lb.Deps

	n := r.N - 1
	if r.Thrifty {
		n = r.Replica.FastQuorumSize() - 1
	}

	sent := 0
	for q := 0; q < r.N-1; q++ {
		if !r.Alive[r.PreferredPeerOrder[q]] {
			continue
		}
		r.SendMsg(r.PreferredPeerOrder[q], r.preAcceptRPC, pa)
		sent++
		if sent >= n {
			break
		}
	}
}

func (r *Replica) bcastTryPreAccept(replicaId int32, instance int32) {
	defer func() {
		if err := recover(); err != nil {
			r.Println("PreAccept bcast failed:", err)
		}
	}()
	lb := r.InstanceSpace[replicaId][instance].Lb
	tpa := new(TryPreAccept)
	tpa.LeaderId = r.Id
	tpa.Replica = replicaId
	tpa.Instance = instance
	tpa.Ballot = lb.LastTriedBallot
	tpa.Command = lb.Cmds
	tpa.Seq = lb.Seq
	tpa.Deps = lb.Deps

	for q := int32(0); q < int32(r.N); q++ {
		if q == r.Id {
			continue
		}
		if !r.Alive[q] {
			continue
		}
		r.SendMsg(q, r.tryPreAcceptRPC, tpa)
	}
}

func (r *Replica) bcastAccept(replicaId int32, instance int32) {
	defer func() {
		if err := recover(); err != nil {
			r.Println("Accept bcast failed:", err)
		}
	}()

	lb := r.InstanceSpace[replicaId][instance].Lb
	ea := new(Accept)
	ea.LeaderId = r.Id
	ea.Replica = replicaId
	ea.Instance = instance
	ea.Ballot = lb.LastTriedBallot
	ea.Seq = lb.Seq
	ea.Deps = lb.Deps

	n := r.N - 1
	if r.Thrifty {
		n = r.N / 2
	}

	sent := 0
	for q := 0; q < r.N-1; q++ {
		if !r.Alive[r.PreferredPeerOrder[q]] {
			continue
		}
		r.SendMsg(r.PreferredPeerOrder[q], r.acceptRPC, ea)
		sent++
		if sent >= n {
			break
		}
	}
}

func (r *Replica) bcastCommit(replicaId int32, instance int32) {
	defer func() {
		if err := recover(); err != nil {
			r.Println("Commit bcast failed:", err)
		}
	}()
	lb := r.InstanceSpace[replicaId][instance].Lb
	ec := new(Commit)
	ec.LeaderId = r.Id
	ec.Replica = replicaId
	ec.Instance = instance
	ec.Command = lb.Cmds
	ec.Seq = lb.Seq
	ec.Deps = lb.Deps
	ec.Ballot = lb.Ballot

	for q := 0; q < r.N-1; q++ {
		if !r.Alive[r.PreferredPeerOrder[q]] {
			continue
		}
		r.SendMsg(r.PreferredPeerOrder[q], r.commitRPC, ec)
	}
}

// Conflict and attribute management

func (r *Replica) clearHashtables() {
	for q := 0; q < r.N; q++ {
		r.conflicts[q] = make(map[state.Key]*InstPair, HT_INIT_SIZE)
	}
}

func (r *Replica) updateCommitted(replicaId int32) {
	r.M.Lock()
	for r.InstanceSpace[replicaId][r.CommittedUpTo[replicaId]+1] != nil &&
		(r.InstanceSpace[replicaId][r.CommittedUpTo[replicaId]+1].Status == COMMITTED ||
			r.InstanceSpace[replicaId][r.CommittedUpTo[replicaId]+1].Status == EXECUTED) {
		r.CommittedUpTo[replicaId] = r.CommittedUpTo[replicaId] + 1
	}
	r.M.Unlock()
}

func (r *Replica) updateConflicts(cmds []state.Command, replicaId int32, instance int32, seq int32) {
	for i := 0; i < len(cmds); i++ {
		if dpair, present := r.conflicts[replicaId][cmds[i].K]; present {
			if dpair.Last < instance {
				r.conflicts[replicaId][cmds[i].K].Last = instance
			}
			if dpair.LastWrite < instance && cmds[i].Op != state.GET {
				r.conflicts[replicaId][cmds[i].K].LastWrite = instance
			}
		} else {
			r.conflicts[replicaId][cmds[i].K] = &InstPair{
				Last:      instance,
				LastWrite: -1,
			}
			if cmds[i].Op != state.GET {
				r.conflicts[replicaId][cmds[i].K].LastWrite = instance
			}
		}
		if s, present := r.maxSeqPerKey[cmds[i].K]; present {
			if s < seq {
				r.maxSeqPerKey[cmds[i].K] = seq
			}
		} else {
			r.maxSeqPerKey[cmds[i].K] = seq
		}
	}
}

func (r *Replica) updateAttributes(cmds []state.Command, seq int32, deps []int32, replicaId int32, instance int32) (int32, []int32, bool) {
	changed := false
	for q := 0; q < r.N; q++ {
		if r.Id != replicaId && int32(q) == replicaId {
			continue
		}
		for i := 0; i < len(cmds); i++ {
			if dpair, present := (r.conflicts[q])[cmds[i].K]; present {
				d := dpair.LastWrite
				if cmds[i].Op != state.GET {
					d = dpair.Last
				}

				if d > deps[q] {
					deps[q] = d
					if seq <= r.InstanceSpace[q][d].Seq {
						seq = r.InstanceSpace[q][d].Seq + 1
					}
					changed = true
					break
				}
			}
		}
	}
	for i := 0; i < len(cmds); i++ {
		if s, present := r.maxSeqPerKey[cmds[i].K]; present {
			if seq <= s {
				changed = true
				seq = s + 1
			}
		}
	}

	return seq, deps, changed
}

func (r *Replica) mergeAttributes(seq1 int32, deps1 []int32, seq2 int32, deps2 []int32) (int32, []int32, bool) {
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
			}
		}
	}
	return seq1, deps1, equal
}

func depsEqual(deps1 []int32, deps2 []int32) bool {
	for i := 0; i < len(deps1); i++ {
		if deps1[i] != deps2[i] {
			return false
		}
	}
	return true
}

// Protocol handlers

func (r *Replica) handlePropose(propose *defs.GPropose) {
	batchSize := len(r.ProposeChan) + 1
	r.M.Lock()
	r.Stats.M["totalBatching"]++
	r.Stats.M["totalBatchingSize"] += batchSize
	r.M.Unlock()

	r.crtInstance[r.Id]++

	cmds := make([]state.Command, batchSize)
	proposals := make([]*defs.GPropose, batchSize)
	cmds[0] = propose.Command
	proposals[0] = propose
	for i := 1; i < batchSize; i++ {
		prop := <-r.ProposeChan
		cmds[i] = prop.Command
		proposals[i] = prop
	}

	r.startPhase1(cmds, r.Id, r.crtInstance[r.Id], r.Id, proposals)

	cpcounter += len(cmds)
}

func (r *Replica) startPhase1(cmds []state.Command, replicaId int32, instance int32, ballot int32, proposals []*defs.GPropose) {
	seq := int32(0)
	deps := make([]int32, r.N)
	for q := 0; q < r.N; q++ {
		deps[q] = -1
	}
	seq, deps, _ = r.updateAttributes(cmds, seq, deps, replicaId, instance)
	comDeps := make([]int32, r.N)
	for i := 0; i < r.N; i++ {
		comDeps[i] = -1
	}

	inst := r.newInstance(replicaId, instance, cmds, ballot, ballot, PREACCEPTED, seq, deps)
	inst.Lb = r.newLeaderBookkeeping(proposals, deps, comDeps, deps, ballot, cmds, PREACCEPTED, -1)
	r.InstanceSpace[replicaId][instance] = inst

	r.updateConflicts(cmds, replicaId, instance, seq)

	if seq >= r.maxSeq {
		r.maxSeq = seq
	}

	r.recordInstanceMetadata(r.InstanceSpace[r.Id][instance])
	r.recordCommands(cmds)
	r.sync()

	r.bcastPreAccept(replicaId, instance)
}

func (r *Replica) handlePreAccept(preAccept *PreAccept) {
	inst := r.InstanceSpace[preAccept.Replica][preAccept.Instance]

	if preAccept.Seq >= r.maxSeq {
		r.maxSeq = preAccept.Seq + 1
	}

	if preAccept.Ballot > r.maxRecvBallot {
		r.maxRecvBallot = preAccept.Ballot
	}

	if preAccept.Instance > r.crtInstance[preAccept.Replica] {
		r.crtInstance[preAccept.Replica] = preAccept.Instance
	}

	if inst == nil {
		inst = r.newInstanceDefault(preAccept.Replica, preAccept.Instance)
		r.InstanceSpace[preAccept.Replica][preAccept.Instance] = inst
	}

	if inst != nil && preAccept.Ballot < inst.Bal {
		return
	}

	inst.Bal = preAccept.Ballot

	if inst.Status >= ACCEPTED {
		if inst.Cmds == nil {
			r.InstanceSpace[preAccept.LeaderId][preAccept.Instance].Cmds = preAccept.Command
			r.updateConflicts(preAccept.Command, preAccept.Replica, preAccept.Instance, preAccept.Seq)
			r.recordCommands(preAccept.Command)
			r.sync()
		}

	} else {
		seq, deps, changed := r.updateAttributes(preAccept.Command, preAccept.Seq, preAccept.Deps, preAccept.Replica, preAccept.Instance)
		status := PREACCEPTED_EQ
		if changed {
			status = PREACCEPTED
		}
		inst.Cmds = preAccept.Command
		inst.Seq = seq
		inst.Deps = deps
		inst.Bal = preAccept.Ballot
		inst.Vbal = preAccept.Ballot
		inst.Status = status

		r.updateConflicts(preAccept.Command, preAccept.Replica, preAccept.Instance, preAccept.Seq)
		r.recordInstanceMetadata(r.InstanceSpace[preAccept.Replica][preAccept.Instance])
		r.recordCommands(preAccept.Command)
		r.sync()

	}

	reply := &PreAcceptReply{
		preAccept.Replica,
		preAccept.Instance,
		inst.Bal,
		inst.Vbal,
		inst.Seq,
		inst.Deps,
		r.CommittedUpTo,
		inst.Status}
	r.replyPreAccept(preAccept.LeaderId, reply)
}

func (r *Replica) handlePreAcceptReply(pareply *PreAcceptReply) {
	inst := r.InstanceSpace[pareply.Replica][pareply.Instance]
	lb := inst.Lb

	if pareply.Ballot > r.maxRecvBallot {
		r.maxRecvBallot = pareply.Ballot
	}

	if lb.LastTriedBallot > pareply.Ballot {
		return
	}

	if lb.LastTriedBallot < pareply.Ballot {
		lb.Nacks++
		if lb.Nacks+1 > r.N>>1 {
			if r.IsLeader {
				r.makeBallot(pareply.Replica, pareply.Instance)
				r.bcastPrepare(pareply.Replica, pareply.Instance)
			}
		}
		return
	}

	if lb.Status != PREACCEPTED && lb.Status != PREACCEPTED_EQ {
		return
	}

	inst.Lb.PreAcceptOKs++

	if pareply.VBallot > lb.Ballot {
		lb.Ballot = pareply.VBallot
		lb.Seq = pareply.Seq
		lb.Deps = pareply.Deps
		lb.Status = pareply.Status
	}

	isInitBallot := IsInitialBallot(lb.LastTriedBallot, pareply.Replica)

	seq, deps, allEqual := r.mergeAttributes(lb.Seq, lb.Deps, pareply.Seq, pareply.Deps)
	if r.N <= 3 && r.Thrifty {
		// no need to check for equality
	} else {
		inst.Lb.AllEqual = inst.Lb.AllEqual && allEqual
		if !allEqual {
			r.M.Lock()
			r.Stats.M["conflicted"]++
			r.M.Unlock()
		}
	}

	allCommitted := true
	if r.N > 7 {
		for q := 0; q < r.N; q++ {
			if inst.Lb.CommittedDeps[q] < pareply.CommittedDeps[q] {
				inst.Lb.CommittedDeps[q] = pareply.CommittedDeps[q]
			}
			if inst.Lb.CommittedDeps[q] < r.CommittedUpTo[q] {
				inst.Lb.CommittedDeps[q] = r.CommittedUpTo[q]
			}
			if inst.Lb.CommittedDeps[q] < inst.Deps[q] {
				allCommitted = false
			}
		}
	}

	if lb.Status <= PREACCEPTED_EQ {
		lb.Deps = deps
		lb.Seq = seq
	}

	precondition := inst.Lb.AllEqual && allCommitted && isInitBallot

	if inst.Lb.PreAcceptOKs >= (r.Replica.FastQuorumSize()-1) && precondition {
		lb.Status = COMMITTED

		inst.Status = lb.Status
		inst.Bal = lb.Ballot
		inst.Cmds = lb.Cmds
		inst.Deps = lb.Deps
		inst.Seq = lb.Seq
		r.recordInstanceMetadata(inst)
		r.sync()

		r.updateCommitted(pareply.Replica)
		if inst.Lb.ClientProposals != nil && !r.Dreply {
			for i := 0; i < len(inst.Lb.ClientProposals); i++ {
				r.ReplyProposeTS(
					&defs.ProposeReplyTS{
						OK:        TRUE,
						CommandId: inst.Lb.ClientProposals[i].CommandId,
						Value:     state.NIL(),
						Timestamp: inst.Lb.ClientProposals[i].Timestamp},
					inst.Lb.ClientProposals[i].Reply,
					inst.Lb.ClientProposals[i].Mutex)
			}
		}

		r.bcastCommit(pareply.Replica, pareply.Instance)

		r.M.Lock()
		r.Stats.M["fast"]++
		if inst.ProposeTime != 0 {
			r.Stats.M["totalCommitTime"] += int(time.Now().UnixNano() - inst.ProposeTime)
		}
		r.M.Unlock()
	} else if inst.Lb.PreAcceptOKs >= r.Replica.FastQuorumSize()-1 {
		lb.Status = ACCEPTED

		inst.Status = lb.Status
		inst.Bal = lb.Ballot
		inst.Cmds = lb.Cmds
		inst.Deps = lb.Deps
		inst.Seq = lb.Seq
		r.recordInstanceMetadata(inst)
		r.sync()

		r.bcastAccept(pareply.Replica, pareply.Instance)

		r.M.Lock()
		r.Stats.M["slow"]++
		if !allCommitted {
			r.Stats.M["weird"]++
		}
		r.M.Unlock()
	}
}

func (r *Replica) handleAccept(accept *Accept) {
	inst := r.InstanceSpace[accept.Replica][accept.Instance]

	if accept.Ballot > r.maxRecvBallot {
		r.maxRecvBallot = accept.Ballot
	}

	if accept.Instance > r.crtInstance[accept.Replica] {
		r.crtInstance[accept.Replica] = accept.Instance
	}

	if inst == nil {
		inst = r.newInstanceDefault(accept.Replica, accept.Instance)
		r.InstanceSpace[accept.Replica][accept.Instance] = inst
	}

	if accept.Ballot < inst.Bal {
		r.Printf("Smaller ballot %d < %d\n", accept.Ballot, inst.Bal)
	} else if inst.Status >= COMMITTED {
		r.Printf("Already committed / executed \n")
	} else {
		inst.Deps = accept.Deps
		inst.Seq = accept.Seq
		inst.Bal = accept.Ballot
		inst.Vbal = accept.Ballot
		r.recordInstanceMetadata(r.InstanceSpace[accept.Replica][accept.Instance])
		r.sync()
	}

	reply := &AcceptReply{accept.Replica, accept.Instance, inst.Bal}
	r.replyAccept(accept.LeaderId, reply)
}

func (r *Replica) handleAcceptReply(areply *AcceptReply) {
	inst := r.InstanceSpace[areply.Replica][areply.Instance]
	lb := inst.Lb

	if areply.Ballot > r.maxRecvBallot {
		r.maxRecvBallot = areply.Ballot
	}

	if lb.Status != ACCEPTED {
		return
	}

	if lb.LastTriedBallot != areply.Ballot {
		return
	}

	if areply.Ballot > lb.LastTriedBallot {
		lb.Nacks++
		if lb.Nacks+1 > r.N>>1 {
			if r.IsLeader {
				r.makeBallot(areply.Replica, areply.Instance)
				r.bcastPrepare(areply.Replica, areply.Instance)
			}
		}
		return
	}

	inst.Lb.AcceptOKs++

	if inst.Lb.AcceptOKs+1 > r.N/2 {
		lb.Status = COMMITTED
		inst.Status = COMMITTED
		r.updateCommitted(areply.Replica)
		r.recordInstanceMetadata(inst)
		r.sync()

		if inst.Lb.ClientProposals != nil && !r.Dreply {
			for i := 0; i < len(inst.Lb.ClientProposals); i++ {
				r.ReplyProposeTS(
					&defs.ProposeReplyTS{
						OK:        TRUE,
						CommandId: inst.Lb.ClientProposals[i].CommandId,
						Value:     state.NIL(),
						Timestamp: inst.Lb.ClientProposals[i].Timestamp},
					inst.Lb.ClientProposals[i].Reply,
					inst.Lb.ClientProposals[i].Mutex)
			}
		}

		r.bcastCommit(areply.Replica, areply.Instance)
		r.M.Lock()
		if inst.ProposeTime != 0 {
			r.Stats.M["totalCommitTime"] += int(time.Now().UnixNano() - inst.ProposeTime)
		}
		r.M.Unlock()
	}
}

func (r *Replica) handleCommit(commit *Commit) {
	inst := r.InstanceSpace[commit.Replica][commit.Instance]

	if commit.Instance > r.crtInstance[commit.Replica] {
		r.crtInstance[commit.Replica] = commit.Instance
	}

	if commit.Ballot > r.maxRecvBallot {
		r.maxRecvBallot = commit.Ballot
	}

	if inst == nil {
		r.InstanceSpace[commit.Replica][commit.Instance] = r.newInstanceDefault(commit.Replica, commit.Instance)
		inst = r.InstanceSpace[commit.Replica][commit.Instance]
	}

	if inst.Status >= COMMITTED {
		return
	}

	if commit.Ballot < inst.Bal {
		return
	}

	if commit.Replica == r.Id {
		if len(commit.Command) == 1 && commit.Command[0].Op == state.NONE && inst.Lb.ClientProposals != nil {
			for _, p := range inst.Lb.ClientProposals {
				r.Printf("In %d.%d, re-proposing %s \n", commit.Replica, commit.Instance, p.Command.String())
				r.ProposeChan <- p
			}
			inst.Lb.ClientProposals = nil
		}
	}

	inst.Bal = commit.Ballot
	inst.Vbal = commit.Ballot
	inst.Cmds = commit.Command
	inst.Seq = commit.Seq
	inst.Deps = commit.Deps
	inst.Status = COMMITTED

	r.updateConflicts(commit.Command, commit.Replica, commit.Instance, commit.Seq)
	r.updateCommitted(commit.Replica)
	r.recordInstanceMetadata(r.InstanceSpace[commit.Replica][commit.Instance])
	r.recordCommands(commit.Command)
}

// Recovery

func (r *Replica) BeTheLeader(args *defs.BeTheLeaderArgs, reply *defs.BeTheLeaderReply) error {
	r.IsLeader = true
	r.Println("I am the leader")
	return nil
}

func (r *Replica) startRecoveryForInstance(replicaId int32, instance int32) {
	inst := r.InstanceSpace[replicaId][instance]
	if inst == nil {
		inst = r.newInstanceDefault(replicaId, instance)
		r.InstanceSpace[replicaId][instance] = inst
	} else if inst.Status >= COMMITTED && inst.Cmds != nil {
		r.Printf("No need to recover %d.%d", replicaId, instance)
		return
	}

	var proposals []*defs.GPropose = nil
	if inst.Lb != nil {
		proposals = inst.Lb.ClientProposals
	}
	inst.Lb = r.newLeaderBookkeepingDefault()
	lb := inst.Lb
	lb.ClientProposals = proposals
	lb.Ballot = inst.Vbal
	lb.Seq = inst.Seq
	lb.Cmds = inst.Cmds
	lb.Deps = inst.Deps
	lb.Status = inst.Status
	r.makeBallot(replicaId, instance)

	inst.Bal = lb.LastTriedBallot
	inst.Vbal = lb.LastTriedBallot
	preply := &PrepareReply{
		r.Id,
		replicaId,
		instance,
		inst.Bal,
		inst.Vbal,
		inst.Status,
		inst.Cmds,
		inst.Seq,
		inst.Deps}

	lb.PrepareReplies = append(lb.PrepareReplies, preply)
	lb.LeaderResponded = r.Id == replicaId

	r.bcastPrepare(replicaId, instance)
}

func (r *Replica) handlePrepare(prepare *Prepare) {
	inst := r.InstanceSpace[prepare.Replica][prepare.Instance]
	var preply *PrepareReply

	if prepare.Ballot > r.maxRecvBallot {
		r.maxRecvBallot = prepare.Ballot
	}

	if inst == nil {
		r.InstanceSpace[prepare.Replica][prepare.Instance] = r.newInstanceDefault(prepare.Replica, prepare.Instance)
		inst = r.InstanceSpace[prepare.Replica][prepare.Instance]
	}

	if prepare.Ballot < inst.Bal {
		r.Printf("Joined higher ballot %d < %d", prepare.Ballot, inst.Bal)
	} else if inst.Bal < prepare.Ballot {
		r.Printf("Joining ballot %d ", prepare.Ballot)
		inst.Bal = prepare.Ballot
	}

	preply = &PrepareReply{
		r.Id,
		prepare.Replica,
		prepare.Instance,
		inst.Bal,
		inst.Vbal,
		inst.Status,
		inst.Cmds,
		inst.Seq,
		inst.Deps}
	r.replyPrepare(prepare.LeaderId, preply)
}

func (r *Replica) handlePrepareReply(preply *PrepareReply) {
	inst := r.InstanceSpace[preply.Replica][preply.Instance]
	lb := inst.Lb

	if preply.Ballot > r.maxRecvBallot {
		r.maxRecvBallot = preply.Ballot
	}

	if inst == nil || lb == nil || !lb.Preparing {
		return
	}

	if preply.Ballot != lb.LastTriedBallot {
		lb.Nacks++
		return
	}

	lb.PrepareReplies = append(lb.PrepareReplies, preply)
	if len(lb.PrepareReplies) < r.Replica.SlowQuorumSize() {
		return
	}

	lb.Preparing = false

	preAcceptCount := 0
	subCase := 0
	allEqual := true
	for _, element := range lb.PrepareReplies {
		if element.VBallot >= lb.Ballot {
			lb.Ballot = element.VBallot
			lb.Cmds = element.Command
			lb.Seq = element.Seq
			lb.Deps = element.Deps
			lb.Status = element.Status
		}
		if element.AcceptorId == element.Replica {
			lb.LeaderResponded = true
		}
		if element.Status == PREACCEPTED_EQ || element.Status == PREACCEPTED {
			preAcceptCount++
		}
	}

	if lb.Status >= COMMITTED {
		subCase = 1
	} else if lb.Status == ACCEPTED {
		subCase = 2
	} else if lb.Status == PREACCEPTED || lb.Status == PREACCEPTED_EQ {
		for _, element := range lb.PrepareReplies {
			if element.VBallot == lb.Ballot && element.Status >= PREACCEPTED {
				_, _, eq := r.mergeAttributes(lb.Seq, lb.Deps, element.Seq, element.Deps)
				if !eq {
					allEqual = false
					break
				}
			}
		}
		if preAcceptCount >= r.Replica.SlowQuorumSize()-1 && !lb.LeaderResponded && allEqual {
			subCase = 3
		} else if preAcceptCount >= r.Replica.SlowQuorumSize()-1 && !lb.LeaderResponded && allEqual {
			subCase = 4
		} else if preAcceptCount > 0 && (lb.LeaderResponded || !allEqual || preAcceptCount < r.Replica.SlowQuorumSize()-1) {
			subCase = 5
		} else {
			panic("Cannot occur")
		}
	} else if lb.Status == NONE {
		subCase = 6
	} else {
		panic("Status unknown")
	}

	inst.Cmds = lb.Cmds
	inst.Bal = lb.LastTriedBallot
	inst.Vbal = lb.LastTriedBallot
	inst.Seq = lb.Seq
	inst.Deps = lb.Deps
	inst.Status = lb.Status

	if subCase == 1 {
		// nothing to do
	} else if subCase == 2 || subCase == 3 {
		inst.Status = ACCEPTED
		lb.Status = ACCEPTED
		r.bcastAccept(preply.Replica, preply.Instance)
	} else if subCase == 4 {
		lb.TryingToPreAccept = true
		r.bcastTryPreAccept(preply.Replica, preply.Instance)
	} else {
		cmd := state.NOOP()
		if inst.Lb.Cmds != nil {
			cmd = inst.Lb.Cmds
		}
		r.startPhase1(cmd, preply.Replica, preply.Instance, lb.LastTriedBallot, lb.ClientProposals)
	}
}

func (r *Replica) handleTryPreAccept(tpa *TryPreAccept) {
	inst := r.InstanceSpace[tpa.Replica][tpa.Instance]

	if inst == nil {
		r.InstanceSpace[tpa.Replica][tpa.Instance] = r.newInstanceDefault(tpa.Replica, tpa.Instance)
		inst = r.InstanceSpace[tpa.Replica][tpa.Instance]
	}

	if inst.Bal > tpa.Ballot {
		r.Printf("Smaller ballot %d < %d\n", tpa.Ballot, inst.Bal)
		return
	}
	inst.Bal = tpa.Ballot

	confRep := int32(0)
	confInst := int32(0)
	confStatus := NONE
	if inst.Status == NONE {
		if conflict, cr, ci := r.findPreAcceptConflicts(tpa.Command, tpa.Replica, tpa.Instance, tpa.Seq, tpa.Deps); conflict {
			confRep = cr
			confInst = ci
		} else {
			if tpa.Instance > r.crtInstance[tpa.Replica] {
				r.crtInstance[tpa.Replica] = tpa.Instance
			}
			inst.Cmds = tpa.Command
			inst.Seq = tpa.Seq
			inst.Deps = tpa.Deps
			inst.Status = PREACCEPTED
		}
	}

	rtpa := &TryPreAcceptReply{r.Id, tpa.Replica, tpa.Instance, inst.Bal, inst.Vbal, confRep, confInst, confStatus}

	r.replyTryPreAccept(tpa.LeaderId, rtpa)
}

func (r *Replica) findPreAcceptConflicts(cmds []state.Command, replicaId int32, instance int32, seq int32, deps []int32) (bool, int32, int32) {
	inst := r.InstanceSpace[replicaId][instance]
	if inst != nil && len(inst.Cmds) > 0 {
		if inst.Status >= ACCEPTED {
			return true, replicaId, instance
		}
		if inst.Seq == seq && depsEqual(inst.Deps, deps) {
			return false, replicaId, instance
		}
	}
	for q := int32(0); q < int32(r.N); q++ {
		for i := r.ExecedUpTo[q]; i <= r.crtInstance[q]; i++ {
			if i == -1 {
				continue
			}
			if replicaId == q && instance == i {
				break
			}
			if i == deps[q] {
				continue
			}
			inst := r.InstanceSpace[q][i]
			if inst == nil || inst.Cmds == nil || len(inst.Cmds) == 0 {
				continue
			}
			if inst.Deps[replicaId] >= instance {
				continue
			}
			if r.LRead || state.ConflictBatch(inst.Cmds, cmds) {
				if i > deps[q] ||
					(i < deps[q] && inst.Seq >= seq && (q != replicaId || inst.Status > PREACCEPTED_EQ)) {
					return true, q, i
				}
			}
		}
	}
	return false, -1, -1
}

func (r *Replica) handleTryPreAcceptReply(tpar *TryPreAcceptReply) {
	inst := r.InstanceSpace[tpar.Replica][tpar.Instance]

	if tpar.Ballot > r.maxRecvBallot {
		r.maxRecvBallot = tpar.Ballot
	}

	if inst == nil {
		r.InstanceSpace[tpar.Replica][tpar.Instance] = r.newInstanceDefault(tpar.Replica, tpar.Instance)
		inst = r.InstanceSpace[tpar.Replica][tpar.Instance]
	}

	lb := inst.Lb
	if lb == nil || !lb.TryingToPreAccept {
		return
	}

	if tpar.Ballot != lb.LastTriedBallot {
		return
	}

	lb.TpaReps++

	if tpar.VBallot == lb.LastTriedBallot {
		lb.PreAcceptOKs++
		if lb.PreAcceptOKs >= r.N/2 {
			lb.Status = ACCEPTED
			lb.TryingToPreAccept = false
			lb.AcceptOKs = 0

			inst.Cmds = lb.Cmds
			inst.Seq = lb.Seq
			inst.Deps = lb.Deps
			inst.Status = lb.Status
			inst.Vbal = lb.LastTriedBallot
			inst.Bal = lb.LastTriedBallot

			r.bcastAccept(tpar.Replica, tpar.Instance)
			return
		}
	} else {
		lb.Nacks++
		lb.PossibleQuorum[tpar.AcceptorId] = false
		lb.PossibleQuorum[tpar.ConflictReplica] = false
	}

	lb.TpaAccepted = lb.TpaAccepted || (tpar.ConflictStatus >= ACCEPTED)

	if lb.TpaReps >= r.Replica.SlowQuorumSize()-1 && lb.TpaAccepted {
		lb.TryingToPreAccept = false
		r.startPhase1(lb.Cmds, tpar.Replica, tpar.Instance, lb.LastTriedBallot, lb.ClientProposals)
		return
	}

	notInQuorum := 0
	for q := 0; q < r.N; q++ {
		if !lb.PossibleQuorum[tpar.AcceptorId] {
			notInQuorum++
		}
	}

	if notInQuorum == r.N/2 {
		if present, dq, _ := deferredByInstance(tpar.Replica, tpar.Instance); present {
			if lb.PossibleQuorum[dq] {
				lb.TryingToPreAccept = false
				r.makeBallot(tpar.Replica, tpar.Instance)
				r.startPhase1(lb.Cmds, tpar.Replica, tpar.Instance, lb.LastTriedBallot, lb.ClientProposals)
				return
			}
		}
	}

	if lb.TpaReps >= r.N/2 {
		updateDeferred(tpar.Replica, tpar.Instance, tpar.ConflictReplica, tpar.ConflictInstance)
		lb.TryingToPreAccept = false
	}
}

// Defer cycle prevention helpers

var deferMap = make(map[uint64]uint64)

func updateDeferred(dr int32, di int32, r int32, i int32) {
	daux := (uint64(dr) << 32) | uint64(di)
	aux := (uint64(r) << 32) | uint64(i)
	deferMap[aux] = daux
}

func deferredByInstance(q int32, i int32) (bool, int32, int32) {
	aux := (uint64(q) << 32) | uint64(i)
	daux, present := deferMap[aux]
	if !present {
		return false, 0, 0
	}
	dq := int32(daux >> 32)
	di := int32(daux)
	return true, dq, di
}

// Instance constructors

func (r *Replica) newInstanceDefault(replicaId int32, instance int32) *Instance {
	return r.newInstance(replicaId, instance, nil, -1, -1, NONE, -1, nil)
}

func (r *Replica) newInstance(replicaId int32, instance int32, cmds []state.Command, cballot int32, lballot int32, status int8, seq int32, deps []int32) *Instance {
	return NewInstance(replicaId, instance, cmds, cballot, lballot, status, seq, deps)
}

func (r *Replica) newLeaderBookkeepingDefault() *LeaderBookkeeping {
	return r.newLeaderBookkeeping(nil, r.newNilDeps(), r.newNilDeps(), r.newNilDeps(), 0, nil, NONE, -1)
}

func (r *Replica) newLeaderBookkeeping(p []*defs.GPropose, originalDeps []int32, committedDeps []int32, deps []int32, lastTriedBallot int32, cmds []state.Command, status int8, seq int32) *LeaderBookkeeping {
	return &LeaderBookkeeping{
		ClientProposals:   p,
		Ballot:            -1,
		AllEqual:          true,
		PreAcceptOKs:      0,
		AcceptOKs:         0,
		Nacks:             0,
		OriginalDeps:      originalDeps,
		CommittedDeps:     committedDeps,
		PrepareReplies:    nil,
		Preparing:         true,
		TryingToPreAccept: false,
		PossibleQuorum:    make([]bool, r.N),
		TpaReps:           0,
		TpaAccepted:       false,
		LastTriedBallot:   lastTriedBallot,
		Cmds:              cmds,
		Status:            status,
		Seq:               seq,
		Deps:              deps,
		LeaderResponded:   false,
	}
}

func (r *Replica) newNilDeps() []int32 {
	nildeps := make([]int32, r.N)
	for i := 0; i < r.N; i++ {
		nildeps[i] = -1
	}
	return nildeps
}

func (r *Replica) makeBallot(replicaId int32, instance int32) {
	lb := r.InstanceSpace[replicaId][instance].Lb
	lb.LastTriedBallot = MakeBallot(r.Id, replicaId, r.N, r.maxRecvBallot, r.IsLeader)
}
