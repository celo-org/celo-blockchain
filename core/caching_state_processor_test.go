package core

import (
	"math/big"
	"reflect"
	"testing"

	mockEngine "github.com/celo-org/celo-blockchain/consensus/consensustest"
	"github.com/celo-org/celo-blockchain/core/rawdb"
	"github.com/celo-org/celo-blockchain/core/state"
	"github.com/celo-org/celo-blockchain/core/types"
	"github.com/celo-org/celo-blockchain/core/vm"
	"github.com/celo-org/celo-blockchain/params"
)

// TestCache tests
// * cache hit
// * cache miss
func TestCache(t *testing.T) {
	// Generate a canonical chain to act as the main dataset
	engine := mockEngine.NewFaker()

	// Generate and import the canonical chain
	diskdb := rawdb.NewMemoryDatabase()
	new(Genesis).MustCommit(diskdb)
	chain, err := NewBlockChain(diskdb, nil, params.TestChainConfig, engine, vm.Config{}, nil)
	if err != nil {
		t.Fatalf("failed to create tester chain: %v", err)
	}
	root := chain.GetBlockByNumber(0).Root()
	state, _ := state.New(root, chain.stateCache, chain.snaps)
	block1 := makeBlock(100)
	cachingStateProcessor := chain.processor.(*CachingStateProcessor)
	// Before processing first block, shouldn't have any cache entry
	if cachingStateProcessor.cache.Len() != 0 {
		t.Error("Wrong cache length", "expected", 0, "actual", cachingStateProcessor.cache.Len())
	}

	// After processing block1, cache should have length of 1
	cachingStateProcessor.Process(block1, state, chain.vmConfig)
	if cachingStateProcessor.cache.Len() != 1 {
		t.Error("Wrong cache length", "expected", 1, "actual", cachingStateProcessor.cache.Len())
	}

	// After processing block1 again, cache should have length of 2
	cachingStateProcessor.Process(block1, state, chain.vmConfig)
	if cachingStateProcessor.cache.Len() != 2 {
		t.Error("Wrong cache length", "expected", 2, "actual", cachingStateProcessor.cache.Len())
	}
}

// testAlternativeProcessors tests if StateProcessor & CachingStateProcessor will produce expected results.
func testAlternativeProcessors(t *testing.T, block1, block2 *types.Block, same bool) {
	// Generate a canonical chain to act as the main dataset
	engine := mockEngine.NewFaker()

	// Generate and import the canonical chain
	diskdb := rawdb.NewMemoryDatabase()
	new(Genesis).MustCommit(diskdb)
	chain, err := NewBlockChain(diskdb, nil, params.TestChainConfig, engine, vm.Config{}, nil)
	if err != nil {
		t.Fatalf("failed to create tester chain: %v", err)
	}

	cachingStateProcessor := chain.processor
	stateProcessor := NewStateProcessor(chain.chainConfig, chain, engine)

	root := chain.GetBlockByNumber(0).Root()
	state1, _ := state.New(root, chain.stateCache, chain.snaps)
	state2, _ := state.New(root, chain.stateCache, chain.snaps)

	r1, l1, g1, e1 := cachingStateProcessor.Process(block1, state1, chain.vmConfig)
	r2, l2, g2, e2 := stateProcessor.Process(block2, state2, chain.vmConfig)

	if isEqual(r1, r2, l1, l2, g1, g2, e1, e2, state1, state2) != same {
		t.Error("Not expected")
	}
}

// TestAlternativeProcessors tests:
// * Given same block, two processors will give same results
// * Given different blocks, two processors will give different results
func TestAlternativeProcessors(t *testing.T) {
	block1 := makeBlock(1)
	block2 := makeBlock(2)
	testAlternativeProcessors(t, block1, block1, true)
	testAlternativeProcessors(t, block1, block2, false)
}

func makeBlock(number int64) *types.Block {
	header := &types.Header{
		Number:  big.NewInt(number),
		GasUsed: 0,
		Time:    uint64(0),
	}
	return types.NewBlock(header, nil, nil, nil)
}

func isEqual(r1, r2 types.Receipts, l1, l2 []*types.Log, g1, g2 uint64, e1, e2 error, s1, s2 *state.StateDB) bool {
	return reflect.DeepEqual(r1, r2) && reflect.DeepEqual(l1, l2) && reflect.DeepEqual(g1, g2) && reflect.DeepEqual(e1, e2) && reflect.DeepEqual(s1, s2)
	//&& s1.IntermediateRoot(true) == s2.IntermediateRoot(true)
}
