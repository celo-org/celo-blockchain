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
	"fmt"
	"math"
	"math/big"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/prque"
	"github.com/ethereum/go-ethereum/consensus/istanbul"
	"github.com/ethereum/go-ethereum/consensus/istanbul/validator"
	"github.com/ethereum/go-ethereum/core/types"
	blscrypto "github.com/ethereum/go-ethereum/crypto/bls"
	"github.com/ethereum/go-ethereum/event"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/metrics"
	"github.com/syndtr/goleveldb/leveldb"
)

// New creates an Istanbul consensus core
func New(backend istanbul.Backend, config *istanbul.Config) Engine {
	rsdb, err := newRoundStateDB(config.RoundStateDBPath, nil)
	if err != nil {
		log.Crit("Failed to open RoundStateDB", "err", err)
	}

	c := &core{
		config:             config,
		address:            backend.Address(),
		logger:             log.New("address", backend.Address()),
		selectProposer:     validator.GetProposerSelector(config.ProposerPolicy),
		handlerWg:          new(sync.WaitGroup),
		backend:            backend,
		pendingRequests:    prque.New(nil),
		pendingRequestsMu:  new(sync.Mutex),
		consensusTimestamp: time.Time{},
		rsdb:               rsdb,
		roundMeter:         metrics.NewRegisteredMeter("consensus/istanbul/core/round", nil),
		sequenceMeter:      metrics.NewRegisteredMeter("consensus/istanbul/core/sequence", nil),
		consensusTimer:     metrics.NewRegisteredTimer("consensus/istanbul/core/consensus", nil),
	}
	msgBacklog := newMsgBacklog(
		func(msg *istanbul.Message) {
			c.sendEvent(backlogEvent{
				msg: msg,
			})
		}, c.checkMessage)
	c.backlog = msgBacklog
	c.validateFn = c.checkValidatorSignature
	return c
}

// ----------------------------------------------------------------------------

type core struct {
	config         *istanbul.Config
	address        common.Address
	logger         log.Logger
	selectProposer istanbul.ProposerSelector

	backend               istanbul.Backend
	events                *event.TypeMuxSubscription
	finalCommittedSub     *event.TypeMuxSubscription
	timeoutSub            *event.TypeMuxSubscription
	futurePreprepareTimer *time.Timer

	validateFn func([]byte, []byte) (common.Address, error)

	backlog MsgBacklog

	rsdb      RoundStateDB
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
	var seq, round, desired *big.Int
	var state State
	if c.current != nil {
		state = c.current.State()
		seq = c.current.Sequence()
		round = c.current.Round()
		desired = c.current.DesiredRound()
	} else {
		seq = common.Big0
		round = big.NewInt(-1)
		desired = big.NewInt(-1)
	}
	logger := c.logger.New(ctx...)
	return logger.New("cur_seq", seq, "cur_round", round, "desired_round", desired, "state", state, "address", c.address)
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

// Send message to all current validators
func (c *core) broadcast(msg *istanbul.Message) {
	c.sendMsgTo(msg, istanbul.GetAddressesFromValidatorList(c.current.ValidatorSet().List()))
}

// Send message to a specific address
func (c *core) unicast(msg *istanbul.Message, addr common.Address) {
	c.sendMsgTo(msg, []common.Address{addr})
}

func (c *core) sendMsgTo(msg *istanbul.Message, addresses []common.Address) {
	logger := c.newLogger("func", "sendMsgTo")

	payload, err := c.finalizeMessage(msg)
	if err != nil {
		logger.Error("Failed to finalize message", "msg", msg, "err", err)
		return
	}

	// Send payload to the specified addresses
	if err := c.backend.BroadcastConsensusMsg(addresses, payload); err != nil {
		logger.Error("Failed to send message", "msg", msg, "err", err)
		return
	}
}

func (c *core) commit() error {
	err := c.current.TransitionToCommitted()
	if err != nil {
		return err
	}

	// Process Backlog Messages
	c.backlog.updateState(c.current.View(), c.current.State())

	proposal := c.current.Proposal()
	if proposal != nil {
		aggregatedSeal, err := GetAggregatedSeal(c.current.Commits(), c.current.Round())
		if err != nil {
			c.waitForDesiredRound(new(big.Int).Add(c.current.Round(), common.Big1))
			return nil
		}
		if err := c.backend.Commit(proposal, aggregatedSeal); err != nil {
			c.waitForDesiredRound(new(big.Int).Add(c.current.Round(), common.Big1))
			return nil
		}
	}
	return nil
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
	roundChangeCertificate, err := c.roundChangeSet.getCertificate(round, c.current.ValidatorSet().MinQuorumSize())
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
func (c *core) startNewRound(round *big.Int) error {
	logger := c.newLogger("func", "startNewRound", "tag", "stateTransition")

	roundChange := false
	// Try to get last proposal
	headBlock, headAuthor := c.backend.GetCurrentHeadBlockAndAuthor()

	if headBlock.Number().Cmp(c.current.Sequence()) >= 0 {
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
			return nil
		} else if round.Cmp(c.current.Round()) < 0 {
			logger.Warn("New round should not be smaller than current round", "lastBlockNumber", headBlock.Number().Int64(), "new_round", round)
			return nil
		}
		roundChange = true
	} else {
		logger.Warn("New sequence should be larger than current sequence", "new_seq", headBlock.Number().Int64())
		return nil
	}

	// Generate next view and pre-prepare
	var newView *istanbul.View
	var roundChangeCertificate istanbul.RoundChangeCertificate
	var request *istanbul.Request

	valSet := c.current.ValidatorSet()
	if roundChange {
		newView = &istanbul.View{
			Sequence: new(big.Int).Set(c.current.Sequence()),
			Round:    new(big.Int).Set(round),
		}

		var err error
		request, roundChangeCertificate, err = c.getPreprepareWithRoundChangeCertificate(round)
		if err != nil {
			logger.Error("Unable to produce round change certificate", "err", err, "new_round", round)
			return nil
		}
	} else {
		request = c.current.PendingRequest()
		newView = &istanbul.View{
			Sequence: new(big.Int).Add(headBlock.Number(), common.Big1),
			Round:    new(big.Int),
		}
		valSet = c.backend.Validators(headBlock)
		c.roundChangeSet = newRoundChangeSet(valSet)
	}

	logger = logger.New("old_proposer", c.current.Proposer())

	// Calculate new proposer
	nextProposer := c.selectProposer(valSet, headAuthor, newView.Round.Uint64())
	err := c.resetRoundState(newView, valSet, nextProposer, roundChange)

	if err != nil {
		return err
	}

	// Process backlog
	c.processPendingRequests()
	c.backlog.updateState(c.current.View(), c.current.State())

	if roundChange && c.isProposer() && request != nil {
		c.sendPreprepare(request, roundChangeCertificate)
	}
	c.newRoundChangeTimer()

	logger.Debug("New round", "new_round", newView.Round, "new_seq", newView.Sequence, "new_proposer", c.current.Proposer(), "valSet", c.current.ValidatorSet().List(), "size", c.current.ValidatorSet().Size(), "isProposer", c.isProposer())
	return nil
}

// All actions that occur when transitioning to waiting for round change state.
func (c *core) waitForDesiredRound(r *big.Int) error {
	logger := c.newLogger("func", "waitForDesiredRound", "old_desired_round", c.current.DesiredRound(), "new_desired_round", r)

	// Don't wait for an older round
	if c.current.DesiredRound().Cmp(r) >= 0 {
		logger.Debug("New desired round not greater than current desired round")
		return nil
	}

	logger.Debug("Round Change: Waiting for desired round")
	desiredView := &istanbul.View{
		Sequence: new(big.Int).Set(c.current.Sequence()),
		Round:    new(big.Int).Set(r),
	}

	// Perform all of the updates
	_, headAuthor := c.backend.GetCurrentHeadBlockAndAuthor()
	nextProposer := c.selectProposer(c.current.ValidatorSet(), headAuthor, r.Uint64())
	err := c.current.TransitionToWaitingForNewRound(r, nextProposer)
	if err != nil {
		return err
	}

	c.newRoundChangeTimerForView(desiredView)

	// Process Backlog Messages
	c.backlog.updateState(c.current.View(), c.current.State())

	// Send round change
	c.sendRoundChange(r)
	return nil
}

func (c *core) createRoundState() (RoundState, error) {
	var roundState RoundState

	logger := c.newLogger("func", "createRoundState")

	if c.current != nil {
		return nil, fmt.Errorf("BUG? Attempting to Start() core with existing c.current")
	}

	headBlock, headAuthor := c.backend.GetCurrentHeadBlockAndAuthor()
	nextSequence := new(big.Int).Add(headBlock.Number(), common.Big1)
	lastStoredView, err := c.rsdb.GetLastView()

	if err != nil && err != leveldb.ErrNotFound {
		logger.Error("Failed to fetch lastStoredView", "err", err)
		return nil, err
	}

	if err == leveldb.ErrNotFound || lastStoredView.Sequence.Cmp(nextSequence) < 0 {
		if err == leveldb.ErrNotFound {
			logger.Info("Creating new RoundState", "reason", "No storedView found")
		} else {
			logger.Info("Creating new RoundState", "reason", "old view", "stored_view", lastStoredView, "requested_seq", nextSequence)
		}
		valSet := c.backend.Validators(headBlock)
		proposer := c.selectProposer(valSet, headAuthor, 0)
		roundState = newRoundState(&istanbul.View{Sequence: nextSequence, Round: common.Big0}, valSet, proposer)
	} else {
		logger.Info("Retrieving stored RoundState", "stored_view", lastStoredView, "requested_seq", nextSequence)
		roundState, err = c.rsdb.GetRoundStateFor(lastStoredView)

		if err != nil {
			logger.Error("Failed to fetch lastStoredRoundState", "err", err)
			return nil, err
		}
	}

	return withSavingDecorator(c.rsdb, roundState), nil
}

// resetRoundState will modify the RoundState to either start a new round or a new sequence
// based on the `roundChange` flag given
func (c *core) resetRoundState(view *istanbul.View, validatorSet istanbul.ValidatorSet, nextProposer istanbul.Validator, roundChange bool) error {
	// TODO(Joshua): Include desired round here.
	if roundChange {
		return c.current.StartNewRound(view.Round, validatorSet, nextProposer)
	}

	// sequence change
	// TODO remove this when we refactor startNewRound()
	if view.Round.Cmp(common.Big0) != 0 {
		c.logger.Crit("BUG: DevError: trying to start a new sequence with round != 0", "wanted_round", view.Round)
	}

	var newParentCommits MessageSet
	lastSubject, err := c.backend.LastSubject()
	if err == nil && c.current.Proposal() != nil && c.current.Proposal().Hash() == lastSubject.Digest && c.current.Round().Cmp(lastSubject.View.Round) == 0 {
		// When changing sequences, if our current Commit messages match the latest block in the chain
		// (i.e. they're for the same block hash and round), we use this sequence's commits as the ParentCommits field
		// in the next round.
		newParentCommits = c.current.Commits()
	} else {
		// Otherwise, we will initialize an empty ParentCommits field with the validator set of the last proposal.
		headBlock := c.backend.GetCurrentHeadBlock()
		newParentCommits = newMessageSet(c.backend.ParentBlockValidators(headBlock))
	}
	return c.current.StartNewSequence(view.Sequence, validatorSet, nextProposer, newParentCommits)

}

func (c *core) isProposer() bool {
	if c.current == nil {
		return false
	}
	return c.current.IsProposer(c.address)
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
		// timeout for subsequent rounds adds an exponential backoff.
		timeout += time.Duration(math.Pow(2, float64(round))) * time.Second
	}

	c.roundChangeTimer = time.AfterFunc(timeout, func() {
		c.sendEvent(timeoutEvent{view})
	})
}

func (c *core) checkValidatorSignature(data []byte, sig []byte) (common.Address, error) {
	return istanbul.CheckValidatorSignature(c.current.ValidatorSet(), data, sig)
}

// PrepareCommittedSeal returns a committed seal for the given hash and round number.
func PrepareCommittedSeal(hash common.Hash, round *big.Int) []byte {
	var buf bytes.Buffer
	buf.Write(hash.Bytes())
	buf.Write(round.Bytes())
	buf.Write([]byte{byte(istanbul.MsgCommit)})
	return buf.Bytes()
}
