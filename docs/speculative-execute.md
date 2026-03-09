# CURP Speculative Execute — Correct Implementation (Future Work)

## Problem
Current speculative reply in deliver() either:
1. Waits for ALL previous slots to execute (original baseline, correct but slow — 27.6K peak)
2. Skips slot ordering entirely (CURP-HO bug from Phase 19, ported to baseline/HT in Phase 77 — fast but stale reads)

Both are wrong. Option 1 is too conservative (blocks on unrelated slots). Option 2 is incorrect (violates linearizability for strong GETs).

## Correct Approach
Speculative execute should:
1. Only need a slot ID assigned (no waiting for previous slots)
2. Start from committed state (`r.State`)
3. Find all commands ordered before this slot that touch the same key but haven't been executed yet
4. Apply those conflicting commands in slot order
5. Then compute and return the result

This gives correct speculative results without the slot ordering bottleneck.

## Existing Infrastructure
- `leaderUnsync()` tracks per-key latest slot via `r.unsynced[key] = slot`
- `desc.dep` records the direct predecessor (same-key previous slot)
- Full dep chain can be reconstructed by following desc.dep recursively
- CURP-HO has `computeSpeculativeResultWithUnsynced` which partially does this (checks unsynced weak writes pool) but doesn't replay pending strong writes along the dep chain

## When to Implement
After Phase 83 results — if slot ordering bottleneck limits throughput too much, this optimization restores throughput while maintaining correctness.
