// Copyright 2019 The Celo Authors
// This file is part of the celo library.
//
// The celo library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The celo library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the celo library. If not, see <http://www.gnu.org/licenses/>.

package core

import (
	"time"

	"github.com/ethereum/go-ethereum/consensus/istanbul"
)

func (c *core) sendVote(vote *istanbul.Vote) {
	logger := c.logger.New("func", "sendVote")

	encodedVote, err := Encode(vote)
	if err != nil {
		logger.Error("Failed to encode", "vote", vote)
		return
	}
	c.broadcast(&istanbul.Message{
		Code: istanbul.MsgVote,
		Msg:  encodedVote,
	})
}

func (c *core) broadcastNextProposal() {
	logger := c.logger.New("func", "broadcastNextProposal")

	// TODO: Increment Height somewhere??
	req := c.current.PendingRequest()
	proposal := &istanbul.Node{
		Block: req.Proposal,
		QuorumCertificate: *(c.current.HighestQC()),
	}
	encodedProposal, err := Encode(proposal)
	if err != nil {
		logger.Error("Failed to encode", "proposal", proposal)
		return
	}
	// TODO: ensure that this check doesn't need to be here.
	if c.isProposer() {
		c.broadcast(&istanbul.Message{
			Code: istanbul.MsgVote,
			Msg:  encodedProposal,
		})
	}
}

// Handle a new block
func (c *core) update(block *istanbul.Node) {
	preCommitBlock := c.current.QCParentNode(block)        // b'' (Proposal type)
	commitBlock := c.current.QCParentNode(preCommitBlock)  // b'
	decideBlock := c.current.QCParentNode(commitBlock)     // b

	// PRE-COMMIT phase b''
	c.current.UpdateHighestQC(&block.QuorumCertificate)
	// COMMIT phase on b'
	if preCommitBlock.Block.Number().Cmp(c.current.LockedBlock().Block.Number()) > 0 {
		c.current.SetLockedBlock(commitBlock)
	}
	// DECIDE phase on b
	if preCommitBlock.Block.ParentHash() == commitBlock.Block.Hash() && commitBlock.Block.ParentHash() == decideBlock.Block.Hash() {
		// This is where we send to the full chain.
		// onCommit
		c.commit(decideBlock)
		// Set B_exec
	}

}


func (c *core) handleVote(msg *istanbul.Message) error {
	// Decode VOTE istanbul.Message
	var vote *istanbul.Vote
	err := msg.Decode(&vote)
	if err != nil {
		// return errFailedDecodeVote
		return errInvalidValidatorAddress
	}

	// TODO: Reject based on number earliers on?
	// if err := c.checkMessage(istanbul.MsgView, msg.Number); err != nil {
	// 	return err
	// }

	_, validator := c.valSet.GetByAddress(msg.Address)
	if validator == nil {
		return errInvalidValidatorAddress
	}

	// TODO: check duplicates

	// Valid vote
	c.acceptVote(msg)

	// Send the next proposal (if proposer) with this QC when we have enough votes.
	if  c.isProposer() && c.current.Votes.Size() >= c.valSet.MinQuorumSize() {
		// TODO: aggregate signatures
		c.current.BuildNewHighestQC()
		c.broadcastNextProposal()
	}

	return nil
}


// Collects & aggregates signatues
func (c *core) acceptVote(msg *istanbul.Message) error {
	logger := c.logger.New("from", msg.Address)

	// Add the Vote message to the current round state
	if err := c.current.Votes.Add(msg); err != nil {
		logger.Error("Failed to record vote message", "msg", msg, "err", err)
		return err
	}

	return nil
}

func (c *core) handleProposal(msg *istanbul.Message) error {
	logger := c.logger.New("from", msg.Address, "func", "handleProposal")
	var block *istanbul.Node
	err := msg.Decode(&block)
	if err != nil {
		logger.Error("Failed to decode proposed block and QC.")
		// return errFailedDecodeProposalBlock
		return errNotFromProposer
	}

	// TODO: Reject based on number earliers on?
	if err := c.checkMessage(msg.Number); err != nil {
		return err
	}

	// Check if the message comes from current proposer
	if !c.valSet.IsProposer(msg.Address) {
		logger.Warn("Ignore preprepare messages from non-proposer")
		return errNotFromProposer
	}

	// Verify the proposal we received. TODO: more checking here.
	if duration, err := c.backend.Verify(block.Block); err != nil {
		logger.Warn("Failed to verify proposal", "err", err, "duration", duration)
		// if it's a future block, we will handle it again after the duration
		if err == errFutureMessage {
			// c.stopFuturePreprepareTimer()
			c.futurePreprepareTimer = time.AfterFunc(duration, func() {
				c.sendEvent(backlogEvent{
					msg: msg,
				})
			})
		}
		return err
	}

	// TODO: what happens when update is not a safe node
	c.update(block)

	if err := c.current.SafeNode(block); err != nil {
		return err
	}

	// TODO: generate vote here
	c.sendVote(nil) // TODO: params here

	return nil
}

// TODO(Joshua) Implement, not sure what to do here
func (c *core) handleNewView(msg *istanbul.Message) error {
	return nil
}

