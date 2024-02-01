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
	"github.com/celo-org/celo-blockchain/contracts"
	"github.com/celo-org/celo-blockchain/contracts/config"
	"github.com/celo-org/celo-blockchain/core"
	"github.com/celo-org/celo-blockchain/core/types"
	"github.com/celo-org/celo-blockchain/eth"
	"github.com/celo-org/celo-blockchain/ethclient"
	"github.com/celo-org/celo-blockchain/internal/ethapi"
	"github.com/celo-org/celo-blockchain/mycelo/env"
	"github.com/celo-org/celo-blockchain/rpc"
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
	gingerbreadBlock := common.Big0
	gc, ec, err := test.BuildConfig(ac, gingerbreadBlock)
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
	gateWayFeeRecipient := devAccounts[2]

	// Get datum to set GasPrice/MaxFeePerGas/MaxPriorityFeePerGas to sensible values
	header, err := network[0].WsClient.HeaderByNumber(ctx, common.Big1)
	require.NoError(t, err)
	datum := header.BaseFee

	testCases := []struct {
		name        string
		txArgs      *ethapi.TransactionArgs
		expectedErr error
	}{
		{
			name: "eth compatible LegacyTxType",
			txArgs: &ethapi.TransactionArgs{
				To:       &recipient.Address,
				Value:    (*hexutil.Big)(new(big.Int).SetInt64(oneCelo)),
				GasPrice: (*hexutil.Big)(datum.Mul(datum, new(big.Int).SetInt64(4))),
			},
			expectedErr: nil,
		},
		{
			name: "eth incompatible LegacyTxType",
			txArgs: &ethapi.TransactionArgs{
				To:                  &recipient.Address,
				Value:               (*hexutil.Big)(new(big.Int).SetInt64(oneCelo)),
				GasPrice:            (*hexutil.Big)(datum.Mul(datum, new(big.Int).SetInt64(4))),
				GatewayFee:          (*hexutil.Big)(new(big.Int).SetInt64(oneCelo / 10)),
				GatewayFeeRecipient: &gateWayFeeRecipient.Address,
			},
			expectedErr: core.ErrGatewayFeeDeprecated,
		},
		{
			name: "AccessListTxType",
			txArgs: &ethapi.TransactionArgs{
				To:         &recipient.Address,
				Value:      (*hexutil.Big)(new(big.Int).SetInt64(oneCelo)),
				GasPrice:   (*hexutil.Big)(datum.Mul(datum, new(big.Int).SetInt64(4))),
				AccessList: &types.AccessList{},
			},
			expectedErr: nil,
		},
		{
			name: "DynamicFeeTxType - tip = MaxFeePerGas - BaseFee",
			txArgs: &ethapi.TransactionArgs{
				To:                   &recipient.Address,
				Value:                (*hexutil.Big)(new(big.Int).SetInt64(oneCelo)),
				MaxFeePerGas:         (*hexutil.Big)(datum.Mul(datum, new(big.Int).SetInt64(4))),
				MaxPriorityFeePerGas: (*hexutil.Big)(datum.Mul(datum, new(big.Int).SetInt64(4))),
			},
			expectedErr: nil,
		},
		{
			name: "DynamicFeeTxType - tip = MaxPriorityFeePerGas",
			txArgs: &ethapi.TransactionArgs{
				To:                   &recipient.Address,
				Value:                (*hexutil.Big)(new(big.Int).SetInt64(oneCelo)),
				MaxFeePerGas:         (*hexutil.Big)(datum.Mul(datum, new(big.Int).SetInt64(4))),
				MaxPriorityFeePerGas: (*hexutil.Big)(datum),
			},
			expectedErr: nil,
		},
		{
			name: "CeloDynamicFeeTxType - gas = MaxFeePerGas - BaseFee",
			txArgs: &ethapi.TransactionArgs{
				To:                   &recipient.Address,
				Value:                (*hexutil.Big)(new(big.Int).SetInt64(oneCelo)),
				MaxFeePerGas:         (*hexutil.Big)(datum.Mul(datum, new(big.Int).SetInt64(4))),
				MaxPriorityFeePerGas: (*hexutil.Big)(datum.Mul(datum, new(big.Int).SetInt64(4))),
				GatewayFee:           (*hexutil.Big)(new(big.Int).SetInt64(oneCelo / 10)),
				GatewayFeeRecipient:  &gateWayFeeRecipient.Address,
			},
			expectedErr: core.ErrGatewayFeeDeprecated,
		},
		{
			name: "CeloDynamicFeeTxType - MaxPriorityFeePerGas",
			txArgs: &ethapi.TransactionArgs{
				To:                   &recipient.Address,
				Value:                (*hexutil.Big)(new(big.Int).SetInt64(oneCelo)),
				MaxFeePerGas:         (*hexutil.Big)(datum.Mul(datum, new(big.Int).SetInt64(4))),
				MaxPriorityFeePerGas: (*hexutil.Big)(datum),
				GatewayFee:           (*hexutil.Big)(new(big.Int).SetInt64(oneCelo / 10)),
				GatewayFeeRecipient:  &gateWayFeeRecipient.Address,
			},
			expectedErr: core.ErrGatewayFeeDeprecated,
		},
		{
			name: "CeloDynamicFeeTxV2Type - gas = MaxFeePerGas - BaseFee",
			txArgs: &ethapi.TransactionArgs{
				To:                   &recipient.Address,
				Value:                (*hexutil.Big)(new(big.Int).SetInt64(oneCelo)),
				MaxFeePerGas:         (*hexutil.Big)(datum.Mul(datum, new(big.Int).SetInt64(4))),
				MaxPriorityFeePerGas: (*hexutil.Big)(datum.Mul(datum, new(big.Int).SetInt64(4))),
			},
			expectedErr: nil,
		},
		{
			name: "CeloDynamicFeeTxV2Type - MaxPriorityFeePerGas",
			txArgs: &ethapi.TransactionArgs{
				To:                   &recipient.Address,
				Value:                (*hexutil.Big)(new(big.Int).SetInt64(oneCelo)),
				MaxFeePerGas:         (*hexutil.Big)(datum.Mul(datum, new(big.Int).SetInt64(4))),
				MaxPriorityFeePerGas: (*hexutil.Big)(datum),
			},
			expectedErr: nil,
		},
	}

	// Get feeHandlerAddress
	var backend *eth.EthAPIBackend = network[0].Eth.APIBackend
	var lastestBlockNum rpc.BlockNumber = (rpc.BlockNumber)(backend.CurrentBlock().Header().Number.Int64())
	state, header, err := backend.StateAndHeaderByNumber(ctx, lastestBlockNum)
	require.NoError(t, err)
	caller := backend.NewEVMRunner(header, state)
	feeHandlerAddress, err := contracts.GetRegisteredAddress(caller, config.FeeHandlerId)
	require.NoError(t, err)
	require.NotEqual(t, common.ZeroAddress, feeHandlerAddress, "feeHandlerAddress must not be zero address.")

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			watcher := test.NewBalanceWatcher(client, []common.Address{sender.Address, recipient.Address, gateWayFeeRecipient.Address, node.Address, feeHandlerAddress})
			blockNum, err := client.BlockNumber(ctx)
			require.NoError(t, err)
			signer := types.MakeSigner(devAccounts[0].ChainConfig, new(big.Int).SetUint64(blockNum))
			tx, err := prepareTransaction(*tc.txArgs, sender.Key, sender.Address, signer, client, true)
			require.NoError(t, err)
			err = client.SendTransaction(ctx, tx)
			if tc.expectedErr != nil {
				// Once the error is checked, there's nothing more to do
				if err == nil || err.Error() != tc.expectedErr.Error() {
					t.Error("Expected error", tc.expectedErr, "got", err)
				}
				return
			}

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
			case types.DynamicFeeTxType, types.CeloDynamicFeeTxType, types.CeloDynamicFeeTxV2Type:
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
			case types.DynamicFeeTxType, types.CeloDynamicFeeTxType, types.CeloDynamicFeeTxV2Type:
				tip := tx.EffectiveGasTipValue(gpm)
				tip.Mul(tip, new(big.Int).SetUint64(receipt.GasUsed))
				fee = new(big.Int).Add(tip, baseFee)
			}
			consumed := new(big.Int).Add(tx.Value(), fee)
			if tx.GatewayFeeRecipient() != nil && tx.GatewayFee() != nil {
				consumed.Add(consumed, tx.GatewayFee())
			}
			expected = new(big.Int).Neg(consumed)
			actual = watcher.Delta(sender.Address)
			assert.Equal(t, expected, actual, "Sender's balance decrease unexpected", "expected", expected.Int64(), "actual", expected.Int64())

			// Check gateway fee
			if tx.GatewayFeeRecipient() != nil && tx.GatewayFee() != nil {
				expected = tx.GatewayFee()
				actual = watcher.Delta(gateWayFeeRecipient.Address)
				assert.Equal(t, expected, actual, "gateWayFeeRecipient's balance increase unexpected", "expected", expected.Int64(), "actual", actual.Int64())
			}

			// Check base fee was sent to FeeHandler
			fmt.Printf("%s ==> fee: %d, GatewayFee: %d, tip: %d\n", tc.name, tx.Fee(), tx.GatewayFee(), tx.EffectiveGasTipValue(gpm))
			expected = baseFee
			fmt.Printf("feeHandlerAddress: %x\n", feeHandlerAddress)
			actual = watcher.Delta(feeHandlerAddress)
			assert.Equal(t, expected, actual, "feeHandlers's balance increase unexpected", "expected", expected.Int64(), "actual", actual.Int64())
		})
	}
}

// TestTransferCELO checks following accounts:
// - Sender account has transfer value, transaction fee deducted
// - Receiver account has transfer value added.
// - Governance account has base fee added.
// - validator account has tip fee added.
func TestTransferCELOPreGingerbread(t *testing.T) {
	ac := test.AccountConfig(1, 3)
	var gingerbreadBlock *big.Int = nil
	gc, ec, err := test.BuildConfig(ac, gingerbreadBlock)

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
	gateWayFeeRecipient := devAccounts[2]

	// Get datum to set GasPrice/MaxFeePerGas/MaxPriorityFeePerGas to sensible values
	header, err := network[0].WsClient.HeaderByNumber(ctx, common.Big1)
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
				To:                  &recipient.Address,
				Value:               (*hexutil.Big)(new(big.Int).SetInt64(oneCelo)),
				GasPrice:            (*hexutil.Big)(datum.Mul(datum, new(big.Int).SetInt64(4))),
				GatewayFee:          (*hexutil.Big)(new(big.Int).SetInt64(oneCelo / 10)),
				GatewayFeeRecipient: &gateWayFeeRecipient.Address,
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
		{
			name: "CeloDynamicFeeTxType - gas = MaxFeePerGas - BaseFee",
			txArgs: &ethapi.TransactionArgs{
				To:                   &recipient.Address,
				Value:                (*hexutil.Big)(new(big.Int).SetInt64(oneCelo)),
				MaxFeePerGas:         (*hexutil.Big)(datum.Mul(datum, new(big.Int).SetInt64(4))),
				MaxPriorityFeePerGas: (*hexutil.Big)(datum.Mul(datum, new(big.Int).SetInt64(4))),
				GatewayFee:           (*hexutil.Big)(new(big.Int).SetInt64(oneCelo / 10)),
				GatewayFeeRecipient:  &gateWayFeeRecipient.Address,
			},
		},
		{
			name: "CeloDynamicFeeTxType - MaxPriorityFeePerGas",
			txArgs: &ethapi.TransactionArgs{
				To:                   &recipient.Address,
				Value:                (*hexutil.Big)(new(big.Int).SetInt64(oneCelo)),
				MaxFeePerGas:         (*hexutil.Big)(datum.Mul(datum, new(big.Int).SetInt64(4))),
				MaxPriorityFeePerGas: (*hexutil.Big)(datum),
				GatewayFee:           (*hexutil.Big)(new(big.Int).SetInt64(oneCelo / 10)),
				GatewayFeeRecipient:  &gateWayFeeRecipient.Address,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			watcher := test.NewBalanceWatcher(client, []common.Address{sender.Address, recipient.Address, gateWayFeeRecipient.Address, node.Address})
			blockNum, err := client.BlockNumber(ctx)
			require.NoError(t, err)
			signer := types.MakeSigner(devAccounts[0].ChainConfig, new(big.Int).SetUint64(blockNum))
			tx, err := prepareTransaction(*tc.txArgs, sender.Key, sender.Address, signer, client, false)
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
			case types.DynamicFeeTxType, types.CeloDynamicFeeTxType, types.CeloDynamicFeeTxV2Type:
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
			case types.DynamicFeeTxType, types.CeloDynamicFeeTxType, types.CeloDynamicFeeTxV2Type:
				tip := tx.EffectiveGasTipValue(gpm)
				tip.Mul(tip, new(big.Int).SetUint64(receipt.GasUsed))
				fee = new(big.Int).Add(tip, baseFee)
			}
			consumed := new(big.Int).Add(tx.Value(), fee)
			if tx.GatewayFeeRecipient() != nil && tx.GatewayFee() != nil {
				consumed.Add(consumed, tx.GatewayFee())
			}
			expected = new(big.Int).Neg(consumed)
			actual = watcher.Delta(sender.Address)
			assert.Equal(t, expected, actual, "Sender's balance decrease unexpected", "expected", expected.Int64(), "actual", expected.Int64())

			// Check gateway fee
			if tx.GatewayFeeRecipient() != nil && tx.GatewayFee() != nil {
				expected = tx.GatewayFee()
				actual = watcher.Delta(gateWayFeeRecipient.Address)
				assert.Equal(t, expected, actual, "gateWayFeeRecipient's balance increase unexpected", "expected", expected.Int64(), "actual", actual.Int64())
			}
		})
	}
}

// prepareTransaction prepares gasPrice, gasLimit and sign the transaction.
func prepareTransaction(txArgs ethapi.TransactionArgs, senderKey *ecdsa.PrivateKey, sender common.Address, signer types.Signer, client *ethclient.Client, isGingerbreadP2 bool) (*types.Transaction, error) {
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
	rawTx := txArgs.ToTransaction(isGingerbreadP2)
	signed, err := types.SignTx(rawTx, signer, senderKey)
	if err != nil {
		return nil, err
	}
	return signed, nil
}

func TestTransferERC20(t *testing.T) {
	ac := test.AccountConfig(1, 3)
	gingerbreadBlock := common.Big0
	gc, ec, err := test.BuildConfig(ac, gingerbreadBlock)
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
	gateWayFeeRecipient := devAccounts[2]

	// Get datum to set GasPrice/MaxFeePerGas/MaxPriorityFeePerGas to sensible values
	header, err := network[0].WsClient.HeaderByNumber(ctx, common.Big1)
	require.NoError(t, err)
	datum := header.BaseFee
	stableTokenAddress := env.MustProxyAddressFor("StableToken")
	intrinsicGas := hexutil.Uint64(config.IntrinsicGasForAlternativeFeeCurrency + 21000)

	testCases := []struct {
		name        string
		txArgs      *ethapi.TransactionArgs
		expectedErr error
	}{
		{
			name: "Celo transfer with fee currency",
			txArgs: &ethapi.TransactionArgs{
				To:                   &recipient.Address,
				Value:                (*hexutil.Big)(new(big.Int).SetInt64(oneCelo)),
				MaxFeePerGas:         (*hexutil.Big)(datum.Mul(datum, new(big.Int).SetInt64(4))),
				MaxPriorityFeePerGas: (*hexutil.Big)(datum),
				Gas:                  &intrinsicGas,
				FeeCurrency:          &stableTokenAddress,
			},
			expectedErr: nil,
		},
	}

	// Get feeHandlerAddress
	var backend *eth.EthAPIBackend = network[0].Eth.APIBackend
	var lastestBlockNum rpc.BlockNumber = (rpc.BlockNumber)(backend.CurrentBlock().Header().Number.Int64())
	state, header, err := backend.StateAndHeaderByNumber(ctx, lastestBlockNum)
	require.NoError(t, err)
	caller := backend.NewEVMRunner(header, state)
	feeHandlerAddress, err := contracts.GetRegisteredAddress(caller, config.FeeHandlerId)
	require.NoError(t, err)
	require.NotEqual(t, common.ZeroAddress, feeHandlerAddress, "feeHandlerAddress must not be zero address.")

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			watcher := test.NewBalanceWatcher(client, []common.Address{sender.Address, recipient.Address, gateWayFeeRecipient.Address, node.Address, feeHandlerAddress})
			blockNum, err := client.BlockNumber(ctx)
			require.NoError(t, err)
			signer := types.MakeSigner(devAccounts[0].ChainConfig, new(big.Int).SetUint64(blockNum))
			tx, err := prepareTransaction(*tc.txArgs, sender.Key, sender.Address, signer, client, true)
			require.NoError(t, err)
			err = client.SendTransaction(ctx, tx)
			if tc.expectedErr != nil {
				// Once the error is checked, there's nothing more to do
				if err == nil || err.Error() != tc.expectedErr.Error() {
					t.Error("Expected error", tc.expectedErr, "got", err)
				}
				return
			}

			require.NoError(t, err, "SendTransaction failed", "tx", *tx)
			err = network.AwaitTransactions(ctx, tx)
			require.NoError(t, err)
			watcher.Update()
			receipt, _ := client.TransactionReceipt(ctx, tx.Hash())

			// Check events
			// Transfer event id
			transferEvent := common.HexToHash("0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef")

			require.Equal(t, 2, len(receipt.Logs))

			require.Equal(t, transferEvent, receipt.Logs[0].Topics[0])
			require.Equal(t, sender.Address.Hash(), receipt.Logs[0].Topics[1])
			require.Equal(t, feeHandlerAddress, common.BytesToAddress(receipt.Logs[0].Topics[2].Bytes()))

			require.Equal(t, transferEvent, receipt.Logs[1].Topics[0])
			require.Equal(t, sender.Address.Hash(), receipt.Logs[1].Topics[1])
			require.Equal(t, node.Address.Hash(), receipt.Logs[1].Topics[2])

			// Check value goes to recipient
			expected := tx.Value()
			actual := watcher.Delta(recipient.Address)
			assert.Equal(t, expected, actual, "Recipient's balance increase unexpected", "expected", expected.Int64(), "actual", actual.Int64())
		})
	}
}
