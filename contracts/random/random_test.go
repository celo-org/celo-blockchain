package random

import (
	"math/big"
	"testing"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/contracts"
	"github.com/celo-org/celo-blockchain/contracts/config"
	"github.com/celo-org/celo-blockchain/contracts/testutil"
	. "github.com/onsi/gomega"
)

func TestIsRunning(t *testing.T) {
	t.Run("should be False if runner fails", func(t *testing.T) {
		g := NewGomegaWithT(t)
		g.Expect(IsRunning(testutil.FailingVmRunner{})).To(BeFalse())
	})
	t.Run("should be False if Registry Not deployed", func(t *testing.T) {
		g := NewGomegaWithT(t)
		vmrunner := testutil.NewMockEVMRunner()
		g.Expect(IsRunning(vmrunner)).To(BeFalse())

	})
	t.Run("should be False if Random Not deployed", func(t *testing.T) {
		g := NewGomegaWithT(t)
		vmrunner := testutil.NewMockEVMRunner()
		registry := testutil.NewRegistryMock()
		vmrunner.RegisterContract(config.RegistrySmartContractAddress, registry)
		g.Expect(IsRunning(vmrunner)).To(BeFalse())

	})
	t.Run("should be True if Random is deployed", func(t *testing.T) {
		g := NewGomegaWithT(t)
		vmrunner := testutil.NewMockEVMRunner()
		registry := testutil.NewRegistryMock()
		vmrunner.RegisterContract(config.RegistrySmartContractAddress, registry)
		registry.AddContract(config.RandomRegistryId, common.HexToAddress("0x033"))
		g.Expect(IsRunning(vmrunner)).To(BeTrue())

	})
}

func TestGetLastCommitment(t *testing.T) {
	validatorAddress := common.HexToAddress("0x09")
	someCommitment := common.HexToHash("0x666")

	testutil.TestFailOnFailingRunner(t, GetLastCommitment, validatorAddress)
	testutil.TestFailsWhenContractNotDeployed(t, contracts.ErrSmartContractNotDeployed, GetLastCommitment, validatorAddress)

	t.Run("should retrieve last commitment", func(t *testing.T) {
		g := NewGomegaWithT(t)
		vmrunner := testutil.NewSingleMethodRunner(config.RandomRegistryId, "commitments", func(validator common.Address) common.Hash {
			g.Expect(validator).To(Equal(validatorAddress))
			return someCommitment
		})

		ret, err := GetLastCommitment(vmrunner, validatorAddress)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(ret).To(Equal(someCommitment))
	})
}

func TestComputeCommitment(t *testing.T) {
	someRandomness := common.HexToHash("0x077777")
	someCommitment := common.HexToHash("0x666")

	testutil.TestFailOnFailingRunner(t, ComputeCommitment, someRandomness)
	testutil.TestFailsWhenContractNotDeployed(t, contracts.ErrSmartContractNotDeployed, ComputeCommitment, someRandomness)

	t.Run("should compute commitment", func(t *testing.T) {
		g := NewGomegaWithT(t)
		vmrunner := testutil.NewSingleMethodRunner(config.RandomRegistryId, "computeCommitment", func(randomness common.Hash) common.Hash {
			g.Expect(randomness).To(Equal(someRandomness))
			return someCommitment
		})

		ret, err := ComputeCommitment(vmrunner, someRandomness)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(ret).To(Equal(someCommitment))
	})
}

func TestRandom(t *testing.T) {
	someRandomness := common.HexToHash("0x077777")

	testutil.TestFailOnFailingRunner(t, Random)
	testutil.TestFailsWhenContractNotDeployed(t, contracts.ErrSmartContractNotDeployed, Random)

	t.Run("should retrieve current randomness", func(t *testing.T) {
		g := NewGomegaWithT(t)
		vmrunner := testutil.NewSingleMethodRunner(config.RandomRegistryId, "random", func() common.Hash {
			return someRandomness
		})

		ret, err := Random(vmrunner)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(ret).To(Equal(someRandomness))
	})
}

func TestBlockRandomness(t *testing.T) {
	blockNumber := uint64(999)
	someRandomness := common.HexToHash("0x077777")

	testutil.TestFailOnFailingRunner(t, BlockRandomness, blockNumber)
	testutil.TestFailsWhenContractNotDeployed(t, contracts.ErrSmartContractNotDeployed, BlockRandomness, blockNumber)

	t.Run("should retrieve randomness for block", func(t *testing.T) {
		g := NewGomegaWithT(t)
		vmrunner := testutil.NewSingleMethodRunner(config.RandomRegistryId, "getBlockRandomness", func(block *big.Int) common.Hash {
			g.Expect(block.Uint64()).To(Equal(blockNumber))
			return someRandomness
		})

		ret, err := BlockRandomness(vmrunner, blockNumber)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(ret).To(Equal(someRandomness))
	})
}
