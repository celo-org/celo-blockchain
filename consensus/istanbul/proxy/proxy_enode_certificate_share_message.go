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

// handleProxyEnodeCertificateShareMsg is invoked by the proxy
// It will set it's enode certificate used during the connection handshakes to
// prove that it's enode is the proxied validator's.
func (p *proxyEngine) handleProxyEnodeCertificateShareMsg(peer consensus.Peer, payload []byte) (bool, error) {
	logger := p.logger.New("func", "handleProxyEnodeCertificateShareMsg")

	logger.Debug("Handling an Istanbul ProxyEnodeCertificate message")

	// Verify that it's coming from the proxied peer
	if p.proxiedValidator == nil || p.proxiedValidator.Node().ID() != peer.Node().ID() {
		logger.Warn("Got a proxyEnodeCertificateShare message from a peer that is not the proxy's proxied validator. Ignoring it", "from", peer.Node().ID())
		return false, nil
	}

	msg := new(istanbul.Message)
	// Decode message
	err := msg.FromPayload(payload, istanbul.GetSignatureAddress)
	if err != nil {
		logger.Error("Error in decoding received Istanbul ProxyEnodeCertificateShare message", "err", err, "payload", hex.EncodeToString(payload))
		return true, err
	}

	// Verify that the sender is from the proxied validator
	if msg.Address != p.config.ProxiedValidatorAddress {
		logger.Error("Unauthorized proxyEnodeCertificateShare message", "sender address", msg.Address, "authorized sender address", p.config.ProxiedValidatorAddress)
		return true, errUnauthorizedMessageFromProxiedValidator
	}

	var enodeCertificate istanbul.EnodeCertificate
	if err := rlp.DecodeBytes(msg.Msg, &enodeCertificate); err != nil {
		logger.Warn("Error in decoding received Istanbul Enode Certificate message content", "err", err, "IstanbulMsg", msg.String())
		return true, err
	}

	enodeCertificateNode, err := enode.ParseV4(enodeCertificate.EnodeURL)
	if err != nil {
		logger.Warn("Malformed v4 node in received Istanbul ProxyEnodeCertificateShare message", "enodeCertificate", enodeCertificate, "err", err)
		return true, err
	}

	// Check to make sure that the enodeCertificate's nodeID matches this node's external nodeID.
	// There may be a difference in the URLv4 string because of `discport`, so instead compare the ID.
	selfNode := p.backend.SelfNode()
	if enodeCertificateNode.ID() != selfNode.ID() {
		logger.Warn("Received Istanbul ProxyEnodeCertificateShare message with an incorrect external enode url", "message enode url", enodeCertificate.EnodeURL, "self enode url", selfNode.URLv4())
		return true, errInvalidEnodeCertificate
	}

	if err := p.backend.SetEnodeCertificateMsg(msg); err != nil {
		logger.Warn("Error in setting proxy's enode certificate", "err", err, "enodeCertificate", enodeCertificate)
		return true, err
	}

	return true, nil
}
