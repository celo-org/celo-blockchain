package e2e_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/celo-org/celo-blockchain/core/types"
	"github.com/celo-org/celo-blockchain/test"
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
	accounts := test.Accounts(3)
	gc, ec, err := test.BuildConfig(accounts)
	require.NoError(t, err)
	network, err := test.NewNetwork(accounts, gc, ec)
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

func TestStartStopValidators(t *testing.T) {
	accounts := test.Accounts(4)
	gc, ec, err := test.BuildConfig(accounts)
	require.NoError(t, err)
	network, err := test.NewNetwork(accounts, gc, ec)
	require.NoError(t, err)
	defer network.Shutdown()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
	defer cancel()

	var txs []*types.Transaction

	println("------------------------------------ sending first tx")

	// Send 1 celo from the dev account attached to node 0 to the dev account
	// attached to node 1.
	tx, err := network[0].SendCelo(ctx, network[1].DevAddress, 1)
	require.NoError(t, err)
	txs = append(txs, tx)

	println("------------------------------------ awaiting first tx")
	// Wait for the whole network to process the transaction.
	err = network.AwaitTransactions(ctx, txs...)
	require.NoError(t, err)
	println("------------------------------------ received first tx")

	println("------------------------------------ stopping first node")
	// Stop one node, the rest of the network should still be able to progress
	err = network[3].Close()
	require.NoError(t, err)
	println("------------------------------------ stopped first node")
	println("------------------------------------ sending second tx")

	// Send 1 celo from the dev account attached to node 0 to the dev account
	// attached to node 1.
	tx, err = network[0].SendCelo(ctx, network[1].DevAddress, 1)
	require.NoError(t, err)
	txs = append(txs, tx)

	println("------------------------------------ awaiting second tx")
	// Check that the remaining network can still process this transction.
	err = network[:3].AwaitTransactions(ctx, txs...)
	require.NoError(t, err)
	println("------------------------------------ received second tx")

	println("------------------------------------ stopping second node")
	// Stop another node, the network should now be stuck
	err = network[2].Close()
	require.NoError(t, err)
	println("------------------------------------ stopped second node")

	// Now we will check that the network does not process transactions in this
	// state, by waiting for a reasonable amount of time for it to process a
	// transaction and assuming it is not processing transactions if we time out.
	shortCtx, cancel := context.WithTimeout(context.Background(), time.Second*2)
	defer cancel()
	// Send 1 celo from the dev account attached to node 0 to the dev account
	// attached to node 1.
	println("------------------------------------ sending third tx")
	tx, err = network[0].SendCelo(shortCtx, network[1].DevAddress, 1)
	require.NoError(t, err)
	txs = append(txs, tx)

	println("------------------------------------ awaiting third tx")
	err = network[:2].AwaitTransactions(shortCtx, txs...)
	// Expect DeadlineExceeded error
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expecting %q, instead got: %v ", context.DeadlineExceeded.Error(), err)
	}
	println("------------------------------------ did not receive third tx (good)")

	println("------------------------------------ starting second node")
	// Start the last stopped node
	err = network[2].Start()
	require.NoError(t, err)
	println("------------------------------------ started second node")

	// Connect last stopped node to running nodes
	network[2].AddPeers(network[:2]...)
	time.Sleep(25 * time.Millisecond)
	// println("gossipingenode")
	for _, n := range network[:3] {
		err = n.GossipEnodeCertificatge()
		require.NoError(t, err)
	}

	println("------------------------------------ awaiting third tx again")
	// Check that the  network now processes the previous transaction.
	err = network[:3].AwaitTransactions(ctx, txs...)
	require.NoError(t, err)
	println("------------------------------------ recived third tx again")

	// Check that the network now quickly processes incoming transactions.
	println("------------------------------------ sending fourth tx")
	tx, err = network[0].SendCelo(ctx, network[1].DevAddress, 1)
	require.NoError(t, err)
	txs = append(txs, tx)

	println("------------------------------------ awaiting fourth tx")
	err = network[:3].AwaitTransactions(ctx, txs...)
	require.NoError(t, err)

	println("------------------------------------ starting first node")
	// Start the first stopped node
	err = network[3].Start()
	require.NoError(t, err)
	println("------------------------------------ started first node")

	// Connect final node to
	network[3].AddPeers(network[:3]...)
	time.Sleep(25 * time.Millisecond)
	// println("gossipingenode")
	for _, n := range network {
		err = n.GossipEnodeCertificatge()
		require.NoError(t, err)
	}

	// Check that the network continues to quickly processes incoming transactions.
	println("------------------------------------ sending fifth tx")
	tx, err = network[0].SendCelo(ctx, network[1].DevAddress, 1)
	require.NoError(t, err)
	txs = append(txs, tx)

	println("------------------------------------ awaiting fifth tx")
	err = network.AwaitTransactions(ctx, txs...)
	require.NoError(t, err)

}

// func TestWaitAroundForABit(t *testing.T) {
// 	accounts := test.Accounts(4)
// 	gc, ec, err := test.BuildConfig(accounts)
// 	require.NoError(t, err)
// 	network, err := test.NewNetwork(accounts, gc, ec)
// 	require.NoError(t, err)
// 	defer network.Shutdown()
// 	// ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
// 	// defer cancel()

// 	println("------------------------------------ wait for 100s")
// 	time.Sleep(100 * time.Second)
// 	println("------------------------------------ waited for 100s")

// }
