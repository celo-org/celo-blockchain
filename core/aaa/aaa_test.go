package aaa

import (
	"math/big"
	"testing"

	"github.com/celo-org/celo-blockchain/common"
	mockEngine "github.com/celo-org/celo-blockchain/consensus/consensustest"
	"github.com/celo-org/celo-blockchain/core"
	"github.com/celo-org/celo-blockchain/core/rawdb"
	"github.com/celo-org/celo-blockchain/core/types"
	"github.com/celo-org/celo-blockchain/core/vm"
	"github.com/celo-org/celo-blockchain/crypto"
	genesis2 "github.com/celo-org/celo-blockchain/mycelo/genesis"
	"github.com/celo-org/celo-blockchain/params"
	"github.com/celo-org/celo-blockchain/test"
)

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
		//gspec   = &core.Genesis{
		//	Config: params.IstanbulEHFTestChainConfig,
		//	Alloc: core.GenesisAlloc{
		//		addr1: {Balance: funds},
		//		addr2: {Balance: funds},
		//		// The address 0xAAAA sloads 0x00 and 0x01
		//		aa: {
		//			Code: []byte{
		//				byte(vm.PC),
		//				byte(vm.PC),
		//				byte(vm.SLOAD),
		//				byte(vm.SLOAD),
		//			},
		//			Nonce:   0,
		//			Balance: big.NewInt(0),
		//		},
		//	},
		//}
	)

	accounts := test.Accounts(1)
	gc, ec, _ := test.BuildConfig(accounts)
	gc.Istanbul = params.IstanbulConfig{
		Epoch:          ec.Istanbul.Epoch,
		ProposerPolicy: uint64(ec.Istanbul.ProposerPolicy),
		LookbackWindow: ec.Istanbul.DefaultLookbackWindow,
		BlockPeriod:    ec.Istanbul.BlockPeriod,
		RequestTimeout: ec.Istanbul.RequestTimeout,
	}
	gspec, err := genesis2.GenerateGenesis(accounts, gc, "../../compiled-system-contracts")
	gspec.Config = params.IstanbulEHFTestChainConfig
	gspec.Config.DonutBlock = common.Big0
	gspec.Config.EBlock = common.Big0
	gspec.Config.Faker = true
	gspec.Alloc[addr1] = core.GenesisAccount{Balance: funds}
	gspec.Alloc[addr2] = core.GenesisAccount{Balance: funds}
	gspec.Alloc[aa] = core.GenesisAccount{
		Code: []byte{
			byte(vm.PC),
			byte(vm.PC),
			byte(vm.SLOAD),
			byte(vm.SLOAD),
		},
		Nonce:   0,
		Balance: big.NewInt(0),
	}
	genesis := gspec.MustCommit(db)
	signer := types.LatestSigner(gspec.Config)
	txFeeRecipient := common.Address{10}

	blocks, _ := core.GenerateChain(gspec.Config, genesis, engine, db, 1, func(i int, b *core.BlockGen) {
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
			GasFeeCap:  new(big.Int).Mul(big.NewInt(5), big.NewInt(params.GWei)),
			GasTipCap:  big.NewInt(2),
			AccessList: accesses,
			Data:       []byte{},
		}
		tx := types.NewTx(txdata)
		tx, _ = types.SignTx(tx, signer, key1)

		b.AddTx(tx)
	})

	diskdb := rawdb.NewMemoryDatabase()
	gspec.MustCommit(diskdb)

	chain, err := core.NewBlockChain(diskdb, nil, gspec.Config, engine, vm.Config{}, nil, nil)
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
	baseFee := core.MockSysContractCallCtx().GetGasPriceMinimum(nil).Uint64()
	actual = new(big.Int).Sub(funds, state.GetBalance(addr1))
	expected = new(big.Int).SetUint64(block.GasUsed() * (block.Transactions()[0].GasTipCap().Uint64() + baseFee))
	if actual.Cmp(expected) != 0 {
		t.Fatalf("sender paid fee incorrect: expected %d, got %d", expected, actual)
	}

	blocks, _ = core.GenerateChain(gspec.Config, block, engine, db, 1, func(i int, b *core.BlockGen) {
		b.SetCoinbase(common.Address{2})

		txdata := &types.LegacyTx{
			Nonce:    0,
			To:       &aa,
			Gas:      30000,
			GasPrice: new(big.Int).Mul(big.NewInt(5), big.NewInt(params.GWei)),
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
