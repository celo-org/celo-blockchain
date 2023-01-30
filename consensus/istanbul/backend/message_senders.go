// Copyright 2017 The celo Authors
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
	"fmt"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/consensus"
	"github.com/celo-org/celo-blockchain/consensus/istanbul"
	"github.com/celo-org/celo-blockchain/crypto"
	"github.com/celo-org/celo-blockchain/p2p"
	"github.com/celo-org/celo-blockchain/p2p/enode"
)

// This function will return the peers with the addresses in the "destAddresses" parameter.
func (sb *Backend) getPeersFromDestAddresses(destAddresses []common.Address) map[enode.ID]consensus.Peer {
	var targets map[enode.ID]bool
	if destAddresses != nil {
		targets = make(map[enode.ID]bool)
		for _, addr := range destAddresses {
			if valNode, err := sb.valEnodeTable.GetNodeFromAddress(addr); valNode != nil && err == nil {
				targets[valNode.ID()] = true
			}
		}
	}
	return sb.broadcaster.FindPeers(targets, p2p.AnyPurpose)
}

// Multicast implements istanbul.Backend.Multicast
// Multicast will send the eth message (with the message's payload and msgCode field set to the params
// payload and ethMsgCode respectively) to the nodes with the signing address in the destAddresses param.
// If this node is proxied and destAddresses is not nil, the message will be wrapped
// in an istanbul.ForwardMessage to ensure the proxy sends it to the correct
// destAddresses.
func (sb *Backend) Multicast(destAddresses []common.Address, msg *istanbul.Message, ethMsgCode uint64, sendToSelf bool) error {
	logger := sb.logger.New("func", "Multicast")

	var err error
	payload, err := msg.Payload()
	if err != nil {
		return fmt.Errorf("error getting payload from msg: %w", err)
	}

	if sb.IsProxiedValidator() {
		err = sb.proxiedValidatorEngine.SendForwardMsgToAllProxies(destAddresses, ethMsgCode, payload)
		if err != nil {
			logger.Warn("Error in sending forward message to the proxies", "err", err)
		}
	} else {
		destPeers := sb.getPeersFromDestAddresses(destAddresses)
		if len(destPeers) > 0 {
			sb.asyncMulticast(destPeers, payload, ethMsgCode)
		}
	}

	if sendToSelf {
		// Send to self.  Note that it will never be a wrapped version of the consensus message.
		go func() {
			if err := sb.istanbulEventMux.Post(msg); err != nil {
				logger.Warn("Error in posting message to self", "err", err)
			}
		}()
	}

	return err
}

// Gossip implements istanbul.Backend.Gossip
// Gossip will gossip the eth message to all connected peers
func (sb *Backend) Gossip(payload []byte, ethMsgCode uint64) error {
	logger := sb.logger.New("func", "Gossip")

	// Get all connected peers
	peersToSendMsg := sb.broadcaster.FindPeers(nil, p2p.AnyPurpose)

	// Mark that this node gossiped/processed this message, so that it will ignore it if
	// one of it's peers sends the message to it.
	sb.gossipCache.MarkMessageProcessedBySelf(payload)

	// Filter out peers that already sent us this gossip message
	for nodeID, peer := range peersToSendMsg {
		nodePubKey := peer.Node().Pubkey()
		nodeAddr := crypto.PubkeyToAddress(*nodePubKey)
		if sb.gossipCache.CheckIfMessageProcessedByPeer(nodeAddr, payload) {
			delete(peersToSendMsg, nodeID)
			logger.Trace("Peer already gossiped this message.  Not sending message to it", "peer", peer)
			continue
		} else {
			sb.gossipCache.MarkMessageProcessedByPeer(nodeAddr, payload)
		}
	}

	sb.asyncMulticast(peersToSendMsg, payload, ethMsgCode)

	return nil
}

// sendMsg will asynchronously send the the Celo messages to all the peers in the destPeers param.
func (sb *Backend) asyncMulticast(destPeers map[enode.ID]consensus.Peer, payload []byte, ethMsgCode uint64) {
	logger := sb.logger.New("func", "AsyncMulticastCeloMsg", "msgCode", ethMsgCode)

	for _, peer := range destPeers {
		peer := peer // Create new instance of peer for the goroutine
		go func() {
			logger.Trace("Sending istanbul message(s) to peer", "peer", peer, "node", peer.Node())
			if err := peer.Send(ethMsgCode, payload); err != nil {
				logger.Warn("Error in sending message", "peer", peer, "ethMsgCode", ethMsgCode, "err", err)
			}
		}()
	}
}

// Unicast asynchronously sends a message to a single peer.
func (sb *Backend) Unicast(peer consensus.Peer, payload []byte, ethMsgCode uint64) {
	peerMap := map[enode.ID]consensus.Peer{peer.Node().ID(): peer}
	sb.asyncMulticast(peerMap, payload, ethMsgCode)
}
