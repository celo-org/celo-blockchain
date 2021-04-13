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

// BlockContext represents contextual information about the blockchain state
// for a give block
type BlockContext interface {
	// GetGoldGasPriceMinimum retrieves the gas price minimum for the CELO token
	GetGoldGasPriceMinimum() *big.Int

	// GetGasPriceMinimum retrieves the gas price minimum for any currency
	// Also indicates if the currency is not whitelisted
	GetGasPriceMinimum(feeCurrency *common.Address) (gpm *big.Int, isWhitelisted bool)

	// GetIntrinsicGasForAlternativeFeeCurrency retrieves intrisic gas to be paid for
	// any tx with a non native fee currency
	GetIntrinsicGasForAlternativeFeeCurrency() uint64
}

// defaultBlockContext is the default implementation of BlockContext
type defaultBlockContext struct {
	goldGasPriceMinimum *big.Int
	nonGoldCurrencies   map[common.Address]*big.Int

	gasForAlternativeCurrency uint64
}

// NewBlockContext creates a block context for a given block (represented by the
// header & state).
// state MUST be pointing to header's stateRoot
func NewBlockContext(header *types.Header, state vm.StateDB) BlockContext {
	gasForAlternativeCurrency := blockchain_parameters.GetIntrinsicGasForAlternativeFeeCurrency(header, state)

	whitelistedCurrencies, err := currency.CurrencyWhitelist(header, state)
	if err != nil {
		whitelistedCurrencies = []common.Address{}
	}

	goldGasPriceMinimum, err := gpm.GetGasPriceMinimum(nil, header, state)
	_ = err // Ignore the error since gpm.GetGasPriceMinimum returns the Fallback value on error

	nonGoldCurrencies := make(map[common.Address]*big.Int, len(whitelistedCurrencies))
	for _, currency := range whitelistedCurrencies {
		gpm, err := gpm.GetGasPriceMinimum(&currency, header, state)
		if err != nil {
			// we ignore currencies from which we can't get GasPriceMinimum
			continue
		}
		nonGoldCurrencies[currency] = gpm
	}

	return &defaultBlockContext{
		nonGoldCurrencies:         nonGoldCurrencies,
		gasForAlternativeCurrency: gasForAlternativeCurrency,
		goldGasPriceMinimum:       goldGasPriceMinimum,
	}
}

// GetIntrinsicGasForAlternativeFeeCurrency retrieves intrisic gas to be paid for
// any tx with a non native fee currency
func (bc *defaultBlockContext) GetIntrinsicGasForAlternativeFeeCurrency() uint64 {
	return bc.gasForAlternativeCurrency
}

// GetGoldGasPriceMinimum retrieves the gas price minimum for the CELO token
func (bc *defaultBlockContext) GetGoldGasPriceMinimum() *big.Int {
	return bc.goldGasPriceMinimum
}

// GetGasPriceMinimum retrieves the gas price minimum for any currency
// Also indicates if the currency is not whitelisted
func (bc *defaultBlockContext) GetGasPriceMinimum(feeCurrency *common.Address) (gpm *big.Int, isWhitelisted bool) {
	if feeCurrency == nil {
		return bc.goldGasPriceMinimum, true
	}
	gpm, ok := bc.nonGoldCurrencies[*feeCurrency]
	return gpm, ok
}
