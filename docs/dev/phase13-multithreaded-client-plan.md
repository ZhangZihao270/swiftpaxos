# Phase 13: Multi-threaded Client Support Implementation Plan

## Overview

Enable each client process to run multiple client threads, allowing higher throughput from fewer physical machines.

## Design Decisions

### 1. Configuration Approach

**Option A**: Per-client inline syntax (`client0 127.0.0.4 threads=4`)
- Pros: Flexible per-client configuration
- Cons: Requires parsing changes for client lines

**Option B**: Global `clientThreads` parameter
- Pros: Simple, consistent with existing parameters
- Cons: All clients get same thread count

**Decision**: Implement Option B first (global `clientThreads` parameter) as it's simpler and matches the existing config pattern. This provides immediate value with minimal complexity. Per-client override (Option A) can be added later if needed.

### 2. Thread Implementation

Each thread will:
- Have unique client ID: `baseClientId + threadIndex`
- Create separate connection to replicas
- Run independent benchmark loop
- Track its own metrics

Aggregation:
- Wait for all threads to complete
- Merge latency arrays
- Sum operation counts
- Calculate combined throughput

### 3. Backward Compatibility

- Default `clientThreads = 0` means use existing `clones` behavior
- When `clientThreads > 0`, it overrides `clones` for thread count
- Existing single-threaded benchmarks work unchanged

## Implementation Tasks

### Phase 13.1: Configuration (Tasks 13.1.1-13.1.3)

1. Add `ClientThreads int` field to Config struct
2. Add `clientthreads` case to config parser
3. Add tests for parsing

### Phase 13.2: Multi-threaded Execution (Tasks 13.2.1-13.2.3)

1. Update main.go to spawn goroutines based on ClientThreads
2. Each goroutine creates independent HybridBufferClient
3. Implement metrics aggregation after all threads complete

### Phase 13.3-13.4: Scripts and Testing

1. Update benchmark scripts for thread-aware output
2. Add comprehensive tests

## Code Changes Summary

### config/config.go
- Add `ClientThreads int` field to Config struct
- Add parsing case for "clientthreads" parameter

### main.go
- Modify runClient() to use ClientThreads if > 0
- Launch multiple goroutines with WaitGroup
- Aggregate metrics at end

## Test Plan

1. Config parsing tests for clientThreads
2. Default value (0) backward compatibility
3. Multi-threaded execution correctness
