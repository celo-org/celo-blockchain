package e2e_test

import (
	"context"
	"fmt"
	"math"
	"os"
	"testing"
	"time"

	"github.com/celo-org/celo-blockchain/log"
	"github.com/celo-org/celo-blockchain/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() {
	// This statement is commented out but left here since its very useful for
	// debugging problems and its non trivial to construct.
	//
	log.Root().SetHandler(log.LvlFilterHandler(log.LvlInfo, log.StreamHandler(os.Stdout, log.TerminalFormat(true))))
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

// This test is intended to ensure that epoch blocks can be correctly marshalled.
// We previously had an open bug for this https://github.com/celo-org/celo-blockchain/issues/1574
func TestEpochBlockMarshaling(t *testing.T) {
	accounts := test.Accounts(1)
	gc, ec, err := test.BuildConfig(accounts)
	require.NoError(t, err)

	// Configure the shortest possible epoch, uptimeLookbackWindow minimum is 3
	// and it needs to be < (epoch -2).
	ec.Istanbul.Epoch = 6
	ec.Istanbul.DefaultLookbackWindow = 3
	network, err := test.NewNetwork(accounts, gc, ec)
	require.NoError(t, err)
	defer network.Shutdown()
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

// This test starts a network submits a transaction and waits for the whole
// network to process the transaction.
func TestRunNode(t *testing.T) {
	accounts := test.Accounts(1)
	gc, ec, err := test.BuildConfig(accounts)
	ec.Istanbul.BlockPeriod = 5
	require.NoError(t, err)
	network, err := test.NewNetwork(accounts, gc, ec)
	require.NoError(t, err)
	defer network.Shutdown()
	fmt.Printf("Node rpc addr: %v\n", network[0].HTTPEndpoint())
	fmt.Printf("accounts: [\n")
	for _, a := range accounts.DeveloperAccounts() {
		fmt.Printf("%q,\n", a.PrivateKeyHex())
	}
	fmt.Printf("],\n")

	time.Sleep(math.MaxInt64)
}
