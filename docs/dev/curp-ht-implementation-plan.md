# CURP-HT (Hybrid Transparency) Implementation Plan

## Overview

Implement Hybrid Consistency on top of CURP, supporting both Strong (Linearizable) and Weak (Causal) consistency levels while satisfying the Transparency property.

### Core Design

| Consistency Level | Target | Completion Condition | RTT |
|-------------------|--------|---------------------|-----|
| Strong | All Replicas | 3/4 no-conflict replies (including Leader) or Leader final confirmation | 1-2 RTT |
| Weak | Leader Only | Leader replies with speculative result | 1 RTT |

---

## Phase 1: Project Structure Setup

### 1.1 Copy Base Files

```bash
mkdir -p curp-ht
cp curp/curp.go curp-ht/curp-ht.go
cp curp/client.go curp-ht/client.go
cp curp/defs.go curp-ht/defs.go
cp curp/batcher.go curp-ht/batcher.go
cp curp/timer.go curp-ht/timer.go
```

### 1.2 Modify Package Names

- Change `package curp` to `package curpht` in all files
- Update all import paths

---

## Phase 2: Message Protocol Modifications

### 2.1 Define Consistency Level Constants (defs.go)

```go
// Consistency Level
const (
    STRONG uint8 = 0  // Linearizable
    WEAK   uint8 = 1  // Causal
)
```

### 2.2 Modify Propose Message Structure

**Option A (Recommended): Add new message types in curp-ht/defs.go**

```go
// New MWeakPropose message specifically for weak commands
// Advantages: Avoids modifying shared replica/defs, reduces impact on other protocols
type MWeakPropose struct {
    CommandId int32
    ClientId  int32
    Command   state.Command
    Timestamp int64
}
```

**Option B: Modify Propose in replica/defs/defs.go**

```go
type Propose struct {
    CommandId   int32
    ClientId    int32
    Command     state.Command
    Timestamp   int64
    Consistency uint8  // New field: STRONG=0, WEAK=1
}
```

**Recommendation: Choose Option A** because:
1. Avoids affecting other protocol implementations
2. Weak commands can have a more streamlined message structure
3. Facilitates future performance optimization (weak path can be optimized independently)

### 2.3 Add Weak-Related Message Types (defs.go)

```go
// Weak command Propose (sent only to Leader)
type MWeakPropose struct {
    CommandId int32
    ClientId  int32
    Command   state.Command
    Timestamp int64
}

// Weak command Reply (Leader replies immediately)
type MWeakReply struct {
    Replica   int32
    Ballot    int32
    CmdId     CommandId
    Rep       []byte
}
```

### 2.4 Generate Serialization Code

Implement for new message types:
- `BinarySize() (int, bool)`
- `Marshal(io.Writer)`
- `Unmarshal(io.Reader) error`
- `New() fastrpc.Serializable`
- Cache structures (optional, for object pool optimization)

---

## Phase 3: Client-Side Modifications

### 3.1 Add Client Structure Fields (client.go)

```go
type Client struct {
    *client.BufferClient

    // ... existing fields ...

    // Weak command tracking
    weakPending map[int32]struct{}  // Track pending weak commands
}
```

### 3.2 Add Send Methods

```go
// SendWeakWrite - Send weak consistency write operation
func (c *Client) SendWeakWrite(key int64, value []byte) int32 {
    c.seqnum++
    p := &MWeakPropose{
        CommandId: c.seqnum,
        ClientId:  c.ClientId,
        Command: state.Command{
            Op: state.PUT,
            K:  state.Key(key),
            V:  value,
        },
        Timestamp: 0,
    }

    // Send only to Leader
    c.SendMsg(c.leader, c.cs.weakProposeRPC, p)
    return c.seqnum
}

// SendWeakRead - Send weak consistency read operation
func (c *Client) SendWeakRead(key int64) int32 {
    c.seqnum++
    p := &MWeakPropose{
        CommandId: c.seqnum,
        ClientId:  c.ClientId,
        Command: state.Command{
            Op: state.GET,
            K:  state.Key(key),
            V:  state.NIL(),
        },
        Timestamp: 0,
    }

    c.SendMsg(c.leader, c.cs.weakProposeRPC, p)
    return c.seqnum
}
```

### 3.3 Handle Weak Reply

```go
func (c *Client) handleWeakReply(rep *MWeakReply) {
    if _, exists := c.delivered[rep.CmdId.SeqNum]; exists {
        return
    }

    // Weak command completes upon receiving Leader's reply
    c.val = state.Value(rep.Rep)
    c.delivered[rep.CmdId.SeqNum] = struct{}{}
    c.RegisterReply(c.val, rep.CmdId.SeqNum)
}
```

### 3.4 Update Message Handling Loop

```go
func (c *Client) handleMsgs() {
    for {
        select {
        // ... existing cases ...

        case m := <-c.cs.weakReplyChan:
            rep := m.(*MWeakReply)
            c.handleWeakReply(rep)
        }
    }
}
```

---

## Phase 4: Replica-Side Modifications

### 4.1 Add Communication Channels (defs.go)

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

### 4.2 Register New RPCs (defs.go)

```go
func initCs(cs *CommunicationSupply, t *fastrpc.Table) {
    // ... existing registrations ...

    cs.weakProposeChan = make(chan fastrpc.Serializable, defs.CHAN_BUFFER_SIZE)
    cs.weakReplyChan = make(chan fastrpc.Serializable, defs.CHAN_BUFFER_SIZE)

    cs.weakProposeRPC = t.Register(new(MWeakPropose), cs.weakProposeChan)
    cs.weakReplyRPC = t.Register(new(MWeakReply), cs.weakReplyChan)
}
```

### 4.3 Add Weak Command Handling (Core Logic)

**Add to run() function in curp-ht.go:**

```go
func (r *Replica) run() {
    // ... existing initialization ...

    for !r.Shutdown {
        select {
        // ... existing cases ...

        case m := <-r.cs.weakProposeChan:
            if r.isLeader {
                weakPropose := m.(*MWeakPropose)
                r.handleWeakPropose(weakPropose)
            }
            // Non-Leader ignores weak propose (should not receive it)
        }
    }
}
```

### 4.4 Implement handleWeakPropose

```go
func (r *Replica) handleWeakPropose(propose *MWeakPropose) {
    // 1. Assign slot (share slot space with strong for global ordering)
    slot := r.lastCmdSlot
    r.lastCmdSlot++

    // 2. Record dependency (for causal ordering)
    dep := r.leaderUnsync(propose.Command, slot)

    // 3. Create command descriptor
    desc := r.getWeakCmdDesc(slot, propose, dep)

    // 4. Speculative execution (execute immediately)
    desc.val = propose.Command.Execute(r.State)
    r.executed.Set(strconv.Itoa(slot), struct{}{})

    // 5. Reply to Client immediately (don't wait for replication)
    rep := &MWeakReply{
        Replica: r.Id,
        Ballot:  r.ballot,
        CmdId: CommandId{
            ClientId: propose.ClientId,
            SeqNum:   propose.CommandId,
        },
        Rep: desc.val,
    }
    r.sender.SendToClient(propose.ClientId, rep, r.cs.weakReplyRPC)

    // 6. Async replication (background, non-blocking)
    go r.asyncReplicateWeak(desc, slot)
}
```

### 4.5 Implement Async Replication

```go
func (r *Replica) asyncReplicateWeak(desc *commandDesc, slot int) {
    // Send Accept to other replicas
    acc := &MAccept{
        Replica: r.Id,
        Ballot:  r.ballot,
        CmdId:   desc.cmdId,
        CmdSlot: slot,
        // Optional: mark as weak command
    }

    r.batcher.SendAccept(acc)
    r.handleAccept(acc, desc)

    // Wait for majority ack then commit
    // (reuse existing accept/commit flow)
}
```

### 4.6 Differentiate Weak and Strong Command Descriptors

```go
type commandDesc struct {
    // ... existing fields ...

    isWeak bool  // Mark if this is a weak command
}

func (r *Replica) getWeakCmdDesc(slot int, propose *MWeakPropose, dep int) *commandDesc {
    desc := r.newDesc()
    desc.isWeak = true
    desc.cmdSlot = slot
    desc.dep = dep
    desc.cmdId = CommandId{
        ClientId: propose.ClientId,
        SeqNum:   propose.CommandId,
    }
    desc.cmd = propose.Command

    return desc
}
```

---

## Phase 5: Performance Optimizations

### 5.1 Original Protocol Performance Issues Analysis

| Issue | Location | Impact | Optimization |
|-------|----------|--------|--------------|
| Frequent string conversions | `strconv.Itoa`, `strconv.FormatInt` | CPU overhead, GC pressure | Use integer-keyed concurrent map |
| Batcher allocates new slices each time | `batcher.go:22-25` | Memory allocation, GC pressure | Use sync.Pool for reuse |
| New goroutine per command | `curp.go:570` | Scheduling overhead | Use worker pool |
| MsgSet callback not optimized | `handleAcks` | Lock contention | Use lock-free counters |

### 5.2 Optimization Implementations

#### 5.2.1 Use Integer-Keyed Map

```go
// Replace cmap.ConcurrentMap with custom implementation
type IntConcurrentMap struct {
    shards    []*intMapShard
    shardMask uint64
}

type intMapShard struct {
    sync.RWMutex
    m map[int]interface{}
}

// Avoid strconv.Itoa overhead
func (r *Replica) getSlotKey(slot int) int {
    return slot  // Use integer directly
}
```

#### 5.2.2 Batcher Optimization

```go
type Batcher struct {
    acks chan rpc.Serializable
    accs chan rpc.Serializable

    // Object pool
    aacksPool sync.Pool
}

func NewBatcher(r *Replica, size int) *Batcher {
    b := &Batcher{
        acks: make(chan rpc.Serializable, size),
        accs: make(chan rpc.Serializable, size),
        aacksPool: sync.Pool{
            New: func() interface{} {
                return &MAAcks{
                    Acks:    make([]MAcceptAck, 0, 16),
                    Accepts: make([]MAccept, 0, 16),
                }
            },
        },
    }
    // ...
}
```

#### 5.2.3 Worker Pool for Command Processing

```go
type Replica struct {
    // ... existing fields ...

    workerPool chan func()
}

func (r *Replica) initWorkerPool(size int) {
    r.workerPool = make(chan func(), 1000)

    for i := 0; i < size; i++ {
        go func() {
            for f := range r.workerPool {
                f()
            }
        }()
    }
}

// Replace go r.handleDesc(...)
func (r *Replica) submitWork(f func()) {
    select {
    case r.workerPool <- f:
    default:
        // Execute directly when pool is full
        go f()
    }
}
```

#### 5.2.4 Weak Path Specific Optimization

```go
// Weak command uses a simpler processing path
func (r *Replica) handleWeakProposeFast(propose *MWeakPropose) {
    slot := atomic.AddInt64(&r.lastCmdSlot, 1) - 1

    // Execute directly without creating full descriptor
    val := propose.Command.Execute(r.State)

    // Use pre-allocated reply object
    rep := r.weakReplyPool.Get().(*MWeakReply)
    rep.Replica = r.Id
    rep.Ballot = r.ballot
    rep.CmdId.ClientId = propose.ClientId
    rep.CmdId.SeqNum = propose.CommandId
    rep.Rep = val

    r.sender.SendToClient(propose.ClientId, rep, r.cs.weakReplyRPC)

    // Async replication
    r.submitWork(func() {
        r.replicateWeakAsync(propose, int(slot))
        r.weakReplyPool.Put(rep)
    })
}
```

### 5.3 Weak Command Batch Processing Optimization

```go
// Batch process weak commands to improve throughput
type WeakBatcher struct {
    commands chan *MWeakPropose
    ticker   *time.Ticker
}

func (r *Replica) runWeakBatcher() {
    batch := make([]*MWeakPropose, 0, 32)

    for {
        select {
        case cmd := <-r.weakBatcher.commands:
            batch = append(batch, cmd)
            if len(batch) >= 32 {
                r.processWeakBatch(batch)
                batch = batch[:0]
            }

        case <-r.weakBatcher.ticker.C:
            if len(batch) > 0 {
                r.processWeakBatch(batch)
                batch = batch[:0]
            }
        }
    }
}
```

---

## Phase 6: Causal Ordering Guarantee

### 6.1 Design Description

Weak commands need to guarantee Causal Consistency:
1. Operation order from the same Client is preserved
2. Read operations can see writes from causal predecessors

### 6.2 Implementation Mechanism

```go
// Client-side: Track last weak command
type Client struct {
    // ...
    lastWeakSeqNum int32  // Sequence number of last weak command
}

// Include dependency information when sending
type MWeakPropose struct {
    CommandId    int32
    ClientId     int32
    Command      state.Command
    Timestamp    int64
    CausalDep    int32  // Depends on the seq num of previous weak command
}
```

```go
// Leader-side: Ensure causal order
func (r *Replica) handleWeakPropose(propose *MWeakPropose) {
    // Wait for causal dependency to finish execution
    if propose.CausalDep > 0 {
        r.waitForExecution(propose.ClientId, propose.CausalDep)
    }

    // Continue execution...
}
```

---

## Phase 7: Testing and Verification

### 7.1 Unit Tests

- `TestWeakCommandExecution` - Weak command executes correctly
- `TestStrongCommandUnchanged` - Strong command behavior unchanged
- `TestMixedCommands` - Correctness with mixed weak/strong
- `TestCausalOrdering` - Causal ordering guarantee
- `TestWeakReplicationEventual` - Weak command eventually replicates

### 7.2 Performance Tests

- Pure Weak throughput test
- Pure Strong throughput test (ensure no regression)
- Mixed workload test (80% weak, 20% strong)
- Latency distribution test

### 7.3 Consistency Tests

- Jepsen test (optional)
- Manual fault injection test

---

## File Modification Checklist

| File | Modification Type | Description |
|------|-------------------|-------------|
| `curp-ht/defs.go` | New/Modify | New message types, constant definitions |
| `curp-ht/client.go` | Modify | Weak command send and handling |
| `curp-ht/curp-ht.go` | Modify | Weak command processing logic |
| `curp-ht/batcher.go` | Modify | Object pool optimization |
| `curp-ht/weak.go` | New | Weak command specific handling (optional split) |
| `replica/defs/defs.go` | No change | Keep original Propose structure |

---

## Recommended Implementation Order

1. **Phase 1**: Copy files, modify package names
2. **Phase 2**: Add new message types and serialization code
3. **Phase 3**: Implement Client-side weak command sending
4. **Phase 4**: Implement Leader-side weak command handling (basic version)
5. **Phase 5**: Add performance optimizations (object pools, worker pool)
6. **Phase 6**: Implement causal ordering
7. **Phase 7**: Testing and tuning

---

## Risks and Considerations

1. **Shared Slot Space**: Weak and Strong commands must share the slot sequence, otherwise global ordering may be broken
2. **Recovery**: Ensure weak commands can be correctly recovered during recovery
3. **Leader Switch**: Handle pending weak commands during leader switch
4. **Memory Management**: Watch for memory leaks in async replication of weak commands

---

## Estimated Code Lines

| Component | New Lines | Modified Lines |
|-----------|-----------|----------------|
| defs.go | ~200 | ~20 |
| client.go | ~100 | ~50 |
| curp-ht.go | ~150 | ~100 |
| batcher.go | ~50 | ~30 |
| weak.go (optional) | ~200 | - |
| **Total** | **~700** | **~200** |
