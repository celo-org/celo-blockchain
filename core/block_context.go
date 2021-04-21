package core

import (
	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/contract_comm/blockchain_parameters"
	"github.com/celo-org/celo-blockchain/contract_comm/currency"
	"github.com/celo-org/celo-blockchain/core/types"
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
func NewBlockContext(header *types.Header, state vm.StateDB) BlockContext {
	gasForAlternativeCurrency := blockchain_parameters.GetIntrinsicGasForAlternativeFeeCurrency(header, state)

	whitelistedCurrenciesArr, err := currency.CurrencyWhitelist(header, state)
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

// GetGasPriceMinimum retrieves the gas price minimum for any currency
// Also indicates if the currency is not whitelisted
func (bc *BlockContext) IsWhitelisted(feeCurrency *common.Address) bool {
	if feeCurrency == nil {
		return true
	}

	_, ok := bc.whitelistedCurrencies[*feeCurrency]
	return ok
}
