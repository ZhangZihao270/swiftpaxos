---- MODULE EPaxosHO_Orca_Properties ----
\* Hybrid consistency properties for EPaxosHO, written in our framework's style.
\*
\* These properties are equivalent to our RaftHT/CurpHT invariants but expressed
\* over Orca's cmdLog state (protocol-centric) rather than a client history
\* (client-centric). No modifications to the original spec are needed.
\*
\* Mapping from our framework to Orca's state:
\*   - client/session  →  ctxid (context ID)
\*   - slot            →  inst (instance = <<replica, seq>>)
\*   - consistency     →  consistency ("strong" / "causal")
\*   - op type         →  cmd.op.type ("w" = write, "r" = read)
\*   - key             →  cmd.op.key
\*   - commit_order    →  real-time ordering proxy for strong ops
\*   - execution_order →  execution sequence on a replica
\*
\* We check three classes of properties:
\*   1. LinearizabilityInv  — strong operations are linearizable
\*   2. CausalConsistencyInv — all operations satisfy session guarantees
\*   3. HybridCompatibilityInv — total and causal orders don't contradict

EXTENDS EPaxosHO_Orca

\* ============================================================================
\* Helper: completed operations on a replica
\* ============================================================================

\* An operation is "completed" if it has reached a committed or executed state
IsCompleted(rec) ==
    rec.status \in {"causally-committed", "strongly-committed", "executed", "discarded"}

\* An operation is fully executed
IsExecuted(rec) ==
    rec.status \in {"executed", "discarded"}

\* Filter to strong operations
IsStrong(rec) == rec.consistency = "strong"

\* Filter to causal operations
IsCausal(rec) == rec.consistency = "causal"

\* Check if a record is a write
IsWrite(rec) == rec.cmd.op.type = "w"

\* Check if a record is a read
IsRead(rec) == rec.cmd.op.type = "r"

\* Instance ordering: inst1 < inst2 (lexicographic on <<replica, seq>>)
InstLT(i1, i2) ==
    \/ i1[1] < i2[1]
    \/ (i1[1] = i2[1] /\ i1[2] < i2[2])

\* ============================================================================
\* 1. Linearizability of Strong Operations
\* ============================================================================
\*
\* (a) RealTimeRespect: if strong op γ completes before strong op δ starts
\*     (commit_order tracks this: higher commit_order = later in real time),
\*     then γ must be executed before δ on every replica.
\*
\* This is equivalent to Orca's RealTimeOrderingOfStrong but stated more
\* precisely: commit_order respects execution_order.

OrcaRealTimeRespect ==
    \A replica \in Replicas :
        \A rec1, rec2 \in cmdLog[replica] :
            /\ IsStrong(rec1)
            /\ IsStrong(rec2)
            /\ IsExecuted(rec1)
            /\ IsExecuted(rec2)
            /\ rec1.commit_order > 0
            /\ rec2.commit_order > 0
            /\ rec1.commit_order < rec2.commit_order
            => rec1.execution_order < rec2.execution_order

\* (b) StrongReadConsistency: a strong read must return the value of the
\*     latest write (by execution order) on the same key that was executed
\*     before the read.
\*
\* Since Orca doesn't track return values explicitly, we check a weaker
\* but equivalent property: all dependent writes of a strong read are
\* majority-committed (ensuring the read observes them).
\* This is equivalent to Orca's GlobalOrderingOfRead.

OrcaStrongReadConsistency ==
    \A replica \in Replicas :
        \A rec \in cmdLog[replica] :
            /\ IsStrong(rec)
            /\ IsRead(rec)
            /\ IsExecuted(rec)
            => CheckDependentWrite(rec, replica)

OrcaLinearizabilityInv == OrcaRealTimeRespect /\ OrcaStrongReadConsistency

\* ============================================================================
\* 2. Causal Consistency (Session Guarantees)
\* ============================================================================

\* (a) SameSessionOrdering (= ReadYourWrites + MonotonicWrites + MonotonicReads):
\*     Within a session (same ctxid), operations must be executed in the order
\*     they were issued (instance number order).
\*
\* This is equivalent to Orca's SameSessionCausality.

OrcaSameSessionOrdering ==
    \A replica \in Replicas :
        \A rec1, rec2 \in cmdLog[replica] :
            /\ IsCompleted(rec1)
            /\ IsCompleted(rec2)
            /\ rec1.ctxid = rec2.ctxid
            /\ rec1.ctxid # 0
            /\ IsExecuted(rec1)
            /\ IsExecuted(rec2)
            /\ InstLT(rec1.inst, rec2.inst)
            => rec1.execution_order < rec2.execution_order

\* (b) GetFromCausality (= WritesFollowReads + ReadsReturnValidValues):
\*     If a read R depends on write W (W ∈ R.deps), then W must be
\*     executed before any operation that comes after R in instance order.
\*
\* This is equivalent to Orca's GetFromCausality.

OrcaGetFromCausality ==
    \A replica \in Replicas :
        \A rec \in cmdLog[replica] :
            /\ IsRead(rec)
            /\ IsExecuted(rec)
            => LET maxWriteInst == MaxWriteInstance(replica, rec.deps)
                   maxWriteExecOrder == maxWriteInst.execution_order
                   laterRecs == {r \in cmdLog[replica] : r.inst[2] > rec.inst[2]}
               IN
                   IF laterRecs = {} THEN TRUE
                   ELSE LET minLaterExecOrder == MinExecutionOrderRecs(laterRecs).execution_order
                        IN minLaterExecOrder > maxWriteExecOrder

OrcaCausalConsistencyInv ==
    /\ OrcaSameSessionOrdering
    /\ OrcaGetFromCausality

\* ============================================================================
\* 3. Hybrid Compatibility
\* ============================================================================
\*
\* The total order (slot/instance order for committed ops) must not contradict
\* the causal order (session order). If op1 is ordered before op2 in the total
\* order and they are from the same session, then op1 must have been issued
\* before op2 (i.e., op1's instance number < op2's instance number within the
\* same session).
\*
\* In EPaxos, each replica may have different execution orders, so we check
\* on each replica: for same-session ops, execution order matches instance order.

OrcaHybridCompatibilityInv ==
    \A replica \in Replicas :
        \A rec1, rec2 \in cmdLog[replica] :
            /\ IsExecuted(rec1)
            /\ IsExecuted(rec2)
            /\ rec1.ctxid = rec2.ctxid
            /\ rec1.ctxid # 0
            /\ rec1.execution_order > 0
            /\ rec2.execution_order > 0
            /\ rec1.execution_order < rec2.execution_order
            => InstLT(rec1.inst, rec2.inst)

\* ============================================================================
\* 4. Convergence (EPaxos-specific)
\* ============================================================================
\*
\* When all operations are executed/discarded, all replicas agree on the latest
\* write for each key. This is not part of our hybrid consistency definition
\* but is an EPaxos safety property.

OrcaConvergence ==
    LET allDone == \A r \in Replicas :
                     \A rec \in cmdLog[r] :
                       rec.status \in {"executed", "discarded"}
    IN allDone =>
       \A key \in Keys :
           LET latestWrites == LatestWriteofSpecificKey(key)
           IN \A w1, w2 \in latestWrites : w1.inst = w2.inst

\* ============================================================================
\* Combined Safety Invariant
\* ============================================================================

OrcaSafetyInv ==
    /\ OrcaLinearizabilityInv
    /\ OrcaCausalConsistencyInv
    /\ OrcaHybridCompatibilityInv

====
