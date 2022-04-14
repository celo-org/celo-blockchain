package ethclient

import (
	"math/big"
	"testing"
	"time"

	"github.com/celo-org/celo-blockchain/common"
	mockEngine "github.com/celo-org/celo-blockchain/consensus/consensustest"
	"github.com/celo-org/celo-blockchain/core"
	"github.com/celo-org/celo-blockchain/core/rawdb"
	"github.com/celo-org/celo-blockchain/core/types"
	"github.com/celo-org/celo-blockchain/eth"
	"github.com/celo-org/celo-blockchain/eth/ethconfig"
	"github.com/celo-org/celo-blockchain/node"
	"github.com/celo-org/celo-blockchain/params"
	"github.com/stretchr/testify/require"
)

func NewTestBackend(t *testing.T, testAddr common.Address, testBalance *big.Int) (*node.Node, []*types.Block) {
	// Generate test chain.
	genesis, blocks := generateTestChain(testAddr, testBalance)

	// Create node
	n, err := node.New(&node.Config{})
	require.NoError(t, err, "can't create new node")

	// Create Ethereum Service
	config := &ethconfig.Config{Genesis: genesis}
	ethservice, err := eth.New(n, config)
	require.NoError(t, err, "can't create new ethereum service")

	// Import the test chain.
	err = n.Start()
	require.NoError(t, err, "can't start test node")

	_, err = ethservice.BlockChain().InsertChain(blocks[1:])
	require.NoError(t, err, "can't import test blocks")

	require.Eventually(t, ethservice.IsListening, 5*time.Second, 100*time.Millisecond)

	return n, blocks
}

func generateTestChain(testAddr common.Address, testBalance *big.Int) (*core.Genesis, []*types.Block) {
	db := rawdb.NewMemoryDatabase()
	config := params.TestChainConfig

	engine := mockEngine.NewFaker()

	genesis := &core.Genesis{
		Config:    config,
		Alloc:     core.GenesisAlloc{testAddr: {Balance: testBalance}},
		ExtraData: []byte("test genesis"),
		Timestamp: 9000,
	}
	generate := func(i int, g *core.BlockGen) {
		g.OffsetTime(5)
		g.SetExtra(core.CreateEmptyIstanbulExtra([]byte("test")))
	}
	gblock := genesis.ToBlock(db)
	blocks, _ := core.GenerateChain(config, gblock, engine, db, 1, generate)
	blocks = append([]*types.Block{gblock}, blocks...)
	return genesis, blocks
}
