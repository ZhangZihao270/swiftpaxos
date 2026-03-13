package epaxos

import (
	"sort"
	"time"

	"github.com/imdea-software/swiftpaxos/replica/defs"
	"github.com/imdea-software/swiftpaxos/state"
)

const MAX_INSTANCE = 10 * 1024 * 1024

const MAX_DEPTH_DEP = 10
const TRUE = uint8(1)
const FALSE = uint8(0)
const ADAPT_TIME_SEC = 10

const COMMIT_GRACE_PERIOD = 10 * 1e9 // 10 second(s)

const BF_K = 4
const BF_M_N = 32.0

const HT_INIT_SIZE = 200000

type InstanceId struct {
	Replica  int32
	Instance int32
}

type InstPair struct {
	Last      int32
	LastWrite int32
}

type Instance struct {
	Cmds           []state.Command
	Bal, Vbal      int32
	Status         int8
	Seq            int32
	Deps           []int32
	Lb             *LeaderBookkeeping
	Index, Lowlink int
	Bfilter        any
	ProposeTime    int64
	Id             *InstanceId
}

type LeaderBookkeeping struct {
	ClientProposals   []*defs.GPropose
	Ballot            int32
	AllEqual          bool
	PreAcceptOKs      int
	AcceptOKs         int
	Nacks             int
	OriginalDeps      []int32
	CommittedDeps     []int32
	PrepareReplies    []*PrepareReply
	Preparing         bool
	TryingToPreAccept bool
	PossibleQuorum    []bool
	TpaReps           int
	TpaAccepted       bool
	LastTriedBallot   int32
	Cmds              []state.Command
	Status            int8
	Seq               int32
	Deps              []int32
	LeaderResponded   bool
}

// NewInstance creates a new Instance with the given parameters.
func NewInstance(replica, instance int32, cmds []state.Command, cballot, lballot int32, status int8, seq int32, deps []int32) *Instance {
	return &Instance{
		Cmds:        cmds,
		Bal:         cballot,
		Vbal:        lballot,
		Status:      status,
		Seq:         seq,
		Deps:        deps,
		ProposeTime: time.Now().UnixNano(),
		Id:          &InstanceId{replica, instance},
	}
}

// IsInitialBallot checks whether the given ballot is the initial ballot
// assigned to a replica (i.e., equal to the replica's ID).
func IsInitialBallot(ballot int32, replica int32) bool {
	return ballot == replica
}

// MakeBallot computes a unique ballot number for a given replica.
// The ballot is guaranteed to be > maxRecvBallot if isLeader is true,
// and is always congruent to replicaId mod n (ensuring uniqueness across replicas).
func MakeBallot(replicaId int32, ownerReplica int32, n int, maxRecvBallot int32, isLeader bool) int32 {
	b := replicaId
	if replicaId != ownerReplica {
		b += int32(n)
	}
	if isLeader {
		for b < maxRecvBallot {
			b += int32(n)
		}
	}
	return b
}

// NodeArray implements sort.Interface for sorting instances by Seq
// (and by replica ID and propose time as tiebreakers).
type NodeArray []*Instance

func (na NodeArray) Len() int {
	return len(na)
}

func (na NodeArray) Less(i, j int) bool {
	return na[i].Seq < na[j].Seq ||
		(na[i].Seq == na[j].Seq && na[i].Id.Replica < na[j].Id.Replica) ||
		(na[i].Seq == na[j].Seq && na[i].Id.Replica == na[j].Id.Replica && na[i].ProposeTime < na[j].ProposeTime)
}

func (na NodeArray) Swap(i, j int) {
	na[i], na[j] = na[j], na[i]
}

// SortInstances sorts a slice of instances using the EPaxos ordering:
// by Seq ascending, then replica ID, then propose time.
func SortInstances(instances []*Instance) {
	sort.Sort(NodeArray(instances))
}
