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
	"math/big"

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
	BlockNum  *big.Int
	Signature []byte
}

func (am *announceMessage) String() string {
	return fmt.Sprintf("{BlockNum: %v, EnodeURL: %v, Signature: %v}", am.BlockNum, am.EnodeURL, hex.EncodeToString(am.Signature))
}

// ==============================================
//
// define the functions that needs to be provided for rlp Encoder/Decoder.

// EncodeRLP serializes am into the Ethereum RLP format.
func (am *announceMessage) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, []interface{}{am.Address, am.EnodeURL, am.BlockNum, am.Signature})
}

// DecodeRLP implements rlp.Decoder, and load the am fields from a RLP stream.
func (am *announceMessage) DecodeRLP(s *rlp.Stream) error {
	var msg struct {
		Address   common.Address
		EnodeURL  string
		BlockNum  *big.Int
		Signature []byte
	}

	if err := s.Decode(&msg); err != nil {
		return err
	}
	am.Address, am.EnodeURL, am.BlockNum, am.Signature = msg.Address, msg.EnodeURL, msg.BlockNum, msg.Signature
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
	payload, err := rlp.EncodeToBytes(am)
	if err != nil {
		return nil, err
	}

	return payload, nil
}

func (am *announceMessage) Sign(signingFn func(data []byte) ([]byte, error)) error {
	msg := &announceMessage{Address: am.Address,
		EnodeURL: am.EnodeURL,
		BlockNum: am.BlockNum}

	data, err := rlp.EncodeToBytes(msg)
	if err != nil {
		return err
	}
	am.Signature, err = signingFn(data)
	return err
}

func (am *announceMessage) VerifySig() error {
	// Construct and encode a message with no signature
	var payloadNoSig []byte
	payloadNoSig, err := rlp.EncodeToBytes(&announceMessage{Address: am.Address,
		EnodeURL:  am.EnodeURL,
		BlockNum:  am.BlockNum,
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
	block := sb.currentBlock()

	logger.Trace("Broadcasting an announce message", "blockNum", block.Number(), "enodeURL", enodeUrl)

	msg := &announceMessage{Address: sb.Address(),
		EnodeURL: enodeUrl,
		BlockNum: block.Number()}

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

	sb.Gossip(nil, payload, istanbulAnnounceMsg)

	return nil
}

func (sb *Backend) handleIstAnnounce(payload []byte) error {
	sb.logger.Trace("Received a handleAnnounce message")

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

	// Save in the valEnodeTable if mining
	if sb.coreStarted {
		sb.valEnodeTableMu.Lock()
		defer sb.valEnodeTableMu.Unlock()
		if valEnodeEntry, ok := sb.valEnodeTable[msg.Address]; ok {
			// If it is old message, ignore it.
			if msg.BlockNum.Cmp(valEnodeEntry.blockNum) <= 0 {
				sb.logger.Trace("Received an old announce message.  Ignoring it.", "from", msg.Address.Hex(), "blockNum", msg.BlockNum, "enode", msg.EnodeURL)
				return errOldAnnounceMessage
			} else {
				// Check if the enode has been changed
				if msg.EnodeURL != valEnodeEntry.enodeURL {
					if valEnodeEntry.addPeerAttempted {
						// Remove the peer
						sb.RemoveStaticPeer(valEnodeEntry.enodeURL)
						valEnodeEntry.addPeerAttempted = false
					}
					valEnodeEntry.enodeURL = msg.EnodeURL
				}
				valEnodeEntry.blockNum = msg.BlockNum
			}
		} else {
			sb.valEnodeTable[msg.Address] = &ValidatorEnode{blockNum: msg.BlockNum, enodeURL: msg.EnodeURL}
		}

		// If the msg.Address is part of the current validator set, then check if we need to add it as a static peer.
		// TODO(kevjue) - This should be changed to check if the msg.Address
		//                is a potential validator for the upcoming epoch.
		block := sb.currentBlock()
		valSet := sb.getValidators(block.Number().Uint64(), block.Hash())

		if _, v := valSet.GetByAddress(msg.Address); v != nil {
			if !sb.valEnodeTable[msg.Address].addPeerAttempted {
				// Add the peer
				sb.AddStaticPeer(msg.EnodeURL)
				sb.valEnodeTable[msg.Address].addPeerAttempted = true
			}
		}
	}

	// Regossip the announce message.
	// TODO(kevjue) - Only regossip if it's a potential validator for the upcoming epoch
	sb.Gossip(nil, payload, istanbulAnnounceMsg)

	return nil
}
