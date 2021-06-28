package epoch_rewards

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/contracts"
	"github.com/ethereum/go-ethereum/contracts/testutil"
	"github.com/ethereum/go-ethereum/params"
	. "github.com/onsi/gomega"
)

// func TestUpdateTargetVotingYield(t *testing.T) {
// 	fn := UpdateTargetVotingYield
// 	testutil.TestFailOnFailingRunner(t, fn)
// 	testutil.TestFailsWhenContractNotDeployed(t, contracts.ErrSmartContractNotDeployed, fn)

// }
func TestCalculateTargetEpochRewards(t *testing.T) {
	fn := CalculateTargetEpochRewards
	testutil.TestFailOnFailingRunner(t, fn)
	testutil.TestFailsWhenContractNotDeployed(t, contracts.ErrSmartContractNotDeployed, fn)

	t.Run("should return target epoch rewards", func(t *testing.T) {
		g := NewGomegaWithT(t)

		runner := testutil.NewSingleMethodRunner(
			params.EpochRewardsRegistryId,
			"calculateTargetEpochRewards",
			func() (*big.Int, *big.Int, *big.Int, *big.Int) {
				return big.NewInt(5), big.NewInt(4), big.NewInt(3), big.NewInt(10)
			},
		)

		validatorEpochReward, totalVoterRewards, totalCommunityReward, totalCarbonOffsettingPartnerReward, err := CalculateTargetEpochRewards(runner)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(validatorEpochReward.Int64()).To(Equal(int64(5)))
		g.Expect(totalVoterRewards.Int64()).To(Equal(int64(4)))
		g.Expect(totalCommunityReward.Int64()).To(Equal(int64(3)))
		g.Expect(totalCarbonOffsettingPartnerReward.Int64()).To(Equal(int64(10)))
	})
}
func TestIsReserveLow(t *testing.T) {
	fn := IsReserveLow
	testutil.TestFailOnFailingRunner(t, fn)
	testutil.TestFailsWhenContractNotDeployed(t, contracts.ErrSmartContractNotDeployed, fn)

	t.Run("should indicate if reserve is low", func(t *testing.T) {
		g := NewGomegaWithT(t)

		runner := testutil.NewSingleMethodRunner(
			params.EpochRewardsRegistryId,
			"isReserveLow",
			func() bool { return true },
		)

		ret, err := IsReserveLow(runner)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(ret).To(BeTrue())
	})
}
func TestGetCarbonOffsettingPartnerAddress(t *testing.T) {
	fn := GetCarbonOffsettingPartnerAddress

	testutil.TestFailOnFailingRunner(t, fn)
	testutil.TestFailsWhenContractNotDeployed(t, contracts.ErrSmartContractNotDeployed, fn)

	t.Run("should indicate if reserve is low", func(t *testing.T) {
		g := NewGomegaWithT(t)

		runner := testutil.NewSingleMethodRunner(
			params.EpochRewardsRegistryId,
			"carbonOffsettingPartner",
			func() common.Address { return common.HexToAddress("0x00045") },
		)

		ret, err := GetCarbonOffsettingPartnerAddress(runner)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(ret).To(Equal(common.HexToAddress("0x00045")))
	})
}
