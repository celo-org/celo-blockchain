// Copyright 2020 The go-ethereum Authors
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
	"testing"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/consensus"
	mockEngine "github.com/celo-org/celo-blockchain/consensus/consensustest"
	"github.com/celo-org/celo-blockchain/core/rawdb"
	"github.com/celo-org/celo-blockchain/core/types"
	"github.com/celo-org/celo-blockchain/core/vm"
	"github.com/celo-org/celo-blockchain/crypto"
	"github.com/celo-org/celo-blockchain/params"
	"github.com/celo-org/celo-blockchain/trie"
	"golang.org/x/crypto/sha3"
)

// TestStateProcessorErrors tests the output from the 'core' errors
// as defined in core/error.go. These errors are generated when the
// blockchain imports bad blocks, meaning blocks which have valid headers but
// contain invalid transactions
func TestStateProcessorErrors(t *testing.T) {
	var (
		config = &params.ChainConfig{
			ChainID:             big.NewInt(1),
			HomesteadBlock:      big.NewInt(0),
			EIP150Block:         big.NewInt(0),
			EIP155Block:         big.NewInt(0),
			EIP158Block:         big.NewInt(0),
			ByzantiumBlock:      big.NewInt(0),
			ConstantinopleBlock: big.NewInt(0),
			PetersburgBlock:     big.NewInt(0),
			IstanbulBlock:       big.NewInt(0),
			ChurritoBlock:       big.NewInt(0),
			DonutBlock:          big.NewInt(0),
			EspressoBlock:       big.NewInt(0),
			GForkBlock:          new(big.Int),
			Faker:               true,
		}
		signer     = types.LatestSigner(config)
		testKey, _ = crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")
	)
	var makeTx = func(nonce uint64, to common.Address, amount *big.Int, gasLimit uint64, gasPrice *big.Int, feeCurrency *common.Address, gatewayFeeRecipient *common.Address, gatewayFee *big.Int, data []byte) *types.Transaction {
		tx, _ := types.SignTx(types.NewTx(&types.LegacyTx{
			Nonce:               nonce,
			GasPrice:            gasPrice,
			Gas:                 gasLimit,
			FeeCurrency:         feeCurrency,
			GatewayFeeRecipient: gatewayFeeRecipient,
			GatewayFee:          gatewayFee,
			To:                  &to,
			Value:               amount,
			Data:                data,
		}), signer, testKey)
		return tx
	}

	var mkDynamicTx = func(nonce uint64, to common.Address, gasLimit uint64, gasTipCap, gasFeeCap *big.Int) *types.Transaction {
		tx, _ := types.SignTx(types.NewTx(&types.DynamicFeeTx{
			Nonce:     nonce,
			GasTipCap: gasTipCap,
			GasFeeCap: gasFeeCap,
			Gas:       gasLimit,
			To:        &to,
			Value:     big.NewInt(0),
		}), signer, testKey)
		return tx
	}
	{ // Tests against a 'recent' chain definition
		var (
			db    = rawdb.NewMemoryDatabase()
			gspec = &Genesis{
				Config: config,
				Alloc: GenesisAlloc{
					common.HexToAddress("0x71562b71999873DB5b286dF957af199Ec94617F7"): GenesisAccount{
						Balance: big.NewInt(params.Ether),
						Nonce:   0,
					},
				},
			}
			genesis       = gspec.MustCommit(db)
			blockchain, _ = NewBlockChain(db, nil, gspec.Config, mockEngine.NewFaker(), vm.Config{}, nil, nil)
		)
		defer blockchain.Stop()
		bigNumber := new(big.Int).SetBytes(common.FromHex("0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"))
		tooBigNumber := new(big.Int).Set(bigNumber)
		tooBigNumber.Add(tooBigNumber, common.Big1)
		for i, tt := range []struct {
			name string
			txs  []*types.Transaction
			want string
		}{
			{
				name: "ErrNonceTooLow",
				txs: []*types.Transaction{
					makeTx(0, common.Address{}, big.NewInt(0), params.TxGas, big.NewInt(875000000), nil, nil, nil, nil),
					makeTx(0, common.Address{}, big.NewInt(0), params.TxGas, big.NewInt(875000000), nil, nil, nil, nil),
				},
				want: "could not apply tx 1 [0x007b02f957123ca016999a9e826e6dfd4f7b36d2406f0057526aeb9a6202adfa]: nonce too low: address 0x71562b71999873DB5b286dF957af199Ec94617F7, tx: 0 state: 1",
			},
			{
				name: "ErrNonceTooHigh",
				txs: []*types.Transaction{
					makeTx(100, common.Address{}, big.NewInt(0), params.TxGas, big.NewInt(875000000), nil, nil, nil, nil),
				},
				want: "could not apply tx 0 [0xa0f823b5e4c81b1b0528531f25f06514b8d8c766de99747ad3137577987ff0b7]: nonce too high: address 0x71562b71999873DB5b286dF957af199Ec94617F7, tx: 100 state: 0",
			},
			{
				name: "ErrGasLimitReached",
				txs: []*types.Transaction{
					makeTx(0, common.Address{}, big.NewInt(0), 21000000, big.NewInt(875000000), nil, nil, nil, nil),
				},
				want: "could not apply tx 0 [0xbd4180e51b03a814b5c96016cc47575fdf777288fc165dc5ad3f9dd3e0e8dcee]: gas limit reached",
			},
			{
				name: "ErrInsufficientFundsForTransfer",
				txs: []*types.Transaction{
					makeTx(0, common.Address{}, big.NewInt(1000000000000000000), params.TxGas, big.NewInt(875000000), nil, nil, nil, nil),
				},
				want: "could not apply tx 0 [0x4610b4dc2b2508d3555499e34eed7315ad3b5215ec8dbc1cb22bda45e1667cf6]: insufficient funds for gas * price + value + gatewayFee: address 0x71562b71999873DB5b286dF957af199Ec94617F7 have 1000000000000000000 want 1000018375000000000",
			},
			{
				name: "ErrInsufficientFunds",
				txs: []*types.Transaction{
					makeTx(0, common.Address{}, big.NewInt(0), params.TxGas, big.NewInt(900000000000000000), nil, nil, nil, nil),
				},
				want: "could not apply tx 0 [0x99113426d253b063416fc51259f7f78c832d49a9bf09f2748fe50fd3130e7162]: insufficient funds for gas * price + value + gatewayFee: address 0x71562b71999873DB5b286dF957af199Ec94617F7 have 1000000000000000000 want 18900000000000000000000",
			},
			// ErrGasUintOverflow
			// One missing 'core' error is ErrGasUintOverflow: "gas uint64 overflow",
			// In order to trigger that one, we'd have to allocate a _huge_ chunk of data, such that the
			// multiplication len(data) +gas_per_byte overflows uint64. Not testable at the moment
			{
				name: "ErrIntrinsicGas",
				txs: []*types.Transaction{
					makeTx(0, common.Address{}, big.NewInt(0), params.TxGas-1000, big.NewInt(875000000), nil, nil, nil, nil),
				},
				want: "could not apply tx 0 [0x436f2e580794590a8627d8155b638c4ed1b41375283ba19f13a5a8b76ed51fdc]: intrinsic gas too low: have 20000, want 21000",
			},
			{
				name: "ErrGasLimitReached",
				txs: []*types.Transaction{
					makeTx(0, common.Address{}, big.NewInt(0), params.TxGas*1000, big.NewInt(875000000), nil, nil, nil, nil),
				},
				want: "could not apply tx 0 [0xbd4180e51b03a814b5c96016cc47575fdf777288fc165dc5ad3f9dd3e0e8dcee]: gas limit reached",
			},
			{
				name: "ErrFeeCapTooLow",
				txs: []*types.Transaction{
					mkDynamicTx(0, common.Address{}, params.TxGas, big.NewInt(0), big.NewInt(0)),
				},
				want: "could not apply tx 0 [0xc4ab868fef0c82ae0387b742aee87907f2d0fc528fc6ea0a021459fb0fc4a4a8]: max fee per gas less than block base fee: address 0x71562b71999873DB5b286dF957af199Ec94617F7, maxFeePerGas: 0 baseFee: 3", // MockSysContractCallCtx expects baseFee to be 3
			},
			{
				name: "ErrTipVeryHigh",
				txs: []*types.Transaction{
					mkDynamicTx(0, common.Address{}, params.TxGas, tooBigNumber, big.NewInt(1)),
				},
				want: "could not apply tx 0 [0x15b8391b9981f266b32f3ab7da564bbeb3d6c21628364ea9b32a21139f89f712]: max priority fee per gas higher than 2^256-1: address 0x71562b71999873DB5b286dF957af199Ec94617F7, maxPriorityFeePerGas bit length: 257",
			},
			{
				name: "ErrFeeCapVeryHigh",
				txs: []*types.Transaction{
					mkDynamicTx(0, common.Address{}, params.TxGas, big.NewInt(1), tooBigNumber),
				},
				want: "could not apply tx 0 [0x48bc299b83fdb345c57478f239e89814bb3063eb4e4b49f3b6057a69255c16bd]: max fee per gas higher than 2^256-1: address 0x71562b71999873DB5b286dF957af199Ec94617F7, maxFeePerGas bit length: 257",
			},
			{
				name: "ErrTipAboveFeeCap",
				txs: []*types.Transaction{
					mkDynamicTx(0, common.Address{}, params.TxGas, big.NewInt(2), big.NewInt(1)),
				},
				want: "could not apply tx 0 [0xf987a31ff0c71895780a7612f965a0c8b056deb54e020bb44fa478092f14c9b4]: max priority fee per gas higher than max fee per gas: address 0x71562b71999873DB5b286dF957af199Ec94617F7, maxPriorityFeePerGas: 2, maxFeePerGas: 1",
			},
			{ // ErrInsufficientFunds
				// Available balance:           1000000000000000000
				// Effective cost:                            84000
				// FeeCap * gas:                1050000000000000000
				// This test is designed to have the effective cost be covered by the balance, but
				// the extended requirement on FeeCap*gas < balance to fail
				name: "ErrInsufficientFunds",
				txs: []*types.Transaction{
					mkDynamicTx(0, common.Address{}, params.TxGas, big.NewInt(1), big.NewInt(50_000_000_000_000)),
				},
				want: "could not apply tx 0 [0x413603cd096a87f41b1660d3ed3e27d62e1da78eac138961c0a1314ed43bd129]: insufficient funds for gas * price + value + gatewayFee: address 0x71562b71999873DB5b286dF957af199Ec94617F7 have 1000000000000000000 want 1050000000000000000",
			},
			{ // Another ErrInsufficientFunds, this one to ensure that feecap/tip of max u256 is allowed
				name: "Another ErrInsufficientFunds",
				txs: []*types.Transaction{
					mkDynamicTx(0, common.Address{}, params.TxGas, bigNumber, bigNumber),
				},
				want: "could not apply tx 0 [0xd82a0c2519acfeac9a948258c47e784acd20651d9d80f9a1c67b4137651c3a24]: insufficient funds for gas * price + value + gatewayFee: address 0x71562b71999873DB5b286dF957af199Ec94617F7 have 1000000000000000000 want 2431633873983640103894990685182446064918669677978451844828609264166175722438635000",
			},
		} {
			block := GenerateBadBlock(genesis, mockEngine.NewFaker(), tt.txs, gspec.Config)
			_, err := blockchain.InsertChain(types.Blocks{block})
			if err == nil {
				t.Fatal("block imported without errors")
			}
			if have, want := err.Error(), tt.want; have != want {
				t.Errorf("test %d %v:\nhave \"%v\"\nwant \"%v\"\n", i, tt.name, have, want)
			}
		}
	}

	// ErrTxTypeNotSupported, For this, we need an older chain
	{
		var (
			db    = rawdb.NewMemoryDatabase()
			gspec = &Genesis{
				Config: &params.ChainConfig{
					ChainID:             big.NewInt(1),
					HomesteadBlock:      big.NewInt(0),
					EIP150Block:         big.NewInt(0),
					EIP155Block:         big.NewInt(0),
					EIP158Block:         big.NewInt(0),
					ByzantiumBlock:      big.NewInt(0),
					ConstantinopleBlock: big.NewInt(0),
					PetersburgBlock:     big.NewInt(0),
					IstanbulBlock:       big.NewInt(0),
				},
				Alloc: GenesisAlloc{
					common.HexToAddress("0x71562b71999873DB5b286dF957af199Ec94617F7"): GenesisAccount{
						Balance: big.NewInt(1000000000000000000), // 1 ether
						Nonce:   0,
					},
				},
			}
			genesis       = gspec.MustCommit(db)
			blockchain, _ = NewBlockChain(db, nil, gspec.Config, mockEngine.NewFaker(), vm.Config{}, nil, nil)
		)
		defer blockchain.Stop()
		for i, tt := range []struct {
			txs  []*types.Transaction
			want string
		}{
			{ // ErrTxTypeNotSupported
				txs: []*types.Transaction{
					mkDynamicTx(0, common.Address{}, params.TxGas-1000, big.NewInt(0), big.NewInt(0)),
				},
				want: "could not apply tx 0 [0x88626ac0d53cb65308f2416103c62bb1f18b805573d4f96a3640bbbfff13c14f]: transaction type not supported",
			},
		} {
			block := GenerateBadBlock(genesis, mockEngine.NewFaker(), tt.txs, gspec.Config)
			_, err := blockchain.InsertChain(types.Blocks{block})
			if err == nil {
				t.Fatal("block imported without errors")
			}
			if have, want := err.Error(), tt.want; have != want {
				t.Errorf("test %d:\nhave \"%v\"\nwant \"%v\"\n", i, have, want)
			}
		}
	}

	// // We do not check for EOAs at this time.
	// // ErrSenderNoEOA, for this we need the sender to have contract code
	// {
	// 	var (
	// 		db    = rawdb.NewMemoryDatabase()
	// 		gspec = &Genesis{
	// 			Config: config,
	// 			Alloc: GenesisAlloc{
	// 				common.HexToAddress("0x71562b71999873DB5b286dF957af199Ec94617F7"): GenesisAccount{
	// 					Balance: big.NewInt(1000000000000000000), // 1 ether
	// 					Nonce:   0,
	// 					Code:    common.FromHex("0xB0B0FACE"),
	// 				},
	// 			},
	// 		}
	// 		genesis       = gspec.MustCommit(db)
	// 		blockchain, _ = NewBlockChain(db, nil, gspec.Config, mockEngine.NewFaker(), vm.Config{}, nil, nil)
	// 	)
	// 	defer blockchain.Stop()
	// 	for i, tt := range []struct {
	// 		txs  []*types.Transaction
	// 		want string
	// 	}{
	// 		{ // ErrSenderNoEOA
	// 			txs: []*types.Transaction{
	// 				mkDynamicTx(0, common.Address{}, params.TxGas-1000, big.NewInt(0), big.NewInt(0)),
	// 			},
	// 			want: "could not apply tx 0 [0x88626ac0d53cb65308f2416103c62bb1f18b805573d4f96a3640bbbfff13c14f]: sender not an eoa: address 0x71562b71999873DB5b286dF957af199Ec94617F7, codehash: 0x9280914443471259d4570a8661015ae4a5b80186dbc619658fb494bebc3da3d1",
	// 		},
	// 	} {
	// 		block := GenerateBadBlock(genesis, mockEngine.NewFaker(), tt.txs, gspec.Config)
	// 		_, err := blockchain.InsertChain(types.Blocks{block})
	// 		if err == nil {
	// 			t.Fatal("block imported without errors")
	// 		}
	// 		if have, want := err.Error(), tt.want; have != want {
	// 			t.Errorf("test %d:\nhave \"%v\"\nwant \"%v\"\n", i, have, want)
	// 		}
	// 	}
	// }
}

// GenerateBadBlock constructs a "block" which contains the transactions. The transactions are not expected to be
// valid, and no proper post-state can be made. But from the perspective of the blockchain, the block is sufficiently
// valid to be considered for import:
// - valid pow (fake), ancestry, difficulty, gaslimit etc
func GenerateBadBlock(parent *types.Block, engine consensus.Engine, txs types.Transactions, config *params.ChainConfig) *types.Block {
	header := &types.Header{
		ParentHash: parent.Hash(),
		Coinbase:   parent.Coinbase(),
		Number:     new(big.Int).Add(parent.Number(), common.Big1),
		Time:       parent.Time() + 10,
		Extra:      CreateEmptyIstanbulExtra(nil),
	}
	header.Extra = CreateEmptyIstanbulExtra(header.Extra)

	var receipts []*types.Receipt
	// The post-state result doesn't need to be correct (this is a bad block), but we do need something there
	// Preferably something unique. So let's use a combo of blocknum + txhash
	hasher := sha3.NewLegacyKeccak256()
	hasher.Write(header.Number.Bytes())
	var cumulativeGas uint64
	for _, tx := range txs {
		txh := tx.Hash()
		hasher.Write(txh[:])
		receipt := types.NewReceipt(nil, false, cumulativeGas+tx.Gas())
		receipt.TxHash = tx.Hash()
		receipt.GasUsed = tx.Gas()
		receipts = append(receipts, receipt)
		cumulativeGas += tx.Gas()
	}
	header.Root = common.BytesToHash(hasher.Sum(nil))
	// Assemble and return the final block for sealing
	return types.NewBlock(header, txs, receipts, nil, trie.NewStackTrie(nil))

}
