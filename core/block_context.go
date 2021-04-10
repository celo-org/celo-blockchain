package core

import (
	"math/big"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/contract_comm/blockchain_parameters"
	"github.com/celo-org/celo-blockchain/contract_comm/currency"
	gpm "github.com/celo-org/celo-blockchain/contract_comm/gasprice_minimum"
	"github.com/celo-org/celo-blockchain/core/types"
	"github.com/celo-org/celo-blockchain/core/vm"
)

type BlockContext interface {
	GetGasPriceMinimum(feeCurrency *common.Address) *big.Int
	IsWhitelisted(feeCurrency *common.Address) bool
	GetIntrinsicGasForAlternativeFeeCurrency() uint64
}

type defaultBlockContext struct {
	goldGasPriceMinimum *big.Int
	gasPriceMinimum     map[common.Address]*big.Int

	whitelistedCurrencies     []common.Address
	noCurrencies              bool
	gasForAlternativeCurrency uint64
}

func NewBlockContext(header *types.Header, state vm.StateDB) BlockContext {
	gasForAlternativeCurrency := blockchain_parameters.GetIntrinsicGasForAlternativeFeeCurrency(header, state)

	var noCurrencies bool
	whitelistedCurrencies, err := currency.CurrencyWhitelist(header, state)
	if err != nil {
		whitelistedCurrencies = []common.Address{}
		noCurrencies = true
	}

	goldGasPriceMinimum, err := gpm.GetGasPriceMinimum(nil, header, state)
	_ = err // Ignore the error since gpm.GetGasPriceMinimum returns the Fallback value on error
	// TODO clean this up

	gasPriceMinimum := make(map[common.Address]*big.Int, len(whitelistedCurrencies))
	for _, currency := range whitelistedCurrencies {
		gpm, err := gpm.GetGasPriceMinimum(&currency, header, state)
		_ = err // Ignore the error since gpm.GetGasPriceMinimum returns the Fallback value on error
		// TODO clean this up
		gasPriceMinimum[currency] = gpm
	}

	return &defaultBlockContext{
		whitelistedCurrencies:     whitelistedCurrencies,
		noCurrencies:              noCurrencies,
		gasForAlternativeCurrency: gasForAlternativeCurrency,
		gasPriceMinimum:           gasPriceMinimum,
		goldGasPriceMinimum:       goldGasPriceMinimum,
	}
}

func (bc *defaultBlockContext) GetIntrinsicGasForAlternativeFeeCurrency() uint64 {
	return bc.gasForAlternativeCurrency
}

func (bc *defaultBlockContext) IsWhitelisted(feeCurrency *common.Address) bool {
	if bc.noCurrencies {
		return true
	}

	if feeCurrency == nil {
		return true
	}

	for _, addr := range bc.whitelistedCurrencies {
		if addr == *feeCurrency {
			return true
		}
	}
	return false
}

func (bc *defaultBlockContext) GetGasPriceMinimum(feeCurrency *common.Address) *big.Int {
	if feeCurrency == nil {
		return bc.goldGasPriceMinimum
	}

	gpm, ok := bc.gasPriceMinimum[*feeCurrency]
	if !ok {
		// TODO what to do here?
		return common.Big0
	}

	return gpm
}
