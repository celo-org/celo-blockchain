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
		receipts    types.Receipts
		usedGas     = new(uint64)
		header      = block.Header()
		blockHash   = block.Hash()
		blockNumber = block.Number()
		allLogs     []*types.Log
		vmRunner    = p.bc.NewEVMRunner(block.Header(), statedb)
		gp          = new(GasPool).AddGas(blockchain_parameters.GetBlockGasLimitOrDefault(vmRunner))
	)

	// This checks that the baseFee and the gaLimit are correct.
	// As we need state to address this, the header Verify is not useful because
	// the client not necessary will have the state of the parent.
	if p.config.IsGingerbread(header.Number) {
		parentHeader := p.bc.GetHeaderByHash(block.ParentHash())
		// Needs the baseFee at the final state of the last block
		parentVmRunner := p.bc.NewEVMRunner(parentHeader, statedb.Copy())
		// Verifies both the gasLimit and the baseFee of the header are correct
		err := misc.VerifyEip1559Header(p.config, parentHeader, header, parentVmRunner)
		if err != nil {
			return nil, nil, 0, err
		}
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

	var (
		baseFee *big.Int
		sysCtx  *SysContractCallCtx
	)
	if p.config.IsEspresso(blockNumber) {
		sysCtx = NewSysContractCallCtx(header, statedb, p.bc)
		if p.config.FakeBaseFee != nil {
			sysCtx = MockSysContractCallCtx(p.bc.Config().FakeBaseFee)
		}
	}
	blockContext := NewEVMBlockContext(header, p.bc, nil)
	vmenv := vm.NewEVM(blockContext, vm.TxContext{}, statedb, p.config, cfg)
	// Iterate over and process the individual transactions
	for i, tx := range block.Transactions() {
		if p.config.IsEspresso(header.Number) {
			baseFee = sysCtx.GetGasPriceMinimum(tx.FeeCurrency())
		}
		msg, err := tx.AsMessage(types.MakeSigner(p.config, header.Number), baseFee)
		if err != nil {
			return nil, nil, 0, fmt.Errorf("could not apply tx %d [%v]: %w", i, tx.Hash().Hex(), err)
		}
		statedb.Prepare(tx.Hash(), i)
		receipt, err := applyTransaction(msg, p.config, gp, statedb, blockNumber, blockHash, tx, usedGas, vmenv, vmRunner, sysCtx)
		if err != nil {
			return nil, nil, 0, fmt.Errorf("could not apply tx %d [%v]: %w", i, tx.Hash().Hex(), err)
		}
		receipts = append(receipts, receipt)
		allLogs = append(allLogs, receipt.Logs...)
	}
	// Finalize the block, applying any consensus engine specific extras (e.g. block rewards)
	p.engine.Finalize(p.bc, header, statedb, block.Transactions())

	// Add the block receipt with logs from the non-transaction core contract calls (if there were any)
	receipts = AddBlockReceipt(receipts, statedb, block.Hash())

	return receipts, allLogs, *usedGas, nil
}

func applyTransaction(msg types.Message, config *params.ChainConfig, gp *GasPool, statedb *state.StateDB, blockNumber *big.Int, blockHash common.Hash, tx *types.Transaction, usedGas *uint64, evm *vm.EVM, vmRunner vm.EVMRunner, sysCtx *SysContractCallCtx) (*types.Receipt, error) {
	if config.IsDonut(blockNumber) && !config.IsEspresso(blockNumber) && !tx.Protected() {
		return nil, ErrUnprotectedTransaction
	}

	// CIP 57 deprecates full node incentives
	// Check that neither `GatewayFeeRecipient` nor `GatewayFee` are set, otherwise reject the transaction
	if config.IsGingerbread(blockNumber) {
		gatewayFeeSet := !(msg.GatewayFee() == nil || msg.GatewayFee().Cmp(common.Big0) == 0)
		if msg.GatewayFeeRecipient() != nil || gatewayFeeSet {
			return nil, ErrGatewayFeeDeprecated
		}
	}

	// Create a new context to be used in the EVM environment
	txContext := NewEVMTxContext(msg)
	evm.Reset(txContext, statedb)

	// Apply the transaction to the current state (included in the env).
	result, err := ApplyMessage(evm, msg, gp, vmRunner, sysCtx)
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
func ApplyTransaction(config *params.ChainConfig, bc ChainContext, txFeeRecipient *common.Address, gp *GasPool, statedb *state.StateDB, header *types.Header, tx *types.Transaction, usedGas *uint64, cfg vm.Config, vmRunner vm.EVMRunner, sysCtx *SysContractCallCtx) (*types.Receipt, error) {
	var baseFee *big.Int
	if config.IsEspresso(header.Number) {
		baseFee = sysCtx.GetGasPriceMinimum(tx.FeeCurrency())
	}
	msg, err := tx.AsMessage(types.MakeSigner(config, header.Number), baseFee)
	if err != nil {
		return nil, err
	}
	// Create a new context to be used in the EVM environment
	blockContext := NewEVMBlockContext(header, bc, txFeeRecipient)
	vmenv := vm.NewEVM(blockContext, vm.TxContext{}, statedb, config, cfg)
	return applyTransaction(msg, config, gp, statedb, header.Number, header.Hash(), tx, usedGas, vmenv, vmRunner, sysCtx)
}
