// Copyright 2016 The go-ethereum Authors
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

package les

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/consensus/istanbul"
	istanbulBackend "github.com/ethereum/go-ethereum/consensus/istanbul/backend"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/crypto"
	blscrypto "github.com/ethereum/go-ethereum/crypto/bls"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/eth/downloader"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/light"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/rlp"
)

var (
	nodeKeys   = make([]*ecdsa.PrivateKey, 3)
	validators = make([]istanbul.ValidatorData, 3)
	blockchain *core.BlockChain
	engine     *istanbulBackend.Backend
)

type odrTestFn func(ctx context.Context, db ethdb.Database, config *params.ChainConfig, bc *core.BlockChain, lc *light.LightChain, bhash common.Hash) []byte
type odrTestFnNum func(ctx context.Context, db ethdb.Database, bc *core.BlockChain, lc *light.LightChain, origin light.BlockHashOrNumber) ([]byte, error)

func TestOdrGetBlockLes2(t *testing.T) { testOdr(t, 2, 1, true, odrGetBlock) }
func TestOdrGetBlockLes3(t *testing.T) { testOdr(t, 3, 1, true, odrGetBlock) }

func TestOdrGetBlockLightest(t *testing.T) { testLightestChainOdr(t, 3, odrGetBlockHashOrNumber) }

func odrGetBlock(ctx context.Context, db ethdb.Database, config *params.ChainConfig, bc *core.BlockChain, lc *light.LightChain, bhash common.Hash) []byte {
	var block *types.Block
	if bc != nil {
		block = bc.GetBlockByHash(bhash)
	} else {
		block, _ = lc.GetBlockByHash(ctx, bhash)
	}
	if block == nil {
		return nil
	}
	rlp, _ := rlp.EncodeToBytes(block)
	return rlp
}

func odrGetBlockHashOrNumber(ctx context.Context, db ethdb.Database, bc *core.BlockChain, lc *light.LightChain, origin light.BlockHashOrNumber) ([]byte, error) {
	var block *types.Block
	var err error
	if bc != nil {
		if origin.Hash != (common.Hash{}) {
			block = bc.GetBlockByHash(origin.Hash)
		} else {
			block = bc.GetBlockByNumber(*origin.Number)
		}
	} else {
		if origin.Hash != (common.Hash{}) {
			block, err = lc.GetBlockByHash(ctx, origin.Hash)
		} else {
			block, err = lc.GetBlockByNumber(ctx, *origin.Number)
		}
		if err != nil {
			return nil, err
		}
	}
	if block == nil {
		return nil, nil
	}
	rlp, _ := rlp.EncodeToBytes(block)
	return rlp, nil
}

func TestOdrGetReceiptsLes2(t *testing.T) { testOdr(t, 2, 1, true, odrGetReceipts) }
func TestOdrGetReceiptsLes3(t *testing.T) { testOdr(t, 3, 1, true, odrGetReceipts) }

func odrGetReceipts(ctx context.Context, db ethdb.Database, config *params.ChainConfig, bc *core.BlockChain, lc *light.LightChain, bhash common.Hash) []byte {
	var receipts types.Receipts
	if bc != nil {
		if number := rawdb.ReadHeaderNumber(db, bhash); number != nil {
			receipts = rawdb.ReadReceipts(db, bhash, *number, config)
		}
	} else {
		if number := rawdb.ReadHeaderNumber(db, bhash); number != nil {
			receipts, _ = light.GetBlockReceipts(ctx, lc.Odr(), bhash, *number)
		}
	}
	if receipts == nil {
		return nil
	}
	rlp, _ := rlp.EncodeToBytes(receipts)
	return rlp
}

func TestOdrAccountsLes2(t *testing.T) { testOdr(t, 2, 1, true, odrAccounts) }
func TestOdrAccountsLes3(t *testing.T) { testOdr(t, 3, 1, true, odrAccounts) }

func odrAccounts(ctx context.Context, db ethdb.Database, config *params.ChainConfig, bc *core.BlockChain, lc *light.LightChain, bhash common.Hash) []byte {
	dummyAddr := common.HexToAddress("1234567812345678123456781234567812345678")
	acc := []common.Address{bankAddr, userAddr1, userAddr2, dummyAddr}

	var (
		res []byte
		st  *state.StateDB
		err error
	)
	for _, addr := range acc {
		if bc != nil {
			header := bc.GetHeaderByHash(bhash)
			st, err = state.New(header.Root, state.NewDatabase(db))
		} else {
			header := lc.GetHeaderByHash(bhash)
			st = light.NewState(ctx, header, lc.Odr())
		}
		if err == nil {
			bal := st.GetBalance(addr)
			rlp, _ := rlp.EncodeToBytes(bal)
			res = append(res, rlp...)
		}
	}
	return res
}

func TestOdrContractCallLes2(t *testing.T) { testOdr(t, 2, 2, true, odrContractCall) }
func TestOdrContractCallLes3(t *testing.T) { testOdr(t, 3, 2, true, odrContractCall) }

type callmsg struct {
	types.Message
}

func (callmsg) CheckNonce() bool { return false }

func odrContractCall(ctx context.Context, db ethdb.Database, config *params.ChainConfig, bc *core.BlockChain, lc *light.LightChain, bhash common.Hash) []byte {
	data := common.Hex2Bytes("60CD26850000000000000000000000000000000000000000000000000000000000000000")

	var res []byte
	for i := 0; i < 3; i++ {
		data[35] = byte(i)
		if bc != nil {
			header := bc.GetHeaderByHash(bhash)
			statedb, err := state.New(header.Root, state.NewDatabase(db))

			if err == nil {
				from := statedb.GetOrNewStateObject(bankAddr)
				from.SetBalance(math.MaxBig256)

				msg := callmsg{types.NewMessage(from.Address(), &testContractAddr, 0, new(big.Int), 100000, new(big.Int), nil, nil, new(big.Int), data, false)}

				context := vm.NewEVMContext(msg, header, bc, nil)
				vmenv := vm.NewEVM(context, statedb, config, vm.Config{})

				//vmenv := core.NewEnv(statedb, config, bc, msg, header, vm.Config{})
				gp := new(core.GasPool).AddGas(math.MaxUint64)
				ret, _, _, _ := core.ApplyMessage(vmenv, msg, gp)
				res = append(res, ret...)
			}
		} else {
			header := lc.GetHeaderByHash(bhash)
			state := light.NewState(ctx, header, lc.Odr())
			state.SetBalance(bankAddr, math.MaxBig256)
			msg := callmsg{types.NewMessage(bankAddr, &testContractAddr, 0, new(big.Int), 100000, new(big.Int), nil, nil, new(big.Int), data, false)}
			context := vm.NewEVMContext(msg, header, lc, nil)
			vmenv := vm.NewEVM(context, state, config, vm.Config{})
			gp := new(core.GasPool).AddGas(math.MaxUint64)
			ret, _, _, _ := core.ApplyMessage(vmenv, msg, gp)
			if state.Error() == nil {
				res = append(res, ret...)
			}
		}
	}
	return res
}

func TestOdrTxStatusLes2(t *testing.T) { testOdr(t, 2, 1, false, odrTxStatus) }
func TestOdrTxStatusLes3(t *testing.T) { testOdr(t, 3, 1, false, odrTxStatus) }

func odrTxStatus(ctx context.Context, db ethdb.Database, config *params.ChainConfig, bc *core.BlockChain, lc *light.LightChain, bhash common.Hash) []byte {
	var txs types.Transactions
	if bc != nil {
		block := bc.GetBlockByHash(bhash)
		txs = block.Transactions()
	} else {
		if block, _ := lc.GetBlockByHash(ctx, bhash); block != nil {
			btxs := block.Transactions()
			txs = make(types.Transactions, len(btxs))
			for i, tx := range btxs {
				var err error
				txs[i], _, _, _, err = light.GetTransaction(ctx, lc.Odr(), tx.Hash())
				if err != nil {
					return nil
				}
			}
		}
	}
	rlp, _ := rlp.EncodeToBytes(txs)
	return rlp
}

// testOdr tests odr requests whose validation guaranteed by block headers.
func testOdr(t *testing.T, protocol int, expFail uint64, checkCached bool, fn odrTestFn) {
	// Assemble the test environment
	server, client, tearDown := newClientServerEnv(t, downloader.LightSync, 4, protocol, nil, nil, 0, false, true, false)
	defer tearDown()

	client.handler.synchronise(client.peer.peer)

	// Ensure the client has synced all necessary data.
	clientHead := client.handler.backend.blockchain.CurrentHeader()
	if clientHead.Number.Uint64() != 4 {
		t.Fatalf("Failed to sync the chain with server, head: %v", clientHead.Number.Uint64())
	}
	// Disable the mechanism that we will wait a few time for request
	// even there is no suitable peer to send right now.
	waitForPeers = 0

	test := func(expFail uint64) {
		// Mark this as a helper to put the failures at the correct lines
		t.Helper()

		for i := uint64(0); i <= server.handler.blockchain.CurrentHeader().Number.Uint64(); i++ {
			bhash := rawdb.ReadCanonicalHash(server.db, i)
			b1 := fn(light.NoOdr, server.db, server.handler.server.chainConfig, server.handler.blockchain, nil, bhash)

			// Set the timeout as 1 second here, ensure there is enough time
			// for travis to make the action.
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			b2 := fn(ctx, client.db, client.handler.backend.chainConfig, nil, client.handler.backend.blockchain, bhash)
			cancel()

			eq := bytes.Equal(b1, b2)
			exp := i < expFail
			if exp && !eq {
				t.Fatalf("odr mismatch: have %x, want %x", b2, b1)
			}
			if !exp && eq {
				t.Fatalf("unexpected odr match")
			}
		}
	}

	// expect retrievals to fail (except genesis block) without a les peer
	client.handler.backend.peers.lock.Lock()
	client.peer.peer.hasBlock = func(common.Hash, *uint64, bool) bool { return false }
	client.handler.backend.peers.lock.Unlock()
	test(expFail)

	// expect all retrievals to pass
	client.handler.backend.peers.lock.Lock()
	client.peer.peer.hasBlock = func(common.Hash, *uint64, bool) bool { return true }
	client.handler.backend.peers.lock.Unlock()
	test(5)

	// still expect all retrievals to pass, now data should be cached locally
	if checkCached {
		client.handler.backend.peers.Unregister(client.peer.peer.id)
		time.Sleep(time.Millisecond * 10) // ensure that all peerSetNotify callbacks are executed
		test(5)
	}
}

func testLightChainGen(i int, block *core.BlockGen) {
	madeBlock, _ := istanbulBackend.MakeBlock(nodeKeys, blockchain, engine, block.PrevBlock(-1))
	block.SetHeader(madeBlock.Header())
}

func testLightestChainOdr(t *testing.T, protocol int, fn odrTestFnNum) {
	var (
		sdb     = rawdb.NewMemoryDatabase()
		ldb     = rawdb.NewMemoryDatabase()
		genesis = core.DefaultGenesisBlock()
	)
	for i := 0; i < 3; i++ {
		var addr common.Address
		nodeKeys[i], _ = crypto.GenerateKey()
		addr = crypto.PubkeyToAddress(nodeKeys[i].PublicKey)
		blsPrivateKey, _ := blscrypto.ECDSAToBLS(nodeKeys[i])
		blsPublicKey, _ := blscrypto.PrivateToPublic(blsPrivateKey)
		validators[i] = istanbul.ValidatorData{
			Address:      addr,
			BLSPublicKey: blsPublicKey,
		}
	}

	istanbulBackend.AppendValidatorsToGenesisBlock(genesis, validators)
	genesis.MustCommit(sdb)
	genesis.MustCommit(ldb)
	engine = istanbulBackend.New(istanbul.DefaultConfig, sdb).(*istanbulBackend.Backend)
	engine.Authorize(validators[0].Address, &nodeKeys[0].PublicKey, nil, istanbulBackend.SignFn(nodeKeys[0]), istanbulBackend.SignBLSFn(nodeKeys[0]))
	blockchain, _ = core.NewBlockChain(sdb, nil, params.DefaultChainConfig, engine, vm.Config{}, nil)
	gchain, _ := core.GenerateChain(params.DefaultChainConfig, genesis.ToBlock(sdb), engine, sdb, 4, testLightChainGen)
	if _, err := blockchain.InsertChain(gchain); err != nil {
		t.Fatal(err)
	}

	databases := []ethdb.Database{sdb, ldb}
	_, client, tearDown := NewIstanbulClientServerEnv(t, downloader.LightSync, 4, protocol, nil, nil, 0, false, true, databases, genesis, engine, nodeKeys)
	// odr := &testOdr{sdb: sdb, ldb: ldb, indexerConfig: TestClientIndexerConfig}
	defer tearDown()
	odr := client.GetOdr()
	lightchain, err := light.NewLightChain(odr, params.DefaultChainConfig, engine, nil)
	if err != nil {
		t.Fatal(err)
	}

	test := func(expFail int, tryHash bool) {
		for i := uint64(0); i <= blockchain.CurrentHeader().Number.Uint64(); i++ {
			bhash := rawdb.ReadCanonicalHash(sdb, i)
			var origin light.BlockHashOrNumber
			if tryHash {
				origin = light.BlockHashOrNumber{Hash: bhash}
			} else {
				origin = light.BlockHashOrNumber{Number: &i}
			}
			b1, err := fn(light.NoOdr, sdb, blockchain, nil, origin)
			if err != nil {
				t.Fatalf("error in full-node test for block %d: %v", i, err)
			}

			ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
			defer cancel()

			exp := i < uint64(expFail)
			b2, err := fn(ctx, ldb, nil, lightchain, origin)
			if err != nil && exp {
				t.Errorf("error in ODR test for block %d: %v", i, err)
			}

			eq := bytes.Equal(b1, b2)
			if exp && !eq {
				t.Errorf("ODR test output for tryHash %t block %d doesn't match full node", tryHash, i)
			}

			header := rawdb.ReadHeader(odr.Database(), bhash, i)
			if exp {
				if header == nil {
					t.Errorf("ODR test for tryHash %t block %d did not properly receive/store header", tryHash, i)
				}
				headers := []*types.Header{header}

				if _, err := lightchain.InsertHeaderChain(headers, 1, true); err != nil {
					t.Errorf("ODR test for tryHash %t block %d could not insert header to headerchain", tryHash, i)
				}
			}
		}
	}
	// expect retrievals to fail (except genesis block) without a les peer
	t.Log("checking without ODR")
	// odr.disable = true
	test(1, true)
	test(1, false)

	// expect all retrievals to pass with ODR enabled
	// t.Log("checking with ODR")
	// odr.disable = false
	// test(len(gchain), true)
	// test(len(gchain), false)

	// // still expect all retrievals to pass, now data should be cached locally
	// t.Log("checking without ODR, should be cached")
	// odr.disable = true
	// test(len(gchain), true)
	// test(len(gchain), false)
}
