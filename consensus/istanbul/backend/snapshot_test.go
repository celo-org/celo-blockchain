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
	"path/filepath"
	"reflect"
	"testing"

	bls "github.com/celo-org/bls-zexe/go"
	ethAccounts "github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/istanbul"
	"github.com/ethereum/go-ethereum/consensus/istanbul/validator"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	blscrypto "github.com/ethereum/go-ethereum/crypto/bls"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/rlp"
)

type testerValSetDiff struct {
	proposer          string
	addedValidators   []string
	removedValidators []string
}

// testerAccountPool is a pool to maintain currently active tester accounts,
// mapped from textual names used in the tests below to actual Ethereum private
// keys capable of signing transactions.
type testerAccountPool struct {
	accounts map[string]*ecdsa.PrivateKey
}

func newTesterAccountPool() *testerAccountPool {
	return &testerAccountPool{
		accounts: make(map[string]*ecdsa.PrivateKey),
	}
}

func (ap *testerAccountPool) sign(header *types.Header, validator string) {
	// Ensure we have a persistent key for the validator
	if ap.accounts[validator] == nil {
		ap.accounts[validator], _ = crypto.GenerateKey()
	}
	// Sign the header and embed the signature in extra data
	hashData := crypto.Keccak256([]byte(sigHash(header).Bytes()))
	sig, _ := crypto.Sign(hashData, ap.accounts[validator])

	writeSeal(header, sig)
}

func (ap *testerAccountPool) address(account string) common.Address {
	// Ensure we have a persistent key for the account
	if account == "" {
		return common.Address{}
	}
	if ap.accounts[account] == nil {
		ap.accounts[account], _ = crypto.GenerateKey()
	}
	// Resolve and return the Ethereum address
	return crypto.PubkeyToAddress(ap.accounts[account].PublicKey)
}

func indexOf(element string, data []string) int {
	for k, v := range data {
		if element == v {
			return k
		}
	}
	return -1 //not found.
}

func convertValNamesToRemovedValidators(accounts *testerAccountPool, oldVals []istanbul.ValidatorData, valNames []string) *big.Int {
	bitmap := big.NewInt(0)
	for _, v := range valNames {
		for j := range oldVals {
			if accounts.address(v) == oldVals[j].Address {
				bitmap = bitmap.SetBit(bitmap, j, 1)
			}
		}
	}

	return bitmap
}

func convertValNames(accounts *testerAccountPool, valNames []string) []common.Address {
	returnArray := make([]common.Address, len(valNames))

	for i, valName := range valNames {
		returnArray[i] = accounts.address(valName)
	}

	return returnArray
}

func convertValNamesToValidatorsData(accounts *testerAccountPool, valNames []string) []istanbul.ValidatorData {
	returnArray := make([]istanbul.ValidatorData, len(valNames))

	for i, valName := range valNames {
		returnArray[i] = istanbul.ValidatorData{
			accounts.address(valName),
			nil,
		}
	}

	return returnArray
}

// Define a mock blockchain
type mockBlockchain struct {
	headers map[uint64]*types.Header
}

func (bc *mockBlockchain) AddHeader(number uint64, header *types.Header) {
	bc.headers[number] = header
}

func (bc *mockBlockchain) GetHeaderByNumber(number uint64) *types.Header {
	return bc.headers[number]
}

func (bc *mockBlockchain) Config() *params.ChainConfig {
	return &params.ChainConfig{FullHeaderChainAvailable: true}
}

func (bc *mockBlockchain) CurrentHeader() *types.Header {
	return nil
}

func (bc *mockBlockchain) GetHeader(hash common.Hash, number uint64) *types.Header {
	return nil
}

func (bc *mockBlockchain) GetHeaderByHash(hash common.Hash) *types.Header {
	return nil
}

func (bc *mockBlockchain) GetBlock(hash common.Hash, number uint64) *types.Block {
	return nil
}

// Tests that validator set changes are evaluated correctly for various simple and complex scenarios.
func TestValSetChange(t *testing.T) {
	// Define the various voting scenarios to test
	tests := []struct {
		epoch       uint64
		validators  []string
		valsetdiffs []testerValSetDiff
		results     []string
		err         error
	}{
		{
			// Single validator, empty val set diff
			epoch:       1,
			validators:  []string{"A"},
			valsetdiffs: []testerValSetDiff{{proposer: "A", addedValidators: []string{}, removedValidators: []string{}}},
			results:     []string{"A"},
			err:         nil,
		}, {
			// Single validator, add two new validators
			epoch:       1,
			validators:  []string{"A"},
			valsetdiffs: []testerValSetDiff{{proposer: "A", addedValidators: []string{"B", "C"}, removedValidators: []string{}}},
			results:     []string{"A", "B", "C"},
			err:         nil,
		}, {
			// Two validator, remove two validators
			epoch:       1,
			validators:  []string{"A", "B"},
			valsetdiffs: []testerValSetDiff{{proposer: "B", addedValidators: []string{}, removedValidators: []string{"A", "B"}}},
			results:     []string{"", ""},
			err:         nil,
		}, {
			// Three validator, add two validators and remove two validators
			epoch:       1,
			validators:  []string{"A", "B", "C"},
			valsetdiffs: []testerValSetDiff{{proposer: "A", addedValidators: []string{"D", "E"}, removedValidators: []string{"B", "C"}}},
			results:     []string{"A", "D", "E"},
			err:         nil,
		}, {
			// Three validator, add two validators and remove two validators with an unauthorized proposer
			epoch:       1,
			validators:  []string{"A", "B", "C"},
			valsetdiffs: []testerValSetDiff{{proposer: "D", addedValidators: []string{"D", "E"}, removedValidators: []string{"B", "C"}}},
			results:     []string{"A", "D", "E"},
			err:         errUnauthorized,
		},
		{
			// Three validator, add two validators and remove two validators.  Second header will add 1 validators and remove 2 validators.
			epoch:      1,
			validators: []string{"A", "B", "C"},
			valsetdiffs: []testerValSetDiff{{proposer: "A", addedValidators: []string{"D", "E"}, removedValidators: []string{"B", "C"}},
				{proposer: "E", addedValidators: []string{"F"}, removedValidators: []string{"A", "D"}}},
			results: []string{"F", "", "E"},
			err:     nil,
		}, {
			// Three validator, add two validators and remove two validators.  Second header will add 1 validators and remove 2 validators.  The first header will
			// be ignored, since it's no the last header of an epoch
			epoch:      2,
			validators: []string{"A", "B", "C"},
			valsetdiffs: []testerValSetDiff{{proposer: "A", addedValidators: []string{"D", "E"}, removedValidators: []string{"B", "C"}},
				{proposer: "A", addedValidators: []string{"F"}, removedValidators: []string{"A", "B"}}},
			results: []string{"F", "", "C"},
			err:     nil,
		},
	}
	// Run through the scenarios and test them
	for i, tt := range tests {
		// Create the account pool and generate the initial set of validators
		accounts := newTesterAccountPool()

		validators := make([]istanbul.ValidatorData, len(tt.validators))
		for j, validator := range tt.validators {
			validators[j] = istanbul.ValidatorData{
				accounts.address(validator),
				nil,
			}
		}

		// Create the genesis block with the initial set of validators
		genesis := &core.Genesis{
			Difficulty: defaultDifficulty,
			Mixhash:    types.IstanbulDigest,
			Config:     params.TestChainConfig,
		}
		extra, _ := rlp.EncodeToBytes(&types.IstanbulExtra{})
		genesis.ExtraData = append(make([]byte, types.IstanbulExtraVanity), extra...)
		b := genesis.ToBlock(nil)
		h := b.Header()
		err := writeValidatorSetDiff(h, []istanbul.ValidatorData{}, validators)
		if err != nil {
			t.Errorf("Could not update genesis validator set, got err: %v", err)
		}
		genesis.ExtraData = h.Extra
		db := ethdb.NewMemDatabase()

		config := istanbul.DefaultConfig
		if tt.epoch != 0 {
			config.Epoch = tt.epoch
		}

		chain := &mockBlockchain{
			headers: make(map[uint64]*types.Header),
		}

		dataDir := filepath.Join("/tmp", string(rand.Int()))
		engine := New(config, db, dataDir).(*Backend)

		privateKey := accounts.accounts[tt.validators[0]]
		address := crypto.PubkeyToAddress(privateKey.PublicKey)
		signerFn := func(_ ethAccounts.Account, data []byte) ([]byte, error) {
			return crypto.Sign(data, privateKey)
		}

		signerBLSHashFn := func(_ ethAccounts.Account, data []byte) ([]byte, error) {
			key := privateKey
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

		signerBLSMessageFn := func(_ ethAccounts.Account, data []byte, extraData []byte) ([]byte, error) {
			key := privateKey
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

		engine.Authorize(address, signerFn, signerBLSHashFn, signerBLSMessageFn)

		chain.AddHeader(0, genesis.ToBlock(nil).Header())

		// Assemble a chain of headers from header validator set diffs
		var prevHeader *types.Header
		var currentVals []istanbul.ValidatorData
		var snap *Snapshot
		for j, valsetdiff := range tt.valsetdiffs {
			header := &types.Header{
				Number:     big.NewInt(int64(j) + 1),
				Time:       big.NewInt(int64(j) * int64(config.BlockPeriod)),
				Difficulty: defaultDifficulty,
				MixDigest:  types.IstanbulDigest,
			}

			var buf bytes.Buffer

			buf.Write(bytes.Repeat([]byte{0x00}, types.IstanbulExtraVanity))

			var oldVals []istanbul.ValidatorData
			if currentVals == nil {
				oldVals = convertValNamesToValidatorsData(accounts, tests[i].validators)
			} else {
				oldVals = currentVals
			}

			ist := &types.IstanbulExtra{
				AddedValidators:           convertValNames(accounts, valsetdiff.addedValidators),
				AddedValidatorsPublicKeys: make([][]byte, len(valsetdiff.addedValidators)),
				RemovedValidators:         convertValNamesToRemovedValidators(accounts, oldVals, valsetdiff.removedValidators),
				Bitmap:                    big.NewInt(0),
				Seal:                      []byte{},
				CommittedSeal:             []byte{},
				EpochData:                 []byte{},
				ParentCommit:              []byte{},
				ParentBitmap:              big.NewInt(0),
			}

			payload, err := rlp.EncodeToBytes(&ist)
			if err != nil {
				t.Errorf("test %d, valsetdiff %d: error in encoding extra header info", i, j)
			}
			header.Extra = append(buf.Bytes(), payload...)

			if j > 0 {
				header.ParentHash = prevHeader.Hash()
			}

			accounts.sign(header, valsetdiff.proposer)

			chain.AddHeader(uint64(j+1), header)

			prevHeader = header
			snap, err = engine.snapshot(chain, prevHeader.Number.Uint64(), prevHeader.Hash(), nil)
			if err != tt.err {
				t.Errorf("test %d: error mismatch:  have %v, want %v", i, err, tt.err)
			}

			if err != nil {
				continue
			}

			currentVals = snap.validators()
		}
		if tt.err != nil {
			continue
		}

		// Verify the final list of validators against the expected ones
		validators = make([]istanbul.ValidatorData, len(tt.results))
		for j, validator := range tt.results {
			validators[j] = istanbul.ValidatorData{
				accounts.address(validator),
				nil,
			}
		}
		result := snap.validators()
		if len(result) != len(validators) {
			t.Errorf("test %d: validators mismatch: have %x, want %x", i, result, validators)
			continue
		}

		for j := 0; j < len(result); j++ {
			if !bytes.Equal(result[j].Address[:], validators[j].Address[:]) {
				t.Errorf("test %d, validator %d: validator mismatch: have %x, want %x", i, j, result[j], validators[j])
			}
		}
	}
}

func TestSaveAndLoad(t *testing.T) {
	snap := &Snapshot{
		Epoch:  5,
		Number: 10,
		Hash:   common.HexToHash("1234567890"),
		ValSet: validator.NewSet([]istanbul.ValidatorData{
			{
				common.BytesToAddress([]byte("1234567894")),
				nil,
			},
			{
				common.BytesToAddress([]byte("1234567895")),
				nil,
			},
		}, istanbul.RoundRobin),
	}
	db := ethdb.NewMemDatabase()
	err := snap.store(db)
	if err != nil {
		t.Errorf("store snapshot failed: %v", err)
	}

	snap1, err := loadSnapshot(snap.Epoch, db, snap.Hash)
	if err != nil {
		t.Errorf("load snapshot failed: %v", err)
	}
	if snap.Epoch != snap1.Epoch {
		t.Errorf("epoch mismatch: have %v, want %v", snap1.Epoch, snap.Epoch)
	}
	if snap.Hash != snap1.Hash {
		t.Errorf("hash mismatch: have %v, want %v", snap1.Number, snap.Number)
	}
	if !reflect.DeepEqual(snap.ValSet, snap.ValSet) {
		t.Errorf("validator set mismatch: have %v, want %v", snap1.ValSet, snap.ValSet)
	}
}
