// Copyright 2014 The go-ethereum Authors
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

import "errors"

var (
	// ErrKnownBlock is returned when a block to import is already known locally.
	ErrKnownBlock = errors.New("block already known")

	// ErrBlacklistedHash is returned if a block to import is on the blacklist.
	ErrBlacklistedHash = errors.New("blacklisted hash")

	// ErrNoGenesis is returned when there is no Genesis Block.
	ErrNoGenesis = errors.New("genesis not found in chain")

	// ErrNotHeadBlock is returned when block to insert is not the next head
	// of the canonical chain
	ErrNotHeadBlock = errors.New("block is not next head block")
)

// List of evm-call-message pre-checking errors. All state transition messages will
// be pre-checked before execution. If any invalidation detected, the corresponding
// error should be returned which is defined here.
//
// - If the pre-checking happens in the miner, then the transaction won't be packed.
// - If the pre-checking happens in the block processing procedure, then a "BAD BLOCk"
// error should be emitted.
var (
	// ErrNonceTooLow is returned if the nonce of a transaction is lower than the
	// one present in the local chain.
	ErrNonceTooLow = errors.New("nonce too low")

	// ErrNonceTooHigh is returned if the nonce of a transaction is higher than the
	// next one expected based on the local chain.
	ErrNonceTooHigh = errors.New("nonce too high")

	// ErrGasLimitReached is returned by the gas pool if the amount of gas required
	// by a transaction is higher than what's left in the block.
	ErrGasLimitReached = errors.New("gas limit reached")

	// ErrInsufficientFundsForTransfer is returned if the transaction sender doesn't
	// have enough funds for transfer(topmost call only).
	// Note that the check for this is done after buying the gas.
	ErrInsufficientFundsForTransfer = errors.New("insufficient funds for transfer (after fees)")

	// ErrInsufficientFunds is returned if the total cost of executing a transaction
	// is higher than the balance of the user's account.
	ErrInsufficientFunds = errors.New("insufficient funds for gas * price + value + gatewayFee")

	// ErrGasPriceDoesNotExceedMinimum is returned if the gas price specified doesn't meet the
	// minimum specified by the GasPriceMinimum contract.
	ErrGasPriceDoesNotExceedMinimum = errors.New("gasprice is less than gas price minimum")

	// ErrInsufficientFundsForFees is returned if the account does have enough funds (in the
	// fee currency used for the transaction) to pay for the gas.
	ErrInsufficientFundsForFees = errors.New("insufficient funds to pay for fees")

	// ErrNonWhitelistedFeeCurrency is returned if the currency specified to use for the fees
	// isn't one of the currencies whitelisted for that purpose.
	ErrNonWhitelistedFeeCurrency = errors.New("non-whitelisted fee currency address")

	// ErrGasUintOverflow is returned when calculating gas usage.
	ErrGasUintOverflow = errors.New("gas uint64 overflow")

	// ErrIntrinsicGas is returned if the transaction is specified to use less gas
	// than required to start the invocation.
	ErrIntrinsicGas = errors.New("intrinsic gas too low")

	// ErrEthCompatibleTransactionsNotSupported is returned if the transaction omits the 3 Celo-only
	// fields (FeeCurrency & co.) but support for this kind of transaction is not enabled.
	ErrEthCompatibleTransactionsNotSupported = errors.New("support for eth-compatible transactions is not enabled")

	// ErrUnprotectedTransaction is returned if replay protection is required (post-Donut) but the transaction doesn't
	// use it.
	ErrUnprotectedTransaction = errors.New("replay protection is required")
)
