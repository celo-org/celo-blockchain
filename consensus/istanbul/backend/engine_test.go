// Copyright 2017 The go-ethereum Authors
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

package backend

import (
	"bytes"
	"crypto/ecdsa"
	"math/big"
	"math/rand"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"
	"time"

	bls "github.com/celo-org/bls-zexe/go"
	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/consensus/istanbul"
	"github.com/ethereum/go-ethereum/contract_comm"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/crypto"
	blscrypto "github.com/ethereum/go-ethereum/crypto/bls"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/params"
)

// in this test, we can set n to 1, and it means we can process Istanbul and commit a
// block by one node. Otherwise, if n is larger than 1, we have to generate
// other fake events to process Istanbul.
func newBlockChain(n int, isFullChain bool) (*core.BlockChain, *Backend) {
	genesis, nodeKeys := getGenesisAndKeys(n, isFullChain)
	memDB := ethdb.NewMemDatabase()
	config := istanbul.DefaultConfig
	// Use the first key as private key
	address := crypto.PubkeyToAddress(nodeKeys[0].PublicKey)
	signerFn := func(_ accounts.Account, data []byte) ([]byte, error) {
		return crypto.Sign(data, nodeKeys[0])
	}

	signerBLSHashFn := func(_ accounts.Account, data []byte) ([]byte, error) {
		key := nodeKeys[0]
		privateKeyBytes, err := blscrypto.ECDSAToBLS(key)
		if err != nil {
			return nil, err
		}

		privateKey, err := bls.DeserializePrivateKey(privateKeyBytes)
		if err != nil {
			return nil, err
		}
		defer privateKey.Destroy()

		signature, err := privateKey.SignMessage(data, []byte{}, false)
		if err != nil {
			return nil, err
		}
		defer signature.Destroy()
		signatureBytes, err := signature.Serialize()
		if err != nil {
			return nil, err
		}

		return signatureBytes, nil
	}

	signerBLSMessageFn := func(_ accounts.Account, data []byte, extraData []byte) ([]byte, error) {
		key := nodeKeys[0]
		privateKeyBytes, err := blscrypto.ECDSAToBLS(key)
		if err != nil {
			return nil, err
		}

		privateKey, err := bls.DeserializePrivateKey(privateKeyBytes)
		if err != nil {
			return nil, err
		}
		defer privateKey.Destroy()

		signature, err := privateKey.SignMessage(data, extraData, true)
		if err != nil {
			return nil, err
		}
		defer signature.Destroy()
		signatureBytes, err := signature.Serialize()
		if err != nil {
			return nil, err
		}

		return signatureBytes, nil
	}

	dataDir := createRandomDataDir()
	b, _ := New(config, memDB, dataDir).(*Backend)
	b.Authorize(address, signerFn, signerBLSHashFn, signerBLSMessageFn)

	genesis.MustCommit(memDB)

	blockchain, err := core.NewBlockChain(memDB, nil, genesis.Config, b, vm.Config{}, nil)
	if err != nil {
		panic(err)
	}

	b.SetChain(blockchain, blockchain.CurrentBlock)

	b.Start(blockchain.HasBadBlock,
		func(parentHash common.Hash) (*state.StateDB, error) {
			parentStateRoot := blockchain.GetHeaderByHash(parentHash).Root
			return blockchain.StateAt(parentStateRoot)
		},
		func(block *types.Block, state *state.StateDB) (types.Receipts, []*types.Log, uint64, error) {
			return blockchain.Processor().Process(block, state, *blockchain.GetVMConfig())
		},
		func(block *types.Block, state *state.StateDB, receipts types.Receipts, usedGas uint64) error {
			return blockchain.Validator().ValidateState(block, nil, state, receipts, usedGas)
		})
	snap, err := b.snapshot(blockchain, 0, common.Hash{}, nil)
	if err != nil {
		panic(err)
	}
	if snap == nil {
		panic("failed to get snapshot")
	}
	proposerAddr := snap.ValSet.GetProposer().Address()

	// find proposer key
	for _, key := range nodeKeys {
		addr := crypto.PubkeyToAddress(key.PublicKey)
		if addr.String() == proposerAddr.String() {
			signerFn := func(_ accounts.Account, data []byte) ([]byte, error) {
				return crypto.Sign(data, key)
			}
			signerBLSHashFn := func(_ accounts.Account, data []byte) ([]byte, error) {
				privateKeyBytes, err := blscrypto.ECDSAToBLS(key)
				if err != nil {
					return nil, err
				}

				privateKey, err := bls.DeserializePrivateKey(privateKeyBytes)
				if err != nil {
					return nil, err
				}
				defer privateKey.Destroy()

				signature, err := privateKey.SignMessage(data, []byte{}, false)
				if err != nil {
					return nil, err
				}
				defer signature.Destroy()
				signatureBytes, err := signature.Serialize()
				if err != nil {
					return nil, err
				}

				return signatureBytes, nil
			}

			signerBLSMessageFn := func(_ accounts.Account, data []byte, extraData []byte) ([]byte, error) {
				privateKeyBytes, err := blscrypto.ECDSAToBLS(key)
				if err != nil {
					return nil, err
				}

				privateKey, err := bls.DeserializePrivateKey(privateKeyBytes)
				if err != nil {
					return nil, err
				}
				defer privateKey.Destroy()

				signature, err := privateKey.SignMessage(data, extraData, true)
				if err != nil {
					return nil, err
				}
				defer signature.Destroy()
				signatureBytes, err := signature.Serialize()
				if err != nil {
					return nil, err
				}

				return signatureBytes, nil
			}

			b.Authorize(address, signerFn, signerBLSHashFn, signerBLSMessageFn)
			b.SetBroadcaster(&MockBroadcaster{privateKey: key})
			break
		}
	}

	contract_comm.SetInternalEVMHandler(blockchain)

	return blockchain, b
}

func createRandomDataDir() string {
	rand.Seed(time.Now().UnixNano())
	for {
		dirName := "geth_ibft_" + strconv.Itoa(rand.Int()%1000000)
		dataDir := filepath.Join("/tmp", dirName)
		err := os.Mkdir(dataDir, 0700)
		if os.IsExist(err) {
			continue // Re-try
		}
		if err != nil {
			panic("Failed to create dir: " + dataDir + " error: " + err.Error())
		}
		return dataDir
	}
}

func getGenesisAndKeys(n int, isFullChain bool) (*core.Genesis, []*ecdsa.PrivateKey) {
	// Setup validators
	var nodeKeys = make([]*ecdsa.PrivateKey, n)
	validators := make([]istanbul.ValidatorData, n)
	for i := 0; i < n; i++ {
		var addr common.Address
		if i == 1 {
			nodeKeys[i], _ = generatePrivateKey()
			addr = getAddress()
		} else {
			nodeKeys[i], _ = crypto.GenerateKey()
			addr = crypto.PubkeyToAddress(nodeKeys[i].PublicKey)
		}
		blsPrivateKey, _ := blscrypto.ECDSAToBLS(nodeKeys[i])
		blsPublicKey, _ := blscrypto.PrivateToPublic(blsPrivateKey)
		validators[i] = istanbul.ValidatorData{
			addr,
			blsPublicKey,
		}

	}

	// generate genesis block
	genesis := core.DefaultGenesisBlock()
	genesis.Config = params.TestChainConfig
	if !isFullChain {
		genesis.Config.FullHeaderChainAvailable = false
	}
	// force enable Istanbul engine
	genesis.Config.Istanbul = &params.IstanbulConfig{}
	genesis.Config.Ethash = nil
	genesis.Difficulty = defaultDifficulty
	genesis.Nonce = emptyNonce.Uint64()
	genesis.Mixhash = types.IstanbulDigest

	AppendValidatorsToGenesisBlock(genesis, validators)
	return genesis, nodeKeys
}

func makeHeader(parent *types.Block, config *istanbul.Config) *types.Header {
	header := &types.Header{
		ParentHash: parent.Hash(),
		Number:     parent.Number().Add(parent.Number(), common.Big1),
		GasLimit:   core.CalcGasLimit(parent, nil),
		GasUsed:    0,
		Extra:      parent.Extra(),
		Time:       new(big.Int).Add(parent.Time(), new(big.Int).SetUint64(config.BlockPeriod)),
		Difficulty: defaultDifficulty,
	}
	return header
}

func makeBlock(chain *core.BlockChain, engine *Backend, parent *types.Block) *types.Block {
	block := makeBlockWithoutSeal(chain, engine, parent)
	results := make(chan *types.Block)
	go func() { engine.Seal(chain, block, results, nil) }()
	block = <-results
	return block
}

func makeBlockWithoutSeal(chain *core.BlockChain, engine *Backend, parent *types.Block) *types.Block {
	header := makeHeader(parent, engine.config)
	engine.Prepare(chain, header)
	state, _ := chain.StateAt(parent.Root())
	block, _ := engine.Finalize(chain, header, state, nil, nil, nil, nil)
	return block
}

func TestPrepare(t *testing.T) {
	chain, engine := newBlockChain(1, true)
	header := makeHeader(chain.Genesis(), engine.config)
	err := engine.Prepare(chain, header)
	if err != nil {
		t.Errorf("error mismatch: have %v, want nil", err)
	}
	header.ParentHash = common.BytesToHash([]byte("1234567890"))
	err = engine.Prepare(chain, header)
	if err != consensus.ErrUnknownAncestor {
		t.Errorf("error mismatch: have %v, want %v", err, consensus.ErrUnknownAncestor)
	}
}

func TestSealReturns(t *testing.T) {
	chain, engine := newBlockChain(2, true)
	block := makeBlockWithoutSeal(chain, engine, chain.Genesis())
	stop := make(chan struct{}, 1)
	results := make(chan *types.Block)
	returns := make(chan struct{}, 1)
	go func() {
		err := engine.Seal(chain, block, results, stop)
		if err != nil {
			t.Errorf("error mismatch: have %v, want nil", err)
		}
		returns <- struct{}{}
	}()

	select {
	case <-returns:
	case <-time.After(time.Second):
		t.Errorf("Never returned from seal")
	}

}
func TestSealStopChannel(t *testing.T) {
	chain, engine := newBlockChain(1, true)
	block := makeBlockWithoutSeal(chain, engine, chain.Genesis())
	stop := make(chan struct{}, 1)
	eventSub := engine.EventMux().Subscribe(istanbul.RequestEvent{})
	eventLoop := func() {
		ev := <-eventSub.Chan()
		_, ok := ev.Data.(istanbul.RequestEvent)
		if !ok {
			t.Errorf("unexpected event comes: %v", reflect.TypeOf(ev.Data))
		}
		stop <- struct{}{}
		eventSub.Unsubscribe()
	}
	go eventLoop()
	results := make(chan *types.Block)

	err := engine.Seal(chain, block, results, stop)
	if err != nil {
		t.Errorf("error mismatch: have %v, want nil", err)
	}

	select {
	case <-results:
		t.Errorf("Did not expect a block")
	case <-time.After(time.Second):
	}
}

func TestSealCommittedOtherHash(t *testing.T) {
	chain, engine := newBlockChain(4, true)
	block := makeBlockWithoutSeal(chain, engine, chain.Genesis())
	otherBlock := makeBlockWithoutSeal(chain, engine, block)
	eventSub := engine.EventMux().Subscribe(istanbul.RequestEvent{})
	eventLoop := func() {
		ev := <-eventSub.Chan()
		_, ok := ev.Data.(istanbul.RequestEvent)
		if !ok {
			t.Errorf("unexpected event comes: %v", reflect.TypeOf(ev.Data))
		}
		engine.Commit(otherBlock, big.NewInt(0), []byte{})
		eventSub.Unsubscribe()
	}
	go eventLoop()
	results := make(chan *types.Block)

	go func() { engine.Seal(chain, block, results, nil) }()
	select {
	case <-results:
		t.Error("seal should not be completed")
	case <-time.After(2 * time.Second):
	}
}

func TestSealCommitted(t *testing.T) {
	chain, engine := newBlockChain(1, true)
	block := makeBlockWithoutSeal(chain, engine, chain.Genesis())
	expectedBlock, _ := engine.updateBlock(engine.chain.GetHeader(block.ParentHash(), block.NumberU64()-1), block)

	results := make(chan *types.Block)
	go func() {
		err := engine.Seal(chain, block, results, nil)
		if err != nil {
			t.Errorf("error mismatch: have %v, want nil", err)
		}
	}()

	finalBlock := <-results
	if finalBlock.Hash() != expectedBlock.Hash() {
		t.Errorf("hash mismatch: have %v, want %v", finalBlock.Hash(), expectedBlock.Hash())
	}
}

func TestVerifyHeader(t *testing.T) {
	chain, engine := newBlockChain(1, true)

	// errEmptyCommittedSeals case
	block := makeBlockWithoutSeal(chain, engine, chain.Genesis())
	block, _ = engine.updateBlock(chain.Genesis().Header(), block)
	err := engine.VerifyHeader(chain, block.Header(), false)
	if err != errEmptyCommittedSeals {
		t.Errorf("error mismatch: have %v, want %v", err, errEmptyCommittedSeals)
	}

	// short extra data
	header := block.Header()
	header.Extra = []byte{}
	err = engine.VerifyHeader(chain, header, false)
	if err != errInvalidExtraDataFormat {
		t.Errorf("error mismatch: have %v, want %v", err, errInvalidExtraDataFormat)
	}
	// incorrect extra format
	header.Extra = []byte("0000000000000000000000000000000012300000000000000000000000000000000000000000000000000000000000000000")
	err = engine.VerifyHeader(chain, header, false)
	if err != errInvalidExtraDataFormat {
		t.Errorf("error mismatch: have %v, want %v", err, errInvalidExtraDataFormat)
	}

	// non zero MixDigest
	block = makeBlockWithoutSeal(chain, engine, chain.Genesis())
	header = block.Header()
	header.MixDigest = common.BytesToHash([]byte("123456789"))
	err = engine.VerifyHeader(chain, header, false)
	if err != errInvalidMixDigest {
		t.Errorf("error mismatch: have %v, want %v", err, errInvalidMixDigest)
	}

	// invalid uncles hash
	block = makeBlockWithoutSeal(chain, engine, chain.Genesis())
	header = block.Header()
	header.UncleHash = common.BytesToHash([]byte("123456789"))
	err = engine.VerifyHeader(chain, header, false)
	if err != errInvalidUncleHash {
		t.Errorf("error mismatch: have %v, want %v", err, errInvalidUncleHash)
	}

	// invalid difficulty
	block = makeBlockWithoutSeal(chain, engine, chain.Genesis())
	header = block.Header()
	header.Difficulty = big.NewInt(2)
	err = engine.VerifyHeader(chain, header, false)
	if err != errInvalidDifficulty {
		t.Errorf("error mismatch: have %v, want %v", err, errInvalidDifficulty)
	}

	// invalid timestamp
	block = makeBlockWithoutSeal(chain, engine, chain.Genesis())
	header = block.Header()
	header.Time = new(big.Int).Add(chain.Genesis().Time(), new(big.Int).SetUint64(engine.config.BlockPeriod-1))
	err = engine.VerifyHeader(chain, header, false)
	if err != errInvalidTimestamp {
		t.Errorf("error mismatch: have %v, want %v", err, errInvalidTimestamp)
	}

	// future block
	block = makeBlockWithoutSeal(chain, engine, chain.Genesis())
	header = block.Header()
	header.Time = new(big.Int).Add(big.NewInt(now().Unix()), new(big.Int).SetUint64(10))
	err = engine.VerifyHeader(chain, header, false)
	if err != consensus.ErrFutureBlock {
		t.Errorf("error mismatch: have %v, want %v", err, consensus.ErrFutureBlock)
	}

	// invalid nonce
	block = makeBlockWithoutSeal(chain, engine, chain.Genesis())
	header = block.Header()
	copy(header.Nonce[:], hexutil.MustDecode("0x111111111111"))
	header.Number = big.NewInt(int64(engine.config.Epoch))
	err = engine.VerifyHeader(chain, header, false)
	if err != errInvalidNonce {
		t.Errorf("error mismatch: have %v, want %v", err, errInvalidNonce)
	}
}

func TestVerifySeal(t *testing.T) {
	chain, engine := newBlockChain(1, true)
	genesis := chain.Genesis()
	// cannot verify genesis
	err := engine.VerifySeal(chain, genesis.Header())
	if err != errUnknownBlock {
		t.Errorf("error mismatch: have %v, want %v", err, errUnknownBlock)
	}

	block := makeBlock(chain, engine, genesis)
	// change block content
	header := block.Header()
	header.Number = big.NewInt(4)
	block1 := block.WithSeal(header)
	err = engine.VerifySeal(chain, block1.Header())
	if err != errUnauthorized {
		t.Errorf("error mismatch: have %v, want %v", err, errUnauthorized)
	}

	// unauthorized users but still can get correct signer address
	engine.Authorize(common.Address{}, nil, nil, nil)
	err = engine.VerifySeal(chain, block.Header())
	if err != nil {
		t.Errorf("error mismatch: have %v, want nil", err)
	}
}

func TestVerifyHeaders(t *testing.T) {
	chain, engine := newBlockChain(1, true)
	genesis := chain.Genesis()

	// success case
	headers := []*types.Header{}
	blocks := []*types.Block{}
	size := 100

	for i := 0; i < size; i++ {
		var b *types.Block
		if i == 0 {
			b = makeBlockWithoutSeal(chain, engine, genesis)
			b, _ = engine.updateBlock(genesis.Header(), b)
		} else {
			b = makeBlockWithoutSeal(chain, engine, blocks[i-1])
			b, _ = engine.updateBlock(blocks[i-1].Header(), b)
		}
		blocks = append(blocks, b)
		headers = append(headers, blocks[i].Header())
	}
	now = func() time.Time {
		return time.Unix(headers[size-1].Time.Int64(), 0)
	}
	_, results := engine.VerifyHeaders(chain, headers, nil)
	const timeoutDura = 2 * time.Second
	timeout := time.NewTimer(timeoutDura)
	index := 0
OUT1:
	for {
		select {
		case err := <-results:
			if err != nil {
				if err != errEmptyCommittedSeals && err != errInvalidCommittedSeals {
					t.Errorf("error mismatch: have %v, want errEmptyCommittedSeals|errInvalidCommittedSeals", err)
					break OUT1
				}
			}
			index++
			if index == size {
				break OUT1
			}
		case <-timeout.C:
			break OUT1
		}
	}
	// abort cases
	abort, results := engine.VerifyHeaders(chain, headers, nil)
	timeout = time.NewTimer(timeoutDura)
	index = 0
OUT2:
	for {
		select {
		case err := <-results:
			if err != nil {
				if err != errEmptyCommittedSeals && err != errInvalidCommittedSeals {
					t.Errorf("error mismatch: have %v, want errEmptyCommittedSeals|errInvalidCommittedSeals", err)
					break OUT2
				}
			}
			index++
			if index == 5 {
				abort <- struct{}{}
			}
			if index >= size {
				t.Errorf("verifyheaders should be aborted")
				break OUT2
			}
		case <-timeout.C:
			break OUT2
		}
	}
	// error header cases
	headers[2].Number = big.NewInt(100)
	abort, results = engine.VerifyHeaders(chain, headers, nil)
	timeout = time.NewTimer(timeoutDura)
	index = 0
	errors := 0
	expectedErrors := 2
OUT3:
	for {
		select {
		case err := <-results:
			if err != nil {
				if err != errEmptyCommittedSeals && err != errInvalidCommittedSeals {
					errors++
				}
			}
			index++
			if index == size {
				if errors != expectedErrors {
					t.Errorf("error mismatch: have %v, want %v", err, expectedErrors)
				}
				break OUT3
			}
		case <-timeout.C:
			break OUT3
		}
	}
}

func TestVerifyHeaderWithoutFullChain(t *testing.T) {
	chain, engine := newBlockChain(1, false)

	// allow future block without full chain available
	block := makeBlockWithoutSeal(chain, engine, chain.Genesis())
	header := block.Header()
	header.Time = new(big.Int).Add(big.NewInt(now().Unix()), new(big.Int).SetUint64(3))
	err := engine.VerifyHeader(chain, header, false)
	if err != errEmptyCommittedSeals {
		t.Errorf("error mismatch: have %v, want %v", err, errEmptyCommittedSeals)
	}

	// reject future block without full chain available
	block = makeBlockWithoutSeal(chain, engine, chain.Genesis())
	header = block.Header()
	header.Time = new(big.Int).Add(big.NewInt(now().Unix()), new(big.Int).SetUint64(10))
	err = engine.VerifyHeader(chain, header, false)
	if err != consensus.ErrFutureBlock {
		t.Errorf("error mismatch: have %v, want %v", err, consensus.ErrFutureBlock)
	}
}

func TestPrepareExtra(t *testing.T) {
	oldValidators := make([]istanbul.ValidatorData, 2)
	oldValidators[0] = istanbul.ValidatorData{
		common.BytesToAddress(hexutil.MustDecode("0x44add0ec310f115a0e603b2d7db9f067778eaf8a")),
		make([]byte, blscrypto.PUBLICKEYBYTES),
	}
	oldValidators[1] = istanbul.ValidatorData{
		common.BytesToAddress(hexutil.MustDecode("0x294fc7e8f22b3bcdcf955dd7ff3ba2ed833f8212")),
		make([]byte, blscrypto.PUBLICKEYBYTES),
	}

	newValidators := make([]istanbul.ValidatorData, 2)
	newValidators[0] = istanbul.ValidatorData{
		common.BytesToAddress(hexutil.MustDecode("0x6beaaed781d2d2ab6350f5c4566a2c6eaac407a6")),
		make([]byte, blscrypto.PUBLICKEYBYTES),
	}
	newValidators[1] = istanbul.ValidatorData{
		common.BytesToAddress(hexutil.MustDecode("0x8be76812f765c24641ec63dc2852b378aba2b440")),
		make([]byte, blscrypto.PUBLICKEYBYTES),
	}

	vanity := make([]byte, types.IstanbulExtraVanity)
	expectedResult := append(vanity, hexutil.MustDecode("0xf896ea946beaaed781d2d2ab6350f5c4566a2c6eaac407a6948be76812f765c24641ec63dc2852b378aba2b440f862b0000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000b000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000003808080808080")...)

	h := &types.Header{
		Extra: vanity,
	}

	payload, err := assembleExtra(h, oldValidators, newValidators)
	if err != nil {
		t.Errorf("error mismatch: have %v, want: nil", err)
	}

	if !reflect.DeepEqual(payload, expectedResult) {
		t.Errorf("payload mismatch: have %v, want %v", payload, expectedResult)
	}

	// append useless information to extra-data
	h.Extra = append(vanity, make([]byte, 15)...)

	payload, err = assembleExtra(h, oldValidators, newValidators)
	if !reflect.DeepEqual(payload, expectedResult) {
		t.Errorf("payload mismatch: have %v, want %v", payload, expectedResult)
	}
}

func TestWriteSeal(t *testing.T) {
	vanity := bytes.Repeat([]byte{0x00}, types.IstanbulExtraVanity)
	istRawData := hexutil.MustDecode("0xf875ea946beaaed781d2d2ab6350f5c4566a2c6eaac407a6948be76812f765c24641ec63dc2852b378aba2b440c00cb84100000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000008080808080")
	expectedSeal := make([]byte, types.IstanbulExtraSeal)
	expectedIstExtra := &types.IstanbulExtra{
		AddedValidators: []common.Address{
			common.BytesToAddress(hexutil.MustDecode("0x6beaaed781d2d2ab6350f5c4566a2c6eaac407a6")),
			common.BytesToAddress(hexutil.MustDecode("0x8be76812f765c24641ec63dc2852b378aba2b440")),
		},
		AddedValidatorsPublicKeys: [][]byte{},
		RemovedValidators:         big.NewInt(12), // 1100, remove third and fourth validators
		Seal:                      expectedSeal,
		Bitmap:                    big.NewInt(0),
		CommittedSeal:             []byte{},
		EpochData:                 []byte{},
		ParentCommit:              []byte{},
		ParentBitmap:              big.NewInt(0),
	}
	var expectedErr error

	h := &types.Header{
		Extra: append(vanity, istRawData...),
	}

	// normal case
	err := writeSeal(h, expectedSeal)
	if err != expectedErr {
		t.Errorf("error mismatch: have %v, want %v", err, expectedErr)
	}

	// verify istanbul extra-data
	istExtra, err := types.ExtractIstanbulExtra(h)
	if err != nil {
		t.Errorf("error mismatch: have %v, want nil", err)
	}
	if !reflect.DeepEqual(istExtra, expectedIstExtra) {
		t.Errorf("extra data mismatch: have %v, want %v", istExtra, expectedIstExtra)
	}

	// invalid seal
	unexpectedSeal := append(expectedSeal, make([]byte, 1)...)
	err = writeSeal(h, unexpectedSeal)
	if err != errInvalidSignature {
		t.Errorf("error mismatch: have %v, want %v", err, errInvalidSignature)
	}
}

func TestWriteCommittedSeals(t *testing.T) {
	vanity := bytes.Repeat([]byte{0x00}, types.IstanbulExtraVanity)
	istRawData := hexutil.MustDecode("0xf894ea946beaaed781d2d2ab6350f5c4566a2c6eaac407a6948be76812f765c24641ec63dc2852b378aba2b440c00c8080b860010203000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000808080")
	expectedCommittedSeal := append([]byte{1, 2, 3}, bytes.Repeat([]byte{0x00}, types.IstanbulExtraCommittedSeal-3)...)
	expectedIstExtra := &types.IstanbulExtra{
		AddedValidators: []common.Address{
			common.BytesToAddress(hexutil.MustDecode("0x6beaaed781d2d2ab6350f5c4566a2c6eaac407a6")),
			common.BytesToAddress(hexutil.MustDecode("0x8be76812f765c24641ec63dc2852b378aba2b440")),
		},
		AddedValidatorsPublicKeys: [][]byte{},
		RemovedValidators:         big.NewInt(12), // 1100, remove third and fourth validators
		Bitmap:                    big.NewInt(0),
		Seal:                      []byte{},
		CommittedSeal:             expectedCommittedSeal,
		EpochData:                 []byte{},
		ParentBitmap:              big.NewInt(0),
		ParentCommit:              []byte{},
	}
	var expectedErr error

	h := &types.Header{
		Extra: append(vanity, istRawData...),
	}

	// normal case
	err := writeCommittedSeals(h, big.NewInt(0), expectedCommittedSeal)
	if err != expectedErr {
		t.Errorf("error mismatch: have %v, want %v", err, expectedErr)
	}

	// verify istanbul extra-data
	istExtra, err := types.ExtractIstanbulExtra(h)
	if err != nil {
		t.Errorf("error mismatch: have %v, want nil", err)
	}
	if !reflect.DeepEqual(istExtra, expectedIstExtra) {
		t.Errorf("extra data mismatch: have %v, want %v", istExtra, expectedIstExtra)
	}

	// invalid seal
	unexpectedCommittedSeal := append(expectedCommittedSeal, make([]byte, 1)...)
	err = writeCommittedSeals(h, big.NewInt(0), unexpectedCommittedSeal)
	if err != errInvalidCommittedSeals {
		t.Errorf("error mismatch: have %v, want %v", err, errInvalidCommittedSeals)
	}
}
