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
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/consensus/istanbul"
	vet "github.com/ethereum/go-ethereum/consensus/istanbul/backend/internal/enodes"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/rlp"
)

// ==============================================
//
// define the validator enode share message

type sharedValidatorEnode struct {
	Address  common.Address
	EnodeURL string
	Version  uint
}

type valEnodesShareData struct {
	ValEnodes []sharedValidatorEnode
}

func (sve *sharedValidatorEnode) String() string {
	return fmt.Sprintf("{Address: %s, EnodeURL: %v, Version: %v}", sve.Address.Hex(), sve.EnodeURL, sve.Version)
}

func (sd *valEnodesShareData) String() string {
	outputStr := "{ValEnodes:"
	for _, valEnode := range sd.ValEnodes {
		outputStr = fmt.Sprintf("%s %s", outputStr, valEnode.String())
	}
	return fmt.Sprintf("%s}", outputStr)
}

// ==============================================
//
// define the functions that needs to be provided for rlp Encoder/Decoder.

// EncodeRLP serializes sd into the Ethereum RLP format.
func (sd *valEnodesShareData) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, []interface{}{sd.ValEnodes})
}

// DecodeRLP implements rlp.Decoder, and load the sd fields from a RLP stream.
func (sd *valEnodesShareData) DecodeRLP(s *rlp.Stream) error {
	var msg struct {
		ValEnodes []sharedValidatorEnode
	}

	if err := s.Decode(&msg); err != nil {
		return err
	}
	sd.ValEnodes = msg.ValEnodes
	return nil
}

// This function is meant to be run as a goroutine.  It will periodically send validator enode share messages
// to this node's proxies so that proxies know the enodes of validators
func (sb *Backend) valEnodesShareThread() {
	sb.valEnodesShareThreadWg.Add(1)
	defer sb.valEnodesShareThreadWg.Done()

	ticker := time.NewTicker(time.Minute)

	for {
		select {
		case <-ticker.C:
			// output the valEnodeTable for debugging purposes
			log.Trace("ValidatorEnodeTable dump", "ValidatorEnodeTable", sb.valEnodeTable.String())
			go sb.sendValEnodesShareMsg()

		case <-sb.valEnodesShareThreadQuit:
			ticker.Stop()
			return
		}
	}
}

func (sb *Backend) generateValEnodesShareMsg() (*istanbul.Message, error) {
	vetEntries, err := sb.valEnodeTable.GetAllValEnodes()

	if err != nil {
		sb.logger.Error("Error in retrieving all the entries from the ValEnodeTable", "err", err)
		return nil, err
	}

	sharedValidatorEnodes := make([]sharedValidatorEnode, 0, len(vetEntries))
	for address, vetEntry := range vetEntries {
		if vetEntry.Node == nil {
			continue
		}
		sharedValidatorEnodes = append(sharedValidatorEnodes, sharedValidatorEnode{
			Address:  address,
			EnodeURL: vetEntry.Node.String(),
			Version:  vetEntry.Version,
		})
	}

	valEnodesShareData := &valEnodesShareData{
		ValEnodes: sharedValidatorEnodes,
	}

	valEnodesShareBytes, err := rlp.EncodeToBytes(valEnodesShareData)
	if err != nil {
		sb.logger.Error("Error encoding Istanbul Validator Enodes Share message content", "ValEnodesShareData", valEnodesShareData.String(), "err", err)
		return nil, err
	}

	msg := &istanbul.Message{
		Code:      istanbul.ValEnodesShareMsg,
		Msg:       valEnodesShareBytes,
		Address:   sb.Address(),
		Signature: []byte{},
	}

	sb.logger.Trace("Generated a Istanbul Validator Enodes Share message", "IstanbulMsg", msg.String(), "ValEnodesShareData", valEnodesShareData.String())

	return msg, nil
}

func (sb *Backend) sendValEnodesShareMsg() error {
	logger := sb.logger.New("func", "sendValEnodesShareMsg")
	if sb.proxyNode == nil || sb.proxyNode.peer == nil {
		logger.Warn("No proxy peers, cannot send Istanbul Validator Enodes Share message")
		return nil
	}

	msg, err := sb.generateValEnodesShareMsg()
	if err != nil {
		logger.Error("Error generating Istanbul ValEnodesShare Message", "err", err)
		return err
	}

	// Sign the validator enode share message
	if err := msg.Sign(sb.Sign); err != nil {
		logger.Error("Error in signing an Istanbul ValEnodesShare Message", "ValEnodesShareMsg", msg.String(), "err", err)
		return err
	}

	// Convert to payload
	payload, err := msg.Payload()
	if err != nil {
		logger.Error("Error in converting Istanbul ValEnodesShare Message to payload", "ValEnodesShareMsg", msg.String(), "err", err)
		return err
	}

	logger.Trace("Sending Istanbul Validator Enodes Share payload to proxy peer")
	if err := sb.proxyNode.peer.Send(istanbul.ValEnodesShareMsg, payload); err != nil {
		logger.Error("Error sending Istanbul ValEnodesShare Message to proxy", "err", err)
		return err
	}

	return nil
}

func (sb *Backend) handleValEnodesShareMsg(_ common.Address, _ consensus.Peer, payload []byte) error {
	sb.logger.Debug("Handling an Istanbul Validator Enodes Share message")

	msg := new(istanbul.Message)
	// Decode message
	err := msg.FromPayload(payload, istanbul.GetSignatureAddress)
	if err != nil {
		sb.logger.Error("Error in decoding received Istanbul Validator Enode Share message", "err", err, "payload", hex.EncodeToString(payload))
		return err
	}

	// Verify that the sender is from the proxied validator
	if msg.Address != sb.config.ProxiedValidatorAddress {
		sb.logger.Error("Unauthorized valEnodesShare message", "sender address", msg.Address, "authorized sender address", sb.config.ProxiedValidatorAddress)
		return errUnauthorizedValEnodesShareMessage
	}

	var valEnodesShareData valEnodesShareData
	err = rlp.DecodeBytes(msg.Msg, &valEnodesShareData)
	if err != nil {
		sb.logger.Error("Error in decoding received Istanbul Validator Enodes Share message content", "err", err, "IstanbulMsg", msg.String())
		return err
	}

	sb.logger.Trace("Received an Istanbul Validator Enodes Share message", "IstanbulMsg", msg.String(), "ValEnodesShareData", valEnodesShareData.String())

	var upsertBatch []*vet.AddressEntry
	for _, sharedValidatorEnode := range valEnodesShareData.ValEnodes {
		if node, err := enode.ParseV4(sharedValidatorEnode.EnodeURL); err != nil {
			sb.logger.Warn("Error in parsing enodeURL", "enodeURL", sharedValidatorEnode.EnodeURL)
			continue
		} else {
			upsertBatch = append(upsertBatch, &vet.AddressEntry{
				Address: sharedValidatorEnode.Address,
				Node:    node,
				Version: sharedValidatorEnode.Version,
			})
		}
	}

	if len(upsertBatch) > 0 {
		if err := sb.valEnodeTable.UpsertVersionAndEnode(upsertBatch); err != nil {
			sb.logger.Warn("Error in upserting a batch to the valEnodeTable", "IstanbulMsg", msg.String(), "UpsertBatch", upsertBatch, "error", err)
		}
	}

	sb.logger.Trace("ValidatorEnodeTable dump", "ValidatorEnodeTable", sb.valEnodeTable.String())

	return nil
}
