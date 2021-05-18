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
	"math/big"
	"reflect"
	"testing"
	"time"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/common/hexutil"
	"github.com/celo-org/celo-blockchain/consensus"
	"github.com/celo-org/celo-blockchain/consensus/istanbul"
	"github.com/celo-org/celo-blockchain/core/types"
	blscrypto "github.com/celo-org/celo-blockchain/crypto/bls"
	"github.com/celo-org/celo-blockchain/rlp"
	"github.com/stretchr/testify/assert"
)

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

func TestMakeBlockWithSignature(t *testing.T) {
	numValidators := 4
	genesisCfg, nodeKeys := getGenesisAndKeys(numValidators, true)
	chain, engine, _ := newBlockChainWithKeys(false, common.Address{}, false, genesisCfg, nodeKeys[0])
	genesis := chain.Genesis()
	block, err := makeBlock(nodeKeys, chain, engine, genesis)
	if err != nil {
		t.Errorf("error mismatch: have %v, want nil", err)
	}

	block2, err := makeBlock(nodeKeys, chain, engine, block)
	if err != nil {
		t.Errorf("error mismatch: have %v, want nil", err)
	}

	_, err = makeBlock(nodeKeys, chain, engine, block2)
	if err != nil {
		t.Errorf("error mismatch: have %v, want nil", err)
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

// TestSealCommittedOtherHash checks that when Seal() ask for a commit, if we send a
// different block hash, it will abort
func TestSealCommittedOtherHash(t *testing.T) {
	for numValidators := 1; numValidators < 5; numValidators++ {
		testSealCommittedOtherHash(t, numValidators)
	}
}

func testSealCommittedOtherHash(t *testing.T, numValidators int) {
	chain, engine := newBlockChain(numValidators, true)
	block := makeBlockWithoutSeal(chain, engine, chain.Genesis())
	// create a second block which will have a different hash
	h := block.Header()
	h.ParentHash = block.Hash()
	otherBlock := types.NewBlock(h, nil, nil, nil)
	if block.Hash() == otherBlock.Hash() {
		t.Fatalf("did not create different blocks")
	}

	results := make(chan *types.Block)
	engine.Seal(chain, block, results, nil)

	// in the single validator case, the result will instantly be processed
	// so we should discard it and check that the Commit will not do anything
	if numValidators == 1 {
		<-results
	}
	// this commit should _NOT_ push a new message to the queue
	engine.Commit(otherBlock, types.IstanbulAggregatedSeal{}, types.IstanbulEpochValidatorSetSeal{})

	select {
	case res := <-results:
		t.Fatal("seal should not be completed", res)
	case <-time.After(100 * time.Millisecond):
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

	// errEmptyAggregatedSeal case
	block := makeBlockWithoutSeal(chain, engine, chain.Genesis())
	block, _ = engine.updateBlock(chain.Genesis().Header(), block)
	err := engine.VerifyHeader(chain, block.Header(), false)
	if err != errEmptyAggregatedSeal {
		t.Errorf("error mismatch: have %v, want %v", err, errEmptyAggregatedSeal)
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

	// invalid timestamp
	block = makeBlockWithoutSeal(chain, engine, chain.Genesis())
	header = block.Header()
	header.Time = chain.Genesis().Time() + engine.config.BlockPeriod - 1
	err = engine.VerifyHeader(chain, header, false)
	if err != errInvalidTimestamp {
		t.Errorf("error mismatch: have %v, want %v", err, errInvalidTimestamp)
	}

	// future block
	block = makeBlockWithoutSeal(chain, engine, chain.Genesis())
	header = block.Header()
	header.Time = uint64(now().Unix() + 10)
	err = engine.VerifyHeader(chain, header, false)
	if err != consensus.ErrFutureBlock {
		t.Errorf("error mismatch: have %v, want %v", err, consensus.ErrFutureBlock)
	}
}

func TestVerifySeal(t *testing.T) {
	numValidators := 4
	genesisCfg, nodeKeys := getGenesisAndKeys(numValidators, true)
	chain, engine, _ := newBlockChainWithKeys(false, common.Address{}, false, genesisCfg, nodeKeys[0])
	genesis := chain.Genesis()

	// cannot verify genesis
	err := engine.VerifySeal(chain, genesis.Header())
	assert.Error(t, err)

	block, _ := makeBlock(nodeKeys, chain, engine, genesis)
	header := block.Header()
	err = engine.VerifySeal(chain, header)
	assert.NoError(t, err)

	// change header content and expect to invalidate signature
	header.Number = big.NewInt(4)
	err = engine.VerifySeal(chain, header)
	assert.Error(t, err)

	// delete istanbul extra data and expect invalid extra data format
	header = block.Header()
	header.Extra = nil
	err = engine.VerifySeal(chain, header)
	assert.Error(t, err)

	// modify seal bitmap and expect to fail the quorum check
	header = block.Header()
	extra, err := types.ExtractIstanbulExtra(header)
	assert.NoError(t, err)

	extra.AggregatedSeal.Bitmap = big.NewInt(0)
	encoded, err := rlp.EncodeToBytes(extra)
	assert.NoError(t, err)

	header.Extra = append(header.Extra[:types.IstanbulExtraVanity], encoded...)
	err = engine.VerifySeal(chain, header)
	assert.Error(t, err)

	// verifiy the seal on the unmodified block.
	err = engine.VerifySeal(chain, block.Header())
	assert.NoError(t, err)
}

func TestVerifyHeaders(t *testing.T) {
	numValidators := 4
	genesisCfg, nodeKeys := getGenesisAndKeys(numValidators, true)
	chain, engine, _ := newBlockChainWithKeys(false, common.Address{}, false, genesisCfg, nodeKeys[0])
	genesis := chain.Genesis()

	// success case
	headers := []*types.Header{}
	blocks := []*types.Block{}
	size := 10

	for i := 0; i < size; i++ {
		var b *types.Block
		if i == 0 {
			b, _ = makeBlock(nodeKeys, chain, engine, genesis)
		} else {
			b, _ = makeBlock(nodeKeys, chain, engine, blocks[i-1])
		}

		blocks = append(blocks, b)
		headers = append(headers, blocks[i].Header())
	}
	now = func() time.Time {
		return time.Unix(int64(headers[size-1].Time), 0)
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
				t.Errorf("error mismatch: have %v, want nil", err)
				break OUT1
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
				t.Errorf("error mismatch: have %v, want nil", err)
				break OUT2
			}
			index++
			if index == 1 {
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
	_, results = engine.VerifyHeaders(chain, headers, nil)
	timeout = time.NewTimer(timeoutDura)
	index = 0
	errors := 0
	expectedErrors := 8
OUT3:
	for {
		select {
		case err := <-results:
			if err != nil {
				errors++
			}
			index++
			if index == size {
				if errors != expectedErrors {
					t.Errorf("error mismatch: have %v, want %v", errors, expectedErrors)
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
	header.Time = uint64(now().Unix() + 3)
	err := engine.VerifyHeader(chain, header, false)
	if err != errEmptyAggregatedSeal {
		t.Errorf("error mismatch: have %v, want %v", err, errEmptyAggregatedSeal)
	}

	// reject future block without full chain available
	block = makeBlockWithoutSeal(chain, engine, chain.Genesis())
	header = block.Header()
	header.Time = uint64(now().Unix() + 10)
	err = engine.VerifyHeader(chain, header, false)
	if err != consensus.ErrFutureBlock {
		t.Errorf("error mismatch: have %v, want %v", err, consensus.ErrFutureBlock)
	}
}

func TestPrepareExtra(t *testing.T) {
	oldValidators := make([]istanbul.ValidatorData, 2)
	oldValidators[0] = istanbul.ValidatorData{
		Address:      common.BytesToAddress(hexutil.MustDecode("0x44add0ec310f115a0e603b2d7db9f067778eaf8a")),
		BLSPublicKey: blscrypto.SerializedPublicKey{},
	}
	oldValidators[1] = istanbul.ValidatorData{
		Address:      common.BytesToAddress(hexutil.MustDecode("0x294fc7e8f22b3bcdcf955dd7ff3ba2ed833f8212")),
		BLSPublicKey: blscrypto.SerializedPublicKey{},
	}

	newValidators := make([]istanbul.ValidatorData, 2)
	newValidators[0] = istanbul.ValidatorData{
		Address:      common.BytesToAddress(hexutil.MustDecode("0x6beaaed781d2d2ab6350f5c4566a2c6eaac407a6")),
		BLSPublicKey: blscrypto.SerializedPublicKey{},
	}
	newValidators[1] = istanbul.ValidatorData{
		Address:      common.BytesToAddress(hexutil.MustDecode("0x8be76812f765c24641ec63dc2852b378aba2b440")),
		BLSPublicKey: blscrypto.SerializedPublicKey{},
	}

	extra, err := rlp.EncodeToBytes(&types.IstanbulExtra{
		AddedValidators:           []common.Address{},
		AddedValidatorsPublicKeys: []blscrypto.SerializedPublicKey{},
		RemovedValidators:         big.NewInt(0),
		Seal:                      []byte{},
		AggregatedSeal:            types.IstanbulAggregatedSeal{},
		ParentAggregatedSeal:      types.IstanbulAggregatedSeal{},
	})
	if err != nil {
		t.Errorf("error: %v", err)
	}
	h := &types.Header{
		Extra: append(make([]byte, types.IstanbulExtraVanity), extra...),
	}

	err = writeValidatorSetDiff(h, oldValidators, newValidators)
	if err != nil {
		t.Errorf("error mismatch: have %v, want: nil", err)
	}

	// the header must have the updated extra data
	updatedExtra, err := types.ExtractIstanbulExtra(h)
	if err != nil {
		t.Errorf("error mismatch: have %v, want: nil", err)
	}

	var updatedExtraVals []istanbul.ValidatorData
	for i := range updatedExtra.AddedValidators {
		updatedExtraVals = append(updatedExtraVals, istanbul.ValidatorData{
			Address:      updatedExtra.AddedValidators[i],
			BLSPublicKey: updatedExtra.AddedValidatorsPublicKeys[i],
		})
	}

	if !reflect.DeepEqual(updatedExtraVals, newValidators) {
		t.Errorf("validators were not properly updated")
	}

	// the validators which were removed were 2, so the bitmap is 11, meaning it
	// should be 3
	if updatedExtra.RemovedValidators.Cmp(big.NewInt(3)) != 0 {
		t.Errorf("Invalid removed validators bitmap, got %v, want %v", updatedExtra.RemovedValidators, big.NewInt(3))
	}

}

func TestWriteSeal(t *testing.T) {
	vanity := bytes.Repeat([]byte{0x00}, types.IstanbulExtraVanity)
	istExtra := &types.IstanbulExtra{
		AddedValidators: []common.Address{
			common.BytesToAddress(hexutil.MustDecode("0x6beaaed781d2d2ab6350f5c4566a2c6eaac407a6")),
			common.BytesToAddress(hexutil.MustDecode("0x8be76812f765c24641ec63dc2852b378aba2b440")),
		},
		AddedValidatorsPublicKeys: []blscrypto.SerializedPublicKey{},
		RemovedValidators:         big.NewInt(12), // 1100, remove third and fourth validators
		Seal:                      []byte{},
		AggregatedSeal:            types.IstanbulAggregatedSeal{Bitmap: big.NewInt(0), Signature: []byte{}, Round: big.NewInt(0)},
		ParentAggregatedSeal:      types.IstanbulAggregatedSeal{Bitmap: big.NewInt(0), Signature: []byte{}, Round: big.NewInt(0)},
	}
	istExtraRaw, err := rlp.EncodeToBytes(&istExtra)
	if err != nil {
		t.Errorf("error: %v", err)
	}

	expectedSeal := hexutil.MustDecode("0x29fe2612266a3965321c23a2e0382cd819e992f293d9a0032439728e41201d2c387cc9de5914a734873d79addb76c59ce73c1085a98b968384811b4ad050dddc56")
	if len(expectedSeal) != types.IstanbulExtraSeal {
		t.Errorf("incorrect length for seal: have %v, want %v", len(expectedSeal), types.IstanbulExtraSeal)
	}
	expectedIstExtra := istExtra
	expectedIstExtra.Seal = expectedSeal

	var expectedErr error

	h := &types.Header{
		Extra: append(vanity, istExtraRaw...),
	}

	// normal case
	err = writeSeal(h, expectedSeal)
	if err != expectedErr {
		t.Errorf("error mismatch: have %v, want %v", err, expectedErr)
	}

	// verify istanbul extra-data
	actualIstExtra, err := types.ExtractIstanbulExtra(h)
	if err != nil {
		t.Errorf("error mismatch: have %v, want nil", err)
	}
	if !reflect.DeepEqual(actualIstExtra, expectedIstExtra) {
		t.Errorf("extra data mismatch: have %v, want %v", actualIstExtra, expectedIstExtra)
	}

	// invalid seal
	unexpectedSeal := append(expectedSeal, make([]byte, 1)...)
	err = writeSeal(h, unexpectedSeal)
	if err != errInvalidSignature {
		t.Errorf("error mismatch: have %v, want %v", err, errInvalidSignature)
	}
}

func TestWriteAggregatedSeal(t *testing.T) {
	vanity := bytes.Repeat([]byte{0x00}, types.IstanbulExtraVanity)
	istExtra := &types.IstanbulExtra{
		AddedValidators: []common.Address{
			common.BytesToAddress(hexutil.MustDecode("0x6beaaed781d2d2ab6350f5c4566a2c6eaac407a6")),
			common.BytesToAddress(hexutil.MustDecode("0x8be76812f765c24641ec63dc2852b378aba2b440")),
		},
		AddedValidatorsPublicKeys: []blscrypto.SerializedPublicKey{},
		RemovedValidators:         big.NewInt(12), // 1100, remove third and fourth validators
		Seal:                      []byte{},
		AggregatedSeal:            types.IstanbulAggregatedSeal{},
		ParentAggregatedSeal:      types.IstanbulAggregatedSeal{},
	}
	istExtraRaw, err := rlp.EncodeToBytes(&istExtra)
	if err != nil {
		t.Errorf("error: %v", err)
	}

	aggregatedSeal := types.IstanbulAggregatedSeal{
		Round:     big.NewInt(2),
		Bitmap:    big.NewInt(3),
		Signature: append([]byte{1, 2, 3}, bytes.Repeat([]byte{0x00}, types.IstanbulExtraBlsSignature-3)...),
	}

	expectedIstExtra := istExtra
	expectedIstExtra.AggregatedSeal = aggregatedSeal
	expectedIstExtra.ParentAggregatedSeal = aggregatedSeal

	h := &types.Header{
		Extra: append(vanity, istExtraRaw...),
	}

	// normal case
	err = writeAggregatedSeal(h, aggregatedSeal, false)
	if err != nil {
		t.Errorf("error mismatch: have %v, want nil", err)
	}

	err = writeAggregatedSeal(h, aggregatedSeal, true)
	if err != nil {
		t.Errorf("error mismatch: have %v, want nil", err)
	}

	// verify istanbul extra-data
	actualIstExtra, err := types.ExtractIstanbulExtra(h)
	if err != nil {
		t.Errorf("error mismatch: have %v, want nil", err)
	}
	if !reflect.DeepEqual(actualIstExtra, expectedIstExtra) {
		t.Errorf("extra data mismatch: have %v, want %v", actualIstExtra, expectedIstExtra)
	}

	// try to write an invalid length seal to the CommitedSeal or ParentCommit field
	invalidAggregatedSeal := types.IstanbulAggregatedSeal{
		Round:     big.NewInt(3),
		Bitmap:    big.NewInt(3),
		Signature: append(aggregatedSeal.Signature, make([]byte, 1)...),
	}
	err = writeAggregatedSeal(h, invalidAggregatedSeal, false)
	if err != errInvalidAggregatedSeal {
		t.Errorf("error mismatch: have %v, want %v", err, errInvalidAggregatedSeal)
	}

	err = writeAggregatedSeal(h, invalidAggregatedSeal, true)
	if err != errInvalidAggregatedSeal {
		t.Errorf("error mismatch: have %v, want %v", err, errInvalidAggregatedSeal)
	}
}
