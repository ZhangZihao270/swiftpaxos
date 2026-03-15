// Package mongotunable implements MongoDB-Tunable consistency on top of Raft-HT.
//
// Server-side: identical to Raft-HT. The causal tracking (MinIndex wait) is
// built into Raft-HT's processWeakRead. All log replication, broadcast,
// commit, and election logic is delegated to the underlying Raft-HT replica.
//
// Client-side: tracks the last weak write's log index (from MWeakReply.Slot)
// and sends it as MinIndex in weak read requests, providing read-your-writes
// causal consistency for weak operations.
package mongotunable

import (
	"github.com/imdea-software/swiftpaxos/config"
	"github.com/imdea-software/swiftpaxos/dlog"
	raftht "github.com/imdea-software/swiftpaxos/raft-ht"
)

// Replica is a MongoDB-Tunable replica (delegates to Raft-HT).
type Replica = raftht.Replica

// New creates a MongoDB-Tunable replica. The server behavior is identical
// to Raft-HT; the causal tracking difference is entirely client-side.
func New(alias string, id int, addrs []string, isLeader bool, f int,
	conf *config.Config, logger *dlog.Logger) *Replica {
	return raftht.New(alias, id, addrs, isLeader, f, conf, logger)
}
