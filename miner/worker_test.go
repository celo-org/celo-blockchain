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
	"fmt"
	"math/big"
	"math/rand"
	"sync/atomic"
	"testing"
	"time"

	"github.com/celo-org/bls-zexe/go/bls"
	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/consensus/consensustest"
	mockEngine "github.com/ethereum/go-ethereum/consensus/consensustest"
	"github.com/ethereum/go-ethereum/consensus/istanbul"
	istanbulBackend "github.com/ethereum/go-ethereum/consensus/istanbul/backend"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	blscrypto "github.com/ethereum/go-ethereum/crypto/bls"

	"github.com/ethereum/go-ethereum/contract_comm"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/crypto/ecies"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/event"
	"github.com/ethereum/go-ethereum/params"
)

const (
	// testCode is the testing contract binary code which will initialises some
	// variables in constructor
	testCode = "0x60806040527fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff0060005534801561003457600080fd5b5060fc806100436000396000f3fe6080604052348015600f57600080fd5b506004361060325760003560e01c80630c4dae8814603757806398a213cf146053575b600080fd5b603d607e565b6040518082815260200191505060405180910390f35b607c60048036036020811015606757600080fd5b81019080803590602001909291905050506084565b005b60005481565b806000819055507fe9e44f9f7da8c559de847a3232b57364adc0354f15a2cd8dc636d54396f9587a6000546040518082815260200191505060405180910390a15056fea265627a7a723058208ae31d9424f2d0bc2a3da1a5dd659db2d71ec322a17db8f87e19e209e3a1ff4a64736f6c634300050a0032"

	// testGas is the gas required for contract deployment.
	testGas = 144109
)

var (
	// Test chain configurations
	testTxPoolConfig    core.TxPoolConfig
	istanbulChainConfig *params.ChainConfig

	// Test accounts
	testBankKey, _  = crypto.GenerateKey()
	testBankAddress = crypto.PubkeyToAddress(testBankKey.PublicKey)
	testBankFunds   = big.NewInt(1000000000000000000)

	testUserKey, _  = crypto.GenerateKey()
	testUserAddress = crypto.PubkeyToAddress(testUserKey.PublicKey)

	// Test transactions
	pendingTxs []*types.Transaction
	newTxs     []*types.Transaction

	testConfig = &Config{
		Recommit: time.Second,
		GasFloor: 0,
		GasCeil:  0,
	}
)

func init() {
	testTxPoolConfig = core.DefaultTxPoolConfig
	testTxPoolConfig.Journal = ""
	istanbulChainConfig = params.DefaultChainConfig
	istanbulChainConfig.Istanbul = &params.IstanbulConfig{
		Epoch:          30000,
		ProposerPolicy: 0,
	}

	tx1, _ := types.SignTx(types.NewTransaction(0, testUserAddress, big.NewInt(1000), params.TxGas, nil, nil, nil, nil, nil), types.HomesteadSigner{}, testBankKey)
	pendingTxs = append(pendingTxs, tx1)
	tx2, _ := types.SignTx(types.NewTransaction(1, testUserAddress, big.NewInt(1000), params.TxGas, nil, nil, nil, nil, nil), types.HomesteadSigner{}, testBankKey)
	newTxs = append(newTxs, tx2)
	rand.Seed(time.Now().UnixNano())
}

// testWorkerBackend implements worker.Backend interfaces and wraps all information needed during the testing.
type testWorkerBackend struct {
	accountManager *accounts.Manager
	db             ethdb.Database
	txPool         *core.TxPool
	chain          *core.BlockChain
	genesis        *core.Genesis
}

func newTestWorkerBackend(t *testing.T, chainConfig *params.ChainConfig, engine consensus.Engine, db ethdb.Database, n int) *testWorkerBackend {
	var gspec = core.Genesis{
		Config: chainConfig,
		Alloc:  core.GenesisAlloc{testBankAddress: {Balance: testBankFunds}},
	}

	switch engine.(type) {
	case *consensustest.MockEngine:
	case *istanbulBackend.Backend:
		blsPrivateKey, _ := blscrypto.ECDSAToBLS(testBankKey)
		blsPublicKey, _ := blscrypto.PrivateToPublic(blsPrivateKey)
		istanbulBackend.AppendValidatorsToGenesisBlock(&gspec, []istanbul.ValidatorData{
			{
				Address:      testBankAddress,
				BLSPublicKey: blsPublicKey,
			},
		})
	default:
		t.Fatalf("unexpected consensus engine type: %T", engine)
	}
	genesis := gspec.MustCommit(db)

	chain, _ := core.NewBlockChain(db, &core.CacheConfig{TrieDirtyDisabled: true}, gspec.Config, engine, vm.Config{}, nil)
	contract_comm.SetInternalEVMHandler(chain)

	txpool := core.NewTxPool(testTxPoolConfig, chainConfig, chain)

	// If istanbul engine used, set the objects in that engine
	if istanbul, ok := engine.(consensus.Istanbul); ok {
		istanbul.SetChain(chain, chain.CurrentBlock, func(parentHash common.Hash) (*state.StateDB, error) {
			parentStateRoot := chain.GetHeaderByHash(parentHash).Root
			return chain.StateAt(parentStateRoot)
		})
	}

	// Generate a small n-block chain.
	if n > 0 {
		blocks, _ := core.GenerateChain(chainConfig, genesis, engine, db, n, func(i int, gen *core.BlockGen) {
			gen.SetCoinbase(testBankAddress)
		})
		if _, err := chain.InsertChain(blocks); err != nil {
			t.Fatalf("failed to insert origin chain: %v", err)
		}
	}

	var backends []accounts.Backend
	accountManager := accounts.NewManager(&accounts.Config{InsecureUnlockAllowed: true}, backends...)

	return &testWorkerBackend{
		accountManager: accountManager,
		db:             db,
		chain:          chain,
		txPool:         txpool,
		genesis:        &gspec,
	}
}

func (b *testWorkerBackend) AccountManager() *accounts.Manager { return b.accountManager }
func (b *testWorkerBackend) BlockChain() *core.BlockChain      { return b.chain }
func (b *testWorkerBackend) TxPool() *core.TxPool              { return b.txPool }
func (b *testWorkerBackend) PostChainEvents(events []interface{}) {
	b.chain.PostChainEvents(events, nil)
}

func (b *testWorkerBackend) newRandomTx(creation bool) *types.Transaction {
	var tx *types.Transaction
	if creation {
		tx, _ = types.SignTx(types.NewContractCreation(b.txPool.Nonce(testBankAddress), big.NewInt(0), testGas, nil, nil, nil, nil, common.FromHex(testCode)), types.HomesteadSigner{}, testBankKey)
	} else {
		tx, _ = types.SignTx(types.NewTransaction(b.txPool.Nonce(testBankAddress), testUserAddress, big.NewInt(1000), params.TxGas, nil, nil, nil, nil, nil), types.HomesteadSigner{}, testBankKey)
	}
	return tx
}

func newTestWorker(t *testing.T, chainConfig *params.ChainConfig, engine consensus.Engine, db ethdb.Database, blocks int, shouldAddPendingTxs bool) (*worker, *testWorkerBackend) {
	backend := newTestWorkerBackend(t, chainConfig, engine, db, blocks)
	if shouldAddPendingTxs {
		backend.txPool.AddLocals(pendingTxs)
	}
	w := newWorker(testConfig, chainConfig, engine, backend, new(event.TypeMux), nil, &backend.db, false)
	w.setEtherbase(testBankAddress)
	return w, backend
}

func TestGenerateBlockAndImport(t *testing.T) {
	var (
		engine      consensus.Engine
		chainConfig *params.ChainConfig
		db          = rawdb.NewMemoryDatabase()
	)
	chainConfig = params.DefaultChainConfig
	engine = mockEngine.NewFaker()

	w, b := newTestWorker(t, chainConfig, engine, db, 0, true)
	defer w.close()

	db2 := rawdb.NewMemoryDatabase()
	b.genesis.MustCommit(db2)
	chain, _ := core.NewBlockChain(db2, nil, b.chain.Config(), engine, vm.Config{}, nil)
	defer chain.Stop()

	loopErr := make(chan error)
	newBlock := make(chan struct{})
	listenNewBlock := func() {
		sub := w.mux.Subscribe(core.NewMinedBlockEvent{})
		defer sub.Unsubscribe()

		for item := range sub.Chan() {
			block := item.Data.(core.NewMinedBlockEvent).Block
			_, err := chain.InsertChain([]*types.Block{block})
			if err != nil {
				loopErr <- fmt.Errorf("failed to insert new mined block:%d, error:%v", block.NumberU64(), err)
			}
			newBlock <- struct{}{}
		}
	}
	// Ignore empty commit here for less noise
	w.skipSealHook = func(task *task) bool {
		return len(task.receipts) == 0
	}
	w.start() // Start mining!
	go listenNewBlock()

	for i := 0; i < 5; i++ {
		b.txPool.AddLocal(b.newRandomTx(true))
		b.txPool.AddLocal(b.newRandomTx(false))
		select {
		case e := <-loopErr:
			t.Fatal(e)
		case <-newBlock:
		case <-time.NewTimer(3 * time.Second).C: // Worker needs 1s to include new changes.
			t.Fatalf("timeout")
		}
	}
}

func getAuthorizedIstanbulEngine() consensus.Istanbul {

	decryptFn := func(_ accounts.Account, c, s1, s2 []byte) ([]byte, error) {
		eciesKey := ecies.ImportECDSA(testBankKey)
		return eciesKey.Decrypt(c, s1, s2)
	}

	signerFn := func(_ accounts.Account, mimeType string, data []byte) ([]byte, error) {
		return crypto.Sign(data, testBankKey)
	}

	signHashBLSFn := func(_ accounts.Account, data []byte) (blscrypto.SerializedSignature, error) {
		privateKeyBytes, err := blscrypto.ECDSAToBLS(testBankKey)
		if err != nil {
			return blscrypto.SerializedSignature{}, err
		}

		privateKey, err := bls.DeserializePrivateKey(privateKeyBytes)
		if err != nil {
			return blscrypto.SerializedSignature{}, err
		}
		defer privateKey.Destroy()

		signature, err := privateKey.SignMessage(data, []byte{}, false)
		if err != nil {
			return blscrypto.SerializedSignature{}, err
		}
		defer signature.Destroy()
		signatureBytes, err := signature.Serialize()
		if err != nil {
			return blscrypto.SerializedSignature{}, err
		}

		return blscrypto.SerializedSignatureFromBytes(signatureBytes)
	}

	signMessageBLSFn := func(_ accounts.Account, msg []byte, extraData []byte) (blscrypto.SerializedSignature, error) {
		privateKeyBytes, err := blscrypto.ECDSAToBLS(testBankKey)
		if err != nil {
			return blscrypto.SerializedSignature{}, err
		}

		privateKey, err := bls.DeserializePrivateKey(privateKeyBytes)
		if err != nil {
			return blscrypto.SerializedSignature{}, err
		}
		defer privateKey.Destroy()

		signature, err := privateKey.SignMessage(msg, extraData, true)
		if err != nil {
			return blscrypto.SerializedSignature{}, err
		}
		defer signature.Destroy()
		signatureBytes, err := signature.Serialize()
		if err != nil {
			return blscrypto.SerializedSignature{}, err
		}

		return blscrypto.SerializedSignatureFromBytes(signatureBytes)
	}

	config := istanbul.DefaultConfig
	config.RoundStateDBPath = ""
	config.ValidatorEnodeDBPath = ""
	config.VersionCertificateDBPath = ""

	engine := istanbulBackend.New(config, rawdb.NewMemoryDatabase())
	engine.(*istanbulBackend.Backend).SetBroadcaster(&consensustest.MockBroadcaster{})
	engine.(*istanbulBackend.Backend).SetP2PServer(consensustest.NewMockP2PServer())
	engine.(*istanbulBackend.Backend).Authorize(crypto.PubkeyToAddress(testBankKey.PublicKey), &testBankKey.PublicKey, decryptFn, signerFn, signHashBLSFn, signMessageBLSFn)
	engine.(*istanbulBackend.Backend).StartAnnouncing()
	return engine
}

func TestEmptyWorkIstanbul(t *testing.T) {
	// TODO(nambrot): Fix this
	t.Skip("Disabled due to flakyness")
	testEmptyWork(t, istanbulChainConfig, getAuthorizedIstanbulEngine(), false, true)
	testEmptyWork(t, istanbulChainConfig, getAuthorizedIstanbulEngine(), true, false)
}

func testEmptyWork(t *testing.T, chainConfig *params.ChainConfig, engine consensus.Engine, expectEmptyBlock bool, shouldAddPendingTxs bool) {
	defer engine.Close()

	w, _ := newTestWorker(t, chainConfig, engine, rawdb.NewMemoryDatabase(), 0, shouldAddPendingTxs)
	defer w.close()

	var (
		taskIndex int
		taskCh    = make(chan struct{}, 2)
	)
	checkEqual := func(t *testing.T, task *task, index int) {
		// The first empty work without any txs included
		receiptLen, balance := 0, big.NewInt(0)

		// if !expectEmptyBlock || (index == 1 && shouldAddPendingTxs) {
		if index == 1 {
			// The second full work with 1 tx included
			receiptLen, balance = 1, big.NewInt(1000)
		}

		if len(task.receipts) != receiptLen {
			t.Fatalf("receipt number mismatch: have %d, want %d", len(task.receipts), receiptLen)
		}
		if task.state.GetBalance(testUserAddress).Cmp(balance) != 0 {
			t.Fatalf("account balance mismatch: have %d, want %d", task.state.GetBalance(testUserAddress), balance)
		}
	}
	w.newTaskHook = func(task *task) {
		if task.block.NumberU64() == 1 {
			checkEqual(t, task, taskIndex)
			taskIndex += 1
			taskCh <- struct{}{}
		}
	}
	w.skipSealHook = func(task *task) bool { return true }
	w.fullTaskHook = func() {
		// Aarch64 unit tests are running in a VM on travis, they must
		// be given more time to execute.
		time.Sleep(time.Second)
	}
	w.start() // Start mining!
	expectedTasksLen := 1
	if shouldAddPendingTxs && expectEmptyBlock {
		expectedTasksLen = 2
	}
	for i := 0; i < expectedTasksLen; i += 1 {
		select {
		case <-taskCh:
		case <-time.NewTimer(3 * time.Second).C:
			t.Error("new task timeout")
		}
	}

	select {
	case <-taskCh:
		t.Error("should have not received another task")
	case <-time.NewTimer(time.Second).C:
	}
}

// For Ethhash and Clique, it is safe and even desired to start another seal process in the presence of new transactions
// that potentially increase the fee revenue for the sealer. In Istanbul, that is not possible and even counter productive
// as proposing another block after having already done so is clearly byzantine behavior.
func TestRegenerateMiningBlockIstanbul(t *testing.T) {
	chainConfig := istanbulChainConfig
	engine := getAuthorizedIstanbulEngine()

	defer engine.Close()

	w, b := newTestWorker(t, chainConfig, engine, rawdb.NewMemoryDatabase(), 0, true)
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

// nolint: unused
func testRegenerateMiningBlock(t *testing.T, chainConfig *params.ChainConfig, engine consensus.Engine) {
	defer engine.Close()

	w, b := newTestWorker(t, chainConfig, engine, rawdb.NewMemoryDatabase(), 0, true)
	defer w.close()

	var taskCh = make(chan struct{})

	taskIndex := 0
	w.newTaskHook = func(task *task) {
		if task.block.NumberU64() == 1 {
			// The first task is an empty task, the second
			// one has 1 pending tx, the third one has 2 txs
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

// nolint: unused
func testAdjustInterval(t *testing.T, chainConfig *params.ChainConfig, engine consensus.Engine) {
	defer engine.Close()

	w, _ := newTestWorker(t, chainConfig, engine, rawdb.NewMemoryDatabase(), 0, true)
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
		start    uint32
	)
	w.resubmitHook = func(minInterval time.Duration, recommitInterval time.Duration) {
		// Short circuit if interval checking hasn't started.
		if atomic.LoadUint32(&start) == 0 {
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
	w.start()

	time.Sleep(time.Second) // Ensure two tasks have been summitted due to start opt
	atomic.StoreUint32(&start, 1)

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
