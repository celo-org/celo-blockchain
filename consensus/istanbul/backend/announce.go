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
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	mrand "math/rand"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	// "github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/consensus/istanbul"
	"github.com/ethereum/go-ethereum/crypto/ecies"
	"github.com/ethereum/go-ethereum/log"
	// "github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/rlp"
)

// ==============================================
//
// define the istanbul announce message

type announceMessage struct {
	Address            common.Address
	IncompleteEnodeURL string //TODO(nguo) remove this field
	EncryptedIPData    []byte
	View               *istanbul.View
	Signature          []byte
}

func (am *announceMessage) String() string {
	return fmt.Sprintf("{Address: %s, View: %v, IncompleteEnodeURL: %v, Signature: %v}", am.Address.String(), am.View, am.IncompleteEnodeURL, hex.EncodeToString(am.Signature))
}

// ==============================================
//
// define the functions that needs to be provided for rlp Encoder/Decoder.

// EncodeRLP serializes am into the Ethereum RLP format.
func (am *announceMessage) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, []interface{}{am.Address, am.IncompleteEnodeURL, am.EncryptedIPData, am.View, am.Signature})
}

// DecodeRLP implements rlp.Decoder, and load the am fields from a RLP stream.
func (am *announceMessage) DecodeRLP(s *rlp.Stream) error {
	var msg struct {
		Address            common.Address
		IncompleteEnodeURL string
		EncryptedIPData    []byte
		View               *istanbul.View
		Signature          []byte
	}

	if err := s.Decode(&msg); err != nil {
		return err
	}
	am.Address, am.IncompleteEnodeURL, am.EncryptedIPData, am.View, am.Signature = msg.Address, msg.IncompleteEnodeURL, msg.EncryptedIPData, msg.View, msg.Signature
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
		Address:            am.Address,
		IncompleteEnodeURL: am.IncompleteEnodeURL,
		EncryptedIPData:    am.EncryptedIPData,
		View:               am.View,
		Signature:          []byte{},
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
		Address:            am.Address,
		IncompleteEnodeURL: am.IncompleteEnodeURL,
		EncryptedIPData:    am.EncryptedIPData,
		View:               am.View,
		Signature:          []byte{},
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
	ipData := enodeUrl[strings.Index(enodeUrl, "@"):]

	regVals, err := sb.retrieveRegisteredValidators()
	// The validator contract may not be deployed yet.
	// Even if it is deployed, it may not have any registered validators yet.
	if err == errValidatorsContractNotRegistered || len(regVals) == 0 {
		sb.logger.Trace("Can't retrieve the registered validators.  Only allowing the initial validator set to send announce messages", "err", err, "regVals", regVals)
		valSet := sb.getValidators(block.Number().Uint64(), block.Hash())

		regVals = make(map[common.Address]bool)
		for _, val := range valSet.List() {
			regVals[val.Address()] = true
		}
	} else if err != nil {
		sb.logger.Error("Error in retrieving the registered validators", "err", err)
		return nil, err
	}

	encryptedIPs := make(map[common.Address][]byte)
	for addr := range regVals {
		if validatorEnodeEntry, ok := sb.valEnodeTable.valEnodeTable[addr]; ok {
			validatorEnode, err := enode.ParseV4(validatorEnodeEntry.enodeURL)
			pubKey := ecies.ImportECDSAPublic(validatorEnode.Pubkey())
			encryptedIP, err := ecies.Encrypt(rand.Reader, pubKey, []byte(ipData), nil, nil)
			if err != nil {
				log.Warn("Unable to unmarshal public key", "err", err)
			} else {
				encryptedIPs[addr] = encryptedIP
			}
		}
	}

	encryptedIPData, err := json.Marshal(encryptedIPs)
	if err != nil {
		sb.logger.Error("Error in marshaling encrypted enode data", "err", err)
		return nil, err
	}

	msg := &announceMessage{
		Address:            sb.Address(),
		IncompleteEnodeURL: incompleteEnodeUrl,
		EncryptedIPData:    encryptedIPData,
		View:               view,
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

func (sb *Backend) handleIstAnnounce(payload []byte) error {
	sb.logger.Trace("Handling an IstanbulAnnounce message")

	msg := new(announceMessage)
	// Decode message
	err := msg.FromPayload(payload)
	if err != nil {
		sb.logger.Error("Error in decoding received Istanbul Announce message", "err", err, "payload", hex.EncodeToString(payload))
		return err
	}

	log.Error("Handling an IstanbulAnnounce message", "actual", msg.IncompleteEnodeURL)

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
	regVals, err := sb.retrieveRegisteredValidators()

	// The validator contract may not be deployed yet.
	// Even if it is deployed, it may not have any registered validators yet.
	if err == errValidatorsContractNotRegistered || len(regVals) == 0 {
		sb.logger.Trace("Can't retrieve the registered validators.  Only allowing the initial validator set to send announce messages", "err", err, "regVals", regVals)
		block := sb.currentBlock()
		valSet := sb.getValidators(block.Number().Uint64(), block.Hash())

		regVals = make(map[common.Address]bool)
		for _, val := range valSet.List() {
			regVals[val.Address()] = true
		}
	} else if err != nil {
		sb.logger.Error("Error in retrieving the registered validators", "err", err)
		return err
	}

	if !regVals[msg.Address] {
		sb.logger.Warn("Received an IstanbulAnnounce message from a non registered validator. Ignoring it.", "AnnounceMsg", msg.String(), "validators", regVals, "err", err)
		return errUnauthorizedAnnounceMessage
	}

	// Decrypt the EnodeURL
	nodeKey := ecies.ImportECDSA(sb.GetNodeKey())

	var encryptedIPs map[common.Address][]byte
	json.Unmarshal(msg.EncryptedIPData, &encryptedIPs)
	encryptedIP := encryptedIPs[sb.Address()]
	IPBytes, err := nodeKey.Decrypt(encryptedIP, nil, nil)
	if err != nil {
		sb.logger.Warn("Error in decrypting ip", "err", err)
	}
	enodeUrl := msg.IncompleteEnodeURL + string(IPBytes)

	log.Error("Handling an IstanbulAnnounce message", "actual", msg.IncompleteEnodeURL, "lol", enodeUrl)

	// Save in the valEnodeTable if mining
	if sb.coreStarted {
		block := sb.currentBlock()
		valSet := sb.getValidators(block.Number().Uint64(), block.Hash())

		newValEnode := &validatorEnode{enodeURL: enodeUrl, view: msg.View}
		if err := sb.valEnodeTable.upsert(msg.Address, newValEnode, valSet, sb.Address()); err != nil {
			sb.logger.Error("Error in upserting a valenode entry", "AnnounceMsg", msg, "error", err)
			return err
		}
	}

	// If we gossiped this address/enodeURL within the last 60 seconds, then don't regossip
	if lastGossipTs, ok := sb.lastAnnounceGossiped[msg.Address]; ok {
		if lastGossipTs.enodeURL == enodeUrl && time.Since(lastGossipTs.timestamp) < time.Minute {
			sb.logger.Trace("Already regossiped the msg within the last minute, so not regossiping.", "AnnounceMsg", msg)
			return nil
		}
	}

	sb.logger.Trace("Regossiping the istanbul announce message", "AnnounceMsg", msg)
	sb.Gossip(nil, payload, istanbulAnnounceMsg, true)

	sb.lastAnnounceGossiped[msg.Address] = &AnnounceGossipTimestamp{enodeURL: enodeUrl, timestamp: time.Now()}

	// prune non registered validator entries in the valEnodeTable, reverseValEnodeTable, and lastAnnounceGossiped tables about 5% of the times that an announce msg is handled
	if (mrand.Int() % 100) <= 5 {
		for remoteAddress := range sb.lastAnnounceGossiped {
			if !regVals[remoteAddress] {
				log.Trace("Deleting entry from the lastAnnounceGossiped table", "address", remoteAddress, "gossip timestamp", sb.lastAnnounceGossiped[remoteAddress])
				delete(sb.lastAnnounceGossiped, remoteAddress)
			}
		}

		sb.valEnodeTable.pruneEntries(regVals)
	}

	return nil
}
