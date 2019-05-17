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
	logger := c.logger.New("state", c.state)

	cv := c.currentView()
	if cv.Round.Cmp(round) >= 0 {
		logger.Error("Cannot send out the round change", "current round", cv.Round, "target round", round)
		return
	}

	c.catchUpRound(&istanbul.View{
		// The round number we'd like to transfer to.
		Round:    new(big.Int).Set(round),
		Sequence: new(big.Int).Set(cv.Sequence),
	})

	// Now we have the new round number and sequence number
	cv = c.currentView()
	rc := &istanbul.RoundChange{
		View:                cv,
		PreparedCertificate: c.current.GetPreparedCertificate(c.valSet.F()),
	}

	payload, err := Encode(rc)
	if err != nil {
		logger.Error("Failed to encode ROUND CHANGE", "rc", rc, "err", err)
		return
	}

	c.broadcast(&istanbul.Message{
		Code: istanbul.MsgRoundChange,
		Msg:  payload,
	})
}

func (c *core) ValidatePreparedCertificate(preparedCertificate istanbul.PreparedCertificate) error {
	logger := c.logger.New("state", c.state)

	// Validate the attached proposal
	if _, err := c.backend.Verify(preparedCertificate.Proposal); err != nil {
		return errInvalidPreparedCertificateProposal
	}

	if len(preparedCertificate.PrepareMessages) > c.valSet.Size() || len(preparedCertificate.PrepareMessages) < 2*c.valSet.F()+1 {
		return errInvalidPreparedCertificateNumMsgs
	}

	seen := make(map[common.Address]bool)
	for _, message := range preparedCertificate.PrepareMessages {
		data, err := message.PayloadNoSig()
		if err != nil {
			return err
		}

		// Verify message signed by a validator
		signer, err := c.validateFn(data, message.Signature)
		if err != nil {
			return errInvalidPreparedCertificateMsgSignature
		}

		if signer != message.Address {
			return errInvalidPreparedCertificateMsgSignature
		}

		// Check for duplicate messages
		if seen[signer] {
			return errInvalidPreparedCertificateDuplicate
		}
		seen[signer] = true

		// Check that the message is a PREPARE message
		if istanbul.MsgPrepare != message.Code {
			return errInvalidPreparedCertificateMsgCode
		}

		var prepare *istanbul.Subject
		if err := message.Decode(&prepare); err != nil {
			logger.Error("Failed to decode PREPARE in ROUND CHANGE", "err", err)
			return errInvalidPreparedCertificateMsgDecode
		}

		// Verify PREPARE message for the proper view
		// TODO(asa): Figure this out.
		/*
			if err := c.checkMessage(istanbul.MsgPrepare, prepare.View); err != nil {
				return err
			}
		*/

		if prepare.Digest != preparedCertificate.Proposal.Hash() {
			return errInvalidPreparedCertificateDigestMismatch
		}
	}
	return nil
}

func (c *core) handleRoundChange(msg *istanbul.Message, src istanbul.Validator) error {
	logger := c.logger.New("state", c.state, "from", src.Address().Hex())

	// Decode ROUND CHANGE message
	var rc *istanbul.RoundChange
	if err := msg.Decode(&rc); err != nil {
		logger.Error("Failed to decode ROUND CHANGE", "err", err)
		return errInvalidMessage
	}

	// Validate the PREPARED certificate if present.
	if rc.HasPreparedCertificate() {
		if err := c.ValidatePreparedCertificate(rc.PreparedCertificate); err != nil {
			// TODO(asa): Should we still accept the round change message without the certificate if this fails?
			return err
		}
	}

	if err := c.checkMessage(istanbul.MsgRoundChange, rc.View); err != nil {
		return err
	}

	cv := c.currentView()
	roundView := rc.View

	// Add the ROUND CHANGE message to its message set and return how many
	// messages we've got with the same round number and sequence number.
	num, err := c.roundChangeSet.Add(roundView.Round, msg, rc.PreparedCertificate.Proposal)
	if err != nil {
		logger.Warn("Failed to add round change message", "from", src, "msg", msg, "err", err)
		return err
	}

	// Once we received f+1 ROUND CHANGE messages, those messages form a weak certificate.
	// If our round number is smaller than the certificate's round number, we would
	// try to catch up the round number.
	if c.waitingForRoundChange && num == c.valSet.F()+1 {
		if cv.Round.Cmp(roundView.Round) < 0 {
			// TODO(asa): If we saw a PREPARED certificate, do we include one in our round change message?
			c.sendRoundChange(roundView.Round)
		}
		return nil
	} else if num == 2*c.valSet.F()+1 && (c.waitingForRoundChange || cv.Round.Cmp(roundView.Round) < 0) {
		// We've received 2f+1 ROUND CHANGE messages, start a new round immediately.
		c.startNewRound(roundView.Round)
		return nil
	} else if cv.Round.Cmp(roundView.Round) < 0 {
		// Only gossip the message with current round to other validators.
		return errIgnored
	}
	return nil
}

// ----------------------------------------------------------------------------

func newRoundChangeSet(valSet istanbul.ValidatorSet) *roundChangeSet {
	return &roundChangeSet{
		validatorSet:     valSet,
		preparedProposal: make(map[uint64]istanbul.Proposal),
		roundChanges:     make(map[uint64]*messageSet),
		mu:               new(sync.Mutex),
	}
}

type roundChangeSet struct {
	validatorSet     istanbul.ValidatorSet
	preparedProposal map[uint64]istanbul.Proposal
	roundChanges     map[uint64]*messageSet
	mu               *sync.Mutex
}

// Add adds the round and message into round change set
func (rcs *roundChangeSet) Add(r *big.Int, msg *istanbul.Message, proposal istanbul.Proposal) (int, error) {
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
	rcs.preparedProposal[round] = proposal
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

	for k, preparedProposal := range rcs.preparedProposal {
		if preparedProposal != nil || k < round.Uint64() {
			delete(rcs.preparedProposal, k)
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
		r := big.NewInt(int64(k))
		if maxRound == nil || maxRound.Cmp(r) < 0 {
			maxRound = r
		}
	}
	return maxRound
}

func (rcs *roundChangeSet) GetCertificate(r *big.Int, f int) (istanbul.RoundChangeCertificate, error) {
	rcs.mu.Lock()
	defer rcs.mu.Unlock()

	round := r.Uint64()
	if rcs.roundChanges[round].Size() > 2*f {
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

func (rcs *roundChangeSet) GetPreparedProposal(r *big.Int) istanbul.Proposal {
	rcs.mu.Lock()
	defer rcs.mu.Unlock()

	round := r.Uint64()
	return rcs.preparedProposal[round]
}
