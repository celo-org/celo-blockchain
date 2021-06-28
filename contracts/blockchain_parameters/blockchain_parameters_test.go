package blockchain_parameters

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/contracts"
	"github.com/ethereum/go-ethereum/contracts/testutil"
	"github.com/ethereum/go-ethereum/params"
	. "github.com/onsi/gomega"
)

func TestGetMinimumVersion(t *testing.T) {
	testutil.TestFailOnFailingRunner(t, getMinimumVersion)
	testutil.TestFailsWhenContractNotDeployed(t, contracts.ErrSmartContractNotDeployed, getMinimumVersion)

	t.Run("should return minimum version", func(t *testing.T) {
		g := NewGomegaWithT(t)

		runner := testutil.NewSingleMethodRunner(
			params.BlockchainParametersRegistryId,
			"getMinimumClientVersion",
			func() (*big.Int, *big.Int, *big.Int) {
				return big.NewInt(5), big.NewInt(4), big.NewInt(3)
			},
		)

		version, err := getMinimumVersion(runner)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(version).To(Equal(&params.VersionInfo{Major: 5, Minor: 4, Patch: 3}))
	})
}

func TestGetIntrinsicGasForAlternativeFeeCurrency(t *testing.T) {
	testutil.TestFailOnFailingRunner(t, getIntrinsicGasForAlternativeFeeCurrency)
	testutil.TestFailsWhenContractNotDeployed(t, contracts.ErrSmartContractNotDeployed, getIntrinsicGasForAlternativeFeeCurrency)

	t.Run("should return gas for alternative currency", func(t *testing.T) {
		g := NewGomegaWithT(t)

		runner := testutil.NewSingleMethodRunner(
			params.BlockchainParametersRegistryId,
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
	testutil.TestReturnsDefaultOnFailingRunner(t, params.IntrinsicGasForAlternativeFeeCurrency, GetIntrinsicGasForAlternativeFeeCurrencyOrDefault)
	t.Run("should return gas for alternative currency", func(t *testing.T) {
		g := NewGomegaWithT(t)

		runner := testutil.NewSingleMethodRunner(
			params.BlockchainParametersRegistryId,
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
	testutil.TestFailOnFailingRunner(t, getBlockGasLimit)
	testutil.TestFailsWhenContractNotDeployed(t, contracts.ErrSmartContractNotDeployed, getBlockGasLimit)
	t.Run("should return block gas limit", func(t *testing.T) {
		g := NewGomegaWithT(t)

		runner := testutil.NewSingleMethodRunner(
			params.BlockchainParametersRegistryId,
			"blockGasLimit",
			func() *big.Int {
				return big.NewInt(50000)
			},
		)

		gas, err := getBlockGasLimit(runner)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(gas).To(Equal(uint64(50000)))
	})
}
func TestGetBlockGasLimitOrDefault(t *testing.T) {
	testutil.TestReturnsDefaultOnFailingRunner(t, params.DefaultGasLimit, GetBlockGasLimitOrDefault)
	t.Run("should return block gas limit", func(t *testing.T) {
		g := NewGomegaWithT(t)

		runner := testutil.NewSingleMethodRunner(
			params.BlockchainParametersRegistryId,
			"blockGasLimit",
			func() *big.Int {
				return big.NewInt(50000)
			},
		)

		gas := GetBlockGasLimitOrDefault(runner)
		g.Expect(gas).To(Equal(uint64(50000)))
	})
}

func TestGetLookbackWindow(t *testing.T) {
	testutil.TestFailOnFailingRunner(t, GetLookbackWindow)
	testutil.TestFailsWhenContractNotDeployed(t, contracts.ErrSmartContractNotDeployed, GetLookbackWindow)
	t.Run("should return lookback window", func(t *testing.T) {
		g := NewGomegaWithT(t)

		runner := testutil.NewSingleMethodRunner(
			params.BlockchainParametersRegistryId,
			"getUptimeLookbackWindow",
			func() *big.Int {
				return big.NewInt(15)
			},
		)

		lookbackWindow, err := GetLookbackWindow(runner)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(lookbackWindow).To(Equal(uint64(15)))
	})
}
