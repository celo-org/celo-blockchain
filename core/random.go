package core

import (
	"crypto/rand"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
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
	gasAmount = 1000000
)

var (
	revealAndCommitFuncABI, _   = abi.JSON(strings.NewReader(revealAndCommitABI))
	commitmentsFuncABI, _       = abi.JSON(strings.NewReader(commitmentsAbi))
	computeCommitmentFuncABI, _ = abi.JSON(strings.NewReader(computeCommitmentAbi))
	zeroValue                   = common.Big0
	dbRandomnessPrefix          = []byte("commitment-to-randomness")
)

func commitmentDbLocation(commitment common.Hash) []byte {
	return append(dbRandomnessPrefix, commitment.Bytes()...)
}

type Random struct {
	registeredAddresses *RegisteredAddresses
	iEvmH               *InternalEVMHandler
}

func NewRandom(registeredAddresses *RegisteredAddresses, iEvmH *InternalEVMHandler) *Random {
	r := &Random{
		registeredAddresses: registeredAddresses,
		iEvmH:               iEvmH,
	}
	return r
}

func (r *Random) address() *common.Address {
	if r.registeredAddresses != nil {
		randomAddress, err := r.registeredAddresses.GetRegisteredAddressAtCurrentHeader(params.RandomRegistryId)
		if err == ErrSmartContractNotDeployed {
			log.Warn("Registry address lookup failed", "err", err)
		} else if err != nil {
			log.Error(err.Error())
		}
		return randomAddress
	} else {
		return nil
	}
}

func (r *Random) Running() bool {
	randomAddress := r.address()
	return randomAddress != nil && *randomAddress != common.ZeroAddress
}

// GetLastRandomness returns up the last randomness we committed to by first
// looking up our last commitment in the smart contract, and then finding the
// corresponding preimage in a (commitment => randomness) mapping we keep in the
// database.
func (r *Random) GetLastRandomness(coinbase common.Address, db *ethdb.Database, header *types.Header, state *state.StateDB) (common.Hash, error) {
	lastCommitment := common.Hash{}
	_, err := r.iEvmH.MakeStaticCall(*r.address(), commitmentsFuncABI, "commitments", []interface{}{coinbase}, &lastCommitment, gasAmount, header, state)
	if err != nil {
		log.Error("Failed to get last commitment", "err", err)
		return lastCommitment, err
	}

	if (lastCommitment == common.Hash{}) {
		log.Debug("Unable to find last randomness commitment in smart contract")
		return common.Hash{}, nil
	}

	randomness := common.Hash{}
	randomnessSlice, err := (*db).Get(commitmentDbLocation(lastCommitment))
	if err != nil {
		log.Error("Failed to get randomness from database", "commitment", lastCommitment.Hex(), "err", err)
	} else {
		randomness = common.BytesToHash(randomnessSlice)
	}
	return randomness, err
}

// GenerateNewRandomnessAndCommitment generates a new random number and a corresponding commitment.
// The random number is stored in the database, keyed by the corresponding commitment.
func (r *Random) GenerateNewRandomnessAndCommitment(header *types.Header, state *state.StateDB, db *ethdb.Database) (common.Hash, error) {
	commitment := common.Hash{}

	randomBytes := [32]byte{}
	_, err := rand.Read(randomBytes[0:32])
	if err != nil {
		log.Error("Failed to generate randomness", "err", err)
		return commitment, err
	}
	randomness := common.BytesToHash(randomBytes[:])
	// TODO(asa): Make an issue to not have to do this via StaticCall
	_, err = r.iEvmH.MakeStaticCall(*r.address(), computeCommitmentFuncABI, "computeCommitment", []interface{}{randomness}, &commitment, gasAmount, header, state)
	err = (*db).Put(commitmentDbLocation(commitment), randomness[:])
	if err != nil {
		log.Error("Failed to save randomness to the database", "err", err)
	}

	return commitment, err
}

// RevealAndCommit performs an internal call to the EVM that reveals a
// proposer's previously committed to randomness, and commits new randomness for
// a future block.
func (r *Random) RevealAndCommit(randomness, newCommitment common.Hash, proposer common.Address, header *types.Header, state *state.StateDB) error {
	args := []interface{}{randomness, newCommitment, proposer}
	log.Trace("Revealing and committing randomness", "randomness", randomness.Hex(), "commitment", newCommitment.Hex())
	_, err := r.iEvmH.MakeCall(*r.address(), revealAndCommitFuncABI, "revealAndCommit", args, nil, gasAmount, zeroValue, header, state)
	return err
}
