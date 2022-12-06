package core

import (
	"math/big"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/contracts/blockchain_parameters"
	"github.com/celo-org/celo-blockchain/contracts/currency"
	"github.com/celo-org/celo-blockchain/contracts/gasprice_minimum"
	"github.com/celo-org/celo-blockchain/core/state"
	"github.com/celo-org/celo-blockchain/core/types"
	"github.com/celo-org/celo-blockchain/core/vm"
)

// SysContractCallCtx acts as a cache holding information obtained through
// system contract calls to be used during block processing.
// Note: This struct should be a read only one to be safe for concurrent use
type SysContractCallCtx struct {
	whitelistedCurrencies map[common.Address]struct{}
	// The gas required for a non celo (cUSD, cEUR, ...) transfer.
	nonCeloCurrencyIntrinsicGas uint64
	// gasPriceMinimums stores values for whitelisted currencies keyed by their contract address
	// Note that native token(CELO) is keyed by common.ZeroAddress
	gasPriceMinimums GasPriceMinimums
}

// runnerFactory exists to allow multiple different implementations to be
// used as input to NewSysContractCallCtx.
type runnerFactory interface {
	NewEVMRunner(*types.Header, vm.StateDB) vm.EVMRunner
}

// NewSysContractCallCtx returns a SysContractCallCtx filled with data obtained
// by calling the relevant system contracts.  This is a read only operation, no
// state changing operations should be performed here. The provided header and
// state should be for the parent of the block to be processed, in normal
// operation that will be the head block.
//
// Since geth introduced the access list, even read only contract calls modify
// the state (by adding to the access list) as such the provided state is
// copied to ensure that the state provided by the caller is not modified by
// this operation.
func NewSysContractCallCtx(header *types.Header, state *state.StateDB, factory runnerFactory) (sc *SysContractCallCtx) {
	vmRunner := factory.NewEVMRunner(header, state.Copy())
	sc = &SysContractCallCtx{
		whitelistedCurrencies: make(map[common.Address]struct{}),
		gasPriceMinimums:      make(map[common.Address]*big.Int),
	}
	// intrinsic gas
	sc.nonCeloCurrencyIntrinsicGas = blockchain_parameters.GetIntrinsicGasForAlternativeFeeCurrencyOrDefault(vmRunner)
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
	return sc
}

// GetIntrinsicGasForAlternativeFeeCurrency retrieves intrinsic gas for non-native fee currencies.
func (sc *SysContractCallCtx) GetIntrinsicGasForAlternativeFeeCurrency() uint64 {
	return sc.nonCeloCurrencyIntrinsicGas
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
