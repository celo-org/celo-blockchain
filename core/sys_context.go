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
	vmRunner vm.EVMRunner // vmRunner represents the parent block state on which the contract calls will be made

	whitelistedCurrencies     map[common.Address]struct{}
	gasForAlternativeCurrency uint64
	// gasPriceMinimums stores values for whitelisted currencies keyed by their contract address
	// Note that native token(CELO) is keyed by common.ZeroAddress
	gasPriceMinimums map[common.Address]*big.Int
}

func NewSysContractCallCtx(vmRunner vm.EVMRunner) *SysContractCallCtx {
	return &SysContractCallCtx{
		vmRunner: vmRunner,
	}
}

// GetIntrinsicGasForAlternativeFeeCurrency retrieves intrinsic gas for non-native fee currencies.
func (sc *SysContractCallCtx) GetIntrinsicGasForAlternativeFeeCurrency() uint64 {
	if sc.gasForAlternativeCurrency == 0 {
		sc.gasForAlternativeCurrency = blockchain_parameters.GetIntrinsicGasForAlternativeFeeCurrencyOrDefault(sc.vmRunner)
	}
	return sc.gasForAlternativeCurrency
}

// IsWhitelisted indicates if the fee currency is whitelisted.
func (sc *SysContractCallCtx) IsWhitelisted(feeCurrency *common.Address) bool {
	if feeCurrency == nil {
		return true
	}

	if sc.whitelistedCurrencies == nil {
		whiteListedArr, err := currency.CurrencyWhitelist(sc.vmRunner)
		if err != nil {
			whiteListedArr = []common.Address{}
		}
		whiteListedCurrencies := make(map[common.Address]struct{}, len(whiteListedArr))
		for _, feeCurrency := range whiteListedArr {
			whiteListedCurrencies[feeCurrency] = struct{}{}
		}
		sc.whitelistedCurrencies = whiteListedCurrencies
	}

	_, ok := sc.whitelistedCurrencies[*feeCurrency]
	return ok
}

// GetGasPriceMinimum retrieves gas price minimum for given fee currency address.
func (sc *SysContractCallCtx) GetGasPriceMinimum(feeCurrency *common.Address) *big.Int {
	if sc.IsWhitelisted(feeCurrency) {
		return nil
	}

	// feeCurrency for native token CELO is nil, so we bind common.ZeroAddress as key
	var key common.Address
	if feeCurrency == nil {
		key = common.ZeroAddress
	} else {
		key = *feeCurrency
	}

	gasPriceMinimum, ok := sc.gasPriceMinimums[key]
	if !ok {
		// Must succeed because it checked with sc.IsWhitelisted
		gasPriceMinimum, _ = gasprice_minimum.GetGasPriceMinimum(sc.vmRunner, feeCurrency)
		sc.gasPriceMinimums[key] = gasPriceMinimum
	}
	return gasPriceMinimum
}
