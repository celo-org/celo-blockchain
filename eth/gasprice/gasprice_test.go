package gasprice

import (
	"math"
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
		t.Run("should return baseFee multiplied with factor if currency == nil", func(t *testing.T) {
			g := NewGomegaWithT(t)

			runner := testutil.NewMockEVMRunner()
			registry := testutil.NewRegistryMock()
			runner.RegisterContract(config.RegistrySmartContractAddress, registry)
			registry.AddContract(config.GoldTokenRegistryId, celoAddress)

			contract := testutil.NewSingleMethodContract(config.GasPriceMinimumRegistryId, "getGasPriceMinimum",
				func(currency common.Address) *big.Int { return big.NewInt(555555) },
			)
			runner.RegisterContract(gpmAddress, contract)
			registry.AddContract(config.GasPriceMinimumRegistryId, gpmAddress)

			suggestedGpm, err := GetGasPriceSuggestion(runner, nil, big.NewInt(777777), big.NewInt(500))
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(suggestedGpm.Uint64()).To(Equal(uint64(777777 * 5)))

			suggestedGpm, err = GetGasPriceSuggestion(runner, nil, big.NewInt(777777), big.NewInt(100))
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(suggestedGpm.Uint64()).To(Equal(uint64(777777)))

			suggestedGpm, err = GetGasPriceSuggestion(runner, nil, big.NewInt(777777), big.NewInt(110))
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(suggestedGpm.Uint64()).To(Equal(uint64(math.Floor(777777 * 1.1))))
		})

		t.Run("should return baseFee * exchangeRateOfCurrency multiplied with factor if currency != nil", func(t *testing.T) {
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

			suggestedGpm, err := GetGasPriceSuggestion(runner, &altFeeCurrency, big.NewInt(555555), big.NewInt(500))
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(suggestedGpm.Uint64()).To(Equal(uint64(555555 * 5 * 2)))

			suggestedGpm, err = GetGasPriceSuggestion(runner, &altFeeCurrency, big.NewInt(555555), big.NewInt(100))
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(suggestedGpm.Uint64()).To(Equal(uint64(555555 * 2)))

			suggestedGpm, err = GetGasPriceSuggestion(runner, &altFeeCurrency, big.NewInt(555555), big.NewInt(110))
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(suggestedGpm.Uint64()).To(Equal(uint64(math.Floor(555555 * 1.1 * 2))))
		})
	})

	t.Run("with baseFee == nil", func(t *testing.T) {

		t.Run("should return gasPriceMinimum contract multiplied with factor if current == nil", func(t *testing.T) {
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

			suggestedGpm, err := GetGasPriceSuggestion(runner, nil, nil, big.NewInt(500))
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(suggestedGpm.Uint64()).To(Equal(uint64(777777 * 5)))

			suggestedGpm, err = GetGasPriceSuggestion(runner, nil, nil, big.NewInt(100))
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(suggestedGpm.Uint64()).To(Equal(uint64(777777)))

			suggestedGpm, err = GetGasPriceSuggestion(runner, nil, nil, big.NewInt(110))
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(suggestedGpm.Uint64()).To(Equal(uint64(math.Floor(777777 * 1.1))))
		})

		t.Run("should return gasPriceMinimum(feeCurrency) multiplied with factor if baseFee != nil", func(t *testing.T) {
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

			suggestedGpm, err := GetGasPriceSuggestion(runner, &altFeeCurrency, nil, big.NewInt(500))
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(suggestedGpm.Uint64()).To(Equal(uint64(555555 * 5)))

			suggestedGpm, err = GetGasPriceSuggestion(runner, &altFeeCurrency, nil, big.NewInt(100))
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(suggestedGpm.Uint64()).To(Equal(uint64(555555)))

			suggestedGpm, err = GetGasPriceSuggestion(runner, &altFeeCurrency, nil, big.NewInt(110))
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(suggestedGpm.Uint64()).To(Equal(uint64(math.Floor(555555 * 1.1))))
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
