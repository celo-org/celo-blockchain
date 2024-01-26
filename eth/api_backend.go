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

package eth

import (
	"context"
	"errors"
	"fmt"
	"math/big"

	ethereum "github.com/celo-org/celo-blockchain"
	"github.com/celo-org/celo-blockchain/accounts"
	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/consensus"
	"github.com/celo-org/celo-blockchain/contracts/blockchain_parameters"
	"github.com/celo-org/celo-blockchain/core"
	"github.com/celo-org/celo-blockchain/core/bloombits"
	"github.com/celo-org/celo-blockchain/core/rawdb"
	"github.com/celo-org/celo-blockchain/core/state"
	"github.com/celo-org/celo-blockchain/core/types"
	"github.com/celo-org/celo-blockchain/core/vm"
	gp "github.com/celo-org/celo-blockchain/eth/gasprice"
	"github.com/celo-org/celo-blockchain/ethdb"
	"github.com/celo-org/celo-blockchain/event"
	"github.com/celo-org/celo-blockchain/log"
	"github.com/celo-org/celo-blockchain/miner"
	"github.com/celo-org/celo-blockchain/params"
	"github.com/celo-org/celo-blockchain/rpc"
)

// EthAPIBackend implements ethapi.Backend for full nodes
type EthAPIBackend struct {
	extRPCEnabled       bool
	allowUnprotectedTxs bool
	eth                 *Ethereum
}

// ChainConfig returns the active chain configuration.
func (b *EthAPIBackend) ChainConfig() *params.ChainConfig {
	return b.eth.blockchain.Config()
}

func (b *EthAPIBackend) CurrentBlock() *types.Block {
	return b.eth.blockchain.CurrentBlock()
}

func (b *EthAPIBackend) SetHead(number uint64) {
	b.eth.handler.downloader.Cancel()
	b.eth.blockchain.SetHead(number)
}

func (b *EthAPIBackend) HeaderByNumber(ctx context.Context, number rpc.BlockNumber) (*types.Header, error) {
	// Pending block is only known by the miner
	if number == rpc.PendingBlockNumber {
		block := b.eth.miner.PendingBlock()
		return block.Header(), nil
	}
	// Otherwise resolve and return the block
	if number == rpc.LatestBlockNumber {
		return b.eth.blockchain.CurrentBlock().Header(), nil
	}
	return b.eth.blockchain.GetHeaderByNumber(uint64(number)), nil
}

func (b *EthAPIBackend) HeaderByNumberOrHash(ctx context.Context, blockNrOrHash rpc.BlockNumberOrHash) (*types.Header, error) {
	if blockNr, ok := blockNrOrHash.Number(); ok {
		return b.HeaderByNumber(ctx, blockNr)
	}
	if hash, ok := blockNrOrHash.Hash(); ok {
		header := b.eth.blockchain.GetHeaderByHash(hash)
		if header == nil {
			return nil, errors.New("header for hash not found")
		}
		if blockNrOrHash.RequireCanonical && b.eth.blockchain.GetCanonicalHash(header.Number.Uint64()) != hash {
			return nil, errors.New("hash is not currently canonical")
		}
		return header, nil
	}
	return nil, errors.New("invalid arguments; neither block nor hash specified")
}

func (b *EthAPIBackend) HeaderByHash(ctx context.Context, hash common.Hash) (*types.Header, error) {
	return b.eth.blockchain.GetHeaderByHash(hash), nil
}

func (b *EthAPIBackend) BlockByNumber(ctx context.Context, number rpc.BlockNumber) (*types.Block, error) {
	// Pending block is only known by the miner
	if number == rpc.PendingBlockNumber {
		block := b.eth.miner.PendingBlock()
		return block, nil
	}
	// Otherwise resolve and return the block
	if number == rpc.LatestBlockNumber {
		return b.eth.blockchain.CurrentBlock(), nil
	}
	return b.eth.blockchain.GetBlockByNumber(uint64(number)), nil
}

func (b *EthAPIBackend) BlockByHash(ctx context.Context, hash common.Hash) (*types.Block, error) {
	return b.eth.blockchain.GetBlockByHash(hash), nil
}

func (b *EthAPIBackend) BlockByNumberOrHash(ctx context.Context, blockNrOrHash rpc.BlockNumberOrHash) (*types.Block, error) {
	if blockNr, ok := blockNrOrHash.Number(); ok {
		return b.BlockByNumber(ctx, blockNr)
	}
	if hash, ok := blockNrOrHash.Hash(); ok {
		header := b.eth.blockchain.GetHeaderByHash(hash)
		if header == nil {
			return nil, errors.New("header for hash not found")
		}
		if blockNrOrHash.RequireCanonical && b.eth.blockchain.GetCanonicalHash(header.Number.Uint64()) != hash {
			return nil, errors.New("hash is not currently canonical")
		}
		block := b.eth.blockchain.GetBlock(hash, header.Number.Uint64())
		if block == nil {
			return nil, errors.New("header found, but block body is missing")
		}
		return block, nil
	}
	return nil, errors.New("invalid arguments; neither block nor hash specified")
}

func (b *EthAPIBackend) PendingBlockAndReceipts() (*types.Block, types.Receipts) {
	return b.eth.miner.PendingBlockAndReceipts()
}

func (b *EthAPIBackend) StateAndHeaderByNumber(ctx context.Context, number rpc.BlockNumber) (*state.StateDB, *types.Header, error) {
	// Pending state is only known by the miner
	if number == rpc.PendingBlockNumber {
		block, state := b.eth.miner.Pending()
		if block == nil && state == nil {
			return nil, nil, errors.New("no pending block")
		}
		return state, block.Header(), nil
	}
	// Otherwise resolve the block number and return its state
	header, err := b.HeaderByNumber(ctx, number)
	if err != nil {
		return nil, nil, err
	}
	if header == nil {
		return nil, nil, errors.New("header not found")
	}
	stateDb, err := b.eth.BlockChain().StateAt(header.Root)
	return stateDb, header, err
}

func (b *EthAPIBackend) StateAndHeaderByNumberOrHash(ctx context.Context, blockNrOrHash rpc.BlockNumberOrHash) (*state.StateDB, *types.Header, error) {
	if blockNr, ok := blockNrOrHash.Number(); ok {
		return b.StateAndHeaderByNumber(ctx, blockNr)
	}
	if hash, ok := blockNrOrHash.Hash(); ok {
		header, err := b.HeaderByHash(ctx, hash)
		if err != nil {
			return nil, nil, err
		}
		if header == nil {
			return nil, nil, errors.New("header for hash not found")
		}
		if blockNrOrHash.RequireCanonical && b.eth.blockchain.GetCanonicalHash(header.Number.Uint64()) != hash {
			return nil, nil, errors.New("hash is not currently canonical")
		}
		stateDb, err := b.eth.BlockChain().StateAt(header.Root)
		return stateDb, header, err
	}
	return nil, nil, errors.New("invalid arguments; neither block nor hash specified")
}

func (b *EthAPIBackend) GetReceipts(ctx context.Context, hash common.Hash) (types.Receipts, error) {
	return b.eth.blockchain.GetReceiptsByHash(hash), nil
}

func (b *EthAPIBackend) GetLogs(ctx context.Context, hash common.Hash) ([][]*types.Log, error) {
	db := b.eth.ChainDb()
	number := rawdb.ReadHeaderNumber(db, hash)
	if number == nil {
		return nil, errors.New("failed to get block number from hash")
	}
	logs := rawdb.ReadLogs(db, hash, *number)
	if logs == nil {
		// Even if this changes the behaviour of the old rpc call (was returning an empty list, not an error) which we try to avoid
		// we decided to keep the error to maintain tooling compatibility with upstream
		return nil, errors.New("failed to get logs for block")
	}
	return logs, nil
}

func (b *EthAPIBackend) GetTd(ctx context.Context, hash common.Hash) *big.Int {
	return b.eth.blockchain.GetTdByHash(hash)
}

func (b *EthAPIBackend) GetEVM(ctx context.Context, msg core.Message, state *state.StateDB, header *types.Header, vmConfig *vm.Config) (*vm.EVM, func() error, error) {
	vmError := func() error { return nil }
	if vmConfig == nil {
		vmConfig = b.eth.blockchain.GetVMConfig()
	}
	txContext := core.NewEVMTxContext(msg)
	context := core.NewEVMBlockContext(header, b.eth.BlockChain(), nil)
	return vm.NewEVM(context, txContext, state, b.eth.blockchain.Config(), *vmConfig), vmError, nil
}

func (b *EthAPIBackend) SubscribeRemovedLogsEvent(ch chan<- core.RemovedLogsEvent) event.Subscription {
	return b.eth.BlockChain().SubscribeRemovedLogsEvent(ch)
}

func (b *EthAPIBackend) SubscribePendingLogsEvent(ch chan<- []*types.Log) event.Subscription {
	return b.eth.miner.SubscribePendingLogs(ch)
}

func (b *EthAPIBackend) SubscribeChainEvent(ch chan<- core.ChainEvent) event.Subscription {
	return b.eth.BlockChain().SubscribeChainEvent(ch)
}

func (b *EthAPIBackend) SubscribeChainHeadEvent(ch chan<- core.ChainHeadEvent) event.Subscription {
	return b.eth.BlockChain().SubscribeChainHeadEvent(ch)
}

func (b *EthAPIBackend) SubscribeChainSideEvent(ch chan<- core.ChainSideEvent) event.Subscription {
	return b.eth.BlockChain().SubscribeChainSideEvent(ch)
}

func (b *EthAPIBackend) SubscribeLogsEvent(ch chan<- []*types.Log) event.Subscription {
	return b.eth.BlockChain().SubscribeLogsEvent(ch)
}

func (b *EthAPIBackend) SendTx(ctx context.Context, signedTx *types.Transaction) error {
	return b.eth.txPool.AddLocal(signedTx)
}

func (b *EthAPIBackend) GetPoolTransactions() (types.Transactions, error) {
	pending, err := b.eth.txPool.Pending(false)
	if err != nil {
		return nil, err
	}
	var txs types.Transactions
	for _, batch := range pending {
		txs = append(txs, batch...)
	}
	return txs, nil
}

func (b *EthAPIBackend) GetPoolTransaction(hash common.Hash) *types.Transaction {
	return b.eth.txPool.Get(hash)
}

func (b *EthAPIBackend) GetTransaction(ctx context.Context, txHash common.Hash) (*types.Transaction, common.Hash, uint64, uint64, error) {
	tx, blockHash, blockNumber, index := rawdb.ReadTransaction(b.eth.ChainDb(), txHash)
	return tx, blockHash, blockNumber, index, nil
}

func (b *EthAPIBackend) GetPoolNonce(ctx context.Context, addr common.Address) (uint64, error) {
	return b.eth.txPool.Nonce(addr), nil
}

func (b *EthAPIBackend) Stats() (pending int, queued int) {
	return b.eth.txPool.Stats()
}

func (b *EthAPIBackend) TxPoolContent() (map[common.Address]types.Transactions, map[common.Address]types.Transactions) {
	return b.eth.TxPool().Content()
}

func (b *EthAPIBackend) TxPoolContentFrom(addr common.Address) (types.Transactions, types.Transactions) {
	return b.eth.TxPool().ContentFrom(addr)
}

func (b *EthAPIBackend) TxPool() *core.TxPool {
	return b.eth.TxPool()
}

func (b *EthAPIBackend) SubscribeNewTxsEvent(ch chan<- core.NewTxsEvent) event.Subscription {
	return b.eth.TxPool().SubscribeNewTxsEvent(ch)
}

func (b *EthAPIBackend) SyncProgress() ethereum.SyncProgress {
	return b.eth.Downloader().Progress()
}

func (b *EthAPIBackend) SuggestGasTipCap(ctx context.Context, currencyAddress *common.Address) (*big.Int, error) {
	vmRunner, err := b.eth.BlockChain().NewEVMRunnerForCurrentBlock()
	if err != nil {
		return nil, err
	}
	return gp.GetGasTipCapSuggestion(vmRunner, currencyAddress)
}

func (b *EthAPIBackend) CurrentGasPriceMinimum(ctx context.Context, currencyAddress *common.Address) (*big.Int, error) {
	header := b.CurrentHeader()
	if header.BaseFee != nil && currencyAddress == nil {
		return header.BaseFee, nil
	}
	vmRunner, err := b.eth.BlockChain().NewEVMRunnerForCurrentBlock()
	if err != nil {
		return nil, err
	}
	return gp.GetBaseFeeForCurrency(vmRunner, currencyAddress, header.BaseFee)
}

func (b *EthAPIBackend) GasPriceMinimumForHeader(ctx context.Context, currencyAddress *common.Address, header *types.Header) (*big.Int, error) {
	if header.BaseFee != nil && currencyAddress == nil {
		return header.BaseFee, nil
	}
	// The gasPriceMinimum (celo or alternative currency) of a specific block, is the one at the beginning of the block,
	// not the end of it (the state_root of the header is the a state resulted of applying the block). So, the state to
	// be used, MUST be the state result of the parent block unleess this is the genesis block.
	h := header.ParentOrGenesisHash()
	state, parent, err := b.StateAndHeaderByNumberOrHash(ctx, rpc.BlockNumberOrHash{BlockHash: &h})
	if err != nil {
		return nil, err
	}
	vmRunner := b.eth.BlockChain().NewEVMRunner(parent, state)
	return gp.GetBaseFeeForCurrency(vmRunner, currencyAddress, header.BaseFee)
}

func (b *EthAPIBackend) RealGasPriceMinimumForHeader(ctx context.Context, currencyAddress *common.Address, header *types.Header) (*big.Int, error) {
	if header.BaseFee != nil && currencyAddress == nil {
		return header.BaseFee, nil
	}
	// The gasPriceMinimum (celo or alternative currency) of a specific block, is the one at the beginning of the block,
	// not the end of it (the state_root of the header is the a state resulted of applying the block). So, the state to
	// be used, MUST be the state result of the parent block unleess this is the genesis block.
	h := header.ParentOrGenesisHash()
	state, parent, err := b.StateAndHeaderByNumberOrHash(ctx, rpc.BlockNumberOrHash{BlockHash: &h})
	if err != nil {
		return nil, err
	}
	vmRunner := b.eth.BlockChain().NewEVMRunner(parent, state)
	return gp.GetRealBaseFeeForCurrency(vmRunner, currencyAddress, header.BaseFee)
}

func (b *EthAPIBackend) SuggestPrice(ctx context.Context, currencyAddress *common.Address) (*big.Int, error) {
	vmRunner, err := b.eth.BlockChain().NewEVMRunnerForCurrentBlock()
	if err != nil {
		return nil, err
	}
	return gp.GetGasPriceSuggestion(vmRunner, currencyAddress, b.CurrentHeader().BaseFee, b.eth.config.RPCGasPriceMultiplier)
}

func (b *EthAPIBackend) GetBlockGasLimit(ctx context.Context, blockNrOrHash rpc.BlockNumberOrHash) uint64 {
	header, err := b.HeaderByNumberOrHash(ctx, blockNrOrHash)
	if err != nil {
		log.Warn("Cannot retrieve the header for blockGasLimit", "err", err)
		return params.DefaultGasLimit
	}
	if header.GasLimit > 0 {
		return header.GasLimit
	}
	// The gasLimit of a specific block, is the one at the beginning of the block,
	// not the end of it (the state_root of the header is the a state resulted of applying the block). So, the state to
	// be used, MUST be the state result of the parent block
	state, parent, err := b.StateAndHeaderByNumberOrHash(ctx, rpc.BlockNumberOrHash{BlockHash: &header.ParentHash})
	if err != nil {
		log.Warn("Cannot create evmCaller to get blockGasLimit", "err", err)
		return params.DefaultGasLimit
	}
	vmRunner := b.eth.BlockChain().NewEVMRunner(parent, state)
	return blockchain_parameters.GetBlockGasLimitOrDefault(vmRunner)
}

func (b *EthAPIBackend) GetRealBlockGasLimit(ctx context.Context, blockNrOrHash rpc.BlockNumberOrHash) (uint64, error) {
	header, err := b.HeaderByNumberOrHash(ctx, blockNrOrHash)
	if err != nil {
		return 0, fmt.Errorf("EthApiBackend failed to retrieve header for gas limit %v: %w", blockNrOrHash, err)
	}
	if header.GasLimit > 0 {
		return header.GasLimit, nil
	}
	// The gasLimit of a specific block, is the one at the beginning of the block,
	// not the end of it (the state_root of the header is the a state resulted of applying the block). So, the state to
	// be used, MUST be the state result of the parent block
	state, parent, err := b.StateAndHeaderByNumberOrHash(ctx, rpc.BlockNumberOrHash{BlockHash: &header.ParentHash})
	if err != nil {
		return 0, fmt.Errorf("EthApiBackend failed to retrieve state for block gas limit for block %v: %w", blockNrOrHash, err)
	}
	vmRunner := b.eth.BlockChain().NewEVMRunner(parent, state)
	limit, err := blockchain_parameters.GetBlockGasLimit(vmRunner)
	if err != nil {
		return 0, fmt.Errorf("EthApiBackend failed to retrieve block gas limit from blockchain parameters constract for block %v: %w", blockNrOrHash, err)
	}
	return limit, nil
}

func (b *EthAPIBackend) NewEVMRunner(header *types.Header, state vm.StateDB) vm.EVMRunner {
	return b.eth.BlockChain().NewEVMRunner(header, state)
}

func (b *EthAPIBackend) GetIntrinsicGasForAlternativeFeeCurrency(ctx context.Context) uint64 {
	vmRunner, err := b.eth.BlockChain().NewEVMRunnerForCurrentBlock()
	if err != nil {
		log.Warn("Cannot create evmCaller to get intrinsic gas for alternative fee currency", "err", err)
		return blockchain_parameters.DefaultIntrinsicGasForAlternativeFeeCurrency
	}
	return blockchain_parameters.GetIntrinsicGasForAlternativeFeeCurrencyOrDefault(vmRunner)
}

func (b *EthAPIBackend) ChainDb() ethdb.Database {
	return b.eth.ChainDb()
}

func (b *EthAPIBackend) EventMux() *event.TypeMux {
	return b.eth.EventMux()
}

func (b *EthAPIBackend) AccountManager() *accounts.Manager {
	return b.eth.AccountManager()
}

func (b *EthAPIBackend) ExtRPCEnabled() bool {
	return b.extRPCEnabled
}

func (b *EthAPIBackend) UnprotectedAllowed() bool {
	return b.allowUnprotectedTxs
}

func (b *EthAPIBackend) RPCGasInflationRate() float64 {
	return b.eth.config.RPCGasInflationRate
}

func (b *EthAPIBackend) RPCGasCap() uint64 {
	return b.eth.config.RPCGasCap
}

func (b *EthAPIBackend) RPCEthCompatibility() bool {
	return b.eth.config.RPCEthCompatibility
}

func (b *EthAPIBackend) RPCTxFeeCap() float64 {
	return b.eth.config.RPCTxFeeCap
}

func (b *EthAPIBackend) BloomStatus() (uint64, uint64) {
	sections, _, _ := b.eth.bloomIndexer.Sections()
	return params.BloomBitsBlocks, sections
}

func (b *EthAPIBackend) ServiceFilter(ctx context.Context, session *bloombits.MatcherSession) {
	for i := 0; i < bloomFilterThreads; i++ {
		go session.Multiplex(bloomRetrievalBatch, bloomRetrievalWait, b.eth.bloomRequests)
	}
}

func (b *EthAPIBackend) GatewayFeeRecipient() common.Address {
	return b.eth.GatewayFeeRecipient()
}

func (b *EthAPIBackend) GatewayFee() *big.Int {
	return b.eth.GatewayFee()
}

func (b *EthAPIBackend) Engine() consensus.Engine {
	return b.eth.engine
}

func (b *EthAPIBackend) CurrentHeader() *types.Header {
	return b.eth.blockchain.CurrentHeader()
}

func (b *EthAPIBackend) Miner() *miner.Miner {
	return b.eth.Miner()
}

func (b *EthAPIBackend) StartMining() error {
	return b.eth.StartMining()
}

func (b *EthAPIBackend) StateAtBlock(ctx context.Context, block *types.Block, reexec uint64, base *state.StateDB, checkLive, preferDisk bool, commitRandomness bool) (*state.StateDB, error) {
	return b.eth.celoStateAtBlock(block, reexec, base, checkLive, preferDisk, commitRandomness)
}

func (b *EthAPIBackend) StateAtTransaction(ctx context.Context, block *types.Block, txIndex int, reexec uint64) (core.Message, vm.BlockContext, vm.EVMRunner, *state.StateDB, error) {
	return b.eth.stateAtTransaction(block, txIndex, reexec)
}
