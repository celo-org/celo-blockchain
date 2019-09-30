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
	// "bytes"
	// "crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	// mrand "math/rand"
	// "strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/istanbul"
	// contract_errors "github.com/ethereum/go-ethereum/contract_comm/errors"
	// "github.com/ethereum/go-ethereum/contract_comm/validators"
	// "github.com/ethereum/go-ethereum/crypto/ecies"
	"github.com/ethereum/go-ethereum/log"
	// "github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/rlp"
)

// ==============================================
//
// define the istanbul announce message

type valEnodeShareMessage struct {
	Address               common.Address
    TestPayload           string
	View                  *istanbul.View
	Signature             []byte
}

func (sm *valEnodeShareMessage) String() string {
	return fmt.Sprintf("{Address: %s, TestPayload: %s, View: %v, Signature: %v}", sm.TestPayload, sm.View, hex.EncodeToString(sm.Signature))
}

// ==============================================
//
// define the functions that needs to be provided for rlp Encoder/Decoder.

// EncodeRLP serializes sm into the Ethereum RLP format.
func (sm *valEnodeShareMessage) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, []interface{}{sm.Address, sm.TestPayload, sm.View, sm.Signature})
}

// DecodeRLP implements rlp.Decoder, and load the am fields from a RLP stream.
func (sm *valEnodeShareMessage) DecodeRLP(s *rlp.Stream) error {
	var msg struct {
		Address     common.Address
        TestPayload string
		View        *istanbul.View
		Signature   []byte
	}

	if err := s.Decode(&msg); err != nil {
		return err
	}
	sm.Address, sm.TestPayload, sm.View, sm.Signature = msg.Address, msg.TestPayload, msg.View, msg.Signature
	return nil
}

// ==============================================
//
// define the functions that needs to be provided for the istanbul announce sender and handler
func (sm *valEnodeShareMessage) FromPayload(b []byte) error {
	// Decode message
	err := rlp.DecodeBytes(b, &sm)
	return err
}

func (sm *valEnodeShareMessage) Payload() ([]byte, error) {
	return rlp.EncodeToBytes(sm)
}

func (sm *valEnodeShareMessage) Sign(signingFn func(data []byte) ([]byte, error)) error {
	// Construct and encode a message with no signature
	var payloadNoSig []byte
	payloadNoSig, err := rlp.EncodeToBytes(&valEnodeShareMessage{
		Address:     sm.Address,
		TestPayload: sm.TestPayload,
		View:        sm.View,
		Signature:   []byte{},
	})
	if err != nil {
		return err
	}
	sm.Signature, err = signingFn(payloadNoSig)
	return err
}

func (sm *valEnodeShareMessage) VerifySig() error {
	// Construct and encode a message with no signature
	var payloadNoSig []byte
	payloadNoSig, err := rlp.EncodeToBytes(&valEnodeShareMessage{
		Address:     sm.Address,
		TestPayload: sm.TestPayload,
		View:        sm.View,
		Signature:   []byte{},
	})
	if err != nil {
		return err
	}

	sigAddr, err := istanbul.GetSignatureAddress(payloadNoSig, sm.Signature)
	if err != nil {
		return err
	}

	if sigAddr != sm.Address {
		log.Error("Address in the message is different than the address that signed it",
			"sigAddr", sigAddr.Hex(),
			"msg.Address", sm.Address.Hex())
		return errInvalidSignature
	}

	return nil
}

// This function is meant to be run as a goroutine.  It will periodically gossip announce messages
// to the rest of the registered validators to communicate it's enodeURL to them.
func (sb *Backend) sendValEnodeShareMsgs() {
	sb.valEnodeShareWg.Add(1)
	defer sb.valEnodeShareWg.Done()

	ticker := time.NewTicker(time.Minute / 5.0)

	for {
		select {
		case <-ticker.C:
			// output the valEnodeTable for debugging purposes
			log.Trace("ValidatorEnodeTable dump", "ValidatorEnodeTable", sb.valEnodeTable.String())
			go sb.sendValEnodeShareMsg()

		case <-sb.valEnodeShareQuit:
			ticker.Stop()
			return
		}
	}
}

func (sb *Backend) generateValEnodeShareMsg() ([]byte, error) {
	view := sb.core.CurrentView()

	msg := &valEnodeShareMessage{
		Address:     sb.Address(),
		TestPayload: "hello there",
		View:        view,
	}

	// Sign the announce message
	if err := msg.Sign(sb.Sign); err != nil {
		sb.logger.Error("Error in signing an Istanbul Validator Enode Share message", "ValEnodeShareMsg", msg.String(), "err", err)
		return nil, err
	}

	// Convert to payload
	payload, err := msg.Payload()
	if err != nil {
		sb.logger.Error("Error in converting Istanbul Validator Enode Share message to payload", "ValEnodeShareMsg", msg.String(), "err", err)
		return nil, err
	}

	sb.logger.Trace("Generated a Istanbul Validator Enode Share message", "ValEnodeShareMsg", msg)

	return payload, nil
}

func (sb *Backend) sendValEnodeShareMsg() error {
	payload, err := sb.generateValEnodeShareMsg()
	if err != nil {
		return err
	}

	if payload == nil {
		return nil
	}

	sentryPeers := sb.broadcaster.GetSentryPeers()
	if len(sentryPeers) > 0 {
		sb.logger.Debug("Sending Istanbul Validator Enode Share Message to sentry peers", "sentryPeers", sentryPeers, "len", len(sentryPeers))
		for _, sentryPeer := range sentryPeers {
			sentryPeer.Send(istanbulValEnodeShareMsg, payload)
		}
	} else {
		sb.logger.Warn("No sentry peers, cannot send Istanbul Validator Enode Share message")
	}

	return nil
}

// TODO: once we add a command line flag indicating a sentry is proxying for a
// certain validator, add a check in here to make sure the message came from
// the correct validator.
func (sb *Backend) handleValEnodeShareMsg(payload []byte) error {
	sb.logger.Warn("Handling an Istanbul Validator Enode Message message")

	msg := new(valEnodeShareMessage)
	// Decode message
	err := msg.FromPayload(payload)
	if err != nil {
		sb.logger.Error("Error in decoding received Istanbul Validator Enode Share Message message", "err", err, "payload", hex.EncodeToString(payload))
		return err
	}

	// Verify message signature
	if err := msg.VerifySig(); err != nil {
		sb.logger.Error("Error in verifying the signature of an Istanbul Validator Enode Share message", "err", err, "ValEnodeShareMsg", msg.String())
		return err
	}

	// If the message is originally from this node, then ignore it
	if msg.Address == sb.Address() {
		sb.logger.Trace("Received a ValEnodeShareMsg message originating from this node. Ignoring it.")
		return nil
	}

	sb.logger.Warn("woo! got Istanbul Validator Enode Share message", "msg", msg.String())

	// // Decrypt the EnodeURL
	// nodeKey := ecies.ImportECDSA(sb.GetNodeKey())

	// // Save in the valEnodeTable if mining
	// if sb.coreStarted {
	// 	block := sb.currentBlock()
	// 	valSet := sb.getValidators(block.Number().Uint64(), block.Hash())
	//
	// 	newValEnode := &validatorEnode{enodeURL: enodeUrl, view: msg.View}
	// 	if err := sb.valEnodeTable.upsert(msg.Address, newValEnode, valSet, sb.Address()); err != nil {
	// 		sb.logger.Warn("Error in upserting a valenode entry", "AnnounceMsg", msg, "error", err)
	// 		return err
	// 	}
	// }

	return nil
}
