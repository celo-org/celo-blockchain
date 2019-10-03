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
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	mrand "math/rand"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/istanbul"
	contract_errors "github.com/ethereum/go-ethereum/contract_comm/errors"
	"github.com/ethereum/go-ethereum/contract_comm/validators"
	"github.com/ethereum/go-ethereum/crypto/ecies"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/rlp"
)

// ==============================================
//
// define the istanbul announce message

type announceMessage struct {
	IncompleteEnodeURL    string
	EncryptedEndpointData [][][]byte
	View                  *istanbul.View
}

func (am *announceMessage) String() string {
	return fmt.Sprintf("{View: %v, IncompleteEnodeURL: %v}", am.View, am.IncompleteEnodeURL)
}

// ==============================================
//
// define the functions that needs to be provided for rlp Encoder/Decoder.

// EncodeRLP serializes am into the Ethereum RLP format.
func (am *announceMessage) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, []interface{}{am.IncompleteEnodeURL, am.EncryptedEndpointData, am.View})
}

// DecodeRLP implements rlp.Decoder, and load the am fields from a RLP stream.
func (am *announceMessage) DecodeRLP(s *rlp.Stream) error {
	var msg struct {
		IncompleteEnodeURL    string
		EncryptedEndpointData [][][]byte
		View                  *istanbul.View
	}

	if err := s.Decode(&msg); err != nil {
		return err
	}
	am.IncompleteEnodeURL, am.EncryptedEndpointData, am.View = msg.IncompleteEnodeURL, msg.EncryptedEndpointData, msg.View
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
		case <-ticker.C:
			// output the valEnodeTable for debugging purposes
			log.Trace("ValidatorEnodeTable dump", "ValidatorEnodeTable", sb.valEnodeTable.String())
			go sb.sendIstAnnounce()

		case <-sb.announceQuit:
			ticker.Stop()
			return
		}
	}
}

func (sb *Backend) generateIstAnnounce() ([]byte, error) {
	block := sb.currentBlock()
	selfEnode := sb.Enode()

	if selfEnode == nil {
		sb.logger.Error("Enode is nil in sendIstAnnounce")
		return nil, nil
	}

	enodeUrl := selfEnode.String()
	view := sb.core.CurrentView()
	incompleteEnodeUrl := enodeUrl[:strings.Index(enodeUrl, "@")]
	endpointData := enodeUrl[strings.Index(enodeUrl, "@"):]

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

	encryptedEndpoints := make([][][]byte, 0)
	for addr := range regAndActiveVals {
		if validatorEnodeEntry, ok := sb.valEnodeTable.valEnodeTable[addr]; ok {
			validatorEnode, err := enode.ParseV4(validatorEnodeEntry.enodeURL)
			pubKey := ecies.ImportECDSAPublic(validatorEnode.Pubkey())
			encryptedEndpoint, err := ecies.Encrypt(rand.Reader, pubKey, []byte(endpointData), nil, nil)
			if err != nil {
				log.Warn("Unable to unmarshal public key", "err", err)
			} else {
				encryptedEndpoints = append(encryptedEndpoints, [][]byte{addr.Bytes(), encryptedEndpoint})
			}
		}
	}

	announceMessage := &announceMessage{
		IncompleteEnodeURL:    incompleteEnodeUrl,
		EncryptedEndpointData: encryptedEndpoints,
		View:                  view,
	}

	announceBytes, err := rlp.EncodeToBytes(announceMessage)
	if err != nil {
		sb.logger.Error("Error encoding announce content in an Istanbul Validator Enode Share message", "AnnounceMsg", announceMessage.String(), "err", err)
	}

	msg := &istanbul.Message{
		Code:          istanbulAnnounceMsg,
		Msg:           announceBytes,
		Address:       sb.Address(),
		Signature:     []byte{},
		CommittedSeal: []byte{},
	}

	// Sign the announce message
	if err := msg.Sign(sb.Sign); err != nil {
		sb.logger.Error("Error in signing an Istanbul Announce Message", "IstanbulMsg", msg.String(), "AnnounceMsg", announceMessage.String(), "err", err)
		return nil, err
	}

	// Convert to payload
	payload, err := msg.Payload()
	if err != nil {
		sb.logger.Error("Error in converting Istanbul Announce Message to payload", "IstanbulMsg", msg.String(), "AnnounceMsg", announceMessage.String(), "err", err)
		return nil, err
	}

	sb.logger.Trace("Broadcasting an announce message", "IstanbulMsg", msg.String(), "AnnounceMsg", announceMessage.String())

	return payload, nil
}

func (sb *Backend) sendIstAnnounce() error {
	payload, err := sb.generateIstAnnounce()
	if err != nil {
		return err
	}

	if payload == nil {
		return nil
	}

	sb.Gossip(nil, payload, istanbulAnnounceMsg, true)

	return nil
}

func (sb *Backend) handleIstAnnounce(payload []byte) error {
	sb.logger.Trace("Handling an IstanbulAnnounce message")

	msg := new(istanbul.Message)
	// Decode message
	err := msg.FromPayload(payload, istanbul.GetSignatureAddress)
	if err != nil {
		sb.logger.Error("Error in decoding received Istanbul Announce message", "err", err, "payload", hex.EncodeToString(payload))
		return err
	}

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

	// Decrypt the EnodeURL
	nodeKey := ecies.ImportECDSA(sb.GetNodeKey())

	var announceMessage announceMessage
	err = rlp.DecodeBytes(msg.Msg, &announceMessage)
	if err != nil {
		sb.logger.Error("Error in decoding received Istanbul Announce message content", "err", err, "IstanbulMsg", msg.String())
	}

	encryptedEndpoint := []byte("")
	for _, entry := range announceMessage.EncryptedEndpointData {
		if bytes.Equal(entry[0], sb.Address().Bytes()) {
			encryptedEndpoint = entry[1]
		}
	}
	endpointBytes, err := nodeKey.Decrypt(encryptedEndpoint, nil, nil)
	if err != nil && len(encryptedEndpoint) > 0 {
		sb.logger.Warn("Error in decrypting endpoint", "err", err, "encryptedEndpoint", encryptedEndpoint)
	}
	enodeUrl := announceMessage.IncompleteEnodeURL + string(endpointBytes)

	// Save in the valEnodeTable if mining
	if sb.coreStarted {
		block := sb.currentBlock()
		valSet := sb.getValidators(block.Number().Uint64(), block.Hash())

		newValEnode := &validatorEnode{enodeURL: enodeUrl, view: announceMessage.View}
		if err := sb.valEnodeTable.upsert(msg.Address, newValEnode, valSet, sb.Address(), sb.Proxied(), false); err != nil {
			sb.logger.Warn("Error in upserting a valenode entry", "AnnounceMsg", announceMessage.String(), "error", err)
			return err
		}
	}

	// If we gossiped this address/enodeURL within the last 60 seconds, then don't regossip
	sb.lastAnnounceGossipedMu.RLock()
	if lastGossipTs, ok := sb.lastAnnounceGossiped[msg.Address]; ok {
		if lastGossipTs.enodeURL == enodeUrl && time.Since(lastGossipTs.timestamp) < time.Minute {
			sb.logger.Trace("Already regossiped the msg within the last minute, so not regossiping.", "IstanbulMsg", msg.String(), "AnnounceMsg", announceMessage.String())
			sb.lastAnnounceGossipedMu.RUnlock()
			return nil
		}
	}
	sb.lastAnnounceGossipedMu.RUnlock()

	sb.logger.Trace("Regossiping the istanbul announce message", "IstanbulMsg", msg.String(), "AnnounceMsg", announceMessage.String())
	sb.Gossip(nil, payload, istanbulAnnounceMsg, true)

	sb.lastAnnounceGossipedMu.Lock()
	defer sb.lastAnnounceGossipedMu.Unlock()
	sb.lastAnnounceGossiped[msg.Address] = &AnnounceGossipTimestamp{enodeURL: enodeUrl, timestamp: time.Now()}

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

	return nil
}
