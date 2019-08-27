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
	c.waitForNewRound(nextView)

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
		if roundChange.View.Cmp(proposal.View) != 0 {
			return errInvalidRoundChangeCertificateMsgView
		}

		decodedMessages[i] = *roundChange
		// TODO: see some prepared certificates proposals but supporting messages.
		// Store valid round change messages
		if roundChange.HasPreparedCertificate() {
			// Verify message for the proper proposal.
			if proposal.Digest != roundChange.PreparedCertificate.Proposal.Hash() {
				return errInvalidPreparedCertificateDigestMismatch
			}

			if err := c.verifyPreparedCertificate(roundChange.PreparedCertificate); err != nil {
				return err
			}
		}
		// TODO: Check against seen messages (but handled in round change set???)
		// if c.backend.IsKnownMessage(message) {
			c.roundChangeSet.Add(roundChange.View.Round, &message)
		// }
	}

	// TODO(joshua) Fix this.
	// On good round change certificate. Don't touch our own prepared certifiacte and start a new round
	// If the new round fails, we're still free to reject it.
	// TODO: figure out why byzantine nodes cannot just go to a new seq instead of new round.
	// Do need to add each round change messge into the round change set.
	// Can just drop repeated messages in b/c message sets do stuff by addresses.
	logger.Trace("Accepted round change certificate")
	if c.current.Sequence().Cmp(proposal.View.Sequence) <= 0 && c.current.Round().Cmp(proposal.View.Round) < 0{
		logger.Trace("Moving to next round based on round change certificate")
		c.startNewRound(proposal.View.Round)
	} else {
		logger.Trace("Theoretically in the round for which the round change certificate wanted to move us to.")
	}
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

	if err := c.checkMessage(istanbul.MsgRoundChange, rc.View); err != nil {
		logger.Info("Check round change message failed", "err", err)
		return err
	}

	// Verify the PREPARED certificate if present.
	if rc.HasPreparedCertificate() {
		// TODO(Joshua): I don't think we should clobber our own prepared certificate here.
		// TODO(asa): Should we still accept the round change message without the certificate if this fails?
		if err := c.verifyPreparedCertificate(rc.PreparedCertificate); err != nil {
			return err
		}
	}

	cv := c.currentView()
	roundView := rc.View

	// Add the ROUND CHANGE message to its message set and return how many
	// messages we've got with the same round number and sequence number.
	num, err := c.roundChangeSet.Add(roundView.Round, msg)
	if err != nil {
		logger.Warn("Failed to add round change message", "message", msg, "err", err)
		return err
	}

	// Once we received f+1 ROUND CHANGE messages, those messages form a weak certificate.
	// If our round number is smaller than the certificate's round number, we would
	// try to catch up the round number.
	if c.waitingForNewRound && num == c.valSet.F()+1 {
		if cv.Round.Cmp(roundView.Round) < 0 {
			// TODO(asa): If we saw a PREPARED certificate, do we include one in our round change message?
			c.sendRoundChange(roundView.Round)
		}
		return nil
	} else if num == c.valSet.MinQuorumSize() && (c.waitingForNewRound || cv.Round.Cmp(roundView.Round) < 0) {
		// We've received the minimum quorum size ROUND CHANGE messages, start a new round immediately.
		c.startNewRound(roundView.Round)
		return nil
	} else if cv.Round.Cmp(roundView.Round) < 0 {
		// Round of message > current round?
		// Only gossip the message with current round to other validators.
		return errIgnored
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
		r := big.NewInt(int64(k))
		if maxRound == nil || maxRound.Cmp(r) < 0 {
			maxRound = r
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
