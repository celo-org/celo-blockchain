package core

import (
	"container/heap"
	"math/big"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/core/types"
)

// multiCurrencyPriceHeap is a heap.Interface implementation over transactions
// with different fee currencies for retrieving price-sorted transactions to discard
// when the pool fills up. If baseFee is set then the heap is sorted based on the
// effective tip based on the given base fee. If baseFee is nil then the sorting
// is based on gasFeeCap.
type multiCurrencyPriceHeap struct {
	currencyCmpFn       func(*big.Int, *common.Address, *big.Int, *common.Address) int
	baseFeeFn           func(*common.Address) *big.Int // heap should always be re-sorted after baseFee is changed
	nonNilCurrencyHeaps map[common.Address]*priceHeap  // Heap of prices of all the stored non-nil currency transactions
	nilCurrencyHeap     *priceHeap                     // Heap of prices of all the stored nil currency transactions
}

// getHeapFor returns the proper heap for the given transaction, and creates it
// if it's not available in the nonNilCurrencyHeaps
func (h *multiCurrencyPriceHeap) getHeapFor(tx *types.Transaction) *priceHeap {
	fc := tx.FeeCurrency()
	if fc == nil {
		return h.nilCurrencyHeap
	}
	if _, ok := h.nonNilCurrencyHeaps[*fc]; !ok {
		h.nonNilCurrencyHeaps[*fc] = &priceHeap{
			baseFee: h.baseFeeFn(fc),
		}
	}
	return h.nonNilCurrencyHeaps[*fc]
}

// Add to the heap. Must call Init afterwards to retain the heap invariants.
func (h *multiCurrencyPriceHeap) Add(tx *types.Transaction) {
	ph := h.getHeapFor(tx)
	ph.list = append(ph.list, tx)
}

// Push to the heap, maintains heap invariants.
func (h *multiCurrencyPriceHeap) Push(tx *types.Transaction) {
	ph := h.getHeapFor(tx)
	ph.Push(tx)
}

func (h *multiCurrencyPriceHeap) cheapestTxs() []*types.Transaction {
	txs := make([]*types.Transaction, 0, 1+len(h.nonNilCurrencyHeaps))
	if len(h.nilCurrencyHeap.list) > 0 {
		txs = append(txs, h.nilCurrencyHeap.list[0])
	}
	for _, ph := range h.nonNilCurrencyHeaps {
		if len(ph.list) > 0 {
			txs = append(txs, ph.list[0])
		}
	}
	return txs
}

func (h *multiCurrencyPriceHeap) isCheaper(tx1, tx2 *types.Transaction) bool {
	return h.currencyCmpFn(tx1.GasPrice(), tx1.FeeCurrency(), tx2.GasPrice(), tx2.FeeCurrency()) < 0
}

func (h *multiCurrencyPriceHeap) cheapestTx() *types.Transaction {
	txs := h.cheapestTxs()
	var cheapestTx *types.Transaction
	for _, tx := range txs {
		if cheapestTx == nil || h.isCheaper(tx, cheapestTx) {
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
	r := len(h.nilCurrencyHeap.list)
	for _, priceHeap := range h.nonNilCurrencyHeaps {
		r += len(priceHeap.list)
	}
	return r
}

func (h *multiCurrencyPriceHeap) Init() {
	heap.Init(h.nilCurrencyHeap)
	for _, priceHeap := range h.nonNilCurrencyHeaps {
		heap.Init(priceHeap)
	}
}

func (h *multiCurrencyPriceHeap) Clear() {
	h.nilCurrencyHeap.list = nil
	for _, priceHeap := range h.nonNilCurrencyHeaps {
		priceHeap.list = nil
	}
}

func (h *multiCurrencyPriceHeap) SetBaseFee(txCtx *txPoolContext) {
	h.currencyCmpFn = txCtx.CmpValues
	h.baseFeeFn = txCtx.GetGasPriceMinimum
	h.nilCurrencyHeap.baseFee = txCtx.GetGasPriceMinimum(nil)
	for currencyAddr, heap := range h.nonNilCurrencyHeaps {
		heap.baseFee = txCtx.GetGasPriceMinimum(&currencyAddr)
	}

}
