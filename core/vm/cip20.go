package vm

import (
	"errors"

	"github.com/dchest/blake2s"
	"github.com/ethereum/go-ethereum/params"
	"golang.org/x/crypto/sha3"
)

const (
	blake2sConfigLen = 32
)

// Cip20Hash is an interface for CIP20 hash functions. it is a trimmed down
// version of the Precompile interface.
type Cip20Hash interface {
	RequiredGas(input []byte) uint64  // RequiredGas calculates the contract gas use
	Run(input []byte) ([]byte, error) // Run runs the precompiled contract
}

// Cip20HashesDonut is the set of hashes active in the Donut hard fork.
var Cip20HashesDonut = map[uint8]Cip20Hash{
	0:    &Sha3_256{},
	1:    &Sha3_512{},
	2:    &Keccak512{},
	0x10: &Blake2s{},
}

// The Sha3_256 hash function
type Sha3_256 struct{}

// RequiredGas for Sha3_256
func (c *Sha3_256) RequiredGas(input []byte) uint64 {
	words := uint64(len(input) / 64)
	return params.Sha3_256BaseGas + (words * params.Sha3_256PerWordGas)
}

// Run function for Sha3_256
func (c *Sha3_256) Run(input []byte) ([]byte, error) {
	hasher := sha3.New256()
	hasher.Write(input)
	output := hasher.Sum(nil)
	return output, nil
}

// The Sha3_512 hash function
type Sha3_512 struct{}

// RequiredGas for Sha3_512
func (c *Sha3_512) RequiredGas(input []byte) uint64 {
	words := uint64(len(input) / 64)
	return params.Sha3_512BaseGas + (words * params.Sha3_512PerWordGas)
}

// Run function for Sha3_512
func (c *Sha3_512) Run(input []byte) ([]byte, error) {
	hasher := sha3.New512()
	hasher.Write(input)
	output := hasher.Sum(nil)
	return output, nil
}

// The Keccak512 hash function
type Keccak512 struct{}

// RequiredGas for Keccak512
func (c *Keccak512) RequiredGas(input []byte) uint64 {
	words := uint64(len(input) / 64)
	return params.Keccak512BaseGas + (words * params.Keccak512PerWordGas)
}

// Run function for Keccak512
func (c *Keccak512) Run(input []byte) ([]byte, error) {
	hasher := sha3.NewLegacyKeccak512()
	hasher.Write(input)
	output := hasher.Sum(nil)
	return output, nil
}

// The Blake2s hash function
type Blake2s struct{}

// RequiredGas for Blake2s
func (c *Blake2s) RequiredGas(input []byte) uint64 {
	return 0 // TODO: James to benchmark
}

// Run function for Blake2s
func (c *Blake2s) Run(input []byte) ([]byte, error) {
	config, err := unmarshalBlake2sConfig(input)
	if err != nil {
		return nil, err
	}
	h, err := blake2s.New(config)
	if err != nil {
		return nil, err
	}

	preimage := input[blake2sConfigLen+len(config.Key):]
	h.Write(preimage)
	digest := h.Sum(nil)
	return digest[:], nil
}

// The blake2s config is a 32-byte block that is XORed with the IV. It is
// ocumented in the blake2 specification. The key is added to the state after it
// is initialized with the config, and thus is technically not part of the
// config, however, the underlying library requires the key with the config.
//
// NB: numbers longer than 1 byte are LE.
func unmarshalBlake2sConfig(input []byte) (*blake2s.Config, error) {
	if len(input) < blake2sConfigLen {
		return nil, errors.New("Blake2s unmarshalling error. Received fewer than 32 bytes")
	}

	c := &blake2s.Config{
		Tree: &blake2s.Tree{},
	}
	c.Size = input[0]
	keySize := input[1]

	if keySize > 32 {
		return nil, errors.New("Blake2s unmarshalling error. Key size must be 32 bytes or fewer")
	}

	c.Tree.Fanout = input[2]
	c.Tree.MaxDepth = input[3]

	c.Tree.LeafSize |= uint32(input[4]) << 0
	c.Tree.LeafSize |= uint32(input[5]) << 8
	c.Tree.LeafSize |= uint32(input[6]) << 16
	c.Tree.LeafSize |= uint32(input[7]) << 24

	for i := 0; i < 6; i++ {
		c.Tree.NodeOffset |= uint64(input[i+8]) << (i * 8)
	}

	c.Tree.NodeDepth = input[14]
	c.Tree.InnerHashSize = input[15]
	c.Salt = input[16:24]
	c.Person = input[24:32]

	if len(input) < blake2sConfigLen+int(keySize) {
		return nil, errors.New("Blake2s unmarshalling error. Too few bytes to unmarshal Key")
	}

	c.Key = input[blake2sConfigLen : blake2sConfigLen+keySize]

	return c, nil
}
