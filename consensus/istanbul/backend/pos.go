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

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/istanbul"
	"github.com/ethereum/go-ethereum/contract_comm"
	"github.com/ethereum/go-ethereum/contract_comm/currency"
	"github.com/ethereum/go-ethereum/contract_comm/election"
	"github.com/ethereum/go-ethereum/contract_comm/epoch_rewards"
	"github.com/ethereum/go-ethereum/contract_comm/gold_token"
	"github.com/ethereum/go-ethereum/contract_comm/validators"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/params"
)

func (sb *Backend) distributeEpochPaymentsAndRewards(header *types.Header, state *state.StateDB) error {
	err := epoch_rewards.UpdateTargetVotingYield(header, state)
	if err != nil {
		return err
	}
	validatorEpochPayment, totalVoterRewards, err := epoch_rewards.CalculateTargetEpochPaymentAndRewards(header, state)
	if err != nil {
		return err
	}
	log.Debug("Calculated target epoch payment and rewards", "validatorEpochPayment", validatorEpochPayment, "totalVoterRewards", totalVoterRewards)

	// The validator set that signs off on the last block of the epoch is the one that we need to
	// iterate over.
	valSet := sb.GetValidators(big.NewInt(header.Number.Int64()-1), header.ParentHash)
	if len(valSet) == 0 {
		sb.logger.Error("Unable to fetch validator set to update scores and distribute payments and rewards")
	}

	err = sb.updateValidatorScores(header, state, valSet)
	if err != nil {
		return err
	}

	totalEpochPayments, err := sb.distributeEpochPayments(header, state, valSet, validatorEpochPayment)
	if err != nil {
		return err
	}

	totalEpochRewards, err := sb.distributeEpochRewards(header, state, valSet, totalVoterRewards)
	if err != nil {
		return err
	}

	stableTokenAddress, err := contract_comm.GetRegisteredAddress(params.StableTokenRegistryId, header, state)
	if err != nil {
		return err
	}
	totalEpochPaymentsConvertedToGold, err := currency.Convert(totalEpochPayments, stableTokenAddress, nil)

	return sb.increaseGoldTokenTotalSupply(header, state, big.NewInt(0).Add(totalEpochRewards, totalEpochPaymentsConvertedToGold))
}

func (sb *Backend) updateValidatorScores(header *types.Header, state *state.StateDB, valSet []istanbul.Validator) error {
	epoch := istanbul.GetEpochNumber(header.Number.Uint64(), sb.EpochSize())
	logger := sb.logger.New("func", "Backend.updateValidatorScores", "blocknum", header.Number.Uint64(), "epoch", epoch, "epochsize", sb.EpochSize(), "window", sb.LookbackWindow())
	sb.logger.Trace("Updating validator scores")

	// The denominator is the (last block - first block + 1) of the val score tally window
	denominator := istanbul.GetValScoreTallyLastBlockNumber(epoch, sb.EpochSize()) - istanbul.GetValScoreTallyFirstBlockNumber(epoch, sb.EpochSize(), sb.LookbackWindow()) + 1

	// get all the uptimes for this epoch
	// note(@gakonst): `db` _might_ be possible to be replaced with `sb.db`,
	// but I believe it's a different database handle
	bc := sb.chain.(*core.BlockChain)
	db := bc.GetDatabase()
	uptimes := rawdb.ReadAccumulatedEpochUptime(db, epoch)
	if len(uptimes.Entries) == 0 {
		logger.Error("no accumulated uptimes found, will not update validator scores")
		return errors.New("no accumulated uptimes found, will not update validator scores")
	}

	for i, val := range valSet {
		scoreTally := uptimes.Entries[i].ScoreTally
		logger = logger.New("scoreTally", scoreTally, "denominator", denominator, "index", i, "address", val.Address())
		numerator := big.NewInt(0).Mul(big.NewInt(int64(uptimes.Entries[i].ScoreTally)), params.Fixidity1)
		uptime := big.NewInt(0).Div(numerator, big.NewInt(int64(denominator)))

		if scoreTally > denominator {
			logger.Error("ScoreTally exceeds max possible")
			// 1.0 in fixidity
			uptime = params.Fixidity1
		}

		logger.Trace("Updating validator score", "uptime", uptime)
		err := validators.UpdateValidatorScore(header, state, val.Address(), uptime)
		if err != nil {
			return err
		}
	}
	return nil
}

func (sb *Backend) distributeEpochPayments(header *types.Header, state *state.StateDB, valSet []istanbul.Validator, maxPayment *big.Int) (*big.Int, error) {
	totalEpochPayments := big.NewInt(0)
	for _, val := range valSet {
		sb.logger.Debug("Distributing epoch payment for address", "address", val.Address())
		epochPayment, err := validators.DistributeEpochPayment(header, state, val.Address(), maxPayment)
		if err != nil {
			return totalEpochPayments, nil
		}
		totalEpochPayments.Add(totalEpochPayments, epochPayment)
	}
	return totalEpochPayments, nil
}

func (sb *Backend) distributeEpochRewards(header *types.Header, state *state.StateDB, valSet []istanbul.Validator, maxTotalRewards *big.Int) (*big.Int, error) {
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

	var groups []common.Address
	groupElectedValidator := make(map[common.Address]bool)
	for _, val := range valSet {
		group, err := validators.GetMembershipInLastEpoch(header, state, val.Address())
		if err != nil {
			return totalEpochRewards, err
		}
		if _, ok := groupElectedValidator[group]; !ok {
			groups = append(groups, group)
			sb.logger.Debug("Group elected validator", "group", group.String())
		}
		groupElectedValidator[group] = true
	}

	electionRewards, err := election.DistributeEpochRewards(header, state, groups, maxTotalRewards)
	if err != nil {
		return totalEpochRewards, err
	}
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
