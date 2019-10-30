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
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/consensus/istanbul"
	"github.com/ethereum/go-ethereum/contract_comm"
	"github.com/ethereum/go-ethereum/contract_comm/election"
	"github.com/ethereum/go-ethereum/contract_comm/gold_token"
	"github.com/ethereum/go-ethereum/contract_comm/validators"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/params"
)

func (sb *Backend) updateValidatorScoresAndDistributeEpochPaymentsAndRewards(header *types.Header, state *state.StateDB) error {
	// The validator set that signs off on the last block of the epoch is the one that we need to
	// iterate over.
	valSet := sb.GetValidators(big.NewInt(header.Number.Int64()-1), header.ParentHash)
	if len(valSet) == 0 {
		sb.logger.Error("Unable to fetch validator set to update scores and distribute payments and rewards")
	}

	err := sb.updateValidatorScores(header, state, valSet)
	if err != nil {
		return err
	}
	err = sb.distributeEpochPayments(header, state, valSet)
	if err != nil {
		return err
	}
	totalEpochRewards, err := sb.distributeEpochRewards(header, state, valSet)
	if err != nil {
		return err
	}
	return sb.increaseGoldTokenTotalSupply(header, state, totalEpochRewards)
}

func (sb *Backend) updateValidatorScores(header *types.Header, state *state.StateDB, valSet []istanbul.Validator) error {
	for _, val := range valSet {
		// TODO: Use actual uptime metric.
		// 1.0 in fixidity
		uptime := math.BigPow(10, 24)
		sb.logger.Info("Updating validator score for address", "address", val.Address(), "uptime", uptime.String())
		err := validators.UpdateValidatorScore(header, state, val.Address(), uptime)
		if err != nil {
			return err
		}
	}
	return nil
}

func (sb *Backend) distributeEpochPayments(header *types.Header, state *state.StateDB, valSet []istanbul.Validator) error {
	for _, val := range valSet {
		sb.logger.Info("Distributing epoch payment for address", "address", val.Address())
		err := validators.DistributeEpochPayment(header, state, val.Address())
		if err != nil {
			return nil
		}
	}
	return nil
}

func (sb *Backend) distributeEpochRewards(header *types.Header, state *state.StateDB, valSet []istanbul.Validator) (*big.Int, error) {
	totalEpochRewards := big.NewInt(0)

	// Fixed epoch reward to the infrastructure fund.
	// TODO(asa): This should be a fraction of the overall reward to stakers.
	infrastructureEpochReward := big.NewInt(params.Ether)
	governanceAddress, err := contract_comm.GetRegisteredAddress(params.GovernanceRegistryId, header, state)
	if err != nil {
		return totalEpochRewards, err
	}

	if governanceAddress != nil {
		state.AddBalance(*governanceAddress, infrastructureEpochReward)
		totalEpochRewards.Add(totalEpochRewards, infrastructureEpochReward)
	}

	groupElectedValidator := make(map[common.Address]bool)
	for _, val := range valSet {
		group, err := validators.GetMembershipInLastEpoch(header, state, val.Address())
		if err != nil {
			return totalEpochRewards, err
		} else {
			groupElectedValidator[group] = true
		}
	}

	// One gold
	// TODO(asa): This should not be fixed.
	electionRewardsCeiling := math.BigPow(10, 18)

	groups := make([]common.Address, 0, len(groupElectedValidator))
	for group := range groupElectedValidator {
		groups = append(groups, group)
	}
	electionRewards, err := election.DistributeEpochRewards(header, state, groups, electionRewardsCeiling)
	lockedGoldAddress, err := contract_comm.GetRegisteredAddress(params.LockedGoldRegistryId, header, state)
	if err != nil {
		return totalEpochRewards, err
	}
	if lockedGoldAddress != nil {
		state.AddBalance(*lockedGoldAddress, electionRewards)
		totalEpochRewards.Add(totalEpochRewards, electionRewards)
	}
	return totalEpochRewards, err
}

func (sb *Backend) setInitialGoldTokenTotalSupplyIfUnset(header *types.Header, state *state.StateDB) error {
	totalSupply, err := gold_token.GetTotalSupply(header, state)
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

		err = sb.increaseGoldTokenTotalSupply(header, state, genesisSupply)
		if err != nil {
			return err
		}
	}
	return nil
}

func (sb *Backend) increaseGoldTokenTotalSupply(header *types.Header, state *state.StateDB, increase *big.Int) error {
	if increase.Cmp(common.Big0) > 0 {
		return gold_token.IncreaseSupply(header, state, increase)
	}
	return nil
}
