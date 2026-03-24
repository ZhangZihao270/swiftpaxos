package epaxosho

import (
	"log"
	"sort"
	"sync/atomic"
	"time"

	"github.com/imdea-software/swiftpaxos/replica/defs"
	"github.com/imdea-software/swiftpaxos/state"
)

const EXEC_SLEEP_TIME_NS = 1000 // 1 microsecond

var execVAL state.Value // default zero value returned for discarded writes

// executeCommands is the main execution loop that runs as a goroutine.
// It scans all instance slots across replicas and executes committed commands.
func (r *Replica) executeCommands() {
	problemInstance := make([]int32, r.N)
	timeout := make([]uint64, r.N)
	for q := 0; q < r.N; q++ {
		problemInstance[q] = -1
		timeout[q] = 0
	}

	execCount := int64(0)
	lastLog := time.Now()

	for !r.Shutdown {
		executed := false

		for q := 0; q < r.N; q++ {
			for inst := r.ExecedUpTo[q] + 1; inst < r.crtInstance[q]; inst++ {
				if r.InstanceSpace[q][inst] != nil &&
					(r.InstanceSpace[q][inst].Status == EXECUTED || r.InstanceSpace[q][inst].Status == DISCARDED) {
					if inst == r.ExecedUpTo[q]+1 {
						r.ExecedUpTo[q] = inst
					}
					continue
				}

				// Advance ExecedUpTo past CAUSALLY_COMMITTED instances.
				// Causal instances don't participate in strong SCC ordering,
				// so keeping ExecedUpTo behind them bloats strongconnect's
				// scan range (ExecedUpTo+1 to deps[q]) causing O(N*range) overhead.
				if r.InstanceSpace[q][inst] != nil &&
					r.InstanceSpace[q][inst].Status == CAUSALLY_COMMITTED &&
					inst == r.ExecedUpTo[q]+1 {
					r.ExecedUpTo[q] = inst
				}

				if r.InstanceSpace[q][inst] == nil ||
					r.InstanceSpace[q][inst].State == WAITING ||
					(r.InstanceSpace[q][inst].Status != STRONGLY_COMMITTED && r.InstanceSpace[q][inst].Status != CAUSALLY_COMMITTED) {
					if inst == problemInstance[q] {
						timeout[q] += EXEC_SLEEP_TIME_NS
						if timeout[q] >= COMMIT_GRACE_PERIOD {
							r.instancesToRecover <- &instanceId{int32(q), inst}
							timeout[q] = 0
						}
					} else {
						problemInstance[q] = inst
						timeout[q] = 0
					}
					if r.InstanceSpace[q][inst] == nil {
						continue
					}
					// Continue past non-nil uncommitted instances instead of breaking.
					// Prevents stuck strong instances from blocking executable
					// causal instances further in the sequence.
					continue
				}

				if ok := r.exec.executeCommand(int32(q), inst); ok {
					if r.InstanceSpace[q][inst].Status == EXECUTED || r.InstanceSpace[q][inst].Status == DISCARDED {
						executed = true
						execCount++
						if inst == r.ExecedUpTo[q]+1 {
							r.ExecedUpTo[q] = inst
						}
					}
				}
			}
		}
		if !executed {
			time.Sleep(EXEC_SLEEP_TIME_NS)
		}
		if time.Since(lastLog) > 5*time.Second {
			skipped := atomic.LoadInt64(&r.exec.skippedDeps)
			log.Printf("EXEC: executed=%d skippedCausalDeps=%d execedUpTo=%v", execCount, skipped, r.ExecedUpTo)
			lastLog = time.Now()
		}
	}
}

// executeCommand decides how to execute a single committed instance.
func (e *Exec) executeCommand(replica int32, instance int32) bool {
	if e.r.InstanceSpace[replica][instance] == nil {
		return false
	}
	inst := e.r.InstanceSpace[replica][instance]
	if inst.Status == EXECUTED || inst.Status == DISCARDED {
		return true
	}
	if inst.State == WAITING {
		return false
	}
	if inst.Status != STRONGLY_COMMITTED && inst.Status != CAUSALLY_COMMITTED {
		return false
	}

	if inst.Status == CAUSALLY_COMMITTED && replica == e.r.Id {
		e.executeCausalCommand(replica, instance)
	} else {
		if !e.findSCC(inst) {
			return false
		}
	}

	if instance == e.r.ExecedUpTo[replica]+1 {
		e.r.ExecedUpTo[replica] = instance
	}
	return true
}

// executeCausalCommand executes causal commands on the home replica.
// Uses "latest write wins" semantics for PUT operations.
func (e *Exec) executeCausalCommand(replica int32, instance int32) {
	inst := e.r.InstanceSpace[replica][instance]
	for inst.Cmds == nil {
		time.Sleep(1000 * 1000)
	}

	flag := 0
	for idx := 0; idx < len(inst.Cmds); idx++ {
		if inst.Cmds[idx].Op == state.GET {
			flag = 1
			val := inst.Cmds[idx].Execute(e.r.State)
			if e.r.Dreply && inst.lb != nil && inst.lb.clientProposals != nil && idx < len(inst.lb.clientProposals) {
				e.r.ReplyProposeTS(
					&defs.ProposeReplyTS{
						OK:        TRUE,
						CommandId: inst.lb.clientProposals[idx].CommandId,
						Value:     val,
						Timestamp: inst.lb.clientProposals[idx].Timestamp,
					},
					inst.lb.clientProposals[idx].Reply,
					inst.lb.clientProposals[idx].Mutex)
			}
		} else {
			latestSeq := e.latestWriteSeq(inst.Cmds[idx].K)
			if latestSeq < inst.Seq {
				flag = 1
				val := inst.Cmds[idx].Execute(e.r.State)
				e.r.maxWriteSeqPerKeyMu.Lock()
				e.r.maxWriteSeqPerKey[inst.Cmds[idx].K] = inst.Seq
				e.r.maxWriteSeqPerKeyMu.Unlock()
				e.r.maxWriteInstancePerKeyMu.Lock()
				e.r.maxWriteInstancePerKey[inst.Cmds[idx].K] = inst.instanceId
				e.r.maxWriteInstancePerKeyMu.Unlock()
				if e.r.Dreply && inst.lb != nil && inst.lb.clientProposals != nil && idx < len(inst.lb.clientProposals) {
					e.r.ReplyProposeTS(
						&defs.ProposeReplyTS{
							OK:        TRUE,
							CommandId: inst.lb.clientProposals[idx].CommandId,
							Value:     val,
							Timestamp: inst.lb.clientProposals[idx].Timestamp,
						},
						inst.lb.clientProposals[idx].Reply,
						inst.lb.clientProposals[idx].Mutex)
				}
			} else {
				// Stale write — discard but still reply to client.
				if e.r.Dreply && inst.lb != nil && inst.lb.clientProposals != nil && idx < len(inst.lb.clientProposals) {
					e.r.ReplyProposeTS(
						&defs.ProposeReplyTS{
							OK:        TRUE,
							CommandId: inst.lb.clientProposals[idx].CommandId,
							Value:     execVAL,
							Timestamp: inst.lb.clientProposals[idx].Timestamp,
						},
						inst.lb.clientProposals[idx].Reply,
						inst.lb.clientProposals[idx].Mutex)
				}
			}
		}
	}

	if flag == 1 {
		inst.Status = EXECUTED
	} else {
		inst.Status = DISCARDED
	}
	inst.State = DONE
}

// latestWriteSeq returns the highest sequence number of any previously
// executed PUT to the given key, or -1 if no prior write exists.
func (e *Exec) latestWriteSeq(key state.Key) int32 {
	max := int32(-1)
	e.r.maxWriteSeqPerKeyMu.RLock()
	seq, present := e.r.maxWriteSeqPerKey[key]
	e.r.maxWriteSeqPerKeyMu.RUnlock()
	if present {
		max = seq
	}
	return max
}

// --- Tarjan SCC algorithm for strong command execution ---

var sccStack []*Instance = make([]*Instance, 0, 100)

// findSCC finds the strongly connected component containing root and
// executes all commands in it. Returns false if dependencies are not ready.
func (e *Exec) findSCC(root *Instance) bool {
	index := 1
	sccStack = sccStack[0:0]
	return e.strongconnect(root, &index)
}

func (e *Exec) strongconnect(v *Instance, index *int) bool {
	v.Index = *index
	v.Lowlink = *index
	*index++

	l := len(sccStack)
	if l == cap(sccStack) {
		newSlice := make([]*Instance, l, 2*l)
		copy(newSlice, sccStack)
		sccStack = newSlice
	}
	sccStack = sccStack[0 : l+1]
	sccStack[l] = v

	for q := int32(0); q < int32(e.r.N); q++ {
		depInst := v.Deps[q]
		graphDepth := e.r.ExecedUpTo[q] + MAX_DEPTH_DEP
		if graphDepth > depInst {
			graphDepth = depInst
		}

		for i := e.r.ExecedUpTo[q] + 1; i <= graphDepth; i++ {
			if v.Cmds == nil {
				return false
			}
			if e.r.InstanceSpace[q][i] == nil || e.r.InstanceSpace[q][i].Cmds == nil {
				// Dep instance not yet received — skip instead of blocking.
				// This can happen when a causal instance was committed on the
				// leader but not yet propagated to this replica.
				atomic.AddInt64(&e.skippedDeps, 1)
				continue
			}
			if e.r.InstanceSpace[q][i].Status == EXECUTED || e.r.InstanceSpace[q][i].Status == DISCARDED {
				continue
			}
			if (e.r.InstanceSpace[q][i].Status != STRONGLY_COMMITTED && e.r.InstanceSpace[q][i].Status != CAUSALLY_COMMITTED) || e.r.InstanceSpace[q][i].State == WAITING {
				// Not committed yet or still WAITING — skip if causal, block if strong
				if e.r.InstanceSpace[q][i].Cmds != nil && e.r.InstanceSpace[q][i].Cmds[0].CL == state.CAUSAL {
					atomic.AddInt64(&e.skippedDeps, 1)
					continue
				}
				return false
			}

			w := e.r.InstanceSpace[q][i]

			if w.Index == 0 {
				if !e.strongconnect(w, index) {
					for j := l; j < len(sccStack); j++ {
						sccStack[j].Index = 0
					}
					sccStack = sccStack[0:l]
					return false
				}
				if w.Lowlink < v.Lowlink {
					v.Lowlink = w.Lowlink
				}
			} else {
				if w.Index < v.Lowlink {
					v.Lowlink = w.Index
				}
			}
		}
	}

	if v.Lowlink == v.Index {
		// Found SCC — execute commands in increasing Seq order.
		list := sccStack[l:len(sccStack)]
		sort.Sort(sortBySeq(list))

		for _, w := range list {
			for w.Cmds == nil {
				time.Sleep(1000 * 1000)
			}
			flag := 0
			for idx := 0; idx < len(w.Cmds); idx++ {
				if w.Cmds[idx].Op == state.GET {
					flag = 1
					val := w.Cmds[idx].Execute(e.r.State)
					if e.r.Dreply && w.lb != nil && w.lb.clientProposals != nil && idx < len(w.lb.clientProposals) {
						e.r.ReplyProposeTS(
							&defs.ProposeReplyTS{
								OK:        TRUE,
								CommandId: w.lb.clientProposals[idx].CommandId,
								Value:     val,
								Timestamp: w.lb.clientProposals[idx].Timestamp,
							},
							w.lb.clientProposals[idx].Reply,
							w.lb.clientProposals[idx].Mutex)
					}
				} else {
					latestSeq := e.latestWriteSeq(w.Cmds[idx].K)
					if latestSeq < w.Seq {
						flag = 1
						val := w.Cmds[idx].Execute(e.r.State)
						e.r.maxWriteSeqPerKeyMu.Lock()
						e.r.maxWriteSeqPerKey[w.Cmds[idx].K] = w.Seq
						e.r.maxWriteSeqPerKeyMu.Unlock()
						e.r.maxWriteInstancePerKeyMu.Lock()
						e.r.maxWriteInstancePerKey[w.Cmds[idx].K] = w.instanceId
						e.r.maxWriteInstancePerKeyMu.Unlock()
						if e.r.Dreply && w.lb != nil && w.lb.clientProposals != nil && idx < len(w.lb.clientProposals) {
							e.r.ReplyProposeTS(
								&defs.ProposeReplyTS{
									OK:        TRUE,
									CommandId: w.lb.clientProposals[idx].CommandId,
									Value:     val,
									Timestamp: w.lb.clientProposals[idx].Timestamp,
								},
								w.lb.clientProposals[idx].Reply,
								w.lb.clientProposals[idx].Mutex)
						}
					} else {
						// Stale write — discard.
						if e.r.Dreply && w.lb != nil && w.lb.clientProposals != nil && idx < len(w.lb.clientProposals) {
							e.r.ReplyProposeTS(
								&defs.ProposeReplyTS{
									OK:        TRUE,
									CommandId: w.lb.clientProposals[idx].CommandId,
									Value:     execVAL,
									Timestamp: w.lb.clientProposals[idx].Timestamp,
								},
								w.lb.clientProposals[idx].Reply,
								w.lb.clientProposals[idx].Mutex)
						}
					}
				}
			}
			if flag == 1 {
				w.Status = EXECUTED
			} else {
				w.Status = DISCARDED
			}
			w.State = DONE
		}
		sccStack = sccStack[0:l]
	}

	return true
}

// sortBySeq sorts instances by sequence number (for deterministic SCC execution).
type sortBySeq []*Instance

func (s sortBySeq) Len() int           { return len(s) }
func (s sortBySeq) Less(i, j int) bool { return s[i].Seq < s[j].Seq }
func (s sortBySeq) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
