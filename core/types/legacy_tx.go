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
	"io"
	"math/big"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/rlp"
)

// LegacyTx is the transaction data of regular Ethereum transactions.
type LegacyTx struct {
	Nonce               uint64          // nonce of sender account
	GasPrice            *big.Int        // wei per gas
	Gas                 uint64          // gas limit
	FeeCurrency         *common.Address // nil means native currency
	GatewayFeeRecipient *common.Address // nil means no gateway fee is paid
	GatewayFee          *big.Int
	To                  *common.Address `rlp:"nil"` // nil means contract creation
	Value               *big.Int        // wei amount
	Data                []byte          // contract invocation input data
	V, R, S             *big.Int        // signature values

	// This is only used when marshaling to JSON.
	Hash *common.Hash `rlp:"-"`

	// Whether this is an ethereum-compatible transaction (i.e. with FeeCurrency, GatewayFeeRecipient and GatewayFee omitted)
	EthCompatible bool `rlp:"-"`
}

// ethCompatibleTxRlpList is used for RLP encoding/decoding of eth-compatible transactions.
// As such, it:
// (a) excludes the Celo-only fields,
// (b) doesn't need the Hash or EthCompatible fields, and
// (c) doesn't need the `json` or `gencodec` tags
type ethCompatibleTxRlpList struct {
	Nonce    uint64          // nonce of sender account
	GasPrice *big.Int        // wei per gas
	Gas      uint64          // gas limit
	To       *common.Address `rlp:"nil"` // nil means contract creation
	Value    *big.Int        // wei amount
	Data     []byte          // contract invocation input data
	V, R, S  *big.Int        // signature values
}

// celoTxRlpList is used for RLP encoding/decoding of celo transactions.
type celoTxRlpList struct {
	Nonce               uint64          // nonce of sender account
	GasPrice            *big.Int        // wei per gas
	Gas                 uint64          // gas limit
	FeeCurrency         *common.Address `rlp:"nil"` // nil means native currency
	GatewayFeeRecipient *common.Address `rlp:"nil"` // nil means no gateway fee is paid
	GatewayFee          *big.Int        `rlp:"nil"`
	To                  *common.Address `rlp:"nil"` // nil means contract creation
	Value               *big.Int        // wei amount
	Data                []byte          // contract invocation input data
	V, R, S             *big.Int        // signature values
}

func toEthCompatibleRlpList(tx LegacyTx) ethCompatibleTxRlpList {
	return ethCompatibleTxRlpList{
		Nonce:    tx.Nonce,
		GasPrice: tx.GasPrice,
		Gas:      tx.Gas,
		To:       tx.To,
		Value:    tx.Value,
		Data:     tx.Data,
		V:        tx.V,
		R:        tx.R,
		S:        tx.S,
	}
}

func toCeloRlpList(tx LegacyTx) celoTxRlpList {
	return celoTxRlpList{
		Nonce:               tx.Nonce,
		GasPrice:            tx.GasPrice,
		Gas:                 tx.Gas,
		FeeCurrency:         tx.FeeCurrency,
		GatewayFeeRecipient: tx.GatewayFeeRecipient,
		GatewayFee:          tx.GatewayFee,
		To:                  tx.To,
		Value:               tx.Value,
		Data:                tx.Data,
		V:                   tx.V,
		R:                   tx.R,
		S:                   tx.S,
	}
}

func (tx *LegacyTx) setTxFromEthCompatibleRlpList(rlplist ethCompatibleTxRlpList) {
	tx.Nonce = rlplist.Nonce
	tx.GasPrice = rlplist.GasPrice
	tx.Gas = rlplist.Gas
	tx.FeeCurrency = nil
	tx.GatewayFeeRecipient = nil
	tx.GatewayFee = big.NewInt(0)
	tx.To = rlplist.To
	tx.Value = rlplist.Value
	tx.Data = rlplist.Data
	tx.V = rlplist.V
	tx.R = rlplist.R
	tx.S = rlplist.S
	tx.Hash = nil // txdata.Hash is calculated and saved inside tx.Hash()
	tx.EthCompatible = true
}

func (tx *LegacyTx) setTxFromCeloRlpList(rlplist celoTxRlpList) {
	tx.Nonce = rlplist.Nonce
	tx.GasPrice = rlplist.GasPrice
	tx.Gas = rlplist.Gas
	tx.FeeCurrency = rlplist.FeeCurrency
	tx.GatewayFeeRecipient = rlplist.GatewayFeeRecipient
	tx.GatewayFee = rlplist.GatewayFee
	tx.To = rlplist.To
	tx.Value = rlplist.Value
	tx.Data = rlplist.Data
	tx.V = rlplist.V
	tx.R = rlplist.R
	tx.S = rlplist.S
	tx.Hash = nil // txdata.Hash is calculated and saved inside tx.Hash()
	tx.EthCompatible = false
}

// NewTransaction creates an unsigned legacy transaction.
// Deprecated: use NewTx instead.
func NewTransaction(nonce uint64, to common.Address, amount *big.Int, gasLimit uint64, gasPrice *big.Int, feeCurrency, gatewayFeeRecipient *common.Address, gatewayFee *big.Int, data []byte) *Transaction {
	return NewTx(&LegacyTx{
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

// NewTransaction creates an unsigned legacy transaction.
// Deprecated: use NewTx instead.
func NewTransactionEthCompatible(nonce uint64, to common.Address, amount *big.Int, gasLimit uint64, gasPrice *big.Int, data []byte) *Transaction {
	return NewTx(&LegacyTx{
		Nonce:         nonce,
		To:            &to,
		Value:         amount,
		Gas:           gasLimit,
		GasPrice:      gasPrice,
		Data:          data,
		EthCompatible: true,
	})
}

// NewContractCreation creates an unsigned legacy transaction.
// Deprecated: use NewTx instead.
func NewContractCreation(nonce uint64, amount *big.Int, gasLimit uint64, gasPrice *big.Int, feeCurrency, gatewayFeeRecipient *common.Address, gatewayFee *big.Int, data []byte) *Transaction {
	return NewTx(&LegacyTx{
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

// NewContractCreation creates an unsigned legacy transaction.
// Deprecated: use NewTx instead.
func NewContractCreationEthCompatible(nonce uint64, amount *big.Int, gasLimit uint64, gasPrice *big.Int, data []byte) *Transaction {
	return NewTx(&LegacyTx{
		Nonce:         nonce,
		Value:         amount,
		Gas:           gasLimit,
		GasPrice:      gasPrice,
		Data:          data,
		EthCompatible: true,
	})
}

// EncodeRLP implements rlp.Encoder
func (tx *LegacyTx) EncodeRLP(w io.Writer) error {
	if tx.EthCompatible {
		return rlp.Encode(w, toEthCompatibleRlpList(*tx))
	} else {
		return rlp.Encode(w, toCeloRlpList(*tx))
	}
}

// DecodeRLP implements rlp.Decoder
func (tx *LegacyTx) DecodeRLP(s *rlp.Stream) (err error) {
	_, size, _ := s.Kind()
	var raw rlp.RawValue
	err = s.Decode(&raw)
	if err != nil {
		return err
	}
	headerSize := len(raw) - int(size)
	numElems, err := rlp.CountValues(raw[headerSize:])
	if err != nil {
		return err
	}
	if numElems == ethCompatibleTxNumFields {
		rlpList := ethCompatibleTxRlpList{}
		err = rlp.DecodeBytes(raw, &rlpList)
		tx.setTxFromEthCompatibleRlpList(rlpList)
	} else {
		var rlpList celoTxRlpList
		err = rlp.DecodeBytes(raw, &rlpList)
		tx.setTxFromCeloRlpList(rlpList)
	}

	return err
}

// // MarshalJSON encodes the web3 RPC transaction format.
// func (tx *LegacyTx) MarshalJSON() ([]byte, error) {
// 	hash := tx.Hash()
// 	data := tx.data
// 	data.Hash = &hash
// 	return data.MarshalJSON()
// }

// // UnmarshalJSON decodes the web3 RPC transaction format.
// func (tx *LegacyTx) UnmarshalJSON(input []byte) error {
// 	var dec txdata
// 	if err := dec.UnmarshalJSON(input); err != nil {
// 		return err
// 	}
// 	withSignature := dec.V.Sign() != 0 || dec.R.Sign() != 0 || dec.S.Sign() != 0
// 	if withSignature {
// 		var V byte
// 		if isProtectedV(dec.V) {
// 			chainID := deriveChainId(dec.V).Uint64()
// 			V = byte(dec.V.Uint64() - 35 - 2*chainID)
// 		} else {
// 			V = byte(dec.V.Uint64() - 27)
// 		}
// 		if !crypto.ValidateSignatureValues(V, dec.R, dec.S, false) {
// 			return ErrInvalidSig
// 		}
// 	}
// 	*tx = Transaction{
// 		data: dec,
// 		time: time.Now(),
// 	}
// 	return nil
// }

// copy creates a deep copy of the transaction data and initializes all fields.
func (tx *LegacyTx) copy() TxData {
	cpy := &LegacyTx{
		Nonce:               tx.Nonce,
		To:                  tx.To, // TODO: copy pointed-to address
		Data:                common.CopyBytes(tx.Data),
		Gas:                 tx.Gas,
		FeeCurrency:         tx.FeeCurrency,         // TODO: copy pointed-to address
		GatewayFeeRecipient: tx.GatewayFeeRecipient, // TODO: copy pointed-to address
		EthCompatible:       tx.EthCompatible,
		// These are initialized below.
		GatewayFee: new(big.Int),
		Value:      new(big.Int),
		GasPrice:   new(big.Int),
		V:          new(big.Int),
		R:          new(big.Int),
		S:          new(big.Int),
	}
	if tx.GatewayFee != nil {
		cpy.GatewayFee.Set(tx.GatewayFee)
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
func (tx *LegacyTx) txType() byte                         { return LegacyTxType }
func (tx *LegacyTx) chainID() *big.Int                    { return deriveChainId(tx.V) }
func (tx *LegacyTx) accessList() AccessList               { return nil }
func (tx *LegacyTx) data() []byte                         { return tx.Data }
func (tx *LegacyTx) gas() uint64                          { return tx.Gas }
func (tx *LegacyTx) gasPrice() *big.Int                   { return tx.GasPrice }
func (tx *LegacyTx) gasTipCap() *big.Int                  { return tx.GasPrice }
func (tx *LegacyTx) gasFeeCap() *big.Int                  { return tx.GasPrice }
func (tx *LegacyTx) value() *big.Int                      { return tx.Value }
func (tx *LegacyTx) nonce() uint64                        { return tx.Nonce }
func (tx *LegacyTx) to() *common.Address                  { return tx.To }
func (tx *LegacyTx) feeCurrency() *common.Address         { return tx.FeeCurrency }
func (tx *LegacyTx) gatewayFeeRecipient() *common.Address { return tx.GatewayFeeRecipient }
func (tx *LegacyTx) gatewayFee() *big.Int                 { return tx.GatewayFee }
func (tx *LegacyTx) ethCompatible() bool                  { return tx.EthCompatible }

func (tx *LegacyTx) rawSignatureValues() (v, r, s *big.Int) {
	return tx.V, tx.R, tx.S
}

func (tx *LegacyTx) setSignatureValues(chainID, v, r, s *big.Int) {
	tx.V, tx.R, tx.S = v, r, s
}
