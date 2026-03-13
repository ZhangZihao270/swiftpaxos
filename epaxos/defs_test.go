package epaxos

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
	orig := &Prepare{LeaderId: 1, Replica: 2, Instance: 3, Ballot: 100}
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
		AcceptorId: 1,
		Replica:    2,
		Instance:   3,
		Ballot:     100,
		Status:     PREACCEPTED,
		Command:    []state.Command{makeCommand(1, 10, []byte{20})},
		Seq:        5,
		Deps:       []int32{1, 2, 3},
	}
	var buf bytes.Buffer
	orig.Marshal(&buf)

	got := &PrepareReply{}
	if err := got.Unmarshal(&buf); err != nil {
		t.Fatal(err)
	}
	if got.AcceptorId != orig.AcceptorId || got.Replica != orig.Replica ||
		got.Instance != orig.Instance || got.Ballot != orig.Ballot ||
		got.Status != orig.Status || got.Seq != orig.Seq {
		t.Errorf("scalar fields mismatch: got %+v, want %+v", got, orig)
	}
	if !commandsEqual(got.Command, orig.Command) {
		t.Errorf("Command mismatch")
	}
	if len(got.Deps) != 3 || got.Deps[0] != 1 || got.Deps[1] != 2 || got.Deps[2] != 3 {
		t.Errorf("Deps mismatch: %v", got.Deps)
	}
}

func TestPreAccept_MarshalRoundTrip(t *testing.T) {
	orig := &PreAccept{
		LeaderId: 0,
		Replica:  1,
		Instance: 42,
		Ballot:   7,
		Command:  []state.Command{makeCommand(0, 5, []byte{10}), makeCommand(1, 6, []byte{11})},
		Seq:      3,
		Deps:     []int32{10, 20, 30},
	}
	var buf bytes.Buffer
	orig.Marshal(&buf)

	got := &PreAccept{}
	if err := got.Unmarshal(&buf); err != nil {
		t.Fatal(err)
	}
	if got.LeaderId != orig.LeaderId || got.Replica != orig.Replica ||
		got.Instance != orig.Instance || got.Ballot != orig.Ballot || got.Seq != orig.Seq {
		t.Errorf("scalar mismatch")
	}
	if !commandsEqual(got.Command, orig.Command) {
		t.Errorf("Command mismatch")
	}
	if len(got.Deps) != 3 {
		t.Fatalf("Deps len = %d, want 3", len(got.Deps))
	}
	for i := range orig.Deps {
		if got.Deps[i] != orig.Deps[i] {
			t.Errorf("Deps[%d] = %d, want %d", i, got.Deps[i], orig.Deps[i])
		}
	}
}

func TestPreAcceptReply_MarshalRoundTrip(t *testing.T) {
	orig := &PreAcceptReply{
		Replica:       2,
		Instance:      5,
		Ballot:        10,
		VBallot:       9,
		Seq:           3,
		Deps:          []int32{1, 2},
		CommittedDeps: []int32{0, 1},
		Status:        ACCEPTED,
	}
	var buf bytes.Buffer
	orig.Marshal(&buf)

	got := &PreAcceptReply{}
	if err := got.Unmarshal(&buf); err != nil {
		t.Fatal(err)
	}
	if got.Replica != orig.Replica || got.Instance != orig.Instance ||
		got.Ballot != orig.Ballot || got.VBallot != orig.VBallot ||
		got.Seq != orig.Seq || got.Status != orig.Status {
		t.Errorf("scalar mismatch: got %+v, want %+v", got, orig)
	}
	if len(got.Deps) != 2 || got.Deps[0] != 1 || got.Deps[1] != 2 {
		t.Errorf("Deps mismatch: %v", got.Deps)
	}
	if len(got.CommittedDeps) != 2 || got.CommittedDeps[0] != 0 || got.CommittedDeps[1] != 1 {
		t.Errorf("CommittedDeps mismatch: %v", got.CommittedDeps)
	}
}

func TestPreAcceptOK_MarshalRoundTrip(t *testing.T) {
	orig := &PreAcceptOK{Instance: 42}
	var buf bytes.Buffer
	orig.Marshal(&buf)

	got := &PreAcceptOK{}
	if err := got.Unmarshal(&buf); err != nil {
		t.Fatal(err)
	}
	if *got != *orig {
		t.Errorf("got %+v, want %+v", got, orig)
	}
}

func TestAccept_MarshalRoundTrip(t *testing.T) {
	orig := &Accept{
		LeaderId: 0,
		Replica:  1,
		Instance: 10,
		Ballot:   5,
		Seq:      3,
		Deps:     []int32{7, 8, 9},
	}
	var buf bytes.Buffer
	orig.Marshal(&buf)

	got := &Accept{}
	if err := got.Unmarshal(&buf); err != nil {
		t.Fatal(err)
	}
	if got.LeaderId != orig.LeaderId || got.Replica != orig.Replica ||
		got.Instance != orig.Instance || got.Ballot != orig.Ballot || got.Seq != orig.Seq {
		t.Errorf("scalar mismatch")
	}
	if len(got.Deps) != 3 {
		t.Fatalf("Deps len = %d, want 3", len(got.Deps))
	}
	for i := range orig.Deps {
		if got.Deps[i] != orig.Deps[i] {
			t.Errorf("Deps[%d] mismatch", i)
		}
	}
}

func TestAcceptReply_MarshalRoundTrip(t *testing.T) {
	orig := &AcceptReply{Replica: 1, Instance: 5, Ballot: 10}
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
		LeaderId: 0,
		Replica:  2,
		Instance: 15,
		Ballot:   3,
		Command:  []state.Command{makeCommand(1, 100, []byte{200})},
		Seq:      7,
		Deps:     []int32{4, 5},
	}
	var buf bytes.Buffer
	orig.Marshal(&buf)

	got := &Commit{}
	if err := got.Unmarshal(&buf); err != nil {
		t.Fatal(err)
	}
	if got.LeaderId != orig.LeaderId || got.Replica != orig.Replica ||
		got.Instance != orig.Instance || got.Ballot != orig.Ballot || got.Seq != orig.Seq {
		t.Errorf("scalar mismatch")
	}
	if !commandsEqual(got.Command, orig.Command) {
		t.Errorf("Command mismatch")
	}
	if len(got.Deps) != 2 || got.Deps[0] != 4 || got.Deps[1] != 5 {
		t.Errorf("Deps mismatch: %v", got.Deps)
	}
}

func TestTryPreAccept_MarshalRoundTrip(t *testing.T) {
	orig := &TryPreAccept{
		LeaderId: 1,
		Replica:  0,
		Instance: 8,
		Ballot:   12,
		Command:  []state.Command{makeCommand(0, 50, []byte{60})},
		Seq:      4,
		Deps:     []int32{2, 3},
	}
	var buf bytes.Buffer
	orig.Marshal(&buf)

	got := &TryPreAccept{}
	if err := got.Unmarshal(&buf); err != nil {
		t.Fatal(err)
	}
	if got.LeaderId != orig.LeaderId || got.Replica != orig.Replica ||
		got.Instance != orig.Instance || got.Ballot != orig.Ballot || got.Seq != orig.Seq {
		t.Errorf("scalar mismatch")
	}
	if !commandsEqual(got.Command, orig.Command) {
		t.Errorf("Command mismatch")
	}
	if len(got.Deps) != 2 || got.Deps[0] != 2 || got.Deps[1] != 3 {
		t.Errorf("Deps mismatch: %v", got.Deps)
	}
}

func TestTryPreAcceptReply_MarshalRoundTrip(t *testing.T) {
	orig := &TryPreAcceptReply{
		AcceptorId:       1,
		Replica:          2,
		Instance:         3,
		Ballot:           10,
		VBallot:          9,
		ConflictReplica:  0,
		ConflictInstance: 5,
		ConflictStatus:   COMMITTED,
	}
	var buf bytes.Buffer
	orig.Marshal(&buf)

	got := &TryPreAcceptReply{}
	if err := got.Unmarshal(&buf); err != nil {
		t.Fatal(err)
	}
	if *got != *orig {
		t.Errorf("got %+v, want %+v", got, orig)
	}
}

func TestEmptySlices_MarshalRoundTrip(t *testing.T) {
	orig := &PreAccept{
		LeaderId: 0,
		Replica:  1,
		Instance: 1,
		Ballot:   1,
		Command:  []state.Command{},
		Seq:      0,
		Deps:     []int32{},
	}
	var buf bytes.Buffer
	orig.Marshal(&buf)

	got := &PreAccept{}
	if err := got.Unmarshal(&buf); err != nil {
		t.Fatal(err)
	}
	if len(got.Command) != 0 {
		t.Errorf("Command len = %d, want 0", len(got.Command))
	}
	if len(got.Deps) != 0 {
		t.Errorf("Deps len = %d, want 0", len(got.Deps))
	}
}

func TestCacheGetPut(t *testing.T) {
	c := NewPreAcceptCache()

	// Get from empty cache returns fresh instance.
	pa := c.Get()
	if pa == nil {
		t.Fatal("Get() returned nil")
	}

	// Put and Get returns the same pointer.
	pa.Instance = 42
	c.Put(pa)
	pa2 := c.Get()
	if pa2 != pa {
		t.Error("expected same pointer from cache")
	}
	if pa2.Instance != 42 {
		t.Errorf("Instance = %d, want 42", pa2.Instance)
	}
}

func TestNew_ReturnsFreshInstance(t *testing.T) {
	p := &Prepare{}
	s := p.New()
	if s == nil {
		t.Fatal("New() returned nil")
	}
	if _, ok := s.(*Prepare); !ok {
		t.Error("New() returned wrong type")
	}
}

func TestBinarySize(t *testing.T) {
	tests := []struct {
		name      string
		size      int
		sizeKnown bool
	}{
		{"Prepare", 16, true},
		{"PreAcceptOK", 4, true},
		{"AcceptReply", 13, true},
		{"TryPreAcceptReply", 26, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var size int
			var known bool
			switch tt.name {
			case "Prepare":
				size, known = (&Prepare{}).BinarySize()
			case "PreAcceptOK":
				size, known = (&PreAcceptOK{}).BinarySize()
			case "AcceptReply":
				size, known = (&AcceptReply{}).BinarySize()
			case "TryPreAcceptReply":
				size, known = (&TryPreAcceptReply{}).BinarySize()
			}
			if size != tt.size || known != tt.sizeKnown {
				t.Errorf("BinarySize() = (%d, %v), want (%d, %v)", size, known, tt.size, tt.sizeKnown)
			}
		})
	}

	// Variable-size types should return sizeKnown=false.
	varTypes := []string{"PrepareReply", "PreAccept", "PreAcceptReply", "Accept", "Commit", "TryPreAccept"}
	for _, name := range varTypes {
		t.Run(name+"_variable", func(t *testing.T) {
			var known bool
			switch name {
			case "PrepareReply":
				_, known = (&PrepareReply{}).BinarySize()
			case "PreAccept":
				_, known = (&PreAccept{}).BinarySize()
			case "PreAcceptReply":
				_, known = (&PreAcceptReply{}).BinarySize()
			case "Accept":
				_, known = (&Accept{}).BinarySize()
			case "Commit":
				_, known = (&Commit{}).BinarySize()
			case "TryPreAccept":
				_, known = (&TryPreAccept{}).BinarySize()
			}
			if known {
				t.Errorf("%s.BinarySize() sizeKnown = true, want false", name)
			}
		})
	}
}

func TestStatusConstants(t *testing.T) {
	if NONE != 0 || PREACCEPTED != 1 || PREACCEPTED_EQ != 2 ||
		ACCEPTED != 3 || COMMITTED != 4 || EXECUTED != 5 {
		t.Error("status constants have unexpected values")
	}
}
