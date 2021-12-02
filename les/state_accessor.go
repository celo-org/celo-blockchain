// Copyright 2021 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package les

import (
	"context"
	"errors"
	"fmt"

	"github.com/celo-org/celo-blockchain/core"
	"github.com/celo-org/celo-blockchain/core/state"
	"github.com/celo-org/celo-blockchain/core/types"
	"github.com/celo-org/celo-blockchain/core/vm"
	"github.com/celo-org/celo-blockchain/core/vm/vmcontext"
	"github.com/celo-org/celo-blockchain/light"
)

// stateAtBlock retrieves the state database associated with a certain block.
func (leth *LightEthereum) stateAtBlock(ctx context.Context, block *types.Block, reexec uint64) (*state.StateDB, error) {
	return light.NewState(ctx, block.Header(), leth.odr), nil
}

// stateAtTransaction returns the execution environment of a certain transaction.
func (leth *LightEthereum) stateAtTransaction(ctx context.Context, block *types.Block, txIndex int, reexec uint64) (core.Message, vm.BlockContext, vm.EVMRunner, *state.StateDB, error) {
	// Short circuit if it's genesis block.
	if block.NumberU64() == 0 {
		return nil, vm.BlockContext{}, nil, nil, errors.New("no transaction in genesis")
	}
	// Create the parent state database
	parent, err := leth.blockchain.GetBlock(ctx, block.ParentHash(), block.NumberU64()-1)
	if err != nil {
		return nil, vm.BlockContext{}, nil, nil, err
	}
	statedb, err := leth.stateAtBlock(ctx, parent, reexec)
	if err != nil {
		return nil, vm.BlockContext{}, nil, nil, err
	}
	if txIndex == 0 && len(block.Transactions()) == 0 {
		return nil, vm.BlockContext{}, nil, statedb, nil
	}
	// Create SysContractCallCtx
	var sysCtx *core.SysContractCallCtx
	if leth.chainConfig.IsEspresso(block.Number()) {
		vmRunner := vmcontext.NewEVMRunner(leth.blockchain, block.Header(), statedb.Copy())
		sysCtx = core.NewSysContractCallCtx(vmRunner)
	}
	// Recompute transactions up to the target index.
	signer := types.MakeSigner(leth.blockchain.Config(), block.Number())
	for idx, tx := range block.Transactions() {
		// Assemble the transaction call message and return if the requested offset
		msg, _ := tx.AsMessage(signer, sysCtx.GetGasPriceMinimum(tx.FeeCurrency()))
		txContext := core.NewEVMTxContext(msg)
		context := core.NewEVMBlockContext(block.Header(), leth.blockchain, nil)
		statedb.Prepare(tx.Hash(), idx)
		vmRunner := leth.blockchain.NewEVMRunner(block.Header(), statedb)
		if idx == txIndex {
			return msg, context, vmRunner, statedb, nil
		}
		// Not yet the searched for transaction, execute on top of the current state
		vmenv := vm.NewEVM(context, txContext, statedb, leth.blockchain.Config(), vm.Config{})
		if _, err := core.ApplyMessage(vmenv, msg, new(core.GasPool).AddGas(tx.Gas()), vmRunner, sysCtx); err != nil {
			return nil, vm.BlockContext{}, nil, nil, fmt.Errorf("transaction %#x failed: %v", tx.Hash(), err)
		}
		// Ensure any modifications are committed to the state
		// Only delete empty objects if EIP158/161 (a.k.a Spurious Dragon) is in effect
		statedb.Finalise(vmenv.ChainConfig().IsEIP158(block.Number()))
	}
	return nil, vm.BlockContext{}, nil, nil, fmt.Errorf("transaction index %d out of range for block %#x", txIndex, block.Hash())
}
