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
	"bytes"
	"context"
	"crypto/ecdsa"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/celo-org/celo-blockchain/accounts/abi"
	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/common/hexutil"
	"github.com/celo-org/celo-blockchain/consensus"
	mockEngine "github.com/celo-org/celo-blockchain/consensus/consensustest"
	"github.com/celo-org/celo-blockchain/contracts/testutil"
	"github.com/celo-org/celo-blockchain/core"
	"github.com/celo-org/celo-blockchain/core/rawdb"
	"github.com/celo-org/celo-blockchain/core/state"
	"github.com/celo-org/celo-blockchain/core/types"
	"github.com/celo-org/celo-blockchain/core/vm"
	"github.com/celo-org/celo-blockchain/crypto"
	"github.com/celo-org/celo-blockchain/ethdb"
	"github.com/celo-org/celo-blockchain/internal/ethapi"
	"github.com/celo-org/celo-blockchain/params"
	"github.com/celo-org/celo-blockchain/rpc"
)

var (
	errStateNotFound       = errors.New("state not found")
	errBlockNotFound       = errors.New("block not found")
	errTransactionNotFound = errors.New("transaction not found")
)

type testBackend struct {
	chainConfig *params.ChainConfig
	engine      consensus.Engine
	chaindb     ethdb.Database
	chain       *core.BlockChain
}

func newTestBackend(t *testing.T, n int, gspec *core.Genesis, generator func(i int, b *core.BlockGen)) *testBackend {
	chainConfig := params.TestChainConfig
	if gspec.Config != nil {
		chainConfig = gspec.Config
	}
	chainConfig.Faker = true
	backend := &testBackend{
		chainConfig: chainConfig,
		engine:      mockEngine.NewFaker(),
		chaindb:     rawdb.NewMemoryDatabase(),
	}
	// Generate blocks for testing
	gspec.Config = backend.chainConfig
	var (
		gendb   = rawdb.NewMemoryDatabase()
		genesis = gspec.MustCommit(gendb)
	)
	blocks, _ := core.GenerateChain(backend.chainConfig, genesis, backend.engine, gendb, n, generator)

	// Import the canonical chain
	gspec.MustCommit(backend.chaindb)
	cacheConfig := &core.CacheConfig{
		TrieCleanLimit:    256,
		TrieDirtyLimit:    256,
		TrieTimeLimit:     5 * time.Minute,
		SnapshotLimit:     0,
		TrieDirtyDisabled: true, // Archive mode
	}
	chain, err := core.NewBlockChain(backend.chaindb, cacheConfig, backend.chainConfig, backend.engine, vm.Config{}, nil, nil)
	if err != nil {
		t.Fatalf("failed to create tester chain: %v", err)
	}
	if n, err := chain.InsertChain(blocks); err != nil {
		t.Fatalf("block %d: failed to insert into chain: %v", n, err)
	}
	backend.chain = chain
	return backend
}

func (b *testBackend) HeaderByHash(ctx context.Context, hash common.Hash) (*types.Header, error) {
	return b.chain.GetHeaderByHash(hash), nil
}

func (b *testBackend) HeaderByNumber(ctx context.Context, number rpc.BlockNumber) (*types.Header, error) {
	if number == rpc.PendingBlockNumber || number == rpc.LatestBlockNumber {
		return b.chain.CurrentHeader(), nil
	}
	return b.chain.GetHeaderByNumber(uint64(number)), nil
}

func (b *testBackend) BlockByHash(ctx context.Context, hash common.Hash) (*types.Block, error) {
	return b.chain.GetBlockByHash(hash), nil
}

func (b *testBackend) BlockByNumber(ctx context.Context, number rpc.BlockNumber) (*types.Block, error) {
	if number == rpc.PendingBlockNumber || number == rpc.LatestBlockNumber {
		return b.chain.CurrentBlock(), nil
	}
	return b.chain.GetBlockByNumber(uint64(number)), nil
}

func (b *testBackend) GetTransaction(ctx context.Context, txHash common.Hash) (*types.Transaction, common.Hash, uint64, uint64, error) {
	tx, hash, blockNumber, index := rawdb.ReadTransaction(b.chaindb, txHash)
	if tx == nil {
		return nil, common.Hash{}, 0, 0, errTransactionNotFound
	}
	return tx, hash, blockNumber, index, nil
}

func (b *testBackend) RPCGasInflationRate() float64 {
	return 1
}

func (b *testBackend) RPCGasCap() uint64 {
	return 25000000
}

func (b *testBackend) ChainConfig() *params.ChainConfig {
	return b.chainConfig
}

func (b *testBackend) Engine() consensus.Engine {
	return b.engine
}

func (b *testBackend) ChainDb() ethdb.Database {
	return b.chaindb
}

func (b *testBackend) StateAtBlock(ctx context.Context, block *types.Block, reexec uint64, base *state.StateDB, checkLive bool) (*state.StateDB, error) {
	statedb, err := b.chain.StateAt(block.Root())
	if err != nil {
		return nil, errStateNotFound
	}
	return statedb, nil
}

func (b *testBackend) NewEVMRunner(header *types.Header, state vm.StateDB) vm.EVMRunner {
	return testutil.NewMockEVMRunner()
}

func (b *testBackend) StateAtTransaction(ctx context.Context, block *types.Block, txIndex int, reexec uint64) (core.Message, vm.BlockContext, vm.EVMRunner, *state.StateDB, error) {
	parent := b.chain.GetBlock(block.ParentHash(), block.NumberU64()-1)
	if parent == nil {
		return nil, vm.BlockContext{}, nil, nil, errBlockNotFound
	}
	statedb, err := b.chain.StateAt(parent.Root())
	if err != nil {
		return nil, vm.BlockContext{}, nil, nil, errStateNotFound
	}
	if txIndex == 0 && len(block.Transactions()) == 0 {
		return nil, vm.BlockContext{}, nil, statedb, nil
	}
	// Recompute transactions up to the target index.
	signer := types.MakeSigner(b.chainConfig, block.Number())
	for idx, tx := range block.Transactions() {
		msg, _ := tx.AsMessage(signer, nil)
		txContext := core.NewEVMTxContext(msg)
		context := core.NewEVMBlockContext(block.Header(), b.chain, nil)
		vmRunner := b.chain.NewEVMRunner(block.Header(), statedb)
		if idx == txIndex {
			return msg, context, vmRunner, statedb, nil
		}
		vmenv := vm.NewEVM(context, txContext, statedb, b.chainConfig, vm.Config{})
		if _, err := core.ApplyMessage(vmenv, msg, new(core.GasPool).AddGas(tx.Gas()), vmRunner, nil); err != nil {
			return nil, vm.BlockContext{}, nil, nil, fmt.Errorf("transaction %#x failed: %v", tx.Hash(), err)
		}
		statedb.Finalise(vmenv.ChainConfig().IsEIP158(block.Number()))
	}
	return nil, vm.BlockContext{}, nil, nil, fmt.Errorf("transaction index %d out of range for block %#x", txIndex, block.Hash())
}

func TestTraceCall(t *testing.T) {
	t.Parallel()

	// Initialize test accounts
	accounts := newAccounts(3)
	genesis := &core.Genesis{Alloc: core.GenesisAlloc{
		accounts[0].addr: {Balance: big.NewInt(params.Ether)},
		accounts[1].addr: {Balance: big.NewInt(params.Ether)},
		accounts[2].addr: {Balance: big.NewInt(params.Ether)},
	}}
	genBlocks := 10
	signer := types.HomesteadSigner{}
	api := NewAPI(newTestBackend(t, genBlocks, genesis, func(i int, b *core.BlockGen) {
		// Transfer from account[0] to account[1]
		//    value: 1000 wei
		//    fee:   0 wei
		tx, _ := types.SignTx(types.NewTransaction(uint64(i), accounts[1].addr, big.NewInt(1000), params.TxGas, nil, nil, nil, nil, nil), signer, accounts[0].key)
		b.AddTx(tx)
	}))

	var testSuite = []struct {
		blockNumber rpc.BlockNumber
		call        ethapi.TransactionArgs
		config      *TraceCallConfig
		expectErr   error
		expect      interface{}
	}{
		// Standard JSON trace upon the genesis, plain transfer.
		{
			blockNumber: rpc.BlockNumber(0),
			call: ethapi.TransactionArgs{
				From:  &accounts[0].addr,
				To:    &accounts[1].addr,
				Value: (*hexutil.Big)(big.NewInt(1000)),
			},
			config:    nil,
			expectErr: nil,
			expect: &ethapi.ExecutionResult{
				Gas:         params.TxGas,
				Failed:      false,
				ReturnValue: "",
				StructLogs:  []ethapi.StructLogRes{},
			},
		},
		// Standard JSON trace upon the head, plain transfer.
		{
			blockNumber: rpc.BlockNumber(genBlocks),
			call: ethapi.TransactionArgs{
				From:  &accounts[0].addr,
				To:    &accounts[1].addr,
				Value: (*hexutil.Big)(big.NewInt(1000)),
			},
			config:    nil,
			expectErr: nil,
			expect: &ethapi.ExecutionResult{
				Gas:         params.TxGas,
				Failed:      false,
				ReturnValue: "",
				StructLogs:  []ethapi.StructLogRes{},
			},
		},
		// Standard JSON trace upon the non-existent block, error expects
		{
			blockNumber: rpc.BlockNumber(genBlocks + 1),
			call: ethapi.TransactionArgs{
				From:  &accounts[0].addr,
				To:    &accounts[1].addr,
				Value: (*hexutil.Big)(big.NewInt(1000)),
			},
			config:    nil,
			expectErr: fmt.Errorf("block #%d not found", genBlocks+1),
			expect:    nil,
		},
		// Standard JSON trace upon the latest block
		{
			blockNumber: rpc.LatestBlockNumber,
			call: ethapi.TransactionArgs{
				From:  &accounts[0].addr,
				To:    &accounts[1].addr,
				Value: (*hexutil.Big)(big.NewInt(1000)),
			},
			config:    nil,
			expectErr: nil,
			expect: &ethapi.ExecutionResult{
				Gas:         params.TxGas,
				Failed:      false,
				ReturnValue: "",
				StructLogs:  []ethapi.StructLogRes{},
			},
		},
		// Standard JSON trace upon the pending block
		{
			blockNumber: rpc.PendingBlockNumber,
			call: ethapi.TransactionArgs{
				From:  &accounts[0].addr,
				To:    &accounts[1].addr,
				Value: (*hexutil.Big)(big.NewInt(1000)),
			},
			config:    nil,
			expectErr: nil,
			expect: &ethapi.ExecutionResult{
				Gas:         params.TxGas,
				Failed:      false,
				ReturnValue: "",
				StructLogs:  []ethapi.StructLogRes{},
			},
		},
	}
	for _, testspec := range testSuite {
		result, err := api.TraceCall(context.Background(), testspec.call, rpc.BlockNumberOrHash{BlockNumber: &testspec.blockNumber}, testspec.config)
		if testspec.expectErr != nil {
			if err == nil {
				t.Errorf("Expect error %v, get nothing", testspec.expectErr)
				continue
			}
			if !reflect.DeepEqual(err, testspec.expectErr) {
				t.Errorf("Error mismatch, want %v, get %v", testspec.expectErr, err)
			}
		} else {
			if err != nil {
				t.Errorf("Expect no error, get %v", err)
				continue
			}
			if !reflect.DeepEqual(result, testspec.expect) {
				t.Errorf("Result mismatch, want %v, get %v", testspec.expect, result)
			}
		}
	}
}

func TestOverriddenTraceCall(t *testing.T) {
	t.Parallel()

	// Initialize test accounts
	accounts := newAccounts(3)
	genesis := &core.Genesis{Alloc: core.GenesisAlloc{
		accounts[0].addr: {Balance: big.NewInt(params.Ether)},
		accounts[1].addr: {Balance: big.NewInt(params.Ether)},
		accounts[2].addr: {Balance: big.NewInt(params.Ether)},
	}}
	genBlocks := 10
	signer := types.HomesteadSigner{}
	api := NewAPI(newTestBackend(t, genBlocks, genesis, func(i int, b *core.BlockGen) {
		// Transfer from account[0] to account[1]
		//    value: 1000 wei
		//    fee:   0 wei
		tx, _ := types.SignTx(types.NewTransaction(uint64(i), accounts[1].addr, big.NewInt(1000), params.TxGas, nil, nil, nil, nil, nil), signer, accounts[0].key)
		b.AddTx(tx)
	}))
	randomAccounts, tracer := newAccounts(3), "callTracer"

	var testSuite = []struct {
		blockNumber rpc.BlockNumber
		call        ethapi.TransactionArgs
		config      *TraceCallConfig
		expectErr   error
		expect      *callTrace
	}{
		// Succcessful call with state overriding
		{
			blockNumber: rpc.PendingBlockNumber,
			call: ethapi.TransactionArgs{
				From:  &randomAccounts[0].addr,
				To:    &randomAccounts[1].addr,
				Value: (*hexutil.Big)(big.NewInt(1000)),
			},
			config: &TraceCallConfig{
				Tracer: &tracer,
				StateOverrides: &ethapi.StateOverride{
					randomAccounts[0].addr: ethapi.OverrideAccount{Balance: newRPCBalance(new(big.Int).Mul(big.NewInt(1), big.NewInt(params.Ether)))},
				},
			},
			expectErr: nil,
			expect: &callTrace{
				Type:    "CALL",
				From:    randomAccounts[0].addr,
				To:      randomAccounts[1].addr,
				Gas:     newRPCUint64(24979000),
				GasUsed: newRPCUint64(0),
				Value:   (*hexutil.Big)(big.NewInt(1000)),
			},
		},
		// Invalid call without state overriding
		{
			blockNumber: rpc.PendingBlockNumber,
			call: ethapi.TransactionArgs{
				From:  &randomAccounts[0].addr,
				To:    &randomAccounts[1].addr,
				Value: (*hexutil.Big)(big.NewInt(1000)),
			},
			config: &TraceCallConfig{
				Tracer: &tracer,
			},
			expectErr: core.ErrInsufficientFundsForTransfer,
			expect:    nil,
		},
		// Successful simple contract call
		//
		// // SPDX-License-Identifier: GPL-3.0
		//
		//  pragma solidity >=0.7.0 <0.8.0;
		//
		//  /**
		//   * @title Storage
		//   * @dev Store & retrieve value in a variable
		//   */
		//  contract Storage {
		//      uint256 public number;
		//      constructor() {
		//          number = block.number;
		//      }
		//  }
		{
			blockNumber: rpc.PendingBlockNumber,
			call: ethapi.TransactionArgs{
				From: &randomAccounts[0].addr,
				To:   &randomAccounts[2].addr,
				Data: newRPCBytes(common.Hex2Bytes("8381f58a")), // call number()
			},
			config: &TraceCallConfig{
				Tracer: &tracer,
				StateOverrides: &ethapi.StateOverride{
					randomAccounts[2].addr: ethapi.OverrideAccount{
						Code:      newRPCBytes(common.Hex2Bytes("6080604052348015600f57600080fd5b506004361060285760003560e01c80638381f58a14602d575b600080fd5b60336049565b6040518082815260200191505060405180910390f35b6000548156fea2646970667358221220eab35ffa6ab2adfe380772a48b8ba78e82a1b820a18fcb6f59aa4efb20a5f60064736f6c63430007040033")),
						StateDiff: newStates([]common.Hash{{}}, []common.Hash{common.BigToHash(big.NewInt(123))}),
					},
				},
			},
			expectErr: nil,
			expect: &callTrace{
				Type:    "CALL",
				From:    randomAccounts[0].addr,
				To:      randomAccounts[2].addr,
				Input:   hexutil.Bytes(common.Hex2Bytes("8381f58a")),
				Output:  hexutil.Bytes(common.BigToHash(big.NewInt(123)).Bytes()),
				Gas:     newRPCUint64(24978936),
				GasUsed: newRPCUint64(383), // TODO ethereum cost 2283, check if this is right
				Value:   (*hexutil.Big)(big.NewInt(0)),
			},
		},
	}
	for i, testspec := range testSuite {
		result, err := api.TraceCall(context.Background(), testspec.call, rpc.BlockNumberOrHash{BlockNumber: &testspec.blockNumber}, testspec.config)
		if testspec.expectErr != nil {
			if err == nil {
				t.Errorf("test %d: want error %v, have nothing", i, testspec.expectErr)
				continue
			}
			if !errors.Is(err, testspec.expectErr) {
				t.Errorf("test %d: error mismatch, want %v, have %v", i, testspec.expectErr, err)
			}
		} else {
			if err != nil {
				t.Errorf("test %d: want no error, have %v", i, err)
				continue
			}
			ret := new(callTrace)
			if err := json.Unmarshal(result.(json.RawMessage), ret); err != nil {
				t.Fatalf("test %d: failed to unmarshal trace result: %v", i, err)
			}
			if !jsonEqual(ret, testspec.expect) {
				// uncomment this for easier debugging
				//have, _ := json.MarshalIndent(ret, "", " ")
				//want, _ := json.MarshalIndent(testspec.expect, "", " ")
				//t.Fatalf("trace mismatch: \nhave %+v\nwant %+v", string(have), string(want))
				t.Fatalf("trace mismatch: \nhave %+v\nwant %+v", ret, testspec.expect)
			}
		}
	}
}

func TestTraceTransaction(t *testing.T) {
	t.Parallel()

	// Initialize test accounts
	accounts := newAccounts(2)
	genesis := &core.Genesis{Alloc: core.GenesisAlloc{
		accounts[0].addr: {Balance: big.NewInt(params.Ether)},
		accounts[1].addr: {Balance: big.NewInt(params.Ether)},
	}}
	target := common.Hash{}
	signer := types.HomesteadSigner{}
	api := NewAPI(newTestBackend(t, 1, genesis, func(i int, b *core.BlockGen) {
		// Transfer from account[0] to account[1]
		//    value: 1000 wei
		//    fee:   0 wei
		tx, _ := types.SignTx(types.NewTransaction(uint64(i), accounts[1].addr, big.NewInt(1000), params.TxGas, nil, nil, nil, nil, nil), signer, accounts[0].key)
		b.AddTx(tx)
		target = tx.Hash()
	}))
	result, err := api.TraceTransaction(context.Background(), target, nil)
	if err != nil {
		t.Errorf("Failed to trace transaction %v", err)
	}
	if !reflect.DeepEqual(result, &ethapi.ExecutionResult{
		Gas:         params.TxGas,
		Failed:      false,
		ReturnValue: "",
		StructLogs:  []ethapi.StructLogRes{},
	}) {
		t.Error("Transaction tracing result is different")
	}
}

// Regression test for debug get/set logic in the EVM & Interpreter.
// Include registry to ensure that Celo-specific EVM Call's within
// Tobin Tax and fee distribution logic are triggered.
func TestTraceTransactionWithRegistryDeployed(t *testing.T) {
	t.Parallel()

	// Initialize test accounts
	accounts := newAccounts(2)
	genesis := &core.Genesis{Alloc: core.GenesisAlloc{
		accounts[0].addr: {Balance: big.NewInt(params.Ether)},
		accounts[1].addr: {Balance: big.NewInt(params.Ether)},
		common.HexToAddress("0xce10"): { // Registry Proxy
			Code: testutil.RegistryProxyOpcodes,
			Storage: map[common.Hash]common.Hash{
				common.HexToHash("0x360894a13ba1a3210667c828492db98dca3e2076cc3735a920a3ca505d382bbc"): common.HexToHash("0xce11"), // Registry Implementation
				common.HexToHash("0x91646b8507bf2e54d7c3de9155442ba111546b81af1cbdd1f68eeb6926b98d58"): common.HexToHash("0xd023"), // Governance Proxy
			},
			Balance: big.NewInt(0),
		},
		common.HexToAddress("0xce11"): { // Registry Implementation
			Code:    testutil.RegistryOpcodes,
			Balance: big.NewInt(0),
		},
	}}

	target := common.Hash{}
	signer := types.HomesteadSigner{}
	api := NewAPI(newTestBackend(t, 1, genesis, func(i int, b *core.BlockGen) {
		// Transfer from account[0] to account[1]
		//    value: 1000 wei
		//    fee:   0 wei
		tx, _ := types.SignTx(types.NewTransaction(uint64(i), accounts[1].addr, big.NewInt(1000), params.TxGas, nil, nil, nil, nil, nil), signer, accounts[0].key)
		b.AddTx(tx)
		target = tx.Hash()
	}))
	result, err := api.TraceTransaction(context.Background(), target, nil)
	if err != nil {
		t.Errorf("Failed to trace transaction %v", err)
	}
	if !reflect.DeepEqual(result, &ethapi.ExecutionResult{
		Gas:         params.TxGas,
		Failed:      false,
		ReturnValue: "",
		StructLogs:  []ethapi.StructLogRes{},
	}) {
		t.Error("Transaction tracing result is different")
	}
}

// Use the callTracer to trace a native CELO transfer after the
// registry has been deployed, as above.
func TestCallTraceTransactionNativeTransfer(t *testing.T) {
	t.Parallel()

	// Initialize test accounts
	accounts := newAccounts(2)
	genesis := &core.Genesis{Alloc: core.GenesisAlloc{
		accounts[0].addr: {Balance: big.NewInt(params.Ether)},
		accounts[1].addr: {Balance: big.NewInt(params.Ether)},
		common.HexToAddress("0xce10"): { // Registry Proxy
			Code: testutil.RegistryProxyOpcodes,
			Storage: map[common.Hash]common.Hash{
				// Hashes represent the storage slot for Registry.sol's `registry` mapping
				// which is stored in the RegistryProxy's storage.
				// Hashes are computed by taking: keccack(packed(params.GoldTokenRegistryId, 1)),
				// where 1 is the storage offset)
				common.HexToHash("0x360894a13ba1a3210667c828492db98dca3e2076cc3735a920a3ca505d382bbc"): common.HexToHash("0xce11"), // Registry Implementation
				common.HexToHash("0x91646b8507bf2e54d7c3de9155442ba111546b81af1cbdd1f68eeb6926b98d58"): common.HexToHash("0xd023"), // Governance Proxy
			},
			Balance: big.NewInt(0),
		},
		common.HexToAddress("0xce11"): { // Registry Implementation
			Code:    testutil.RegistryOpcodes,
			Balance: big.NewInt(0),
		},
	}}

	target := common.Hash{}
	signer := types.HomesteadSigner{}
	transferVal := big.NewInt(1000)
	api := NewAPI(newTestBackend(t, 1, genesis, func(i int, b *core.BlockGen) {
		// Transfer from account[0] to account[1]
		//    value: 1000 wei
		//    fee:   0 wei
		tx, _ := types.SignTx(types.NewTransaction(uint64(i), accounts[1].addr, transferVal, params.TxGas, nil, nil, nil, nil, nil), signer, accounts[0].key)
		b.AddTx(tx)
		target = tx.Hash()
	}))
	tracerStr := "callTracer"
	result, err := api.TraceTransaction(context.Background(), target, &TraceConfig{Tracer: &tracerStr})
	if err != nil {
		t.Errorf("Failed to trace transaction %v", err)
	}

	ret := new(callTrace)
	if err := json.Unmarshal(result.(json.RawMessage), ret); err != nil {
		t.Fatalf("failed to unmarshal trace result: %v", err)
	}
	expectedTrace := &callTrace{
		Type:    "CALL",
		From:    accounts[0].addr,
		To:      accounts[1].addr,
		Input:   hexutil.Bytes(common.Hex2Bytes("0x")),
		Output:  hexutil.Bytes(common.Hex2Bytes("0x")),
		Gas:     newRPCUint64(0),
		GasUsed: newRPCUint64(0),
		Value:   (*hexutil.Big)(transferVal),
	}
	if !jsonEqual(ret, expectedTrace) {
		t.Fatalf("trace mismatch: \nhave %+v\nwant %+v", ret, expectedTrace)
	}
}

// Use the callTracer to trace a CELO transfer made via the transfer
// precompile (by sending this from the registered GoldToken contract).
func TestCallTraceTransactionPrecompileTransfer(t *testing.T) {
	// Invoke the transfer precompile by sending a transfer from an account
	// registered as GoldToken in the mock registry
	t.Parallel()
	// Initialize test accounts
	accounts := newAccounts(3)
	goldToken := accounts[0]
	registryProxyAddr := common.HexToAddress("0xce10")
	registryImplAddr := common.HexToAddress("0xce11")
	genesis := &core.Genesis{Alloc: core.GenesisAlloc{
		goldToken.addr:   {Balance: big.NewInt(params.Ether)},
		accounts[1].addr: {Balance: big.NewInt(params.Ether)},
		accounts[2].addr: {Balance: big.NewInt(params.Ether)},
		registryProxyAddr: { // Registry Proxy
			Code: testutil.RegistryProxyOpcodes,
			Storage: map[common.Hash]common.Hash{
				common.HexToHash("0x360894a13ba1a3210667c828492db98dca3e2076cc3735a920a3ca505d382bbc"): common.HexToHash("0xce11"), // Registry Implementation
				common.HexToHash("0x91646b8507bf2e54d7c3de9155442ba111546b81af1cbdd1f68eeb6926b98d58"): common.HexToHash("0xd023"), // Governance Proxy
				common.HexToHash("0x773cc8652456781771d48fb3d560d7ba3d6d1882654492b68beec583d2c590aa"): goldToken.addr.Hash(),      // GoldToken Implementation
				common.HexToHash("0x2131a4338f6fb8d4507e234a7c72af8efefbbf2f1817ed570bce33eb6667feb9"): common.HexToHash("0xd007"), // Reserve Implementation
			},
			Balance: big.NewInt(0),
		},
		registryImplAddr: { // Registry Implementation
			Code:    testutil.RegistryOpcodes,
			Balance: big.NewInt(0),
		},
	}}

	target := common.Hash{}
	signer := types.HomesteadSigner{}
	transferVal := big.NewInt(1000)

	// Construct and pack data for transfer precompile representing [from, to, value]
	addressTy, err := abi.NewType("address", "", nil)
	if err != nil {
		t.Fatalf("failed to create address type: %v", err)
	}
	uint256Ty, err := abi.NewType("uint256", "", nil)
	if err != nil {
		t.Fatalf("failed to create uint256 type: %v", err)
	}

	transferArgs := abi.Arguments{
		{
			Type: addressTy,
		},
		{
			Type: addressTy,
		},
		{
			Type: uint256Ty,
		},
	}
	data, err := transferArgs.Pack(accounts[1].addr, accounts[2].addr, transferVal)
	if err != nil {
		t.Fatalf("failed to pack args: %v", err)
	}
	transferPrecompile := common.HexToAddress("0xfd")
	gas := params.TxGas * 2
	api := NewAPI(newTestBackend(t, 1, genesis, func(i int, b *core.BlockGen) {
		// Transfer via transfer precompile by sending tx from GoldToken addr
		tx, _ := types.SignTx(types.NewTransaction(uint64(i), transferPrecompile, big.NewInt(0), gas, big.NewInt(3), nil, nil, nil, data), signer, goldToken.key)
		b.AddTx(tx)
		target = tx.Hash()
	}))
	tracerStr := "callTracer"
	result, err := api.TraceTransaction(context.Background(), target, &TraceConfig{Tracer: &tracerStr})

	if err != nil {
		t.Errorf("Failed to trace transaction %v", err)
	}

	ret := new(callTrace)
	if err := json.Unmarshal(result.(json.RawMessage), ret); err != nil {
		t.Fatalf("failed to unmarshal trace result: %v", err)
	}

	// Registry ABI packed version of Registry.getAddressFor("GoldToken")
	packedGetAddressForGoldToken := common.FromHex("0xdd927233d7e89ade8430819f08bf97a087285824af3351ee12d72a2d132b0c6c0687bfaf")
	expectedTrace := &callTrace{
		// Outer transaction call
		Type:   "CALL",
		From:   goldToken.addr,
		To:     transferPrecompile,
		Input:  data,
		Output: data,
		Gas:    newRPCUint64(20112),
		// Note that top-level traces do not currently include intrinsic gas
		GasUsed: newRPCUint64(params.CallValueTransferGas),
		Value:   (*hexutil.Big)(big.NewInt(0)),
		Calls: []callTrace{
			{
				// Lookup of GoldToken contract address with capped gas
				Type:    "STATICCALL",
				From:    common.ZeroAddress,
				To:      registryProxyAddr,
				Input:   packedGetAddressForGoldToken,
				Output:  common.LeftPadBytes(goldToken.addr.Bytes(), 32),
				Gas:     newRPCUint64(params.MaxGasForGetAddressFor),
				GasUsed: newRPCUint64(0),
				Calls: []callTrace{
					{
						// RegistryProxy -> RegistryImpl delegate call to
						// retrieve GoldToken address
						Type:    "DELEGATECALL",
						From:    registryProxyAddr,
						To:      registryImplAddr,
						Input:   packedGetAddressForGoldToken,
						Output:  common.LeftPadBytes(goldToken.addr.Bytes(), 32),
						Gas:     newRPCUint64(0),
						GasUsed: newRPCUint64(0),
					},
				},
			},
		},
	}
	if !jsonEqual(ret, expectedTrace) {
		t.Fatalf("trace mismatch: \nhave %+v\nwant %+v", ret, expectedTrace)
	}
}

func TestTraceBlock(t *testing.T) {
	t.Parallel()

	// Initialize test accounts
	accounts := newAccounts(3)
	genesis := &core.Genesis{Alloc: core.GenesisAlloc{
		accounts[0].addr: {Balance: big.NewInt(params.Ether)},
		accounts[1].addr: {Balance: big.NewInt(params.Ether)},
		accounts[2].addr: {Balance: big.NewInt(params.Ether)},
	}}
	genBlocks := 10
	signer := types.HomesteadSigner{}
	api := NewAPI(newTestBackend(t, genBlocks, genesis, func(i int, b *core.BlockGen) {
		// Transfer from account[0] to account[1]
		//    value: 1000 wei
		//    fee:   0 wei
		tx, _ := types.SignTx(types.NewTransaction(uint64(i), accounts[1].addr, big.NewInt(1000), params.TxGas, nil, nil, nil, nil, nil), signer, accounts[0].key)
		b.AddTx(tx)
	}))

	var testSuite = []struct {
		blockNumber rpc.BlockNumber
		config      *TraceConfig
		expect      interface{}
		expectErr   error
	}{
		// Trace genesis block, expect error
		{
			blockNumber: rpc.BlockNumber(0),
			config:      nil,
			expect:      nil,
			expectErr:   errors.New("genesis is not traceable"),
		},
		// Trace head block
		{
			blockNumber: rpc.BlockNumber(genBlocks),
			config:      nil,
			expectErr:   nil,
			expect: []*txTraceResult{
				{
					Result: &ethapi.ExecutionResult{
						Gas:         params.TxGas,
						Failed:      false,
						ReturnValue: "",
						StructLogs:  []ethapi.StructLogRes{},
					},
				},
			},
		},
		// Trace non-existent block
		{
			blockNumber: rpc.BlockNumber(genBlocks + 1),
			config:      nil,
			expectErr:   fmt.Errorf("block #%d not found", genBlocks+1),
			expect:      nil,
		},
		// Trace latest block
		{
			blockNumber: rpc.LatestBlockNumber,
			config:      nil,
			expectErr:   nil,
			expect: []*txTraceResult{
				{
					Result: &ethapi.ExecutionResult{
						Gas:         params.TxGas,
						Failed:      false,
						ReturnValue: "",
						StructLogs:  []ethapi.StructLogRes{},
					},
				},
			},
		},
		// Trace pending block
		{
			blockNumber: rpc.PendingBlockNumber,
			config:      nil,
			expectErr:   nil,
			expect: []*txTraceResult{
				{
					Result: &ethapi.ExecutionResult{
						Gas:         params.TxGas,
						Failed:      false,
						ReturnValue: "",
						StructLogs:  []ethapi.StructLogRes{},
					},
				},
			},
		},
	}
	for _, testspec := range testSuite {
		result, err := api.TraceBlockByNumber(context.Background(), testspec.blockNumber, testspec.config)
		if testspec.expectErr != nil {
			if err == nil {
				t.Errorf("Expect error %v, get nothing", testspec.expectErr)
				continue
			}
			if !reflect.DeepEqual(err, testspec.expectErr) {
				t.Errorf("Error mismatch, want %v, get %v", testspec.expectErr, err)
			}
		} else {
			if err != nil {
				t.Errorf("Expect no error, get %v", err)
				continue
			}
			if !reflect.DeepEqual(result, testspec.expect) {
				t.Errorf("Result mismatch, want %v, get %v", testspec.expect, result)
			}
		}
	}
}

// Regression test for https://github.com/celo-org/celo-blockchain/issues/2002
// The tracer module didn't correctly calculate gas prices when EIP1559 style
// transactions are used.
func TestTraceBlockWithEIP1559Tx(t *testing.T) {
	// Initialize test accounts
	accounts := newAccounts(2)
	genesis := &core.Genesis{
		Config: params.IstanbulEHFTestChainConfig,
		Alloc: core.GenesisAlloc{
			accounts[0].addr: {Balance: big.NewInt(231001)},
			accounts[1].addr: {Balance: common.Big0},
			common.HexToAddress("0xce10"): { // Registry Proxy
				Code: testutil.RegistryProxyOpcodes,
				Storage: map[common.Hash]common.Hash{
					common.HexToHash("0x360894a13ba1a3210667c828492db98dca3e2076cc3735a920a3ca505d382bbc"): common.HexToHash("0xce11"), // Registry Implementation
					common.HexToHash("0x91646b8507bf2e54d7c3de9155442ba111546b81af1cbdd1f68eeb6926b98d58"): common.HexToHash("0xd023"), // Governance Proxy
				},
				Balance: big.NewInt(0),
			},
			common.HexToAddress("0xce11"): { // Registry Implementation
				Code:    testutil.RegistryOpcodes,
				Balance: big.NewInt(0),
			},
		},
	}

	api := NewAPI(newTestBackend(t, 1, genesis, func(i int, b *core.BlockGen) {
		// The block base fee is mocked to be 3
		// Two transactions are build, so that the correct gas price is 5 (base fee of 3 + tip of 2), but the
		// incorrect calculation leads to a gas price of 6.
		// The account balance is chosen in a way that the second transaction won't be able to execute if the
		// calculation is wrong.

		bf := core.MockSysContractCallCtx().GetGasPriceMinimum(nil)
		tip := big.NewInt(2)
		cap := new(big.Int).Set(common.Big1)
		cap = cap.Add(cap, tip).Add(cap, bf)

		txdata1 := types.NewTx(&types.CeloDynamicFeeTx{
			ChainID:   b.Config().ChainID,
			Nonce:     0,
			GasTipCap: tip,
			GasFeeCap: cap,
			Gas:       21_000,
			To:        &accounts[1].addr,
		})
		tx1, _ := types.SignTx(
			txdata1,
			types.LatestSignerForChainID(b.Config().ChainID),
			accounts[0].key,
		)
		b.AddTx(tx1)

		txdata2 := types.NewTx(&types.CeloDynamicFeeTx{
			ChainID:   b.Config().ChainID,
			Nonce:     1,
			GasTipCap: tip,
			GasFeeCap: cap,
			Gas:       21_000,
			To:        &accounts[1].addr,
		})
		tx2, _ := types.SignTx(
			txdata2,
			types.LatestSignerForChainID(b.Config().ChainID),
			accounts[0].key,
		)
		b.AddTx(tx2)
	}))

	// Run tracing, this should not throw
	_, err := api.TraceBlockByNumber(context.Background(), rpc.LatestBlockNumber, nil)
	if err != nil {
		t.Errorf("Expect no error, get %v", err)
	}
}

type Account struct {
	key  *ecdsa.PrivateKey
	addr common.Address
}

type Accounts []Account

func (a Accounts) Len() int           { return len(a) }
func (a Accounts) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a Accounts) Less(i, j int) bool { return bytes.Compare(a[i].addr.Bytes(), a[j].addr.Bytes()) < 0 }

func newAccounts(n int) (accounts Accounts) {
	for i := 0; i < n; i++ {
		key, _ := crypto.GenerateKey()
		addr := crypto.PubkeyToAddress(key.PublicKey)
		accounts = append(accounts, Account{key: key, addr: addr})
	}
	sort.Sort(accounts)
	return accounts
}

func newRPCBalance(balance *big.Int) **hexutil.Big {
	rpcBalance := (*hexutil.Big)(balance)
	return &rpcBalance
}

func newRPCUint64(number uint64) *hexutil.Uint64 {
	rpcUint64 := hexutil.Uint64(number)
	return &rpcUint64
}

func newRPCBytes(bytes []byte) *hexutil.Bytes {
	rpcBytes := hexutil.Bytes(bytes)
	return &rpcBytes
}

func newStates(keys []common.Hash, vals []common.Hash) *map[common.Hash]common.Hash {
	if len(keys) != len(vals) {
		panic("invalid input")
	}
	m := make(map[common.Hash]common.Hash)
	for i := 0; i < len(keys); i++ {
		m[keys[i]] = vals[i]
	}
	return &m
}
