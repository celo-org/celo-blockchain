package ethapi

import (
	"errors"
	"math/big"
	"testing"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/common/hexutil"
	"github.com/celo-org/celo-blockchain/core/types"
	"github.com/stretchr/testify/assert"
)

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
