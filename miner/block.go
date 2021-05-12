// Copyright 2021 The celo Authors
// This file is part of the celo library.
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

package miner

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/consensus"
	"github.com/celo-org/celo-blockchain/contract_comm/random"
	"github.com/celo-org/celo-blockchain/core"
	"github.com/celo-org/celo-blockchain/core/rawdb"
	"github.com/celo-org/celo-blockchain/core/state"
	"github.com/celo-org/celo-blockchain/core/types"
	"github.com/celo-org/celo-blockchain/log"
	"github.com/celo-org/celo-blockchain/params"
)

type blockState struct {
	signer types.Signer

	state    *state.StateDB // apply state changes here
	tcount   int            // tx count in cycle
	gasPool  *core.GasPool  // available gas used to pack transactions
	gasLimit uint64

	header         *types.Header
	txs            []*types.Transaction
	receipts       []*types.Receipt
	randomness     *types.Randomness // The types.Randomness of the last block by mined by this worker.
	txFeeRecipient common.Address
}

// newBlockState creates a new environment for the current cycle.
func (w *worker) newBlockState(parent *types.Block, header *types.Header) (*blockState, error) {
	state, err := w.chain.StateAt(parent.Root())
	if err != nil {
		return nil, err
	}

	env := &blockState{
		signer:   types.NewEIP155Signer(w.chainConfig.ChainID),
		state:    state,
		tcount:   0,
		gasLimit: core.CalcGasLimit(parent, state),
		header:   header,
	}

	return env, nil
}

func (b *blockState) commitTransaction(w *worker, tx *types.Transaction, txFeeRecipient common.Address) ([]*types.Log, error) {
	snap := b.state.Snapshot()

	receipt, err := core.ApplyTransaction(w.chainConfig, w.chain, &txFeeRecipient, b.gasPool, b.state, b.header, tx, &b.header.GasUsed, *w.chain.GetVMConfig())
	if err != nil {
		b.state.RevertToSnapshot(snap)
		return nil, err
	}
	b.txs = append(b.txs, tx)
	b.receipts = append(b.receipts, receipt)

	return receipt.Logs, nil
}

func (b *blockState) commitTransactions(ctx context.Context, w *worker, txs *types.TransactionsByPriceAndNonce, txFeeRecipient common.Address) error {
	if b.gasPool == nil {
		b.gasPool = new(core.GasPool).AddGas(b.gasLimit)
	}

	var coalescedLogs []*types.Log
	defer func() {
		if !w.isRunning() && len(coalescedLogs) > 0 {
			// We don't push the pendingLogsEvent while we are mining. The reason is that
			// when we are mining, the worker will regenerate a mining block every 3 seconds.
			// In order to avoid pushing the repeated pendingLog, we disable the pending log pushing.

			// make a copy, the state caches the logs and these logs get "upgraded" from pending to mined
			// logs by filling in the block hash when the block was mined by the local miner. This can
			// cause a race condition if a log was "upgraded" before the PendingLogsEvent is processed.
			cpy := make([]*types.Log, len(coalescedLogs))
			for i, l := range coalescedLogs {
				cpy[i] = new(types.Log)
				*cpy[i] = *l
			}
			w.pendingLogsFeed.Send(cpy)
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			// pass
		}
		// If we don't have enough gas for any further transactions then we're done
		if b.gasPool.Gas() < params.TxGas {
			log.Trace("Not enough gas for further transactions", "have", b.gasPool, "want", params.TxGas)
			return nil
		}
		// Retrieve the next transaction and abort if all done
		tx := txs.Peek()
		if tx == nil {
			return nil
		}
		// Short-circuit if the transaction requires more gas than we have in the pool.
		// If we didn't short-circuit here, we would get core.ErrGasLimitReached below.
		// Short-circuiting here saves us the trouble of checking the GPM and so on when the tx can't be included
		// anyway due to the block not having enough gas left.
		if b.gasPool.Gas() < tx.Gas() {
			log.Trace("Skipping transaction which requires more gas than is left in the block", "hash", tx.Hash(), "gas", b.gasPool.Gas(), "txgas", tx.Gas())
			txs.Pop()
			continue
		}
		// Error may be ignored here. The error has already been checked
		// during transaction acceptance is the transaction pool.
		//
		// We use the eip155 signer regardless of the current hf.
		from, _ := types.Sender(b.signer, tx)
		// Check whether the tx is replay protected. If we're not in the EIP155 hf
		// phase, start ignoring the sender until we do.
		if tx.Protected() && !w.chainConfig.IsEIP155(b.header.Number) {
			log.Trace("Ignoring reply protected transaction", "hash", tx.Hash(), "eip155", w.chainConfig.EIP155Block)

			txs.Pop()
			continue
		}
		// Start executing the transaction
		b.state.Prepare(tx.Hash(), common.Hash{}, b.tcount)

		logs, err := b.commitTransaction(w, tx, txFeeRecipient)
		switch err {
		case core.ErrGasLimitReached:
			// Pop the current out-of-gas transaction without shifting in the next from the account
			log.Trace("Gas limit exceeded for current block", "sender", from)
			txs.Pop()

		case core.ErrNonceTooLow:
			// New head notification data race between the transaction pool and miner, shift
			log.Trace("Skipping transaction with low nonce", "sender", from, "nonce", tx.Nonce())
			txs.Shift()

		case core.ErrNonceTooHigh:
			// Reorg notification data race between the transaction pool and miner, skip account =
			log.Trace("Skipping account with hight nonce", "sender", from, "nonce", tx.Nonce())
			txs.Pop()

		case core.ErrGasPriceDoesNotExceedMinimum:
			// We are below the GPM, so we can stop (the rest of the transactions will either have
			// even lower gas price or won't be mineable yet due to their nonce)
			log.Trace("Skipping remaining transaction below the gas price minimum")
			return nil

		case nil:
			// Everything ok, collect the logs and shift in the next transaction from the same account
			coalescedLogs = append(coalescedLogs, logs...)
			b.tcount++
			txs.Shift()

		default:
			// Strange error, discard the transaction and get the next in line (note, the
			// nonce-too-high clause will prevent us from executing in vain).
			log.Debug("Transaction failed, account skipped", "hash", tx.Hash(), "err", err)
			txs.Shift()
		}
	}

}

// prepareBlock intializes a new blockState that is ready to have transaction included to.
func (w *worker) prepareBlock() (*blockState, error) {
	w.mu.RLock()
	defer w.mu.RUnlock()

	timestamp := time.Now().Unix()
	parent := w.chain.CurrentBlock()

	if parent.Time() >= uint64(timestamp) {
		timestamp = int64(parent.Time() + 1)
	}

	num := parent.Number()
	header := &types.Header{
		ParentHash: parent.Hash(),
		Number:     num.Add(num, common.Big1),
		Extra:      w.extra,
		Time:       uint64(timestamp),
	}

	txFeeRecipient := w.txFeeRecipient
	if !w.chainConfig.IsDonut(header.Number) && w.txFeeRecipient != w.validator {
		txFeeRecipient = w.validator
		log.Warn("TxFeeRecipient and Validator flags set before split etherbase fork is active. Defaulting to the given validator address for the coinbase.")
	}

	// Only set the coinbase if our consensus engine is running (avoid spurious block rewards)
	if w.isRunning() {
		if txFeeRecipient == (common.Address{}) {
			return nil, errors.New("Refusing to mine without etherbase")
		}
		header.Coinbase = txFeeRecipient
	}
	if err := w.engine.Prepare(w.chain, header); err != nil {
		log.Error("Failed to prepare header for mining", "err", err)
		return nil, fmt.Errorf("Failed to prepare header for mining: %w", err)
	}

	// Initialize the block state itself
	state, err := w.chain.StateAt(parent.Root())
	if err != nil {
		return nil, fmt.Errorf("Failed to get the parent state: %w:", err)
	}

	b := &blockState{
		signer:         types.NewEIP155Signer(w.chainConfig.ChainID),
		state:          state,
		tcount:         0,
		gasLimit:       core.CalcGasLimit(parent, state),
		header:         header,
		txFeeRecipient: txFeeRecipient,
	}

	// Play our part in generating the random beacon.
	if w.isRunning() && random.IsRunning() {
		istanbul, ok := w.engine.(consensus.Istanbul)
		if !ok {
			log.Crit("Istanbul consensus engine must be in use for the randomness beacon")
		}

		lastCommitment, err := random.GetLastCommitment(w.validator, b.header, b.state)
		if err != nil {
			log.Error("Failed to get last commitment", "err", err)
			return nil, fmt.Errorf("Failed to get last commitment: %w", err)
		}

		lastRandomness := common.Hash{}
		if (lastCommitment != common.Hash{}) {
			lastRandomnessParentHash := rawdb.ReadRandomCommitmentCache(w.db, lastCommitment)
			if (lastRandomnessParentHash == common.Hash{}) {
				log.Error("Failed to get last randomness cache entry")
				return nil, errors.New("Failed to get last randomness cache entry")
			}

			var err error
			lastRandomness, _, err = istanbul.GenerateRandomness(lastRandomnessParentHash)
			if err != nil {
				log.Error("Failed to generate last randomness", "err", err)
				return nil, fmt.Errorf("Failed to generate last randomness: %w", err)
			}
		}

		_, newCommitment, err := istanbul.GenerateRandomness(b.header.ParentHash)
		if err != nil {
			log.Error("Failed to generate new randomness", "err", err)
			return nil, fmt.Errorf("Failed to generate new randomness: %w", err)
		}

		err = random.RevealAndCommit(lastRandomness, newCommitment, w.validator, b.header, b.state)
		if err != nil {
			log.Error("Failed to reveal and commit randomness", "randomness", lastRandomness.Hex(), "commitment", newCommitment.Hex(), "err", err)
			return nil, fmt.Errorf("Failed to reveal and commit randomness: %w", err)
		}
		// always true (EIP158)
		b.state.IntermediateRoot(true)

		b.randomness = &types.Randomness{Revealed: lastRandomness, Committed: newCommitment}
	} else {
		b.randomness = &types.EmptyRandomness
	}

	// TODO:  Pull this out?
	w.updateSnapshot(b)

	return b, nil
}

// commitNewWork generates several new sealing tasks based on the parent block.
func (w *worker) commitNewWork(timestamp int64) {
	tstart := time.Now()

	// Initialize the block.
	b, err := w.prepareBlock()
	if err != nil {
		log.Error("Failed to create mining context", "err", err)
		return
	}
	// TODO: Sleep here instead of in prepareBlock()

	istanbulEmptyBlockCommit := func() {
		if w.isIstanbulEngine() {
			b.commit(w, false, tstart)
		}
	}

	// Fill the block with all available pending transactions.
	pending, err := w.eth.TxPool().Pending()

	if err != nil {
		log.Error("Failed to fetch pending transactions", "err", err)
		istanbulEmptyBlockCommit()
		return
	}

	// Short circuit if there is no available pending transactions.
	if len(pending) == 0 {
		istanbulEmptyBlockCommit()
		return
	}
	// Split the pending transactions into locals and remotes
	localTxs, remoteTxs := make(map[common.Address]types.Transactions), pending
	for _, account := range w.eth.TxPool().Locals() {
		if txs := remoteTxs[account]; len(txs) > 0 {
			delete(remoteTxs, account)
			localTxs[account] = txs
		}
	}

	txComparator := w.createTxCmp()
	if len(localTxs) > 0 {
		txs := types.NewTransactionsByPriceAndNonce(b.signer, localTxs, txComparator)
		if err := b.commitTransactions(context.TODO(), w, txs, b.txFeeRecipient); err != nil {
			return
		}
	}
	if len(remoteTxs) > 0 {
		txs := types.NewTransactionsByPriceAndNonce(b.signer, remoteTxs, txComparator)
		if err := b.commitTransactions(context.TODO(), w, txs, b.txFeeRecipient); err != nil {
			return
		}
	}
	b.commit(w, true, tstart)
}

// commit runs any post-transaction state modifications, assembles the final block
// and commits new work if consensus engine is running.
func (b *blockState) commit(w *worker, update bool, start time.Time) error {
	// Deep copy receipts here to avoid interaction between different tasks.
	receipts := make([]*types.Receipt, len(b.receipts))
	for i, l := range b.receipts {
		receipts[i] = new(types.Receipt)
		*receipts[i] = *l
	}
	s := b.state.Copy()

	block, err := w.engine.FinalizeAndAssemble(w.chain, b.header, s, b.txs, b.receipts, b.randomness)

	// Set the validator set diff in the new header if we're using Istanbul and it's the last block of the epoch
	if istanbul, ok := w.engine.(consensus.Istanbul); ok {
		if err := istanbul.UpdateValSetDiff(w.chain, block.MutableHeader(), s); err != nil {
			log.Error("Unable to update Validator Set Diff", "err", err)
			return err
		}
	}

	if len(s.GetLogs(common.Hash{})) > 0 {
		receipt := types.NewReceipt(nil, false, 0)
		receipt.Logs = s.GetLogs(common.Hash{})
		for i := range receipt.Logs {
			receipt.Logs[i].TxIndex = uint(len(receipts))
		}
		receipt.Bloom = types.CreateBloom(types.Receipts{receipt})
		receipts = append(receipts, receipt)
	}

	if err != nil {
		log.Error("Unable to finalize block", "err", err)
		return err
	}
	if w.isRunning() {
		w.handleTask(&task{receipts: receipts, state: s, block: block, createdAt: time.Now()})

		feesEth := totalFees(block, receipts)
		log.Info("Commit new mining work", "number", block.Number(), "sealhash", w.engine.SealHash(block.Header()),
			"txs", b.tcount, "gas", block.GasUsed(), "fees", feesEth, "elapsed", common.PrettyDuration(time.Since(start)))

	}
	if update {
		w.updateSnapshot(b)
	}
	return nil
}

// totalFees computes total consumed fees in ETH. Block transactions and receipts have to have the same order.
func totalFees(block *types.Block, receipts []*types.Receipt) *big.Float {
	feesWei := new(big.Int)
	for i, tx := range block.Transactions() {
		feesWei.Add(feesWei, new(big.Int).Mul(new(big.Int).SetUint64(receipts[i].GasUsed), tx.GasPrice()))
	}
	return new(big.Float).Quo(new(big.Float).SetInt(feesWei), new(big.Float).SetInt(big.NewInt(params.Ether)))
}
