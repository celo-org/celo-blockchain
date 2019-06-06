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

package core

import (
	"github.com/ethereum/go-ethereum/consensus/istanbul"
)

func (c *core) sendAnnounce() {
	logger := c.logger.New()

	block, _ := c.backend.LastProposal()

	enode := c.backend.Enode()
	if enode == nil {
		logger.Warn("Enode is nil in sendAnnounce")
		return
	}

	enodeUrl := c.backend.Enode().String()

	announce, err := Encode(&istanbul.Announce{BlockNum: block.Number(),
		EnodeURL: enodeUrl})

	if err != nil {
		logger.Error("Failed to encode", "err", err)
		return
	}

	logger.Trace("Broadcasting an announce message", "blockNumber", block.Number(), "enodeURL", enodeUrl)

	c.gossip(&message{
		Code: msgAnnounce,
		Msg:  announce,
	})
}

func (c *core) handleAnnounce(msg *message) error {
	logger := c.logger.New("from", msg.Address, "msg", msg)
	logger.Trace("Received a handleAnnounce message")

	// Decode ANNOUNCE
	var announce *istanbul.Announce
	err := msg.Decode(&announce)
	if err != nil {
		return errFailedDecodeAnnounce
	}

	fromAddress := msg.Address
	c.valEnodeTableMu.Lock()
	defer c.valEnodeTableMu.Unlock()
	if valEnodeEntry, ok := c.valEnodeTable[fromAddress]; ok {
		// If it is old message, ignore it.
		if announce.BlockNum.Cmp(valEnodeEntry.blockNum) <= 0 {
			logger.Trace("Received an old announe message.  Ignoring it.", "from", msg.Address.Hex(), "blockNum", announce.BlockNum, "enode", announce.EnodeURL)
			return errOldMessage
		} else {
			// Check if the enode has been changed
			if announce.EnodeURL != valEnodeEntry.enodeURL {
				if valEnodeEntry.addPeerAttempted {
					// Remove the peer
					c.backend.RemoveStaticPeer(valEnodeEntry.enodeURL)
					valEnodeEntry.addPeerAttempted = false
				}
				valEnodeEntry.enodeURL = announce.EnodeURL
			}
			valEnodeEntry.blockNum = announce.BlockNum
		}
	} else {
		c.valEnodeTable[fromAddress] = &ValidatorEnode{blockNum: announce.BlockNum, enodeURL: announce.EnodeURL}
	}

	// Check if we need to add the peer
	if _, v := c.valSet.GetByAddress(fromAddress); v != nil {
		if !c.valEnodeTable[fromAddress].addPeerAttempted {
			// Add the peer
			c.backend.AddStaticPeer(announce.EnodeURL)
			c.valEnodeTable[fromAddress].addPeerAttempted = true
		}
	}

	return nil
}
