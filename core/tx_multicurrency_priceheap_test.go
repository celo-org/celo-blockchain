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

func TestMultiPushPop(t *testing.T) {
	c1 := curr(1)
	c2 := curr(2)

	gpm := map[common.Address]*big.Int{
		*c1: big.NewInt(10),
		*c2: big.NewInt(20),
	}
	var cmp CurrencyCmpFn = func(p1 *big.Int, cc1 *common.Address, p2 *big.Int, cc2 *common.Address) int {
		var val1 int = int(p1.Int64())
		var val2 int = int(p2.Int64())
		if cc1 == c1 {
			val1 *= 10
		}
		if cc2 == c1 {
			val2 *= 10
		}
		if cc1 == c2 {
			val1 *= 100
		}
		if cc2 == c2 {
			val2 *= 100
		}
		return val1 - val2
	}
	m := newMultiCurrencyPriceHeap(cmp, gpm)
	m.Push(txC(100, c1)) // 1000
	m.Push(txC(250, c1)) // 2500
	m.Push(txC(50, c1))  // 500
	m.Push(txC(200, c1)) // 2000
	m.Push(txC(75, c1))  // 750

	m.Push(txC(9, c2))  // 900
	m.Push(txC(26, c2)) // 2600
	m.Push(txC(4, c2))  // 400
	m.Push(txC(21, c2)) // 2100
	m.Push(txC(7, c2))  // 700

	m.Push(tx(1100)) // 1100
	m.Push(tx(2700)) // 2700
	m.Push(tx(560))  // 560
	m.Push(tx(2150)) // 2150
	m.Push(tx(750))  // 750

	assert.Equal(t, 15, m.Len())
	tm := m.Pop()
	assert.Equal(t, 14, m.Len())
	// 400
	assert.Equal(t, big.NewInt(4), tm.GasPrice())
	assert.Equal(t, c2, tm.FeeCurrency())

	tm2 := m.Pop()
	assert.Equal(t, 13, m.Len())
	// 500
	assert.Equal(t, big.NewInt(50), tm2.GasPrice())
	assert.Equal(t, c1, tm2.FeeCurrency())

	tm3 := m.Pop()
	assert.Equal(t, 12, m.Len())
	// 560
	assert.Equal(t, big.NewInt(560), tm3.GasPrice())
	assert.Nil(t, tm3.FeeCurrency())

	// A few more re-pushes
	m.Push(tx(585))    // 585
	m.Push(txC(3, c2)) // 300
	assert.Equal(t, 14, m.Len())

	tm4 := m.Pop()
	assert.Equal(t, 13, m.Len())
	// 300
	assert.Equal(t, big.NewInt(3), tm4.GasPrice())
	assert.Equal(t, c2, tm4.FeeCurrency())

	tm5 := m.Pop()
	assert.Equal(t, 12, m.Len())
	// 585
	assert.Equal(t, big.NewInt(585), tm5.GasPrice())
	assert.Nil(t, tm5.FeeCurrency())
}

func TestMultiAddInit(t *testing.T) {
	c1 := curr(1)
	c2 := curr(2)

	gpm := map[common.Address]*big.Int{
		*c1: big.NewInt(10),
		*c2: big.NewInt(20),
	}
	var cmp CurrencyCmpFn = func(p1 *big.Int, cc1 *common.Address, p2 *big.Int, cc2 *common.Address) int {
		var val1 int = int(p1.Int64())
		var val2 int = int(p2.Int64())
		if cc1 == c1 {
			val1 *= 10
		}
		if cc2 == c1 {
			val2 *= 10
		}
		if cc1 == c2 {
			val1 *= 100
		}
		if cc2 == c2 {
			val2 *= 100
		}
		return val1 - val2
	}
	m := newMultiCurrencyPriceHeap(cmp, gpm)
	m.Add(txC(100, c1)) // 1000
	m.Add(txC(250, c1)) // 2500
	m.Add(txC(50, c1))  // 500
	m.Add(txC(200, c1)) // 2000
	m.Add(txC(75, c1))  // 750

	m.Add(txC(9, c2))  // 900
	m.Add(txC(26, c2)) // 2600
	m.Add(txC(4, c2))  // 400
	m.Add(txC(21, c2)) // 2100
	m.Add(txC(7, c2))  // 700

	m.Add(tx(1100)) // 1100
	m.Add(tx(2700)) // 2700
	m.Add(tx(560))  // 560
	m.Add(tx(2150)) // 2150
	m.Add(tx(750))  // 750

	// no init yet, returns the cheapest of the first of every currency
	assert.Equal(t, 15, m.Len())
	odd := m.Pop()
	assert.Equal(t, 14, m.Len())
	assert.Equal(t, big.NewInt(9), odd.GasPrice())
	assert.Equal(t, c2, odd.FeeCurrency())

	m.Init()

	tm := m.Pop()
	assert.Equal(t, 13, m.Len())
	// 400
	assert.Equal(t, big.NewInt(4), tm.GasPrice())
	assert.Equal(t, c2, tm.FeeCurrency())

	tm2 := m.Pop()
	assert.Equal(t, 12, m.Len())
	// 500
	assert.Equal(t, big.NewInt(50), tm2.GasPrice())
	assert.Equal(t, c1, tm2.FeeCurrency())

	tm3 := m.Pop()
	assert.Equal(t, 11, m.Len())
	// 560
	assert.Equal(t, big.NewInt(560), tm3.GasPrice())
	assert.Nil(t, tm3.FeeCurrency())

	// Re add and break it
	m.Add(tx(585))    // 585
	m.Add(txC(3, c2)) // 300
	assert.Equal(t, 13, m.Len())

	tm4 := m.Pop()
	assert.Equal(t, 12, m.Len())
	// No Init, next in line should be the 700 tx
	assert.Equal(t, big.NewInt(7), tm4.GasPrice())
	assert.Equal(t, c2, tm4.FeeCurrency())

	m.Init()
	tm5 := m.Pop()
	assert.Equal(t, 11, m.Len())
	// Init called, new 300 one should be popped first
	assert.Equal(t, big.NewInt(3), tm5.GasPrice())
	assert.Equal(t, c2, tm5.FeeCurrency())
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
