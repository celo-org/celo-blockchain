package blockchain_parameters

import (
	"testing"

	"github.com/celo-org/celo-blockchain/contracts/testutil"
	"github.com/celo-org/celo-blockchain/params"
)

func TestGetMinimumVersion(t *testing.T) {
	testutil.TestFailOnFailingRunner(t, getMinimumVersion)
}

func TestGetIntrinsicGasForAlternativeFeeCurrency(t *testing.T) {
	testutil.TestFailOnFailingRunner(t, getIntrinsicGasForAlternativeFeeCurrency)
}

func TestGetIntrinsicGasForAlternativeFeeCurrencyOrDefault(t *testing.T) {
	testutil.TestReturnsDefaultOnFailingRunner(t, params.IntrinsicGasForAlternativeFeeCurrency, GetIntrinsicGasForAlternativeFeeCurrencyOrDefault)
}

func TestGetBlockGasLimit(t *testing.T) {
	testutil.TestFailOnFailingRunner(t, getBlockGasLimit)
}
func TestGetBlockGasLimitOrDefault(t *testing.T) {
	testutil.TestReturnsDefaultOnFailingRunner(t, params.DefaultGasLimit, GetBlockGasLimitOrDefault)
}

func TestGetLookbackWindow(t *testing.T) {
	testutil.TestFailOnFailingRunner(t, GetLookbackWindow)
}
