package raft

import (
	"sync"
	"time"

	"github.com/imdea-software/swiftpaxos/config"
	"github.com/imdea-software/swiftpaxos/dlog"
	"github.com/imdea-software/swiftpaxos/replica"
	"github.com/imdea-software/swiftpaxos/replica/defs"
	"github.com/imdea-software/swiftpaxos/state"
)

// Raft role states
const (
	FOLLOWER  = iota
	CANDIDATE
	LEADER
)

// LogEntry represents a single entry in the Raft replicated log.
// Each entry contains a command, the term when it was received by the leader,
// and the client command ID for reply routing.
type LogEntry struct {
	Command state.Command
	Term    int32
	CmdId   CommandId
}

// Replica implements the Raft consensus protocol.
type Replica struct {
	*replica.Replica // embedded base

	// Persistent state (on all servers)
	currentTerm int32
	votedFor    int32 // candidateId that received vote in current term, -1 if none
	log         []LogEntry

	// Volatile state (on all servers)
	commitIndex int32 // highest log entry known to be committed
	lastApplied int32 // highest log entry applied to state machine
	role        int   // FOLLOWER, CANDIDATE, or LEADER

	// Volatile state (on leaders, reinitialized after election)
	nextIndex  []int32 // for each server, index of the next log entry to send
	matchIndex []int32 // for each server, highest log entry known to be replicated

	// Pending client proposals awaiting commit (log index → proposal)
	pendingMu        sync.Mutex
	pendingProposals map[int32]*defs.GPropose

	// Election state
	votesReceived int
	votesNeeded   int

	// Communication
	cs     CommunicationSupply
	sender replica.Sender

	// Timers
	electionTimeout  time.Duration
	heartbeatTimeout time.Duration

	// Cache pools for message allocation
	appendEntriesCache      *AppendEntriesCache
	appendEntriesReplyCache *AppendEntriesReplyCache
	requestVoteCache        *RequestVoteCache
	requestVoteReplyCache   *RequestVoteReplyCache
	raftReplyCache          *RaftReplyCache
}

// New creates a new Raft replica.
// The isLeader flag from the master determines if this replica starts as leader at term 0.
func New(alias string, id int, addrs []string, isLeader bool, f int,
	conf *config.Config, logger *dlog.Logger) *Replica {

	n := len(addrs)

	r := &Replica{
		Replica: replica.New(alias, id, f, addrs, false, true, false, conf, logger),

		currentTerm: 0,
		votedFor:    -1,
		log:         make([]LogEntry, 0),

		commitIndex: -1,
		lastApplied: -1,
		role:        FOLLOWER,

		nextIndex:  make([]int32, n),
		matchIndex: make([]int32, n),

		pendingProposals: make(map[int32]*defs.GPropose),

		votesReceived: 0,
		votesNeeded:   (n / 2) + 1,

		appendEntriesCache:      NewAppendEntriesCache(),
		appendEntriesReplyCache: NewAppendEntriesReplyCache(),
		requestVoteCache:        NewRequestVoteCache(),
		requestVoteReplyCache:   NewRequestVoteReplyCache(),
		raftReplyCache:          NewRaftReplyCache(),
	}

	// Initialize leader volatile state
	for i := 0; i < n; i++ {
		r.nextIndex[i] = 0
		r.matchIndex[i] = -1
	}

	// Register message types with RPC table
	initCs(&r.cs, r.RPC)

	// Create async sender
	r.sender = replica.NewSender(r.Replica)

	// If designated as leader by master, become leader immediately at term 0
	if isLeader {
		r.BeTheLeader(nil, nil)
	}

	// Launch event loop
	go r.run()

	return r
}

// BeTheLeader is called by the master via RPC to designate this replica as leader.
// For Raft, this means transitioning to LEADER state and initializing leader state.
func (r *Replica) BeTheLeader(args *defs.BeTheLeaderArgs, reply *defs.BeTheLeaderReply) error {
	r.role = LEADER
	r.votedFor = r.Id

	// Initialize nextIndex and matchIndex for all peers
	lastLogIndex := int32(len(r.log) - 1)
	for i := 0; i < r.N; i++ {
		r.nextIndex[i] = lastLogIndex + 1
		r.matchIndex[i] = -1
	}
	// Leader knows its own match index
	r.matchIndex[r.Id] = lastLogIndex

	r.Println("I am the Raft leader at term", r.currentTerm)

	if reply != nil {
		reply.Leader = r.Id
		reply.NextLeader = r.Id
	}
	return nil
}

// run is the main event loop for the Raft replica.
// It handles all incoming messages and timer events in a single goroutine.
func (r *Replica) run() {
	r.ConnectToPeers()
	r.ComputeClosestPeers()

	go r.WaitForClientConnections()

	// TODO(39.2b): Implement event loop with election/heartbeat timers
	// TODO(39.2h): Launch executeCommands() goroutine

	for !r.Shutdown {
		select {
		case propose := <-r.ProposeChan:
			r.handlePropose(propose)

		case m := <-r.cs.appendEntriesChan:
			ae := m.(*AppendEntries)
			r.handleAppendEntries(ae)

		case m := <-r.cs.appendEntriesReplyChan:
			aer := m.(*AppendEntriesReply)
			r.handleAppendEntriesReply(aer)

		case m := <-r.cs.requestVoteChan:
			rv := m.(*RequestVote)
			r.handleRequestVote(rv)

		case m := <-r.cs.requestVoteReplyChan:
			rvr := m.(*RequestVoteReply)
			r.handleRequestVoteReply(rvr)
		}
	}
}

// --- Stub handlers (to be implemented in subsequent phases) ---

func (r *Replica) handlePropose(propose *defs.GPropose) {
	// TODO(39.2c): Batch proposals, append to log, broadcast AppendEntries
}

func (r *Replica) handleAppendEntries(msg *AppendEntries) {
	// TODO(39.2d): Term check, log matching, entry append, commitIndex advance
}

func (r *Replica) handleAppendEntriesReply(msg *AppendEntriesReply) {
	// TODO(39.2e): Update nextIndex/matchIndex, advance commitIndex, reply clients
}

func (r *Replica) handleRequestVote(msg *RequestVote) {
	// TODO(39.2f): Grant vote if term higher + log up-to-date
}

func (r *Replica) handleRequestVoteReply(msg *RequestVoteReply) {
	// TODO(39.2f): Count votes, become leader on majority
}
