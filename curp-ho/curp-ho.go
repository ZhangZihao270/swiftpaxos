package curpho

import (
	"strconv"
	"sync"
	"time"

	"github.com/imdea-software/swiftpaxos/config"
	"github.com/imdea-software/swiftpaxos/dlog"
	"github.com/imdea-software/swiftpaxos/hook"
	"github.com/imdea-software/swiftpaxos/replica"
	"github.com/imdea-software/swiftpaxos/replica/defs"
	"github.com/imdea-software/swiftpaxos/state"
	"github.com/orcaman/concurrent-map"
)

type Replica struct {
	*replica.Replica

	ballot  int32
	cballot int32
	status  int

	optimized      bool
	contactClients bool

	Q replica.Majority

	isLeader    bool
	lastCmdSlot int

	slots     map[CommandId]int
	synced    cmap.ConcurrentMap
	values    cmap.ConcurrentMap
	proposes  cmap.ConcurrentMap
	cmdDescs  cmap.ConcurrentMap
	unsynced  cmap.ConcurrentMap
	executed  cmap.ConcurrentMap
	committed cmap.ConcurrentMap
	delivered cmap.ConcurrentMap

	sender  replica.Sender
	batcher *Batcher
	history []commandStaticDesc

	cs CommunicationSupply

	deliverChan chan int

	descPool     sync.Pool
	poolLevel    int
	routineCount int

	// Object pool for weak reply messages
	weakReplyPool sync.Pool

	// Track executed weak commands per client for causal ordering
	// Key: clientId, Value: last executed weak command seqnum
	weakExecuted cmap.ConcurrentMap

	// Track pending (uncommitted) writes per client for non-blocking speculative reads
	// Key: "clientId:key", Value: *pendingWrite
	pendingWrites cmap.ConcurrentMap

	// CURP-HO: Track which clients are bound to this replica.
	// A client binds to its closest replica for 1-RTT causal op replies.
	// Key: clientId, Value: true if bound to this replica.
	boundClients map[int32]bool

	// Notification channels for async waiting (replaces spin-waits)
	commitNotify  map[int]chan struct{} // slot -> notification channel for commit
	executeNotify map[int]chan struct{} // slot -> notification channel for execution
	notifyMu      sync.Mutex            // protects commitNotify and executeNotify

	// String conversion cache to avoid repeated strconv.FormatInt calls
	// Key: int32, Value: string representation
	stringCache sync.Map

	// Pre-allocated closed channel for immediate notifications (avoids repeated allocations)
	closedChan chan struct{}
}

// pendingWrite tracks an uncommitted write for speculative read computation
type pendingWrite struct {
	seqNum int32
	value  state.Value
}

type commandDesc struct {
	cmdId CommandId

	cmd     state.Command
	phase   int
	cmdSlot int
	propose *defs.GPropose
	val     []byte

	dep        int
	successor  int
	successorL sync.Mutex

	acks         *replica.MsgSet
	afterPayload *hook.OptCondF

	msgs   chan interface{}
	active bool
	seq    bool

	accepted    bool
	pendingCall func()

	isWeak  bool // Mark if this is a weak command
	applied bool // Track if command has been applied to state machine

	// Cached string keys to avoid repeated conversions
	slotStr  string // cached strconv.Itoa(cmdSlot)
	cmdIdStr string // cached cmdId.String()
}

type commandStaticDesc struct {
	cmdSlot int
	phase   int
	cmd     state.Command
}

func New(alias string, rid int, addrs []string, exec bool, pl, f int,
	opt bool, conf *config.Config, logger *dlog.Logger) *Replica {
	cmap.SHARD_COUNT = 32768

	r := &Replica{
		Replica: replica.New(alias, rid, f, addrs, false, exec, false, conf, logger),

		ballot:  0,
		cballot: 0,
		status:  NORMAL,

		optimized:      opt,
		contactClients: false,

		isLeader:    false,
		lastCmdSlot: 0,

		slots:     make(map[CommandId]int),
		synced:    cmap.New(),
		values:    cmap.New(),
		proposes:  cmap.New(),
		cmdDescs:  cmap.New(),
		unsynced:  cmap.New(),
		executed:  cmap.New(),
		committed: cmap.New(),
		delivered:     cmap.New(),
		weakExecuted:  cmap.New(),
		pendingWrites: cmap.New(),
		boundClients:  make(map[int32]bool),
		history:       make([]commandStaticDesc, HISTORY_SIZE),

		commitNotify:  make(map[int]chan struct{}),
		executeNotify: make(map[int]chan struct{}),

		deliverChan: make(chan int, defs.CHAN_BUFFER_SIZE),

		poolLevel:    pl,
		routineCount: 0,

		descPool: sync.Pool{
			New: func() interface{} {
				return &commandDesc{}
			},
		},

		weakReplyPool: sync.Pool{
			New: func() interface{} {
				return &MWeakReply{}
			},
		},
	}

	r.Q = replica.NewMajorityOf(r.N)
	r.sender = replica.NewSender(r.Replica)
	r.batcher = NewBatcher(r, 128) // Increased from 8 for better batching

	// Initialize pre-allocated closed channel for immediate notifications
	r.closedChan = make(chan struct{})
	close(r.closedChan)

	_, leaderIds, err := replica.NewQuorumsFromFile(conf.Quorum, r.Replica)
	if err == nil && len(leaderIds) != 0 {
		r.ballot = leaderIds[0]
		r.cballot = leaderIds[0]
		r.isLeader = (leaderIds[0] == r.Id)
	} else if err == replica.NO_QUORUM_FILE {
		r.isLeader = (r.ballot == r.Id)
	} else {
		r.Fatal(err)
	}

	initCs(&r.cs, r.RPC)

	hook.HookUser1(func() {
		totalNum := 0
		for i := 0; i < HISTORY_SIZE; i++ {
			if r.history[i].phase == 0 {
				continue
			}
			totalNum++
		}

		r.Printf("Total number of commands: %d\n", totalNum)
	})

	go r.run()

	return r
}

// BeTheLeader always returns 0 as the leader for CURP-HO.
// In CURP-HO, the leader is determined by the ballot (ballot=0 means replica 0 is leader).
func (r *Replica) BeTheLeader(args *defs.BeTheLeaderArgs, reply *defs.BeTheLeaderReply) error {
	reply.Leader = 0
	reply.NextLeader = 0
	return nil
}

func (r *Replica) run() {
	r.ConnectToPeers()
	latencies := r.ComputeClosestPeers()
	for _, l := range latencies {
		d := time.Duration(l*1000*1000) * time.Nanosecond
		if d > r.cs.maxLatency {
			r.cs.maxLatency = d
		}
	}

	go r.WaitForClientConnections()

	var cmdId CommandId
	for !r.Shutdown {
		select {
		case int := <-r.deliverChan:
			r.getCmdDesc(int, "deliver", -1)

		case propose := <-r.ProposeChan:
			if r.isLeader {
				proposeCmdId := CommandId{ClientId: propose.ClientId, SeqNum: propose.CommandId}
				dep := r.leaderUnsyncStrong(propose.Command, r.lastCmdSlot, proposeCmdId)
				desc := r.getCmdDescSeq(r.lastCmdSlot, propose, dep, true) // why Seq?
				if desc == nil {
					r.Fatal("Got propose for the delivered command:",
						propose.ClientId, propose.CommandId)
				}
				r.lastCmdSlot++
			} else {
				cmdId.ClientId = propose.ClientId
				cmdId.SeqNum = propose.CommandId
				if r.values.Has(cmdId.String()) {
					continue
				}
				r.proposes.Set(cmdId.String(), propose)
				ok, weakDep := r.okWithWeakDep(propose.Command)
				recAck := &MRecordAck{
					Replica: r.Id,
					Ballot:  r.ballot,
					CmdId:   cmdId,
					Ok:      ok,
					WeakDep: weakDep,
				}
				r.sender.SendToClient(propose.ClientId, recAck, r.cs.recordAckRPC)
				r.unsyncStrong(propose.Command, cmdId)
				slot, exists := r.slots[cmdId]
				if exists {
					r.getCmdDesc(slot, "deliver", -1)
				}
			}

		case m := <-r.cs.acceptChan:
			acc := m.(*MAccept)
			if r.values.Has(acc.CmdId.String()) {
				continue
			}
			r.slots[acc.CmdId] = acc.CmdSlot
			r.getCmdDesc(acc.CmdSlot, acc, -1)

		case m := <-r.cs.acceptAckChan:
			ack := m.(*MAcceptAck)
			r.getCmdDesc(ack.CmdSlot, ack, -1)

		case m := <-r.cs.aacksChan:
			aacks := m.(*MAAcks)
			for _, a := range aacks.Accepts {
				ta := a
				if r.values.Has(a.CmdId.String()) {
					continue
				}
				r.slots[a.CmdId] = a.CmdSlot
				r.getCmdDesc(a.CmdSlot, &ta, -1)
			}
			for _, b := range aacks.Acks {
				tb := b
				r.getCmdDesc(b.CmdSlot, &tb, -1)
			}

		case m := <-r.cs.commitChan:
			commit := m.(*MCommit)
			r.getCmdDesc(commit.CmdSlot, commit, -1)

		case m := <-r.cs.syncChan:
			sync := m.(*MSync)
			val, exists := r.values.Get(sync.CmdId.String())
			if exists {
				rep := &MSyncReply{
					Replica: r.Id,
					Ballot:  r.ballot,
					CmdId:   sync.CmdId,
					Rep:     val.([]byte),
				}
				r.sender.SendToClient(sync.CmdId.ClientId, rep, r.cs.syncReplyRPC)
			}

		case m := <-r.cs.weakProposeChan:
			if r.isLeader {
				weakPropose := m.(*MWeakPropose)
				r.handleWeakPropose(weakPropose)
			}
			// Non-Leader ignores weak propose (should not receive it)

		case m := <-r.cs.causalProposeChan:
			causalPropose := m.(*MCausalPropose)
			r.handleCausalPropose(causalPropose)
			// ALL replicas handle causal proposes (witness pool + reply)
		}
	}
}

func (r *Replica) handlePropose(msg *defs.GPropose, desc *commandDesc, slot int, dep int) {
	if r.status != NORMAL || desc.propose != nil {
		return
	}

	desc.propose = msg
	desc.cmd = msg.Command
	desc.cmdId = CommandId{
		ClientId: msg.ClientId,
		SeqNum:   msg.CommandId,
	}
	desc.cmdSlot = slot
	desc.dep = dep
	if dep != -1 {
		depDesc := r.getCmdDesc(dep, nil, -1)
		if depDesc != nil {
			depDesc.successorL.Lock()
			depDesc.successor = slot
			depDesc.successorL.Unlock()
		}
	}

	acc := &MAccept{
		Replica: r.Id,
		Ballot:  r.ballot,
		CmdId:   desc.cmdId,
		CmdSlot: slot,
	}

	r.deliver(desc, slot)
	r.batcher.SendAccept(acc)
	r.handleAccept(acc, desc)
}

func (r *Replica) handleAccept(msg *MAccept, desc *commandDesc) {
	if r.status != NORMAL || r.ballot != msg.Ballot {
		return
	}

	slotStr := strconv.Itoa(msg.CmdSlot)
	if r.delivered.Has(slotStr) {
		return
	}

	desc.cmdId = msg.CmdId
	desc.cmdSlot = msg.CmdSlot
	// Copy command from Accept message if not already set (needed for weak commands
	// which aren't in r.proposes on non-leaders)
	if desc.cmd.Op == 0 && msg.Cmd.Op != 0 {
		desc.cmd = msg.Cmd
	}

	desc.afterPayload.Call(func() {

		if desc.accepted {
			return
		}

		desc.accepted = true

		if desc.phase == START {
			desc.phase = ACCEPT
			desc.Call()
		}

		// Non-leaders should always send ORDERED reply when previous commands are ready
		// This is needed for the macks quorum to complete when there are key conflicts
		// (when non-leaders initially return Ok=FALSE instead of TRUE)
		if !r.isLeader {
			prop, exists := r.proposes.Get(desc.cmdId.String())
			if exists { // or if desc.propose != nil ?
				r.IfPreviousAreReady(desc, func() {
					propose := prop.(*defs.GPropose)
					recAck := &MRecordAck{
						Replica: r.Id,
						Ballot:  r.ballot,
						CmdId:   desc.cmdId,
						Ok:      ORDERED,
					}
					r.sender.SendToClient(propose.ClientId, recAck, r.cs.recordAckRPC)
				})
			}
		}

		ack := &MAcceptAck{
			Replica: r.Id,
			Ballot:  msg.Ballot,
			CmdSlot: msg.CmdSlot,
		}

		if r.optimized {
			r.batcher.SendAcceptAck(ack)
			r.handleAcceptAck(ack, desc)
		} else {
			if r.isLeader {
				r.handleAcceptAck(ack, desc)
			} else {
				r.sender.SendTo(msg.Replica, ack, r.cs.acceptAckRPC)
			}
		}
	})
}

func (r *Replica) handleAcceptAck(msg *MAcceptAck, desc *commandDesc) {
	if r.status != NORMAL || r.ballot != msg.Ballot {
		return
	}

	desc.acks.Add(msg.Replica, false, msg)
}

func getAcksHandler(r *Replica, desc *commandDesc) replica.MsgSetHandler {
	return func(_ interface{}, _ []interface{}) {
		commit := &MCommit{
			Replica: r.Id,
			Ballot:  r.ballot,
			CmdSlot: desc.cmdSlot,
		}
		if r.optimized {
			r.handleCommit(commit, desc)
		} else if r.isLeader {
			r.sender.SendToAll(commit, r.cs.commitRPC)
			r.handleCommit(commit, desc)
		}
	}
}

func (r *Replica) handleCommit(msg *MCommit, desc *commandDesc) {
	slotStr := strconv.Itoa(msg.CmdSlot)
	if r.delivered.Has(slotStr) {
		return
	}

	desc.afterPayload.Call(func() {
		if r.status != NORMAL || r.ballot != msg.Ballot || desc.phase == COMMIT {
			return
		}

		desc.phase = COMMIT
		if r.isLeader {
			r.committed.Set(strconv.Itoa(desc.cmdSlot), struct{}{})
			r.notifyCommit(desc.cmdSlot) // Notify waiters that slot is committed
		}

		defer func() {
			desc.successorL.Lock()
			succ := desc.successor
			desc.successorL.Unlock()
			if succ != -1 {
				go func() {
					r.deliverChan <- succ
				}()
			}
		}()
		r.deliver(desc, desc.cmdSlot)
	})
}

func (r *Replica) sync(cmdId CommandId, cmd state.Command) {
	if r.isLeader {
		return
	}
	key := r.int32ToString(int32(cmd.K))
	r.unsynced.Upsert(key, nil,
		func(exists bool, mapV, _ interface{}) interface{} {
			if exists {
				if r.synced.Has(cmdId.String()) {
					return mapV
				}
				r.synced.Set(cmdId.String(), struct{}{})
				entry := mapV.(*UnsyncedEntry)
				newCount := entry.Slot - 1
				if newCount < 0 {
					newCount = 0
				}
				if newCount == 0 {
					// Return a zeroed entry to indicate nothing pending
					return &UnsyncedEntry{Slot: 0}
				}
				// Decrement count, keep metadata of most recent entry
				return &UnsyncedEntry{
					Slot:     newCount,
					IsStrong: entry.IsStrong,
					Op:       entry.Op,
					Value:    entry.Value,
					ClientId: entry.ClientId,
					SeqNum:   entry.SeqNum,
					CmdId:    entry.CmdId,
				}
			}
			r.synced.Set(cmdId.String(), struct{}{})
			return &UnsyncedEntry{Slot: 0}
		})
}

// syncLeader cleans up the unsynced map on the leader after a command is executed.
// For the leader, unsynced entries track the latest slot. After execution, we
// remove the entry only if this command's slot matches (no newer op has taken over).
func (r *Replica) syncLeader(cmdId CommandId, cmd state.Command) {
	key := r.int32ToString(int32(cmd.K))
	v, exists := r.unsynced.Get(key)
	if !exists {
		return
	}
	entry := v.(*UnsyncedEntry)
	// Only remove if this entry's CmdId matches (no newer op for this key)
	if entry.CmdId == cmdId {
		r.unsynced.Remove(key)
	}
}

// unsyncStrong adds a strong (linearizable) op to the unsynced map on non-leaders.
// On non-leaders, entry.Slot is used as a count of pending ops for this key.
func (r *Replica) unsyncStrong(cmd state.Command, cmdId CommandId) {
	key := r.int32ToString(int32(cmd.K))
	r.unsynced.Upsert(key, nil,
		func(exists bool, mapV, _ interface{}) interface{} {
			if exists {
				entry := mapV.(*UnsyncedEntry)
				return &UnsyncedEntry{
					Slot:     entry.Slot + 1,
					IsStrong: true,
					Op:       cmd.Op,
					Value:    cmd.V,
					ClientId: cmdId.ClientId,
					SeqNum:   cmdId.SeqNum,
					CmdId:    cmdId,
				}
			}
			return &UnsyncedEntry{
				Slot:     1,
				IsStrong: true,
				Op:       cmd.Op,
				Value:    cmd.V,
				ClientId: cmdId.ClientId,
				SeqNum:   cmdId.SeqNum,
				CmdId:    cmdId,
			}
		})
}

// unsyncCausal adds a causal (weak) op to the unsynced map (witness pool).
// Called by ALL replicas when receiving a causal propose.
func (r *Replica) unsyncCausal(cmd state.Command, cmdId CommandId) {
	key := r.int32ToString(int32(cmd.K))
	r.unsynced.Upsert(key, nil,
		func(exists bool, mapV, _ interface{}) interface{} {
			if exists {
				entry := mapV.(*UnsyncedEntry)
				return &UnsyncedEntry{
					Slot:     entry.Slot + 1,
					IsStrong: false,
					Op:       cmd.Op,
					Value:    cmd.V,
					ClientId: cmdId.ClientId,
					SeqNum:   cmdId.SeqNum,
					CmdId:    cmdId,
				}
			}
			return &UnsyncedEntry{
				Slot:     1,
				IsStrong: false,
				Op:       cmd.Op,
				Value:    cmd.V,
				ClientId: cmdId.ClientId,
				SeqNum:   cmdId.SeqNum,
				CmdId:    cmdId,
			}
		})
}

// leaderUnsyncStrong adds a strong op to the unsynced map on the leader.
// On leader, entry.Slot stores the actual slot number for dependency tracking.
// Returns the previous slot (dependency) or -1 if no dependency.
func (r *Replica) leaderUnsyncStrong(cmd state.Command, slot int, cmdId CommandId) int {
	depSlot := -1
	key := r.int32ToString(int32(cmd.K))
	r.unsynced.Upsert(key, nil,
		func(exists bool, mapV, _ interface{}) interface{} {
			if exists {
				entry := mapV.(*UnsyncedEntry)
				if entry.Slot > slot {
					r.Fatal(entry.Slot, slot)
					return mapV
				}
				depSlot = entry.Slot
			}
			return &UnsyncedEntry{
				Slot:     slot,
				IsStrong: true,
				Op:       cmd.Op,
				Value:    cmd.V,
				ClientId: cmdId.ClientId,
				SeqNum:   cmdId.SeqNum,
				CmdId:    cmdId,
			}
		})
	return depSlot
}

// leaderUnsyncCausal adds a causal op to the unsynced map on the leader.
// Returns the previous slot (dependency) or -1 if no dependency.
func (r *Replica) leaderUnsyncCausal(cmd state.Command, slot int, cmdId CommandId) int {
	depSlot := -1
	key := r.int32ToString(int32(cmd.K))
	r.unsynced.Upsert(key, nil,
		func(exists bool, mapV, _ interface{}) interface{} {
			if exists {
				entry := mapV.(*UnsyncedEntry)
				if entry.Slot > slot {
					r.Fatal(entry.Slot, slot)
					return mapV
				}
				depSlot = entry.Slot
			}
			return &UnsyncedEntry{
				Slot:     slot,
				IsStrong: false,
				Op:       cmd.Op,
				Value:    cmd.V,
				ClientId: cmdId.ClientId,
				SeqNum:   cmdId.SeqNum,
				CmdId:    cmdId,
			}
		})
	return depSlot
}

// ok checks the unsynced map for conflicts with an incoming strong op.
// Returns TRUE if no conflict, FALSE if there's a strong write conflict.
// In CURP-HO, this is used by non-leaders when processing strong proposes.
func (r *Replica) ok(cmd state.Command) uint8 {
	key := r.int32ToString(int32(cmd.K))
	v, exists := r.unsynced.Get(key)
	if !exists {
		return TRUE
	}
	entry := v.(*UnsyncedEntry)
	if entry.Slot <= 0 {
		return TRUE // No pending entries
	}
	// In CURP-HO: strong write in unsynced → conflict (FALSE)
	if entry.IsStrong && entry.Op == state.PUT {
		return FALSE
	}
	// Weak entries don't cause conflicts for strong ops (they create weakDep instead)
	// But a pending counter > 0 with a non-strong entry means there are causal ops pending
	// For backward compat with CURP-HT behavior: any pending strong op is a conflict
	if entry.IsStrong {
		return FALSE
	}
	return TRUE
}

// okWithWeakDep checks unsynced for conflicts and returns both ok status and weakDep.
// Used by non-leaders in CURP-HO when processing strong proposes.
// weakDep is non-nil if there's an uncommitted weak write on the same key.
func (r *Replica) okWithWeakDep(cmd state.Command) (uint8, *CommandId) {
	key := r.int32ToString(int32(cmd.K))
	v, exists := r.unsynced.Get(key)
	if !exists {
		return TRUE, nil
	}
	entry := v.(*UnsyncedEntry)
	if entry.Slot <= 0 {
		return TRUE, nil
	}
	// Strong write conflict
	if entry.IsStrong && entry.Op == state.PUT {
		return FALSE, nil
	}
	// Any strong op pending → conflict
	if entry.IsStrong {
		return FALSE, nil
	}
	// Causal (weak) write pending → return weakDep
	if !entry.IsStrong && entry.Op == state.PUT {
		dep := entry.CmdId
		return TRUE, &dep
	}
	return TRUE, nil
}

// checkStrongWriteConflict checks if there's a pending strong write on the given key.
// Used in CURP-HO strong op handling to detect write-write conflicts.
func (r *Replica) checkStrongWriteConflict(key state.Key) bool {
	keyStr := r.int32ToString(int32(key))
	if v, exists := r.unsynced.Get(keyStr); exists {
		entry := v.(*UnsyncedEntry)
		return entry.Slot > 0 && entry.IsStrong && entry.Op == state.PUT
	}
	return false
}

// getWeakWriteDep returns the CmdId of a pending weak write on the given key, if any.
// Used in CURP-HO to track weak write dependencies for strong reads.
func (r *Replica) getWeakWriteDep(key state.Key) *CommandId {
	keyStr := r.int32ToString(int32(key))
	if v, exists := r.unsynced.Get(keyStr); exists {
		entry := v.(*UnsyncedEntry)
		if entry.Slot > 0 && !entry.IsStrong && entry.Op == state.PUT {
			dep := entry.CmdId
			return &dep
		}
	}
	return nil
}

// getWeakWriteValue returns the value of a pending weak write on the given key.
// Used for speculative execution: strong reads can see uncommitted weak writes.
func (r *Replica) getWeakWriteValue(key state.Key) (state.Value, bool) {
	keyStr := r.int32ToString(int32(key))
	if v, exists := r.unsynced.Get(keyStr); exists {
		entry := v.(*UnsyncedEntry)
		if entry.Slot > 0 && !entry.IsStrong && entry.Op == state.PUT {
			return entry.Value, true
		}
	}
	return nil, false
}

// isBoundReplicaFor checks if this replica is the bound (closest) replica for a given client.
// In CURP-HO, bound replicas execute causal ops speculatively and reply immediately.
func (r *Replica) isBoundReplicaFor(clientId int32) bool {
	return r.boundClients[clientId]
}

// registerBoundClient registers a client as bound to this replica.
// Called when a client's first causal propose arrives (auto-detect binding).
func (r *Replica) registerBoundClient(clientId int32) {
	r.boundClients[clientId] = true
}

func (r *Replica) deliver(desc *commandDesc, slot int) {
	desc.afterPayload.Call(func() {
		slotStr := strconv.Itoa(slot)
		if r.delivered.Has(slotStr) || !r.Exec {
			return
		}

		if desc.phase != COMMIT && !r.isLeader {
			return
		}

		// For COMMIT phase, check slot ordering before execution
		// For speculative replies (leader, phase != COMMIT), skip this check
		if desc.phase == COMMIT && slot > 0 && !r.executed.Has(strconv.Itoa(slot-1)) {
			return
		}

		p, exists := r.proposes.Get(desc.cmdId.String())
		if exists {
			desc.propose = p.(*defs.GPropose)
		}
		// For weak commands on non-leaders, desc.propose is nil but desc.cmd is set
		// from the Accept message (via handleAccept). Check desc.cmd.Op to handle this case.
		if desc.propose == nil && desc.cmd.Op == 0 {
			return
		}

		if !r.isLeader && desc.propose != nil {
			desc.cmd = desc.propose.Command
			r.sync(desc.cmdId, desc.cmd)
		} else if !r.isLeader && desc.cmd.Op != 0 {
			// Weak command on non-leader: cmd is already set from Accept message
			r.sync(desc.cmdId, desc.cmd)
		}

		// Speculative execution: compute result WITHOUT modifying state
		// CURP-HO: Strong speculative CAN see unsynced (including uncommitted weak writes)
		if desc.val == nil && desc.phase != COMMIT {
			desc.val = r.computeSpeculativeResultWithUnsynced(desc.cmd)
		}

		// Speculative reply to client for strong commands (leader only, before commit)
		// Weak commands are handled separately via handleWeakPropose
		if r.isLeader && desc.phase != COMMIT && desc.propose != nil {
			rep := &MReply{
				Replica: r.Id,
				Ballot:  r.ballot,
				CmdId:   desc.cmdId,
				Rep:     desc.val,
			}
			if desc.dep != -1 && !r.committed.Has(strconv.Itoa(desc.dep)) {
				rep.Ok = FALSE
			} else {
				rep.Ok = TRUE
			}
			// Always send reply to client so they can complete via macks quorum
			// even when rep.Ok == FALSE (pending dependency). Without this,
			// the client hangs waiting for a leader reply that never comes.
			r.sender.SendToClient(desc.propose.ClientId, rep, r.cs.replyRPC)
		}

		// After commit: actually execute and modify state
		if desc.phase == COMMIT && !desc.applied {
			desc.val = desc.cmd.Execute(r.State)
			desc.applied = true
			r.executed.Set(slotStr, struct{}{})
			r.notifyExecute(slot) // Notify waiters that slot is executed

			// CURP-HO: Clean up unsynced entry on leader after execution.
			// On non-leaders, sync() handles cleanup. On leader, we clean up here
			// by decrementing the count or removing the entry if no other pending ops.
			if r.isLeader {
				r.syncLeader(desc.cmdId, desc.cmd)
			}

			go func(nextSlot int) {
				r.deliverChan <- nextSlot
			}(slot + 1)
		}

		if desc.phase == COMMIT {
			// Sync reply is only for strong commands (which have desc.propose set)
			// Weak commands are handled separately
			if !r.contactClients && desc.propose != nil {
				if (r.optimized && desc.propose.Proxy) ||
					(!r.optimized && r.isLeader) {
					rep := &MSyncReply{
						Replica: r.Id,
						Ballot:  r.ballot,
						CmdId:   desc.cmdId,
						Rep:     desc.val,
					}
					r.sender.SendToClient(desc.propose.ClientId, rep, r.cs.syncReplyRPC)
				}
			}
			desc.msgs <- slot
			r.delivered.Set(strconv.Itoa(slot), struct{}{})
			if desc.seq {
				for {
					switch hSlot := (<-desc.msgs).(type) {
					case int:
						r.handleMsg(hSlot, desc, slot, desc.dep)
						return
					}
				}
			}
		}
	})
}

func (r *Replica) getCmdDesc(slot int, msg interface{}, dep int) *commandDesc {
	return r.getCmdDescSeq(slot, msg, dep, false)
}

func (r *Replica) getCmdDescSeq(slot int, msg interface{}, dep int, seq bool) *commandDesc {
	slotStr := strconv.Itoa(slot)
	if r.delivered.Has(slotStr) {
		return nil
	}

	var desc *commandDesc

	r.cmdDescs.Upsert(slotStr, nil,
		func(exists bool, mapV, _ interface{}) interface{} {
			if exists {
				desc = mapV.(*commandDesc)
				return desc
			}

			desc = r.newDesc()
			desc.seq = seq || desc.seq
			desc.cmdSlot = slot
			desc.slotStr = slotStr // Cache the string key
			if !desc.seq {
				go r.handleDesc(desc, slot, dep)
				r.routineCount++
			}

			return desc
		})

	if msg != nil {
		if desc.seq {
			r.handleMsg(msg, desc, slot, dep)
		} else {
			desc.msgs <- msg
		}
	}

	return desc
}

func (r *Replica) newDesc() *commandDesc {
	desc := r.allocDesc()
	desc.cmdSlot = -1
	if desc.msgs == nil {
		desc.msgs = make(chan interface{}, 8)
	}
	desc.active = true
	desc.phase = START
	desc.seq = (r.routineCount >= MaxDescRoutines)
	desc.propose = nil
	desc.val = nil
	desc.cmdId.SeqNum = -42
	desc.dep = -1
	desc.successor = -1
	desc.successorL = sync.Mutex{}
	desc.accepted = false

	desc.afterPayload = desc.afterPayload.ReinitCondF(func() bool {
		// For weak commands on non-leaders, desc.cmd is set from the Accept message
		// even though desc.propose is nil and proposes doesn't have the command
		return (desc.propose != nil || r.proposes.Has(desc.cmdId.String()) || desc.cmd.Op != 0)
	})

	desc.acks = desc.acks.ReinitMsgSet(r.Q, func(_, _ interface{}) bool {
		return true
	}, func(interface{}) {}, getAcksHandler(r, desc))

	return desc
}

func (desc *commandDesc) Call() {
	if desc == nil || desc.pendingCall == nil {
		return
	}
	desc.pendingCall()
	desc.pendingCall = nil
}

func (r *Replica) IfPreviousAreReady(desc *commandDesc, f func()) {
	var pdesc *commandDesc
	if s := desc.cmdSlot - 1; s < 0 {
		pdesc = nil
	} else {
		pdesc = r.getCmdDesc(s, nil, -1)
	}
	if pdesc == nil || pdesc.phase != START {
		f()
	} else {
		pdesc.pendingCall = f
	}
}

func (r *Replica) allocDesc() *commandDesc {
	if r.poolLevel > 0 {
		desc := r.descPool.Get().(*commandDesc)
		slotStr := strconv.Itoa(desc.cmdSlot)
		if r.delivered.Has(slotStr) && r.values.Has(desc.cmdId.String()) {
			return desc
		}
	}
	return &commandDesc{}
}

func (r *Replica) freeDesc(desc *commandDesc) {
	if r.poolLevel > 0 {
		r.descPool.Put(desc)
	}
}

func (r *Replica) handleDesc(desc *commandDesc, slot int, dep int) {
	for desc.active {
		if r.handleMsg(<-desc.msgs, desc, slot, dep) {
			r.routineCount--
			return
		}
	}
}

func (r *Replica) handleMsg(m interface{}, desc *commandDesc, slot int, dep int) bool {
	switch msg := m.(type) {

	case *defs.GPropose:
		r.handlePropose(msg, desc, slot, dep)

	case *MAccept:
		if msg.CmdSlot == slot {
			r.handleAccept(msg, desc)
		}

	case *MAcceptAck:
		if msg.CmdSlot == slot {
			r.handleAcceptAck(msg, desc)
		}

	case *MCommit:
		if msg.CmdSlot == slot {
			r.handleCommit(msg, desc)
		}

	case string:
		if msg == "deliver" {
			r.deliver(desc, slot)
		}

	case int:
		desc.Call()
		r.history[msg].cmdSlot = slot
		r.history[msg].phase = desc.phase
		r.history[msg].cmd = desc.cmd
		desc.active = false
		slotStr := strconv.Itoa(slot)
		r.values.Set(desc.cmdId.String(), desc.val)
		r.cmdDescs.Remove(slotStr)
		r.freeDesc(desc)
		return true
	}

	return false
}

// handleWeakPropose handles weak consistency command from client
func (r *Replica) handleWeakPropose(propose *MWeakPropose) {
	// 1. Assign slot (share slot space with strong for global ordering)
	slot := r.lastCmdSlot
	r.lastCmdSlot++

	// 2. Record dependency (for causal ordering)
	weakCmdId := CommandId{ClientId: propose.ClientId, SeqNum: propose.CommandId}
	dep := r.leaderUnsyncCausal(propose.Command, slot, weakCmdId)

	// 3. Create weak command descriptor
	desc := r.getWeakCmdDesc(slot, propose, dep)

	// 4. Track pending write for non-blocking speculative reads
	// If this is a PUT, add to pendingWrites so subsequent reads can see it immediately
	if propose.Command.Op == state.PUT {
		r.addPendingWrite(propose.ClientId, propose.Command.K, propose.CommandId, propose.Command.V)
	}

	// 5. Speculative execution: compute result WITHOUT modifying state
	// Uses pending writes from this client if available (non-blocking read-after-write)
	// State modification happens after commit in slot order (see asyncReplicateWeak)
	desc.val = r.computeSpeculativeResult(propose.ClientId, propose.CausalDep, propose.Command)
	// Note: Do NOT mark as executed yet - that happens after commit

	// 6. Reply to client immediately (don't wait for replication)
	// Use object pool to reduce allocations
	rep := r.weakReplyPool.Get().(*MWeakReply)
	rep.Replica = r.Id
	rep.Ballot = r.ballot
	rep.CmdId = desc.cmdId
	rep.Rep = desc.val
	r.sender.SendToClient(propose.ClientId, rep, r.cs.weakReplyRPC)
	// Note: We don't return to pool immediately since sender may still use it
	// In a production system, we'd need to track send completion for proper recycling

	// 7. Async replication (background, non-blocking)
	// Actual state modification and marking as executed happens after commit
	// CausalDep waiting is done in asyncReplicateWeak for execution ordering only
	go r.asyncReplicateWeak(desc, slot, propose.ClientId, propose.CommandId, propose.CausalDep)
}

// getWeakCmdDesc creates a command descriptor for weak commands
func (r *Replica) getWeakCmdDesc(slot int, propose *MWeakPropose, dep int) *commandDesc {
	desc := r.newDesc()
	desc.isWeak = true
	desc.cmdSlot = slot
	desc.dep = dep
	desc.cmdId = CommandId{
		ClientId: propose.ClientId,
		SeqNum:   propose.CommandId,
	}
	desc.cmd = propose.Command
	desc.phase = ACCEPT // Skip START phase for weak commands

	// Track dependency
	if dep != -1 {
		depDesc := r.getCmdDesc(dep, nil, -1)
		if depDesc != nil {
			depDesc.successorL.Lock()
			depDesc.successor = slot
			depDesc.successorL.Unlock()
		}
	}

	return desc
}

// asyncReplicateWeak replicates weak command to other replicas asynchronously
// After replication completes (commit), it executes the command in slot order
func (r *Replica) asyncReplicateWeak(desc *commandDesc, slot int, clientId int32, seqNum int32, causalDep int32) {
	// Send Accept to other replicas
	acc := &MAccept{
		Replica: r.Id,
		Ballot:  r.ballot,
		Cmd:     desc.cmd,
		CmdId:   desc.cmdId,
		CmdSlot: slot,
	}

	r.batcher.SendAccept(acc)
	r.handleAccept(acc, desc)

	// The accept/commit flow will continue through normal message handling
	// Once majority acks are received, the command will be committed
	// The actual execution happens through the deliver() mechanism which
	// ensures slot ordering is maintained

	// After commit is complete (tracked via committed map), execute in slot order
	// Wait for commit using channel notification (replaces spin-wait)
	commitCh := r.getOrCreateCommitNotify(slot)
	select {
	case <-commitCh:
		// Committed
	case <-time.After(1 * time.Second):
		// Timeout - proceed anyway to avoid deadlock
	}

	// Wait for slot-1 to be executed (slot ordering)
	if slot > 0 {
		executeCh := r.getOrCreateExecuteNotify(slot - 1)
		select {
		case <-executeCh:
			// Slot-1 executed
		case <-time.After(1 * time.Second):
			// Timeout - proceed anyway to avoid deadlock
		}
	}

	// Wait for causal dependency (session ordering within client)
	// This is now done here instead of blocking the speculative result computation
	if causalDep > 0 {
		r.waitForWeakDep(clientId, causalDep)
	}

	// Now execute and mark as executed (state modification happens here)
	if !desc.applied {
		desc.val = desc.cmd.Execute(r.State)
		desc.applied = true
		slotStr := strconv.Itoa(slot)
		r.executed.Set(slotStr, struct{}{})
		r.notifyExecute(slot) // Notify waiters that slot is executed

		// Mark this weak command as executed for causal ordering
		r.markWeakExecuted(clientId, seqNum)

		// Clean up pending write after execution (for PUT commands)
		if desc.cmd.Op == state.PUT {
			r.removePendingWrite(clientId, desc.cmd.K, seqNum)
		}

		// Trigger next slot
		go func(nextSlot int) {
			r.deliverChan <- nextSlot
		}(slot + 1)
	}
}

// handleCausalPropose handles a CURP-HO causal command received from a client.
// Unlike handleWeakPropose (leader-only), ALL replicas process causal proposes:
//   1. Add to witness pool (unsyncCausal) for conflict detection
//   2. Track pending write (for speculative read-after-write)
//   3. Compute speculative result and reply with MCausalReply
//   4. If leader: assign slot and coordinate replication (accept/commit flow)
//
// All replicas reply with MCausalReply. The client filters by boundReplica,
// ignoring non-bound replies. This avoids needing a binding protocol.
func (r *Replica) handleCausalPropose(propose *MCausalPropose) {
	cmdId := CommandId{ClientId: propose.ClientId, SeqNum: propose.CommandId}

	// 1. ALL replicas: add to witness pool for conflict detection
	r.unsyncCausal(propose.Command, cmdId)

	// 2. ALL replicas: track pending write for speculative reads
	if propose.Command.Op == state.PUT {
		r.addPendingWrite(propose.ClientId, propose.Command.K, propose.CommandId, propose.Command.V)
	}

	// 3. ALL replicas: compute speculative result and reply
	val := r.computeSpeculativeResult(propose.ClientId, propose.CausalDep, propose.Command)
	rep := &MCausalReply{
		Replica: r.Id,
		CmdId:   cmdId,
		Rep:     val,
	}
	r.sender.SendToClient(propose.ClientId, rep, r.cs.causalReplyRPC)

	// 4. If leader: assign slot and coordinate replication
	if r.isLeader {
		slot := r.lastCmdSlot
		r.lastCmdSlot++

		dep := r.leaderUnsyncCausal(propose.Command, slot, cmdId)
		desc := r.getCausalCmdDesc(slot, propose, dep)

		go r.asyncReplicateCausal(desc, slot, propose.ClientId, propose.CommandId, propose.CausalDep)
	}
}

// getCausalCmdDesc creates a command descriptor for causal commands.
// Similar to getWeakCmdDesc but takes MCausalPropose instead of MWeakPropose.
func (r *Replica) getCausalCmdDesc(slot int, propose *MCausalPropose, dep int) *commandDesc {
	desc := r.newDesc()
	desc.isWeak = true
	desc.cmdSlot = slot
	desc.dep = dep
	desc.cmdId = CommandId{
		ClientId: propose.ClientId,
		SeqNum:   propose.CommandId,
	}
	desc.cmd = propose.Command
	desc.phase = ACCEPT // Skip START phase for causal commands

	// Track dependency
	if dep != -1 {
		depDesc := r.getCmdDesc(dep, nil, -1)
		if depDesc != nil {
			depDesc.successorL.Lock()
			depDesc.successor = slot
			depDesc.successorL.Unlock()
		}
	}

	return desc
}

// asyncReplicateCausal replicates a causal command to other replicas asynchronously.
// After replication completes (commit), it executes the command in slot order.
// Similar to asyncReplicateWeak but also cleans up leader unsynced entries.
func (r *Replica) asyncReplicateCausal(desc *commandDesc, slot int, clientId int32, seqNum int32, causalDep int32) {
	// Send Accept to other replicas
	acc := &MAccept{
		Replica: r.Id,
		Ballot:  r.ballot,
		Cmd:     desc.cmd,
		CmdId:   desc.cmdId,
		CmdSlot: slot,
	}

	r.batcher.SendAccept(acc)
	r.handleAccept(acc, desc)

	// Wait for commit using channel notification
	commitCh := r.getOrCreateCommitNotify(slot)
	select {
	case <-commitCh:
		// Committed
	case <-time.After(1 * time.Second):
		// Timeout - proceed anyway to avoid deadlock
	}

	// Wait for slot-1 to be executed (slot ordering)
	if slot > 0 {
		executeCh := r.getOrCreateExecuteNotify(slot - 1)
		select {
		case <-executeCh:
			// Slot-1 executed
		case <-time.After(1 * time.Second):
			// Timeout - proceed anyway to avoid deadlock
		}
	}

	// Wait for causal dependency (session ordering within client)
	if causalDep > 0 {
		r.waitForWeakDep(clientId, causalDep)
	}

	// Execute and mark as executed (state modification happens here)
	if !desc.applied {
		desc.val = desc.cmd.Execute(r.State)
		desc.applied = true
		slotStr := strconv.Itoa(slot)
		r.executed.Set(slotStr, struct{}{})
		r.notifyExecute(slot)

		// Mark this causal command as executed for causal ordering
		r.markWeakExecuted(clientId, seqNum)

		// Clean up pending write after execution (for PUT commands)
		if desc.cmd.Op == state.PUT {
			r.removePendingWrite(clientId, desc.cmd.K, seqNum)
		}

		// Clean up leader unsynced entry
		r.syncLeader(desc.cmdId, desc.cmd)

		// Trigger next slot
		go func(nextSlot int) {
			r.deliverChan <- nextSlot
		}(slot + 1)
	}
}

// waitForWeakDep waits for a causal dependency to be executed
// This ensures that weak commands from the same client execute in order
// Optimized with shorter sleep intervals and string caching
func (r *Replica) waitForWeakDep(clientId int32, depSeqNum int32) {
	clientKey := r.int32ToString(clientId)

	// Optimized spin-wait with 10x faster polling (10us instead of 100us)
	// This reduces latency while still avoiding busy-waiting
	for i := 0; i < 10000; i++ { // Max ~100ms wait (10000 * 10us)
		if lastExec, exists := r.weakExecuted.Get(clientKey); exists {
			if lastExec.(int32) >= depSeqNum {
				return // Dependency satisfied
			}
		}
		// Brief sleep to avoid busy-waiting (10us)
		time.Sleep(10 * time.Microsecond)
	}
	// Timeout: proceed anyway to avoid deadlock
}

// markWeakExecuted marks a weak command as executed for causal ordering
func (r *Replica) markWeakExecuted(clientId int32, seqNum int32) {
	clientKey := r.int32ToString(clientId)

	// Update the executed seqNum
	r.weakExecuted.Upsert(clientKey, nil,
		func(exists bool, mapV, _ interface{}) interface{} {
			if exists {
				prevSeqNum := mapV.(int32)
				// Only update if this seqNum is newer
				if seqNum > prevSeqNum {
					return seqNum
				}
				return mapV
			}
			return seqNum
		})
}

// int32ToString converts an int32 to string using a cache to avoid repeated conversions
func (r *Replica) int32ToString(val int32) string {
	// Try to load from cache first
	if cached, ok := r.stringCache.Load(val); ok {
		return cached.(string)
	}
	// Not in cache, convert and store
	str := strconv.FormatInt(int64(val), 10)
	r.stringCache.Store(val, str)
	return str
}

// pendingWriteKey creates a unique key for pending writes: "clientId:key"
func (r *Replica) pendingWriteKey(clientId int32, key state.Key) string {
	return r.int32ToString(clientId) + ":" + r.int32ToString(int32(key))
}

// addPendingWrite tracks an uncommitted write for speculative read computation
func (r *Replica) addPendingWrite(clientId int32, key state.Key, seqNum int32, value state.Value) {
	pwKey := r.pendingWriteKey(clientId, key)
	r.pendingWrites.Upsert(pwKey, nil,
		func(exists bool, mapV, _ interface{}) interface{} {
			if exists {
				existing := mapV.(*pendingWrite)
				// Only update if this seqNum is newer
				if seqNum > existing.seqNum {
					return &pendingWrite{seqNum: seqNum, value: value}
				}
				return existing
			}
			return &pendingWrite{seqNum: seqNum, value: value}
		})
}

// removePendingWrite removes a pending write after it's been committed and executed
func (r *Replica) removePendingWrite(clientId int32, key state.Key, seqNum int32) {
	pwKey := r.pendingWriteKey(clientId, key)
	// Only remove if the seqNum matches (don't remove a newer pending write)
	if pw, exists := r.pendingWrites.Get(pwKey); exists {
		if pw.(*pendingWrite).seqNum == seqNum {
			r.pendingWrites.Remove(pwKey)
		}
	}
}

// getPendingWrite returns the pending write value if it exists and satisfies the causal dependency
func (r *Replica) getPendingWrite(clientId int32, key state.Key, causalDep int32) *pendingWrite {
	pwKey := r.pendingWriteKey(clientId, key)
	if pw, exists := r.pendingWrites.Get(pwKey); exists {
		pending := pw.(*pendingWrite)
		// Only return if this pending write is the one we're looking for (seqNum <= causalDep)
		if pending.seqNum <= causalDep {
			return pending
		}
	}
	return nil
}

// computeSpeculativeResultWithUnsynced computes the speculative result for strong ops.
// CURP-HO: Strong speculative execution CAN see unsynced (including uncommitted weak writes).
// For GET: checks unsynced witness pool for weak write first, then falls back to committed state.
// For PUT: returns NIL during speculation.
func (r *Replica) computeSpeculativeResultWithUnsynced(cmd state.Command) state.Value {
	switch cmd.Op {
	case state.GET:
		// Check unsynced witness pool for weak write value first
		if val, found := r.getWeakWriteValue(cmd.K); found {
			return val
		}
		// Fall back to committed state
		return cmd.ComputeResult(r.State)

	case state.PUT:
		// For PUT, return NIL during speculation
		return state.NIL()

	default:
		return cmd.ComputeResult(r.State)
	}
}

// computeSpeculativeResult computes the speculative result for a command
// For GET: checks pending writes from this client first, then falls back to committed state
// For SCAN: currently just uses committed state (pending write overlay is complex)
// For PUT: returns NIL (no value for writes during speculation)
func (r *Replica) computeSpeculativeResult(clientId int32, causalDep int32, cmd state.Command) state.Value {
	switch cmd.Op {
	case state.GET:
		// Check pending writes from this client for this key
		if pending := r.getPendingWrite(clientId, cmd.K, causalDep); pending != nil {
			return pending.value
		}
		// Fall back to committed state
		return cmd.ComputeResult(r.State)

	case state.SCAN:
		// For SCAN, we need to merge pending writes with committed state
		// This is complex - for now, just use committed state
		// TODO: Implement proper SCAN with pending write overlay
		return cmd.ComputeResult(r.State)

	case state.PUT:
		// For PUT, return NIL during speculation
		return state.NIL()

	default:
		return state.NIL()
	}
}

// getOrCreateCommitNotify returns a channel that will be closed when the slot is committed
func (r *Replica) getOrCreateCommitNotify(slot int) chan struct{} {
	r.notifyMu.Lock()
	defer r.notifyMu.Unlock()

	// Check if already committed
	if r.committed.Has(strconv.Itoa(slot)) {
		// Return pre-allocated closed channel (avoids allocation)
		return r.closedChan
	}

	// Get or create notification channel
	if ch, ok := r.commitNotify[slot]; ok {
		return ch
	}
	ch := make(chan struct{})
	r.commitNotify[slot] = ch
	return ch
}

// notifyCommit notifies waiters that a slot has been committed
func (r *Replica) notifyCommit(slot int) {
	r.notifyMu.Lock()
	defer r.notifyMu.Unlock()

	if ch, ok := r.commitNotify[slot]; ok {
		close(ch)
		delete(r.commitNotify, slot)
	}
}

// getOrCreateExecuteNotify returns a channel that will be closed when the slot is executed
func (r *Replica) getOrCreateExecuteNotify(slot int) chan struct{} {
	r.notifyMu.Lock()
	defer r.notifyMu.Unlock()

	// Check if already executed
	if r.executed.Has(strconv.Itoa(slot)) {
		// Return pre-allocated closed channel (avoids allocation)
		return r.closedChan
	}

	// Get or create notification channel
	if ch, ok := r.executeNotify[slot]; ok {
		return ch
	}
	ch := make(chan struct{})
	r.executeNotify[slot] = ch
	return ch
}

// notifyExecute notifies waiters that a slot has been executed
func (r *Replica) notifyExecute(slot int) {
	r.notifyMu.Lock()
	defer r.notifyMu.Unlock()

	if ch, ok := r.executeNotify[slot]; ok {
		close(ch)
		delete(r.executeNotify, slot)
	}
}
