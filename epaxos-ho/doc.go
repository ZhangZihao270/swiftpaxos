// Package epaxosho implements EPaxos-HO (Hybrid Optimal), a leaderless
// consensus protocol that supports hybrid consistency levels (causal and strong).
//
// Ported from Orca's hybrid protocol implementation.
// Causal ops use 1-RTT fast commit with causal dependency tracking.
// Strong ops use EPaxos-style 2-RTT commit with SCC-based execution ordering.
package epaxosho
