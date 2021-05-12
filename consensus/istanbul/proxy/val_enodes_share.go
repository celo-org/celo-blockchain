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
	"errors"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/consensus"
	"github.com/celo-org/celo-blockchain/consensus/istanbul"
	"github.com/celo-org/celo-blockchain/p2p/enode"
	"github.com/celo-org/celo-blockchain/rlp"
)

func (pv *proxiedValidatorEngine) generateValEnodesShareMsg(remoteValidators []common.Address) (*istanbul.Message, error) {
	logger := pv.logger.New("func", "generateValEnodesShareMsg")

	logger.Trace("generateValEnodesShareMsg called", "remoteValidators", common.ConvertToStringSlice(remoteValidators))
	vetEntries, err := pv.backend.GetValEnodeTableEntries(remoteValidators)
	logger.Trace("GetValEnodeTableEntries returned", "vetEntries", vetEntries)

	if err != nil {
		logger.Error("Error in retrieving all the entries from the ValEnodeTable", "err", err)
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
		logger.Error("Error encoding Istanbul Validator Enodes Share message content", "ValEnodesShareData", valEnodesShareData.String(), "err", err)
		return nil, err
	}

	msg := &istanbul.Message{
		Code:      istanbul.ValEnodesShareMsg,
		Msg:       valEnodesShareBytes,
		Address:   pv.backend.Address(),
		Signature: []byte{},
	}

	// Sign the validator enode share message
	if err := msg.Sign(pv.backend.Sign); err != nil {
		logger.Error("Error in signing an Istanbul ValEnodesShare Message", "ValEnodesShareMsg", msg.String(), "err", err)
		return nil, err
	}

	logger.Trace("Generated a Istanbul Validator Enodes Share message", "IstanbulMsg", msg.String(), "ValEnodesShareData", valEnodesShareData.String())

	return msg, nil
}

// sendValEnodesShareMsg generates and then sends a ValEnodesShare message to the proxy
// This is a no-op for replica validators.
func (pv *proxiedValidatorEngine) sendValEnodesShareMsg(proxyPeer consensus.Peer, remoteValidators []common.Address) error {
	logger := pv.logger.New("func", "sendValEnodesShareMsg")

	if !pv.backend.IsValidating() {
		logger.Info("Skipping sending ValEnodesShareMsg b/c not validating")
		return errors.New("Not validating")
	}

	msg, err := pv.generateValEnodesShareMsg(remoteValidators)
	if err != nil {
		logger.Error("Error generating Istanbul ValEnodesShare Message", "err", err)
		return err
	}

	// Convert to payload
	payload, err := msg.Payload()
	if err != nil {
		logger.Error("Error in converting Istanbul ValEnodesShare Message to payload", "ValEnodesShareMsg", msg.String(), "err", err)
		return err
	}

	logger.Trace("Sending Istanbul Validator Enodes Share payload to proxy peer", "proxyPeer", proxyPeer)
	if err := proxyPeer.Send(istanbul.ValEnodesShareMsg, payload); err != nil {
		logger.Error("Error sending Istanbul ValEnodesShare Message to proxy", "err", err)
		return err
	}

	return nil
}

func (p *proxyEngine) handleValEnodesShareMsg(peer consensus.Peer, payload []byte) (bool, error) {
	logger := p.logger.New("func", "handleValEnodesShareMsg")

	logger.Trace("Handling an Istanbul Validator Enodes Share message")

	p.proxiedValidatorsMu.RLock()

	// Verify that it's coming from the proxied peer
	if ok := p.proxiedValidatorIDs[peer.Node().ID()]; !ok {
		logger.Warn("Got a valEnodesShare message from a peer that is not the proxy's proxied validator. Ignoring it", "from", peer.Node().ID())
		p.proxiedValidatorsMu.RUnlock()
		return false, nil
	}

	p.proxiedValidatorsMu.RUnlock()
	msg := new(istanbul.Message)
	// Decode message
	err := msg.FromPayload(payload, istanbul.GetSignatureAddress)
	if err != nil {
		logger.Error("Error in decoding received Istanbul Validator Enode Share message", "err", err, "payload", hex.EncodeToString(payload), "sender address", msg.Address)
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

	valEnodeEntries := make(map[common.Address]*istanbul.AddressEntry)
	for _, sharedValidatorEnode := range valEnodesShareData.ValEnodes {
		if node, err := enode.ParseV4(sharedValidatorEnode.EnodeURL); err != nil {
			logger.Warn("Error in parsing enodeURL", "enodeURL", sharedValidatorEnode.EnodeURL)
			continue
		} else {
			valEnodeEntries[sharedValidatorEnode.Address] = &istanbul.AddressEntry{Address: sharedValidatorEnode.Address, Node: node, Version: sharedValidatorEnode.Version}
		}
	}

	if err := p.backend.RewriteValEnodeTableEntries(valEnodeEntries); err != nil {
		logger.Warn("Error in rewriting the valEnodeTable", "IstanbulMsg", msg.String(), "valEnodeEntries", valEnodeEntries, "error", err)
	}

	return true, nil
}
