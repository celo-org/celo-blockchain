package random

import (
	"math/big"
	"strings"

	"github.com/celo-org/celo-blockchain/accounts/abi"
	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/common/hexutil"
	"github.com/celo-org/celo-blockchain/contract_comm/errors"
	"github.com/celo-org/celo-blockchain/contracts"
	"github.com/celo-org/celo-blockchain/core/vm"
	"github.com/celo-org/celo-blockchain/log"
	"github.com/celo-org/celo-blockchain/params"
)

const (
	// This is taken from celo-monorepo/packages/protocol/build/<env>/contracts/Random.json
	randomABI = `[
		{
			"constant": false,
			"inputs": [
				{
					"name": "randomness",
					"type": "bytes32"
				},
				{
					"name": "newCommitment",
					"type": "bytes32"
				},
				{
					"name": "proposer",
					"type": "address"
				}
			],
			"name": "revealAndCommit",
			"outputs": [],
			"payable": false,
			"stateMutability": "nonpayable",
			"type": "function"
		},
		{
			"constant": true,
			"inputs": [
				{
					"name": "",
					"type": "address"
				}
			],
			"name": "commitments",
			"outputs": [
				{
					"name": "",
					"type": "bytes32"
				}
			],
			"payable": false,
			"stateMutability": "view",
			"type": "function"
		},
		{
			"constant": true,
			"inputs": [
				{
					"name": "randomness",
					"type": "bytes32"
				}
			],
			"name": "computeCommitment",
			"outputs": [
				{
					"name": "",
					"type": "bytes32"
				}
			],
			"payable": false,
			"stateMutability": "view",
			"type": "function"
		},
		{
			"constant": true,
			"inputs": [],
			"name": "random",
			"outputs": [
				{
					"name": "",
					"type": "bytes32"
				}
			],
			"payable": false,
			"stateMutability": "view",
			"type": "function"
		},
		{
			"constant": true,
			"inputs": [
				{
					"name": "blockNumber",
					"type": "uint256"
				}
			],
			"name": "getBlockRandomness",
			"outputs": [
				{
					"name": "",
					"type": "bytes32"
				}
			],
			"payable": false,
			"stateMutability": "view",
			"type": "function"
		}
	]`
)

var (
	revealAndCommitMethod    *contracts.BoundMethod
	commitmentsMethod        *contracts.BoundMethod
	computeCommitmentMethod  *contracts.BoundMethod
	randomMethod             *contracts.BoundMethod
	getBlockRandomnessMethod *contracts.BoundMethod
)

func init() {
	randomAbi, err := abi.JSON(strings.NewReader(randomABI))
	if err != nil {
		panic(err)
	}

	revealAndCommitMethod = contracts.NewRegisteredContractMethod(params.RandomRegistryId, &randomAbi, "revealAndCommit", params.MaxGasForRevealAndCommit)
	commitmentsMethod = contracts.NewRegisteredContractMethod(params.RandomRegistryId, &randomAbi, "commitments", params.MaxGasForCommitments)
	computeCommitmentMethod = contracts.NewRegisteredContractMethod(params.RandomRegistryId, &randomAbi, "computeCommitment", params.MaxGasForComputeCommitment)
	randomMethod = contracts.NewRegisteredContractMethod(params.RandomRegistryId, &randomAbi, "random", params.MaxGasForBlockRandomness)
	getBlockRandomnessMethod = contracts.NewRegisteredContractMethod(params.RandomRegistryId, &randomAbi, "getBlockRandomness", params.MaxGasForBlockRandomness)
}

func IsRunning(vmRunner vm.EVMRunner) bool {
	randomAddress, err := contracts.GetRegisteredAddress(vmRunner, params.RandomRegistryId)

	if err == errors.ErrSmartContractNotDeployed || err == errors.ErrRegistryContractNotDeployed {
		log.Debug("Registry address lookup failed", "err", err, "contract", hexutil.Encode(params.RandomRegistryId[:]))
	} else if err != nil {
		log.Error(err.Error())
	}

	return err != nil && randomAddress != common.ZeroAddress
}

// GetLastCommitment returns up the last commitment in the smart contract
func GetLastCommitment(vmRunner vm.EVMRunner, validator common.Address) (common.Hash, error) {
	lastCommitment := common.Hash{}
	err := commitmentsMethod.Query(vmRunner, &lastCommitment, validator)
	if err != nil {
		log.Error("Failed to get last commitment", "err", err)
		return lastCommitment, err
	}

	if (lastCommitment == common.Hash{}) {
		log.Debug("Unable to find last randomness commitment in smart contract")
	}

	return lastCommitment, nil
}

// ComputeCommitment calulcates the commitment for a given randomness.
func ComputeCommitment(vmRunner vm.EVMRunner, randomness common.Hash) (common.Hash, error) {
	commitment := common.Hash{}
	// TODO(asa): Make an issue to not have to do this via StaticCall
	err := computeCommitmentMethod.Query(vmRunner, &commitment, randomness)
	if err != nil {
		log.Error("Failed to call computeCommitment()", "err", err)
		return common.Hash{}, err
	}

	return commitment, err
}

// RevealAndCommit performs an internal call to the EVM that reveals a
// proposer's previously committed to randomness, and commits new randomness for
// a future block.
func RevealAndCommit(vmRunner vm.EVMRunner, randomness, newCommitment common.Hash, proposer common.Address) error {

	log.Trace("Revealing and committing randomness", "randomness", randomness.Hex(), "commitment", newCommitment.Hex())
	err := revealAndCommitMethod.Execute(vmRunner, nil, common.Big0, randomness, newCommitment, proposer)

	// FIXME this should be done by the caller, remove from here after
	if err == nil {
		vmRunner.GetStateDB().Finalise(true)
	}
	return err
}

// Random performs an internal call to the EVM to retrieve the current randomness from the official Random contract.
func Random(vmRunner vm.EVMRunner) (common.Hash, error) {
	randomness := common.Hash{}
	err := randomMethod.Query(vmRunner, &randomness)
	return randomness, err
}

func BlockRandomness(vmRunner vm.EVMRunner, blockNumber uint64) (common.Hash, error) {
	randomness := common.Hash{}
	err := getBlockRandomnessMethod.Query(vmRunner, &randomness, big.NewInt(int64(blockNumber)))
	return randomness, err
}
