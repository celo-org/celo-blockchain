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
	"fmt"
	"time"

	"github.com/celo-org/celo-blockchain/consensus"
	"github.com/celo-org/celo-blockchain/consensus/istanbul"
	"github.com/celo-org/celo-blockchain/consensus/istanbul/algorithm"
)

func (c *core) sendPreprepare(request *istanbul.Request, roundChangeCertificate istanbul.RoundChangeCertificate) {
	logger := c.newLogger("func", "sendPreprepare")

	// If I'm the proposer and I have the same sequence with the proposal
	if c.current.Sequence().Cmp(request.Proposal.Number()) == 0 && c.isProposer() {
		m := istanbul.NewPreprepareMessage(&istanbul.Preprepare{
			View:                   c.current.View(),
			Proposal:               request.Proposal,
			RoundChangeCertificate: roundChangeCertificate,
		}, c.address)
		logger.Debug("Sending preprepare", "m", m)
		c.broadcast(m)
	}
}

func (c *core) handlePreprepare(msg *istanbul.Message) error {
	defer c.handlePrePrepareTimer.UpdateSince(time.Now())

	preprepare := msg.Preprepare()
	logger := c.newLogger().New("func", "handlePreprepare", "tag", "handleMsg", "from", msg.Address, "msg_num", preprepare.Proposal.Number(),
		"msg_hash", preprepare.Proposal.Hash(), "msg_seq", preprepare.View.Sequence, "msg_round", preprepare.View.Round)
	// Set the rcc
	var rccID *algorithm.Value
	var err error
	rccID, err = c.algo.O.(*RoundStateOracle).SetRoundChangeCertificate(preprepare)
	if err != nil {
		return fmt.Errorf("failed to set round change certificate in oracle: %w", err)
	}
	if preprepare.View.Round.Uint64() > 0 {
		logger.Trace("Trying to move to round change certificate's round", "target round", preprepare.View.Round)
		err := c.startNewRound(preprepare.View.Round)
		if err != nil {
			logger.Warn("Failed to move to new round", "err", err)
			return err
		}
	}
	m, _ := c.algo.HandleMessage(&algorithm.Msg{
		MsgType:         algorithm.Type(msg.Code),
		Height:          preprepare.View.Sequence.Uint64(),
		Round:           preprepare.View.Round.Uint64(),
		Val:             algorithm.Value(preprepare.Proposal.Hash()),
		RoundChangeCert: rccID,
	})
	if m != nil && m.MsgType == algorithm.Prepare {
		// Verify the proposal we received
		if duration, err := c.verifyProposal(preprepare.Proposal); err != nil {
			logger.Warn("Failed to verify proposal", "err", err, "duration", duration)
			// if it's a future block, we will handle it again after the duration
			if err == consensus.ErrFutureBlock {
				c.stopFuturePreprepareTimer()
				c.futurePreprepareTimer = time.AfterFunc(duration, func() {
					c.sendEvent(backlogEvent{
						msg: msg,
					})
				})
			}
			return err
		}

		if c.current.State() == StateAcceptRequest {
			logger.Trace("Accepted preprepare", "tag", "stateTransition")
			c.consensusTimestamp = time.Now()

			err := c.current.TransitionToPreprepared(preprepare)
			if err != nil {
				return err
			}

			// Process Backlog Messages
			c.backlog.updateState(c.current.View(), c.current.State())
			c.sendPrepare()
		}
	}

	return nil
}
