package core

import (
	"container/heap"
	"math/big"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/core/types"
)

type CurrencyCmpFn func(*big.Int, *common.Address, *big.Int, *common.Address) int

// IsCheaper returns true if tx1 is cheaper than tx2 (GasPrice with currency comparison)
func (cc CurrencyCmpFn) IsCheaper(tx1, tx2 *types.Transaction) bool {
	return cc(tx1.GasPrice(), tx1.FeeCurrency(), tx2.GasPrice(), tx2.FeeCurrency()) < 0
}

// multiCurrencyPriceHeap is a heap.Interface implementation over transactions
// with different fee currencies for retrieving price-sorted transactions to discard
// when the pool fills up. If baseFee is set then the heap is sorted based on the
// effective tip based on the given base fee. If baseFee is nil then the sorting
// is based on gasFeeCap.
type multiCurrencyPriceHeap struct {
	currencyCmp        CurrencyCmpFn
	gpm                GasPriceMinimums              // heap should always be re-sorted after gas price minimums (baseFees) is changed
	currencyHeaps      map[common.Address]*priceHeap // Heap of prices of all the stored non-nil currency transactions
	nativeCurrencyHeap *priceHeap                    // Heap of prices of all the stored nil currency transactions
}

func newMultiCurrencyPriceHeap(currencyCmp CurrencyCmpFn, gpm GasPriceMinimums) multiCurrencyPriceHeap {
	return multiCurrencyPriceHeap{
		currencyCmp: currencyCmp,
		gpm:         gpm,

		// inner state

		nativeCurrencyHeap: &priceHeap{},
		currencyHeaps:      make(map[common.Address]*priceHeap),
	}
}

// getHeapFor returns the proper heap for the given transaction, and creates it
// if it's not available in the currencyHeaps
func (h *multiCurrencyPriceHeap) getHeapFor(tx *types.Transaction) *priceHeap {
	fc := tx.FeeCurrency()
	if fc == nil {
		return h.nativeCurrencyHeap
	}
	if _, ok := h.currencyHeaps[*fc]; !ok {
		h.currencyHeaps[*fc] = &priceHeap{
			baseFee: h.gpm.GetGasPriceMinimum(fc),
		}
	}
	return h.currencyHeaps[*fc]
}

// Add to the heap. Must call Init afterwards to retain the heap invariants.
func (h *multiCurrencyPriceHeap) Add(tx *types.Transaction) {
	ph := h.getHeapFor(tx)
	ph.list = append(ph.list, tx)
}

// Push to the heap, maintains heap invariants.
func (h *multiCurrencyPriceHeap) Push(tx *types.Transaction) {
	ph := h.getHeapFor(tx)
	heap.Push(ph, tx)
}

func (h *multiCurrencyPriceHeap) cheapestTxs() []*types.Transaction {
	txs := make([]*types.Transaction, 0, 1+len(h.currencyHeaps))
	if len(h.nativeCurrencyHeap.list) > 0 {
		txs = append(txs, h.nativeCurrencyHeap.list[0])
	}
	for _, ph := range h.currencyHeaps {
		if len(ph.list) > 0 {
			txs = append(txs, ph.list[0])
		}
	}
	return txs
}

func (h *multiCurrencyPriceHeap) cheapestTx() *types.Transaction {
	txs := h.cheapestTxs()
	var cheapestTx *types.Transaction
	for _, tx := range txs {
		if cheapestTx == nil || h.currencyCmp.IsCheaper(tx, cheapestTx) {
			cheapestTx = tx
		}
	}
	return cheapestTx
}

func (h *multiCurrencyPriceHeap) Pop() *types.Transaction {
	cheapestTx := h.cheapestTx()
	if cheapestTx == nil {
		return nil
	}
	ph := h.getHeapFor(cheapestTx)
	return heap.Pop(ph).(*types.Transaction)
}

func (h *multiCurrencyPriceHeap) Len() int {
	r := len(h.nativeCurrencyHeap.list)
	for _, priceHeap := range h.currencyHeaps {
		r += len(priceHeap.list)
	}
	return r
}

func (h *multiCurrencyPriceHeap) Init() {
	heap.Init(h.nativeCurrencyHeap)
	for _, priceHeap := range h.currencyHeaps {
		heap.Init(priceHeap)
	}
}

func (h *multiCurrencyPriceHeap) Clear() {
	h.nativeCurrencyHeap.list = nil
	for _, priceHeap := range h.currencyHeaps {
		priceHeap.list = nil
	}
}

func (h *multiCurrencyPriceHeap) UpdateFeesAndCurrencies(currencyCmpFn CurrencyCmpFn, gpm GasPriceMinimums) {
	h.currencyCmp = currencyCmpFn
	h.gpm = gpm
	h.nativeCurrencyHeap.baseFee = gpm.GetNativeGPM()
	for currencyAddr, heap := range h.currencyHeaps {
		heap.baseFee = gpm.GetGasPriceMinimum(&currencyAddr)
	}

}
