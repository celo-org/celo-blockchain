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
	"sort"
	"strings"
	"time"

	"github.com/syndtr/goleveldb/leveldb"

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
	Address               common.Address
	IncompleteEnodeURL    string
	EncryptedEndpointData [][][]byte
	View                  *istanbul.View
	Signature             []byte
}

func (am *announceMessage) String() string {
	return fmt.Sprintf("{Address: %s, View: %v, IncompleteEnodeURL: %v, Signature: %v}", am.Address.String(), am.View, am.IncompleteEnodeURL, hex.EncodeToString(am.Signature))
}

// ==============================================
//
// define the functions that needs to be provided for rlp Encoder/Decoder.

// EncodeRLP serializes am into the Ethereum RLP format.
func (am *announceMessage) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, []interface{}{am.Address, am.IncompleteEnodeURL, am.EncryptedEndpointData, am.View, am.Signature})
}

// DecodeRLP implements rlp.Decoder, and load the am fields from a RLP stream.
func (am *announceMessage) DecodeRLP(s *rlp.Stream) error {
	var msg struct {
		Address               common.Address
		IncompleteEnodeURL    string
		EncryptedEndpointData [][][]byte
		View                  *istanbul.View
		Signature             []byte
	}

	if err := s.Decode(&msg); err != nil {
		return err
	}
	am.Address, am.IncompleteEnodeURL, am.EncryptedEndpointData, am.View, am.Signature = msg.Address, msg.IncompleteEnodeURL, msg.EncryptedEndpointData, msg.View, msg.Signature
	return nil
}

// ==============================================
//
// define the functions that needs to be provided for the istanbul announce sender and handler
func (am *announceMessage) FromPayload(b []byte) error {
	// Decode message
	err := rlp.DecodeBytes(b, &am)
	return err
}

func (am *announceMessage) Payload() ([]byte, error) {
	return rlp.EncodeToBytes(am)
}

func (am *announceMessage) Sign(signingFn func(data []byte) ([]byte, error)) error {
	// Construct and encode a message with no signature
	var payloadNoSig []byte
	payloadNoSig, err := rlp.EncodeToBytes(&announceMessage{
		Address:               am.Address,
		IncompleteEnodeURL:    am.IncompleteEnodeURL,
		EncryptedEndpointData: am.EncryptedEndpointData,
		View:                  am.View,
		Signature:             []byte{},
	})
	if err != nil {
		return err
	}
	am.Signature, err = signingFn(payloadNoSig)
	return err
}

func (am *announceMessage) VerifySig() error {
	// Construct and encode a message with no signature
	var payloadNoSig []byte
	payloadNoSig, err := rlp.EncodeToBytes(&announceMessage{
		Address:               am.Address,
		IncompleteEnodeURL:    am.IncompleteEnodeURL,
		EncryptedEndpointData: am.EncryptedEndpointData,
		View:                  am.View,
		Signature:             []byte{},
	})
	if err != nil {
		return err
	}

	sigAddr, err := istanbul.GetSignatureAddress(payloadNoSig, am.Signature)
	if err != nil {
		return err
	}

	if sigAddr != am.Address {
		log.Error("Address in the message is different than the address that signed it",
			"sigAddr", sigAddr.Hex(),
			"msg.Address", am.Address.Hex())
		return errInvalidSignature
	}

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

func (sb *Backend) generateIstAnnounce() ([]byte, error) {
	selfEnode := sb.Enode()

	if selfEnode == nil {
		sb.logger.Error("Enode is nil in sendIstAnnounce")
		return nil, nil
	}

	enodeURL := selfEnode.String()
	view := sb.core.CurrentView()
	incompleteEnodeURL := enodeURL[:strings.Index(enodeURL, "@")]
	endpointData := enodeURL[strings.Index(enodeURL, "@"):]

	// If the message is not within the registered validator set, then ignore it
	regAndActiveVals, err := sb.retrieveActiveAndRegisteredValidators()
	if err != nil {
		return nil, err
	}

	encryptedEndpoints := make([][][]byte, 0)
	for addr := range regAndActiveVals {
		enodeURL, err := sb.valEnodeTable.GetEnodeURLFromAddress(addr)
		if err == nil {
			validatorEnode, err := enode.ParseV4(enodeURL)
			pubKey := ecies.ImportECDSAPublic(validatorEnode.Pubkey())
			encryptedEndpoint, err := ecies.Encrypt(rand.Reader, pubKey, []byte(endpointData), nil, nil)
			if err != nil {
				log.Warn("Unable to unmarshal public key", "err", err)
			} else {
				encryptedEndpoints = append(encryptedEndpoints, [][]byte{addr.Bytes(), encryptedEndpoint})
			}
		} else if err != leveldb.ErrNotFound {
			log.Error("Unable to read valEnodeTable", "err", err, "addr", addr)
		}
	}

	msg := &announceMessage{
		Address:               sb.Address(),
		IncompleteEnodeURL:    incompleteEnodeURL,
		EncryptedEndpointData: encryptedEndpoints,
		View:                  view,
	}

	// Sign the announce message
	if err := msg.Sign(sb.Sign); err != nil {
		sb.logger.Error("Error in signing an Istanbul Announce Message", "AnnounceMsg", msg.String(), "err", err)
		return nil, err
	}

	// Convert to payload
	payload, err := msg.Payload()
	if err != nil {
		sb.logger.Error("Error in converting Istanbul Announce Message to payload", "AnnounceMsg", msg.String(), "err", err)
		return nil, err
	}

	sb.logger.Trace("Broadcasting an announce message", "AnnounceMsg", msg)

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

func (sb *Backend) retrieveActiveAndRegisteredValidators() (map[common.Address]bool, error) {
	validatorsSet := make(map[common.Address]bool)

	registeredValidators, err := validators.RetrieveRegisteredValidators(nil, nil)

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
	sb.logger.Trace("Handling an IstanbulAnnounce message")

	msg := new(announceMessage)

	// Decode message
	err := msg.FromPayload(payload)
	if err != nil {
		sb.logger.Error("Error in decoding received Istanbul Announce message", "err", err, "payload", hex.EncodeToString(payload))
		return err
	}

	// Verify message signature
	if err := msg.VerifySig(); err != nil {
		sb.logger.Error("Error in verifying the signature of an Istanbul Announce message", "err", err, "AnnounceMsg", msg.String())
		return err
	}

	// If the message is originally from this node, then ignore it
	if msg.Address == sb.Address() {
		sb.logger.Trace("Received an IstanbulAnnounce message originating from this node. Ignoring it.")
		return nil
	}

	// If the message is not within the registered validator set, then ignore it
	regAndActiveVals, err := sb.retrieveActiveAndRegisteredValidators()
	if err != nil {
		return err
	}

	if !regAndActiveVals[msg.Address] {
		sb.logger.Warn("Received an IstanbulAnnounce message from a non registered validator. Ignoring it.", "AnnounceMsg", msg.String(), "validators", regAndActiveVals, "err", err)
		return errUnauthorizedAnnounceMessage
	}

	// Decrypt the EnodeURL
	nodeKey := ecies.ImportECDSA(sb.GetNodeKey())

	encryptedEndpoint := []byte("")
	destAddresses := make([]string, 0, len(msg.EncryptedEndpointData))
	for _, entry := range msg.EncryptedEndpointData {
		destAddresses = append(destAddresses, common.BytesToAddress(entry[0]).String())
		if bytes.Equal(entry[0], sb.Address().Bytes()) {
			encryptedEndpoint = entry[1]
		}
	}
	endpointBytes, err := nodeKey.Decrypt(encryptedEndpoint, nil, nil)
	if err != nil && len(encryptedEndpoint) > 0 {
		sb.logger.Warn("Error in decrypting endpoint", "err", err, "encryptedEndpoint", encryptedEndpoint)
	}
	enodeURL := msg.IncompleteEnodeURL + string(endpointBytes)

	// Save in the valEnodeTable if mining
	if sb.coreStarted {
		oldEnodeURL, err := sb.valEnodeTable.Upsert(msg.Address, enodeURL, msg.View)
		if err != nil {
			sb.logger.Warn("Error in upserting a valenode entry", "AnnounceMsg", msg, "error", err)
			return err
		}

		// Disconnect from old peer
		if oldEnodeURL != "" {
			sb.RemoveValidatorPeer(oldEnodeURL)
		}

		// Connect to the remote peer if it's part of the current epoch's valset and
		// if this node is also part of the current epoch's valset
		block := sb.currentBlock()
		valSet := sb.getValidators(block.Number().Uint64(), block.Hash())
		if _, remoteNode := valSet.GetByAddress(msg.Address); remoteNode != nil {
			if _, localNode := valSet.GetByAddress(sb.Address()); localNode != nil {
				sb.AddValidatorPeer(enodeURL)
			}
		}
	}

	// Generate the destAddresses hash
	sort.Strings(destAddresses)
	destAddressesHash := istanbul.RLPHash(destAddresses)

	// If we gossiped this address/enodeURL within the last 60 seconds, then don't regossip
	sb.lastAnnounceGossipedMu.RLock()
	if lastGossipTs, ok := sb.lastAnnounceGossiped[msg.Address]; ok {
		if lastGossipTs.enodeURL == enodeURL && bytes.Equal(lastGossipTs.destAddressesHash.Bytes(), destAddressesHash.Bytes()) && time.Since(lastGossipTs.timestamp) < time.Minute {
			sb.logger.Trace("Already regossiped the msg within the last minute, so not regossiping.", "AnnounceMsg", msg)
			sb.lastAnnounceGossipedMu.RUnlock()
			return nil
		}
	}
	sb.lastAnnounceGossipedMu.RUnlock()

	sb.logger.Trace("Regossiping the istanbul announce message", "AnnounceMsg", msg)
	sb.Gossip(nil, payload, istanbulAnnounceMsg, true)

	sb.lastAnnounceGossipedMu.Lock()
	defer sb.lastAnnounceGossipedMu.Unlock()
	sb.lastAnnounceGossiped[msg.Address] = &AnnounceGossipTimestamp{enodeURL: enodeURL, timestamp: time.Now(), destAddressesHash: destAddressesHash}

	// prune non registered validator entries in the valEnodeTable, reverseValEnodeTable, and lastAnnounceGossiped tables about 5% of the times that an announce msg is handled
	if (mrand.Int() % 100) <= 5 {
		for remoteAddress := range sb.lastAnnounceGossiped {
			if !regAndActiveVals[remoteAddress] {
				log.Trace("Deleting entry from the lastAnnounceGossiped table", "address", remoteAddress, "gossip timestamp", sb.lastAnnounceGossiped[remoteAddress])
				delete(sb.lastAnnounceGossiped, remoteAddress)
			}
		}

		err = sb.valEnodeTable.PruneEntries(regAndActiveVals)
		return err
	}

	return nil
}
