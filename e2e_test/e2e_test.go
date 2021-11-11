package e2e_test

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"math/big"
	"testing"
	"time"

	ethereum "github.com/celo-org/celo-blockchain"

	bind "github.com/celo-org/celo-blockchain/accounts/abi/bind_v2"
	"github.com/celo-org/celo-blockchain/common/decimal/token"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/core/types"
	"github.com/celo-org/celo-blockchain/ethclient"
	"github.com/celo-org/celo-blockchain/mycelo/contract"
	"github.com/celo-org/celo-blockchain/mycelo/env"
	"github.com/celo-org/celo-blockchain/test"
	"github.com/stretchr/testify/assert"
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

// This test starts a network submits a transaction with the fee specified in
// cUSD but without sufficient balance to pay the fee and checks that the
// transaction is pruned.
func TestCusdFeeTxPrunedWhenInsufficientCusdBalance(t *testing.T) {
	accounts := test.Accounts(2)
	gc, ec, err := test.BuildConfig(accounts)
	require.NoError(t, err)
	network, err := test.NewNetwork(accounts, gc, ec)
	require.NoError(t, err)
	defer network.Shutdown()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
	defer cancel()

	// signer := types.MakeSigner(network[0].EthConfig.Genesis.Config, common.Big0)
	// tx, err := CusdTransaction(network[0].WsClient, network[0].DevKey, network[0].DevAddress, network[1].DevAddress, network[0].Nonce, big.NewInt(5), signer)

	// require.NoError(t, err)
	// tx, err := test.ValueTransferTransaction(network[0].WsClient, network[0].DevKey, network[0].DevAddress, network[1].DevAddress, network[0].Nonce, big.NewInt(50000), signer)

	abi := contract.AbiFor("StableToken")
	stableToken := bind.NewBoundContract(env.MustProxyAddressFor("StableToken"), *abi, network[0].WsClient)

	transactor, err := bind.NewKeyedTransactorWithChainID(network[0].DevKey, gc.ChainID)
	require.NoError(t, err)
	transactor.Context = ctx
	transactor.ChainID = gc.ChainID
	transactor.Nonce = new(big.Int).SetUint64(network[0].Nonce)
	stableTokenAddress := env.MustProxyAddressFor("StableToken")
	transactor.FeeCurrency = &stableTokenAddress

	// the starting balance of dev accounts is token.MustNew("50000") so if we spend that much then there should be nothing left for gas fees. Uncomment the line below to see the tx fail because it exceeds the balance.
	// tx, err := stableToken.TxObj(transactor, "transferWithComment", network[1].DevAddress, token.MustNew("50001").BigInt(), "need to proivde some long comment to make it similar to an encrypted comment").Transaction()
	tx, err := stableToken.TxObj(transactor, "transferWithComment", network[1].DevAddress, token.MustNew("50000").BigInt(), "need to proivde some long comment to make it similar to an encrypted comment").Transaction()
	require.NoError(t, err)

	err = network[0].WsClient.SendTransaction(ctx, tx)
	require.NoError(t, err)

	// Wait for the whole network to process the transaction.
	err = network.AwaitTransactions(ctx, tx)
	require.NoError(t, err)

	// Just double check the tx was processed
	processed := network[0].Tracker.GetProcessedTx(tx.Hash())
	assert.NotNil(t, processed)
}

func CusdTransaction(
	client *ethclient.Client,
	senderKey *ecdsa.PrivateKey,
	sender,
	recipient common.Address,
	nonce uint64,
	value *big.Int,
	signer types.Signer,
) (*types.Transaction, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*2)
	defer cancel()
	// Figure out the gas allowance and gas price values
	gasPrice, err := client.SuggestGasPrice(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to suggest gas price: %v", err)
	}

	msg := ethereum.CallMsg{From: sender, To: &recipient, GasPrice: gasPrice, Value: value}
	gasLimit, err := client.EstimateGas(ctx, msg)
	if err != nil {
		return nil, fmt.Errorf("failed to estimate gas needed: %v", err)
	}
	addr := env.MustProxyAddressFor("StableToken")
	rawTx := types.NewTransaction(nonce, recipient, value, gasLimit, gasPrice, &addr, nil, nil, nil)

	// Create the transaction and sign it
	// rawTx := types.NewTransactionEthCompatible(nonce, recipient, value, gasLimit, gasPrice, nil)
	signed, err := types.SignTx(rawTx, signer, senderKey)
	if err != nil {
		return nil, err
	}
	return signed, nil
}
