// Package pileus implements Pileus-style consistency on top of Raft-HT.
//
// Server-side: identical to Raft-HT (and MongoDB-Tunable). All log replication,
// broadcast, commit, and election logic is delegated to the underlying Raft-HT replica.
//
// Client-side: forces ALL writes to the strong path (no weak writes). Weak reads
// use the same causal MinIndex mechanism as MongoDB-Tunable: the client tracks the
// last strong write's log index and sends it as MinIndex in weak read requests.
package pileus

import (
	"github.com/imdea-software/swiftpaxos/config"
	"github.com/imdea-software/swiftpaxos/dlog"
	raftht "github.com/imdea-software/swiftpaxos/raft-ht"
)

// Replica is a Pileus replica (delegates to Raft-HT).
type Replica = raftht.Replica

// New creates a Pileus replica. The server behavior is identical
// to Raft-HT; the Pileus difference is entirely client-side.
func New(alias string, id int, addrs []string, isLeader bool, f int,
	conf *config.Config, logger *dlog.Logger) *Replica {
	return raftht.New(alias, id, addrs, isLeader, f, conf, logger)
}
