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
	"math"
	"math/big"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/prque"
	"github.com/ethereum/go-ethereum/consensus/istanbul"

	"github.com/ethereum/go-ethereum/core/types"
	blscrypto "github.com/ethereum/go-ethereum/crypto/bls"
	"github.com/ethereum/go-ethereum/event"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/metrics"
)

// New creates an Istanbul consensus core
func New(backend istanbul.Backend, config *istanbul.Config) Engine {
	c := &core{
		config:             config,
		address:            backend.Address(),
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
	logger  log.Logger

	backend               istanbul.Backend
	events                *event.TypeMuxSubscription
	finalCommittedSub     *event.TypeMuxSubscription
	timeoutSub            *event.TypeMuxSubscription
	futurePreprepareTimer *time.Timer

	valSet     istanbul.ValidatorSet
	validateFn func([]byte, []byte) (common.Address, error)

	backlogs   map[istanbul.Validator]*prque.Prque
	backlogsMu *sync.Mutex

	current   RoundState
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

// Appends the current view and state to the given context.
func (c *core) newLogger(ctx ...interface{}) log.Logger {
	var seq, round *big.Int
	var state State
	if c.current != nil {
		state = c.current.State()
		seq = c.current.Sequence()
		round = c.current.Round()
	} else {
		seq = common.Big0
		round = big.NewInt(-1)
	}
	logger := c.logger.New(ctx...)
	return logger.New("cur_seq", seq, "cur_round", round, "state", state, "address", c.address)
}

func (c *core) SetAddress(address common.Address) {
	c.address = address
	c.logger = log.New("address", address)
}

func (c *core) finalizeMessage(msg *istanbul.Message) ([]byte, error) {
	// Add sender address
	msg.Address = c.address

	if err := msg.Sign(c.backend.Sign); err != nil {
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
	logger := c.newLogger("func", "broadcast")

	payload, err := c.finalizeMessage(msg)
	if err != nil {
		logger.Error("Failed to finalize message", "msg", msg, "err", err)
		return
	}

	// Broadcast payload
	if err := c.backend.BroadcastConsensusMsg(istanbul.GetAddressesFromValidatorList(c.valSet.FilteredList()), payload); err != nil {
		logger.Error("Failed to broadcast message", "msg", msg, "err", err)
		return
	}
}

func (c *core) commit() {
	c.current.TransitionToCommited()

	// Process Backlog Messages
	c.processBacklog()

	proposal := c.current.Proposal()
	if proposal != nil {
		aggregatedSeal, err := GetAggregatedSeal(c.current.Commits(), c.current.Round())
		if err != nil {
			c.sendNextRoundChange()
			return
		}
		if err := c.backend.Commit(proposal, aggregatedSeal); err != nil {
			c.sendNextRoundChange()
			return
		}
	}
}

// GetAggregatedSeal aggregates all the given seals for a given message set to a bls aggregated
// signature and bitmap
func GetAggregatedSeal(seals MessageSet, round *big.Int) (types.IstanbulAggregatedSeal, error) {
	bitmap := big.NewInt(0)
	committedSeals := make([][]byte, seals.Size())
	for i, v := range seals.Values() {
		committedSeals[i] = make([]byte, types.IstanbulExtraBlsSignature)

		var commit *istanbul.CommittedSubject
		err := v.Decode(&commit)
		if err != nil {
			return types.IstanbulAggregatedSeal{}, err
		}
		copy(committedSeals[i][:], commit.CommittedSeal[:])

		j, err := seals.GetAddressIndex(v.Address)
		if err != nil {
			return types.IstanbulAggregatedSeal{}, err
		}
		bitmap.SetBit(bitmap, int(j), 1)
	}

	asig, err := blscrypto.AggregateSignatures(committedSeals)
	if err != nil {
		return types.IstanbulAggregatedSeal{}, err
	}
	return types.IstanbulAggregatedSeal{Bitmap: bitmap, Signature: asig, Round: round}, nil
}

// UnionOfSeals combines a BLS aggregated signature with an array of signatures. Accounts for
// double aggregating the same signature by only adding aggregating if the
// validator was not found in the previous bitmap.
// This function assumes that the provided seals' validator set is the same one
// which produced the provided bitmap
func UnionOfSeals(aggregatedSignature types.IstanbulAggregatedSeal, seals MessageSet) (types.IstanbulAggregatedSeal, error) {
	// TODO(asa): Check for round equality...
	// Check who already has signed the message
	newBitmap := aggregatedSignature.Bitmap
	committedSeals := [][]byte{}
	committedSeals = append(committedSeals, aggregatedSignature.Signature)
	for _, v := range seals.Values() {
		valIndex, err := seals.GetAddressIndex(v.Address)
		if err != nil {
			return types.IstanbulAggregatedSeal{}, err
		}

		var commit *istanbul.CommittedSubject
		err = v.Decode(&commit)
		if err != nil {
			return types.IstanbulAggregatedSeal{}, err
		}

		// if the bit was not set, this means we should add this signature to
		// the batch
		if aggregatedSignature.Bitmap.Bit(int(valIndex)) == 0 {
			newBitmap.SetBit(newBitmap, (int(valIndex)), 1)
			committedSeals = append(committedSeals, commit.CommittedSeal)
		}
	}

	asig, err := blscrypto.AggregateSignatures(committedSeals)
	if err != nil {
		return types.IstanbulAggregatedSeal{}, err
	}

	return types.IstanbulAggregatedSeal{
		Bitmap:    newBitmap,
		Signature: asig,
		Round:     aggregatedSignature.Round,
	}, nil
}

// Generates the next preprepare request and associated round change certificate
func (c *core) getPreprepareWithRoundChangeCertificate(round *big.Int) (*istanbul.Request, istanbul.RoundChangeCertificate, error) {
	roundChangeCertificate, err := c.roundChangeSet.getCertificate(round, c.valSet.MinQuorumSize())
	if err != nil {
		return &istanbul.Request{}, istanbul.RoundChangeCertificate{}, err
	}
	// Start with pending request
	request := c.current.PendingRequest()
	// Search for a valid request in round change messages.
	// The proposal must come from the prepared certificate with the highest round number.
	// All pre-prepared certificates from the same round are assumed to be the same proposal or no proposal (guaranteed by quorum intersection)
	maxRound := big.NewInt(-1)
	for _, message := range roundChangeCertificate.RoundChangeMessages {
		var roundChangeMsg *istanbul.RoundChange
		if err := message.Decode(&roundChangeMsg); err != nil {
			continue
		}
		preparedCertificateView := roundChangeMsg.PreparedCertificate.View()
		if roundChangeMsg.HasPreparedCertificate() && preparedCertificateView != nil && preparedCertificateView.Round.Cmp(maxRound) > 0 {
			maxRound = preparedCertificateView.Round
			request = &istanbul.Request{
				Proposal: roundChangeMsg.PreparedCertificate.Proposal,
			}
		}
	}
	return request, roundChangeCertificate, nil
}

// startNewRound starts a new round. if round equals to 0, it means to starts a new sequence
func (c *core) startNewRound(round *big.Int) {
	logger := c.newLogger("func", "startNewRound", "tag", "stateTransition")

	roundChange := false
	// Try to get last proposal
	headBlock, headAuthor := c.backend.GetCurrentHeadBlockAndAuthor()
	if c.current == nil {
		logger.Trace("Start the initial round")
	} else if headBlock.Number().Cmp(c.current.Sequence()) >= 0 {
		// Want to be working on the block 1 beyond the last committed block.
		diff := new(big.Int).Sub(headBlock.Number(), c.current.Sequence())
		c.sequenceMeter.Mark(new(big.Int).Add(diff, common.Big1).Int64())

		if !c.consensusTimestamp.IsZero() {
			c.consensusTimer.UpdateSince(c.consensusTimestamp)
			c.consensusTimestamp = time.Time{}
		}
		logger.Trace("Catch up to the latest proposal.", "number", headBlock.Number().Uint64(), "hash", headBlock.Hash())
	} else if headBlock.Number().Cmp(big.NewInt(c.current.Sequence().Int64()-1)) == 0 {
		// Working on the block immediately after the last committed block.
		if round.Cmp(c.current.Round()) == 0 {
			logger.Trace("Already in the desired round.")
			return
		} else if round.Cmp(c.current.Round()) < 0 {
			logger.Warn("New round should not be smaller than current round", "lastBlockNumber", headBlock.Number().Int64(), "new_round", round)
			return
		}
		roundChange = true
	} else {
		logger.Warn("New sequence should be larger than current sequence", "new_seq", headBlock.Number().Int64())
		return
	}

	// Generate next view and pre-prepare
	var newView *istanbul.View
	var roundChangeCertificate istanbul.RoundChangeCertificate
	var request *istanbul.Request
	if roundChange {
		newView = &istanbul.View{
			Sequence: new(big.Int).Set(c.current.Sequence()),
			Round:    new(big.Int).Set(round),
		}

		var err error
		request, roundChangeCertificate, err = c.getPreprepareWithRoundChangeCertificate(round)
		if err != nil {
			logger.Error("Unable to produce round change certificate", "err", err, "new_round", round)
			return
		}
	} else {
		if c.current != nil {
			request = c.current.PendingRequest()
		}
		newView = &istanbul.View{
			Sequence: new(big.Int).Add(headBlock.Number(), common.Big1),
			Round:    new(big.Int),
		}
		c.valSet = c.backend.Validators(headBlock)
		c.roundChangeSet = newRoundChangeSet(c.valSet)
	}

	// Update logger
	logger = logger.New("old_proposer", c.valSet.GetProposer())
	// Calculate new proposer
	c.valSet.CalcProposer(headAuthor, newView.Round.Uint64())
	// New snapshot for new round
	c.resetRoundState(newView, c.valSet, roundChange)
	c.processPendingRequests()
	c.processBacklog()

	if roundChange && c.current != nil && c.isProposer() && request != nil {
		c.sendPreprepare(request, roundChangeCertificate)
	}
	c.newRoundChangeTimer()

	logger.Debug("New round", "new_round", newView.Round, "new_seq", newView.Sequence, "new_proposer", c.valSet.GetProposer(), "valSet", c.valSet.List(), "size", c.valSet.Size(), "isProposer", c.isProposer())
}

// All actions that occur when transitioning to waiting for round change state.
func (c *core) waitForDesiredRound(r *big.Int) {
	logger := c.newLogger("func", "waitForDesiredRound", "old_desired_round", c.current.DesiredRound(), "new_desired_round", r)

	// Don't wait for an older round
	if c.current.DesiredRound().Cmp(r) >= 0 {
		logger.Debug("New desired round not greater than current desired round")
		return
	}

	logger.Debug("Waiting for desired round")
	desiredView := &istanbul.View{
		Sequence: new(big.Int).Set(c.current.Sequence()),
		Round:    new(big.Int).Set(r),
	}
	// Perform all of the updates
	c.current.TransitionToWaitingForNewRound(r)

	_, headAuthor := c.backend.GetCurrentHeadBlockAndAuthor()
	c.valSet.CalcProposer(headAuthor, r.Uint64())
	c.newRoundChangeTimerForView(desiredView)

	// Process Backlog Messages
	c.processBacklog()

	// Send round change
	c.sendRoundChange(r)
}

func (c *core) resetRoundState(view *istanbul.View, validatorSet istanbul.ValidatorSet, roundChange bool) {
	// TODO(Joshua): Include desired round here.
	if c.current == nil {
		// When the current round is nil, we must start with the current validator set in the parent commits
		// either `validatorSet` or `backend.Validators(lastProposal)` works here
		c.current = newRoundState(view, validatorSet)
	} else if roundChange {
		c.current.StartNewRound(view.Round, validatorSet)
	} else {
		// sequence change

		var newParentCommits MessageSet
		lastSubject, err := c.backend.LastSubject()
		if err != nil && c.current.Proposal() != nil && c.current.Proposal().Hash() == lastSubject.Digest && c.current.Round().Cmp(lastSubject.View.Round) == 0 {
			// When changing sequences, if our current Commit messages match the latest block in the chain
			// (i.e. they're for the same block hash and round), we use this sequence's commits as the ParentCommits field
			// in the next round.
			newParentCommits = c.current.Commits()
		} else {
			// Otherwise, we will initialize an empty ParentCommits field with the validator set of the last proposal.
			headBlock := c.backend.GetCurrentHeadBlock()
			newParentCommits = newMessageSet(c.backend.ParentBlockValidators(headBlock))
		}
		c.current.StartNewSequence(view, validatorSet, newParentCommits)
	}
}

func (c *core) isProposer() bool {
	if c.valSet == nil {
		return false
	}
	return c.valSet.IsProposer(c.address)
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
	c.newRoundChangeTimerForView(c.current.View())
}

func (c *core) newRoundChangeTimerForView(view *istanbul.View) {
	c.stopTimer()

	timeout := time.Duration(c.config.RequestTimeout) * time.Millisecond
	round := view.Round.Uint64()
	if round == 0 {
		// timeout for first round takes into account expected block period
		timeout += time.Duration(c.config.BlockPeriod) * time.Second
	} else {
		// timeout for subsequent rounds adds an exponential backup, capped at 2**5 = 32s
		timeout += time.Duration(math.Pow(2, math.Min(float64(round), 5.))) * time.Second
	}

	c.roundChangeTimer = time.AfterFunc(timeout, func() {
		c.sendEvent(timeoutEvent{view})
	})
}

func (c *core) checkValidatorSignature(data []byte, sig []byte) (common.Address, error) {
	return istanbul.CheckValidatorSignature(c.valSet, data, sig)
}

// PrepareCommittedSeal returns a committed seal for the given hash and round number.
func PrepareCommittedSeal(hash common.Hash, round *big.Int) []byte {
	var buf bytes.Buffer
	buf.Write(hash.Bytes())
	buf.Write(round.Bytes())
	buf.Write([]byte{byte(istanbul.MsgCommit)})
	return buf.Bytes()
}
