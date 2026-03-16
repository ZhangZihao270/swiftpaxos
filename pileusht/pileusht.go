// Package pileusht implements Pileus-HT: Pileus with fast weak writes.
//
// Server-side: identical to Raft-HT. All log replication, broadcast, commit,
// and election logic is delegated to the underlying Raft-HT replica.
//
// Client-side: weak writes get fast leader reply (like Raft-HT/MongoDB-Tunable),
// instead of going through the strong majority-commit path (like plain Pileus).
// Weak reads use causal MinIndex tracking (read-your-writes from weak writes).
//
// Key difference from Raft-HT: weak reads enforce causal ordering via MinIndex.
// Key difference from Pileus: weak writes get immediate leader reply (not majority commit).
// Equivalent to MongoDB-Tunable in behavior; differs in conceptual framing.
package pileusht

import (
	"github.com/imdea-software/swiftpaxos/config"
	"github.com/imdea-software/swiftpaxos/dlog"
	raftht "github.com/imdea-software/swiftpaxos/raft-ht"
)

// Replica is a Pileus-HT replica (delegates to Raft-HT).
type Replica = raftht.Replica

// New creates a Pileus-HT replica. The server behavior is identical
// to Raft-HT; the Pileus-HT difference is entirely client-side.
func New(alias string, id int, addrs []string, isLeader bool, f int,
	conf *config.Config, logger *dlog.Logger) *Replica {
	return raftht.New(alias, id, addrs, isLeader, f, conf, logger)
}
