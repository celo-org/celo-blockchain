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
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/istanbul"
	"github.com/ethereum/go-ethereum/consensus/istanbul/validator"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/crypto"
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
	if ap.accounts[account] == nil {
		ap.accounts[account], _ = crypto.GenerateKey()
	}
	// Resolve and return the Ethereum address
	return crypto.PubkeyToAddress(ap.accounts[account].PublicKey)
}

func convertValNames(accounts *testerAccountPool, valNames []string) []common.Address {
	returnArray := make([]common.Address, len(valNames))

	for i, valName := range valNames {
		returnArray[i] = accounts.address(valName)
	}

	sort.Slice(returnArray, func(i, j int) bool {
		return strings.Compare(returnArray[i].String(), returnArray[j].String()) < 0
	})
	return returnArray
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
			results:     []string{},
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
		}, {
			// Three validator, add two validators and remove two validators.  Second header will add 1 validators and remove 2 validators.
			epoch:      1,
			validators: []string{"A", "B", "C"},
			valsetdiffs: []testerValSetDiff{{proposer: "A", addedValidators: []string{"D", "E"}, removedValidators: []string{"B", "C"}},
				{proposer: "E", addedValidators: []string{"F"}, removedValidators: []string{"A", "D"}}},
			results: []string{"E", "F"},
			err:     nil,
		}, {
			// Three validator, add two validators and remove two validators.  Second header will add 1 validators and remove 2 validators.  The first header will
			// be ignored, since it's no the last header of an epoch
			epoch:      2,
			validators: []string{"A", "B", "C"},
			valsetdiffs: []testerValSetDiff{{proposer: "A", addedValidators: []string{"D", "E"}, removedValidators: []string{"B", "C"}},
				{proposer: "A", addedValidators: []string{"F"}, removedValidators: []string{"A", "B"}}},
			results: []string{"C", "F"},
			err:     nil,
		},
	}
	// Run through the scenarios and test them
	for i, tt := range tests {
		// Create the account pool and generate the initial set of validators
		accounts := newTesterAccountPool()

		validators := make([]common.Address, len(tt.validators))
		for j, validator := range tt.validators {
			validators[j] = accounts.address(validator)
		}

		// Sort the validators
		for j := 0; j < len(validators); j++ {
			for k := j + 1; k < len(validators); k++ {
				if bytes.Compare(validators[j][:], validators[k][:]) > 0 {
					validators[j], validators[k] = validators[k], validators[j]
				}
			}
		}

		// Create the genesis block with the initial set of validators
		genesis := &core.Genesis{
			Difficulty: defaultDifficulty,
			Mixhash:    types.IstanbulDigest,
			Config:     params.TestChainConfig,
		}
		b := genesis.ToBlock(nil)
		extra, _ := assembleExtra(b.Header(), []common.Address{}, validators)
		genesis.ExtraData = extra
		// Create a pristine blockchain with the genesis injected
		db := ethdb.NewMemDatabase()
		genesis.Commit(db)

		config := istanbul.DefaultConfig
		if tt.epoch != 0 {
			config.Epoch = tt.epoch
		}
		engine := New(config, accounts.accounts[tt.validators[0]], db).(*Backend)
		chain, err := core.NewBlockChain(db, nil, genesis.Config, engine, vm.Config{}, nil)

		// Assemble a chain of headers from the cast votes
		headers := make([]*types.Header, len(tt.valsetdiffs))
		for j, valsetdiff := range tt.valsetdiffs {
			headers[j] = &types.Header{
				Number:     big.NewInt(int64(j) + 1),
				Time:       big.NewInt(int64(j) * int64(config.BlockPeriod)),
				Difficulty: defaultDifficulty,
				MixDigest:  types.IstanbulDigest,
			}

			var buf bytes.Buffer

			buf.Write(bytes.Repeat([]byte{0x00}, types.IstanbulExtraVanity))

			ist := &types.IstanbulExtra{
				AddedValidators:   convertValNames(accounts, valsetdiff.addedValidators),
				RemovedValidators: convertValNames(accounts, valsetdiff.removedValidators),
				Seal:              []byte{},
				CommittedSeal:     [][]byte{},
			}

			payload, err := rlp.EncodeToBytes(&ist)
			if err != nil {
				t.Errorf("test %d, valsetdiff %d: error in encoding extra header info", i, j)
			}
			headers[j].Extra = append(buf.Bytes(), payload...)

			if j > 0 {
				headers[j].ParentHash = headers[j-1].Hash()
			}

			accounts.sign(headers[j], valsetdiff.proposer)
		}
		// Pass all the headers through clique and ensure tallying succeeds
		head := headers[len(headers)-1]

		snap, err := engine.snapshot(chain, head.Number.Uint64(), head.Hash(), headers)
		if err != tt.err {
			t.Errorf("test %d: error mismatch:  have %v, want %v", i, err, tt.err)
			continue
		}

		if tt.err != nil {
			continue
		}

		// Verify the final list of validators against the expected ones
		validators = make([]common.Address, len(tt.results))
		for j, validator := range tt.results {
			validators[j] = accounts.address(validator)
		}
		for j := 0; j < len(validators); j++ {
			for k := j + 1; k < len(validators); k++ {
				if bytes.Compare(validators[j][:], validators[k][:]) > 0 {
					validators[j], validators[k] = validators[k], validators[j]
				}
			}
		}
		result := snap.validators()
		if len(result) != len(validators) {
			t.Errorf("test %d: validators mismatch: have %x, want %x", i, result, validators)
			continue
		}
		for j := 0; j < len(result); j++ {
			if !bytes.Equal(result[j][:], validators[j][:]) {
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
		ValSet: validator.NewSet([]common.Address{
			common.BytesToAddress([]byte("1234567894")),
			common.BytesToAddress([]byte("1234567895")),
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
