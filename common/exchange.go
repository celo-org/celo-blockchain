package common

import "math/big"

type MaxExchangeRate struct {
	// TODO: define the specifics of the representation of the max exchange
	// rate for celo denominated txs.
}

// TotalCost returns the given value converted by the maxExchangeRate param.
func TotalCost(celoTotalCost *big.Int, maxExchangeRate MaxExchangeRate) *big.Int {
	// TODO: exchange convert the celo total cost using the exchange rate.
	return celoTotalCost
}
