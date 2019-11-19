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

package core

import (
	"fmt"
	"strings"
	"sync"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/istanbul"
)

// Construct a new message set to accumulate messages for given sequence/view number.
func newMessageSet(valSet istanbul.ValidatorSet) MessageSet {
	return &messageSetImpl{
		messagesMu: new(sync.Mutex),
		messages:   make(map[common.Address]*istanbul.Message),
		valSet:     valSet,
	}
}

// ----------------------------------------------------------------------------

type MessageSet interface {
	fmt.Stringer
	Add(msg *istanbul.Message) error
	GetAddressIndex(addr common.Address) (uint64, error)
	Remove(address common.Address)
	Values() (result []*istanbul.Message)
	Size() int
	Get(addr common.Address) *istanbul.Message
}

type messageSetImpl struct {
	valSet     istanbul.ValidatorSet
	messagesMu *sync.Mutex
	messages   map[common.Address]*istanbul.Message
}

func (ms *messageSetImpl) Add(msg *istanbul.Message) error {
	ms.messagesMu.Lock()
	defer ms.messagesMu.Unlock()

	if !ms.valSet.ContainsByAddress(msg.Address) {
		return istanbul.ErrUnauthorizedAddress
	}
	ms.messages[msg.Address] = msg

	return nil
}

func (ms *messageSetImpl) GetAddressIndex(addr common.Address) (uint64, error) {
	ms.messagesMu.Lock()
	defer ms.messagesMu.Unlock()

	i, v := ms.valSet.GetByAddress(addr)
	if v == nil {
		return 0, istanbul.ErrUnauthorizedAddress
	}

	return uint64(i), nil
}

func (ms *messageSetImpl) Remove(address common.Address) {
	ms.messagesMu.Lock()
	defer ms.messagesMu.Unlock()

	delete(ms.messages, address)
}

func (ms *messageSetImpl) Values() (result []*istanbul.Message) {
	ms.messagesMu.Lock()
	defer ms.messagesMu.Unlock()

	for _, v := range ms.messages {
		result = append(result, v)
	}

	return result
}

func (ms *messageSetImpl) Size() int {
	ms.messagesMu.Lock()
	defer ms.messagesMu.Unlock()
	return len(ms.messages)
}

func (ms *messageSetImpl) Get(addr common.Address) *istanbul.Message {
	ms.messagesMu.Lock()
	defer ms.messagesMu.Unlock()
	return ms.messages[addr]
}

func (ms *messageSetImpl) String() string {
	ms.messagesMu.Lock()
	defer ms.messagesMu.Unlock()
	addresses := make([]string, 0, len(ms.messages))
	for _, v := range ms.messages {
		addresses = append(addresses, v.Address.String())
	}
	return fmt.Sprintf("[<%v> %v]", len(ms.messages), strings.Join(addresses, ", "))
}
