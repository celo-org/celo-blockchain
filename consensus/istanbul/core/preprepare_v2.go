package core

import (
	"errors"

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

// ResendPreprepareV2 sends again the preprepare message.
func (c *core) ResendPreprepareV2() error {
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
