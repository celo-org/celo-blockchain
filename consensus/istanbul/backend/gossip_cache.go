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
	lru "github.com/hashicorp/golang-lru"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/consensus/istanbul"
)

func (sb *Backend) markMessageProcessedByPeer(peerNodeAddr common.Address, payload []byte) {
	ms, ok := sb.peerRecentMessages.Get(peerNodeAddr)
	var m *lru.ARCCache
	if ok {
		m, _ = ms.(*lru.ARCCache)
	} else {
		m, _ = lru.NewARC(inmemoryMessages)
		sb.peerRecentMessages.Add(peerNodeAddr, m)
	}
	payloadHash := istanbul.RLPHash(payload)
	m.Add(payloadHash, true)
}

func (sb *Backend) checkIfMessageProcessedByPeer(peerNodeAddr common.Address, payload []byte) bool {
	ms, ok := sb.peerRecentMessages.Get(peerNodeAddr)
	var m *lru.ARCCache
	if ok {
		m, _ = ms.(*lru.ARCCache)
		payloadHash := istanbul.RLPHash(payload)
		_, ok := m.Get(payloadHash)
		return ok
	}

	return false
}

func (sb *Backend) markMessageProcessedBySelf(payload []byte) {
	payloadHash := istanbul.RLPHash(payload)
	sb.selfRecentMessages.Add(payloadHash, true)
}

func (sb *Backend) checkIfMessageProcessedBySelf(payload []byte) bool {
	payloadHash := istanbul.RLPHash(payload)
	_, ok := sb.selfRecentMessages.Get(payloadHash)
	return ok
}
