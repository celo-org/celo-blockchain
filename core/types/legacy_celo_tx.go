// Copyright 2020 The go-ethereum Authors
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

package types

import (
	"math/big"

	"github.com/celo-org/celo-blockchain/common"
)

// LegacyCeloTx is the transaction data of regular Ethereum transactions.
type LegacyCeloTx struct {
	Nonce    uint64          // nonce of sender account
	GasPrice *big.Int        // wei per gas
	Gas      uint64          // gas limit
	To       *common.Address `rlp:"nil"` // nil means contract creation
	Value    *big.Int        // wei amount
	Data     []byte          // contract invocation input data
	V, R, S  *big.Int        // signature values

	FeeCurrency         *common.Address // nil means native currency
	GatewayFeeRecipient *common.Address // nil means no gateway fee is paid
	GatewayFee          *big.Int
}

// NewTransaction creates an unsigned legacy transaction.
// Deprecated: use NewTx instead.
func NewTransaction(nonce uint64, to common.Address, amount *big.Int, gasLimit uint64, gasPrice *big.Int, feeCurrency, gatewayFeeRecipient *common.Address, gatewayFee *big.Int, data []byte) *Transaction {
	return NewTx(&LegacyCeloTx{
		Nonce:               nonce,
		To:                  &to,
		Value:               amount,
		Gas:                 gasLimit,
		GasPrice:            gasPrice,
		Data:                data,
		FeeCurrency:         feeCurrency,
		GatewayFeeRecipient: gatewayFeeRecipient,
		GatewayFee:          gatewayFee,
	})
}

// NewContractCreation creates an unsigned legacy transaction.
// Deprecated: use NewTx instead.
func NewContractCreation(nonce uint64, amount *big.Int, gasLimit uint64, gasPrice *big.Int, feeCurrency, gatewayFeeRecipient *common.Address, gatewayFee *big.Int, data []byte) *Transaction {
	return NewTx(&LegacyCeloTx{
		Nonce:               nonce,
		Value:               amount,
		Gas:                 gasLimit,
		GasPrice:            gasPrice,
		Data:                data,
		FeeCurrency:         feeCurrency,
		GatewayFeeRecipient: gatewayFeeRecipient,
		GatewayFee:          gatewayFee,
	})
}

// copy creates a deep copy of the transaction data and initializes all fields.
func (tx *LegacyCeloTx) copy() TxData {
	cpy := &LegacyCeloTx{
		Nonce: tx.Nonce,
		To:    tx.To, // TODO: copy pointed-to address
		Data:  common.CopyBytes(tx.Data),
		Gas:   tx.Gas,
		// These are initialized below.
		Value:    new(big.Int),
		GasPrice: new(big.Int),
		V:        new(big.Int),
		R:        new(big.Int),
		S:        new(big.Int),
	}
	if tx.Value != nil {
		cpy.Value.Set(tx.Value)
	}
	if tx.GasPrice != nil {
		cpy.GasPrice.Set(tx.GasPrice)
	}
	if tx.V != nil {
		cpy.V.Set(tx.V)
	}
	if tx.R != nil {
		cpy.R.Set(tx.R)
	}
	if tx.S != nil {
		cpy.S.Set(tx.S)
	}
	return cpy
}

// accessors for innerTx.
func (tx *LegacyCeloTx) txType() byte                         { return LegacyCeloTxType }
func (tx *LegacyCeloTx) chainID() *big.Int                    { return deriveChainId(tx.V) }
func (tx *LegacyCeloTx) accessList() AccessList               { return nil }
func (tx *LegacyCeloTx) data() []byte                         { return tx.Data }
func (tx *LegacyCeloTx) gas() uint64                          { return tx.Gas }
func (tx *LegacyCeloTx) gasPrice() *big.Int                   { return tx.GasPrice }
func (tx *LegacyCeloTx) gasTipCap() *big.Int                  { return tx.GasPrice }
func (tx *LegacyCeloTx) gasFeeCap() *big.Int                  { return tx.GasPrice }
func (tx *LegacyCeloTx) value() *big.Int                      { return tx.Value }
func (tx *LegacyCeloTx) nonce() uint64                        { return tx.Nonce }
func (tx *LegacyCeloTx) to() *common.Address                  { return tx.To }
func (tx *LegacyCeloTx) feeCurrency() *common.Address         { return tx.FeeCurrency }
func (tx *LegacyCeloTx) gatewayFeeRecipient() *common.Address { return tx.GatewayFeeRecipient }
func (tx *LegacyCeloTx) gatewayFee() *big.Int                 { return tx.GatewayFee }
func (tx *LegacyCeloTx) ethCompatible() bool                  { return false }

func (tx *LegacyCeloTx) rawSignatureValues() (v, r, s *big.Int) {
	return tx.V, tx.R, tx.S
}

func (tx *LegacyCeloTx) setSignatureValues(chainID, v, r, s *big.Int) {
	tx.V, tx.R, tx.S = v, r, s
}
