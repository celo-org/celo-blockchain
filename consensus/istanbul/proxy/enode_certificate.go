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
	"encoding/hex"

	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/consensus/istanbul"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/rlp"
)

// handleEnodeCertificateMsgFromRemoteVal will handle an enode certificate sent from
// a remote validator.
func (p *proxyEngine) handleEnodeCertificateMsgFromRemoteVal(peer consensus.Peer, payload []byte) (bool, error) {
	logger := p.logger.New("func", "handleEnodeCertificateMsgFromRemoteVal")

	msg := new(istanbul.Message)

	// Verify that this message is created by a legitimate validator before forwarding to the proxied validator (it should
	// be within the validator connection set).
	if err := msg.FromPayload(payload, p.backend.VerifyValidatorConnectionSetSignature); err != nil {
		logger.Error("Got an enodeCertificate message signed by a validator not within the validator connection set.", "err", err)
		return true, istanbul.ErrUnauthorizedAddress
	}

	// Need to forward the message to the proxied validators
	logger.Trace("Forwarding enode certificate message to proxied validators", "from", peer.Node().ID())
	for proxiedValidator := range p.proxiedValidators {
		p.backend.Unicast(proxiedValidator, payload, istanbul.EnodeCertificateMsg)
	}

	// We could add an optimization here where the proxy will save thie enodeCertificate in it's own valEnodeTable.
	// For now, the proxies entry will get updated via a valEnodesShare message from the proxied validator.

	return true, nil
}

// handleEnodeCertificateFromProxiedValidator will handle an enode certifcate message sent from the proxied validator
func (p *proxyEngine) handleEnodeCertificateMsgFromProxiedValidator(peer consensus.Peer, payload []byte) (bool, error) {
	logger := p.logger.New("func", "handleEnodeCertificateMsgFromProxiedValidator")

	logger.Trace("Handling an enode certificate msg from proxied validator")

	msg := new(istanbul.Message)
	// Decode message
	err := msg.FromPayload(payload, istanbul.GetSignatureAddress)
	if err != nil {
		logger.Error("Error in decoding received Enode Certificate message from forward message", "err", err, "payload", hex.EncodeToString(payload))
		return false, err
	}

	// Verify that the sender is from the proxied validator
	if msg.Address != p.config.ProxiedValidatorAddress {
		logger.Error("Unauthorized Enode Certificate message", "sender address", msg.Address, "authorized sender address", p.config.ProxiedValidatorAddress)
		return false, errUnauthorizedMessageFromProxiedValidator
	}

	var enodeCertificate istanbul.EnodeCertificate
	if err := rlp.DecodeBytes(msg.Msg, &enodeCertificate); err != nil {
		logger.Warn("Error in decoding received Istanbul Enode Certificate message content", "err", err, "IstanbulMsg", msg.String())
		return false, err
	}

	enodeCertificateNode, err := enode.ParseV4(enodeCertificate.EnodeURL)
	if err != nil {
		logger.Warn("Malformed v4 node in received Istanbul Enode Certificate message", "enodeCertificate", enodeCertificate, "err", err)
		return false, err
	}

	// If this enode certificate's nodeID is the same as the node's external nodeID, then save it.
	selfNode := p.backend.SelfNode()
	if enodeCertificateNode.ID() == selfNode.ID() {
		enodeCertMsgMap := make(map[enode.ID]*istanbul.EnodeCertMsg)
		enodeCertMsgMap[selfNode.ID()] = &istanbul.EnodeCertMsg{Msg: msg}
		if err := p.backend.SetEnodeCertificateMsgMap(enodeCertMsgMap); err != nil {
			logger.Warn("Error in setting proxy's enode certificate", "err", err, "enodeCertificate", enodeCertificate)
			// Don't drop validators when switching over.
			if err == istanbul.ErrInvalidEnodeCertMsgMapOldVersion {
				return true, nil
			}
			return true, err
		}
	}

	return true, nil
}
