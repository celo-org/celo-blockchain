package core

import (
	"math/big"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/contracts/blockchain_parameters"
	"github.com/celo-org/celo-blockchain/contracts/currency"
	"github.com/celo-org/celo-blockchain/contracts/gasprice_minimum"
	"github.com/celo-org/celo-blockchain/core/vm"
)

// SysContractCallCtx represents a system contract call context for a given block (represented by vm.EVMRunner).
// It MUST sit on the header.Root of a block, which is parent of the block we intend to deal with.
type SysContractCallCtx struct {
	whitelistedCurrencies     map[common.Address]struct{}
	gasForAlternativeCurrency uint64
	// gasPriceMinimums stores values for whitelisted currencies keyed by their contract address
	// Note that native token(CELO) is keyed by common.ZeroAddress
	gasPriceMinimums GasPriceMinimums
}

// NewSysContractCallCtx creates the SysContractCallCtx object and makes the contract calls.
func NewSysContractCallCtx(vmRunner vm.EVMRunner) (sc *SysContractCallCtx) {
	sc = &SysContractCallCtx{
		whitelistedCurrencies: make(map[common.Address]struct{}),
		gasPriceMinimums:      make(map[common.Address]*big.Int),
	}
	// intrinsic gas
	sc.gasForAlternativeCurrency = blockchain_parameters.GetIntrinsicGasForAlternativeFeeCurrencyOrDefault(vmRunner)
	// whitelist
	whiteListedArr, err := currency.CurrencyWhitelist(vmRunner)
	if err != nil {
		whiteListedArr = []common.Address{}
	}
	for _, feeCurrency := range whiteListedArr {
		sc.whitelistedCurrencies[feeCurrency] = struct{}{}
	}
	// gas price minimum
	celoGPM, _ := gasprice_minimum.GetGasPriceMinimum(vmRunner, nil)
	sc.gasPriceMinimums[common.ZeroAddress] = celoGPM

	for feeCurrency := range sc.whitelistedCurrencies {
		gasPriceMinimum, _ := gasprice_minimum.GetGasPriceMinimum(vmRunner, &feeCurrency)
		sc.gasPriceMinimums[feeCurrency] = gasPriceMinimum
	}

	return
}

// GetIntrinsicGasForAlternativeFeeCurrency retrieves intrinsic gas for non-native fee currencies.
func (sc *SysContractCallCtx) GetIntrinsicGasForAlternativeFeeCurrency() uint64 {
	return sc.gasForAlternativeCurrency
}

// IsWhitelisted indicates if the fee currency is whitelisted, or it's native token(CELO).
func (sc *SysContractCallCtx) IsWhitelisted(feeCurrency *common.Address) bool {
	if feeCurrency == nil {
		return true
	}
	_, ok := sc.whitelistedCurrencies[*feeCurrency]
	return ok
}

// GetGasPriceMinimum retrieves gas price minimum for given fee currency address.
// Note that the CELO currency is keyed by the Zero address.
func (sc *SysContractCallCtx) GetGasPriceMinimum(feeCurrency *common.Address) *big.Int {
	return sc.gasPriceMinimums.GetGasPriceMinimum(feeCurrency)
}

// GetCurrentGasPriceMinimumMap returns the gas price minimum map for all whitelisted currencies.
// Note that the CELO currency is keyed by the Zero address.
func (sc *SysContractCallCtx) GetCurrentGasPriceMinimumMap() GasPriceMinimums {
	return sc.gasPriceMinimums
}

type GasPriceMinimums map[common.Address]*big.Int

func (gpm GasPriceMinimums) valOrDefault(key common.Address) *big.Int {
	val, ok := gpm[key]
	if !ok {
		return gasprice_minimum.FallbackGasPriceMinimum
	}
	return val
}

// GetNativeGPM retrieves the gas price minimum for the native currency.
func (gpm GasPriceMinimums) GetNativeGPM() *big.Int {
	return gpm.valOrDefault(common.ZeroAddress)
}

// GetGasPriceMinimum retrieves gas price minimum for given fee currency address, it returns gasprice_minimum.FallbackGasPriceMinimum when there is an error
func (gpm GasPriceMinimums) GetGasPriceMinimum(feeCurrency *common.Address) *big.Int {
	// feeCurrency for native token(CELO) is nil, so we bind common.ZeroAddress as key
	var key common.Address
	if feeCurrency == nil {
		key = common.ZeroAddress
	} else {
		key = *feeCurrency
	}

	return gpm.valOrDefault(key)
}
