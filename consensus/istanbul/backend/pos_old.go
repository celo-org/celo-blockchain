// Copyright 2017 The celo Authors
// This file is part of the celo library.
//
// The celo library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The celo library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the celo library. If not, see <http://www.gnu.org/licenses/>.

package backend

import (
	"errors"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/istanbul"
	"github.com/ethereum/go-ethereum/contract_comm"
	"github.com/ethereum/go-ethereum/contract_comm/currency"
	"github.com/ethereum/go-ethereum/contract_comm/election"
	"github.com/ethereum/go-ethereum/contract_comm/epoch_rewards"
	"github.com/ethereum/go-ethereum/contract_comm/freezer"
	"github.com/ethereum/go-ethereum/contract_comm/gold_token"
	"github.com/ethereum/go-ethereum/contract_comm/validators"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/params"
)

func (sb *Backend) distributeEpochRewardsNoMint(header *types.Header, state *state.StateDB) error {
	start := time.Now()
	defer sb.rewardDistributionTimer.UpdateSince(start)
	logger := sb.logger.New("func", "Backend.distributeEpochPaymentsAndRewards", "blocknum", header.Number.Uint64())

	// Check if reward distribution has been frozen and return early without error if it is.
	if frozen, err := freezer.IsFrozen(params.EpochRewardsRegistryId, header, state); err != nil {
		logger.Warn("Failed to determine if epoch rewards are frozen", "err", err)
	} else if frozen {
		logger.Debug("Epoch rewards are frozen, skipping distribution")
		return nil
	}

	err := epoch_rewards.UpdateTargetVotingYield(header, state)
	if err != nil {
		return err
	}
	validatorReward, totalVoterRewards, communityReward, carbonOffsettingPartnerReward, err := epoch_rewards.CalculateTargetEpochRewards(header, state)
	if err != nil {
		return err
	}

	logger.Debug("Calculated target rewards", "validatorReward", validatorReward, "totalVoterRewards", totalVoterRewards, "communityReward", communityReward)

	// The validator set that signs off on the last block of the epoch is the one that we need to
	// iterate over.
	valSet := sb.GetValidators(big.NewInt(header.Number.Int64()-1), header.ParentHash)
	if len(valSet) == 0 {

		err := errors.New("Unable to fetch validator set to update scores and distribute rewards")
		logger.Error(err.Error())
		return err
	}

	uptimes, err := sb.updateValidatorScores(header, state, valSet)
	if err != nil {
		return err
	}

	totalValidatorRewards, err := sb.distributeValidatorRewards(header, state, valSet, validatorReward)
	if err != nil {
		return err
	}
	totalCommunityRewards, err := sb.distributeCommunityRewardsNoMint(header, state, communityReward)
	if err != nil {
		return err
	}

	totalDistributedVoterRewards, err := sb.distributeVoterRewardsNoMint(header, state, valSet, totalVoterRewards, uptimes)
	if err != nil {
		return err
	}

	stableTokenAddress, err := contract_comm.GetRegisteredAddress(params.StableTokenRegistryId, header, state)
	if err != nil {
		return err
	}

	totalValidatorRewardsConvertedToGold, err := currency.Convert(totalValidatorRewards, stableTokenAddress, nil)
	if err != nil {
		return err
	}

	reserveAddress, err := contract_comm.GetRegisteredAddress(params.ReserveRegistryId, header, state)
	if err != nil {
		return err
	}
	if reserveAddress != nil {
		state.AddBalance(*reserveAddress, totalValidatorRewardsConvertedToGold)
	} else {
		return errors.New("Unable to fetch reserve address for epoch rewards distribution")
	}

	carbonOffsettingPartnerAddress, err := epoch_rewards.GetCarbonOffsettingPartnerAddress(header, state)
	if err != nil {
		return err
	}
	if carbonOffsettingPartnerAddress != common.ZeroAddress {
		state.AddBalance(carbonOffsettingPartnerAddress, carbonOffsettingPartnerReward)
	} else {
		carbonOffsettingPartnerReward = big.NewInt(0)
	}

	mintedGold := big.NewInt(0).Add(totalDistributedVoterRewards, totalValidatorRewardsConvertedToGold)
	mintedGold.Add(mintedGold, totalCommunityRewards)
	mintedGold.Add(mintedGold, carbonOffsettingPartnerReward)
	return sb.increaseGoldTokenTotalSupply(header, state, mintedGold)
}

func (sb *Backend) distributeCommunityRewardsNoMint(header *types.Header, state *state.StateDB, communityReward *big.Int) (*big.Int, error) {
	totalCommunityRewards := big.NewInt(0)
	governanceAddress, err := contract_comm.GetRegisteredAddress(params.GovernanceRegistryId, header, state)
	if err != nil {
		return totalCommunityRewards, err
	}
	reserveAddress, err := contract_comm.GetRegisteredAddress(params.ReserveRegistryId, header, state)
	if err != nil {
		return totalCommunityRewards, err
	}
	lowReserve, err := epoch_rewards.IsReserveLow(header, state)
	if err != nil {
		return totalCommunityRewards, err
	}
	if lowReserve && reserveAddress != nil {
		state.AddBalance(*reserveAddress, communityReward)
		totalCommunityRewards.Add(totalCommunityRewards, communityReward)
	} else if governanceAddress != nil {
		// TODO: How to split eco fund here
		state.AddBalance(*governanceAddress, communityReward)
		totalCommunityRewards.Add(totalCommunityRewards, communityReward)
	}

	return totalCommunityRewards, err
}

func (sb *Backend) distributeVoterRewardsNoMint(header *types.Header, state *state.StateDB, valSet []istanbul.Validator, maxTotalRewards *big.Int, uptimes []*big.Int) (*big.Int, error) {
	totalVoterRewards := big.NewInt(0)

	// Select groups that elected at least one validator aggregate their uptimes.
	var groups []common.Address
	groupUptimes := make(map[common.Address][]*big.Int)
	groupElectedValidator := make(map[common.Address]bool)
	for i, val := range valSet {
		group, err := validators.GetMembershipInLastEpoch(header, state, val.Address())
		if err != nil {
			return totalVoterRewards, err
		}
		if _, ok := groupElectedValidator[group]; !ok {
			groups = append(groups, group)
			sb.logger.Debug("Group elected validator", "group", group.String())
		}
		groupElectedValidator[group] = true
		groupUptimes[group] = append(groupUptimes[group], uptimes[i])
	}

	electionRewards, err := election.DistributeEpochRewards(header, state, groups, maxTotalRewards, groupUptimes)
	if err != nil {
		return totalVoterRewards, err
	}
	lockedGoldAddress, err := contract_comm.GetRegisteredAddress(params.LockedGoldRegistryId, header, state)
	if err != nil {
		return totalVoterRewards, err
	}
	if lockedGoldAddress != nil {
		state.AddBalance(*lockedGoldAddress, electionRewards)
		totalVoterRewards.Add(totalVoterRewards, electionRewards)
	} else {
		return totalVoterRewards, errors.New("Unable to fetch locked gold address for epoch rewards distribution")
	}
	return totalVoterRewards, err
}

func (sb *Backend) increaseGoldTokenTotalSupply(header *types.Header, state *state.StateDB, increase *big.Int) error {
	if increase.Cmp(common.Big0) > 0 {
		return gold_token.IncreaseSupply(header, state, increase)
	}
	return nil
}
