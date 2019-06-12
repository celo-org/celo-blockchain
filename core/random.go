package core

import (
	"math/big"
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
				"name": "newSealedRandomness",
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

	sealAbi = `[

    {
      "constant": true,
      "inputs": [
        {
          "name": "randomness",
          "type": "bytes32"
        }
      ],
      "name": "seal",
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
	sealFuncABI, _            = abi.JSON(strings.NewReader(sealAbi))
	zeroValue                 = big.NewInt(0)
	dbRandomnessPrefix        = []byte("commitment-to-randomness")
)

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

// StoreCommitment stores a mapping from `sealedRandomness` to its preimage,
// randomness, for later retrieval in GetLastRandomness.
func (r *Random) StoreCommitment(randomness, sealedRandomness [32]byte, db *ethdb.Database) error {
	return (*db).Put(commitmentDbLocation(sealedRandomness), randomness[:])
}

func (r *Random) randomAddress() common.Address {
	return *r.registeredAddresses.GetRegisteredAddress(params.RandomRegistryId)
}

func (r *Random) getLastCommitment(coinbase common.Address, header *types.Header, state *state.StateDB) ([32]byte, error) {
	sealedRandomness := [32]byte{}
	_, err := r.iEvmH.MakeStaticCall(r.randomAddress(), commitmentsFuncABI, "commitments", []interface{}{coinbase}, &sealedRandomness, gasAmount, header, state)
	return sealedRandomness, err
}

func commitmentDbLocation(sealedRandomness [32]byte) []byte {
	return append(dbRandomnessPrefix, sealedRandomness[:]...)
}

func (r *Random) getRandomnessFromCommitment(sealedRandomness [32]byte, coinbase common.Address, db *ethdb.Database) ([32]byte, error) {
	randomness := [32]byte{}
	randomnessSlice, err := (*db).Get(commitmentDbLocation(sealedRandomness))
	if err != nil {
		log.Debug("Failed to get randomness from database", "err", err)
	} else {
		copy(randomness[:], randomnessSlice)
	}
	return randomness, err
}

// SealRandomness computes the commitment to a randomness value by calling a
// public static function on the Random contract.
func (r *Random) SealRandomness(randomness [32]byte, header *types.Header, state *state.StateDB) ([32]byte, error) {
	sealedRandomness := [32]byte{}
	_, err := r.iEvmH.MakeStaticCall(r.randomAddress(), sealFuncABI, "seal", []interface{}{randomness}, &sealedRandomness, gasAmount, header, state)
	return sealedRandomness, err
}

// GetLastRandomness returns up the last randomness we committed to by first
// looking up our last commitment in the smart contract, and then finding the
// corresponding preimage in a (commitment => randomness) mapping we keep in the
// database.
func (r *Random) GetLastRandomness(coinbase common.Address, db *ethdb.Database, header *types.Header, state *state.StateDB) ([32]byte, error) {
	sealedRandomness, err := r.getLastCommitment(coinbase, header, state)
	if err != nil {
		log.Debug("Failed to get last commitment", "err", err)
		return [32]byte{}, err
	}

	return r.getRandomnessFromCommitment(sealedRandomness, coinbase, db)
}

// RevealAndCommit performs an internal call to the EVM that reveals a
// proposer's previously committed to randomness, and commits new randomness for
// a future block.
func (r *Random) RevealAndCommit(randomness, newSealedRandomness [32]byte, proposer common.Address, header *types.Header, state *state.StateDB) error {
	args := []interface{}{randomness, newSealedRandomness, proposer}
	_, err := r.iEvmH.MakeCall(r.randomAddress(), revealAndCommitFuncABI, "revealAndCommit", args, []interface{}{}, gasAmount, zeroValue, header, state)
	if err != nil {
		log.Error("MakeCall failed", "err", err)
		return err
	}

	return nil
}
