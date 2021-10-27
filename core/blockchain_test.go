// Copyright 2014 The go-ethereum Authors
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
	"errors"
	"fmt"
	"io/ioutil"
	"math/big"
	"math/rand"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/consensus"
	mockEngine "github.com/celo-org/celo-blockchain/consensus/consensustest"
	"github.com/celo-org/celo-blockchain/core/rawdb"
	"github.com/celo-org/celo-blockchain/core/state"
	"github.com/celo-org/celo-blockchain/core/types"
	"github.com/celo-org/celo-blockchain/core/vm"
	"github.com/celo-org/celo-blockchain/crypto"
	"github.com/celo-org/celo-blockchain/ethdb"
	"github.com/celo-org/celo-blockchain/params"
	"github.com/celo-org/celo-blockchain/trie"
)

// So we can deterministically seed different blockchains
var (
	canonicalSeed = 1
	forkSeed      = 2
)

// newCanonical creates a chain database, and injects a deterministic canonical
// chain. Depending on the full flag, if creates either a full block chain or a
// header only chain.
func newCanonical(engine consensus.Engine, n int, full bool) (ethdb.Database, *BlockChain, error) {
	var (
		db      = rawdb.NewMemoryDatabase()
		genesis = new(Genesis).MustCommit(db)
	)

	// Initialize a fresh chain with only a genesis block
	blockchain, _ := NewBlockChain(db, nil, params.IstanbulTestChainConfig, engine, vm.Config{}, nil, nil)
	// Create and inject the requested chain
	if n == 0 {
		return db, blockchain, nil
	}
	if full {
		// Full block-chain requested
		blocks := makeBlockChain(genesis, n, engine, db, canonicalSeed)
		_, err := blockchain.InsertChain(blocks)
		return db, blockchain, err
	}
	// Header-only chain requested
	headers := makeHeaderChain(genesis.Header(), n, engine, db, canonicalSeed)
	_, err := blockchain.InsertHeaderChain(headers, 1, true)
	return db, blockchain, err
}

func newGwei(n int64) *big.Int {
	return new(big.Int).Mul(big.NewInt(n), big.NewInt(params.GWei))
}

// Test fork of length N starting from block i
func testFork(t *testing.T, blockchain *BlockChain, i, n int, full bool, comparator func(td1, td2 *big.Int)) {
	// Copy old chain up to #i into a new db
	db, blockchain2, err := newCanonical(mockEngine.NewFaker(), i, full)
	if err != nil {
		t.Fatal("could not make new canonical in testFork", err)
	}
	defer blockchain2.Stop()

	// Assert the chains have the same header/block at #i
	var hash1, hash2 common.Hash
	if full {
		hash1 = blockchain.GetBlockByNumber(uint64(i)).Hash()
		hash2 = blockchain2.GetBlockByNumber(uint64(i)).Hash()
	} else {
		hash1 = blockchain.GetHeaderByNumber(uint64(i)).Hash()
		hash2 = blockchain2.GetHeaderByNumber(uint64(i)).Hash()
	}
	if hash1 != hash2 {
		t.Errorf("chain content mismatch at %d: have hash %v, want hash %v", i, hash2, hash1)
	}
	// Extend the newly created chain
	var (
		blockChainB  []*types.Block
		headerChainB []*types.Header
	)
	if full {
		blockChainB = makeBlockChain(blockchain2.CurrentBlock(), n, mockEngine.NewFaker(), db, forkSeed)
		if _, err := blockchain2.InsertChain(blockChainB); err != nil {
			t.Fatalf("failed to insert forking chain: %v", err)
		}
	} else {
		headerChainB = makeHeaderChain(blockchain2.CurrentHeader(), n, mockEngine.NewFaker(), db, forkSeed)
		if _, err := blockchain2.InsertHeaderChain(headerChainB, 1, true); err != nil {
			t.Fatalf("failed to insert forking chain: %v", err)
		}
	}
	// Sanity check that the forked chain can be imported into the original
	var tdPre, tdPost *big.Int

	if full {
		tdPre = blockchain.GetTdByHash(blockchain.CurrentBlock().Hash())
		if err := testBlockChainImport(blockChainB, blockchain); err != nil {
			t.Fatalf("failed to import forked block chain: %v", err)
		}
		tdPost = blockchain.GetTdByHash(blockChainB[len(blockChainB)-1].Hash())
	} else {
		tdPre = blockchain.GetTdByHash(blockchain.CurrentHeader().Hash())
		if err := testHeaderChainImport(headerChainB, blockchain); err != nil {
			t.Fatalf("failed to import forked header chain: %v", err)
		}
		tdPost = blockchain.GetTdByHash(headerChainB[len(headerChainB)-1].Hash())
	}
	// Compare the total difficulties of the chains
	comparator(tdPre, tdPost)
}

// testBlockChainImport tries to process a chain of blocks, writing them into
// the database if successful.
func testBlockChainImport(chain types.Blocks, blockchain *BlockChain) error {
	for _, block := range chain {
		// Try and process the block
		err := blockchain.engine.VerifyHeader(blockchain, block.Header(), true)
		if err == nil {
			err = blockchain.validator.ValidateBody(block)
		}
		if err != nil {
			if err == ErrKnownBlock {
				continue
			}
			return err
		}
		statedb, err := state.New(blockchain.GetBlockByHash(block.ParentHash()).Root(), blockchain.stateCache, nil)
		if err != nil {
			return err
		}
		receipts, _, usedGas, err := blockchain.processor.Process(block, statedb, vm.Config{})
		if err != nil {
			blockchain.reportBlock(block, receipts, err)
			return err
		}
		err = blockchain.validator.ValidateState(block, statedb, receipts, usedGas)
		if err != nil {
			blockchain.reportBlock(block, receipts, err)
			return err
		}
		blockchain.chainmu.Lock()
		rawdb.WriteTd(blockchain.db, block.Hash(), block.NumberU64(), block.TotalDifficulty())
		rawdb.WriteBlock(blockchain.db, block)
		statedb.Commit(false)
		blockchain.chainmu.Unlock()
	}
	return nil
}

// testHeaderChainImport tries to process a chain of header, writing them into
// the database if successful.
func testHeaderChainImport(chain []*types.Header, blockchain *BlockChain) error {
	for _, header := range chain {
		// Try and validate the header
		if err := blockchain.engine.VerifyHeader(blockchain, header, false); err != nil {
			return err
		}
		// Manually insert the header into the database, but don't reorganise (allows subsequent testing)
		blockchain.chainmu.Lock()
		rawdb.WriteTd(blockchain.db, header.Hash(), header.Number.Uint64(), new(big.Int).Add(header.Number, big.NewInt(1)))
		rawdb.WriteHeader(blockchain.db, header)
		blockchain.chainmu.Unlock()
	}
	return nil
}

func TestLastBlock(t *testing.T) {
	_, blockchain, err := newCanonical(mockEngine.NewFaker(), 0, true)
	if err != nil {
		t.Fatalf("failed to create pristine chain: %v", err)
	}
	defer blockchain.Stop()

	blocks := makeBlockChain(blockchain.CurrentBlock(), 1, mockEngine.NewFullFaker(), blockchain.db, 0)
	if _, err := blockchain.InsertChain(blocks); err != nil {
		t.Fatalf("Failed to insert block: %v", err)
	}
	if blocks[len(blocks)-1].Hash() != rawdb.ReadHeadBlockHash(blockchain.db) {
		t.Fatalf("Write/Get HeadBlockHash failed")
	}
}

// Tests that given a starting canonical chain of a given size, it can be extended
// with various length chains.
func TestExtendCanonicalHeaders(t *testing.T) { testExtendCanonical(t, false) }
func TestExtendCanonicalBlocks(t *testing.T)  { testExtendCanonical(t, true) }

func testExtendCanonical(t *testing.T, full bool) {
	length := 5

	// Make first chain starting from genesis
	_, processor, err := newCanonical(mockEngine.NewFaker(), length, full)
	if err != nil {
		t.Fatalf("failed to make new canonical chain: %v", err)
	}
	defer processor.Stop()

	// Define the difficulty comparator
	better := func(td1, td2 *big.Int) {
		if td2.Cmp(td1) <= 0 {
			t.Errorf("total difficulty mismatch: have %v, expected more than %v", td2, td1)
		}
	}
	// Start fork from current height
	testFork(t, processor, length, 1, full, better)
	testFork(t, processor, length, 2, full, better)
	testFork(t, processor, length, 5, full, better)
	testFork(t, processor, length, 10, full, better)
}

// Tests that given a starting canonical chain of a given size, creating shorter
// forks do not take canonical ownership.
func TestShorterForkHeaders(t *testing.T) { testShorterFork(t, false) }
func TestShorterForkBlocks(t *testing.T)  { testShorterFork(t, true) }

func testShorterFork(t *testing.T, full bool) {
	length := 10

	// Make first chain starting from genesis
	_, processor, err := newCanonical(mockEngine.NewFaker(), length, full)
	if err != nil {
		t.Fatalf("failed to make new canonical chain: %v", err)
	}
	defer processor.Stop()

	// Define the difficulty comparator
	worse := func(td1, td2 *big.Int) {
		if td2.Cmp(td1) >= 0 {
			t.Errorf("total difficulty mismatch: have %v, expected less than %v", td2, td1)
		}
	}
	// Sum of numbers must be less than `length` for this to be a shorter fork
	testFork(t, processor, 0, 3, full, worse)
	testFork(t, processor, 0, 7, full, worse)
	testFork(t, processor, 1, 1, full, worse)
	testFork(t, processor, 1, 7, full, worse)
	testFork(t, processor, 5, 3, full, worse)
	testFork(t, processor, 5, 4, full, worse)
}

// Tests that given a starting canonical chain of a given size, creating longer
// forks do take canonical ownership.
func TestLongerForkHeaders(t *testing.T) { testLongerFork(t, false) }
func TestLongerForkBlocks(t *testing.T)  { testLongerFork(t, true) }

func testLongerFork(t *testing.T, full bool) {
	length := 10

	// Make first chain starting from genesis
	_, processor, err := newCanonical(mockEngine.NewFaker(), length, full)
	if err != nil {
		t.Fatalf("failed to make new canonical chain: %v", err)
	}
	defer processor.Stop()

	// Define the difficulty comparator
	better := func(td1, td2 *big.Int) {
		if td2.Cmp(td1) <= 0 {
			t.Errorf("total difficulty mismatch: have %v, expected more than %v", td2, td1)
		}
	}
	// Sum of numbers must be greater than `length` for this to be a longer fork
	testFork(t, processor, 0, 11, full, better)
	testFork(t, processor, 0, 15, full, better)
	testFork(t, processor, 1, 10, full, better)
	testFork(t, processor, 1, 12, full, better)
	testFork(t, processor, 5, 6, full, better)
	testFork(t, processor, 5, 8, full, better)
}

// Tests that given a starting canonical chain of a given size, creating equal
// forks do take canonical ownership.
func TestEqualForkHeaders(t *testing.T) { testEqualFork(t, false) }
func TestEqualForkBlocks(t *testing.T)  { testEqualFork(t, true) }

func testEqualFork(t *testing.T, full bool) {
	length := 10

	// Make first chain starting from genesis
	_, processor, err := newCanonical(mockEngine.NewFaker(), length, full)
	if err != nil {
		t.Fatalf("failed to make new canonical chain: %v", err)
	}
	defer processor.Stop()

	// Define the difficulty comparator
	equal := func(td1, td2 *big.Int) {
		if td2.Cmp(td1) != 0 {
			t.Errorf("total difficulty mismatch: have %v, want %v", td2, td1)
		}
	}
	// Sum of numbers must be equal to `length` for this to be an equal fork
	testFork(t, processor, 0, 10, full, equal)
	testFork(t, processor, 1, 9, full, equal)
	testFork(t, processor, 2, 8, full, equal)
	testFork(t, processor, 5, 5, full, equal)
	testFork(t, processor, 6, 4, full, equal)
	testFork(t, processor, 9, 1, full, equal)
}

// Tests that chains missing links do not get accepted by the processor.
func TestBrokenHeaderChain(t *testing.T) { testBrokenChain(t, false) }
func TestBrokenBlockChain(t *testing.T)  { testBrokenChain(t, true) }

func testBrokenChain(t *testing.T, full bool) {
	// Make chain starting from genesis
	db, blockchain, err := newCanonical(mockEngine.NewFaker(), 10, full)
	if err != nil {
		t.Fatalf("failed to make new canonical chain: %v", err)
	}
	defer blockchain.Stop()

	// Create a forked chain, and try to insert with a missing link
	if full {
		chain := makeBlockChain(blockchain.CurrentBlock(), 5, mockEngine.NewFaker(), db, forkSeed)[1:]
		if err := testBlockChainImport(chain, blockchain); err == nil {
			t.Errorf("broken block chain not reported")
		}
	} else {
		chain := makeHeaderChain(blockchain.CurrentHeader(), 5, mockEngine.NewFaker(), db, forkSeed)[1:]
		if err := testHeaderChainImport(chain, blockchain); err == nil {
			t.Errorf("broken header chain not reported")
		}
	}
}

// Tests that reorganising a long difficult chain after a short easy one
// overwrites the canonical numbers and links in the database.
func TestReorgLongHeaders(t *testing.T) { testReorgLong(t, false) }
func TestReorgLongBlocks(t *testing.T)  { testReorgLong(t, true) }

func testReorgLong(t *testing.T, full bool) {
	testReorg(t, []int64{0, 0, -9}, []int64{0, 0, 0, -9}, 5, full)
}

// Tests that reorganising a short difficult chain after a long easy one
// overwrites the canonical numbers and links in the database.
func TestReorgShortHeaders(t *testing.T) { testReorgShort(t, false) }
func TestReorgShortBlocks(t *testing.T)  { testReorgShort(t, true) }

func testReorgShort(t *testing.T, full bool) {
	// Create a long easy chain vs. a short heavy one. Due to difficulty adjustment
	// we need a fairly long chain of blocks with different difficulties for a short
	// one to become heavyer than a long one. The 96 is an empirical value.
	easy := make([]int64, 96)
	for i := 0; i < len(easy); i++ {
		easy[i] = 60
	}
	diff := make([]int64, len(easy)-1)
	for i := 0; i < len(diff); i++ {
		diff[i] = -9
	}
	testReorg(t, easy, diff, 97, full)
}

func testReorg(t *testing.T, first, second []int64, td int64, full bool) {
	// Create a pristine chain and database
	db, blockchain, err := newCanonical(mockEngine.NewFaker(), 0, full)
	if err != nil {
		t.Fatalf("failed to create pristine chain: %v", err)
	}
	defer blockchain.Stop()

	// Insert an easy and a difficult chain afterwards
	easyBlocks, _ := GenerateChain(params.IstanbulTestChainConfig, blockchain.CurrentBlock(), mockEngine.NewFaker(), db, len(first), func(i int, b *BlockGen) {
		b.OffsetTime(first[i])
	})
	diffBlocks, _ := GenerateChain(params.IstanbulTestChainConfig, blockchain.CurrentBlock(), mockEngine.NewFaker(), db, len(second), func(i int, b *BlockGen) {
		b.OffsetTime(second[i])
	})
	if full {
		if _, err := blockchain.InsertChain(easyBlocks); err != nil {
			t.Fatalf("failed to insert easy chain: %v", err)
		}
		if _, err := blockchain.InsertChain(diffBlocks); err != nil {
			t.Fatalf("failed to insert difficult chain: %v", err)
		}
	} else {
		easyHeaders := make([]*types.Header, len(easyBlocks))
		for i, block := range easyBlocks {
			easyHeaders[i] = block.Header()
		}
		diffHeaders := make([]*types.Header, len(diffBlocks))
		for i, block := range diffBlocks {
			diffHeaders[i] = block.Header()
		}
		if _, err := blockchain.InsertHeaderChain(easyHeaders, 1, true); err != nil {
			t.Fatalf("failed to insert easy chain: %v", err)
		}
		if _, err := blockchain.InsertHeaderChain(diffHeaders, 1, true); err != nil {
			t.Fatalf("failed to insert difficult chain: %v", err)
		}
	}
	// Check that the chain is valid number and link wise
	if full {
		prev := blockchain.CurrentBlock()
		for block := blockchain.GetBlockByNumber(blockchain.CurrentBlock().NumberU64() - 1); block.NumberU64() != 0; prev, block = block, blockchain.GetBlockByNumber(block.NumberU64()-1) {
			if prev.ParentHash() != block.Hash() {
				t.Errorf("parent block hash mismatch: have %x, want %x", prev.ParentHash(), block.Hash())
			}
		}
	} else {
		prev := blockchain.CurrentHeader()
		for header := blockchain.GetHeaderByNumber(blockchain.CurrentHeader().Number.Uint64() - 1); header.Number.Uint64() != 0; prev, header = header, blockchain.GetHeaderByNumber(header.Number.Uint64()-1) {
			if prev.ParentHash != header.Hash() {
				t.Errorf("parent header hash mismatch: have %x, want %x", prev.ParentHash, header.Hash())
			}
		}
	}
	// Make sure the chain total difficulty is the correct one
	want := big.NewInt(td)
	if full {
		if have := blockchain.GetTdByHash(blockchain.CurrentBlock().Hash()); have.Cmp(want) != 0 {
			t.Errorf("total difficulty mismatch: have %v, want %v", have, want)
		}
	} else {
		if have := blockchain.GetTdByHash(blockchain.CurrentHeader().Hash()); have.Cmp(want) != 0 {
			t.Errorf("total difficulty mismatch: have %v, want %v", have, want)
		}
	}
}

// Tests that the insertion functions detect banned hashes.
func TestBadHeaderHashes(t *testing.T) { testBadHashes(t, false) }
func TestBadBlockHashes(t *testing.T)  { testBadHashes(t, true) }

func testBadHashes(t *testing.T, full bool) {
	// Create a pristine chain and database
	db, blockchain, err := newCanonical(mockEngine.NewFaker(), 0, full)
	if err != nil {
		t.Fatalf("failed to create pristine chain: %v", err)
	}
	defer blockchain.Stop()

	// Create a chain, ban a hash and try to import
	if full {
		blocks := makeBlockChain(blockchain.CurrentBlock(), 3, mockEngine.NewFaker(), db, 10)

		BadHashes[blocks[2].Header().Hash()] = true
		defer func() { delete(BadHashes, blocks[2].Header().Hash()) }()

		_, err = blockchain.InsertChain(blocks)
	} else {
		headers := makeHeaderChain(blockchain.CurrentHeader(), 3, mockEngine.NewFaker(), db, 10)

		BadHashes[headers[2].Hash()] = true
		defer func() { delete(BadHashes, headers[2].Hash()) }()

		_, err = blockchain.InsertHeaderChain(headers, 1, true)
	}
	if !errors.Is(err, ErrBannedHash) {
		t.Errorf("error mismatch: have: %v, want: %v", err, ErrBannedHash)
	}
}

// Tests that bad hashes are detected on boot, and the chain rolled back to a
// good state prior to the bad hash.
func TestReorgBadHeaderHashes(t *testing.T) { testReorgBadHashes(t, false) }
func TestReorgBadBlockHashes(t *testing.T)  { testReorgBadHashes(t, true) }

func testReorgBadHashes(t *testing.T, full bool) {
	// Create a pristine chain and database
	db, blockchain, err := newCanonical(mockEngine.NewFaker(), 0, full)
	if err != nil {
		t.Fatalf("failed to create pristine chain: %v", err)
	}
	// Create a chain, import and ban afterwards
	headers := makeHeaderChain(blockchain.CurrentHeader(), 4, mockEngine.NewFaker(), db, 10)
	blocks := makeBlockChain(blockchain.CurrentBlock(), 4, mockEngine.NewFaker(), db, 10)

	if full {
		if _, err = blockchain.InsertChain(blocks); err != nil {
			t.Errorf("failed to import blocks: %v", err)
		}
		if blockchain.CurrentBlock().Hash() != blocks[3].Hash() {
			t.Errorf("last block hash mismatch: have: %x, want %x", blockchain.CurrentBlock().Hash(), blocks[3].Header().Hash())
		}
		BadHashes[blocks[3].Header().Hash()] = true
		defer func() { delete(BadHashes, blocks[3].Header().Hash()) }()
	} else {
		if _, err = blockchain.InsertHeaderChain(headers, 1, true); err != nil {
			t.Errorf("failed to import headers: %v", err)
		}
		if blockchain.CurrentHeader().Hash() != headers[3].Hash() {
			t.Errorf("last header hash mismatch: have: %x, want %x", blockchain.CurrentHeader().Hash(), headers[3].Hash())
		}
		BadHashes[headers[3].Hash()] = true
		defer func() { delete(BadHashes, headers[3].Hash()) }()
	}
	blockchain.Stop()

	// Create a new BlockChain and check that it rolled back the state.
	ncm, err := NewBlockChain(blockchain.db, nil, blockchain.chainConfig, mockEngine.NewFaker(), vm.Config{}, nil, nil)
	if err != nil {
		t.Fatalf("failed to create new chain manager: %v", err)
	}
	if full {
		if ncm.CurrentBlock().Hash() != blocks[2].Header().Hash() {
			t.Errorf("last block hash mismatch: have: %x, want %x", ncm.CurrentBlock().Hash(), blocks[2].Header().Hash())
		}
	} else {
		if ncm.CurrentHeader().Hash() != headers[2].Hash() {
			t.Errorf("last header hash mismatch: have: %x, want %x", ncm.CurrentHeader().Hash(), headers[2].Hash())
		}
	}
	ncm.Stop()
}

// Tests chain insertions in the face of one entity containing an invalid nonce.
func TestHeadersInsertNonceError(t *testing.T) { testInsertNonceError(t, false) }
func TestBlocksInsertNonceError(t *testing.T)  { testInsertNonceError(t, true) }

func testInsertNonceError(t *testing.T, full bool) {
	for i := 1; i < 25 && !t.Failed(); i++ {
		// Create a pristine chain and database
		db, blockchain, err := newCanonical(mockEngine.NewFaker(), 0, full)
		if err != nil {
			t.Fatalf("failed to create pristine chain: %v", err)
		}
		defer blockchain.Stop()

		// Create and insert a chain with a failing nonce
		var (
			failAt  int
			failRes int
			failNum uint64
		)
		if full {
			blocks := makeBlockChain(blockchain.CurrentBlock(), i, mockEngine.NewFaker(), db, 0)

			failAt = rand.Int() % len(blocks)
			failNum = blocks[failAt].NumberU64()

			blockchain.engine = mockEngine.NewFakeFailer(failNum)
			failRes, err = blockchain.InsertChain(blocks)
		} else {
			headers := makeHeaderChain(blockchain.CurrentHeader(), i, mockEngine.NewFaker(), db, 0)

			failAt = rand.Int() % len(headers)
			failNum = headers[failAt].Number.Uint64()

			blockchain.engine = mockEngine.NewFakeFailer(failNum)
			blockchain.hc.engine = blockchain.engine
			failRes, err = blockchain.InsertHeaderChain(headers, 1, true)
		}
		// Check that the returned error indicates the failure
		if failRes != failAt {
			t.Errorf("test %d: failure (%v) index mismatch: have %d, want %d", i, err, failRes, failAt)
		}
		// Check that all blocks after the failing block have been inserted
		for j := 0; j < i-failAt; j++ {
			if full {
				if block := blockchain.GetBlockByNumber(failNum + uint64(j)); block != nil {
					t.Errorf("test %d: invalid block in chain: %v", i, block)
				}
			} else {
				if header := blockchain.GetHeaderByNumber(failNum + uint64(j)); header != nil {
					t.Errorf("test %d: invalid header in chain: %v", i, header)
				}
			}
		}
	}
}

// Tests that fast importing a block chain produces the same chain data as the
// classical full block processing.
func TestFastVsFullChains(t *testing.T) {
	// Configure and generate a sample block chain
	var (
		gendb   = rawdb.NewMemoryDatabase()
		key, _  = crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")
		address = crypto.PubkeyToAddress(key.PublicKey)
		funds   = big.NewInt(1000000000000000)
		gspec   = &Genesis{
			Config: params.IstanbulTestChainConfig,
			Alloc:  GenesisAlloc{address: {Balance: funds}},
		}
		genesis = gspec.MustCommit(gendb)
		signer  = types.LatestSigner(gspec.Config)
	)
	blocks, receipts := GenerateChain(gspec.Config, genesis, mockEngine.NewFaker(), gendb, 1024, func(i int, block *BlockGen) {
		block.SetCoinbase(common.Address{0x00})

		// If the block number is multiple of 3, send a few bonus transactions to the miner
		if i%3 == 2 {
			for j := 0; j < i%4+1; j++ {
				tx, err := types.SignTx(types.NewTransaction(block.TxNonce(address), common.Address{0x00}, big.NewInt(1000), params.TxGas, nil, nil, nil, nil, nil), signer, key)
				if err != nil {
					panic(err)
				}
				block.AddTx(tx)
			}
		}
	})
	// Import the chain as an archive node for the comparison baseline
	archiveDb := rawdb.NewMemoryDatabase()
	gspec.MustCommit(archiveDb)
	archive, _ := NewBlockChain(archiveDb, nil, gspec.Config, mockEngine.NewFaker(), vm.Config{}, nil, nil)
	defer archive.Stop()

	if n, err := archive.InsertChain(blocks); err != nil {
		t.Fatalf("failed to process block %d: %v", n, err)
	}
	// Fast import the chain as a non-archive node to test
	fastDb := rawdb.NewMemoryDatabase()
	gspec.MustCommit(fastDb)
	fast, _ := NewBlockChain(fastDb, nil, gspec.Config, mockEngine.NewFaker(), vm.Config{}, nil, nil)
	defer fast.Stop()

	headers := make([]*types.Header, len(blocks))
	for i, block := range blocks {
		headers[i] = block.Header()
	}
	if n, err := fast.InsertHeaderChain(headers, 1, true); err != nil {
		t.Fatalf("failed to insert header %d: %v", n, err)
	}
	if n, err := fast.InsertReceiptChain(blocks, receipts, 0); err != nil {
		t.Fatalf("failed to insert receipt %d: %v", n, err)
	}
	// Freezer style fast import the chain.
	frdir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatalf("failed to create temp freezer dir: %v", err)
	}
	defer os.Remove(frdir)
	ancientDb, err := rawdb.NewDatabaseWithFreezer(rawdb.NewMemoryDatabase(), frdir, "", false)
	if err != nil {
		t.Fatalf("failed to create temp freezer db: %v", err)
	}
	gspec.MustCommit(ancientDb)
	ancient, _ := NewBlockChain(ancientDb, nil, gspec.Config, mockEngine.NewFaker(), vm.Config{}, nil, nil)
	defer ancient.Stop()

	if n, err := ancient.InsertHeaderChain(headers, 1, true); err != nil {
		t.Fatalf("failed to insert header %d: %v", n, err)
	}
	if n, err := ancient.InsertReceiptChain(blocks, receipts, uint64(len(blocks)/2)); err != nil {
		t.Fatalf("failed to insert receipt %d: %v", n, err)
	}
	// Iterate over all chain data components, and cross reference
	for i := 0; i < len(blocks); i++ {
		num, hash := blocks[i].NumberU64(), blocks[i].Hash()

		if ftd, atd := fast.GetTdByHash(hash), archive.GetTdByHash(hash); ftd.Cmp(atd) != 0 {
			t.Errorf("block #%d [%x]: td mismatch: fastdb %v, archivedb %v", num, hash, ftd, atd)
		}
		if antd, artd := ancient.GetTdByHash(hash), archive.GetTdByHash(hash); antd.Cmp(artd) != 0 {
			t.Errorf("block #%d [%x]: td mismatch: ancientdb %v, archivedb %v", num, hash, antd, artd)
		}
		if fheader, aheader := fast.GetHeaderByHash(hash), archive.GetHeaderByHash(hash); fheader.Hash() != aheader.Hash() {
			t.Errorf("block #%d [%x]: header mismatch: fastdb %v, archivedb %v", num, hash, fheader, aheader)
		}
		if anheader, arheader := ancient.GetHeaderByHash(hash), archive.GetHeaderByHash(hash); anheader.Hash() != arheader.Hash() {
			t.Errorf("block #%d [%x]: header mismatch: ancientdb %v, archivedb %v", num, hash, anheader, arheader)
		}
		if fblock, arblock, anblock := fast.GetBlockByHash(hash), archive.GetBlockByHash(hash), ancient.GetBlockByHash(hash); fblock.Hash() != arblock.Hash() || anblock.Hash() != arblock.Hash() {
			t.Errorf("block #%d [%x]: block mismatch: fastdb %v, ancientdb %v, archivedb %v", num, hash, fblock, anblock, arblock)
		} else if types.DeriveSha(fblock.Transactions(), trie.NewStackTrie(nil)) != types.DeriveSha(arblock.Transactions(), trie.NewStackTrie(nil)) || types.DeriveSha(anblock.Transactions(), trie.NewStackTrie(nil)) != types.DeriveSha(arblock.Transactions(), trie.NewStackTrie(nil)) {
			t.Errorf("block #%d [%x]: transactions mismatch: fastdb %v, ancientdb %v, archivedb %v", num, hash, fblock.Transactions(), anblock.Transactions(), arblock.Transactions())
		}
		if freceipts, anreceipts, areceipts := rawdb.ReadReceipts(fastDb, hash, *rawdb.ReadHeaderNumber(fastDb, hash), fast.Config()), rawdb.ReadReceipts(ancientDb, hash, *rawdb.ReadHeaderNumber(ancientDb, hash), fast.Config()), rawdb.ReadReceipts(archiveDb, hash, *rawdb.ReadHeaderNumber(archiveDb, hash), fast.Config()); types.DeriveSha(freceipts, trie.NewStackTrie(nil)) != types.DeriveSha(areceipts, trie.NewStackTrie(nil)) {
			t.Errorf("block #%d [%x]: receipts mismatch: fastdb %v, ancientdb %v, archivedb %v", num, hash, freceipts, anreceipts, areceipts)
		}
	}
	// Check that the canonical chains are the same between the databases
	for i := 0; i < len(blocks)+1; i++ {
		if fhash, ahash := rawdb.ReadCanonicalHash(fastDb, uint64(i)), rawdb.ReadCanonicalHash(archiveDb, uint64(i)); fhash != ahash {
			t.Errorf("block #%d: canonical hash mismatch: fastdb %v, archivedb %v", i, fhash, ahash)
		}
		if anhash, arhash := rawdb.ReadCanonicalHash(ancientDb, uint64(i)), rawdb.ReadCanonicalHash(archiveDb, uint64(i)); anhash != arhash {
			t.Errorf("block #%d: canonical hash mismatch: ancientdb %v, archivedb %v", i, anhash, arhash)
		}
	}
}

// Tests that various import methods move the chain head pointers to the correct
// positions.
func TestLightVsFastVsFullChainHeads(t *testing.T) {
	// Configure and generate a sample block chain
	var (
		gendb   = rawdb.NewMemoryDatabase()
		key, _  = crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")
		address = crypto.PubkeyToAddress(key.PublicKey)
		funds   = big.NewInt(1000000000)
		gspec   = &Genesis{Config: params.IstanbulTestChainConfig, Alloc: GenesisAlloc{address: {Balance: funds}}}
		genesis = gspec.MustCommit(gendb)
	)
	height := uint64(1024)
	blocks, receipts := GenerateChain(gspec.Config, genesis, mockEngine.NewFaker(), gendb, int(height), nil)

	// makeDb creates a db instance for testing.
	makeDb := func() (ethdb.Database, func()) {
		dir, err := ioutil.TempDir("", "")
		if err != nil {
			t.Fatalf("failed to create temp freezer dir: %v", err)
		}
		defer os.Remove(dir)
		db, err := rawdb.NewDatabaseWithFreezer(rawdb.NewMemoryDatabase(), dir, "", false)
		if err != nil {
			t.Fatalf("failed to create temp freezer db: %v", err)
		}
		gspec.MustCommit(db)
		return db, func() { os.RemoveAll(dir) }
	}
	// Configure a subchain to roll back
	remove := blocks[height/2].NumberU64()

	// Create a small assertion method to check the three heads
	assert := func(t *testing.T, kind string, chain *BlockChain, header uint64, fast uint64, block uint64) {
		t.Helper()

		if num := chain.CurrentBlock().NumberU64(); num != block {
			t.Errorf("%s head block mismatch: have #%v, want #%v", kind, num, block)
		}
		if num := chain.CurrentFastBlock().NumberU64(); num != fast {
			t.Errorf("%s head fast-block mismatch: have #%v, want #%v", kind, num, fast)
		}
		if num := chain.CurrentHeader().Number.Uint64(); num != header {
			t.Errorf("%s head header mismatch: have #%v, want #%v", kind, num, header)
		}
	}
	// Import the chain as an archive node and ensure all pointers are updated
	archiveDb, delfn := makeDb()
	defer delfn()

	archiveCaching := *defaultCacheConfig
	archiveCaching.TrieDirtyDisabled = true

	archive, _ := NewBlockChain(archiveDb, &archiveCaching, gspec.Config, mockEngine.NewFaker(), vm.Config{}, nil, nil)
	if n, err := archive.InsertChain(blocks); err != nil {
		t.Fatalf("failed to process block %d: %v", n, err)
	}
	defer archive.Stop()

	assert(t, "archive", archive, height, height, height)
	archive.SetHead(remove - 1)
	assert(t, "archive", archive, height/2, height/2, height/2)

	// Import the chain as a non-archive node and ensure all pointers are updated
	fastDb, delfn := makeDb()
	defer delfn()
	fast, _ := NewBlockChain(fastDb, nil, gspec.Config, mockEngine.NewFaker(), vm.Config{}, nil, nil)
	defer fast.Stop()

	headers := make([]*types.Header, len(blocks))
	for i, block := range blocks {
		headers[i] = block.Header()
	}
	if n, err := fast.InsertHeaderChain(headers, 1, true); err != nil {
		t.Fatalf("failed to insert header %d: %v", n, err)
	}
	if n, err := fast.InsertReceiptChain(blocks, receipts, 0); err != nil {
		t.Fatalf("failed to insert receipt %d: %v", n, err)
	}
	assert(t, "fast", fast, height, height, 0)
	fast.SetHead(remove - 1)
	assert(t, "fast", fast, height/2, height/2, 0)

	// Import the chain as a ancient-first node and ensure all pointers are updated
	ancientDb, delfn := makeDb()
	defer delfn()
	ancient, _ := NewBlockChain(ancientDb, nil, gspec.Config, mockEngine.NewFaker(), vm.Config{}, nil, nil)
	defer ancient.Stop()

	if n, err := ancient.InsertHeaderChain(headers, 1, true); err != nil {
		t.Fatalf("failed to insert header %d: %v", n, err)
	}
	if n, err := ancient.InsertReceiptChain(blocks, receipts, uint64(3*len(blocks)/4)); err != nil {
		t.Fatalf("failed to insert receipt %d: %v", n, err)
	}
	assert(t, "ancient", ancient, height, height, 0)
	ancient.SetHead(remove - 1)
	assert(t, "ancient", ancient, 0, 0, 0)

	if frozen, err := ancientDb.Ancients(); err != nil || frozen != 1 {
		t.Fatalf("failed to truncate ancient store, want %v, have %v", 1, frozen)
	}
	// Import the chain as a light node and ensure all pointers are updated
	lightDb, delfn := makeDb()
	defer delfn()
	light, _ := NewBlockChain(lightDb, nil, gspec.Config, mockEngine.NewFaker(), vm.Config{}, nil, nil)
	if n, err := light.InsertHeaderChain(headers, 1, true); err != nil {
		t.Fatalf("failed to insert header %d: %v", n, err)
	}
	defer light.Stop()

	assert(t, "light", light, height, 0, 0)
	light.SetHead(remove - 1)
	assert(t, "light", light, height/2, 0, 0)
}

// Tests that chain reorganisations handle transaction removals and reinsertions.
func TestChainTxReorgs(t *testing.T) {
	var (
		key1, _ = crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")
		key2, _ = crypto.HexToECDSA("8a1f9a8f95be41cd7ccb6168179afb4504aefe388d1e14474d32c45c72ce7b7a")
		key3, _ = crypto.HexToECDSA("49a7b37aa6f6645917e7b807e9d1c00d4fa71f18343b0d4122a4d2df64dd6fee")
		addr1   = crypto.PubkeyToAddress(key1.PublicKey)
		addr2   = crypto.PubkeyToAddress(key2.PublicKey)
		addr3   = crypto.PubkeyToAddress(key3.PublicKey)
		db      = rawdb.NewMemoryDatabase()
		gspec   = &Genesis{
			Config: params.IstanbulTestChainConfig,
			Alloc: GenesisAlloc{
				addr1: {Balance: big.NewInt(1000000000000000)},
				addr2: {Balance: big.NewInt(1000000000000000)},
				addr3: {Balance: big.NewInt(1000000000000000)},
			},
		}
		genesis = gspec.MustCommit(db)
		signer  = types.LatestSigner(gspec.Config)
	)

	// Create two transactions shared between the chains:
	//  - postponed: transaction included at a later block in the forked chain
	//  - swapped: transaction included at the same block number in the forked chain
	postponed, _ := types.SignTx(types.NewTransaction(0, addr1, big.NewInt(1000), params.TxGas, nil, nil, nil, nil, nil), signer, key1)
	swapped, _ := types.SignTx(types.NewTransaction(1, addr1, big.NewInt(1000), params.TxGas, nil, nil, nil, nil, nil), signer, key1)

	// Create two transactions that will be dropped by the forked chain:
	//  - pastDrop: transaction dropped retroactively from a past block
	//  - freshDrop: transaction dropped exactly at the block where the reorg is detected
	var pastDrop, freshDrop *types.Transaction

	// Create three transactions that will be added in the forked chain:
	//  - pastAdd:   transaction added before the reorganization is detected
	//  - freshAdd:  transaction added at the exact block the reorg is detected
	//  - futureAdd: transaction added after the reorg has already finished
	var pastAdd, freshAdd, futureAdd *types.Transaction

	chain, _ := GenerateChain(gspec.Config, genesis, mockEngine.NewFaker(), db, 3, func(i int, gen *BlockGen) {
		switch i {
		case 0:
			pastDrop, _ = types.SignTx(types.NewTransaction(gen.TxNonce(addr2), addr2, big.NewInt(1000), params.TxGas, nil, nil, nil, nil, nil), signer, key2)
			gen.AddTx(pastDrop)  // This transaction will be dropped in the fork from below the split point
			gen.AddTx(postponed) // This transaction will be postponed till block #3 in the fork

		case 2:
			freshDrop, _ = types.SignTx(types.NewTransaction(gen.TxNonce(addr2), addr2, big.NewInt(1000), params.TxGas, nil, nil, nil, nil, nil), signer, key2)

			gen.AddTx(freshDrop) // This transaction will be dropped in the fork from exactly at the split point
			gen.AddTx(swapped)   // This transaction will be swapped out at the exact height

			gen.OffsetTime(9) // Lower the block difficulty to simulate a weaker chain
		}
	})
	// Import the chain. This runs all block validation rules.
	blockchain, _ := NewBlockChain(db, nil, gspec.Config, mockEngine.NewFaker(), vm.Config{}, nil, nil)
	if i, err := blockchain.InsertChain(chain); err != nil {
		t.Fatalf("failed to insert original chain[%d]: %v", i, err)
	}
	defer blockchain.Stop()

	// overwrite the old chain
	chain, _ = GenerateChain(gspec.Config, genesis, mockEngine.NewFaker(), db, 5, func(i int, gen *BlockGen) {
		switch i {
		case 0:
			pastAdd, _ = types.SignTx(types.NewTransaction(gen.TxNonce(addr3), addr3, big.NewInt(1000), params.TxGas, nil, nil, nil, nil, nil), signer, key3)
			gen.AddTx(pastAdd) // This transaction needs to be injected during reorg

		case 2:
			gen.AddTx(postponed) // This transaction was postponed from block #1 in the original chain
			gen.AddTx(swapped)   // This transaction was swapped from the exact current spot in the original chain

			freshAdd, _ = types.SignTx(types.NewTransaction(gen.TxNonce(addr3), addr3, big.NewInt(1000), params.TxGas, nil, nil, nil, nil, nil), signer, key3)
			gen.AddTx(freshAdd) // This transaction will be added exactly at reorg time

		case 3:
			futureAdd, _ = types.SignTx(types.NewTransaction(gen.TxNonce(addr3), addr3, big.NewInt(1000), params.TxGas, nil, nil, nil, nil, nil), signer, key3)
			gen.AddTx(futureAdd) // This transaction will be added after a full reorg
		}
	})
	if _, err := blockchain.InsertChain(chain); err != nil {
		t.Fatalf("failed to insert forked chain: %v", err)
	}

	// removed tx
	for i, tx := range (types.Transactions{pastDrop, freshDrop}) {
		if txn, _, _, _ := rawdb.ReadTransaction(db, tx.Hash()); txn != nil {
			t.Errorf("drop %d: tx %v found while shouldn't have been", i, txn)
		}
		if rcpt, _, _, _ := rawdb.ReadReceipt(db, tx.Hash(), blockchain.Config()); rcpt != nil {
			t.Errorf("drop %d: receipt %v found while shouldn't have been", i, rcpt)
		}
	}
	// added tx
	for i, tx := range (types.Transactions{pastAdd, freshAdd, futureAdd}) {
		if txn, _, _, _ := rawdb.ReadTransaction(db, tx.Hash()); txn == nil {
			t.Errorf("add %d: expected tx to be found", i)
		}
		if rcpt, _, _, _ := rawdb.ReadReceipt(db, tx.Hash(), blockchain.Config()); rcpt == nil {
			t.Errorf("add %d: expected receipt to be found", i)
		}
	}
	// shared tx
	for i, tx := range (types.Transactions{postponed, swapped}) {
		if txn, _, _, _ := rawdb.ReadTransaction(db, tx.Hash()); txn == nil {
			t.Errorf("share %d: expected tx to be found", i)
		}
		if rcpt, _, _, _ := rawdb.ReadReceipt(db, tx.Hash(), blockchain.Config()); rcpt == nil {
			t.Errorf("share %d: expected receipt to be found", i)
		}
	}
}

func TestLogReorgs(t *testing.T) {
	var (
		key1, _ = crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")
		addr1   = crypto.PubkeyToAddress(key1.PublicKey)
		db      = rawdb.NewMemoryDatabase()
		// this code generates a log
		code    = common.Hex2Bytes("60606040525b7f24ec1d3ff24c2f6ff210738839dbc339cd45a5294d85c79361016243157aae7b60405180905060405180910390a15b600a8060416000396000f360606040526008565b00")
		gspec   = &Genesis{Config: params.IstanbulTestChainConfig, Alloc: GenesisAlloc{addr1: {Balance: big.NewInt(10000000000000)}}}
		genesis = gspec.MustCommit(db)
		signer  = types.LatestSigner(gspec.Config)
	)

	blockchain, _ := NewBlockChain(db, nil, gspec.Config, mockEngine.NewFaker(), vm.Config{}, nil, nil)
	defer blockchain.Stop()

	rmLogsCh := make(chan RemovedLogsEvent, 100)
	blockchain.SubscribeRemovedLogsEvent(rmLogsCh)
	chain, _ := GenerateChain(params.IstanbulTestChainConfig, genesis, mockEngine.NewFaker(), db, 2, func(i int, gen *BlockGen) {
		if i == 1 {
			tx, err := types.SignTx(types.NewContractCreation(gen.TxNonce(addr1), new(big.Int), 1000000, new(big.Int), nil, nil, nil, code), signer, key1)

			if err != nil {
				t.Fatalf("failed to create tx: %v", err)
			}
			gen.AddTx(tx)
		}
	})
	if _, err := blockchain.InsertChain(chain); err != nil {
		t.Fatalf("failed to insert chain: %v", err)
	}

	chain, _ = GenerateChain(params.IstanbulTestChainConfig, genesis, mockEngine.NewFaker(), db, 3, func(i int, gen *BlockGen) {})
	if _, err := blockchain.InsertChain(chain); err != nil {
		t.Fatalf("failed to insert forked chain: %v", err)
	}
	timeout := time.NewTimer(1 * time.Second)
	defer timeout.Stop()
	select {
	case ev := <-rmLogsCh:
		if len(ev.Logs) == 0 {
			t.Error("expected logs")
		}
	case <-timeout.C:
		t.Fatal("Timeout. There is no RemovedLogsEvent has been sent.")
	}
}

// This EVM code generates a log when the contract is created.
var logCode = common.Hex2Bytes("60606040525b7f24ec1d3ff24c2f6ff210738839dbc339cd45a5294d85c79361016243157aae7b60405180905060405180910390a15b600a8060416000396000f360606040526008565b00")

// This test checks that log events and RemovedLogsEvent are sent
// when the chain reorganizes.
func TestLogRebirth(t *testing.T) {
	var (
		key1, _       = crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")
		addr1         = crypto.PubkeyToAddress(key1.PublicKey)
		db            = rawdb.NewMemoryDatabase()
		gspec         = &Genesis{Config: params.IstanbulTestChainConfig, Alloc: GenesisAlloc{addr1: {Balance: big.NewInt(10000000000000)}}}
		genesis       = gspec.MustCommit(db)
		signer        = types.NewEIP155Signer(gspec.Config.ChainID)
		engine        = mockEngine.NewFaker()
		blockchain, _ = NewBlockChain(db, nil, gspec.Config, engine, vm.Config{}, nil, nil)
	)

	defer blockchain.Stop()

	// The event channels.
	newLogCh := make(chan []*types.Log, 10)
	rmLogsCh := make(chan RemovedLogsEvent, 10)
	blockchain.SubscribeLogsEvent(newLogCh)
	blockchain.SubscribeRemovedLogsEvent(rmLogsCh)

	// This chain contains a single log.
	chain, _ := GenerateChain(params.IstanbulTestChainConfig, genesis, engine, db, 2, func(i int, gen *BlockGen) {
		if i == 1 {
			tx, err := types.SignTx(types.NewContractCreation(gen.TxNonce(addr1), new(big.Int), 1000000, new(big.Int), nil, nil, nil, logCode), signer, key1)
			if err != nil {
				t.Fatalf("failed to create tx: %v", err)
			}
			gen.AddTx(tx)
		}
	})
	if _, err := blockchain.InsertChain(chain); err != nil {
		t.Fatalf("failed to insert chain: %v", err)
	}
	checkLogEvents(t, newLogCh, rmLogsCh, 1, 0)

	// Generate long reorg chain containing another log. Inserting the
	// chain removes one log and adds one.
	forkChain, _ := GenerateChain(params.IstanbulTestChainConfig, genesis, engine, db, 3, func(i int, gen *BlockGen) {
		if i == 1 {
			tx, err := types.SignTx(types.NewContractCreation(gen.TxNonce(addr1), new(big.Int), 1000000, new(big.Int), nil, nil, nil, logCode), signer, key1)
			if err != nil {
				t.Fatalf("failed to create tx: %v", err)
			}
			gen.AddTx(tx)
			gen.OffsetTime(-9) // higher block difficulty
		}
	})
	if _, err := blockchain.InsertChain(forkChain); err != nil {
		t.Fatalf("failed to insert forked chain: %v", err)
	}
	checkLogEvents(t, newLogCh, rmLogsCh, 1, 1)

	// This chain segment is rooted in the original chain, but doesn't contain any logs.
	// When inserting it, the canonical chain switches away from forkChain and re-emits
	// the log event for the old chain, as well as a RemovedLogsEvent for forkChain.
	newBlocks, _ := GenerateChain(params.IstanbulTestChainConfig, chain[len(chain)-1], engine, db, 2, func(i int, gen *BlockGen) {})
	if _, err := blockchain.InsertChain(newBlocks); err != nil {
		t.Fatalf("failed to insert forked chain: %v", err)
	}
	checkLogEvents(t, newLogCh, rmLogsCh, 1, 1)
}

// This test is a variation of TestLogRebirth. It verifies that log events are emitted
// when a side chain containing log events overtakes the canonical chain.
func TestSideLogRebirth(t *testing.T) {
	var (
		key1, _       = crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")
		addr1         = crypto.PubkeyToAddress(key1.PublicKey)
		db            = rawdb.NewMemoryDatabase()
		gspec         = &Genesis{Config: params.IstanbulTestChainConfig, Alloc: GenesisAlloc{addr1: {Balance: big.NewInt(10000000000000)}}}
		genesis       = gspec.MustCommit(db)
		signer        = types.NewEIP155Signer(gspec.Config.ChainID)
		blockchain, _ = NewBlockChain(db, nil, gspec.Config, mockEngine.NewFaker(), vm.Config{}, nil, nil)
	)

	defer blockchain.Stop()

	newLogCh := make(chan []*types.Log, 10)
	rmLogsCh := make(chan RemovedLogsEvent, 10)
	blockchain.SubscribeLogsEvent(newLogCh)
	blockchain.SubscribeRemovedLogsEvent(rmLogsCh)

	chain, _ := GenerateChain(params.IstanbulTestChainConfig, genesis, mockEngine.NewFaker(), db, 3, func(i int, gen *BlockGen) {
		if i == 1 {
			gen.OffsetTime(-9) // higher block difficulty

		}
	})
	if _, err := blockchain.InsertChain(chain); err != nil {
		t.Fatalf("failed to insert forked chain: %v", err)
	}
	checkLogEvents(t, newLogCh, rmLogsCh, 0, 0)

	// Generate side chain with lower difficulty
	sideChain, _ := GenerateChain(params.IstanbulTestChainConfig, genesis, mockEngine.NewFaker(), db, 2, func(i int, gen *BlockGen) {
		if i == 1 {
			tx, err := types.SignTx(types.NewContractCreation(gen.TxNonce(addr1), new(big.Int), 1000000, new(big.Int), nil, nil, nil, logCode), signer, key1)

			if err != nil {
				t.Fatalf("failed to create tx: %v", err)
			}
			gen.AddTx(tx)
		}
	})
	if _, err := blockchain.InsertChain(sideChain); err != nil {
		t.Fatalf("failed to insert forked chain: %v", err)
	}
	checkLogEvents(t, newLogCh, rmLogsCh, 0, 0)

	// Generate two new blocks based on side chain, to trigger a reorg
	newBlocks, _ := GenerateChain(params.IstanbulTestChainConfig, sideChain[len(sideChain)-1], mockEngine.NewFaker(), db, 2, func(i int, gen *BlockGen) {})
	if _, err := blockchain.InsertChain(newBlocks); err != nil {
		t.Fatalf("failed to insert forked chain: %v", err)
	}
	checkLogEvents(t, newLogCh, rmLogsCh, 1, 0)
}

func checkLogEvents(t *testing.T, logsCh <-chan []*types.Log, rmLogsCh <-chan RemovedLogsEvent, wantNew, wantRemoved int) {
	t.Helper()

	if len(logsCh) != wantNew {
		t.Fatalf("wrong number of log events: got %d, want %d", len(logsCh), wantNew)
	}
	if len(rmLogsCh) != wantRemoved {
		t.Fatalf("wrong number of removed log events: got %d, want %d", len(rmLogsCh), wantRemoved)
	}
	// Drain events.
	for i := 0; i < len(logsCh); i++ {
		<-logsCh
	}
	for i := 0; i < len(rmLogsCh); i++ {
		<-rmLogsCh
	}
}

func TestReorgSideEvent(t *testing.T) {
	var (
		db      = rawdb.NewMemoryDatabase()
		key1, _ = crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")
		addr1   = crypto.PubkeyToAddress(key1.PublicKey)
		gspec   = &Genesis{
			Config: params.IstanbulTestChainConfig,
			Alloc:  GenesisAlloc{addr1: {Balance: big.NewInt(10000000000000)}},
		}
		genesis = gspec.MustCommit(db)
		signer  = types.LatestSigner(gspec.Config)
	)

	blockchain, _ := NewBlockChain(db, nil, gspec.Config, mockEngine.NewFaker(), vm.Config{}, nil, nil)
	defer blockchain.Stop()

	chain, _ := GenerateChain(gspec.Config, genesis, mockEngine.NewFaker(), db, 3, func(i int, gen *BlockGen) {})
	if _, err := blockchain.InsertChain(chain); err != nil {
		t.Fatalf("failed to insert chain: %v", err)
	}

	replacementBlocks, _ := GenerateChain(gspec.Config, genesis, mockEngine.NewFaker(), db, 4, func(i int, gen *BlockGen) {
		tx, err := types.SignTx(types.NewContractCreation(gen.TxNonce(addr1), new(big.Int), 1000000, new(big.Int), nil, nil, nil, nil), signer, key1)
		if i == 2 {
			gen.OffsetTime(-9)
		}
		if err != nil {
			t.Fatalf("failed to create tx: %v", err)
		}
		gen.AddTx(tx)
	})
	chainSideCh := make(chan ChainSideEvent, 64)
	blockchain.SubscribeChainSideEvent(chainSideCh)
	if _, err := blockchain.InsertChain(replacementBlocks); err != nil {
		t.Fatalf("failed to insert chain: %v", err)
	}

	// first two block of the secondary chain are for a brief moment considered
	// side chains because up to that point the first one is considered the
	// heavier chain.
	// the third may or may not be, depending on whether it triggers a reorg (the
	// difficulties of the two chains are equal at this time).
	// the boolean value indicates whether we are still waiting for that block's event.
	expectedSideHashes := map[common.Hash]bool{
		replacementBlocks[0].Hash(): true,
		replacementBlocks[1].Hash(): true,
		replacementBlocks[2].Hash(): false, // may not be sent (if reorg was on the 3rd block)
		chain[0].Hash():             true,
		chain[1].Hash():             true,
		chain[2].Hash():             true,
	}

	i := 0

	const timeoutDura = 10 * time.Second
	timeout := time.NewTimer(timeoutDura)
done:
	for {
		select {
		case ev := <-chainSideCh:
			block := ev.Block
			if _, ok := expectedSideHashes[block.Hash()]; !ok {
				t.Errorf("%d: didn't expect %x to be in side chain", i, block.Hash())
			}
			expectedSideHashes[block.Hash()] = false
			i++

			numLeft := 0
			for _, isLeft := range expectedSideHashes {
				if isLeft {
					numLeft += 1
				}
			}
			if numLeft == 0 {
				timeout.Stop()

				break done
			}
			timeout.Reset(timeoutDura)

		case <-timeout.C:
			t.Fatal("Timeout. Possibly not all blocks were triggered for sideevent")
		}
	}

	// make sure no more events are fired
	select {
	case e := <-chainSideCh:
		t.Errorf("unexpected event fired: %v", e)
	case <-time.After(250 * time.Millisecond):
	}

}

// Tests if the canonical block can be fetched from the database during chain insertion.
func TestCanonicalBlockRetrieval(t *testing.T) {
	_, blockchain, err := newCanonical(mockEngine.NewFaker(), 0, true)
	if err != nil {
		t.Fatalf("failed to create pristine chain: %v", err)
	}
	defer blockchain.Stop()

	chain, _ := GenerateChain(blockchain.chainConfig, blockchain.genesisBlock, mockEngine.NewFaker(), blockchain.db, 10, func(i int, gen *BlockGen) {})

	var pend sync.WaitGroup
	pend.Add(len(chain))

	for i := range chain {
		go func(block *types.Block) {
			defer pend.Done()

			// try to retrieve a block by its canonical hash and see if the block data can be retrieved.
			for {
				ch := rawdb.ReadCanonicalHash(blockchain.db, block.NumberU64())
				if ch == (common.Hash{}) {
					continue // busy wait for canonical hash to be written
				}
				if ch != block.Hash() {
					t.Errorf("unknown canonical hash, want %s, got %s", block.Hash().Hex(), ch.Hex())
					return
				}
				fb := rawdb.ReadBlock(blockchain.db, ch, block.NumberU64())
				if fb == nil {
					t.Errorf("unable to retrieve block %d for canonical hash: %s", block.NumberU64(), ch.Hex())
					return
				}
				if fb.Hash() != block.Hash() {
					t.Errorf("invalid block hash for block %d, want %s, got %s", block.NumberU64(), block.Hash().Hex(), fb.Hash().Hex())
					return
				}
				return
			}
		}(chain[i])

		if _, err := blockchain.InsertChain(types.Blocks{chain[i]}); err != nil {
			t.Fatalf("failed to insert block %d: %v", i, err)
		}
	}
	pend.Wait()
}

func TestEIP155Transition(t *testing.T) {
	// Configure and generate a sample block chain
	var (
		db         = rawdb.NewMemoryDatabase()
		key, _     = crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")
		address    = crypto.PubkeyToAddress(key.PublicKey)
		funds      = big.NewInt(1000000000)
		deleteAddr = common.Address{1}
		gspec      = &Genesis{
			Config: &params.ChainConfig{ChainID: big.NewInt(1), EIP150Block: big.NewInt(0), EIP155Block: big.NewInt(2), HomesteadBlock: new(big.Int)},
			Alloc:  GenesisAlloc{address: {Balance: funds}, deleteAddr: {Balance: new(big.Int)}},
		}
		genesis = gspec.MustCommit(db)
	)

	blockchain, _ := NewBlockChain(db, nil, gspec.Config, mockEngine.NewFaker(), vm.Config{}, nil, nil)
	defer blockchain.Stop()

	blocks, _ := GenerateChain(gspec.Config, genesis, mockEngine.NewFaker(), db, 4, func(i int, block *BlockGen) {
		var (
			tx      *types.Transaction
			err     error
			basicTx = func(signer types.Signer) (*types.Transaction, error) {
				return types.SignTx(types.NewTransaction(block.TxNonce(address), common.Address{}, new(big.Int), 21000, new(big.Int), nil, nil, nil, nil), signer, key)
			}
		)
		switch i {
		case 0:
			tx, err = basicTx(types.HomesteadSigner{})
			if err != nil {
				t.Fatal(err)
			}
			block.AddTx(tx)
		case 2:
			tx, err = basicTx(types.HomesteadSigner{})
			if err != nil {
				t.Fatal(err)
			}
			block.AddTx(tx)

			tx, err = basicTx(types.LatestSigner(gspec.Config))
			if err != nil {
				t.Fatal(err)
			}
			block.AddTx(tx)
		case 3:
			tx, err = basicTx(types.HomesteadSigner{})
			if err != nil {
				t.Fatal(err)
			}
			block.AddTx(tx)

			tx, err = basicTx(types.LatestSigner(gspec.Config))
			if err != nil {
				t.Fatal(err)
			}
			block.AddTx(tx)
		}
	})

	if _, err := blockchain.InsertChain(blocks); err != nil {
		t.Fatal(err)
	}
	block := blockchain.GetBlockByNumber(1)
	if block.Transactions()[0].Protected() {
		t.Error("Expected block[0].txs[0] to not be replay protected")
	}

	block = blockchain.GetBlockByNumber(3)
	if block.Transactions()[0].Protected() {
		t.Error("Expected block[3].txs[0] to not be replay protected")
	}
	if !block.Transactions()[1].Protected() {
		t.Error("Expected block[3].txs[1] to be replay protected")
	}
	if _, err := blockchain.InsertChain(blocks[4:]); err != nil {
		t.Fatal(err)
	}

	// generate an invalid chain id transaction
	config := &params.ChainConfig{ChainID: big.NewInt(2), EIP150Block: big.NewInt(0), EIP155Block: big.NewInt(2), HomesteadBlock: new(big.Int)}
	blocks, _ = GenerateChain(config, blocks[len(blocks)-1], mockEngine.NewFaker(), db, 4, func(i int, block *BlockGen) {
		var (
			tx      *types.Transaction
			err     error
			basicTx = func(signer types.Signer) (*types.Transaction, error) {
				return types.SignTx(types.NewTransaction(block.TxNonce(address), common.Address{}, new(big.Int), 21000, new(big.Int), nil, nil, nil, nil), signer, key)
			}
		)
		if i == 0 {
			tx, err = basicTx(types.LatestSigner(config))
			if err != nil {
				t.Fatal(err)
			}
			block.AddTx(tx)
		}
	})
	_, err := blockchain.InsertChain(blocks)
	if have, want := err, types.ErrInvalidChainId; !errors.Is(have, want) {
		t.Errorf("have %v, want %v", have, want)
	}
}

func TestEIP161AccountRemoval(t *testing.T) {
	// Configure and generate a sample block chain
	var (
		db      = rawdb.NewMemoryDatabase()
		key, _  = crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")
		address = crypto.PubkeyToAddress(key.PublicKey)
		funds   = big.NewInt(1000000000)
		theAddr = common.Address{1}
		gspec   = &Genesis{
			Config: &params.ChainConfig{
				ChainID:        big.NewInt(1),
				HomesteadBlock: new(big.Int),
				EIP155Block:    new(big.Int),
				EIP150Block:    new(big.Int),
				EIP158Block:    big.NewInt(2),
			},
			Alloc: GenesisAlloc{address: {Balance: funds}},
		}
		genesis = gspec.MustCommit(db)
	)
	blockchain, _ := NewBlockChain(db, nil, gspec.Config, mockEngine.NewFaker(), vm.Config{}, nil, nil)
	defer blockchain.Stop()

	blocks, _ := GenerateChain(gspec.Config, genesis, mockEngine.NewFaker(), db, 3, func(i int, block *BlockGen) {
		var (
			tx     *types.Transaction
			err    error
			signer = types.LatestSigner(gspec.Config)
		)
		switch i {
		case 0:
			tx, err = types.SignTx(types.NewTransaction(block.TxNonce(address), theAddr, new(big.Int), 21000, new(big.Int), nil, nil, nil, nil), signer, key)
		case 1:
			tx, err = types.SignTx(types.NewTransaction(block.TxNonce(address), theAddr, new(big.Int), 21000, new(big.Int), nil, nil, nil, nil), signer, key)
		case 2:
			tx, err = types.SignTx(types.NewTransaction(block.TxNonce(address), theAddr, new(big.Int), 21000, new(big.Int), nil, nil, nil, nil), signer, key)
		}
		if err != nil {
			t.Fatal(err)
		}
		block.AddTx(tx)
	})
	// account must exist pre eip 161
	if _, err := blockchain.InsertChain(types.Blocks{blocks[0]}); err != nil {
		t.Fatal(err)
	}
	if st, _ := blockchain.State(); !st.Exist(theAddr) {
		t.Error("expected account to exist")
	}

	// account needs to be deleted post eip 161
	if _, err := blockchain.InsertChain(types.Blocks{blocks[1]}); err != nil {
		t.Fatal(err)
	}
	if st, _ := blockchain.State(); st.Exist(theAddr) {
		t.Error("account should not exist")
	}

	// account mustn't be created post eip 161
	if _, err := blockchain.InsertChain(types.Blocks{blocks[2]}); err != nil {
		t.Fatal(err)
	}
	if st, _ := blockchain.State(); st.Exist(theAddr) {
		t.Error("account should not exist")
	}
}

// This is a regression test (i.e. as weird as it is, don't delete it ever), which
// tests that under weird reorg conditions the blockchain and its internal header-
// chain return the same latest block/header.
//
// https://github.com/ethereum/go-ethereum/pull/15941
func TestBlockchainHeaderchainReorgConsistency(t *testing.T) {
	// Generate a canonical chain to act as the main dataset
	engine := mockEngine.NewFaker()

	db := rawdb.NewMemoryDatabase()
	genesis := new(Genesis).MustCommit(db)
	blocks, _ := GenerateChain(params.IstanbulTestChainConfig, genesis, engine, db, 64, func(i int, b *BlockGen) { b.SetCoinbase(common.Address{1}) })

	// Generate a bunch of fork blocks, each side forking from the canonical chain
	forks := make([]*types.Block, len(blocks))
	for i := 0; i < len(forks); i++ {
		parent := genesis
		if i > 0 {
			parent = blocks[i-1]
		}
		fork, _ := GenerateChain(params.IstanbulTestChainConfig, parent, engine, db, 1, func(i int, b *BlockGen) { b.SetCoinbase(common.Address{2}) })
		forks[i] = fork[0]
	}
	// Import the canonical and fork chain side by side, verifying the current block
	// and current header consistency
	diskdb := rawdb.NewMemoryDatabase()
	new(Genesis).MustCommit(diskdb)

	chain, err := NewBlockChain(diskdb, nil, params.IstanbulTestChainConfig, engine, vm.Config{}, nil, nil)
	if err != nil {
		t.Fatalf("failed to create tester chain: %v", err)
	}
	for i := 0; i < len(blocks); i++ {
		if _, err := chain.InsertChain(blocks[i : i+1]); err != nil {
			t.Fatalf("block %d: failed to insert into chain: %v", i, err)
		}
		if chain.CurrentBlock().Hash() != chain.CurrentHeader().Hash() {
			t.Errorf("block %d: current block/header mismatch: block #%d [%x..], header #%d [%x..]", i, chain.CurrentBlock().Number(), chain.CurrentBlock().Hash().Bytes()[:4], chain.CurrentHeader().Number, chain.CurrentHeader().Hash().Bytes()[:4])
		}
		if _, err := chain.InsertChain(forks[i : i+1]); err != nil {
			t.Fatalf(" fork %d: failed to insert into chain: %v", i, err)
		}
		if chain.CurrentBlock().Hash() != chain.CurrentHeader().Hash() {
			t.Errorf(" fork %d: current block/header mismatch: block #%d [%x..], header #%d [%x..]", i, chain.CurrentBlock().Number(), chain.CurrentBlock().Hash().Bytes()[:4], chain.CurrentHeader().Number, chain.CurrentHeader().Hash().Bytes()[:4])
		}
	}
}

// Tests that importing small side forks doesn't leave junk in the trie database
// cache (which would eventually cause memory issues).
func TestTrieForkGC(t *testing.T) {
	// Generate a canonical chain to act as the main dataset
	engine := mockEngine.NewFaker()

	db := rawdb.NewMemoryDatabase()
	genesis := new(Genesis).MustCommit(db)
	blocks, _ := GenerateChain(params.IstanbulTestChainConfig, genesis, engine, db, 2*TriesInMemory, func(i int, b *BlockGen) { b.SetCoinbase(common.Address{1}) })

	// Generate a bunch of fork blocks, each side forking from the canonical chain
	forks := make([]*types.Block, len(blocks))
	for i := 0; i < len(forks); i++ {
		parent := genesis
		if i > 0 {
			parent = blocks[i-1]
		}
		fork, _ := GenerateChain(params.IstanbulTestChainConfig, parent, engine, db, 1, func(i int, b *BlockGen) { b.SetCoinbase(common.Address{2}) })
		forks[i] = fork[0]
	}
	// Import the canonical and fork chain side by side, forcing the trie cache to cache both
	diskdb := rawdb.NewMemoryDatabase()
	new(Genesis).MustCommit(diskdb)

	chain, err := NewBlockChain(diskdb, nil, params.IstanbulTestChainConfig, engine, vm.Config{}, nil, nil)
	if err != nil {
		t.Fatalf("failed to create tester chain: %v", err)
	}
	for i := 0; i < len(blocks); i++ {
		if _, err := chain.InsertChain(blocks[i : i+1]); err != nil {
			t.Fatalf("block %d: failed to insert into chain: %v", i, err)
		}
		if _, err := chain.InsertChain(forks[i : i+1]); err != nil {
			t.Fatalf("fork %d: failed to insert into chain: %v", i, err)
		}
	}
	// Dereference all the recent tries and ensure no past trie is left in
	for i := 0; i < TriesInMemory; i++ {
		chain.stateCache.TrieDB().Dereference(blocks[len(blocks)-1-i].Root())
		chain.stateCache.TrieDB().Dereference(forks[len(blocks)-1-i].Root())
	}
	if len(chain.stateCache.TrieDB().Nodes()) > 0 {
		t.Fatalf("stale tries still alive after garbase collection")
	}
}

// Tests that doing large reorgs works even if the state associated with the
// forking point is not available any more.
func TestLargeReorgTrieGC(t *testing.T) {
	// Generate the original common chain segment and the two competing forks
	engine := mockEngine.NewFaker()

	db := rawdb.NewMemoryDatabase()
	genesis := new(Genesis).MustCommit(db)

	shared, _ := GenerateChain(params.IstanbulTestChainConfig, genesis, engine, db, 64, func(i int, b *BlockGen) { b.SetCoinbase(common.Address{1}) })
	original, _ := GenerateChain(params.IstanbulTestChainConfig, shared[len(shared)-1], engine, db, 2*TriesInMemory, func(i int, b *BlockGen) { b.SetCoinbase(common.Address{2}) })
	competitor, _ := GenerateChain(params.IstanbulTestChainConfig, shared[len(shared)-1], engine, db, 2*TriesInMemory+1, func(i int, b *BlockGen) { b.SetCoinbase(common.Address{3}) })

	// Import the shared chain and the original canonical one
	diskdb := rawdb.NewMemoryDatabase()
	new(Genesis).MustCommit(diskdb)

	chain, err := NewBlockChain(diskdb, nil, params.IstanbulTestChainConfig, engine, vm.Config{}, nil, nil)
	if err != nil {
		t.Fatalf("failed to create tester chain: %v", err)
	}
	if _, err := chain.InsertChain(shared); err != nil {
		t.Fatalf("failed to insert shared chain: %v", err)
	}
	if _, err := chain.InsertChain(original); err != nil {
		t.Fatalf("failed to insert original chain: %v", err)
	}
	// Ensure that the state associated with the forking point is pruned away
	if node, _ := chain.stateCache.TrieDB().Node(shared[len(shared)-1].Root()); node != nil {
		t.Fatalf("common-but-old ancestor still cache")
	}
	// Import the competitor chain without exceeding the canonical's TD and ensure
	// we have not processed any of the blocks (protection against malicious blocks)
	if _, err := chain.InsertChain(competitor[:len(competitor)-2]); err != nil {
		t.Fatalf("failed to insert competitor chain: %v", err)
	}
	for i, block := range competitor[:len(competitor)-2] {
		if node, _ := chain.stateCache.TrieDB().Node(block.Root()); node != nil {
			t.Fatalf("competitor %d: low TD chain became processed", i)
		}
	}
	// Import the head of the competitor chain, triggering the reorg and ensure we
	// successfully reprocess all the stashed away blocks.
	if _, err := chain.InsertChain(competitor[len(competitor)-2:]); err != nil {
		t.Fatalf("failed to finalize competitor chain: %v", err)
	}
	for i, block := range competitor[:len(competitor)-TriesInMemory] {
		if node, _ := chain.stateCache.TrieDB().Node(block.Root()); node != nil {
			t.Fatalf("competitor %d: competing chain state missing", i)
		}
	}
}

func TestBlockchainRecovery(t *testing.T) {
	// Configure and generate a sample block chain
	var (
		gendb   = rawdb.NewMemoryDatabase()
		key, _  = crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")
		address = crypto.PubkeyToAddress(key.PublicKey)
		funds   = big.NewInt(1000000000)
		gspec   = &Genesis{Config: params.IstanbulTestChainConfig, Alloc: GenesisAlloc{address: {Balance: funds}}}
		genesis = gspec.MustCommit(gendb)
	)
	height := uint64(1024)
	blocks, receipts := GenerateChain(gspec.Config, genesis, mockEngine.NewFaker(), gendb, int(height), nil)

	// Import the chain as a ancient-first node and ensure all pointers are updated
	frdir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatalf("failed to create temp freezer dir: %v", err)
	}
	defer os.Remove(frdir)

	ancientDb, err := rawdb.NewDatabaseWithFreezer(rawdb.NewMemoryDatabase(), frdir, "", false)
	if err != nil {
		t.Fatalf("failed to create temp freezer db: %v", err)
	}
	gspec.MustCommit(ancientDb)
	ancient, _ := NewBlockChain(ancientDb, nil, gspec.Config, mockEngine.NewFaker(), vm.Config{}, nil, nil)

	headers := make([]*types.Header, len(blocks))
	for i, block := range blocks {
		headers[i] = block.Header()
	}
	if n, err := ancient.InsertHeaderChain(headers, 1, true); err != nil {
		t.Fatalf("failed to insert header %d: %v", n, err)
	}
	if n, err := ancient.InsertReceiptChain(blocks, receipts, uint64(3*len(blocks)/4)); err != nil {
		t.Fatalf("failed to insert receipt %d: %v", n, err)
	}
	rawdb.WriteLastPivotNumber(ancientDb, blocks[len(blocks)-1].NumberU64()) // Force fast sync behavior
	ancient.Stop()

	// Destroy head fast block manually
	midBlock := blocks[len(blocks)/2]
	rawdb.WriteHeadFastBlockHash(ancientDb, midBlock.Hash())

	// Reopen broken blockchain again
	ancient, _ = NewBlockChain(ancientDb, nil, gspec.Config, mockEngine.NewFaker(), vm.Config{}, nil, nil)
	defer ancient.Stop()
	if num := ancient.CurrentBlock().NumberU64(); num != 0 {
		t.Errorf("head block mismatch: have #%v, want #%v", num, 0)
	}
	if num := ancient.CurrentFastBlock().NumberU64(); num != midBlock.NumberU64() {
		t.Errorf("head fast-block mismatch: have #%v, want #%v", num, midBlock.NumberU64())
	}
	if num := ancient.CurrentHeader().Number.Uint64(); num != midBlock.NumberU64() {
		t.Errorf("head header mismatch: have #%v, want #%v", num, midBlock.NumberU64())
	}
}

func TestIncompleteAncientReceiptChainInsertion(t *testing.T) {
	// Configure and generate a sample block chain
	var (
		gendb   = rawdb.NewMemoryDatabase()
		key, _  = crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")
		address = crypto.PubkeyToAddress(key.PublicKey)
		funds   = big.NewInt(1000000000)
		gspec   = &Genesis{Config: params.IstanbulTestChainConfig, Alloc: GenesisAlloc{address: {Balance: funds}}}
		genesis = gspec.MustCommit(gendb)
	)
	height := uint64(1024)
	blocks, receipts := GenerateChain(gspec.Config, genesis, mockEngine.NewFaker(), gendb, int(height), nil)

	// Import the chain as a ancient-first node and ensure all pointers are updated
	frdir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatalf("failed to create temp freezer dir: %v", err)
	}
	defer os.Remove(frdir)
	ancientDb, err := rawdb.NewDatabaseWithFreezer(rawdb.NewMemoryDatabase(), frdir, "", false)
	if err != nil {
		t.Fatalf("failed to create temp freezer db: %v", err)
	}
	gspec.MustCommit(ancientDb)
	ancient, _ := NewBlockChain(ancientDb, nil, gspec.Config, mockEngine.NewFaker(), vm.Config{}, nil, nil)
	defer ancient.Stop()

	headers := make([]*types.Header, len(blocks))
	for i, block := range blocks {
		headers[i] = block.Header()
	}
	if n, err := ancient.InsertHeaderChain(headers, 1, true); err != nil {
		t.Fatalf("failed to insert header %d: %v", n, err)
	}
	// Abort ancient receipt chain insertion deliberately
	ancient.terminateInsert = func(hash common.Hash, number uint64) bool {
		return number == blocks[len(blocks)/2].NumberU64()
	}
	previousFastBlock := ancient.CurrentFastBlock()
	if n, err := ancient.InsertReceiptChain(blocks, receipts, uint64(3*len(blocks)/4)); err == nil {
		t.Fatalf("failed to insert receipt %d: %v", n, err)
	}
	if ancient.CurrentFastBlock().NumberU64() != previousFastBlock.NumberU64() {
		t.Fatalf("failed to rollback ancient data, want %d, have %d", previousFastBlock.NumberU64(), ancient.CurrentFastBlock().NumberU64())
	}
	if frozen, err := ancient.db.Ancients(); err != nil || frozen != 1 {
		t.Fatalf("failed to truncate ancient data")
	}
	ancient.terminateInsert = nil
	if n, err := ancient.InsertReceiptChain(blocks, receipts, uint64(3*len(blocks)/4)); err != nil {
		t.Fatalf("failed to insert receipt %d: %v", n, err)
	}
	if ancient.CurrentFastBlock().NumberU64() != blocks[len(blocks)-1].NumberU64() {
		t.Fatalf("failed to insert ancient recept chain after rollback")
	}
}

// Tests that importing a very large side fork, which is larger than the canon chain,
// but where the difficulty per block is kept low: this means that it will not
// overtake the 'canon' chain until after it's passed canon by about 200 blocks.
//
// Details at:
//  - https://github.com/celo-org/celo-blockchain/issues/18977
//  - https://github.com/celo-org/celo-blockchain/pull/18988
func TestLowDiffLongChain(t *testing.T) {
	// Generate a canonical chain to act as the main dataset
	engine := mockEngine.NewFaker()
	db := rawdb.NewMemoryDatabase()
	genesis := new(Genesis).MustCommit(db)

	// We must use a pretty long chain to ensure that the fork doesn't overtake us
	// until after at least 128 blocks post tip
	blocks, _ := GenerateChain(params.IstanbulTestChainConfig, genesis, engine, db, 6*TriesInMemory, func(i int, b *BlockGen) {
		b.SetCoinbase(common.Address{1})
		b.OffsetTime(-9)
	})

	// Import the canonical chain
	diskdb := rawdb.NewMemoryDatabase()
	new(Genesis).MustCommit(diskdb)

	chain, err := NewBlockChain(diskdb, nil, params.IstanbulTestChainConfig, engine, vm.Config{}, nil, nil)
	if err != nil {
		t.Fatalf("failed to create tester chain: %v", err)
	}
	if n, err := chain.InsertChain(blocks); err != nil {
		t.Fatalf("block %d: failed to insert into chain: %v", n, err)
	}
	// Generate fork chain, starting from an early block
	parent := blocks[10]
	fork, _ := GenerateChain(params.IstanbulTestChainConfig, parent, engine, db, 8*TriesInMemory, func(i int, b *BlockGen) {
		b.SetCoinbase(common.Address{2})
	})

	// And now import the fork
	if i, err := chain.InsertChain(fork); err != nil {
		t.Fatalf("block %d: failed to insert into chain: %v", i, err)
	}
	head := chain.CurrentBlock()
	if got := fork[len(fork)-1].Hash(); got != head.Hash() {
		t.Fatalf("head wrong, expected %x got %x", head.Hash(), got)
	}
	// Sanity check that all the canonical numbers are present
	header := chain.CurrentHeader()
	for number := head.NumberU64(); number > 0; number-- {
		if hash := chain.GetHeaderByNumber(number).Hash(); hash != header.Hash() {
			t.Fatalf("header %d: canonical hash mismatch: have %x, want %x", number, hash, header.Hash())
		}
		header = chain.GetHeader(header.ParentHash, number-1)
	}
}

// Tests that importing a sidechain (S), where
// - S is sidechain, containing blocks [Sn...Sm]
// - C is canon chain, containing blocks [G..Cn..Cm]
// - A common ancestor is placed at prune-point + blocksBetweenCommonAncestorAndPruneblock
// - The sidechain S is prepended with numCanonBlocksInSidechain blocks from the canon chain
func testSideImport(t *testing.T, numCanonBlocksInSidechain, blocksBetweenCommonAncestorAndPruneblock int) {

	// Generate a canonical chain to act as the main dataset
	engine := mockEngine.NewFaker()
	db := rawdb.NewMemoryDatabase()
	genesis := new(Genesis).MustCommit(db)

	// Generate and import the canonical chain
	blocks, _ := GenerateChain(params.TestChainConfig, genesis, engine, db, 2*TriesInMemory, nil)
	diskdb := rawdb.NewMemoryDatabase()
	new(Genesis).MustCommit(diskdb)
	chain, err := NewBlockChain(diskdb, nil, params.TestChainConfig, engine, vm.Config{}, nil, nil)
	if err != nil {
		t.Fatalf("failed to create tester chain: %v", err)
	}
	if n, err := chain.InsertChain(blocks); err != nil {
		t.Fatalf("block %d: failed to insert into chain: %v", n, err)
	}

	lastPrunedIndex := len(blocks) - TriesInMemory - 1
	lastPrunedBlock := blocks[lastPrunedIndex]
	firstNonPrunedBlock := blocks[len(blocks)-TriesInMemory]

	// Verify pruning of lastPrunedBlock
	if chain.HasBlockAndState(lastPrunedBlock.Hash(), lastPrunedBlock.NumberU64()) {
		t.Errorf("Block %d not pruned", lastPrunedBlock.NumberU64())
	}
	// Verify firstNonPrunedBlock is not pruned
	if !chain.HasBlockAndState(firstNonPrunedBlock.Hash(), firstNonPrunedBlock.NumberU64()) {
		t.Errorf("Block %d pruned", firstNonPrunedBlock.NumberU64())
	}
	// Generate the sidechain
	// First block should be a known block, block after should be a pruned block. So
	// canon(pruned), side, side...

	// Generate fork chain, make it longer than canon
	parentIndex := lastPrunedIndex + blocksBetweenCommonAncestorAndPruneblock
	parent := blocks[parentIndex]
	fork, _ := GenerateChain(params.TestChainConfig, parent, engine, db, 2*TriesInMemory, func(i int, b *BlockGen) {
		b.SetCoinbase(common.Address{2})
	})
	// Prepend the parent(s)
	var sidechain []*types.Block
	for i := numCanonBlocksInSidechain; i > 0; i-- {
		sidechain = append(sidechain, blocks[parentIndex+1-i])
	}
	sidechain = append(sidechain, fork...)
	_, err = chain.InsertChain(sidechain)
	if err != nil {
		t.Errorf("Got error, %v", err)
	}
	head := chain.CurrentBlock()
	if got := fork[len(fork)-1].Hash(); got != head.Hash() {
		t.Fatalf("head wrong, expected %x got %x", head.Hash(), got)
	}
}

// Tests that importing a sidechain (S), where
// - S is sidechain, containing blocks [Sn...Sm]
// - C is canon chain, containing blocks [G..Cn..Cm]
// - The common ancestor Cc is pruned
// - The first block in S: Sn, is == Cn
// That is: the sidechain for import contains some blocks already present in canon chain.
// So the blocks are
// [ Cn, Cn+1, Cc, Sn+3 ... Sm]
//   ^    ^    ^  pruned
func TestPrunedImportSide(t *testing.T) {
	//glogger := log.NewGlogHandler(log.StreamHandler(os.Stdout, log.TerminalFormat(false)))
	//glogger.Verbosity(3)
	//log.Root().SetHandler(log.Handler(glogger))
	testSideImport(t, 3, 3)
	testSideImport(t, 3, -3)
	testSideImport(t, 10, 0)
	testSideImport(t, 1, 10)
	testSideImport(t, 1, -10)
}

func TestInsertKnownHeaders(t *testing.T)      { testInsertKnownChainData(t, "headers") }
func TestInsertKnownReceiptChain(t *testing.T) { testInsertKnownChainData(t, "receipts") }
func TestInsertKnownBlocks(t *testing.T)       { testInsertKnownChainData(t, "blocks") }

func testInsertKnownChainData(t *testing.T, typ string) {
	engine := mockEngine.NewFaker()

	db := rawdb.NewMemoryDatabase()
	genesis := new(Genesis).MustCommit(db)

	blocks, receipts := GenerateChain(params.IstanbulTestChainConfig, genesis, engine, db, 32, func(i int, b *BlockGen) { b.SetCoinbase(common.Address{1}) })
	blocks2, receipts2 := GenerateChain(params.IstanbulTestChainConfig, blocks[len(blocks)-1], engine, db, 65, func(i int, b *BlockGen) { b.SetCoinbase(common.Address{1}) })
	// Total difficulty is higher.
	blocks3, receipts3 := GenerateChain(params.IstanbulTestChainConfig, blocks[len(blocks)-1], engine, db, 66, func(i int, b *BlockGen) {
		b.SetCoinbase(common.Address{1})
		b.OffsetTime(-9)
	})
	// Import the shared chain and the original canonical one
	dir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatalf("failed to create temp freezer dir: %v", err)
	}
	defer os.Remove(dir)
	chaindb, err := rawdb.NewDatabaseWithFreezer(rawdb.NewMemoryDatabase(), dir, "", false)
	if err != nil {
		t.Fatalf("failed to create temp freezer db: %v", err)
	}
	new(Genesis).MustCommit(chaindb)
	defer os.RemoveAll(dir)

	chain, err := NewBlockChain(chaindb, nil, params.IstanbulTestChainConfig, engine, vm.Config{}, nil, nil)
	if err != nil {
		t.Fatalf("failed to create tester chain: %v", err)
	}

	var (
		inserter func(blocks []*types.Block, receipts []types.Receipts) error
		asserter func(t *testing.T, block *types.Block)
	)
	if typ == "headers" {
		inserter = func(blocks []*types.Block, receipts []types.Receipts) error {
			headers := make([]*types.Header, 0, len(blocks))
			for _, block := range blocks {
				headers = append(headers, block.Header())
			}
			_, err := chain.InsertHeaderChain(headers, 1, true)
			return err
		}
		asserter = func(t *testing.T, block *types.Block) {
			if chain.CurrentHeader().Hash() != block.Hash() {
				t.Fatalf("current head header mismatch, have %v, want %v", chain.CurrentHeader().Hash().Hex(), block.Hash().Hex())
			}
		}
	} else if typ == "receipts" {
		inserter = func(blocks []*types.Block, receipts []types.Receipts) error {
			headers := make([]*types.Header, 0, len(blocks))
			for _, block := range blocks {
				headers = append(headers, block.Header())
			}
			_, err := chain.InsertHeaderChain(headers, 1, true)
			if err != nil {
				return err
			}
			_, err = chain.InsertReceiptChain(blocks, receipts, 0)
			return err
		}
		asserter = func(t *testing.T, block *types.Block) {
			if chain.CurrentFastBlock().Hash() != block.Hash() {
				t.Fatalf("current head fast block mismatch, have %v, want %v", chain.CurrentFastBlock().Hash().Hex(), block.Hash().Hex())
			}
		}
	} else {
		inserter = func(blocks []*types.Block, receipts []types.Receipts) error {
			_, err := chain.InsertChain(blocks)
			return err
		}
		asserter = func(t *testing.T, block *types.Block) {
			if chain.CurrentBlock().Hash() != block.Hash() {
				t.Fatalf("current head block mismatch, have %v, want %v", chain.CurrentBlock().Hash().Hex(), block.Hash().Hex())
			}
		}
	}

	if err := inserter(blocks, receipts); err != nil {
		t.Fatalf("failed to insert chain data: %v", err)
	}

	// Reimport the chain data again. All the imported
	// chain data are regarded "known" data.
	if err := inserter(blocks, receipts); err != nil {
		t.Fatalf("failed to insert chain data: %v", err)
	}
	asserter(t, blocks[len(blocks)-1])

	// Import a long canonical chain with some known data as prefix.
	rollback := blocks[len(blocks)/2].NumberU64()

	chain.SetHead(rollback - 1)
	if err := inserter(append(blocks, blocks2...), append(receipts, receipts2...)); err != nil {
		t.Fatalf("failed to insert chain data: %v", err)
	}
	asserter(t, blocks2[len(blocks2)-1])

	// Import a heavier shorter but higher total difficulty chain with some known data as prefix.
	if err := inserter(append(blocks, blocks3...), append(receipts, receipts3...)); err != nil {
		t.Fatalf("failed to insert chain data: %v", err)
	}
	asserter(t, blocks3[len(blocks3)-1])

	// Import a longer but lower total difficulty chain with some known data as prefix.
	if err := inserter(append(blocks, blocks2...), append(receipts, receipts2...)); err != nil {
		t.Fatalf("failed to insert chain data: %v", err)
	}
	// The head shouldn't change.
	asserter(t, blocks3[len(blocks3)-1])

	// Rollback the heavier chain and re-insert the longer chain again
	chain.SetHead(rollback - 1)
	if err := inserter(append(blocks, blocks2...), append(receipts, receipts2...)); err != nil {
		t.Fatalf("failed to insert chain data: %v", err)
	}
	asserter(t, blocks2[len(blocks2)-1])
}

func TestTransactionIndices(t *testing.T) {
	// Configure and generate a sample block chain
	var (
		gendb   = rawdb.NewMemoryDatabase()
		key, _  = crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")
		address = crypto.PubkeyToAddress(key.PublicKey)
		funds   = big.NewInt(100000000000000000)
		gspec   = &Genesis{
			Config: params.TestChainConfig,
			Alloc:  GenesisAlloc{address: {Balance: funds}},
		}
		genesis = gspec.MustCommit(gendb)
		signer  = types.LatestSigner(gspec.Config)
	)
	height := uint64(128)
	blocks, receipts := GenerateChain(gspec.Config, genesis, mockEngine.NewFaker(), gendb, int(height), func(i int, block *BlockGen) {
		tx, err := types.SignTx(types.NewTransaction(block.TxNonce(address), common.Address{0x00}, big.NewInt(1000), params.TxGas, nil, nil, nil, nil, nil), signer, key)

		if err != nil {
			panic(err)
		}
		block.AddTx(tx)
	})
	blocks2, _ := GenerateChain(gspec.Config, blocks[len(blocks)-1], mockEngine.NewFaker(), gendb, 10, nil)

	check := func(tail *uint64, chain *BlockChain) {
		stored := rawdb.ReadTxIndexTail(chain.db)
		if tail == nil && stored != nil {
			t.Fatalf("Oldest indexded block mismatch, want nil, have %d", *stored)
		}
		if tail != nil && *stored != *tail {
			t.Fatalf("Oldest indexded block mismatch, want %d, have %d", *tail, *stored)
		}
		if tail != nil {
			for i := *tail; i <= chain.CurrentBlock().NumberU64(); i++ {
				block := rawdb.ReadBlock(chain.db, rawdb.ReadCanonicalHash(chain.db, i), i)
				if block.Transactions().Len() == 0 {
					continue
				}
				for _, tx := range block.Transactions() {
					if index := rawdb.ReadTxLookupEntry(chain.db, tx.Hash()); index == nil {
						t.Fatalf("Miss transaction indice, number %d hash %s", i, tx.Hash().Hex())
					}
				}
			}
			for i := uint64(0); i < *tail; i++ {
				block := rawdb.ReadBlock(chain.db, rawdb.ReadCanonicalHash(chain.db, i), i)
				if block.Transactions().Len() == 0 {
					continue
				}
				for _, tx := range block.Transactions() {
					if index := rawdb.ReadTxLookupEntry(chain.db, tx.Hash()); index != nil {
						t.Fatalf("Transaction indice should be deleted, number %d hash %s", i, tx.Hash().Hex())
					}
				}
			}
		}
	}
	frdir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatalf("failed to create temp freezer dir: %v", err)
	}
	defer os.Remove(frdir)
	ancientDb, err := rawdb.NewDatabaseWithFreezer(rawdb.NewMemoryDatabase(), frdir, "", false)
	if err != nil {
		t.Fatalf("failed to create temp freezer db: %v", err)
	}
	gspec.MustCommit(ancientDb)

	// Import all blocks into ancient db
	l := uint64(0)
	chain, err := NewBlockChain(ancientDb, nil, params.TestChainConfig, mockEngine.NewFaker(), vm.Config{}, nil, &l)
	if err != nil {
		t.Fatalf("failed to create tester chain: %v", err)
	}
	headers := make([]*types.Header, len(blocks))
	for i, block := range blocks {
		headers[i] = block.Header()
	}
	if n, err := chain.InsertHeaderChain(headers, 0, true); err != nil {
		t.Fatalf("failed to insert header %d: %v", n, err)
	}
	if n, err := chain.InsertReceiptChain(blocks, receipts, 128); err != nil {
		t.Fatalf("block %d: failed to insert into chain: %v", n, err)
	}
	chain.Stop()
	ancientDb.Close()

	// Init block chain with external ancients, check all needed indices has been indexed.
	limit := []uint64{0, 32, 64, 128}
	for _, l := range limit {
		ancientDb, err = rawdb.NewDatabaseWithFreezer(rawdb.NewMemoryDatabase(), frdir, "", false)
		if err != nil {
			t.Fatalf("failed to create temp freezer db: %v", err)
		}
		gspec.MustCommit(ancientDb)
		chain, err = NewBlockChain(ancientDb, nil, params.TestChainConfig, mockEngine.NewFaker(), vm.Config{}, nil, &l)
		if err != nil {
			t.Fatalf("failed to create tester chain: %v", err)
		}
		time.Sleep(50 * time.Millisecond) // Wait for indices initialisation
		var tail uint64
		if l != 0 {
			tail = uint64(128) - l + 1
		}
		check(&tail, chain)
		chain.Stop()
		ancientDb.Close()
	}

	// Reconstruct a block chain which only reserves HEAD-64 tx indices
	ancientDb, err = rawdb.NewDatabaseWithFreezer(rawdb.NewMemoryDatabase(), frdir, "", false)
	if err != nil {
		t.Fatalf("failed to create temp freezer db: %v", err)
	}
	gspec.MustCommit(ancientDb)

	limit = []uint64{0, 64 /* drop stale */, 32 /* shorten history */, 64 /* extend history */, 0 /* restore all */}
	tails := []uint64{0, 67 /* 130 - 64 + 1 */, 100 /* 131 - 32 + 1 */, 69 /* 132 - 64 + 1 */, 0}
	for i, l := range limit {
		chain, err = NewBlockChain(ancientDb, nil, params.TestChainConfig, mockEngine.NewFaker(), vm.Config{}, nil, &l)
		if err != nil {
			t.Fatalf("failed to create tester chain: %v", err)
		}
		chain.InsertChain(blocks2[i : i+1]) // Feed chain a higher block to trigger indices updater.
		time.Sleep(50 * time.Millisecond)   // Wait for indices initialisation
		check(&tails[i], chain)
		chain.Stop()
	}
}

func TestSkipStaleTxIndicesInFastSync(t *testing.T) {
	// Configure and generate a sample block chain
	var (
		gendb   = rawdb.NewMemoryDatabase()
		key, _  = crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")
		address = crypto.PubkeyToAddress(key.PublicKey)
		funds   = big.NewInt(100000000000000000)
		gspec   = &Genesis{Config: params.TestChainConfig, Alloc: GenesisAlloc{address: {Balance: funds}}}
		genesis = gspec.MustCommit(gendb)
		signer  = types.LatestSigner(gspec.Config)
	)
	height := uint64(128)
	blocks, receipts := GenerateChain(gspec.Config, genesis, mockEngine.NewFaker(), gendb, int(height), func(i int, block *BlockGen) {
		tx, err := types.SignTx(types.NewTransaction(block.TxNonce(address), common.Address{0x00}, big.NewInt(1000), params.TxGas, nil, nil, nil, nil, nil), signer, key)
		if err != nil {
			panic(err)
		}
		block.AddTx(tx)
	})

	check := func(tail *uint64, chain *BlockChain) {
		stored := rawdb.ReadTxIndexTail(chain.db)
		if tail == nil && stored != nil {
			t.Fatalf("Oldest indexded block mismatch, want nil, have %d", *stored)
		}
		if tail != nil && *stored != *tail {
			t.Fatalf("Oldest indexded block mismatch, want %d, have %d", *tail, *stored)
		}
		if tail != nil {
			for i := *tail; i <= chain.CurrentBlock().NumberU64(); i++ {
				block := rawdb.ReadBlock(chain.db, rawdb.ReadCanonicalHash(chain.db, i), i)
				if block.Transactions().Len() == 0 {
					continue
				}
				for _, tx := range block.Transactions() {
					if index := rawdb.ReadTxLookupEntry(chain.db, tx.Hash()); index == nil {
						t.Fatalf("Miss transaction indice, number %d hash %s", i, tx.Hash().Hex())
					}
				}
			}
			for i := uint64(0); i < *tail; i++ {
				block := rawdb.ReadBlock(chain.db, rawdb.ReadCanonicalHash(chain.db, i), i)
				if block.Transactions().Len() == 0 {
					continue
				}
				for _, tx := range block.Transactions() {
					if index := rawdb.ReadTxLookupEntry(chain.db, tx.Hash()); index != nil {
						t.Fatalf("Transaction indice should be deleted, number %d hash %s", i, tx.Hash().Hex())
					}
				}
			}
		}
	}

	frdir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatalf("failed to create temp freezer dir: %v", err)
	}
	defer os.Remove(frdir)
	ancientDb, err := rawdb.NewDatabaseWithFreezer(rawdb.NewMemoryDatabase(), frdir, "", false)
	if err != nil {
		t.Fatalf("failed to create temp freezer db: %v", err)
	}
	gspec.MustCommit(ancientDb)

	// Import all blocks into ancient db, only HEAD-32 indices are kept.
	l := uint64(32)
	chain, err := NewBlockChain(ancientDb, nil, params.TestChainConfig, mockEngine.NewFaker(), vm.Config{}, nil, &l)
	if err != nil {
		t.Fatalf("failed to create tester chain: %v", err)
	}
	headers := make([]*types.Header, len(blocks))
	for i, block := range blocks {
		headers[i] = block.Header()
	}
	if n, err := chain.InsertHeaderChain(headers, 0, true); err != nil {
		t.Fatalf("failed to insert header %d: %v", n, err)
	}
	// The indices before ancient-N(32) should be ignored. After that all blocks should be indexed.
	if n, err := chain.InsertReceiptChain(blocks, receipts, 64); err != nil {
		t.Fatalf("block %d: failed to insert into chain: %v", n, err)
	}
	tail := uint64(32)
	check(&tail, chain)
}

// Benchmarks large blocks with value transfers to non-existing accounts
func benchmarkLargeNumberOfValueToNonexisting(b *testing.B, numTxs, numBlocks int, recipientFn func(uint64) common.Address, dataFn func(uint64) []byte) {
	var (
		signer          = types.HomesteadSigner{}
		testBankKey, _  = crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")
		testBankAddress = crypto.PubkeyToAddress(testBankKey.PublicKey)
		bankFunds       = big.NewInt(100000000000000000)
		gspec           = Genesis{
			Config: params.IstanbulTestChainConfig,
			Alloc: GenesisAlloc{
				testBankAddress: {Balance: bankFunds},
				common.HexToAddress("0xc0de"): {
					Code:    []byte{0x60, 0x01, 0x50},
					Balance: big.NewInt(0),
				}, // push 1, pop
			},
		}
	)
	// Generate the original common chain segment and the two competing forks
	engine := mockEngine.NewFaker()
	db := rawdb.NewMemoryDatabase()
	genesis := gspec.MustCommit(db)

	blockGenerator := func(i int, block *BlockGen) {
		block.SetCoinbase(common.Address{1})
		for txi := 0; txi < numTxs; txi++ {
			uniq := uint64(i*numTxs + txi)
			recipient := recipientFn(uniq)
			//recipient := common.BigToAddress(big.NewInt(0).SetUint64(1337 + uniq))
			tx, err := types.SignTx(types.NewTransaction(uniq, recipient, big.NewInt(1), params.TxGas, big.NewInt(1), nil, nil, nil, nil), signer, testBankKey)
			if err != nil {
				b.Error(err)
			}
			block.AddTx(tx)
		}
	}

	shared, _ := GenerateChain(params.IstanbulTestChainConfig, genesis, engine, db, numBlocks, blockGenerator)
	b.StopTimer()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Import the shared chain and the original canonical one
		diskdb := rawdb.NewMemoryDatabase()
		gspec.MustCommit(diskdb)

		chain, err := NewBlockChain(diskdb, nil, params.IstanbulTestChainConfig, engine, vm.Config{}, nil, nil)
		if err != nil {
			b.Fatalf("failed to create tester chain: %v", err)
		}
		b.StartTimer()
		if _, err := chain.InsertChain(shared); err != nil {
			b.Fatalf("failed to insert shared chain: %v", err)
		}
		b.StopTimer()
		if got := chain.CurrentBlock().Transactions().Len(); got != numTxs*numBlocks {
			b.Fatalf("Transactions were not included, expected %d, got %d", numTxs*numBlocks, got)

		}
	}
}

func BenchmarkBlockChain_1x1000ValueTransferToNonexisting(b *testing.B) {
	var (
		numTxs    = 1000
		numBlocks = 1
	)
	recipientFn := func(nonce uint64) common.Address {
		return common.BigToAddress(big.NewInt(0).SetUint64(1337 + nonce))
	}
	dataFn := func(nonce uint64) []byte {
		return nil
	}
	benchmarkLargeNumberOfValueToNonexisting(b, numTxs, numBlocks, recipientFn, dataFn)
}

func BenchmarkBlockChain_1x1000ValueTransferToExisting(b *testing.B) {
	var (
		numTxs    = 1000
		numBlocks = 1
	)
	b.StopTimer()
	b.ResetTimer()

	recipientFn := func(nonce uint64) common.Address {
		return common.BigToAddress(big.NewInt(0).SetUint64(1337))
	}
	dataFn := func(nonce uint64) []byte {
		return nil
	}
	benchmarkLargeNumberOfValueToNonexisting(b, numTxs, numBlocks, recipientFn, dataFn)
}

func BenchmarkBlockChain_1x1000Executions(b *testing.B) {
	var (
		numTxs    = 1000
		numBlocks = 1
	)
	b.StopTimer()
	b.ResetTimer()

	recipientFn := func(nonce uint64) common.Address {
		return common.BigToAddress(big.NewInt(0).SetUint64(0xc0de))
	}
	dataFn := func(nonce uint64) []byte {
		return nil
	}
	benchmarkLargeNumberOfValueToNonexisting(b, numTxs, numBlocks, recipientFn, dataFn)
}

// Tests that importing a some old blocks, where all blocks are before the
// pruning point.
// This internally leads to a sidechain import, since the blocks trigger an
// ErrPrunedAncestor error.
// This may e.g. happen if
//   1. Downloader rollbacks a batch of inserted blocks and exits
//   2. Downloader starts to sync again
//   3. The blocks fetched are all known and canonical blocks
func TestSideImportPrunedBlocks(t *testing.T) {
	//t.Skip("disabled temporarily, do not merge.")
	// Generate a canonical chain to act as the main dataset
	engine := mockEngine.NewFaker()
	db := rawdb.NewMemoryDatabase()
	genesis := new(Genesis).MustCommit(db)

	// Generate and import the canonical chain
	blocks, _ := GenerateChain(params.IstanbulTestChainConfig, genesis, engine, db, 2*TriesInMemory, nil)
	diskdb := rawdb.NewMemoryDatabase()
	new(Genesis).MustCommit(diskdb)
	chain, err := NewBlockChain(diskdb, nil, params.IstanbulTestChainConfig, engine, vm.Config{}, nil, nil)

	if err != nil {
		t.Fatalf("failed to create tester chain: %v", err)
	}
	if n, err := chain.InsertChain(blocks); err != nil {
		t.Fatalf("block %d: failed to insert into chain: %v", n, err)
	}

	lastPrunedIndex := len(blocks) - TriesInMemory - 1
	lastPrunedBlock := blocks[lastPrunedIndex]

	// Verify pruning of lastPrunedBlock
	if chain.HasBlockAndState(lastPrunedBlock.Hash(), lastPrunedBlock.NumberU64()) {
		t.Errorf("Block %d not pruned", lastPrunedBlock.NumberU64())
	}
	firstNonPrunedBlock := blocks[len(blocks)-TriesInMemory]
	// Verify firstNonPrunedBlock is not pruned
	if !chain.HasBlockAndState(firstNonPrunedBlock.Hash(), firstNonPrunedBlock.NumberU64()) {
		t.Errorf("Block %d pruned", firstNonPrunedBlock.NumberU64())
	}
	// Now re-import some old blocks
	blockToReimport := blocks[5:8]
	_, err = chain.InsertChain(blockToReimport)
	if err != nil {
		t.Errorf("Got error, %v", err)
	}
}

// TestDeleteCreateRevert tests a weird state transition corner case that we hit
// while changing the internals of statedb. The workflow is that a contract is
// self destructed, then in a followup transaction (but same block) it's created
// again and the transaction reverted.
//
// The original statedb implementation flushed dirty objects to the tries after
// each transaction, so this works ok. The rework accumulated writes in memory
// first, but the journal wiped the entire state object on create-revert.
func TestDeleteCreateRevert(t *testing.T) {
	var (
		aa = common.HexToAddress("0x000000000000000000000000000000000000aaaa")
		bb = common.HexToAddress("0x000000000000000000000000000000000000bbbb")
		// Generate a canonical chain to act as the main dataset
		engine = mockEngine.NewFaker()
		db     = rawdb.NewMemoryDatabase()

		// A sender who makes transactions, has some funds
		key, _  = crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")
		address = crypto.PubkeyToAddress(key.PublicKey)
		funds   = big.NewInt(100000000000000000)
		gspec   = &Genesis{
			Config: params.IstanbulTestChainConfig,
			Alloc: GenesisAlloc{
				address: {Balance: funds},
				// The address 0xAAAAA selfdestructs if called
				aa: {
					// Code needs to just selfdestruct
					Code:    []byte{byte(vm.PC), byte(vm.SELFDESTRUCT)},
					Nonce:   1,
					Balance: big.NewInt(0),
				},
				// The address 0xBBBB send 1 wei to 0xAAAA, then reverts
				bb: {
					Code: []byte{
						byte(vm.PC),          // [0]
						byte(vm.DUP1),        // [0,0]
						byte(vm.DUP1),        // [0,0,0]
						byte(vm.DUP1),        // [0,0,0,0]
						byte(vm.PUSH1), 0x01, // [0,0,0,0,1] (value)
						byte(vm.PUSH2), 0xaa, 0xaa, // [0,0,0,0,1, 0xaaaa]
						byte(vm.GAS),
						byte(vm.CALL),
						byte(vm.REVERT),
					},
					Balance: big.NewInt(1),
				},
			},
		}
		genesis = gspec.MustCommit(db)
	)

	blocks, _ := GenerateChain(params.IstanbulTestChainConfig, genesis, engine, db, 1, func(i int, b *BlockGen) {
		b.SetCoinbase(common.Address{1})
		// One transaction to AAAA
		tx, _ := types.SignTx(types.NewTransaction(0, aa,
			big.NewInt(0), 50000, big.NewInt(1), nil, nil, nil, nil), types.HomesteadSigner{}, key)
		b.AddTx(tx)
		// One transaction to BBBB
		tx, _ = types.SignTx(types.NewTransaction(1, bb,
			big.NewInt(0), 100000, big.NewInt(1), nil, nil, nil, nil), types.HomesteadSigner{}, key)
		b.AddTx(tx)
	})
	// Import the canonical chain
	diskdb := rawdb.NewMemoryDatabase()
	gspec.MustCommit(diskdb)

	chain, err := NewBlockChain(diskdb, nil, params.IstanbulTestChainConfig, engine, vm.Config{}, nil, nil)
	if err != nil {
		t.Fatalf("failed to create tester chain: %v", err)
	}
	if n, err := chain.InsertChain(blocks); err != nil {
		t.Fatalf("block %d: failed to insert into chain: %v", n, err)
	}
}

// TestDeleteRecreateSlots tests a state-transition that contains both deletion
// and recreation of contract state.
// Contract A exists, has slots 1 and 2 set
// Tx 1: Selfdestruct A
// Tx 2: Re-create A, set slots 3 and 4
// Expected outcome is that _all_ slots are cleared from A, due to the selfdestruct,
// and then the new slots exist
func TestDeleteRecreateSlots(t *testing.T) {
	var (
		// Generate a canonical chain to act as the main dataset
		engine = mockEngine.NewFaker()
		db     = rawdb.NewMemoryDatabase()
		// A sender who makes transactions, has some funds
		key, _    = crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")
		address   = crypto.PubkeyToAddress(key.PublicKey)
		funds     = big.NewInt(1000000000000000)
		bb        = common.HexToAddress("0x000000000000000000000000000000000000bbbb")
		aaStorage = make(map[common.Hash]common.Hash)          // Initial storage in AA
		aaCode    = []byte{byte(vm.PC), byte(vm.SELFDESTRUCT)} // Code for AA (simple selfdestruct)
	)
	// Populate two slots
	aaStorage[common.HexToHash("01")] = common.HexToHash("01")
	aaStorage[common.HexToHash("02")] = common.HexToHash("02")

	// The bb-code needs to CREATE2 the aa contract. It consists of
	// both initcode and deployment code
	// initcode:
	// 1. Set slots 3=3, 4=4,
	// 2. Return aaCode

	initCode := []byte{
		byte(vm.PUSH1), 0x3, // value
		byte(vm.PUSH1), 0x3, // location
		byte(vm.SSTORE),     // Set slot[3] = 1
		byte(vm.PUSH1), 0x4, // value
		byte(vm.PUSH1), 0x4, // location
		byte(vm.SSTORE), // Set slot[4] = 1
		// Slots are set, now return the code
		byte(vm.PUSH2), byte(vm.PC), byte(vm.SELFDESTRUCT), // Push code on stack
		byte(vm.PUSH1), 0x0, // memory start on stack
		byte(vm.MSTORE),
		// Code is now in memory.
		byte(vm.PUSH1), 0x2, // size
		byte(vm.PUSH1), byte(32 - 2), // offset
		byte(vm.RETURN),
	}
	if l := len(initCode); l > 32 {
		t.Fatalf("init code is too long for a pushx, need a more elaborate deployer")
	}
	bbCode := []byte{
		// Push initcode onto stack
		byte(vm.PUSH1) + byte(len(initCode)-1)}
	bbCode = append(bbCode, initCode...)
	bbCode = append(bbCode, []byte{
		byte(vm.PUSH1), 0x0, // memory start on stack
		byte(vm.MSTORE),
		byte(vm.PUSH1), 0x00, // salt
		byte(vm.PUSH1), byte(len(initCode)), // size
		byte(vm.PUSH1), byte(32 - len(initCode)), // offset
		byte(vm.PUSH1), 0x00, // endowment
		byte(vm.CREATE2),
	}...)

	initHash := crypto.Keccak256Hash(initCode)
	aa := crypto.CreateAddress2(bb, [32]byte{}, initHash[:])
	t.Logf("Destination address: %x\n", aa)

	gspec := &Genesis{
		Config: params.TestChainConfig,
		Alloc: GenesisAlloc{
			address: {Balance: funds},
			// The address 0xAAAAA selfdestructs if called
			aa: {
				// Code needs to just selfdestruct
				Code:    aaCode,
				Nonce:   1,
				Balance: big.NewInt(0),
				Storage: aaStorage,
			},
			// The contract BB recreates AA
			bb: {
				Code:    bbCode,
				Balance: big.NewInt(1),
			},
		},
	}
	genesis := gspec.MustCommit(db)

	blocks, _ := GenerateChain(params.TestChainConfig, genesis, engine, db, 1, func(i int, b *BlockGen) {
		b.SetCoinbase(common.Address{1})
		// One transaction to AA, to kill it
		tx, _ := types.SignTx(types.NewTransaction(0, aa,
			big.NewInt(0), 50000, big.NewInt(1), nil, nil, nil, nil), types.HomesteadSigner{}, key)

		b.AddTx(tx)
		// One transaction to BB, to recreate AA
		tx, _ = types.SignTx(types.NewTransaction(1, bb,
			big.NewInt(0), 100000, big.NewInt(1), nil, nil, nil, nil), types.HomesteadSigner{}, key)
		b.AddTx(tx)
	})
	// Import the canonical chain
	diskdb := rawdb.NewMemoryDatabase()
	gspec.MustCommit(diskdb)
	chain, err := NewBlockChain(diskdb, nil, params.TestChainConfig, engine, vm.Config{
		Debug:  true,
		Tracer: vm.NewJSONLogger(nil, os.Stdout),
	}, nil, nil)
	if err != nil {
		t.Fatalf("failed to create tester chain: %v", err)
	}
	if n, err := chain.InsertChain(blocks); err != nil {
		t.Fatalf("block %d: failed to insert into chain: %v", n, err)
	}
	statedb, _ := chain.State()

	// If all is correct, then slot 1 and 2 are zero
	if got, exp := statedb.GetState(aa, common.HexToHash("01")), (common.Hash{}); got != exp {
		t.Errorf("got %x exp %x", got, exp)
	}
	if got, exp := statedb.GetState(aa, common.HexToHash("02")), (common.Hash{}); got != exp {
		t.Errorf("got %x exp %x", got, exp)
	}
	// Also, 3 and 4 should be set
	if got, exp := statedb.GetState(aa, common.HexToHash("03")), common.HexToHash("03"); got != exp {
		t.Fatalf("got %x exp %x", got, exp)
	}
	if got, exp := statedb.GetState(aa, common.HexToHash("04")), common.HexToHash("04"); got != exp {
		t.Fatalf("got %x exp %x", got, exp)
	}
}

// TestDeleteRecreateAccount tests a state-transition that contains deletion of a
// contract with storage, and a recreate of the same contract via a
// regular value-transfer
// Expected outcome is that _all_ slots are cleared from A
func TestDeleteRecreateAccount(t *testing.T) {
	var (
		// Generate a canonical chain to act as the main dataset
		engine = mockEngine.NewFaker()
		db     = rawdb.NewMemoryDatabase()
		// A sender who makes transactions, has some funds
		key, _  = crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")
		address = crypto.PubkeyToAddress(key.PublicKey)
		funds   = big.NewInt(1000000000000000)

		aa        = common.HexToAddress("0x7217d81b76bdd8707601e959454e3d776aee5f43")
		aaStorage = make(map[common.Hash]common.Hash)          // Initial storage in AA
		aaCode    = []byte{byte(vm.PC), byte(vm.SELFDESTRUCT)} // Code for AA (simple selfdestruct)
	)
	// Populate two slots
	aaStorage[common.HexToHash("01")] = common.HexToHash("01")
	aaStorage[common.HexToHash("02")] = common.HexToHash("02")

	gspec := &Genesis{
		Config: params.TestChainConfig,
		Alloc: GenesisAlloc{
			address: {Balance: funds},
			// The address 0xAAAAA selfdestructs if called
			aa: {
				// Code needs to just selfdestruct
				Code:    aaCode,
				Nonce:   1,
				Balance: big.NewInt(0),
				Storage: aaStorage,
			},
		},
	}
	genesis := gspec.MustCommit(db)

	blocks, _ := GenerateChain(params.TestChainConfig, genesis, engine, db, 1, func(i int, b *BlockGen) {
		b.SetCoinbase(common.Address{1})
		// One transaction to AA, to kill it
		tx, _ := types.SignTx(types.NewTransaction(0, aa,
			big.NewInt(0), 50000, big.NewInt(1), nil, nil, nil, nil), types.HomesteadSigner{}, key)
		b.AddTx(tx)
		// One transaction to AA, to recreate it (but without storage
		tx, _ = types.SignTx(types.NewTransaction(1, aa,
			big.NewInt(1), 100000, big.NewInt(1), nil, nil, nil, nil), types.HomesteadSigner{}, key)

		b.AddTx(tx)
	})
	// Import the canonical chain
	diskdb := rawdb.NewMemoryDatabase()
	gspec.MustCommit(diskdb)
	chain, err := NewBlockChain(diskdb, nil, params.TestChainConfig, engine, vm.Config{
		Debug:  true,
		Tracer: vm.NewJSONLogger(nil, os.Stdout),
	}, nil, nil)
	if err != nil {
		t.Fatalf("failed to create tester chain: %v", err)
	}
	if n, err := chain.InsertChain(blocks); err != nil {
		t.Fatalf("block %d: failed to insert into chain: %v", n, err)
	}
	statedb, _ := chain.State()

	// If all is correct, then both slots are zero
	if got, exp := statedb.GetState(aa, common.HexToHash("01")), (common.Hash{}); got != exp {
		t.Errorf("got %x exp %x", got, exp)
	}
	if got, exp := statedb.GetState(aa, common.HexToHash("02")), (common.Hash{}); got != exp {
		t.Errorf("got %x exp %x", got, exp)
	}
}

// TestDeleteRecreateSlotsAcrossManyBlocks tests multiple state-transition that contains both deletion
// and recreation of contract state.
// Contract A exists, has slots 1 and 2 set
// Tx 1: Selfdestruct A
// Tx 2: Re-create A, set slots 3 and 4
// Expected outcome is that _all_ slots are cleared from A, due to the selfdestruct,
// and then the new slots exist
func TestDeleteRecreateSlotsAcrossManyBlocks(t *testing.T) {
	var (
		// Generate a canonical chain to act as the main dataset
		engine = mockEngine.NewFaker()
		db     = rawdb.NewMemoryDatabase()
		// A sender who makes transactions, has some funds
		key, _    = crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")
		address   = crypto.PubkeyToAddress(key.PublicKey)
		funds     = big.NewInt(1000000000000000)
		bb        = common.HexToAddress("0x000000000000000000000000000000000000bbbb")
		aaStorage = make(map[common.Hash]common.Hash)          // Initial storage in AA
		aaCode    = []byte{byte(vm.PC), byte(vm.SELFDESTRUCT)} // Code for AA (simple selfdestruct)
	)
	// Populate two slots
	aaStorage[common.HexToHash("01")] = common.HexToHash("01")
	aaStorage[common.HexToHash("02")] = common.HexToHash("02")

	// The bb-code needs to CREATE2 the aa contract. It consists of
	// both initcode and deployment code
	// initcode:
	// 1. Set slots 3=blocknum+1, 4=4,
	// 2. Return aaCode

	initCode := []byte{
		byte(vm.PUSH1), 0x1, //
		byte(vm.NUMBER),     // value = number + 1
		byte(vm.ADD),        //
		byte(vm.PUSH1), 0x3, // location
		byte(vm.SSTORE),     // Set slot[3] = number + 1
		byte(vm.PUSH1), 0x4, // value
		byte(vm.PUSH1), 0x4, // location
		byte(vm.SSTORE), // Set slot[4] = 4
		// Slots are set, now return the code
		byte(vm.PUSH2), byte(vm.PC), byte(vm.SELFDESTRUCT), // Push code on stack
		byte(vm.PUSH1), 0x0, // memory start on stack
		byte(vm.MSTORE),
		// Code is now in memory.
		byte(vm.PUSH1), 0x2, // size
		byte(vm.PUSH1), byte(32 - 2), // offset
		byte(vm.RETURN),
	}
	if l := len(initCode); l > 32 {
		t.Fatalf("init code is too long for a pushx, need a more elaborate deployer")
	}
	bbCode := []byte{
		// Push initcode onto stack
		byte(vm.PUSH1) + byte(len(initCode)-1)}
	bbCode = append(bbCode, initCode...)
	bbCode = append(bbCode, []byte{
		byte(vm.PUSH1), 0x0, // memory start on stack
		byte(vm.MSTORE),
		byte(vm.PUSH1), 0x00, // salt
		byte(vm.PUSH1), byte(len(initCode)), // size
		byte(vm.PUSH1), byte(32 - len(initCode)), // offset
		byte(vm.PUSH1), 0x00, // endowment
		byte(vm.CREATE2),
	}...)

	initHash := crypto.Keccak256Hash(initCode)
	aa := crypto.CreateAddress2(bb, [32]byte{}, initHash[:])
	t.Logf("Destination address: %x\n", aa)
	gspec := &Genesis{
		Config: params.TestChainConfig,
		Alloc: GenesisAlloc{
			address: {Balance: funds},
			// The address 0xAAAAA selfdestructs if called
			aa: {
				// Code needs to just selfdestruct
				Code:    aaCode,
				Nonce:   1,
				Balance: big.NewInt(0),
				Storage: aaStorage,
			},
			// The contract BB recreates AA
			bb: {
				Code:    bbCode,
				Balance: big.NewInt(1),
			},
		},
	}
	genesis := gspec.MustCommit(db)
	var nonce uint64

	type expectation struct {
		exist    bool
		blocknum int
		values   map[int]int
	}
	var current = &expectation{
		exist:    true, // exists in genesis
		blocknum: 0,
		values:   map[int]int{1: 1, 2: 2},
	}
	var expectations []*expectation
	var newDestruct = func(e *expectation, b *BlockGen) *types.Transaction {
		tx, _ := types.SignTx(types.NewTransaction(nonce, aa,
			big.NewInt(0), 50000, big.NewInt(1), nil, nil, nil, nil), types.HomesteadSigner{}, key)
		nonce++
		if e.exist {
			e.exist = false
			e.values = nil
		}
		t.Logf("block %d; adding destruct\n", e.blocknum)
		return tx
	}
	var newResurrect = func(e *expectation, b *BlockGen) *types.Transaction {
		tx, _ := types.SignTx(types.NewTransaction(nonce, bb,
			big.NewInt(0), 100000, big.NewInt(1), nil, nil, nil, nil), types.HomesteadSigner{}, key)
		nonce++
		if !e.exist {
			e.exist = true
			e.values = map[int]int{3: e.blocknum + 1, 4: 4}
		}
		t.Logf("block %d; adding resurrect\n", e.blocknum)
		return tx
	}

	blocks, _ := GenerateChain(params.TestChainConfig, genesis, engine, db, 150, func(i int, b *BlockGen) {
		var exp = new(expectation)
		exp.blocknum = i + 1
		exp.values = make(map[int]int)
		for k, v := range current.values {
			exp.values[k] = v
		}
		exp.exist = current.exist

		b.SetCoinbase(common.Address{1})
		if i%2 == 0 {
			b.AddTx(newDestruct(exp, b))
		}
		if i%3 == 0 {
			b.AddTx(newResurrect(exp, b))
		}
		if i%5 == 0 {
			b.AddTx(newDestruct(exp, b))
		}
		if i%7 == 0 {
			b.AddTx(newResurrect(exp, b))
		}
		expectations = append(expectations, exp)
		current = exp
	})
	// Import the canonical chain
	diskdb := rawdb.NewMemoryDatabase()
	gspec.MustCommit(diskdb)
	chain, err := NewBlockChain(diskdb, nil, params.TestChainConfig, engine, vm.Config{
		//Debug:  true,
		//Tracer: vm.NewJSONLogger(nil, os.Stdout),
	}, nil, nil)
	if err != nil {
		t.Fatalf("failed to create tester chain: %v", err)
	}
	var asHash = func(num int) common.Hash {
		return common.BytesToHash([]byte{byte(num)})
	}
	for i, block := range blocks {
		blockNum := i + 1
		if n, err := chain.InsertChain([]*types.Block{block}); err != nil {
			t.Fatalf("block %d: failed to insert into chain: %v", n, err)
		}
		statedb, _ := chain.State()
		// If all is correct, then slot 1 and 2 are zero
		if got, exp := statedb.GetState(aa, common.HexToHash("01")), (common.Hash{}); got != exp {
			t.Errorf("block %d, got %x exp %x", blockNum, got, exp)
		}
		if got, exp := statedb.GetState(aa, common.HexToHash("02")), (common.Hash{}); got != exp {
			t.Errorf("block %d, got %x exp %x", blockNum, got, exp)
		}
		exp := expectations[i]
		if exp.exist {
			if !statedb.Exist(aa) {
				t.Fatalf("block %d, expected %v to exist, it did not", blockNum, aa)
			}
			for slot, val := range exp.values {
				if gotValue, expValue := statedb.GetState(aa, asHash(slot)), asHash(val); gotValue != expValue {
					t.Fatalf("block %d, slot %d, got %x exp %x", blockNum, slot, gotValue, expValue)
				}
			}
		} else {
			if statedb.Exist(aa) {
				t.Fatalf("block %d, expected %v to not exist, it did", blockNum, aa)
			}
		}
	}
}

// TestInitThenFailCreateContract tests a pretty notorious case that happened
// on mainnet over blocks 7338108, 7338110 and 7338115.
// - Block 7338108: address e771789f5cccac282f23bb7add5690e1f6ca467c is initiated
//   with 0.001 ether (thus created but no code)
// - Block 7338110: a CREATE2 is attempted. The CREATE2 would deploy code on
//   the same address e771789f5cccac282f23bb7add5690e1f6ca467c. However, the
//   deployment fails due to OOG during initcode execution
// - Block 7338115: another tx checks the balance of
//   e771789f5cccac282f23bb7add5690e1f6ca467c, and the snapshotter returned it as
//   zero.
//
// The problem being that the snapshotter maintains a destructset, and adds items
// to the destructset in case something is created "onto" an existing item.
// We need to either roll back the snapDestructs, or not place it into snapDestructs
// in the first place.
//
func TestInitThenFailCreateContract(t *testing.T) {
	var (
		// Generate a canonical chain to act as the main dataset
		engine = mockEngine.NewFaker()
		db     = rawdb.NewMemoryDatabase()
		// A sender who makes transactions, has some funds
		key, _  = crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")
		address = crypto.PubkeyToAddress(key.PublicKey)
		funds   = big.NewInt(1000000000000000)
		bb      = common.HexToAddress("0x000000000000000000000000000000000000bbbb")
	)

	// The bb-code needs to CREATE2 the aa contract. It consists of
	// both initcode and deployment code
	// initcode:
	// 1. If blocknum < 1, error out (e.g invalid opcode)
	// 2. else, return a snippet of code
	initCode := []byte{
		byte(vm.PUSH1), 0x1, // y (2)
		byte(vm.NUMBER), // x (number)
		byte(vm.GT),     // x > y?
		byte(vm.PUSH1), byte(0x8),
		byte(vm.JUMPI), // jump to label if number > 2
		byte(0xFE),     // illegal opcode
		byte(vm.JUMPDEST),
		byte(vm.PUSH1), 0x2, // size
		byte(vm.PUSH1), 0x0, // offset
		byte(vm.RETURN), // return 2 bytes of zero-code
	}
	if l := len(initCode); l > 32 {
		t.Fatalf("init code is too long for a pushx, need a more elaborate deployer")
	}
	bbCode := []byte{
		// Push initcode onto stack
		byte(vm.PUSH1) + byte(len(initCode)-1)}
	bbCode = append(bbCode, initCode...)
	bbCode = append(bbCode, []byte{
		byte(vm.PUSH1), 0x0, // memory start on stack
		byte(vm.MSTORE),
		byte(vm.PUSH1), 0x00, // salt
		byte(vm.PUSH1), byte(len(initCode)), // size
		byte(vm.PUSH1), byte(32 - len(initCode)), // offset
		byte(vm.PUSH1), 0x00, // endowment
		byte(vm.CREATE2),
	}...)

	initHash := crypto.Keccak256Hash(initCode)
	aa := crypto.CreateAddress2(bb, [32]byte{}, initHash[:])
	t.Logf("Destination address: %x\n", aa)

	gspec := &Genesis{
		Config: params.TestChainConfig,
		Alloc: GenesisAlloc{
			address: {Balance: funds},
			// The address aa has some funds
			aa: {Balance: big.NewInt(100000)},
			// The contract BB tries to create code onto AA
			bb: {
				Code:    bbCode,
				Balance: big.NewInt(1),
			},
		},
	}
	genesis := gspec.MustCommit(db)
	nonce := uint64(0)
	blocks, _ := GenerateChain(params.TestChainConfig, genesis, engine, db, 4, func(i int, b *BlockGen) {
		b.SetCoinbase(common.Address{1})
		// One transaction to BB
		tx, _ := types.SignTx(types.NewTransaction(nonce, bb,
			big.NewInt(0), 100000, big.NewInt(1), nil, nil, nil, nil), types.HomesteadSigner{}, key)
		b.AddTx(tx)
		nonce++
	})

	// Import the canonical chain
	diskdb := rawdb.NewMemoryDatabase()
	gspec.MustCommit(diskdb)
	chain, err := NewBlockChain(diskdb, nil, params.TestChainConfig, engine, vm.Config{
		//Debug:  true,
		//Tracer: vm.NewJSONLogger(nil, os.Stdout),
	}, nil, nil)
	if err != nil {
		t.Fatalf("failed to create tester chain: %v", err)
	}
	statedb, _ := chain.State()
	if got, exp := statedb.GetBalance(aa), big.NewInt(100000); got.Cmp(exp) != 0 {
		t.Fatalf("Genesis err, got %v exp %v", got, exp)
	}
	// First block tries to create, but fails
	{
		block := blocks[0]
		if _, err := chain.InsertChain([]*types.Block{blocks[0]}); err != nil {
			t.Fatalf("block %d: failed to insert into chain: %v", block.NumberU64(), err)
		}
		statedb, _ = chain.State()
		if got, exp := statedb.GetBalance(aa), big.NewInt(100000); got.Cmp(exp) != 0 {
			t.Fatalf("block %d: got %v exp %v", block.NumberU64(), got, exp)
		}
	}
	// Import the rest of the blocks
	for _, block := range blocks[1:] {
		if _, err := chain.InsertChain([]*types.Block{block}); err != nil {
			t.Fatalf("block %d: failed to insert into chain: %v", block.NumberU64(), err)
		}
	}
}

// TestEIP2718Transition tests that an EIP-2718 transaction will be accepted
// after the fork block has passed. This is verified by sending an EIP-2930
// access list transaction, which specifies a single slot access, and then
// checking that the gas usage of a hot SLOAD and a cold SLOAD are calculated
// correctly.
func TestEIP2718Transition(t *testing.T) {
	var (
		aa = common.HexToAddress("0x000000000000000000000000000000000000aaaa")

		// Generate a canonical chain to act as the main dataset
		engine = mockEngine.NewFaker()
		db     = rawdb.NewMemoryDatabase()

		// A sender who makes transactions, has some funds
		key, _  = crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")
		address = crypto.PubkeyToAddress(key.PublicKey)
		funds   = big.NewInt(1000000000000000)
		gspec   = &Genesis{
			Config: params.IstanbulEHFTestChainConfig,
			Alloc: GenesisAlloc{
				address: {Balance: funds},
				// The address 0xAAAA sloads 0x00 and 0x01
				aa: {
					Code: []byte{
						byte(vm.PC),
						byte(vm.PC),
						byte(vm.SLOAD),
						byte(vm.SLOAD),
					},
					Nonce:   0,
					Balance: big.NewInt(0),
				},
			},
		}
	)
	gspec.Config.Faker = true
	genesis := gspec.MustCommit(db)

	blocks, _ := GenerateChain(gspec.Config, genesis, engine, db, 1, func(i int, b *BlockGen) {
		b.SetCoinbase(common.Address{1})

		// One transaction to 0xAAAA
		signer := types.LatestSigner(gspec.Config)
		tx, _ := types.SignNewTx(key, signer, &types.AccessListTx{
			ChainID:  gspec.Config.ChainID,
			Nonce:    0,
			To:       &aa,
			Gas:      30000,
			GasPrice: MockSysContractCallCtx().GetGasPriceMinimum(nil),
			AccessList: types.AccessList{{
				Address:     aa,
				StorageKeys: []common.Hash{{0}},
			}},
		})
		b.AddTx(tx)
	})

	// Import the canonical chain
	diskdb := rawdb.NewMemoryDatabase()
	gspec.MustCommit(diskdb)

	chain, err := NewBlockChain(diskdb, nil, gspec.Config, engine, vm.Config{}, nil, nil)
	if err != nil {
		t.Fatalf("failed to create tester chain: %v", err)
	}
	if n, err := chain.InsertChain(blocks); err != nil {
		t.Fatalf("block %d: failed to insert into chain: %v", n, err)
	}

	block := chain.GetBlockByNumber(1)

	// Expected gas is intrinsic + 2 * pc + hot load + cold load, since only one load is in the access list
	expected := params.TxGas + params.TxAccessListAddressGas + params.TxAccessListStorageKeyGas +
		vm.GasQuickStep*2 + params.WarmStorageReadCostEIP2929 + params.ColdSloadCostEIP2929
	if block.GasUsed() != expected {
		t.Fatalf("incorrect amount of gas spent: expected %d, got %d", expected, block.GasUsed())

	}
}

// TestEIP1559Transition tests the following:
//
// 1. A transaction whose gasFeeCap is greater than the baseFee is valid.
// 2. Gas accounting for access lists on EIP-1559 transactions is correct.
// 3. Only the transaction's tip will be received by the coinbase.
// 4. The transaction sender pays for both the tip and baseFee.
// 5. The coinbase receives only the partially realized tip when
//    gasFeeCap - gasTipCap < baseFee.
// 6. Legacy transaction behave as expected (e.g. gasPrice = gasFeeCap = gasTipCap).
func TestEIP1559Transition(t *testing.T) {
	var (
		aa = common.HexToAddress("0x000000000000000000000000000000000000aaaa")

		// Generate a canonical chain to act as the main dataset
		engine = mockEngine.NewFaker()
		db     = rawdb.NewMemoryDatabase()

		// A sender who makes transactions, has some funds
		key1, _ = crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")
		key2, _ = crypto.HexToECDSA("8a1f9a8f95be41cd7ccb6168179afb4504aefe388d1e14474d32c45c72ce7b7a")
		addr1   = crypto.PubkeyToAddress(key1.PublicKey)
		addr2   = crypto.PubkeyToAddress(key2.PublicKey)
		funds   = new(big.Int).Mul(common.Big1, big.NewInt(params.Ether))
		gspec   = &Genesis{
			Config: params.IstanbulEHFTestChainConfig,
			Alloc: GenesisAlloc{
				addr1: {Balance: funds},
				addr2: {Balance: funds},
				// The address 0xAAAA sloads 0x00 and 0x01
				aa: {
					Code: []byte{
						byte(vm.PC),
						byte(vm.PC),
						byte(vm.SLOAD),
						byte(vm.SLOAD),
					},
					Nonce:   0,
					Balance: big.NewInt(0),
				},
				common.HexToAddress("000000000000000000000000000000000000a001"): {
					Code:    []byte("0x73000000000000000000000000000000000000000030146080604052600080fdfea265627a7a7231582099b904e82bc6cbeddd57f9cea6a1e6e0184c49b2cae2764fd9e194e0fffd3b0764736f6c634300050d0032"),
					Balance: big.NewInt(0),
				},
				common.HexToAddress("000000000000000000000000000000000000a002"): {
					Code:    []byte("0x73000000000000000000000000000000000000000030146080604052600436106100615760003560e01c80633053123f1461006657806354255be01461031e578063b05cd27f14610351578063c67e7b4b146103ba578063e6a5192f146103f5575b600080fd5b81801561007257600080fd5b5061031c600480360360e081101561008957600080fd5b8101908080359060200190929190803590602001906401000000008111156100b057600080fd5b8201836020820111156100c257600080fd5b803590602001918460208302840111640100000000831117156100e457600080fd5b919080806020026020016040519081016040528093929190818152602001838360200280828437600081840152601f19601f8201169050808301925050505050505091929192908035906020019064010000000081111561014457600080fd5b82018360208201111561015657600080fd5b8035906020019184602083028401116401000000008311171561017857600080fd5b919080806020026020016040519081016040528093929190818152602001838360200280828437600081840152601f19601f820116905080830192505050505050509192919290803590602001906401000000008111156101d857600080fd5b8201836020820111156101ea57600080fd5b8035906020019184600183028401116401000000008311171561020c57600080fd5b91908080601f016020809104026020016040519081016040528093929190818152602001838380828437600081840152601f19601f8201169050808301925050505050505091929192908035906020019064010000000081111561026f57600080fd5b82018360208201111561028157600080fd5b803590602001918460208302840111640100000000831117156102a357600080fd5b919080806020026020016040519081016040528093929190818152602001838360200280828437600081840152601f19601f820116905080830192505050505050509192919290803573ffffffffffffffffffffffffffffffffffffffff169060200190929190803590602001909291905050506104e0565b005b61032661074f565b6040518085815260200184815260200183815260200182815260200194505050505060405180910390f35b81801561035d57600080fd5b506103b8600480360360a081101561037457600080fd5b81019080803590602001909291908035906020019092919080359060200190929190803560ff169060200190929190803560ff169060200190929190505050610776565b005b8180156103c657600080fd5b506103f3600480360360208110156103dd57600080fd5b810190808035906020019092919050505061092d565b005b61042b6004803603604081101561040b57600080fd5b810190808035906020019092919080359060200190929190505050610a94565b604051808481526020018373ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200180602001828103825283818151815260200191508051906020019080838360005b838110156104a3578082015181840152602081019050610488565b50505050905090810190601f1680156104d05780820380516001836020036101000a031916815260200191505b5094505050505060405180910390f35b845186511480156104f2575082518551145b610564576040517f08c379a00000000000000000000000000000000000000000000000000000000081526004018080602001828103825260158152602001807f4172726179206c656e677468206d69736d61746368000000000000000000000081525060200191505060405180910390fd5b600086519050828860000160006101000a81548173ffffffffffffffffffffffffffffffffffffffff021916908373ffffffffffffffffffffffffffffffffffffffff16021790555081886001018190555042886002018190555060008090508860060160006105d4919061102c565b60008090505b82811015610743578960060160405180606001604052808b84815181106105fd57fe5b602002602001015181526020018a848151811061061657fe5b602002602001015173ffffffffffffffffffffffffffffffffffffffff168152602001610661858a868151811061064957fe5b60200260200101518c610c0e9092919063ffffffff16565b815250908060018154018082558091505090600182039060005260206000209060030201600090919290919091506000820151816000015560208201518160010160006101000a81548173ffffffffffffffffffffffffffffffffffffffff021916908373ffffffffffffffffffffffffffffffffffffffff16021790555060408201518160020190805190602001906106fc929190611050565b5050505061072686828151811061070f57fe5b602002602001015183610c9a90919063ffffffff16565b915061073c600182610c9a90919063ffffffff16565b90506105da565b50505050505050505050565b60008060008060018060016000839350829250819150809050935093509350935090919293565b6001600381111561078357fe5b82600381111561078f57fe5b14156107bf576107af848660030160020154610d2290919063ffffffff16565b856003016002018190555061084e565b6003808111156107cb57fe5b8260038111156107d757fe5b1415610807576107f7848660030160000154610d2290919063ffffffff16565b856003016000018190555061084d565b6002600381111561081457fe5b82600381111561082057fe5b141561084c57610840848660030160010154610d2290919063ffffffff16565b85600301600101819055505b5b5b6001600381111561085b57fe5b81600381111561086757fe5b141561089757610887838660030160020154610c9a90919063ffffffff16565b8560030160020181905550610926565b6003808111156108a357fe5b8160038111156108af57fe5b14156108df576108cf838660030160000154610c9a90919063ffffffff16565b8560030160000181905550610925565b600260038111156108ec57fe5b8160038111156108f857fe5b141561092457610918838660030160010154610c9a90919063ffffffff16565b85600301600101819055505b5b5b5050505050565b610a9181600601805480602002602001604051908101604052809291908181526020016000905b82821015610a885783829060005260206000209060030201604051806060016040529081600082015481526020016001820160009054906101000a900473ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff168152602001600282018054600181600116156101000203166002900480601f016020809104026020016040519081016040528092919081815260200182805460018160011615610100020316600290048015610a705780601f10610a4557610100808354040283529160200191610a70565b820191906000526020600020905b815481529060010190602001808311610a5357829003601f168201915b50505050508152505081526020019060010190610954565b50505050610d6c565b50565b600080606084600601805490508410610b15576040517f08c379a00000000000000000000000000000000000000000000000000000000081526004018080602001828103825260198152602001807f6765745472616e73616374696f6e3a2062616420696e6465780000000000000081525060200191505060405180910390fd5b6000856006018581548110610b2657fe5b9060005260206000209060030201905080600001548160010160009054906101000a900473ffffffffffffffffffffffffffffffffffffffff1682600201808054600181600116156101000203166002900480601f016020809104026020016040519081016040528092919081815260200182805460018160011615610100020316600290048015610bf95780601f10610bce57610100808354040283529160200191610bf9565b820191906000526020600020905b815481529060010190602001808311610bdc57829003601f168201915b50505050509050935093509350509250925092565b606081830184511015610c2057600080fd5b6060821560008114610c3d57604051915060208201604052610c8e565b6040519150601f8416801560200281840101858101878315602002848b0101015b81831015610c7b5780518352602083019250602081019050610c5e565b50868552601f19601f8301166040525050505b50809150509392505050565b600080828401905083811015610d18576040517f08c379a000000000000000000000000000000000000000000000000000000000815260040180806020018281038252601b8152602001807f536166654d6174683a206164646974696f6e206f766572666c6f77000000000081525060200191505060405180910390fd5b8091505092915050565b6000610d6483836040518060400160405280601e81526020017f536166654d6174683a207375627472616374696f6e206f766572666c6f770000815250610e75565b905092915050565b60008090505b8151811015610e7157610de4828281518110610d8a57fe5b602002602001015160200151838381518110610da257fe5b602002602001015160000151848481518110610dba57fe5b60200260200101516040015151858581518110610dd357fe5b602002602001015160400151610f35565b610e56576040517f08c379a00000000000000000000000000000000000000000000000000000000081526004018080602001828103825260198152602001807f50726f706f73616c20657865637574696f6e206661696c65640000000000000081525060200191505060405180910390fd5b610e6a600182610c9a90919063ffffffff16565b9050610d72565b5050565b6000838311158290610f22576040517f08c379a00000000000000000000000000000000000000000000000000000000081526004018080602001828103825283818151815260200191508051906020019080838360005b83811015610ee7578082015181840152602081019050610ecc565b50505050905090810190601f168015610f145780820380516001836020036101000a031916815260200191505b509250505060405180910390fd5b5060008385039050809150509392505050565b6000806000841115610fbd57610f4a86610fe1565b610fbc576040517f08c379a00000000000000000000000000000000000000000000000000000000081526004018080602001828103825260188152602001807f496e76616c696420636f6e74726163742061646472657373000000000000000081525060200191505060405180910390fd5b5b6040516020840160008287838a8c6187965a03f19250505080915050949350505050565b60008060007fc5d2460186f7233c927e7db2dcc703c0e500b653ca82273b7bfad8045d85a47060001b9050833f915080821415801561102357506000801b8214155b92505050919050565b508054600082556003029060005260206000209081019061104d91906110d0565b50565b828054600181600116156101000203166002900490600052602060002090601f016020900481019282601f1061109157805160ff19168380011785556110bf565b828001600101855582156110bf579182015b828111156110be5782518255916020019190600101906110a3565b5b5090506110cc919061112e565b5090565b61112b91905b80821115611127576000808201600090556001820160006101000a81549073ffffffffffffffffffffffffffffffffffffffff021916905560028201600061111e9190611153565b506003016110d6565b5090565b90565b61115091905b8082111561114c576000816000905550600101611134565b5090565b90565b50805460018160011615610100020316600290046000825580601f106111795750611198565b601f016020900490600052602060002090810190611197919061112e565b5b5056fea265627a7a7231582096bb20c6db9a6f357c22e7fa39a61e9016a70a7b4090a08952bff9a3747a3fa864736f6c634300050d0032"),
					Balance: big.NewInt(0),
				},
				common.HexToAddress("000000000000000000000000000000000000a003"): {
					Code:    []byte("0x73000000000000000000000000000000000000000030146080604052600080fdfea265627a7a72315820b0948d304c237fa63de9ce048ba6704e9ee859fa687ec434b5ffd2db6499d14a64736f6c634300050d0032"),
					Balance: big.NewInt(0),
				},
				common.HexToAddress("000000000000000000000000000000000000a004"): {
					Code:    []byte("0x73000000000000000000000000000000000000000030146080604052600080fdfea265627a7a72315820cfb8d22e3b3fa4f69f022b787b64293ca10f72080c844cc086c6a0c690f1573664736f6c634300050d0032"),
					Balance: big.NewInt(0),
				},
				common.HexToAddress("000000000000000000000000000000000000a006"): {
					Code:    []byte("0x73000000000000000000000000000000000000000030146080604052600436106100a85760003560e01c8063593b79fe11610070578063593b79fe146102aa578063b1cfea4314610302578063b2f8fe961461038f578063e2c0c56a1461042a578063fe3c7a8e14610485576100a8565b806307debf7c146100ad57806326afac4914610148578063341f6623146101a3578063542424fb1461021157806354255be014610277575b600080fd5b8180156100b957600080fd5b50610146600480360360808110156100d057600080fd5b8101908080359060200190929190803573ffffffffffffffffffffffffffffffffffffffff169060200190929190803573ffffffffffffffffffffffffffffffffffffffff169060200190929190803573ffffffffffffffffffffffffffffffffffffffff169060200190929190505050610508565b005b81801561015457600080fd5b506101a16004803603604081101561016b57600080fd5b8101908080359060200190929190803573ffffffffffffffffffffffffffffffffffffffff16906020019092919050505061053d565b005b6101cf600480360360208110156101b957600080fd5b8101908080359060200190929190505050610567565b604051808273ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200191505060405180910390f35b61025d6004803603604081101561022757600080fd5b8101908080359060200190929190803573ffffffffffffffffffffffffffffffffffffffff169060200190929190505050610578565b604051808215151515815260200191505060405180910390f35b61027f6105b0565b6040518085815260200184815260200183815260200182815260200194505050505060405180910390f35b6102ec600480360360208110156102c057600080fd5b81019080803573ffffffffffffffffffffffffffffffffffffffff1690602001909291905050506105d7565b6040518082815260200191505060405180910390f35b6103386004803603604081101561031857600080fd5b8101908080359060200190929190803590602001909291905050506105fe565b6040518080602001828103825283818151815260200191508051906020019060200280838360005b8381101561037b578082015181840152602081019050610360565b505050509050019250505060405180910390f35b81801561039b57600080fd5b50610428600480360360808110156103b257600080fd5b8101908080359060200190929190803573ffffffffffffffffffffffffffffffffffffffff169060200190929190803573ffffffffffffffffffffffffffffffffffffffff169060200190929190803573ffffffffffffffffffffffffffffffffffffffff1690602001909291905050506106e1565b005b81801561043657600080fd5b506104836004803603604081101561044d57600080fd5b8101908080359060200190929190803573ffffffffffffffffffffffffffffffffffffffff169060200190929190505050610716565b005b6104b16004803603602081101561049b57600080fd5b8101908080359060200190929190505050610735565b6040518080602001828103825283818151815260200191508051906020019060200280838360005b838110156104f45780820151818401526020810190506104d9565b505050509050019250505060405180910390f35b610537610514846105d7565b61051d846105d7565b610526846105d7565b8761074c909392919063ffffffff16565b50505050565b610563610549826105d7565b6000801b84600101548561074c909392919063ffffffff16565b5050565b600060608260001c901c9050919050565b6000826003016000610589846105d7565b815260200190815260200160002060020160009054906101000a900460ff16905092915050565b60008060008060018060016000839350829250819150809050935093509350935090919293565b600060608273ffffffffffffffffffffffffffffffffffffffff16901b60001b9050919050565b6060806106148385610b9390919063ffffffff16565b90506060836040519080825280602002602001820160405280156106475781602001602082028038833980820191505090505b50905060008090505b848110156106d55761067483828151811061066757fe5b6020026020010151610567565b82828151811061068057fe5b602002602001019073ffffffffffffffffffffffffffffffffffffffff16908173ffffffffffffffffffffffffffffffffffffffff16815250506106ce600182610cb590919063ffffffff16565b9050610650565b50809250505092915050565b6107106106ed846105d7565b6106f6846105d7565b6106ff846105d7565b87610d3d909392919063ffffffff16565b50505050565b610731610722826105d7565b83610dfc90919063ffffffff16565b5050565b60606107458283600201546105fe565b9050919050565b6000801b8314156107c5576040517f08c379a00000000000000000000000000000000000000000000000000000000081526004018080602001828103825260138152602001807f4b6579206d75737420626520646566696e65640000000000000000000000000081525060200191505060405180910390fd5b6107cf8484610f99565b15610842576040517f08c379a00000000000000000000000000000000000000000000000000000000081526004018080602001828103825260208152602001807f43616e277420696e7365727420616e206578697374696e6720656c656d656e7481525060200191505060405180910390fd5b8282141580156108525750828114155b6108a7576040517f08c379a00000000000000000000000000000000000000000000000000000000081526004018080602001828103825260308152602001806110d46030913960400191505060405180910390fd5b6000846003016000858152602001908152602001600020905060018160020160006101000a81548160ff02191690831515021790555060008560020154141561090157838560010181905550838560000181905550610b6c565b6000801b8314158061091657506000801b8214155b61096b576040517f08c379a000000000000000000000000000000000000000000000000000000000815260040180806020018281038252602d81526020018061118f602d913960400191505060405180910390fd5b8281600001819055508181600101819055506000801b8314610a6a576109918584610f99565b6109e6576040517f08c379a000000000000000000000000000000000000000000000000000000000815260040180806020018281038252603481526020018061112b6034913960400191505060405180910390fd5b6000856003016000858152602001908152602001600020905082816001015414610a5b576040517f08c379a00000000000000000000000000000000000000000000000000000000081526004018080602001828103825260278152602001806111046027913960400191505060405180910390fd5b84816001018190555050610a74565b8385600101819055505b6000801b8214610b6157610a888583610f99565b610add576040517f08c379a000000000000000000000000000000000000000000000000000000000815260040180806020018281038252603081526020018061115f6030913960400191505060405180910390fd5b6000856003016000848152602001908152602001600020905083816000015414610b52576040517f08c379a00000000000000000000000000000000000000000000000000000000081526004018080602001828103825260278152602001806111046027913960400191505060405180910390fd5b84816000018190555050610b6b565b8385600001819055505b5b610b8460018660020154610cb590919063ffffffff16565b85600201819055505050505050565b60608260020154821115610c0f576040517f08c379a00000000000000000000000000000000000000000000000000000000081526004018080602001828103825260138152602001807f6e6f7420656e6f75676820656c656d656e74730000000000000000000000000081525060200191505060405180910390fd5b606082604051908082528060200260200182016040528015610c405781602001602082028038833980820191505090505b50905060008460000154905060008090505b84811015610ca95781838281518110610c6757fe5b602002602001018181525050856003016000838152602001908152602001600020600001549150610ca2600182610cb590919063ffffffff16565b9050610c52565b50819250505092915050565b600080828401905083811015610d33576040517f08c379a000000000000000000000000000000000000000000000000000000000815260040180806020018281038252601b8152602001807f536166654d6174683a206164646974696f6e206f766572666c6f77000000000081525060200191505060405180910390fd5b8091505092915050565b6000801b8314158015610d505750818314155b8015610d5c5750808314155b8015610d6e5750610d6d8484610f99565b5b610de0576040517f08c379a000000000000000000000000000000000000000000000000000000000815260040180806020018281038252600e8152602001807f6b6579206f6e20696e206c69737400000000000000000000000000000000000081525060200191505060405180910390fd5b610dea8484610dfc565b610df68484848461074c565b50505050565b600082600301600083815260200190815260200160002090506000801b8214158015610e2e5750610e2d8383610f99565b5b610ea0576040517f08c379a000000000000000000000000000000000000000000000000000000000815260040180806020018281038252600f8152602001807f6b6579206e6f7420696e206c697374000000000000000000000000000000000081525060200191505060405180910390fd5b6000801b816000015414610ede5760008360030160008360000154815260200190815260200160002090508160010154816001018190555050610eec565b806001015483600101819055505b6000801b816001015414610f2a5760008360030160008360010154815260200190815260200160002090508160000154816000018190555050610f38565b806000015483600001819055505b82600301600083815260200190815260200160002060008082016000905560018201600090556002820160006101000a81549060ff02191690555050610f8c60018460020154610fc990919063ffffffff16565b8360020181905550505050565b600082600301600083815260200190815260200160002060020160009054906101000a900460ff16905092915050565b600061100b83836040518060400160405280601e81526020017f536166654d6174683a207375627472616374696f6e206f766572666c6f770000815250611013565b905092915050565b60008383111582906110c0576040517f08c379a00000000000000000000000000000000000000000000000000000000081526004018080602001828103825283818151815260200191508051906020019080838360005b8381101561108557808201518184015260208101905061106a565b50505050905090810190601f1680156110b25780820380516001836020036101000a031916815260200191505b509250505060405180910390fd5b506000838503905080915050939250505056fe4b65792063616e6e6f74206265207468652073616d652061732070726576696f75734b6579206f72206e6578744b657970726576696f75734b6579206d7573742062652061646a6163656e7420746f206e6578744b657949662070726576696f75734b657920697320646566696e65642c206974206d75737420657869737420696e20746865206c6973744966206e6578744b657920697320646566696e65642c206974206d75737420657869737420696e20746865206c6973744569746865722070726576696f75734b6579206f72206e6578744b6579206d75737420626520646566696e6564a265627a7a72315820a2a83b72344173a79eacc514047d838f4e8379a8536a4c6f01049b9bfeeb57ca64736f6c634300050d0032"),
					Balance: big.NewInt(0),
				},
				common.HexToAddress("000000000000000000000000000000000000a007"): {
					Code:    []byte("0x73000000000000000000000000000000000000000030146080604052600436106100be5760003560e01c806354255be01161007b57806354255be014610370578063593b79fe146103a357806369b317e3146103fb578063cab455ae146104c6578063dcb2a4dd1461056b578063e0fe44b3146105f8576100be565b806302f13028146100c357806328135929146101295780632dedbbf014610184578063341f6623146102295780633a72e8021461029757806342b6351a1461031a575b600080fd5b61010f600480360360408110156100d957600080fd5b8101908080359060200190929190803573ffffffffffffffffffffffffffffffffffffffff16906020019092919050505061065a565b604051808215151515815260200191505060405180910390f35b81801561013557600080fd5b506101826004803603604081101561014c57600080fd5b8101908080359060200190929190803573ffffffffffffffffffffffffffffffffffffffff16906020019092919050505061067f565b005b81801561019057600080fd5b50610227600480360360a08110156101a757600080fd5b8101908080359060200190929190803573ffffffffffffffffffffffffffffffffffffffff16906020019092919080359060200190929190803573ffffffffffffffffffffffffffffffffffffffff169060200190929190803573ffffffffffffffffffffffffffffffffffffffff16906020019092919050505061069e565b005b6102556004803603602081101561023f57600080fd5b81019080803590602001909291905050506106d6565b604051808273ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200191505060405180910390f35b6102c3600480360360208110156102ad57600080fd5b81019080803590602001909291905050506106e7565b6040518080602001828103825283818151815260200191508051906020019060200280838360005b838110156103065780820151818401526020810190506102eb565b505050509050019250505060405180910390f35b61035a6004803603606081101561033057600080fd5b81019080803590602001909291908035906020019092919080359060200190929190505050610701565b6040518082815260200191505060405180910390f35b61037861079d565b6040518085815260200184815260200183815260200182815260200194505050505060405180910390f35b6103e5600480360360208110156103b957600080fd5b81019080803573ffffffffffffffffffffffffffffffffffffffff1690602001909291905050506107c4565b6040518082815260200191505060405180910390f35b6104276004803603602081101561041157600080fd5b81019080803590602001909291905050506107eb565b604051808060200180602001838103835285818151815260200191508051906020019060200280838360005b8381101561046e578082015181840152602081019050610453565b50505050905001838103825284818151815260200191508051906020019060200280838360005b838110156104b0578082015181840152602081019050610495565b5050505090500194505050505060405180910390f35b8180156104d257600080fd5b50610569600480360360a08110156104e957600080fd5b8101908080359060200190929190803573ffffffffffffffffffffffffffffffffffffffff16906020019092919080359060200190929190803573ffffffffffffffffffffffffffffffffffffffff169060200190929190803573ffffffffffffffffffffffffffffffffffffffff169060200190929190505050610941565b005b6105a16004803603604081101561058157600080fd5b810190808035906020019092919080359060200190929190505050610979565b6040518080602001828103825283818151815260200191508051906020019060200280838360005b838110156105e45780820151818401526020810190506105c9565b505050509050019250505060405180910390f35b6106446004803603604081101561060e57600080fd5b8101908080359060200190929190803573ffffffffffffffffffffffffffffffffffffffff169060200190929190505050610a5c565b6040518082815260200191505060405180910390f35b6000610677610668836107c4565b84610a8190919063ffffffff16565b905092915050565b61069a61068b826107c4565b83610aa190919063ffffffff16565b5050565b6106cf6106aa856107c4565b846106b4856107c4565b6106bd856107c4565b89610ad690949392919063ffffffff16565b5050505050565b600060608260001c901c9050919050565b60606106fa828360000160020154610979565b9050919050565b600080610715838660000160020154610d74565b905060008560000160000154905060008090505b8281101561078f57856107458389610d8d90919063ffffffff16565b101561075657809350505050610796565b866000016003016000838152602001908152602001600020600001549150610788600182610dad90919063ffffffff16565b9050610729565b5081925050505b9392505050565b60008060008060018060016000839350829250819150809050935093509350935090919293565b600060608273ffffffffffffffffffffffffffffffffffffffff16901b60001b9050919050565b60608060606107f984610e35565b90506060815160405190808252806020026020018201604052801561082d5781602001602082028038833980820191505090505b509050606082516040519080825280602002602001820160405280156108625781602001602082028038833980820191505090505b50905060008090505b83518110156109325761089084828151811061088357fe5b60200260200101516106d6565b83828151811061089c57fe5b602002602001019073ffffffffffffffffffffffffffffffffffffffff16908173ffffffffffffffffffffffffffffffffffffffff16815250508660040160008583815181106108e857fe5b602002602001015181526020019081526020016000205482828151811061090b57fe5b60200260200101818152505061092b600182610dad90919063ffffffff16565b905061086b565b50818194509450505050915091565b61097261094d856107c4565b84610957856107c4565b610960856107c4565b89610e4a90949392919063ffffffff16565b5050505050565b60608061098f8385610e6890919063ffffffff16565b90506060836040519080825280602002602001820160405280156109c25781602001602082028038833980820191505090505b50905060008090505b84811015610a50576109ef8382815181106109e257fe5b60200260200101516106d6565b8282815181106109fb57fe5b602002602001019073ffffffffffffffffffffffffffffffffffffffff16908173ffffffffffffffffffffffffffffffffffffffff1681525050610a49600182610dad90919063ffffffff16565b90506109cb565b50809250505092915050565b6000610a79610a6a836107c4565b84610d8d90919063ffffffff16565b905092915050565b6000610a998284600001610e8890919063ffffffff16565b905092915050565b610ab78183600001610eb890919063ffffffff16565b6000826004016000838152602001908152602001600020819055505050565b6000801b8414158015610ae95750818414155b8015610af55750808414155b8015610b085750610b068585610a81565b155b610b7a576040517f08c379a000000000000000000000000000000000000000000000000000000000815260040180806020018281038252600b8152602001807f696e76616c6964206b657900000000000000000000000000000000000000000081525060200191505060405180910390fd5b6000801b82141580610b8f57506000801b8114155b80610ba1575060008560000160020154145b610c13576040517f08c379a000000000000000000000000000000000000000000000000000000000815260040180806020018281038252601b8152602001807f6772656174657220616e64206c6573736572206b6579207a65726f000000000081525060200191505060405180910390fd5b610c1d8583610a81565b80610c2a57506000801b82145b610c9c576040517f08c379a00000000000000000000000000000000000000000000000000000000081526004018080602001828103825260128152602001807f696e76616c6964206c6573736572206b6579000000000000000000000000000081525060200191505060405180910390fd5b610ca68582610a81565b80610cb357506000801b81145b610d25576040517f08c379a00000000000000000000000000000000000000000000000000000000081526004018080602001828103825260138152602001807f696e76616c69642067726561746572206b65790000000000000000000000000081525060200191505060405180910390fd5b610d3185848484611055565b8092508193505050610d5384838388600001611208909392919063ffffffff16565b82856004016000868152602001908152602001600020819055505050505050565b6000818310610d835781610d85565b825b905092915050565b600082600401600083815260200190815260200160002054905092915050565b600080828401905083811015610e2b576040517f08c379a000000000000000000000000000000000000000000000000000000000815260040180806020018281038252601b8152602001807f536166654d6174683a206164646974696f6e206f766572666c6f77000000000081525060200191505060405180910390fd5b8091505092915050565b6060610e438260000161164f565b9050919050565b610e548585610aa1565b610e618585858585610ad6565b5050505050565b6060610e80828460000161166690919063ffffffff16565b905092915050565b600082600301600083815260200190815260200160002060020160009054906101000a900460ff16905092915050565b600082600301600083815260200190815260200160002090506000801b8214158015610eea5750610ee98383610e88565b5b610f5c576040517f08c379a000000000000000000000000000000000000000000000000000000000815260040180806020018281038252600f8152602001807f6b6579206e6f7420696e206c697374000000000000000000000000000000000081525060200191505060405180910390fd5b6000801b816000015414610f9a5760008360030160008360000154815260200190815260200160002090508160010154816001018190555050610fa8565b806001015483600101819055505b6000801b816001015414610fe65760008360030160008360010154815260200190815260200160002090508160000154816000018190555050610ff4565b806000015483600001819055505b82600301600083815260200190815260200160002060008082016000905560018201600090556002820160006101000a81549060ff021916905550506110486001846002015461178890919063ffffffff16565b8360020181905550505050565b6000806000801b84148015611079575061107886868689600001600101546117d2565b5b1561109057838660000160010154915091506111ff565b6000801b831480156110b157506110b086868860000160000154866117d2565b5b156110c857856000016000015483915091506111ff565b6000801b84141580156110fe57506110fd868686896000016003016000898152602001908152602001600020600101546117d2565b5b15611129578386600001600301600086815260200190815260200160002060010154915091506111ff565b6000801b831415801561115f575061115e868688600001600301600087815260200190815260200160002060000154866117d2565b5b1561118a578560000160030160008481526020019081526020016000206000015483915091506111ff565b60006111fe576040517f08c379a000000000000000000000000000000000000000000000000000000000815260040180806020018281038252601e8152602001807f676574206c657373657220616e642067726561746572206661696c757265000081525060200191505060405180910390fd5b5b94509492505050565b6000801b831415611281576040517f08c379a00000000000000000000000000000000000000000000000000000000081526004018080602001828103825260138152602001807f4b6579206d75737420626520646566696e65640000000000000000000000000081525060200191505060405180910390fd5b61128b8484610e88565b156112fe576040517f08c379a00000000000000000000000000000000000000000000000000000000081526004018080602001828103825260208152602001807f43616e277420696e7365727420616e206578697374696e6720656c656d656e7481525060200191505060405180910390fd5b82821415801561130e5750828114155b611363576040517f08c379a00000000000000000000000000000000000000000000000000000000081526004018080602001828103825260308152602001806118fe6030913960400191505060405180910390fd5b6000846003016000858152602001908152602001600020905060018160020160006101000a81548160ff0219169083151502179055506000856002015414156113bd57838560010181905550838560000181905550611628565b6000801b831415806113d257506000801b8214155b611427576040517f08c379a000000000000000000000000000000000000000000000000000000000815260040180806020018281038252602d8152602001806119b9602d913960400191505060405180910390fd5b8281600001819055508181600101819055506000801b83146115265761144d8584610e88565b6114a2576040517f08c379a00000000000000000000000000000000000000000000000000000000081526004018080602001828103825260348152602001806119556034913960400191505060405180910390fd5b6000856003016000858152602001908152602001600020905082816001015414611517576040517f08c379a000000000000000000000000000000000000000000000000000000000815260040180806020018281038252602781526020018061192e6027913960400191505060405180910390fd5b84816001018190555050611530565b8385600101819055505b6000801b821461161d576115448583610e88565b611599576040517f08c379a00000000000000000000000000000000000000000000000000000000081526004018080602001828103825260308152602001806119896030913960400191505060405180910390fd5b600085600301600084815260200190815260200160002090508381600001541461160e576040517f08c379a000000000000000000000000000000000000000000000000000000000815260040180806020018281038252602781526020018061192e6027913960400191505060405180910390fd5b84816000018190555050611627565b8385600001819055505b5b61164060018660020154610dad90919063ffffffff16565b85600201819055505050505050565b606061165f828360020154611666565b9050919050565b606082600201548211156116e2576040517f08c379a00000000000000000000000000000000000000000000000000000000081526004018080602001828103825260138152602001807f6e6f7420656e6f75676820656c656d656e74730000000000000000000000000081525060200191505060405180910390fd5b6060826040519080825280602002602001820160405280156117135781602001602082028038833980820191505090505b50905060008460000154905060008090505b8481101561177c578183828151811061173a57fe5b602002602001018181525050856003016000838152602001908152602001600020600001549150611775600182610dad90919063ffffffff16565b9050611725565b50819250505092915050565b60006117ca83836040518060400160405280601e81526020017f536166654d6174683a207375627472616374696f6e206f766572666c6f77000081525061183d565b905092915050565b6000806000801b8414806117fb5750848660040160008681526020019081526020016000205411155b905060008060001b8414806118255750858760040160008681526020019081526020016000205410155b90508180156118315750805b92505050949350505050565b60008383111582906118ea576040517f08c379a00000000000000000000000000000000000000000000000000000000081526004018080602001828103825283818151815260200191508051906020019080838360005b838110156118af578082015181840152602081019050611894565b50505050905090810190601f1680156118dc5780820380516001836020036101000a031916815260200191505b509250505060405180910390fd5b506000838503905080915050939250505056fe4b65792063616e6e6f74206265207468652073616d652061732070726576696f75734b6579206f72206e6578744b657970726576696f75734b6579206d7573742062652061646a6163656e7420746f206e6578744b657949662070726576696f75734b657920697320646566696e65642c206974206d75737420657869737420696e20746865206c6973744966206e6578744b657920697320646566696e65642c206974206d75737420657869737420696e20746865206c6973744569746865722070726576696f75734b6579206f72206e6578744b6579206d75737420626520646566696e6564a265627a7a72315820166b586ab824f2710adc96c360d8f787d26babdb2eace5d293353700814f603164736f6c634300050d0032"),
					Balance: big.NewInt(0),
				},
				common.HexToAddress("000000000000000000000000000000000000a008"): {
					Code:    []byte("0x730000000000000000000000000000000000000000301460806040526004361061009d5760003560e01c806377b024791161007057806377b024791461024f578063b4bd30b5146102e9578063bfc516381461034c578063d7a8acc11461039c578063eed5f7be146103e15761009d565b8063239491ba146100a257806354255be01461010557806369b317e3146101385780637577759914610203575b600080fd5b8180156100ae57600080fd5b50610103600480360360a08110156100c557600080fd5b810190808035906020019092919080359060200190929190803590602001909291908035906020019092919080359060200190929190505050610426565b005b61010d61044f565b6040518085815260200184815260200183815260200182815260200194505050505060405180910390f35b6101646004803603602081101561014e57600080fd5b8101908080359060200190929190505050610476565b604051808060200180602001838103835285818151815260200191508051906020019060200280838360005b838110156101ab578082015181840152602081019050610190565b50505050905001838103825284818151815260200191508051906020019060200280838360005b838110156101ed5780820151818401526020810190506101d2565b5050505090500194505050505060405180910390f35b6102396004803603604081101561021957600080fd5b810190808035906020019092919080359060200190929190505050610599565b6040518082815260200191505060405180910390f35b81801561025b57600080fd5b506102926004803603604081101561027257600080fd5b8101908080359060200190929190803590602001909291905050506105b9565b6040518080602001828103825283818151815260200191508051906020019060200280838360005b838110156102d55780820151818401526020810190506102ba565b505050509050019250505060405180910390f35b8180156102f557600080fd5b5061034a600480360360a081101561030c57600080fd5b81019080803590602001909291908035906020019092919080359060200190929190803590602001909291908035906020019092919050505061066b565b005b6103826004803603604081101561036257600080fd5b810190808035906020019092919080359060200190929190505050610694565b604051808215151515815260200191505060405180910390f35b8180156103a857600080fd5b506103df600480360360408110156103bf57600080fd5b8101908080359060200190929190803590602001909291905050506106b4565b005b8180156103ed57600080fd5b506104246004803603604081101561040457600080fd5b8101908080359060200190929190803590602001909291905050506106ce565b005b6104488460001b848460001b8460001b896106e890949392919063ffffffff16565b5050505050565b60008060008060018060016000839350829250819150809050935093509350935090919293565b606080606061048484610706565b9050606081516040519080825280602002602001820160405280156104b85781602001602082028038833980820191505090505b509050606082516040519080825280602002602001820160405280156104ed5781602001602082028038833980820191505090505b50905060008090505b835181101561058a5783818151811061050b57fe5b602002602001015160001c83828151811061052257fe5b60200260200101818152505086600401600085838151811061054057fe5b602002602001015181526020019081526020016000205482828151811061056357fe5b60200260200101818152505061058360018261071b90919063ffffffff16565b90506104f6565b50818194509450505050915091565b60006105b18260001b846107a390919063ffffffff16565b905092915050565b6060806105cf83856107c390919063ffffffff16565b9050606081516040519080825280602002602001820160405280156106035781602001602082028038833980820191505090505b50905060008090505b825181101561065f5782818151811061062157fe5b602002602001015160001c82828151811061063857fe5b60200260200101818152505061065860018261071b90919063ffffffff16565b905061060c565b50809250505092915050565b61068d8460001b848460001b8460001b896108da90949392919063ffffffff16565b5050505050565b60006106ac8260001b84610b7890919063ffffffff16565b905092915050565b6106ca8160001b83610b9890919063ffffffff16565b5050565b6106e48160001b83610bb490919063ffffffff16565b5050565b6106f28585610bb4565b6106ff85858585856108da565b5050505050565b606061071482600001610be9565b9050919050565b600080828401905083811015610799576040517f08c379a000000000000000000000000000000000000000000000000000000000815260040180806020018281038252601b8152602001807f536166654d6174683a206164646974696f6e206f766572666c6f77000000000081525060200191505060405180910390fd5b8091505092915050565b600082600401600083815260200190815260200160002054905092915050565b60608260000160020154821115610842576040517f08c379a00000000000000000000000000000000000000000000000000000000081526004018080602001828103825260138152602001807f6e6f7420656e6f75676820656c656d656e74730000000000000000000000000081525060200191505060405180910390fd5b6060826040519080825280602002602001820160405280156108735781602001602082028038833980820191505090505b50905060008090505b838110156108cf576000856000016000015490508083838151811061089d57fe5b6020026020010181815250506108b38682610bb4565b506108c860018261071b90919063ffffffff16565b905061087c565b508091505092915050565b6000801b84141580156108ed5750818414155b80156108f95750808414155b801561090c575061090a8585610b78565b155b61097e576040517f08c379a000000000000000000000000000000000000000000000000000000000815260040180806020018281038252600b8152602001807f696e76616c6964206b657900000000000000000000000000000000000000000081525060200191505060405180910390fd5b6000801b8214158061099357506000801b8114155b806109a5575060008560000160020154145b610a17576040517f08c379a000000000000000000000000000000000000000000000000000000000815260040180806020018281038252601b8152602001807f6772656174657220616e64206c6573736572206b6579207a65726f000000000081525060200191505060405180910390fd5b610a218583610b78565b80610a2e57506000801b82145b610aa0576040517f08c379a00000000000000000000000000000000000000000000000000000000081526004018080602001828103825260128152602001807f696e76616c6964206c6573736572206b6579000000000000000000000000000081525060200191505060405180910390fd5b610aaa8582610b78565b80610ab757506000801b81145b610b29576040517f08c379a00000000000000000000000000000000000000000000000000000000081526004018080602001828103825260138152602001807f696e76616c69642067726561746572206b65790000000000000000000000000081525060200191505060405180910390fd5b610b3585848484610c00565b8092508193505050610b5784838388600001610db3909392919063ffffffff16565b82856004016000868152602001908152602001600020819055505050505050565b6000610b9082846000016111fa90919063ffffffff16565b905092915050565b610bb0828260008060001b86600001600101546108da565b5050565b610bca818360000161122a90919063ffffffff16565b6000826004016000838152602001908152602001600020819055505050565b6060610bf98283600201546113c7565b9050919050565b6000806000801b84148015610c245750610c2386868689600001600101546114e9565b5b15610c3b5783866000016001015491509150610daa565b6000801b83148015610c5c5750610c5b86868860000160000154866114e9565b5b15610c735785600001600001548391509150610daa565b6000801b8414158015610ca95750610ca8868686896000016003016000898152602001908152602001600020600101546114e9565b5b15610cd457838660000160030160008681526020019081526020016000206001015491509150610daa565b6000801b8314158015610d0a5750610d09868688600001600301600087815260200190815260200160002060000154866114e9565b5b15610d3557856000016003016000848152602001908152602001600020600001548391509150610daa565b6000610da9576040517f08c379a000000000000000000000000000000000000000000000000000000000815260040180806020018281038252601e8152602001807f676574206c657373657220616e642067726561746572206661696c757265000081525060200191505060405180910390fd5b5b94509492505050565b6000801b831415610e2c576040517f08c379a00000000000000000000000000000000000000000000000000000000081526004018080602001828103825260138152602001807f4b6579206d75737420626520646566696e65640000000000000000000000000081525060200191505060405180910390fd5b610e3684846111fa565b15610ea9576040517f08c379a00000000000000000000000000000000000000000000000000000000081526004018080602001828103825260208152602001807f43616e277420696e7365727420616e206578697374696e6720656c656d656e7481525060200191505060405180910390fd5b828214158015610eb95750828114155b610f0e576040517f08c379a000000000000000000000000000000000000000000000000000000000815260040180806020018281038252603081526020018061165f6030913960400191505060405180910390fd5b6000846003016000858152602001908152602001600020905060018160020160006101000a81548160ff021916908315150217905550600085600201541415610f68578385600101819055508385600001819055506111d3565b6000801b83141580610f7d57506000801b8214155b610fd2576040517f08c379a000000000000000000000000000000000000000000000000000000000815260040180806020018281038252602d81526020018061171a602d913960400191505060405180910390fd5b8281600001819055508181600101819055506000801b83146110d157610ff885846111fa565b61104d576040517f08c379a00000000000000000000000000000000000000000000000000000000081526004018080602001828103825260348152602001806116b66034913960400191505060405180910390fd5b60008560030160008581526020019081526020016000209050828160010154146110c2576040517f08c379a000000000000000000000000000000000000000000000000000000000815260040180806020018281038252602781526020018061168f6027913960400191505060405180910390fd5b848160010181905550506110db565b8385600101819055505b6000801b82146111c8576110ef85836111fa565b611144576040517f08c379a00000000000000000000000000000000000000000000000000000000081526004018080602001828103825260308152602001806116ea6030913960400191505060405180910390fd5b60008560030160008481526020019081526020016000209050838160000154146111b9576040517f08c379a000000000000000000000000000000000000000000000000000000000815260040180806020018281038252602781526020018061168f6027913960400191505060405180910390fd5b848160000181905550506111d2565b8385600001819055505b5b6111eb6001866002015461071b90919063ffffffff16565b85600201819055505050505050565b600082600301600083815260200190815260200160002060020160009054906101000a900460ff16905092915050565b600082600301600083815260200190815260200160002090506000801b821415801561125c575061125b83836111fa565b5b6112ce576040517f08c379a000000000000000000000000000000000000000000000000000000000815260040180806020018281038252600f8152602001807f6b6579206e6f7420696e206c697374000000000000000000000000000000000081525060200191505060405180910390fd5b6000801b81600001541461130c576000836003016000836000015481526020019081526020016000209050816001015481600101819055505061131a565b806001015483600101819055505b6000801b8160010154146113585760008360030160008360010154815260200190815260200160002090508160000154816000018190555050611366565b806000015483600001819055505b82600301600083815260200190815260200160002060008082016000905560018201600090556002820160006101000a81549060ff021916905550506113ba6001846002015461155490919063ffffffff16565b8360020181905550505050565b60608260020154821115611443576040517f08c379a00000000000000000000000000000000000000000000000000000000081526004018080602001828103825260138152602001807f6e6f7420656e6f75676820656c656d656e74730000000000000000000000000081525060200191505060405180910390fd5b6060826040519080825280602002602001820160405280156114745781602001602082028038833980820191505090505b50905060008460000154905060008090505b848110156114dd578183828151811061149b57fe5b6020026020010181815250508560030160008381526020019081526020016000206000015491506114d660018261071b90919063ffffffff16565b9050611486565b50819250505092915050565b6000806000801b8414806115125750848660040160008681526020019081526020016000205411155b905060008060001b84148061153c5750858760040160008681526020019081526020016000205410155b90508180156115485750805b92505050949350505050565b600061159683836040518060400160405280601e81526020017f536166654d6174683a207375627472616374696f6e206f766572666c6f77000081525061159e565b905092915050565b600083831115829061164b576040517f08c379a00000000000000000000000000000000000000000000000000000000081526004018080602001828103825283818151815260200191508051906020019080838360005b838110156116105780820151818401526020810190506115f5565b50505050905090810190601f16801561163d5780820380516001836020036101000a031916815260200191505b509250505060405180910390fd5b506000838503905080915050939250505056fe4b65792063616e6e6f74206265207468652073616d652061732070726576696f75734b6579206f72206e6578744b657970726576696f75734b6579206d7573742062652061646a6163656e7420746f206e6578744b657949662070726576696f75734b657920697320646566696e65642c206974206d75737420657869737420696e20746865206c6973744966206e6578744b657920697320646566696e65642c206974206d75737420657869737420696e20746865206c6973744569746865722070726576696f75734b6579206f72206e6578744b6579206d75737420626520646566696e6564a265627a7a7231582007402e6cba19826aaab252197e296c539e3f899caeee04498cd9e385d097de7c64736f6c634300050d0032"),
					Balance: big.NewInt(0),
				},
				common.HexToAddress("000000000000000000000000000000000000a009"): {
					Code:    []byte("0x73000000000000000000000000000000000000000030146080604052600436106100f45760003560e01c80636eafa6c31161009657806395073a791161007057806395073a791461056c578063c1e728e9146105d2578063d4a092721461062d578063d938ec7b146106d2576100f4565b80636eafa6c3146104235780637c6bb86214610465578063832a2147146104c7576100f4565b806354255be0116100d257806354255be014610243578063593b79fe1461027657806359d556a8146102ce5780636cfa387314610310576100f4565b80630944c594146100f95780633118159e14610167578063341f6623146101d5575b600080fd5b6101256004803603602081101561010f57600080fd5b8101908080359060200190929190505050610740565b604051808273ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200191505060405180910390f35b6101936004803603602081101561017d57600080fd5b810190808035906020019092919050505061075a565b604051808273ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200191505060405180910390f35b610201600480360360208110156101eb57600080fd5b8101908080359060200190929190505050610774565b604051808273ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200191505060405180910390f35b61024b610785565b6040518085815260200184815260200183815260200182815260200194505050505060405180910390f35b6102b86004803603602081101561028c57600080fd5b81019080803573ffffffffffffffffffffffffffffffffffffffff1690602001909291905050506107ac565b6040518082815260200191505060405180910390f35b6102fa600480360360208110156102e457600080fd5b81019080803590602001909291905050506107d3565b6040518082815260200191505060405180910390f35b61033c6004803603602081101561032657600080fd5b81019080803590602001909291905050506107f3565b60405180806020018060200180602001848103845287818151815260200191508051906020019060200280838360005b8381101561038757808201518184015260208101905061036c565b50505050905001848103835286818151815260200191508051906020019060200280838360005b838110156103c95780820151818401526020810190506103ae565b50505050905001848103825285818151815260200191508051906020019060200280838360005b8381101561040b5780820151818401526020810190506103f0565b50505050905001965050505050505060405180910390f35b61044f6004803603602081101561043957600080fd5b81019080803590602001909291905050506109e8565b6040518082815260200191505060405180910390f35b6104b16004803603604081101561047b57600080fd5b8101908080359060200190929190803573ffffffffffffffffffffffffffffffffffffffff1690602001909291905050506109fa565b6040518082815260200191505060405180910390f35b8180156104d357600080fd5b5061056a600480360360a08110156104ea57600080fd5b8101908080359060200190929190803573ffffffffffffffffffffffffffffffffffffffff16906020019092919080359060200190929190803573ffffffffffffffffffffffffffffffffffffffff169060200190929190803573ffffffffffffffffffffffffffffffffffffffff169060200190929190505050610a1f565b005b6105b86004803603604081101561058257600080fd5b8101908080359060200190929190803573ffffffffffffffffffffffffffffffffffffffff169060200190929190505050610a57565b604051808215151515815260200191505060405180910390f35b8180156105de57600080fd5b5061062b600480360360408110156105f557600080fd5b8101908080359060200190929190803573ffffffffffffffffffffffffffffffffffffffff169060200190929190505050610a7c565b005b81801561063957600080fd5b506106d0600480360360a081101561065057600080fd5b8101908080359060200190929190803573ffffffffffffffffffffffffffffffffffffffff16906020019092919080359060200190929190803573ffffffffffffffffffffffffffffffffffffffff169060200190929190803573ffffffffffffffffffffffffffffffffffffffff169060200190929190505050610a9b565b005b6106fe600480360360208110156106e857600080fd5b8101908080359060200190929190505050610ad3565b604051808273ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200191505060405180910390f35b600061075361074e83610aed565b610774565b9050919050565b600061076d61076883610b01565b610774565b9050919050565b600060608260001c901c9050919050565b60008060008060018060016000839350829250819150809050935093509350935090919293565b600060608273ffffffffffffffffffffffffffffffffffffffff16901b60001b9050919050565b60006107ec826005015483610b0f90919063ffffffff16565b9050919050565b60608060608061080285610b32565b9050606081516040519080825280602002602001820160405280156108365781602001602082028038833980820191505090505b5090506060825160405190808252806020026020018201604052801561086b5781602001602082028038833980820191505090505b509050606082516040519080825280602002602001820160405280156108a05781602001602082028038833980820191505090505b50905060008090505b84518110156109d3576108ce8582815181106108c157fe5b6020026020010151610774565b8482815181106108da57fe5b602002602001019073ffffffffffffffffffffffffffffffffffffffff16908173ffffffffffffffffffffffffffffffffffffffff168152505061093a85828151811061092357fe5b60200260200101518a610b0f90919063ffffffff16565b83828151811061094657fe5b60200260200101818152505088600601600086838151811061096457fe5b6020026020010151815260200190815260200160002060009054906101000a900460ff1682828151811061099457fe5b602002602001019060038111156109a757fe5b908160038111156109b457fe5b815250506109cc600182610b4790919063ffffffff16565b90506108a9565b50828282965096509650505050509193909250565b60006109f382610bcf565b9050919050565b6000610a17610a08836107ac565b84610b0f90919063ffffffff16565b905092915050565b610a50610a2b856107ac565b84610a35856107ac565b610a3e856107ac565b89610be390949392919063ffffffff16565b5050505050565b6000610a74610a65836107ac565b84610c0190919063ffffffff16565b905092915050565b610a97610a88826107ac565b83610c2190919063ffffffff16565b5050565b610acc610aa7856107ac565b84610ab1856107ac565b610aba856107ac565b89610da390949392919063ffffffff16565b5050505050565b6000610ae6610ae18361100e565b610774565b9050919050565b600081600001600001600001549050919050565b600081600501549050919050565b600082600001600401600083815260200190815260200160002054905092915050565b6060610b4082600001611022565b9050919050565b600080828401905083811015610bc5576040517f08c379a000000000000000000000000000000000000000000000000000000000815260040180806020018281038252601b8152602001807f536166654d6174683a206164646974696f6e206f766572666c6f77000000000081525060200191505060405180910390fd5b8091505092915050565b600081600001600001600201549050919050565b610bed8585610c21565b610bfa8585858585610da3565b5050505050565b6000610c19828460000161103790919063ffffffff16565b905092915050565b6000809050600083600001600001600201541415610c4a576000801b8360050181905550610d7e565b60006002846000016000016002015481610c6057fe5b061415610cf45760026003811115610c7457fe5b83600601600084815260200190815260200160002060009054906101000a900460ff166003811115610ca257fe5b1480610ce55750600380811115610cb557fe5b83600601600084815260200190815260200160002060009054906101000a900460ff166003811115610ce357fe5b145b15610cef57600190505b610d7d565b60016003811115610d0157fe5b83600601600084815260200190815260200160002060009054906101000a900460ff166003811115610d2f57fe5b1480610d725750600380811115610d4257fe5b83600601600084815260200190815260200160002060009054906101000a900460ff166003811115610d7057fe5b145b15610d7c57600290505b5b5b610d888382611057565b610d9e828460000161118e90919063ffffffff16565b505050565b610dbf84848484896000016111c390949392919063ffffffff16565b600085600001600001600301600086815260200190815260200160002090506000809050600187600001600001600201541415610e3b57858760050181905550600387600601600088815260200190815260200160002060006101000a81548160ff02191690836003811115610e3157fe5b0217905550610ffb565b60016002886000016000016002015481610e5157fe5b061415610f2b576000801b82600001541480610ea9575060016003811115610e7557fe5b8760060160008460000154815260200190815260200160002060009054906101000a900460ff166003811115610ea757fe5b145b15610eee5760019050600187600601600088815260200190815260200160002060006101000a81548160ff02191690836003811115610ee457fe5b0217905550610f26565b600287600601600088815260200190815260200160002060006101000a81548160ff02191690836003811115610f2057fe5b02179055505b610ffa565b6000801b82600101541480610f7c575060026003811115610f4857fe5b8760060160008460010154815260200190815260200160002060009054906101000a900460ff166003811115610f7a57fe5b145b15610fc15760029050600287600601600088815260200190815260200160002060006101000a81548160ff02191690836003811115610fb757fe5b0217905550610ff9565b600187600601600088815260200190815260200160002060006101000a81548160ff02191690836003811115610ff357fe5b02179055505b5b5b6110058782611057565b50505050505050565b600081600001600001600101549050919050565b606061103082600001611461565b9050919050565b600061104f828460000161147890919063ffffffff16565b905092915050565b60008260000160000160030160008460050154815260200190815260200160002090506001600281111561108757fe5b82600281111561109357fe5b14156110e65760028360060160008560050154815260200190815260200160002060006101000a81548160ff021916908360038111156110cf57fe5b02179055508060000154836005018190555061114e565b6002808111156110f257fe5b8260028111156110fe57fe5b141561114d5760018360060160008560050154815260200190815260200160002060006101000a81548160ff0219169083600381111561113a57fe5b0217905550806001015483600501819055505b5b60038360060160008560050154815260200190815260200160002060006101000a81548160ff0219169083600381111561118457fe5b0217905550505050565b6111a481836000016114a890919063ffffffff16565b6000826004016000838152602001908152602001600020819055505050565b6000801b84141580156111d65750818414155b80156111e25750808414155b80156111f557506111f38585611037565b155b611267576040517f08c379a000000000000000000000000000000000000000000000000000000000815260040180806020018281038252600b8152602001807f696e76616c6964206b657900000000000000000000000000000000000000000081525060200191505060405180910390fd5b6000801b8214158061127c57506000801b8114155b8061128e575060008560000160020154145b611300576040517f08c379a000000000000000000000000000000000000000000000000000000000815260040180806020018281038252601b8152602001807f6772656174657220616e64206c6573736572206b6579207a65726f000000000081525060200191505060405180910390fd5b61130a8583611037565b8061131757506000801b82145b611389576040517f08c379a00000000000000000000000000000000000000000000000000000000081526004018080602001828103825260128152602001807f696e76616c6964206c6573736572206b6579000000000000000000000000000081525060200191505060405180910390fd5b6113938582611037565b806113a057506000801b81145b611412576040517f08c379a00000000000000000000000000000000000000000000000000000000081526004018080602001828103825260138152602001807f696e76616c69642067726561746572206b65790000000000000000000000000081525060200191505060405180910390fd5b61141e85848484611645565b8092508193505050611440848383886000016117f8909392919063ffffffff16565b82856004016000868152602001908152602001600020819055505050505050565b6060611471828360020154611c3f565b9050919050565b600082600301600083815260200190815260200160002060020160009054906101000a900460ff16905092915050565b600082600301600083815260200190815260200160002090506000801b82141580156114da57506114d98383611478565b5b61154c576040517f08c379a000000000000000000000000000000000000000000000000000000000815260040180806020018281038252600f8152602001807f6b6579206e6f7420696e206c697374000000000000000000000000000000000081525060200191505060405180910390fd5b6000801b81600001541461158a5760008360030160008360000154815260200190815260200160002090508160010154816001018190555050611598565b806001015483600101819055505b6000801b8160010154146115d657600083600301600083600101548152602001908152602001600020905081600001548160000181905550506115e4565b806000015483600001819055505b82600301600083815260200190815260200160002060008082016000905560018201600090556002820160006101000a81549060ff0219169055505061163860018460020154611d6190919063ffffffff16565b8360020181905550505050565b6000806000801b8414801561166957506116688686868960000160010154611dab565b5b1561168057838660000160010154915091506117ef565b6000801b831480156116a157506116a08686886000016000015486611dab565b5b156116b857856000016000015483915091506117ef565b6000801b84141580156116ee57506116ed86868689600001600301600089815260200190815260200160002060010154611dab565b5b15611719578386600001600301600086815260200190815260200160002060010154915091506117ef565b6000801b831415801561174f575061174e86868860000160030160008781526020019081526020016000206000015486611dab565b5b1561177a578560000160030160008481526020019081526020016000206000015483915091506117ef565b60006117ee576040517f08c379a000000000000000000000000000000000000000000000000000000000815260040180806020018281038252601e8152602001807f676574206c657373657220616e642067726561746572206661696c757265000081525060200191505060405180910390fd5b5b94509492505050565b6000801b831415611871576040517f08c379a00000000000000000000000000000000000000000000000000000000081526004018080602001828103825260138152602001807f4b6579206d75737420626520646566696e65640000000000000000000000000081525060200191505060405180910390fd5b61187b8484611478565b156118ee576040517f08c379a00000000000000000000000000000000000000000000000000000000081526004018080602001828103825260208152602001807f43616e277420696e7365727420616e206578697374696e6720656c656d656e7481525060200191505060405180910390fd5b8282141580156118fe5750828114155b611953576040517f08c379a0000000000000000000000000000000000000000000000000000000008152600401808060200182810382526030815260200180611ed76030913960400191505060405180910390fd5b6000846003016000858152602001908152602001600020905060018160020160006101000a81548160ff0219169083151502179055506000856002015414156119ad57838560010181905550838560000181905550611c18565b6000801b831415806119c257506000801b8214155b611a17576040517f08c379a000000000000000000000000000000000000000000000000000000000815260040180806020018281038252602d815260200180611f92602d913960400191505060405180910390fd5b8281600001819055508181600101819055506000801b8314611b1657611a3d8584611478565b611a92576040517f08c379a0000000000000000000000000000000000000000000000000000000008152600401808060200182810382526034815260200180611f2e6034913960400191505060405180910390fd5b6000856003016000858152602001908152602001600020905082816001015414611b07576040517f08c379a0000000000000000000000000000000000000000000000000000000008152600401808060200182810382526027815260200180611f076027913960400191505060405180910390fd5b84816001018190555050611b20565b8385600101819055505b6000801b8214611c0d57611b348583611478565b611b89576040517f08c379a0000000000000000000000000000000000000000000000000000000008152600401808060200182810382526030815260200180611f626030913960400191505060405180910390fd5b6000856003016000848152602001908152602001600020905083816000015414611bfe576040517f08c379a0000000000000000000000000000000000000000000000000000000008152600401808060200182810382526027815260200180611f076027913960400191505060405180910390fd5b84816000018190555050611c17565b8385600001819055505b5b611c3060018660020154610b4790919063ffffffff16565b85600201819055505050505050565b60608260020154821115611cbb576040517f08c379a00000000000000000000000000000000000000000000000000000000081526004018080602001828103825260138152602001807f6e6f7420656e6f75676820656c656d656e74730000000000000000000000000081525060200191505060405180910390fd5b606082604051908082528060200260200182016040528015611cec5781602001602082028038833980820191505090505b50905060008460000154905060008090505b84811015611d555781838281518110611d1357fe5b602002602001018181525050856003016000838152602001908152602001600020600001549150611d4e600182610b4790919063ffffffff16565b9050611cfe565b50819250505092915050565b6000611da383836040518060400160405280601e81526020017f536166654d6174683a207375627472616374696f6e206f766572666c6f770000815250611e16565b905092915050565b6000806000801b841480611dd45750848660040160008681526020019081526020016000205411155b905060008060001b841480611dfe5750858760040160008681526020019081526020016000205410155b9050818015611e0a5750805b92505050949350505050565b6000838311158290611ec3576040517f08c379a00000000000000000000000000000000000000000000000000000000081526004018080602001828103825283818151815260200191508051906020019080838360005b83811015611e88578082015181840152602081019050611e6d565b50505050905090810190601f168015611eb55780820380516001836020036101000a031916815260200191505b509250505060405180910390fd5b506000838503905080915050939250505056fe4b65792063616e6e6f74206265207468652073616d652061732070726576696f75734b6579206f72206e6578744b657970726576696f75734b6579206d7573742062652061646a6163656e7420746f206e6578744b657949662070726576696f75734b657920697320646566696e65642c206974206d75737420657869737420696e20746865206c6973744966206e6578744b657920697320646566696e65642c206974206d75737420657869737420696e20746865206c6973744569746865722070726576696f75734b6579206f72206e6578744b6579206d75737420626520646566696e6564a265627a7a723158208db24506c31ec89432fb0d14e748c901256e3fc9fa059266cbbcc8e6d6c2515864736f6c634300050d0032"),
					Balance: big.NewInt(0),
				},
				common.HexToAddress("000000000000000000000000000000000000a010"): {
					Code:    []byte("0x73000000000000000000000000000000000000000030146080604052600436106100615760003560e01c806334d1a2331461006657806354255be0146100ff578063683317091461013257806396ef41a11461017e578063b3abdb0c14610223575b600080fd5b6100bd600480360360a081101561007c57600080fd5b810190808035906020019092919080359060200190929190803560ff16906020019092919080359060200190929190803590602001909291905050506102b2565b604051808273ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200191505060405180910390f35b610107610324565b6040518085815260200184815260200183815260200182815260200194505050505060405180910390f35b6101686004803603604081101561014857600080fd5b81019080803590602001909291908035906020019092919050505061034b565b6040518082815260200191505060405180910390f35b6101e16004803603608081101561019457600080fd5b81019080803573ffffffffffffffffffffffffffffffffffffffff169060200190929190803560ff16906020019092919080359060200190929190803590602001909291905050506103ac565b604051808273ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200191505060405180910390f35b6102706004803603608081101561023957600080fd5b8101908080359060200190929190803560ff169060200190929190803590602001909291908035906020019092919050505061041e565b604051808273ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200191505060405180910390f35b6000606060416040519080825280601f01601f1916602001820160405280156102ea5781602001600182028038833980820191505090505b509050836020820152826040820152846060820153600061030b888861034b565b9050610317818361048e565b9250505095945050505050565b60008060008060018060026000839350829250819150809050935093509350935090919293565b6000828260405160200180807f19010000000000000000000000000000000000000000000000000000000000008152506002018381526020018281526020019250505060405160208183030381529060405280519060200120905092915050565b60008085604051602001808273ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff1660601b81526014019150506040516020818303038152906040528051906020012090506104138186868661041e565b915050949350505050565b6000606060416040519080825280601f01601f1916602001820160405280156104565781602001600182028038833980820191505090505b509050836020820152826040820152846060820153600061047687610592565b9050610482818361048e565b92505050949350505050565b600060418251146104a2576000905061058c565b60008060006020850151925060408501519150606085015160001a90507f7fffffffffffffffffffffffffffffff5d576e7357a4501ddfe92f46681b20a08260001c11156104f6576000935050505061058c565b601b8160ff161415801561050e5750601c8160ff1614155b1561051f576000935050505061058c565b60018682858560405160008152602001604052604051808581526020018460ff1660ff1681526020018381526020018281526020019450505050506020604051602081039080840390855afa15801561057c573d6000803e3d6000fd5b5050506020604051035193505050505b92915050565b60008160405160200180807f19457468657265756d205369676e6564204d6573736167653a0a333200000000815250601c0182815260200191505060405160208183030381529060405280519060200120905091905056fea265627a7a72315820b53824bfd445c7dd50399a7e96deb3b57975b4bf8578ff1f640de3d6bcb603dd64736f6c634300050d0032"),
					Balance: big.NewInt(0),
				},
				common.HexToAddress("000000000000000000000000000000000000a051"): {
					Code:    []byte("0x73000000000000000000000000000000000000000030146080604052600080fdfea265627a7a72315820c59715602a39865a36c8a0ffab6c61daaad54b84fd14c35084f951098ffe752a64736f6c634300050d0032"),
					Balance: big.NewInt(0),
				},
				common.HexToAddress("0xce10"): {
					Code: []byte("0x60806040526004361061004a5760003560e01c806303386ba3146101e757806342404e0714610280578063bb913f41146102d7578063d29d44ee14610328578063f7e6af8014610379575b6000600160405180807f656970313936372e70726f78792e696d706c656d656e746174696f6e00000000815250601c019050604051809103902060001c0360001b9050600081549050600073ffffffffffffffffffffffffffffffffffffffff168173ffffffffffffffffffffffffffffffffffffffff161415610136576040517f08c379a00000000000000000000000000000000000000000000000000000000081526004018080602001828103825260158152602001807f4e6f20496d706c656d656e746174696f6e20736574000000000000000000000081525060200191505060405180910390fd5b61013f816103d0565b6101b1576040517f08c379a00000000000000000000000000000000000000000000000000000000081526004018080602001828103825260188152602001807f496e76616c696420636f6e74726163742061646472657373000000000000000081525060200191505060405180910390fd5b60405136810160405236600082376000803683855af43d604051818101604052816000823e82600081146101e3578282f35b8282fd5b61027e600480360360408110156101fd57600080fd5b81019080803573ffffffffffffffffffffffffffffffffffffffff1690602001909291908035906020019064010000000081111561023a57600080fd5b82018360208201111561024c57600080fd5b8035906020019184600183028401116401000000008311171561026e57600080fd5b909192939192939050505061041b565b005b34801561028c57600080fd5b506102956105c1565b604051808273ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200191505060405180910390f35b3480156102e357600080fd5b50610326600480360360208110156102fa57600080fd5b81019080803573ffffffffffffffffffffffffffffffffffffffff16906020019092919050505061060d565b005b34801561033457600080fd5b506103776004803603602081101561034b57600080fd5b81019080803573ffffffffffffffffffffffffffffffffffffffff1690602001909291905050506107bd565b005b34801561038557600080fd5b5061038e610871565b604051808273ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200191505060405180910390f35b60008060007fc5d2460186f7233c927e7db2dcc703c0e500b653ca82273b7bfad8045d85a47060001b9050833f915080821415801561041257506000801b8214155b92505050919050565b610423610871565b73ffffffffffffffffffffffffffffffffffffffff163373ffffffffffffffffffffffffffffffffffffffff16146104c3576040517f08c379a00000000000000000000000000000000000000000000000000000000081526004018080602001828103825260148152602001807f73656e64657220776173206e6f74206f776e657200000000000000000000000081525060200191505060405180910390fd5b6104cc8361060d565b600060608473ffffffffffffffffffffffffffffffffffffffff168484604051808383808284378083019250505092505050600060405180830381855af49150503d8060008114610539576040519150601f19603f3d011682016040523d82523d6000602084013e61053e565b606091505b508092508193505050816105ba576040517f08c379a000000000000000000000000000000000000000000000000000000000815260040180806020018281038252601e8152602001807f696e697469616c697a6174696f6e2063616c6c6261636b206661696c6564000081525060200191505060405180910390fd5b5050505050565b600080600160405180807f656970313936372e70726f78792e696d706c656d656e746174696f6e00000000815250601c019050604051809103902060001c0360001b9050805491505090565b610615610871565b73ffffffffffffffffffffffffffffffffffffffff163373ffffffffffffffffffffffffffffffffffffffff16146106b5576040517f08c379a00000000000000000000000000000000000000000000000000000000081526004018080602001828103825260148152602001807f73656e64657220776173206e6f74206f776e657200000000000000000000000081525060200191505060405180910390fd5b6000600160405180807f656970313936372e70726f78792e696d706c656d656e746174696f6e00000000815250601c019050604051809103902060001c0360001b9050610701826103d0565b610773576040517f08c379a00000000000000000000000000000000000000000000000000000000081526004018080602001828103825260188152602001807f496e76616c696420636f6e74726163742061646472657373000000000000000081525060200191505060405180910390fd5b8181558173ffffffffffffffffffffffffffffffffffffffff167fab64f92ab780ecbf4f3866f57cee465ff36c89450dcce20237ca7a8d81fb7d1360405160405180910390a25050565b6107c5610871565b73ffffffffffffffffffffffffffffffffffffffff163373ffffffffffffffffffffffffffffffffffffffff1614610865576040517f08c379a00000000000000000000000000000000000000000000000000000000081526004018080602001828103825260148152602001807f73656e64657220776173206e6f74206f776e657200000000000000000000000081525060200191505060405180910390fd5b61086e816108bd565b50565b600080600160405180807f656970313936372e70726f78792e61646d696e000000000000000000000000008152506013019050604051809103902060001c0360001b9050805491505090565b600073ffffffffffffffffffffffffffffffffffffffff168173ffffffffffffffffffffffffffffffffffffffff161415610960576040517f08c379a00000000000000000000000000000000000000000000000000000000081526004018080602001828103825260118152602001807f6f776e65722063616e6e6f74206265203000000000000000000000000000000081525060200191505060405180910390fd5b6000600160405180807f656970313936372e70726f78792e61646d696e000000000000000000000000008152506013019050604051809103902060001c0360001b90508181558173ffffffffffffffffffffffffffffffffffffffff167f50146d0e3c60aa1d17a70635b05494f864e86144a2201275021014fbf08bafe260405160405180910390a2505056fea165627a7a72305820f4f741dbef8c566cb1690ae708b8ef1113bdb503225629cc1f9e86bd47efd1a40029"),
					Storage: map[common.Hash]common.Hash{
						common.HexToHash("0x0000000000000000000000000000000000000000000000000000000000000000"): common.HexToHash("0x00000000000000000000000168305d4c0d5fbfe94c41611d872999ff03041a0c"),
						common.HexToHash("0x007996a2955dfb0e6cc1f05f5df3579d5831b46aaaa0fe130da794ddfdda18ad"): common.HexToHash("0x000000000000000000000000000000000000000000000000000000000000d020"),
						common.HexToHash("0x1e25a176e2a29be4895c82c1fea3a7bb055502dbeea521433ecfc88437374d29"): common.HexToHash("0x000000000000000000000000000000000000000000000000000000000000d018"),
						common.HexToHash("0x20bba94ff21426d24a63b6beab6173053751a645a7d15588358171dd6e96ee0b"): common.HexToHash("0x000000000000000000000000000000000000000000000000000000000000d011"),
						common.HexToHash("0x2131a4338f6fb8d4507e234a7c72af8efefbbf2f1817ed570bce33eb6667feb9"): common.HexToHash("0x000000000000000000000000000000000000000000000000000000000000d007"),
						common.HexToHash("0x29629c343b5c9fff49cd3a964bb74da872f3e0e9073cf3ea89a87d628c2d2ca7"): common.HexToHash("0x000000000000000000000000000000000000000000000000000000000000d015"),
						common.HexToHash("0x2f4e7b858e1ed178207a10238ebf5b1358e6a4a41e765a3edf3260730760b38c"): common.HexToHash("0x000000000000000000000000000000000000000000000000000000000000d008"),
						common.HexToHash("0x360894a13ba1a3210667c828492db98dca3e2076cc3735a920a3ca505d382bbc"): common.HexToHash("0x000000000000000000000000000000000000000000000000000000000000ce11"),
						common.HexToHash("0x75183b8aabdad5fbffc715bbe70535afe50ff8ed453a4223c47a5d1ab0048387"): common.HexToHash("0x000000000000000000000000fce9990d3f6ec6f32f0fba3bb54c5d657aa0eaa7"),
						common.HexToHash("0x7710c8bd31e5abcf8e2338ad2569e2a8461b242c3be6fae253ffa1f963ddd0df"): common.HexToHash("0x000000000000000000000000000000000000000000000000000000000000d009"),
						common.HexToHash("0x773cc8652456781771d48fb3d560d7ba3d6d1882654492b68beec583d2c590aa"): common.HexToHash("0x000000000000000000000000000000000000000000000000000000000000d003"),
						common.HexToHash("0x78e70a8344b05ed06597d2ea30a73432dda3d18e13fe04cdf5f151b6125a4a80"): common.HexToHash("0x000000000000000000000000000000000000000000000000000000000000d016"),
						common.HexToHash("0x793b1416a9371863f87b090796dcb7b72023635eca4ce3d9e5fa1d1e0fa079f1"): common.HexToHash("0x000000000000000000000000000000000000000000000000000000000000d001"),
						common.HexToHash("0x91646b8507bf2e54d7c3de9155442ba111546b81af1cbdd1f68eeb6926b98d58"): common.HexToHash("0x000000000000000000000000000000000000000000000000000000000000d023"),
						common.HexToHash("0x92ddd299fce8b8859b13f0abb8acd4201708ef817ce47d1e5c73bfcf022d4219"): common.HexToHash("0x000000000000000000000000000000000000000000000000000000000000d025"),
						common.HexToHash("0x99f4d1d2143fa272a89811a6763d70f10f8ea59f7bed6c8d26575068d6c25b36"): common.HexToHash("0x000000000000000000000000000000000000000000000000000000000000d014"),
						common.HexToHash("0xa183d5e9218bd0cfb583447ccc3cef80b84785f7bdc0238ffd8eabb05d65d1ea"): common.HexToHash("0x000000000000000000000000000000000000000000000000000000000000d017"),
						common.HexToHash("0xa9245761f51d6db8b57b57f6a4e786d36bae68f0b6e37574f144134797f83226"): common.HexToHash("0x000000000000000000000000000000000000000000000000000000000000d019"),
						common.HexToHash("0xb52eb58cb1dd3553b2be57a59d9c31b6c860a30720b166c0bbc7357126eb2cab"): common.HexToHash("0x000000000000000000000000000000000000000000000000000000000000d010"),
						common.HexToHash("0xb53127684a568b3173ae13b9f8a6016e243e63b6e8ee1178d6a717850b5d6103"): common.HexToHash("0x00000000000000000000000068305d4c0d5fbfe94c41611d872999ff03041a0c"),
						common.HexToHash("0xc175c49c4a7dbc28c923d41d18b2e5bfc6ac7519c367d7fddecfb6166035d8e2"): common.HexToHash("0x000000000000000000000000000000000000000000000000000000000000d021"),
						common.HexToHash("0xc2afdc46de17996f5f21e9c9578a94e0fd98d1b2dfd1606177b845f447daa8c5"): common.HexToHash("0x000000000000000000000000000000000000000000000000000000000000d024"),
						common.HexToHash("0xc8df4f680dc540ec9b0c613a7976e29988d07a5b20c6b972785f8d9cac08f4f5"): common.HexToHash("0x000000000000000000000000000000000000000000000000000000000000d005"),
						common.HexToHash("0xcff58fbbf19e274834dbbb0d6c1c84666c663932b89291bf061ab49d7e3f639a"): common.HexToHash("0x000000000000000000000000000000000000000000000000000000000000d004"),
						common.HexToHash("0xd367e4a67661f77f499d4555e9708608d5c2a630ce781de18076a8937a468d1e"): common.HexToHash("0x000000000000000000000000000000000000000000000000000000000000d013"),
						common.HexToHash("0xd830678365b5dbc6b4dde9665b5546573fb66956cd1a3e9c931e98cd38f26f60"): common.HexToHash("0x000000000000000000000000000000000000000000000000000000000000d002"),
						common.HexToHash("0xdb86e256b54dc557d351c1e75149bf37b30262c1488d7a96a5b42562c7344c93"): common.HexToHash("0x000000000000000000000000000000000000000000000000000000000000ce10"),
						common.HexToHash("0xe79a7848066cc9bfbdadfb6b63d8a5f3431bf53fec40f4cdce6d5fc90cc4ad78"): common.HexToHash("0x000000000000000000000000000000000000000000000000000000000000d012"),
					},
					Balance: big.NewInt(0),
				},
				common.HexToAddress("0xce11"): {
					Code:    []byte("0x608060405234801561001057600080fd5b50600436106100cf5760003560e01c80638932cbf41161008c578063c586579311610066578063c586579314610407578063dcf0aaed146104a0578063dd9272331461050e578063f2fde38b1461057c576100cf565b80638932cbf4146102e25780638da5cb5b1461039b5780638f32d59b146103e5576100cf565b8063158ef93e146100d457806317c50818146100f6578063715018a6146101a75780637ef50298146101b15780638129fc1c1461021f578063853db32314610229575b600080fd5b6100dc6105c0565b604051808215151515815260200191505060405180910390f35b61018d6004803603604081101561010c57600080fd5b810190808035906020019064010000000081111561012957600080fd5b82018360208201111561013b57600080fd5b8035906020019184602083028401116401000000008311171561015d57600080fd5b9091929391929390803573ffffffffffffffffffffffffffffffffffffffff1690602001909291905050506105d3565b604051808215151515815260200191505060405180910390f35b6101af610691565b005b6101dd600480360360208110156101c757600080fd5b81019080803590602001909291905050506107ca565b604051808273ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200191505060405180910390f35b6102276107fd565b005b6102a06004803603602081101561023f57600080fd5b810190808035906020019064010000000081111561025c57600080fd5b82018360208201111561026e57600080fd5b8035906020019184600183028401116401000000008311171561029057600080fd5b90919293919293905050506108a6565b604051808273ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200191505060405180910390f35b610359600480360360208110156102f857600080fd5b810190808035906020019064010000000081111561031557600080fd5b82018360208201111561032757600080fd5b8035906020019184600183028401116401000000008311171561034957600080fd5b9091929391929390505050610918565b604051808273ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200191505060405180910390f35b6103a3610a60565b604051808273ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200191505060405180910390f35b6103ed610a89565b604051808215151515815260200191505060405180910390f35b61049e6004803603604081101561041d57600080fd5b810190808035906020019064010000000081111561043a57600080fd5b82018360208201111561044c57600080fd5b8035906020019184600183028401116401000000008311171561046e57600080fd5b9091929391929390803573ffffffffffffffffffffffffffffffffffffffff169060200190929190505050610ae7565b005b6104cc600480360360208110156104b657600080fd5b8101908080359060200190929190505050610c68565b604051808273ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200191505060405180910390f35b61053a6004803603602081101561052457600080fd5b8101908080359060200190929190505050610d7a565b604051808273ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200191505060405180910390f35b6105be6004803603602081101561059257600080fd5b81019080803573ffffffffffffffffffffffffffffffffffffffff169060200190929190505050610db7565b005b600060149054906101000a900460ff1681565b600080600090505b84849050811015610684578273ffffffffffffffffffffffffffffffffffffffff166001600087878581811061060d57fe5b90506020020135815260200190815260200160002060009054906101000a900473ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16141561066957600191505061068a565b61067d600182610e3d90919063ffffffff16565b90506105db565b50600090505b9392505050565b610699610a89565b61070b576040517f08c379a00000000000000000000000000000000000000000000000000000000081526004018080602001828103825260208152602001807f4f776e61626c653a2063616c6c6572206973206e6f7420746865206f776e657281525060200191505060405180910390fd5b600073ffffffffffffffffffffffffffffffffffffffff166000809054906101000a900473ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff167f8be0079c531659141344cd1fd0a4f28419497f9722a3daafe3b4186f6b6457e060405160405180910390a360008060006101000a81548173ffffffffffffffffffffffffffffffffffffffff021916908373ffffffffffffffffffffffffffffffffffffffff160217905550565b60016020528060005260406000206000915054906101000a900473ffffffffffffffffffffffffffffffffffffffff1681565b600060149054906101000a900460ff1615610880576040517f08c379a000000000000000000000000000000000000000000000000000000000815260040180806020018281038252601c8152602001807f636f6e747261637420616c726561647920696e697469616c697a65640000000081525060200191505060405180910390fd5b6001600060146101000a81548160ff0219169083151502179055506108a433610ec5565b565b60008083836040516020018083838082843780830192505050925050506040516020818303038152906040528051906020012090506001600082815260200190815260200160002060009054906101000a900473ffffffffffffffffffffffffffffffffffffffff1691505092915050565b6000808383604051602001808383808284378083019250505092505050604051602081830303815290604052805190602001209050600073ffffffffffffffffffffffffffffffffffffffff166001600083815260200190815260200160002060009054906101000a900473ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff161415610a23576040517f08c379a00000000000000000000000000000000000000000000000000000000081526004018080602001828103825260208152602001807f6964656e74696669657220686173206e6f20726567697374727920656e74727981525060200191505060405180910390fd5b6001600082815260200190815260200160002060009054906101000a900473ffffffffffffffffffffffffffffffffffffffff1691505092915050565b60008060009054906101000a900473ffffffffffffffffffffffffffffffffffffffff16905090565b60008060009054906101000a900473ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16610acb611009565b73ffffffffffffffffffffffffffffffffffffffff1614905090565b610aef610a89565b610b61576040517f08c379a00000000000000000000000000000000000000000000000000000000081526004018080602001828103825260208152602001807f4f776e61626c653a2063616c6c6572206973206e6f7420746865206f776e657281525060200191505060405180910390fd5b60008383604051602001808383808284378083019250505092505050604051602081830303815290604052805190602001209050816001600083815260200190815260200160002060006101000a81548173ffffffffffffffffffffffffffffffffffffffff021916908373ffffffffffffffffffffffffffffffffffffffff1602179055508173ffffffffffffffffffffffffffffffffffffffff16817f4166d073a7a5e704ce0db7113320f88da2457f872d46dc020c805c562c1582a0868660405180806020018281038252848482818152602001925080828437600081840152601f19601f820116905080830192505050935050505060405180910390a350505050565b60008073ffffffffffffffffffffffffffffffffffffffff166001600084815260200190815260200160002060009054906101000a900473ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff161415610d3f576040517f08c379a00000000000000000000000000000000000000000000000000000000081526004018080602001828103825260208152602001807f6964656e74696669657220686173206e6f20726567697374727920656e74727981525060200191505060405180910390fd5b6001600083815260200190815260200160002060009054906101000a900473ffffffffffffffffffffffffffffffffffffffff169050919050565b60006001600083815260200190815260200160002060009054906101000a900473ffffffffffffffffffffffffffffffffffffffff169050919050565b610dbf610a89565b610e31576040517f08c379a00000000000000000000000000000000000000000000000000000000081526004018080602001828103825260208152602001807f4f776e61626c653a2063616c6c6572206973206e6f7420746865206f776e657281525060200191505060405180910390fd5b610e3a81610ec5565b50565b600080828401905083811015610ebb576040517f08c379a000000000000000000000000000000000000000000000000000000000815260040180806020018281038252601b8152602001807f536166654d6174683a206164646974696f6e206f766572666c6f77000000000081525060200191505060405180910390fd5b8091505092915050565b600073ffffffffffffffffffffffffffffffffffffffff168173ffffffffffffffffffffffffffffffffffffffff161415610f4b576040517f08c379a00000000000000000000000000000000000000000000000000000000081526004018080602001828103825260268152602001806110126026913960400191505060405180910390fd5b8073ffffffffffffffffffffffffffffffffffffffff166000809054906101000a900473ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff167f8be0079c531659141344cd1fd0a4f28419497f9722a3daafe3b4186f6b6457e060405160405180910390a3806000806101000a81548173ffffffffffffffffffffffffffffffffffffffff021916908373ffffffffffffffffffffffffffffffffffffffff16021790555050565b60003390509056fe4f776e61626c653a206e6577206f776e657220697320746865207a65726f2061646472657373a265627a7a7231582021f804e21a59bd673149265b250bf50ab40b86b057ee8f9aeab3d3c904a82cdc64736f6c634300050d0032"),
					Balance: big.NewInt(0),
				},
			},
		}
	)

	gspec.Config.Faker = true
	fmt.Println("ready")
	genesis := gspec.MustCommit(db)
	fmt.Println("MustCommit")
	signer := types.LatestSigner(gspec.Config)
	txFeeRecipient := common.Address{10}

	blocks, _ := GenerateChain(gspec.Config, genesis, engine, db, 1, func(i int, b *BlockGen) {
		b.SetCoinbase(txFeeRecipient)

		// One transaction to 0xAAAA
		accesses := types.AccessList{types.AccessTuple{
			Address:     aa,
			StorageKeys: []common.Hash{{0}},
		}}

		txdata := &types.DynamicFeeTx{
			ChainID:    gspec.Config.ChainID,
			Nonce:      0,
			To:         &aa,
			Gas:        30000,
			GasFeeCap:  newGwei(5),
			GasTipCap:  big.NewInt(2),
			AccessList: accesses,
			Data:       []byte{},
		}
		tx := types.NewTx(txdata)
		tx, _ = types.SignTx(tx, signer, key1)

		b.AddTx(tx)
	})
	fmt.Println("GenerateChain")

	diskdb := rawdb.NewMemoryDatabase()
	gspec.MustCommit(diskdb)

	chain, err := NewBlockChain(diskdb, nil, gspec.Config, engine, vm.Config{}, nil, nil)
	defer chain.Stop()
	if err != nil {
		t.Fatalf("failed to create tester chain: %v", err)
	}
	if n, err := chain.InsertChain(blocks); err != nil {
		t.Fatalf("block %d: failed to insert into chain: %v", n, err)
	}

	block := chain.GetBlockByNumber(1)

	// 1+2: Ensure EIP-1559 access lists are accounted for via gas usage.
	expectedGas := params.TxGas + params.TxAccessListAddressGas + params.TxAccessListStorageKeyGas +
		vm.GasQuickStep*2 + params.WarmStorageReadCostEIP2929 + params.ColdSloadCostEIP2929
	if block.GasUsed() != expectedGas {
		t.Fatalf("incorrect amount of gas spent: expected %d, got %d", expectedGas, block.GasUsed())
	}

	state, _ := chain.State()

	// 3: Ensure that miner received only the tx's tip.
	actual := state.GetBalance(txFeeRecipient)
	expected := new(big.Int).SetUint64(block.GasUsed()*block.Transactions()[0].GasTipCap().Uint64() + 1) // 1 is added by accumulateRewards in consensustest.MockEngine, and will break blockchain_repair_test.go if set 0

	if actual.Cmp(expected) != 0 {
		t.Fatalf("miner balance incorrect: expected %d, got %d", expected, actual)
	}

	// 4: Ensure the tx sender paid for the gasUsed * (tip + block baseFee).
	baseFee := MockSysContractCallCtx().GetGasPriceMinimum(nil).Uint64()
	actual = new(big.Int).Sub(funds, state.GetBalance(addr1))
	expected = new(big.Int).SetUint64(block.GasUsed() * (block.Transactions()[0].GasTipCap().Uint64() + baseFee))
	if actual.Cmp(expected) != 0 {
		t.Fatalf("sender paid fee incorrect: expected %d, got %d", expected, actual)
	}

	blocks, _ = GenerateChain(gspec.Config, block, engine, db, 1, func(i int, b *BlockGen) {
		b.SetCoinbase(common.Address{2})

		txdata := &types.LegacyTx{
			Nonce:    0,
			To:       &aa,
			Gas:      30000,
			GasPrice: newGwei(5),
		}
		tx := types.NewTx(txdata)
		tx, _ = types.SignTx(tx, signer, key2)

		b.AddTx(tx)
	})

	if n, err := chain.InsertChain(blocks); err != nil {
		t.Fatalf("block %d: failed to insert into chain: %v", n, err)
	}

	block = chain.GetBlockByNumber(2)
	state, _ = chain.State()
	effectiveTip := block.Transactions()[0].GasTipCap().Uint64() - baseFee

	// 6+5: Ensure that miner received only the tx's effective tip.
	actual = state.GetBalance(block.Coinbase())
	expected = new(big.Int).SetUint64(block.GasUsed()*effectiveTip + 1) // 1 is added by accumulateRewards in consensustest.MockEngine, and will break blockchain_repair_test.go if set 0
	if actual.Cmp(expected) != 0 {
		t.Fatalf("miner balance incorrect: expected %d, got %d", expected, actual)
	}

	// 4: Ensure the tx sender paid for the gasUsed * (effectiveTip + block baseFee).
	actual = new(big.Int).Sub(funds, state.GetBalance(addr2))
	expected = new(big.Int).SetUint64(block.GasUsed() * (effectiveTip + baseFee))
	if actual.Cmp(expected) != 0 {
		t.Fatalf("sender paid fee incorrect: expected %d, got %d", expected, actual)
	}
}
