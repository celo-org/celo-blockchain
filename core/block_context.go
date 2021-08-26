package core

import (
	"math/big"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/contracts/blockchain_parameters"
	"github.com/celo-org/celo-blockchain/contracts/currency"
	gpm "github.com/celo-org/celo-blockchain/contracts/gasprice_minimum"
	"github.com/celo-org/celo-blockchain/core/vm"
)

// BlockContext represents contextual information about the blockchain state
// for a given block
type BlockContext struct {
	whitelistedCurrencies     map[common.Address]struct{}
	gasForAlternativeCurrency uint64

	// gasPriceMinimums stores values for whitelist currency under their currency addresses
	// Note that native currency(CELO) is under common.ZeroAddress
	gasPriceMinimums map[common.Address]*big.Int
}

// NewBlockContext creates a block context for a given block (represented by the
// header & state).
// state MUST be pointing to header's stateRoot
func NewBlockContext(vmRunner vm.EVMRunner) *BlockContext {
	// gasForAlternativeCurrency
	gasForAlternativeCurrency := blockchain_parameters.GetIntrinsicGasForAlternativeFeeCurrencyOrDefault(vmRunner)

	// whitelistedCurrencies
	whitelistedCurrenciesArr, err := currency.CurrencyWhitelist(vmRunner)
	if err != nil {
		whitelistedCurrenciesArr = []common.Address{}
	}

	whitelistedCurrencies := make(map[common.Address]struct{}, len(whitelistedCurrenciesArr))
	for _, currency := range whitelistedCurrenciesArr {
		whitelistedCurrencies[currency] = struct{}{}
	}

	// gasPriceMinimums
	gasPriceMinimums := make(map[common.Address]*big.Int)

	nativeTokenGpm, _ := gpm.GetGasPriceMinimum(vmRunner, nil)
	gasPriceMinimums[common.ZeroAddress] = nativeTokenGpm

	for currency := range whitelistedCurrencies {
		gasPriceMinimums[currency], _ = gpm.GetGasPriceMinimum(vmRunner, &currency)
	}

	return &BlockContext{
		whitelistedCurrencies:     whitelistedCurrencies,
		gasForAlternativeCurrency: gasForAlternativeCurrency,
		gasPriceMinimums:          gasPriceMinimums,
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

// GetGasPriceMinimum gets the gasPriceMinimum given the address
// - if the input address is nil or common.ZeroAddress, return value for native currency(CELO)
// - otherwise it returns value based on address
func (bc *BlockContext) GetGasPriceMinimum(address *common.Address) (gasPriceMinimum *big.Int) {
	if address == nil || *address == common.ZeroAddress {
		return bc.gasPriceMinimums[common.ZeroAddress]
	}
	return bc.gasPriceMinimums[*address]
}

// WithZeroGasPriceMinimum returns a new BlockContext with GasPriceMinimums set to zero
// One use case is core.ApplyMessageWithoutGasPriceMinimum
func (bc *BlockContext) WithZeroGasPriceMinimum() *BlockContext{
	cpy := *bc
	for address := range cpy.gasPriceMinimums {
		bc.gasPriceMinimums[address] = common.Big0
	}

	return &cpy
}
