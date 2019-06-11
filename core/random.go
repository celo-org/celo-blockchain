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
	dbPrefix                  = []byte{0x12, 0x34, 0x56, 0x78, 0x90}
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

func (r *Random) StoreCommitment(randomness [32]byte, sealedRandomness [32]byte, db *ethdb.Database) error {
	dbLocation := append(dbPrefix, sealedRandomness[:]...)
	return (*db).Put(dbLocation, randomness[:])
}

func (r *Random) getLastCommitment(coinbase common.Address, header *types.Header, state *state.StateDB) ([32]byte, error) {
	randomAddress := r.registeredAddresses.GetRegisteredAddress("Random")
	sealedRandomness := [32]byte{}
	_, err := r.iEvmH.MakeStaticCall(*randomAddress, commitmentsFuncABI, "commitments", []interface{}{coinbase}, &sealedRandomness, 100000, header, state)
	return sealedRandomness, err
}

func (r *Random) getRandomnessFromCommitment(sealedRandomness [32]byte, coinbase common.Address, db *ethdb.Database) ([32]byte, error) {
	dbLocation := append(dbPrefix, sealedRandomness[:]...)
	randomness := [32]byte{}
	randomnessSlice, err := (*db).Get(dbLocation)
	if err != nil {
		log.Debug("Failed to get randomness from database")
	} else {
		log.Debug("Got randomness from db", "randomnessSlice", randomnessSlice)
		copy(randomness[:], randomnessSlice)
	}
	return randomness, err
}

func (r *Random) SealRandomness(randomness [32]byte, header *types.Header, state *state.StateDB) ([32]byte, error) {
	randomAddress := r.registeredAddresses.GetRegisteredAddress("Random")
	sealedRandomness := [32]byte{}
	_, err := r.iEvmH.MakeStaticCall(*randomAddress, sealFuncABI, "seal", []interface{}{randomness}, &sealedRandomness, 100000, header, state)
	return sealedRandomness, err
}

func (r *Random) GetLastRandomness(coinbase common.Address, db *ethdb.Database, header *types.Header, state *state.StateDB) ([32]byte, error) {
	sealedRandomness, err := r.getLastCommitment(coinbase, header, state)
	if err != nil {
		log.Error("Failed to get last commitment")
		return [32]byte{}, err
	}

	return r.getRandomnessFromCommitment(sealedRandomness, coinbase, db)
}

func (r *Random) RevealAndCommit(randomness [32]byte, newSealedRandomness [32]byte, proposer common.Address, header *types.Header, state *state.StateDB) error {
	randomAddress := r.registeredAddresses.GetRegisteredAddress("Random")
	args := []interface{}{randomness, newSealedRandomness, proposer}
	_, err := r.iEvmH.MakeCall(*randomAddress, revealAndCommitFuncABI, "revealAndCommit", args, []interface{}{}, gasAmount, zeroValue, header, state)
	if err != nil {
		log.Error("MakeCall failed", "err", err)
		return err
	}

	return nil
}
