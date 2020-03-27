// Package random implements a language independent method of producing random validator orderings.
// It is designed to use standard methods that could be implemented in any language or platform.
package random

import (
	"encoding/binary"
	"io"

	"github.com/ethereum/go-ethereum/common"
	"golang.org/x/crypto/sha3"
)

// Permutation produces an array with a random permutation of [0, 1, ... n-1]
// Based on the Fisher-Yates method https://en.wikipedia.org/wiki/Fisher%E2%80%93Yates_shuffle
func Permutation(seed common.Hash, n int) []int {
	if n <= 0 {
		return nil
	}

	// Create the unshuffled array [0, 1, ... n-1]
	array := make([]int, n)
	for i := 0; i < n; i++ {
		array[i] = i
	}

	// Create the Shake256 pseudo random stream.
	randomness := sha3.NewShake256()
	_, err := randomness.Write(seed[:])
	if err != nil {
		// ShakeHash never returns an error.
		panic(err)
	}

	// Shuffle the array using the Fisher-Yates method.
	for i := 0; i < n-1; i++ {
		j := i + int(uniform(randomness.(io.Reader), uint64(n-i))) // j in [i, n)
		array[i], array[j] = array[j], array[i]
	}
	return array
}

// compress produces a 64-bit random value from a byte stream.
func randUint64(randomness io.Reader) uint64 {
	raw := make([]byte, 8)
	_, err := randomness.Read(raw)
	if err != nil {
		// Random stream should never return an error.
		panic(err)
	}
	return binary.BigEndian.Uint64(raw)
}

// uniform produces an integer in the range [0, k) from the provided randomness.
// Based on Algorithm 4 of https://arxiv.org/pdf/1805.10941.pdf
func uniform(randomness io.Reader, k uint64) uint64 {
	x := randUint64(randomness)
	r := x % k
	for x-r > (-k) {
		x = randUint64(randomness)
		r = x % k
	}
	return r
}
