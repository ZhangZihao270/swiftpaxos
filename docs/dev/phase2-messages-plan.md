# Phase 2 Plan: Message Protocol Modifications

## Overview
Add new message types and constants to support weak consistency commands in curp-ht.

## Task 2.1: Define Consistency Level Constants

### Location
curp-ht/defs.go

### Changes
Add constants after existing phase constants:

```go
// Consistency Level
const (
    STRONG uint8 = 0  // Linearizable
    WEAK   uint8 = 1  // Causal
)
```

### Design Rationale
- STRONG=0 maintains backward compatibility (default value)
- Using uint8 is efficient for serialization
- Constants are named clearly to indicate consistency semantics

---

## Task 2.2: Add MWeakPropose Message Type

### Structure
```go
type MWeakPropose struct {
    CommandId int32
    ClientId  int32
    Command   state.Command
    Timestamp int64
}
```

### Required Methods
1. `New() fastrpc.Serializable` - Create new instance
2. `BinarySize() (int, bool)` - Return serialized size
3. `Marshal(io.Writer)` - Serialize to binary
4. `Unmarshal(io.Reader) error` - Deserialize from binary

### Cache Structure (for object pooling)
```go
type MWeakProposeCache struct {
    mu    sync.Mutex
    cache []*MWeakPropose
}
```

---

## Task 2.3: Add MWeakReply Message Type

### Structure
```go
type MWeakReply struct {
    Replica int32
    Ballot  int32
    CmdId   CommandId
    Rep     []byte
}
```

### Required Methods
Same as MWeakPropose (New, BinarySize, Marshal, Unmarshal)

### Cache Structure
```go
type MWeakReplyCache struct {
    mu    sync.Mutex
    cache []*MWeakReply
}
```

---

## Task 2.4: Add Communication Channels

### Changes to CommunicationSupply struct
```go
type CommunicationSupply struct {
    // ... existing fields ...

    // Weak command channels
    weakProposeChan chan fastrpc.Serializable
    weakReplyChan   chan fastrpc.Serializable

    weakProposeRPC uint8
    weakReplyRPC   uint8
}
```

### Changes to initCs function
```go
func initCs(cs *CommunicationSupply, t *fastrpc.Table) {
    // ... existing code ...

    cs.weakProposeChan = make(chan fastrpc.Serializable, defs.CHAN_BUFFER_SIZE)
    cs.weakReplyChan = make(chan fastrpc.Serializable, defs.CHAN_BUFFER_SIZE)

    cs.weakProposeRPC = t.Register(new(MWeakPropose), cs.weakProposeChan)
    cs.weakReplyRPC = t.Register(new(MWeakReply), cs.weakReplyChan)
}
```

---

## Estimated LOC
- Task 2.1: ~5 lines
- Task 2.2: ~100 lines (message + cache + serialization)
- Task 2.3: ~100 lines (message + cache + serialization)
- Task 2.4: ~10 lines

Total: ~215 lines (within 500 LOC limit)
