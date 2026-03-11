package epaxosho

import (
	"bufio"
	"encoding/binary"
	"io"
	"sync"

	fastrpc "github.com/imdea-software/swiftpaxos/rpc"
	"github.com/imdea-software/swiftpaxos/state"
)

type byteReader interface {
	io.Reader
	ReadByte() (c byte, err error)
}

// ---- Helper: marshal/unmarshal a varint-prefixed []int32 slice ----

func marshalInt32Slice(wire io.Writer, s []int32) {
	var b [10]byte // max varint size
	n := binary.PutVarint(b[:], int64(len(s)))
	wire.Write(b[:n])
	var buf [4]byte
	for _, v := range s {
		buf[0] = byte(v)
		buf[1] = byte(v >> 8)
		buf[2] = byte(v >> 16)
		buf[3] = byte(v >> 24)
		wire.Write(buf[:])
	}
}

func unmarshalInt32Slice(wire byteReader) ([]int32, error) {
	alen, err := binary.ReadVarint(wire)
	if err != nil {
		return nil, err
	}
	s := make([]int32, alen)
	var b [4]byte
	for i := int64(0); i < alen; i++ {
		if _, err := io.ReadAtLeast(wire, b[:], 4); err != nil {
			return nil, err
		}
		s[i] = int32(uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16 | uint32(b[3])<<24)
	}
	return s, nil
}

func marshalCommandSlice(wire io.Writer, cmds []state.Command) {
	var b [10]byte
	n := binary.PutVarint(b[:], int64(len(cmds)))
	wire.Write(b[:n])
	for i := range cmds {
		cmds[i].Marshal(wire)
	}
}

func unmarshalCommandSlice(wire byteReader) ([]state.Command, error) {
	alen, err := binary.ReadVarint(wire)
	if err != nil {
		return nil, err
	}
	cmds := make([]state.Command, alen)
	for i := int64(0); i < alen; i++ {
		if err := cmds[i].Unmarshal(wire); err != nil {
			return nil, err
		}
	}
	return cmds, nil
}

func ensureByteReader(r io.Reader) byteReader {
	if br, ok := r.(byteReader); ok {
		return br
	}
	return bufio.NewReader(r)
}

// putInt32 writes a little-endian int32 into bs[off:off+4].
func putInt32(bs []byte, off int, v int32) {
	bs[off] = byte(v)
	bs[off+1] = byte(v >> 8)
	bs[off+2] = byte(v >> 16)
	bs[off+3] = byte(v >> 24)
}

// getInt32 reads a little-endian int32 from bs[off:off+4].
func getInt32(bs []byte, off int) int32 {
	return int32(uint32(bs[off]) | uint32(bs[off+1])<<8 | uint32(bs[off+2])<<16 | uint32(bs[off+3])<<24)
}

// =============================================================================
// Prepare — 4×int32, fixed 16 bytes
// =============================================================================

func (t *Prepare) New() fastrpc.Serializable        { return new(Prepare) }
func (t *Prepare) BinarySize() (int, bool)           { return 16, true }
func (t *Prepare) Marshal(wire io.Writer) {
	var b [16]byte
	putInt32(b[:], 0, t.LeaderId)
	putInt32(b[:], 4, t.Replica)
	putInt32(b[:], 8, t.Instance)
	putInt32(b[:], 12, t.Ballot)
	wire.Write(b[:])
}
func (t *Prepare) Unmarshal(wire io.Reader) error {
	var b [16]byte
	if _, err := io.ReadAtLeast(wire, b[:], 16); err != nil {
		return err
	}
	t.LeaderId = getInt32(b[:], 0)
	t.Replica = getInt32(b[:], 4)
	t.Instance = getInt32(b[:], 8)
	t.Ballot = getInt32(b[:], 12)
	return nil
}

type PrepareCache struct {
	mu    sync.Mutex
	cache []*Prepare
}

func NewPrepareCache() *PrepareCache           { return &PrepareCache{cache: make([]*Prepare, 0)} }
func (p *PrepareCache) Get() *Prepare {
	var t *Prepare
	p.mu.Lock()
	if len(p.cache) > 0 {
		t = p.cache[len(p.cache)-1]
		p.cache = p.cache[:len(p.cache)-1]
	}
	p.mu.Unlock()
	if t == nil {
		t = &Prepare{}
	}
	return t
}
func (p *PrepareCache) Put(t *Prepare) {
	p.mu.Lock()
	p.cache = append(p.cache, t)
	p.mu.Unlock()
}

// =============================================================================
// PreAcceptOK — 1×int32, fixed 4 bytes
// =============================================================================

func (t *PreAcceptOK) New() fastrpc.Serializable    { return new(PreAcceptOK) }
func (t *PreAcceptOK) BinarySize() (int, bool)       { return 4, true }
func (t *PreAcceptOK) Marshal(wire io.Writer) {
	var b [4]byte
	putInt32(b[:], 0, t.Instance)
	wire.Write(b[:])
}
func (t *PreAcceptOK) Unmarshal(wire io.Reader) error {
	var b [4]byte
	if _, err := io.ReadAtLeast(wire, b[:], 4); err != nil {
		return err
	}
	t.Instance = getInt32(b[:], 0)
	return nil
}

type PreAcceptOKCache struct {
	mu    sync.Mutex
	cache []*PreAcceptOK
}

func NewPreAcceptOKCache() *PreAcceptOKCache   { return &PreAcceptOKCache{cache: make([]*PreAcceptOK, 0)} }
func (p *PreAcceptOKCache) Get() *PreAcceptOK {
	var t *PreAcceptOK
	p.mu.Lock()
	if len(p.cache) > 0 {
		t = p.cache[len(p.cache)-1]
		p.cache = p.cache[:len(p.cache)-1]
	}
	p.mu.Unlock()
	if t == nil {
		t = &PreAcceptOK{}
	}
	return t
}
func (p *PreAcceptOKCache) Put(t *PreAcceptOK) {
	p.mu.Lock()
	p.cache = append(p.cache, t)
	p.mu.Unlock()
}

// =============================================================================
// AcceptReply — 3×int32 + 1×uint8, fixed 13 bytes
// =============================================================================

func (t *AcceptReply) New() fastrpc.Serializable    { return new(AcceptReply) }
func (t *AcceptReply) BinarySize() (int, bool)       { return 13, true }
func (t *AcceptReply) Marshal(wire io.Writer) {
	var b [13]byte
	putInt32(b[:], 0, t.Replica)
	putInt32(b[:], 4, t.Instance)
	b[8] = byte(t.OK)
	putInt32(b[:], 9, t.Ballot)
	wire.Write(b[:])
}
func (t *AcceptReply) Unmarshal(wire io.Reader) error {
	var b [13]byte
	if _, err := io.ReadAtLeast(wire, b[:], 13); err != nil {
		return err
	}
	t.Replica = getInt32(b[:], 0)
	t.Instance = getInt32(b[:], 4)
	t.OK = uint8(b[8])
	t.Ballot = getInt32(b[:], 9)
	return nil
}

type AcceptReplyCache struct {
	mu    sync.Mutex
	cache []*AcceptReply
}

func NewAcceptReplyCache() *AcceptReplyCache   { return &AcceptReplyCache{cache: make([]*AcceptReply, 0)} }
func (p *AcceptReplyCache) Get() *AcceptReply {
	var t *AcceptReply
	p.mu.Lock()
	if len(p.cache) > 0 {
		t = p.cache[len(p.cache)-1]
		p.cache = p.cache[:len(p.cache)-1]
	}
	p.mu.Unlock()
	if t == nil {
		t = &AcceptReply{}
	}
	return t
}
func (p *AcceptReplyCache) Put(t *AcceptReply) {
	p.mu.Lock()
	p.cache = append(p.cache, t)
	p.mu.Unlock()
}

// =============================================================================
// TryPreAcceptReply — 5×int32 + 1×uint8 + 1×int8, fixed 26 bytes
// =============================================================================

func (t *TryPreAcceptReply) New() fastrpc.Serializable { return new(TryPreAcceptReply) }
func (t *TryPreAcceptReply) BinarySize() (int, bool)    { return 26, true }
func (t *TryPreAcceptReply) Marshal(wire io.Writer) {
	var b [26]byte
	putInt32(b[:], 0, t.AcceptorId)
	putInt32(b[:], 4, t.Replica)
	putInt32(b[:], 8, t.Instance)
	b[12] = byte(t.OK)
	putInt32(b[:], 13, t.Ballot)
	putInt32(b[:], 17, t.ConflictReplica)
	putInt32(b[:], 21, t.ConflictInstance)
	b[25] = byte(t.ConflictStatus)
	wire.Write(b[:])
}
func (t *TryPreAcceptReply) Unmarshal(wire io.Reader) error {
	var b [26]byte
	if _, err := io.ReadAtLeast(wire, b[:], 26); err != nil {
		return err
	}
	t.AcceptorId = getInt32(b[:], 0)
	t.Replica = getInt32(b[:], 4)
	t.Instance = getInt32(b[:], 8)
	t.OK = uint8(b[12])
	t.Ballot = getInt32(b[:], 13)
	t.ConflictReplica = getInt32(b[:], 17)
	t.ConflictInstance = getInt32(b[:], 21)
	t.ConflictStatus = int8(b[25])
	return nil
}

type TryPreAcceptReplyCache struct {
	mu    sync.Mutex
	cache []*TryPreAcceptReply
}

func NewTryPreAcceptReplyCache() *TryPreAcceptReplyCache {
	return &TryPreAcceptReplyCache{cache: make([]*TryPreAcceptReply, 0)}
}
func (p *TryPreAcceptReplyCache) Get() *TryPreAcceptReply {
	var t *TryPreAcceptReply
	p.mu.Lock()
	if len(p.cache) > 0 {
		t = p.cache[len(p.cache)-1]
		p.cache = p.cache[:len(p.cache)-1]
	}
	p.mu.Unlock()
	if t == nil {
		t = &TryPreAcceptReply{}
	}
	return t
}
func (p *TryPreAcceptReplyCache) Put(t *TryPreAcceptReply) {
	p.mu.Lock()
	p.cache = append(p.cache, t)
	p.mu.Unlock()
}

// =============================================================================
// PreAcceptReply — 4×int32 + 1×uint8 (17B header) + 3 []int32 slices
// =============================================================================

func (t *PreAcceptReply) New() fastrpc.Serializable { return new(PreAcceptReply) }
func (t *PreAcceptReply) BinarySize() (int, bool)    { return 0, false }
func (t *PreAcceptReply) Marshal(wire io.Writer) {
	var b [17]byte
	putInt32(b[:], 0, t.Replica)
	putInt32(b[:], 4, t.Instance)
	b[8] = byte(t.OK)
	putInt32(b[:], 9, t.Ballot)
	putInt32(b[:], 13, t.Seq)
	wire.Write(b[:])
	marshalInt32Slice(wire, t.Deps)
	marshalInt32Slice(wire, t.CL)
	marshalInt32Slice(wire, t.CommittedDeps)
}
func (t *PreAcceptReply) Unmarshal(rr io.Reader) error {
	wire := ensureByteReader(rr)
	var b [17]byte
	if _, err := io.ReadAtLeast(wire, b[:], 17); err != nil {
		return err
	}
	t.Replica = getInt32(b[:], 0)
	t.Instance = getInt32(b[:], 4)
	t.OK = uint8(b[8])
	t.Ballot = getInt32(b[:], 9)
	t.Seq = getInt32(b[:], 13)
	var err error
	if t.Deps, err = unmarshalInt32Slice(wire); err != nil {
		return err
	}
	if t.CL, err = unmarshalInt32Slice(wire); err != nil {
		return err
	}
	if t.CommittedDeps, err = unmarshalInt32Slice(wire); err != nil {
		return err
	}
	return nil
}

type PreAcceptReplyCache struct {
	mu    sync.Mutex
	cache []*PreAcceptReply
}

func NewPreAcceptReplyCache() *PreAcceptReplyCache {
	return &PreAcceptReplyCache{cache: make([]*PreAcceptReply, 0)}
}
func (p *PreAcceptReplyCache) Get() *PreAcceptReply {
	var t *PreAcceptReply
	p.mu.Lock()
	if len(p.cache) > 0 {
		t = p.cache[len(p.cache)-1]
		p.cache = p.cache[:len(p.cache)-1]
	}
	p.mu.Unlock()
	if t == nil {
		t = &PreAcceptReply{}
	}
	return t
}
func (p *PreAcceptReplyCache) Put(t *PreAcceptReply) {
	p.mu.Lock()
	p.cache = append(p.cache, t)
	p.mu.Unlock()
}

// =============================================================================
// Accept — 6×int32 (24B header) + 2 []int32 slices (Deps, CL)
// =============================================================================

func (t *Accept) New() fastrpc.Serializable { return new(Accept) }
func (t *Accept) BinarySize() (int, bool)    { return 0, false }
func (t *Accept) Marshal(wire io.Writer) {
	var b [24]byte
	putInt32(b[:], 0, t.LeaderId)
	putInt32(b[:], 4, t.Replica)
	putInt32(b[:], 8, t.Instance)
	putInt32(b[:], 12, t.Ballot)
	putInt32(b[:], 16, t.Count)
	putInt32(b[:], 20, t.Seq)
	wire.Write(b[:])
	marshalInt32Slice(wire, t.Deps)
	marshalInt32Slice(wire, t.CL)
}
func (t *Accept) Unmarshal(rr io.Reader) error {
	wire := ensureByteReader(rr)
	var b [24]byte
	if _, err := io.ReadAtLeast(wire, b[:], 24); err != nil {
		return err
	}
	t.LeaderId = getInt32(b[:], 0)
	t.Replica = getInt32(b[:], 4)
	t.Instance = getInt32(b[:], 8)
	t.Ballot = getInt32(b[:], 12)
	t.Count = getInt32(b[:], 16)
	t.Seq = getInt32(b[:], 20)
	var err error
	if t.Deps, err = unmarshalInt32Slice(wire); err != nil {
		return err
	}
	if t.CL, err = unmarshalInt32Slice(wire); err != nil {
		return err
	}
	return nil
}

type AcceptCache struct {
	mu    sync.Mutex
	cache []*Accept
}

func NewAcceptCache() *AcceptCache             { return &AcceptCache{cache: make([]*Accept, 0)} }
func (p *AcceptCache) Get() *Accept {
	var t *Accept
	p.mu.Lock()
	if len(p.cache) > 0 {
		t = p.cache[len(p.cache)-1]
		p.cache = p.cache[:len(p.cache)-1]
	}
	p.mu.Unlock()
	if t == nil {
		t = &Accept{}
	}
	return t
}
func (p *AcceptCache) Put(t *Accept) {
	p.mu.Lock()
	p.cache = append(p.cache, t)
	p.mu.Unlock()
}

// =============================================================================
// CommitShort — 1×Operation + 5×int32 (21B header) + 2 []int32 slices
// =============================================================================

func (t *CommitShort) New() fastrpc.Serializable { return new(CommitShort) }
func (t *CommitShort) BinarySize() (int, bool)    { return 0, false }
func (t *CommitShort) Marshal(wire io.Writer) {
	var b [21]byte
	b[0] = byte(t.Consistency)
	putInt32(b[:], 1, t.LeaderId)
	putInt32(b[:], 5, t.Replica)
	putInt32(b[:], 9, t.Instance)
	putInt32(b[:], 13, t.Count)
	putInt32(b[:], 17, t.Seq)
	wire.Write(b[:])
	marshalInt32Slice(wire, t.Deps)
	marshalInt32Slice(wire, t.CL)
}
func (t *CommitShort) Unmarshal(rr io.Reader) error {
	wire := ensureByteReader(rr)
	var b [21]byte
	if _, err := io.ReadAtLeast(wire, b[:], 21); err != nil {
		return err
	}
	t.Consistency = state.Operation(b[0])
	t.LeaderId = getInt32(b[:], 1)
	t.Replica = getInt32(b[:], 5)
	t.Instance = getInt32(b[:], 9)
	t.Count = getInt32(b[:], 13)
	t.Seq = getInt32(b[:], 17)
	var err error
	if t.Deps, err = unmarshalInt32Slice(wire); err != nil {
		return err
	}
	if t.CL, err = unmarshalInt32Slice(wire); err != nil {
		return err
	}
	return nil
}

type CommitShortCache struct {
	mu    sync.Mutex
	cache []*CommitShort
}

func NewCommitShortCache() *CommitShortCache   { return &CommitShortCache{cache: make([]*CommitShort, 0)} }
func (p *CommitShortCache) Get() *CommitShort {
	var t *CommitShort
	p.mu.Lock()
	if len(p.cache) > 0 {
		t = p.cache[len(p.cache)-1]
		p.cache = p.cache[:len(p.cache)-1]
	}
	p.mu.Unlock()
	if t == nil {
		t = &CommitShort{}
	}
	return t
}
func (p *CommitShortCache) Put(t *CommitShort) {
	p.mu.Lock()
	p.cache = append(p.cache, t)
	p.mu.Unlock()
}

// =============================================================================
// PrepareReply — 3×int32 + 1×uint8 + 2×int32 + 1×int8 (22B header)
//                + []Command + 1×int32(Seq) + 2 []int32 slices (Deps, CL)
// =============================================================================

func (t *PrepareReply) New() fastrpc.Serializable { return new(PrepareReply) }
func (t *PrepareReply) BinarySize() (int, bool)    { return 0, false }
func (t *PrepareReply) Marshal(wire io.Writer) {
	var b [22]byte
	putInt32(b[:], 0, t.AcceptorId)
	putInt32(b[:], 4, t.Replica)
	putInt32(b[:], 8, t.Instance)
	b[12] = byte(t.OK)
	putInt32(b[:], 13, t.Bal)
	putInt32(b[:], 17, t.VBal)
	b[21] = byte(t.Status)
	wire.Write(b[:])
	marshalCommandSlice(wire, t.Command)
	var sb [4]byte
	putInt32(sb[:], 0, t.Seq)
	wire.Write(sb[:])
	marshalInt32Slice(wire, t.Deps)
	marshalInt32Slice(wire, t.CL)
}
func (t *PrepareReply) Unmarshal(rr io.Reader) error {
	wire := ensureByteReader(rr)
	var b [22]byte
	if _, err := io.ReadAtLeast(wire, b[:], 22); err != nil {
		return err
	}
	t.AcceptorId = getInt32(b[:], 0)
	t.Replica = getInt32(b[:], 4)
	t.Instance = getInt32(b[:], 8)
	t.OK = uint8(b[12])
	t.Bal = getInt32(b[:], 13)
	t.VBal = getInt32(b[:], 17)
	t.Status = int8(b[21])
	var err error
	if t.Command, err = unmarshalCommandSlice(wire); err != nil {
		return err
	}
	var sb [4]byte
	if _, err := io.ReadAtLeast(wire, sb[:], 4); err != nil {
		return err
	}
	t.Seq = getInt32(sb[:], 0)
	if t.Deps, err = unmarshalInt32Slice(wire); err != nil {
		return err
	}
	if t.CL, err = unmarshalInt32Slice(wire); err != nil {
		return err
	}
	return nil
}

type PrepareReplyCache struct {
	mu    sync.Mutex
	cache []*PrepareReply
}

func NewPrepareReplyCache() *PrepareReplyCache {
	return &PrepareReplyCache{cache: make([]*PrepareReply, 0)}
}
func (p *PrepareReplyCache) Get() *PrepareReply {
	var t *PrepareReply
	p.mu.Lock()
	if len(p.cache) > 0 {
		t = p.cache[len(p.cache)-1]
		p.cache = p.cache[:len(p.cache)-1]
	}
	p.mu.Unlock()
	if t == nil {
		t = &PrepareReply{}
	}
	return t
}
func (p *PrepareReplyCache) Put(t *PrepareReply) {
	p.mu.Lock()
	p.cache = append(p.cache, t)
	p.mu.Unlock()
}

// =============================================================================
// PreAccept — 4×int32 (16B header) + []Command + 1×int32(Seq) + 2 []int32 (Deps, CL)
// =============================================================================

func (t *PreAccept) New() fastrpc.Serializable { return new(PreAccept) }
func (t *PreAccept) BinarySize() (int, bool)    { return 0, false }
func (t *PreAccept) Marshal(wire io.Writer) {
	var b [16]byte
	putInt32(b[:], 0, t.LeaderId)
	putInt32(b[:], 4, t.Replica)
	putInt32(b[:], 8, t.Instance)
	putInt32(b[:], 12, t.Ballot)
	wire.Write(b[:])
	marshalCommandSlice(wire, t.Command)
	var sb [4]byte
	putInt32(sb[:], 0, t.Seq)
	wire.Write(sb[:])
	marshalInt32Slice(wire, t.Deps)
	marshalInt32Slice(wire, t.CL)
}
func (t *PreAccept) Unmarshal(rr io.Reader) error {
	wire := ensureByteReader(rr)
	var b [16]byte
	if _, err := io.ReadAtLeast(wire, b[:], 16); err != nil {
		return err
	}
	t.LeaderId = getInt32(b[:], 0)
	t.Replica = getInt32(b[:], 4)
	t.Instance = getInt32(b[:], 8)
	t.Ballot = getInt32(b[:], 12)
	var err error
	if t.Command, err = unmarshalCommandSlice(wire); err != nil {
		return err
	}
	var sb [4]byte
	if _, err := io.ReadAtLeast(wire, sb[:], 4); err != nil {
		return err
	}
	t.Seq = getInt32(sb[:], 0)
	if t.Deps, err = unmarshalInt32Slice(wire); err != nil {
		return err
	}
	if t.CL, err = unmarshalInt32Slice(wire); err != nil {
		return err
	}
	return nil
}

type PreAcceptCache struct {
	mu    sync.Mutex
	cache []*PreAccept
}

func NewPreAcceptCache() *PreAcceptCache       { return &PreAcceptCache{cache: make([]*PreAccept, 0)} }
func (p *PreAcceptCache) Get() *PreAccept {
	var t *PreAccept
	p.mu.Lock()
	if len(p.cache) > 0 {
		t = p.cache[len(p.cache)-1]
		p.cache = p.cache[:len(p.cache)-1]
	}
	p.mu.Unlock()
	if t == nil {
		t = &PreAccept{}
	}
	return t
}
func (p *PreAcceptCache) Put(t *PreAccept) {
	p.mu.Lock()
	p.cache = append(p.cache, t)
	p.mu.Unlock()
}

// =============================================================================
// Commit — 1×Operation + 3×int32 (13B header) + []Command + 1×int32(Seq) + 2 []int32
// =============================================================================

func (t *Commit) New() fastrpc.Serializable { return new(Commit) }
func (t *Commit) BinarySize() (int, bool)    { return 0, false }
func (t *Commit) Marshal(wire io.Writer) {
	var b [13]byte
	b[0] = byte(t.Consistency)
	putInt32(b[:], 1, t.LeaderId)
	putInt32(b[:], 5, t.Replica)
	putInt32(b[:], 9, t.Instance)
	wire.Write(b[:])
	marshalCommandSlice(wire, t.Command)
	var sb [4]byte
	putInt32(sb[:], 0, t.Seq)
	wire.Write(sb[:])
	marshalInt32Slice(wire, t.Deps)
	marshalInt32Slice(wire, t.CL)
}
func (t *Commit) Unmarshal(rr io.Reader) error {
	wire := ensureByteReader(rr)
	var b [13]byte
	if _, err := io.ReadAtLeast(wire, b[:], 13); err != nil {
		return err
	}
	t.Consistency = state.Operation(b[0])
	t.LeaderId = getInt32(b[:], 1)
	t.Replica = getInt32(b[:], 5)
	t.Instance = getInt32(b[:], 9)
	var err error
	if t.Command, err = unmarshalCommandSlice(wire); err != nil {
		return err
	}
	var sb [4]byte
	if _, err := io.ReadAtLeast(wire, sb[:], 4); err != nil {
		return err
	}
	t.Seq = getInt32(sb[:], 0)
	if t.Deps, err = unmarshalInt32Slice(wire); err != nil {
		return err
	}
	if t.CL, err = unmarshalInt32Slice(wire); err != nil {
		return err
	}
	return nil
}

type CommitCache struct {
	mu    sync.Mutex
	cache []*Commit
}

func NewCommitCache() *CommitCache             { return &CommitCache{cache: make([]*Commit, 0)} }
func (p *CommitCache) Get() *Commit {
	var t *Commit
	p.mu.Lock()
	if len(p.cache) > 0 {
		t = p.cache[len(p.cache)-1]
		p.cache = p.cache[:len(p.cache)-1]
	}
	p.mu.Unlock()
	if t == nil {
		t = &Commit{}
	}
	return t
}
func (p *CommitCache) Put(t *Commit) {
	p.mu.Lock()
	p.cache = append(p.cache, t)
	p.mu.Unlock()
}

// =============================================================================
// CausalCommit — identical wire format to Commit
// =============================================================================

func (t *CausalCommit) New() fastrpc.Serializable { return new(CausalCommit) }
func (t *CausalCommit) BinarySize() (int, bool)    { return 0, false }
func (t *CausalCommit) Marshal(wire io.Writer) {
	var b [13]byte
	b[0] = byte(t.Consistency)
	putInt32(b[:], 1, t.LeaderId)
	putInt32(b[:], 5, t.Replica)
	putInt32(b[:], 9, t.Instance)
	wire.Write(b[:])
	marshalCommandSlice(wire, t.Command)
	var sb [4]byte
	putInt32(sb[:], 0, t.Seq)
	wire.Write(sb[:])
	marshalInt32Slice(wire, t.Deps)
	marshalInt32Slice(wire, t.CL)
}
func (t *CausalCommit) Unmarshal(rr io.Reader) error {
	wire := ensureByteReader(rr)
	var b [13]byte
	if _, err := io.ReadAtLeast(wire, b[:], 13); err != nil {
		return err
	}
	t.Consistency = state.Operation(b[0])
	t.LeaderId = getInt32(b[:], 1)
	t.Replica = getInt32(b[:], 5)
	t.Instance = getInt32(b[:], 9)
	var err error
	if t.Command, err = unmarshalCommandSlice(wire); err != nil {
		return err
	}
	var sb [4]byte
	if _, err := io.ReadAtLeast(wire, sb[:], 4); err != nil {
		return err
	}
	t.Seq = getInt32(sb[:], 0)
	if t.Deps, err = unmarshalInt32Slice(wire); err != nil {
		return err
	}
	if t.CL, err = unmarshalInt32Slice(wire); err != nil {
		return err
	}
	return nil
}

type CausalCommitCache struct {
	mu    sync.Mutex
	cache []*CausalCommit
}

func NewCausalCommitCache() *CausalCommitCache {
	return &CausalCommitCache{cache: make([]*CausalCommit, 0)}
}
func (p *CausalCommitCache) Get() *CausalCommit {
	var t *CausalCommit
	p.mu.Lock()
	if len(p.cache) > 0 {
		t = p.cache[len(p.cache)-1]
		p.cache = p.cache[:len(p.cache)-1]
	}
	p.mu.Unlock()
	if t == nil {
		t = &CausalCommit{}
	}
	return t
}
func (p *CausalCommitCache) Put(t *CausalCommit) {
	p.mu.Lock()
	p.cache = append(p.cache, t)
	p.mu.Unlock()
}

// =============================================================================
// TryPreAccept — 4×int32 (16B header) + []Command + 1×int32(Seq) + 2 []int32 (CL, Deps)
// =============================================================================

func (t *TryPreAccept) New() fastrpc.Serializable { return new(TryPreAccept) }
func (t *TryPreAccept) BinarySize() (int, bool)    { return 0, false }
func (t *TryPreAccept) Marshal(wire io.Writer) {
	var b [16]byte
	putInt32(b[:], 0, t.LeaderId)
	putInt32(b[:], 4, t.Replica)
	putInt32(b[:], 8, t.Instance)
	putInt32(b[:], 12, t.Ballot)
	wire.Write(b[:])
	marshalCommandSlice(wire, t.Command)
	var sb [4]byte
	putInt32(sb[:], 0, t.Seq)
	wire.Write(sb[:])
	marshalInt32Slice(wire, t.CL)
	marshalInt32Slice(wire, t.Deps)
}
func (t *TryPreAccept) Unmarshal(rr io.Reader) error {
	wire := ensureByteReader(rr)
	var b [16]byte
	if _, err := io.ReadAtLeast(wire, b[:], 16); err != nil {
		return err
	}
	t.LeaderId = getInt32(b[:], 0)
	t.Replica = getInt32(b[:], 4)
	t.Instance = getInt32(b[:], 8)
	t.Ballot = getInt32(b[:], 12)
	var err error
	if t.Command, err = unmarshalCommandSlice(wire); err != nil {
		return err
	}
	var sb [4]byte
	if _, err := io.ReadAtLeast(wire, sb[:], 4); err != nil {
		return err
	}
	t.Seq = getInt32(sb[:], 0)
	if t.CL, err = unmarshalInt32Slice(wire); err != nil {
		return err
	}
	if t.Deps, err = unmarshalInt32Slice(wire); err != nil {
		return err
	}
	return nil
}

type TryPreAcceptCache struct {
	mu    sync.Mutex
	cache []*TryPreAccept
}

func NewTryPreAcceptCache() *TryPreAcceptCache {
	return &TryPreAcceptCache{cache: make([]*TryPreAccept, 0)}
}
func (p *TryPreAcceptCache) Get() *TryPreAccept {
	var t *TryPreAccept
	p.mu.Lock()
	if len(p.cache) > 0 {
		t = p.cache[len(p.cache)-1]
		p.cache = p.cache[:len(p.cache)-1]
	}
	p.mu.Unlock()
	if t == nil {
		t = &TryPreAccept{}
	}
	return t
}
func (p *TryPreAcceptCache) Put(t *TryPreAccept) {
	p.mu.Lock()
	p.cache = append(p.cache, t)
	p.mu.Unlock()
}
