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
		Nonce:    tx.Nonce,
		GasPrice: tx.GasPrice,
		Gas:      tx.Gas,
		To:       tx.To,
		Value:    tx.Value,
		Data:     tx.Data,
		V:        tx.V,
		R:        tx.R,
		S:        tx.S,

		// Celo specific fields
		FeeCurrency: tx.FeeCurrency,
	}
}

func (tx *LegacyTx) setTxFromEthCompatibleRlpList(rlplist ethCompatibleTxRlpList) {
	tx.Nonce = rlplist.Nonce
	tx.GasPrice = rlplist.GasPrice
	tx.Gas = rlplist.Gas
	tx.To = rlplist.To
	tx.Value = rlplist.Value
	tx.Data = rlplist.Data
	tx.V = rlplist.V
	tx.R = rlplist.R
	tx.S = rlplist.S
	tx.Hash = nil // txdata.Hash is calculated and saved inside tx.Hash()

	// Celo specific fields
	tx.FeeCurrency = nil
	tx.EthCompatible = true
}

func (tx *LegacyTx) setTxFromCeloRlpList(rlplist celoTxRlpList) {
	tx.Nonce = rlplist.Nonce
	tx.GasPrice = rlplist.GasPrice
	tx.Gas = rlplist.Gas
	tx.To = rlplist.To
	tx.Value = rlplist.Value
	tx.Data = rlplist.Data
	tx.V = rlplist.V
	tx.R = rlplist.R
	tx.S = rlplist.S
	tx.Hash = nil // txdata.Hash is calculated and saved inside tx.Hash()

	// Celo specific fields
	tx.FeeCurrency = rlplist.FeeCurrency
	tx.EthCompatible = false
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
