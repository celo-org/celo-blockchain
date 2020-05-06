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
	"github.com/ethereum/go-ethereum/consensus/istanbul"
	"github.com/ethereum/go-ethereum/rlp"
)

func (p *proxy) SendForwardMsg(finalDestAddresses []common.Address, ethMsgCode uint64, payload []byte) error {
        logger := p.logger.New("func", "SendForwardMsg")
        if p.proxyPeer == nil {
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

	go p.proxyPeer.Send(istanbul.FwdMsg, fwdMsgPayload)

	return nil
}

func (p *proxy) handleForwardMsg() error {
     return nil
}