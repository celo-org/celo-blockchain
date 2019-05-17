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
	logger := c.logger.New("state", c.state)

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
	logger := c.logger.New("state", c.state, "from", src.Address().Hex())

	//logger.Info("RC", "msg", msg)

	// Decode ROUND CHANGE message
	var rc *istanbul.Subject
	if err := msg.Decode(&rc); err != nil {
		logger.Error("Failed to decode ROUND CHANGE", "err", err)
		return errInvalidMessage
	}

	// checkMessage ensures that message is for a current or future round.
	if err := c.checkMessage(msgRoundChange, rc.View); err != nil {
		logger.Info("RC checkMessage", "err", err, "msg_view", rc.View, "current_view", c.currentView())
		return err
	}

	// Add the ROUND CHANGE message to its message set.
	if err := c.roundChangeSet.Add(rc.View, msg, src); err != nil {
		logger.Warn("Failed to add round change message", "from", src, "msg", msg, "err", err)
		return err
	}

	cv := c.currentView()

	// TODO - including ourselves?

	//logger.Info("RC2", "rcs", c.roundChangeSet.String(), "qR", c.roundChangeSet.GreatestRoundForThreshold(2*c.valSet.F()+1), "ffR", c.roundChangeSet.GreatestRoundForThreshold(c.valSet.F()+1))

	// See if we now have quorum for a round change. Find the maximal round r (where r > current, or = current if we are already in a round change state)
	// if one exists, s.t we have seen ROUND CHANGE messages from 2f+1 unique other nodes for any round r'>=r,  move to r, and start round immediately.
	if quorumRound := c.roundChangeSet.GreatestRoundForThreshold(2*c.valSet.F() + 1); quorumRound != nil && (c.waitingForRoundChange || quorumRound.Cmp(cv.Round) > 0) {
		c.startNewRound(quorumRound)
		return nil
	}

	if c.waitingForRoundChange {
		// No quoroum yet, but we are already waiting for a new round, so see if we should fast-forward. Find the maximal
		// round r > current, if one exists, s.t we have seen ROUND CHANGE messages from f+1 unique other nodes for any
		// round r'>=r, and send a ROUND CHANGE message for r.
		if ffRound := c.roundChangeSet.GreatestRoundForThreshold(c.valSet.F() + 1); ffRound != nil && ffRound.Cmp(cv.Round) > 0 {
			c.sendRoundChange(ffRound)
		}
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
		least:             nil,
		greatest:          nil,
	}
}

type roundChangeSet struct {
	validatorSet      istanbul.ValidatorSet
	msgsForRound      map[uint64]*messageSet
	latestRoundForVal map[common.Address]uint64
	least             *messageSet
	greatest          *messageSet
	mu                *sync.Mutex
}

// Add adds the ROUND CHANGE message into the set, and returns the total number now
// received from unique validators for that round
func (rcs *roundChangeSet) Add(view *istanbul.View, msg *message, src istanbul.Validator) error {
	rcs.mu.Lock()
	defer rcs.mu.Unlock()

	round := view.Round.Uint64()

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
					rcs.Unlink(rcs.msgsForRound[prevLatestRound])
					delete(rcs.msgsForRound, prevLatestRound)
				}
			}
		}
	}

	rcs.latestRoundForVal[src.Address()] = round

	if rcs.msgsForRound[round] == nil {
		// First time we've seen this round -> new MessageSet.
		rms := newMessageSet(rcs.validatorSet, view)
		rcs.msgsForRound[round] = rms
		rcs.Link(rms)
	}
	return rcs.msgsForRound[round].Add(msg)
}

// Clear deletes the messages with smaller round
func (rcs *roundChangeSet) Clear(round *big.Int) {
	rcs.mu.Lock()
	defer rcs.mu.Unlock()

	for k, rms := range rcs.msgsForRound {
		if len(rms.Values()) == 0 || k < round.Uint64() {
			if rms != nil {
				for _, msg := range rms.Values() {
					if latestRound, ok := rcs.latestRoundForVal[msg.Address]; ok && k == latestRound {
						delete(rcs.latestRoundForVal, msg.Address)
					}
				}
			}
			rcs.Unlink(rms)
			delete(rcs.msgsForRound, k)
		}
	}
}

func (rcs *roundChangeSet) Link(rms *messageSet) {
	if rcs.least == nil {
		rcs.least = rms
		rcs.greatest = rms
		return
	}
	r := rms.View().Round
	rmsAfter := rcs.least
	for rmsAfter != nil {
		if rmsAfter.View().Round.Cmp(r) > 0 {
			if rmsAfter == rcs.least {
				rcs.least = rms
			} else {
				rmsAfter.prev = rms
			}
			rms.next = rmsAfter
		}
		rmsAfter = rmsAfter.next
	}
	if rmsAfter == nil {
		rms.prev = rcs.greatest
		rms.prev.next = rms
		rcs.greatest = rms
	}
}

func (rcs *roundChangeSet) Unlink(rms *messageSet) {
	if rcs.least == rms {
		rcs.least = rms.next
	} else {
		rms.prev.next = rms.next
	}
	if rcs.greatest == rms {
		rcs.greatest = rms.prev
	} else {
		rms.next.prev = rms.prev
	}
}

// GreatestRoundForThreshold returns the greatest round for which the cumulative messages
// in that and subsequent rounds from unique validators exceeds the parameter
func (rcs *roundChangeSet) GreatestRoundForThreshold(threshold int) *big.Int {
	rcs.mu.Lock()
	defer rcs.mu.Unlock()

	acc := 0
	rms := rcs.greatest
	//	fmt.Println("start")
	for rms != nil {
		//		fmt.Printf("r=%v acc=%v rms=%v\n", rms.View().Round, acc, rms.String())
		acc += rms.Size()
		if acc >= threshold {
			//			fmt.Printf("found r=%v acc=%v\n", rms.View().Round, acc)
			return rms.View().Round
		}
		rms = rms.prev
	}
	return nil
}

func (rcs *roundChangeSet) String() string {
	rcs.mu.Lock()
	defer rcs.mu.Unlock()

	msgsForRoundStr := make([]string, 0, len(rcs.msgsForRound))
	rms := rcs.least
	for rms != nil {
		msgsForRoundStr = append(msgsForRoundStr, fmt.Sprintf("%v: %v", rms.View().Round, rms.String()))
		rms = rms.next
	}

	latestRoundForValStr := make([]string, 0, len(rcs.latestRoundForVal))
	for addr, r := range rcs.latestRoundForVal {
		latestRoundForValStr = append(latestRoundForValStr, fmt.Sprintf("%v: %v", addr.String(), r))
	}

	return fmt.Sprintf("RCS by round: {<%v> %v} by val: {<%v> %v}",
		len(rcs.msgsForRound),
		strings.Join(msgsForRoundStr, ", "),
		len(rcs.latestRoundForVal),
		strings.Join(latestRoundForValStr, ", "))
}
