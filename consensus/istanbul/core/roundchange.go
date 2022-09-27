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
	"math/big"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/consensus/istanbul"
)

// buildRoundChangeMsg creates a round change msg for the given round
func (c *core) buildRoundChangeMsgV1(round *big.Int) *istanbul.Message {
	nextView := &istanbul.View{
		Round:    new(big.Int).Set(round),
		Sequence: new(big.Int).Set(c.current.View().Sequence),
	}
	return istanbul.NewRoundChangeMessage(&istanbul.RoundChange{
		View:                nextView,
		PreparedCertificate: c.current.PreparedCertificate(),
	}, c.address)
}

func (c *core) handleRoundChangeCertificate(proposal istanbul.Subject, roundChangeCertificate istanbul.RoundChangeCertificate) error {
	logger := c.newLogger("func", "handleRoundChangeCertificate", "proposal_round", proposal.View.Round, "proposal_seq", proposal.View.Sequence, "proposal_digest", proposal.Digest.String())

	if len(roundChangeCertificate.RoundChangeMessages) > c.current.ValidatorSet().Size() || len(roundChangeCertificate.RoundChangeMessages) < c.current.ValidatorSet().MinQuorumSize() {
		return errInvalidRoundChangeCertificateNumMsgs
	}

	maxRound := big.NewInt(-1)
	preferredDigest := common.Hash{}
	seen := make(map[common.Address]bool)
	decodedMessages := make([]istanbul.RoundChange, len(roundChangeCertificate.RoundChangeMessages))
	for i := range roundChangeCertificate.RoundChangeMessages {
		// use a different variable each time since we'll store a pointer to the variable
		message := roundChangeCertificate.RoundChangeMessages[i]

		// Verify message signed by a validator
		if err := istanbul.CheckSignedBy(&message, message.Signature,
			message.Address, errInvalidRoundChangeCertificateMsgSignature, c.validateFn); err != nil {
			return err
		}

		// Check for duplicate ROUND CHANGE messages
		if seen[message.Address] {
			return errInvalidRoundChangeCertificateDuplicate
		}
		seen[message.Address] = true

		// Check that the message is a ROUND CHANGE message
		if istanbul.MsgRoundChange != message.Code {
			return errInvalidRoundChangeCertificateMsgCode
		}

		roundChange := message.RoundChange()
		if roundChange.View == nil || roundChange.View.Sequence == nil || roundChange.View.Round == nil {
			return errInvalidRoundChangeCertificateMsgView
		}

		msgLogger := logger.New("msg_round", roundChange.View.Round, "msg_seq", roundChange.View.Sequence)

		// Verify ROUND CHANGE message is for the same sequence AND an equal or subsequent round as the proposal.
		// We have already called checkMessage by this point and checked the proposal's and PREPREPARE's sequence match.
		if roundChange.View.Sequence.Cmp(proposal.View.Sequence) != 0 || roundChange.View.Round.Cmp(proposal.View.Round) < 0 {
			msgLogger.Error("Round change in certificate for a different sequence or an earlier round")
			return errInvalidRoundChangeCertificateMsgView
		}

		if roundChange.HasPreparedCertificate() {
			msgLogger.Trace("Round change message has prepared certificate")
			preparedView, err := c.verifyPreparedCertificate(roundChange.PreparedCertificate)
			if err != nil {
				return err
			}
			// We must use the proposal in the prepared certificate with the highest round number. (See OSDI 99, Section 4.4)
			// Older prepared certificates may be generated, but if no node committed, there is no guarantee that
			// it will be the next pre-prepare. If one node committed, that block is guaranteed (by quorum intersection)
			// to be the next pre-prepare. That (higher view) prepared cert should override older perpared certs for
			// blocks that were not committed.
			// Also reject round change messages where the prepared view is greater than the round change view.
			msgLogger = msgLogger.New("prepared_round", preparedView.Round, "prepared_seq", preparedView.Sequence)
			if preparedView == nil || preparedView.Round.Cmp(proposal.View.Round) > 0 {
				return errInvalidRoundChangeViewMismatch
			} else if preparedView.Round.Cmp(maxRound) > 0 {
				msgLogger.Trace("Prepared certificate is latest in round change certificate")
				maxRound = preparedView.Round
				preferredDigest = roundChange.PreparedCertificate.Proposal.Hash()
			}
		}

		decodedMessages[i] = *roundChange
		// TODO(joshua): startNewRound needs these round change messages to generate a
		// round change certificate even if this node is not the next proposer
		c.roundChangeSet.Add(roundChange.View.Round, &message)
	}

	if maxRound.Cmp(big.NewInt(-1)) > 0 && proposal.Digest != preferredDigest {
		return errInvalidPreparedCertificateDigestMismatch
	}

	// May have already moved to this round based on quorum round change messages.
	logger.Trace("Trying to move to round change certificate's round", "target round", proposal.View.Round)

	return c.startNewRound(proposal.View.Round, false)
}

func (c *core) handleRoundChange(msg *istanbul.Message) error {
	logger := c.newLogger("func", "handleRoundChange", "tag", "handleMsg", "from", msg.Address)

	rc := msg.RoundChange()

	// Check consensus fork
	if c.isConsensusFork(rc.View.Sequence) {
		logger.Info("Received RoundChange (V1) for forked block sequence", "sequence", rc.View.Sequence.Uint64())
		return errors.New("Received RoundChange (V1) for forked block")
	}

	logger = logger.New("msg_round", rc.View.Round, "msg_seq", rc.View.Sequence)

	// Must be same sequence and future round.
	err := c.checkMessage(istanbul.MsgRoundChange, rc.View)

	// If the RC message is for the current sequence but a prior round, help the sender fast forward
	// by sending back to it (not broadcasting) a round change message for our desired round.
	if err == errOldMessage && rc.View.Sequence.Cmp(c.current.Sequence()) == 0 {
		logger.Trace("Sending round change for desired round to node with a previous desired round", "msg_round", rc.View.Round)
		c.sendRoundChangeAgain(msg.Address)
		return nil
	} else if err != nil {
		logger.Debug("Check round change message failed", "err", err)
		return err
	}

	// Verify the PREPARED certificate if present.
	if rc.HasPreparedCertificate() {
		preparedView, err := c.verifyPreparedCertificate(rc.PreparedCertificate)
		if err != nil {
			return err
		} else if preparedView == nil || preparedView.Round.Cmp(rc.View.Round) > 0 {
			return errInvalidRoundChangeViewMismatch
		}
	}

	roundView := rc.View

	// Add the ROUND CHANGE message to its message set.
	if err := c.roundChangeSet.Add(roundView.Round, msg); err != nil {
		logger.Warn("Failed to add round change message", "roundView", roundView, "err", err)
		return err
	}

	// Skip to the highest round we know F+1 (one honest validator) is at, but
	// don't start a round until we have a quorum who want to start a given round.
	ffRound := c.roundChangeSet.MaxRound(c.current.ValidatorSet().F() + 1)
	quorumRound := c.roundChangeSet.MaxOnOneRound(c.current.ValidatorSet().MinQuorumSize())
	logger = logger.New("ffRound", ffRound, "quorumRound", quorumRound)
	logger.Trace("Got round change message", "rcs", c.roundChangeSet.String())
	// On f+1 round changes we send a round change and wait for the next round if we haven't done so already
	// On quorum round change messages we go to the next round immediately.
	if quorumRound != nil && quorumRound.Cmp(c.current.DesiredRound()) >= 0 {
		logger.Debug("Got quorum round change messages, starting new round.")
		return c.startNewRound(quorumRound, true)
	} else if ffRound != nil {
		logger.Debug("Got f+1 round change messages, sending own round change message and waiting for next round.")
		c.waitForDesiredRound(ffRound)
	}

	return nil
}

// ----------------------------------------------------------------------------

// CurrentRoundChangeSet returns the current round change set summary.
func (c *core) CurrentRoundChangeSet() *RoundChangeSetSummary {
	if c.isConsensusFork(c.current.Sequence()) {
		rcs := c.roundChangeSetV2
		if rcs != nil {
			return rcs.Summary()
		}
	} else {
		rcs := c.roundChangeSet
		if rcs != nil {
			return rcs.Summary()
		}
	}
	return nil
}
