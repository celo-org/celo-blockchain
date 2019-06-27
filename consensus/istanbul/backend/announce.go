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
	"fmt"
	"io"
	mrand "math/rand"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/istanbul"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/crypto/ecies"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/rlp"
)

const (
	getValidatorABI = `[{
		"constant": true,
		"inputs": [
		  {
			"name": "account",
			"type": "address"
		  }
		],
		"name": "getValidator",
		"outputs": [
		  {
			"name": "",
			"type": "string"
		  },
		  {
			"name": "",
			"type": "string"
		  },
		  {
			"name": "",
			"type": "string"
		  },
		  {
			"name": "",
			"type": "bytes"
		  },
		  {
			"name": "",
			"type": "address"
		  }
		],
		"payable": false,
		"stateMutability": "view",
		"type": "function"
	  }]`
)

var (
	getValidatorFuncABI, _ = abi.JSON(strings.NewReader(getValidatorABI))
)
// ==============================================
//
// define the istanbul announce message

type announceMessage struct {
	Address   common.Address
	EnodeURL  string
	View      *istanbul.View
	Signature []byte
}

func (am *announceMessage) String() string {
	return fmt.Sprintf("{Address: %s, View: %v, EnodeURL: %v, Signature: %v}", am.Address.String(), am.View, am.EnodeURL, hex.EncodeToString(am.Signature))
}

// ==============================================
//
// define the functions that needs to be provided for rlp Encoder/Decoder.

// EncodeRLP serializes am into the Ethereum RLP format.
func (am *announceMessage) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, []interface{}{am.Address, am.EnodeURL, am.View, am.Signature})
}

// DecodeRLP implements rlp.Decoder, and load the am fields from a RLP stream.
func (am *announceMessage) DecodeRLP(s *rlp.Stream) error {
	var msg struct {
		Address   common.Address
		EnodeURL  string
		View      *istanbul.View
		Signature []byte
	}

	if err := s.Decode(&msg); err != nil {
		return err
	}
	am.Address, am.EnodeURL, am.View, am.Signature = msg.Address, msg.EnodeURL, msg.View, msg.Signature
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
	payloadNoSig, err := rlp.EncodeToBytes(&announceMessage{Address: am.Address,
		EnodeURL:  am.EnodeURL,
		View:      am.View,
		Signature: []byte{}})
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
		Address:   am.Address,
		EnodeURL:  am.EnodeURL,
		View:      am.View,
		Signature: []byte{}})
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

func (sb *Backend) sendIstAnnounce() error {
	log.Info("hello")
	block := sb.currentBlock()
	state, err := sb.stateAt(block.Header().ParentHash)
	if err != nil {
		log.Error("verify - Error in getting the block's parent's state", "parentHash", block.Header().ParentHash.Hex(), "err", err)
		return err
	}

	enode := sb.Enode()
	if enode == nil {
		sb.logger.Error("Enode is nil in sendIstAnnounce")
		return nil
	}

	enodeUrl := enode.String()
	view := sb.core.CurrentView()

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
		return err
	}

	validatorsAddress, err := sb.regAdd.GetRegisteredAddress(params.ValidatorsRegistryId)
	if err != nil {
		log.Warn("Registry address lookup failed", "err", err)
		return errValidatorsContractNotRegistered
	}

	encryptedEnodeUrls := make(map[common.Address]string)
	log.Info("wow", "poo", regVals)
	for addr := range regVals {
		var newValSet interface{}
		if _, err := sb.iEvmH.MakeStaticCall(*validatorsAddress, getValidatorFuncABI, "getValidator", []interface{}{}, &newValSet, uint64(1000000), block.Header(), state); err != nil {
			log.Error("Unable to retrieve total supply from the Gold token smart contract", "err", err)
			return err
		}
		ECDSAKey, err := crypto.UnmarshalPubkey(newValSet.publicKey)
		pubKey := ecies.ImportECDSAPublic(ECDSAKey)
		encryptedEnodeUrl, err := ecies.Encrypt(rand.Reader, pubKey, []byte(enodeUrl), nil, nil)
		if err != nil {
			log.Error("Unable to unmarshal public key", "err", err)
		} else {
			encryptedEnodeUrls[addr] = string(encryptedEnodeUrl)
		}
	}

	msg := &announceMessage{Address: sb.Address(),
		EnodeURL: enodeUrl,
		View:     view}

	// Sign the announce message
	if err := msg.Sign(sb.Sign); err != nil {
		sb.logger.Error("Error in signing an Istanbul Announce Message", "AnnounceMsg", msg.String(), "err", err)
		return err
	}

	// Convert to payload
	payload, err := msg.Payload()
	if err != nil {
		sb.logger.Error("Error in converting Istanbul Announce Message to payload", "AnnounceMsg", msg.String(), "err", err)
		return err
	}

	sb.logger.Trace("Broadcasting an announce message", "AnnounceMsg", msg)

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
		sb.logger.Warn("Received an IstanbulAnnounce message from a non registered validator. Ignoring it.", "AnnounceMsg", msg.String())
		return errUnauthorizedAnnounceMessage
	}

	// Save in the valEnodeTable if mining
	if sb.coreStarted {
		block := sb.currentBlock()
		valSet := sb.getValidators(block.Number().Uint64(), block.Hash())

		newValEnode := &validatorEnode{enodeURL: msg.EnodeURL, view: msg.View}
		if err := sb.valEnodeTable.upsert(msg.Address, newValEnode, valSet, sb.Address()); err != nil {
			sb.logger.Error("Error in upserting a valenode entry", "AnnounceMsg", msg, "error", err)
			return err
		}
	}

	// If we gossiped this address/enodeURL within the last 60 seconds, then don't regossip
	if lastGossipTs, ok := sb.lastAnnounceGossiped[msg.Address]; ok {
		if lastGossipTs.enodeURL == msg.EnodeURL && time.Since(lastGossipTs.timestamp) < time.Minute {
			sb.logger.Trace("Already regossiped the msg within the last minute, so not regossiping.", "AnnounceMsg", msg)
			return nil
		}
	}

	sb.logger.Trace("Regossiping the istanbul announce message", "AnnounceMsg", msg)
	sb.Gossip(nil, payload, istanbulAnnounceMsg, true)

	sb.lastAnnounceGossiped[msg.Address] = &AnnounceGossipTimestamp{enodeURL: msg.EnodeURL, timestamp: time.Now()}

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
