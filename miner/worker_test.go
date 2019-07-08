// Copyright 2018 The go-ethereum Authors
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

package miner

import (
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/consensus/clique"
	"github.com/ethereum/go-ethereum/consensus/ethash"
	"github.com/ethereum/go-ethereum/consensus/istanbul"
	istanbulBackend "github.com/ethereum/go-ethereum/consensus/istanbul/backend"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/event"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/userspace_communication"
)

var (
	// Test chain configurations
	testTxPoolConfig    core.TxPoolConfig
	ethashChainConfig   *params.ChainConfig
	cliqueChainConfig   *params.ChainConfig
	istanbulChainConfig *params.ChainConfig

	// Test accounts
	testBankKey, _  = crypto.GenerateKey()
	testBankAddress = crypto.PubkeyToAddress(testBankKey.PublicKey)
	testBankFunds   = big.NewInt(1000000000000000000)

	testUserKey, _  = crypto.GenerateKey()
	testUserAddress = crypto.PubkeyToAddress(testUserKey.PublicKey)

	testVerificationService = ""

	// Test transactions
	pendingTxs []*types.Transaction
	newTxs     []*types.Transaction
)

func init() {
	testTxPoolConfig = core.DefaultTxPoolConfig
	testTxPoolConfig.Journal = ""
	ethashChainConfig = params.TestChainConfig
	cliqueChainConfig = params.TestChainConfig
	cliqueChainConfig.Clique = &params.CliqueConfig{
		Period: 10,
		Epoch:  30000,
	}
	istanbulChainConfig = params.TestChainConfig
	istanbulChainConfig.Istanbul = &params.IstanbulConfig{
		Epoch:          30000,
		ProposerPolicy: 0,
	}

	tx1, _ := types.SignTx(types.NewTransaction(0, testUserAddress, big.NewInt(1000), params.TxGas, nil, nil, nil, nil), types.HomesteadSigner{}, testBankKey)
	pendingTxs = append(pendingTxs, tx1)
	tx2, _ := types.SignTx(types.NewTransaction(1, testUserAddress, big.NewInt(1000), params.TxGas, nil, nil, nil, nil), types.HomesteadSigner{}, testBankKey)
	newTxs = append(newTxs, tx2)
}

// testWorkerBackend implements worker.Backend interfaces and wraps all information needed during the testing.
type testWorkerBackend struct {
	accountManager *accounts.Manager
	db             ethdb.Database
	txPool         *core.TxPool
	chain          *core.BlockChain
	testTxFeed     event.Feed
	uncleBlock     *types.Block
	regAdd         *core.RegisteredAddresses
	gcWl           *core.GasCurrencyWhitelist
	gpm            *core.GasPriceMinimum
}

func newTestWorkerBackend(t *testing.T, chainConfig *params.ChainConfig, engine consensus.Engine, n int) *testWorkerBackend {
	var (
		db    = ethdb.NewMemDatabase()
		gspec = core.Genesis{
			Config: chainConfig,
			Alloc:  core.GenesisAlloc{testBankAddress: {Balance: testBankFunds}},
		}
	)

	switch engine.(type) {
	case *clique.Clique:
		gspec.ExtraData = make([]byte, 52+common.AddressLength+65)
		copy(gspec.ExtraData[52:], testBankAddress[:])
		copy(gspec.ExtraData[52:], testBankAddress[:])
	case *ethash.Ethash:
	case *istanbulBackend.Backend:
		addrs := []common.Address{testBankAddress}
		istanbulBackend.AppendValidatorsToGenesisBlock(&gspec, addrs)

		gspec.Mixhash = types.IstanbulDigest
		gspec.Difficulty = big.NewInt(1)
	default:
		t.Fatalf("unexpected consensus engine type: %T", engine)
	}
	genesis := gspec.MustCommit(db)

	chain, _ := core.NewBlockChain(db, nil, gspec.Config, engine, vm.Config{}, nil)
	userspace_communication.SetInternalEVMHandler(chain)

	regAdd := core.NewRegisteredAddresses()
	gcWl := core.NewGasCurrencyWhitelist(regAdd)
	gpm := core.NewGasPriceMinimum(regAdd)
	co := core.NewCurrencyOperator(gcWl, regAdd)

	txpool := core.NewTxPool(testTxPoolConfig, chainConfig, chain, co, nil)

	// If istanbul engine used, set the regAddr objects in that engine
	if istanbul, ok := engine.(consensus.Istanbul); ok {
		istanbul.SetRegisteredAddresses(regAdd)
		istanbul.SetGasPriceMinimum(gpm)
		istanbul.SetChain(chain, chain.CurrentBlock)
	}

	// Generate a small n-block chain and an uncle block for it
	if n > 0 {
		blocks, _ := core.GenerateChain(chainConfig, genesis, engine, db, n, func(i int, gen *core.BlockGen) {
			gen.SetCoinbase(testBankAddress)
		})
		if _, err := chain.InsertChain(blocks); err != nil {
			t.Fatalf("failed to insert origin chain: %v", err)
		}
	}
	parent := genesis
	if n > 0 {
		parent = chain.GetBlockByHash(chain.CurrentBlock().ParentHash())
	}
	blocks, _ := core.GenerateChain(chainConfig, parent, engine, db, 1, func(i int, gen *core.BlockGen) {
		gen.SetCoinbase(testUserAddress)
	})
	var backends []accounts.Backend
	accountManager := accounts.NewManager(backends...)

	return &testWorkerBackend{
		accountManager: accountManager,
		db:             db,
		chain:          chain,
		txPool:         txpool,
		uncleBlock:     blocks[0],
		gcWl:           gcWl,
		regAdd:         regAdd,
		gpm:            gpm,
	}
}

func (b *testWorkerBackend) AccountManager() *accounts.Manager { return b.accountManager }
func (b *testWorkerBackend) BlockChain() *core.BlockChain      { return b.chain }
func (b *testWorkerBackend) TxPool() *core.TxPool              { return b.txPool }
func (b *testWorkerBackend) PostChainEvents(events []interface{}) {
	b.chain.PostChainEvents(events, nil)
}
func (b *testWorkerBackend) GasCurrencyWhitelist() *core.GasCurrencyWhitelist { return b.gcWl }
func (b *testWorkerBackend) RegisteredAddresses() *core.RegisteredAddresses   { return b.regAdd }
func (b *testWorkerBackend) GasPriceMinimum() *core.GasPriceMinimum           { return b.gpm }

func newTestWorker(t *testing.T, chainConfig *params.ChainConfig, engine consensus.Engine, blocks int, shouldAddPendingTxs bool) (*worker, *testWorkerBackend) {
	backend := newTestWorkerBackend(t, chainConfig, engine, blocks)
	if shouldAddPendingTxs {
		backend.txPool.AddLocals(pendingTxs)
	}
	co := core.NewCurrencyOperator(nil, nil)
	random := core.NewRandom(backend.regAdd)
	w := newWorker(chainConfig, engine, backend, new(event.TypeMux), time.Second, params.GenesisGasLimit, params.GenesisGasLimit, nil, testVerificationService, co, random, &backend.db)
	w.setEtherbase(testBankAddress)
	return w, backend
}

func TestPendingStateAndBlockEthash(t *testing.T) {
	testPendingStateAndBlock(t, ethashChainConfig, ethash.NewFaker())
}
func TestPendingStateAndBlockClique(t *testing.T) {
	testPendingStateAndBlock(t, cliqueChainConfig, clique.New(cliqueChainConfig.Clique, ethdb.NewMemDatabase()))
}

func getAuthorizedIstanbulEngine() consensus.Istanbul {

	signerFn := func(_ accounts.Account, data []byte) ([]byte, error) {
		return crypto.Sign(data, testBankKey)
	}

	engine := istanbulBackend.New(istanbul.DefaultConfig, ethdb.NewMemDatabase())
	engine.(*istanbulBackend.Backend).Authorize(crypto.PubkeyToAddress(testBankKey.PublicKey), signerFn)
	return engine
}

func TestPendingStateAndBlockIstanbul(t *testing.T) {
	testPendingStateAndBlock(t, cliqueChainConfig, getAuthorizedIstanbulEngine())
}

func testPendingStateAndBlock(t *testing.T, chainConfig *params.ChainConfig, engine consensus.Engine) {
	defer engine.Close()

	w, b := newTestWorker(t, chainConfig, engine, 0, true)
	defer w.close()

	// Ensure snapshot has been updated.
	time.Sleep(100 * time.Millisecond)
	block, state := w.pending()
	if block.NumberU64() != 1 {
		t.Errorf("block number mismatch: have %d, want %d", block.NumberU64(), 1)
	}
	if balance := state.GetBalance(testUserAddress); balance.Cmp(big.NewInt(1000)) != 0 {
		t.Errorf("account balance mismatch: have %d, want %d", balance, 1000)
	}
	b.txPool.AddLocals(newTxs)

	// Ensure the new tx events has been processed
	time.Sleep(100 * time.Millisecond)
	block, state = w.pending()
	if balance := state.GetBalance(testUserAddress); balance.Cmp(big.NewInt(2000)) != 0 {
		t.Errorf("account balance mismatch: have %d, want %d", balance, 2000)
	}
}

func TestEmptyWorkEthash(t *testing.T) {
	// TODO(nambrot): Fix this
	t.Skip("Disabled due to flakyness")
	testEmptyWork(t, ethashChainConfig, ethash.NewFaker(), true, true)
	testEmptyWork(t, ethashChainConfig, ethash.NewFaker(), true, false)
}
func TestEmptyWorkClique(t *testing.T) {
	testEmptyWork(t, cliqueChainConfig, clique.New(cliqueChainConfig.Clique, ethdb.NewMemDatabase()), true, true)
	testEmptyWork(t, cliqueChainConfig, clique.New(cliqueChainConfig.Clique, ethdb.NewMemDatabase()), true, false)
}

func TestEmptyWorkIstanbul(t *testing.T) {
	// TODO(nambrot): Fix this
	t.Skip("Disabled due to flakyness")
	testEmptyWork(t, istanbulChainConfig, getAuthorizedIstanbulEngine(), false, true)
	testEmptyWork(t, istanbulChainConfig, getAuthorizedIstanbulEngine(), true, false)
}

func testEmptyWork(t *testing.T, chainConfig *params.ChainConfig, engine consensus.Engine, expectEmptyBlock bool, shouldAddPendingTxs bool) {
	defer engine.Close()

	w, _ := newTestWorker(t, chainConfig, engine, 0, shouldAddPendingTxs)
	defer w.close()

	var (
		taskCh    = make(chan struct{}, 2)
		taskIndex int
	)

	checkEqual := func(t *testing.T, task *task, index int) {
		receiptLen, balance := 0, big.NewInt(0)

		if !expectEmptyBlock && len(task.block.Body().Transactions) == 0 {
			t.Errorf("Should have not received an empty block")
		}

		if !expectEmptyBlock || (index == 1 && shouldAddPendingTxs) {
			receiptLen, balance = 1, big.NewInt(1000)
		}

		if len(task.receipts) != receiptLen {
			t.Errorf("receipt number mismatch: have %d, want %d", len(task.receipts), receiptLen)
		}
		if task.state.GetBalance(testUserAddress).Cmp(balance) != 0 {
			t.Errorf("account balance mismatch: have %d, want %d", task.state.GetBalance(testUserAddress), balance)
		}
	}

	w.newTaskHook = func(task *task) {
		if task.block.NumberU64() == 1 {
			checkEqual(t, task, taskIndex)
			taskIndex += 1
			taskCh <- struct{}{}
		}
	}
	w.fullTaskHook = func() {
		time.Sleep(100 * time.Millisecond)
	}
	// Ensure worker has finished initialization
	for {
		b := w.pendingBlock()
		if b != nil && b.NumberU64() == 1 {
			break
		}
	}

	w.start()
	expectedTasksLen := 1
	if shouldAddPendingTxs && expectEmptyBlock {
		expectedTasksLen = 2
	}
	for i := 0; i < expectedTasksLen; i += 1 {
		select {
		case <-taskCh:
		case <-time.NewTimer(time.Second).C:
			t.Error("new task timeout")
		}
	}

	select {
	case <-taskCh:
		t.Error("should have not received another task")
	case <-time.NewTimer(time.Second).C:
	}
}

func TestStreamUncleBlock(t *testing.T) {
	ethash := ethash.NewFaker()
	defer ethash.Close()

	w, b := newTestWorker(t, ethashChainConfig, ethash, 1, true)
	defer w.close()

	var taskCh = make(chan struct{})

	taskIndex := 0
	w.newTaskHook = func(task *task) {
		if task.block.NumberU64() == 2 {
			if taskIndex == 2 {
				have := task.block.Header().UncleHash
				want := types.CalcUncleHash([]*types.Header{b.uncleBlock.Header()})
				if have != want {
					t.Errorf("uncle hash mismatch: have %s, want %s", have.Hex(), want.Hex())
				}
			}
			taskCh <- struct{}{}
			taskIndex += 1
		}
	}
	w.skipSealHook = func(task *task) bool {
		return true
	}
	w.fullTaskHook = func() {
		time.Sleep(100 * time.Millisecond)
	}

	// Ensure worker has finished initialization
	for {
		b := w.pendingBlock()
		if b != nil && b.NumberU64() == 2 {
			break
		}
	}
	w.start()

	// Ignore the first two works
	for i := 0; i < 2; i += 1 {
		select {
		case <-taskCh:
		case <-time.NewTimer(time.Second).C:
			t.Error("new task timeout")
		}
	}
	b.PostChainEvents([]interface{}{core.ChainSideEvent{Block: b.uncleBlock}})

	select {
	case <-taskCh:
	case <-time.NewTimer(time.Second).C:
		t.Error("new task timeout")
	}
}

func TestRegenerateMiningBlockEthash(t *testing.T) {
	testRegenerateMiningBlock(t, ethashChainConfig, ethash.NewFaker())
}

func TestRegenerateMiningBlockClique(t *testing.T) {
	testRegenerateMiningBlock(t, cliqueChainConfig, clique.New(cliqueChainConfig.Clique, ethdb.NewMemDatabase()))
}

// For Ethhash and Clique, it is safe and even desired to start another seal process in the presence of new transactions
// that potentially increase the fee revenue for the sealer. In Istanbul, that is not possible and even counter productive
// as proposing another block after having already done so is clearly byzantine behavior.
func TestRegenerateMiningBlockIstanbul(t *testing.T) {
	chainConfig := istanbulChainConfig
	engine := getAuthorizedIstanbulEngine()

	defer engine.Close()

	w, b := newTestWorker(t, chainConfig, engine, 0, true)
	defer w.close()

	var taskCh = make(chan struct{})

	taskIndex := 0
	w.newTaskHook = func(task *task) {
		if task.block.NumberU64() == 1 {
			receiptLen, balance := 1, big.NewInt(1000)
			if len(task.receipts) != receiptLen {
				t.Errorf("receipt number mismatch: have %d, want %d", len(task.receipts), receiptLen)
			}
			if task.state.GetBalance(testUserAddress).Cmp(balance) != 0 {
				t.Errorf("account balance mismatch: have %d, want %d", task.state.GetBalance(testUserAddress), balance)
			}
			taskCh <- struct{}{}
			taskIndex += 1
		}
	}
	w.skipSealHook = func(task *task) bool {
		return true
	}
	w.fullTaskHook = func() {
		time.Sleep(100 * time.Millisecond)
	}
	// Ensure worker has finished initialization
	for {
		b := w.pendingBlock()
		if b != nil && b.NumberU64() == 1 {
			break
		}
	}

	w.start()
	// expect one work
	select {
	case <-taskCh:
	case <-time.NewTimer(time.Second).C:
		t.Error("new task timeout")
	}

	b.txPool.AddLocals(newTxs)
	time.Sleep(time.Second)

	select {
	case <-taskCh:
		t.Error("Should have not received another task")
	case <-time.NewTimer(time.Second).C:
	}
}

func testRegenerateMiningBlock(t *testing.T, chainConfig *params.ChainConfig, engine consensus.Engine) {
	defer engine.Close()

	w, b := newTestWorker(t, chainConfig, engine, 0, true)
	defer w.close()

	var taskCh = make(chan struct{})

	taskIndex := 0
	w.newTaskHook = func(task *task) {
		if task.block.NumberU64() == 1 {
			if taskIndex == 2 {
				receiptLen, balance := 2, big.NewInt(2000)
				if len(task.receipts) != receiptLen {
					t.Errorf("receipt number mismatch: have %d, want %d", len(task.receipts), receiptLen)
				}
				if task.state.GetBalance(testUserAddress).Cmp(balance) != 0 {
					t.Errorf("account balance mismatch: have %d, want %d", task.state.GetBalance(testUserAddress), balance)
				}
			}
			taskCh <- struct{}{}
			taskIndex += 1
		}
	}
	w.skipSealHook = func(task *task) bool {
		return true
	}
	w.fullTaskHook = func() {
		time.Sleep(100 * time.Millisecond)
	}
	// Ensure worker has finished initialization
	for {
		b := w.pendingBlock()
		if b != nil && b.NumberU64() == 1 {
			break
		}
	}

	w.start()
	// Ignore the first two works
	for i := 0; i < 2; i += 1 {
		select {
		case <-taskCh:
		case <-time.NewTimer(time.Second).C:
			t.Error("new task timeout")
		}
	}
	b.txPool.AddLocals(newTxs)
	time.Sleep(time.Second)

	select {
	case <-taskCh:
	case <-time.NewTimer(time.Second).C:
		t.Error("new task timeout")
	}
}

func TestAdjustIntervalEthash(t *testing.T) {
	testAdjustInterval(t, ethashChainConfig, ethash.NewFaker())
}

func TestAdjustIntervalClique(t *testing.T) {
	testAdjustInterval(t, cliqueChainConfig, clique.New(cliqueChainConfig.Clique, ethdb.NewMemDatabase()))
}

func testAdjustInterval(t *testing.T, chainConfig *params.ChainConfig, engine consensus.Engine) {
	defer engine.Close()

	w, _ := newTestWorker(t, chainConfig, engine, 0, true)
	defer w.close()

	w.skipSealHook = func(task *task) bool {
		return true
	}
	w.fullTaskHook = func() {
		time.Sleep(100 * time.Millisecond)
	}
	var (
		progress = make(chan struct{}, 10)
		result   = make([]float64, 0, 10)
		index    = 0
		start    = false
	)
	w.resubmitHook = func(minInterval time.Duration, recommitInterval time.Duration) {
		// Short circuit if interval checking hasn't started.
		if !start {
			return
		}
		var wantMinInterval, wantRecommitInterval time.Duration

		switch index {
		case 0:
			wantMinInterval, wantRecommitInterval = 3*time.Second, 3*time.Second
		case 1:
			origin := float64(3 * time.Second.Nanoseconds())
			estimate := origin*(1-intervalAdjustRatio) + intervalAdjustRatio*(origin/0.8+intervalAdjustBias)
			wantMinInterval, wantRecommitInterval = 3*time.Second, time.Duration(estimate)*time.Nanosecond
		case 2:
			estimate := result[index-1]
			min := float64(3 * time.Second.Nanoseconds())
			estimate = estimate*(1-intervalAdjustRatio) + intervalAdjustRatio*(min-intervalAdjustBias)
			wantMinInterval, wantRecommitInterval = 3*time.Second, time.Duration(estimate)*time.Nanosecond
		case 3:
			wantMinInterval, wantRecommitInterval = time.Second, time.Second
		}

		// Check interval
		if minInterval != wantMinInterval {
			t.Errorf("resubmit min interval mismatch: have %v, want %v ", minInterval, wantMinInterval)
		}
		if recommitInterval != wantRecommitInterval {
			t.Errorf("resubmit interval mismatch: have %v, want %v", recommitInterval, wantRecommitInterval)
		}
		result = append(result, float64(recommitInterval.Nanoseconds()))
		index += 1
		progress <- struct{}{}
	}
	// Ensure worker has finished initialization
	for {
		b := w.pendingBlock()
		if b != nil && b.NumberU64() == 1 {
			break
		}
	}

	w.start()

	time.Sleep(time.Second)

	start = true
	w.setRecommitInterval(3 * time.Second)
	select {
	case <-progress:
	case <-time.NewTimer(time.Second).C:
		t.Error("interval reset timeout")
	}

	w.resubmitAdjustCh <- &intervalAdjust{inc: true, ratio: 0.8}
	select {
	case <-progress:
	case <-time.NewTimer(time.Second).C:
		t.Error("interval reset timeout")
	}

	w.resubmitAdjustCh <- &intervalAdjust{inc: false}
	select {
	case <-progress:
	case <-time.NewTimer(time.Second).C:
		t.Error("interval reset timeout")
	}

	w.setRecommitInterval(500 * time.Millisecond)
	select {
	case <-progress:
	case <-time.NewTimer(time.Second).C:
		t.Error("interval reset timeout")
	}
}
