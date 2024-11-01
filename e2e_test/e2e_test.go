package e2e_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/common/hexutil"
	"github.com/celo-org/celo-blockchain/core/types"
	"github.com/celo-org/celo-blockchain/eth/downloader"
	"github.com/celo-org/celo-blockchain/eth/tracers"
	"github.com/celo-org/celo-blockchain/log"
	"github.com/celo-org/celo-blockchain/mycelo/env"
	"github.com/celo-org/celo-blockchain/mycelo/genesis"
	"github.com/celo-org/celo-blockchain/node"
	"github.com/celo-org/celo-blockchain/rpc"
	"github.com/celo-org/celo-blockchain/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	// Force-load native and js packages, to trigger registration
	_ "github.com/celo-org/celo-blockchain/eth/tracers/js"
	_ "github.com/celo-org/celo-blockchain/eth/tracers/native"
)

func init() {
	// This statement is commented out but left here since its very useful for
	// debugging problems and its non trivial to construct.
	//
	// log.Root().SetHandler(log.LvlFilterHandler(log.LvlInfo, log.StreamHandler(os.Stdout, log.TerminalFormat(true))))

	// This disables all logging which in general we want, because there is a lot
	log.Root().SetHandler(log.DiscardHandler())
}

// This test starts a network submits a transaction and waits for the whole
// network to process the transaction.
func TestSendCelo(t *testing.T) {
	ac := test.AccountConfig(3, 2)
	gingerbreadBlock := common.Big0
	gc, ec, err := test.BuildConfig(ac, gingerbreadBlock, nil)
	require.NoError(t, err)
	network, shutdown, err := test.NewNetwork(ac, gc, ec)
	require.NoError(t, err)
	defer shutdown()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	accounts := test.Accounts(ac.DeveloperAccounts(), gc.ChainConfig())

	// Send one celo from external account 0 to 1 via node 0.
	tx, err := accounts[0].SendCelo(ctx, accounts[1].Address, 1, network[0])
	require.NoError(t, err)

	// Wait for the whole network to process the transaction.
	err = network.AwaitTransactions(ctx, tx)
	require.NoError(t, err)
}

// This test starts a network, submits a GoldToken.transfer tx, waits for the whole
// network to process the transaction, and traces that tx.
// This tests that CELO transfers made via the transfer precompile work e2e
// and can be useful for debugging these traces.
func TestTraceSendCeloViaGoldToken(t *testing.T) {
	ac := test.AccountConfig(3, 2)
	gingerbreadBlock := common.Big0
	gc, ec, err := test.BuildConfig(ac, gingerbreadBlock, nil)
	require.NoError(t, err)
	network, shutdown, err := test.NewNetwork(ac, gc, ec)
	require.NoError(t, err)
	defer shutdown()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	accounts := test.Accounts(ac.DeveloperAccounts(), gc.ChainConfig())
	// Send 1 wei of CELO from accounts[0] to accounts[1] by calling GoldToken.transfer
	tx, err := accounts[0].SendCeloViaGoldToken(ctx, accounts[1].Address, 1, network[0])
	require.NoError(t, err)

	// Wait for the whole network to process the transaction.
	err = network.AwaitTransactions(ctx, tx)
	require.NoError(t, err)
	c, err := rpc.DialContext(ctx, network[0].WSEndpoint())
	require.NoError(t, err)

	var result map[string]interface{}
	tracerStr := "callTracer"
	err = c.CallContext(ctx, &result, "debug_traceTransaction", tx.Hash().String(), tracers.TraceConfig{Tracer: &tracerStr})

	require.NoError(t, err)
	// Check top level gas values
	require.Equal(t, "0x45d6", result["gasUsed"])
	require.Equal(t, "0x4653", result["gas"])
	// TODO add more specific trace-checking as part of
	// this issue: https://github.com/celo-org/celo-blockchain/issues/2078
}

// Moved from API tests because registering the callTracer (necessary after the
// go native tracer refactor) causes a circular import.
// Use the callTracer to trace a native CELO transfer.
func TestCallTraceTransactionNativeTransfer(t *testing.T) {
	ac := test.AccountConfig(1, 2)
	gingerbreadBlock := common.Big0
	gc, ec, err := test.BuildConfig(ac, gingerbreadBlock, nil)
	require.NoError(t, err)
	network, shutdown, err := test.NewNetwork(ac, gc, ec)
	require.NoError(t, err)
	defer shutdown()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	accounts := test.Accounts(ac.DeveloperAccounts(), gc.ChainConfig())

	// Send one celo from external account 0 to 1 via node 0.
	tx, err := accounts[0].SendCelo(ctx, accounts[1].Address, 1, network[0])
	require.NoError(t, err)

	// Wait for the whole network to process the transaction.
	err = network.AwaitTransactions(ctx, tx)
	require.NoError(t, err)
	c, err := rpc.DialContext(ctx, network[0].WSEndpoint())
	require.NoError(t, err)

	var result map[string]interface{}
	tracerStr := "callTracer"
	err = c.CallContext(ctx, &result, "debug_traceTransaction", tx.Hash().String(), tracers.TraceConfig{Tracer: &tracerStr})
	require.NoError(t, err)
	res_json, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("failed to marshal result: %v", err)
	}
	expectedTraceStr := fmt.Sprintf(`{"from":"0x%x","gas":"0x0","gasUsed":"0x0","input":"0x","output":"0x","to":"0x%x","type":"CALL","value":"0x1"}`, accounts[0].Address, accounts[1].Address)
	require.JSONEq(t, expectedTraceStr, string(res_json))
}

// Moved from API tests because registering the prestateTracer (necessary after the
// go native tracer refactor) causes a circular import.
// Use the prestateTracer to trace a native CELO transfer.
func TestPrestateTransactionNativeTransfer(t *testing.T) {
	ac := test.AccountConfig(1, 2)
	gingerbreadBlock := common.Big0
	gc, ec, err := test.BuildConfig(ac, gingerbreadBlock, nil)
	require.NoError(t, err)
	network, shutdown, err := test.NewNetwork(ac, gc, ec)
	require.NoError(t, err)
	defer shutdown()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	accounts := test.Accounts(ac.DeveloperAccounts(), gc.ChainConfig())

	// Send one celo from external account 0 to 1 via node 0.
	tx, err := accounts[0].SendCelo(ctx, accounts[1].Address, 1, network[0])
	require.NoError(t, err)

	// Wait for the whole network to process the transaction.
	err = network.AwaitTransactions(ctx, tx)
	require.NoError(t, err)
	c, err := rpc.DialContext(ctx, network[0].WSEndpoint())
	require.NoError(t, err)

	var result map[string]interface{}
	tracerStr := "prestateTracer"
	err = c.CallContext(ctx, &result, "debug_traceTransaction", tx.Hash().String(), tracers.TraceConfig{Tracer: &tracerStr})
	require.NoError(t, err)

	toAddrLowercase := strings.ToLower(accounts[1].Address.String())
	if _, has := result[toAddrLowercase]; !has {
		t.Fatalf("Expected %s in result", toAddrLowercase)
	}
}

// This test verifies correct behavior in a network of size one, in the case that
// this fails we know that the problem does not lie with our network code.
func TestSingleNodeNetworkManyTxs(t *testing.T) {
	iterations := 5
	txsPerIteration := 5
	ac := test.AccountConfig(1, 1)
	gingerbreadBlock := common.Big0
	gc, ec, err := test.BuildConfig(ac, gingerbreadBlock, nil)
	require.NoError(t, err)
	gc.Istanbul.Epoch = uint64(iterations) * 50 // avoid the epoch for this test
	network, shutdown, err := test.NewNetwork(ac, gc, ec)
	require.NoError(t, err)
	defer shutdown()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
	defer cancel()
	accounts := test.Accounts(ac.DeveloperAccounts(), gc.ChainConfig())
	for r := 0; r < iterations; r++ {
		txs := make([]*types.Transaction, 0, txsPerIteration)
		for j := 0; j < txsPerIteration; j++ {
			tx, err := accounts[0].SendCelo(ctx, common.Address{}, 1, network[0])
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
	gingerbreadBlock := common.Big0
	gc, ec, err := test.BuildConfig(accounts, gingerbreadBlock, nil)
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
	gingerbreadBlock := common.Big0
	gc, ec, err := test.BuildConfig(ac, gingerbreadBlock, nil)
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

	// We need to wait here to allow the call to "Backend.RefreshValPeers" to
	// complete before adding peers. This is because "Backend.RefreshValPeers"
	// deletes all peers and then re-adds any peers from the cached
	// connections, but in the case that peers were recently added there may
	// not have been enough time to connect to them and populate the connection
	// cache, and in that case "Backend.RefreshValPeers" simply removes all the
	// peers.
	time.Sleep(250 * time.Millisecond)
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

	// We need to wait here to allow the call to "Backend.RefreshValPeers" to
	// complete before adding peers. This is because "Backend.RefreshValPeers"
	// deletes all peers and then re-adds any peers from the cached
	// connections, but in the case that peers were recently added there may
	// not have been enough time to connect to them and populate the connection
	// cache, and in that case "Backend.RefreshValPeers" simply removes all the
	// peers.
	time.Sleep(250 * time.Millisecond)
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

func runStopNetworkAtL2BlockTest(ctx context.Context, t *testing.T, network test.Network, l2Block *big.Int) {
	err := network.AwaitBlock(ctx, l2Block.Uint64()-1)
	require.NoError(t, err)

	shortCtx, cancel := context.WithTimeout(ctx, time.Second*2)
	defer cancel()

	// fail if any node adds a new block >= the migration block
	var wg sync.WaitGroup
	errorChan := make(chan error, len(network))

	for _, n := range network {
		wg.Add(1)
		go func(n *test.Node) {
			defer wg.Done()
			err := n.Tracker.AwaitBlock(shortCtx, l2Block.Uint64())
			errorChan <- err
		}(n)
	}

	wg.Wait()
	close(errorChan)

	// Collect and check errors
	for err := range errorChan {
		require.EqualError(t, err, context.DeadlineExceeded.Error())
	}
}

func TestStopNetworkAtL2BlockSimple(t *testing.T) {
	numValidators := 3
	numFullNodes := 2
	numFastNodes := 1
	ac := test.AccountConfig(numValidators, 2)
	gingerbreadBlock := common.Big0
	l2BlockOG := big.NewInt(3)
	gc, ec, err := test.BuildConfig(ac, gingerbreadBlock, l2BlockOG)
	require.NoError(t, err)
	network, _, err := test.NewNetwork(ac, gc, ec)
	require.NoError(t, err)
	network, _, err = test.AddNonValidatorNodes(network, ec, uint64(numFullNodes), downloader.FullSync)
	require.NoError(t, err)
	network, shutdown, err := test.AddNonValidatorNodes(network, ec, uint64(numFastNodes), downloader.FastSync)
	require.NoError(t, err)

	defer shutdown()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*40)
	defer cancel()

	runStopNetworkAtL2BlockTest(ctx, t, network, l2BlockOG)
}

func TestStopNetworkAtL2Block(t *testing.T) {
	numValidators := 3
	numFullNodes := 2
	numFastNodes := 1
	ac := test.AccountConfig(numValidators, 2)
	gingerbreadBlock := common.Big0
	l2BlockOG := big.NewInt(3)
	gc, ec, err := test.BuildConfig(ac, gingerbreadBlock, l2BlockOG)
	require.NoError(t, err)
	network, _, err := test.NewNetwork(ac, gc, ec)
	require.NoError(t, err)
	network, _, err = test.AddNonValidatorNodes(network, ec, uint64(numFullNodes), downloader.FullSync)
	require.NoError(t, err)
	network, shutdown, err := test.AddNonValidatorNodes(network, ec, uint64(numFastNodes), downloader.FastSync)
	require.NoError(t, err)

	defer shutdown()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*400)
	defer cancel()

	runStopNetworkAtL2BlockTest(ctx, t, network, l2BlockOG)

	shutdown()

	// Restart nodes with --l2-migration-block set to the next block
	err = network.RestartNetworkWithMigrationBlockOffsets(l2BlockOG, []int64{1, 1, 1, 1, 1, 1})
	require.NoError(t, err)

	l2BlockPlusOne := new(big.Int).Add(l2BlockOG, big.NewInt(1))

	runStopNetworkAtL2BlockTest(ctx, t, network, l2BlockPlusOne)

	shutdown()

	// Restart nodes with --l2-migration-block set to the same block
	err = network.RestartNetworkWithMigrationBlockOffsets(l2BlockOG, []int64{1, 1, 1, 1, 1, 1})
	require.NoError(t, err)

	runStopNetworkAtL2BlockTest(ctx, t, network, l2BlockPlusOne)

	shutdown()

	// Restart nodes with different --l2-migration-block offsets
	// If 2/3 validators (validators are the first 3 nodes in the network array)
	// have the same migration block, the network should not be able to add any more blocks
	err = network.RestartNetworkWithMigrationBlockOffsets(l2BlockOG, []int64{1, 1, 2, 2, 2, 2})
	require.NoError(t, err)

	runStopNetworkAtL2BlockTest(ctx, t, network, l2BlockPlusOne)

	shutdown()

	// Restart nodes with different --l2-migration-block offsets
	// If 2/3 validators (validators are the first 3 nodes in the network array)
	// have a greater migration block, the rest of the network should be able to add more blocks
	err = network.RestartNetworkWithMigrationBlockOffsets(l2BlockOG, []int64{1, 2, 2, 2, 2, 2})
	require.NoError(t, err)

	l2BlockPlusTwo := new(big.Int).Add(l2BlockOG, big.NewInt(2))

	runStopNetworkAtL2BlockTest(ctx, t, network[:1], l2BlockPlusOne)
	runStopNetworkAtL2BlockTest(ctx, t, network[1:], l2BlockPlusTwo)

	shutdown()

	// Restart nodes with --l2-migration-block set to a prev block
	err = network.RestartNetworkWithMigrationBlockOffsets(l2BlockOG, []int64{-1, -1, -1, -1, -1, -1})
	require.NoError(t, err)

	// The network should be unchanged
	runStopNetworkAtL2BlockTest(ctx, t, network[:1], l2BlockPlusOne)
	runStopNetworkAtL2BlockTest(ctx, t, network[1:], l2BlockPlusTwo)
}

// This test was created to reproduce the concurrent map access error in
// https://github.com/celo-org/celo-blockchain/issues/1799
//
// It does this by calling debug_traceBlockByNumber a number of times since the
// trace block code was the source of the concurrent map access.
func TestBlockTracingConcurrentMapAccess(t *testing.T) {
	ac := test.AccountConfig(1, 2)
	gingerbreadBlock := common.Big0
	gc, ec, err := test.BuildConfig(ac, gingerbreadBlock, nil)
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

// Sends and traces a single native transfer.
// Helpful for debugging issues in TestBlockTracingConcurrentMapAccess.
func TestBlockTracingSequentialAccess(t *testing.T) {
	ac := test.AccountConfig(1, 2)
	gingerbreadBlock := common.Big0
	gc, ec, err := test.BuildConfig(ac, gingerbreadBlock, nil)
	require.NoError(t, err)
	network, shutdown, err := test.NewNetwork(ac, gc, ec)
	require.NoError(t, err)
	defer shutdown()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*300)
	defer cancel()
	n := network[0]
	accounts := test.Accounts(ac.DeveloperAccounts(), gc.ChainConfig())
	// Send one celo from external account 0 to 1 via node 0.
	tx, err := accounts[0].SendCelo(ctx, accounts[1].Address, 1, n)
	require.NoError(t, err)
	// Wait for the whole network to process the transaction.
	err = network.AwaitTransactions(ctx, tx)
	require.NoError(t, err)
	b := n.Tracker.GetProcessedBlockForTx(tx.Hash())
	c, err := rpc.DialContext(ctx, n.WSEndpoint())
	require.NoError(t, err)
	var result []interface{}
	err = c.CallContext(ctx, &result, "debug_traceBlockByNumber", hexutil.EncodeUint64(uint64(int(b.NumberU64()))))
	require.NoError(t, err)
}

type rpcCustomTransaction struct {
	BlockNumber *hexutil.Big `json:"blockNumber"`
	BlockHash   *common.Hash `json:"blockHash"`
	GasPrice    *hexutil.Big `json:"gasPrice"`
}

// TestRPCDynamicTxGasPriceWithBigFeeCap test that after a dynamic tx
// was added to a block, the rpc sends in the gasPrice the actual consumed
// price by the tx, which could be less than the feeCap (as in this example)
func TestRPCDynamicTxGasPriceWithBigFeeCap(t *testing.T) {
	ac := test.AccountConfig(3, 2)
	gingerbreadBlock := common.Big0
	gc, ec, err := test.BuildConfig(ac, gingerbreadBlock, nil)
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
	gingerbreadBlock := common.Big0
	gc, ec, err := test.BuildConfig(ac, gingerbreadBlock, nil)
	require.NoError(t, err)
	ec.TxLookupLimit = 0
	ec.NoPruning = true
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

	// Create a block containing a transaction, we will prune the state of this block.
	_, err = accounts[0].SendCeloTracked(ctx, accounts[1].Address, 1, network[0])
	require.NoError(t, err)

	// Prune state
	// As the gasPrice of the block N, is the one from the state of the block N-1, we need to prune the parent block
	err = pruneStateOfBlock(ctx, network[0], new(big.Int).Sub(json.BlockNumber.ToInt(), common.Big1))
	require.NoError(t, err)

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
func TestRPCDynamicTxGasPriceWithoutStateBeforeGingerbread(t *testing.T) {
	testRPCDynamicTxGasPriceWithoutState(t, false, false)
}

func TestRPCDynamicTxGasPriceWithoutStateAfterGingerbread(t *testing.T) {
	testRPCDynamicTxGasPriceWithoutState(t, true, false)
}

func TestRPCDynamicTxGasPriceWithoutStateForAlternativeCurrencyBeforeGingerbread(t *testing.T) {
	testRPCDynamicTxGasPriceWithoutState(t, false, true)
}

func TestRPCDynamicTxGasPriceWithoutStateForAlternativeCurrencyAfterGingerbread(t *testing.T) {
	testRPCDynamicTxGasPriceWithoutState(t, true, true)
}

func testRPCDynamicTxGasPriceWithoutState(t *testing.T, afterGingerbread, alternativeCurrency bool) {
	var gingerbreadBlock *big.Int
	if afterGingerbread {
		gingerbreadBlock = common.Big0
	}
	cusdAddress := common.HexToAddress("0xd008")
	ac := test.AccountConfig(3, 2)
	gc, ec, err := test.BuildConfig(ac, gingerbreadBlock, nil)
	ec.TrieDirtyCache = 5
	require.NoError(t, err)
	network, shutdown, err := test.NewNetwork(ac, gc, ec)
	require.NoError(t, err)
	defer shutdown()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*90)
	defer cancel()

	accounts := test.Accounts(ac.DeveloperAccounts(), gc.ChainConfig())

	var feeCurrency *common.Address
	if alternativeCurrency {
		feeCurrency = &cusdAddress
	}
	suggestedGasPrice, err := network[0].WsClient.SuggestGasPriceInCurrency(ctx, feeCurrency)
	require.NoError(t, err)
	gasFeeCap := big.NewInt(0).Mul(suggestedGasPrice, big.NewInt(90))
	gasTipCap := big.NewInt(0).Mul(suggestedGasPrice, big.NewInt(2))

	// Send one celo from external account 0 to 1 via node 0.
	tx, err := accounts[0].SendValueWithDynamicFee(ctx, accounts[1].Address, 1, feeCurrency, gasFeeCap, gasTipCap, network[0], 0)
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

	// Create one block to be able to prune the last state
	_, err = accounts[0].SendCeloTracked(ctx, accounts[1].Address, 1, network[0])
	require.NoError(t, err)
	// Prune state
	// As the gasPrice of the block N, is the one from the state of the block N-1, we need to prune the parent block
	err = pruneStateOfBlock(ctx, network[0], new(big.Int).Sub(json.BlockNumber.ToInt(), common.Big1))
	require.NoError(t, err)

	var json2 *rpcCustomTransaction
	// Check that the transaction can still be retrieved via the rpc api
	err = network[0].WsClient.GetRPCClient().CallContext(ctx, &json2, "eth_getTransactionByHash", tx.Hash())
	require.NoError(t, err)
	// if the object is nil, it means that was not found
	require.NotNil(t, json2)
	// Blocknumber != nil it means that it eas already processed
	require.NotNil(t, json2.BlockNumber)

	if afterGingerbread && !alternativeCurrency {
		require.Equal(t, json.GasPrice, json2.GasPrice)
	} else {
		require.Nil(t, json2.GasPrice)
	}
}

func pruneStateOfBlock(ctx context.Context, node *test.Node, blockNumber *big.Int) error {
	var block *types.Block
	block, err := node.WsClient.BlockByNumber(ctx, blockNumber)
	if err != nil {
		return err
	}
	root := block.Root()
	node.Eth.BlockChain().StateCache().TrieDB().Dereference(root)

	return nil
}

func runMochaTest(t *testing.T, add_args func(*env.AccountsConfig, *genesis.Config, test.Network) []string) {
	ac := test.AccountConfig(1, 1)
	gingerbreadBlock := common.Big0
	gc, ec, err := test.BuildConfig(ac, gingerbreadBlock, nil)
	require.NoError(t, err)
	network, shutdown, err := test.NewNetwork(ac, gc, ec)
	require.NoError(t, err)
	defer shutdown()

	// Execute typescript tests to check ethers.js compatibility.
	//
	// The '--networkaddr' and '--blocknum' flags are npm config variables, the
	// values become available under 'process.env.npm_config_networkaddr' and
	// 'process.env.npm_config_blocknum' in typescript test. Everything after
	// '--' are flags that are passed to mocha and these flags are controlling
	// which tests to run.

	// The tests don't seem to work on CI with IPV6 addresses so we convert to IPV4 here
	addr := strings.Replace(network[0].Node.HTTPEndpoint(), "[::]", "127.0.0.1", 1)

	common_args := []string{"run", "--networkaddr=" + addr}
	custom_args := add_args(ac, gc, network)

	cmd := exec.Command("npm", append(common_args, custom_args...)...)

	cmd.Dir = "./ethersjs-api-check/"
	println("executing mocha test with", cmd.String())
	output, err := cmd.CombinedOutput()
	println(string(output))
	require.NoError(t, err)
}

func TestEthersJSCompatibility(t *testing.T) {
	add_args := func(ac *env.AccountsConfig, gc *genesis.Config, network test.Network) []string {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*20)
		defer cancel()

		num, err := network[0].WsClient.BlockNumber(ctx)
		require.NoError(t, err)
		return []string{"test", "--blocknum=" + hexutil.Uint64(num).String(), "--", "--grep", "ethers.js compatibility tests with state"}
	}
	runMochaTest(t, add_args)
}

// This test checks the functionality of the configuration to enable/disable
// returning the 'gasLimit' and 'baseFeePerGas' fields on RPC blocks.
func TestEthersJSCompatibilityDisableAfterGingerbread(t *testing.T) {
	ac := test.AccountConfig(1, 1)
	gingerbreadBlock := common.Big0
	gc, ec, err := test.BuildConfig(ac, gingerbreadBlock, nil)
	require.NoError(t, err)

	// Check fields present (compatibility set by default)
	network, shutdown, err := test.NewNetwork(ac, gc, ec)
	require.NoError(t, err)
	defer shutdown()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*20)
	defer cancel()

	result := make(map[string]interface{})
	err = network[0].WsClient.GetRPCClient().CallContext(ctx, &result, "eth_getBlockByNumber", "latest", true)
	require.NoError(t, err)

	for _, field := range []string{"gasLimit", "baseFeePerGas", "sha3Uncles", "uncles", "nonce", "mixHash", "difficulty"} {
		_, ok := result[field]
		assert.Truef(t, ok, "%s field should be present on RPC block after Gingerbread", field)
	}
	require.Equal(t, result["sha3Uncles"], "0x1dcc4de8dec75d7aab85b567b6ccd41ad312451b948a7413f0a142fd40d49347")

	// Turn off compatibility and check fields are still present
	ec.RPCEthCompatibility = false
	network, shutdown, err = test.NewNetwork(ac, gc, ec)
	require.NoError(t, err)
	defer shutdown()

	ctx, cancel = context.WithTimeout(context.Background(), time.Second*20)
	defer cancel()

	result = make(map[string]interface{})
	err = network[0].WsClient.GetRPCClient().CallContext(ctx, &result, "eth_getBlockByNumber", "0x0", true)
	require.NoError(t, err)

	// After Gingerbread, gasLimit should be returned directly from the header, even if
	// RPCEthCompatibility is off, since it is now part of the header hash.
	_, ok := result["gasLimit"]
	assert.True(t, ok, "gasLimit field must be present on RPC block after Gingerbread")
	_, ok = result["baseFeePerGas"]
	assert.True(t, ok, "baseFeePerGas field must be present on RPC block after Gingerbread")
}

// This test checks the functionality of the configuration to enable/disable
// returning the 'gasLimit' and 'baseFeePerGas' fields on RPC blocks before the Gingerbread happened.
// Gingerbread is relevant because it added the gasLimit to the header.
func TestEthersJSCompatibilityDisableBeforeGingerbread(t *testing.T) {
	ac := test.AccountConfig(1, 1)
	var gingerbreadBlock *big.Int = nil
	gc, ec, err := test.BuildConfig(ac, gingerbreadBlock, nil)
	require.NoError(t, err)

	// Check fields present (compatibility set by default)
	network, shutdown, err := test.NewNetwork(ac, gc, ec)
	require.NoError(t, err)
	defer shutdown()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*20)
	defer cancel()

	result := make(map[string]interface{})
	err = network[0].WsClient.GetRPCClient().CallContext(ctx, &result, "eth_getBlockByNumber", "latest", true)
	require.NoError(t, err)

	for _, field := range []string{"gasLimit", "baseFeePerGas", "difficulty"} {
		_, ok := result[field]
		assert.Truef(t, ok, "%s field should be present on RPC block before Gingerbread", field)
	}
	for _, field := range []string{"sha3Uncles", "uncles", "nonce", "mixHash"} {
		_, ok := result[field]
		assert.Falsef(t, ok, "%s field should not be present on RPC block before Gingerbread", field)
	}

	// Turn off compatibility and check fields are not present
	ec.RPCEthCompatibility = false
	network, shutdown, err = test.NewNetwork(ac, gc, ec)
	require.NoError(t, err)
	defer shutdown()

	ctx, cancel = context.WithTimeout(context.Background(), time.Second*20)
	defer cancel()

	result = make(map[string]interface{})
	err = network[0].WsClient.GetRPCClient().CallContext(ctx, &result, "eth_getBlockByNumber", "latest", true)
	require.NoError(t, err)

	for _, field := range []string{"gasLimit", "baseFeePerGas", "sha3Uncles", "uncles", "nonce", "mixHash", "difficulty"} {
		_, ok := result[field]
		assert.Falsef(t, ok, "%s field should not be present on RPC block before Gingerbread", field)
	}
}

// Initially we could not retrieve the eth compatibility fields (gasLimit &
// baseFee) on the genesis block because our logic dictated that the effective
// base fee and gas limit for a block were the ones set in the previous block
// (because those values are required by the vm before processing a block).
// There was no special case to handle the genesis block. We now have a special
// case to handle the genesis block which returns the values retrieved from the
// state at the genesis block.
func TestEthCompatibilityFieldsOnGenesisBlock(t *testing.T) {
	ac := test.AccountConfig(1, 1)
	// Gingerbread needs to be disabled for this test to be meaningful (since
	// gingerbread added gasLimt & baseFee fields to the block)
	var gingerbreadBlock *big.Int = nil

	// Fist we test without eth compatibility to ensure that the setting has an effect.
	gc, ec, err := test.BuildConfig(ac, gingerbreadBlock, nil)
	ec.RPCEthCompatibility = false
	require.NoError(t, err)
	network, shutdown, err := test.NewNetwork(ac, gc, ec)
	require.NoError(t, err)
	defer shutdown()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*20)
	defer cancel()

	block, err := network[0].WsClient.BlockByNumber(ctx, big.NewInt(0))
	require.NoError(t, err)

	var nilBigInt *big.Int
	require.Equal(t, nilBigInt, block.BaseFee())
	require.Equal(t, uint64(0), block.GasLimit())

	// Now we with eth compatility enabled and see that gasLimit and baseFee
	// are returned on the block.
	gc, ec, err = test.BuildConfig(ac, gingerbreadBlock, nil)
	ec.RPCEthCompatibility = true
	require.NoError(t, err)
	network, shutdown, err = test.NewNetwork(ac, gc, ec)
	require.NoError(t, err)
	defer shutdown()

	block, err = network[0].WsClient.BlockByNumber(ctx, big.NewInt(0))
	require.NoError(t, err)

	require.NotEqual(t, nilBigInt, block.BaseFee())
	require.Greater(t, block.BaseFee().Uint64(), uint64(0))
	require.Greater(t, block.GasLimit(), uint64(0))
}

// Initially we were not able to set the gingerbread activation block to be the genesis block, this test checks that it's possible.
func TestSettingGingerbreadOnGenesisBlock(t *testing.T) {
	ac := test.AccountConfig(1, 1)

	// Fist we test without gingerbread to ensure that setting the gingerbread
	// actually has an effect.
	var gingerbreadBlock *big.Int = nil
	gc, ec, err := test.BuildConfig(ac, gingerbreadBlock, nil)
	ec.RPCEthCompatibility = false
	require.NoError(t, err)
	network, shutdown, err := test.NewNetwork(ac, gc, ec)
	require.NoError(t, err)
	defer shutdown()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*20)
	defer cancel()

	block, err := network[0].WsClient.BlockByNumber(ctx, big.NewInt(0))
	require.NoError(t, err)

	var nilBigInt *big.Int
	require.Equal(t, nilBigInt, block.BaseFee())
	require.Equal(t, uint64(0), block.GasLimit())

	// Now we check that setting the gingerbread block at genesis causes gasLimit and baseFee to be set on the block.
	gingerbreadBlock = big.NewInt(0)
	gc, ec, err = test.BuildConfig(ac, gingerbreadBlock, nil)
	ec.RPCEthCompatibility = false
	require.NoError(t, err)
	network, shutdown, err = test.NewNetwork(ac, gc, ec)
	require.NoError(t, err)
	defer shutdown()

	block, err = network[0].WsClient.BlockByNumber(ctx, big.NewInt(0))
	require.NoError(t, err)

	require.NotEqual(t, nilBigInt, block.BaseFee())
	require.Greater(t, block.BaseFee().Uint64(), uint64(0))
	require.Greater(t, block.GasLimit(), uint64(0))
}

// This test checks that retreiveing the "finalized" block results in the same response as retrieving the "latest" block.
func TestGetFinalizedBlock(t *testing.T) {
	ac := test.AccountConfig(2, 2)
	gingerbreadBlock := common.Big0
	gc, ec, err := test.BuildConfig(ac, gingerbreadBlock, nil)
	require.NoError(t, err)
	network, shutdown, err := test.NewNetwork(ac, gc, ec)
	require.NoError(t, err)
	defer shutdown()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	// Wait for at least one block to be built.
	err = network.AwaitBlock(ctx, 1)
	require.NoError(t, err)

	// Stop one of the two validators, so no more blocks can be created.
	err = network[1].Close()
	require.NoError(t, err)

	c := network[0].WsClient.GetRPCClient()
	h := types.Header{}
	err = c.CallContext(ctx, &h, "eth_getHeaderByNumber", "latest")
	require.NoError(t, err)
	require.GreaterOrEqual(t, h.Number.Uint64(), uint64(1))

	h2 := types.Header{}
	err = c.CallContext(ctx, &h2, "eth_getHeaderByNumber", "finalized")
	require.NoError(t, err)

	// Check latest and finalzed block are the same
	require.Equal(t, h.Hash(), h2.Hash())
}

// TestManyFeeCurrencyTransactions is intended to test that we don't have race conditions in the tx pool when handling
// fee currency transactions. It does this by submitting many fee currency transactions from 3 different goroutines over
// a period of roughly 5 seconds which with the configured block time of 1 second means that the transactions should
// span multiple block boundaries with the goal of ensuring that both runReorg and AddRemotes are being called
// concurrently in the txPool. This issue https://github.com/celo-org/celo-blockchain/issues/2318 is occurring somewhat
// randomly and could be the result of some race condition in tx pool handling for fee currency transactions. However
// this test seems to run fairly reliably with the race flag enabled, which seems to indicate that the problem is not a
// result of racy behavior in the tx pool.
func TestManyFeeCurrencyTransactions(t *testing.T) {
	ac := test.AccountConfig(3, 3)
	gingerbreadBlock := common.Big0
	gc, ec, err := test.BuildConfig(ac, gingerbreadBlock, nil)
	require.NoError(t, err)
	ec.Istanbul.BlockPeriod = 1
	network, shutdown, err := test.NewNetwork(ac, gc, ec)
	require.NoError(t, err)
	defer shutdown()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*200)
	defer cancel()

	cUSD := common.HexToAddress("0x000000000000000000000000000000000000d008")
	cEUR := common.HexToAddress("0x000000000000000000000000000000000000D024")
	cREAL := common.HexToAddress("0x000000000000000000000000000000000000d026")

	accounts := test.Accounts(ac.DeveloperAccounts(), gc.ChainConfig())

	time.Sleep(2 * time.Second)
	txsChan := make(chan []*types.Transaction, 3)
	for nodeIndex := 0; nodeIndex < len(network); nodeIndex++ {
		go func(nodeIndex int) {
			txs := make([]*types.Transaction, 0, 3000)
			for i := 0; i < 100; i++ {
				for _, feeCurrency := range []*common.Address{&cUSD, &cEUR, &cREAL} {
					baseFee, err := network[nodeIndex].WsClient.SuggestGasPriceInCurrency(ctx, feeCurrency)
					require.NoError(t, err)
					tip, err := network[nodeIndex].WsClient.SuggestGasTipCapInCurrency(ctx, feeCurrency)
					require.NoError(t, err)

					// Send one celo from external account 0 to 1 via node 0.
					tx, err := accounts[nodeIndex].SendValueWithDynamicFee(ctx, accounts[nodeIndex].Address, 1, feeCurrency, baseFee.Add(baseFee, tip), tip, network[nodeIndex], 71000)
					require.NoError(t, err)
					txs = append(txs, tx)
					time.Sleep(10 * time.Millisecond)
				}
			}
			txsChan <- txs
		}(nodeIndex)
	}

	allTxs := make([]*types.Transaction, 0, 3000*len(network))
	count := 0
	for txs := range txsChan {
		allTxs = append(allTxs, txs...)
		count++
		if count == len(network) {
			break
		}
	}

	err = network.AwaitTransactions(ctx, allTxs...)
	require.NoError(t, err)
}
