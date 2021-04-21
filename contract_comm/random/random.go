package random

import (
	"math/big"
	"strings"

	"github.com/celo-org/celo-blockchain/accounts/abi"
	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/common/hexutil"
	"github.com/celo-org/celo-blockchain/contract_comm"
	"github.com/celo-org/celo-blockchain/contracts"
	"github.com/celo-org/celo-blockchain/core/types"
	"github.com/celo-org/celo-blockchain/core/vm"
	"github.com/celo-org/celo-blockchain/log"
	"github.com/celo-org/celo-blockchain/params"
)

const (
	// This is taken from celo-monorepo/packages/protocol/build/<env>/contracts/Random.json
	revealAndCommitABI = `[
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
	}
]`
	commitmentsAbi = `[
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
	}
]`
	computeCommitmentAbi = `[
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
    }
]`
	randomAbi = `[
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
    }
]`

	historicRandomAbi = `[
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
	revealAndCommitFuncABI, _   = abi.JSON(strings.NewReader(revealAndCommitABI))
	commitmentsFuncABI, _       = abi.JSON(strings.NewReader(commitmentsAbi))
	computeCommitmentFuncABI, _ = abi.JSON(strings.NewReader(computeCommitmentAbi))
	randomFuncABI, _            = abi.JSON(strings.NewReader(randomAbi))
	historicRandomFuncABI, _    = abi.JSON(strings.NewReader(historicRandomAbi))
	zeroValue                   = common.Big0
)

func address() common.Address {
	randomAddress, err := contract_comm.GetRegisteredAddress(params.RandomRegistryId, nil, nil)
	if err == contracts.ErrSmartContractNotDeployed || err == contracts.ErrRegistryContractNotDeployed {
		log.Debug("Registry address lookup failed", "err", err, "contract", hexutil.Encode(params.RandomRegistryId[:]))
	} else if err != nil {
		log.Error(err.Error())
	}
	return randomAddress
}

func IsRunning() bool {
	randomAddress := address()
	return randomAddress != common.ZeroAddress
}

// GetLastCommitment returns up the last commitment in the smart contract
func GetLastCommitment(validator common.Address, header *types.Header, state vm.StateDB) (common.Hash, error) {
	lastCommitment := common.Hash{}
	err := contract_comm.MakeStaticCall(params.RandomRegistryId, commitmentsFuncABI, "commitments", []interface{}{validator}, &lastCommitment, params.MaxGasForCommitments, header, state)
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
func ComputeCommitment(header *types.Header, state vm.StateDB, randomness common.Hash) (common.Hash, error) {
	commitment := common.Hash{}
	// TODO(asa): Make an issue to not have to do this via StaticCall
	err := contract_comm.MakeStaticCall(params.RandomRegistryId, computeCommitmentFuncABI, "computeCommitment", []interface{}{randomness}, &commitment, params.MaxGasForComputeCommitment, header, state)
	if err != nil {
		log.Error("Failed to call computeCommitment()", "err", err)
		return common.Hash{}, err
	}

	return commitment, err
}

// RevealAndCommit performs an internal call to the EVM that reveals a
// proposer's previously committed to randomness, and commits new randomness for
// a future block.
func RevealAndCommit(randomness, newCommitment common.Hash, proposer common.Address, header *types.Header, state vm.StateDB) error {
	args := []interface{}{randomness, newCommitment, proposer}
	log.Trace("Revealing and committing randomness", "randomness", randomness.Hex(), "commitment", newCommitment.Hex())
	err := contract_comm.MakeCall(params.RandomRegistryId, revealAndCommitFuncABI, "revealAndCommit", args, nil, params.MaxGasForRevealAndCommit, zeroValue, header, state, true)
	return err
}

// Random performs an internal call to the EVM to retrieve the current randomness from the official Random contract.
func Random(header *types.Header, state vm.StateDB) (common.Hash, error) {
	randomness := common.Hash{}
	err := contract_comm.MakeStaticCall(params.RandomRegistryId, randomFuncABI, "random", []interface{}{}, &randomness, params.MaxGasForBlockRandomness, header, state)
	return randomness, err
}

func BlockRandomness(header *types.Header, state vm.StateDB, blockNumber uint64) (common.Hash, error) {
	randomness := common.Hash{}
	err := contract_comm.MakeStaticCall(params.RandomRegistryId, historicRandomFuncABI, "getBlockRandomness", []interface{}{big.NewInt(int64(blockNumber))}, &randomness, params.MaxGasForBlockRandomness, header, state)
	return randomness, err
}
