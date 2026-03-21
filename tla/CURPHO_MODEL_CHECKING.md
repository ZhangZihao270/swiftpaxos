# CurpHO TLA+ Model Checking Log

## Model Configuration (MC_CurpHO)

- Replicas: {r1 (leader), r2, r3}
- Clients: {c1, c2}
- Keys: {k1}
- Values: {v1, v2}
- MaxOps: 2
- Symmetry: Permutations({r2,r3}) ∪ Permutations({c1,c2}) ∪ Permutations({v1,v2})

## Major Design Change: writeId Version Tracking

The original spec used `keyVersion` (log slot) for strong ops and `clientSeq` as proxy
for weak ops — two incomparable namespaces. This made session guarantee invariants
unable to cover weak ops (all guarded with `retVer > 0` / `slot > 0`).

**Fix**: Added a global `nextWriteId` counter. Every write (strong or weak) gets a
unique, monotonically increasing writeId at issue time. The writeId travels through
the protocol: client → CausalPropose/StrongPropose → log entry + unsynced → kvWriteId.
All session guarantee invariants now use writeId-based `retVer` for both strong and
weak ops.

New variables: `nextWriteId` (global counter), `kvWriteId[r][k]` (committed writeId).

## Violation 1: ReadYourWrites — writeId vs slot reordering

**Found**: 2026-03-17, ~4 min into model checking (57M states, depth 15)

**Trace** (15 states):
1. c1 issues strong write k1=v1 (writeId=1, epoch=1)
2. c2 issues weak write k1=v1 (writeId=2, epoch=2)
3. Leader assigns: c2's weak write → slot 1, c1's strong write → slot 2
4. c2 receives CausalReply → completes, history retVer=2
5. c2 issues strong read k1
6. Leader returns speculative value v1 from unsynced (c1's strong write, writeId=1)
7. c2 completes strong read → history retVer=1

**Violated invariant**: `ReadYourWrites` — c2 wrote with retVer=2, then read with
retVer=1. 1 < 2.

**Root cause**: Concurrent writes can be reordered: writeId order ≠ slot order.
c2's weak write (writeId=2) at slot 1, c1's strong write (writeId=1) at slot 2.
`SpeculativeWriteId` returned writeId=1 (from the unsynced entry at slot 2), but
the log also contains writeId=2 at slot 1 which is higher.

**Fix**: Added `MaxLogWriteId(logSeq, k, maxSlot, acc)` recursive helper that scans
the entire log prefix for the maximum writeId of all writes to key k. Strong read's
retVer now uses `MaxLogWriteId(newLog, k, slot-1, 0)` instead of
`SpeculativeWriteId(r, k)`.

In the example: MaxLogWriteId returns max(1, 2) = 2 → retVer=2 ≥ 2 → OK.

## Violation 2: StrongReadConsistency — SpeculativeVal uses stale unsynced

**Found**: 2026-03-17, ~10 min into model checking (after Violation 1 fix)

**Trace** (14 states):
1. c1 issues weak write k1=v1 (writeId=1)
2. c2 issues strong read k1
3. Leader processes c1's CausalPropose → slot 1, unsynced[r1][k1] = weak write v1
4. c1 issues strong read k1
5. Leader processes c1's StrongReadPropose:
   - specVal = SpeculativeVal = v1 (correct, unsynced has weak write)
   - **Overwrites unsynced[r1][k1] with strong read entry** (op=Read, writeId=0)
6. Leader processes c2's StrongReadPropose:
   - specVal = SpeculativeVal(r1, k1) → unsynced is a Read entry → falls through
     to kvStore[r1][k1] = **nil** (nothing committed yet!)
   - But log has c1's write at slot 1 → correct value should be **v1**

**Violated invariant**: `StrongReadConsistency` — strong read at slot 3 returned nil
but log[slot 1] has write k1=v1.

**Root cause**: `SpeculativeVal(r, k)` only checks the single unsynced entry per key.
When a later strong op overwrites the unsynced entry, the previous write's value is
lost from the speculative view. But the value IS in the leader's log.

**Fix**: Added `LastLogWriteVal(logSeq, k, idx)` recursive helper that scans the log
backward for the last write to key k. Strong read's specVal now uses
`LastLogWriteVal(newLog, k, slot-1)` instead of `SpeculativeVal(r, k)`.

This is always correct on the leader because the leader has the full log with all
assigned entries.

## Spec vs Implementation Gaps

| # | Gap | Severity | Status |
|---|-----|----------|--------|
| 1 | Missing causal barrier on leader | CRITICAL | Spec has it (line ~637,858); impl missing |
| 2 | HandleWeakRead ignores witness pool | HIGH | Spec returns own-client speculative; impl returns committed only |
| 3 | No writeId in impl | MEDIUM | Spec uses writeId; impl uses slot/keyVersion |
| 4 | Cache merge uses > not >= | LOW | Spec prefers cache on tie; impl prefers replica |
| 5 | ReadDep validation incomplete | LOW | Minor |

See parent conversation for detailed analysis of each gap.

## Current Status

Model checking restarted 2026-03-17 with both fixes applied. Running with
MaxOps=2, 3 replicas, 2 clients, 1 key, 2 values.
