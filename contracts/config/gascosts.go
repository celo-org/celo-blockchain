package config

import "github.com/celo-org/celo-blockchain/contracts/internal/n"

const (
	// Default intrinsic gas cost of transactions paying for gas in alternative currencies.
	// Calculated to estimate 1 balance read, 1 debit, and 4 credit transactions.
	IntrinsicGasForAlternativeFeeCurrency uint64 = 50 * n.Thousand
)
