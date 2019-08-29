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
	"bytes"
	"encoding/hex"
	"fmt"
	"math"
	"math/big"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/prque"
	"github.com/ethereum/go-ethereum/consensus/istanbul"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto/bls"
	"github.com/ethereum/go-ethereum/event"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/metrics"
)

// New creates an Istanbul consensus core
func New(backend istanbul.Backend, config *istanbul.Config) Engine {
	c := &core{
		config:             config,
		address:            backend.Address(),
		state:              StateAcceptRequest,
		handlerWg:          new(sync.WaitGroup),
		logger:             log.New("address", backend.Address()),
		backend:            backend,
		backlogs:           make(map[istanbul.Validator]*prque.Prque),
		backlogsMu:         new(sync.Mutex),
		pendingRequests:    prque.New(nil),
		pendingRequestsMu:  new(sync.Mutex),
		consensusTimestamp: time.Time{},
		roundMeter:         metrics.NewRegisteredMeter("consensus/istanbul/core/round", nil),
		sequenceMeter:      metrics.NewRegisteredMeter("consensus/istanbul/core/sequence", nil),
		consensusTimer:     metrics.NewRegisteredTimer("consensus/istanbul/core/consensus", nil),
	}
	c.validateFn = c.checkValidatorSignature
	return c
}

// ----------------------------------------------------------------------------

type core struct {
	config  *istanbul.Config
	address common.Address
	state   State
	logger  log.Logger

	backend               istanbul.Backend
	events                *event.TypeMuxSubscription
	finalCommittedSub     *event.TypeMuxSubscription
	timeoutSub            *event.TypeMuxSubscription
	futurePreprepareTimer *time.Timer

	valSet             istanbul.ValidatorSet
	waitingForNewRound bool
	validateFn         func([]byte, []byte) (common.Address, error)

	backlogs   map[istanbul.Validator]*prque.Prque
	backlogsMu *sync.Mutex

	current   *roundState
	handlerWg *sync.WaitGroup

	roundChangeSet   *roundChangeSet
	roundChangeTimer *time.Timer

	pendingRequests   *prque.Prque
	pendingRequestsMu *sync.Mutex

	consensusTimestamp time.Time
	// the meter to record the round change rate
	roundMeter metrics.Meter
	// the meter to record the sequence update rate
	sequenceMeter metrics.Meter
	// the timer to record consensus duration (from accepting a preprepare to final committed stage)
	consensusTimer metrics.Timer
}

func (c *core) SetAddress(address common.Address) {
	c.address = address
	c.logger = log.New("address", address)
}

func (c *core) finalizeMessage(msg *istanbul.Message) ([]byte, error) {
	var err error
	// Add sender address
	msg.Address = c.Address()

	// Add proof of consensus
	msg.CommittedSeal = []byte{}
	// Assign the CommittedSeal if it's a COMMIT message and proposal is not nil
	if msg.Code == istanbul.MsgCommit && c.current.Proposal() != nil {
		seal := PrepareCommittedSeal(c.current.Proposal().Hash())
		msg.CommittedSeal, err = c.backend.SignBlockHeader(seal)
		if err != nil {
			return nil, err
		}
	}

	// Sign message
	data, err := msg.PayloadNoSig()
	if err != nil {
		return nil, err
	}
	msg.Signature, err = c.backend.Sign(data)
	if err != nil {
		return nil, err
	}

	// Convert to payload
	payload, err := msg.Payload()
	if err != nil {
		return nil, err
	}

	return payload, nil
}

func (c *core) broadcast(msg *istanbul.Message) {
	logger := c.logger.New("state", c.state, "cur_round", c.current.Round(), "cur_seq", c.current.Sequence())

	payload, err := c.finalizeMessage(msg)
	if err != nil {
		logger.Error("Failed to finalize message", "msg", msg, "err", err)
		return
	}

	// Broadcast payload
	if err = c.backend.Broadcast(c.valSet, payload); err != nil {
		logger.Error("Failed to broadcast message", "msg", msg, "err", err)
		return
	}
}

func (c *core) currentView() *istanbul.View {
	return &istanbul.View{
		Sequence: new(big.Int).Set(c.current.Sequence()),
		Round:    new(big.Int).Set(c.current.Round()),
	}
}

func (c *core) isProposer() bool {
	v := c.valSet
	if v == nil {
		return false
	}
	return v.IsProposer(c.backend.Address())
}

func (c *core) commit() {
	c.setState(StateCommitted)

	proposal := c.current.Proposal()
	bitmap := big.NewInt(0)
	publicKeys := [][]byte{}
	if proposal != nil {
		committedSeals := make([][]byte, c.current.Commits.Size())
		for i, v := range c.current.Commits.Values() {
			committedSeals[i] = make([]byte, types.IstanbulExtraCommittedSeal)
			copy(committedSeals[i][:], v.CommittedSeal[:])
			j, err := c.current.Commits.GetAddressIndex(v.Address)
			if err != nil {
				panic(fmt.Sprintf("commit: couldn't get address index for address %s", hex.EncodeToString(v.Address[:])))
			}
			publicKey, err := c.current.Commits.GetAddressPublicKey(v.Address)
			if err != nil {
				panic(fmt.Sprintf("commit: couldn't get public key for address %s", hex.EncodeToString(v.Address[:])))
			}

			publicKeys = append(publicKeys, publicKey)

			bitmap.SetBit(bitmap, int(j), 1)
		}
		asig, err := blscrypto.AggregateSignatures(committedSeals)
		if err != nil {
			panic("commit: couldn't aggregate signatures which have been verified in the commit phase")
		}

		if err := c.backend.Commit(proposal, bitmap, asig); err != nil {
			c.sendNextRoundChange()
			return
		}
	}
}

// startNewRound starts a new round. if round equals to 0, it means to starts a new sequence
func (c *core) startNewRound(round *big.Int) {
	var logger log.Logger
	if c.current == nil {
		logger = c.logger.New("cur_round", -1, "cur_seq", 0, "next_round", 0, "next_seq", 0, "func", "startNewRound", "tag", "stateTransition")
	} else {
		logger = c.logger.New("cur_round", c.current.Round(), "cur_seq", c.current.Sequence(), "func", "startNewRound", "tag", "stateTransition")
	}

	roundChange := false
	// Try to get last proposal
	lastProposal, lastProposer := c.backend.LastProposal()
	if c.current == nil {
		logger.Trace("Start the initial round")
	} else if lastProposal.Number().Cmp(c.current.Sequence()) >= 0 {
		// Want to be working on the block 1 beyond the last committed block.
		diff := new(big.Int).Sub(lastProposal.Number(), c.current.Sequence())
		c.sequenceMeter.Mark(new(big.Int).Add(diff, common.Big1).Int64())

		if !c.consensusTimestamp.IsZero() {
			c.consensusTimer.UpdateSince(c.consensusTimestamp)
			c.consensusTimestamp = time.Time{}
		}
		logger.Trace("Catch up to the latest proposal.", "number", lastProposal.Number().Uint64(), "hash", lastProposal.Hash())
	} else if lastProposal.Number().Cmp(big.NewInt(c.current.Sequence().Int64()-1)) == 0 {
		// Working on the block immediately after the last committed block.
		if round.Cmp(c.current.Round()) == 0 {
			logger.Trace("Already in the desired round.")
			return
		} else if round.Cmp(c.current.Round()) < 0 {
			logger.Warn("New round should not be smaller than current round", "lastProposalNumber", lastProposal.Number().Int64(), "new_round", round)
			return
		}
		roundChange = true
	} else {
		logger.Warn("New sequence should be larger than current sequence", "new_seq", lastProposal.Number().Int64())
		return
	}

	var newView *istanbul.View
	var roundChangeCertificate istanbul.RoundChangeCertificate
	// Go to waiting state if we can't produce a round change certificate.
	c.waitingForNewRound = false
	if roundChange {
		newView = &istanbul.View{
			Sequence: new(big.Int).Set(c.current.Sequence()),
			Round:    new(big.Int).Set(round),
		}

		var err error
		roundChangeCertificate, err = c.roundChangeSet.getCertificate(round, c.valSet.MinQuorumSize())
		if err != nil {
			logger.Error("Unable to produce round change certificate", "err", err, "new_round", round)
			c.waitingForNewRound = true
		}
	} else {
		newView = &istanbul.View{
			Sequence: new(big.Int).Add(lastProposal.Number(), common.Big1),
			Round:    new(big.Int),
		}
		c.valSet = c.backend.Validators(lastProposal)
	}

	// Update logger
	logger = logger.New("old_proposer", c.valSet.GetProposer())
	// Clear invalid ROUND CHANGE messages
	c.roundChangeSet = newRoundChangeSet(c.valSet)
	// New snapshot for new round
	c.updateRoundState(newView, c.valSet, roundChange)
	// Calculate new proposer
	c.valSet.CalcProposer(lastProposer, newView.Round.Uint64())
	c.setState(StateAcceptRequest)
	if roundChange && c.isProposer() && c.current != nil && !c.waitingForNewRound {
		// Start with pending request
		request := c.current.pendingRequest
		// Search for a valid request in round change messages.
		// The round change certificate should be generated such that it is consistent in it's proposed subject.
		for _, message := range roundChangeCertificate.RoundChangeMessages {
			var roundChangeMsg *istanbul.RoundChange
			if err := message.Decode(&roundChangeMsg); err != nil {
				logger.Error("Failed to decode ROUND CHANGE in certificate. Skipping to next ROUND CHANGE message.", "err", err)
				continue
			}
			if roundChangeMsg.HasPreparedCertificate() {
				request = &istanbul.Request{
					Proposal: roundChangeMsg.PreparedCertificate.Proposal,
				}
				break
			}
		}
		if request != nil {
			c.sendPreprepare(request, roundChangeCertificate)
		}
	}
	c.newRoundChangeTimer()
	// If going into an immediate waiting state, need round change timer for the next round.
	if c.waitingForNewRound {
		c.waitForNewRound()
	}

	logger.Debug("New round", "new_round", newView.Round, "new_seq", newView.Sequence, "new_proposer", c.valSet.GetProposer(), "valSet", c.valSet.List(), "size", c.valSet.Size(), "isProposer", c.isProposer())
}

func (c *core) waitForNewRound() {
	nextView := &istanbul.View{
		Sequence: new(big.Int).Set(c.current.Sequence()),
		Round:    new(big.Int).Add(c.current.Round(), common.Big1),
	}
	c.newRoundChangeTimerForView(nextView)
	c.waitingForNewRound = true
}

func (c *core) updateRoundState(view *istanbul.View, validatorSet istanbul.ValidatorSet, roundChange bool) {
	if roundChange && c.current != nil {
		c.current = newRoundState(view, validatorSet, nil, c.current.pendingRequest, c.current.preparedCertificate, c.backend.HasBadProposal)
	} else {
		c.current = newRoundState(view, validatorSet, nil, nil, istanbul.EmptyPreparedCertificate(), c.backend.HasBadProposal)
	}
}

func (c *core) setState(state State) {
	if c.state != state {
		c.state = state
	}
	if state == StateAcceptRequest {
		c.processPendingRequests()
	}
	c.processBacklog()
}

func (c *core) Address() common.Address {
	return c.address
}

func (c *core) stopFuturePreprepareTimer() {
	if c.futurePreprepareTimer != nil {
		c.futurePreprepareTimer.Stop()
	}
}

func (c *core) stopTimer() {
	c.stopFuturePreprepareTimer()
	if c.roundChangeTimer != nil {
		c.roundChangeTimer.Stop()
	}
}

func (c *core) newRoundChangeTimer() {
	c.newRoundChangeTimerForView(c.currentView())
}

func (c *core) newRoundChangeTimerForView(view *istanbul.View) {
	c.stopTimer()

	// set timeout based on the round number
	timeout := time.Duration(c.config.RequestTimeout) * time.Millisecond
	round := view.Round.Uint64()
	if round > 0 {
		timeout += time.Duration(math.Pow(2, float64(round))) * time.Second
	}

	c.roundChangeTimer = time.AfterFunc(timeout, func() {
		c.sendEvent(timeoutEvent{view})
	})
}

func (c *core) checkValidatorSignature(data []byte, sig []byte) (common.Address, error) {
	return istanbul.CheckValidatorSignature(c.valSet, data, sig)
}

// PrepareCommittedSeal returns a committed seal for the given hash
func PrepareCommittedSeal(hash common.Hash) []byte {
	var buf bytes.Buffer
	buf.Write(hash.Bytes())
	buf.Write([]byte{byte(istanbul.MsgCommit)})
	return buf.Bytes()
}
