package e2e

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"math/big"
	"testing"
	"time"

	ethereum "github.com/celo-org/celo-blockchain"
	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/common/hexutil"
	"github.com/celo-org/celo-blockchain/core/types"
	"github.com/celo-org/celo-blockchain/ethclient"
	"github.com/celo-org/celo-blockchain/internal/ethapi"
	"github.com/celo-org/celo-blockchain/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	oneCelo = 1000000000000000000
)

// TestTransferCELO checks following accounts:
// - Sender account has transfer value, transaction fee deducted
// - Receiver account has transfer value added.
// - Governance account has base fee added.
// - validator account has tip fee added.
func TestTransferCELO(t *testing.T) {
	ac := test.AccountConfig(1, 3)
	gc, ec, err := test.BuildConfig(ac)
	gc.Hardforks.EspressoBlock = big.NewInt(0)
	require.NoError(t, err)
	network, shutdown, err := test.NewNetwork(ac, gc, ec)
	require.NoError(t, err)
	defer shutdown()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
	defer cancel()

	node := network[0] // validator node
	client := node.WsClient
	devAccounts := test.Accounts(ac.DeveloperAccounts(), gc.ChainConfig())
	sender := devAccounts[0]
	recipient := devAccounts[1]

	// Get datum to set GasPrice/MaxFeePerGas/MaxPriorityFeePerGas to sensible values
	header, err := network[0].WsClient.HeaderByNumber(ctx, common.Big0)
	require.NoError(t, err)
	datum, err := network[0].Eth.APIBackend.GasPriceMinimumForHeader(ctx, nil, header)
	require.NoError(t, err)

	testCases := []struct {
		name   string
		txArgs *ethapi.TransactionArgs
	}{
		{
			name: "eth compatible LegacyTxType",
			txArgs: &ethapi.TransactionArgs{
				To:       &recipient.Address,
				Value:    (*hexutil.Big)(new(big.Int).SetInt64(oneCelo)),
				GasPrice: (*hexutil.Big)(datum.Mul(datum, new(big.Int).SetInt64(4))),
			},
		},
		{
			name: "eth incompatible LegacyTxType",
			txArgs: &ethapi.TransactionArgs{
				To:       &recipient.Address,
				Value:    (*hexutil.Big)(new(big.Int).SetInt64(oneCelo)),
				GasPrice: (*hexutil.Big)(datum.Mul(datum, new(big.Int).SetInt64(4))),
				// It needs something different than nil. This test is not validating that the currency was whitelisted
				FeeCurrency: &common.ZeroAddress,
			},
		},
		{
			name: "AccessListTxType",
			txArgs: &ethapi.TransactionArgs{
				To:         &recipient.Address,
				Value:      (*hexutil.Big)(new(big.Int).SetInt64(oneCelo)),
				GasPrice:   (*hexutil.Big)(datum.Mul(datum, new(big.Int).SetInt64(4))),
				AccessList: &types.AccessList{},
			},
		},
		{
			name: "DynamicFeeTxType - tip = MaxFeePerGas - BaseFee",
			txArgs: &ethapi.TransactionArgs{
				To:                   &recipient.Address,
				Value:                (*hexutil.Big)(new(big.Int).SetInt64(oneCelo)),
				MaxFeePerGas:         (*hexutil.Big)(datum.Mul(datum, new(big.Int).SetInt64(4))),
				MaxPriorityFeePerGas: (*hexutil.Big)(datum.Mul(datum, new(big.Int).SetInt64(4))),
			},
		},
		{
			name: "DynamicFeeTxType - tip = MaxPriorityFeePerGas",
			txArgs: &ethapi.TransactionArgs{
				To:                   &recipient.Address,
				Value:                (*hexutil.Big)(new(big.Int).SetInt64(oneCelo)),
				MaxFeePerGas:         (*hexutil.Big)(datum.Mul(datum, new(big.Int).SetInt64(4))),
				MaxPriorityFeePerGas: (*hexutil.Big)(datum),
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			watcher := test.NewBalanceWatcher(client, []common.Address{sender.Address, recipient.Address, node.Address})
			blockNum, err := client.BlockNumber(ctx)
			require.NoError(t, err)
			signer := types.MakeSigner(devAccounts[0].ChainConfig, new(big.Int).SetUint64(blockNum))
			tx, err := prepareTransaction(*tc.txArgs, sender.Key, sender.Address, signer, client)
			require.NoError(t, err)
			err = client.SendTransaction(ctx, tx)
			require.NoError(t, err, "SendTransaction failed", "tx", *tx)
			err = network.AwaitTransactions(ctx, tx)
			require.NoError(t, err)
			watcher.Update()
			receipt, err := client.TransactionReceipt(ctx, tx.Hash())
			require.NoError(t, err)

			// check value goes to recipient
			expected := tx.Value()
			actual := watcher.Delta(recipient.Address)
			assert.Equal(t, expected, actual, "Recipient's balance increase unexpected", "expected", expected.Int64(), "actual", actual.Int64())

			// Check tip goes to validator
			header, err := network[0].WsClient.HeaderByNumber(ctx, receipt.BlockNumber)
			require.NoError(t, err)
			gpm, err := network[0].Eth.APIBackend.GasPriceMinimumForHeader(ctx, nil, header)
			require.NoError(t, err)
			baseFee := new(big.Int).Mul(gpm, new(big.Int).SetUint64(receipt.GasUsed))
			switch tx.Type() {
			case types.LegacyTxType, types.AccessListTxType:
				fee := new(big.Int).Mul(tx.GasPrice(), new(big.Int).SetUint64(receipt.GasUsed))
				expected = new(big.Int).Sub(fee, baseFee)
			case types.DynamicFeeTxType:
				expected = tx.EffectiveGasTipValue(gpm)
				expected.Mul(expected, new(big.Int).SetUint64(receipt.GasUsed))
			}
			actual = watcher.Delta(node.Address)
			assert.Equal(t, expected, actual, "Validator's balance increase unexpected", "expected", expected.Int64(), "actual", actual.Int64())

			// check value + tx fee + gateway fee are subtracted from sender
			var fee *big.Int
			switch tx.Type() {
			case types.LegacyTxType, types.AccessListTxType:
				fee = new(big.Int).Mul(tx.GasPrice(), new(big.Int).SetUint64(receipt.GasUsed))
			case types.DynamicFeeTxType:
				tip := tx.EffectiveGasTipValue(gpm)
				tip.Mul(tip, new(big.Int).SetUint64(receipt.GasUsed))
				fee = new(big.Int).Add(tip, baseFee)
			}
			consumed := new(big.Int).Add(tx.Value(), fee)
			expected = new(big.Int).Neg(consumed)
			actual = watcher.Delta(sender.Address)
			assert.Equal(t, expected, actual, "Sender's balance decrease unexpected", "expected", expected.Int64(), "actual", expected.Int64())
		})
	}
}

// prepareTransaction prepares gasPrice, gasLimit and sign the transaction.
func prepareTransaction(txArgs ethapi.TransactionArgs, senderKey *ecdsa.PrivateKey, sender common.Address, signer types.Signer, client *ethclient.Client) (*types.Transaction, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*2)
	defer cancel()

	// Set nonce
	nonce, err := client.PendingNonceAt(ctx, sender)
	if err != nil {
		return nil, err
	}
	txArgs.Nonce = (*hexutil.Uint64)(&nonce)

	// Set gasLimit
	if txArgs.Gas == nil {
		msg := ethereum.CallMsg{From: sender, To: txArgs.To, GasPrice: txArgs.GasPrice.ToInt(), Value: txArgs.Value.ToInt()}
		gasLimit, err := client.EstimateGas(ctx, msg)
		if err != nil {
			return nil, fmt.Errorf("failed to estimate gas needed: %v", err)
		}
		txArgs.Gas = (*hexutil.Uint64)(&gasLimit)
	}

	// Create the transaction and sign it
	rawTx := txArgs.ToTransaction()
	signed, err := types.SignTx(rawTx, signer, senderKey)
	if err != nil {
		return nil, err
	}
	return signed, nil
}
