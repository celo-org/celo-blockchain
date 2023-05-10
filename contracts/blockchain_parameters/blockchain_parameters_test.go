package blockchain_parameters

import (
	"math/big"
	"testing"

	"github.com/celo-org/celo-blockchain/contracts"

	"github.com/celo-org/celo-blockchain/contracts/config"
	"github.com/celo-org/celo-blockchain/contracts/testutil"
	"github.com/celo-org/celo-blockchain/params"
	. "github.com/onsi/gomega"
)

func TestGetIntrinsicGasForAlternativeFeeCurrency(t *testing.T) {
	testutil.TestFailOnFailingRunner(t, getIntrinsicGasForAlternativeFeeCurrency)
	testutil.TestFailsWhenContractNotDeployed(t, contracts.ErrSmartContractNotDeployed, getIntrinsicGasForAlternativeFeeCurrency)

	t.Run("should return gas for alternative currency", func(t *testing.T) {
		g := NewGomegaWithT(t)

		runner := testutil.NewSingleMethodRunner(
			config.BlockchainParametersRegistryId,
			"intrinsicGasForAlternativeFeeCurrency",
			func() *big.Int {
				return big.NewInt(50000)
			},
		)

		gas, err := getIntrinsicGasForAlternativeFeeCurrency(runner)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(gas).To(Equal(uint64(50000)))
	})
}

func TestGetIntrinsicGasForAlternativeFeeCurrencyOrDefault(t *testing.T) {
	testutil.TestReturnsDefaultOnFailingRunner(t, DefaultIntrinsicGasForAlternativeFeeCurrency, GetIntrinsicGasForAlternativeFeeCurrencyOrDefault)
	t.Run("should return gas for alternative currency", func(t *testing.T) {
		g := NewGomegaWithT(t)

		runner := testutil.NewSingleMethodRunner(
			config.BlockchainParametersRegistryId,
			"intrinsicGasForAlternativeFeeCurrency",
			func() *big.Int {
				return big.NewInt(50000)
			},
		)

		gas := GetIntrinsicGasForAlternativeFeeCurrencyOrDefault(runner)
		g.Expect(gas).To(Equal(uint64(50000)))
	})
}

func TestGetBlockGasLimit(t *testing.T) {
	testutil.TestFailOnFailingRunner(t, GetBlockGasLimit)
	testutil.TestFailsWhenContractNotDeployed(t, contracts.ErrSmartContractNotDeployed, GetBlockGasLimit)
	t.Run("should return block gas limit", func(t *testing.T) {
		g := NewGomegaWithT(t)

		runner := testutil.NewSingleMethodRunner(
			config.BlockchainParametersRegistryId,
			"blockGasLimit",
			func() *big.Int {
				return big.NewInt(50000)
			},
		)

		gas, err := GetBlockGasLimit(runner)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(gas).To(Equal(uint64(50000)))
	})
}
func TestGetBlockGasLimitOrDefault(t *testing.T) {
	testutil.TestReturnsDefaultOnFailingRunner(t, params.DefaultGasLimit, GetBlockGasLimitOrDefault)
	t.Run("should return block gas limit", func(t *testing.T) {
		g := NewGomegaWithT(t)

		runner := testutil.NewSingleMethodRunner(
			config.BlockchainParametersRegistryId,
			"blockGasLimit",
			func() *big.Int {
				return big.NewInt(50000)
			},
		)

		gas := GetBlockGasLimitOrDefault(runner)
		g.Expect(gas).To(Equal(uint64(50000)))
	})
}
