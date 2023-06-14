package gasprice_minimum

import (
	"math/big"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/contracts/config"
	"github.com/celo-org/celo-blockchain/contracts/testutil"
)

func TestGetGasPriceMinimum(t *testing.T) {
	cusdAddress := common.HexToAddress("0x077")
	celoAddress := common.HexToAddress("0x076")
	gpmAddress := common.HexToAddress("0x090")

	t.Run("should fail when vmRunner is failing", func(t *testing.T) {
		g := NewGomegaWithT(t)

		runner := testutil.FailingVmRunner{}

		// with gold currency
		ret, err := GetGasPriceMinimum(runner, nil)
		g.Expect(err).To(MatchError(testutil.ErrFailingRunner))
		g.Expect(ret).To(Equal(FallbackGasPriceMinimum))

		// with non gold currency
		ret, err = GetGasPriceMinimum(runner, &cusdAddress)
		g.Expect(err).To(MatchError(testutil.ErrFailingRunner))
		g.Expect(ret).To(Equal(FallbackGasPriceMinimum))
	})

	t.Run("should return fallback price when registry is not deployed", func(t *testing.T) {
		g := NewGomegaWithT(t)

		runner := testutil.NewMockEVMRunner()

		// with gold currency
		ret, err := GetGasPriceMinimum(runner, nil)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(ret).To(Equal(FallbackGasPriceMinimum))
	})

	t.Run("should return fallback price when goldToken is not deployed", func(t *testing.T) {
		g := NewGomegaWithT(t)

		runner := testutil.NewMockEVMRunner()
		registry := testutil.NewRegistryMock()
		runner.RegisterContract(config.RegistrySmartContractAddress, registry)

		// with gold currency
		ret, err := GetGasPriceMinimum(runner, nil)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(ret).To(Equal(FallbackGasPriceMinimum))
	})

	t.Run("should return fallback price when gasPriceMinimum is not deployed", func(t *testing.T) {
		g := NewGomegaWithT(t)

		runner := testutil.NewMockEVMRunner()
		registry := testutil.NewRegistryMock()
		runner.RegisterContract(config.RegistrySmartContractAddress, registry)
		registry.AddContract(config.StableTokenRegistryId, cusdAddress)
		registry.AddContract(config.GoldTokenRegistryId, celoAddress)

		// with gold currency
		ret, err := GetGasPriceMinimum(runner, nil)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(ret).To(Equal(FallbackGasPriceMinimum))

		// with non gold currency
		ret, err = GetGasPriceMinimum(runner, &cusdAddress)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(ret).To(Equal(FallbackGasPriceMinimum))
	})

	t.Run("should return gasPriceMinimum for CELO", func(t *testing.T) {
		g := NewGomegaWithT(t)

		runner := testutil.NewMockEVMRunner()
		registry := testutil.NewRegistryMock()
		runner.RegisterContract(config.RegistrySmartContractAddress, registry)
		registry.AddContract(config.GoldTokenRegistryId, celoAddress)

		contract := testutil.NewSingleMethodContract(config.GasPriceMinimumRegistryId, "getGasPriceMinimum", func(currency common.Address) *big.Int {
			g.Expect(currency).To(Equal(celoAddress))
			return big.NewInt(777777)
		})
		runner.RegisterContract(gpmAddress, contract)
		registry.AddContract(config.GasPriceMinimumRegistryId, gpmAddress)

		ret, err := GetGasPriceMinimum(runner, nil)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(ret.Uint64()).To(Equal(uint64(777777)))
	})

	t.Run("should return gasPriceMinimum for CELO", func(t *testing.T) {
		g := NewGomegaWithT(t)

		runner := testutil.NewMockEVMRunner()
		registry := testutil.NewRegistryMock()
		runner.RegisterContract(config.RegistrySmartContractAddress, registry)
		registry.AddContract(config.StableTokenRegistryId, cusdAddress)

		contract := testutil.NewSingleMethodContract(config.GasPriceMinimumRegistryId, "getGasPriceMinimum", func(currency common.Address) *big.Int {
			g.Expect(currency).To(Equal(cusdAddress))
			return big.NewInt(777777)
		})
		runner.RegisterContract(gpmAddress, contract)
		registry.AddContract(config.GasPriceMinimumRegistryId, gpmAddress)

		// with non gold currency
		ret, err := GetGasPriceMinimum(runner, &cusdAddress)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(ret.Uint64()).To(Equal(uint64(777777)))
	})

}
func TestUpdateGasPriceMinimum(t *testing.T) {
	t.Run("should update gasPriceMinimum with current blockGasLimit", func(t *testing.T) {
		g := NewGomegaWithT(t)

		var (
			gpmAddress                         = common.HexToAddress("0x99")
			blockchainParametersAddress        = common.HexToAddress("0xAA")
			lastUsedGas                 uint64 = 50000
			blockGasLimit               uint64 = 100000
		)

		runner := testutil.NewMockEVMRunner()
		registry := testutil.NewRegistryMock()
		runner.RegisterContract(config.RegistrySmartContractAddress, registry)
		registry.AddContract(config.BlockchainParametersRegistryId, blockchainParametersAddress)
		registry.AddContract(config.GasPriceMinimumRegistryId, gpmAddress)

		runner.RegisterContract(blockchainParametersAddress,
			testutil.NewSingleMethodContract(config.BlockchainParametersRegistryId, "blockGasLimit",
				func() *big.Int {
					return big.NewInt(int64(blockGasLimit))
				}),
		)
		runner.RegisterContract(gpmAddress,
			testutil.NewSingleMethodContract(config.GasPriceMinimumRegistryId, "updateGasPriceMinimum",
				func(gas *big.Int, maxGas *big.Int) *big.Int {
					g.Expect(gas.Uint64()).To(Equal(lastUsedGas))
					g.Expect(maxGas.Uint64()).To(Equal(blockGasLimit))
					return new(big.Int).SetUint64(999999)
				}),
		)

		newGpm, err := UpdateGasPriceMinimum(runner, lastUsedGas)

		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(newGpm.Uint64()).To(Equal(uint64(999999)))
	})
}
