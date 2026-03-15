package mongotunable

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/imdea-software/swiftpaxos/state"
)

func makeCommand(op uint8, k int64, v []byte) state.Command {
	return state.Command{
		Op: state.Operation(op),
		K:  state.Key(k),
		V:  state.Value(v),
	}
}

func commandsEqual(a, b []state.Command) bool {
	return reflect.DeepEqual(a, b)
}

func TestPrepare_MarshalRoundTrip(t *testing.T) {
	orig := &Prepare{LeaderId: 1, Instance: 42, Ballot: 100, ToInfinity: 1}
	var buf bytes.Buffer
	orig.Marshal(&buf)

	got := &Prepare{}
	if err := got.Unmarshal(&buf); err != nil {
		t.Fatal(err)
	}
	if *got != *orig {
		t.Errorf("got %+v, want %+v", got, orig)
	}
}

func TestPrepareReply_MarshalRoundTrip(t *testing.T) {
	orig := &PrepareReply{
		Instance: 3,
		OK:       1,
		Ballot:   100,
		Command:  []state.Command{makeCommand(1, 10, []byte{20})},
	}
	var buf bytes.Buffer
	orig.Marshal(&buf)

	got := &PrepareReply{}
	if err := got.Unmarshal(&buf); err != nil {
		t.Fatal(err)
	}
	if got.Instance != orig.Instance || got.OK != orig.OK || got.Ballot != orig.Ballot {
		t.Errorf("scalar fields mismatch: got %+v, want %+v", got, orig)
	}
	if !commandsEqual(got.Command, orig.Command) {
		t.Errorf("Command mismatch: got %+v, want %+v", got.Command, orig.Command)
	}
}

func TestPrepareReply_EmptyCommand(t *testing.T) {
	orig := &PrepareReply{Instance: 5, OK: 0, Ballot: 50, Command: []state.Command{}}
	var buf bytes.Buffer
	orig.Marshal(&buf)

	got := &PrepareReply{}
	if err := got.Unmarshal(&buf); err != nil {
		t.Fatal(err)
	}
	if len(got.Command) != 0 {
		t.Errorf("expected empty command, got %d", len(got.Command))
	}
}

func TestAccept_MarshalRoundTrip(t *testing.T) {
	orig := &Accept{
		LeaderId:            2,
		Instance:            10,
		Ballot:              5,
		MajorityCommitPoint: 7,
		Deps:                3,
		Command:             []state.Command{makeCommand(0, 100, []byte{1, 2, 3}), makeCommand(1, 200, []byte{4, 5})},
	}
	var buf bytes.Buffer
	orig.Marshal(&buf)

	got := &Accept{}
	if err := got.Unmarshal(&buf); err != nil {
		t.Fatal(err)
	}
	if got.LeaderId != orig.LeaderId || got.Instance != orig.Instance ||
		got.Ballot != orig.Ballot || got.MajorityCommitPoint != orig.MajorityCommitPoint ||
		got.Deps != orig.Deps {
		t.Errorf("scalar fields mismatch: got %+v, want %+v", got, orig)
	}
	if !commandsEqual(got.Command, orig.Command) {
		t.Errorf("Command mismatch: got %+v, want %+v", got.Command, orig.Command)
	}
}

func TestAcceptReply_MarshalRoundTrip(t *testing.T) {
	orig := &AcceptReply{Instance: 42, OK: 1, Ballot: 99}
	var buf bytes.Buffer
	orig.Marshal(&buf)

	got := &AcceptReply{}
	if err := got.Unmarshal(&buf); err != nil {
		t.Fatal(err)
	}
	if *got != *orig {
		t.Errorf("got %+v, want %+v", got, orig)
	}
}

func TestCommit_MarshalRoundTrip(t *testing.T) {
	orig := &Commit{
		LeaderId:            1,
		Instance:            20,
		Ballot:              10,
		MajorityCommitPoint: 15,
		Deps:                8,
		Command:             []state.Command{makeCommand(1, 50, []byte{10, 20})},
	}
	var buf bytes.Buffer
	orig.Marshal(&buf)

	got := &Commit{}
	if err := got.Unmarshal(&buf); err != nil {
		t.Fatal(err)
	}
	if got.LeaderId != orig.LeaderId || got.Instance != orig.Instance ||
		got.Ballot != orig.Ballot || got.MajorityCommitPoint != orig.MajorityCommitPoint ||
		got.Deps != orig.Deps {
		t.Errorf("scalar fields mismatch: got %+v, want %+v", got, orig)
	}
	if !commandsEqual(got.Command, orig.Command) {
		t.Errorf("Command mismatch: got %+v, want %+v", got.Command, orig.Command)
	}
}

func TestCommitShort_MarshalRoundTrip(t *testing.T) {
	orig := &CommitShort{
		LeaderId:            3,
		Instance:            50,
		Count:               10,
		Ballot:              25,
		MajorityCommitPoint: 40,
	}
	var buf bytes.Buffer
	orig.Marshal(&buf)

	got := &CommitShort{}
	if err := got.Unmarshal(&buf); err != nil {
		t.Fatal(err)
	}
	if *got != *orig {
		t.Errorf("got %+v, want %+v", got, orig)
	}
}

func TestCommitAck_MarshalRoundTrip(t *testing.T) {
	orig := &CommitAck{Instance: 77}
	var buf bytes.Buffer
	orig.Marshal(&buf)

	got := &CommitAck{}
	if err := got.Unmarshal(&buf); err != nil {
		t.Fatal(err)
	}
	if *got != *orig {
		t.Errorf("got %+v, want %+v", got, orig)
	}
}

func TestPrepareCache_GetPut(t *testing.T) {
	cache := NewPrepareCache()
	p := &Prepare{LeaderId: 1, Instance: 2, Ballot: 3, ToInfinity: 1}
	cache.Put(p)
	got := cache.Get()
	if got != p {
		t.Errorf("expected same pointer from cache")
	}
	// Get from empty cache returns new
	got2 := cache.Get()
	if got2 == nil {
		t.Errorf("expected non-nil from empty cache")
	}
}

func TestAcceptCache_GetPut(t *testing.T) {
	cache := NewAcceptCache()
	a := &Accept{LeaderId: 1, Instance: 2}
	cache.Put(a)
	got := cache.Get()
	if got != a {
		t.Errorf("expected same pointer from cache")
	}
}

func TestCommitAckCache_GetPut(t *testing.T) {
	cache := NewCommitAckCache()
	c := &CommitAck{Instance: 5}
	cache.Put(c)
	got := cache.Get()
	if got != c {
		t.Errorf("expected same pointer from cache")
	}
}

func TestBinarySize(t *testing.T) {
	tests := []struct {
		name      string
		size      int
		sizeKnown bool
	}{
		{"Prepare", 13, true},
		{"AcceptReply", 9, true},
		{"CommitShort", 20, true},
		{"CommitAck", 4, true},
	}
	msgs := []interface {
		BinarySize() (int, bool)
	}{
		&Prepare{}, &AcceptReply{}, &CommitShort{}, &CommitAck{},
	}
	for i, tt := range tests {
		size, known := msgs[i].BinarySize()
		if size != tt.size || known != tt.sizeKnown {
			t.Errorf("%s: BinarySize()=(%d,%v), want (%d,%v)", tt.name, size, known, tt.size, tt.sizeKnown)
		}
	}

	// Variable-size messages
	varMsgs := []interface {
		BinarySize() (int, bool)
	}{
		&PrepareReply{}, &Accept{}, &Commit{},
	}
	for _, m := range varMsgs {
		_, known := m.BinarySize()
		if known {
			t.Errorf("expected sizeKnown=false for variable-size message")
		}
	}
}

func TestNegativeValues_MarshalRoundTrip(t *testing.T) {
	orig := &Accept{
		LeaderId:            -1,
		Instance:            -100,
		Ballot:              -50,
		MajorityCommitPoint: -25,
		Deps:                -10,
		Command:             []state.Command{makeCommand(1, -999, []byte{0xFF})},
	}
	var buf bytes.Buffer
	orig.Marshal(&buf)

	got := &Accept{}
	if err := got.Unmarshal(&buf); err != nil {
		t.Fatal(err)
	}
	if got.LeaderId != orig.LeaderId || got.Instance != orig.Instance ||
		got.Ballot != orig.Ballot || got.MajorityCommitPoint != orig.MajorityCommitPoint ||
		got.Deps != orig.Deps {
		t.Errorf("scalar fields mismatch with negative values: got %+v, want %+v", got, orig)
	}
}

func TestMultipleCommands_MarshalRoundTrip(t *testing.T) {
	cmds := make([]state.Command, 10)
	for i := range cmds {
		cmds[i] = makeCommand(uint8(i%2), int64(i*100), []byte{byte(i)})
	}
	orig := &Commit{
		LeaderId:            1,
		Instance:            1,
		Ballot:              1,
		MajorityCommitPoint: 0,
		Deps:                0,
		Command:             cmds,
	}
	var buf bytes.Buffer
	orig.Marshal(&buf)

	got := &Commit{}
	if err := got.Unmarshal(&buf); err != nil {
		t.Fatal(err)
	}
	if len(got.Command) != 10 {
		t.Errorf("expected 10 commands, got %d", len(got.Command))
	}
	if !commandsEqual(got.Command, orig.Command) {
		t.Errorf("commands mismatch")
	}
}

func TestConstants(t *testing.T) {
	// Verify RPC type constants
	if PREPARE != 0 || PREPARE_REPLY != 1 || ACCEPT != 2 || ACCEPT_REPLY != 3 || COMMIT != 4 || COMMIT_SHORT != 5 {
		t.Errorf("RPC type constants have unexpected values")
	}

	// Verify status constants
	if PREPARING != 0 || PREPARED != 1 || ACCEPTED != 2 || COMMITTED != 3 || EXECUTED != 4 || DISCARDED != 5 {
		t.Errorf("status constants have unexpected values")
	}

	// Verify consistency level constants
	if CL_STRONG != 0 || CL_CAUSAL != 1 {
		t.Errorf("CL constants have unexpected values")
	}
}
