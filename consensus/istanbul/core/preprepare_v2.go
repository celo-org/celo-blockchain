package core

import "github.com/celo-org/celo-blockchain/consensus/istanbul"

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
