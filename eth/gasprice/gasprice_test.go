package gasprice

import (
	"math/big"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/contracts/config"
	"github.com/celo-org/celo-blockchain/contracts/testutil"
)

func TestGetGasPriceSuggestion(t *testing.T) {
	celoAddress := common.HexToAddress("0x076")
	altFeeCurrency := common.HexToAddress("0x0AA")
	gpmAddress := common.HexToAddress("0x090")
	sortedOracleAddress := common.HexToAddress("0x091")

	t.Run("with baseFee != nil", func(t *testing.T) {
		t.Run("should return baseFee * 5 currency == nil", func(t *testing.T) {
			g := NewGomegaWithT(t)

			runner := testutil.NewMockEVMRunner()
			registry := testutil.NewRegistryMock()
			runner.RegisterContract(config.RegistrySmartContractAddress, registry)
			registry.AddContract(config.GoldTokenRegistryId, celoAddress)

			contract := testutil.NewSingleMethodContract(config.GasPriceMinimumRegistryId, "getGasPriceMinimum",
				func(currency common.Address) *big.Int { return big.NewInt(777777) },
			)
			runner.RegisterContract(gpmAddress, contract)
			registry.AddContract(config.GasPriceMinimumRegistryId, gpmAddress)

			suggestedGpm, err := GetGasPriceSuggestion(runner, nil, common.Big1)
			g.Expect(err).NotTo(HaveOccurred())

			g.Expect(suggestedGpm).To(Equal(new(big.Int).Mul(common.Big1, big.NewInt(5))))
		})

		t.Run("should return baseFee * exchangeRateOfCurrency * 5 currency != nil", func(t *testing.T) {
			g := NewGomegaWithT(t)

			runner := testutil.NewMockEVMRunner()
			registry := testutil.NewRegistryMock()
			runner.RegisterContract(config.RegistrySmartContractAddress, registry)
			registry.AddContract(config.GoldTokenRegistryId, celoAddress)

			// Adding the contract to check that is not using this numbers instead
			contractGPM := testutil.NewSingleMethodContract(config.GasPriceMinimumRegistryId, "getGasPriceMinimum",
				func(currency common.Address) *big.Int { return big.NewInt(777777) },
			)
			runner.RegisterContract(gpmAddress, contractGPM)
			registry.AddContract(config.GasPriceMinimumRegistryId, gpmAddress)

			contractSO := testutil.NewSingleMethodContract(config.SortedOraclesRegistryId, "medianRate",
				func(currency common.Address) (*big.Int, *big.Int) {
					if currency == altFeeCurrency {
						return common.Big2, common.Big1
					}
					return common.Big1, common.Big1
				},
			)
			runner.RegisterContract(sortedOracleAddress, contractSO)
			registry.AddContract(config.SortedOraclesRegistryId, sortedOracleAddress)

			suggestedGpm, err := GetGasPriceSuggestion(runner, &altFeeCurrency, common.Big1)
			g.Expect(err).NotTo(HaveOccurred())

			g.Expect(suggestedGpm).To(Equal(new(big.Int).Mul(new(big.Int).Mul(common.Big1, common.Big2), big.NewInt(5))))
		})
	})

	t.Run("with baseFee == nil", func(t *testing.T) {

		t.Run("should return gasPriceMinimum contract * 5 if current == nil", func(t *testing.T) {
			g := NewGomegaWithT(t)

			runner := testutil.NewMockEVMRunner()
			registry := testutil.NewRegistryMock()
			runner.RegisterContract(config.RegistrySmartContractAddress, registry)
			registry.AddContract(config.GoldTokenRegistryId, celoAddress)

			contract := testutil.NewSingleMethodContract(config.GasPriceMinimumRegistryId, "getGasPriceMinimum",
				func(currency common.Address) *big.Int {
					if currency == altFeeCurrency {
						return big.NewInt(555555)
					}
					return big.NewInt(777777)
				},
			)
			runner.RegisterContract(gpmAddress, contract)
			registry.AddContract(config.GasPriceMinimumRegistryId, gpmAddress)

			suggestedGpm, err := GetGasPriceSuggestion(runner, nil, nil)
			g.Expect(err).NotTo(HaveOccurred())

			g.Expect(suggestedGpm.Uint64()).To(Equal(uint64(777777 * 5)))
		})

		t.Run("should return gasPriceMinimum(feeCurrency) * 5 if baseFee != nil", func(t *testing.T) {
			g := NewGomegaWithT(t)

			runner := testutil.NewMockEVMRunner()
			registry := testutil.NewRegistryMock()
			runner.RegisterContract(config.RegistrySmartContractAddress, registry)
			registry.AddContract(config.GoldTokenRegistryId, celoAddress)

			contract := testutil.NewSingleMethodContract(config.GasPriceMinimumRegistryId, "getGasPriceMinimum",
				func(currency common.Address) *big.Int {
					if currency == altFeeCurrency {
						return big.NewInt(555555)
					}
					return big.NewInt(777777)
				},
			)
			runner.RegisterContract(gpmAddress, contract)
			registry.AddContract(config.GasPriceMinimumRegistryId, gpmAddress)

			suggestedGpm, err := GetGasPriceSuggestion(runner, &altFeeCurrency, nil)
			g.Expect(err).NotTo(HaveOccurred())

			g.Expect(suggestedGpm.Uint64()).To(Equal(uint64(555555 * 5)))
		})
	})
}

func TestGetBaseFeeForCurrency(t *testing.T) {
	celoAddress := common.HexToAddress("0x076")
	altFeeCurrency := common.HexToAddress("0x0AA")
	gpmAddress := common.HexToAddress("0x090")
	sortedOracleAddress := common.HexToAddress("0x091")

	t.Run("with baseFee != nil", func(t *testing.T) {
		t.Run("should return baseFee currency == nil", func(t *testing.T) {
			g := NewGomegaWithT(t)

			runner := testutil.NewMockEVMRunner()
			registry := testutil.NewRegistryMock()
			runner.RegisterContract(config.RegistrySmartContractAddress, registry)
			registry.AddContract(config.GoldTokenRegistryId, celoAddress)

			contract := testutil.NewSingleMethodContract(config.GasPriceMinimumRegistryId, "getGasPriceMinimum",
				func(currency common.Address) *big.Int { return big.NewInt(777777) },
			)
			runner.RegisterContract(gpmAddress, contract)
			registry.AddContract(config.GasPriceMinimumRegistryId, gpmAddress)

			contractSO := testutil.NewSingleMethodContract(config.SortedOraclesRegistryId, "medianRate",
				func(currency common.Address) (*big.Int, *big.Int) {
					if currency == altFeeCurrency {
						return common.Big2, common.Big1
					}
					return common.Big1, common.Big1
				},
			)
			runner.RegisterContract(sortedOracleAddress, contractSO)
			registry.AddContract(config.SortedOraclesRegistryId, sortedOracleAddress)

			suggestedGpm, err := GetBaseFeeForCurrency(runner, nil, common.Big1)
			g.Expect(err).NotTo(HaveOccurred())

			g.Expect(suggestedGpm).To(Equal(common.Big1))
		})

		t.Run("should return baseFee * exchangeRateOfCurrency currency != nil", func(t *testing.T) {
			g := NewGomegaWithT(t)

			runner := testutil.NewMockEVMRunner()
			registry := testutil.NewRegistryMock()
			runner.RegisterContract(config.RegistrySmartContractAddress, registry)
			registry.AddContract(config.GoldTokenRegistryId, celoAddress)

			// Adding the contract to check that is not using this numbers instead
			contractGPM := testutil.NewSingleMethodContract(config.GasPriceMinimumRegistryId, "getGasPriceMinimum",
				func(currency common.Address) *big.Int { return big.NewInt(777777) },
			)
			runner.RegisterContract(gpmAddress, contractGPM)
			registry.AddContract(config.GasPriceMinimumRegistryId, gpmAddress)

			contractSO := testutil.NewSingleMethodContract(config.SortedOraclesRegistryId, "medianRate",
				func(currency common.Address) (*big.Int, *big.Int) {
					if currency == altFeeCurrency {
						return common.Big2, common.Big1
					}
					return common.Big1, common.Big1
				},
			)
			runner.RegisterContract(sortedOracleAddress, contractSO)
			registry.AddContract(config.SortedOraclesRegistryId, sortedOracleAddress)

			suggestedGpm, err := GetBaseFeeForCurrency(runner, &altFeeCurrency, common.Big1)
			g.Expect(err).NotTo(HaveOccurred())

			g.Expect(suggestedGpm).To(Equal(new(big.Int).Mul(common.Big1, common.Big2)))
		})
	})

	t.Run("with baseFee == nil", func(t *testing.T) {

		t.Run("should return gasPriceMinimum if current == nil", func(t *testing.T) {
			g := NewGomegaWithT(t)

			runner := testutil.NewMockEVMRunner()
			registry := testutil.NewRegistryMock()
			runner.RegisterContract(config.RegistrySmartContractAddress, registry)
			registry.AddContract(config.GoldTokenRegistryId, celoAddress)

			contract := testutil.NewSingleMethodContract(config.GasPriceMinimumRegistryId, "getGasPriceMinimum",
				func(currency common.Address) *big.Int {
					if currency == altFeeCurrency {
						return big.NewInt(555555)
					}
					return big.NewInt(777777)
				},
			)
			runner.RegisterContract(gpmAddress, contract)
			registry.AddContract(config.GasPriceMinimumRegistryId, gpmAddress)

			suggestedGpm, err := GetBaseFeeForCurrency(runner, nil, nil)
			g.Expect(err).NotTo(HaveOccurred())

			g.Expect(suggestedGpm.Uint64()).To(Equal(uint64(777777)))
		})

		t.Run("should return gasPriceMinimum(feeCurrency) if baseFee != nil", func(t *testing.T) {
			g := NewGomegaWithT(t)

			runner := testutil.NewMockEVMRunner()
			registry := testutil.NewRegistryMock()
			runner.RegisterContract(config.RegistrySmartContractAddress, registry)
			registry.AddContract(config.GoldTokenRegistryId, celoAddress)

			contract := testutil.NewSingleMethodContract(config.GasPriceMinimumRegistryId, "getGasPriceMinimum",
				func(currency common.Address) *big.Int {
					if currency == altFeeCurrency {
						return big.NewInt(555555)
					}
					return big.NewInt(777777)
				},
			)
			runner.RegisterContract(gpmAddress, contract)
			registry.AddContract(config.GasPriceMinimumRegistryId, gpmAddress)

			suggestedGpm, err := GetBaseFeeForCurrency(runner, &altFeeCurrency, nil)
			g.Expect(err).NotTo(HaveOccurred())

			g.Expect(suggestedGpm.Uint64()).To(Equal(uint64(555555)))
		})
	})
}
