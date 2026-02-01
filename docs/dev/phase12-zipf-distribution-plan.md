# Phase 12: Zipf Distribution for Key Access Pattern Implementation Plan

## Overview

Add support for varying key access skewness using Zipf distribution, enabling realistic workload simulation where some keys are accessed more frequently than others.

## Design

### Zipf Distribution

The Zipf distribution follows: P(k) ∝ 1/k^s

Where:
- k is the rank of an item
- s is the skewness parameter (exponent)

Parameter values:
- s = 0: Uniform distribution (current behavior)
- s = 0.99: Moderate skew (top 20% keys get ~80% accesses)
- s = 1.5: High skew (top 1% keys get majority of accesses)

### Implementation Approach

1. Use Go's built-in `math/rand.Zipf` which implements the Zipf distribution
2. Create a KeyGenerator interface for abstraction
3. Implement UniformKeyGenerator (current behavior) and ZipfKeyGenerator

### Configuration Parameters

- `zipfSkew`: Skewness parameter s (default: 0 = uniform)
- `keySpace`: Total number of unique keys (default: 10000)

## Implementation Tasks

### Phase 12.1: Zipf Generator

1. Create `client/zipf.go` with KeyGenerator interface and implementations
2. Add zipfSkew and keySpace to config
3. Update config parser

### Phase 12.2: Benchmark Integration

1. Update genGetKey() in buffer.go to use KeyGenerator
2. Update HybridLoop to use KeyGenerator
3. Maintain backward compatibility

### Phase 12.3: Testing

1. Test Zipf distribution correctness
2. Test config parsing
3. Validate distribution with histogram

## Code Structure

```go
// client/zipf.go

type KeyGenerator interface {
    NextKey() int64
}

type UniformKeyGenerator struct {
    rand     *rand.Rand
    keySpace int64
}

type ZipfKeyGenerator struct {
    zipf     *rand.Zipf
    keySpace int64
}
```
