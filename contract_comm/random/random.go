package random

import (
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/contract_comm"
	"github.com/ethereum/go-ethereum/contract_comm/errors"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/params"
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
)

var (
	revealAndCommitFuncABI, _   = abi.JSON(strings.NewReader(revealAndCommitABI))
	commitmentsFuncABI, _       = abi.JSON(strings.NewReader(commitmentsAbi))
	computeCommitmentFuncABI, _ = abi.JSON(strings.NewReader(computeCommitmentAbi))
	zeroValue                   = common.Big0
	dbRandomnessPrefix          = []byte("db-randomness-prefix")
)

func commitmentDbLocation(commitment common.Hash) []byte {
	return append(dbRandomnessPrefix, commitment.Bytes()...)
}

func address() *common.Address {
	randomAddress, err := contract_comm.GetRegisteredAddress(params.RandomRegistryId, nil, nil)
	if err == errors.ErrSmartContractNotDeployed || err == errors.ErrRegistryContractNotDeployed {
		log.Debug("Registry address lookup failed", "err", err, "contract id", params.RandomRegistryId)
	} else if err != nil {
		log.Error(err.Error())
	}
	return randomAddress
}

func IsRunning() bool {
	randomAddress := address()
	return randomAddress != nil && *randomAddress != common.ZeroAddress
}

// GetLastRandomness returns up the last randomness we committed to by first
// looking up our last commitment in the smart contract, and then finding the
// corresponding preimage in a (commitment => randomness) mapping we keep in the
// database.
func GetLastRandomness(coinbase common.Address, db *ethdb.Database, header *types.Header, state vm.StateDB, chain consensus.ChainReader, seed []byte) (common.Hash, error) {
	lastCommitment := common.Hash{}
	_, err := contract_comm.MakeStaticCall(params.RandomRegistryId, commitmentsFuncABI, "commitments", []interface{}{coinbase}, &lastCommitment, params.MaxGasForCommitments, header, state)
	if err != nil {
		log.Error("Failed to get last commitment", "err", err)
		return lastCommitment, err
	}

	if (lastCommitment == common.Hash{}) {
		log.Debug("Unable to find last randomness commitment in smart contract")
		return common.Hash{}, nil
	}

	parentBlockHashBytes, err := (*db).Get(commitmentDbLocation(lastCommitment))
	if err != nil {
		log.Error("Failed to get last block proposed from database", "commitment", lastCommitment.Hex(), "err", err)
		parentBlockHash := header.ParentHash
		for {
			blockHeader := chain.GetHeaderByHash(parentBlockHash)
			parentBlockHash = blockHeader.ParentHash
			if blockHeader.Coinbase == coinbase {
				break
			}
		}
		parentBlockHashBytes = parentBlockHash.Bytes()
	}
	return crypto.Keccak256Hash(append(seed, parentBlockHashBytes...)), nil
}

// GenerateNewRandomnessAndCommitment generates a new random number and a corresponding commitment.
// The random number is stored in the database, keyed by the corresponding commitment.
func GenerateNewRandomnessAndCommitment(header *types.Header, state vm.StateDB, db *ethdb.Database, seed []byte) (common.Hash, error) {
	commitment := common.Hash{}
	randomness := crypto.Keccak256Hash(append(seed, header.ParentHash.Bytes()...))
	// TODO(asa): Make an issue to not have to do this via StaticCall
	_, err := contract_comm.MakeStaticCall(params.RandomRegistryId, computeCommitmentFuncABI, "computeCommitment", []interface{}{randomness}, &commitment, params.MaxGasForComputeCommitment, header, state)
	err = (*db).Put(commitmentDbLocation(commitment), header.ParentHash.Bytes())
	if err != nil {
		log.Error("Failed to save last block parentHash to the database", "err", err)
	}
	return commitment, err
}

// RevealAndCommit performs an internal call to the EVM that reveals a
// proposer's previously committed to randomness, and commits new randomness for
// a future block.
func RevealAndCommit(randomness, newCommitment common.Hash, proposer common.Address, header *types.Header, state vm.StateDB) error {
	args := []interface{}{randomness, newCommitment, proposer}
	log.Trace("Revealing and committing randomness", "randomness", randomness.Hex(), "commitment", newCommitment.Hex())
	_, err := contract_comm.MakeCall(params.RandomRegistryId, revealAndCommitFuncABI, "revealAndCommit", args, nil, params.MaxGasForRevealAndCommit, zeroValue, header, state)
	return err
}
