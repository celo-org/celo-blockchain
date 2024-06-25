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

package tracers

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/celo-org/celo-blockchain/accounts/abi"
	"io/ioutil"
	"math/big"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/common/hexutil"
	"github.com/celo-org/celo-blockchain/consensus"
	"github.com/celo-org/celo-blockchain/core"
	"github.com/celo-org/celo-blockchain/core/rawdb"
	"github.com/celo-org/celo-blockchain/core/state"
	"github.com/celo-org/celo-blockchain/core/types"
	"github.com/celo-org/celo-blockchain/core/vm"
	"github.com/celo-org/celo-blockchain/ethdb"
	"github.com/celo-org/celo-blockchain/internal/ethapi"
	"github.com/celo-org/celo-blockchain/log"
	"github.com/celo-org/celo-blockchain/params"
	"github.com/celo-org/celo-blockchain/rlp"
	"github.com/celo-org/celo-blockchain/rpc"
)

const (
	// defaultTraceTimeout is the amount of time a single transaction can execute
	// by default before being forcefully aborted.
	defaultTraceTimeout = 5 * time.Second

	// defaultTraceReexec is the number of blocks the tracer is willing to go back
	// and reexecute to produce missing historical state necessary to run a specific
	// trace.
	defaultTraceReexec = uint64(128)

	// defaultTracechainMemLimit is the size of the triedb, at which traceChain
	// switches over and tries to use a disk-backed database instead of building
	// on top of memory.
	// For non-archive nodes, this limit _will_ be overblown, as disk-backed tries
	// will only be found every ~15K blocks or so.
	defaultTracechainMemLimit = common.StorageSize(500 * 1024 * 1024)
)

// Backend interface provides the common API services (that are provided by
// both full and light clients) with access to necessary functions.
type Backend interface {
	HeaderByHash(ctx context.Context, hash common.Hash) (*types.Header, error)
	HeaderByNumber(ctx context.Context, number rpc.BlockNumber) (*types.Header, error)
	BlockByHash(ctx context.Context, hash common.Hash) (*types.Block, error)
	BlockByNumber(ctx context.Context, number rpc.BlockNumber) (*types.Block, error)
	GetTransaction(ctx context.Context, txHash common.Hash) (*types.Transaction, common.Hash, uint64, uint64, error)
	RPCGasCap() uint64
	ChainConfig() *params.ChainConfig
	Engine() consensus.Engine
	ChainDb() ethdb.Database
	// StateAtBlock returns the state corresponding to the stateroot of the block.
	// N.B: For executing transactions on block N, the required stateRoot is block N-1,
	// so this method should be called with the parent.
	// On Celo, this should also include the random commitment of the current block.
	StateAtBlock(ctx context.Context, block *types.Block, reexec uint64, base *state.StateDB, checkLive, preferDisk bool, commitRandomness bool) (*state.StateDB, error)
	StateAtTransaction(ctx context.Context, block *types.Block, txIndex int, reexec uint64) (core.Message, vm.BlockContext, vm.EVMRunner, *state.StateDB, error)
	NewEVMRunner(*types.Header, vm.StateDB) vm.EVMRunner
}

// API is the collection of tracing APIs exposed over the private debugging endpoint.
type API struct {
	backend       Backend
	tokenContract TokenContract
}

// NewAPI creates a new API definition for the tracing methods of the Ethereum service.
func NewAPI(backend Backend) *API {
	return &API{backend: backend, tokenContract: NewTokenContract()}
}

type chainContext struct {
	api *API
	ctx context.Context
}

func (context *chainContext) Engine() consensus.Engine {
	return context.api.backend.Engine()
}

func (context *chainContext) GetHeader(hash common.Hash, number uint64) *types.Header {
	header, err := context.api.backend.HeaderByNumber(context.ctx, rpc.BlockNumber(number))
	if err != nil {
		return nil
	}
	if header.Hash() == hash {
		return header
	}
	header, err = context.api.backend.HeaderByHash(context.ctx, hash)
	if err != nil {
		return nil
	}
	return header
}

func (context *chainContext) GetHeaderByNumber(headerNumber uint64) *types.Header {
	header, err := context.api.backend.HeaderByNumber(context.ctx, rpc.BlockNumber(headerNumber))
	if err != nil {
		return nil
	}
	return header
}

func (context *chainContext) Config() *params.ChainConfig {
	return context.api.backend.ChainConfig()
}

// chainContext construts the context reader which is used by the evm for reading
// the necessary chain context.
func (api *API) chainContext(ctx context.Context) core.ChainContext {
	return &chainContext{api: api, ctx: ctx}
}

// blockByNumber is the wrapper of the chain access function offered by the backend.
// It will return an error if the block is not found.
func (api *API) blockByNumber(ctx context.Context, number rpc.BlockNumber) (*types.Block, error) {
	block, err := api.backend.BlockByNumber(ctx, number)
	if err != nil {
		return nil, err
	}
	if block == nil {
		return nil, fmt.Errorf("block #%d not found", number)
	}
	return block, nil
}

// blockByHash is the wrapper of the chain access function offered by the backend.
// It will return an error if the block is not found.
func (api *API) blockByHash(ctx context.Context, hash common.Hash) (*types.Block, error) {
	block, err := api.backend.BlockByHash(ctx, hash)
	if err != nil {
		return nil, err
	}
	if block == nil {
		return nil, fmt.Errorf("block %s not found", hash.Hex())
	}
	return block, nil
}

// blockByNumberAndHash is the wrapper of the chain access function offered by
// the backend. It will return an error if the block is not found.
//
// Note this function is friendly for the light client which can only retrieve the
// historical(before the CHT) header/block by number.
func (api *API) blockByNumberAndHash(ctx context.Context, number rpc.BlockNumber, hash common.Hash) (*types.Block, error) {
	block, err := api.blockByNumber(ctx, number)
	if err != nil {
		return nil, err
	}
	if block.Hash() == hash {
		return block, nil
	}
	return api.blockByHash(ctx, hash)
}

// TraceConfig holds extra parameters to trace functions.
type TraceConfig struct {
	*vm.LogConfig
	Tracer  *string
	Timeout *string
	Reexec  *uint64
}

// TraceCallConfig is the config for traceCall API. It holds one more
// field to override the state for tracing.
type TraceCallConfig struct {
	*vm.LogConfig
	Tracer         *string
	Timeout        *string
	Reexec         *uint64
	StateOverrides *ethapi.StateOverride
}

// StdTraceConfig holds extra parameters to standard-json trace functions.
type StdTraceConfig struct {
	vm.LogConfig
	Reexec *uint64
	TxHash common.Hash
}

// txTraceResult is the result of a single transaction trace.
type txTraceResult struct {
	TxHash common.Hash `json:"txHash"`           // transaction hash
	Result interface{} `json:"result,omitempty"` // Trace results produced by the tracer
	Error  string      `json:"error,omitempty"`  // Trace failure produced by the tracer
}

// blockTraceTask represents a single block trace task when an entire chain is
// being traced.
type blockTraceTask struct {
	statedb *state.StateDB   // Intermediate state prepped for tracing
	block   *types.Block     // Block to trace the transactions from
	rootref common.Hash      // Trie root reference held for this task
	results []*txTraceResult // Trace results procudes by the task
}

// blockTraceResult represets the results of tracing a single block when an entire
// chain is being traced.
type blockTraceResult struct {
	Block  hexutil.Uint64   `json:"block"`  // Block number corresponding to this trace
	Hash   common.Hash      `json:"hash"`   // Block hash corresponding to this trace
	Traces []*txTraceResult `json:"traces"` // Trace results produced by the task
}

// txTraceTask represents a single transaction trace task when an entire block
// is being traced.
type txTraceTask struct {
	statedb *state.StateDB // Intermediate state prepped for tracing
	index   int            // Transaction offset in the block
}

// TraceChain returns the structured logs created during the execution of EVM
// between two blocks (excluding start) and returns them as a JSON object.
func (api *API) TraceChain(ctx context.Context, start, end rpc.BlockNumber, config *TraceConfig) (*rpc.Subscription, error) { // Fetch the block interval that we want to trace
	from, err := api.blockByNumber(ctx, start)
	if err != nil {
		return nil, err
	}
	to, err := api.blockByNumber(ctx, end)
	if err != nil {
		return nil, err
	}
	if from.Number().Cmp(to.Number()) >= 0 {
		return nil, fmt.Errorf("end block (#%d) needs to come after start block (#%d)", end, start)
	}
	return api.traceChain(ctx, from, to, config)
}

// traceChain configures a new tracer according to the provided configuration, and
// executes all the transactions contained within. The return value will be one item
// per transaction, dependent on the requested tracer.
func (api *API) traceChain(ctx context.Context, start, end *types.Block, config *TraceConfig) (*rpc.Subscription, error) {
	// Tracing a chain is a **long** operation, only do with subscriptions
	notifier, supported := rpc.NotifierFromContext(ctx)
	if !supported {
		return &rpc.Subscription{}, rpc.ErrNotificationsUnsupported
	}
	sub := notifier.CreateSubscription()

	// Prepare all the states for tracing. Note this procedure can take very
	// long time. Timeout mechanism is necessary.
	reexec := defaultTraceReexec
	if config != nil && config.Reexec != nil {
		reexec = *config.Reexec
	}
	blocks := int(end.NumberU64() - start.NumberU64())
	threads := runtime.NumCPU()
	if threads > blocks {
		threads = blocks
	}

	// Celo addition, required for correct block randomness values when tracing.
	// This is an optimization that prevents needing to regenerate and process
	// all preceding blocks for every single block when tracing on a full node;
	// this is largely irrelevant when tracing on an archive node.
	var baseStatedb *state.StateDB
	if start.NumberU64() > 0 {
		previousBlock, err := api.blockByNumber(ctx, rpc.BlockNumber(start.NumberU64()-1))
		if err != nil {
			return nil, fmt.Errorf("failed to get block state prior to start")
		}
		baseStatedb, err = api.backend.StateAtBlock(ctx, previousBlock, reexec, nil, false, false, false)
		if err != nil {
			return nil, fmt.Errorf("failed to compute base statedb")
		}
	}

	var (
		pend     = new(sync.WaitGroup)
		tasks    = make(chan *blockTraceTask, threads)
		results  = make(chan *blockTraceTask, threads)
		localctx = context.Background()
	)
	for th := 0; th < threads; th++ {
		pend.Add(1)
		go func() {
			defer pend.Done()

			// Fetch and execute the next block trace tasks
			for task := range tasks {
				signer := types.MakeSigner(api.backend.ChainConfig(), task.block.Number())
				blockCtx := core.NewEVMBlockContext(task.block.Header(), api.chainContext(localctx), nil)

				isEspresso := api.backend.ChainConfig().IsEspresso(blockCtx.BlockNumber)
				var sysCtx *core.SysContractCallCtx
				if isEspresso {
					sysCtx = core.NewSysContractCallCtx(task.block.Header(), task.statedb, api.backend)
				}
				// Trace all the transactions contained within
				for i, tx := range task.block.Transactions() {
					baseFee := getBaseFee(isEspresso, sysCtx, tx.FeeCurrency())
					msg, _ := tx.AsMessage(signer, baseFee)
					txctx := &Context{
						BlockHash: task.block.Hash(),
						TxIndex:   i,
						TxHash:    tx.Hash(),
					}
					vmRunner := api.backend.NewEVMRunner(task.block.Header(), task.statedb)
					res, err := api.traceTx(localctx, msg, txctx, blockCtx, vmRunner, task.statedb, sysCtx, config)
					if err != nil {
						task.results[i] = &txTraceResult{TxHash: tx.Hash(), Error: err.Error()}
						log.Warn("Tracing failed", "hash", tx.Hash(), "block", task.block.NumberU64(), "err", err)
						break
					}
					// Only delete empty objects if EIP158/161 (a.k.a Spurious Dragon) is in effect
					task.statedb.Finalise(api.backend.ChainConfig().IsEIP158(task.block.Number()))
					task.results[i] = &txTraceResult{TxHash: tx.Hash(), Result: res}
				}
				// Stream the result back to the user or abort on teardown
				select {
				case results <- task:
				case <-notifier.Closed():
					return
				}
			}
		}()
	}
	// Start a goroutine to feed all the blocks into the tracers
	var (
		begin     = time.Now()
		derefTodo []common.Hash // list of hashes to dereference from the db
		derefsMu  sync.Mutex    // mutex for the derefs
	)

	go func() {
		var (
			logged  time.Time
			number  uint64
			traced  uint64
			failed  error
			parent  common.Hash
			statedb *state.StateDB
		)
		// Ensure everything is properly cleaned up on any exit path
		defer func() {
			close(tasks)
			pend.Wait()

			switch {
			case failed != nil:
				log.Warn("Chain tracing failed", "start", start.NumberU64(), "end", end.NumberU64(), "transactions", traced, "elapsed", time.Since(begin), "err", failed)
			case number < end.NumberU64():
				log.Warn("Chain tracing aborted", "start", start.NumberU64(), "end", end.NumberU64(), "abort", number, "transactions", traced, "elapsed", time.Since(begin))
			default:
				log.Info("Chain tracing finished", "start", start.NumberU64(), "end", end.NumberU64(), "transactions", traced, "elapsed", time.Since(begin))
			}
			close(results)
		}()
		var preferDisk bool
		// Feed all the blocks both into the tracer, as well as fast process concurrently
		for number = start.NumberU64(); number < end.NumberU64(); number++ {
			// Stop tracing if interruption was requested
			select {
			case <-notifier.Closed():
				return
			default:
			}
			// clean out any derefs
			derefsMu.Lock()
			for _, h := range derefTodo {
				statedb.Database().TrieDB().Dereference(h)
			}
			derefTodo = derefTodo[:0]
			derefsMu.Unlock()

			// Print progress logs if long enough time elapsed
			if time.Since(logged) > 8*time.Second {
				logged = time.Now()
				log.Info("Tracing chain segment", "start", start.NumberU64(), "end", end.NumberU64(), "current", number, "transactions", traced, "elapsed", time.Since(begin))
			}
			// Retrieve the parent state to trace on top
			block, err := api.blockByNumber(localctx, rpc.BlockNumber(number))
			if err != nil {
				failed = err
				break
			}
			// Prepare the statedb for tracing. Don't use the live database for
			// tracing to avoid persisting state junks into the database.
			// N.B. Celo does not pass in the previous statedb as a `base`,
			// as this persists the previous version with the next block's random commitment.
			// Instead, it passes in `baseStatedb`, defined above, which commits
			// the random commitment made by `Process` in (eth.stateAtBlock),
			// but NOT the next block's commitment made in eth.celoStateAtBlock.
			statedb, err = api.backend.StateAtBlock(localctx, block, reexec, baseStatedb, false, preferDisk, true)
			if err != nil {
				failed = err
				break
			}
			if trieDb := statedb.Database().TrieDB(); trieDb != nil {
				// Hold the reference for tracer, will be released at the final stage
				trieDb.Reference(block.Root(), common.Hash{})

				// Release the parent state because it's already held by the tracer
				if parent != (common.Hash{}) {
					trieDb.Dereference(parent)
				}
				// Prefer disk if the trie db memory grows too much
				s1, s2 := trieDb.Size()
				if !preferDisk && (s1+s2) > defaultTracechainMemLimit {
					log.Info("Switching to prefer-disk mode for tracing", "size", s1+s2)
					preferDisk = true
				}
			}
			parent = block.Root()

			next, err := api.blockByNumber(localctx, rpc.BlockNumber(number+1))
			if err != nil {
				failed = err
				break
			}
			// Send the block over to the concurrent tracers (if not in the fast-forward phase)
			txs := next.Transactions()
			select {
			case tasks <- &blockTraceTask{statedb: statedb.Copy(), block: next, rootref: block.Root(), results: make([]*txTraceResult, len(txs))}:
			case <-notifier.Closed():
				return
			}
			traced += uint64(len(txs))
		}
	}()

	// Keep reading the trace results and stream the to the user
	go func() {
		var (
			done = make(map[uint64]*blockTraceResult)
			next = start.NumberU64() + 1
		)
		for res := range results {
			// Queue up next received result
			result := &blockTraceResult{
				Block:  hexutil.Uint64(res.block.NumberU64()),
				Hash:   res.block.Hash(),
				Traces: res.results,
			}
			// Schedule any parent tries held in memory by this task for dereferencing
			done[uint64(result.Block)] = result
			derefsMu.Lock()
			derefTodo = append(derefTodo, res.rootref)
			derefsMu.Unlock()
			// Stream completed traces to the user, aborting on the first error
			for result, ok := done[next]; ok; result, ok = done[next] {
				if len(result.Traces) > 0 || next == end.NumberU64() {
					notifier.Notify(sub.ID, result)
				}
				delete(done, next)
				next++
			}
		}
	}()
	return sub, nil
}

func (api *API) TraceBlockToken(ctx context.Context, number rpc.BlockNumber, config *TraceConfig) ([]*txTraceResult, error) {
	block, err := api.blockByNumber(ctx, number)
	if err != nil {
		return nil, err
	}
	return api.traceBlock(ctx, block, config)
}

func (api *API) traceBlockToken(ctx context.Context, block *types.Block, config *TraceConfig) ([]*txTraceResult, error) {
	if block.NumberU64() == 0 {
		return nil, errors.New("genesis is not traceable")
	}
	parent, err := api.blockByNumberAndHash(ctx, rpc.BlockNumber(block.NumberU64()-1), block.ParentHash())
	if err != nil {
		return nil, err
	}
	reexec := defaultTraceReexec
	if config != nil && config.Reexec != nil {
		reexec = *config.Reexec
	}
	statedb, err := api.backend.StateAtBlock(ctx, parent, reexec, nil, true, false, true)
	if err != nil {
		return nil, err
	}
	// Execute all the transaction contained within the block concurrently
	var (
		signer  = types.MakeSigner(api.backend.ChainConfig(), block.Number())
		txs     = block.Transactions()
		results = make([]*txTraceResult, len(txs))

		pend = new(sync.WaitGroup)
		jobs = make(chan *txTraceTask, len(txs))
	)
	threads := runtime.NumCPU()
	if threads > len(txs) {
		threads = len(txs)
	}
	blockHash := block.Hash()

	isEspresso := api.backend.ChainConfig().IsEspresso(block.Number())
	var sysCtx *core.SysContractCallCtx
	if isEspresso {
		sysCtx = core.NewSysContractCallCtx(block.Header(), statedb, api.backend)
	}
	for th := 0; th < threads; th++ {
		pend.Add(1)
		go func() {
			blockCtx := core.NewEVMBlockContext(block.Header(), api.chainContext(ctx), nil)
			defer pend.Done()
			// Fetch and execute the next transaction trace tasks
			for task := range jobs {
				baseFee := getBaseFee(isEspresso, sysCtx, txs[task.index].FeeCurrency())
				msg, _ := txs[task.index].AsMessage(signer, baseFee)
				txctx := &Context{
					BlockHash: blockHash,
					TxIndex:   task.index,
					TxHash:    txs[task.index].Hash(),
				}
				vmRunner := api.backend.NewEVMRunner(block.Header(), task.statedb)
				res, err := api.traceTx(ctx, msg, txctx, blockCtx, vmRunner, task.statedb, sysCtx, config)
				if err != nil {
					results[task.index] = &txTraceResult{TxHash: txs[task.index].Hash(), Error: err.Error()}
					continue
				}
				results[task.index] = &txTraceResult{TxHash: txs[task.index].Hash(), Result: res}
			}
		}()
	}
	vmRunner := api.backend.NewEVMRunner(block.Header(), statedb)
	// Feed the transactions into the tracers and return
	var failed error
	blockCtx := core.NewEVMBlockContext(block.Header(), api.chainContext(ctx), nil)
	for i, tx := range txs {
		// Send the trace task over for execution
		jobs <- &txTraceTask{statedb: statedb.Copy(), index: i}

		// Generate the next state snapshot fast without tracing
		baseFee := getBaseFee(isEspresso, sysCtx, tx.FeeCurrency())
		msg, _ := tx.AsMessage(signer, baseFee)
		statedb.Prepare(tx.Hash(), i)
		vmenv := vm.NewEVM(blockCtx, core.NewEVMTxContext(msg), statedb, api.backend.ChainConfig(), vm.Config{})
		if _, err := core.ApplyMessage(vmenv, msg, new(core.GasPool).AddGas(msg.Gas()), vmRunner, sysCtx); err != nil {
			failed = err
			break
		}
		// Finalize the state so any modifications are written to the trie
		// Only delete empty objects if EIP158/161 (a.k.a Spurious Dragon) is in effect
		statedb.Finalise(vmenv.ChainConfig().IsEIP158(block.Number()))
	}
	close(jobs)
	pend.Wait()

	// If execution failed in between, abort
	if failed != nil {
		return nil, failed
	}
	return results, nil
}

func (api *API) TraceTokenTransaction(ctx context.Context, hash common.Hash, config *TraceConfig) (interface{}, error) {
	_, blockHash, blockNumber, index, err := api.backend.GetTransaction(ctx, hash)
	if err != nil {
		return nil, err
	}
	// It shouldn't happen in practice.
	if blockNumber == 0 {
		return nil, errors.New("genesis is not traceable")
	}
	reexec := defaultTraceReexec
	if config != nil && config.Reexec != nil {
		reexec = *config.Reexec
	}
	block, err := api.blockByNumberAndHash(ctx, rpc.BlockNumber(blockNumber), blockHash)
	if err != nil {
		return nil, err
	}

	var sysCtx *core.SysContractCallCtx
	if api.backend.ChainConfig().IsEspresso(block.Number()) {
		parent, err := api.blockByNumber(ctx, rpc.BlockNumber(blockNumber-1))
		if err != nil {
			return nil, err
		}
		sysStateDB, err := api.backend.StateAtBlock(ctx, parent, reexec, nil, true, false, true)
		if err != nil {
			return nil, err
		}
		sysCtx = core.NewSysContractCallCtx(block.Header(), sysStateDB, api.backend)
	}

	msg, vmctx, vmRunner, statedb, err := api.backend.StateAtTransaction(ctx, block, int(index), reexec)
	if err != nil {
		return nil, err
	}
	txctx := &Context{
		BlockHash: blockHash,
		TxIndex:   int(index),
		TxHash:    hash,
	}
	return api.tractTxToken(ctx, msg, txctx, vmctx, vmRunner, statedb, sysCtx, config)
}

func (api *API) tractTxToken(ctx context.Context, message core.Message, txctx *Context, vmctx vm.BlockContext, vmRunner vm.EVMRunner, statedb *state.StateDB, sysCtx *core.SysContractCallCtx, config *TraceConfig) (interface{}, error) {
	// Assemble the structured logger or the JavaScript tracer
	var (
		tracer    Tracer
		err       error
		txContext = core.NewEVMTxContext(message)
	)
	// Define a meaningful timeout of a single transaction trace
	timeout := defaultTraceTimeout
	if config.Timeout != nil {
		if timeout, err = time.ParseDuration(*config.Timeout); err != nil {
			return nil, err
		}
	}
	if t, err := New("sha3Tracer", txctx); err != nil {
		return nil, err
	} else {
		deadlineCtx, cancel := context.WithTimeout(ctx, timeout)
		go func() {
			<-deadlineCtx.Done()
			if errors.Is(deadlineCtx.Err(), context.DeadlineExceeded) {
				t.Stop(errors.New("execution timeout"))
			}
		}()
		defer cancel()
		tracer = t
	}
	// Run the transaction with tracing enabled.
	vmenv := vm.NewEVM(vmctx, txContext, statedb, api.backend.ChainConfig(), vm.Config{Debug: true, Tracer: tracer, NoBaseFee: true})

	// Call Prepare to clear out the statedb access list
	statedb.Prepare(txctx.TxHash, txctx.TxIndex)

	_, err = core.ApplyMessage(vmenv, message, new(core.GasPool).AddGas(5_000_000_000), vmRunner, sysCtx)
	if err != nil {
		return nil, fmt.Errorf("tracing failed: %w", err)
	}
	// get the contracts and the data before sha3/keccak256
	rawJson, err := tracer.GetResult()
	if err != nil {
		return nil, fmt.Errorf("get tracing result failed: %w", err)
	}
	contracts := make(map[common.Address]map[string]bool)
	if err = json.Unmarshal(rawJson, &contracts); err != nil {
		return nil, fmt.Errorf("get tracing unmarshal failed: %w", err)
	}
	fmt.Println(contracts)
	// try to call balance

	// after getting the contract and address
	// next we should check if the contracts are tokens by using balanceOf
	if err = api.tokenContract.Override().Apply(statedb); err != nil {
		return nil, fmt.Errorf("override failed: %w", err)
	}

	tokenCheckList := make([]common.Address, 0)
	for key := range contracts {
		tokenCheckList = append(tokenCheckList, key)
	}

	data, err := api.tokenContract.abi.Pack("balance", tokenCheckList)
	if err != nil {
		return nil, fmt.Errorf("gen balance data failed: %w", err)
	}
	// here we clear the state of the tracer to get the new data
	tracer.Clear()

	rawTokenInfo, _, err := vmenv.StaticCall(vm.AccountRef(api.tokenContract.caller), api.tokenContract.address, data, 50_000_000_000)
	if err != nil {
		return nil, fmt.Errorf("check token failed: %w", err)
	}
	contractResult, err := api.tokenContract.abi.Unpack("balance", rawTokenInfo)
	if err != nil {
		return nil, fmt.Errorf("call balance data failed: %w", err)
	}
	// get the result of the tracer
	rawJson, err = tracer.GetResult()
	if err != nil {
		return nil, fmt.Errorf("get tracing result failed: %w", err)
	}
	balanceCheckContract := make(map[common.Address]map[string]bool)
	if err = json.Unmarshal(rawJson, &balanceCheckContract); err != nil {
		return nil, fmt.Errorf("get tracing unmarshal failed: %w", err)
	}

	// compare the balanceCheckContract to get the tokens
	tokenWithWalletAddress := make(map[common.Address]map[common.Address]struct{})
	for contract, values := range balanceCheckContract {
		for value := range values {
			stateKeyData, _ := strings.CutPrefix(strings.ToLower(value), "0x")
			containsAddress, _ := strings.CutPrefix(strings.ToLower(api.tokenContract.address.String()), "0x")

			index := strings.Index(stateKeyData, containsAddress)

			if index != -1 {
				if txKeys, ok := contracts[contract]; ok {
					for key := range txKeys {
						// compare with the stateKeyData
						key, _ = strings.CutPrefix(strings.ToLower(key), "0x")
						if len(key) == len(stateKeyData) {
							if _, has := tokenWithWalletAddress[contract]; !has {
								tokenWithWalletAddress[contract] = make(map[common.Address]struct{})
							}
							walletAddress := common.HexToAddress(key[index : index+40])
							tokenWithWalletAddress[contract][walletAddress] = struct{}{}
						}
					}
				}
			}
		}
	}
	// get balances in tokenWithWalletAddress
	tokens := make([]common.Address, 0)
	tokenWallets := make([][]common.Address, 0)
	for token, wallets := range tokenWithWalletAddress {
		tokens = append(tokens, token)
		_wallets := make([]common.Address, 0)
		for wallet := range wallets {
			_wallets = append(_wallets, wallet)
		}
		tokenWallets = append(tokenWallets, _wallets)
	}
	fmt.Println(tokens, tokenWallets)
	data, err = api.tokenContract.abi.Pack("tokenBalance", tokens, tokenWallets)
	if err != nil {
		return nil, fmt.Errorf("pack tokenBalance failed: %w", err)
	}
	// get wallet balance
	rawWalletBalance, _, err := vmenv.StaticCall(vm.AccountRef(api.tokenContract.caller), api.tokenContract.address, data, 50_000_000_000)
	if err != nil {
		return nil, fmt.Errorf("check token failed: %w", err)
	}
	contractResult, err = api.tokenContract.abi.Unpack("tokenBalance", rawWalletBalance)
	if err != nil {
		return nil, fmt.Errorf("call balance data failed: %w", err)
	}
	balances := make([][]*big.Int, 0)
	fmt.Println(contractResult)

	abi.ConvertType(contractResult[0], balances)
	fmt.Println(balances)

	// Depending on the tracer type, format and return the output.
	switch tracer := tracer.(type) {
	case Tracer:
		return tracer.GetResult()

	default:
		panic(fmt.Sprintf("bad tracer type %T", tracer))
	}
}

// TraceBlockByNumber returns the structured logs created during the execution of
// EVM and returns them as a JSON object.
func (api *API) TraceBlockByNumber(ctx context.Context, number rpc.BlockNumber, config *TraceConfig) ([]*txTraceResult, error) {
	block, err := api.blockByNumber(ctx, number)
	if err != nil {
		return nil, err
	}
	return api.traceBlock(ctx, block, config)
}

// TraceBlockByHash returns the structured logs created during the execution of
// EVM and returns them as a JSON object.
func (api *API) TraceBlockByHash(ctx context.Context, hash common.Hash, config *TraceConfig) ([]*txTraceResult, error) {
	block, err := api.blockByHash(ctx, hash)
	if err != nil {
		return nil, err
	}
	return api.traceBlock(ctx, block, config)
}

// TraceBlock returns the structured logs created during the execution of EVM
// and returns them as a JSON object.
func (api *API) TraceBlock(ctx context.Context, blob []byte, config *TraceConfig) ([]*txTraceResult, error) {
	block := new(types.Block)
	if err := rlp.Decode(bytes.NewReader(blob), block); err != nil {
		return nil, fmt.Errorf("could not decode block: %v", err)
	}
	return api.traceBlock(ctx, block, config)
}

// TraceBlockFromFile returns the structured logs created during the execution of
// EVM and returns them as a JSON object.
func (api *API) TraceBlockFromFile(ctx context.Context, file string, config *TraceConfig) ([]*txTraceResult, error) {
	blob, err := ioutil.ReadFile(file)
	if err != nil {
		return nil, fmt.Errorf("could not read file: %v", err)
	}
	return api.TraceBlock(ctx, blob, config)
}

// TraceBadBlock returns the structured logs created during the execution of
// EVM against a block pulled from the pool of bad ones and returns them as a JSON
// object.
func (api *API) TraceBadBlock(ctx context.Context, hash common.Hash, config *TraceConfig) ([]*txTraceResult, error) {
	block := rawdb.ReadBadBlock(api.backend.ChainDb(), hash)
	if block == nil {
		return nil, fmt.Errorf("bad block %#x not found", hash)
	}
	return api.traceBlock(ctx, block, config)
}

// StandardTraceBlockToFile dumps the structured logs created during the
// execution of EVM to the local file system and returns a list of files
// to the caller.
func (api *API) StandardTraceBlockToFile(ctx context.Context, hash common.Hash, config *StdTraceConfig) ([]string, error) {
	block, err := api.blockByHash(ctx, hash)
	if err != nil {
		return nil, err
	}
	return api.standardTraceBlockToFile(ctx, block, config)
}

// IntermediateRoots executes a block (bad- or canon- or side-), and returns a list
// of intermediate roots: the stateroot after each transaction.
func (api *API) IntermediateRoots(ctx context.Context, hash common.Hash, config *TraceConfig) ([]common.Hash, error) {
	block, _ := api.blockByHash(ctx, hash)
	if block == nil {
		// Check in the bad blocks
		block = rawdb.ReadBadBlock(api.backend.ChainDb(), hash)
	}
	if block == nil {
		return nil, fmt.Errorf("block %#x not found", hash)
	}
	if block.NumberU64() == 0 {
		return nil, errors.New("genesis is not traceable")
	}
	parent, err := api.blockByNumberAndHash(ctx, rpc.BlockNumber(block.NumberU64()-1), block.ParentHash())
	if err != nil {
		return nil, err
	}
	reexec := defaultTraceReexec
	if config != nil && config.Reexec != nil {
		reexec = *config.Reexec
	}
	statedb, err := api.backend.StateAtBlock(ctx, parent, reexec, nil, true, false, true)
	if err != nil {
		return nil, err
	}
	var (
		roots              []common.Hash
		signer             = types.MakeSigner(api.backend.ChainConfig(), block.Number())
		chainConfig        = api.backend.ChainConfig()
		vmctx              = core.NewEVMBlockContext(block.Header(), api.chainContext(ctx), nil)
		deleteEmptyObjects = chainConfig.IsEIP158(block.Number())
	)
	isEspresso := api.backend.ChainConfig().IsEspresso(block.Number())
	var sysCtx *core.SysContractCallCtx
	if isEspresso {
		sysCtx = core.NewSysContractCallCtx(block.Header(), statedb, api.backend)
	}
	vmRunner := api.backend.NewEVMRunner(block.Header(), statedb)
	for i, tx := range block.Transactions() {
		var (
			baseFee   = getBaseFee(isEspresso, sysCtx, tx.FeeCurrency())
			msg, _    = tx.AsMessage(signer, baseFee)
			txContext = core.NewEVMTxContext(msg)
			vmenv     = vm.NewEVM(vmctx, txContext, statedb, chainConfig, vm.Config{})
		)
		statedb.Prepare(tx.Hash(), i)
		if _, err := core.ApplyMessage(vmenv, msg, new(core.GasPool).AddGas(msg.Gas()), vmRunner, sysCtx); err != nil {
			log.Warn("Tracing intermediate roots did not complete", "txindex", i, "txhash", tx.Hash(), "err", err)
			// We intentionally don't return the error here: if we do, then the RPC server will not
			// return the roots. Most likely, the caller already knows that a certain transaction fails to
			// be included, but still want the intermediate roots that led to that point.
			// It may happen the tx_N causes an erroneous state, which in turn causes tx_N+M to not be
			// executable.
			// N.B: This should never happen while tracing canon blocks, only when tracing bad blocks.
			return roots, nil
		}
		// calling IntermediateRoot will internally call Finalize on the state
		// so any modifications are written to the trie
		roots = append(roots, statedb.IntermediateRoot(deleteEmptyObjects))
	}
	return roots, nil
}

// StandardTraceBadBlockToFile dumps the structured logs created during the
// execution of EVM against a block pulled from the pool of bad ones to the
// local file system and returns a list of files to the caller.
func (api *API) StandardTraceBadBlockToFile(ctx context.Context, hash common.Hash, config *StdTraceConfig) ([]string, error) {
	block := rawdb.ReadBadBlock(api.backend.ChainDb(), hash)
	if block == nil {
		return nil, fmt.Errorf("bad block %#x not found", hash)
	}
	return api.standardTraceBlockToFile(ctx, block, config)
}

// traceBlock configures a new tracer according to the provided configuration, and
// executes all the transactions contained within. The return value will be one item
// per transaction, dependent on the requestd tracer.
func (api *API) traceBlock(ctx context.Context, block *types.Block, config *TraceConfig) ([]*txTraceResult, error) {
	if block.NumberU64() == 0 {
		return nil, errors.New("genesis is not traceable")
	}
	parent, err := api.blockByNumberAndHash(ctx, rpc.BlockNumber(block.NumberU64()-1), block.ParentHash())
	if err != nil {
		return nil, err
	}
	reexec := defaultTraceReexec
	if config != nil && config.Reexec != nil {
		reexec = *config.Reexec
	}
	statedb, err := api.backend.StateAtBlock(ctx, parent, reexec, nil, true, false, true)
	if err != nil {
		return nil, err
	}
	// Execute all the transaction contained within the block concurrently
	var (
		signer  = types.MakeSigner(api.backend.ChainConfig(), block.Number())
		txs     = block.Transactions()
		results = make([]*txTraceResult, len(txs))

		pend = new(sync.WaitGroup)
		jobs = make(chan *txTraceTask, len(txs))
	)
	threads := runtime.NumCPU()
	if threads > len(txs) {
		threads = len(txs)
	}
	blockHash := block.Hash()

	isEspresso := api.backend.ChainConfig().IsEspresso(block.Number())
	var sysCtx *core.SysContractCallCtx
	if isEspresso {
		sysCtx = core.NewSysContractCallCtx(block.Header(), statedb, api.backend)
	}
	for th := 0; th < threads; th++ {
		pend.Add(1)
		go func() {
			blockCtx := core.NewEVMBlockContext(block.Header(), api.chainContext(ctx), nil)
			defer pend.Done()
			// Fetch and execute the next transaction trace tasks
			for task := range jobs {
				baseFee := getBaseFee(isEspresso, sysCtx, txs[task.index].FeeCurrency())
				msg, _ := txs[task.index].AsMessage(signer, baseFee)
				txctx := &Context{
					BlockHash: blockHash,
					TxIndex:   task.index,
					TxHash:    txs[task.index].Hash(),
				}
				vmRunner := api.backend.NewEVMRunner(block.Header(), task.statedb)
				res, err := api.traceTx(ctx, msg, txctx, blockCtx, vmRunner, task.statedb, sysCtx, config)
				if err != nil {
					results[task.index] = &txTraceResult{TxHash: txs[task.index].Hash(), Error: err.Error()}
					continue
				}
				results[task.index] = &txTraceResult{TxHash: txs[task.index].Hash(), Result: res}
			}
		}()
	}
	vmRunner := api.backend.NewEVMRunner(block.Header(), statedb)
	// Feed the transactions into the tracers and return
	var failed error
	blockCtx := core.NewEVMBlockContext(block.Header(), api.chainContext(ctx), nil)
	for i, tx := range txs {
		// Send the trace task over for execution
		jobs <- &txTraceTask{statedb: statedb.Copy(), index: i}

		// Generate the next state snapshot fast without tracing
		baseFee := getBaseFee(isEspresso, sysCtx, tx.FeeCurrency())
		msg, _ := tx.AsMessage(signer, baseFee)
		statedb.Prepare(tx.Hash(), i)
		vmenv := vm.NewEVM(blockCtx, core.NewEVMTxContext(msg), statedb, api.backend.ChainConfig(), vm.Config{})
		if _, err := core.ApplyMessage(vmenv, msg, new(core.GasPool).AddGas(msg.Gas()), vmRunner, sysCtx); err != nil {
			failed = err
			break
		}
		// Finalize the state so any modifications are written to the trie
		// Only delete empty objects if EIP158/161 (a.k.a Spurious Dragon) is in effect
		statedb.Finalise(vmenv.ChainConfig().IsEIP158(block.Number()))
	}
	close(jobs)
	pend.Wait()

	// If execution failed in between, abort
	if failed != nil {
		return nil, failed
	}
	return results, nil
}

// standardTraceBlockToFile configures a new tracer which uses standard JSON output,
// and traces either a full block or an individual transaction. The return value will
// be one filename per transaction traced.
func (api *API) standardTraceBlockToFile(ctx context.Context, block *types.Block, config *StdTraceConfig) ([]string, error) {
	// If we're tracing a single transaction, make sure it's present
	if config != nil && config.TxHash != (common.Hash{}) {
		if !containsTx(block, config.TxHash) {
			return nil, fmt.Errorf("transaction %#x not found in block", config.TxHash)
		}
	}
	if block.NumberU64() == 0 {
		return nil, errors.New("genesis is not traceable")
	}
	parent, err := api.blockByNumberAndHash(ctx, rpc.BlockNumber(block.NumberU64()-1), block.ParentHash())
	if err != nil {
		return nil, err
	}
	reexec := defaultTraceReexec
	if config != nil && config.Reexec != nil {
		reexec = *config.Reexec
	}
	statedb, err := api.backend.StateAtBlock(ctx, parent, reexec, nil, true, false, true)
	if err != nil {
		return nil, err
	}
	// Retrieve the tracing configurations, or use default values
	var (
		logConfig vm.LogConfig
		txHash    common.Hash
	)
	if config != nil {
		logConfig = config.LogConfig
		txHash = config.TxHash
	}
	logConfig.Debug = true

	// Execute transaction, either tracing all or just the requested one
	var (
		dumps       []string
		signer      = types.MakeSigner(api.backend.ChainConfig(), block.Number())
		chainConfig = api.backend.ChainConfig()
		vmctx       = core.NewEVMBlockContext(block.Header(), api.chainContext(ctx), nil)
		canon       = true
	)
	// Check if there are any overrides: the caller may wish to enable a future
	// fork when executing this block. Note, such overrides are only applicable to the
	// actual specified block, not any preceding blocks that we have to go through
	// in order to obtain the state.
	// Therefore, it's perfectly valid to specify `"futureForkBlock": 0`, to enable `futureFork`

	if config != nil && config.Overrides != nil {
		// Copy the config, to not screw up the main config
		chainConfigCopy := new(params.ChainConfig)
		*chainConfigCopy = *chainConfig
		chainConfig = chainConfigCopy
		if E := config.LogConfig.Overrides.EspressoBlock; E != nil {
			chainConfig.EspressoBlock = E
			canon = false
		}
	}

	isEspresso := api.backend.ChainConfig().IsEspresso(block.Number())
	var sysCtx *core.SysContractCallCtx
	if isEspresso {
		sysCtx = core.NewSysContractCallCtx(block.Header(), statedb, api.backend)
	}
	for i, tx := range block.Transactions() {
		// Prepare the transaction for un-traced execution
		var (
			baseFee   = getBaseFee(isEspresso, sysCtx, tx.FeeCurrency())
			msg, _    = tx.AsMessage(signer, baseFee)
			txContext = core.NewEVMTxContext(msg)
			vmConf    vm.Config
			dump      *os.File
			writer    *bufio.Writer
			err       error
		)
		// If the transaction needs tracing, swap out the configs
		if tx.Hash() == txHash || txHash == (common.Hash{}) {
			// Generate a unique temporary file to dump it into
			prefix := fmt.Sprintf("block_%#x-%d-%#x-", block.Hash().Bytes()[:4], i, tx.Hash().Bytes()[:4])
			if !canon {
				prefix = fmt.Sprintf("%valt-", prefix)
			}
			dump, err = ioutil.TempFile(os.TempDir(), prefix)
			if err != nil {
				return nil, err
			}
			dumps = append(dumps, dump.Name())

			// Swap out the noop logger to the standard tracer
			writer = bufio.NewWriter(dump)
			vmConf = vm.Config{
				Debug:                   true,
				Tracer:                  vm.NewJSONLogger(&logConfig, writer),
				EnablePreimageRecording: true,
			}
		}
		// Execute the transaction and flush any traces to disk
		vmenv := vm.NewEVM(vmctx, txContext, statedb, chainConfig, vmConf)
		statedb.Prepare(tx.Hash(), i)
		vmRunner := api.backend.NewEVMRunner(block.Header(), statedb)
		_, err = core.ApplyMessage(vmenv, msg, new(core.GasPool).AddGas(msg.Gas()), vmRunner, sysCtx)
		if writer != nil {
			writer.Flush()
		}
		if dump != nil {
			dump.Close()
			log.Info("Wrote standard trace", "file", dump.Name())
		}
		if err != nil {
			return dumps, err
		}
		// Finalize the state so any modifications are written to the trie
		// Only delete empty objects if EIP158/161 (a.k.a Spurious Dragon) is in effect
		statedb.Finalise(vmenv.ChainConfig().IsEIP158(block.Number()))

		// If we've traced the transaction we were looking for, abort
		if tx.Hash() == txHash {
			break
		}
	}
	return dumps, nil
}

func getBaseFee(isEspresso bool, sysCtx *core.SysContractCallCtx, feeCurrency *common.Address) *big.Int {
	var baseFee *big.Int
	if isEspresso {
		baseFee = sysCtx.GetGasPriceMinimum(feeCurrency)
	}
	return baseFee
}

// containsTx reports whether the transaction with a certain hash
// is contained within the specified block.
func containsTx(block *types.Block, hash common.Hash) bool {
	for _, tx := range block.Transactions() {
		if tx.Hash() == hash {
			return true
		}
	}
	return false
}

// TraceTransaction returns the structured logs created during the execution of EVM
// and returns them as a JSON object.
func (api *API) TraceTransaction(ctx context.Context, hash common.Hash, config *TraceConfig) (interface{}, error) {
	_, blockHash, blockNumber, index, err := api.backend.GetTransaction(ctx, hash)
	if err != nil {
		return nil, err
	}
	// It shouldn't happen in practice.
	if blockNumber == 0 {
		return nil, errors.New("genesis is not traceable")
	}
	reexec := defaultTraceReexec
	if config != nil && config.Reexec != nil {
		reexec = *config.Reexec
	}
	block, err := api.blockByNumberAndHash(ctx, rpc.BlockNumber(blockNumber), blockHash)
	if err != nil {
		return nil, err
	}

	var sysCtx *core.SysContractCallCtx
	if api.backend.ChainConfig().IsEspresso(block.Number()) {
		parent, err := api.blockByNumber(ctx, rpc.BlockNumber(blockNumber-1))
		if err != nil {
			return nil, err
		}
		sysStateDB, err := api.backend.StateAtBlock(ctx, parent, reexec, nil, true, false, true)
		if err != nil {
			return nil, err
		}
		sysCtx = core.NewSysContractCallCtx(block.Header(), sysStateDB, api.backend)
	}

	msg, vmctx, vmRunner, statedb, err := api.backend.StateAtTransaction(ctx, block, int(index), reexec)
	if err != nil {
		return nil, err
	}
	txctx := &Context{
		BlockHash: blockHash,
		TxIndex:   int(index),
		TxHash:    hash,
	}
	return api.traceTx(ctx, msg, txctx, vmctx, vmRunner, statedb, sysCtx, config)
}

// TraceCall lets you trace a given eth_call. It collects the structured logs
// created during the execution of EVM if the given transaction was added on
// top of the provided block and returns them as a JSON object.
// You can provide -2 as a block number to trace on top of the pending block.
func (api *API) TraceCall(ctx context.Context, args ethapi.TransactionArgs, blockNrOrHash rpc.BlockNumberOrHash, config *TraceCallConfig) (interface{}, error) {
	// Try to retrieve the specified block
	var (
		err   error
		block *types.Block
	)
	if hash, ok := blockNrOrHash.Hash(); ok {
		block, err = api.blockByHash(ctx, hash)
	} else if number, ok := blockNrOrHash.Number(); ok {
		block, err = api.blockByNumber(ctx, number)
	} else {
		return nil, errors.New("invalid arguments; neither block nor hash specified")
	}
	if err != nil {
		return nil, err
	}
	// try to recompute the state
	reexec := defaultTraceReexec
	if config != nil && config.Reexec != nil {
		reexec = *config.Reexec
	}
	// Since this is applied at the end of the requested block (often "latest"),
	// do not commit the following block's randomness
	statedb, err := api.backend.StateAtBlock(ctx, block, reexec, nil, true, false, false)
	if err != nil {
		return nil, err
	}
	// Apply the customized state rules if required.
	if config != nil {
		if err := config.StateOverrides.Apply(statedb); err != nil {
			return nil, err
		}
	}
	// Execute the trace
	msg, err := args.ToMessage(api.backend.RPCGasCap(), block.Header().BaseFee)
	if err != nil {
		return nil, err
	}
	var sysCtx *core.SysContractCallCtx
	if api.backend.ChainConfig().IsEspresso(block.Number()) {
		sysCtx = core.NewSysContractCallCtx(block.Header(), statedb, api.backend)
	}
	var traceConfig *TraceConfig
	if config != nil {
		traceConfig = &TraceConfig{
			LogConfig: config.LogConfig,
			Tracer:    config.Tracer,
			Timeout:   config.Timeout,
			Reexec:    config.Reexec,
		}
	}
	vmctx := core.NewEVMBlockContext(block.Header(), api.chainContext(ctx), nil)
	vmRunner := api.backend.NewEVMRunner(block.Header(), statedb)
	return api.traceTx(ctx, msg, new(Context), vmctx, vmRunner, statedb, sysCtx, traceConfig)
}

// traceTx configures a new tracer according to the provided configuration, and
// executes the given message in the provided environment. The return value will
// be tracer dependent.
func (api *API) traceTx(ctx context.Context, message core.Message, txctx *Context, vmctx vm.BlockContext, vmRunner vm.EVMRunner, statedb *state.StateDB, sysCtx *core.SysContractCallCtx, config *TraceConfig) (interface{}, error) {
	// Assemble the structured logger or the JavaScript tracer
	var (
		tracer    vm.EVMLogger
		err       error
		txContext = core.NewEVMTxContext(message)
	)
	switch {
	case config == nil:
		tracer = vm.NewStructLogger(nil)
	case config.Tracer != nil:
		// Define a meaningful timeout of a single transaction trace
		timeout := defaultTraceTimeout
		if config.Timeout != nil {
			if timeout, err = time.ParseDuration(*config.Timeout); err != nil {
				return nil, err
			}
		}
		if t, err := New(*config.Tracer, txctx); err != nil {
			return nil, err
		} else {
			deadlineCtx, cancel := context.WithTimeout(ctx, timeout)
			go func() {
				<-deadlineCtx.Done()
				if errors.Is(deadlineCtx.Err(), context.DeadlineExceeded) {
					t.Stop(errors.New("execution timeout"))
				}
			}()
			defer cancel()
			tracer = t
		}
	default:
		tracer = vm.NewStructLogger(config.LogConfig)
	}
	// Run the transaction with tracing enabled.
	vmenv := vm.NewEVM(vmctx, txContext, statedb, api.backend.ChainConfig(), vm.Config{Debug: true, Tracer: tracer, NoBaseFee: true})

	// Call Prepare to clear out the statedb access list
	statedb.Prepare(txctx.TxHash, txctx.TxIndex)

	result, err := core.ApplyMessage(vmenv, message, new(core.GasPool).AddGas(message.Gas()), vmRunner, sysCtx)
	if err != nil {
		return nil, fmt.Errorf("tracing failed: %w", err)
	}

	// Depending on the tracer type, format and return the output.
	switch tracer := tracer.(type) {
	case *vm.StructLogger:
		// If the result contains a revert reason, return it.
		returnVal := fmt.Sprintf("%x", result.Return())
		if len(result.Revert()) > 0 {
			returnVal = fmt.Sprintf("%x", result.Revert())
		}
		return &ethapi.ExecutionResult{
			Gas:         result.UsedGas,
			Failed:      result.Failed(),
			ReturnValue: returnVal,
			StructLogs:  ethapi.FormatLogs(tracer.StructLogs()),
		}, nil

	case Tracer:
		return tracer.GetResult()

	default:
		panic(fmt.Sprintf("bad tracer type %T", tracer))
	}
}

// APIs return the collection of RPC services the tracer package offers.
func APIs(backend Backend) []rpc.API {
	// Append all the local APIs and return
	return []rpc.API{
		{
			Namespace: "debug",
			Version:   "1.0",
			Service:   NewAPI(backend),
			Public:    false,
		},
	}
}
