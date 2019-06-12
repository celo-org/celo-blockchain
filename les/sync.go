// Copyright 2016 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package les

import (
	"context"
	"time"

	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/eth/downloader"
	"github.com/ethereum/go-ethereum/light"
	"github.com/ethereum/go-ethereum/log"
)

// syncer is responsible for periodically synchronising with the network, both
// downloading hashes and blocks as well as handling the announcement handler.
func (pm *ProtocolManager) syncer() {
	// Start and ensure cleanup of sync mechanisms
	//pm.fetcher.Start()
	//defer pm.fetcher.Stop()
	defer pm.downloader.Terminate()

	// Wait for different events to fire synchronisation operations
	//forceSync := time.Tick(forceSyncCycle)
	for {
		select {
		case <-pm.newPeerCh:
			/*			// Make sure we have peers to select from, then sync
						if pm.peers.Len() < minDesiredPeerCount {
							break
						}
						go pm.synchronise(pm.peers.BestPeer())
			*/
		/*case <-forceSync:
		// Force a sync even if not enough peers are present
		go pm.synchronise(pm.peers.BestPeer())
		*/
		case <-pm.noMorePeers:
			return
		}
	}
}

func (pm *ProtocolManager) needToSync(peerHead blockInfo, mode downloader.SyncMode) bool {
	head := pm.blockchain.CurrentHeader()
	if !mode.SyncFullHeaderChain() {
		// There are two modes where this holds, celolatest and ultralight.
		// In the celolatest mode, the difficulty calculation cannot be done since we don't have all the blocks,
		// so, we rely on the block numbers.
		// In the ultralight mode used for IBFT, each block has a difficulty of 1, so, block number
		// corresponds to the difficulty level.
		currentHead := head.Number.Uint64()
		log.Debug("needToSync", "currentHead", currentHead, "peerHead", peerHead.Number)
		return peerHead.Number > currentHead
	} else {
		currentTd := rawdb.ReadTd(pm.chainDb, head.Hash(), head.Number.Uint64())
		log.Debug("needToSync", "currentTd", currentTd, "peerHead.Td", peerHead.Td)
		return currentTd != nil && peerHead.Td.Cmp(currentTd) > 0
	}
}

// synchronise tries to sync up our local block chain with a remote peer.
func (pm *ProtocolManager) synchronise(peer *peer, mode downloader.SyncMode) {
	// Short circuit if no peers are available
	if peer == nil {
		log.Debug("Synchronise no peer available")
		return
	}

	// Make sure the peer's TD is higher than our own.
	if !pm.needToSync(peer.headBlockInfo(), mode) {
		log.Debug("synchronise no need to sync")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()
	pm.blockchain.(*light.LightChain).SyncCht(ctx)
	pm.downloader.Synchronise(peer.id, peer.Head(), peer.Td(), mode)
}
