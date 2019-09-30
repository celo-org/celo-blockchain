// Copyright 2017 The go-ethereum Authors
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

package backend

import (
	"errors"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/consensus/istanbul"
	"github.com/ethereum/go-ethereum/p2p"
	lru "github.com/hashicorp/golang-lru"
)

const (
	istanbulMsg              = 0x11
	istanbulAnnounceMsg      = 0x12
	istanbulValEnodeShareMsg = 0x13
)

var (
	// errDecodeFailed is returned when decode message fails
	errDecodeFailed = errors.New("fail to decode istanbul message")
)

// Protocol implements consensus.Engine.Protocol
func (sb *Backend) Protocol() consensus.Protocol {
	return consensus.Protocol{
		Name:     "istanbul",
		Versions: []uint{64},
		Lengths:  []uint64{20},
		Primary:  true,
	}
}

// HandleMsg implements consensus.Handler.HandleMsg
func (sb *Backend) HandleMsg(addr common.Address, msg p2p.Msg) (bool, error) {
	sb.coreMu.Lock()
	defer sb.coreMu.Unlock()

	if (msg.Code == istanbulMsg) || (msg.Code == istanbulAnnounceMsg) || (msg.Code == istanbulValEnodeShareMsg) {
		if !sb.coreStarted && (msg.Code == istanbulMsg) {
			return true, istanbul.ErrStoppedEngine
		}

		var data []byte
		if err := msg.Decode(&data); err != nil {
			return true, errDecodeFailed
		}

		hash := istanbul.RLPHash(data)

		// Mark peer's message
		ms, ok := sb.recentMessages.Get(addr)
		var m *lru.ARCCache
		if ok {
			m, _ = ms.(*lru.ARCCache)
		} else {
			m, _ = lru.NewARC(inmemoryMessages)
			sb.recentMessages.Add(addr, m)
		}
		m.Add(hash, true)

		// Mark self known message
		if _, ok := sb.knownMessages.Get(hash); ok {
			return true, nil
		}
		sb.knownMessages.Add(hash, true)

		if msg.Code == istanbulMsg {
			go sb.istanbulEventMux.Post(istanbul.MessageEvent{
				Payload: data,
			})
		} else if msg.Code == istanbulAnnounceMsg {
			go sb.handleIstAnnounce(data)
		} else if msg.Code == istanbulValEnodeShareMsg {
			go sb.handleValEnodeShareMsg(data)
		}

		return true, nil
	}
	return false, nil
}

// SetBroadcaster implements consensus.Handler.SetBroadcaster
func (sb *Backend) SetBroadcaster(broadcaster consensus.Broadcaster) {
	sb.broadcaster = broadcaster
}

func (sb *Backend) NewChainHead() error {
	sb.coreMu.RLock()
	defer sb.coreMu.RUnlock()
	if !sb.coreStarted {
		return istanbul.ErrStoppedEngine
	}

	// If the last block of the epoch has just been added to the blockchain, then
	// establish 'validator' type connections to all validators in the upcoming epoch
	// and disconnect from the ones that are no longer in the val set.
	currentBlock := sb.currentBlock()
	if istanbul.IsLastBlockOfEpoch(currentBlock.Number().Uint64(), sb.config.Epoch) {
		sb.logger.Trace("At end of epoch and going to refresh validator peers", "current block number", currentBlock.Number().Uint64())
		valset := sb.getValidators(currentBlock.Number().Uint64(), currentBlock.Hash())
		if _, val := valset.GetByAddress(sb.Address()); val == nil {
			sb.logger.Info("Validators Election Results: Node OUT ValidatorSet")
		} else {
			sb.logger.Info("Validators Election Results: Node IN ValidatorSet")
		}
		go sb.RefreshValPeers(valset)
	}

	go sb.istanbulEventMux.Post(istanbul.FinalCommittedEvent{})
	return nil
}
