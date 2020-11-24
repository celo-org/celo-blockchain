// Written in 2012 by Dmitry Chestnykh.
//
// To the extent possible under law, the author have dedicated all copyright
// and related and neighboring rights to this software to the public domain
// worldwide. This software is distributed without any warranty.
// http://creativecommons.org/publicdomain/zero/1.0/

// Package blake2s implements BLAKE2s cryptographic hash function.
package blake2s

import (
	"encoding/binary"
	"errors"
	"hash"
)

const (
	BlockSize  = 64 // block size of algorithm
	Size       = 32 // maximum digest size
	SaltSize   = 8  // maximum salt size
	PersonSize = 8  // maximum personalization string size
	KeySize    = 32 // maximum size of key
)

type digest struct {
	h  [8]uint32       // current chain value
	t  [2]uint32       // message bytes counter
	f  [2]uint32       // finalization flags
	x  [BlockSize]byte // buffer for data not yet compressed
	nx int             // number of bytes in buffer

	ih         [8]uint32       // initial chain value (after config)
	paddedKey  [BlockSize]byte // copy of key, padded with zeros
	isKeyed    bool            // indicates whether hash was keyed
	size       uint8           // digest size in bytes
	isLastNode bool            // indicates processing of the last node in tree hashing
}

// Initialization values.
var iv = [8]uint32{
	0x6a09e667, 0xbb67ae85, 0x3c6ef372, 0xa54ff53a,
	0x510e527f, 0x9b05688c, 0x1f83d9ab, 0x5be0cd19,
}

// Config is used to configure hash function parameters and keying.
// All parameters are optional.
type Config struct {
	Size   uint8  // digest size (if zero, default size of 32 bytes is used)
	Key    []byte // key for prefix-MAC
	Salt   []byte // salt (if < 8 bytes, padded with zeros)
	Person []byte // personalization (if < 8 bytes, padded with zeros)
	Tree   *Tree  // parameters for tree hashing
}

// Tree represents parameters for tree hashing.
type Tree struct {
	Fanout        uint8  // fanout
	MaxDepth      uint8  // maximal depth
	LeafSize      uint32 // leaf maximal byte length (0 for unlimited)
	NodeOffset    uint64 // node offset (0 for first, leftmost or leaf), max 2⁴⁸-1
	NodeDepth     uint8  // node depth (0 for leaves)
	InnerHashSize uint8  // inner hash byte length
	IsLastNode    bool   // indicates processing of the last node of layer
}

var defaultConfig = &Config{Size: Size}

func verifyConfig(c *Config) error {
	if c.Size > Size {
		return errors.New("digest size is too large")
	}
	if len(c.Key) > KeySize {
		return errors.New("key is too large")
	}
	if len(c.Salt) > SaltSize {
		// Smaller salt is okay: it will be padded with zeros.
		return errors.New("salt is too large")
	}
	if len(c.Person) > PersonSize {
		// Smaller personalization is okay: it will be padded with zeros.
		return errors.New("personalization is too large")
	}
	if c.Tree != nil {
		if c.Tree.InnerHashSize > Size {
			return errors.New("incorrect tree inner hash size")
		}
		if c.Tree.NodeOffset > (1<<48)-1 {
			return errors.New("tree node offset is too large")
		}
	}
	return nil
}

// New returns a new hash.Hash configured with the given Config.
// Config can be nil, in which case the default one is used, calculating 32-byte digest.
// Returns non-nil error if Config contains invalid parameters.
func New(c *Config) (hash.Hash, error) {
	if c == nil {
		c = defaultConfig
	} else {
		if c.Size == 0 {
			// Set default size if it's zero.
			c.Size = Size
		}
		if err := verifyConfig(c); err != nil {
			return nil, err
		}
	}
	d := new(digest)
	d.initialize(c)
	return d, nil
}

func (d *digest) initialize(c *Config) {
	// Create parameter block.
	var p [BlockSize]byte
	p[0] = c.Size
	p[1] = uint8(len(c.Key))
	if c.Salt != nil {
		copy(p[16:], c.Salt)
	}
	if c.Person != nil {
		copy(p[24:], c.Person)
	}
	if c.Tree != nil {
		p[2] = c.Tree.Fanout
		p[3] = c.Tree.MaxDepth
		binary.LittleEndian.PutUint32(p[4:], c.Tree.LeafSize)
		p[8] = byte(c.Tree.NodeOffset)
		p[9] = byte(c.Tree.NodeOffset >> 8)
		p[10] = byte(c.Tree.NodeOffset >> 16)
		p[11] = byte(c.Tree.NodeOffset >> 24)
		p[12] = byte(c.Tree.NodeOffset >> 32)
		p[13] = byte(c.Tree.NodeOffset >> 40)
		p[14] = c.Tree.NodeDepth
		p[15] = c.Tree.InnerHashSize
	} else {
		p[2] = 1
		p[3] = 1
	}
	// Initialize.
	d.size = c.Size
	for i := 0; i < 8; i++ {
		d.h[i] = iv[i] ^ binary.LittleEndian.Uint32(p[i*4:])
	}
	if c.Tree != nil && c.Tree.IsLastNode {
		d.isLastNode = true
	}
	// Process key.
	if len(c.Key) > 0 {
		copy(d.paddedKey[:], c.Key)
		d.Write(d.paddedKey[:])
		d.isKeyed = true
	}
	// Save a copy of initialized state.
	copy(d.ih[:], d.h[:])
}

// New256 returns a new hash.Hash computing the BLAKE2s 32-byte checksum.
func New256() hash.Hash {
	d := new(digest)
	d.initialize(defaultConfig)
	return d
}

// NewMAC returns a new hash.Hash computing BLAKE2s prefix-
// Message Authentication Code of the given size in bytes
// (up to 32) with the given key (up to 32 bytes in length).
func NewMAC(outBytes uint8, key []byte) hash.Hash {
	d, err := New(&Config{Size: outBytes, Key: key})
	if err != nil {
		panic(err.Error())
	}
	return d
}

// Reset resets the state of digest to the initial state
// after configuration and keying.
func (d *digest) Reset() {
	copy(d.h[:], d.ih[:])
	d.t[0] = 0
	d.t[1] = 0
	d.f[0] = 0
	d.f[1] = 0
	d.nx = 0
	if d.isKeyed {
		d.Write(d.paddedKey[:])
	}
}

// Size returns the digest size in bytes.
func (d *digest) Size() int { return int(d.size) }

// BlockSize returns the algorithm block size in bytes.
func (d *digest) BlockSize() int { return BlockSize }

func (d *digest) Write(p []byte) (nn int, err error) {
	nn = len(p)
	left := BlockSize - d.nx
	if len(p) > left {
		// Process buffer.
		copy(d.x[d.nx:], p[:left])
		p = p[left:]
		blocks(d, d.x[:])
		d.nx = 0
	}
	// Process full blocks except for the last one.
	if len(p) > BlockSize {
		n := len(p) &^ (BlockSize - 1)
		if n == len(p) {
			n -= BlockSize
		}
		blocks(d, p[:n])
		p = p[n:]
	}
	// Fill buffer.
	d.nx += copy(d.x[d.nx:], p)
	return
}

// Sum returns the calculated checksum.
func (d0 *digest) Sum(in []byte) []byte {
	// Make a copy of d0 so that caller can keep writing and summing.
	d := *d0
	hash := d.checkSum()
	return append(in, hash[:d.size]...)
}

func (d *digest) checkSum() [Size]byte {
	// Do not create unnecessary copies of the key.
	if d.isKeyed {
		for i := 0; i < len(d.paddedKey); i++ {
			d.paddedKey[i] = 0
		}
	}

	dec := BlockSize - uint32(d.nx)
	if d.t[0] < dec {
		d.t[1]--
	}
	d.t[0] -= dec

	// Pad buffer with zeros.
	for i := d.nx; i < len(d.x); i++ {
		d.x[i] = 0
	}
	// Set last block flag.
	d.f[0] = 0xffffffff
	if d.isLastNode {
		d.f[1] = 0xffffffff
	}
	// Compress last block.
	blocks(d, d.x[:])

	var out [Size]byte
	j := 0
	for _, s := range d.h[:(d.size-1)/4+1] {
		out[j+0] = byte(s >> 0)
		out[j+1] = byte(s >> 8)
		out[j+2] = byte(s >> 16)
		out[j+3] = byte(s >> 24)
		j += 4
	}
	return out
}

// Sum256 returns a 32-byte BLAKE2s hash of data.
func Sum256(data []byte) [Size]byte {
	var d digest
	d.initialize(defaultConfig)
	d.Write(data)
	return d.checkSum()
}
