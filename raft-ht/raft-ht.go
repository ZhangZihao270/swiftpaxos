package raftht

import (
	"math/rand"
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

// Replica implements the Raft-HT consensus protocol.
// Raft-HT extends vanilla Raft with weak (causal) operations:
// - Strong ops: unchanged Raft (2-RTT, linearizable)
// - Weak writes: leader assigns log slot, replies immediately (1-RTT)
// - Weak reads: any replica reads committed state (local)
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

	// Pending client proposals awaiting commit (log index → proposal).
	// Lock-free: event loop writes at append-time indices, executeCommands
	// reads+nils at committed indices. Non-overlapping due to happens-before
	// via commitNotify channel.
	pendingProposals []*defs.GPropose

	// Election state
	votesReceived int
	votesNeeded   int

	// Communication
	cs     CommunicationSupply
	sender replica.Sender

	// Timers
	electionTimeout  time.Duration
	heartbeatTimeout time.Duration

	// Replica identity and cluster size
	id int32
	n  int

	// Commit notification (replaces polling in executeCommands)
	commitNotify chan struct{}

	// Batching
	batchWait int // batch delay in microseconds (0 = disabled)

	// Cache pools for message allocation
	appendEntriesCache      *AppendEntriesCache
	appendEntriesReplyCache *AppendEntriesReplyCache
	requestVoteCache        *RequestVoteCache
	requestVoteReplyCache   *RequestVoteReplyCache
	raftReplyCache          *RaftReplyCache

	// Raft-HT: per-key version tracking for weak reads
	keyVersions map[int64]int32 // key → log index of last committed write
}

// New creates a new Raft-HT replica.
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

		id:         int32(id),
		n:          n,
		nextIndex:  make([]int32, n),
		matchIndex: make([]int32, n),

		pendingProposals: make([]*defs.GPropose, 0, 1024),

		commitNotify: make(chan struct{}, 1),

		votesReceived: 0,
		votesNeeded:   (n / 2) + 1,

		appendEntriesCache:      NewAppendEntriesCache(),
		appendEntriesReplyCache: NewAppendEntriesReplyCache(),
		requestVoteCache:        NewRequestVoteCache(),
		requestVoteReplyCache:   NewRequestVoteReplyCache(),
		raftReplyCache:          NewRaftReplyCache(),

		keyVersions: make(map[int64]int32),
	}

	// Set timer durations
	r.electionTimeout = time.Duration(300+rand.Intn(200)) * time.Millisecond
	r.heartbeatTimeout = 100 * time.Millisecond

	// Set batch delay from config
	if conf.BatchDelayUs > 0 {
		r.batchWait = conf.BatchDelayUs
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
	r.votedFor = r.id

	// Initialize nextIndex and matchIndex for all peers
	lastLogIndex := int32(len(r.log) - 1)
	for i := 0; i < r.n; i++ {
		r.nextIndex[i] = lastLogIndex + 1
		r.matchIndex[i] = -1
	}
	// Leader knows its own match index
	r.matchIndex[r.id] = lastLogIndex

	r.println("I am the Raft-HT leader at term", r.currentTerm)

	if reply != nil {
		reply.Leader = r.id
		reply.NextLeader = r.id
	}
	return nil
}

// run is the main event loop for the Raft-HT replica.
// All message handling and timer events are processed in this single goroutine.
func (r *Replica) run() {
	r.ConnectToPeers()
	r.ComputeClosestPeers()

	// Launch command execution goroutine
	go r.executeCommands()

	// Set up batch timer
	var batchClockChan chan bool
	if r.batchWait > 0 {
		batchClockChan = make(chan bool, 1)
		go func() {
			for !r.Shutdown {
				time.Sleep(time.Duration(r.batchWait) * time.Microsecond)
				batchClockChan <- true
			}
		}()
	}

	// Set up election and heartbeat timers
	electionTimer := time.NewTimer(r.electionTimeout)
	heartbeatTimer := time.NewTimer(r.heartbeatTimeout)

	// Leader doesn't need election timer; followers don't need heartbeat timer
	if r.role == LEADER {
		electionTimer.Stop()
	} else {
		heartbeatTimer.Stop()
	}

	onOffProposeChan := r.ProposeChan

	go r.WaitForClientConnections()

	for !r.Shutdown {
		select {
		case propose := <-onOffProposeChan:
			r.handlePropose(propose)
			if r.batchWait > 0 {
				onOffProposeChan = nil
			}

		case <-batchClockChan:
			onOffProposeChan = r.ProposeChan

		case m := <-r.cs.appendEntriesChan:
			ae := m.(*AppendEntries)
			r.handleAppendEntries(ae)
			// Reset election timer on valid AppendEntries (leader is alive)
			if ae.Term >= r.currentTerm {
				r.resetElectionTimer(electionTimer)
			}

		case m := <-r.cs.appendEntriesReplyChan:
			aer := m.(*AppendEntriesReply)
			r.handleAppendEntriesReply(aer)

		case m := <-r.cs.requestVoteChan:
			rv := m.(*RequestVote)
			r.handleRequestVote(rv)

		case m := <-r.cs.requestVoteReplyChan:
			rvr := m.(*RequestVoteReply)
			r.handleRequestVoteReply(rvr)

		// Raft-HT: weak write proposals (leader only)
		case m := <-r.cs.weakProposeChan:
			wp := m.(*MWeakPropose)
			r.handleWeakPropose(wp)

		// Raft-HT: weak read requests (any replica)
		case m := <-r.cs.weakReadChan:
			wr := m.(*MWeakRead)
			r.handleWeakRead(wr)

		case <-electionTimer.C:
			if r.role != LEADER {
				r.startElection()
				r.resetElectionTimer(electionTimer)
			}

		case <-heartbeatTimer.C:
			if r.role == LEADER {
				r.sendHeartbeats()
				heartbeatTimer.Reset(r.heartbeatTimeout)
			}
		}
	}
}

// println logs a message if the base replica is available.
func (r *Replica) println(v ...interface{}) {
	if r.Replica != nil {
		r.Replica.Println(v...)
	}
}

// resetElectionTimer resets the election timer with a randomized timeout.
func (r *Replica) resetElectionTimer(t *time.Timer) {
	timeout := time.Duration(300+rand.Intn(200)) * time.Millisecond
	t.Reset(timeout)
}

// becomeFollower transitions to follower state for a new term.
func (r *Replica) becomeFollower(term int32) {
	r.currentTerm = term
	r.role = FOLLOWER
	r.votedFor = -1
	r.votesReceived = 0
}

// becomeLeader transitions to leader state after winning an election.
func (r *Replica) becomeLeader() {
	r.role = LEADER
	r.println("Became Raft-HT leader at term", r.currentTerm)

	lastLogIndex := int32(len(r.log) - 1)
	for i := 0; i < r.n; i++ {
		r.nextIndex[i] = lastLogIndex + 1
		r.matchIndex[i] = -1
	}
	r.matchIndex[r.id] = lastLogIndex

	// Send immediate heartbeat to assert authority
	if r.Replica != nil {
		r.sendHeartbeats()
	}
}

// --- handlePropose: Batch proposals, append to log, broadcast AppendEntries ---

func (r *Replica) handlePropose(propose *defs.GPropose) {
	if r.role != LEADER {
		// Reject: only leader accepts proposals
		preply := &defs.ProposeReplyTS{
			OK:        defs.FALSE,
			CommandId: propose.CommandId,
			Value:     state.NIL(),
			Timestamp: propose.Timestamp,
		}
		r.ReplyProposeTS(preply, propose.Reply, propose.Mutex)
		return
	}

	// Batch: drain all queued proposals
	batchSize := len(r.ProposeChan) + 1
	proposals := make([]*defs.GPropose, batchSize)
	proposals[0] = propose
	for i := 1; i < batchSize; i++ {
		proposals[i] = <-r.ProposeChan
	}

	// Append entries to log
	entries := make([]state.Command, batchSize)
	entryIds := make([]CommandId, batchSize)

	for i, p := range proposals {
		cmdId := CommandId{ClientId: p.ClientId, SeqNum: p.CommandId}
		entry := LogEntry{
			Command: p.Command,
			Term:    r.currentTerm,
			CmdId:   cmdId,
		}
		r.log = append(r.log, entry)
		entries[i] = p.Command
		entryIds[i] = cmdId

		// Store pending proposal for reply on commit.
		logIdx := int32(len(r.log) - 1)
		for int32(len(r.pendingProposals)) <= logIdx {
			r.pendingProposals = append(r.pendingProposals, nil)
		}
		r.pendingProposals[logIdx] = p
	}

	// Update leader's own matchIndex
	r.matchIndex[r.id] = int32(len(r.log) - 1)

	// Broadcast AppendEntries to all followers
	r.broadcastAppendEntries()
}

// broadcastAppendEntries sends AppendEntries RPCs to all followers.
func (r *Replica) broadcastAppendEntries() {
	for i := int32(0); i < int32(r.n); i++ {
		if i == r.id {
			continue
		}
		ae := r.buildAppendEntries(i)
		r.SendMsgNoFlush(i, r.cs.appendEntriesRPC, ae)
	}
	r.FlushPeers()
}

// sendAppendEntries sends an AppendEntries RPC to a specific follower.
func (r *Replica) sendAppendEntries(peerId int32) {
	ae := r.buildAppendEntries(peerId)
	r.sender.SendTo(peerId, ae, r.cs.appendEntriesRPC)
}

// buildAppendEntries constructs an AppendEntries message for the given follower.
func (r *Replica) buildAppendEntries(peerId int32) *AppendEntries {
	nextIdx := r.nextIndex[peerId]
	if nextIdx < 0 {
		nextIdx = 0
	}

	prevLogIndex := nextIdx - 1
	prevLogTerm := int32(0)
	if prevLogIndex >= 0 && prevLogIndex < int32(len(r.log)) {
		prevLogTerm = r.log[prevLogIndex].Term
	}

	// Collect entries from nextIndex to end of log
	var entries []state.Command
	var entryIds []CommandId
	if nextIdx < int32(len(r.log)) {
		count := int32(len(r.log)) - nextIdx
		entries = make([]state.Command, count)
		entryIds = make([]CommandId, count)
		for j := int32(0); j < count; j++ {
			entries[j] = r.log[nextIdx+j].Command
			entryIds[j] = r.log[nextIdx+j].CmdId
		}
	}

	ae := r.appendEntriesCache.Get()
	ae.LeaderId = r.id
	ae.Term = r.currentTerm
	ae.PrevLogIndex = prevLogIndex
	ae.PrevLogTerm = prevLogTerm
	ae.LeaderCommit = r.commitIndex
	ae.EntryCnt = int32(len(entries))
	ae.Entries = entries
	ae.EntryIds = entryIds

	return ae
}

// --- handleAppendEntries: Term check, log matching, entry append, commitIndex advance ---

func (r *Replica) handleAppendEntries(msg *AppendEntries) {
	// Reply false if term < currentTerm (§5.1)
	if msg.Term < r.currentTerm {
		reply := r.appendEntriesReplyCache.Get()
		reply.FollowerId = r.id
		reply.Term = r.currentTerm
		reply.Success = 0
		reply.MatchIndex = -1
		r.sender.SendTo(msg.LeaderId, reply, r.cs.appendEntriesReplyRPC)
		return
	}

	// If term > currentTerm, step down
	if msg.Term > r.currentTerm {
		r.becomeFollower(msg.Term)
	} else if r.role == CANDIDATE {
		// Same term but valid leader exists — step down from candidacy
		r.role = FOLLOWER
		r.votesReceived = 0
	}

	// Log consistency check: verify entry at PrevLogIndex has matching term
	if msg.PrevLogIndex >= 0 {
		if msg.PrevLogIndex >= int32(len(r.log)) {
			// Log too short
			reply := r.appendEntriesReplyCache.Get()
			reply.FollowerId = r.id
			reply.Term = r.currentTerm
			reply.Success = 0
			reply.MatchIndex = int32(len(r.log) - 1)
			r.sender.SendTo(msg.LeaderId, reply, r.cs.appendEntriesReplyRPC)
			return
		}
		if r.log[msg.PrevLogIndex].Term != msg.PrevLogTerm {
			// Term mismatch: delete this entry and all that follow (§5.3)
			r.log = r.log[:msg.PrevLogIndex]
			reply := r.appendEntriesReplyCache.Get()
			reply.FollowerId = r.id
			reply.Term = r.currentTerm
			reply.Success = 0
			reply.MatchIndex = int32(len(r.log) - 1)
			r.sender.SendTo(msg.LeaderId, reply, r.cs.appendEntriesReplyRPC)
			return
		}
	}

	// Append new entries (not already in the log)
	insertIdx := msg.PrevLogIndex + 1
	for i := 0; i < len(msg.Entries); i++ {
		logIdx := insertIdx + int32(i)
		if logIdx < int32(len(r.log)) {
			if r.log[logIdx].Term != msg.Term {
				// Conflict: truncate from here
				r.log = r.log[:logIdx]
			} else {
				continue // already have this entry
			}
		}
		// Append new entry
		entry := LogEntry{
			Command: msg.Entries[i],
			Term:    msg.Term,
		}
		if i < len(msg.EntryIds) {
			entry.CmdId = msg.EntryIds[i]
		}
		r.log = append(r.log, entry)
	}

	// Advance commitIndex if leader's commit is ahead
	oldCommitIndex := r.commitIndex
	if msg.LeaderCommit > r.commitIndex {
		lastNewIndex := int32(len(r.log) - 1)
		if msg.LeaderCommit < lastNewIndex {
			r.commitIndex = msg.LeaderCommit
		} else {
			r.commitIndex = lastNewIndex
		}
		if r.commitIndex > oldCommitIndex {
			r.notifyCommit()
		}
	}

	// Reply success
	reply := r.appendEntriesReplyCache.Get()
	reply.FollowerId = r.id
	reply.Term = r.currentTerm
	reply.Success = 1
	reply.MatchIndex = int32(len(r.log) - 1)
	r.sender.SendTo(msg.LeaderId, reply, r.cs.appendEntriesReplyRPC)
}

// --- handleAppendEntriesReply: Update nextIndex/matchIndex, advance commitIndex ---

func (r *Replica) handleAppendEntriesReply(msg *AppendEntriesReply) {
	if r.role != LEADER {
		return
	}

	// If reply has higher term, step down
	if msg.Term > r.currentTerm {
		r.becomeFollower(msg.Term)
		return
	}

	if msg.Success == 1 {
		// Update nextIndex and matchIndex for follower
		if msg.MatchIndex >= r.matchIndex[msg.FollowerId] {
			r.matchIndex[msg.FollowerId] = msg.MatchIndex
			r.nextIndex[msg.FollowerId] = msg.MatchIndex + 1
		}
		// Try to advance commitIndex
		r.advanceCommitIndex()
	} else {
		// Decrement nextIndex and retry
		if msg.MatchIndex >= 0 {
			r.nextIndex[msg.FollowerId] = msg.MatchIndex + 1
		} else {
			r.nextIndex[msg.FollowerId] = 0
		}
		// Retry with earlier entries
		r.sendAppendEntries(msg.FollowerId)
	}
}

// advanceCommitIndex checks if any new entries can be committed.
// A log entry is committed when it has been replicated on a majority
// of servers AND its term equals the current term (§5.4.2).
func (r *Replica) advanceCommitIndex() {
	logLen := int32(len(r.log))
	advanced := false

	for candidate := r.commitIndex + 1; candidate < logLen; candidate++ {
		if r.log[candidate].Term != r.currentTerm {
			continue
		}
		count := 0
		for i := 0; i < r.n; i++ {
			if r.matchIndex[i] >= candidate {
				count++
			}
		}
		if count >= r.votesNeeded {
			r.commitIndex = candidate
			advanced = true
		} else {
			break
		}
	}

	if advanced {
		r.notifyCommit()
	}
}

// --- handleRequestVote: Grant vote if term higher + log up-to-date ---

func (r *Replica) handleRequestVote(msg *RequestVote) {
	// If candidate's term is stale, reject
	if msg.Term < r.currentTerm {
		reply := r.requestVoteReplyCache.Get()
		reply.VoterId = r.id
		reply.Term = r.currentTerm
		reply.VoteGranted = 0
		r.sender.SendTo(msg.CandidateId, reply, r.cs.requestVoteReplyRPC)
		return
	}

	// If term is higher, step down
	if msg.Term > r.currentTerm {
		r.becomeFollower(msg.Term)
	}

	// Grant vote if: haven't voted yet (or voted for this candidate)
	// AND candidate's log is at least as up-to-date as ours
	voteGranted := int32(0)
	if (r.votedFor == -1 || r.votedFor == msg.CandidateId) && r.isLogUpToDate(msg) {
		voteGranted = 1
		r.votedFor = msg.CandidateId
	}

	reply := r.requestVoteReplyCache.Get()
	reply.VoterId = r.id
	reply.Term = r.currentTerm
	reply.VoteGranted = voteGranted
	r.sender.SendTo(msg.CandidateId, reply, r.cs.requestVoteReplyRPC)
}

// isLogUpToDate checks if the candidate's log is at least as up-to-date as ours (§5.4.1).
func (r *Replica) isLogUpToDate(msg *RequestVote) bool {
	lastLogIndex := int32(len(r.log) - 1)
	lastLogTerm := int32(0)
	if lastLogIndex >= 0 {
		lastLogTerm = r.log[lastLogIndex].Term
	}

	if msg.LastLogTerm != lastLogTerm {
		return msg.LastLogTerm > lastLogTerm
	}
	return msg.LastLogIndex >= lastLogIndex
}

// --- handleRequestVoteReply: Count votes, become leader on majority ---

func (r *Replica) handleRequestVoteReply(msg *RequestVoteReply) {
	if r.role != CANDIDATE {
		return
	}

	// If reply has higher term, step down
	if msg.Term > r.currentTerm {
		r.becomeFollower(msg.Term)
		return
	}

	if msg.VoteGranted == 1 {
		r.votesReceived++
		if r.votesReceived >= r.votesNeeded {
			r.becomeLeader()
		}
	}
}

// --- startElection: Increment term, vote self, broadcast RequestVote ---

func (r *Replica) startElection() {
	r.currentTerm++
	r.role = CANDIDATE
	r.votedFor = r.id
	r.votesReceived = 1 // vote for self

	r.println("Starting election for term", r.currentTerm)

	lastLogIndex := int32(len(r.log) - 1)
	lastLogTerm := int32(0)
	if lastLogIndex >= 0 {
		lastLogTerm = r.log[lastLogIndex].Term
	}

	for i := int32(0); i < int32(r.n); i++ {
		if i == r.id {
			continue
		}
		rv := r.requestVoteCache.Get()
		rv.CandidateId = r.id
		rv.Term = r.currentTerm
		rv.LastLogIndex = lastLogIndex
		rv.LastLogTerm = lastLogTerm
		r.sender.SendTo(i, rv, r.cs.requestVoteRPC)
	}
}

// --- sendHeartbeats: Empty AppendEntries to all followers ---

func (r *Replica) sendHeartbeats() {
	r.broadcastAppendEntries()
}

// --- executeCommands: Apply committed entries, send reply for strong ops ---
// Also tracks keyVersions for weak read support.

func (r *Replica) executeCommands() {
	for !r.Shutdown {
		for r.lastApplied < r.commitIndex {
			r.lastApplied++
			idx := r.lastApplied

			if idx < 0 || idx >= int32(len(r.log)) {
				break
			}

			entry := r.log[idx]
			val := entry.Command.Execute(r.State)

			// Track per-key version for weak reads
			if entry.Command.Op == state.PUT {
				r.keyVersions[int64(entry.Command.K)] = idx
			}

			// If we're leader and have a pending proposal for this index, reply to client
			var propose *defs.GPropose
			if idx < int32(len(r.pendingProposals)) {
				propose = r.pendingProposals[idx]
				r.pendingProposals[idx] = nil // release for GC
			}

			if propose != nil {
				propreply := &defs.ProposeReplyTS{
					OK:        defs.TRUE,
					CommandId: propose.CommandId,
					Value:     val,
					Timestamp: propose.Timestamp,
				}
				r.ReplyProposeTS(propreply, propose.Reply, propose.Mutex)
			}
		}

		// Block until commitIndex advances instead of polling
		<-r.commitNotify
	}
}

// notifyCommit wakes executeCommands after commitIndex advances.
// Non-blocking send: if a notification is already pending, skip.
func (r *Replica) notifyCommit() {
	select {
	case r.commitNotify <- struct{}{}:
	default:
	}
}

// --- Raft-HT: Weak Write Path ---

// handleWeakPropose handles a weak write from a client.
// The leader assigns a log slot, replies immediately, and replicates in background.
func (r *Replica) handleWeakPropose(propose *MWeakPropose) {
	if r.role != LEADER {
		// Only leader handles weak writes; silently drop on followers
		return
	}

	// Append to log (same log as strong ops — implicit ordering for C1-C3)
	entry := LogEntry{
		Command: propose.Command,
		Term:    r.currentTerm,
		CmdId:   CommandId{ClientId: propose.ClientId, SeqNum: propose.CommandId},
	}
	idx := int32(len(r.log))
	r.log = append(r.log, entry)

	// Ensure pendingProposals is large enough (no pending proposal for weak writes)
	for int32(len(r.pendingProposals)) <= idx {
		r.pendingProposals = append(r.pendingProposals, nil)
	}

	// Update leader's own matchIndex
	r.matchIndex[r.id] = int32(len(r.log) - 1)

	// Reply IMMEDIATELY — don't wait for commit (1 WAN RTT)
	reply := &MWeakReply{
		LeaderId: r.id,
		Term:     r.currentTerm,
		CmdId:    entry.CmdId,
		Slot:     idx,
	}
	r.sender.SendToClient(propose.ClientId, reply, r.cs.weakReplyRPC)

	// Trigger replication via normal AppendEntries
	r.broadcastAppendEntries()
}

// --- Raft-HT: Weak Read Path ---

// handleWeakRead handles a weak read from a client.
// Any replica (including followers) can serve weak reads from committed state.
func (r *Replica) handleWeakRead(msg *MWeakRead) {
	// Read from committed state machine
	cmd := state.Command{Op: state.GET, K: msg.Key, V: state.NIL()}
	value := cmd.Execute(r.State)

	// Look up version (log index of last committed write to this key)
	version := int32(0)
	if v, ok := r.keyVersions[int64(msg.Key)]; ok {
		version = v
	}

	reply := &MWeakReadReply{
		Replica: r.id,
		Term:    r.currentTerm,
		CmdId:   CommandId{ClientId: msg.ClientId, SeqNum: msg.CommandId},
		Rep:     value,
		Version: version,
	}
	r.sender.SendToClient(msg.ClientId, reply, r.cs.weakReadReplyRPC)
}
