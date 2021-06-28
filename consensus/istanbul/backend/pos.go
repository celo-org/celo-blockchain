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
	"github.com/ethereum/go-ethereum/consensus/istanbul/uptime"
	"github.com/ethereum/go-ethereum/consensus/istanbul/uptime/store"
	"github.com/ethereum/go-ethereum/contracts"
	"github.com/ethereum/go-ethereum/contracts/currency"
	"github.com/ethereum/go-ethereum/contracts/election"
	"github.com/ethereum/go-ethereum/contracts/epoch_rewards"
	"github.com/ethereum/go-ethereum/contracts/freezer"
	"github.com/ethereum/go-ethereum/contracts/gold_token"
	"github.com/ethereum/go-ethereum/contracts/validators"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/params"
)

func (sb *Backend) distributeEpochRewards(header *types.Header, state *state.StateDB) error {
	start := time.Now()
	defer sb.rewardDistributionTimer.UpdateSince(start)
	logger := sb.logger.New("func", "Backend.distributeEpochPaymentsAndRewards", "blocknum", header.Number.Uint64())

	vmRunner := sb.chain.NewEVMRunner(header, state)
	// Check if reward distribution has been frozen and return early without error if it is.
	if frozen, err := freezer.IsFrozen(vmRunner, params.EpochRewardsRegistryId); err != nil {
		logger.Warn("Failed to determine if epoch rewards are frozen", "err", err)
	} else if frozen {
		logger.Debug("Epoch rewards are frozen, skipping distribution")
		return nil
	}

	// Get necessary Addresses First
	reserveAddress, err := contracts.GetRegisteredAddress(vmRunner, params.ReserveRegistryId)
	if err != nil {
		return err
	}
	stableTokenAddress, err := contracts.GetRegisteredAddress(vmRunner, params.StableTokenRegistryId)
	if err != nil {
		return err
	}

	carbonOffsettingPartnerAddress, err := epoch_rewards.GetCarbonOffsettingPartnerAddress(vmRunner)
	if err != nil {
		return err
	}

	err = epoch_rewards.UpdateTargetVotingYield(vmRunner)
	if err != nil {
		return err
	}

	validatorReward, totalVoterRewards, communityReward, carbonOffsettingPartnerReward, err := epoch_rewards.CalculateTargetEpochRewards(vmRunner)
	if err != nil {
		return err
	}

	if carbonOffsettingPartnerAddress == common.ZeroAddress {
		carbonOffsettingPartnerReward = big.NewInt(0)
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

	totalValidatorRewards, err := sb.distributeValidatorRewards(vmRunner, valSet, validatorReward)
	if err != nil {
		return err
	}

	// TODO(HF) Use vmRunner instead of current block's one
	currentBlockVMRunner, err := sb.chain.NewEVMRunnerForCurrentBlock()
	if err != nil {
		return err
	}
	currencyManager := currency.NewManager(currentBlockVMRunner)

	// Validator rewards were paid in cUSD, convert that amount to CELO and add it to the Reserve
	stableTokenCurrency, err := currencyManager.GetCurrency(&stableTokenAddress)
	if err != nil {
		return err
	}
	totalValidatorRewardsConvertedToCelo := stableTokenCurrency.ToCELO(totalValidatorRewards)

	if err = gold_token.Mint(vmRunner, reserveAddress, totalValidatorRewardsConvertedToCelo); err != nil {
		return err
	}

	if err := sb.distributeCommunityRewards(vmRunner, communityReward); err != nil {
		return err
	}

	if err := sb.distributeVoterRewards(vmRunner, valSet, totalVoterRewards, uptimes); err != nil {
		return err
	}

	if carbonOffsettingPartnerReward.Cmp(new(big.Int)) != 0 {
		if err = gold_token.Mint(vmRunner, carbonOffsettingPartnerAddress, carbonOffsettingPartnerReward); err != nil {
			return err
		}
	}

	return nil
}

func (sb *Backend) updateValidatorScores(header *types.Header, state *state.StateDB, valSet []istanbul.Validator) ([]*big.Int, error) {
	epoch := istanbul.GetEpochNumber(header.Number.Uint64(), sb.EpochSize())
	logger := sb.logger.New("func", "Backend.updateValidatorScores", "blocknum", header.Number.Uint64(), "epoch", epoch, "epochsize", sb.EpochSize())

	// header (&state) == lastBlockOfEpoch
	// sb.LookbackWindow(header, state) => value at the end of epoch
	// It doesn't matter which was the value at the beginning but how it ends.
	// Notice that exposed metrics compute based on current block (not last of epoch) so if lookback window changed during the epoch, metric uptime score might differ
	lookbackWindow := sb.LookbackWindow(header, state)

	logger = logger.New("window", lookbackWindow)
	logger.Trace("Updating validator scores")

	monitor := uptime.NewMonitor(store.New(sb.db), sb.EpochSize(), lookbackWindow)
	uptimes, err := monitor.ComputeValidatorsUptime(epoch, len(valSet))
	if err != nil {
		return nil, err
	}

	vmRunner := sb.chain.NewEVMRunner(header, state)
	for i, val := range valSet {
		logger.Trace("Updating validator score", "uptime", uptimes[i], "address", val.Address())
		err := validators.UpdateValidatorScore(vmRunner, val.Address(), uptimes[i])
		if err != nil {
			return nil, err
		}
	}
	return uptimes, nil
}

func (sb *Backend) distributeValidatorRewards(vmRunner vm.EVMRunner, valSet []istanbul.Validator, maxReward *big.Int) (*big.Int, error) {
	totalValidatorRewards := big.NewInt(0)
	for _, val := range valSet {
		sb.logger.Debug("Distributing epoch reward for validator", "address", val.Address())
		validatorReward, err := validators.DistributeEpochReward(vmRunner, val.Address(), maxReward)
		if err != nil {
			sb.logger.Error("Error in distributing rewards to validator", "address", val.Address(), "err", err)
			continue
		}
		totalValidatorRewards.Add(totalValidatorRewards, validatorReward)
	}
	return totalValidatorRewards, nil
}

func (sb *Backend) distributeCommunityRewards(vmRunner vm.EVMRunner, communityReward *big.Int) error {
	governanceAddress, err := contracts.GetRegisteredAddress(vmRunner, params.GovernanceRegistryId)
	if err != nil {
		return err
	}
	reserveAddress, err := contracts.GetRegisteredAddress(vmRunner, params.ReserveRegistryId)
	if err != nil {
		return err
	}
	lowReserve, err := epoch_rewards.IsReserveLow(vmRunner)
	if err != nil {
		return err
	}

	if lowReserve && reserveAddress != common.ZeroAddress {
		return gold_token.Mint(vmRunner, reserveAddress, communityReward)
	} else if governanceAddress != common.ZeroAddress {
		// TODO: How to split eco fund here
		return gold_token.Mint(vmRunner, governanceAddress, communityReward)
	}
	return nil
}

func (sb *Backend) distributeVoterRewards(vmRunner vm.EVMRunner, valSet []istanbul.Validator, maxTotalRewards *big.Int, uptimes []*big.Int) error {

	lockedGoldAddress, err := contracts.GetRegisteredAddress(vmRunner, params.LockedGoldRegistryId)
	if err != nil {
		return err
	} else if lockedGoldAddress == common.ZeroAddress {
		return errors.New("Unable to fetch locked gold address for epoch rewards distribution")
	}

	// Select groups that elected at least one validator aggregate their uptimes.
	var groups []common.Address
	groupUptimes := make(map[common.Address][]*big.Int)
	groupElectedValidator := make(map[common.Address]bool)
	for i, val := range valSet {
		group, err := validators.GetMembershipInLastEpoch(vmRunner, val.Address())
		if err != nil {
			return err
		}
		if _, ok := groupElectedValidator[group]; !ok {
			groups = append(groups, group)
			sb.logger.Debug("Group elected validator", "group", group.String())
		}
		groupElectedValidator[group] = true
		groupUptimes[group] = append(groupUptimes[group], uptimes[i])
	}

	electionRewards, err := election.DistributeEpochRewards(vmRunner, groups, maxTotalRewards, groupUptimes)
	if err != nil {
		return err
	}

	return gold_token.Mint(vmRunner, lockedGoldAddress, electionRewards)
}

func (sb *Backend) setInitialGoldTokenTotalSupplyIfUnset(vmRunner vm.EVMRunner) error {
	totalSupply, err := gold_token.GetTotalSupply(vmRunner)
	if err != nil {
		return err
	}
	// totalSupply not yet initialized.
	if totalSupply.Cmp(common.Big0) == 0 {
		data, err := sb.db.Get(core.DBGenesisSupplyKey)
		if err != nil {
			log.Error("Unable to fetch genesisSupply from db", "err", err)
			return err
		}
		genesisSupply := new(big.Int)
		genesisSupply.SetBytes(data)

		err = gold_token.IncreaseSupply(vmRunner, genesisSupply)
		if err != nil {
			return err
		}
	}
	return nil
}
