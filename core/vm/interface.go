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
	"github.com/celo-org/celo-blockchain/core/types"
	"github.com/celo-org/celo-blockchain/params"
)

// StateDB is an EVM database for full state querying.
type StateDB interface {
	CreateAccount(common.Address)

	SubBalance(common.Address, *big.Int)
	AddBalance(common.Address, *big.Int)
	GetBalance(common.Address) *big.Int

	GetNonce(common.Address) uint64
	SetNonce(common.Address, uint64)

	GetCodeHash(common.Address) common.Hash
	GetCode(common.Address) []byte
	SetCode(common.Address, []byte)
	GetCodeSize(common.Address) int

	AddRefund(uint64)
	SubRefund(uint64)
	GetRefund() uint64

	GetCommittedState(common.Address, common.Hash) common.Hash
	GetState(common.Address, common.Hash) common.Hash
	SetState(common.Address, common.Hash, common.Hash)

	Suicide(common.Address) bool
	HasSuicided(common.Address) bool

	// Exist reports whether the given account exists in state.
	// Notably this should also return true for suicided accounts.
	Exist(common.Address) bool
	// Empty returns whether the given account is empty. Empty
	// is defined according to EIP161 (balance = nonce = code = 0).
	Empty(common.Address) bool

	RevertToSnapshot(int)
	Snapshot() int

	AddLog(*types.Log)
	AddPreimage(common.Hash, []byte)

	ForEachStorage(common.Address, func(common.Hash, common.Hash) bool) error

	Finalise(bool)
}

// CallContext provides a basic interface for the EVM calling conventions. The EVM
// depends on this context being implemented for doing subcalls and initialising new EVM contracts.
type CallContext interface {
	// Call another contract
	Call(env *EVM, me ContractRef, addr common.Address, data []byte, gas, value *big.Int) ([]byte, error)
	// Take another's contract code and execute within our own context
	CallCode(env *EVM, me ContractRef, addr common.Address, data []byte, gas, value *big.Int) ([]byte, error)
	// Same as CallCode except sender and value is propagated from parent to child scope
	DelegateCall(env *EVM, me ContractRef, addr common.Address, data []byte, gas *big.Int) ([]byte, error)
	// Create a new contract
	Create(env *EVM, me ContractRef, data []byte, gas, value *big.Int) ([]byte, common.Address, error)
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

	// Config returns the blockchain's chain configuration
	Config() *params.ChainConfig
}

// EVMRunner provides a simplified API to run EVM calls
// EVM's sender, gasPrice, txFeeRecipient and state are set by the runner on each call
// This object can be re-used many times in contrast to the EVM's single use behaviour.
type EVMRunner interface {
	// Execute performs a potentially write operation over the runner's state
	// It can be seen as a message (input,value) from sender to recipient that returns `ret`
	Execute(recipient common.Address, input []byte, gas uint64, value *big.Int) (ret []byte, leftOverGas uint64, err error)

	// Query performs a read operation over the runner's state
	// It can be seen as a message (input,value) from sender to recipient that returns `ret`
	Query(recipient common.Address, input []byte, gas uint64) (ret []byte, leftOverGas uint64, err error)

	// StopGasMetering backward compatibility method to stop gas metering
	// Deprecated. DO NOT USE
	StopGasMetering()

	// StartGasMetering backward compatibility method to start gas metering
	// Deprecated. DO NOT USE
	StartGasMetering()

	// GetStateDB retrieves current stateDB within the runner
	GetStateDB() StateDB
}
