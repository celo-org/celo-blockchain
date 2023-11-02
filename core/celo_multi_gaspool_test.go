package core

import (
	"testing"

	"github.com/celo-org/celo-blockchain/common"
)

func TestMultiCurrencyGasPool(t *testing.T) {
	block_gas_limit := uint64(1_000)
	sub_gas_amount := 100

	cusd_token := common.HexToAddress("0x765DE816845861e75A25fCA122bb6898B8B1282a")
	ceur_token := common.HexToAddress("0xD8763CBa276a3738E6DE85b4b3bF5FDed6D6cA73")

	testCases := []struct {
		name                string
		whitelist           []common.Address
		feeCurrency         *FeeCurrency
		defaultLimit        float64
		limits              FeeCurrencyLimitMapping
		defaultPoolExpected bool
		expectedValue       uint64
	}{
		{
			name:                "Empty whitelist, empty mapping, CELO uses default pool",
			feeCurrency:         nil,
			whitelist:           []FeeCurrency{},
			defaultLimit:        0.9,
			limits:              map[FeeCurrency]float64{},
			defaultPoolExpected: true,
			expectedValue:       900, // block_gas_limit - sub_gas_amount
		},
		{
			name:        "Non-empty whitelist, non-empty mapping, CELO uses default pool",
			feeCurrency: nil,
			whitelist: []FeeCurrency{
				cusd_token,
			},
			defaultLimit: 0.9,
			limits: map[FeeCurrency]float64{
				cusd_token: 0.5,
			},
			defaultPoolExpected: true,
			expectedValue:       900, // block_gas_limit - sub_gas_amount
		},
		{
			name:                "Empty whitelist, empty mapping, non-whitelisted currency fallbacks to the default pool",
			feeCurrency:         &cusd_token,
			whitelist:           []FeeCurrency{},
			defaultLimit:        0.9,
			limits:              map[FeeCurrency]float64{},
			defaultPoolExpected: true,
			expectedValue:       900, // block_gas_limit - sub_gas_amount
		},
		{
			name:        "Non-empty whitelist, non-empty mapping, non-whitelisted currency uses default pool",
			feeCurrency: &ceur_token,
			whitelist: []FeeCurrency{
				cusd_token,
			},
			defaultLimit: 0.9,
			limits: map[FeeCurrency]float64{
				cusd_token: 0.5,
			},
			defaultPoolExpected: true,
			expectedValue:       900, // block_gas_limit - sub_gas_amount
		},
		{
			name:        "Non-empty whitelist, empty mapping, whitelisted currency uses default limit",
			feeCurrency: &cusd_token,
			whitelist: []FeeCurrency{
				cusd_token,
			},
			defaultLimit:        0.9,
			limits:              map[FeeCurrency]float64{},
			defaultPoolExpected: false,
			expectedValue:       800, // block_gas_limit * defaultLimit - sub_gas_amount
		},
		{
			name:        "Non-empty whitelist, non-empty mapping, configured whitelisted currency uses configured limits",
			feeCurrency: &cusd_token,
			whitelist: []FeeCurrency{
				cusd_token,
			},
			defaultLimit: 0.9,
			limits: map[FeeCurrency]float64{
				cusd_token: 0.5,
			},
			defaultPoolExpected: false,
			expectedValue:       400, // block_gas_limit * 0.5 - sub_gas_amount
		},
		{
			name:        "Non-empty whitelist, non-empty mapping, unconfigured whitelisted currency uses default limit",
			feeCurrency: &ceur_token,
			whitelist: []FeeCurrency{
				cusd_token,
				ceur_token,
			},
			defaultLimit: 0.9,
			limits: map[FeeCurrency]float64{
				cusd_token: 0.5,
			},
			defaultPoolExpected: false,
			expectedValue:       800, // block_gas_limit * 0.5 - sub_gas_amount
		},
	}

	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			mgp := NewMultiGasPool(
				block_gas_limit,
				c.whitelist,
				c.defaultLimit,
				c.limits,
			)

			pool := mgp.PoolFor(c.feeCurrency)
			pool.SubGas(uint64(sub_gas_amount))

			if c.defaultPoolExpected {
				result := mgp.defaultPool.Gas()
				if result != c.expectedValue {
					t.Error("Default pool expected", c.expectedValue, "got", result)
				}
			} else {
				pool := mgp.pools[*c.feeCurrency]
				result := pool.Gas()

				if result != c.expectedValue {
					t.Error(
						"Expected pool", c.feeCurrency, "value", c.expectedValue,
						"got", result,
					)
				}
			}
		})
	}
}
