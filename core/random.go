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

	makeCommitmentAbi = `[
    {
      "constant": true,
      "inputs": [
        {
          "name": "randomness",
          "type": "bytes32"
        }
      ],
      "name": "makeCommitment",
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
	revealAndCommitFuncABI, _ = abi.JSON(strings.NewReader(revealAndCommitABI))
	commitmentsFuncABI, _     = abi.JSON(strings.NewReader(commitmentsAbi))
	makeCommitmentFuncABI, _  = abi.JSON(strings.NewReader(makeCommitmentAbi))
	zeroValue                 = common.Big0
	dbRandomnessPrefix        = []byte("commitment-to-randomness")
	emptyReceipt              = types.NewEmptyReceipt()
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
func (r *Random) StoreCommitment(randomness, commitment [32]byte, db *ethdb.Database) error {
	return (*db).Put(commitmentDbLocation(commitment), randomness[:])
}

func (r *Random) Running() bool {
	randomAddress := r.address()
	return randomAddress != nil && *randomAddress != common.ZeroAddress
}

func (r *Random) getLastCommitment(coinbase common.Address, header *types.Header, state *state.StateDB) ([32]byte, error) {
	commitment := [32]byte{}
	_, err := r.iEvmH.MakeStaticCall(*r.address(), commitmentsFuncABI, "commitments", []interface{}{coinbase}, &commitment, gasAmount, header, state)
	return commitment, err
}

func (r *Random) getRandomnessFromCommitment(commitment [32]byte, coinbase common.Address, db *ethdb.Database) ([32]byte, error) {
	randomness := [32]byte{}
	randomnessSlice, err := (*db).Get(commitmentDbLocation(commitment))
	if err != nil {
		log.Debug("Failed to get randomness from database", "err", err)
	} else {
		copy(randomness[:], randomnessSlice)
	}
	return randomness, err
}

// MakeCommitment computes the commitment to a randomness value by calling a
// public static function on the Random contract.
func (r *Random) MakeCommitment(randomness [32]byte, header *types.Header, state *state.StateDB) ([32]byte, error) {
	commitment := [32]byte{}
	_, err := r.iEvmH.MakeStaticCall(*r.address(), makeCommitmentFuncABI, "makeCommitment", []interface{}{randomness}, &commitment, gasAmount, header, state)
	return commitment, err
}

// GetLastRandomness returns up the last randomness we committed to by first
// looking up our last commitment in the smart contract, and then finding the
// corresponding preimage in a (commitment => randomness) mapping we keep in the
// database.
func (r *Random) GetLastRandomness(coinbase common.Address, db *ethdb.Database, header *types.Header, state *state.StateDB) ([32]byte, error) {
	commitment, err := r.getLastCommitment(coinbase, header, state)
	if err != nil {
		log.Debug("Failed to get last commitment", "err", err)
		return [32]byte{}, err
	}

	return r.getRandomnessFromCommitment(commitment, coinbase, db)
}

// RevealAndCommit performs an internal call to the EVM that reveals a
// proposer's previously committed to randomness, and commits new randomness for
// a future block.
func (r *Random) RevealAndCommit(randomness, newCommitment [32]byte, proposer common.Address, header *types.Header, state *state.StateDB) (*types.Receipt, error) {
	args := []interface{}{randomness, newCommitment, proposer}
	_, err := r.iEvmH.MakeCall(*r.address(), revealAndCommitFuncABI, "revealAndCommit", args, nil, gasAmount, zeroValue, header, state)
	if err != nil {
		log.Error("MakeCall failed", "err", err)
		return nil, err
	}

	return emptyReceipt, nil
}
