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

package istanbul

import (
	lru "github.com/hashicorp/golang-lru"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/log"
)

type GossipCache interface {
	MarkMessageProcessedByPeer(peerNodeAddr common.Address, payload []byte)
	CheckIfMessageProcessedByPeer(peerNodeAddr common.Address, payload []byte) bool

	MarkMessageProcessedBySelf(payload []byte)
	CheckIfMessageProcessedBySelf(payload []byte) bool
}

type LRUGossipCache struct {
	messageCacheSize   int
	peerRecentMessages *lru.ARCCache // the cache of peer's recent messages
	selfRecentMessages *lru.ARCCache // the cache of self recent messages
}

func NewLRUGossipCache(peerCacheSize, messageCacheSize int) *LRUGossipCache {
	logger := log.New()
	peerRecentMessages, err := lru.NewARC(peerCacheSize)
	if err != nil {
		logger.Crit("Failed to create recent messages cache", "err", err)
	}
	selfRecentMessages, err := lru.NewARC(messageCacheSize)
	if err != nil {
		logger.Crit("Failed to create known messages cache", "err", err)
	}
	return &LRUGossipCache{
		messageCacheSize:   messageCacheSize,
		peerRecentMessages: peerRecentMessages,
		selfRecentMessages: selfRecentMessages,
	}
}

func (gc *LRUGossipCache) MarkMessageProcessedByPeer(peerNodeAddr common.Address, payload []byte) {
	ms, ok := gc.peerRecentMessages.Get(peerNodeAddr)
	var m *lru.ARCCache
	if ok {
		m, _ = ms.(*lru.ARCCache)
	} else {
		m, _ = lru.NewARC(gc.messageCacheSize)
		gc.peerRecentMessages.Add(peerNodeAddr, m)
	}
	payloadHash := RLPHash(payload)
	m.Add(payloadHash, true)
}

func (gc *LRUGossipCache) CheckIfMessageProcessedByPeer(peerNodeAddr common.Address, payload []byte) bool {
	ms, ok := gc.peerRecentMessages.Get(peerNodeAddr)
	var m *lru.ARCCache
	if ok {
		m, _ = ms.(*lru.ARCCache)
		payloadHash := RLPHash(payload)
		_, ok := m.Get(payloadHash)
		return ok
	}

	return false
}

func (gc *LRUGossipCache) MarkMessageProcessedBySelf(payload []byte) {
	payloadHash := RLPHash(payload)
	gc.selfRecentMessages.Add(payloadHash, true)
}

func (gc *LRUGossipCache) CheckIfMessageProcessedBySelf(payload []byte) bool {
	payloadHash := RLPHash(payload)
	_, ok := gc.selfRecentMessages.Get(payloadHash)
	return ok
}
