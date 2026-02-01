package client

import (
	"math/rand"
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
type ZipfKeyGenerator struct {
	zipf     *rand.Zipf
	keySpace int64
}

// NewZipfKeyGenerator creates a new ZipfKeyGenerator.
// Parameters:
//   - keySpace: total number of unique keys (n)
//   - skew: Zipf skewness parameter (s), higher = more skewed, must be > 1
//   - seed: random seed for reproducibility
//
// The Zipf distribution follows P(k) ∝ 1/k^s where k is the rank.
// Typical values:
//   - skew = 1.01: mild skew
//   - skew = 1.5: moderate skew
//   - skew = 2.0: high skew
//
// Note: Go's rand.NewZipf requires s > 1, so we clamp skew to minimum 1.01
func NewZipfKeyGenerator(keySpace int64, skew float64, seed int64) *ZipfKeyGenerator {
	source := rand.NewSource(seed)
	r := rand.New(source)

	// rand.Zipf requires s > 1, so ensure minimum of 1.01
	if skew <= 1.0 {
		skew = 1.01
	}

	// rand.Zipf parameters:
	// - r: random source
	// - s: exponent (skewness), must be > 1
	// - v: offset >= 1 (we use 1)
	// - imax: maximum value (keySpace - 1)
	zipf := rand.NewZipf(r, skew, 1.0, uint64(keySpace-1))

	return &ZipfKeyGenerator{
		zipf:     zipf,
		keySpace: keySpace,
	}
}

// NextKey returns a key according to Zipf distribution.
// Lower keys (e.g., 0, 1, 2) are more likely to be returned.
func (g *ZipfKeyGenerator) NextKey() int64 {
	return int64(g.zipf.Uint64())
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
