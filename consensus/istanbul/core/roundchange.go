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
	"math/big"
	"sort"
	"strings"
	"sync"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/istanbul"
)

// sendRoundChange sends the ROUND CHANGE message with the given round
func (c *core) sendRoundChange(round *big.Int) {
	logger := c.newLogger("func", "sendRoundChange", "target_round", round)

	if c.current.View().Round.Cmp(round) >= 0 {
		logger.Warn("Cannot send round change for previous round")
		return
	}

	msg, err := c.buildRoundChangeMsg(round)
	if err != nil {
		logger.Error("Could not build round change message", "err", msg)
		return
	}

	c.broadcast(msg)
}

// sendRoundChange sends a ROUND CHANGE message for the current round back to a single address
func (c *core) sendRoundChangeAgain(addr common.Address) {
	logger := c.newLogger("func", "sendRoundChange", "desired_round", c.current.DesiredRound(), "to", addr)

	msg, err := c.buildRoundChangeMsg(c.current.DesiredRound())
	if err != nil {
		logger.Error("Could not build round change message", "err", err)
		return
	}

	c.unicast(msg, addr)
}

// buildRoundChangeMsg creates a round change msg for the given round
func (c *core) buildRoundChangeMsg(round *big.Int) (*istanbul.Message, error) {
	nextView := &istanbul.View{
		Round:    new(big.Int).Set(round),
		Sequence: new(big.Int).Set(c.current.View().Sequence),
	}

	rc := &istanbul.RoundChange{
		View:                nextView,
		PreparedCertificate: c.current.PreparedCertificate(),
	}

	payload, err := Encode(rc)
	if err != nil {
		return nil, err
	}

	return &istanbul.Message{
		Code: istanbul.MsgRoundChange,
		Msg:  payload,
	}, nil
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
			logger.Warn("Failed to decode ROUND CHANGE in certificate", "err", err)
			return err
		} else if roundChange.View == nil || roundChange.View.Sequence == nil || roundChange.View.Round == nil {
			return errInvalidRoundChangeCertificateMsgView
		}

		msgLogger := logger.New("msg_round", roundChange.View.Round, "msg_seq", roundChange.View.Sequence)

		// Verify ROUND CHANGE message is for the same sequence AND an equal or subsequent round as the proposal.
		// We have already called checkMessage by this point and checked the proposal's and PREPREPARE's sequence match.
		if roundChange.View.Sequence.Cmp(proposal.View.Sequence) != 0 || roundChange.View.Round.Cmp(proposal.View.Round) < 0 {
			msgLogger.Error("Round change in certificate for a different sequence or an earlier round", "err", err)
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

	return c.startNewRound(proposal.View.Round)
}

func (c *core) handleRoundChange(msg *istanbul.Message) error {
	logger := c.newLogger("func", "handleRoundChange", "tag", "handleMsg", "from", msg.Address)

	// Decode ROUND CHANGE message
	var rc *istanbul.RoundChange
	if err := msg.Decode(&rc); err != nil {
		logger.Info("Failed to decode ROUND CHANGE", "err", err)
		return errInvalidMessage
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
		return c.startNewRound(quorumRound)
	} else if ffRound != nil {
		logger.Debug("Got f+1 round change messages, sending own round change message and waiting for next round.")
		c.waitForDesiredRound(ffRound)
	}

	return nil
}

// ----------------------------------------------------------------------------

func newRoundChangeSet(valSet istanbul.ValidatorSet) *roundChangeSet {
	return &roundChangeSet{
		validatorSet:      valSet,
		msgsForRound:      make(map[uint64]MessageSet),
		latestRoundForVal: make(map[common.Address]uint64),
		mu:                new(sync.Mutex),
	}
}

type roundChangeSet struct {
	validatorSet      istanbul.ValidatorSet
	msgsForRound      map[uint64]MessageSet
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
			return errOldMessage
		} else if prevLatestRound < round {
			// Already got an RC for an earlier round from this validator.
			// Forget that and remember this.
			if rcs.msgsForRound[prevLatestRound] != nil {
				rcs.msgsForRound[prevLatestRound].Remove(src)
				if rcs.msgsForRound[prevLatestRound].Size() == 0 {
					delete(rcs.msgsForRound, prevLatestRound)
				}
			}
		}
	}

	rcs.latestRoundForVal[src] = round

	if rcs.msgsForRound[round] == nil {
		rcs.msgsForRound[round] = newMessageSet(rcs.validatorSet)
	}
	return rcs.msgsForRound[round].Add(msg)
}

// Clear deletes the messages with smaller round
func (rcs *roundChangeSet) Clear(round *big.Int) {
	rcs.mu.Lock()
	defer rcs.mu.Unlock()

	for k, rms := range rcs.msgsForRound {
		if rms.Size() == 0 || k < round.Uint64() {
			for _, msg := range rms.Values() {
				if _, ok := rcs.latestRoundForVal[msg.Address]; ok {
					delete(rcs.latestRoundForVal, msg.Address)
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
	return nil
}

func (rcs *roundChangeSet) String() string {
	rcs.mu.Lock()
	defer rcs.mu.Unlock()

	// Sort rounds descending
	var sortedRounds []uint64
	for r := range rcs.msgsForRound {
		sortedRounds = append(sortedRounds, r)
	}
	sort.Slice(sortedRounds, func(i, j int) bool { return sortedRounds[i] > sortedRounds[j] })

	modeRound := uint64(0)
	modeRoundSize := 0
	msgsForRoundStr := make([]string, 0, len(sortedRounds))
	for _, r := range sortedRounds {
		rms := rcs.msgsForRound[r]
		if rms.Size() > modeRoundSize {
			modeRound = r
			modeRoundSize = rms.Size()
		}
		msgsForRoundStr = append(msgsForRoundStr, fmt.Sprintf("%v: %v", r, rms.String()))
	}

	latestRoundForValStr := make([]string, 0, len(rcs.latestRoundForVal))
	for addr, r := range rcs.latestRoundForVal {
		latestRoundForValStr = append(latestRoundForValStr, fmt.Sprintf("%v: %v", addr.String(), r))
	}

	return fmt.Sprintf("RCS len=%v mode_round=%v mode_round_len=%v unique_rounds=%v %v",
		len(rcs.latestRoundForVal),
		modeRound,
		modeRoundSize,
		len(rcs.msgsForRound),
		strings.Join(msgsForRoundStr, ", "))
}

// Gets a round change certificate for a specific round. Includes quorumSize messages of that round or later.
// If the total is less than quorumSize, returns an empty cert and errFailedCreateRoundChangeCertificate.
func (rcs *roundChangeSet) getCertificate(minRound *big.Int, quorumSize int) (istanbul.RoundChangeCertificate, error) {
	rcs.mu.Lock()
	defer rcs.mu.Unlock()

	// Sort rounds descending
	var sortedRounds []uint64
	for r := range rcs.msgsForRound {
		sortedRounds = append(sortedRounds, r)
	}
	sort.Slice(sortedRounds, func(i, j int) bool { return sortedRounds[i] > sortedRounds[j] })

	var messages []istanbul.Message
	for _, r := range sortedRounds {
		if r < minRound.Uint64() {
			break
		}
		for _, message := range rcs.msgsForRound[r].Values() {
			messages = append(messages, *message)

			// Stop when we've added a quorum of the highest-round messages.
			if len(messages) >= quorumSize {
				return istanbul.RoundChangeCertificate{
					RoundChangeMessages: messages,
				}, nil
			}
		}
	}

	// Didn't find a quorum of messages. Return an empty certificate with error.
	return istanbul.RoundChangeCertificate{}, errFailedCreateRoundChangeCertificate
}
