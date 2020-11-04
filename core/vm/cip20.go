package vm

import (
	"errors"

	"github.com/dchest/blake2s"
	"github.com/dchest/blake2xs"
	"github.com/ethereum/go-ethereum/params"
	"golang.org/x/crypto/sha3"
)

const (
	blake2sConfigLen  = 32
	blake2xsConfigLen = 33
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
	0x11: &Blake2Xs{},
}

func debitCip20Gas(p Cip20Hash, input []byte, gas uint64) (uint64, error) {
	requiredGas := p.RequiredGas(input)
	if requiredGas > gas {
		return gas, ErrOutOfGas
	}
	return gas - requiredGas, nil
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
	output := hasher.Sum(input)
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
	output := hasher.Sum(input)
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
	output := hasher.Sum(input)
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
	h, err := blake2s.New(config)
	if err != nil {
		return nil, err
	}

	body := input[blake2sConfigLen+len(config.Key):]
	digest := h.Sum(body)
	return digest[:], nil
}

func unmarshalBlake2sConfig(input []byte) (*blake2s.Config, error) {

	if len(input) < blake2sConfigLen {
		return nil, errors.New("Blake2s unmarshalling error. Received fewer than 32 bytes")
	}

	c := &blake2s.Config{}
	c.Size = uint8(input[0])
	keySize := uint8(input[1])
	c.Tree.Fanout = uint8(input[2])
	c.Tree.MaxDepth = uint8(input[3])

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

	c.Key = input[32 : 32+keySize]

	return c, nil
}

// The Blake2Xs hash function
type Blake2Xs struct{}

// RequiredGas for Blake2Xs
func (c *Blake2Xs) RequiredGas(input []byte) uint64 {
	return 0 // TODO: James to benchmark
}

// Run function for Blake2Xs
func (c *Blake2Xs) Run(input []byte) ([]byte, error) {
	var desired uint32
	desired |= uint32(input[0]) << 24
	desired |= uint32(input[1]) << 16
	desired |= uint32(input[2]) << 8
	desired |= uint32(input[3]) << 0
	config, err := unmarshalBlake2xsConfig(input[4:])
	if err != nil {
		return nil, err
	}

	xof, err := blake2xs.NewXOF(config)
	if err != nil {
		return nil, err
	}

	output := make([]byte, desired)
	written, err := xof.Read(output)
	if err != nil {
		return nil, err
	}

	return output[:written], nil
}

// this is exactly the same as previous, but with 1 extra byte, as digest
// length is u16
func unmarshalBlake2xsConfig(input []byte) (*blake2xs.Config, error) {

	if len(input) < blake2xsConfigLen {
		return nil, errors.New("Blake2xs unmarshalling error. Received fewer than 32 bytes")
	}

	c := &blake2xs.Config{}
	c.Size |= uint16(input[0]) << 0
	c.Size |= uint16(input[1]) << 8

	keySize := uint8(input[2])
	c.Tree.Fanout = uint8(input[3])
	c.Tree.MaxDepth = uint8(input[4])

	c.Tree.LeafSize |= uint32(input[5]) << 0
	c.Tree.LeafSize |= uint32(input[6]) << 8
	c.Tree.LeafSize |= uint32(input[7]) << 16
	c.Tree.LeafSize |= uint32(input[8]) << 24

	for i := 0; i < 6; i++ {
		c.Tree.NodeOffset |= uint64(input[i+9]) << (i * 8)
	}

	c.Tree.NodeDepth = input[15]
	c.Tree.InnerHashSize = input[16]
	c.Salt = input[17:25]
	c.Person = input[25:33]

	if len(input) < blake2xsConfigLen+int(keySize) {
		return nil, errors.New("Blake2xs unmarshalling error. Too few bytes to unmarshal Key")
	}

	c.Key = input[33 : 33+keySize]

	return c, nil
}
