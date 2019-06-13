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
	// This is taken from celo-monorepo/packages/protocol/build/<env>/contracts/Rng.json
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
)

type Rng struct {
	registeredAddresses *RegisteredAddresses
	iEvmH               *InternalEVMHandler
}

func NewRng(registeredAddresses *RegisteredAddresses, iEvmH *InternalEVMHandler) *Rng {
	r := &Rng{
		registeredAddresses: registeredAddresses,
		iEvmH:               iEvmH,
	}
	return r
}

// StoreCommitment stores a mapping from `commitment` to its preimage,
// `randomness`, for later retrieval in GetLastRandomness.
func (r *Rng) StoreCommitment(randomness, commitment [32]byte, db *ethdb.Database) error {
	return (*db).Put(commitmentDbLocation(commitment), randomness[:])
}

func (r *Rng) rngAddress() *common.Address {
	if r.registeredAddresses != nil {
		return r.registeredAddresses.GetRegisteredAddress(params.RngRegistryId)
	} else {
		return nil
	}
}

func (r *Rng) RngRunning() bool {
	rngAddress := r.rngAddress()
	return rngAddress != nil && *rngAddress != common.ZeroAddress
}

func (r *Rng) getLastCommitment(coinbase common.Address, header *types.Header, state *state.StateDB) ([32]byte, error) {
	commitment := [32]byte{}
	_, err := r.iEvmH.MakeStaticCall(*r.rngAddress(), commitmentsFuncABI, "commitments", []interface{}{coinbase}, &commitment, gasAmount, header, state)
	return commitment, err
}

func commitmentDbLocation(commitment [32]byte) []byte {
	return append(dbRandomnessPrefix, commitment[:]...)
}

func (r *Rng) getRandomnessFromCommitment(commitment [32]byte, coinbase common.Address, db *ethdb.Database) ([32]byte, error) {
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
func (r *Rng) MakeCommitment(randomness [32]byte, header *types.Header, state *state.StateDB) ([32]byte, error) {
	commitment := [32]byte{}
	_, err := r.iEvmH.MakeStaticCall(*r.rngAddress(), makeCommitmentFuncABI, "makeCommitment", []interface{}{randomness}, &commitment, gasAmount, header, state)
	return commitment, err
}

// GetLastRandomness returns up the last randomness we committed to by first
// looking up our last commitment in the smart contract, and then finding the
// corresponding preimage in a (commitment => randomness) mapping we keep in the
// database.
func (r *Rng) GetLastRandomness(coinbase common.Address, db *ethdb.Database, header *types.Header, state *state.StateDB) ([32]byte, error) {
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
func (r *Rng) RevealAndCommit(randomness, newCommitment [32]byte, proposer common.Address, header *types.Header, state *state.StateDB) error {
	args := []interface{}{randomness, newCommitment, proposer}
	_, err := r.iEvmH.MakeCall(*r.rngAddress(), revealAndCommitFuncABI, "revealAndCommit", args, nil, gasAmount, zeroValue, header, state)
	if err != nil {
		log.Error("MakeCall failed", "err", err)
		return err
	}

	return nil
}
