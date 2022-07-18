package e2e_test

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"sync"
	"testing"
	"time"

	"github.com/celo-org/celo-blockchain/common/hexutil"
	"github.com/celo-org/celo-blockchain/core/types"
	"github.com/celo-org/celo-blockchain/log"
	"github.com/celo-org/celo-blockchain/node"
	"github.com/celo-org/celo-blockchain/params"
	"github.com/celo-org/celo-blockchain/rpc"
	"github.com/celo-org/celo-blockchain/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() {
	// This statement is commented out but left here since its very useful for
	// debugging problems and its non trivial to construct.
	//
	// log.Root().SetHandler(log.LvlFilterHandler(log.LvlError, log.StreamHandler(os.Stdout, log.TerminalFormat(true))))

	// This disables all logging which in general we want, because there is a lot
	log.Root().SetHandler(log.DiscardHandler())
}

// This test starts a network submits a transaction and waits for the whole
// network to process the transaction.
func TestSendCelo(t *testing.T) {

	// Add lots of data into the transaction block gas limit is 50000000
	bgl := 50000000
	remaining := bgl - int(params.TxGas)
	maxCalldataSize := remaining / int(params.TxDataZeroGas)
	minSize, maxSize := 0, maxCalldataSize
	for minSize < maxSize {
		time.Sleep(time.Second)
		target := (minSize + maxSize) / 2
		println()
		println("calldata size", target)
		ac := test.AccountConfig(3, 2)
		gc, ec, err := test.BuildConfig(ac)
		require.NoError(t, err)
		network, shutdown, err := test.NewNetwork(ac, gc, ec)
		require.NoError(t, err)

		ctx, cancel := context.WithTimeout(context.Background(), time.Second*4)
		defer cancel()

		accounts := test.Accounts(ac.DeveloperAccounts(), gc.ChainConfig())

		// Send one celo from external account 0 to 1 via node 0.
		tx, err := accounts[0].SendCelo(ctx, accounts[1].Address, 1, network[0], target)
		require.NoError(t, err)

		// Wait for the whole network to process the transaction.
		err = network.AwaitTransactions(ctx, tx)
		if errors.Is(err, context.DeadlineExceeded) {
			maxSize = target
			shutdown()
			continue
		}
		if err != nil {
			shutdown()
			t.Fatalf("Error awaiting tranasaction %v", err)
		} else {
			shutdown()
			minSize = target
		}
	}
	println("result", minSize, maxSize)
	// println("calldata size mb", calldataSize/(1024*1024))
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

// This test was created to reproduce the concurrent map access error in
// https://github.com/celo-org/celo-blockchain/issues/1799
//
// It does this by calling debug_traceBlockByNumber a number of times since the
// trace block code was the source of the concurrent map access.
func TestBlockTracingConcurrentMapAccess(t *testing.T) {
	ac := test.AccountConfig(1, 2)
	gc, ec, err := test.BuildConfig(ac)
	require.NoError(t, err)
	network, shutdown, err := test.NewNetwork(ac, gc, ec)
	require.NoError(t, err)
	defer shutdown()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*60)
	defer cancel()

	n := network[0]

	accounts := test.Accounts(ac.DeveloperAccounts(), gc.ChainConfig())

	var txs []*types.Transaction
	// Send one celo from external account 0 to 1 via node 0.
	for i := 0; i < 10; i++ {
		tx, err := accounts[0].SendCelo(ctx, accounts[1].Address, 1, n)
		require.NoError(t, err)
		txs = append(txs, tx)
	}

	// Wait for the whole network to process the transactions.
	err = network.AwaitTransactions(ctx, txs...)
	require.NoError(t, err)

	lastTx := txs[len(txs)-1]

	b := n.Tracker.GetProcessedBlockForTx(lastTx.Hash())

	var wg sync.WaitGroup
	for i := 1; i < +int(b.NumberU64()); i++ {
		wg.Add(1)
		num := i
		go func() {
			defer wg.Done()
			c, err := rpc.DialContext(ctx, n.WSEndpoint())
			require.NoError(t, err)

			var result []interface{}
			err = c.CallContext(ctx, &result, "debug_traceBlockByNumber", hexutil.EncodeUint64(uint64(num)))
			require.NoError(t, err)
		}()

	}
	wg.Wait()
}

type rpcCustomTransaction struct {
	BlockNumber *hexutil.Big `json:"blockNumber"`
	GasPrice    *hexutil.Big `json:"gasPrice"`
}

// TestRPCDynamicTxGasPriceWithBigFeeCap test that after a dynamic tx
// was added to a block, the rpc sends in the gasPrice the actual consumed
// price by the tx, which could be less than the feeCap (as in this example)
func TestRPCDynamicTxGasPriceWithBigFeeCap(t *testing.T) {
	ac := test.AccountConfig(3, 2)
	gc, ec, err := test.BuildConfig(ac)
	require.NoError(t, err)
	network, shutdown, err := test.NewNetwork(ac, gc, ec)
	require.NoError(t, err)
	defer shutdown()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
	defer cancel()

	accounts := test.Accounts(ac.DeveloperAccounts(), gc.ChainConfig())

	suggestedGasPrice, err := network[0].WsClient.SuggestGasPrice(ctx)
	require.NoError(t, err)
	gasFeeCap := big.NewInt(0).Mul(suggestedGasPrice, big.NewInt(90))
	gasTipCap := big.NewInt(0).Mul(suggestedGasPrice, big.NewInt(2))

	// Send one celo from external account 0 to 1 via node 0.
	tx, err := accounts[0].SendCeloWithDynamicFee(ctx, accounts[1].Address, 1, gasFeeCap, gasTipCap, network[0])
	require.NoError(t, err)

	// Wait for the whole network to process the transaction.
	err = network.AwaitTransactions(ctx, tx)
	require.NoError(t, err)

	var json *rpcCustomTransaction
	err = network[0].WsClient.GetRPCClient().CallContext(ctx, &json, "eth_getTransactionByHash", tx.Hash())
	require.NoError(t, err)
	require.NotNil(t, json.BlockNumber)
	gasPrice := json.GasPrice.ToInt()
	require.NotNil(t, json.GasPrice)
	require.Greater(t, gasPrice.Int64(), gasTipCap.Int64())
	require.Less(t, gasPrice.Int64(), gasFeeCap.Int64())
}

// TestRPCDynamicTxGasPriceWithState aims to test the scenario where a
// an old dynamic tx is requested via rpc, to an archive node.
// As right now on Celo, we are not storing the baseFee in the header (as ethereum does),
// to know the exactly gasPrice expent in a dynamic tx, depends on consuming the
// GasPriceMinimum contract
func TestRPCDynamicTxGasPriceWithState(t *testing.T) {
	ac := test.AccountConfig(3, 2)
	gc, ec, err := test.BuildConfig(ac)
	ec.TxLookupLimit = 0
	ec.NoPruning = true
	require.NoError(t, err)
	network, shutdown, err := test.NewNetwork(ac, gc, ec)
	require.NoError(t, err)
	defer shutdown()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
	defer cancel()

	accounts := test.Accounts(ac.DeveloperAccounts(), gc.ChainConfig())

	suggestedGasPrice, err := network[0].WsClient.SuggestGasPrice(ctx)
	require.NoError(t, err)
	gasFeeCap := big.NewInt(0).Mul(suggestedGasPrice, big.NewInt(90))
	gasTipCap := big.NewInt(0).Mul(suggestedGasPrice, big.NewInt(2))

	// Send one celo from external account 0 to 1 via node 0.
	tx, err := accounts[0].SendCeloWithDynamicFee(ctx, accounts[1].Address, 1, gasFeeCap, gasTipCap, network[0])
	require.NoError(t, err)

	// Wait for the whole network to process the transaction.
	err = network.AwaitTransactions(ctx, tx)
	require.NoError(t, err)

	var json *rpcCustomTransaction
	// Check that the transaction can be retrieved via the rpc api
	err = network[0].WsClient.GetRPCClient().CallContext(ctx, &json, "eth_getTransactionByHash", tx.Hash())
	require.NoError(t, err)
	// Blocknumber != nil it means that it eas already processed
	require.NotNil(t, json.BlockNumber)

	// Wait until the state is prunned in the case of a full node.
	// For this we create blocks with at least 1 tx
	for i := 0; i < 200; i++ {
		_, err := accounts[0].SendCeloTracked(ctx, accounts[1].Address, 1, network[0])
		require.NoError(t, err)
	}

	var json2 *rpcCustomTransaction
	// Check that the transaction can still be retrieved via the rpc api
	err = network[0].WsClient.GetRPCClient().CallContext(ctx, &json2, "eth_getTransactionByHash", tx.Hash())
	require.NoError(t, err)
	// if the object is nil, it means that was not found
	require.NotNil(t, json2)
	// Blocknumber != nil it means that it eas already processed
	require.NotNil(t, json2.BlockNumber)
	require.Equal(t, json.GasPrice, json2.GasPrice)
}

// TestRPCDynamicTxGasPriceWithoutState aims to test the scenario where a
// an old dynamic tx is requested via rpc, to a full node that does not have
// the state anymore.
// As right now on Celo, we are not storing the baseFee in the header (as ethereum does),
// to know the exactly gasPrice expent in a dynamic tx, depends on consuming the
// GasPriceMinimum contract
func TestRPCDynamicTxGasPriceWithoutState(t *testing.T) {
	ac := test.AccountConfig(3, 2)
	gc, ec, err := test.BuildConfig(ac)
	ec.TrieDirtyCache = 5
	require.NoError(t, err)
	network, shutdown, err := test.NewNetwork(ac, gc, ec)
	require.NoError(t, err)
	defer shutdown()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
	defer cancel()

	accounts := test.Accounts(ac.DeveloperAccounts(), gc.ChainConfig())

	suggestedGasPrice, err := network[0].WsClient.SuggestGasPrice(ctx)
	require.NoError(t, err)
	gasFeeCap := big.NewInt(0).Mul(suggestedGasPrice, big.NewInt(90))
	gasTipCap := big.NewInt(0).Mul(suggestedGasPrice, big.NewInt(2))

	// Send one celo from external account 0 to 1 via node 0.
	tx, err := accounts[0].SendCeloWithDynamicFee(ctx, accounts[1].Address, 1, gasFeeCap, gasTipCap, network[0])
	require.NoError(t, err)

	// Wait for the whole network to process the transaction.
	err = network.AwaitTransactions(ctx, tx)
	require.NoError(t, err)

	var json *rpcCustomTransaction
	// Check that the transaction can be retrieved via the rpc api
	err = network[0].WsClient.GetRPCClient().CallContext(ctx, &json, "eth_getTransactionByHash", tx.Hash())
	require.NoError(t, err)
	// Blocknumber != nil it means that it eas already processed
	require.NotNil(t, json.BlockNumber)

	// Wait until the state is prunned. For this we create blocks with at least 1 tx
	for i := 0; i < 200; i++ {
		_, err := accounts[0].SendCeloTracked(ctx, accounts[1].Address, 1, network[0])
		require.NoError(t, err)
	}

	var json2 *rpcCustomTransaction
	// Check that the transaction can still be retrieved via the rpc api
	err = network[0].WsClient.GetRPCClient().CallContext(ctx, &json2, "eth_getTransactionByHash", tx.Hash())
	require.NoError(t, err)
	// if the object is nil, it means that was not found
	require.NotNil(t, json2)
	// Blocknumber != nil it means that it eas already processed
	require.NotNil(t, json2.BlockNumber)

	require.Nil(t, json2.GasPrice)
}
