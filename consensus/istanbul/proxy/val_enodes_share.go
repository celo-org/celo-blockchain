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
	"time"

	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/consensus/istanbul"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/rlp"
)

// This function is meant to be run as a goroutine.  It will periodically send validator enode share messages
// to this node's proxies so that proxies know the enodes of validators
func (p *proxyEngine) sendValEnodesShareMsgEventLoop() {
	p.valEnodesShareWg.Add(1)
	defer p.valEnodesShareWg.Done()

	ticker := time.NewTicker(time.Minute)

	for {
		select {
		case <-ticker.C:
			go p.sendValEnodesShareMsg()

		case <-p.sendValEnodesShareMsgCh:
			// TODO:  Right now the proxied validator will send the full valEnodeTable to the proxies.
			//        The reduce network usage, this can be changed so that only diffs of the table are sent.
			go p.sendValEnodesShareMsg()

		case <-p.valEnodesShareQuit:
			ticker.Stop()
			return
		}
	}
}

func (p *proxyEngine) SendValEnodesShareMsg() {
	p.sendValEnodesShareMsgCh <- struct{}{}
}

func (p *proxyEngine) generateValEnodesShareMsg() (*istanbul.Message, error) {
	vetEntries, err := p.backend.GetAllValEnodeTableEntries()

	if err != nil {
		p.logger.Error("Error in retrieving all the entries from the ValEnodeTable", "err", err)
		return nil, err
	}

	sharedValidatorEnodes := make([]sharedValidatorEnode, 0, len(vetEntries))
	for address, vetEntry := range vetEntries {
		if vetEntry.GetNode() == nil {
			continue
		}
		sharedValidatorEnodes = append(sharedValidatorEnodes, sharedValidatorEnode{
			Address:  address,
			EnodeURL: vetEntry.GetNode().String(),
			Version:  vetEntry.GetVersion(),
		})
	}

	valEnodesShareData := &valEnodesShareData{
		ValEnodes: sharedValidatorEnodes,
	}

	valEnodesShareBytes, err := rlp.EncodeToBytes(valEnodesShareData)
	if err != nil {
		p.logger.Error("Error encoding Istanbul Validator Enodes Share message content", "ValEnodesShareData", valEnodesShareData.String(), "err", err)
		return nil, err
	}

	msg := &istanbul.Message{
		Code:      istanbul.ValEnodesShareMsg,
		Msg:       valEnodesShareBytes,
		Address:   p.address,
		Signature: []byte{},
	}

	p.logger.Trace("Generated a Istanbul Validator Enodes Share message", "IstanbulMsg", msg.String(), "ValEnodesShareData", valEnodesShareData.String())

	return msg, nil
}

func (p *proxyEngine) sendValEnodesShareMsg() error {
	logger := p.logger.New("func", "sendValEnodesShareMsg")
	if p.proxyNode == nil || p.proxyNode.peer == nil {
		logger.Warn("No proxy peers, cannot send Istanbul Validator Enodes Share message")
		return nil
	}

	msg, err := p.generateValEnodesShareMsg()
	if err != nil {
		logger.Error("Error generating Istanbul ValEnodesShare Message", "err", err)
		return err
	}

	// Sign the validator enode share message
	if err := msg.Sign(p.backend.Sign); err != nil {
		logger.Error("Error in signing an Istanbul ValEnodesShare Message", "ValEnodesShareMsg", msg.String(), "err", err)
		return err
	}

	// Convert to payload
	payload, err := msg.Payload()
	if err != nil {
		logger.Error("Error in converting Istanbul ValEnodesShare Message to payload", "ValEnodesShareMsg", msg.String(), "err", err)
		return err
	}

	logger.Trace("Sending Istanbul Validator Enodes Share payload to proxy peer")
	if err := p.proxyNode.peer.Send(istanbul.ValEnodesShareMsg, payload); err != nil {
		logger.Error("Error sending Istanbul ValEnodesShare Message to proxy", "err", err)
		return err
	}

	return nil
}

func (p *proxyEngine) handleValEnodesShareMsg(peer consensus.Peer, payload []byte) (bool, error) {
	logger := p.logger.New("func", "handleValEnodesShareMsg")

	logger.Debug("Handling an Istanbul Validator Enodes Share message")

	// Verify that it's coming from the proxied peer
	if p.proxiedValidator == nil || p.proxiedValidator.Node().ID() != peer.Node().ID() {
		logger.Warn("Got a valEnodesShare message from a peer that is not the proxy's proxied validator. Ignoring it", "from", peer.Node().ID())
		return false, nil
	}

	msg := new(istanbul.Message)
	// Decode message
	err := msg.FromPayload(payload, istanbul.GetSignatureAddress)
	if err != nil {
		logger.Error("Error in decoding received Istanbul Validator Enode Share message", "err", err, "payload", hex.EncodeToString(payload))
		return true, err
	}

	// Verify that the sender is from the proxied validator
	if msg.Address != p.config.ProxiedValidatorAddress {
		logger.Error("Unauthorized valEnodesShare message", "sender address", msg.Address, "authorized sender address", p.config.ProxiedValidatorAddress)
		return true, errUnauthorizedMessageFromProxiedValidator
	}

	var valEnodesShareData valEnodesShareData
	err = rlp.DecodeBytes(msg.Msg, &valEnodesShareData)
	if err != nil {
		logger.Error("Error in decoding received Istanbul Validator Enodes Share message content", "err", err, "IstanbulMsg", msg.String())
		return true, err
	}

	logger.Trace("Received an Istanbul Validator Enodes Share message", "IstanbulMsg", msg.String(), "ValEnodesShareData", valEnodesShareData.String())

	var upsertBatch []istanbul.ValEnodeTableEntry
	for _, sharedValidatorEnode := range valEnodesShareData.ValEnodes {
		if node, err := enode.ParseV4(sharedValidatorEnode.EnodeURL); err != nil {
			logger.Warn("Error in parsing enodeURL", "enodeURL", sharedValidatorEnode.EnodeURL)
			continue
		} else {
			upsertBatch = append(upsertBatch, p.backend.NewValEnodeTableEntry(sharedValidatorEnode.Address, node, sharedValidatorEnode.Version))
		}
	}

	if len(upsertBatch) > 0 {
		if err := p.backend.UpsertValEnodeTableEntries(upsertBatch); err != nil {
			logger.Warn("Error in upserting a batch to the valEnodeTable", "IstanbulMsg", msg.String(), "UpsertBatch", upsertBatch, "error", err)
		}
	}

	return true, nil
}
