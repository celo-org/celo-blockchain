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

package vm

import (
	"math/big"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/consensus"
	"github.com/celo-org/celo-blockchain/core/state"
	"github.com/celo-org/celo-blockchain/core/types"
	"github.com/celo-org/celo-blockchain/params"
)

// Message represents a message sent to a contract.
type Message interface {
	From() common.Address
	To() *common.Address

	GasPrice() *big.Int
	Gas() uint64

	// FeeCurrency specifies the currency for gas and gateway fees.
	// nil correspond to Celo Gold (native currency).
	// All other values should correspond to ERC20 contract addresses extended to be compatible with gas payments.
	FeeCurrency() *common.Address
	GatewayFeeRecipient() *common.Address
	GatewayFee() *big.Int
	Value() *big.Int

	Nonce() uint64
	CheckNonce() bool
	Data() []byte

	// Whether this transaction omitted the 3 Celo-only fields (FeeCurrency & co.)
	EthCompatible() bool
}

func CheckEthCompatibility(msg Message) error {
	if msg.EthCompatible() && !(msg.FeeCurrency() == nil && msg.GatewayFeeRecipient() == nil && msg.GatewayFee().Sign() == 0) {
		return types.ErrEthCompatibleTransactionIsntCompatible
	}
	return nil
}

// ChainContext supports retrieving chain data and consensus parameters
// from the blockchain to be used during transaction processing.
type ChainContext interface {
	// Engine retrieves the blockchain's consensus engine.
	Engine() consensus.Engine

	// GetHeader returns the hash corresponding to the given hash and number.
	GetHeader(common.Hash, uint64) *types.Header

	// GetHeaderByNumber returns the hash corresponding number.
	// FIXME: Use of this function, as implemented, in the EVM context produces undefined behavior
	// in the pressence of forks. A new method needs to be created to retrieve a header by number
	// in the correct fork.
	GetHeaderByNumber(uint64) *types.Header

	// GetVMConfig returns the node's vm configuration
	GetVMConfig() *Config

	CurrentHeader() *types.Header

	State() (*state.StateDB, error)

	// Config returns the blockchain's chain configuration
	Config() *params.ChainConfig
}
