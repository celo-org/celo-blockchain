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

// sendNextRoundChange sends the ROUND CHANGE message with current round + 1
func (c *core) sendNextRoundChange() {
	cv := c.currentView()
	c.sendRoundChange(new(big.Int).Add(cv.Round, common.Big1))
}

// sendRoundChange sends the ROUND CHANGE message with the given round
func (c *core) sendRoundChange(round *big.Int) {
	logger := c.logger.New("state", c.state, "cur_round", c.current.Round(), "cur_seq", c.current.Sequence(), "func", "sendRoundChange", "target round", round)

	cv := c.currentView()
	if cv.Round.Cmp(round) >= 0 {
		logger.Error("Cannot send out the round change")
		return
	}

	nextView := &istanbul.View{
		// The round number we'd like to transfer to.
		Round:    new(big.Int).Set(round),
		Sequence: new(big.Int).Set(cv.Sequence),
	}

	rc := &istanbul.RoundChange{
		View:                nextView,
		PreparedCertificate: c.current.preparedCertificate,
	}

	payload, err := Encode(rc)
	if err != nil {
		logger.Error("Failed to encode ROUND CHANGE", "rc", rc, "err", err)
		return
	}
	logger.Trace("Sending round change message", "rcs", c.roundChangeSet)
	c.broadcast(&istanbul.Message{
		Code: istanbul.MsgRoundChange,
		Msg:  payload,
	})
}

func (c *core) handleRoundChangeCertificate(proposal istanbul.Subject, roundChangeCertificate istanbul.RoundChangeCertificate) error {
	logger := c.logger.New("state", c.state, "cur_round", c.current.Round(), "cur_seq", c.current.Sequence(), "func", "handleRoundChangeCertificate")

	if len(roundChangeCertificate.RoundChangeMessages) > c.valSet.Size() || len(roundChangeCertificate.RoundChangeMessages) < c.valSet.MinQuorumSize() {
		return errInvalidRoundChangeCertificateNumMsgs
	}

	maxRound := big.NewInt(-1)
	preferredDigest := common.Hash{}
	seen := make(map[common.Address]bool)
	decodedMessages := make([]istanbul.RoundChange, len(roundChangeCertificate.RoundChangeMessages))
	for i, message := range roundChangeCertificate.RoundChangeMessages {
		// Verify message signed by a validator
		data, err := message.PayloadNoSig()
		if err != nil {
			return err
		}

		signer, err := c.validateFn(data, message.Signature)
		if err != nil {
			return err
		}

		if signer != message.Address {
			return errInvalidRoundChangeCertificateMsgSignature
		}

		// Check for duplicate ROUND CHANGE messages
		if seen[signer] {
			return errInvalidRoundChangeCertificateDuplicate
		}
		seen[signer] = true

		// Check that the message is a ROUND CHANGE message
		if istanbul.MsgRoundChange != message.Code {
			return errInvalidRoundChangeCertificateMsgCode
		}

		var roundChange *istanbul.RoundChange
		if err := message.Decode(&roundChange); err != nil {
			logger.Error("Failed to decode ROUND CHANGE in certificate", "err", err)
			return err
		}

		// Verify ROUND CHANGE message is for a proper view
		if roundChange.View.Cmp(proposal.View) != 0 || roundChange.View.Round.Cmp(c.current.DesiredRound()) < 0 {
			return errInvalidRoundChangeCertificateMsgView
		}

		if roundChange.HasPreparedCertificate() {
			if err := c.verifyPreparedCertificate(roundChange.PreparedCertificate); err != nil {
				return err
			}
			// We must use the proposal in the prepared certificate with the highest round number. (See OSDI 99, Section 4.4)
			// Older prepared certificates may be generated, but if no node committed, there is no guarantee that
			// it will be the next pre-prepare. If one node committed, that block is guaranteed (by quorum intersection)
			// to be the next pre-prepare. That (higher view) prepared cert should override older perpared certs for
			// blocks that were not committed.
			// Also reject round change messages where the prepared view is greater than the round change view.
			preparedView := roundChange.PreparedCertificate.View()
			if preparedView == nil || preparedView.Round.Cmp(proposal.View.Round) > 0 {
				return errInvalidRoundChangeViewMismatch
			} else if preparedView.Round.Cmp(maxRound) > 0 {
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
	c.startNewRound(proposal.View.Round)

	return nil
}

func (c *core) handleRoundChange(msg *istanbul.Message) error {
	logger := c.logger.New("state", c.state, "from", msg.Address, "cur_round", c.current.Round(), "cur_seq", c.current.Sequence(), "func", "handleRoundChange", "tag", "handleMsg")

	// Decode ROUND CHANGE message
	var rc *istanbul.RoundChange
	if err := msg.Decode(&rc); err != nil {
		logger.Error("Failed to decode ROUND CHANGE", "err", err)
		return errInvalidMessage
	}

	// Must be same sequence and future round.
	if err := c.checkMessage(istanbul.MsgRoundChange, rc.View); err != nil {
		logger.Info("Check round change message failed", "err", err)
		return err
	}

	// Verify the PREPARED certificate if present.
	if rc.HasPreparedCertificate() {
		if err := c.verifyPreparedCertificate(rc.PreparedCertificate); err != nil {
			return err
		}
		preparedCertView := rc.PreparedCertificate.View()
		if preparedCertView == nil || preparedCertView.Round.Cmp(rc.View.Round) > 0 {
			return errInvalidRoundChangeViewMismatch
		}
	}

	roundView := rc.View

	// Add the ROUND CHANGE message to its message set and return how many
	// messages we've got with the same round number and sequence number.
	num, err := c.roundChangeSet.Add(roundView.Round, msg)
	if err != nil {
		logger.Warn("Failed to add round change message", "message", msg, "err", err)
		return err
	}
	logger.Trace("Got round change message", "num", num, "message_round", roundView.Round)

	// On f+1 round changes we send a round change and wait for the next round if we haven't done so already
	// On quorum round change messages we go to the next round immediately.
	if num == c.valSet.F()+1 {
		logger.Trace("Got f+1 round change messages, sending own round change message and waiting for next round.")
		c.waitForDesiredRound(roundView.Round)
	} else if num == c.valSet.MinQuorumSize() {
		logger.Trace("Got quorum round change messages, starting new round.")
		c.startNewRound(roundView.Round)
	}
	return nil
}

// ----------------------------------------------------------------------------

func newRoundChangeSet(valSet istanbul.ValidatorSet) *roundChangeSet {
	return &roundChangeSet{
		validatorSet: valSet,
		roundChanges: make(map[uint64]*messageSet),
		mu:           new(sync.Mutex),
	}
}

type roundChangeSet struct {
	validatorSet istanbul.ValidatorSet
	roundChanges map[uint64]*messageSet
	mu           *sync.Mutex
}

// Add adds the round and message into round change set
func (rcs *roundChangeSet) Add(r *big.Int, msg *istanbul.Message) (int, error) {
	rcs.mu.Lock()
	defer rcs.mu.Unlock()

	round := r.Uint64()
	if rcs.roundChanges[round] == nil {
		rcs.roundChanges[round] = newMessageSet(rcs.validatorSet)
	}
	err := rcs.roundChanges[round].Add(msg)
	if err != nil {
		return 0, err
	}
	return rcs.roundChanges[round].Size(), nil
}

// Clear deletes the messages with smaller round
func (rcs *roundChangeSet) Clear(round *big.Int) {
	rcs.mu.Lock()
	defer rcs.mu.Unlock()

	for k, rms := range rcs.roundChanges {
		if len(rms.Values()) == 0 || k < round.Uint64() {
			delete(rcs.roundChanges, k)
		}
	}
}

// MaxRound returns the max round which the number of messages is equal or larger than num
func (rcs *roundChangeSet) MaxRound(num int) *big.Int {
	rcs.mu.Lock()
	defer rcs.mu.Unlock()

	var maxRound *big.Int
	for k, rms := range rcs.roundChanges {
		if rms.Size() < num {
			continue
		}
	}

	return nil
}

// MaxOnOneRound returns the max round which the number of messages is >= num
func (rcs *roundChangeSet) MaxOnOneRound(num int) *big.Int {
	rcs.mu.Lock()
	defer rcs.mu.Unlock()

	// Sort rounds descending
	var sortedRounds []uint64
	for r := range rcs.msgsForRound {
		sortedRounds = append(sortedRounds, r)
	}
	sort.Slice(sortedRounds, func(i, j int) bool { return sortedRounds[i] > sortedRounds[j] })

	for _, r := range sortedRounds {
		rms := rcs.msgsForRound[r]
		if rms.Size() >= num {
			return new(big.Int).SetUint64(r)
		}
	}
	return maxRound
}

func (rcs *roundChangeSet) getCertificate(r *big.Int, quorumSize int) (istanbul.RoundChangeCertificate, error) {
	rcs.mu.Lock()
	defer rcs.mu.Unlock()

	round := r.Uint64()
	if rcs.roundChanges[round] != nil && rcs.roundChanges[round].Size() >= quorumSize {
		messages := make([]istanbul.Message, rcs.roundChanges[round].Size())
		for i, message := range rcs.roundChanges[round].Values() {
			messages[i] = *message
		}
		return istanbul.RoundChangeCertificate{
			RoundChangeMessages: messages,
		}, nil
	} else {
		return istanbul.RoundChangeCertificate{}, errFailedCreateRoundChangeCertificate
	}
}
