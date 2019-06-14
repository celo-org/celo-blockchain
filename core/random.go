package core

import (
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

func commitmentDbLocation(commitment [32]byte) []byte {
	return append(dbRandomnessPrefix, commitment[:]...)
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
		return r.registeredAddresses.GetRegisteredAddress(params.RandomRegistryId)
	} else {
		return nil
	}
}

// StoreCommitment stores a mapping from `commitment` to its preimage,
// `randomness`, for later retrieval in GetLastRandomness.
func (r *Random) StoreCommitment(randomness, commitment common.Hash, db *ethdb.Database) error {
	return (*db).Put(commitmentDbLocation(commitment), randomness[:])
}

func (r *Random) Running() bool {
	randomAddress := r.address()
	return randomAddress != nil && *randomAddress != common.ZeroAddress
}

func (r *Random) getLastCommitment(coinbase common.Address, header *types.Header, state *state.StateDB) (common.Hash, error) {
	commitment := common.Hash{}
	_, err := r.iEvmH.MakeStaticCall(*r.address(), commitmentsFuncABI, "commitments", []interface{}{coinbase}, &commitment, gasAmount, header, state)
	return commitment, err
}

func (r *Random) getRandomnessFromCommitment(commitment common.Hash, coinbase common.Address, db *ethdb.Database) (common.Hash, error) {
	if (commitment == common.Hash{}) {
		return common.Hash{}, nil
	}

	randomness := common.Hash{}
	randomnessSlice, err := (*db).Get(commitmentDbLocation(commitment))
	if err != nil {
		log.Debug("Failed to get randomness from database", "err", err)
	} else {
		randomness = common.BytesToHash(randomnessSlice)
	}
	return randomness, err
}

// ComputeCommitment computes the commitment to a randomness value by calling a
// public static function on the Random contract.
func (r *Random) ComputeCommitment(randomness common.Hash, header *types.Header, state *state.StateDB) (common.Hash, error) {
	commitment := common.Hash{}
	_, err := r.iEvmH.MakeStaticCall(*r.address(), computeCommitmentFuncABI, "computeCommitment", []interface{}{randomness}, &commitment, gasAmount, header, state)
	return commitment, err
}

// GetLastRandomness returns up the last randomness we committed to by first
// looking up our last commitment in the smart contract, and then finding the
// corresponding preimage in a (commitment => randomness) mapping we keep in the
// database.
func (r *Random) GetLastRandomness(coinbase common.Address, db *ethdb.Database, header *types.Header, state *state.StateDB) (common.Hash, error) {
	commitment, err := r.getLastCommitment(coinbase, header, state)
	if err != nil {
		log.Debug("Failed to get last commitment", "err", err)
		return common.Hash{}, err
	}

	return r.getRandomnessFromCommitment(commitment, coinbase, db)
}

func emptyReceipt() *types.Receipt {
	receipt := types.NewReceipt([]byte{}, false, 0)
	receipt.GasUsed = 0
	receipt.Bloom = types.CreateBloom(types.Receipts{receipt})

	return receipt
}

// RevealAndCommit performs an internal call to the EVM that reveals a
// proposer's previously committed to randomness, and commits new randomness for
// a future block.
func (r *Random) RevealAndCommit(randomness, newCommitment common.Hash, proposer common.Address, header *types.Header, state *state.StateDB) (*types.Receipt, error) {
	args := []interface{}{randomness, newCommitment, proposer}
	_, err := r.iEvmH.MakeCall(*r.address(), revealAndCommitFuncABI, "revealAndCommit", args, nil, gasAmount, zeroValue, header, state)
	if err != nil {
		log.Error("MakeCall failed", "err", err)
		return nil, err
	}

	return emptyReceipt(), nil
}
