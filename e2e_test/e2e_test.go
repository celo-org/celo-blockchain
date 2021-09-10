package e2e_test

import (
	"context"
	"testing"
	"time"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/core/types"
	"github.com/celo-org/celo-blockchain/test"
	"github.com/stretchr/testify/require"
)

func init() {
	// This statement is commented out but left here since its very useful for
	// debugging problems and its non trivial to construct.
	//
	//log.Root().SetHandler(log.LvlFilterHandler(log.LvlTrace, log.StreamHandler(os.Stdout, log.TerminalFormat(true))))
}

// This test starts a network submits a transaction and waits for the whole
// network to process the transaction.
func TestSendCelo(t *testing.T) {
	accounts := test.Accounts(3)
	gc := test.GenesisConfig(accounts)
	network, err := test.NewNetwork(accounts, gc)
	require.NoError(t, err)
	defer network.Shutdown()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
	defer cancel()

	// Send 1 celo from the dev account attached to node 0 to the dev account
	// attached to node 1.
	tx, err := network[0].SendCelo(ctx, network[1].DevAddress, 1)
	require.NoError(t, err)

	// Wait for the whole network to process the transaction.
	err = network.AwaitTransactions(ctx, tx)
	require.NoError(t, err)
}

func TestSingleNodeManyTxs(t *testing.T) {
	rounds := 100
	txsPerRound := 5
	accounts := test.Accounts(1)
	gc := test.GenesisConfig(accounts)
	gc.Istanbul.Epoch = uint64(rounds) * 50 // avoid the epoch for this test
	network, err := test.NewNetwork(accounts, gc)
	require.NoError(t, err)
	defer network.Shutdown()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
	defer cancel()

	for r := 0; r < rounds; r++ {
		txs := make([]*types.Transaction, 0, txsPerRound)
		for j := 0; j < txsPerRound; j++ {
			tx, err := network[0].SendCelo(ctx, common.Address{}, 1)
			require.NoError(t, err)
			require.NotNil(t, tx)
		}
		err = network.AwaitTransactions(ctx, txs...)
		require.NoError(t, err)
	}
}

func TestManyNodes(t *testing.T) {
	accounts := test.Accounts(4)
	gc := test.GenesisConfig(accounts)
	rounds := 5
	txsPerNodePerRound := 3
	gc.Istanbul.Epoch = 10000                               // avoid the epoch
	network, err := test.NewConcurrentNetwork(accounts, gc) // start concurrently all nodes
	require.NoError(t, err)
	defer network.Shutdown()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*15)
	defer cancel()

	for r := 0; r < rounds; r++ {
		txs := make([]*types.Transaction, 0, len(network)*txsPerNodePerRound)
		for i := range network {
			for j := 0; j < txsPerNodePerRound; j++ {
				tx, err := network[i].SendCelo(ctx, network[(i+1)%len(network)].Address, 1)
				require.NoError(t, err)
				require.NotNil(t, tx)
				txs = append(txs, tx)
			}
		}
		err = network.AwaitTransactions(ctx, txs...)
		require.NoError(t, err)
	}
}
