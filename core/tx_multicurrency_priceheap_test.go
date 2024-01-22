package core

import (
	"math/big"
	"sync/atomic"
	"testing"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/contracts/currency"
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

	// BaseFee per currency
	gpm := map[common.Address]*big.Int{
		*c1:                big.NewInt(10),
		*c2:                big.NewInt(1),
		common.ZeroAddress: big.NewInt(150), // celo currency base fee
	}
	var cmp CurrencyCmpFn = func(p1 *big.Int, cc1 *common.Address, p2 *big.Int, cc2 *common.Address) int {
		// currency1 = x10, currency2 = x100, currency nil (or other) = x1
		var val1 int = int(p1.Int64())
		var val2 int = int(p2.Int64())
		if common.AreEqualAddresses(cc1, c1) {
			val1 *= 10
		}
		if common.AreEqualAddresses(cc2, c1) {
			val2 *= 10
		}
		if common.AreEqualAddresses(cc1, c2) {
			val1 *= 100
		}
		if common.AreEqualAddresses(cc2, c2) {
			val2 *= 100
		}
		return val1 - val2
	}
	m := newMultiCurrencyPriceHeap(cmp, gpm)
	m.UpdateFeesAndCurrencies(cmp, gpm)
	m.Push(txC(100, c1)) // 100 * 10 - 10 * 10 = 900 (subtracting basefee x currencyValue)
	m.Push(txC(250, c1)) // 2500 - 10 * 10 = 2400
	m.Push(txC(50, c1))  // 500 - 100 = 400
	m.Push(txC(200, c1)) // 2000 - 100 = 1900
	m.Push(txC(75, c1))  // 750 - 100 = 650

	m.Push(txC(9, c2))  // 900 - 1 * 100 = 800
	m.Push(txC(26, c2)) // 2600 - 100 = 2500
	m.Push(txC(4, c2))  // 400 - 100 = 300
	m.Push(txC(21, c2)) // 2100 - 100 = 2000
	m.Push(txC(7, c2))  // 700 - 100 = 600

	m.Push(tx(1100)) // 1100 - 150 = 950
	m.Push(tx(2700)) // 2700 - 150 = 2550
	m.Push(tx(560))  // 560 - 150 = 410
	m.Push(tx(2150)) // 2150 - 150 = 2000
	m.Push(tx(750))  // 750 - 150 = 600

	assert.Equal(t, 15, m.Len())
	tm := m.Pop()
	assert.Equal(t, 14, m.Len())
	// 300
	assert.Equal(t, big.NewInt(4), tm.GasPrice())
	assert.Equal(t, c2, tm.FeeCurrency())

	tm2 := m.Pop()
	assert.Equal(t, 13, m.Len())
	// 400
	assert.Equal(t, big.NewInt(50), tm2.GasPrice())
	assert.Equal(t, c1, tm2.FeeCurrency())

	tm3 := m.Pop()
	assert.Equal(t, 12, m.Len())
	// 410
	assert.Equal(t, big.NewInt(560), tm3.GasPrice())
	assert.Nil(t, tm3.FeeCurrency())

	// A few more re-pushes
	m.Push(tx(585))    // 435
	m.Push(txC(3, c2)) // 200
	assert.Equal(t, 14, m.Len())

	tm4 := m.Pop()
	assert.Equal(t, 13, m.Len())
	// 200
	assert.Equal(t, big.NewInt(3), tm4.GasPrice())
	assert.Equal(t, c2, tm4.FeeCurrency())

	tm5 := m.Pop()
	assert.Equal(t, 12, m.Len())
	// 435
	assert.Equal(t, big.NewInt(585), tm5.GasPrice())
	assert.Nil(t, tm5.FeeCurrency())
}

func TestMultiAddInit(t *testing.T) {
	c1 := curr(1)
	c2 := curr(2)

	gpm := map[common.Address]*big.Int{
		*c1:                big.NewInt(10),
		*c2:                big.NewInt(1),
		common.ZeroAddress: big.NewInt(150), // celo currency base fee
	}
	var cmp CurrencyCmpFn = func(p1 *big.Int, cc1 *common.Address, p2 *big.Int, cc2 *common.Address) int {
		var val1 int = int(p1.Int64())
		var val2 int = int(p2.Int64())
		if common.AreEqualAddresses(cc1, c1) {
			val1 *= 10
		}
		if common.AreEqualAddresses(cc2, c1) {
			val2 *= 10
		}
		if common.AreEqualAddresses(cc1, c2) {
			val1 *= 100
		}
		if common.AreEqualAddresses(cc2, c2) {
			val2 *= 100
		}
		return val1 - val2
	}
	m := newMultiCurrencyPriceHeap(cmp, gpm)
	m.UpdateFeesAndCurrencies(cmp, gpm)
	m.Add(txC(100, c1)) // 100 * 10 - 10 * 10 = 900 (subtracting basefee x currencyValue)
	m.Add(txC(250, c1)) // 2500 - 10 * 10 = 2400
	m.Add(txC(50, c1))  // 500 - 100 = 400
	m.Add(txC(200, c1)) // 2000 - 100 = 1900
	m.Add(txC(75, c1))  // 750 - 100 = 650

	m.Add(txC(9, c2))  // 900 - 1 * 100 = 800
	m.Add(txC(26, c2)) // 2600 - 100 = 2500
	m.Add(txC(4, c2))  // 400 - 100 = 300
	m.Add(txC(21, c2)) // 2100 - 100 = 2000
	m.Add(txC(7, c2))  // 700 - 100 = 600

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

func TestCmp(t *testing.T) {
	curr1 := common.HexToAddress("abc1")
	ex1, _ := currency.NewExchangeRate(common.Big1, common.Big2) // 1 of curr1 is 2 celos
	curr2 := common.HexToAddress("abc2")
	ex2, _ := currency.NewExchangeRate(common.Big1, common.Big3) // 1 of curr2 is 3 celos
	rates := make(map[common.Address]*currency.Currency)
	rates[curr1] = currency.NewCurrency(curr1, *ex1)
	rates[curr2] = currency.NewCurrency(curr2, *ex2)
	var fn CurrencyCmpFn = func(p1 *big.Int, c1 *common.Address, p2 *big.Int, c2 *common.Address) int {
		if c1 == nil || c2 == nil {
			t.Fatal()
		}
		var c1Obj, c2Obj *currency.Currency
		c1Obj = rates[*c1]
		c2Obj = rates[*c2]
		if c1Obj == nil || c2Obj == nil {
			t.Fatal()
		}
		return c1Obj.CmpToCurrency(p1, p2, c2Obj)
	}

	// Same currency, No basefees
	assert.Equal(t, 0, fn.Cmp(txC(1, &curr1), txC(1, &curr1), nil, nil))
	assert.Equal(t, 1, fn.Cmp(txC(3, &curr2), txC(2, &curr2), nil, nil))
	assert.Equal(t, -1, fn.Cmp(txC(3, &curr2), txC(4, &curr2), nil, nil))

	// Diff currencies, No basefees
	assert.Equal(t, -1, fn.Cmp(txC(1, &curr1), txC(1, &curr2), nil, nil))
	assert.Equal(t, 1, fn.Cmp(txC(1, &curr2), txC(1, &curr1), nil, nil))
	assert.Equal(t, 0, fn.Cmp(txC(3, &curr1), txC(2, &curr2), nil, nil))

	// Same currency, with basefee
	assert.Equal(t, 0, fn.Cmp(txC(10, &curr1), txC(10, &curr1), common.Big2, common.Big2))
	assert.Equal(t, 1, fn.Cmp(txC(9, &curr1), txC(8, &curr1), common.Big2, common.Big2))
	assert.Equal(t, -1, fn.Cmp(txC(5, &curr1), txC(6, &curr1), common.Big3, common.Big3))

	// Diff currencies, with basefee
	assert.Equal(t, 0, fn.Cmp(txC(6, &curr1), txC(4, &curr2), common.Big3, common.Big2))
	assert.Equal(t, 1, fn.Cmp(txC(6, &curr1), txC(4, &curr2), common.Big2, common.Big2))
	assert.Equal(t, -1, fn.Cmp(txC(6, &curr1), txC(4, &curr2), common.Big3, common.Big1))
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
	// Tests that the CurrencyCmpFn receives the
	// proper fields for comparison in a gas fee cap cmp
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
	assert.True(t, cmp.Cmp(tx1, tx2, nil, nil) == -1)
}

func TestIsCheaper(t *testing.T) {
	// Tests that the result of the currency comparison function is
	// properly being returned in the tx comparison function
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
	assert.True(t, cheaper.Cmp(tx1, tx2, nil, nil) <= 0)
	assert.True(t, equal.Cmp(tx1, tx2, nil, nil) <= 0)
	assert.False(t, notCheaper.Cmp(tx1, tx2, nil, nil) <= 0)
}

// TestMulticurrencyUnderpriced tests that the underpriced method from pricedList functions
// properly when handling many different currencies.
func TestMulticurrencyUnderpriced(t *testing.T) {
	curr1 := common.HexToAddress("aaaa1")
	rate1, _ := currency.NewExchangeRate(common.Big1, common.Big1)
	curr2 := common.HexToAddress("aaaa2")
	rate2, _ := currency.NewExchangeRate(common.Big1, common.Big2)
	curr3 := common.HexToAddress("aaaa3")
	rate3, _ := currency.NewExchangeRate(common.Big1, common.Big3)
	currCache := map[common.Address]*currency.Currency{
		curr1: currency.NewCurrency(curr1, *rate1),
		curr2: currency.NewCurrency(curr2, *rate2),
		curr3: currency.NewCurrency(curr3, *rate3),
	}
	currencyManager := currency.NewCacheOnlyManager(currCache)
	txPoolCtx := txPoolContext{
		&SysContractCallCtx{
			whitelistedCurrencies: map[common.Address]struct{}{curr1: {}, curr2: {}, curr3: {}},
			gasPriceMinimums:      map[common.Address]*big.Int{curr1: nil, curr2: nil, curr3: nil},
		},
		currencyManager,
		nil,
	}
	ctxVal := atomic.Value{}
	ctxVal.Store(txPoolCtx)

	type addTx struct {
		tx    *types.Transaction
		local bool
	}
	type testCase struct {
		desc     string
		newTx    *types.Transaction
		expected bool
	}
	tests := []struct {
		name        string
		existingTxs []addTx
		cases       []testCase
	}{
		{
			name:        "Empty tx list (Never underpriced)",
			existingTxs: []addTx{},
			cases: []testCase{
				{
					desc:     "Normal tx",
					newTx:    tx(5),
					expected: false,
				},
				{
					desc:     "Celo tx",
					newTx:    txC(5, &curr2),
					expected: false,
				},
			},
		},
		{
			name: "Single tx in list",
			existingTxs: []addTx{
				{txC(5, &curr2), false},
			},
			cases: []testCase{
				{
					desc:     "Underpriced normal transaction",
					newTx:    tx(10),
					expected: true,
				},
				{
					desc:     "Accepted normal transaction",
					newTx:    tx(11),
					expected: false,
				},
				{
					desc:     "Underpriced celo transaction",
					newTx:    txC(5, &curr2),
					expected: true,
				},
				{
					desc:     "Accepted celo transaction",
					newTx:    txC(6, &curr2),
					expected: false,
				},
				{
					desc:     "Underpriced celo transaction with different currency",
					newTx:    txC(3, &curr3),
					expected: true,
				},
				{
					desc:     "Accepted celo transaction with different currency",
					newTx:    txC(4, &curr3),
					expected: false,
				},
			},
		},
		{
			name: "Mixed existing transactions",
			existingTxs: []addTx{
				{tx(3), false},
				{txC(5, &curr1), false},
			},
			cases: []testCase{
				{
					desc:     "Accepted native",
					newTx:    tx(4),
					expected: false,
				},
				{
					desc:     "Underpriced native",
					newTx:    tx(3),
					expected: true,
				},
				{
					desc:     "Accepted cheapest curr1",
					newTx:    txC(5, &curr1),
					expected: false,
				},
				{
					desc:     "Underpriced curr1",
					newTx:    txC(3, &curr1),
					expected: true,
				},
				{
					desc:     "Accepted cheapest curr3",
					newTx:    txC(2, &curr3),
					expected: false,
				},
				{
					desc:     "Underpriced curr3",
					newTx:    txC(1, &curr3),
					expected: true,
				},
			},
		},
		{
			name: "Multiple existing transactions",
			existingTxs: []addTx{
				{tx(3), false},
				{tx(5), false},
			},
			cases: []testCase{
				{
					desc:     "Accepted native",
					newTx:    tx(4),
					expected: false,
				},
				{
					desc:     "Underpriced native",
					newTx:    tx(3),
					expected: true,
				},
				{
					desc:     "Accepted cheapest curr1",
					newTx:    txC(5, &curr1),
					expected: false,
				},
				{
					desc:     "Underpriced curr1",
					newTx:    txC(3, &curr1),
					expected: true,
				},
				{
					desc:     "Accepted cheapest curr3",
					newTx:    txC(2, &curr3),
					expected: false,
				},
				{
					desc:     "Underpriced curr3",
					newTx:    txC(1, &curr3),
					expected: true,
				},
			},
		},
		{
			name: "Many txs from many currencies",
			existingTxs: []addTx{
				{txC(3, nil), false}, // Cheapest native currency: 3
				{txC(6, nil), false},
				{txC(5, &curr1), false}, // Cheapest curr1: 5
				{txC(2, &curr2), false}, // Cheapest curr2: 4
				{txC(2, &curr2), false},
				{txC(1, &curr3), false}, // Cheapest curr3: 3
			},
			cases: []testCase{
				{
					desc:     "Accepted cheapest curr1",
					newTx:    txC(4, &curr1),
					expected: false,
				},
				{
					desc:     "Underpriced native",
					newTx:    tx(3),
					expected: true,
				},
				{
					desc:     "Underpriced curr2",
					newTx:    txC(1, &curr2),
					expected: true,
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			all := newTxLookup()

			for _, tx := range tt.existingTxs {
				all.Add(tx.tx, tx.local)
			}

			pricedList := newTxPricedList(all, &ctxVal, 1024)
			pricedList.Reheap()

			for _, tc := range tt.cases {
				if got := pricedList.Underpriced(tc.newTx); got != tc.expected {
					t.Errorf("%s: txPricedList.Underpriced() = %v, want %v", tc.desc, got, tc.expected)
				}
			}
		})
	}
}
