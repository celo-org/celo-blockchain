package miner

import (
	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/params"
)

// cStables addresses on mainnet
var cUSD_TOKEN = common.HexToAddress("0x765DE816845861e75A25fCA122bb6898B8B1282a")
var cEUR_TOKEN = common.HexToAddress("0xD8763CBa276a3738E6DE85b4b3bF5FDed6D6cA73")
var cREAL_TOKEN = common.HexToAddress("0xe8537a3d056DA446677B9E9d6c5dB704EaAb4787")

// default limits configuration
var DefaultFeeCurrencyLimits = map[uint64]map[common.Address]float64{
	params.MainnetNetworkId: {
		cUSD_TOKEN:  0.9,
		cEUR_TOKEN:  0.5,
		cREAL_TOKEN: 0.5,
	},
}
