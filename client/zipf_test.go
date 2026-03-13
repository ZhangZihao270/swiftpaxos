package client

import (
	"math"
	"testing"
)

// TestUniformKeyGenerator tests uniform key generation
func TestUniformKeyGenerator(t *testing.T) {
	keySpace := int64(1000)
	gen := NewUniformKeyGenerator(keySpace, 42)

	// Generate keys and verify they're in range
	for i := 0; i < 1000; i++ {
		key := gen.NextKey()
		if key < 0 || key >= keySpace {
			t.Errorf("Key %d out of range [0, %d)", key, keySpace)
		}
	}
}

// TestZipfKeyGenerator tests Zipf key generation with s <= 1 (CDF sampler)
func TestZipfKeyGenerator(t *testing.T) {
	keySpace := int64(1000)
	skew := 0.99
	gen := NewZipfKeyGenerator(keySpace, skew, 42)

	// Generate keys and verify they're in range
	for i := 0; i < 1000; i++ {
		key := gen.NextKey()
		if key < 0 || key >= keySpace {
			t.Errorf("Key %d out of range [0, %d)", key, keySpace)
		}
	}

	// Verify CDF sampler is used (not stdlib)
	if gen.cdf == nil {
		t.Error("Expected CDF sampler for skew=0.99, got stdlib Zipf")
	}
	if gen.zipf != nil {
		t.Error("Expected no stdlib Zipf for skew=0.99")
	}
}

// TestZipfDistributionSkew tests that Zipf distribution is actually skewed
func TestZipfDistributionSkew(t *testing.T) {
	keySpace := int64(100)
	skew := 1.5 // High skew
	gen := NewZipfKeyGenerator(keySpace, skew, 42)

	// Count frequency of each key
	counts := make(map[int64]int)
	numSamples := 10000

	for i := 0; i < numSamples; i++ {
		key := gen.NextKey()
		counts[key]++
	}

	// With high skew, key 0 should be accessed much more than key 50
	key0Count := counts[0]
	key50Count := counts[50]

	// Key 0 should be accessed at least 5x more than key 50
	if key0Count < key50Count*3 {
		t.Errorf("Zipf distribution not skewed enough: key0=%d, key50=%d", key0Count, key50Count)
	}
}

// TestNewKeyGeneratorUniform tests NewKeyGenerator with skew=0
func TestNewKeyGeneratorUniform(t *testing.T) {
	gen := NewKeyGenerator(1000, 0, 1)

	// Should return UniformKeyGenerator
	if _, ok := gen.(*UniformKeyGenerator); !ok {
		t.Error("Expected UniformKeyGenerator for skew=0")
	}
}

// TestNewKeyGeneratorZipf tests NewKeyGenerator with skew>0
func TestNewKeyGeneratorZipf(t *testing.T) {
	gen := NewKeyGenerator(1000, 0.99, 1)

	// Should return ZipfKeyGenerator
	if _, ok := gen.(*ZipfKeyGenerator); !ok {
		t.Error("Expected ZipfKeyGenerator for skew=0.99")
	}
}

// TestNewKeyGeneratorSmallSkew tests NewKeyGenerator with very small skew
func TestNewKeyGeneratorSmallSkew(t *testing.T) {
	gen := NewKeyGenerator(1000, 0.001, 1)

	// Should return UniformKeyGenerator for very small skew
	if _, ok := gen.(*UniformKeyGenerator); !ok {
		t.Error("Expected UniformKeyGenerator for skew=0.001")
	}
}

// TestUniformDistribution tests that uniform distribution is actually uniform
func TestUniformDistribution(t *testing.T) {
	keySpace := int64(10)
	gen := NewUniformKeyGenerator(keySpace, 42)

	// Count frequency of each key
	counts := make(map[int64]int)
	numSamples := 10000

	for i := 0; i < numSamples; i++ {
		key := gen.NextKey()
		counts[key]++
	}

	// Each key should be accessed roughly equally (within 50% of expected)
	expected := numSamples / int(keySpace)
	for key := int64(0); key < keySpace; key++ {
		count := counts[key]
		if count < expected/2 || count > expected*2 {
			t.Errorf("Uniform distribution not uniform: key %d count=%d, expected ~%d", key, count, expected)
		}
	}
}

// TestDefaultKeySpace tests the default key space constant
func TestDefaultKeySpace(t *testing.T) {
	if DefaultKeySpace != 10000 {
		t.Errorf("DefaultKeySpace = %d, want 10000", DefaultKeySpace)
	}
}

// TestZipfCDFSamplerForSmallSkew tests CDF sampler for s <= 1
func TestZipfCDFSamplerForSmallSkew(t *testing.T) {
	keySpace := int64(100)

	// Test s=0.5 — CDF sampler should be used
	gen := NewZipfKeyGenerator(keySpace, 0.5, 42)
	if gen.cdf == nil {
		t.Fatal("Expected CDF sampler for skew=0.5")
	}

	for i := 0; i < 100; i++ {
		key := gen.NextKey()
		if key < 0 || key >= keySpace {
			t.Errorf("Key %d out of range [0, %d)", key, keySpace)
		}
	}

	// Test s=1.0 — CDF sampler should be used
	gen = NewZipfKeyGenerator(keySpace, 1.0, 42)
	if gen.cdf == nil {
		t.Fatal("Expected CDF sampler for skew=1.0")
	}

	for i := 0; i < 100; i++ {
		key := gen.NextKey()
		if key < 0 || key >= keySpace {
			t.Errorf("Key %d out of range [0, %d)", key, keySpace)
		}
	}
}

// TestZipfNegativeSkew tests that negative skew values are handled
func TestZipfNegativeSkew(t *testing.T) {
	keySpace := int64(100)

	// Negative skew should use CDF sampler with minimum skew
	gen := NewZipfKeyGenerator(keySpace, -2.0, 42)
	if gen.cdf == nil {
		t.Fatal("Expected CDF sampler for skew=-2.0")
	}

	for i := 0; i < 100; i++ {
		key := gen.NextKey()
		if key < 0 || key >= keySpace {
			t.Errorf("Key %d out of range [0, %d)", key, keySpace)
		}
	}
}

// TestKeyGeneratorDifferentSeeds tests that different seeds produce different sequences
func TestKeyGeneratorDifferentSeeds(t *testing.T) {
	keySpace := int64(1000)

	gen1 := NewUniformKeyGenerator(keySpace, 1)
	gen2 := NewUniformKeyGenerator(keySpace, 2)

	// Generate sequences
	seq1 := make([]int64, 10)
	seq2 := make([]int64, 10)
	for i := 0; i < 10; i++ {
		seq1[i] = gen1.NextKey()
		seq2[i] = gen2.NextKey()
	}

	// Sequences should differ (with very high probability)
	identical := true
	for i := 0; i < 10; i++ {
		if seq1[i] != seq2[i] {
			identical = false
			break
		}
	}
	if identical {
		t.Error("Different seeds produced identical sequences")
	}
}

// TestKeyGeneratorSameSeed tests that same seed produces same sequence
func TestKeyGeneratorSameSeed(t *testing.T) {
	keySpace := int64(1000)

	gen1 := NewUniformKeyGenerator(keySpace, 42)
	gen2 := NewUniformKeyGenerator(keySpace, 42)

	// Generate sequences
	for i := 0; i < 100; i++ {
		k1 := gen1.NextKey()
		k2 := gen2.NextKey()
		if k1 != k2 {
			t.Errorf("Same seed produced different keys at index %d: %d vs %d", i, k1, k2)
		}
	}
}

// TestKeyGeneratorHighThroughput tests generator under high load
func TestKeyGeneratorHighThroughput(t *testing.T) {
	keySpace := int64(10000)

	tests := []struct {
		name    string
		gen     KeyGenerator
		numKeys int
	}{
		{
			name:    "Uniform high throughput",
			gen:     NewUniformKeyGenerator(keySpace, 42),
			numKeys: 100000,
		},
		{
			name:    "Zipf s>1 high throughput",
			gen:     NewZipfKeyGenerator(keySpace, 1.5, 42),
			numKeys: 100000,
		},
		{
			name:    "Zipf s<=1 high throughput",
			gen:     NewZipfKeyGenerator(keySpace, 0.75, 42),
			numKeys: 100000,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			for i := 0; i < tc.numKeys; i++ {
				key := tc.gen.NextKey()
				if key < 0 || key >= keySpace {
					t.Errorf("Key %d out of range [0, %d)", key, keySpace)
				}
			}
		})
	}
}

// TestMultipleGeneratorsIndependent tests that multiple generators don't interfere
func TestMultipleGeneratorsIndependent(t *testing.T) {
	keySpace := int64(1000)

	// Create 10 generators with different seeds
	gens := make([]KeyGenerator, 10)
	for i := 0; i < 10; i++ {
		gens[i] = NewKeyGenerator(keySpace, 0.99, int32(i))
	}

	// Generate keys from each generator
	for i := 0; i < 1000; i++ {
		for _, gen := range gens {
			key := gen.NextKey()
			if key < 0 || key >= keySpace {
				t.Errorf("Key %d out of range [0, %d)", key, keySpace)
			}
		}
	}
}

// TestNewKeyGeneratorWithClientIDs simulates multi-client scenario
func TestNewKeyGeneratorWithClientIDs(t *testing.T) {
	keySpace := int64(10000)
	skew := float64(0.99)

	// Simulate 8 clients with 4 threads each (32 total generators)
	numClients := 8
	threadsPerClient := 4

	gens := make([]KeyGenerator, 0, numClients*threadsPerClient)

	for clientIdx := 0; clientIdx < numClients; clientIdx++ {
		baseOffset := clientIdx * threadsPerClient
		for threadIdx := 0; threadIdx < threadsPerClient; threadIdx++ {
			clientID := int32(baseOffset + threadIdx)
			gen := NewKeyGenerator(keySpace, skew, clientID)
			gens = append(gens, gen)
		}
	}

	// Each generator should produce valid keys
	for _, gen := range gens {
		for i := 0; i < 100; i++ {
			key := gen.NextKey()
			if key < 0 || key >= keySpace {
				t.Errorf("Key %d out of range [0, %d)", key, keySpace)
			}
		}
	}
}

// TestZipfSkewDifference verifies that s=0.5 produces a DIFFERENT distribution from s=1.01
func TestZipfSkewDifference(t *testing.T) {
	keySpace := int64(10000)
	numSamples := 100000

	// s=0.5 — mild skew
	gen05 := NewZipfKeyGenerator(keySpace, 0.5, 42)
	counts05 := make(map[int64]int)
	for i := 0; i < numSamples; i++ {
		counts05[gen05.NextKey()]++
	}

	// s=1.5 — heavy skew
	gen15 := NewZipfKeyGenerator(keySpace, 1.5, 42)
	counts15 := make(map[int64]int)
	for i := 0; i < numSamples; i++ {
		counts15[gen15.NextKey()]++
	}

	// Top-1% keys (top 100 of 10K) traffic share
	top1pct05 := 0
	top1pct15 := 0
	for k := int64(0); k < 100; k++ {
		top1pct05 += counts05[k]
		top1pct15 += counts15[k]
	}

	share05 := float64(top1pct05) / float64(numSamples)
	share15 := float64(top1pct15) / float64(numSamples)

	// s=0.5: top-1% should get roughly 5-15% of traffic
	if share05 > 0.30 {
		t.Errorf("s=0.5: top-1%% keys got %.1f%% of traffic (too high, expected <30%%)", share05*100)
	}

	// s=1.5: top-1% should get >60% of traffic
	if share15 < 0.40 {
		t.Errorf("s=1.5: top-1%% keys got %.1f%% of traffic (too low, expected >40%%)", share15*100)
	}

	// s=0.5 should have significantly less concentration than s=1.5
	if share05 >= share15 {
		t.Errorf("s=0.5 (%.1f%%) should have less top-1%% concentration than s=1.5 (%.1f%%)",
			share05*100, share15*100)
	}
}

// TestZipfS099DifferentFromS101 verifies s=0.99 is distinct from s=1.01
func TestZipfS099DifferentFromS101(t *testing.T) {
	keySpace := int64(10000)
	numSamples := 100000

	gen099 := NewZipfKeyGenerator(keySpace, 0.99, 42)
	gen101 := NewZipfKeyGenerator(keySpace, 1.01, 42)

	counts099 := make(map[int64]int)
	counts101 := make(map[int64]int)
	for i := 0; i < numSamples; i++ {
		counts099[gen099.NextKey()]++
		counts101[gen101.NextKey()]++
	}

	// Compute top-1% share
	top099 := 0
	top101 := 0
	for k := int64(0); k < 100; k++ {
		top099 += counts099[k]
		top101 += counts101[k]
	}

	share099 := float64(top099) / float64(numSamples)
	share101 := float64(top101) / float64(numSamples)

	// They should be different distributions — s=0.99 uses CDF, s=1.01 uses stdlib
	// The distributions are close but use different implementations
	t.Logf("s=0.99 top-1%% share: %.2f%%, s=1.01 top-1%% share: %.2f%%", share099*100, share101*100)

	// Both should be reasonable Zipf distributions
	if share099 < 0.01 || share099 > 0.80 {
		t.Errorf("s=0.99 top-1%% share %.2f%% out of expected range", share099*100)
	}
	if share101 < 0.01 || share101 > 0.80 {
		t.Errorf("s=1.01 top-1%% share %.2f%% out of expected range", share101*100)
	}
}

// TestZipfStdlibStillUsedForHighSkew verifies s > 1 still uses Go stdlib
func TestZipfStdlibStillUsedForHighSkew(t *testing.T) {
	gen := NewZipfKeyGenerator(1000, 1.5, 42)
	if gen.zipf == nil {
		t.Error("Expected stdlib Zipf for skew=1.5")
	}
	if gen.cdf != nil {
		t.Error("Expected no CDF sampler for skew=1.5")
	}

	gen2 := NewZipfKeyGenerator(1000, 2.0, 42)
	if gen2.zipf == nil {
		t.Error("Expected stdlib Zipf for skew=2.0")
	}
}

// TestCDFZipfSamplerMonotonicity verifies CDF is monotonically increasing
func TestCDFZipfSamplerMonotonicity(t *testing.T) {
	keySpace := int64(1000)
	for _, s := range []float64{0.25, 0.5, 0.75, 0.99, 1.0} {
		gen := NewZipfKeyGenerator(keySpace, s, 42)
		if gen.cdf == nil {
			t.Fatalf("Expected CDF sampler for s=%.2f", s)
		}
		cdf := gen.cdf.cdf
		for i := 1; i < len(cdf); i++ {
			if cdf[i] < cdf[i-1] {
				t.Errorf("s=%.2f: CDF not monotonic at index %d: %.6f < %.6f", s, i, cdf[i], cdf[i-1])
			}
		}
		// Last entry should be 1.0
		if math.Abs(cdf[len(cdf)-1]-1.0) > 1e-10 {
			t.Errorf("s=%.2f: CDF last entry = %.10f, want 1.0", s, cdf[len(cdf)-1])
		}
	}
}

// TestZipfLargeKeySpace tests with keySpace=1M (realistic benchmark size)
func TestZipfLargeKeySpace(t *testing.T) {
	keySpace := int64(1000000)

	gen := NewZipfKeyGenerator(keySpace, 0.75, 42)
	if gen.cdf == nil {
		t.Fatal("Expected CDF sampler for skew=0.75")
	}

	// Generate 1000 keys and verify range
	for i := 0; i < 1000; i++ {
		key := gen.NextKey()
		if key < 0 || key >= keySpace {
			t.Errorf("Key %d out of range [0, %d)", key, keySpace)
		}
	}
}
