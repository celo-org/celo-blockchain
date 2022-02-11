package core

import (
	"math/big"
	"testing"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/core/types"
	"github.com/stretchr/testify/assert"
)

func curr(currency int) *common.Address {
	curr := common.BigToAddress(big.NewInt(int64(currency)))
	return &curr
}

func tx(price int) *types.Transaction {
	return types.NewTx(&types.LegacyTx{GasPrice: big.NewInt(int64(price))})
}

func txC(price int, currency *common.Address) *types.Transaction {
	return types.NewTx(&types.LegacyTx{
		GasPrice:    big.NewInt(int64(price)),
		FeeCurrency: currency,
	})
}

func TestNilPushes(t *testing.T) {
	m := newMultiCurrencyPriceHeap(nil, nil)
	m.Push(tx(100))
	m.Push(tx(50))
	m.Push(tx(200))
	m.Push(tx(75))
	assert.Equal(t, 4, m.Len())
	tm := m.Pop()
	assert.Equal(t, big.NewInt(50), tm.GasPrice())
	assert.Equal(t, 3, m.Len())
}

func TestCurrencyPushes(t *testing.T) {
	c := curr(1)
	gpm := map[common.Address]*big.Int{
		*c: big.NewInt(1000),
	}
	m := newMultiCurrencyPriceHeap(nil, gpm)
	m.Push(txC(100, c))
	m.Push(txC(50, c))
	m.Push(txC(200, c))
	m.Push(txC(75, c))
	assert.Equal(t, 4, m.Len())
	tm := m.Pop()
	assert.Equal(t, big.NewInt(50), tm.GasPrice())
	assert.Equal(t, 3, m.Len())
}

func TestNilAdds(t *testing.T) {
	m := newMultiCurrencyPriceHeap(nil, nil)
	m.Add(tx(100))
	m.Add(tx(250))
	m.Add(tx(50))
	m.Add(tx(200))
	m.Add(tx(75))
	assert.Equal(t, 5, m.Len())
	tm := m.Pop()
	// there was no Init after the adds, so it should return them in FIFO order
	assert.Equal(t, big.NewInt(100), tm.GasPrice())
	assert.Equal(t, 4, m.Len())

	m.Init()
	tm2 := m.Pop()
	assert.Equal(t, big.NewInt(50), tm2.GasPrice())
	assert.Equal(t, 3, m.Len())
}

func TestCurrencyAdds(t *testing.T) {
	c := curr(1)
	gpm := map[common.Address]*big.Int{
		*c: big.NewInt(1000),
	}
	m := newMultiCurrencyPriceHeap(nil, gpm)
	m.Add(txC(100, c))
	m.Add(txC(250, c))
	m.Add(txC(50, c))
	m.Add(txC(200, c))
	m.Add(txC(75, c))
	assert.Equal(t, 5, m.Len())
	tm := m.Pop()
	// there was no Init after the adds, so it should return them in FIFO order
	assert.Equal(t, big.NewInt(100), tm.GasPrice())
	assert.Equal(t, 4, m.Len())

	m.Init()
	tm2 := m.Pop()
	assert.Equal(t, big.NewInt(50), tm2.GasPrice())
	assert.Equal(t, 3, m.Len())
}

func TestClear(t *testing.T) {
	c := curr(1)
	gpm := map[common.Address]*big.Int{
		*c: big.NewInt(1000),
	}
	m := newMultiCurrencyPriceHeap(nil, gpm)
	m.Push(txC(100, c))
	m.Push(txC(250, c))
	m.Push(txC(50, c))
	m.Push(txC(200, c))
	m.Push(txC(75, c))
	m.Push(tx(100))
	m.Push(tx(700))
	assert.Equal(t, 7, m.Len())
	m.Clear()
	assert.Equal(t, 0, m.Len())
	assert.Nil(t, m.Pop())
}

func TestIsCheaper_FwdFields(t *testing.T) {
	curr1 := common.BigToAddress(big.NewInt(123))
	price1 := big.NewInt(100)
	curr2 := common.BigToAddress(big.NewInt(123))
	price2 := big.NewInt(200)
	tx1 := types.NewTx(&types.LegacyTx{
		GasPrice:    price1,
		FeeCurrency: &curr1,
	})
	tx2 := types.NewTx(&types.LegacyTx{
		GasPrice:    price2,
		FeeCurrency: &curr2,
	})
	var cmp CurrencyCmpFn = func(p1 *big.Int, c1 *common.Address, p2 *big.Int, c2 *common.Address) int {
		assert.Equal(t, price1, p1)
		assert.Equal(t, price2, p2)
		assert.Equal(t, curr1, *c1)
		assert.Equal(t, curr2, *c2)
		return -1
	}
	assert.True(t, cmp.IsCheaper(tx1, tx2))
}

func TestIsCheaper(t *testing.T) {
	tx1 := types.NewTx(&types.LegacyTx{})
	tx2 := types.NewTx(&types.LegacyTx{})
	var cheaper CurrencyCmpFn = func(p1 *big.Int, c1 *common.Address, p2 *big.Int, c2 *common.Address) int {
		return -1
	}
	var equal CurrencyCmpFn = func(p1 *big.Int, c1 *common.Address, p2 *big.Int, c2 *common.Address) int {
		return 0
	}
	var notCheaper CurrencyCmpFn = func(p1 *big.Int, c1 *common.Address, p2 *big.Int, c2 *common.Address) int {
		return 1
	}
	assert.True(t, cheaper.IsCheaper(tx1, tx2))
	assert.False(t, equal.IsCheaper(tx1, tx2))
	assert.False(t, notCheaper.IsCheaper(tx1, tx2))
}
