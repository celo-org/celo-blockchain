package core

import (
	"github.com/celo-org/celo-blockchain/common"
)

type FeeCurrency = common.Address

// MultiGasPool tracks the amount of gas available during execution
// of the transactions in a block per fee currency. The zero value is a pool
// with zero gas available.
type MultiGasPool struct {
	pools       map[FeeCurrency]*GasPool
	defaultPool *GasPool
}

type FeeCurrencyLimitMapping = map[FeeCurrency]float64

// NewMultiGasPool creates a multi-fee currency gas pool and a default fallback
// pool for any unconfigured currencies and CELO
func NewMultiGasPool(
	block_gas_limit uint64,
	whitelist []FeeCurrency,
	defaultLimit float64,
	limitsMapping FeeCurrencyLimitMapping,
) MultiGasPool {
	pools := make(map[FeeCurrency]*GasPool, len(whitelist))

	for i := range whitelist {
		currency := whitelist[i]
		fraction, ok := limitsMapping[currency]
		if !ok {
			fraction = defaultLimit
		}

		pools[currency] = new(GasPool).AddGas(
			uint64(float64(block_gas_limit) * fraction),
		)
	}

	// A special case for CELO which doesn't have a limit
	celoPool := new(GasPool).AddGas(block_gas_limit)

	return MultiGasPool{
		pools:       pools,
		defaultPool: celoPool,
	}
}

// PoolFor returns a configured pool for the given fee currency or the default
// one otherwise
func (mgp MultiGasPool) PoolFor(feeCurrency *FeeCurrency) *GasPool {
	if feeCurrency == nil || mgp.pools[*feeCurrency] == nil {
		return mgp.defaultPool
	}

	return mgp.pools[*feeCurrency]
}
