package ethapi

import (
	"context"
	"math/big"

	"github.com/celo-org/celo-blockchain/contract_comm/currency"
	"github.com/celo-org/celo-blockchain/params"
	"github.com/celo-org/celo-blockchain/rpc"
)

// NewCurrencyManager creates and returns a currencyManager pointing to the latest block
// from the underlying chain from the Backend.
func NewCurrencyManager(ctx context.Context, b Backend) (*currency.CurrencyManager, error) {
	stateDb, header, err := b.StateAndHeaderByNumber(ctx, rpc.LatestBlockNumber)
	if err != nil {
		return nil, err
	}
	return currency.NewManager(
		header,
		stateDb), nil
}

// GetWei converts a celo float to a big.Int Wei representation
func GetWei(celo float64) *big.Int {
	floatWei := new(big.Float).Mul(big.NewFloat(params.Ether), big.NewFloat(celo))
	wei, _ := floatWei.Int(nil)
	return wei
}
