package epaxosho

import (
	"github.com/imdea-software/swiftpaxos/state"
)

// Instance status constants for EPaxos-HO.
const (
	NONE int8 = iota
	PREACCEPTED
	PREACCEPTED_EQ
	CAUSAL_ACCEPTED
	ACCEPTED
	CAUSALLY_COMMITTED
	STRONGLY_COMMITTED
	EXECUTED
	DISCARDED
	READY
	WAITING
	DONE
)

// --- Message types ---

type Prepare struct {
	LeaderId int32
	Replica  int32
	Instance int32
	Ballot   int32
}

type PrepareReply struct {
	AcceptorId int32
	Replica    int32
	Instance   int32
	OK         uint8
	Bal        int32
	VBal       int32
	Status     int8
	Command    []state.Command
	Seq        int32
	Deps       []int32
	CL         []int32
}

type PreAccept struct {
	LeaderId int32
	Replica  int32
	Instance int32
	Ballot   int32
	Command  []state.Command
	Seq      int32
	Deps     []int32
	CL       []int32
}

type PreAcceptReply struct {
	Replica       int32
	Instance      int32
	OK            uint8
	Ballot        int32
	Seq           int32
	Deps          []int32
	CL            []int32
	CommittedDeps []int32
}

type PreAcceptOK struct {
	Instance int32
}

type Accept struct {
	LeaderId int32
	Replica  int32
	Instance int32
	Ballot   int32
	Count    int32
	Seq      int32
	Deps     []int32
	CL       []int32
}

type AcceptReply struct {
	Replica  int32
	Instance int32
	OK       uint8
	Ballot   int32
}

type Commit struct {
	Consistency state.Operation
	LeaderId    int32
	Replica     int32
	Instance    int32
	Command     []state.Command
	Seq         int32
	Deps        []int32
	CL          []int32
}

type CausalCommit struct {
	Consistency state.Operation
	LeaderId    int32
	Replica     int32
	Instance    int32
	Command     []state.Command
	Seq         int32
	Deps        []int32
	CL          []int32
}

type CommitShort struct {
	Consistency state.Operation
	LeaderId    int32
	Replica     int32
	Instance    int32
	Count       int32
	Seq         int32
	Deps        []int32
	CL          []int32
}

type TryPreAccept struct {
	LeaderId int32
	Replica  int32
	Instance int32
	Ballot   int32
	Command  []state.Command
	Seq      int32
	CL       []int32
	Deps     []int32
}

type TryPreAcceptReply struct {
	AcceptorId       int32
	Replica          int32
	Instance         int32
	OK               uint8
	Ballot           int32
	ConflictReplica  int32
	ConflictInstance int32
	ConflictStatus   int8
}
