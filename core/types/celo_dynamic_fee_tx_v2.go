package types

import (
	"math/big"

	"github.com/celo-org/celo-blockchain/common"
)

type CeloDynamicFeeTxV2 struct {
	ChainID    *big.Int
	Nonce      uint64
	GasTipCap  *big.Int
	GasFeeCap  *big.Int
	Gas        uint64
	To         *common.Address `rlp:"nil"` // nil means contract creation
	Value      *big.Int
	Data       []byte
	AccessList AccessList

	FeeCurrency *common.Address `rlp:"nil"` // nil means native currency

	// Signature values
	V *big.Int `json:"v" gencodec:"required"`
	R *big.Int `json:"r" gencodec:"required"`
	S *big.Int `json:"s" gencodec:"required"`
}

// copy creates a deep copy of the transaction data and initializes all fields.
func (tx *CeloDynamicFeeTxV2) copy() TxData {
	cpy := &CeloDynamicFeeTxV2{
		Nonce:       tx.Nonce,
		To:          tx.To, // TODO: copy pointed-to address
		Data:        common.CopyBytes(tx.Data),
		Gas:         tx.Gas,
		FeeCurrency: tx.FeeCurrency,
		// These are copied below.
		AccessList: make(AccessList, len(tx.AccessList)),
		Value:      new(big.Int),
		ChainID:    new(big.Int),
		GasTipCap:  new(big.Int),
		GasFeeCap:  new(big.Int),
		V:          new(big.Int),
		R:          new(big.Int),
		S:          new(big.Int),
	}
	copy(cpy.AccessList, tx.AccessList)
	if tx.Value != nil {
		cpy.Value.Set(tx.Value)
	}
	if tx.ChainID != nil {
		cpy.ChainID.Set(tx.ChainID)
	}
	if tx.GasTipCap != nil {
		cpy.GasTipCap.Set(tx.GasTipCap)
	}
	if tx.GasFeeCap != nil {
		cpy.GasFeeCap.Set(tx.GasFeeCap)
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
func (tx *CeloDynamicFeeTxV2) txType() byte           { return CeloDynamicFeeTxV2Type }
func (tx *CeloDynamicFeeTxV2) chainID() *big.Int      { return tx.ChainID }
func (tx *CeloDynamicFeeTxV2) protected() bool        { return true }
func (tx *CeloDynamicFeeTxV2) accessList() AccessList { return tx.AccessList }
func (tx *CeloDynamicFeeTxV2) data() []byte           { return tx.Data }
func (tx *CeloDynamicFeeTxV2) gas() uint64            { return tx.Gas }
func (tx *CeloDynamicFeeTxV2) gasFeeCap() *big.Int    { return tx.GasFeeCap }
func (tx *CeloDynamicFeeTxV2) gasTipCap() *big.Int    { return tx.GasTipCap }
func (tx *CeloDynamicFeeTxV2) gasPrice() *big.Int     { return tx.GasFeeCap }
func (tx *CeloDynamicFeeTxV2) value() *big.Int        { return tx.Value }
func (tx *CeloDynamicFeeTxV2) nonce() uint64          { return tx.Nonce }
func (tx *CeloDynamicFeeTxV2) to() *common.Address    { return tx.To }

func (tx *CeloDynamicFeeTxV2) rawSignatureValues() (v, r, s *big.Int) {
	return tx.V, tx.R, tx.S
}

func (tx *CeloDynamicFeeTxV2) setSignatureValues(chainID, v, r, s *big.Int) {
	tx.ChainID, tx.V, tx.R, tx.S = chainID, v, r, s
}

func (tx *CeloDynamicFeeTxV2) feeCurrency() *common.Address         { return tx.FeeCurrency }
func (tx *CeloDynamicFeeTxV2) gatewayFeeRecipient() *common.Address { return nil }
func (tx *CeloDynamicFeeTxV2) gatewayFee() *big.Int                 { return nil }
func (tx *CeloDynamicFeeTxV2) ethCompatible() bool                  { return false }
