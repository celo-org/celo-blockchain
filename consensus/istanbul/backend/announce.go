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
	"bytes"
	"encoding/hex"
	"fmt"
	"io"
	mrand "math/rand"
	"sort"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/istanbul"
	vet "github.com/ethereum/go-ethereum/consensus/istanbul/backend/internal/enodes"
	contract_errors "github.com/ethereum/go-ethereum/contract_comm/errors"
	"github.com/ethereum/go-ethereum/contract_comm/validators"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/rlp"
)

// ===============================================================
//
// define the istanbul announce data and announce record structure

type announceRecord struct {
	DestAddress       common.Address
	EncryptedEnodeURL []byte
}

func (ar *announceRecord) String() string {
	return fmt.Sprintf("{DestAddress: %s, EncryptedEnodeURL length: %d}", ar.DestAddress.String(), len(ar.EncryptedEnodeURL))
}

type announceData struct {
	AnnounceRecords []*announceRecord
	EnodeURLHash    common.Hash
	View            *istanbul.View
}

func (ad *announceData) String() string {
	return fmt.Sprintf("{View: %v, EnodeURLHash: %v, AnnounceRecords: %v}", ad.View, ad.EnodeURLHash.Hex(), ad.AnnounceRecords)
}

// ==============================================
//
// define the functions that needs to be provided for rlp Encoder/Decoder.

// EncodeRLP serializes ar into the Ethereum RLP format.
func (ar *announceRecord) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, []interface{}{ar.DestAddress, ar.EncryptedEnodeURL})
}

// DecodeRLP implements rlp.Decoder, and load the ar fields from a RLP stream.
func (ar *announceRecord) DecodeRLP(s *rlp.Stream) error {
	var msg struct {
		DestAddress       common.Address
		EncryptedEnodeURL []byte
	}

	if err := s.Decode(&msg); err != nil {
		return err
	}
	ar.DestAddress, ar.EncryptedEnodeURL = msg.DestAddress, msg.EncryptedEnodeURL
	return nil
}

// EncodeRLP serializes ad into the Ethereum RLP format.
func (ad *announceData) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, []interface{}{ad.AnnounceRecords, ad.EnodeURLHash, ad.View})
}

// DecodeRLP implements rlp.Decoder, and load the ad fields from a RLP stream.
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
		case <-sb.newEpochCh:
			go sb.sendIstAnnounce()
		case <-ticker.C:
			// output the valEnodeTable for debugging purposes
			log.Trace("ValidatorEnodeDB dump", "ValidatorEnodeDB", sb.valEnodeTable.String())
			go sb.sendIstAnnounce()
		case <-sb.announceQuit:
			ticker.Stop()
			return
		}
	}
}

func (sb *Backend) generateIstAnnounce() (*istanbul.Message, error) {
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

	// If the message is not within the registered validator set, then ignore it
	regAndActiveVals, err := sb.retrieveActiveAndRegisteredValidators()
	if err != nil {
		return nil, err
	}

	announceRecords := make([]*announceRecord, 0, len(regAndActiveVals))
	for addr := range regAndActiveVals {
		// TODO - Need to encrypt using the remote validator's validator key
		announceRecords = append(announceRecords, &announceRecord{DestAddress: addr, EncryptedEnodeURL: []byte(enodeUrl)})
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

func (sb *Backend) retrieveActiveAndRegisteredValidators() (map[common.Address]bool, error) {
	validatorsSet := make(map[common.Address]bool)

	registeredValidators, err := validators.RetrieveRegisteredValidatorSigners(nil, nil)

	// The validator contract may not be deployed yet.
	// Even if it is deployed, it may not have any registered validators yet.
	if err == contract_errors.ErrSmartContractNotDeployed || len(registeredValidators) == 0 {
		sb.logger.Trace("Can't retrieve the registered validators.  Only allowing the initial validator set to send announce messages", "err", err, "registeredValidators", registeredValidators)
	} else if err != nil {
		sb.logger.Error("Error in retrieving the registered validators", "err", err)
		return validatorsSet, err
	}

	for _, address := range registeredValidators {
		validatorsSet[address] = true
	}

	// Add active validators regardless
	block := sb.currentBlock()
	valSet := sb.getValidators(block.Number().Uint64(), block.Hash())
	for _, val := range valSet.List() {
		validatorsSet[val.Address()] = true
	}

	return validatorsSet, nil
}

func (sb *Backend) handleIstAnnounce(payload []byte) error {
	logger := sb.logger.New("func", "handleIstAnnounce")

	msg := new(istanbul.Message)

	// Decode message
	err := msg.FromPayload(payload, istanbul.GetSignatureAddress)
	if err != nil {
		logger.Error("Error in decoding received Istanbul Announce message", "err", err, "payload", hex.EncodeToString(payload))
		return err
	}
	logger.Trace("Handling an IstanbulAnnounce message", "from", msg.Address)

	// If the message is originally from this node, then ignore it
	if msg.Address == sb.Address() {
		logger.Trace("Received an IstanbulAnnounce message originating from this node. Ignoring it.")
		return nil
	}

	var announceData announceData
	err = rlp.DecodeBytes(msg.Msg, &announceData)
	if err != nil {
		logger.Error("Error in decoding received Istanbul Announce message content", "err", err, "IstanbulMsg", msg.String())
		return err
	}

	logger = logger.New("msgAddress", msg.Address, "msg_round", announceData.View.Round, "msg_seq", announceData.View.Sequence)

	if view, err := sb.valEnodeTable.GetViewFromAddress(msg.Address); err == nil && announceData.View.Cmp(view) <= 0 {
		logger.Trace("Received an old announce message", "senderAddr", msg.Address, "messageView", announceData.View, "currentEntryView", view)
		return errOldAnnounceMessage
	}

	// If the message is not within the registered validator set, then ignore it
	regAndActiveVals, err := sb.retrieveActiveAndRegisteredValidators()
	if err != nil {
		return err
	}

	if !regAndActiveVals[msg.Address] {
		logger.Warn("Received an IstanbulAnnounce message from a non registered validator. Ignoring it.", "AnnounceMsg", msg.String(), "err", err)
		return errUnauthorizedAnnounceMessage
	}

	var node *enode.Node
	var destAddresses = make([]string, 0, len(announceData.AnnounceRecords))
	var processedAddresses = make(map[common.Address]bool)
	var msgHasDupsOrIrrelevantEntries bool = false
	for _, announceRecord := range announceData.AnnounceRecords {
		// Don't process duplicate entries or entries that are not in the regAndActive valset
		if !regAndActiveVals[announceRecord.DestAddress] || processedAddresses[announceRecord.DestAddress] {
			msgHasDupsOrIrrelevantEntries = true
			continue
		}

		if announceRecord.DestAddress == sb.Address() {
			// TODO: Decrypt the enodeURL using this validator's validator key after making changes to encrypt it
			enodeUrl := string(announceRecord.EncryptedEnodeURL)
			node, err = enode.ParseV4(enodeUrl)
			if err != nil {
				logger.Error("Error in parsing enodeURL", "enodeUrl", enodeUrl)
				return err
			}
		}
		destAddresses = append(destAddresses, announceRecord.DestAddress.String())
		processedAddresses[announceRecord.DestAddress] = true
	}
	// Save in the valEnodeTable if mining
	if sb.coreStarted && node != nil {
		if err := sb.valEnodeTable.Upsert(map[common.Address]*vet.AddressEntry{msg.Address: {Node: node, View: announceData.View}}); err != nil {
			logger.Warn("Error in upserting a valenode entry", "AnnounceData", announceData.String(), "error", err)
			return err
		}
	}

	if !msgHasDupsOrIrrelevantEntries {
		if err = sb.regossipIstAnnounce(msg, payload, announceData, regAndActiveVals, destAddresses); err != nil {
			return err
		}
	}

	return nil
}

func (sb *Backend) regossipIstAnnounce(msg *istanbul.Message, payload []byte, announceData announceData, regAndActiveVals map[common.Address]bool, destAddresses []string) error {
	logger := sb.logger.New("func", "regossipIstAnnounce", "msgAddress", msg.Address, "msg_round", announceData.View.Round, "msg_seq", announceData.View.Sequence)
	// If we gossiped this address/enodeURL within the last 60 seconds and the enodeURLHash and destAddressHash didn't change, then don't regossip

	// Generate the destAddresses hash
	sort.Strings(destAddresses)
	destAddressesHash := istanbul.RLPHash(destAddresses)

	sb.lastAnnounceGossipedMu.RLock()
	if lastGossipTs, ok := sb.lastAnnounceGossiped[msg.Address]; ok {

		if lastGossipTs.enodeURLHash == announceData.EnodeURLHash && bytes.Equal(lastGossipTs.destAddressesHash.Bytes(), destAddressesHash.Bytes()) && time.Since(lastGossipTs.timestamp) < time.Minute {
			logger.Trace("Already regossiped the msg within the last minute, so not regossiping.", "IstanbulMsg", msg.String(), "AnnounceData", announceData.String())
			sb.lastAnnounceGossipedMu.RUnlock()
			return nil
		}
	}
	sb.lastAnnounceGossipedMu.RUnlock()

	logger.Trace("Regossiping the istanbul announce message", "IstanbulMsg", msg.String(), "AnnounceMsg", announceData.String())
	sb.Gossip(nil, payload, istanbulAnnounceMsg, true)

	sb.lastAnnounceGossipedMu.Lock()
	defer sb.lastAnnounceGossipedMu.Unlock()
	sb.lastAnnounceGossiped[msg.Address] = &AnnounceGossipTimestamp{enodeURLHash: announceData.EnodeURLHash, timestamp: time.Now(), destAddressesHash: destAddressesHash}

	// prune non registered validator entries in the valEnodeTable, reverseValEnodeTable, and lastAnnounceGossiped tables about 5% of the times that an announce msg is handled
	if (mrand.Int() % 100) <= 5 {
		for remoteAddress := range sb.lastAnnounceGossiped {
			if !regAndActiveVals[remoteAddress] {
				logger.Trace("Deleting entry from the lastAnnounceGossiped table", "address", remoteAddress, "gossip timestamp", sb.lastAnnounceGossiped[remoteAddress])
				delete(sb.lastAnnounceGossiped, remoteAddress)
			}
		}

		if err := sb.valEnodeTable.PruneEntries(regAndActiveVals); err != nil {
			return err
		}
	}

	return nil
}
