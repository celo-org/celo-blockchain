package e2e_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/core/types"
	"github.com/celo-org/celo-blockchain/node"
	"github.com/celo-org/celo-blockchain/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() {
	// This statement is commented out but left here since its very useful for
	// debugging problems and its non trivial to construct.
	//
	// log.Root().SetHandler(log.LvlFilterHandler(log.LvlDebug, log.StreamHandler(os.Stdout, log.TerminalFormat(true))))
}

// This test starts a network submits a transaction and waits for the whole
// network to process the transaction.
func TestSendCelo(t *testing.T) {
	ac := test.AccountConfig(3, 2)
	gc, ec, err := test.BuildConfig(ac)
	require.NoError(t, err)
	network, shutdown, err := test.NewNetwork(ac, gc, ec)
	require.NoError(t, err)
	defer shutdown()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
	defer cancel()

	accounts := test.Accounts(ac.DeveloperAccounts(), gc.ChainConfig())

	// Send one celo from external account 0 to 1 via node 0.
	tx, err := accounts[0].SendCelo(ctx, accounts[1].Address, 1, network[0])
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

// This test is intended to ensure that epoch blocks can be correctly marshalled.
// We previously had an open bug for this https://github.com/celo-org/celo-blockchain/issues/1574
func TestEpochBlockMarshaling(t *testing.T) {
	accounts := test.AccountConfig(1, 0)
	gc, ec, err := test.BuildConfig(accounts)
	require.NoError(t, err)

	// Configure the shortest possible epoch, uptimeLookbackWindow minimum is 3
	// and it needs to be < (epoch -2).
	ec.Istanbul.Epoch = 6
	ec.Istanbul.DefaultLookbackWindow = 3
	network, shutdown, err := test.NewNetwork(accounts, gc, ec)
	require.NoError(t, err)
	defer shutdown()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
	defer cancel()

	// Wait for the whole network to process the transaction.
	err = network.AwaitBlock(ctx, 6)
	require.NoError(t, err)
	b := network[0].Tracker.GetProcessedBlock(6)

	// Check that epoch snark data was actually unmarshalled, I.E there was
	// something there.
	assert.True(t, len(b.EpochSnarkData().Signature) > 0)
	assert.True(t, b.EpochSnarkData().Bitmap.Uint64() > 0)
}

// This test checks that a network can have validators shut down mid operation
// and that it can continue to function, it also checks that if more than f
// validators are shut down, when they restart the network is able to continue.
func TestStartStopValidators(t *testing.T) {
	ac := test.AccountConfig(4, 2)
	gc, ec, err := test.BuildConfig(ac)
	require.NoError(t, err)
	network, _, err := test.NewNetwork(ac, gc, ec)
	require.NoError(t, err)

	// We define our own shutdown function because we don't want to print
	// errors about already stopped nodes. Since this test can fail while we
	// have stopped nodes.
	defer func() {
		for _, err := range network.Shutdown() {
			if !errors.Is(err, test.ErrTrackerAlreadyStopped) && !errors.Is(err, node.ErrNodeStopped) {
				fmt.Println(err.Error())
			}
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*40)
	defer cancel()

	var txs []*types.Transaction

	accounts := test.Accounts(ac.DeveloperAccounts(), gc.ChainConfig())

	// Send one celo from external account 0 to 1 via node 0.
	tx, err := accounts[0].SendCelo(ctx, accounts[1].Address, 1, network[0])
	require.NoError(t, err)
	txs = append(txs, tx)

	// Wait for the whole network to process the transaction.
	err = network.AwaitTransactions(ctx, txs...)
	require.NoError(t, err)

	// Stop one node, the rest of the network should still be able to progress
	err = network[3].Close()
	require.NoError(t, err)

	// Send one celo from external account 0 to 1 via node 0.
	tx, err = accounts[0].SendCelo(ctx, accounts[1].Address, 1, network[0])
	require.NoError(t, err)
	txs = append(txs, tx)

	// Check that the remaining network can still process this transction.
	err = network[:3].AwaitTransactions(ctx, txs...)
	require.NoError(t, err)

	// Stop another node, the network should now be stuck
	err = network[2].Close()
	require.NoError(t, err)

	// Now we will check that the network does not process transactions in this
	// state, by waiting for a reasonable amount of time for it to process a
	// transaction and assuming it is not processing transactions if we time out.
	shortCtx, cancel := context.WithTimeout(context.Background(), time.Second*2)
	defer cancel()
	// Send one celo from external account 0 to 1 via node 0.
	tx, err = accounts[0].SendCelo(ctx, accounts[1].Address, 1, network[0])
	require.NoError(t, err)
	txs = append(txs, tx)

	err = network[:2].AwaitTransactions(shortCtx, txs...)
	// Expect DeadlineExceeded error
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expecting %q, instead got: %v ", context.DeadlineExceeded.Error(), err)
	}

	// Start the last stopped node
	err = network[2].Start()
	require.NoError(t, err)

	// Connect last stopped node to running nodes
	network[2].AddPeers(network[:2]...)
	time.Sleep(25 * time.Millisecond)
	for _, n := range network[:3] {
		err = n.GossipEnodeCertificatge()
		require.NoError(t, err)
	}

	// Check that the  network now processes the previous transaction.
	err = network[:3].AwaitTransactions(ctx, txs...)
	require.NoError(t, err)

	// Check that the network now quickly processes incoming transactions.
	// Send one celo from external account 0 to 1 via node 0.
	tx, err = accounts[0].SendCelo(ctx, accounts[1].Address, 1, network[0])
	require.NoError(t, err)
	txs = append(txs, tx)

	err = network[:3].AwaitTransactions(ctx, txs...)
	require.NoError(t, err)

	// Start the first stopped node
	err = network[3].Start()
	require.NoError(t, err)

	// Connect final node to rest of network
	network[3].AddPeers(network[:3]...)
	time.Sleep(25 * time.Millisecond)
	for _, n := range network {
		err = n.GossipEnodeCertificatge()
		require.NoError(t, err)
	}

	// Check that the network continues to quickly processes incoming transactions.
	// Send one celo from external account 0 to 1 via node 0.
	tx, err = accounts[0].SendCelo(ctx, accounts[1].Address, 1, network[0])
	require.NoError(t, err)
	txs = append(txs, tx)

	err = network.AwaitTransactions(ctx, txs...)
	require.NoError(t, err)

}
