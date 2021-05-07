package ethapi

import (
	"context"

	"github.com/celo-org/celo-blockchain/contract_comm/currency"
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
