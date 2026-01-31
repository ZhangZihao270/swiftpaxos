# Phase 3 Plan: Client-Side Modifications

## Overview
Modify the curp-ht client to support weak consistency commands.

## Task 3.1: Add Weak Command Tracking Fields

### Location
curp-ht/client.go - Client struct

### Changes
Add field to track pending weak commands:
```go
type Client struct {
    // ... existing fields ...

    // Weak command tracking
    weakPending map[int32]struct{}  // Track pending weak commands by seqnum
}
```

### Initialization
In NewClient(), add:
```go
weakPending: make(map[int32]struct{}),
```

---

## Task 3.2: Implement SendWeakWrite Method

### Signature
```go
func (c *Client) SendWeakWrite(key int64, value []byte) int32
```

### Implementation
1. Increment sequence number
2. Create MWeakPropose with PUT command
3. Send to leader only
4. Track in weakPending
5. Return seqnum

---

## Task 3.3: Implement SendWeakRead Method

### Signature
```go
func (c *Client) SendWeakRead(key int64) int32
```

### Implementation
1. Increment sequence number
2. Create MWeakPropose with GET command
3. Send to leader only
4. Track in weakPending
5. Return seqnum

---

## Task 3.4: Implement handleWeakReply Method

### Signature
```go
func (c *Client) handleWeakReply(rep *MWeakReply)
```

### Implementation
1. Check if already delivered (dedup)
2. Update ballot if needed
3. Set value from reply
4. Mark as delivered
5. Register reply

---

## Task 3.5: Update handleMsgs Loop

### Changes
Add case for weakReplyChan:
```go
case m := <-c.cs.weakReplyChan:
    rep := m.(*MWeakReply)
    c.handleWeakReply(rep)
```

---

## Estimated LOC
- Task 3.1: ~5 lines
- Task 3.2: ~20 lines
- Task 3.3: ~20 lines
- Task 3.4: ~15 lines
- Task 3.5: ~5 lines

Total: ~65 lines (well within 500 LOC limit)
