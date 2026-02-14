package curpht

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

	// Track per-key version (slot of last write) for weak read responses
	// Key: key as string, Value: int (slot number)
	keyVersions cmap.ConcurrentMap

	// Notification channels for async waiting (replaces spin-waits)
	commitNotify  map[int]chan struct{} // slot -> notification channel for commit
	executeNotify map[int]chan struct{} // slot -> notification channel for execution
	notifyMu      sync.Mutex            // protects commitNotify and executeNotify

	// String conversion cache to avoid repeated strconv.FormatInt calls
	// Key: int32, Value: string representation
	stringCache sync.Map

	// Pre-allocated closed channel for immediate notifications (avoids repeated allocations)
	closedChan chan struct{}

	// Channel-based causal dep notification (replaces spin-wait in waitForWeakDep)
	weakDepNotify map[int32]chan struct{}
	weakDepMu     sync.Mutex
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
	// Optimized SHARD_COUNT for cache locality (Phase 18.6)
	// 512 shards: good for 4-16 threads, fits in L2 cache, low contention
	// Reduced from 32768 for 98% memory savings and better cache hit rate
	cmap.SHARD_COUNT = 512

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
		weakExecuted: cmap.New(),
		keyVersions:  cmap.New(),
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

	// Apply batch delay from config (Phase 32: network batching optimization)
	if conf.BatchDelayUs > 0 {
		r.batcher.SetBatchDelay(int64(conf.BatchDelayUs * 1000)) // Convert μs to ns
	}

	// Initialize pre-allocated closed channel for immediate notifications
	r.closedChan = make(chan struct{})
	close(r.closedChan)

	// Initialize channel-based weak dep notification (replaces spin-wait)
	r.weakDepNotify = make(map[int32]chan struct{})

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

// BeTheLeader always returns 0 as the leader for CURP-HT.
// In CURP-HT, the leader is determined by the ballot (ballot=0 means replica 0 is leader).
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
				dep := r.leaderUnsync(propose.Command, r.lastCmdSlot)
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
				recAck := &MRecordAck{
					Replica: r.Id,
					Ballot:  r.ballot,
					CmdId:   cmdId,
					Ok:      r.ok(propose.Command),
				}
				r.sender.SendToClient(propose.ClientId, recAck, r.cs.recordAckRPC)
				r.unsync(propose.Command)
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
				slotVal := int32(0)
				if s, ok := r.slots[sync.CmdId]; ok {
					slotVal = int32(s)
				}
				rep := &MSyncReply{
					Replica: r.Id,
					Ballot:  r.ballot,
					CmdId:   sync.CmdId,
					Rep:     val.([]byte),
					Slot:    slotVal,
				}
				r.sender.SendToClient(sync.CmdId.ClientId, rep, r.cs.syncReplyRPC)
			}

		case m := <-r.cs.weakProposeChan:
			if r.isLeader {
				weakPropose := m.(*MWeakPropose)
				r.handleWeakPropose(weakPropose)
			}
			// Non-Leader ignores weak propose (should not receive it)

		case m := <-r.cs.weakReadChan:
			weakRead := m.(*MWeakRead)
			r.handleWeakRead(weakRead)
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
				v := mapV.(int) - 1
				if v < 0 {
					v = 0
				}
				return v
			}
			r.synced.Set(cmdId.String(), struct{}{})
			return 0
		})
}

func (r *Replica) unsync(cmd state.Command) {
	key := r.int32ToString(int32(cmd.K))
	r.unsynced.Upsert(key, nil,
		func(exists bool, mapV, _ interface{}) interface{} {
			if exists {
				return mapV.(int) + 1
			}
			return 1
		})
}

func (r *Replica) leaderUnsync(cmd state.Command, slot int) int {
	depSlot := -1
	key := r.int32ToString(int32(cmd.K))
	r.unsynced.Upsert(key, nil,
		func(exists bool, mapV, _ interface{}) interface{} {
			if exists {
				if mapV.(int) > slot {
					r.Fatal(mapV.(int), slot)
					return mapV
				}
				depSlot = mapV.(int)
			}
			return slot
		})
	return depSlot
}

func (r *Replica) ok(cmd state.Command) uint8 {
	key := r.int32ToString(int32(cmd.K))
	v, exists := r.unsynced.Get(key)
	if exists && v.(int) > 0 {
		return FALSE
	}
	return TRUE
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
		if desc.val == nil && desc.phase != COMMIT {
			// Before commit: use ComputeResult (read-only)
			desc.val = desc.cmd.ComputeResult(r.State)
		}

		// Speculative reply to client for strong commands (leader only, before commit)
		// Weak commands are handled separately via handleWeakPropose
		if r.isLeader && desc.phase != COMMIT && desc.propose != nil {
			rep := &MReply{
				Replica: r.Id,
				Ballot:  r.ballot,
				CmdId:   desc.cmdId,
				Rep:     desc.val,
				Slot:    int32(slot),
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
			// Track per-key version for weak read responses
			if desc.cmd.Op == state.PUT {
				keyStr := r.int32ToString(int32(desc.cmd.K))
				r.keyVersions.Set(keyStr, slot)
			}
			r.notifyExecute(slot) // Notify waiters that slot is executed
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
						Slot:    int32(slot),
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
	dep := r.leaderUnsync(propose.Command, slot)

	// 3. Create weak command descriptor
	desc := r.getWeakCmdDesc(slot, propose, dep)

	// 4. Async replication — reply happens AFTER commit+execute (2 RTT)
	go r.asyncReplicateWeak(desc, slot, propose.ClientId, propose.CommandId, propose.CausalDep)
}

// getWeakCmdDesc creates a command descriptor for weak commands.
// The descriptor is registered in cmdDescs so that AcceptAcks arriving via the
// main run loop are routed to the SAME descriptor used by asyncReplicateWeak.
// Without this, acks would be split across two descriptors and never reach quorum.
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

	// Register in cmdDescs so AcceptAcks are routed to this descriptor
	slotStr := strconv.Itoa(slot)
	desc.slotStr = slotStr
	r.cmdDescs.Set(slotStr, desc)

	// Start handler goroutine to process incoming messages (AcceptAcks, Commits)
	if !desc.seq {
		go r.handleDesc(desc, slot, dep)
		r.routineCount++
	}

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
	slotStr := strconv.Itoa(slot)
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

	// Execute if not already done by deliver() (race: deliver() may run first)
	if !desc.applied {
		desc.val = desc.cmd.Execute(r.State)
		desc.applied = true
		r.executed.Set(slotStr, struct{}{})
		// Track per-key version for weak read responses
		if desc.cmd.Op == state.PUT {
			keyStr := r.int32ToString(int32(desc.cmd.K))
			r.keyVersions.Set(keyStr, slot)
		}
		r.notifyExecute(slot) // Notify waiters that slot is executed

		// Trigger next slot
		go func(nextSlot int) {
			r.deliverChan <- nextSlot
		}(slot + 1)
	}

	// Always mark weak executed and send reply — even if deliver() executed first.
	// deliver() sets desc.val but does NOT send weak replies or mark weak executed.
	r.markWeakExecuted(clientId, seqNum)

	rep := r.weakReplyPool.Get().(*MWeakReply)
	rep.Replica = r.Id
	rep.Ballot = r.ballot
	rep.CmdId = desc.cmdId
	rep.Rep = desc.val
	rep.Slot = int32(slot)
	r.sender.SendToClient(clientId, rep, r.cs.weakReplyRPC)
}

// waitForWeakDep waits for a causal dependency to be executed.
// Uses channel-based notification instead of spin-wait to avoid CPU exhaustion
// at high request counts. When markWeakExecuted advances the seqnum for a client,
// it closes the broadcast channel, waking all waiters.
func (r *Replica) waitForWeakDep(clientId int32, depSeqNum int32) {
	clientKey := r.int32ToString(clientId)

	for {
		// Check if dependency is already satisfied
		if lastExec, exists := r.weakExecuted.Get(clientKey); exists {
			if lastExec.(int32) >= depSeqNum {
				return
			}
		}

		// Get or create notification channel for this client
		ch := r.getWeakDepNotify(clientId)

		// Wait for notification or timeout
		select {
		case <-ch:
			// Notification received — re-check condition in next loop iteration
		case <-time.After(5 * time.Second):
			// Timeout: proceed to avoid deadlock
			return
		}
	}
}

// getWeakDepNotify returns the current notification channel for a client's weak dep.
func (r *Replica) getWeakDepNotify(clientId int32) chan struct{} {
	r.weakDepMu.Lock()
	defer r.weakDepMu.Unlock()
	if ch, ok := r.weakDepNotify[clientId]; ok {
		return ch
	}
	ch := make(chan struct{})
	r.weakDepNotify[clientId] = ch
	return ch
}

// notifyWeakDep broadcasts to all waiters for a client's weak dependency.
func (r *Replica) notifyWeakDep(clientId int32) {
	r.weakDepMu.Lock()
	if ch, ok := r.weakDepNotify[clientId]; ok {
		close(ch)
		delete(r.weakDepNotify, clientId)
	}
	r.weakDepMu.Unlock()
}

// markWeakExecuted marks a weak command as executed for causal ordering,
// then notifies all waiters for this client's weak dependency.
func (r *Replica) markWeakExecuted(clientId int32, seqNum int32) {
	clientKey := r.int32ToString(clientId)
	r.weakExecuted.Upsert(clientKey, nil,
		func(exists bool, mapV, _ interface{}) interface{} {
			if exists {
				// Only update if this seqNum is newer
				if seqNum > mapV.(int32) {
					return seqNum
				}
				return mapV
			}
			return seqNum
		})

	// Notify all waiters for this client
	r.notifyWeakDep(clientId)
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

// handleWeakRead handles a weak read request from any client (sent to nearest replica)
// Returns committed value + version (slot of last write to this key)
func (r *Replica) handleWeakRead(msg *MWeakRead) {
	cmd := state.Command{Op: state.GET, K: msg.Key, V: state.NIL()}
	value := cmd.ComputeResult(r.State)
	version := int32(0)
	keyStr := r.int32ToString(int32(msg.Key))
	if v, exists := r.keyVersions.Get(keyStr); exists {
		version = int32(v.(int))
	}
	reply := &MWeakReadReply{
		Replica: r.Id,
		Ballot:  r.ballot,
		CmdId:   CommandId{ClientId: msg.ClientId, SeqNum: msg.CommandId},
		Rep:     value,
		Version: version,
	}
	r.sender.SendToClient(msg.ClientId, reply, r.cs.weakReadReplyRPC)
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
