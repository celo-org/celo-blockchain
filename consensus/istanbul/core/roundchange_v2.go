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
	"errors"

	"github.com/celo-org/celo-blockchain/consensus/istanbul"
)

func (c *core) handleRoundChangeV2(msg *istanbul.Message) error {
	logger := c.newLogger("func", "handleRoundChangeV2", "tag", "handleMsg", "from", msg.Address)

	rc := msg.RoundChangeV2()

	// Check consensus fork
	if !c.isConsensusFork(rc.Request.View.Sequence) {
		logger.Info("Received RoundChangeV2 for unforked block sequence", "sequence", rc.Request.View.Sequence.Uint64())
		return errors.New("Received RoundChangeV2 for not forked block")
	}

	// Check signature of the internal Request
	if err := istanbul.CheckSignedBy(&rc.Request, rc.Request.Signature,
		rc.Request.Address, errInvalidRoundChangeRequestSignature, c.validateFn); err != nil {
		return err
	}
	// Check message address and request address is the same
	if msg.Address != rc.Request.Address {
		return errRoundChangeRequestAddressMismatch
	}
	logger = logger.New("msg_round", rc.Request.View.Round, "msg_seq", rc.Request.View.Sequence)

	// Must be same sequence and future round.
	err := c.checkMessage(istanbul.MsgRoundChange, &rc.Request.View)

	// If the RC message is for the current sequence but a prior round, help the sender fast forward
	// by sending back to it (not broadcasting) a round change message for our desired round.
	if err == errOldMessage && rc.Request.View.Sequence.Cmp(c.current.Sequence()) == 0 {
		logger.Trace("Sending round change for desired round to node with a previous desired round", "msg_round", rc.Request.View.Round)
		c.sendRoundChangeAgain(msg.Address)
		return nil
	} else if err != nil {
		logger.Debug("Check round change message failed", "err", err)
		return err
	}
	// Verify that it has a proposal only if and only if a prepared certificate is available
	if !rc.ProposalMatch() {
		return errRoundChangeProposalHashMismatch
	}
	// Verify the PREPARED certificate if present.
	if rc.HasPreparedCertificate() {
		preparedView, err := c.verifyPCV2WithProposal(rc.Request.PreparedCertificateV2, rc.PreparedProposal)
		if err != nil {
			return err
		} else if preparedView == nil || preparedView.Round.Cmp(rc.Request.View.Round) > 0 {
			return errInvalidRoundChangeViewMismatch
		}
	}

	roundView := rc.Request.View

	// Add the ROUND CHANGE message to its message set.
	if err := c.roundChangeSetV2.Add(roundView.Round, msg); err != nil {
		logger.Warn("Failed to add round change message", "roundView", roundView, "err", err)
		return err
	}

	// Skip to the highest round we know F+1 (one honest validator) is at, but
	// don't start a round until we have a quorum who want to start a given round.
	ffRound := c.roundChangeSetV2.MaxRound(c.current.ValidatorSet().F() + 1)
	quorumRound := c.roundChangeSetV2.MaxOnOneRound(c.current.ValidatorSet().MinQuorumSize())
	logger = logger.New("ffRound", ffRound, "quorumRound", quorumRound)
	logger.Trace("Got round change message", "rcs", c.roundChangeSetV2.String())
	// On f+1 round changes we send a round change and wait for the next round if we haven't done so already
	// On quorum round change messages we go to the next round immediately.
	if quorumRound != nil && quorumRound.Cmp(c.current.DesiredRound()) >= 0 {
		logger.Debug("Got quorum round change messages, starting new round.")
		return c.startNewRound(quorumRound)
	} else if ffRound != nil {
		logger.Debug("Got f+1 round change messages, sending own round change message and waiting for next round.")
		c.waitForDesiredRound(ffRound)
	}

	return nil
}

// ----------------------------------------------------------------------------

// CurrentRoundChangeSet returns the current round change set summary.
func (c *core) CurrentRoundChangeSetV2() *RoundChangeSetSummary {
	rcs := c.roundChangeSetV2
	if rcs != nil {
		return rcs.Summary()
	}
	return nil
}
