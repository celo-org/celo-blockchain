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

// handleEnodeCertificateMsg is invoked by the proxy
// It will verify that the message is from a validator that is within the
// validator connection set and then forward it to the proxied validator.
func (p *proxy) handleEnodeCertificateMsg(peer consensus.Peer, payload []byte) (bool, error) {
	logger := p.logger.New("func", "handleEnodeCertificateMsg")

	// Verify that this message is not from the proxied validator
	if p.proxiedValidator != nil && peer.Node().ID() == p.proxiedValidator.Node().ID() {
		logger.Warn("Got a enodeCertficate message from the proxied validator. Ignoring it", "from", peer.Node().ID())
		return false, nil
	}

	msg := new(istanbul.Message)

	// Verify that this message is created by a legitimate validator before forwarding to the proxied validator (it should
	// be within the validator connection set).
	if err := msg.FromPayload(payload, p.backend.VerifyValidatorConnectionSetSignature); err != nil {
		logger.Error("Got an enodeCertificate message signed by a validator not within the validator connection set.", "err", err)
		return true, istanbul.ErrUnauthorizedAddress
	}

	// Need to forward the message to the proxied validator
	logger.Trace("Forwarding consensus message to proxied validator", "from", peer.Node().ID())
	if p.proxiedValidator != nil {
		go p.proxiedValidator.Send(istanbul.EnodeCertificateMsg, payload)
	}

	// We could add an optimization here where the proxy will save thie enodeCertificate in it's own valEnodeTable.
	// For now, the proxies entry will get updated via a valEnodesShare message from the proxied validator.

	return true, nil
}

// SendEnodeCertificateMsgToProxiedValidator will send the enode certificate message to
// the proxied validator.
func (p *proxy) SendEnodeCertificateMsgToProxiedValidator(msg *istanbul.Message) error {
	logger := p.logger.New("func", "SendEnodeCertificateMsgToProxiedValidator")
	if p.proxiedValidator != nil {
		payload, err := msg.Payload()
		if err != nil {
			logger.Error("Error getting payload of enode certificate message", "err", err)
			return err
		}
		return p.proxiedValidator.Send(istanbul.EnodeCertificateMsg, payload)
	} else {
		logger.Warn("Proxy has no connected proxied validator.  Not sending enode certificate message.")
		return nil
	}
}
