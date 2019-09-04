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
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/istanbul"
	"math/big"
	"sort"
	"strings"
	"sync"
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
		// TODO(joshua): May be able to relax to the proposal is >= than the round change message.
		if roundChange.View.Cmp(proposal.View) != 0 || roundChange.View.Round.Cmp(c.current.DesiredRound()) < 0 {
			return errInvalidRoundChangeCertificateMsgView
		}

		if roundChange.HasPreparedCertificate() {
			if err := c.verifyPreparedCertificate(roundChange.PreparedCertificate); err != nil {
				return err
			}
			// The prepared certificate with the highest round number carries the proposal we must use
			preparedView := roundChange.PreparedCertificate.View()
			if preparedView != nil && preparedView.Round.Cmp(maxRound) > 0 {
				maxRound = preparedView.Round
				preferredDigest = roundChange.PreparedCertificate.Proposal.Hash()
			}
		}

		decodedMessages[i] = *roundChange
		// TODO(joshua): startNewRound needs these round change messages to generate a
		// prepared certificate even if this node is not the next proposer
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
	}

	roundView := rc.View

	// Add the ROUND CHANGE message to its message set.
	if err := c.roundChangeSet.Add(roundView.Round, msg); err != nil {
		logger.Warn("Failed to add round change message", "roundView", roundView, "err", err)
		return err
	}

	ffRound := c.roundChangeSet.MaxRound(c.valSet.F() + 1)
	quorumRound := c.roundChangeSet.MaxRound(c.valSet.MinQuorumSize())

	logger.Info("handleRoundChange", "msg_round", roundView.Round, "rcs", c.roundChangeSet.String(), "ffRound", ffRound, "quorumRound", quorumRound)
	// On f+1 round changes we send a round change and wait for the next round if we haven't done so already
	// On quorum round change messages we go to the next round immediately.
	if quorumRound != nil && quorumRound.Cmp(c.current.DesiredRound()) >= 0 {
		logger.Trace("Got quorum round change messages, starting new round.")
		c.startNewRound(quorumRound)
	} else if ffRound != nil {
		logger.Trace("Got f+1 round change messages, sending own round change message and waiting for next round.")
		c.waitForDesiredRound(ffRound)
	}

	return nil
}

// ----------------------------------------------------------------------------

func newRoundChangeSet(valSet istanbul.ValidatorSet) *roundChangeSet {
	return &roundChangeSet{
		validatorSet:      valSet,
		msgsForRound:      make(map[uint64]*messageSet),
		latestRoundForVal: make(map[common.Address]uint64),
		mu:                new(sync.Mutex),
	}
}

type roundChangeSet struct {
	validatorSet      istanbul.ValidatorSet
	msgsForRound      map[uint64]*messageSet
	latestRoundForVal map[common.Address]uint64
	mu                *sync.Mutex
}

// Add adds the round and message into round change set
func (rcs *roundChangeSet) Add(r *big.Int, msg *istanbul.Message) error {
	rcs.mu.Lock()
	defer rcs.mu.Unlock()

	src := msg.Address
	round := r.Uint64()

	if prevLatestRound, ok := rcs.latestRoundForVal[src]; ok {
		if prevLatestRound > round {
			// Reject as we have an RC for a later round from this validator.
			// logger.Debug("Message is old")
			return errOldMessage
		} else if prevLatestRound < round {
			// Already got an RC for an earlier round from this validator.
			// Forget that and remember this.
			if rcs.msgsForRound[prevLatestRound] != nil {
				rcs.msgsForRound[prevLatestRound].Remove(src)
				if rcs.msgsForRound[prevLatestRound].Size() == 0 {
					delete(rcs.msgsForRound, prevLatestRound)
				}
				// logger.Debug("Deleting earlier Round Change Messages")
			}
		}
	}

	rcs.latestRoundForVal[src] = round

	if rcs.msgsForRound[round] == nil {
		rcs.msgsForRound[round] = newMessageSet(rcs.validatorSet)
		// logger.Debug("Creating new RC message set")
	}
	return rcs.msgsForRound[round].Add(msg)
}

// Clear deletes the messages with smaller round
func (rcs *roundChangeSet) Clear(round *big.Int) {
	rcs.mu.Lock()
	defer rcs.mu.Unlock()

	for k, rms := range rcs.msgsForRound {
		if rms.Size() == 0 || k < round.Uint64() {
			if rms != nil {
				for _, msg := range rms.Values() {
					if latestRound, ok := rcs.latestRoundForVal[msg.Address]; ok && k == latestRound {
						delete(rcs.latestRoundForVal, msg.Address)
					}
				}
			}
			delete(rcs.msgsForRound, k)
		}
	}
}

// MaxRound returns the max round which the number of messages is equal or larger than num
func (rcs *roundChangeSet) MaxRound(num int) *big.Int {
	rcs.mu.Lock()
	defer rcs.mu.Unlock()

	// Sort rounds descending
	var sortedRounds []uint64
	for r := range rcs.msgsForRound {
		sortedRounds = append(sortedRounds, r)
	}
	sort.Slice(sortedRounds, func(i, j int) bool { return sortedRounds[i] > sortedRounds[j] })

	acc := 0
	for _, r := range sortedRounds {
		rms := rcs.msgsForRound[r]
		acc += rms.Size()
		if acc >= num {
			return new(big.Int).SetUint64(r)
		}
	}

	return nil
}

func (rcs *roundChangeSet) String() string {
	rcs.mu.Lock()
	defer rcs.mu.Unlock()

	msgsForRoundStr := make([]string, 0, len(rcs.msgsForRound))
	for r, rms := range rcs.msgsForRound {
		msgsForRoundStr = append(msgsForRoundStr, fmt.Sprintf("%v: %v", r, rms.String()))
	}

	latestRoundForValStr := make([]string, 0, len(rcs.latestRoundForVal))
	for addr, r := range rcs.latestRoundForVal {
		latestRoundForValStr = append(latestRoundForValStr, fmt.Sprintf("%v: %v", addr.String(), r))
	}

	return fmt.Sprintf("RCS len=%v  By round: {<%v> %v}  By val: {<%v> %v}",
		len(rcs.latestRoundForVal),
		len(rcs.msgsForRound),
		strings.Join(msgsForRoundStr, ", "),
		len(rcs.latestRoundForVal),
		strings.Join(latestRoundForValStr, ", "))
}

func (rcs *roundChangeSet) getCertificate(r *big.Int, quorumSize int) (istanbul.RoundChangeCertificate, error) {
	rcs.mu.Lock()
	defer rcs.mu.Unlock()

	round := r.Uint64()
	if rcs.msgsForRound[round] != nil && rcs.msgsForRound[round].Size() >= quorumSize {
		messages := make([]istanbul.Message, rcs.msgsForRound[round].Size())
		for i, message := range rcs.msgsForRound[round].Values() {
			messages[i] = *message
		}
		return istanbul.RoundChangeCertificate{
			RoundChangeMessages: messages,
		}, nil
	} else {
		return istanbul.RoundChangeCertificate{}, errFailedCreateRoundChangeCertificate
	}
}
