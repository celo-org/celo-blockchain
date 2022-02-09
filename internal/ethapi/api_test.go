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
	baseFeeFn := func(curr *common.Address) (*big.Int, error) {
		if *curr == currency {
			return baseFee, nil
		}
		return nil, errors.New("unexpected")
	}
	// Test GasPrice == GasFeeCap
	rpcTx := newRPCTransaction(types.NewTx(&types.CeloDynamicFeeTx{
		FeeCurrency: &currency,
		ChainID:     big.NewInt(1234567),

		GasFeeCap: big.NewInt(111222),
		GasTipCap: big.NewInt(888400),
	}), blockHash, blockNumber, index, baseFeeFn)
	assert.Equal(t, (*hexutil.Big)(big.NewInt(111222)), rpcTx.GasPrice)
	// Test GasPrice == GasTipCap + baseFee
	rpcTx2 := newRPCTransaction(types.NewTx(&types.CeloDynamicFeeTx{
		FeeCurrency: &currency,
		ChainID:     big.NewInt(1234567),

		GasFeeCap: big.NewInt(1999001),
		GasTipCap: big.NewInt(888400),
	}), blockHash, blockNumber, index, baseFeeFn)
	assert.Equal(t, (*hexutil.Big)(big.NewInt(889000)), rpcTx2.GasPrice)
	// Test unminned transaction. GasPrice == GasFeeCap
	rpcTx3 := newRPCTransaction(types.NewTx(&types.CeloDynamicFeeTx{
		FeeCurrency: &currency,
		ChainID:     big.NewInt(1234567),

		GasFeeCap: big.NewInt(1999001),
		GasTipCap: big.NewInt(888400),
	}), common.Hash{}, 0, 0, baseFeeFn)
	assert.Equal(t, (*hexutil.Big)(big.NewInt(1999001)), rpcTx3.GasPrice)
}
