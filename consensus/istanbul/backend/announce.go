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
	"math/rand"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/istanbul"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/rlp"
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

func (sb *Backend) sendIstAnnounce() error {
	logger := sb.logger.New()

	enode := sb.Enode()
	if enode == nil {
		logger.Warn("Enode is nil in sendIstAnnounce")
		return nil
	}

	enodeUrl := enode.String()
	view := sb.core.CurrentView()

	msg := &announceMessage{Address: sb.Address(),
		EnodeURL: enodeUrl,
		View:     view}

	// Sign the announce message
	if err := msg.Sign(sb.Sign); err != nil {
		logger.Error("Error in signing an Istanbul Announce Message", "msg", msg.String(), "err", err)
		return err
	}

	// Convert to payload
	payload, err := msg.Payload()
	if err != nil {
		logger.Error("Error in converting Istanbul Announce Message to payload", "msg", msg.String(), "err", err)
		return err
	}

	logger.Trace("Broadcasting an announce message", "msg", msg)

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
		sb.logger.Error("Error in verifying the signature of an Istanbul Announce message", "err", err, "msg", msg.String())
		return err
	}

	// If the message is originally from this node, then ignore it
	if msg.Address == sb.Address() {
		sb.logger.Trace("Received an IstanbulAnnounce message originating from this node. Ignoring it.")
		return nil
	}

	// If the message is not within the registered validator set, then ignore it
	regVals := sb.retrieveRegisteredValidators()
	if !regVals[msg.Address] {
		sb.logger.Warn("Received an IstanbulAnnounce message from a non registered validator. Ignoring it.", "msg", msg.String())
		return errUnauthorizedAnnounceMessage
	}

	// Save in the valEnodeTable if mining
	if sb.coreStarted {
		sb.valEnodeTableMu.Lock()
		defer sb.valEnodeTableMu.Unlock()

		if valEnodeEntry, ok := sb.valEnodeTable[msg.Address]; ok {
			// If it is old message, ignore it.
			if msg.View.Cmp(valEnodeEntry.view) <= 0 {
				sb.logger.Trace("Received an old announce message.  Ignoring it.", "from", msg.Address.Hex(), "view", msg.View, "enode", msg.EnodeURL)
				return errOldAnnounceMessage
			} else {
				// Check if the enode has been changed
				if msg.EnodeURL != valEnodeEntry.enodeURL {
					// Disconnect from the peer
					sb.RemoveValidatorPeer(valEnodeEntry.enodeURL)
					valEnodeEntry.enodeURL = msg.EnodeURL
					sb.reverseValEnodeTable[msg.EnodeURL] = msg.Address
				}
				valEnodeEntry.view = msg.View
				sb.logger.Trace("Updated an entry in the valEnodeTable", "address", msg.Address, "ValidatorEnode", sb.valEnodeTable[msg.Address].String())
			}
		} else {
			sb.valEnodeTable[msg.Address] = &ValidatorEnode{view: msg.View, enodeURL: msg.EnodeURL}
			sb.reverseValEnodeTable[msg.EnodeURL] = msg.Address
			sb.logger.Trace("Created an entry in the valEnodeTable", "address", msg.Address, "ValidatorEnode", sb.valEnodeTable[msg.Address].String())
		}

		block := sb.currentBlock()
		valSet := sb.getValidators(block.Number().Uint64(), block.Hash())

		// Connect to the remote peer if it's part of the current epoch's valset and
		// if this node is also part of the current epoch's valset
		if _, remoteVal := valSet.GetByAddress(msg.Address); remoteVal != nil {
			if _, localNode := valSet.GetByAddress(sb.Address()); localNode != nil {
				sb.AddValidatorPeer(msg.EnodeURL)
			}
		}
	}

	// If we gossiped this address/enodeURL within the last 60 seconds, then don't regossip
	if lastGossipTs, ok := sb.lastAnnounceGossiped[msg.Address]; ok {
		if lastGossipTs.enodeURL == msg.EnodeURL && time.Since(lastGossipTs.timestamp) < time.Minute {
			sb.logger.Trace("Already regossiped the msg within the last minute, so not regossiping.", "msg", msg)
			return nil
		}
	}

	sb.logger.Trace("Regossiping the istanbul announce message", "msg", msg)
	sb.Gossip(nil, payload, istanbulAnnounceMsg, true)

	sb.lastAnnounceGossiped[msg.Address] = &AnnounceGossipTimestamp{enodeURL: msg.EnodeURL, timestamp: time.Now()}

	// prune non registered validator entries in the valEnodeTable, reverseValEnodeTable, and lastAnnounceGossiped tables about 5% of the times that an announce msg is handled
	if (rand.Int() % 100) <= 5 {
		for remoteAddress := range sb.lastAnnounceGossiped {
			if !regVals[remoteAddress] {
				log.Trace("Deleting entry from the lastAnnounceGossiped table", "address", remoteAddress, "gossip timestamp", sb.lastAnnounceGossiped[remoteAddress])
				delete(sb.lastAnnounceGossiped, remoteAddress)
			}
		}

		sb.valEnodeTableMu.Lock()
		for remoteAddress := range sb.valEnodeTable {
			if !regVals[remoteAddress] {
				log.Trace("Deleting entry from the valEnodeTable and reverseValEnodeTable table", "address", remoteAddress, "valEnodeEntry", sb.valEnodeTable[remoteAddress].String())
				delete(sb.reverseValEnodeTable, sb.valEnodeTable[remoteAddress].enodeURL)
				delete(sb.valEnodeTable, remoteAddress)
			}
		}
		sb.valEnodeTableMu.Unlock()
	}

	return nil
}
