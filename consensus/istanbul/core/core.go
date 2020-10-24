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
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/consensus/istanbul"
	"github.com/ethereum/go-ethereum/consensus/istanbul/validator"
	"github.com/ethereum/go-ethereum/core/types"
	blscrypto "github.com/ethereum/go-ethereum/crypto/bls"
	"github.com/ethereum/go-ethereum/event"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/metrics"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/syndtr/goleveldb/leveldb"
)

type core struct {
	config         *istanbul.Config
	address        common.Address
	logger         log.Logger
	selectProposer istanbul.ProposerSelector

	backend           istanbul.Backend
	events            *event.TypeMuxSubscription
	finalCommittedSub *event.TypeMuxSubscription
	timeoutSub        *event.TypeMuxSubscription

	futurePreprepareTimer         *time.Timer
	resendRoundChangeMessageTimer *time.Timer
	roundChangeTimer              *time.Timer

	validateFn func([]byte, []byte) (common.Address, error)

	backlog MsgBacklog

	rsdb      RoundStateDB
	current   RoundState
	handlerWg *sync.WaitGroup

	roundChangeSet *roundChangeSet

	pendingRequests   *prque.Prque
	pendingRequestsMu *sync.Mutex

	consensusTimestamp time.Time

	// the timer to record consensus duration (from accepting a preprepare to final committed stage)
	consensusTimer metrics.Timer

	msgSigCache map[string]common.Address
}

// New creates an Istanbul consensus core
func New(backend istanbul.Backend, config *istanbul.Config) Engine {
	rsdb, err := newRoundStateDB(config.RoundStateDBPath, nil)
	if err != nil {
		log.Crit("Failed to open RoundStateDB", "err", err)
	}

	c := &core{
		config:             config,
		address:            backend.Address(),
		logger:             log.New(),
		selectProposer:     validator.GetProposerSelector(config.ProposerPolicy),
		handlerWg:          new(sync.WaitGroup),
		backend:            backend,
		pendingRequests:    prque.New(nil),
		pendingRequestsMu:  new(sync.Mutex),
		consensusTimestamp: time.Time{},
		rsdb:               rsdb,
		consensusTimer:     metrics.NewRegisteredTimer("consensus/istanbul/core/consensus", nil),
		msgSigCache:        make(map[string]common.Address),
	}
	msgBacklog := newMsgBacklog(
		func(msg *istanbul.Message) {
			c.sendEvent(backlogEvent{
				msg: msg,
			})
		}, c.checkMessage)
	c.backlog = msgBacklog
	c.validateFn = c.checkValidatorSignature
	c.logger = istanbul.NewIstLogger(
		func() *big.Int {
			if c != nil && c.current != nil {
				return c.current.Round()
			}
			return common.Big0
		},
	)
	return c
}

// ----------------------------------------------------------------------------

func (c *core) SetAddress(address common.Address) {
	c.address = address
	c.logger = log.New("address", address)
}

func (c *core) CurrentView() *istanbul.View {
	if c.current == nil {
		return nil
	}
	return c.current.View()
}

func (c *core) CurrentRoundState() RoundState { return c.current }

func (c *core) ParentCommits() MessageSet {
	if c.current == nil {
		return nil
	}
	return c.current.ParentCommits()
}

func (c *core) ForceRoundChange() {
	// timeout current DesiredView
	view := &istanbul.View{Sequence: c.current.Sequence(), Round: c.current.DesiredRound()}
	c.sendEvent(timeoutAndMoveToNextRoundEvent{view})
}

// PrepareCommittedSeal returns a committed seal for the given hash and round number.
func PrepareCommittedSeal(hash common.Hash, round *big.Int) []byte {
	var buf bytes.Buffer
	buf.Write(hash.Bytes())
	buf.Write(round.Bytes())
	buf.Write([]byte{byte(istanbul.MsgCommit)})
	return buf.Bytes()
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
	newBitmap := new(big.Int).Set(aggregatedSignature.Bitmap)
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
		if newBitmap.Bit(int(valIndex)) == 0 {
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

// Appends the current view and state to the given context.
func (c *core) newLogger(ctx ...interface{}) log.Logger {
	var seq, round, desired *big.Int
	var state State
	var epoch uint64
	if c.current != nil {
		state = c.current.State()
		seq = c.current.Sequence()
		epoch = istanbul.GetEpochNumber(seq.Uint64(), c.config.Epoch)
		round = c.current.Round()
		desired = c.current.DesiredRound()
	} else {
		seq = common.Big0
		epoch = 0
		round = big.NewInt(-1)
		desired = big.NewInt(-1)
	}
	logger := c.logger.New(ctx...)
	return logger.New("cur_seq", seq, "cur_epoch", epoch, "cur_round", round, "des_round", desired, "state", state, "address", c.address)
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
	c.sendMsgTo(msg, istanbul.MapValidatorsToAddresses(c.current.ValidatorSet().List()))
}

// Send message to a specific address
func (c *core) unicast(msg *istanbul.Message, addr common.Address) {
	c.sendMsgTo(msg, []common.Address{addr})
}

func (c *core) sendMsgTo(msg *istanbul.Message, addresses []common.Address) {
	logger := c.newLogger("func", "sendMsgTo")

	payload, err := c.finalizeMessage(msg)
	if err != nil {
		logger.Error("Failed to finalize message", "m", msg, "err", err)
		return
	}

	// Send payload to the specified addresses
	if err := c.backend.Multicast(addresses, payload, istanbul.ConsensusMsg, true); err != nil {
		logger.Error("Failed to send message", "m", msg, "err", err)
		return
	}
}

func (c *core) commit() error {
	logger := c.newLogger("func", "commit", "proposal", c.current.Proposal())
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
			nextRound := new(big.Int).Add(c.current.Round(), common.Big1)
			logger.Warn("Error on commit, waiting for desired round", "reason", "getAggregatedSeal", "err", err, "desired_round", nextRound)
			c.waitForDesiredRound(nextRound)
			return nil
		}
		aggregatedEpochValidatorSetSeal, err := GetAggregatedEpochValidatorSetSeal(proposal.Number().Uint64(), c.config.Epoch, c.current.Commits())
		if err != nil {
			nextRound := new(big.Int).Add(c.current.Round(), common.Big1)
			c.logger.Warn("Error on commit, waiting for desired round", "reason", "GetAggregatedEpochValidatorSetSeal", "err", err, "desired_round", nextRound)
			c.waitForDesiredRound(nextRound)
			return nil
		}
		if err := c.backend.Commit(proposal, aggregatedSeal, aggregatedEpochValidatorSetSeal); err != nil {
			nextRound := new(big.Int).Add(c.current.Round(), common.Big1)
			logger.Warn("Error on commit, waiting for desired round", "reason", "backend.Commit", "err", err, "desired_round", nextRound)
			c.waitForDesiredRound(nextRound)
			return nil
		}
	}

	logger.Info("Committed")
	return nil
}

// GetAggregatedEpochValidatorSetSeal aggregates all the given seals for the SNARK-friendly epoch encoding
// to a bls aggregated signature. Returns an empty signature on a non-epoch block.
func GetAggregatedEpochValidatorSetSeal(blockNumber, epoch uint64, seals MessageSet) (types.IstanbulEpochValidatorSetSeal, error) {
	if !istanbul.IsLastBlockOfEpoch(blockNumber, epoch) {
		return types.IstanbulEpochValidatorSetSeal{}, nil
	}
	bitmap := big.NewInt(0)
	epochSeals := make([][]byte, seals.Size())
	for i, v := range seals.Values() {
		epochSeals[i] = make([]byte, types.IstanbulExtraBlsSignature)

		var commit *istanbul.CommittedSubject
		err := v.Decode(&commit)
		if err != nil {
			return types.IstanbulEpochValidatorSetSeal{}, err
		}
		copy(epochSeals[i], commit.EpochValidatorSetSeal[:])

		j, err := seals.GetAddressIndex(v.Address)
		if err != nil {
			return types.IstanbulEpochValidatorSetSeal{}, err
		}
		bitmap.SetBit(bitmap, int(j), 1)
	}

	asig, err := blscrypto.AggregateSignatures(epochSeals)
	if err != nil {
		return types.IstanbulEpochValidatorSetSeal{}, err
	}
	return types.IstanbulEpochValidatorSetSeal{Bitmap: bitmap, Signature: asig}, nil
}

// Generates the next preprepare request and associated round change certificate
func (c *core) getPreprepareWithRoundChangeCertificate(round *big.Int) (*istanbul.Request, istanbul.RoundChangeCertificate, error) {
	logger := c.newLogger("func", "getPreprepareWithRoundChangeCertificate", "for_round", round)

	roundChangeCertificate, err := c.roundChangeSet.getCertificate(round, c.current.ValidatorSet().MinQuorumSize())
	if err != nil {
		return &istanbul.Request{}, istanbul.RoundChangeCertificate{}, err
	}
	// Start with pending request
	request := c.current.PendingRequest()
	// Search for a valid request in round change messages.
	// The proposal must come from the prepared certificate with the highest round number.
	// All prepared certificates from the same round are assumed to be the same proposal or no proposal (guaranteed by quorum intersection)
	maxRound := big.NewInt(-1)
	for _, message := range roundChangeCertificate.RoundChangeMessages {
		var roundChangeMsg *istanbul.RoundChange
		if err := message.Decode(&roundChangeMsg); err != nil {
			logger.Error("Unexpected: could not decode a previously received RoundChange message")
			return &istanbul.Request{}, istanbul.RoundChangeCertificate{}, err
		}

		if !roundChangeMsg.HasPreparedCertificate() {
			continue
		}

		preparedCertificateView, err := c.getViewFromVerifiedPreparedCertificate(roundChangeMsg.PreparedCertificate)
		if err != nil {
			logger.Error("Unexpected: could not verify a previously received PreparedCertificate message", "src_m", message)
			return &istanbul.Request{}, istanbul.RoundChangeCertificate{}, err
		}

		if preparedCertificateView != nil && preparedCertificateView.Round.Cmp(maxRound) > 0 {
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

	roundChange := false
	// Try to get most recent block
	headBlock, headAuthor := c.backend.GetCurrentHeadBlockAndAuthor()

	logger := c.newLogger("func", "startNewRound", "tag", "stateTransition", "head_block", headBlock.Number().Uint64(), "head_block_hash", headBlock.Hash())

	if headBlock.Number().Cmp(c.current.Sequence()) >= 0 {
		// Update metrics.
		if !c.consensusTimestamp.IsZero() {
			c.consensusTimer.UpdateSince(c.consensusTimestamp)
			c.consensusTimestamp = time.Time{}
		}
		logger.Trace("Catch up to the latest block.")
	} else if headBlock.Number().Cmp(big.NewInt(c.current.Sequence().Int64()-1)) == 0 {
		// Working on the block immediately after the last committed block.
		if round.Cmp(c.current.Round()) == 0 {
			logger.Trace("Already in the desired round.")
			return nil
		} else if round.Cmp(c.current.Round()) < 0 {
			logger.Warn("New round should not be smaller than current round", "new_round", round)
			return nil
		}
		roundChange = true
	} else {
		logger.Warn("New sequence should be larger than current sequence")
		return nil
	}

	// Generate next view and preprepare
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
	c.resetRoundChangeTimer()

	// Some round info will have changed.
	logger = c.newLogger("func", "startNewRound", "tag", "stateTransition", "old_proposer", c.current.Proposer(), "head_block", headBlock.Number().Uint64(), "head_block_hash", headBlock.Hash())
	logger.Debug("New round", "new_round", newView.Round, "new_seq", newView.Sequence, "new_proposer", c.current.Proposer(), "valSet", c.current.ValidatorSet().List(), "size", c.current.ValidatorSet().Size(), "isProposer", c.isProposer())
	return nil
}

// All actions that occur when transitioning to waiting for round change state.
func (c *core) waitForDesiredRound(r *big.Int) error {
	logger := c.newLogger("func", "waitForDesiredRound", "new_desired_round", r)

	// Don't wait for an older round
	if c.current.DesiredRound().Cmp(r) >= 0 {
		logger.Trace("New desired round not greater than current desired round")
		return nil
	}

	logger.Debug("Round Change: Waiting for desired round")

	// Perform all of the updates
	_, headAuthor := c.backend.GetCurrentHeadBlockAndAuthor()
	nextProposer := c.selectProposer(c.current.ValidatorSet(), headAuthor, r.Uint64())
	err := c.current.TransitionToWaitingForNewRound(r, nextProposer)
	if err != nil {
		return err
	}

	c.resetRoundChangeTimer()

	// Process Backlog Messages
	c.backlog.updateState(c.current.View(), c.current.State())

	// Send round change
	c.sendRoundChange()
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
	c.msgSigCache = make(map[string]common.Address)
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
		c.futurePreprepareTimer = nil
	}
}

func (c *core) stopRoundChangeTimer() {
	if c.roundChangeTimer != nil {
		c.roundChangeTimer.Stop()
		c.roundChangeTimer = nil
	}
}

func (c *core) stopResendRoundChangeTimer() {
	if c.resendRoundChangeMessageTimer != nil {
		c.resendRoundChangeMessageTimer.Stop()
		c.resendRoundChangeMessageTimer = nil
	}
}

func (c *core) stopAllTimers() {
	c.stopFuturePreprepareTimer()
	c.stopRoundChangeTimer()
	c.stopResendRoundChangeTimer()
}

func (c *core) getRoundChangeTimeout() time.Duration {
	baseTimeout := time.Duration(c.config.RequestTimeout) * time.Millisecond
	round := c.current.DesiredRound().Uint64()
	if round == 0 {
		// timeout for first round takes into account expected block period
		return baseTimeout + time.Duration(c.config.BlockPeriod)*time.Second
	} else {
		// timeout for subsequent rounds adds an exponential backoff.
		return baseTimeout + time.Duration(math.Pow(2, float64(round)))*time.Duration(c.config.TimeoutBackoffFactor)*time.Millisecond
	}
}

// Reset then set the timer that causes a timeoutAndMoveToNextRoundEvent to be processed.
// This may also reset the timer for the next resendRoundChangeEvent.
func (c *core) resetRoundChangeTimer() {
	// Stop all timers here since all 'resends' happen within the interval of a round's timeout.
	// (Races are handled anyway by checking the seq and desired round haven't changed between
	// submitting and processing events).
	c.stopAllTimers()

	view := &istanbul.View{Sequence: c.current.Sequence(), Round: c.current.DesiredRound()}
	timeout := c.getRoundChangeTimeout()
	c.roundChangeTimer = time.AfterFunc(timeout, func() {
		c.sendEvent(timeoutAndMoveToNextRoundEvent{view})
	})

	if c.current.DesiredRound().Cmp(common.Big1) > 0 {
		logger := c.newLogger("func", "resetRoundChangeTimer")
		logger.Info("Reset timer to do round change", "timeout", timeout)
	}

	c.resetResendRoundChangeTimer()
}

// Reset then, if in StateWaitingForNewRound and on round whose timeout is greater than MinResendRoundChangeTimeout,
// set a timer that is at most MaxResendRoundChangeTimeout that causes a resendRoundChangeEvent to be processed.
func (c *core) resetResendRoundChangeTimer() {
	c.stopResendRoundChangeTimer()
	if c.current.State() == StateWaitingForNewRound {
		minResendTimeout := time.Duration(c.config.MinResendRoundChangeTimeout) * time.Millisecond
		resendTimeout := c.getRoundChangeTimeout() / 2
		if resendTimeout < minResendTimeout {
			return
		}
		maxResendTimeout := time.Duration(c.config.MaxResendRoundChangeTimeout) * time.Millisecond
		if resendTimeout > maxResendTimeout {
			resendTimeout = maxResendTimeout
		}
		view := &istanbul.View{Sequence: c.current.Sequence(), Round: c.current.DesiredRound()}
		c.resendRoundChangeMessageTimer = time.AfterFunc(resendTimeout, func() {
			c.sendEvent(resendRoundChangeEvent{view})
		})

		logger := c.newLogger("func", "resetResendRoundChangeTimer")
		logger.Info("Reset timer to resend RoundChange msg", "timeout", resendTimeout)
	}
}

// Rebroadcast RoundChange message for desired round if still in StateWaitingForNewRound.
// Do not advance desired round. Then clear/reset timer so we may rebroadcast again.
func (c *core) resendRoundChangeMessage() {
	if c.current.State() == StateWaitingForNewRound {
		c.sendRoundChange()
	}
	c.resetResendRoundChangeTimer()
}

func (c *core) checkValidatorSignature(data []byte, sig []byte) (signerAddress common.Address, err error) {
	logger := c.newLogger("func", "checkValidatorSignature")

	cacheKey := msgSigCacheKey{msgData: data, msgSig: sig}
	cacheKeyBytes, err := rlp.EncodeToBytes(cacheKey)
	if err != nil {
		logger.Error("Failed in encoding the cache key", "err", err)
		return common.Address{}, err
	}

	// Check if the cache has the signer
	if signer, ok := c.msgSigCache[string(cacheKeyBytes)]; ok {
		signerAddress = signer
	} else { // Call the istanbul.CheckValidatorSignature function
		signerAddress, err = istanbul.CheckValidatorSignature(c.current.ValidatorSet(), string(cacheKeyBytes), c.msgSigCache, data, sig)
	}

	return signerAddress, err
}

func (c *core) verifyProposal(proposal istanbul.Proposal) (time.Duration, error) {
	logger := c.newLogger("func", "verifyProposal", "proposal", proposal.Hash())
	if verificationStatus, isCached := c.current.GetProposalVerificationStatus(proposal.Hash()); isCached {
		logger.Trace("verification status cache hit", "verificationStatus", verificationStatus)
		return 0, verificationStatus
	} else {
		logger.Trace("verification status cache miss")

		duration, err := c.backend.Verify(proposal)
		logger.Trace("proposal verify return values", "duration", duration, "err", err)

		// Don't cache the verification status if it's a future block
		if err != consensus.ErrFutureBlock {
			c.current.SetProposalVerificationStatus(proposal.Hash(), err)
		}

		return duration, err
	}
}
