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
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/istanbul"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/rlp"
)

// ==============================================
//
// define the validator enode share message

type sharedValidatorEnode struct {
	Address  common.Address
	EnodeURL string
	View     *istanbul.View
}

type valEnodeShareMessage struct {
	ValEnodes []sharedValidatorEnode
}

func (sve *sharedValidatorEnode) String() string {
	return fmt.Sprintf("{Address: %s, EnodeURL: %v, View: %v}", sve.Address.Hex(), sve.EnodeURL, sve.View)
}

func (sm *valEnodeShareMessage) String() string {
	outputStr := "{ValEnodes:"
	for _, valEnode := range sm.ValEnodes {
		outputStr = fmt.Sprintf("%s %s", outputStr, valEnode.String())
	}
	return fmt.Sprintf("%s}", outputStr)
}

// ==============================================
//
// define the functions that needs to be provided for rlp Encoder/Decoder.

// EncodeRLP serializes sm into the Ethereum RLP format.
func (sm *valEnodeShareMessage) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, []interface{}{sm.ValEnodes})
}

// DecodeRLP implements rlp.Decoder, and load the sm fields from a RLP stream.
func (sm *valEnodeShareMessage) DecodeRLP(s *rlp.Stream) error {
	var msg struct {
		ValEnodes []sharedValidatorEnode
	}

	if err := s.Decode(&msg); err != nil {
		return err
	}
	sm.ValEnodes = msg.ValEnodes
	return nil
}

// This function is meant to be run as a goroutine.  It will periodically gossip validator enode share messages
// to this node's sentries so that sentries know the enodes of validators
func (sb *Backend) sendValEnodeShareMsgs() {
	sb.valEnodeShareWg.Add(1)
	defer sb.valEnodeShareWg.Done()

	ticker := time.NewTicker(time.Minute)

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

func (sb *Backend) generateValEnodeShareMsg() (*istanbul.Message, error) {
	sb.valEnodeTable.valEnodeTableMu.RLock()
	sharedValidatorEnodes := make([]sharedValidatorEnode, len(sb.valEnodeTable.valEnodeTable))
	i := 0
	for address, validatorEnode := range sb.valEnodeTable.valEnodeTable {
		sharedValidatorEnodes[i] = sharedValidatorEnode{
			Address:  address,
			EnodeURL: validatorEnode.node.String(),
			View:     validatorEnode.view,
		}
		i++
	}
	sb.valEnodeTable.valEnodeTableMu.RUnlock()

	valEnodeShareMessage := &valEnodeShareMessage{
		ValEnodes: sharedValidatorEnodes,
	}

	valEnodeShareBytes, err := rlp.EncodeToBytes(valEnodeShareMessage)
	if err != nil {
		sb.logger.Error("Error encoding Istanbul Validator Enode Share message content", "ValEnodeShareMsg", valEnodeShareMessage.String(), "err", err)
		return nil, err
	}

	msg := &istanbul.Message{
		Code:      istanbulValEnodeShareMsg,
		Msg:       valEnodeShareBytes,
		Address:   sb.Address(),
		Signature: []byte{},
	}

	sb.logger.Trace("Generated a Istanbul Validator Enode Share message", "IstanbulMsg", msg.String(), "ValEnodeShareMsg", valEnodeShareMessage.String())

	return msg, nil
}

func (sb *Backend) sendValEnodeShareMsg() error {
	msg, err := sb.generateValEnodeShareMsg()
	if err != nil {
		return err
	}

	if msg == nil {
		return nil
	}

	// Sign the validator enode share message
	if err := msg.Sign(sb.Sign); err != nil {
		sb.logger.Error("Error in signing an Istanbul ValEnodeShare Message", "ValEnodeShareMsg", msg.String(), "err", err)
		return err
	}

	// Convert to payload
	payload, err := msg.Payload()
	if err != nil {
		sb.logger.Error("Error in converting Istanbul ValEnodeShare Message to payload", "ValEnodeShareMsg", msg.String(), "err", err)
		return err
	}

	if sb.proxyNode != nil && sb.proxyNode.peer != nil {
		sb.logger.Debug("Sending Istanbul Validator Enode Share payload to proxy peer")
		go sb.proxyNode.peer.Send(istanbulValEnodeShareMsg, payload)
	} else {
		sb.logger.Error("No proxy peers, cannot send Istanbul Validator Enode Share message")
	}

	return nil
}

// TODO: once we add a command line flag indicating a proxy is proxying for a
// certain validator, add a check in here to make sure the message came from
// the correct validator.
func (sb *Backend) handleValEnodeShareMsg(payload []byte) error {
	sb.logger.Debug("Handling an Istanbul Validator Enode Share message")

	msg := new(istanbul.Message)
	// Decode message
	err := msg.FromPayload(payload, istanbul.GetSignatureAddress)
	if err != nil {
		sb.logger.Error("Error in decoding received Istanbul Validator Enode Share message", "err", err, "payload", hex.EncodeToString(payload))
		return err
	}

	var valEnodeShareMessage valEnodeShareMessage
	err = rlp.DecodeBytes(msg.Msg, &valEnodeShareMessage)
	if err != nil {
		sb.logger.Error("Error in decoding received Istanbul Validator Enode Share message content", "err", err, "IstanbulMsg", msg.String())
		return err
	}

	sb.logger.Debug("Received an Istanbul Validator Enode Share message", "IstanbulMsg", msg.String(), "ValEnodeShareMsg", valEnodeShareMessage.String())

	block := sb.currentBlock()
	valSet := sb.getValidators(block.Number().Uint64(), block.Hash())

	sb.valEnodeTable.valEnodeTableMu.Lock()
	defer sb.valEnodeTable.valEnodeTableMu.Unlock()
	for _, sharedValidatorEnode := range valEnodeShareMessage.ValEnodes {
		if err := sb.valEnodeTable.upsertNonLocking(sharedValidatorEnode.Address, sharedValidatorEnode.EnodeURL, sharedValidatorEnode.View, valSet, sb.ValidatorAddress(), false, true); err != nil {
			sb.logger.Warn("Error in upserting a valenode entry", "IstanbulMsg", msg.String(), "ValEnodeShareMsg", valEnodeShareMessage.String(), "error", err)
		}
	}

	sb.logger.Trace("ValidatorEnodeTable dump", "ValidatorEnodeTable", sb.valEnodeTable.String())

	return nil
}
