package core

import (
	"errors"
	"time"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/consensus"
	"github.com/celo-org/celo-blockchain/consensus/istanbul"
)

func (c *core) sendPreprepareV2(request *istanbul.Request, roundChangeCertificateV2 istanbul.RoundChangeCertificateV2) {
	logger := c.newLogger("func", "sendPreprepareV2")

	// If I'm the proposer and I have the same sequence with the proposal
	if c.current.Sequence().Cmp(request.Proposal.Number()) == 0 && c.isProposer() {
		m := istanbul.NewPreprepareV2Message(&istanbul.PreprepareV2{
			View:                     c.current.View(),
			Proposal:                 request.Proposal,
			RoundChangeCertificateV2: roundChangeCertificateV2,
		}, c.address)
		logger.Debug("Sending preprepareV2", "m", m)
		c.broadcast(m)
	}
}

// ResendPreprepare sends again the preprepare message.
func (c *core) ResendPreprepare() error {
	logger := c.newLogger("func", "resendPreprepare")
	if !c.isProposer() {
		return errors.New("Cant resend preprepare if not proposer")
	}
	st := c.current.State()
	if st != StatePreprepared && st != StatePrepared && st != StateCommitted {
		return errors.New("Cant resend preprepare if not in preprepared, prepared, or committed state")
	}
	if c.isConsensusFork(c.current.Sequence()) {
		m := istanbul.NewPreprepareV2Message(c.current.PreprepareV2(), c.address)
		logger.Debug("Re-Sending preprepare v2", "m", m)
		c.broadcast(m)
	} else {
		m := istanbul.NewPreprepareMessage(c.current.Preprepare(), c.address)
		logger.Debug("Re-Sending preprepare", "m", m)
		c.broadcast(m)
	}
	return nil
}

func (c *core) handlePreprepareV2(msg *istanbul.Message) error {
	defer c.handlePrePrepareTimer.UpdateSince(time.Now())

	logger := c.newLogger("func", "handlePreprepareV2", "tag", "handleMsg", "from", msg.Address)
	logger.Trace("Got preprepareV2 message", "m", msg)

	preprepareV2 := msg.PreprepareV2()
	// Check consensus fork
	if !c.isConsensusFork(preprepareV2.View.Sequence) {
		logger.Info("Received PreprepareV2 for unforked block sequence", "sequence", preprepareV2.View.Sequence.Uint64())
		return errors.New("Received PreprepareV2 for not forked block")
	}

	logger = logger.New("msg_num", preprepareV2.Proposal.Number(), "msg_hash",
		preprepareV2.Proposal.Hash(), "msg_seq", preprepareV2.View.Sequence, "msg_round", preprepareV2.View.Round)

	// Verify that the proposal is for the sequence number of the view we verified.
	if preprepareV2.View.Sequence.Cmp(preprepareV2.Proposal.Number()) != 0 {
		logger.Warn("Received preprepare with invalid block number")
		return errInvalidProposal
	}

	// Ensure we have the same view with the PREPREPARE message.
	if err := c.checkMessage(istanbul.MsgPreprepareV2, preprepareV2.View); err != nil {
		if err == errOldMessage {
			// Get validator set for the given proposal
			valSet := c.backend.ParentBlockValidators(preprepareV2.Proposal)
			prevBlockAuthor := c.backend.AuthorForBlock(preprepareV2.Proposal.Number().Uint64() - 1)
			proposer := c.selectProposer(valSet, prevBlockAuthor, preprepareV2.View.Round.Uint64())

			// We no longer broadcast a COMMIT if this is a PREPREPARE from the correct proposer for an existing block.
			// However, we log a WARN for potential future debugging value.
			if proposer.Address() == msg.Address && c.backend.HasBlock(preprepareV2.Proposal.Hash(), preprepareV2.Proposal.Number()) {
				logger.Warn("Would have sent a commit message for an old block")
				return nil
			}
		}
		// Probably shouldn't errFutureMessage as we should have moved to that round in handleRoundChangeCertificate
		logger.Trace("Check preprepare failed", "err", err)
		return err
	}

	// Check proposer is valid for the message's view (this may be a subsequent round)
	headBlock, headProposer := c.backend.GetCurrentHeadBlockAndAuthor()
	if headBlock == nil {
		logger.Error("Could not determine head proposer")
		return errNotFromProposer
	}
	proposerForMsgRound := c.selectProposer(c.current.ValidatorSet(), headProposer, preprepareV2.View.Round.Uint64())
	if proposerForMsgRound.Address() != msg.Address {
		logger.Warn("Ignore preprepare message from non-proposer", "actual_proposer", proposerForMsgRound.Address())
		return errNotFromProposer
	}

	// If round > 0, handle the ROUND CHANGE certificate. If round = 0, it should not have a ROUND CHANGE certificate
	if preprepareV2.View.Round.Cmp(common.Big0) > 0 {
		if !preprepareV2.HasRoundChangeCertificateV2() {
			logger.Error("Preprepare for non-zero round did not contain a round change certificate.")
			return errMissingRoundChangeCertificate
		}
		// This also moves us to the next round if the certificate is valid.
		err := c.handleRoundChangeCertificateV2(*preprepareV2.View, preprepareV2.RoundChangeCertificateV2, preprepareV2.Proposal)
		if err != nil {
			logger.Warn("Invalid round change certificate with preprepare.", "err", err)
			return err
		}
	} else if preprepareV2.HasRoundChangeCertificateV2() {
		logger.Error("Preprepare for round 0 has a round change certificate.")
		return errInvalidProposal
	}

	// Verify the proposal we received
	if duration, err := c.verifyProposal(preprepareV2.Proposal); err != nil {
		logger.Warn("Failed to verify proposal", "err", err, "duration", duration)
		// if it's a future block, we will handle it again after the duration
		if err == consensus.ErrFutureBlock {
			c.stopFuturePreprepareTimer()
			c.futurePreprepareTimer = time.AfterFunc(duration, func() {
				c.sendEvent(backlogEvent{
					msg: msg,
				})
			})
		}
		return err
	}

	if c.current.State() == StateAcceptRequest {
		logger.Trace("Accepted preprepare v2", "tag", "stateTransition")
		c.consensusTimestamp = time.Now()

		err := c.current.TransitionToPrepreparedV2(preprepareV2)
		if err != nil {
			return err
		}

		// Process Backlog Messages
		c.backlog.updateState(c.current.View(), c.current.State())
		c.sendPrepare()
	}

	return nil
}
