// Copyright 2015 The go-ethereum Authors
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

package core

import (
	"fmt"
	"math/big"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/consensus"
	"github.com/celo-org/celo-blockchain/consensus/misc"
	"github.com/celo-org/celo-blockchain/contracts/blockchain_parameters"
	"github.com/celo-org/celo-blockchain/contracts/random"
	"github.com/celo-org/celo-blockchain/core/state"
	"github.com/celo-org/celo-blockchain/core/types"
	"github.com/celo-org/celo-blockchain/core/vm"
	"github.com/celo-org/celo-blockchain/crypto"
	"github.com/celo-org/celo-blockchain/params"
)

// StateProcessor is a basic Processor, which takes care of transitioning
// state from one point to another.
//
// StateProcessor implements Processor.
type StateProcessor struct {
	config *params.ChainConfig // Chain configuration options
	bc     *BlockChain         // Canonical block chain
	engine consensus.Engine    // Consensus engine used for block rewards
}

// NewStateProcessor initialises a new StateProcessor.
func NewStateProcessor(config *params.ChainConfig, bc *BlockChain, engine consensus.Engine) *StateProcessor {
	return &StateProcessor{
		config: config,
		bc:     bc,
		engine: engine,
	}
}

// Process processes the state changes according to the Ethereum rules by running
// the transaction messages using the statedb and applying any rewards to the processor (coinbase).
//
// Process returns the receipts and logs accumulated during the process and
// returns the amount of gas that was used in the process. If any of the
// transactions failed to execute due to insufficient gas it will return an error.
func (p *StateProcessor) Process(block *types.Block, statedb *state.StateDB, cfg vm.Config) (types.Receipts, []*types.Log, uint64, error) {
	var (
<<<<<<< HEAD
		receipts types.Receipts
		usedGas  = new(uint64)
		header   = block.Header()
		allLogs  []*types.Log
		vmRunner = p.bc.NewEVMRunner(block.Header(), statedb)
		gp       = new(GasPool).AddGas(blockchain_parameters.GetBlockGasLimitOrDefault(vmRunner))
||||||| e78727290
		receipts types.Receipts
		usedGas  = new(uint64)
		header   = block.Header()
		allLogs  []*types.Log
		gp       = new(GasPool).AddGas(block.GasLimit())
=======
		receipts    types.Receipts
		usedGas     = new(uint64)
		header      = block.Header()
		blockHash   = block.Hash()
		blockNumber = block.Number()
		allLogs     []*types.Log
		gp          = new(GasPool).AddGas(block.GasLimit())
>>>>>>> v1.10.7
	)
	// Mutate the block and state according to any hard-fork specs
	if p.config.DAOForkSupport && p.config.DAOForkBlock != nil && p.config.DAOForkBlock.Cmp(block.Number()) == 0 {
		misc.ApplyDAOHardFork(statedb)
	}
	if random.IsRunning(vmRunner) {
		author, err := p.bc.Engine().Author(header)
		if err != nil {
			return nil, nil, 0, err
		}

		err = random.RevealAndCommit(vmRunner, block.Randomness().Revealed, block.Randomness().Committed, author)
		if err != nil {
			return nil, nil, 0, err
		}
		// always true (EIP158)
		statedb.IntermediateRoot(true)
	}

	blockContext := NewEVMBlockContext(header, p.bc, nil)
	vmenv := vm.NewEVM(blockContext, vm.TxContext{}, statedb, p.config, cfg)
	// Iterate over and process the individual transactions
	for i, tx := range block.Transactions() {
		msg, err := tx.AsMessage(types.MakeSigner(p.config, header.Number), header.BaseFee)
		if err != nil {
			return nil, nil, 0, fmt.Errorf("could not apply tx %d [%v]: %w", i, tx.Hash().Hex(), err)
		}
<<<<<<< HEAD
		statedb.Prepare(tx.Hash(), block.Hash(), i)
		receipt, err := applyTransaction(msg, p.config, p.bc, nil, gp, statedb, header, tx, usedGas, vmenv, vmRunner)
||||||| e78727290
		statedb.Prepare(tx.Hash(), block.Hash(), i)
		receipt, err := applyTransaction(msg, p.config, p.bc, nil, gp, statedb, header, tx, usedGas, vmenv)
=======
		statedb.Prepare(tx.Hash(), i)
		receipt, err := applyTransaction(msg, p.config, p.bc, nil, gp, statedb, blockNumber, blockHash, tx, usedGas, vmenv)
>>>>>>> v1.10.7
		if err != nil {
			return nil, nil, 0, fmt.Errorf("could not apply tx %d [%v]: %w", i, tx.Hash().Hex(), err)
		}
		receipts = append(receipts, receipt)
		allLogs = append(allLogs, receipt.Logs...)
	}
	// Finalize the block, applying any consensus engine specific extras (e.g. block rewards)
	statedb.Prepare(common.Hash{}, block.Hash(), len(block.Transactions()))
	p.engine.Finalize(p.bc, header, statedb, block.Transactions())

	if len(statedb.GetLogs(common.Hash{})) > 0 {
		receipt := types.NewReceipt(nil, false, 0)
		receipt.Logs = statedb.GetLogs(common.Hash{})
		receipt.Bloom = types.CreateBloom(types.Receipts{receipt})
		for i := range receipt.Logs {
			receipt.Logs[i].TxIndex = uint(len(receipts))
			receipt.Logs[i].TxHash = block.Hash()
		}
		receipts = append(receipts, receipt)
	}

	return receipts, allLogs, *usedGas, nil
}

<<<<<<< HEAD
func applyTransaction(msg types.Message, config *params.ChainConfig, bc ChainContext, txFeeRecipient *common.Address, gp *GasPool, statedb *state.StateDB, header *types.Header, tx *types.Transaction, usedGas *uint64, evm *vm.EVM, vmRunner vm.EVMRunner) (*types.Receipt, error) {
	if config.IsDonut(header.Number) && !tx.Protected() {
		return nil, ErrUnprotectedTransaction
	}

	// Create a new context to be used in the EVM environment
||||||| e78727290
func applyTransaction(msg types.Message, config *params.ChainConfig, bc ChainContext, author *common.Address, gp *GasPool, statedb *state.StateDB, header *types.Header, tx *types.Transaction, usedGas *uint64, evm *vm.EVM) (*types.Receipt, error) {
	// Create a new context to be used in the EVM environment
=======
func applyTransaction(msg types.Message, config *params.ChainConfig, bc ChainContext, author *common.Address, gp *GasPool, statedb *state.StateDB, blockNumber *big.Int, blockHash common.Hash, tx *types.Transaction, usedGas *uint64, evm *vm.EVM) (*types.Receipt, error) {
	// Create a new context to be used in the EVM environment.
>>>>>>> v1.10.7
	txContext := NewEVMTxContext(msg)
<<<<<<< HEAD

	// Update the evm with the new transaction context.
||||||| e78727290
	// Add addresses to access list if applicable
	if config.IsYoloV2(header.Number) {
		statedb.AddAddressToAccessList(msg.From())
		if dst := msg.To(); dst != nil {
			statedb.AddAddressToAccessList(*dst)
			// If it's a create-tx, the destination will be added inside evm.create
		}
		for _, addr := range evm.ActivePrecompiles() {
			statedb.AddAddressToAccessList(addr)
		}
	}

	// Update the evm with the new transaction context.
=======
>>>>>>> v1.10.7
	evm.Reset(txContext, statedb)
<<<<<<< HEAD
	// Apply the transaction to the current state (included in the env)
	result, err := ApplyMessage(evm, msg, gp, vmRunner)
||||||| e78727290
	// Apply the transaction to the current state (included in the env)
	result, err := ApplyMessage(evm, msg, gp)
=======

	// Apply the transaction to the current state (included in the env).
	result, err := ApplyMessage(evm, msg, gp)
>>>>>>> v1.10.7
	if err != nil {
		return nil, err
	}

	// Update the state with pending changes.
	var root []byte
	if config.IsByzantium(blockNumber) {
		statedb.Finalise(true)
	} else {
		root = statedb.IntermediateRoot(config.IsEIP158(blockNumber)).Bytes()
	}
	*usedGas += result.UsedGas

	// Create a new receipt for the transaction, storing the intermediate root and gas used
	// by the tx.
	receipt := &types.Receipt{Type: tx.Type(), PostState: root, CumulativeGasUsed: *usedGas}
	if result.Failed() {
		receipt.Status = types.ReceiptStatusFailed
	} else {
		receipt.Status = types.ReceiptStatusSuccessful
	}
	receipt.TxHash = tx.Hash()
	receipt.GasUsed = result.UsedGas

	// If the transaction created a contract, store the creation address in the receipt.
	if msg.To() == nil {
		receipt.ContractAddress = crypto.CreateAddress(evm.TxContext.Origin, tx.Nonce())
	}

	// Set the receipt logs and create the bloom filter.
	receipt.Logs = statedb.GetLogs(tx.Hash(), blockHash)
	receipt.Bloom = types.CreateBloom(types.Receipts{receipt})
	receipt.BlockHash = blockHash
	receipt.BlockNumber = blockNumber
	receipt.TransactionIndex = uint(statedb.TxIndex())
	return receipt, err
}

// ApplyTransaction attempts to apply a transaction to the given state database
// and uses the input parameters for its environment. It returns the receipt
// for the transaction, gas used and an error if the transaction failed,
// indicating the block was invalid.
<<<<<<< HEAD
func ApplyTransaction(config *params.ChainConfig, bc ChainContext, txFeeRecipient *common.Address, gp *GasPool, statedb *state.StateDB, header *types.Header, tx *types.Transaction, usedGas *uint64, cfg vm.Config, vmRunner vm.EVMRunner) (*types.Receipt, error) {
	msg, err := tx.AsMessage(types.MakeSigner(config, header.Number))
||||||| e78727290
func ApplyTransaction(config *params.ChainConfig, bc ChainContext, author *common.Address, gp *GasPool, statedb *state.StateDB, header *types.Header, tx *types.Transaction, usedGas *uint64, cfg vm.Config) (*types.Receipt, error) {
	msg, err := tx.AsMessage(types.MakeSigner(config, header.Number))
=======
func ApplyTransaction(config *params.ChainConfig, bc ChainContext, author *common.Address, gp *GasPool, statedb *state.StateDB, header *types.Header, tx *types.Transaction, usedGas *uint64, cfg vm.Config) (*types.Receipt, error) {
	msg, err := tx.AsMessage(types.MakeSigner(config, header.Number), header.BaseFee)
>>>>>>> v1.10.7
	if err != nil {
		return nil, err
	}
	// Create a new context to be used in the EVM environment
	blockContext := NewEVMBlockContext(header, bc, txFeeRecipient)
	vmenv := vm.NewEVM(blockContext, vm.TxContext{}, statedb, config, cfg)
<<<<<<< HEAD
	return applyTransaction(msg, config, bc, txFeeRecipient, gp, statedb, header, tx, usedGas, vmenv, vmRunner)
||||||| e78727290
	return applyTransaction(msg, config, bc, author, gp, statedb, header, tx, usedGas, vmenv)
=======
	return applyTransaction(msg, config, bc, author, gp, statedb, header.Number, header.Hash(), tx, usedGas, vmenv)
>>>>>>> v1.10.7
}
