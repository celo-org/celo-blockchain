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
	"math/big"
	"sort"
	"sync"

	"github.com/celo-org/celo-blockchain/common"
	mockEngine "github.com/celo-org/celo-blockchain/consensus/consensustest"
	"github.com/celo-org/celo-blockchain/consensus/istanbul"
	"github.com/celo-org/celo-blockchain/core"
	"github.com/celo-org/celo-blockchain/core/rawdb"
	"github.com/celo-org/celo-blockchain/core/state"
	"github.com/celo-org/celo-blockchain/core/types"
	"github.com/celo-org/celo-blockchain/core/vm"
	"github.com/celo-org/celo-blockchain/crypto"
	"github.com/celo-org/celo-blockchain/eth/downloader"
	"github.com/celo-org/celo-blockchain/event"
	"github.com/celo-org/celo-blockchain/p2p"
	"github.com/celo-org/celo-blockchain/params"
	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/consensus/ethash"
	"github.com/celo-org/celo-blockchain/core"
	"github.com/celo-org/celo-blockchain/core/rawdb"
	"github.com/celo-org/celo-blockchain/core/state"
	"github.com/celo-org/celo-blockchain/core/types"
	"github.com/celo-org/celo-blockchain/core/vm"
	"github.com/celo-org/celo-blockchain/crypto"
	"github.com/celo-org/celo-blockchain/eth/downloader"
	"github.com/celo-org/celo-blockchain/event"
	"github.com/celo-org/celo-blockchain/p2p"
	"github.com/celo-org/celo-blockchain/params"
	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/consensus/ethash"
	"github.com/celo-org/celo-blockchain/core"
	"github.com/celo-org/celo-blockchain/core/rawdb"
	"github.com/celo-org/celo-blockchain/core/types"
	"github.com/celo-org/celo-blockchain/core/vm"
	"github.com/celo-org/celo-blockchain/crypto"
	"github.com/celo-org/celo-blockchain/eth/downloader"
	"github.com/celo-org/celo-blockchain/ethdb"
	"github.com/celo-org/celo-blockchain/event"
	"github.com/celo-org/celo-blockchain/params"
)

<<<<<<< HEAD
// Tests that block headers can be retrieved from a remote chain based on user queries.
func TestGetBlockHeaders64(t *testing.T) { testGetBlockHeaders(t, 64) }
func TestGetBlockHeaders65(t *testing.T) { testGetBlockHeaders(t, 65) }

func testGetBlockHeaders(t *testing.T, protocol int) {
	pm, _ := newTestProtocolManagerMust(t, downloader.FullSync, downloader.MaxHashFetch+15, nil, nil)
	peer, _ := newTestPeer("peer", protocol, pm, true)
	defer peer.close()

	// Create a "random" unknown hash for testing
	var unknown common.Hash
	for i := range unknown {
		unknown[i] = byte(i)
	}
	// Create a batch of tests for various scenarios
	limit := uint64(downloader.MaxHeaderFetch)
	tests := []struct {
		query  *getBlockHeadersData // The query to execute for header retrieval
		expect []common.Hash        // The hashes of the block whose headers are expected
	}{
		// A single random block should be retrievable by hash and number too
		{
			&getBlockHeadersData{Origin: hashOrNumber{Hash: pm.blockchain.GetBlockByNumber(limit / 2).Hash()}, Amount: 1},
			[]common.Hash{pm.blockchain.GetBlockByNumber(limit / 2).Hash()},
		}, {
			&getBlockHeadersData{Origin: hashOrNumber{Number: limit / 2}, Amount: 1},
			[]common.Hash{pm.blockchain.GetBlockByNumber(limit / 2).Hash()},
		},
		// Multiple headers should be retrievable in both directions
		{
			&getBlockHeadersData{Origin: hashOrNumber{Number: limit / 2}, Amount: 3},
			[]common.Hash{
				pm.blockchain.GetBlockByNumber(limit / 2).Hash(),
				pm.blockchain.GetBlockByNumber(limit/2 + 1).Hash(),
				pm.blockchain.GetBlockByNumber(limit/2 + 2).Hash(),
			},
		}, {
			&getBlockHeadersData{Origin: hashOrNumber{Number: limit / 2}, Amount: 3, Reverse: true},
			[]common.Hash{
				pm.blockchain.GetBlockByNumber(limit / 2).Hash(),
				pm.blockchain.GetBlockByNumber(limit/2 - 1).Hash(),
				pm.blockchain.GetBlockByNumber(limit/2 - 2).Hash(),
			},
		},
		// Multiple headers with skip lists should be retrievable
		{
			&getBlockHeadersData{Origin: hashOrNumber{Number: limit / 2}, Skip: 3, Amount: 3},
			[]common.Hash{
				pm.blockchain.GetBlockByNumber(limit / 2).Hash(),
				pm.blockchain.GetBlockByNumber(limit/2 + 4).Hash(),
				pm.blockchain.GetBlockByNumber(limit/2 + 8).Hash(),
			},
		}, {
			&getBlockHeadersData{Origin: hashOrNumber{Number: limit / 2}, Skip: 3, Amount: 3, Reverse: true},
			[]common.Hash{
				pm.blockchain.GetBlockByNumber(limit / 2).Hash(),
				pm.blockchain.GetBlockByNumber(limit/2 - 4).Hash(),
				pm.blockchain.GetBlockByNumber(limit/2 - 8).Hash(),
			},
		},
		// The chain endpoints should be retrievable
		{
			&getBlockHeadersData{Origin: hashOrNumber{Number: 0}, Amount: 1},
			[]common.Hash{pm.blockchain.GetBlockByNumber(0).Hash()},
		}, {
			&getBlockHeadersData{Origin: hashOrNumber{Number: pm.blockchain.CurrentBlock().NumberU64()}, Amount: 1},
			[]common.Hash{pm.blockchain.CurrentBlock().Hash()},
		},
		// Ensure protocol limits are honored
		{
			&getBlockHeadersData{Origin: hashOrNumber{Number: pm.blockchain.CurrentBlock().NumberU64() - 1}, Amount: limit + 10, Reverse: true},
			pm.blockchain.GetBlockHashesFromHash(pm.blockchain.CurrentBlock().Hash(), limit),
		},
		// Check that requesting more than available is handled gracefully
		{
			&getBlockHeadersData{Origin: hashOrNumber{Number: pm.blockchain.CurrentBlock().NumberU64() - 4}, Skip: 3, Amount: 3},
			[]common.Hash{
				pm.blockchain.GetBlockByNumber(pm.blockchain.CurrentBlock().NumberU64() - 4).Hash(),
				pm.blockchain.GetBlockByNumber(pm.blockchain.CurrentBlock().NumberU64()).Hash(),
			},
		}, {
			&getBlockHeadersData{Origin: hashOrNumber{Number: 4}, Skip: 3, Amount: 3, Reverse: true},
			[]common.Hash{
				pm.blockchain.GetBlockByNumber(4).Hash(),
				pm.blockchain.GetBlockByNumber(0).Hash(),
			},
		},
		// Check that requesting more than available is handled gracefully, even if mid skip
		{
			&getBlockHeadersData{Origin: hashOrNumber{Number: pm.blockchain.CurrentBlock().NumberU64() - 4}, Skip: 2, Amount: 3},
			[]common.Hash{
				pm.blockchain.GetBlockByNumber(pm.blockchain.CurrentBlock().NumberU64() - 4).Hash(),
				pm.blockchain.GetBlockByNumber(pm.blockchain.CurrentBlock().NumberU64() - 1).Hash(),
			},
		}, {
			&getBlockHeadersData{Origin: hashOrNumber{Number: 4}, Skip: 2, Amount: 3, Reverse: true},
			[]common.Hash{
				pm.blockchain.GetBlockByNumber(4).Hash(),
				pm.blockchain.GetBlockByNumber(1).Hash(),
			},
		},
		// Check a corner case where requesting more can iterate past the endpoints
		{
			&getBlockHeadersData{Origin: hashOrNumber{Number: 2}, Amount: 5, Reverse: true},
			[]common.Hash{
				pm.blockchain.GetBlockByNumber(2).Hash(),
				pm.blockchain.GetBlockByNumber(1).Hash(),
				pm.blockchain.GetBlockByNumber(0).Hash(),
			},
		},
		// Check a corner case where skipping overflow loops back into the chain start
		{
			&getBlockHeadersData{Origin: hashOrNumber{Hash: pm.blockchain.GetBlockByNumber(3).Hash()}, Amount: 2, Reverse: false, Skip: math.MaxUint64 - 1},
			[]common.Hash{
				pm.blockchain.GetBlockByNumber(3).Hash(),
			},
		},
		// Check a corner case where skipping overflow loops back to the same header
		{
			&getBlockHeadersData{Origin: hashOrNumber{Hash: pm.blockchain.GetBlockByNumber(1).Hash()}, Amount: 2, Reverse: false, Skip: math.MaxUint64},
			[]common.Hash{
				pm.blockchain.GetBlockByNumber(1).Hash(),
			},
		},
		// Check that non existing headers aren't returned
		{
			&getBlockHeadersData{Origin: hashOrNumber{Hash: unknown}, Amount: 1},
			[]common.Hash{},
		}, {
			&getBlockHeadersData{Origin: hashOrNumber{Number: pm.blockchain.CurrentBlock().NumberU64() + 1}, Amount: 1},
			[]common.Hash{},
		},
	}
	// Run each of the tests and verify the results against the chain
	for i, tt := range tests {
		// Collect the headers to expect in the response
		headers := []*types.Header{}
		for _, hash := range tt.expect {
			headers = append(headers, pm.blockchain.GetBlockByHash(hash).Header())
		}
		// Send the hash request and verify the response
		p2p.Send(peer.app, 0x03, tt.query)
		if err := p2p.ExpectMsg(peer.app, 0x04, headers); err != nil {
			t.Fatalf("test %d: headers mismatch: %v", i, err)
		}
		// If the test used number origins, repeat with hashes as the too
		if tt.query.Origin.Hash == (common.Hash{}) {
			if origin := pm.blockchain.GetBlockByNumber(tt.query.Origin.Number); origin != nil {
				tt.query.Origin.Hash, tt.query.Origin.Number = origin.Hash(), 0

				p2p.Send(peer.app, 0x03, tt.query)
				if err := p2p.ExpectMsg(peer.app, 0x04, headers); err != nil {
					t.Fatalf("test %d: headers mismatch: %v", i, err)
				}
			}
		}
	}
||||||| e78727290
// Tests that block headers can be retrieved from a remote chain based on user queries.
func TestGetBlockHeaders63(t *testing.T) { testGetBlockHeaders(t, 63) }
func TestGetBlockHeaders64(t *testing.T) { testGetBlockHeaders(t, 64) }

func testGetBlockHeaders(t *testing.T, protocol int) {
	pm, _ := newTestProtocolManagerMust(t, downloader.FullSync, downloader.MaxHashFetch+15, nil, nil)
	peer, _ := newTestPeer("peer", protocol, pm, true)
	defer peer.close()

	// Create a "random" unknown hash for testing
	var unknown common.Hash
	for i := range unknown {
		unknown[i] = byte(i)
	}
	// Create a batch of tests for various scenarios
	limit := uint64(downloader.MaxHeaderFetch)
	tests := []struct {
		query  *getBlockHeadersData // The query to execute for header retrieval
		expect []common.Hash        // The hashes of the block whose headers are expected
	}{
		// A single random block should be retrievable by hash and number too
		{
			&getBlockHeadersData{Origin: hashOrNumber{Hash: pm.blockchain.GetBlockByNumber(limit / 2).Hash()}, Amount: 1},
			[]common.Hash{pm.blockchain.GetBlockByNumber(limit / 2).Hash()},
		}, {
			&getBlockHeadersData{Origin: hashOrNumber{Number: limit / 2}, Amount: 1},
			[]common.Hash{pm.blockchain.GetBlockByNumber(limit / 2).Hash()},
		},
		// Multiple headers should be retrievable in both directions
		{
			&getBlockHeadersData{Origin: hashOrNumber{Number: limit / 2}, Amount: 3},
			[]common.Hash{
				pm.blockchain.GetBlockByNumber(limit / 2).Hash(),
				pm.blockchain.GetBlockByNumber(limit/2 + 1).Hash(),
				pm.blockchain.GetBlockByNumber(limit/2 + 2).Hash(),
			},
		}, {
			&getBlockHeadersData{Origin: hashOrNumber{Number: limit / 2}, Amount: 3, Reverse: true},
			[]common.Hash{
				pm.blockchain.GetBlockByNumber(limit / 2).Hash(),
				pm.blockchain.GetBlockByNumber(limit/2 - 1).Hash(),
				pm.blockchain.GetBlockByNumber(limit/2 - 2).Hash(),
			},
		},
		// Multiple headers with skip lists should be retrievable
		{
			&getBlockHeadersData{Origin: hashOrNumber{Number: limit / 2}, Skip: 3, Amount: 3},
			[]common.Hash{
				pm.blockchain.GetBlockByNumber(limit / 2).Hash(),
				pm.blockchain.GetBlockByNumber(limit/2 + 4).Hash(),
				pm.blockchain.GetBlockByNumber(limit/2 + 8).Hash(),
			},
		}, {
			&getBlockHeadersData{Origin: hashOrNumber{Number: limit / 2}, Skip: 3, Amount: 3, Reverse: true},
			[]common.Hash{
				pm.blockchain.GetBlockByNumber(limit / 2).Hash(),
				pm.blockchain.GetBlockByNumber(limit/2 - 4).Hash(),
				pm.blockchain.GetBlockByNumber(limit/2 - 8).Hash(),
			},
		},
		// The chain endpoints should be retrievable
		{
			&getBlockHeadersData{Origin: hashOrNumber{Number: 0}, Amount: 1},
			[]common.Hash{pm.blockchain.GetBlockByNumber(0).Hash()},
		}, {
			&getBlockHeadersData{Origin: hashOrNumber{Number: pm.blockchain.CurrentBlock().NumberU64()}, Amount: 1},
			[]common.Hash{pm.blockchain.CurrentBlock().Hash()},
		},
		// Ensure protocol limits are honored
		{
			&getBlockHeadersData{Origin: hashOrNumber{Number: pm.blockchain.CurrentBlock().NumberU64() - 1}, Amount: limit + 10, Reverse: true},
			pm.blockchain.GetBlockHashesFromHash(pm.blockchain.CurrentBlock().Hash(), limit),
		},
		// Check that requesting more than available is handled gracefully
		{
			&getBlockHeadersData{Origin: hashOrNumber{Number: pm.blockchain.CurrentBlock().NumberU64() - 4}, Skip: 3, Amount: 3},
			[]common.Hash{
				pm.blockchain.GetBlockByNumber(pm.blockchain.CurrentBlock().NumberU64() - 4).Hash(),
				pm.blockchain.GetBlockByNumber(pm.blockchain.CurrentBlock().NumberU64()).Hash(),
			},
		}, {
			&getBlockHeadersData{Origin: hashOrNumber{Number: 4}, Skip: 3, Amount: 3, Reverse: true},
			[]common.Hash{
				pm.blockchain.GetBlockByNumber(4).Hash(),
				pm.blockchain.GetBlockByNumber(0).Hash(),
			},
		},
		// Check that requesting more than available is handled gracefully, even if mid skip
		{
			&getBlockHeadersData{Origin: hashOrNumber{Number: pm.blockchain.CurrentBlock().NumberU64() - 4}, Skip: 2, Amount: 3},
			[]common.Hash{
				pm.blockchain.GetBlockByNumber(pm.blockchain.CurrentBlock().NumberU64() - 4).Hash(),
				pm.blockchain.GetBlockByNumber(pm.blockchain.CurrentBlock().NumberU64() - 1).Hash(),
			},
		}, {
			&getBlockHeadersData{Origin: hashOrNumber{Number: 4}, Skip: 2, Amount: 3, Reverse: true},
			[]common.Hash{
				pm.blockchain.GetBlockByNumber(4).Hash(),
				pm.blockchain.GetBlockByNumber(1).Hash(),
			},
		},
		// Check a corner case where requesting more can iterate past the endpoints
		{
			&getBlockHeadersData{Origin: hashOrNumber{Number: 2}, Amount: 5, Reverse: true},
			[]common.Hash{
				pm.blockchain.GetBlockByNumber(2).Hash(),
				pm.blockchain.GetBlockByNumber(1).Hash(),
				pm.blockchain.GetBlockByNumber(0).Hash(),
			},
		},
		// Check a corner case where skipping overflow loops back into the chain start
		{
			&getBlockHeadersData{Origin: hashOrNumber{Hash: pm.blockchain.GetBlockByNumber(3).Hash()}, Amount: 2, Reverse: false, Skip: math.MaxUint64 - 1},
			[]common.Hash{
				pm.blockchain.GetBlockByNumber(3).Hash(),
			},
		},
		// Check a corner case where skipping overflow loops back to the same header
		{
			&getBlockHeadersData{Origin: hashOrNumber{Hash: pm.blockchain.GetBlockByNumber(1).Hash()}, Amount: 2, Reverse: false, Skip: math.MaxUint64},
			[]common.Hash{
				pm.blockchain.GetBlockByNumber(1).Hash(),
			},
		},
		// Check that non existing headers aren't returned
		{
			&getBlockHeadersData{Origin: hashOrNumber{Hash: unknown}, Amount: 1},
			[]common.Hash{},
		}, {
			&getBlockHeadersData{Origin: hashOrNumber{Number: pm.blockchain.CurrentBlock().NumberU64() + 1}, Amount: 1},
			[]common.Hash{},
		},
	}
	// Run each of the tests and verify the results against the chain
	for i, tt := range tests {
		// Collect the headers to expect in the response
		headers := []*types.Header{}
		for _, hash := range tt.expect {
			headers = append(headers, pm.blockchain.GetBlockByHash(hash).Header())
		}
		// Send the hash request and verify the response
		p2p.Send(peer.app, 0x03, tt.query)
		if err := p2p.ExpectMsg(peer.app, 0x04, headers); err != nil {
			t.Errorf("test %d: headers mismatch: %v", i, err)
		}
		// If the test used number origins, repeat with hashes as the too
		if tt.query.Origin.Hash == (common.Hash{}) {
			if origin := pm.blockchain.GetBlockByNumber(tt.query.Origin.Number); origin != nil {
				tt.query.Origin.Hash, tt.query.Origin.Number = origin.Hash(), 0

				p2p.Send(peer.app, 0x03, tt.query)
				if err := p2p.ExpectMsg(peer.app, 0x04, headers); err != nil {
					t.Errorf("test %d: headers mismatch: %v", i, err)
				}
			}
		}
	}
=======
var (
	// testKey is a private key to use for funding a tester account.
	testKey, _ = crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")

	// testAddr is the Ethereum address of the tester account.
	testAddr = crypto.PubkeyToAddress(testKey.PublicKey)
)

// testTxPool is a mock transaction pool that blindly accepts all transactions.
// Its goal is to get around setting up a valid statedb for the balance and nonce
// checks.
type testTxPool struct {
	pool map[common.Hash]*types.Transaction // Hash map of collected transactions

	txFeed event.Feed   // Notification feed to allow waiting for inclusion
	lock   sync.RWMutex // Protects the transaction pool
>>>>>>> v1.10.7
}

<<<<<<< HEAD
// Tests that block contents can be retrieved from a remote chain based on their hashes.
func TestGetBlockBodies64(t *testing.T) { testGetBlockBodies(t, 64) }
func TestGetBlockBodies65(t *testing.T) { testGetBlockBodies(t, 65) }

func testGetBlockBodies(t *testing.T, protocol int) {
	pm, _ := newTestProtocolManagerMust(t, downloader.FullSync, downloader.MaxBlockFetch+15, nil, nil)
	peer, _ := newTestPeer("peer", protocol, pm, true)
	defer peer.close()

	// Create a batch of tests for various scenarios
	limit := downloader.MaxBlockFetch
	tests := []struct {
		random    int           // Number of blocks to fetch randomly from the chain
		explicit  []common.Hash // Explicitly requested blocks
		available []bool        // Availability of explicitly requested blocks
		expected  int           // Total number of existing blocks to expect
	}{
		{1, nil, nil, 1},             // A single random block should be retrievable
		{10, nil, nil, 10},           // Multiple random blocks should be retrievable
		{limit, nil, nil, limit},     // The maximum possible blocks should be retrievable
		{limit + 1, nil, nil, limit}, // No more than the possible block count should be returned
		{0, []common.Hash{pm.blockchain.Genesis().Hash()}, []bool{true}, 1},      // The genesis block should be retrievable
		{0, []common.Hash{pm.blockchain.CurrentBlock().Hash()}, []bool{true}, 1}, // The chains head block should be retrievable
		{0, []common.Hash{{}}, []bool{false}, 0},                                 // A non existent block should not be returned

		// Existing and non-existing blocks interleaved should not cause problems
		{0, []common.Hash{
			{},
			pm.blockchain.GetBlockByNumber(1).Hash(),
			{},
			pm.blockchain.GetBlockByNumber(10).Hash(),
			{},
			pm.blockchain.GetBlockByNumber(100).Hash(),
			{},
		}, []bool{false, true, false, true, false, true, false}, 3},
	}
	// Run each of the tests and verify the results against the chain
	for i, tt := range tests {
		// Collect the hashes to request, and the response to expect
		hashes, seen := []common.Hash{}, make(map[int64]bool)
		bodiesAndBlockHashes := []*blockBodyWithBlockHash{}

		for j := 0; j < tt.random; j++ {
			for {
				num := rand.Int63n(int64(pm.blockchain.CurrentBlock().NumberU64()))
				if !seen[num] {
					seen[num] = true

					block := pm.blockchain.GetBlockByNumber(uint64(num))
					hashes = append(hashes, block.Hash())
					if len(bodiesAndBlockHashes) < tt.expected {
						bhEntry := &blockBodyWithBlockHash{BlockHash: block.Hash(),
							BlockBody: &types.Body{Transactions: block.Transactions(),
								Randomness:     block.Randomness(),
								EpochSnarkData: block.EpochSnarkData()}}
						bodiesAndBlockHashes = append(bodiesAndBlockHashes, bhEntry)
					}
					break
				}
			}
		}
		for j, hash := range tt.explicit {
			hashes = append(hashes, hash)
			if tt.available[j] && len(bodiesAndBlockHashes) < tt.expected {
				block := pm.blockchain.GetBlockByHash(hash)
				bhEntry := &blockBodyWithBlockHash{BlockHash: block.Hash(),
					BlockBody: &types.Body{Transactions: block.Transactions(),
						Randomness:     block.Randomness(),
						EpochSnarkData: block.EpochSnarkData()}}
				bodiesAndBlockHashes = append(bodiesAndBlockHashes, bhEntry)
			}
		}
		// Send the hash request and verify the response
		p2p.Send(peer.app, 0x05, hashes)
		if err := p2p.ExpectMsg(peer.app, 0x06, bodiesAndBlockHashes); err != nil {
			t.Fatalf("test %d: bodies mismatch: %v", i, err)
		}
||||||| e78727290
// Tests that block contents can be retrieved from a remote chain based on their hashes.
func TestGetBlockBodies63(t *testing.T) { testGetBlockBodies(t, 63) }
func TestGetBlockBodies64(t *testing.T) { testGetBlockBodies(t, 64) }

func testGetBlockBodies(t *testing.T, protocol int) {
	pm, _ := newTestProtocolManagerMust(t, downloader.FullSync, downloader.MaxBlockFetch+15, nil, nil)
	peer, _ := newTestPeer("peer", protocol, pm, true)
	defer peer.close()

	// Create a batch of tests for various scenarios
	limit := downloader.MaxBlockFetch
	tests := []struct {
		random    int           // Number of blocks to fetch randomly from the chain
		explicit  []common.Hash // Explicitly requested blocks
		available []bool        // Availability of explicitly requested blocks
		expected  int           // Total number of existing blocks to expect
	}{
		{1, nil, nil, 1},             // A single random block should be retrievable
		{10, nil, nil, 10},           // Multiple random blocks should be retrievable
		{limit, nil, nil, limit},     // The maximum possible blocks should be retrievable
		{limit + 1, nil, nil, limit}, // No more than the possible block count should be returned
		{0, []common.Hash{pm.blockchain.Genesis().Hash()}, []bool{true}, 1},      // The genesis block should be retrievable
		{0, []common.Hash{pm.blockchain.CurrentBlock().Hash()}, []bool{true}, 1}, // The chains head block should be retrievable
		{0, []common.Hash{{}}, []bool{false}, 0},                                 // A non existent block should not be returned

		// Existing and non-existing blocks interleaved should not cause problems
		{0, []common.Hash{
			{},
			pm.blockchain.GetBlockByNumber(1).Hash(),
			{},
			pm.blockchain.GetBlockByNumber(10).Hash(),
			{},
			pm.blockchain.GetBlockByNumber(100).Hash(),
			{},
		}, []bool{false, true, false, true, false, true, false}, 3},
	}
	// Run each of the tests and verify the results against the chain
	for i, tt := range tests {
		// Collect the hashes to request, and the response to expect
		hashes, seen := []common.Hash{}, make(map[int64]bool)
		bodies := []*blockBody{}

		for j := 0; j < tt.random; j++ {
			for {
				num := rand.Int63n(int64(pm.blockchain.CurrentBlock().NumberU64()))
				if !seen[num] {
					seen[num] = true

					block := pm.blockchain.GetBlockByNumber(uint64(num))
					hashes = append(hashes, block.Hash())
					if len(bodies) < tt.expected {
						bodies = append(bodies, &blockBody{Transactions: block.Transactions(), Uncles: block.Uncles()})
					}
					break
				}
			}
		}
		for j, hash := range tt.explicit {
			hashes = append(hashes, hash)
			if tt.available[j] && len(bodies) < tt.expected {
				block := pm.blockchain.GetBlockByHash(hash)
				bodies = append(bodies, &blockBody{Transactions: block.Transactions(), Uncles: block.Uncles()})
			}
		}
		// Send the hash request and verify the response
		p2p.Send(peer.app, 0x05, hashes)
		if err := p2p.ExpectMsg(peer.app, 0x06, bodies); err != nil {
			t.Errorf("test %d: bodies mismatch: %v", i, err)
		}
=======
// newTestTxPool creates a mock transaction pool.
func newTestTxPool() *testTxPool {
	return &testTxPool{
		pool: make(map[common.Hash]*types.Transaction),
>>>>>>> v1.10.7
	}
}

<<<<<<< HEAD
// Tests that the node state database can be retrieved based on hashes.
func TestGetNodeData64(t *testing.T) { testGetNodeData(t, 64) }
func TestGetNodeData65(t *testing.T) { testGetNodeData(t, 65) }

func testGetNodeData(t *testing.T, protocol int) {
	// Define three accounts to simulate transactions with
	acc1Key, _ := crypto.HexToECDSA("8a1f9a8f95be41cd7ccb6168179afb4504aefe388d1e14474d32c45c72ce7b7a")
	acc2Key, _ := crypto.HexToECDSA("49a7b37aa6f6645917e7b807e9d1c00d4fa71f18343b0d4122a4d2df64dd6fee")
	acc1Addr := crypto.PubkeyToAddress(acc1Key.PublicKey)
	acc2Addr := crypto.PubkeyToAddress(acc2Key.PublicKey)

	signer := types.HomesteadSigner{}
	// Create a chain generator with some simple transactions (blatantly stolen from @fjl/chain_markets_test)
	generator := func(i int, block *core.BlockGen) {
		switch i {
		case 0:
			// In block 1, the test bank sends account #1 some ether.
			tx, _ := types.SignTx(types.NewTransaction(block.TxNonce(testBank), acc1Addr, big.NewInt(10000), params.TxGas, nil, nil, nil, nil, nil), signer, testBankKey)
			block.AddTx(tx)
		case 1:
			// In block 2, the test bank sends some more ether to account #1.
			// acc1Addr passes it on to account #2.
			tx1, _ := types.SignTx(types.NewTransaction(block.TxNonce(testBank), acc1Addr, big.NewInt(1000), params.TxGas, nil, nil, nil, nil, nil), signer, testBankKey)
			tx2, _ := types.SignTx(types.NewTransaction(block.TxNonce(acc1Addr), acc2Addr, big.NewInt(1000), params.TxGas, nil, nil, nil, nil, nil), signer, acc1Key)
			block.AddTx(tx1)
			block.AddTx(tx2)
		case 2:
			// Block 3 is empty but was mined by account #2.
			block.SetCoinbase(acc2Addr)
			block.SetExtra([]byte("yeehaw"))
		}
	}
	// Assemble the test environment
	pm, db := newTestProtocolManagerMust(t, downloader.FullSync, 4, generator, nil)
	peer, _ := newTestPeer("peer", protocol, pm, true)
	defer peer.close()

	// Fetch for now the entire chain db
	hashes := []common.Hash{}

	it := db.NewIterator(nil, nil)
	for it.Next() {
		if key := it.Key(); len(key) == common.HashLength {
			hashes = append(hashes, common.BytesToHash(key))
		}
	}
	it.Release()
||||||| e78727290
// Tests that the node state database can be retrieved based on hashes.
func TestGetNodeData63(t *testing.T) { testGetNodeData(t, 63) }
func TestGetNodeData64(t *testing.T) { testGetNodeData(t, 64) }

func testGetNodeData(t *testing.T, protocol int) {
	// Define three accounts to simulate transactions with
	acc1Key, _ := crypto.HexToECDSA("8a1f9a8f95be41cd7ccb6168179afb4504aefe388d1e14474d32c45c72ce7b7a")
	acc2Key, _ := crypto.HexToECDSA("49a7b37aa6f6645917e7b807e9d1c00d4fa71f18343b0d4122a4d2df64dd6fee")
	acc1Addr := crypto.PubkeyToAddress(acc1Key.PublicKey)
	acc2Addr := crypto.PubkeyToAddress(acc2Key.PublicKey)

	signer := types.HomesteadSigner{}
	// Create a chain generator with some simple transactions (blatantly stolen from @fjl/chain_markets_test)
	generator := func(i int, block *core.BlockGen) {
		switch i {
		case 0:
			// In block 1, the test bank sends account #1 some ether.
			tx, _ := types.SignTx(types.NewTransaction(block.TxNonce(testBank), acc1Addr, big.NewInt(10000), params.TxGas, nil, nil), signer, testBankKey)
			block.AddTx(tx)
		case 1:
			// In block 2, the test bank sends some more ether to account #1.
			// acc1Addr passes it on to account #2.
			tx1, _ := types.SignTx(types.NewTransaction(block.TxNonce(testBank), acc1Addr, big.NewInt(1000), params.TxGas, nil, nil), signer, testBankKey)
			tx2, _ := types.SignTx(types.NewTransaction(block.TxNonce(acc1Addr), acc2Addr, big.NewInt(1000), params.TxGas, nil, nil), signer, acc1Key)
			block.AddTx(tx1)
			block.AddTx(tx2)
		case 2:
			// Block 3 is empty but was mined by account #2.
			block.SetCoinbase(acc2Addr)
			block.SetExtra([]byte("yeehaw"))
		case 3:
			// Block 4 includes blocks 2 and 3 as uncle headers (with modified extra data).
			b2 := block.PrevBlock(1).Header()
			b2.Extra = []byte("foo")
			block.AddUncle(b2)
			b3 := block.PrevBlock(2).Header()
			b3.Extra = []byte("foo")
			block.AddUncle(b3)
		}
	}
	// Assemble the test environment
	pm, db := newTestProtocolManagerMust(t, downloader.FullSync, 4, generator, nil)
	peer, _ := newTestPeer("peer", protocol, pm, true)
	defer peer.close()

	// Fetch for now the entire chain db
	hashes := []common.Hash{}

	it := db.NewIterator(nil, nil)
	for it.Next() {
		if key := it.Key(); len(key) == common.HashLength {
			hashes = append(hashes, common.BytesToHash(key))
		}
	}
	it.Release()
=======
// Has returns an indicator whether txpool has a transaction
// cached with the given hash.
func (p *testTxPool) Has(hash common.Hash) bool {
	p.lock.Lock()
	defer p.lock.Unlock()
>>>>>>> v1.10.7

	return p.pool[hash] != nil
}

<<<<<<< HEAD
// Tests that the transaction receipts can be retrieved based on hashes.
func TestGetReceipt64(t *testing.T) { testGetReceipt(t, 64) }
func TestGetReceipt65(t *testing.T) { testGetReceipt(t, 65) }

func testGetReceipt(t *testing.T, protocol int) {
	// Define three accounts to simulate transactions with
	acc1Key, _ := crypto.HexToECDSA("8a1f9a8f95be41cd7ccb6168179afb4504aefe388d1e14474d32c45c72ce7b7a")
	acc2Key, _ := crypto.HexToECDSA("49a7b37aa6f6645917e7b807e9d1c00d4fa71f18343b0d4122a4d2df64dd6fee")
	acc1Addr := crypto.PubkeyToAddress(acc1Key.PublicKey)
	acc2Addr := crypto.PubkeyToAddress(acc2Key.PublicKey)

	signer := types.HomesteadSigner{}
	// Create a chain generator with some simple transactions (blatantly stolen from @fjl/chain_markets_test)
	generator := func(i int, block *core.BlockGen) {
		switch i {
		case 0:
			// In block 1, the test bank sends account #1 some ether.
			tx, _ := types.SignTx(types.NewTransaction(block.TxNonce(testBank), acc1Addr, big.NewInt(10000), params.TxGas, nil, nil, nil, nil, nil), signer, testBankKey)
			block.AddTx(tx)
		case 1:
			// In block 2, the test bank sends some more ether to account #1.
			// acc1Addr passes it on to account #2.
			tx1, _ := types.SignTx(types.NewTransaction(block.TxNonce(testBank), acc1Addr, big.NewInt(1000), params.TxGas, nil, nil, nil, nil, nil), signer, testBankKey)
			tx2, _ := types.SignTx(types.NewTransaction(block.TxNonce(acc1Addr), acc2Addr, big.NewInt(1000), params.TxGas, nil, nil, nil, nil, nil), signer, acc1Key)
			block.AddTx(tx1)
			block.AddTx(tx2)
		case 2:
			// Block 3 is empty but was mined by account #2.
			block.SetCoinbase(acc2Addr)
			block.SetExtra([]byte("yeehaw"))
		}
	}
	// Assemble the test environment
	pm, _ := newTestProtocolManagerMust(t, downloader.FullSync, 4, generator, nil)
	peer, _ := newTestPeer("peer", protocol, pm, true)
	defer peer.close()

	// Collect the hashes to request, and the response to expect
	hashes, receipts := []common.Hash{}, []types.Receipts{}
	for i := uint64(0); i <= pm.blockchain.CurrentBlock().NumberU64(); i++ {
		block := pm.blockchain.GetBlockByNumber(i)
||||||| e78727290
// Tests that the transaction receipts can be retrieved based on hashes.
func TestGetReceipt63(t *testing.T) { testGetReceipt(t, 63) }
func TestGetReceipt64(t *testing.T) { testGetReceipt(t, 64) }

func testGetReceipt(t *testing.T, protocol int) {
	// Define three accounts to simulate transactions with
	acc1Key, _ := crypto.HexToECDSA("8a1f9a8f95be41cd7ccb6168179afb4504aefe388d1e14474d32c45c72ce7b7a")
	acc2Key, _ := crypto.HexToECDSA("49a7b37aa6f6645917e7b807e9d1c00d4fa71f18343b0d4122a4d2df64dd6fee")
	acc1Addr := crypto.PubkeyToAddress(acc1Key.PublicKey)
	acc2Addr := crypto.PubkeyToAddress(acc2Key.PublicKey)

	signer := types.HomesteadSigner{}
	// Create a chain generator with some simple transactions (blatantly stolen from @fjl/chain_markets_test)
	generator := func(i int, block *core.BlockGen) {
		switch i {
		case 0:
			// In block 1, the test bank sends account #1 some ether.
			tx, _ := types.SignTx(types.NewTransaction(block.TxNonce(testBank), acc1Addr, big.NewInt(10000), params.TxGas, nil, nil), signer, testBankKey)
			block.AddTx(tx)
		case 1:
			// In block 2, the test bank sends some more ether to account #1.
			// acc1Addr passes it on to account #2.
			tx1, _ := types.SignTx(types.NewTransaction(block.TxNonce(testBank), acc1Addr, big.NewInt(1000), params.TxGas, nil, nil), signer, testBankKey)
			tx2, _ := types.SignTx(types.NewTransaction(block.TxNonce(acc1Addr), acc2Addr, big.NewInt(1000), params.TxGas, nil, nil), signer, acc1Key)
			block.AddTx(tx1)
			block.AddTx(tx2)
		case 2:
			// Block 3 is empty but was mined by account #2.
			block.SetCoinbase(acc2Addr)
			block.SetExtra([]byte("yeehaw"))
		case 3:
			// Block 4 includes blocks 2 and 3 as uncle headers (with modified extra data).
			b2 := block.PrevBlock(1).Header()
			b2.Extra = []byte("foo")
			block.AddUncle(b2)
			b3 := block.PrevBlock(2).Header()
			b3.Extra = []byte("foo")
			block.AddUncle(b3)
		}
	}
	// Assemble the test environment
	pm, _ := newTestProtocolManagerMust(t, downloader.FullSync, 4, generator, nil)
	peer, _ := newTestPeer("peer", protocol, pm, true)
	defer peer.close()

	// Collect the hashes to request, and the response to expect
	hashes, receipts := []common.Hash{}, []types.Receipts{}
	for i := uint64(0); i <= pm.blockchain.CurrentBlock().NumberU64(); i++ {
		block := pm.blockchain.GetBlockByNumber(i)
=======
// Get retrieves the transaction from local txpool with given
// tx hash.
func (p *testTxPool) Get(hash common.Hash) *types.Transaction {
	p.lock.Lock()
	defer p.lock.Unlock()
>>>>>>> v1.10.7

<<<<<<< HEAD
		hashes = append(hashes, block.Hash())
		receipts = append(receipts, pm.blockchain.GetReceiptsByHash(block.Hash()))
	}
	// Send the hash request and verify the response
	p2p.Send(peer.app, 0x0f, hashes)
	if err := p2p.ExpectMsg(peer.app, 0x10, receipts); err != nil {
		t.Fatalf("receipts mismatch: %v", err)
	}
||||||| e78727290
		hashes = append(hashes, block.Hash())
		receipts = append(receipts, pm.blockchain.GetReceiptsByHash(block.Hash()))
	}
	// Send the hash request and verify the response
	p2p.Send(peer.app, 0x0f, hashes)
	if err := p2p.ExpectMsg(peer.app, 0x10, receipts); err != nil {
		t.Errorf("receipts mismatch: %v", err)
	}
=======
	return p.pool[hash]
>>>>>>> v1.10.7
}

// AddRemotes appends a batch of transactions to the pool, and notifies any
// listeners if the addition channel is non nil
func (p *testTxPool) AddRemotes(txs []*types.Transaction) []error {
	p.lock.Lock()
	defer p.lock.Unlock()

	for _, tx := range txs {
		p.pool[tx.Hash()] = tx
	}
	p.txFeed.Send(core.NewTxsEvent{Txs: txs})
	return make([]error, len(txs))
}

// Pending returns all the transactions known to the pool
func (p *testTxPool) Pending(enforceTips bool) (map[common.Address]types.Transactions, error) {
	p.lock.RLock()
	defer p.lock.RUnlock()

<<<<<<< HEAD
	// Initialize a chain and generate a fake CHT if checkpointing is enabled
	var (
		db     = rawdb.NewMemoryDatabase()
		config = new(params.ChainConfig)
	)
	(&core.Genesis{Config: config}).MustCommit(db) // Commit genesis block
	// If checkpointing is enabled, create and inject a fake CHT and the corresponding
	// chllenge response.
	var response *types.Header
	var cht *params.TrustedCheckpoint
	if checkpoint {
		index := uint64(rand.Intn(500))
		number := (index+1)*params.CHTFrequency - 1
		response = &types.Header{Number: big.NewInt(int64(number)), Extra: []byte("valid")}

		cht = &params.TrustedCheckpoint{
			SectionIndex: index,
			SectionHead:  response.Hash(),
		}
	}
	// Create a checkpoint aware protocol manager
	blockchain, err := core.NewBlockChain(db, nil, config, mockEngine.NewFaker(), vm.Config{}, nil, nil)
	if err != nil {
		t.Fatalf("failed to create new blockchain: %v", err)
||||||| e78727290
	// Initialize a chain and generate a fake CHT if checkpointing is enabled
	var (
		db     = rawdb.NewMemoryDatabase()
		config = new(params.ChainConfig)
	)
	(&core.Genesis{Config: config}).MustCommit(db) // Commit genesis block
	// If checkpointing is enabled, create and inject a fake CHT and the corresponding
	// chllenge response.
	var response *types.Header
	var cht *params.TrustedCheckpoint
	if checkpoint {
		index := uint64(rand.Intn(500))
		number := (index+1)*params.CHTFrequency - 1
		response = &types.Header{Number: big.NewInt(int64(number)), Extra: []byte("valid")}

		cht = &params.TrustedCheckpoint{
			SectionIndex: index,
			SectionHead:  response.Hash(),
		}
	}
	// Create a checkpoint aware protocol manager
	blockchain, err := core.NewBlockChain(db, nil, config, ethash.NewFaker(), vm.Config{}, nil, nil)
	if err != nil {
		t.Fatalf("failed to create new blockchain: %v", err)
=======
	batches := make(map[common.Address]types.Transactions)
	for _, tx := range p.pool {
		from, _ := types.Sender(types.HomesteadSigner{}, tx)
		batches[from] = append(batches[from], tx)
>>>>>>> v1.10.7
	}
<<<<<<< HEAD
	pm, err := NewProtocolManager(config, cht, syncmode, DefaultConfig.NetworkId, new(event.TypeMux), &testTxPool{pool: make(map[common.Hash]*types.Transaction)}, mockEngine.NewFaker(), blockchain, db, 1, nil, nil, nil)
	if err != nil {
		t.Fatalf("failed to start test protocol manager: %v", err)
	}
	pm.Start(1000)
	defer pm.Stop()

	// Connect a new peer and check that we receive the checkpoint challenge
	peer, _ := newTestPeer("peer", istanbul.Celo64, pm, true)
	defer peer.close()

	if checkpoint {
		challenge := &getBlockHeadersData{
			Origin:  hashOrNumber{Number: response.Number.Uint64()},
			Amount:  1,
			Skip:    0,
			Reverse: false,
		}
		if err := p2p.ExpectMsg(peer.app, GetBlockHeadersMsg, challenge); err != nil {
			t.Fatalf("challenge mismatch: %v", err)
		}
		// Create a block to reply to the challenge if no timeout is simulated
		if !timeout {
			if empty {
				if err := p2p.Send(peer.app, BlockHeadersMsg, []*types.Header{}); err != nil {
					t.Fatalf("failed to answer challenge: %v", err)
				}
			} else if match {
				if err := p2p.Send(peer.app, BlockHeadersMsg, []*types.Header{response}); err != nil {
					t.Fatalf("failed to answer challenge: %v", err)
				}
			} else {
				if err := p2p.Send(peer.app, BlockHeadersMsg, []*types.Header{{Number: response.Number}}); err != nil {
					t.Fatalf("failed to answer challenge: %v", err)
				}
			}
		}
	}
	// Wait until the test timeout passes to ensure proper cleanup
	time.Sleep(syncChallengeTimeout + 300*time.Millisecond)

	// Verify that the remote peer is maintained or dropped
	if drop {
		if peers := pm.peers.Len(); peers != 0 {
			t.Fatalf("peer count mismatch: have %d, want %d", peers, 0)
		}
	} else {
		if peers := pm.peers.Len(); peers != 1 {
			t.Fatalf("peer count mismatch: have %d, want %d", peers, 1)
		}
||||||| e78727290
	pm, err := NewProtocolManager(config, cht, syncmode, DefaultConfig.NetworkId, new(event.TypeMux), &testTxPool{pool: make(map[common.Hash]*types.Transaction)}, ethash.NewFaker(), blockchain, db, 1, nil)
	if err != nil {
		t.Fatalf("failed to start test protocol manager: %v", err)
	}
	pm.Start(1000)
	defer pm.Stop()

	// Connect a new peer and check that we receive the checkpoint challenge
	peer, _ := newTestPeer("peer", eth63, pm, true)
	defer peer.close()

	if checkpoint {
		challenge := &getBlockHeadersData{
			Origin:  hashOrNumber{Number: response.Number.Uint64()},
			Amount:  1,
			Skip:    0,
			Reverse: false,
		}
		if err := p2p.ExpectMsg(peer.app, GetBlockHeadersMsg, challenge); err != nil {
			t.Fatalf("challenge mismatch: %v", err)
		}
		// Create a block to reply to the challenge if no timeout is simulated
		if !timeout {
			if empty {
				if err := p2p.Send(peer.app, BlockHeadersMsg, []*types.Header{}); err != nil {
					t.Fatalf("failed to answer challenge: %v", err)
				}
			} else if match {
				if err := p2p.Send(peer.app, BlockHeadersMsg, []*types.Header{response}); err != nil {
					t.Fatalf("failed to answer challenge: %v", err)
				}
			} else {
				if err := p2p.Send(peer.app, BlockHeadersMsg, []*types.Header{{Number: response.Number}}); err != nil {
					t.Fatalf("failed to answer challenge: %v", err)
				}
			}
		}
	}
	// Wait until the test timeout passes to ensure proper cleanup
	time.Sleep(syncChallengeTimeout + 300*time.Millisecond)

	// Verify that the remote peer is maintained or dropped
	if drop {
		if peers := pm.peers.Len(); peers != 0 {
			t.Fatalf("peer count mismatch: have %d, want %d", peers, 0)
		}
	} else {
		if peers := pm.peers.Len(); peers != 1 {
			t.Fatalf("peer count mismatch: have %d, want %d", peers, 1)
		}
=======
	for _, batch := range batches {
		sort.Sort(types.TxByNonce(batch))
>>>>>>> v1.10.7
	}
	return batches, nil
}

// SubscribeNewTxsEvent should return an event subscription of NewTxsEvent and
// send events to the given channel.
func (p *testTxPool) SubscribeNewTxsEvent(ch chan<- core.NewTxsEvent) event.Subscription {
	return p.txFeed.Subscribe(ch)
}

<<<<<<< HEAD
func testBroadcastBlock(t *testing.T, totalPeers, broadcastExpected int) {
	var (
		evmux   = new(event.TypeMux)
		pow     = mockEngine.NewFaker()
		db      = rawdb.NewMemoryDatabase()
		config  = &params.ChainConfig{}
		gspec   = &core.Genesis{Config: config}
		genesis = gspec.MustCommit(db)
	)
	blockchain, err := core.NewBlockChain(db, nil, config, pow, vm.Config{}, nil, nil)
	if err != nil {
		t.Fatalf("failed to create new blockchain: %v", err)
	}
	pm, err := NewProtocolManager(config, nil, downloader.FullSync, DefaultConfig.NetworkId, evmux, &testTxPool{pool: make(map[common.Hash]*types.Transaction)}, pow, blockchain, db, 1, nil, nil, nil)
	if err != nil {
		t.Fatalf("failed to start test protocol manager: %v", err)
	}
	pm.Start(1000)
	defer pm.Stop()
	var peers []*testPeer
	for i := 0; i < totalPeers; i++ {
		peer, _ := newTestPeer(fmt.Sprintf("peer %d", i), istanbul.Celo64, pm, true)
		defer peer.close()

		peers = append(peers, peer)
	}
	chain, _ := core.GenerateChain(gspec.Config, genesis, mockEngine.NewFaker(), db, 1, func(i int, gen *core.BlockGen) {})
	pm.BroadcastBlock(chain[0], true /*propagate*/)

	errCh := make(chan error, totalPeers)
	doneCh := make(chan struct{}, totalPeers)
	for _, peer := range peers {
		go func(p *testPeer) {
			if err := p2p.ExpectMsg(p.app, NewBlockMsg, &newBlockData{Block: chain[0], TD: big.NewInt(2)}); err != nil {
				errCh <- err
			} else {
				doneCh <- struct{}{}
			}
		}(peer)
	}
	var received int
	for {
		select {
		case <-doneCh:
			received++
			if received > broadcastExpected {
				// We can bail early here
				t.Errorf("broadcast count mismatch: have %d > want %d", received, broadcastExpected)
				return
			}
		case <-time.After(2 * time.Second):
			if received != broadcastExpected {
				t.Errorf("broadcast count mismatch: have %d, want %d", received, broadcastExpected)
			}
			return
		case err = <-errCh:
			t.Fatalf("broadcast failed: %v", err)
		}
	}
||||||| e78727290
func testBroadcastBlock(t *testing.T, totalPeers, broadcastExpected int) {
	var (
		evmux   = new(event.TypeMux)
		pow     = ethash.NewFaker()
		db      = rawdb.NewMemoryDatabase()
		config  = &params.ChainConfig{}
		gspec   = &core.Genesis{Config: config}
		genesis = gspec.MustCommit(db)
	)
	blockchain, err := core.NewBlockChain(db, nil, config, pow, vm.Config{}, nil, nil)
	if err != nil {
		t.Fatalf("failed to create new blockchain: %v", err)
	}
	pm, err := NewProtocolManager(config, nil, downloader.FullSync, DefaultConfig.NetworkId, evmux, &testTxPool{pool: make(map[common.Hash]*types.Transaction)}, pow, blockchain, db, 1, nil)
	if err != nil {
		t.Fatalf("failed to start test protocol manager: %v", err)
	}
	pm.Start(1000)
	defer pm.Stop()
	var peers []*testPeer
	for i := 0; i < totalPeers; i++ {
		peer, _ := newTestPeer(fmt.Sprintf("peer %d", i), eth63, pm, true)
		defer peer.close()

		peers = append(peers, peer)
	}
	chain, _ := core.GenerateChain(gspec.Config, genesis, ethash.NewFaker(), db, 1, func(i int, gen *core.BlockGen) {})
	pm.BroadcastBlock(chain[0], true /*propagate*/)

	errCh := make(chan error, totalPeers)
	doneCh := make(chan struct{}, totalPeers)
	for _, peer := range peers {
		go func(p *testPeer) {
			if err := p2p.ExpectMsg(p.app, NewBlockMsg, &newBlockData{Block: chain[0], TD: big.NewInt(131136)}); err != nil {
				errCh <- err
			} else {
				doneCh <- struct{}{}
			}
		}(peer)
	}
	var received int
	for {
		select {
		case <-doneCh:
			received++
			if received > broadcastExpected {
				// We can bail early here
				t.Errorf("broadcast count mismatch: have %d > want %d", received, broadcastExpected)
				return
			}
		case <-time.After(2 * time.Second):
			if received != broadcastExpected {
				t.Errorf("broadcast count mismatch: have %d, want %d", received, broadcastExpected)
			}
			return
		case err = <-errCh:
			t.Fatalf("broadcast failed: %v", err)
		}
	}
=======
// testHandler is a live implementation of the Ethereum protocol handler, just
// preinitialized with some sane testing defaults and the transaction pool mocked
// out.
type testHandler struct {
	db      ethdb.Database
	chain   *core.BlockChain
	txpool  *testTxPool
	handler *handler
}
>>>>>>> v1.10.7

// newTestHandler creates a new handler for testing purposes with no blocks.
func newTestHandler() *testHandler {
	return newTestHandlerWithBlocks(0)
}

<<<<<<< HEAD
// Tests that a propagated malformed block (uncles or transactions don't match
// with the hashes in the header) gets discarded and not broadcast forward.
func TestBroadcastMalformedBlock(t *testing.T) {
	// Create a live node to test propagation with
	var (
		engine  = mockEngine.NewFaker()
		db      = rawdb.NewMemoryDatabase()
		config  = &params.ChainConfig{}
		gspec   = &core.Genesis{Config: config}
		genesis = gspec.MustCommit(db)
	)
	blockchain, err := core.NewBlockChain(db, nil, config, engine, vm.Config{}, nil, nil)
	if err != nil {
		t.Fatalf("failed to create new blockchain: %v", err)
||||||| e78727290
// Tests that a propagated malformed block (uncles or transactions don't match
// with the hashes in the header) gets discarded and not broadcast forward.
func TestBroadcastMalformedBlock(t *testing.T) {
	// Create a live node to test propagation with
	var (
		engine  = ethash.NewFaker()
		db      = rawdb.NewMemoryDatabase()
		config  = &params.ChainConfig{}
		gspec   = &core.Genesis{Config: config}
		genesis = gspec.MustCommit(db)
	)
	blockchain, err := core.NewBlockChain(db, nil, config, engine, vm.Config{}, nil, nil)
	if err != nil {
		t.Fatalf("failed to create new blockchain: %v", err)
=======
// newTestHandlerWithBlocks creates a new handler for testing purposes, with a
// given number of initial blocks.
func newTestHandlerWithBlocks(blocks int) *testHandler {
	// Create a database pre-initialize with a genesis block
	db := rawdb.NewMemoryDatabase()
	(&core.Genesis{
		Config: params.TestChainConfig,
		Alloc:  core.GenesisAlloc{testAddr: {Balance: big.NewInt(1000000)}},
	}).MustCommit(db)

	chain, _ := core.NewBlockChain(db, nil, params.TestChainConfig, ethash.NewFaker(), vm.Config{}, nil, nil)

	bs, _ := core.GenerateChain(params.TestChainConfig, chain.Genesis(), ethash.NewFaker(), db, blocks, nil)
	if _, err := chain.InsertChain(bs); err != nil {
		panic(err)
	}
	txpool := newTestTxPool()

	handler, _ := newHandler(&handlerConfig{
		Database:   db,
		Chain:      chain,
		TxPool:     txpool,
		Network:    1,
		Sync:       downloader.FastSync,
		BloomCache: 1,
	})
	handler.Start(1000)

	return &testHandler{
		db:      db,
		chain:   chain,
		txpool:  txpool,
		handler: handler,
>>>>>>> v1.10.7
	}
<<<<<<< HEAD
	pm, err := NewProtocolManager(config, nil, downloader.FullSync, DefaultConfig.NetworkId, new(event.TypeMux), new(testTxPool), engine, blockchain, db, 1, nil, nil, nil)
	if err != nil {
		t.Fatalf("failed to start test protocol manager: %v", err)
	}
	pm.Start(2)
	defer pm.Stop()

	// Create two peers, one to send the malformed block with and one to check
	// propagation
	source, _ := newTestPeer("source", istanbul.Celo64, pm, true)
	defer source.close()

	sink, _ := newTestPeer("sink", istanbul.Celo64, pm, true)
	defer sink.close()

	// Create various combinations of malformed blocks
	chain, _ := core.GenerateChain(gspec.Config, genesis, mockEngine.NewFaker(), db, 1, func(i int, gen *core.BlockGen) {})

	malformedTransactions := chain[0].Header()
	malformedTransactions.TxHash[0]++
||||||| e78727290
	pm, err := NewProtocolManager(config, nil, downloader.FullSync, DefaultConfig.NetworkId, new(event.TypeMux), new(testTxPool), engine, blockchain, db, 1, nil)
	if err != nil {
		t.Fatalf("failed to start test protocol manager: %v", err)
	}
	pm.Start(2)
	defer pm.Stop()

	// Create two peers, one to send the malformed block with and one to check
	// propagation
	source, _ := newTestPeer("source", eth63, pm, true)
	defer source.close()

	sink, _ := newTestPeer("sink", eth63, pm, true)
	defer sink.close()

	// Create various combinations of malformed blocks
	chain, _ := core.GenerateChain(gspec.Config, genesis, ethash.NewFaker(), db, 1, func(i int, gen *core.BlockGen) {})

	malformedUncles := chain[0].Header()
	malformedUncles.UncleHash[0]++
	malformedTransactions := chain[0].Header()
	malformedTransactions.TxHash[0]++
	malformedEverything := chain[0].Header()
	malformedEverything.UncleHash[0]++
	malformedEverything.TxHash[0]++
=======
}
>>>>>>> v1.10.7

<<<<<<< HEAD
	// Keep listening to broadcasts and notify if any arrives
	notify := make(chan struct{}, 1)
	go func() {
		if _, err := sink.app.ReadMsg(); err == nil {
			notify <- struct{}{}
		}
	}()
	// Try to broadcast all malformations and ensure they all get discarded
	for _, header := range []*types.Header{malformedTransactions} {
		block := types.NewBlockWithHeader(header).WithBody(chain[0].Transactions(), nil, nil)
		if err := p2p.Send(source.app, NewBlockMsg, []interface{}{block, big.NewInt(131136)}); err != nil {
			t.Fatalf("failed to broadcast block: %v", err)
		}
		select {
		case <-notify:
			t.Fatalf("malformed block forwarded")
		case <-time.After(100 * time.Millisecond):
		}
	}
||||||| e78727290
	// Keep listening to broadcasts and notify if any arrives
	notify := make(chan struct{}, 1)
	go func() {
		if _, err := sink.app.ReadMsg(); err == nil {
			notify <- struct{}{}
		}
	}()
	// Try to broadcast all malformations and ensure they all get discarded
	for _, header := range []*types.Header{malformedUncles, malformedTransactions, malformedEverything} {
		block := types.NewBlockWithHeader(header).WithBody(chain[0].Transactions(), chain[0].Uncles())
		if err := p2p.Send(source.app, NewBlockMsg, []interface{}{block, big.NewInt(131136)}); err != nil {
			t.Fatalf("failed to broadcast block: %v", err)
		}
		select {
		case <-notify:
			t.Fatalf("malformed block forwarded")
		case <-time.After(100 * time.Millisecond):
		}
	}
=======
// close tears down the handler and all its internal constructs.
func (b *testHandler) close() {
	b.handler.Stop()
	b.chain.Stop()
>>>>>>> v1.10.7
}
