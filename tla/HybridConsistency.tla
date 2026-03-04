---- MODULE HybridConsistency ----
\* Hybrid Consistency property definitions.
\*
\* This module defines operators for checking three properties:
\*   1. Linearizability of strong operations
\*   2. Causal consistency of all operations
\*   3. Hybrid compatibility (no contradiction between total and causal orders)
\*
\* These operators are parameterized over a history (sequence of completed
\* operation records) and are used by RaftHT.tla as invariants.
\*
\* Formal background:
\*   - Partial order (prec_P): causal order over ALL operations
\*     - Session Order Preservation: same-session ops ordered by session order
\*     - Read-From Preservation: if o2 reads o1's write, then o1 prec_P o2
\*     - Causal Closure: transitive closure of the above
\*   - Total order (prec_T): linearizable order over O_T (superset of strong ops)
\*     - Completeness: total over O_T
\*     - Read-From Preservation: read in O_T => write source in O_T
\*     - Real-Time Respect: real-time order preserved for O_T
\*   - Hybrid Consistency:
\*     1. Strong Operation Inclusion: all strong ops in O_T
\*     2. Causality Preservation: o1 prec_P o2 => o2 observes o1's write
\*     3. Hybrid Compatibility: NOT (o1 prec_T o2 AND o2 prec_P o1)

EXTENDS Integers, Sequences, FiniteSets

CONSTANTS
    Keys,
    Values,
    Nil,
    Strong,
    Weak,
    Read,
    Write

\* ============================================================================
\* History Entry Structure
\* ============================================================================
\*
\* Each entry in the history sequence has the following fields:
\*   client:      Client ID
\*   op:          "Read" or "Write"
\*   key:         Key operated on
\*   reqVal:      Requested value (for writes; Nil for reads)
\*   retVal:      Returned value
\*   consistency: "Strong" or "Weak"
\*   invClock:    Monotonic clock at invocation (for real-time ordering)
\*   retClock:    Monotonic clock at return
\*   slot:        Log slot (for operations that go through the log; 0 otherwise)

\* ============================================================================
\* Task 55.7: Linearizability Checking
\* ============================================================================

\* Extract strong operations from history
\* StrongOps(hist) == ...

\* Check that strong ops can be linearized:
\* There exists a total order consistent with slot order, respecting real-time,
\* where each read returns the value of the most recent preceding write on
\* the same key.
\* IsLinearizable(hist) == ...

\* ============================================================================
\* Task 55.8: Causal Consistency Checking
\* ============================================================================

\* Build the causal graph from history:
\*   - Session order edges: same client, earlier invClock
\*   - Read-from edges: read returns value written by a specific write
\* CausalEdges(hist) == ...

\* Check that the causal graph is acyclic and reads are consistent:
\* For each read R of key k, the returned value is from the causally-latest
\* write to k in R's causal past.
\* IsCausallyConsistent(hist) == ...

\* ============================================================================
\* Task 55.9: Hybrid Compatibility Checking
\* ============================================================================

\* Construct O_T: all strong ops + weak writes read by strong ops
\* ConstructOT(hist) == ...

\* Construct prec_T: total order over O_T (log slot order for committed ops)
\* TotalOrder(hist) == ...

\* Check: for no pair (o1, o2) in O_T do we have o1 prec_T o2 AND o2 prec_P o1
\* IsHybridCompatible(hist) == ...

\* ============================================================================
\* Combined Check
\* ============================================================================

\* SatisfiesHybridConsistency(hist) ==
\*     /\ IsLinearizable(hist)
\*     /\ IsCausallyConsistent(hist)
\*     /\ IsHybridCompatible(hist)

====
