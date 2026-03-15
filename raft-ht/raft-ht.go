package raftht

import (
	"math/rand"
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

// Replica implements the Raft-HT consensus protocol.
// Raft-HT extends vanilla Raft with weak (causal) operations:
// - Strong ops: unchanged Raft (2-RTT, linearizable)
// - Weak writes: leader assigns log slot, replies immediately (1-RTT), replicates async
// - Weak reads: leader assigns log slot, reads state, replies immediately (1-RTT), replicates async
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
	knownLeader int32 // best-known leader ID (from AppendEntries or self)

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
	electionTimer    *time.Timer
	heartbeatTimer   *time.Timer

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

	// Raft-HT: per-key version tracking for weak reads.
	// Protected by stateMu: executeCommands holds write lock, weak reads hold read lock.
	keyVersions map[int64]int32 // key → log index of last committed write

	// Protects r.log, r.commitIndex, r.lastApplied, and r.pendingProposals
	// for concurrent access between the event loop and executeCommands.
	// Event loop holds Lock() when appending to log and advancing commitIndex.
	// executeCommands holds Lock() when reading log/commitIndex and advancing lastApplied.
	logMu sync.Mutex

	// Protects r.State and r.keyVersions for concurrent access.
	// executeCommands takes Lock() during command execution batches.
	// Weak reads take RLock() to read committed state concurrently.
	stateMu sync.RWMutex
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
		knownLeader: -1,

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
	r.knownLeader = r.id

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

	go r.weakReadLoop()

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

	// Set up election and heartbeat timers.
	// Followers use a longer initial timeout (3s) to allow the designated leader
	// to finish ConnectToPeers and send its first heartbeat before any follower
	// starts an election. After the first heartbeat, normal timeouts apply.
	initialElectionTimeout := 3 * time.Second
	r.electionTimer = time.NewTimer(initialElectionTimeout)
	r.heartbeatTimer = time.NewTimer(r.heartbeatTimeout)

	// Leader doesn't need election timer; followers don't need heartbeat timer.
	// Send immediate heartbeat after peer connections are established to prevent
	// followers from starting elections during the startup window.
	if r.role == LEADER {
		r.electionTimer.Stop()
		r.sendHeartbeats()
	} else {
		r.heartbeatTimer.Stop()
	}

	onOffProposeChan := r.ProposeChan
	onOffWeakProposeChan := r.cs.weakProposeChan

	go r.WaitForClientConnections()

	for !r.Shutdown {
		select {
		case propose := <-onOffProposeChan:
			r.handleAllProposals(propose, nil)
			if r.batchWait > 0 {
				onOffProposeChan = nil
				onOffWeakProposeChan = nil
			}

		case m := <-onOffWeakProposeChan:
			wp := m.(*MWeakPropose)
			r.handleAllProposals(nil, wp)
			if r.batchWait > 0 {
				onOffProposeChan = nil
				onOffWeakProposeChan = nil
			}

		case <-batchClockChan:
			onOffProposeChan = r.ProposeChan
			onOffWeakProposeChan = r.cs.weakProposeChan

		case m := <-r.cs.appendEntriesChan:
			ae := m.(*AppendEntries)
			r.handleAppendEntries(ae)
			// Reset election timer on valid AppendEntries (leader is alive)
			if ae.Term >= r.currentTerm {
				r.resetElectionTimer()
			}

		case m := <-r.cs.appendEntriesReplyChan:
			aer := m.(*AppendEntriesReply)
			r.handleAppendEntriesReplyBatch(aer)

		case m := <-r.cs.requestVoteChan:
			rv := m.(*RequestVote)
			r.handleRequestVote(rv)

		case m := <-r.cs.requestVoteReplyChan:
			rvr := m.(*RequestVoteReply)
			r.handleRequestVoteReply(rvr)

		case <-r.electionTimer.C:
			if r.role != LEADER {
				r.startElection()
				r.resetElectionTimer()
			}

		case <-r.heartbeatTimer.C:
			if r.role == LEADER {
				r.sendHeartbeats()
				r.heartbeatTimer.Reset(r.heartbeatTimeout)
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
func (r *Replica) resetElectionTimer() {
	timeout := time.Duration(300+rand.Intn(200)) * time.Millisecond
	r.electionTimer.Reset(timeout)
}

// becomeFollower transitions to follower state for a new term.
func (r *Replica) becomeFollower(term int32) {
	r.currentTerm = term
	r.role = FOLLOWER
	r.votedFor = -1
	r.votesReceived = 0
	// Stop heartbeat timer (no longer leader) and start election timer
	if r.heartbeatTimer != nil {
		r.heartbeatTimer.Stop()
	}
	if r.electionTimer != nil {
		r.resetElectionTimer()
	}
}

// becomeLeader transitions to leader state after winning an election.
func (r *Replica) becomeLeader() {
	r.role = LEADER
	r.knownLeader = r.id
	r.println("Became Raft-HT leader at term", r.currentTerm)

	lastLogIndex := int32(len(r.log) - 1)
	for i := 0; i < r.n; i++ {
		r.nextIndex[i] = lastLogIndex + 1
		r.matchIndex[i] = -1
	}
	r.matchIndex[r.id] = lastLogIndex

	// Stop election timer (now leader) and start heartbeat timer
	if r.electionTimer != nil {
		r.electionTimer.Stop()
	}
	if r.heartbeatTimer != nil {
		r.heartbeatTimer.Reset(r.heartbeatTimeout)
	}

	// Send immediate heartbeat to assert authority
	if r.Replica != nil {
		r.sendHeartbeats()
	}
}

// maxBatchSize caps proposals processed per event-loop iteration to prevent
// the event loop from blocking long enough to trigger follower election timeouts.
// 256 entries ≈ 50KB payload, safely under 1ms processing time.
const maxBatchSize = 256

// --- handleAllProposals: Unified strong+weak proposal handling with single broadcast ---
// Drains BOTH ProposeChan and weakProposeChan to coalesce into a single
// broadcastAppendEntries call, eliminating the double-broadcast problem.

func (r *Replica) handleAllProposals(firstStrong *defs.GPropose, firstWeak *MWeakPropose) {
	if r.role != LEADER {
		if firstStrong != nil {
			reply := r.raftReplyCache.Get()
			reply.CmdId = CommandId{ClientId: firstStrong.ClientId, SeqNum: firstStrong.CommandId}
			reply.Value = state.NIL()
			reply.LeaderId = r.knownLeader
			r.sender.SendToClient(firstStrong.ClientId, reply, r.cs.raftReplyRPC)
		}
		if firstWeak != nil {
			reply := &MWeakReply{
				LeaderId: r.knownLeader,
				Term:     r.currentTerm,
				CmdId:    CommandId{ClientId: firstWeak.ClientId, SeqNum: firstWeak.CommandId},
				Slot:     -1,
			}
			r.sender.SendToClient(firstWeak.ClientId, reply, r.cs.weakReplyRPC)
		}
		return
	}

	// Collect first proposals
	strongs := make([]*defs.GPropose, 0, maxBatchSize)
	weaks := make([]*MWeakPropose, 0, maxBatchSize)
	if firstStrong != nil {
		strongs = append(strongs, firstStrong)
	}
	if firstWeak != nil {
		weaks = append(weaks, firstWeak)
	}

	// Drain both channels up to maxBatchSize total
	total := len(strongs) + len(weaks)
	// Drain strong proposals
	for total < maxBatchSize {
		select {
		case p := <-r.ProposeChan:
			strongs = append(strongs, p)
			total++
		default:
			goto drainWeak
		}
	}
drainWeak:
	// Drain weak proposals
	for total < maxBatchSize {
		select {
		case m := <-r.cs.weakProposeChan:
			weaks = append(weaks, m.(*MWeakPropose))
			total++
		default:
			goto drained
		}
	}
drained:

	if len(strongs) == 0 && len(weaks) == 0 {
		return
	}

	// Append all entries under logMu
	type weakEntry struct {
		cmdId CommandId
		idx   int32
		wp    *MWeakPropose
	}
	weakBatch := make([]weakEntry, 0, len(weaks))

	r.logMu.Lock()
	// Strong entries (reply on commit)
	for _, p := range strongs {
		cmdId := CommandId{ClientId: p.ClientId, SeqNum: p.CommandId}
		entry := LogEntry{
			Command: p.Command,
			Term:    r.currentTerm,
			CmdId:   cmdId,
		}
		r.log = append(r.log, entry)
		logIdx := int32(len(r.log) - 1)
		for int32(len(r.pendingProposals)) <= logIdx {
			r.pendingProposals = append(r.pendingProposals, nil)
		}
		r.pendingProposals[logIdx] = p
	}
	// Weak entries (reply immediately, no pendingProposal)
	for _, wp := range weaks {
		cmdId := CommandId{ClientId: wp.ClientId, SeqNum: wp.CommandId}
		entry := LogEntry{
			Command: wp.Command,
			Term:    r.currentTerm,
			CmdId:   cmdId,
		}
		idx := int32(len(r.log))
		r.log = append(r.log, entry)
		for int32(len(r.pendingProposals)) <= idx {
			r.pendingProposals = append(r.pendingProposals, nil)
		}
		weakBatch = append(weakBatch, weakEntry{cmdId: cmdId, idx: idx, wp: wp})
	}
	r.matchIndex[r.id] = int32(len(r.log) - 1)
	r.logMu.Unlock()

	// Reply IMMEDIATELY for weak entries — don't wait for commit
	for _, we := range weakBatch {
		reply := &MWeakReply{
			LeaderId: r.id,
			Term:     r.currentTerm,
			CmdId:    we.cmdId,
			Slot:     we.idx,
		}
		r.sender.SendToClient(we.wp.ClientId, reply, r.cs.weakReplyRPC)
	}

	// Single broadcast for ALL entries (strong + weak)
	r.broadcastAppendEntries()
}

// broadcastAppendEntries sends AppendEntries RPCs to all followers.
func (r *Replica) broadcastAppendEntries() {
	// Build all messages under a single logMu hold (was 4 separate acquisitions).
	// When all followers are caught up (common case), entries/entryIds are shared.
	type cachedEntries struct {
		entries  []state.Command
		entryIds []CommandId
		prevTerm int32
	}

	msgs := make([]*AppendEntries, r.n)
	cache := make(map[int32]*cachedEntries, 4)

	r.logMu.Lock()
	logLen := int32(len(r.log))
	commitIdx := r.commitIndex

	for i := int32(0); i < int32(r.n); i++ {
		if i == r.id {
			continue
		}
		nextIdx := r.nextIndex[i]
		if nextIdx < 0 {
			nextIdx = 0
		}
		prevLogIndex := nextIdx - 1

		ce, ok := cache[nextIdx]
		if !ok {
			ce = &cachedEntries{}
			if prevLogIndex >= 0 && prevLogIndex < logLen {
				ce.prevTerm = r.log[prevLogIndex].Term
			}
			if nextIdx < logLen {
				count := logLen - nextIdx
				ce.entries = make([]state.Command, count)
				ce.entryIds = make([]CommandId, count)
				for j := int32(0); j < count; j++ {
					ce.entries[j] = r.log[nextIdx+j].Command
					ce.entryIds[j] = r.log[nextIdx+j].CmdId
				}
			}
			cache[nextIdx] = ce
		}

		ae := r.appendEntriesCache.Get()
		ae.LeaderId = r.id
		ae.Term = r.currentTerm
		ae.PrevLogIndex = prevLogIndex
		ae.PrevLogTerm = ce.prevTerm
		ae.LeaderCommit = commitIdx
		ae.EntryCnt = int32(len(ce.entries))
		ae.Entries = ce.entries
		ae.EntryIds = ce.entryIds
		msgs[i] = ae
	}
	r.logMu.Unlock()

	// Batched synchronous send: one lock acquisition for all writes + flush.
	r.M.Lock()
	for i := int32(0); i < int32(r.n); i++ {
		if i == r.id || msgs[i] == nil {
			continue
		}
		w := r.PeerWriters[i]
		if w == nil {
			continue
		}
		w.WriteByte(r.cs.appendEntriesRPC)
		msgs[i].Marshal(w)
	}
	for _, w := range r.PeerWriters {
		if w != nil {
			w.Flush()
		}
	}
	r.M.Unlock()
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

	// Snapshot log state under logMu (executeCommands reads concurrently).
	r.logMu.Lock()
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
	commitIdx := r.commitIndex
	r.logMu.Unlock()

	ae := r.appendEntriesCache.Get()
	ae.LeaderId = r.id
	ae.Term = r.currentTerm
	ae.PrevLogIndex = prevLogIndex
	ae.PrevLogTerm = prevLogTerm
	ae.LeaderCommit = commitIdx
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
	r.knownLeader = msg.LeaderId

	// Log consistency check, append, and commit under logMu for executeCommands safety.
	r.logMu.Lock()

	if msg.PrevLogIndex >= 0 {
		if msg.PrevLogIndex >= int32(len(r.log)) {
			// Log too short
			matchIdx := int32(len(r.log) - 1)
			r.logMu.Unlock()
			reply := r.appendEntriesReplyCache.Get()
			reply.FollowerId = r.id
			reply.Term = r.currentTerm
			reply.Success = 0
			reply.MatchIndex = matchIdx
			r.sender.SendTo(msg.LeaderId, reply, r.cs.appendEntriesReplyRPC)
			return
		}
		if r.log[msg.PrevLogIndex].Term != msg.PrevLogTerm {
			// Term mismatch: delete this entry and all that follow (§5.3)
			r.log = r.log[:msg.PrevLogIndex]
			matchIdx := int32(len(r.log) - 1)
			r.logMu.Unlock()
			reply := r.appendEntriesReplyCache.Get()
			reply.FollowerId = r.id
			reply.Term = r.currentTerm
			reply.Success = 0
			reply.MatchIndex = matchIdx
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
	}
	advanced := r.commitIndex > oldCommitIndex
	matchIdx := int32(len(r.log) - 1)
	r.logMu.Unlock()

	if advanced {
		r.notifyCommit()
	}

	// Reply success
	reply := r.appendEntriesReplyCache.Get()
	reply.FollowerId = r.id
	reply.Term = r.currentTerm
	reply.Success = 1
	reply.MatchIndex = matchIdx
	r.sender.SendTo(msg.LeaderId, reply, r.cs.appendEntriesReplyRPC)
}

// handleAppendEntriesReplyBatch drains all pending replies before calling
// advanceCommitIndex once, reducing logMu contention.
func (r *Replica) handleAppendEntriesReplyBatch(first *AppendEntriesReply) {
	r.applyReplyUpdate(first)

	// Drain additional replies
	for {
		select {
		case m := <-r.cs.appendEntriesReplyChan:
			r.applyReplyUpdate(m.(*AppendEntriesReply))
		default:
			goto done
		}
	}
done:
	if r.role == LEADER {
		r.advanceCommitIndex()
	}
}

// applyReplyUpdate processes a single AppendEntriesReply without calling advanceCommitIndex.
func (r *Replica) applyReplyUpdate(msg *AppendEntriesReply) {
	if r.role != LEADER {
		return
	}
	if msg.Term > r.currentTerm {
		r.becomeFollower(msg.Term)
		return
	}
	if msg.Success == 1 {
		if msg.MatchIndex >= r.matchIndex[msg.FollowerId] {
			r.matchIndex[msg.FollowerId] = msg.MatchIndex
			r.nextIndex[msg.FollowerId] = msg.MatchIndex + 1
		}
	} else {
		if msg.MatchIndex >= 0 {
			r.nextIndex[msg.FollowerId] = msg.MatchIndex + 1
		} else {
			r.nextIndex[msg.FollowerId] = 0
		}
		r.sendAppendEntries(msg.FollowerId)
	}
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
	r.logMu.Lock()
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
	r.logMu.Unlock()

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
// Uses logMu to safely snapshot committed log entries from the event loop,
// then holds stateMu write lock during execution so weak reads can
// concurrently read committed state via stateMu read lock.

type pendingEntry struct {
	entry   LogEntry
	idx     int32
	propose *defs.GPropose
}

func (r *Replica) executeCommands() {
	for !r.Shutdown {
		// Snapshot committed entries under logMu (brief lock, no I/O).
		r.logMu.Lock()
		var batch []pendingEntry
		for r.lastApplied < r.commitIndex {
			r.lastApplied++
			idx := r.lastApplied
			if idx < 0 || idx >= int32(len(r.log)) {
				r.lastApplied--
				break
			}
			pe := pendingEntry{entry: r.log[idx], idx: idx}
			if idx < int32(len(r.pendingProposals)) {
				pe.propose = r.pendingProposals[idx]
				r.pendingProposals[idx] = nil
			}
			batch = append(batch, pe)
		}
		r.logMu.Unlock()

		// Execute batch under stateMu (protects r.State and r.keyVersions).
		if len(batch) > 0 {
			r.stateMu.Lock()
			for _, pe := range batch {
				val := pe.entry.Command.Execute(r.State)
				if pe.entry.Command.Op == state.PUT {
					r.keyVersions[int64(pe.entry.Command.K)] = pe.idx
				}
				if pe.propose != nil {
					reply := r.raftReplyCache.Get()
					reply.CmdId = CommandId{ClientId: pe.propose.ClientId, SeqNum: pe.propose.CommandId}
					reply.Value = val
					reply.LeaderId = -1 // success: no redirect needed
					r.sender.SendToClient(pe.propose.ClientId, reply, r.cs.raftReplyRPC)
				}
			}
			r.stateMu.Unlock()
		}

		// Block until commitIndex advances
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

// --- Raft-HT: Weak Read Path ---

// weakReadLoop processes weak reads in a dedicated goroutine,
// decoupled from both the event loop and executeCommands.
// Uses stateMu read lock to safely read committed state concurrently
// with executeCommands' write lock.
func (r *Replica) weakReadLoop() {
	for !r.Shutdown {
		m, ok := <-r.cs.weakReadChan
		if !ok {
			return
		}
		msg := m.(*MWeakRead)
		r.processWeakRead(msg)
	}
}

// processWeakRead reads committed state and replies to client.
// Safe to call from any goroutine — acquires stateMu read lock.
func (r *Replica) processWeakRead(msg *MWeakRead) {
	r.stateMu.RLock()
	cmd := state.Command{Op: state.GET, K: msg.Key, V: state.NIL()}
	value := cmd.Execute(r.State)

	version := int32(0)
	if v, ok := r.keyVersions[int64(msg.Key)]; ok {
		version = v
	}
	r.stateMu.RUnlock()

	reply := &MWeakReadReply{
		Replica: r.id,
		Term:    0,
		CmdId:   CommandId{ClientId: msg.ClientId, SeqNum: msg.CommandId},
		Rep:     value,
		Version: version,
	}
	r.sender.SendToClient(msg.ClientId, reply, r.cs.weakReadReplyRPC)
}

// --- Raft-HT: Weak Write Path ---


