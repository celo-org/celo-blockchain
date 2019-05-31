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
)

var (
	revealAndCommitFuncABI, _ = abi.JSON(strings.NewReader(revealAndCommitABI))
)

type Random struct {
	iEvmH *InternalEVMHandler
}

func NewRandom(iEvmH *InternalEVMHandler) *Random {
	r := &Random{
		iEvmH: iEvmH,
	}
	return r
}

func (r *Random) RevealAndCommit(randomness [32]byte, newSealedRandomness [32]byte, proposer common.Address, address common.Address, header *types.Header, state *state.StateDB) error {
	log.Debug("Calling to revealAndCommit", "randomness", randomness, "newSealedRandomness", newSealedRandomness, "proposer", proposer)
	_, err := r.iEvmH.MakeCall(address, revealAndCommitFuncABI, "revealAndCommit", []interface{}{randomness, newSealedRandomness, proposer}, []interface{}{}, 100000, big.NewInt(0), header, state)
	if err != nil {
		log.Error("MakeCall failed", "err", err)
		return err
	}

	return nil
}
