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
	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/consensus"
	"github.com/celo-org/celo-blockchain/consensus/istanbul"
)

func (pv *proxiedValidatorEngine) sendForwardMsg(ps *proxySet, destAddresses []common.Address, ethMsgCode uint64, payload []byte) error {
	logger := pv.logger.New("func", "SendForwardMsg")

	logger.Info("Sending forward msg", "ethMsgCode", ethMsgCode, "destAddresses", common.ConvertToStringSlice(destAddresses))

	// Send the forward messages to the proxies
	for _, proxy := range ps.proxiesByID {
		if proxy.IsPeered() {

			// Convert the message to a fwdMessage
			msg := istanbul.NewForwardMessage(&istanbul.ForwardMessage{
				Code:          ethMsgCode,
				DestAddresses: destAddresses,
				Msg:           payload,
			}, pv.backend.Address())

			// Sign the message
			if err := msg.Sign(pv.backend.Sign); err != nil {
				logger.Error("Error in signing an Istanbul Forward Message", "ForwardMsg", msg.String(), "err", err)
				return err
			}

			fwdMsgPayload, err := msg.Payload()
			if err != nil {
				return err
			}

			pv.backend.Unicast(proxy.peer, fwdMsgPayload, istanbul.FwdMsg)
		}
	}

	return nil
}

func (p *proxyEngine) handleForwardMsg(peer consensus.Peer, payload []byte) (bool, error) {
	logger := p.logger.New("func", "HandleForwardMsg")

	logger.Trace("Handling a forward message")

	// Verify that it's coming from the proxied validator
	p.proxiedValidatorsMu.RLock()
	defer p.proxiedValidatorsMu.RUnlock()
	if ok := p.proxiedValidatorIDs[peer.Node().ID()]; !ok {
		logger.Warn("Got a forward consensus message from a peer that is not the proxy's proxied validator. Ignoring it", "from", peer.Node().ID())
		return false, nil
	}

	istMsg := new(istanbul.Message)

	if err := istMsg.FromPayload(payload, istanbul.GetSignatureAddress); err != nil {
		logger.Error("Failed to decode message from payload", "from", peer.Node().ID(), "err", err)
		return true, err
	}

	// Verify that the sender is from the proxied validator
	if istMsg.Address != p.config.ProxiedValidatorAddress {
		logger.Error("Unauthorized forward message", "sender address", istMsg.Address, "authorized sender address", p.config.ProxiedValidatorAddress)
		return true, errUnauthorizedMessageFromProxiedValidator
	}

	fwdMsg := istMsg.ForwardMessage()
	logger.Trace("Forwarding a message", "msg code", fwdMsg.Code)
	if err := p.backend.Multicast(fwdMsg.DestAddresses, fwdMsg.Msg, fwdMsg.Code, false); err != nil {
		logger.Error("Error in multicasting a forwarded message", "error", err)
		return true, err
	}

	return true, nil
}
