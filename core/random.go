package core

import (
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
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

	gasAmount = 1000000
)

var (
	revealAndCommitFuncABI, _ = abi.JSON(strings.NewReader(revealAndCommitABI))
	zeroValue                 = big.NewInt(0)
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
