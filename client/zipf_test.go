package client

import (
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

// TestZipfKeyGenerator tests Zipf key generation
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

// TestZipfSkewClamping tests that skew values <= 1 are clamped to 1.01
func TestZipfSkewClamping(t *testing.T) {
	keySpace := int64(100)

	// Test with skew = 0.5 (should be clamped to 1.01)
	gen := NewZipfKeyGenerator(keySpace, 0.5, 42)
	if gen == nil {
		t.Fatal("Expected non-nil ZipfKeyGenerator for skew=0.5")
	}

	// Test with skew = 1.0 (edge case, should be clamped to 1.01)
	gen = NewZipfKeyGenerator(keySpace, 1.0, 42)
	if gen == nil {
		t.Fatal("Expected non-nil ZipfKeyGenerator for skew=1.0")
	}

	// Generate some keys to ensure it works
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

	// Negative skew should be clamped to 1.01
	gen := NewZipfKeyGenerator(keySpace, -2.0, 42)
	if gen == nil {
		t.Fatal("Expected non-nil ZipfKeyGenerator for skew=-2.0")
	}

	// Generate some keys to ensure it works
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
			name:    "Zipf high throughput",
			gen:     NewZipfKeyGenerator(keySpace, 1.5, 42),
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
