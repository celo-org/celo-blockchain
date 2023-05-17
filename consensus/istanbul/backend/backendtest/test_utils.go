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

package backendtest

import (
	"crypto/ecdsa"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/consensus"
	"github.com/celo-org/celo-blockchain/consensus/istanbul"
	"github.com/celo-org/celo-blockchain/core"
	"github.com/celo-org/celo-blockchain/p2p"
	"github.com/celo-org/celo-blockchain/rlp"
)

type TestBackendInterface interface {
	HandleMsg(addr common.Address, msg p2p.Msg, peer consensus.Peer) (bool, error)

	Address() common.Address
}

type TestBackendFactory interface {
	New(genesisCfg *core.Genesis, privateKey *ecdsa.PrivateKey) (TestBackendInterface, *istanbul.Config)

	GetGenesisAndKeys(numValidators int, isFullChain bool) (*core.Genesis, []*ecdsa.PrivateKey)
}

var testBackendFactoryImpl TestBackendFactory

func InitTestBackendFactory(impl TestBackendFactory) {
	testBackendFactoryImpl = impl
}

func NewTestBackend(genesisCfg *core.Genesis, privateKey *ecdsa.PrivateKey) (TestBackendInterface, *istanbul.Config) {
	return testBackendFactoryImpl.New(genesisCfg, privateKey)
}

func GetGenesisAndKeys(numValidators int, isFullChain bool) (*core.Genesis, []*ecdsa.PrivateKey) {
	return testBackendFactoryImpl.GetGenesisAndKeys(numValidators, isFullChain)
}

func CreateP2PMsg(code uint64, payload []byte) (p2p.Msg, error) {
	size, r, err := rlp.EncodeToReader(payload)
	if err != nil {
		return p2p.Msg{}, err
	}

	return p2p.Msg{Code: code, Size: uint32(size), Payload: r}, nil
}
