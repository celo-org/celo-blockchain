package freezer

import (
	"testing"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/contracts"
	"github.com/celo-org/celo-blockchain/contracts/config"
	"github.com/celo-org/celo-blockchain/contracts/testutil"
	. "github.com/onsi/gomega"
)

func TestIsFrozen(t *testing.T) {
	testutil.TestFailOnFailingRunner(t, IsFrozen, config.BlockchainParametersRegistryId)
	testutil.TestFailsWhenContractNotDeployed(t, contracts.ErrSmartContractNotDeployed, IsFrozen, config.BlockchainParametersRegistryId)

	t.Run("should indicate if contract is frozen", func(t *testing.T) {
		g := NewGomegaWithT(t)

		var (
			freezerAddress    = common.HexToAddress("0x01")
			blockchainAddress = common.HexToAddress("0x02")
			validatorsAddress = common.HexToAddress("0x03")
		)

		runner := testutil.NewMockEVMRunner()

		contract := testutil.NewSingleMethodContract(config.FreezerRegistryId, "isFrozen", func(addr common.Address) bool {
			return addr == blockchainAddress
		})
		runner.RegisterContract(freezerAddress, contract)

		registry := testutil.NewRegistryMock()
		runner.RegisterContract(config.RegistrySmartContractAddress, registry)
		registry.AddContract(config.FreezerRegistryId, freezerAddress)
		registry.AddContract(config.ValidatorsRegistryId, validatorsAddress)
		registry.AddContract(config.BlockchainParametersRegistryId, blockchainAddress)

		isFrozen, err := IsFrozen(runner, config.BlockchainParametersRegistryId)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(isFrozen).To(BeTrue())

		isFrozen, err = IsFrozen(runner, config.ValidatorsRegistryId)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(isFrozen).To(BeFalse())
	})
}
