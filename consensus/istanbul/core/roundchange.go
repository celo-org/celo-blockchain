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
	logger := c.logger.New("state", c.state, "cur_round", c.current.Round(), "cur_seq", c.current.Sequence(), "func", "sendRoundChange", "round", round)

	cv := c.currentView()
	if cv.Round.Cmp(round) >= 0 {
		logger.Error("Cannot send out the round change", "current round", cv.Round, "target round", round)
		return
	}

	logger.Debug("sendRoundChange", "current round", cv.Round, "target round", round, "rcs", c.roundChangeSet)

	c.catchUpRound(&istanbul.View{
		// The round number we'd like to transfer to.
		Round:    new(big.Int).Set(round),
		Sequence: new(big.Int).Set(cv.Sequence),
	})

	// Now we have the new round number and sequence number
	cv = c.currentView()
	rc := &istanbul.Subject{
		View:   cv,
		Digest: common.Hash{},
	}

	payload, err := Encode(rc)
	if err != nil {
		logger.Error("Failed to encode ROUND CHANGE", "rc", rc, "err", err)
		return
	}

	c.broadcast(&message{
		Code: msgRoundChange,
		Msg:  payload,
	})
}

func (c *core) handleRoundChange(msg *message, src istanbul.Validator) error {
	idx, _ := c.valSet.GetByAddress(src.Address())
	logger := c.logger.New("state", c.state, "from", src.Address().Hex(), "from_id", idx, "cur_round", c.current.Round(), "cur_seq", c.current.Sequence(), "func", "handleRoundChange")

	// Decode ROUND CHANGE message
	var rc *istanbul.Subject
	if err := msg.Decode(&rc); err != nil {
		logger.Error("Failed to decode ROUND CHANGE", "err", err)
		return errInvalidMessage
	}

	if err := c.checkMessage(msgRoundChange, rc.View); err != nil {
		return err
	}

	cv := c.currentView()
	roundView := rc.View

	// Add the ROUND CHANGE message to its message set.
	if err := c.roundChangeSet.Add(roundView.Round, msg, src); err != nil {
		logger.Warn("Failed to add round change message", "from", src, "msg", msg, "err", err)
		return err
	}

	ffRound := c.roundChangeSet.MaxRound(c.valSet.F() + 1)
	quorumRound := c.roundChangeSet.MaxRound(2*c.valSet.F() + 1)

	logger.Info("handleRoundChange", "curr_round", cv.Round, "msg_round", roundView.Round, "rcs", c.roundChangeSet.String(), "wfRC", c.waitingForRoundChange, "ffRound", ffRound, "quorumRound", quorumRound)

	if quorumRound != nil && (c.waitingForRoundChange || cv.Round.Cmp(quorumRound) < 0) {
		// We've received 2f+1 ROUND CHANGE messages, start a new round immediately.
		c.startNewRound(quorumRound)
		return nil
	}

	// Once we received f+1 ROUND CHANGE messages, those messages form a weak certificate.
	// If our round number is smaller than the certificate's round number, we would
	// try to catch up the round number.
	if c.waitingForRoundChange && ffRound != nil {
		if cv.Round.Cmp(ffRound) < 0 {
			c.sendRoundChange(ffRound)
		}
		return nil
	}

	if cv.Round.Cmp(roundView.Round) < 0 {
		// Only gossip the message with current round to other validators.
		// TODO(tim) This in fact also gossips newer messages -- remove comment when gossip disabled
		return errIgnored
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
<<<<<<< HEAD
func (rcs *roundChangeSet) Add(r *big.Int, msg *message, src istanbul.Validator) (int, error) {
=======
func (rcs *roundChangeSet) Add(r *big.Int, msg *istanbul.Message, src istanbul.Validator) error {
>>>>>>> a59f2dfb2... Cumulatively count RCs pt 2
	rcs.mu.Lock()
	defer rcs.mu.Unlock()

	round := r.Uint64()

	if prevLatestRound, ok := rcs.latestRoundForVal[src.Address()]; ok {
		if prevLatestRound > round {
			// Reject as we have an RC for a later round from this validator.
			return errOldMessage
		} else if prevLatestRound < round {
			// Already got an RC for an earlier round from this validator.
			// Forget that and remember this.
			if rcs.msgsForRound[prevLatestRound] != nil {
				rcs.msgsForRound[prevLatestRound].Remove(src.Address())
				if rcs.msgsForRound[prevLatestRound].Size() == 0 {
					delete(rcs.msgsForRound, prevLatestRound)
				}
			}
		}
	}

	rcs.latestRoundForVal[src.Address()] = round

	if rcs.msgsForRound[round] == nil {
		rcs.msgsForRound[round] = newMessageSet(rcs.validatorSet)
	}
<<<<<<< HEAD
	err := rcs.msgsForRound[round].Add(msg)
	if err != nil {
		return 0, err
	}

	num := 0
	for k, rms := range rcs.msgsForRound {
		if k >= round {
			num += rms.Size()
		}
	}
	return num, nil
=======
	return rcs.msgsForRound[round].Add(msg)
>>>>>>> a59f2dfb2... Cumulatively count RCs pt 2
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
