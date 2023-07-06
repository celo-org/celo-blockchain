package e2e_test

import (
	"context"
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/celo-org/celo-blockchain/core/types"
	"github.com/celo-org/celo-blockchain/test"
	"github.com/stretchr/testify/require"
)

func BenchmarkNet100EmptyBlocks(b *testing.B) {
	for _, n := range []int{1, 3, 9} {
		b.Run(fmt.Sprintf("%dNodes", n), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				ac := test.AccountConfig(n, 0)
				gc, ec, err := test.BuildConfig(ac, true)
				require.NoError(b, err)
				network, shutdown, err := test.NewNetwork(ac, gc, ec)
				require.NoError(b, err)
				defer shutdown()
				ctx, cancel := context.WithTimeout(context.Background(), time.Second*60*time.Duration(n))
				defer cancel()
				b.ResetTimer()
				err = network.AwaitBlock(ctx, 100)
				require.NoError(b, err)
			}
		})
	}
}

func BenchmarkNet1000Txs(b *testing.B) {
	b.Skip() // Flaky
	// Seed the random number generator so that the generated numbers are
	// different on each run.
	rand.Seed(time.Now().UnixNano())
	for _, n := range []int{1, 3, 9} {
		b.Run(fmt.Sprintf("%dNodes", n), func(b *testing.B) {
			for i := 0; i < b.N; i++ {

				ac := test.AccountConfig(n, n)
				gc, ec, err := test.BuildConfig(ac, true)
				require.NoError(b, err)
				accounts := test.Accounts(ac.DeveloperAccounts(), gc.ChainConfig())
				network, shutdown, err := test.NewNetwork(ac, gc, ec)
				require.NoError(b, err)
				defer shutdown()
				ctx, cancel := context.WithTimeout(context.Background(), time.Second*60*time.Duration(n))
				defer cancel()
				b.ResetTimer()

				// Send 1000 txs randomly between nodes accounts sending via a
				// random node.
				txs := make([]*types.Transaction, 1000)
				for i := range txs {
					sender := accounts[rand.Intn(n)]
					receiver := accounts[rand.Intn(n)]
					node := network[rand.Intn(n)]
					tx, err := sender.SendCelo(ctx, receiver.Address, 1, node)
					require.NoError(b, err)
					txs[i] = tx
				}
				err = network.AwaitTransactions(ctx, txs...)
				require.NoError(b, err)
				block := network[0].Tracker.GetProcessedBlockForTx(txs[len(txs)-1].Hash())
				fmt.Printf("Processed 1000 txs in %d blocks\n", block.NumberU64())
			}
		})
	}
}
