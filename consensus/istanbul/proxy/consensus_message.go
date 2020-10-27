// Copyright 2017 The Celo Authors
// This file is part of the celo library.
//
// The celo library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The celo library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the celo library. If not, see <http://www.gnu.org/licenses/>.

package proxy

import (
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/consensus/istanbul"
)

// handleConsensusMsg is invoked by the proxy to forward valid consensus messages to
// it's proxied validator
func (p *proxyEngine) handleConsensusMsg(peer consensus.Peer, payload []byte) (bool, error) {
	logger := p.logger.New("func", "handleConsensusMsg")

	// Verify that this message is not from the proxied validator
	p.proxiedValidatorsMu.RLock()
	defer p.proxiedValidatorsMu.RUnlock()
	if ok := p.proxiedValidatorIDs[peer.Node().ID()]; ok {
		logger.Warn("Got a consensus message from the proxied validator. Ignoring it", "from", peer.Node().ID())
		return false, nil
	}

	msg := new(istanbul.Message)

	// Verify that this message is created by a legitimate validator before forwarding to the proxied validator.
	if err := msg.FromPayload(payload, p.backend.VerifyPendingBlockValidatorSignature); err != nil {
		logger.Error("Got a consensus message signed by a validator not within the pending block validator set.", "err", err)
		return true, istanbul.ErrUnauthorizedAddress
	}

	// Need to forward the message to the proxied validators
	logger.Trace("Forwarding consensus message to proxied validators", "from", peer.Node().ID())
	for proxiedValidator := range p.proxiedValidators {
		p.backend.Unicast(proxiedValidator, payload, istanbul.ConsensusMsg)
	}

	return true, nil
}
