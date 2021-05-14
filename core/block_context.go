package core

import (
	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/contracts/blockchain_parameters"
	"github.com/celo-org/celo-blockchain/contracts/currency"
	"github.com/celo-org/celo-blockchain/core/vm"
)

// BlockContext represents contextual information about the blockchain state
// for a given block
type BlockContext struct {
	whitelistedCurrencies     map[common.Address]struct{}
	gasForAlternativeCurrency uint64
}

// NewBlockContext creates a block context for a given block (represented by the
// header & state).
// state MUST be pointing to header's stateRoot
func NewBlockContext(vmRunner vm.EVMRunner) BlockContext {
	gasForAlternativeCurrency := blockchain_parameters.GetIntrinsicGasForAlternativeFeeCurrencyOrDefault(vmRunner)

	whitelistedCurrenciesArr, err := currency.CurrencyWhitelist(vmRunner)
	if err != nil {
		whitelistedCurrenciesArr = []common.Address{}
	}

	whitelistedCurrencies := make(map[common.Address]struct{}, len(whitelistedCurrenciesArr))
	for _, currency := range whitelistedCurrenciesArr {
		whitelistedCurrencies[currency] = struct{}{}
	}

	return BlockContext{
		whitelistedCurrencies:     whitelistedCurrencies,
		gasForAlternativeCurrency: gasForAlternativeCurrency,
	}
}

// GetIntrinsicGasForAlternativeFeeCurrency retrieves intrisic gas to be paid for
// any tx with a non native fee currency
func (bc *BlockContext) GetIntrinsicGasForAlternativeFeeCurrency() uint64 {
	return bc.gasForAlternativeCurrency
}

// IsWhitelisted indicates if the currency is whitelisted as a fee currency
func (bc *BlockContext) IsWhitelisted(feeCurrency *common.Address) bool {
	if feeCurrency == nil {
		return true
	}

	_, ok := bc.whitelistedCurrencies[*feeCurrency]
	return ok
}
