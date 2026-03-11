package epaxosho

import (
	"bytes"
	"io"
	"testing"

	"github.com/imdea-software/swiftpaxos/state"
)

// roundTrip marshals t into a buffer, then unmarshals into dst, returning any error.
func roundTrip(t interface {
	Marshal(w io.Writer)
}, dst interface {
	Unmarshal(r io.Reader) error
}, buf *bytes.Buffer,
) error {
	buf.Reset()
	t.Marshal(buf)
	return dst.Unmarshal(buf)
}

// --- Simple fixed-size messages ---

func TestPrepareRoundTrip(t *testing.T) {
	orig := Prepare{LeaderId: 1, Replica: 2, Instance: 3, Ballot: 42}
	var got Prepare
	var buf bytes.Buffer
	if err := roundTrip(&orig, &got, &buf); err != nil {
		t.Fatal(err)
	}
	if got != orig {
		t.Fatalf("got %+v, want %+v", got, orig)
	}
}

func TestPreAcceptOKRoundTrip(t *testing.T) {
	orig := PreAcceptOK{Instance: 999}
	var got PreAcceptOK
	var buf bytes.Buffer
	if err := roundTrip(&orig, &got, &buf); err != nil {
		t.Fatal(err)
	}
	if got != orig {
		t.Fatalf("got %+v, want %+v", got, orig)
	}
}

func TestAcceptReplyRoundTrip(t *testing.T) {
	orig := AcceptReply{Replica: 1, Instance: 2, OK: 1, Ballot: 10}
	var got AcceptReply
	var buf bytes.Buffer
	if err := roundTrip(&orig, &got, &buf); err != nil {
		t.Fatal(err)
	}
	if got != orig {
		t.Fatalf("got %+v, want %+v", got, orig)
	}
}

func TestTryPreAcceptReplyRoundTrip(t *testing.T) {
	orig := TryPreAcceptReply{
		AcceptorId: 3, Replica: 1, Instance: 5, OK: 0,
		Ballot: 7, ConflictReplica: 2, ConflictInstance: 4, ConflictStatus: ACCEPTED,
	}
	var got TryPreAcceptReply
	var buf bytes.Buffer
	if err := roundTrip(&orig, &got, &buf); err != nil {
		t.Fatal(err)
	}
	if got != orig {
		t.Fatalf("got %+v, want %+v", got, orig)
	}
}

// --- BinarySize ---

func TestBinarySize(t *testing.T) {
	tests := []struct {
		name      string
		size      int
		known     bool
		getSize   func() (int, bool)
	}{
		{"Prepare", 16, true, (&Prepare{}).BinarySize},
		{"PreAcceptOK", 4, true, (&PreAcceptOK{}).BinarySize},
		{"AcceptReply", 13, true, (&AcceptReply{}).BinarySize},
		{"TryPreAcceptReply", 26, true, (&TryPreAcceptReply{}).BinarySize},
		{"PreAcceptReply", 0, false, (&PreAcceptReply{}).BinarySize},
		{"Accept", 0, false, (&Accept{}).BinarySize},
		{"CommitShort", 0, false, (&CommitShort{}).BinarySize},
		{"PrepareReply", 0, false, (&PrepareReply{}).BinarySize},
		{"PreAccept", 0, false, (&PreAccept{}).BinarySize},
		{"Commit", 0, false, (&Commit{}).BinarySize},
		{"CausalCommit", 0, false, (&CausalCommit{}).BinarySize},
		{"TryPreAccept", 0, false, (&TryPreAccept{}).BinarySize},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			size, known := tt.getSize()
			if size != tt.size || known != tt.known {
				t.Errorf("got (%d, %v), want (%d, %v)", size, known, tt.size, tt.known)
			}
		})
	}
}

// --- Messages with []int32 slices ---

func TestPreAcceptReplyRoundTrip(t *testing.T) {
	orig := PreAcceptReply{
		Replica: 1, Instance: 5, OK: 1, Ballot: 10, Seq: 3,
		Deps:          []int32{1, 2, 3},
		CL:            []int32{4, 5},
		CommittedDeps: []int32{0, 1, 2},
	}
	var got PreAcceptReply
	var buf bytes.Buffer
	if err := roundTrip(&orig, &got, &buf); err != nil {
		t.Fatal(err)
	}
	if got.Replica != orig.Replica || got.Instance != orig.Instance ||
		got.OK != orig.OK || got.Ballot != orig.Ballot || got.Seq != orig.Seq {
		t.Fatalf("header mismatch: got %+v, want %+v", got, orig)
	}
	assertInt32Slice(t, "Deps", got.Deps, orig.Deps)
	assertInt32Slice(t, "CL", got.CL, orig.CL)
	assertInt32Slice(t, "CommittedDeps", got.CommittedDeps, orig.CommittedDeps)
}

func TestAcceptRoundTrip(t *testing.T) {
	orig := Accept{
		LeaderId: 0, Replica: 2, Instance: 7, Ballot: 3, Count: 5, Seq: 10,
		Deps: []int32{1, 2, 3, 4, 5},
		CL:   []int32{4, 5, 4, 4, 5},
	}
	var got Accept
	var buf bytes.Buffer
	if err := roundTrip(&orig, &got, &buf); err != nil {
		t.Fatal(err)
	}
	if got.LeaderId != orig.LeaderId || got.Replica != orig.Replica ||
		got.Instance != orig.Instance || got.Ballot != orig.Ballot ||
		got.Count != orig.Count || got.Seq != orig.Seq {
		t.Fatalf("header mismatch: got %+v, want %+v", got, orig)
	}
	assertInt32Slice(t, "Deps", got.Deps, orig.Deps)
	assertInt32Slice(t, "CL", got.CL, orig.CL)
}

func TestCommitShortRoundTrip(t *testing.T) {
	orig := CommitShort{
		Consistency: state.STRONG, LeaderId: 1, Replica: 2,
		Instance: 3, Count: 5, Seq: 10,
		Deps: []int32{1, 2}, CL: []int32{5, 5},
	}
	var got CommitShort
	var buf bytes.Buffer
	if err := roundTrip(&orig, &got, &buf); err != nil {
		t.Fatal(err)
	}
	if got.Consistency != orig.Consistency || got.LeaderId != orig.LeaderId ||
		got.Count != orig.Count || got.Seq != orig.Seq {
		t.Fatalf("header mismatch: got %+v, want %+v", got, orig)
	}
	assertInt32Slice(t, "Deps", got.Deps, orig.Deps)
	assertInt32Slice(t, "CL", got.CL, orig.CL)
}

// --- Complex messages with []Command + []int32 ---

func makeCommands() []state.Command {
	return []state.Command{
		{Op: state.PUT, K: 42, V: state.Value([]byte{1, 2, 3}), CL: state.STRONG, Sid: 100},
		{Op: state.GET, K: 99, V: state.NIL(), CL: state.CAUSAL, Sid: 200},
	}
}

func TestPrepareReplyRoundTrip(t *testing.T) {
	orig := PrepareReply{
		AcceptorId: 1, Replica: 0, Instance: 5, OK: 1,
		Bal: 10, VBal: 8, Status: PREACCEPTED,
		Command: makeCommands(), Seq: 3,
		Deps: []int32{1, 2, 3}, CL: []int32{5, 4, 5},
	}
	var got PrepareReply
	var buf bytes.Buffer
	if err := roundTrip(&orig, &got, &buf); err != nil {
		t.Fatal(err)
	}
	if got.AcceptorId != orig.AcceptorId || got.OK != orig.OK ||
		got.Bal != orig.Bal || got.VBal != orig.VBal || got.Status != orig.Status ||
		got.Seq != orig.Seq {
		t.Fatalf("header mismatch: got %+v, want %+v", got, orig)
	}
	assertCommands(t, got.Command, orig.Command)
	assertInt32Slice(t, "Deps", got.Deps, orig.Deps)
	assertInt32Slice(t, "CL", got.CL, orig.CL)
}

func TestPreAcceptRoundTrip(t *testing.T) {
	orig := PreAccept{
		LeaderId: 0, Replica: 1, Instance: 3, Ballot: 5,
		Command: makeCommands(), Seq: 7,
		Deps: []int32{1, 2}, CL: []int32{5, 4},
	}
	var got PreAccept
	var buf bytes.Buffer
	if err := roundTrip(&orig, &got, &buf); err != nil {
		t.Fatal(err)
	}
	if got.LeaderId != orig.LeaderId || got.Ballot != orig.Ballot || got.Seq != orig.Seq {
		t.Fatalf("header mismatch: got %+v, want %+v", got, orig)
	}
	assertCommands(t, got.Command, orig.Command)
	assertInt32Slice(t, "Deps", got.Deps, orig.Deps)
	assertInt32Slice(t, "CL", got.CL, orig.CL)
}

func TestCommitRoundTrip(t *testing.T) {
	orig := Commit{
		Consistency: state.STRONG, LeaderId: 0, Replica: 1, Instance: 2,
		Command: makeCommands(), Seq: 5,
		Deps: []int32{1, 0}, CL: []int32{5, 4},
	}
	var got Commit
	var buf bytes.Buffer
	if err := roundTrip(&orig, &got, &buf); err != nil {
		t.Fatal(err)
	}
	if got.Consistency != orig.Consistency || got.LeaderId != orig.LeaderId || got.Seq != orig.Seq {
		t.Fatalf("header mismatch: got %+v, want %+v", got, orig)
	}
	assertCommands(t, got.Command, orig.Command)
	assertInt32Slice(t, "Deps", got.Deps, orig.Deps)
	assertInt32Slice(t, "CL", got.CL, orig.CL)
}

func TestCausalCommitRoundTrip(t *testing.T) {
	orig := CausalCommit{
		Consistency: state.CAUSAL, LeaderId: 2, Replica: 3, Instance: 10,
		Command: makeCommands(), Seq: 1,
		Deps: []int32{0}, CL: []int32{4},
	}
	var got CausalCommit
	var buf bytes.Buffer
	if err := roundTrip(&orig, &got, &buf); err != nil {
		t.Fatal(err)
	}
	if got.Consistency != orig.Consistency || got.LeaderId != orig.LeaderId {
		t.Fatalf("header mismatch: got %+v, want %+v", got, orig)
	}
	assertCommands(t, got.Command, orig.Command)
	assertInt32Slice(t, "Deps", got.Deps, orig.Deps)
}

func TestTryPreAcceptRoundTrip(t *testing.T) {
	orig := TryPreAccept{
		LeaderId: 0, Replica: 2, Instance: 5, Ballot: 8,
		Command: makeCommands(), Seq: 3,
		CL:   []int32{5, 4},
		Deps: []int32{1, 2},
	}
	var got TryPreAccept
	var buf bytes.Buffer
	if err := roundTrip(&orig, &got, &buf); err != nil {
		t.Fatal(err)
	}
	if got.LeaderId != orig.LeaderId || got.Ballot != orig.Ballot || got.Seq != orig.Seq {
		t.Fatalf("header mismatch: got %+v, want %+v", got, orig)
	}
	assertCommands(t, got.Command, orig.Command)
	assertInt32Slice(t, "CL", got.CL, orig.CL)
	assertInt32Slice(t, "Deps", got.Deps, orig.Deps)
}

// --- Empty slices ---

func TestEmptySlicesRoundTrip(t *testing.T) {
	orig := PreAccept{
		LeaderId: 0, Replica: 0, Instance: 1, Ballot: 0,
		Command: []state.Command{}, Seq: 0,
		Deps: []int32{}, CL: []int32{},
	}
	var got PreAccept
	var buf bytes.Buffer
	if err := roundTrip(&orig, &got, &buf); err != nil {
		t.Fatal(err)
	}
	if len(got.Command) != 0 || len(got.Deps) != 0 || len(got.CL) != 0 {
		t.Fatalf("expected empty slices, got Command=%d Deps=%d CL=%d",
			len(got.Command), len(got.Deps), len(got.CL))
	}
}

// --- Cache tests ---

func TestCacheGetPut(t *testing.T) {
	c := NewPrepareCache()
	p := &Prepare{LeaderId: 42}
	c.Put(p)
	got := c.Get()
	if got.LeaderId != 42 {
		t.Fatalf("cache returned wrong value: %+v", got)
	}
	// Get from empty cache returns new zero value
	got2 := c.Get()
	if got2.LeaderId != 0 {
		t.Fatalf("expected zero value from empty cache, got %+v", got2)
	}
}

// --- New() tests ---

func TestNewReturnsCorrectType(t *testing.T) {
	tests := []struct {
		name string
		fn   func() interface{}
	}{
		{"Prepare", func() interface{} { return (&Prepare{}).New() }},
		{"PrepareReply", func() interface{} { return (&PrepareReply{}).New() }},
		{"PreAccept", func() interface{} { return (&PreAccept{}).New() }},
		{"PreAcceptReply", func() interface{} { return (&PreAcceptReply{}).New() }},
		{"PreAcceptOK", func() interface{} { return (&PreAcceptOK{}).New() }},
		{"Accept", func() interface{} { return (&Accept{}).New() }},
		{"AcceptReply", func() interface{} { return (&AcceptReply{}).New() }},
		{"Commit", func() interface{} { return (&Commit{}).New() }},
		{"CausalCommit", func() interface{} { return (&CausalCommit{}).New() }},
		{"CommitShort", func() interface{} { return (&CommitShort{}).New() }},
		{"TryPreAccept", func() interface{} { return (&TryPreAccept{}).New() }},
		{"TryPreAcceptReply", func() interface{} { return (&TryPreAcceptReply{}).New() }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := tt.fn()
			if v == nil {
				t.Fatal("New() returned nil")
			}
		})
	}
}

// --- Status constants ---

func TestStatusConstants(t *testing.T) {
	if NONE != 0 {
		t.Errorf("NONE=%d, want 0", NONE)
	}
	if PREACCEPTED != 1 {
		t.Errorf("PREACCEPTED=%d, want 1", PREACCEPTED)
	}
	if CAUSALLY_COMMITTED != 5 {
		t.Errorf("CAUSALLY_COMMITTED=%d, want 5", CAUSALLY_COMMITTED)
	}
	if STRONGLY_COMMITTED != 6 {
		t.Errorf("STRONGLY_COMMITTED=%d, want 6", STRONGLY_COMMITTED)
	}
	if EXECUTED != 7 {
		t.Errorf("EXECUTED=%d, want 7", EXECUTED)
	}
}

// --- helpers ---

func assertInt32Slice(t *testing.T, name string, got, want []int32) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: len got %d, want %d", name, len(got), len(want))
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("%s[%d]: got %d, want %d", name, i, got[i], want[i])
		}
	}
}

func assertCommands(t *testing.T, got, want []state.Command) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("Command len: got %d, want %d", len(got), len(want))
	}
	for i := range got {
		if got[i].Op != want[i].Op || got[i].K != want[i].K ||
			got[i].CL != want[i].CL || got[i].Sid != want[i].Sid {
			t.Fatalf("Command[%d]: got %+v, want %+v", i, got[i], want[i])
		}
		if !bytes.Equal(got[i].V, want[i].V) {
			t.Fatalf("Command[%d].V: got %v, want %v", i, got[i].V, want[i].V)
		}
	}
}
