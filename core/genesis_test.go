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

package core

import (
	"math/big"
	"reflect"
	"testing"

	"github.com/celo-org/celo-blockchain/common"
	mockEngine "github.com/celo-org/celo-blockchain/consensus/consensustest"
	"github.com/celo-org/celo-blockchain/core/rawdb"
	"github.com/celo-org/celo-blockchain/core/vm"
	"github.com/celo-org/celo-blockchain/ethdb"
	"github.com/celo-org/celo-blockchain/params"
	"github.com/davecgh/go-spew/spew"
)

func TestMainnetGenesisBlock(t *testing.T) {
	block := MainnetGenesisBlock().ToBlock(nil)
	if block.Hash() != params.MainnetGenesisHash {
		t.Errorf("wrong mainnet genesis hash, got %v, want %v", block.Hash().Hex(), params.MainnetGenesisHash.Hex())
	}
	block = DefaultBaklavaGenesisBlock().ToBlock(nil)
	if block.Hash() != params.BaklavaGenesisHash {
		t.Errorf("wrong baklava testnet genesis hash, got %v, want %v", block.Hash().Hex(), params.BaklavaGenesisHash.Hex())
	}
	block = DefaultAlfajoresGenesisBlock().ToBlock(nil)
	if block.Hash() != params.AlfajoresGenesisHash {
		t.Errorf("wrong alfajores testnet genesis hash, got %v, want %v", block.Hash().Hex(), params.AlfajoresGenesisHash.Hex())
	}
}

func TestSetupGenesis(t *testing.T) {
	customghash := common.HexToHash("0xade49833713207ecf7d4807ca34b1246b014ef3992ec231deb1e0ee56289c1c8")
	alloc := &GenesisAlloc{}
	alloc.UnmarshalJSON([]byte(devAllocJSON))
	customg := Genesis{
		Config: &params.ChainConfig{HomesteadBlock: big.NewInt(3), Istanbul: &params.IstanbulConfig{}},
		Alloc:  *alloc,
	}
	oldcustomg := customg

	oldcustomg.Config = &params.ChainConfig{HomesteadBlock: big.NewInt(2)}
	tests := []struct {
		name       string
		fn         func(ethdb.Database) (*params.ChainConfig, common.Hash, error)
		wantConfig *params.ChainConfig
		wantHash   common.Hash
		wantErr    error
	}{
		{
			name: "genesis without ChainConfig",
			fn: func(db ethdb.Database) (*params.ChainConfig, common.Hash, error) {
				return SetupGenesisBlock(db, new(Genesis))
			},
			wantErr:    errGenesisNoConfig,
			wantConfig: params.MainnetChainConfig,
		},
		{
			name: "no block in DB, genesis == nil",
			fn: func(db ethdb.Database) (*params.ChainConfig, common.Hash, error) {
				return SetupGenesisBlock(db, nil)
			},
			wantHash:   params.MainnetGenesisHash,
			wantConfig: params.MainnetChainConfig,
		},
		{
			name: "mainnet block in DB, genesis == nil",
			fn: func(db ethdb.Database) (*params.ChainConfig, common.Hash, error) {
				MainnetGenesisBlock().MustCommit(db)
				return SetupGenesisBlock(db, nil)
			},
			wantHash:   params.MainnetGenesisHash,
			wantConfig: params.MainnetChainConfig,
		},
		{
			name: "custom block in DB, genesis == nil",
			fn: func(db ethdb.Database) (*params.ChainConfig, common.Hash, error) {
				customg.MustCommit(db)
				return SetupGenesisBlock(db, nil)
			},
			wantHash:   customghash,
			wantConfig: customg.Config,
		},
		{
			name: "custom block in DB, genesis == baklava",
			fn: func(db ethdb.Database) (*params.ChainConfig, common.Hash, error) {
				customg.MustCommit(db)
				return SetupGenesisBlock(db, DefaultBaklavaGenesisBlock())
			},
			wantErr:    &GenesisMismatchError{Stored: customghash, New: params.BaklavaGenesisHash},
			wantHash:   params.BaklavaGenesisHash,
			wantConfig: params.BaklavaChainConfig,
		},
		{
			name: "custom block in DB, genesis == alfajores",
			fn: func(db ethdb.Database) (*params.ChainConfig, common.Hash, error) {
				customg.MustCommit(db)
				return SetupGenesisBlock(db, DefaultAlfajoresGenesisBlock())
			},
			wantErr:    &GenesisMismatchError{Stored: customghash, New: params.AlfajoresGenesisHash},
			wantHash:   params.AlfajoresGenesisHash,
			wantConfig: params.AlfajoresChainConfig,
		},
		{
			name: "compatible config in DB",
			fn: func(db ethdb.Database) (*params.ChainConfig, common.Hash, error) {
				oldcustomg.MustCommit(db)
				return SetupGenesisBlock(db, &customg)
			},
			wantHash:   customghash,
			wantConfig: customg.Config,
		},
		{
			name: "incompatible config in DB",
			fn: func(db ethdb.Database) (*params.ChainConfig, common.Hash, error) {
				// Commit the 'old' genesis block with Homestead transition at #2.
				// Advance to block #4, past the homestead transition block of customg.
				genesis := oldcustomg.MustCommit(db)

				bc, _ := NewBlockChain(db, nil, oldcustomg.Config, mockEngine.NewFullFaker(), vm.Config{}, nil, nil)
				defer bc.Stop()

				blocks, _ := GenerateChain(oldcustomg.Config, genesis, mockEngine.NewFaker(), db, 4, nil)
				bc.InsertChain(blocks)
				bc.CurrentBlock()
				// This should return a compatibility error.
				return SetupGenesisBlock(db, &customg)
			},
			wantHash:   customghash,
			wantConfig: customg.Config,
			wantErr: &params.ConfigCompatError{
				What:         "Homestead fork block",
				StoredConfig: big.NewInt(2),
				NewConfig:    big.NewInt(3),
				RewindTo:     1,
			},
		},
	}

	for _, test := range tests {
		db := rawdb.NewMemoryDatabase()
		config, hash, err := test.fn(db)
		// Check the return values.
		if !reflect.DeepEqual(err, test.wantErr) {
			spew := spew.ConfigState{DisablePointerAddresses: true, DisableCapacities: true}
			t.Errorf("%s: returned error %#v, want %#v", test.name, spew.NewFormatter(err), spew.NewFormatter(test.wantErr))
		}
		if !reflect.DeepEqual(config, test.wantConfig) {
			t.Errorf("%s:\nreturned %v\nwant     %v", test.name, config, test.wantConfig)
		}
		if hash != test.wantHash {
			t.Errorf("%s: returned hash %s, want %s", test.name, hash.Hex(), test.wantHash.Hex())
		} else if err == nil {
			// Check database content.
			stored := rawdb.ReadBlock(db, test.wantHash, 0)
			if stored.Hash() != test.wantHash {
				t.Errorf("%s: block in DB has hash %s, want %s", test.name, stored.Hash(), test.wantHash)
			}
		}
	}
}

// TestRegistryInGenesis tests if the params.RegistrySmartContract that defined in the genesis block sits in the blockchain
func TestRegistryInGenesis(t *testing.T) {
	tests := []struct {
		name    string
		genesis func() *Genesis
	}{
		{
			name:    "dev",
			genesis: DeveloperGenesisBlock,
		},
		{
			name:    "alfajores",
			genesis: DefaultAlfajoresGenesisBlock,
		},
		{
			name:    "baklava",
			genesis: DefaultBaklavaGenesisBlock,
		},
		{
			name:    "mainnet",
			genesis: MainnetGenesisBlock,
		},
		{
			name: "emptyAlloc",
			genesis: func() *Genesis {
				return &Genesis{}
			},
		},
	}

	for _, test := range tests {
		db := rawdb.NewMemoryDatabase()
		test.genesis().MustCommit(db)
		chain, _ := NewBlockChain(db, nil, params.IstanbulTestChainConfig, mockEngine.NewFaker(), vm.Config{}, nil, nil)
		state, _ := chain.State()
		codeSize := state.GetCodeSize(params.RegistrySmartContractAddress)
		if test.name == "emptyAlloc" {
			if codeSize != 0 {
				t.Errorf("%s: Registry code size is %d, want 0", test.name, codeSize)
			}
		} else if codeSize == 0 {
			t.Errorf("%s: Registry code size should not be 0, actual %d", test.name, codeSize)
		}
		chain.Stop()
	}
}
