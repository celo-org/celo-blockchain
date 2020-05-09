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
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/consensus/istanbul"
	"github.com/ethereum/go-ethereum/rlp"
)

func (p *proxy) SendForwardMsg(finalDestAddresses []common.Address, ethMsgCode uint64, payload []byte) error {
	logger := p.logger.New("func", "SendForwardMsg")
	if p.proxyNode.peer == nil {
		logger.Warn("No connected proxy for sending a fwd message", "ethMsgCode", ethMsgCode, "finalDestAddreses", common.ConvertToStringSlice(finalDestAddresses))
		return errNoConnectedProxy
	}

	// Convert the message to a fwdMessage
	fwdMessage := &istanbul.ForwardMessage{
		Code:          ethMsgCode,
		DestAddresses: finalDestAddresses,
		Msg:           payload,
	}
	fwdMsgBytes, err := rlp.EncodeToBytes(fwdMessage)
	if err != nil {
		logger.Error("Failed to encode", "fwdMessage", fwdMessage)
		return err
	}

	// Note that we are not signing message.  The message that is being wrapped is already signed.
	msg := istanbul.Message{Code: istanbul.FwdMsg, Msg: fwdMsgBytes, Address: p.address}
	fwdMsgPayload, err := msg.Payload()
	if err != nil {
		return err
	}

	go p.proxyNode.peer.Send(istanbul.FwdMsg, fwdMsgPayload)

	return nil
}

func (p *proxy) handleForwardMsg(peer consensus.Peer, payload []byte) (bool, error) {
	logger := p.logger.New("func", "HandleForwardMsg")

	// Verify that it's coming from the proxied peer
	if p.proxiedValidator == nil || p.proxiedValidator.Node().ID() != peer.Node().ID() {
		logger.Warn("Got a forward consensus message from a peer that is not the proxy's proxied validator. Ignoring it", "from", peer.Node().ID())
		return false, nil
	}

	istMsg := new(istanbul.Message)

	// An Istanbul FwdMsg doesn't have a signature since it's coming from a trusted peer and
	// the wrapped message is already signed by the proxied validator.
	if err := istMsg.FromPayload(payload, nil); err != nil {
		logger.Error("Failed to decode message from payload", "from", peer.Node().ID(), "err", err)
		return true, err
	}

	var fwdMsg *istanbul.ForwardMessage
	err := istMsg.Decode(&fwdMsg)
	if err != nil {
		logger.Error("Failed to decode a ForwardMessage", "from", peer.Node().ID(), "err", err)
		return true, err
	}

	logger.Trace("Forwarding a message", "msg code", fwdMsg.Code)
	go p.backend.Multicast(fwdMsg.DestAddresses, fwdMsg.Msg, fwdMsg.Code, false)
	return true, nil
}
