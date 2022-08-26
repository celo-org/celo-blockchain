package freezer

import (
	"testing"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/contracts"
	"github.com/celo-org/celo-blockchain/contracts/testutil"
	"github.com/celo-org/celo-blockchain/params"
	. "github.com/onsi/gomega"
)

func TestIsFrozen(t *testing.T) {
	testutil.TestFailOnFailingRunner(t, IsFrozen, params.BlockchainParametersRegistryId)
	testutil.TestFailsWhenContractNotDeployed(t, contracts.ErrSmartContractNotDeployed, IsFrozen, params.BlockchainParametersRegistryId)

	t.Run("should indicate if contract is frozen", func(t *testing.T) {
		g := NewWithT(t)

		var (
			freezerAddress    = common.HexToAddress("0x01")
			blockchainAddress = common.HexToAddress("0x02")
			validatorsAddress = common.HexToAddress("0x03")
		)

		runner := testutil.NewMockEVMRunner()

		contract := testutil.NewSingleMethodContract(params.FreezerRegistryId, "isFrozen", func(addr common.Address) bool {
			return addr == blockchainAddress
		})
		runner.RegisterContract(freezerAddress, contract)

		registry := testutil.NewRegistryMock()
		runner.RegisterContract(params.RegistrySmartContractAddress, registry)
		registry.AddContract(params.FreezerRegistryId, freezerAddress)
		registry.AddContract(params.ValidatorsRegistryId, validatorsAddress)
		registry.AddContract(params.BlockchainParametersRegistryId, blockchainAddress)

		isFrozen, err := IsFrozen(runner, params.BlockchainParametersRegistryId)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(isFrozen).To(BeTrue())

		isFrozen, err = IsFrozen(runner, params.ValidatorsRegistryId)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(isFrozen).To(BeFalse())
	})
}
