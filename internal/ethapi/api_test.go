package ethapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"testing"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/common/hexutil"
	"github.com/celo-org/celo-blockchain/core/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewRPCTransactionCeloDynamicV2 tests the newRPCTransaction method with a celo dynamic fee tx v2 type.
func TestNewRPCTransactionCeloDynamicV2(t *testing.T) {
	currency := common.HexToAddress("0xCAFE")
	baseFee := big.NewInt(600)
	blockHash := common.BigToHash(big.NewInt(123456))
	blockNumber := uint64(123456)
	index := uint64(7)
	chainId := big.NewInt(1234567)
	gasTipCap := big.NewInt(888400)
	bigFeeCap := big.NewInt(1999001)
	smallFeeCap := big.NewInt(111222)
	baseFeeFn := func(curr *common.Address) (*big.Int, error) {
		if *curr == currency {
			return baseFee, nil
		}
		return nil, errors.New("unexpected")
	}

	t.Run("GasPrice == GasFeeCap", func(*testing.T) {
		rpcTx := newRPCTransaction(types.NewTx(&types.CeloDynamicFeeTxV2{
			FeeCurrency: &currency,
			ChainID:     chainId,

			GasFeeCap: smallFeeCap,
			GasTipCap: gasTipCap,
		}), blockHash, blockNumber, index, baseFeeFn, true)
		assert.Equal(t, (*hexutil.Big)(smallFeeCap), rpcTx.GasPrice)
	})

	t.Run("GasPrice == GasTipCap + baseFee", func(*testing.T) {
		rpcTx := newRPCTransaction(types.NewTx(&types.CeloDynamicFeeTxV2{
			FeeCurrency: &currency,
			ChainID:     chainId,

			GasFeeCap: bigFeeCap,
			GasTipCap: gasTipCap,
		}), blockHash, blockNumber, index, baseFeeFn, true)
		assert.Equal(t, (*hexutil.Big)(big.NewInt(0).Add(gasTipCap, baseFee)), rpcTx.GasPrice)
	})

	t.Run("Unmined transaction. GasPrice == GasFeeCap", func(t *testing.T) {
		rpcTx := newRPCTransaction(types.NewTx(&types.CeloDynamicFeeTxV2{
			FeeCurrency: &currency,
			ChainID:     chainId,

			GasFeeCap: bigFeeCap,
			GasTipCap: gasTipCap,
		}), common.Hash{}, 0, 0, baseFeeFn, false)
		assert.Equal(t, (*hexutil.Big)(bigFeeCap), rpcTx.GasPrice)
	})
}

// TestNewRPCTransactionCeloDynamic tests the newRPCTransaction method with a celo dynamic fee tx type.
func TestNewRPCTransactionCeloDynamic(t *testing.T) {
	currency := common.HexToAddress("0xCAFE")
	baseFee := big.NewInt(600)
	blockHash := common.BigToHash(big.NewInt(123456))
	blockNumber := uint64(123456)
	index := uint64(7)
	chainId := big.NewInt(1234567)
	gasTipCap := big.NewInt(888400)
	bigFeeCap := big.NewInt(1999001)
	smallFeeCap := big.NewInt(111222)
	baseFeeFn := func(curr *common.Address) (*big.Int, error) {
		if *curr == currency {
			return baseFee, nil
		}
		return nil, errors.New("unexpected")
	}

	t.Run("GasPrice == GasFeeCap", func(*testing.T) {
		rpcTx := newRPCTransaction(types.NewTx(&types.CeloDynamicFeeTx{
			FeeCurrency: &currency,
			ChainID:     chainId,

			GasFeeCap: smallFeeCap,
			GasTipCap: gasTipCap,
		}), blockHash, blockNumber, index, baseFeeFn, true)
		assert.Equal(t, (*hexutil.Big)(smallFeeCap), rpcTx.GasPrice)
	})

	t.Run("GasPrice == GasTipCap + baseFee", func(*testing.T) {
		rpcTx := newRPCTransaction(types.NewTx(&types.CeloDynamicFeeTx{
			FeeCurrency: &currency,
			ChainID:     chainId,

			GasFeeCap: bigFeeCap,
			GasTipCap: gasTipCap,
		}), blockHash, blockNumber, index, baseFeeFn, true)
		assert.Equal(t, (*hexutil.Big)(big.NewInt(0).Add(gasTipCap, baseFee)), rpcTx.GasPrice)
	})

	t.Run("Unminned transaction. GasPrice == GasFeeCap", func(t *testing.T) {
		rpcTx := newRPCTransaction(types.NewTx(&types.CeloDynamicFeeTx{
			FeeCurrency: &currency,
			ChainID:     chainId,

			GasFeeCap: bigFeeCap,
			GasTipCap: gasTipCap,
		}), common.Hash{}, 0, 0, baseFeeFn, false)
		assert.Equal(t, (*hexutil.Big)(bigFeeCap), rpcTx.GasPrice)
	})
}

// TestNewRPCTransactionCeloDynamic tests the newRPCTransaction method with a celo dynamic fee tx type.
func TestNewRPCTransactionDynamic(t *testing.T) {
	baseFee := big.NewInt(600)
	blockHash := common.BigToHash(big.NewInt(123456))
	blockNumber := uint64(123456)
	index := uint64(7)
	chainId := big.NewInt(1234567)
	gasTipCap := big.NewInt(888400)
	bigFeeCap := big.NewInt(1999001)
	smallFeeCap := big.NewInt(111222)
	baseFeeFn := func(curr *common.Address) (*big.Int, error) {
		return baseFee, nil
	}

	t.Run("GasPrice == GasFeeCap", func(*testing.T) {
		rpcTx := newRPCTransaction(types.NewTx(&types.DynamicFeeTx{
			ChainID: chainId,

			GasFeeCap: smallFeeCap,
			GasTipCap: gasTipCap,
		}), blockHash, blockNumber, index, baseFeeFn, true)
		assert.Equal(t, (*hexutil.Big)(smallFeeCap), rpcTx.GasPrice)
	})

	t.Run("GasPrice == GasTipCap + baseFee", func(*testing.T) {
		rpcTx2 := newRPCTransaction(types.NewTx(&types.DynamicFeeTx{
			ChainID: chainId,

			GasFeeCap: bigFeeCap,
			GasTipCap: gasTipCap,
		}), blockHash, blockNumber, index, baseFeeFn, true)
		assert.Equal(t, (*hexutil.Big)(big.NewInt(0).Add(gasTipCap, baseFee)), rpcTx2.GasPrice)
	})

	t.Run("Unminned transaction. GasPrice == GasFeeCap", func(t *testing.T) {
		rpcTx := newRPCTransaction(types.NewTx(&types.DynamicFeeTx{
			ChainID: chainId,

			GasFeeCap: bigFeeCap,
			GasTipCap: gasTipCap,
		}), common.Hash{}, 0, 0, baseFeeFn, false)
		assert.Equal(t, (*hexutil.Big)(bigFeeCap), rpcTx.GasPrice)
	})
}

// TestNewRPCTransactionEthCompatible tests that only legacy transactions have the eth compatbile field set.
func TestNewRPCTransactionEthCompatible(t *testing.T) {
	blockHash := common.BigToHash(big.NewInt(123456))
	blockNumber := uint64(123456)
	index := uint64(7)
	baseFeeFn := func(curr *common.Address) (*big.Int, error) {
		return big.NewInt(600), nil
	}

	t.Run("LegacyTx ethCompatible", func(*testing.T) {
		rpcTx := newRPCTransaction(types.NewTx(&types.LegacyTx{
			EthCompatible: true,
		}), blockHash, blockNumber, index, baseFeeFn, true)
		assert.Equal(t, true, jsonRoundtripToMap(t, rpcTx)["ethCompatible"])
	})

	t.Run("LegacyTx not ethCompatible", func(*testing.T) {
		rpcTx := newRPCTransaction(types.NewTx(&types.LegacyTx{
			EthCompatible: false,
		}), blockHash, blockNumber, index, baseFeeFn, true)
		assert.Equal(t, false, jsonRoundtripToMap(t, rpcTx)["ethCompatible"])
	})

	t.Run("Non legacy tx ethCompatible not set", func(*testing.T) {
		rpcTx := newRPCTransaction(types.NewTx(&types.DynamicFeeTx{}), blockHash, blockNumber, index, baseFeeFn, true)
		assert.NotContains(t, jsonRoundtripToMap(t, rpcTx), "ethCompatible")
		fmt.Printf("%+v\n", jsonRoundtripToMap(t, rpcTx))
	})
}

func jsonRoundtripToMap(t *testing.T, tx *RPCTransaction) map[string]interface{} {
	marshaled, err := json.Marshal(tx)
	require.NoError(t, err)
	m := make(map[string]interface{})
	err = json.Unmarshal(marshaled, &m)
	require.NoError(t, err)
	return m
}
