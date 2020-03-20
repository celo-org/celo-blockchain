// Package random inplements a langauge independent method of producing random validator orderings.
// It is designed to use standard methods that could be implemented in any language or platform.
package random

import (
	"encoding/binary"

	"github.com/ethereum/go-ethereum/common"
	"golang.org/x/crypto/sha3"
)

const (
	// Arbitrary locally unique salts to prevent collisions.
	uniformSalt     = 0xe0
	permutationSalt = 0xe1
)

// Permutation produces an array with a random permutation of [0, 1, ... n-1]
// Based on the Fisher-Yates method https://en.wikipedia.org/wiki/Fisher%E2%80%93Yates_shuffle
func Permutation(randomness common.Hash, n int) []int {
	if n <= 0 {
		return nil
	}

	// Create the unshuffled array [0, 1, ... n-1]
	array := make([]int, n)
	for i := 0; i < n; i++ {
		array[i] = i
	}

	// Shuffle the array using the Fisher-Yates method.
	for i := 0; i < n-1; i++ {
		randomness = sha3.Sum256(append(randomness[:], permutationSalt))
		j := i + int(uniform(randomness, uint64(n-i))) // j in [i, n)
		array[i], array[j] = array[j], array[i]
	}
	return array
}

// compress produces a 64-bit random value from a 256-bit random value.
func compress(value common.Hash) uint64 {
	var compressed uint64 = 0
	for i := 0; i < common.HashLength; i += 8 {
		compressed ^= binary.BigEndian.Uint64(value[i : i+8])
	}
	return compressed
}

// uniform produces an integer in the range [0, k) from the provided randomness.
// Based on Algorithm 4 of https://arxiv.org/pdf/1805.10941.pdf
func uniform(randomness common.Hash, k uint64) uint64 {
	x := compress(randomness)
	r := x % k
	for x-r > (-k) {
		randomness = sha3.Sum256(append(randomness[:], uniformSalt))
		x = compress(randomness)
		r = x % k
	}
	return r
}
