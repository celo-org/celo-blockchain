package random

import (
	"math/rand"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

func randomHash() common.Hash {
	var value common.Hash
	// Always returns nil https://golang.org/pkg/math/rand/#Read
	rand.Read(value[:])
	return value
}

func BenchmarkPermutation(b *testing.B) {
	// Setup the random values that will be fed in to the method.
	seeds := make([]common.Hash, b.N)
	for i := range seeds {
		seeds[i] = randomHash()
	}

	b.ResetTimer()
	for _, seed := range seeds {
		Permutation(seed, 1000)
	}
}

func TestUniform(t *testing.T) {
	randomness := rand.New(rand.NewSource(rand.Int63()))

	// Verify that the returned value is always in the desired range.
	t.Run("bounds", func(t *testing.T) {
		for i := uint64(1); i < 10000; i++ {
			if got := uniform(randomness, i); got > i {
				t.Errorf("uniform(_, %d) = %d, want < %d", i, got, i)
			}
		}
	})

	// Verify the algorithm will output every number in the range
	t.Run("coverage", func(t *testing.T) {
		bound := uint64(100)
		coverage := make([]bool, bound)
		var count uint64
		for i := 0; i < 1_000_000; i++ {
			sample := uniform(randomness, bound)
			if !coverage[sample] {
				count++
				coverage[sample] = true
			}

			// Check for full coverage.
			if count == bound {
				return
			}
		}
		// Chance of success with correct code is (1 - (1 - 1/bound)^runs)^bound ~= 1 with runs=1e6, bound=100
		t.Errorf("uniform(_, %d) did not cover [0, %d)", bound, bound)
	})
}
