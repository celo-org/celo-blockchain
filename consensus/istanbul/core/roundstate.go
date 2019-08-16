// Copyright 2017 The go-ethereum Authors
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

package core

import (
	"math/big"
	"sync"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/istanbul"
)

// newRoundState creates a new roundState instance with the given view and validatorSet
// lockedHash and preprepare are for round change when lock exists,
// we need to keep a reference of preprepare in order to propose locked proposal when there is a lock and itself is the proposer
func newRoundState(validatorSet istanbul.ValidatorSet, pendingRequest *istanbul.Request, hasBadProposal func(hash common.Hash) bool) *roundState {
	return &roundState{
		mu:             new(sync.RWMutex),
		pendingRequest: pendingRequest,
		hasBadProposal: hasBadProposal,
	}
}

// roundState stores the consensus state
// In HotStuff this is continuously updated.
type roundState struct {
	number         *big.Int
	voteHeight     *big.Int
	Votes          *messageSet
	lockedBlock    *istanbul.Node 	       
	executedBlock  *istanbul.Node
	highestQC      *istanbul.QuorumCertificate
	pendingRequest *istanbul.Request // ???]

	// Map from parent to child as a tree (blocks go from child to parent in one path)
	reverseTree    map[common.Hash]map[common.Hash]bool
	// Mapping from hash to istanbul node. Should be garbage collected `onCommit`
	localBlocks    map[common.Hash]*istanbul.Node

	mu             *sync.RWMutex
	hasBadProposal func(hash common.Hash) bool
}

func (s *roundState) PendingRequest() *istanbul.Request {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.pendingRequest
}

func (s *roundState) HighestQC() *istanbul.QuorumCertificate {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.highestQC
} 


// Gets the node referred to by the QC in a given node
func (s *roundState) QCParentNode(n *istanbul.Node) *istanbul.Node {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.localBlocks[n.QuorumCertificate.BlockHash]
}

func (s *roundState) UpdateHighestQC(qc *istanbul.QuorumCertificate) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var height *big.Int
	if  parent := s.localBlocks[qc.BlockHash]; parent != nil {
		height = parent.Block.Number()
	} else {
		// TODO: no failure here
		return
	}

	if height.Cmp(s.voteHeight) <= 0 {
		// TOOD: panic
		return
	}
	s.voteHeight = height
}

func (s *roundState) BuildNewHighestQC() {
	s.mu.Lock()
	defer s.mu.Unlock()

	// TODO
}

func (s *roundState) SafeNode(proposal *istanbul.Node) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Verifies that it extends the locked node OR quorum cert can override
	if proposal.Block.Number().Cmp(s.voteHeight) <= 0 {
		//return  errAlreadyVotedAtOrAboveProposedBlock
		return errIgnored
	}
	extends := s.ExtendsLockedBlock(proposal)
	var higher bool
	if  parent := s.QCParentNode(proposal); parent != nil {
		higher = parent.Block.Number().Cmp(s.lockedBlock.Block.Number()) > 0
	} else {
		higher = false
	}
	if extends || higher {
		return nil
	}
	// return errNoExtendOrPolka
	return errIgnored
}

func (s *roundState) SetVoteHeight(height *big.Int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if height.Cmp(s.voteHeight) <= 0{
		// TODO: panic
		return
	}
	s.voteHeight = height
}

func (s *roundState) VoteHeight() *big.Int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.voteHeight
}

func (s *roundState) ExtendsLockedBlock(proposal *istanbul.Node) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Follow tree until it finds the locked block or nil. Assuming properly garbage collected.
	for n := proposal; n != nil; n = s.localBlocks[n.Block.ParentHash()] {
		if n.Block.Hash() == s.lockedBlock.Block.Hash() {
			return true
		}
	}
	return false
}

func (s *roundState) SetLockedBlock(block *istanbul.Node) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.lockedBlock = block
}

func (s *roundState) LockedBlock() *istanbul.Node {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.lockedBlock
}

func (s *roundState) Number() *big.Int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.number
}

func (s *roundState) SetNumber(n *big.Int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.number = n
}





// // The DecodeRLP method should read one value from the given
// // Stream. It is not forbidden to read less or more, but it might
// // be confusing.
// func (s *roundState) DecodeRLP(stream *rlp.Stream) error {
// 	var ss struct {
// 		Round          *big.Int
// 		Sequence       *big.Int
// 		Preprepare     *istanbul.Preprepare
// 		Prepares       *messageSet
// 		Commits        *messageSet
// 		lockedHash     common.Hash
// 		pendingRequest *istanbul.Request
// 	}

// 	if err := stream.Decode(&ss); err != nil {
// 		return err
// 	}
// 	s.round = ss.Round
// 	s.sequence = ss.Sequence
// 	s.Preprepare = ss.Preprepare
// 	s.Prepares = ss.Prepares
// 	s.Commits = ss.Commits
// 	s.lockedHash = ss.lockedHash
// 	s.pendingRequest = ss.pendingRequest
// 	s.mu = new(sync.RWMutex)

// 	return nil
// }

// // EncodeRLP should write the RLP encoding of its receiver to w.
// // If the implementation is a pointer method, it may also be
// // called for nil pointers.
// //
// // Implementations should generate valid RLP. The data written is
// // not verified at the moment, but a future version might. It is
// // recommended to write only a single value but writing multiple
// // values or no value at all is also permitted.
// func (s *roundState) EncodeRLP(w io.Writer) error {
// 	s.mu.RLock()
// 	defer s.mu.RUnlock()

// 	return rlp.Encode(w, []interface{}{
// 		s.round,
// 		s.sequence,
// 		s.Preprepare,
// 		s.Prepares,
// 		s.Commits,
// 		s.lockedHash,
// 		s.pendingRequest,
// 	})
// }
