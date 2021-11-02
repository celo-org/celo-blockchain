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
		signer     = types.HomesteadSigner{}
		testKey, _ = crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")
		db         = rawdb.NewMemoryDatabase()
		gspec      = &Genesis{
			Config: params.TestChainConfig,
		}
		genesis       = gspec.MustCommit(db)
		blockchain, _ = NewBlockChain(db, nil, gspec.Config, mockEngine.NewFaker(), vm.Config{}, nil, nil)
	)
	defer blockchain.Stop()
	var makeTx = func(nonce uint64, to common.Address, amount *big.Int, gasLimit uint64, gasPrice *big.Int, feeCurrency *common.Address, gatewayFeeRecipient *common.Address, gatewayFee *big.Int, data []byte) *types.Transaction {
		tx, _ := types.SignTx(types.NewTransaction(nonce, to, amount, gasLimit, gasPrice, feeCurrency, gatewayFeeRecipient, gatewayFee, data), signer, testKey)
		return tx
	}
	for i, tt := range []struct {
		txs  []*types.Transaction
		want string
	}{
		{
			txs: []*types.Transaction{
				makeTx(0, common.Address{}, big.NewInt(0), params.TxGas, nil, nil, nil, nil, nil),
				makeTx(0, common.Address{}, big.NewInt(0), params.TxGas, nil, nil, nil, nil, nil),
			},
			want: "could not apply tx 1 [0xd7d4ec936b98af7dfe88835c151ae30d6c1a1aec02a6165443927349928666d7]: nonce too low: address 0x71562b71999873DB5b286dF957af199Ec94617F7, tx: 0 state: 1",
		},
		{
			txs: []*types.Transaction{
				makeTx(100, common.Address{}, big.NewInt(0), params.TxGas, nil, nil, nil, nil, nil),
			},
			want: "could not apply tx 0 [0xc6fe0942985c8bd3727ed0705d156880b1fe5f12d8f53a7691bee4322357abf7]: nonce too high: address 0x71562b71999873DB5b286dF957af199Ec94617F7, tx: 100 state: 0",
		},
		{
			txs: []*types.Transaction{
				makeTx(0, common.Address{}, big.NewInt(0), 21000000, nil, nil, nil, nil, nil),
			},
			want: "could not apply tx 0 [0x3ae48e34b0a5bbc796eadee31de3a5d89f64976cc36400b467ec542ebbfa5967]: gas limit reached",
		},
		{
			txs: []*types.Transaction{
				makeTx(0, common.Address{}, big.NewInt(1), params.TxGas, nil, nil, nil, nil, nil),
			},
			want: "could not apply tx 0 [0x58e8ab3dd574583c5195687aae5e9a3f8c3680801422b8ecdb73ca0ba0f0c200]: insufficient funds for transfer (after fees): address 0x71562b71999873DB5b286dF957af199Ec94617F7",
		},
		{
			txs: []*types.Transaction{
				makeTx(0, common.Address{}, big.NewInt(0), params.TxGas, big.NewInt(0xffffff), nil, nil, nil, nil),
			},
			want: "could not apply tx 0 [0x1dd95c056fa099d248f03edebd6de39d65e99f6eb3944bd03dab86296b1cf962]: insufficient funds to pay for fees",
		},
		{
			txs: []*types.Transaction{
				makeTx(0, common.Address{}, big.NewInt(0), params.TxGas, nil, nil, nil, nil, nil),
				makeTx(1, common.Address{}, big.NewInt(0), params.TxGas, nil, nil, nil, nil, nil),
				makeTx(2, common.Address{}, big.NewInt(0), params.TxGas, nil, nil, nil, nil, nil),
				makeTx(3, common.Address{}, big.NewInt(0), params.TxGas-1000, big.NewInt(0), nil, nil, nil, nil),
			},
			want: "could not apply tx 3 [0x28ef3bcf5511bc6073ce432d806cbf86e1e4556fc2006a4f4ca4398eb94113dd]: intrinsic gas too low: have 20000, want 21000",
		},
		// The last 'core' error is ErrGasUintOverflow: "gas uint64 overflow", but in order to
		// trigger that one, we'd have to allocate a _huge_ chunk of data, such that the
		// multiplication len(data) +gas_per_byte overflows uint64. Not testable at the moment
	} {
		block := GenerateBadBlock(genesis, mockEngine.NewFaker(), tt.txs)
		_, err := blockchain.InsertChain(types.Blocks{block})
		if err == nil {
			t.Fatal("block imported without errors")
		}
		if have, want := err.Error(), tt.want; have != want {
			t.Errorf("test %d:\nhave \"%v\"\nwant \"%v\"\n", i, have, want)
		}
	}
}

// GenerateBadBlock constructs a "block" which contains the transactions. The transactions are not expected to be
// valid, and no proper post-state can be made. But from the perspective of the blockchain, the block is sufficiently
// valid to be considered for import:
// - valid pow (fake), ancestry, difficulty, gaslimit etc
func GenerateBadBlock(parent *types.Block, engine consensus.Engine, txs types.Transactions) *types.Block {
	header := &types.Header{
		ParentHash: parent.Hash(),
		Coinbase:   parent.Coinbase(),
		// Difficulty: engine.CalcDifficulty(&fakeChainReader{params.TestChainConfig}, parent.Time()+10, &types.Header{
		// 	Number:     parent.Number(),
		// 	Time:       parent.Time(),
		// 	Difficulty: parent.Difficulty(),
		// 	UncleHash:  parent.UncleHash(),
		// }),
		// GasLimit:  CalcGasLimit(parent, parent.GasLimit(), parent.GasLimit()),
		Number: new(big.Int).Add(parent.Number(), common.Big1),
		Time:   parent.Time() + 10,
		// UncleHash: types.EmptyUncleHash,
	}
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
	return types.NewBlock(header, txs, receipts, nil, new(trie.Trie))
}
