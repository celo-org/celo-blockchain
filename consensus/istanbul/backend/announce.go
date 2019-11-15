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

package backend

import (
	"encoding/hex"
	"fmt"
	"io"
	mrand "math/rand"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/istanbul"
	contract_errors "github.com/ethereum/go-ethereum/contract_comm/errors"
	"github.com/ethereum/go-ethereum/contract_comm/validators"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/rlp"
)

// ===============================================================
//
// define the istanbul announce data and announce record structure

type announceRecord struct {
	RecipientAddress  common.Address
	EncryptedEnodeURL []byte
}

type announceData struct {
	AnnounceRecords []*announceRecord
	EnodeURLHash    common.Hash
	View            *istanbul.View
}

func (ad *announceData) String() string {
	return fmt.Sprintf("{View: %v, EnodeURLHash: %v}", ad.View, ad.EnodeURLHash.Hex())
}

// ==============================================
//
// define the functions that needs to be provided for rlp Encoder/Decoder.

// EncodeRLP serializes am into the Ethereum RLP format.
func (ar *announceRecord) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, []interface{}{ar.RecipientAddress, ar.EncryptedEnodeURL})
}

// DecodeRLP implements rlp.Decoder, and load the am fields from a RLP stream.
func (ar *announceRecord) DecodeRLP(s *rlp.Stream) error {
	var msg struct {
		RecipientAddress  common.Address
		EncryptedEnodeURL []byte
	}

	if err := s.Decode(&msg); err != nil {
		return err
	}
	ar.RecipientAddress, ar.EncryptedEnodeURL = msg.RecipientAddress, msg.EncryptedEnodeURL
	return nil
}

// EncodeRLP serializes am into the Ethereum RLP format.
func (ad *announceData) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, []interface{}{ad.AnnounceRecords, ad.EnodeURLHash, ad.View})
}

// DecodeRLP implements rlp.Decoder, and load the am fields from a RLP stream.
func (ad *announceData) DecodeRLP(s *rlp.Stream) error {
	var msg struct {
		AnnounceRecords []*announceRecord
		EnodeURLHash    common.Hash
		View            *istanbul.View
	}

	if err := s.Decode(&msg); err != nil {
		return err
	}
	ad.AnnounceRecords, ad.EnodeURLHash, ad.View = msg.AnnounceRecords, msg.EnodeURLHash, msg.View
	return nil
}

// This function is meant to be run as a goroutine.  It will periodically gossip announce messages
// to the rest of the registered validators to communicate it's enodeURL to them.
func (sb *Backend) sendAnnounceMsgs() {
	sb.announceWg.Add(1)
	defer sb.announceWg.Done()

	ticker := time.NewTicker(time.Minute)

	for {
		select {
		case <-sb.newEpoch:
			go sb.sendIstAnnounce()
		case <-ticker.C:
			go sb.sendIstAnnounce()
		case <-sb.announceQuit:
			ticker.Stop()
			return
		}
	}
}

func (sb *Backend) generateIstAnnounce() (*istanbul.Message, error) {
	block := sb.currentBlock()

	var enodeUrl string
	if sb.config.Proxied {
		if sb.proxyNode != nil {
			enodeUrl = sb.proxyNode.externalNode.String()
		} else {
			sb.logger.Error("Proxied node is not connected to a proxy")
			return nil, errNoProxyConnection
		}
	} else {
		enodeUrl = sb.p2pserver.Self().String()
	}
	view := sb.core.CurrentView()

	regAndActiveVals, err := validators.RetrieveRegisteredValidators(nil, nil)
	// The validator contract may not be deployed yet.
	// Even if it is deployed, it may not have any registered validators yet.
	if err == contract_errors.ErrSmartContractNotDeployed || len(regAndActiveVals) == 0 {
		sb.logger.Trace("Can't retrieve the registered validators.  Only allowing the initial validator set to send announce messages", "err", err, "regAndActiveVals", regAndActiveVals)
		regAndActiveVals = make(map[common.Address]bool)
	} else if err != nil {
		sb.logger.Error("Error in retrieving the registered validators", "err", err)
		return nil, err
	}

	// Add active validators regardless
	valSet := sb.getValidators(block.Number().Uint64(), block.Hash())
	for _, val := range valSet.List() {
		regAndActiveVals[val.Address()] = true
	}

	announceRecords := make([]*announceRecord, 0, len(regAndActiveVals))
	for addr := range regAndActiveVals {
		// TODO - Need to encrypt using the remote validator's validator key
		announceRecords = append(announceRecords, &announceRecord{RecipientAddress: addr, EncryptedEnodeURL: []byte(enodeUrl)})
	}

	announceData := &announceData{
		AnnounceRecords: announceRecords,
		EnodeURLHash:    istanbul.RLPHash(enodeUrl),
		View:            view,
	}

	announceBytes, err := rlp.EncodeToBytes(announceData)
	if err != nil {
		sb.logger.Error("Error encoding announce content in an Istanbul Validator Enode Share message", "AnnounceData", announceData.String(), "err", err)
		return nil, err
	}

	msg := &istanbul.Message{
		Code:      istanbulAnnounceMsg,
		Msg:       announceBytes,
		Address:   sb.Address(),
		Signature: []byte{},
	}

	sb.logger.Debug("Generated an announce message", "IstanbulMsg", msg.String(), "AnnounceMsg", announceData.String())

	return msg, nil
}

func (sb *Backend) sendIstAnnounce() error {
	istMsg, err := sb.generateIstAnnounce()
	if err != nil {
		return err
	}

	if istMsg == nil {
		return nil
	}

	// Sign the announce message
	if err := istMsg.Sign(sb.Sign); err != nil {
		sb.logger.Error("Error in signing an Istanbul Announce Message", "AnnounceMsg", istMsg.String(), "err", err)
		return err
	}

	// Convert to payload
	payload, err := istMsg.Payload()
	if err != nil {
		sb.logger.Error("Error in converting Istanbul Announce Message to payload", "AnnounceMsg", istMsg.String(), "err", err)
		return err
	}

	sb.Gossip(nil, payload, istanbulAnnounceMsg, true)

	return nil
}

func (sb *Backend) handleIstAnnounce(payload []byte) error {
	msg := new(istanbul.Message)
	// Decode message
	err := msg.FromPayload(payload, istanbul.GetSignatureAddress)
	if err != nil {
		sb.logger.Error("Error in decoding received Istanbul Announce message", "err", err, "payload", hex.EncodeToString(payload))
		return err
	}

	sb.logger.Trace("Handling an IstanbulAnnounce message", "from", msg.Address)

	// If the message is originally from this node, then ignore it
	if msg.Address == sb.Address() {
		sb.logger.Trace("Received an IstanbulAnnounce message originating from this node. Ignoring it.")
		return nil
	}

	// If the message is not within the registered validator set, then ignore it
	regAndActiveVals, err := validators.RetrieveRegisteredValidators(nil, nil)

	// The validator contract may not be deployed yet.
	// Even if it is deployed, it may not have any registered validators yet.
	if err == contract_errors.ErrSmartContractNotDeployed || len(regAndActiveVals) == 0 {
		sb.logger.Trace("Can't retrieve the registered validators.  Only allowing the initial validator set to send announce messages", "err", err, "regAndActiveVals", regAndActiveVals)
		regAndActiveVals = make(map[common.Address]bool)
	} else if err != nil {
		sb.logger.Error("Error in retrieving the registered validators", "err", err)
		return err
	}

	// Add active validators regardless
	block := sb.currentBlock()
	valSet := sb.getValidators(block.Number().Uint64(), block.Hash())
	for _, val := range valSet.List() {
		regAndActiveVals[val.Address()] = true
	}

	if !regAndActiveVals[msg.Address] {
		sb.logger.Warn("Received an IstanbulAnnounce message from a non registered validator. Ignoring it.", "IstanbulMsg", msg.String(), "validators", regAndActiveVals, "err", err)
		return errUnauthorizedAnnounceMessage
	}

	var announceData announceData
	err = rlp.DecodeBytes(msg.Msg, &announceData)
	if err != nil {
		sb.logger.Error("Error in decoding received Istanbul Announce message content", "err", err, "IstanbulMsg", msg.String())
		return err
	}

	// Save in the valEnodeTable if mining
	if sb.coreStarted {
		var enodeUrl string
		for _, announceRecord := range announceData.AnnounceRecords {
			if announceRecord.RecipientAddress == sb.Address() {
				// TODO: Decrypt the enodeURL using this validator's validator key after making changes to encrypt it
				enodeUrl = string(announceRecord.EncryptedEnodeURL)
				block := sb.currentBlock()
				valSet := sb.getValidators(block.Number().Uint64(), block.Hash())

				if err := sb.valEnodeTable.upsert(msg.Address, enodeUrl, announceData.View, valSet, sb.ValidatorAddress(), sb.config.Proxied, false); err != nil {
					sb.logger.Warn("Error in upserting a valenode entry", "AnnounceData", announceData.String(), "error", err)
					return err
				}

				break
			}
		}
	}

	sb.regossipIstAnnounce(msg, payload, announceData, regAndActiveVals)
	return nil
}

func (sb *Backend) regossipIstAnnounce(msg *istanbul.Message, payload []byte, announceData announceData, regAndActiveVals map[common.Address]bool) {
	// If we gossiped this address/enodeURL within the last 60 seconds and the enodeURLHash didn't change, then don't regossip
	sb.lastAnnounceGossipedMu.RLock()
	if lastGossipTs, ok := sb.lastAnnounceGossiped[msg.Address]; ok {
		if (lastGossipTs.enodeURLHash == announceData.EnodeURLHash) && (time.Since(lastGossipTs.timestamp) < time.Minute) {
			sb.logger.Trace("Already regossiped the msg within the last minute, so not regossiping.", "IstanbulMsg", msg.String(), "AnnounceData", announceData.String())
			sb.lastAnnounceGossipedMu.RUnlock()
			return
		}
	}
	sb.lastAnnounceGossipedMu.RUnlock()

	sb.logger.Trace("Regossiping the istanbul announce message", "IstanbulMsg", msg.String(), "AnnounceMsg", announceData.String())
	sb.Gossip(nil, payload, istanbulAnnounceMsg, true)

	sb.lastAnnounceGossipedMu.Lock()
	defer sb.lastAnnounceGossipedMu.Unlock()
	sb.lastAnnounceGossiped[msg.Address] = &AnnounceGossipTimestamp{enodeURLHash: announceData.EnodeURLHash, timestamp: time.Now()}

	// prune non registered validator entries in the valEnodeTable, reverseValEnodeTable, and lastAnnounceGossiped tables about 5% of the times that an announce msg is handled
	if (mrand.Int() % 100) <= 5 {
		for remoteAddress := range sb.lastAnnounceGossiped {
			if !regAndActiveVals[remoteAddress] {
				log.Trace("Deleting entry from the lastAnnounceGossiped table", "address", remoteAddress, "gossip timestamp", sb.lastAnnounceGossiped[remoteAddress])
				delete(sb.lastAnnounceGossiped, remoteAddress)
			}
		}

		sb.valEnodeTable.pruneEntries(regAndActiveVals)
	}
}
