package mongotunable

import (
	"math/rand"
	"time"

	"github.com/imdea-software/swiftpaxos/config"
	"github.com/imdea-software/swiftpaxos/dlog"
	"github.com/imdea-software/swiftpaxos/replica"
	"github.com/imdea-software/swiftpaxos/replica/defs"
	fastrpc "github.com/imdea-software/swiftpaxos/rpc"
	"github.com/imdea-software/swiftpaxos/state"
)

const MAX_DEPTH_DEP = 10000
const CHAN_BUFFER_SIZE = 200000
const MAX_BATCH = 1
const BATCH_INTERVAL = 100 * time.Microsecond

// NO_COMMIT_CHANNEL is the number of commit/commitShort channels per peer
// to avoid head-of-line blocking on commit message delivery.
const NO_COMMIT_CHANNEL = 10

type Instance struct {
	cmds                []state.Command
	ballot              int32
	deps                int32
	status              uint8
	IsMajorityCommitted bool
	lb                  *LeaderBookkeeping
}

type ReadInstance struct {
	cmds   []state.Command
	ballot int32
	deps   int32
	status uint8
	lb     *LeaderBookkeeping
}

type LeaderBookkeeping struct {
	clientProposals []*defs.GPropose
	maxRecvBallot   int32
	prepareOKs      int
	acceptOKs       int
	commitOKs       int
	nacks           int
}

// CommunicationSupply holds all RPC channels and type codes.
type CommunicationSupply struct {
	prepareChan      chan fastrpc.Serializable
	acceptChan       chan fastrpc.Serializable
	commitChan       []chan fastrpc.Serializable
	commitShortChan  []chan fastrpc.Serializable
	prepareReplyChan chan fastrpc.Serializable
	acceptReplyChan  chan fastrpc.Serializable
	commitAckChan    chan fastrpc.Serializable

	prepareRPC      uint8
	acceptRPC       uint8
	commitRPC       []uint8
	commitShortRPC  []uint8
	prepareReplyRPC uint8
	acceptReplyRPC  uint8
	commitAckRPC    uint8
}

func initCs(cs *CommunicationSupply, t *fastrpc.Table, n int) {
	cs.prepareChan = make(chan fastrpc.Serializable, CHAN_BUFFER_SIZE)
	cs.acceptChan = make(chan fastrpc.Serializable, CHAN_BUFFER_SIZE)
	cs.prepareReplyChan = make(chan fastrpc.Serializable, CHAN_BUFFER_SIZE)
	cs.acceptReplyChan = make(chan fastrpc.Serializable, 3*CHAN_BUFFER_SIZE)
	cs.commitAckChan = make(chan fastrpc.Serializable, CHAN_BUFFER_SIZE)

	numCommitChans := n * NO_COMMIT_CHANNEL
	cs.commitChan = make([]chan fastrpc.Serializable, numCommitChans)
	cs.commitShortChan = make([]chan fastrpc.Serializable, numCommitChans)
	cs.commitRPC = make([]uint8, numCommitChans)
	cs.commitShortRPC = make([]uint8, numCommitChans)

	cs.prepareRPC = t.Register(new(Prepare), cs.prepareChan)
	cs.acceptRPC = t.Register(new(Accept), cs.acceptChan)

	for i := 0; i < numCommitChans; i++ {
		cs.commitChan[i] = make(chan fastrpc.Serializable, CHAN_BUFFER_SIZE)
		cs.commitShortChan[i] = make(chan fastrpc.Serializable, CHAN_BUFFER_SIZE)
		cs.commitRPC[i] = t.Register(new(Commit), cs.commitChan[i])
		cs.commitShortRPC[i] = t.Register(new(CommitShort), cs.commitShortChan[i])
	}

	cs.prepareReplyRPC = t.Register(new(PrepareReply), cs.prepareReplyChan)
	cs.acceptReplyRPC = t.Register(new(AcceptReply), cs.acceptReplyChan)
	cs.commitAckRPC = t.Register(new(CommitAck), cs.commitAckChan)
}

// Replica implements the MongoDB-Tunable consensus protocol.
// When isPileus is true, all PUT commands are forced to CL=STRONG (Pileus variant).
type Replica struct {
	*replica.Replica

	isPileus bool

	instanceSpace     []*Instance
	instanceReadSpace []*ReadInstance
	crtInstance       int32
	crtReadInstance   int32
	defaultBallot     int32

	committedUpTo            int32
	executedUpTo             int32
	majorityCommittedUpTo    int32
	leaderCommitPoint        int32
	executedReadUpTo         int32
	putExecutedUpToMajority  int32

	cs     CommunicationSupply
	sender replica.Sender

	// Cache pools
	prepareCache      *PrepareCache
	prepareReplyCache *PrepareReplyCache
	acceptCache       *AcceptCache
	acceptReplyCache  *AcceptReplyCache
	commitCache       *CommitCache
	commitShortCache  *CommitShortCache
	commitAckCache    *CommitAckCache
}

// New creates a new MongoDB-Tunable replica.
// If isPileus is true, all PUT commands are forced to CL=STRONG.
func New(alias string, id int, addrs []string, isLeader bool, f int,
	isPileus bool, conf *config.Config, logger *dlog.Logger) *Replica {

	n := len(addrs)

	r := &Replica{
		Replica: replica.New(alias, id, f, addrs, false, true, false, conf, logger),

		isPileus: isPileus,

		instanceSpace:     make([]*Instance, 15*1024*1024),
		instanceReadSpace: make([]*ReadInstance, 15*1024*1024),
		crtInstance:       0,
		crtReadInstance:   0,
		defaultBallot:     -1,

		committedUpTo:           -1,
		executedUpTo:            -1,
		majorityCommittedUpTo:   -1,
		leaderCommitPoint:       -1,
		executedReadUpTo:        -1,
		putExecutedUpToMajority: -1,

		prepareCache:      NewPrepareCache(),
		prepareReplyCache: NewPrepareReplyCache(),
		acceptCache:       NewAcceptCache(),
		acceptReplyCache:  NewAcceptReplyCache(),
		commitCache:       NewCommitCache(),
		commitShortCache:  NewCommitShortCache(),
		commitAckCache:    NewCommitAckCache(),
	}

	// Register RPC types
	initCs(&r.cs, r.RPC, n)

	// Create async sender
	r.sender = replica.NewSender(r.Replica)

	// If designated as leader, set up leadership
	if isLeader {
		r.BeTheLeader(nil, nil)
	}

	go r.run()

	return r
}

// BeTheLeader is called by the master via RPC to designate this replica as leader.
func (r *Replica) BeTheLeader(args *defs.BeTheLeaderArgs, reply *defs.BeTheLeaderReply) error {
	r.defaultBallot = r.makeUniqueBallot(0)
	r.Println("I am the MongoDB-Tunable leader")
	return nil
}

func (r *Replica) makeUniqueBallot(ballot int32) int32 {
	return (ballot << 4) | r.Id
}

func (r *Replica) run() {
	r.ConnectToPeers()
	r.ComputeClosestPeers()

	// Launch execution goroutines
	go r.executeWeakReadCommands()
	go r.executeWriteCommands()
	go r.executeWriteCommandsMajorityCommitted()

	// Launch majority commit tracking goroutines
	go r.handleMajorityCommit()
	go r.updateMajorityCommittedUpToWrite()

	clockChan := make(chan bool, 1)
	go func() {
		for !r.Shutdown {
			time.Sleep(BATCH_INTERVAL)
			clockChan <- true
		}
	}()

	onOffProposeChan := r.ProposeChan

	go r.WaitForClientConnections()

	for !r.Shutdown {
		// Non-blocking poll of all commit channels (avoid HOL blocking in select)
		for _, ch := range r.cs.commitChan {
			select {
			case commitS := <-ch:
				commit := commitS.(*Commit)
				r.handleCommit(commit)
			default:
			}
		}

		for _, ch := range r.cs.commitShortChan {
			select {
			case commitS := <-ch:
				commitShort := commitS.(*CommitShort)
				r.handleCommitShort(commitShort)
			default:
			}
		}

		select {
		case <-clockChan:
			onOffProposeChan = r.ProposeChan

		case propose := <-onOffProposeChan:
			r.handlePropose(propose)
			if MAX_BATCH > 100 {
				onOffProposeChan = nil
			}

		case prepareS := <-r.cs.prepareChan:
			prepare := prepareS.(*Prepare)
			r.handlePrepare(prepare)

		case acceptS := <-r.cs.acceptChan:
			accept := acceptS.(*Accept)
			r.handleAccept(accept)

		case prepareReplyS := <-r.cs.prepareReplyChan:
			prepareReply := prepareReplyS.(*PrepareReply)
			r.handlePrepareReply(prepareReply)

		case acceptReplyS := <-r.cs.acceptReplyChan:
			acceptReply := acceptReplyS.(*AcceptReply)
			r.handleAcceptReply(acceptReply)

		case commitAckS := <-r.cs.commitAckChan:
			commitAck := commitAckS.(*CommitAck)
			r.handleCommitAck(commitAck)
		}
	}
}

// updateCommittedUpTo advances committedUpTo to the highest contiguous committed instance.
func (r *Replica) updateCommittedUpTo() {
	for r.instanceSpace[r.committedUpTo+1] != nil &&
		(r.instanceSpace[r.committedUpTo+1].status == COMMITTED || r.instanceSpace[r.committedUpTo+1].status == EXECUTED) {
		r.committedUpTo++
	}
}

// updateMajorityCommittedUpToWrite runs in a goroutine, advancing the majority-committed watermark.
func (r *Replica) updateMajorityCommittedUpToWrite() {
	for !r.Shutdown {
		for r.instanceSpace[r.majorityCommittedUpTo+1] != nil &&
			r.instanceSpace[r.majorityCommittedUpTo+1].IsMajorityCommitted &&
			(r.instanceSpace[r.majorityCommittedUpTo+1].status == COMMITTED ||
				r.instanceSpace[r.majorityCommittedUpTo+1].status == EXECUTED ||
				r.instanceSpace[r.majorityCommittedUpTo+1].status == DISCARDED) {
			r.majorityCommittedUpTo++
		}
		time.Sleep(500 * time.Nanosecond)
	}
}

// handleMajorityCommit processes CommitAck-triggered majority commit state.
// In the Orca source this was a separate goroutine that monitored a channel;
// here, majority commit is tracked inline in handleCommitAck.
func (r *Replica) handleMajorityCommit() {
	// In the original protocol, this goroutine watched a majorityCommitChan
	// and set IsMajorityCommitted on instances. Since we handle this directly
	// in handleCommitAck, this goroutine is a no-op placeholder.
}

// trackDependency returns the maximum dependency from a slice of deps.
func trackDependency(deps []int32) int32 {
	if len(deps) == 0 {
		return -1
	}
	max := deps[0]
	for _, dep := range deps {
		if dep > max {
			max = dep
		}
	}
	return max
}

// replyPrepare sends a PrepareReply to the given replica.
func (r *Replica) replyPrepare(replicaId int32, reply *PrepareReply) {
	r.sender.SendTo(replicaId, reply, r.cs.prepareReplyRPC)
}

// replyAccept sends an AcceptReply to the given replica.
func (r *Replica) replyAccept(replicaId int32, reply *AcceptReply) {
	r.sender.SendTo(replicaId, reply, r.cs.acceptReplyRPC)
}

// replyCommitAck sends a CommitAck to the given replica.
func (r *Replica) replyCommitAck(replicaId int32, reply *CommitAck) {
	r.sender.SendTo(replicaId, reply, r.cs.commitAckRPC)
}

// bcastPrepare broadcasts a Prepare message to a majority of peers.
func (r *Replica) bcastPrepare(instance int32, ballot int32, toInfinity bool) {
	ti := uint8(0)
	if toInfinity {
		ti = 1
	}
	args := &Prepare{r.Id, instance, ballot, ti}

	n := r.N - 1
	if r.Thrifty {
		n = r.N >> 1
	}
	q := r.Id
	for sent := 0; sent < n; {
		q = (q + 1) % int32(r.N)
		if q == r.Id {
			break
		}
		if !r.Alive[q] {
			continue
		}
		sent++
		r.sender.SendTo(q, args, r.cs.prepareRPC)
	}
}

// bcastAccept broadcasts an Accept message to a majority of peers.
func (r *Replica) bcastAccept(instance int32, ballot int32, deps int32, command []state.Command, majorityCommitPoint int32) {
	var pa Accept
	pa.LeaderId = r.Id
	pa.Instance = instance
	pa.Ballot = ballot
	pa.Command = command
	pa.MajorityCommitPoint = majorityCommitPoint
	pa.Deps = deps
	args := &pa

	n := r.N - 1
	if r.Thrifty {
		n = r.N >> 1
	}
	q := r.Id
	for sent := 0; sent < n; {
		q = (q + 1) % int32(r.N)
		if q == r.Id {
			break
		}
		if !r.Alive[q] {
			continue
		}
		sent++
		r.sender.SendTo(q, args, r.cs.acceptRPC)
	}
}

// bcastStrongCommit broadcasts commit to majority via CommitShort, rest via full Commit.
func (r *Replica) bcastStrongCommit(instance int32, ballot int32, deps int32, command []state.Command, majorityCommitPoint int32) {
	var pc Commit
	var pcs CommitShort
	pc.LeaderId = r.Id
	pc.Instance = instance
	pc.Ballot = ballot
	pc.Command = command
	pc.MajorityCommitPoint = majorityCommitPoint
	pc.Deps = deps

	pcs.LeaderId = r.Id
	pcs.Instance = instance
	pcs.Ballot = ballot
	pcs.MajorityCommitPoint = majorityCommitPoint
	pcs.Count = int32(len(command))

	n := r.N - 1
	if r.Thrifty {
		n = r.N >> 1
	}
	q := r.Id
	sent := 0

	numChans := len(r.cs.commitShortRPC)

	// Send CommitShort to majority
	for sent < n {
		q = (q + 1) % int32(r.N)
		if q == r.Id {
			break
		}
		if !r.Alive[q] {
			continue
		}
		sent++
		r.sender.SendTo(q, &pcs, r.cs.commitShortRPC[rand.Intn(numChans)])
	}
	// Send full Commit to the rest (if thrifty)
	if r.Thrifty && q != r.Id {
		for sent < r.N-1 {
			q = (q + 1) % int32(r.N)
			if q == r.Id {
				break
			}
			if !r.Alive[q] {
				continue
			}
			sent++
			r.sender.SendTo(q, &pc, r.cs.commitRPC[rand.Intn(numChans)])
		}
	}
}

// bcastCausalCommit broadcasts a full Commit to all peers (causal path, no ack needed).
func (r *Replica) bcastCausalCommit(instance int32, ballot int32, deps int32, command []state.Command, majorityCommitPoint int32) {
	var pcc Commit
	pcc.LeaderId = r.Id
	pcc.Instance = instance
	pcc.Ballot = ballot
	pcc.MajorityCommitPoint = majorityCommitPoint
	pcc.Deps = deps
	pcc.Command = command

	numChans := len(r.cs.commitRPC)
	q := r.Id
	for sent := 0; sent < r.N; {
		q = (q + 1) % int32(r.N)
		if q == r.Id {
			break
		}
		if !r.Alive[q] {
			continue
		}
		sent++
		r.sender.SendTo(q, &pcc, r.cs.commitRPC[rand.Intn(numChans)])
	}
}

// --- handlePropose: processes client proposals ---
func (r *Replica) handlePropose(propose *defs.GPropose) {
	// isPileus: force all PUTs to STRONG
	if r.isPileus && propose.Command.Op == state.PUT {
		propose.Command.CL = state.STRONG
	}

	if propose.Command.Op == state.PUT {
		if r.defaultBallot == -1 {
			// Not the leader
			preply := &defs.ProposeReplyTS{
				OK:        defs.FALSE,
				CommandId: propose.CommandId,
				Value:     state.NIL(),
				Timestamp: propose.Timestamp,
				LeaderId:  -1,
			}
			r.ReplyProposeTS(preply, propose.Reply, propose.Mutex)
			return
		}
		for r.instanceSpace[r.crtInstance] != nil {
			r.crtInstance++
		}
	} else {
		// GET
		if propose.Command.CL == state.STRONG {
			if r.defaultBallot == -1 {
				preply := &defs.ProposeReplyTS{
					OK:        defs.FALSE,
					CommandId: propose.CommandId,
					Value:     state.NIL(),
					Timestamp: propose.Timestamp,
					LeaderId:  -1,
				}
				r.ReplyProposeTS(preply, propose.Reply, propose.Mutex)
				return
			}
			for r.instanceSpace[r.crtInstance] != nil {
				r.crtInstance++
			}
		} else {
			// Causal GET: uses read instance space
			for r.instanceReadSpace[r.crtReadInstance] != nil {
				r.crtReadInstance++
			}
		}
	}

	batchSize := len(r.ProposeChan) + 1
	if batchSize > MAX_BATCH {
		batchSize = MAX_BATCH
	}

	causalCmds := make([]state.Command, 0)
	causalProposals := make([]*defs.GPropose, 0)
	causalDeps := make([]int32, 0)
	strongCmds := make([]state.Command, 0)
	strongDeps := make([]int32, 0)
	strongProposals := make([]*defs.GPropose, 0)

	// Classify first proposal
	dep := r.crtInstance - 1 // server-side dependency: last known write instance
	if propose.Command.CL == state.CAUSAL || propose.Command.CL == state.NONE {
		causalCmds = append(causalCmds, propose.Command)
		causalDeps = append(causalDeps, dep)
		causalProposals = append(causalProposals, propose)
	} else {
		strongCmds = append(strongCmds, propose.Command)
		strongDeps = append(strongDeps, dep)
		strongProposals = append(strongProposals, propose)
	}

	// Batch remaining proposals
	for i := 1; i < batchSize; i++ {
		prop := <-r.ProposeChan
		if r.isPileus && prop.Command.Op == state.PUT {
			prop.Command.CL = state.STRONG
		}
		dep := r.crtInstance - 1
		if prop.Command.CL == state.CAUSAL || prop.Command.CL == state.NONE {
			causalCmds = append(causalCmds, prop.Command)
			causalDeps = append(causalDeps, dep)
			causalProposals = append(causalProposals, prop)
		} else {
			strongCmds = append(strongCmds, prop.Command)
			strongDeps = append(strongDeps, dep)
			strongProposals = append(strongProposals, prop)
		}
	}

	causalDep := trackDependency(causalDeps)
	strongDep := trackDependency(strongDeps)

	if len(causalCmds) != 0 {
		var instNo int32
		if causalCmds[0].Op == state.PUT {
			instNo = r.crtInstance
			r.crtInstance++
		} else {
			instNo = r.crtReadInstance
			r.crtReadInstance++
		}
		r.startCausalCommit(instNo, causalProposals, causalCmds, causalDep)
	}
	if len(strongCmds) != 0 {
		instNo := r.crtInstance
		r.crtInstance++
		r.startStrongCommit(instNo, strongProposals, strongCmds, strongDep)
	}
}

// startStrongCommit initiates the strong commit path (Prepare or Accept phase).
func (r *Replica) startStrongCommit(inst int32, proposals []*defs.GPropose, cmds []state.Command, deps int32) {
	if r.defaultBallot == -1 {
		r.instanceSpace[inst] = &Instance{
			cmds:   cmds,
			ballot: r.makeUniqueBallot(0),
			deps:   deps,
			status: PREPARING,
			lb:     &LeaderBookkeeping{proposals, 0, 0, 0, 0, 0},
		}
		r.bcastPrepare(inst, r.makeUniqueBallot(0), true)
	} else {
		r.instanceSpace[inst] = &Instance{
			cmds:   cmds,
			ballot: r.defaultBallot,
			deps:   deps,
			status: PREPARED,
			lb:     &LeaderBookkeeping{proposals, 0, 0, 0, 0, 0},
		}
		r.bcastAccept(inst, r.defaultBallot, deps, cmds, r.majorityCommittedUpTo)
	}
}

// startCausalCommit initiates the causal commit path.
// GETs: immediately committed and reply sent (if Dreply is false).
// PUTs: if leader, immediately committed + broadcast; else Prepare phase.
func (r *Replica) startCausalCommit(inst int32, proposals []*defs.GPropose, cmds []state.Command, deps int32) {
	if cmds[0].Op == state.GET {
		// Causal GET: place in read instance space, immediately COMMITTED
		r.instanceReadSpace[inst] = &ReadInstance{
			cmds:   cmds,
			ballot: r.defaultBallot,
			deps:   deps,
			status: COMMITTED,
			lb:     &LeaderBookkeeping{proposals, 0, 0, 0, 0, 0},
		}
		if r.instanceReadSpace[inst].lb.clientProposals != nil && !r.Dreply {
			for i := 0; i < len(r.instanceReadSpace[inst].cmds); i++ {
				propreply := &defs.ProposeReplyTS{
					OK:        defs.TRUE,
					CommandId: r.instanceReadSpace[inst].lb.clientProposals[i].CommandId,
					Value:     state.NIL(),
					Timestamp: r.instanceReadSpace[inst].lb.clientProposals[i].Timestamp,
				}
				r.ReplyProposeTS(propreply, r.instanceReadSpace[inst].lb.clientProposals[i].Reply, r.instanceReadSpace[inst].lb.clientProposals[i].Mutex)
			}
		}
	} else {
		// Causal PUT
		if r.defaultBallot == -1 {
			r.instanceSpace[inst] = &Instance{
				cmds:   cmds,
				ballot: r.makeUniqueBallot(0),
				deps:   deps,
				status: PREPARING,
				lb:     &LeaderBookkeeping{proposals, 0, 0, 0, 0, 0},
			}
			r.bcastPrepare(inst, r.makeUniqueBallot(0), true)
		} else {
			r.instanceSpace[inst] = &Instance{
				cmds:   cmds,
				ballot: r.defaultBallot,
				deps:   deps,
				status: COMMITTED,
				lb:     &LeaderBookkeeping{proposals, 0, 0, 0, 0, 0},
			}
			// Immediately reply for causal writes (if Dreply is false)
			if r.instanceSpace[inst].lb.clientProposals != nil && !r.Dreply {
				for i := 0; i < len(r.instanceSpace[inst].cmds); i++ {
					propreply := &defs.ProposeReplyTS{
						OK:        defs.TRUE,
						CommandId: r.instanceSpace[inst].lb.clientProposals[i].CommandId,
						Value:     state.NIL(),
						Timestamp: r.instanceSpace[inst].lb.clientProposals[i].Timestamp,
					}
					r.ReplyProposeTS(propreply, r.instanceSpace[inst].lb.clientProposals[i].Reply, r.instanceSpace[inst].lb.clientProposals[i].Mutex)
				}
			}
			r.bcastCausalCommit(inst, r.defaultBallot, deps, cmds, r.majorityCommittedUpTo)
		}
	}
}

// --- Message handlers ---

func (r *Replica) handlePrepare(prepare *Prepare) {
	inst := r.instanceSpace[prepare.Instance]
	var preply *PrepareReply

	if inst == nil {
		ok := uint8(1)
		if r.defaultBallot > prepare.Ballot {
			ok = 0
		}
		preply = &PrepareReply{prepare.Instance, ok, r.defaultBallot, make([]state.Command, 0)}
	} else {
		ok := uint8(1)
		if prepare.Ballot < inst.ballot {
			ok = 0
		}
		preply = &PrepareReply{prepare.Instance, ok, inst.ballot, inst.cmds}
	}

	r.replyPrepare(prepare.LeaderId, preply)

	if prepare.ToInfinity == 1 && prepare.Ballot > r.defaultBallot {
		r.defaultBallot = prepare.Ballot
	}
}

func (r *Replica) handleAccept(accept *Accept) {
	inst := r.instanceSpace[accept.Instance]

	if accept.Instance >= r.crtInstance {
		r.crtInstance = accept.Instance + 1
	}

	if r.leaderCommitPoint < accept.MajorityCommitPoint {
		r.leaderCommitPoint = accept.MajorityCommitPoint
	}

	var areply *AcceptReply
	if inst == nil {
		if accept.Ballot < r.defaultBallot {
			areply = &AcceptReply{accept.Instance, 0, r.defaultBallot}
		} else {
			r.instanceSpace[accept.Instance] = &Instance{
				cmds:   accept.Command,
				ballot: accept.Ballot,
				deps:   accept.Deps,
				status: ACCEPTED,
				lb:     nil,
			}
			areply = &AcceptReply{accept.Instance, 1, r.defaultBallot}
		}
	} else if inst.ballot > accept.Ballot {
		areply = &AcceptReply{accept.Instance, 0, inst.ballot}
	} else if inst.ballot < accept.Ballot {
		inst.cmds = accept.Command
		inst.ballot = accept.Ballot
		inst.status = ACCEPTED
		inst.IsMajorityCommitted = false
		areply = &AcceptReply{accept.Instance, 1, inst.ballot}
		if inst.lb != nil && inst.lb.clientProposals != nil {
			for i := 0; i < len(inst.lb.clientProposals); i++ {
				r.ProposeChan <- inst.lb.clientProposals[i]
			}
			inst.lb.clientProposals = nil
		}
	} else {
		// Reordered ACCEPT (same ballot)
		inst.cmds = accept.Command
		inst.status = ACCEPTED
		inst.IsMajorityCommitted = false
		areply = &AcceptReply{accept.Instance, 1, inst.ballot}
	}

	r.replyAccept(accept.LeaderId, areply)
}

func (r *Replica) handleCommit(commit *Commit) {
	inst := r.instanceSpace[commit.Instance]

	if commit.Instance >= r.crtInstance {
		r.crtInstance = commit.Instance + 1
	}

	if r.leaderCommitPoint < commit.MajorityCommitPoint {
		r.leaderCommitPoint = commit.MajorityCommitPoint
	}

	if inst == nil {
		r.instanceSpace[commit.Instance] = &Instance{
			cmds:   commit.Command,
			ballot: commit.Ballot,
			deps:   commit.Deps,
			status: COMMITTED,
			lb:     nil,
		}
	} else {
		inst.cmds = commit.Command
		inst.ballot = commit.Ballot
		inst.deps = commit.Deps
		inst.status = COMMITTED
		if inst.lb != nil && inst.lb.clientProposals != nil {
			for i := 0; i < len(inst.lb.clientProposals); i++ {
				r.ProposeChan <- inst.lb.clientProposals[i]
			}
			inst.lb.clientProposals = nil
		}
	}

	r.updateCommittedUpTo()

	// Send CommitAck back to leader
	r.replyCommitAck(commit.LeaderId, &CommitAck{commit.Instance})
}

func (r *Replica) handleCommitShort(commit *CommitShort) {
	inst := r.instanceSpace[commit.Instance]

	if commit.Instance >= r.crtInstance {
		r.crtInstance = commit.Instance + 1
	}

	if r.leaderCommitPoint < commit.MajorityCommitPoint {
		r.leaderCommitPoint = commit.MajorityCommitPoint
	}

	if inst == nil {
		r.instanceSpace[commit.Instance] = &Instance{
			cmds:   nil,
			ballot: commit.Ballot,
			status: COMMITTED,
			lb:     nil,
		}
	} else {
		inst.ballot = commit.Ballot
		inst.status = COMMITTED
		if inst.lb != nil && inst.lb.clientProposals != nil {
			for i := 0; i < len(inst.lb.clientProposals); i++ {
				r.ProposeChan <- inst.lb.clientProposals[i]
			}
			inst.lb.clientProposals = nil
		}
	}

	r.updateCommittedUpTo()

	// Send CommitAck back to leader
	r.replyCommitAck(commit.LeaderId, &CommitAck{commit.Instance})
}

func (r *Replica) handlePrepareReply(preply *PrepareReply) {
	inst := r.instanceSpace[preply.Instance]
	if inst == nil || inst.status != PREPARING {
		return
	}

	if preply.OK == 0 {
		inst.lb.nacks++
		if preply.Ballot > inst.lb.maxRecvBallot {
			inst.lb.maxRecvBallot = preply.Ballot
		}
		return
	}

	inst.lb.prepareOKs++

	// Majority of prepareOKs received
	if inst.lb.prepareOKs+1 > r.N>>1 {
		inst.status = PREPARED
		inst.lb.nacks = 0

		r.defaultBallot = inst.ballot

		r.bcastAccept(preply.Instance, inst.ballot, inst.deps, inst.cmds, r.majorityCommittedUpTo)
	}
}

func (r *Replica) handleAcceptReply(areply *AcceptReply) {
	inst := r.instanceSpace[areply.Instance]
	if inst == nil || inst.status != PREPARED {
		return
	}

	if areply.OK == 0 {
		inst.lb.nacks++
		if areply.Ballot > inst.lb.maxRecvBallot {
			inst.lb.maxRecvBallot = areply.Ballot
		}
		return
	}

	inst.lb.acceptOKs++

	// Majority of acceptOKs received → commit
	if inst.lb.acceptOKs+1 > r.N>>1 {
		inst.status = COMMITTED

		r.updateCommittedUpTo()

		// Reply to client if needed
		if inst.lb.clientProposals != nil && !r.Dreply {
			for i := 0; i < len(inst.cmds); i++ {
				propreply := &defs.ProposeReplyTS{
					OK:        defs.TRUE,
					CommandId: inst.lb.clientProposals[i].CommandId,
					Value:     state.NIL(),
					Timestamp: inst.lb.clientProposals[i].Timestamp,
				}
				r.ReplyProposeTS(propreply, inst.lb.clientProposals[i].Reply, inst.lb.clientProposals[i].Mutex)
			}
		}

		r.bcastStrongCommit(areply.Instance, inst.ballot, inst.deps, inst.cmds, r.majorityCommittedUpTo)
	}
}

func (r *Replica) handleCommitAck(commitAck *CommitAck) {
	inst := r.instanceSpace[commitAck.Instance]
	if inst == nil {
		return
	}

	inst.lb.commitOKs++

	// Majority ack → mark as majority committed
	if inst.lb.commitOKs+1 > r.N>>1 {
		inst.IsMajorityCommitted = true
	}
}

// --- Execution goroutines ---

func (r *Replica) executeWeakReadCommands() {
	const SLEEP_TIME_NS = 1000
	for !r.Shutdown {
		executed := false
		// Scan committed read instances starting from executedReadUpTo+1
		for i := r.executedReadUpTo + 1; i <= r.crtReadInstance; i++ {
			inst := r.instanceReadSpace[i]
			if inst == nil || inst.status != COMMITTED {
				break
			}

			// Check dependency: the write instance this read depends on must be executed
			if inst.deps >= 0 && inst.deps < int32(len(r.instanceSpace)) {
				depInst := r.instanceSpace[inst.deps]
				if depInst == nil || depInst.status != EXECUTED {
					break
				}
			}

			// Execute the read commands
			for j := 0; j < len(inst.cmds); j++ {
				val := inst.cmds[j].Execute(r.State)
				if inst.lb != nil && inst.lb.clientProposals != nil && j < len(inst.lb.clientProposals) && r.Dreply {
					propreply := &defs.ProposeReplyTS{
						OK:        defs.TRUE,
						CommandId: inst.lb.clientProposals[j].CommandId,
						Value:     val,
						Timestamp: inst.lb.clientProposals[j].Timestamp,
					}
					r.ReplyProposeTS(propreply, inst.lb.clientProposals[j].Reply, inst.lb.clientProposals[j].Mutex)
				}
			}
			inst.status = EXECUTED
			r.executedReadUpTo = i
			executed = true
		}

		if !executed {
			time.Sleep(SLEEP_TIME_NS)
		}
	}
}

func (r *Replica) executeWriteCommands() {
	const SLEEP_TIME_NS = 1000
	for !r.Shutdown {
		executed := false
		for i := r.executedUpTo + 1; i <= r.crtInstance; i++ {
			inst := r.instanceSpace[i]
			if inst == nil {
				break
			}

			if inst.status == COMMITTED || inst.status == EXECUTED {
				if inst.cmds[0].CL == state.STRONG || inst.cmds[0].CL == state.NONE {
					if !r.executeStrongCommands(i) {
						break
					}
					executed = true
				} else {
					if !r.executeWeakWriteCommands(i) {
						break
					}
					executed = true
				}
			} else {
				break
			}
		}

		if !executed {
			time.Sleep(SLEEP_TIME_NS)
		}
	}
}

// executeStrongCommands executes strong write/read commands in strict order.
func (r *Replica) executeStrongCommands(i int32) bool {
	inst := r.instanceSpace[i]
	if inst == nil || (inst.status != COMMITTED && inst.status != EXECUTED) {
		return false
	}

	// Strong commands must execute in order
	if i != r.executedUpTo+1 {
		return false
	}

	if inst.status == EXECUTED {
		r.executedUpTo = i
		return true
	}

	for j := 0; j < len(inst.cmds); j++ {
		val := inst.cmds[j].Execute(r.State)
		if inst.lb != nil && inst.lb.clientProposals != nil && j < len(inst.lb.clientProposals) && r.Dreply {
			propreply := &defs.ProposeReplyTS{
				OK:        defs.TRUE,
				CommandId: inst.lb.clientProposals[j].CommandId,
				Value:     val,
				Timestamp: inst.lb.clientProposals[j].Timestamp,
			}
			r.ReplyProposeTS(propreply, inst.lb.clientProposals[j].Reply, inst.lb.clientProposals[j].Mutex)
		}
	}
	inst.status = EXECUTED
	r.executedUpTo = i
	return true
}

// executeWeakWriteCommands executes causal write commands, checking dependencies.
func (r *Replica) executeWeakWriteCommands(i int32) bool {
	inst := r.instanceSpace[i]
	if inst == nil || (inst.status != COMMITTED && inst.status != EXECUTED) {
		return false
	}

	if inst.status == EXECUTED {
		if i == r.executedUpTo+1 {
			r.executedUpTo = i
		}
		return true
	}

	// Check dependency
	if inst.deps >= 0 && inst.deps < int32(len(r.instanceSpace)) {
		depInst := r.instanceSpace[inst.deps]
		if depInst == nil || (depInst.status != COMMITTED && depInst.status != EXECUTED) {
			return false
		}
	}

	for j := 0; j < len(inst.cmds); j++ {
		val := inst.cmds[j].Execute(r.State)
		if inst.lb != nil && inst.lb.clientProposals != nil && j < len(inst.lb.clientProposals) && r.Dreply {
			propreply := &defs.ProposeReplyTS{
				OK:        defs.TRUE,
				CommandId: inst.lb.clientProposals[j].CommandId,
				Value:     val,
				Timestamp: inst.lb.clientProposals[j].Timestamp,
			}
			r.ReplyProposeTS(propreply, inst.lb.clientProposals[j].Reply, inst.lb.clientProposals[j].Mutex)
		}
	}
	inst.status = EXECUTED
	if i == r.executedUpTo+1 {
		r.executedUpTo = i
	}
	return true
}

// executeWriteCommandsMajorityCommitted applies majority-committed commands to stable state.
func (r *Replica) executeWriteCommandsMajorityCommitted() {
	const SLEEP_TIME_NS = 1000
	for !r.Shutdown {
		executed := false
		for i := r.putExecutedUpToMajority + 1; i <= r.majorityCommittedUpTo; i++ {
			inst := r.instanceSpace[i]
			if inst == nil || !inst.IsMajorityCommitted {
				break
			}
			if inst.status != COMMITTED && inst.status != EXECUTED {
				break
			}

			// Strong commands must execute in order
			if inst.cmds[0].CL == state.STRONG || inst.cmds[0].CL == state.NONE {
				if i != r.putExecutedUpToMajority+1 {
					break
				}
			}

			// Execute on stable state (for strong reads that need majority-committed values)
			for j := 0; j < len(inst.cmds); j++ {
				if inst.cmds[j].Op == state.PUT {
					inst.cmds[j].Execute(r.State)
				}
			}
			r.putExecutedUpToMajority = i
			executed = true
		}

		if !executed {
			time.Sleep(SLEEP_TIME_NS)
		}
	}
}
