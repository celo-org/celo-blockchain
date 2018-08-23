// Copyright 2018 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

// Package bmt provides a binary merkle tree implementation
package bmt

import (
	"fmt"
	"hash"
	"strings"
	"sync"
	"sync/atomic"
)

/*
Binary Merkle Tree Hash is a hash function over arbitrary datachunks of limited size
It is defined as the root hash of the binary merkle tree built over fixed size segments
of the underlying chunk using any base hash function (e.g keccak 256 SHA3).
Chunk with data shorter than the fixed size are hashed as if they had zero padding

BMT hash is used as the chunk hash function in swarm which in turn is the basis for the
128 branching swarm hash http://swarm-guide.readthedocs.io/en/latest/architecture.html#swarm-hash

The BMT is optimal for providing compact inclusion proofs, i.e. prove that a
segment is a substring of a chunk starting at a particular offset
The size of the underlying segments is fixed to the size of the base hash (called the resolution
of the BMT hash), Using Keccak256 SHA3 hash is 32 bytes, the EVM word size to optimize for on-chain BMT verification
as well as the hash size optimal for inclusion proofs in the merkle tree of the swarm hash.

Two implementations are provided:

* RefHasher is optimized for code simplicity and meant as a reference implementation
  that is simple to understand
* Hasher is optimized for speed taking advantage of concurrency with minimalistic
  control structure to coordinate the concurrent routines
  It implements the following interfaces
	* standard golang hash.Hash
	* SwarmHash
	* io.Writer
	* TODO: SegmentWriter
*/

const (
	// SegmentCount is the maximum number of segments of the underlying chunk
	// Should be equal to max-chunk-data-size / hash-size
	SegmentCount = 128
	// PoolSize is the maximum number of bmt trees used by the hashers, i.e,
	// the maximum number of concurrent BMT hashing operations performed by the same hasher
	PoolSize = 8
)

// BaseHasherFunc is a hash.Hash constructor function used for the base hash of the BMT.
// implemented by Keccak256 SHA3 sha3.NewKeccak256
type BaseHasherFunc func() hash.Hash

// Hasher a reusable hasher for fixed maximum size chunks representing a BMT
// - implements the hash.Hash interface
// - reuses a pool of trees for amortised memory allocation and resource control
// - supports order-agnostic concurrent segment writes (TODO:)
//   as well as sequential read and write
// - the same hasher instance must not be called concurrently on more than one chunk
// - the same hasher instance is synchronously reuseable
// - Sum gives back the tree to the pool and guaranteed to leave
//   the tree and itself in a state reusable for hashing a new chunk
// - generates and verifies segment inclusion proofs (TODO:)
type Hasher struct {
	pool *TreePool // BMT resource pool
	bmt  *tree     // prebuilt BMT resource for flowcontrol and proofs
}

// New creates a reusable Hasher
// implements the hash.Hash interface
// pulls a new tree from a resource pool for hashing each chunk
func New(p *TreePool) *Hasher {
	return &Hasher{
		pool: p,
	}
}

// TreePool provides a pool of trees used as resources by Hasher
// a tree popped from the pool is guaranteed to have clean state
// for hashing a new chunk
type TreePool struct {
	lock         sync.Mutex
	c            chan *tree     // the channel to obtain a resource from the pool
	hasher       BaseHasherFunc // base hasher to use for the BMT levels
	SegmentSize  int            // size of leaf segments, stipulated to be = hash size
	SegmentCount int            // the number of segments on the base level of the BMT
	Capacity     int            // pool capacity, controls concurrency
	Depth        int            // depth of the bmt trees = int(log2(segmentCount))+1
	Datalength   int            // the total length of the data (count * size)
	count        int            // current count of (ever) allocated resources
	zerohashes   [][]byte       // lookup table for predictable padding subtrees for all levels
}

// NewTreePool creates a tree pool with hasher, segment size, segment count and capacity
// on Hasher.getTree it reuses free trees or creates a new one if capacity is not reached
func NewTreePool(hasher BaseHasherFunc, segmentCount, capacity int) *TreePool {
	// initialises the zerohashes lookup table
	depth := calculateDepthFor(segmentCount)
	segmentSize := hasher().Size()
	zerohashes := make([][]byte, depth)
	zeros := make([]byte, segmentSize)
	zerohashes[0] = zeros
	h := hasher()
	for i := 1; i < depth; i++ {
		h.Reset()
		h.Write(zeros)
		h.Write(zeros)
		zeros = h.Sum(nil)
		zerohashes[i] = zeros
	}
	return &TreePool{
		c:            make(chan *tree, capacity),
		hasher:       hasher,
		SegmentSize:  segmentSize,
		SegmentCount: segmentCount,
		Capacity:     capacity,
		Datalength:   segmentCount * segmentSize,
		Depth:        depth,
		zerohashes:   zerohashes,
	}
}

// Drain drains the pool until it has no more than n resources
func (p *TreePool) Drain(n int) {
	p.lock.Lock()
	defer p.lock.Unlock()
	for len(p.c) > n {
		<-p.c
		p.count--
	}
}

// Reserve is blocking until it returns an available tree
// it reuses free trees or creates a new one if size is not reached
// TODO: should use a context here
func (p *TreePool) reserve() *tree {
	p.lock.Lock()
	defer p.lock.Unlock()
	var t *tree
	if p.count == p.Capacity {
		return <-p.c
	}
	select {
	case t = <-p.c:
	default:
		t = newTree(p.SegmentSize, p.Depth)
		p.count++
	}
	return t
}

// release gives back a tree to the pool.
// this tree is guaranteed to be in reusable state
func (p *TreePool) release(t *tree) {
	p.c <- t // can never fail ...
}

// tree is a reusable control structure representing a BMT
// organised in a binary tree
// Hasher uses a TreePool to obtain a tree for each chunk hash
// the tree is 'locked' while not in the pool
type tree struct {
	leaves  []*node     // leaf nodes of the tree, other nodes accessible via parent links
	cur     int         // index of rightmost currently open segment
	offset  int         // offset (cursor position) within currently open segment
	segment []byte      // the rightmost open segment (not complete)
	section []byte      // the rightmost open section (double segment)
	depth   int         // number of levels
	result  chan []byte // result channel
	hash    []byte      // to record the result
	span    []byte      // The span of the data subsumed under the chunk
}

// node is a reuseable segment hasher representing a node in a BMT
type node struct {
	isLeft      bool   // whether it is left side of the parent double segment
	parent      *node  // pointer to parent node in the BMT
	state       int32  // atomic increment impl concurrent boolean toggle
	left, right []byte // this is where the content segment is set
}

// newNode constructs a segment hasher node in the BMT (used by newTree)
func newNode(index int, parent *node) *node {
	return &node{
		parent: parent,
		isLeft: index%2 == 0,
	}
}

// Draw draws the BMT (badly)
func (t *tree) draw(hash []byte) string {
	var left, right []string
	var anc []*node
	for i, n := range t.leaves {
		left = append(left, fmt.Sprintf("%v", hashstr(n.left)))
		if i%2 == 0 {
			anc = append(anc, n.parent)
		}
		right = append(right, fmt.Sprintf("%v", hashstr(n.right)))
	}
	anc = t.leaves
	var hashes [][]string
	for l := 0; len(anc) > 0; l++ {
		var nodes []*node
		hash := []string{""}
		for i, n := range anc {
			hash = append(hash, fmt.Sprintf("%v|%v", hashstr(n.left), hashstr(n.right)))
			if i%2 == 0 && n.parent != nil {
				nodes = append(nodes, n.parent)
			}
		}
		hash = append(hash, "")
		hashes = append(hashes, hash)
		anc = nodes
	}
	hashes = append(hashes, []string{"", fmt.Sprintf("%v", hashstr(hash)), ""})
	total := 60
	del := "                             "
	var rows []string
	for i := len(hashes) - 1; i >= 0; i-- {
		var textlen int
		hash := hashes[i]
		for _, s := range hash {
			textlen += len(s)
		}
		if total < textlen {
			total = textlen + len(hash)
		}
		delsize := (total - textlen) / (len(hash) - 1)
		if delsize > len(del) {
			delsize = len(del)
		}
		row := fmt.Sprintf("%v: %v", len(hashes)-i-1, strings.Join(hash, del[:delsize]))
		rows = append(rows, row)

	}
	rows = append(rows, strings.Join(left, "  "))
	rows = append(rows, strings.Join(right, "  "))
	return strings.Join(rows, "\n") + "\n"
}

// newTree initialises a tree by building up the nodes of a BMT
// - segment size is stipulated to be the size of the hash
func newTree(segmentSize, depth int) *tree {
	n := newNode(0, nil)
	prevlevel := []*node{n}
	// iterate over levels and creates 2^(depth-level) nodes
	count := 2
	for level := depth - 2; level >= 0; level-- {
		nodes := make([]*node, count)
		for i := 0; i < count; i++ {
			parent := prevlevel[i/2]
			nodes[i] = newNode(i, parent)
		}
		prevlevel = nodes
		count *= 2
	}
	// the datanode level is the nodes on the last level
	return &tree{
		leaves:  prevlevel,
		result:  make(chan []byte, 1),
		segment: make([]byte, segmentSize),
		section: make([]byte, 2*segmentSize),
	}
}

// methods needed by hash.Hash

// Size returns the size
func (h *Hasher) Size() int {
	return h.pool.SegmentSize
}

// BlockSize returns the block size
func (h *Hasher) BlockSize() int {
	return h.pool.SegmentSize
}

// Hash hashes the data and the span using the bmt hasher
func Hash(h *Hasher, span, data []byte) []byte {
	h.ResetWithLength(span)
	h.Write(data)
	return h.Sum(nil)
}

// Datalength returns the maximum data size that is hashed by the hasher =
// segment count times segment size
func (h *Hasher) DataLength() int {
	return h.pool.Datalength
}

// Sum returns the hash of the buffer
// hash.Hash interface Sum method appends the byte slice to the underlying
// data before it calculates and returns the hash of the chunk
// caller must make sure Sum is not called concurrently with Write, writeSection
// and WriteSegment (TODO:)
func (h *Hasher) Sum(b []byte) (r []byte) {
	return h.sum(b, true, true)
}

// sum implements Sum taking parameters
// * if the tree is released right away
// * if sequential write is used (can read sections)
func (h *Hasher) sum(b []byte, release, section bool) (r []byte) {
	t := h.bmt
	h.finalise(section)
	if t.offset > 0 { // get the last node (double segment)

		// padding the segment  with zero
		copy(t.segment[t.offset:], h.pool.zerohashes[0])
	}
	if section {
		if t.cur%2 == 1 {
			// if just finished current segment, copy it to the right half of the chunk
			copy(t.section[h.pool.SegmentSize:], t.segment)
		} else {
			// copy segment to front of section, zero pad the right half
			copy(t.section, t.segment)
			copy(t.section[h.pool.SegmentSize:], h.pool.zerohashes[0])
		}
		h.writeSection(t.cur, t.section)
	} else {
		// TODO: h.writeSegment(t.cur, t.segment)
		panic("SegmentWriter not implemented")
	}
	bmtHash := <-t.result
	span := t.span

	if release {
		h.releaseTree()
	}
	// sha3(span + BMT(pure_chunk))
	if span == nil {
		return bmtHash
	}
	bh := h.pool.hasher()
	bh.Reset()
	bh.Write(span)
	bh.Write(bmtHash)
	return bh.Sum(b)
}

// Hasher implements the SwarmHash interface

// Hasher implements the io.Writer interface

// Write fills the buffer to hash,
// with every full segment calls writeSection
func (h *Hasher) Write(b []byte) (int, error) {
	l := len(b)
	if l <= 0 {
		return 0, nil
	}
	t := h.bmt
	need := (h.pool.SegmentCount - t.cur) * h.pool.SegmentSize
	if l < need {
		need = l
	}
	// calculate missing bit to complete current open segment
	rest := h.pool.SegmentSize - t.offset
	if need < rest {
		rest = need
	}
	copy(t.segment[t.offset:], b[:rest])
	need -= rest
	size := (t.offset + rest) % h.pool.SegmentSize
	// read full segments and the last possibly partial segment
	for need > 0 {
		// push all finished chunks we read
		if t.cur%2 == 0 {
			copy(t.section, t.segment)
		} else {
			copy(t.section[h.pool.SegmentSize:], t.segment)
			h.writeSection(t.cur, t.section)
		}
		size = h.pool.SegmentSize
		if need < size {
			size = need
		}
		copy(t.segment, b[rest:rest+size])
		need -= size
		rest += size
		t.cur++
	}
	t.offset = size % h.pool.SegmentSize
	return l, nil
}

// Reset needs to be called before writing to the hasher
func (h *Hasher) Reset() {
	h.getTree()
}

// Hasher implements the SwarmHash interface

// ResetWithLength needs to be called before writing to the hasher
// the argument is supposed to be the byte slice binary representation of
// the length of the data subsumed under the hash, i.e., span
func (h *Hasher) ResetWithLength(span []byte) {
	h.Reset()
	h.bmt.span = span
}

// releaseTree gives back the Tree to the pool whereby it unlocks
// it resets tree, segment and index
func (h *Hasher) releaseTree() {
	t := h.bmt
	if t != nil {
		t.cur = 0
		t.offset = 0
		t.span = nil
		t.hash = nil
		h.bmt = nil
		h.pool.release(t)
	}
}

// TODO: writeSegment writes the ith segment into the BMT tree
// func (h *Hasher) writeSegment(i int, s []byte) {
// 	go h.run(h.bmt.leaves[i/2], h.pool.hasher(), i%2 == 0, s)
// }

// writeSection writes the hash of i/2-th segction into right level 1 node of the BMT tree
func (h *Hasher) writeSection(i int, section []byte) {
	n := h.bmt.leaves[i/2]
	isLeft := n.isLeft
	n = n.parent
	bh := h.pool.hasher()
	bh.Write(section)
	go func() {
		sum := bh.Sum(nil)
		if n == nil {
			h.bmt.result <- sum
			return
		}
		h.run(n, bh, isLeft, sum)
	}()
}

// run pushes the data to the node
// if it is the first of 2 sisters written the routine returns
// if it is the second, it calculates the hash and writes it
// to the parent node recursively
func (h *Hasher) run(n *node, bh hash.Hash, isLeft bool, s []byte) {
	for {
		if isLeft {
			n.left = s
		} else {
			n.right = s
		}
		// the child-thread first arriving will quit
		if n.toggle() {
			return
		}
		// the second thread now can be sure both left and right children are written
		// it calculates the hash of left|right and take it to the next level
		bh.Reset()
		bh.Write(n.left)
		bh.Write(n.right)
		s = bh.Sum(nil)

		// at the root of the bmt just write the result to the result channel
		if n.parent == nil {
			h.bmt.result <- s
			return
		}

		// otherwise iterate on parent
		isLeft = n.isLeft
		n = n.parent
	}
}

// finalise is following the path starting from the final datasegment to the
// BMT root via parents
// for unbalanced trees it fills in the missing right sister nodes using
// the pool's lookup table for BMT subtree root hashes for all-zero sections
func (h *Hasher) finalise(skip bool) {
	t := h.bmt
	isLeft := t.cur%2 == 0
	n := t.leaves[t.cur/2]
	for level := 0; n != nil; level++ {
		// when the final segment's path is going via left child node
		// we include an all-zero subtree hash for the right level and toggle the node.
		// when the path is going through right child node, nothing to do
		if isLeft && !skip {
			n.right = h.pool.zerohashes[level]
			n.toggle()
		}
		skip = false
		isLeft = n.isLeft
		n = n.parent
	}
}

// getTree obtains a BMT resource by reserving one from the pool
func (h *Hasher) getTree() *tree {
	if h.bmt != nil {
		return h.bmt
	}
	t := h.pool.reserve()
	h.bmt = t
	return t
}

// atomic bool toggle implementing a concurrent reusable 2-state object
// atomic addint with %2 implements atomic bool toggle
// it returns true if the toggler just put it in the active/waiting state
func (n *node) toggle() bool {
	return atomic.AddInt32(&n.state, 1)%2 == 1
}

func hashstr(b []byte) string {
	end := len(b)
	if end > 4 {
		end = 4
	}
	return fmt.Sprintf("%x", b[:end])
}

// calculateDepthFor calculates the depth (number of levels) in the BMT tree
func calculateDepthFor(n int) (d int) {
	c := 2
	for ; c < n; c *= 2 {
		d++
	}
	return d + 1
}
