package eth

import (
	"fmt"

	"github.com/celo-org/celo-blockchain/core"
	"github.com/celo-org/celo-blockchain/core/state"
	"github.com/celo-org/celo-blockchain/core/types"
)

// Wraps `stateAtBlock` with the additional Celo-specific parameter afterNextRandomCommit.
// This parameter executes the random commitment at the start of the next block,
// since this is necessary for properly tracing transactions in the next block.
func (eth *Ethereum) celoStateAtBlock(block *types.Block, reexec uint64, base *state.StateDB, checkLive bool, preferDisk bool, afterNextRandomCommit bool) (statedb *state.StateDB, err error) {
	statedb, err = eth.stateAtBlock(block, reexec, base, checkLive, preferDisk)
	if err != nil {
		return nil, err
	}

	if !afterNextRandomCommit {
		return statedb, nil
	}
	// Fetch next block's random commitment
	nextBlockNum := block.NumberU64() + 1
	nextBlock := eth.blockchain.GetBlockByNumber(nextBlockNum)
	if nextBlock == nil {
		return nil, fmt.Errorf("next block %d not found", nextBlockNum)
	}
	vmRunner := eth.blockchain.NewEVMRunner(nextBlock.Header(), statedb)
	err = core.ApplyBlockRandomnessTx(nextBlock, &vmRunner, statedb, eth.blockchain)
	if err != nil {
		return nil, err
	}
	return statedb, nil
}
