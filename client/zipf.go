package client

import (
	"math"
	"math/rand"
	"sort"
	"time"
)

// KeyGenerator defines the interface for generating keys for benchmark operations.
type KeyGenerator interface {
	// NextKey returns the next key to use for a benchmark operation.
	NextKey() int64
}

// UniformKeyGenerator generates keys uniformly at random from [0, keySpace).
type UniformKeyGenerator struct {
	rand     *rand.Rand
	keySpace int64
}

// NewUniformKeyGenerator creates a new UniformKeyGenerator.
func NewUniformKeyGenerator(keySpace int64, seed int64) *UniformKeyGenerator {
	source := rand.NewSource(seed)
	return &UniformKeyGenerator{
		rand:     rand.New(source),
		keySpace: keySpace,
	}
}

// NextKey returns a uniformly random key in [0, keySpace).
func (g *UniformKeyGenerator) NextKey() int64 {
	return g.rand.Int63n(g.keySpace)
}

// ZipfKeyGenerator generates keys according to Zipf distribution.
// Keys with lower indices are more likely to be selected.
// Supports all s > 0: uses Go stdlib for s > 1, inverse CDF sampler for 0 < s <= 1.
type ZipfKeyGenerator struct {
	zipf     *rand.Zipf      // used when s > 1
	cdf      *cdfZipfSampler // used when 0 < s <= 1
	keySpace int64
}

// cdfZipfSampler implements Zipf sampling via precomputed CDF + binary search.
// This supports any s > 0, including s <= 1 where Go's rand.Zipf doesn't work.
type cdfZipfSampler struct {
	cdf  []float64  // cumulative distribution function, len = keySpace
	rand *rand.Rand // random source
}

// newCDFZipfSampler builds a CDF table for Zipf(s, keySpace) and returns a sampler.
// P(k) ∝ 1/(k+1)^s for k in [0, keySpace).
func newCDFZipfSampler(keySpace int64, s float64, r *rand.Rand) *cdfZipfSampler {
	n := int(keySpace)
	cdf := make([]float64, n)

	// Compute unnormalized PMF: weight[k] = 1/(k+1)^s
	sum := 0.0
	for k := 0; k < n; k++ {
		w := math.Pow(float64(k+1), -s)
		sum += w
		cdf[k] = sum
	}

	// Normalize to [0, 1]
	for k := 0; k < n; k++ {
		cdf[k] /= sum
	}
	// Force last entry to exactly 1.0 to avoid floating-point edge cases
	cdf[n-1] = 1.0

	return &cdfZipfSampler{cdf: cdf, rand: r}
}

// Sample returns a random key in [0, keySpace) according to the Zipf distribution.
func (s *cdfZipfSampler) Sample() int64 {
	u := s.rand.Float64()
	// Binary search for smallest k where cdf[k] >= u
	k := sort.SearchFloat64s(s.cdf, u)
	if k >= len(s.cdf) {
		k = len(s.cdf) - 1
	}
	return int64(k)
}

// NewZipfKeyGenerator creates a new ZipfKeyGenerator.
// Parameters:
//   - keySpace: total number of unique keys (n)
//   - skew: Zipf skewness parameter (s), higher = more skewed
//   - seed: random seed for reproducibility
//
// The Zipf distribution follows P(k) ∝ 1/(k+1)^s where k is the rank.
// For s > 1: uses Go's efficient rand.Zipf (rejection sampling).
// For 0 < s <= 1: uses inverse CDF sampling with precomputed table.
func NewZipfKeyGenerator(keySpace int64, skew float64, seed int64) *ZipfKeyGenerator {
	source := rand.NewSource(seed)
	r := rand.New(source)

	gen := &ZipfKeyGenerator{keySpace: keySpace}

	if skew > 1.0 {
		// Use Go stdlib for s > 1
		gen.zipf = rand.NewZipf(r, skew, 1.0, uint64(keySpace-1))
	} else {
		// Use CDF-based sampler for 0 < s <= 1
		if skew <= 0 {
			skew = 0.01 // minimum positive skew
		}
		gen.cdf = newCDFZipfSampler(keySpace, skew, r)
	}

	return gen
}

// NextKey returns a key according to Zipf distribution.
// Lower keys (e.g., 0, 1, 2) are more likely to be returned.
func (g *ZipfKeyGenerator) NextKey() int64 {
	if g.zipf != nil {
		return int64(g.zipf.Uint64())
	}
	return g.cdf.Sample()
}

// NewKeyGenerator creates a KeyGenerator based on configuration.
// If skew is 0 (or very small), returns UniformKeyGenerator.
// Otherwise returns ZipfKeyGenerator.
func NewKeyGenerator(keySpace int64, skew float64, clientId int32) KeyGenerator {
	seed := time.Now().UnixNano() + int64(clientId)

	if skew < 0.01 {
		// Use uniform distribution for very small or zero skew
		return NewUniformKeyGenerator(keySpace, seed)
	}

	return NewZipfKeyGenerator(keySpace, skew, seed)
}

// DefaultKeySpace is the default number of unique keys.
const DefaultKeySpace int64 = 10000
