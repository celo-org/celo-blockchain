// Copyright 2016 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package core

import (
	"container/heap"
	"math"
	"math/big"
	"sort"
	"sync/atomic"
	"time"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/core/types"
	"github.com/celo-org/celo-blockchain/log"
)

// nonceHeap is a heap.Interface implementation over 64bit unsigned integers for
// retrieving sorted transactions from the possibly gapped future queue.
type nonceHeap []uint64

func (h nonceHeap) Len() int           { return len(h) }
func (h nonceHeap) Less(i, j int) bool { return h[i] < h[j] }
func (h nonceHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }

func (h *nonceHeap) Push(x interface{}) {
	*h = append(*h, x.(uint64))
}

func (h *nonceHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}

// txSortedMap is a nonce->transaction hash map with a heap based index to allow
// iterating over the contents in a nonce-incrementing way.
type txSortedMap struct {
	items map[uint64]*types.Transaction // Hash map storing the transaction data
	index *nonceHeap                    // Heap of nonces of all the stored transactions (non-strict mode)
	cache types.Transactions            // Cache of the transactions already sorted
}

// newTxSortedMap creates a new nonce-sorted transaction map.
func newTxSortedMap() *txSortedMap {
	return &txSortedMap{
		items: make(map[uint64]*types.Transaction),
		index: new(nonceHeap),
	}
}

// Get retrieves the current transactions associated with the given nonce.
func (m *txSortedMap) Get(nonce uint64) *types.Transaction {
	return m.items[nonce]
}

// Put inserts a new transaction into the map, also updating the map's nonce
// index. If a transaction already exists with the same nonce, it's overwritten.
func (m *txSortedMap) Put(tx *types.Transaction) {
	nonce := tx.Nonce()
	if m.items[nonce] == nil {
		heap.Push(m.index, nonce)
	}
	m.items[nonce], m.cache = tx, nil
}

// Forward removes all transactions from the map with a nonce lower than the
// provided threshold. Every removed transaction is returned for any post-removal
// maintenance.
func (m *txSortedMap) Forward(threshold uint64) types.Transactions {
	var removed types.Transactions

	// Pop off heap items until the threshold is reached
	for m.index.Len() > 0 && (*m.index)[0] < threshold {
		nonce := heap.Pop(m.index).(uint64)
		removed = append(removed, m.items[nonce])
		delete(m.items, nonce)
	}
	// If we had a cached order, shift the front
	if m.cache != nil {
		m.cache = m.cache[len(removed):]
	}
	return removed
}

// Filter iterates over the list of transactions and removes all of them for which
// the specified function evaluates to true.
// Filter, as opposed to 'filter', re-initialises the heap after the operation is done.
// If you want to do several consecutive filterings, it's therefore better to first
// do a .filter(func1) followed by .Filter(func2) or reheap()
func (m *txSortedMap) Filter(filter func(*types.Transaction) bool) types.Transactions {
	removed := m.filter(filter)
	// If transactions were removed, the heap and cache are ruined
	if len(removed) > 0 {
		m.reheap()
	}
	return removed
}

func (m *txSortedMap) reheap() {
	*m.index = make([]uint64, 0, len(m.items))
	for nonce := range m.items {
		*m.index = append(*m.index, nonce)
	}
	heap.Init(m.index)
	m.cache = nil
}

// filter is identical to Filter, but **does not** regenerate the heap. This method
// should only be used if followed immediately by a call to Filter or reheap()
func (m *txSortedMap) filter(filter func(*types.Transaction) bool) types.Transactions {
	var removed types.Transactions

	// Collect all the transactions to filter out
	for nonce, tx := range m.items {
		if filter(tx) {
			removed = append(removed, tx)
			delete(m.items, nonce)
		}
	}
	if len(removed) > 0 {
		m.cache = nil
	}
	return removed
}

// Cap places a hard limit on the number of items, returning all transactions
// exceeding that limit.
func (m *txSortedMap) Cap(threshold int) types.Transactions {
	// Short circuit if the number of items is under the limit
	if len(m.items) <= threshold {
		return nil
	}
	// Otherwise gather and drop the highest nonce'd transactions
	var drops types.Transactions

	sort.Sort(*m.index)
	for size := len(m.items); size > threshold; size-- {
		drops = append(drops, m.items[(*m.index)[size-1]])
		delete(m.items, (*m.index)[size-1])
	}
	*m.index = (*m.index)[:threshold]
	heap.Init(m.index)

	// If we had a cache, shift the back
	if m.cache != nil {
		m.cache = m.cache[:len(m.cache)-len(drops)]
	}
	return drops
}

// Remove deletes a transaction from the maintained map, returning whether the
// transaction was found.
func (m *txSortedMap) Remove(nonce uint64) bool {
	// Short circuit if no transaction is present
	_, ok := m.items[nonce]
	if !ok {
		return false
	}
	// Otherwise delete the transaction and fix the heap index
	for i := 0; i < m.index.Len(); i++ {
		if (*m.index)[i] == nonce {
			heap.Remove(m.index, i)
			break
		}
	}
	delete(m.items, nonce)
	m.cache = nil

	return true
}

// Ready retrieves a sequentially increasing list of transactions starting at the
// provided nonce that is ready for processing. The returned transactions will be
// removed from the list.
//
// Note, all transactions with nonces lower than start will also be returned to
// prevent getting into and invalid state. This is not something that should ever
// happen but better to be self correcting than failing!
func (m *txSortedMap) Ready(start uint64) types.Transactions {
	// Short circuit if no transactions are available
	if m.index.Len() == 0 || (*m.index)[0] > start {
		return nil
	}
	// Otherwise start accumulating incremental transactions
	var ready types.Transactions
	for next := (*m.index)[0]; m.index.Len() > 0 && (*m.index)[0] == next; next++ {
		ready = append(ready, m.items[next])
		delete(m.items, next)
		heap.Pop(m.index)
	}
	m.cache = nil

	return ready
}

// Len returns the length of the transaction map.
func (m *txSortedMap) Len() int {
	return len(m.items)
}

func (m *txSortedMap) flatten() types.Transactions {
	// If the sorting was not cached yet, create and cache it
	if m.cache == nil {
		m.cache = make(types.Transactions, 0, len(m.items))
		for _, tx := range m.items {
			m.cache = append(m.cache, tx)
		}
		sort.Sort(types.TxByNonce(m.cache))
	}
	return m.cache
}

// Flatten creates a nonce-sorted slice of transactions based on the loosely
// sorted internal representation. The result of the sorting is cached in case
// it's requested again before any modifications are made to the contents.
func (m *txSortedMap) Flatten() types.Transactions {
	// Copy the cache to prevent accidental modifications
	cache := m.flatten()
	txs := make(types.Transactions, len(cache))
	copy(txs, cache)
	return txs
}

// LastElement returns the last element of a flattened list, thus, the
// transaction with the highest nonce
func (m *txSortedMap) LastElement() *types.Transaction {
	cache := m.flatten()
	return cache[len(cache)-1]
}

// txList is a "list" of transactions belonging to an account, sorted by account
// nonce. The same type can be used both for storing contiguous transactions for
// the executable/pending queue; and for storing gapped transactions for the non-
// executable/future queue, with minor behavioral changes.
type txList struct {
	strict bool         // Whether nonces are strictly continuous or not
	txs    *txSortedMap // Heap indexed sorted hash map of the transactions

	nativecostcap       *big.Int                    // Price of the highest costing transaction paid with native fees (reset only if exceeds balance)
	feecaps             map[common.Address]*big.Int // Price of the highest costing transaction per fee currency (reset only if exceeds balance)
	nativegaspricefloor *big.Int                    // Lowest gas price minimum in the native currency
	gaspricefloors      map[common.Address]*big.Int // Lowest gas price minimum per currency (reset only if it is below the gpm)
	gascap              uint64                      // Gas limit of the highest spending transaction (reset only if exceeds block limit)

	ctx *atomic.Value // transaction pool context
}

// newTxList create a new transaction list for maintaining nonce-indexable fast,
// gapped, sortable transaction lists.
func newTxList(strict bool, ctx *atomic.Value) *txList {
	return &txList{
		ctx:                 ctx,
		strict:              strict,
		txs:                 newTxSortedMap(),
		nativecostcap:       new(big.Int),
		feecaps:             make(map[common.Address]*big.Int),
		nativegaspricefloor: nil,
		gaspricefloors:      make(map[common.Address]*big.Int),
	}
}

// Overlaps returns whether the transaction specified has the same nonce as one
// already contained within the list.
func (l *txList) Overlaps(tx *types.Transaction) bool {
	return l.txs.Get(tx.Nonce()) != nil
}

// FeeCurrencies returns a list of each fee currency used to pay for gas in the txList
func (l *txList) FeeCurrencies() []common.Address {
	var feeCurrencies []common.Address
	for feeCurrency := range l.feecaps {
		feeCurrencies = append(feeCurrencies, feeCurrency)
	}
	return feeCurrencies
}

func toCELO(amount *big.Int, feeCurrency *common.Address, txCtx *txPoolContext) (*big.Int, error) {
	if feeCurrency == nil {
		return amount, nil
	}
	currency, err := txCtx.GetCurrency(feeCurrency)
	if err != nil {
		return nil, err
	}
	return currency.ToCELO(amount), nil
}

// Add tries to insert a new transaction into the list, returning whether the
// transaction was accepted, and if yes, any previous transaction it replaced.
//
// If the new transaction is accepted into the list, the lists' cost, gas and
// gasPriceMinimum thresholds are also potentially updated.
func (l *txList) Add(tx *types.Transaction, priceBump uint64) (bool, *types.Transaction) {
	// If there's an older better transaction, abort
	old := l.txs.Get(tx.Nonce())
	if old != nil {
		var oldGasFeeCap, oldGasTipCap *big.Int
		var newGasFeeCap, newGasTipCap *big.Int

		// Short circuit conversion if both are the same currency
		if old.FeeCurrency() == tx.FeeCurrency() {
			if old.GasFeeCapCmp(tx) >= 0 || old.GasTipCapCmp(tx) >= 0 {
				return false, nil
			}
			oldGasFeeCap = old.GasFeeCap()
			oldGasTipCap = old.GasTipCap()
			newGasFeeCap = tx.GasFeeCap()
			newGasTipCap = tx.GasTipCap()
		} else {
			// Convert old values into tx fee currency
			var err error
			txCtx := l.ctx.Load().(txPoolContext)
			if oldGasFeeCap, err = toCELO(old.GasFeeCap(), old.FeeCurrency(), &txCtx); err != nil {
				return false, nil
			}
			if oldGasTipCap, err = toCELO(old.GasTipCap(), old.FeeCurrency(), &txCtx); err != nil {
				return false, nil
			}
			if newGasFeeCap, err = toCELO(tx.GasFeeCap(), tx.FeeCurrency(), &txCtx); err != nil {
				return false, nil
			}
			if newGasTipCap, err = toCELO(tx.GasTipCap(), tx.FeeCurrency(), &txCtx); err != nil {
				return false, nil
			}

		}
		// thresholdFeeCap = oldFC  * (100 + priceBump) / 100
		a := big.NewInt(100 + int64(priceBump))
		aFeeCap := new(big.Int).Mul(a, oldGasFeeCap)
		aTip := a.Mul(a, oldGasTipCap)

		// thresholdTip    = oldTip * (100 + priceBump) / 100
		b := big.NewInt(100)
		thresholdFeeCap := aFeeCap.Div(aFeeCap, b)
		thresholdTip := aTip.Div(aTip, b)

		// Have to ensure that either the new fee cap or tip is higher than the
		// old ones as well as checking the percentage threshold to ensure that
		// this is accurate for low (Wei-level) gas price replacements
		if newGasFeeCap.Cmp(thresholdFeeCap) < 0 || newGasTipCap.Cmp(thresholdTip) < 0 {
			return false, nil
		}
	}
	// Otherwise overwrite the old transaction with the current one
	// caps can only increase and floors can only decrease in this function
	l.txs.Put(tx)
	if feeCurrency := tx.FeeCurrency(); feeCurrency == nil {
		if cost := tx.Cost(); l.nativecostcap.Cmp(cost) < 0 {
			l.nativecostcap = cost
		}
		if gasPrice := tx.GasPrice(); l.nativegaspricefloor == nil || l.nativegaspricefloor.Cmp(gasPrice) > 0 {
			l.nativegaspricefloor = gasPrice
		}
	} else {
		fee := tx.Fee()
		if oldFee, ok := l.feecaps[*feeCurrency]; !ok || oldFee.Cmp(fee) < 0 {
			l.feecaps[*feeCurrency] = fee
		}
		if gasFloor, ok := l.gaspricefloors[*feeCurrency]; !ok || gasFloor.Cmp(tx.GasPrice()) > 0 {
			l.gaspricefloors[*feeCurrency] = tx.GasPrice()
		}
		if value := tx.Value(); l.nativecostcap.Cmp(value) < 0 {
			l.nativecostcap = value
		}
	}
	if gas := tx.Gas(); l.gascap < gas {
		l.gascap = gas
	}
	return true, old
}

// Forward removes all transactions from the list with a nonce lower than the
// provided threshold. Every removed transaction is returned for any post-removal
// maintenance.
func (l *txList) Forward(threshold uint64) types.Transactions {
	return l.txs.Forward(threshold)
}

// Filter removes all transactions from the list with a cost or gas limit higher
// than the provided thresholds. Every removed transaction is returned for any
// post-removal maintenance. Strict-mode invalidated transactions are also
// returned.
//
// This method uses the cached costcap and gascap to quickly decide if there's even
// a point in calculating all the costs or if the balance covers all. If the threshold
// is lower than the costgas cap, the caps will be reset to a new high after removing
func (l *txList) Filter(nativeCostLimit *big.Int, feeLimits map[common.Address]*big.Int, gasLimit uint64) (types.Transactions, types.Transactions) {

	// check if we can bail & lower caps & raise floors at the same time
	canBail := true
	// Ensure that the cost cap <= the cost limit
	if l.nativecostcap.Cmp(nativeCostLimit) > 0 {
		canBail = false
		l.nativecostcap = new(big.Int).Set(nativeCostLimit)
	}

	// Ensure that the gas cap <= the gas limit
	if l.gascap > gasLimit {
		canBail = false
		l.gascap = gasLimit
	}
	// Ensure that each cost cap <= the per currency cost limit.
	for feeCurrency, feeLimit := range feeLimits {
		if l.feecaps[feeCurrency].Cmp(feeLimit) > 0 {
			canBail = false
			l.feecaps[feeCurrency] = new(big.Int).Set(feeLimit)
		}
	}

	if canBail {
		return nil, nil
	}
	txCtx := l.ctx.Load().(txPoolContext)
	// Filter out all the transactions above the account's funds
	removed := l.txs.Filter(func(tx *types.Transaction) bool {
		if feeCurrency := tx.FeeCurrency(); feeCurrency == nil {
			log.Trace("Transaction Filter", "hash", tx.Hash(), "Fee currency", tx.FeeCurrency(), "Cost", tx.Cost(), "Cost Limit", nativeCostLimit, "Gas", tx.Gas(), "Gas Limit", gasLimit)
			return tx.Cost().Cmp(nativeCostLimit) > 0 || tx.Gas() > gasLimit || txCtx.celoGasPriceMinimumFloor.Cmp(tx.GasPrice()) > 0
		} else {
			feeLimit := feeLimits[*feeCurrency]
			fee := tx.Fee()
			log.Trace("Transaction Filter", "hash", tx.Hash(), "Fee currency", tx.FeeCurrency(), "Value", tx.Value(), "Cost Limit", feeLimit, "Gas", tx.Gas(), "Gas Limit", gasLimit)

			// If any of the following is true, the transaction is invalid
			// The fees are greater than or equal to the balance in the currency
			return fee.Cmp(feeLimit) >= 0 ||
				// The value of the tx is greater than the native balance of the account
				tx.Value().Cmp(nativeCostLimit) > 0 ||
				// The gas price is smaller that the gasPriceMinimumFloor
				txCtx.CmpValues(txCtx.celoGasPriceMinimumFloor, nil, tx.GasPrice(), tx.FeeCurrency()) > 0 ||
				// The gas used is greater than the gas limit
				tx.Gas() > gasLimit
		}
	})

	// If the list was strict, filter anything above the lowest nonce
	var invalids types.Transactions

	if l.strict && len(removed) > 0 {
		lowest := uint64(math.MaxUint64)
		for _, tx := range removed {
			if nonce := tx.Nonce(); lowest > nonce {
				lowest = nonce
			}
		}
		invalids = l.txs.Filter(func(tx *types.Transaction) bool { return tx.Nonce() > lowest })
	}
	return removed, invalids
}

// FilterOnGasLimit removes all transactions from the list with a gas limit higher
// than the provided thresholds. Every removed transaction is returned for any
// post-removal maintenance. Strict-mode invalidated transactions are also
// returned.
//
// This method uses the cached gascap to quickly decide if there's even
// a point in calculating all the gas used
func (l *txList) FilterOnGasLimit(gasLimit uint64) (types.Transactions, types.Transactions) {
	// We can bail if the gas cap <= the gas limit
	if l.gascap <= gasLimit {
		return nil, nil
	}
	l.gascap = gasLimit

	// Filter out all the transactions above the account's funds
	removed := l.txs.Filter(func(tx *types.Transaction) bool {
		return tx.Gas() > gasLimit
	})

	if len(removed) == 0 {
		return nil, nil
	}
	var invalids types.Transactions
	// If the list was strict, filter anything above the lowest nonce
	if l.strict {
		lowest := uint64(math.MaxUint64)
		for _, tx := range removed {
			if nonce := tx.Nonce(); lowest > nonce {
				lowest = nonce
			}
		}
		invalids = l.txs.filter(func(tx *types.Transaction) bool { return tx.Nonce() > lowest })
	}
	l.txs.reheap()
	return removed, invalids
}

// Cap places a hard limit on the number of items, returning all transactions
// exceeding that limit.
func (l *txList) Cap(threshold int) types.Transactions {
	return l.txs.Cap(threshold)
}

// Remove deletes a transaction from the maintained list, returning whether the
// transaction was found, and also returning any transaction invalidated due to
// the deletion (strict mode only).
func (l *txList) Remove(tx *types.Transaction) (bool, types.Transactions) {
	// Remove the transaction from the set
	nonce := tx.Nonce()
	if removed := l.txs.Remove(nonce); !removed {
		return false, nil
	}
	// In strict mode, filter out non-executable transactions
	if l.strict {
		return true, l.txs.Filter(func(tx *types.Transaction) bool { return tx.Nonce() > nonce })
	}
	return true, nil
}

// Ready retrieves a sequentially increasing list of transactions starting at the
// provided nonce that is ready for processing. The returned transactions will be
// removed from the list.
//
// Note, all transactions with nonces lower than start will also be returned to
// prevent getting into and invalid state. This is not something that should ever
// happen but better to be self correcting than failing!
func (l *txList) Ready(start uint64) types.Transactions {
	return l.txs.Ready(start)
}

// Len returns the length of the transaction list.
func (l *txList) Len() int {
	return l.txs.Len()
}

// Empty returns whether the list of transactions is empty or not.
func (l *txList) Empty() bool {
	return l.Len() == 0
}

// Flatten creates a nonce-sorted slice of transactions based on the loosely
// sorted internal representation. The result of the sorting is cached in case
// it's requested again before any modifications are made to the contents.
func (l *txList) Flatten() types.Transactions {
	return l.txs.Flatten()
}

// LastElement returns the last element of a flattened list, thus, the
// transaction with the highest nonce
func (l *txList) LastElement() *types.Transaction {
	return l.txs.LastElement()
}

// priceHeap is a heap.Interface implementation over transactions for retrieving
// price-sorted transactions to discard when the pool fills up. If baseFee is set
// then the heap is sorted based on the effective tip based on the given base fee.
// If baseFee is nil then the sorting is based on gasFeeCap.
// The fee currencies for each transaction should be the same.
type priceHeap struct {
	baseFee *big.Int // heap should always be re-sorted after baseFee is changed
	list    []*types.Transaction
}

func (h *priceHeap) Len() int      { return len(h.list) }
func (h *priceHeap) Swap(i, j int) { h.list[i], h.list[j] = h.list[j], h.list[i] }

func (h *priceHeap) Less(i, j int) bool {
	switch h.cmp(h.list[i], h.list[j]) {
	case -1:
		return true
	case 1:
		return false
	default:
		return h.list[i].Nonce() > h.list[j].Nonce()
	}
}

func (h *priceHeap) cmp(a, b *types.Transaction) int {
	if h.baseFee != nil {
		// Compare effective tips if baseFee is specified
		if c := a.EffectiveGasTipCmp(b, h.baseFee); c != 0 {
			return c
		}
	}
	// Compare fee caps if baseFee is not specified or effective tips are equal
	if c := a.GasFeeCapCmp(b); c != 0 {
		return c
	}
	// Compare tips if effective tips and fee caps are equal
	return a.GasTipCapCmp(b)
}

func (h *priceHeap) Push(x interface{}) {
	tx := x.(*types.Transaction)
	h.list = append(h.list, tx)
}

func (h *priceHeap) Pop() interface{} {
	old := h.list
	n := len(old)
	x := old[n-1]
	old[n-1] = nil
	h.list = old[0 : n-1]
	return x
}

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

// Add to the heap. Must call Init afterwards to retain the heap invariants.
func (h *multiCurrencyPriceHeap) Add(tx *types.Transaction) {
	if fc := tx.FeeCurrency(); fc == nil {
		h.nilCurrencyHeap.list = append(h.nilCurrencyHeap.list, tx)
	} else {
		if _, ok := h.nonNilCurrencyHeaps[*fc]; !ok {
			h.nonNilCurrencyHeaps[*fc] = &priceHeap{
				baseFee: h.baseFeeFn(fc),
			}

		}
		sh := h.nonNilCurrencyHeaps[*fc]
		sh.list = append(sh.list, tx)
	}
}

func (h *multiCurrencyPriceHeap) Push(tx *types.Transaction) {
	if fc := tx.FeeCurrency(); fc == nil {
		h.nilCurrencyHeap.Push(tx)
	} else {
		if _, ok := h.nonNilCurrencyHeaps[*fc]; !ok {
			h.nonNilCurrencyHeaps[*fc] = &priceHeap{
				baseFee: h.baseFeeFn(fc),
			}

		}
		sh := h.nonNilCurrencyHeaps[*fc]
		sh.Push(tx)
	}
}

func (h *multiCurrencyPriceHeap) Pop() *types.Transaction {
	var cheapestHeap *priceHeap
	var cheapestTxn *types.Transaction

	if len(h.nilCurrencyHeap.list) > 0 {
		cheapestHeap = h.nilCurrencyHeap
		cheapestTxn = h.nilCurrencyHeap.list[0]
	}

	for _, priceHeap := range h.nonNilCurrencyHeaps {
		if len(priceHeap.list) > 0 {
			if cheapestHeap == nil {
				cheapestHeap = priceHeap
				cheapestTxn = cheapestHeap.list[0]
			} else {
				txn := priceHeap.list[0]
				if h.currencyCmpFn(txn.GasPrice(), txn.FeeCurrency(), cheapestTxn.GasPrice(), cheapestTxn.FeeCurrency()) < 0 {
					cheapestHeap = priceHeap
				}
			}
		}
	}

	if cheapestHeap != nil {
		return heap.Pop(cheapestHeap).(*types.Transaction)
	}
	return nil

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

// txPricedList is a price-sorted heap to allow operating on transactions pool
// contents in a price-incrementing way. It's built opon the all transactions
// in txpool but only interested in the remote part. It means only remote transactions
// will be considered for tracking, sorting, eviction, etc.
//
// Two heaps are used for sorting: the urgent heap (based on effective tip in the next
// block) and the floating heap (based on gasFeeCap). Always the bigger heap is chosen for
// eviction. Transactions evicted from the urgent heap are first demoted into the floating heap.
// In some cases (during a congestion, when blocks are full) the urgent heap can provide
// better candidates for inclusion while in other cases (at the top of the baseFee peak)
// the floating heap is better. When baseFee is decreasing they behave similarly.
type txPricedList struct {
	ctx              *atomic.Value
	all              *txLookup              // Pointer to the map of all transactions
	urgent, floating multiCurrencyPriceHeap // Heaps of prices of all the stored **remote** transactions
	stales           int                    // Number of stale price points to (re-heap trigger)

}

const (
	// urgentRatio : floatingRatio is the capacity ratio of the two queues
	urgentRatio   = 4
	floatingRatio = 1
)

// newTxPricedList creates a new price-sorted transaction heap.
func newTxPricedList(all *txLookup, ctx *atomic.Value) *txPricedList {
	txCtx := ctx.Load().(txPoolContext)
	return &txPricedList{
		ctx: ctx,
		all: all,
		urgent: multiCurrencyPriceHeap{
			currencyCmpFn:       txCtx.CmpValues,
			nilCurrencyHeap:     &priceHeap{},
			nonNilCurrencyHeaps: make(map[common.Address]*priceHeap),
			baseFeeFn:           txCtx.GetGasPriceMinimum,
		},
		floating: multiCurrencyPriceHeap{
			currencyCmpFn:       txCtx.CmpValues,
			nilCurrencyHeap:     &priceHeap{},
			nonNilCurrencyHeaps: make(map[common.Address]*priceHeap),
			baseFeeFn:           txCtx.GetGasPriceMinimum,
		},
	}
}

// Put inserts a new transaction into the heap.
func (l *txPricedList) Put(tx *types.Transaction, local bool) {
	if local {
		return
	}
	// Insert every new transaction to the urgent heap first; Discard will balance the heaps
	l.urgent.Push(tx)
}

// Removed notifies the prices transaction list that an old transaction dropped
// from the pool. The list will just keep a counter of stale objects and update
// the heap if a large enough ratio of transactions go stale.
func (l *txPricedList) Removed(count int) {
	// Bump the stale counter, but exit if still too low (< 25%)
	l.stales += count
	if l.stales <= (l.urgent.Len() + l.floating.Len()/4) {
		return
	}
	// Seems we've reached a critical number of stale transactions, reheap
	l.Reheap()
}

// Underpriced checks whether a transaction is cheaper than (or as cheap as) the
// lowest priced (remote) transaction currently being tracked.
func (l *txPricedList) Underpriced(tx *types.Transaction) bool {
	// Note: with two queues, being underpriced is defined as being worse than the worst item
	// in all non-empty queues if there is any. If both queues are empty then nothing is underpriced.
	urgentUnderpriced := l.underpricedForMulti(&l.urgent, tx)
	floatingUnderpriced := l.underpricedForMulti(&l.floating, tx)
	return (urgentUnderpriced || l.urgent.Len() == 0) &&
		(floatingUnderpriced || l.floating.Len() == 0) &&
		(l.urgent.Len() != 0 || l.floating.Len() != 0)
}

func (l *txPricedList) underpricedForMulti(h *multiCurrencyPriceHeap, tx *types.Transaction) bool {
	underpriced := l.underpricedFor(h.nilCurrencyHeap, tx)
	for _, sh := range h.nonNilCurrencyHeaps {
		if l.underpricedFor(sh, tx) {
			underpriced = true
		}
	}
	return underpriced
}

// underpricedFor checks whether a transaction is cheaper than (or as cheap as) the
// lowest priced (remote) transaction in the given heap.
func (l *txPricedList) underpricedFor(h *priceHeap, tx *types.Transaction) bool {
	// Discard stale price points if found at the heap start
	for len(h.list) > 0 {
		head := h.list[0]
		if l.all.GetRemote(head.Hash()) == nil { // Removed or migrated
			l.stales--
			heap.Pop(h)
			continue
		}
		break
	}
	// Check if the transaction is underpriced or not
	if len(h.list) == 0 {
		return false // There is no remote transaction at all.
	}
	// If the remote transaction is even cheaper than the
	// cheapest one tracked locally, reject it.
	return h.cmp(h.list[0], tx) >= 0
}

// Discard finds a number of most underpriced transactions, removes them from the
// priced list and returns them for further removal from the entire pool.
//
// Note local transaction won't be considered for eviction.
func (l *txPricedList) Discard(slots int, force bool) (types.Transactions, bool) {
	drop := make(types.Transactions, 0, slots) // Remote underpriced transactions to drop
	for slots > 0 {
		if l.urgent.Len()*floatingRatio > l.floating.Len()*urgentRatio || floatingRatio == 0 {
			// Discard stale transactions if found during cleanup
			tx := l.urgent.Pop()
			if l.all.GetRemote(tx.Hash()) == nil { // Removed or migrated
				l.stales--
				continue
			}
			// Non stale transaction found, move to floating heap
			l.floating.Push(tx)
		} else {
			if l.floating.Len() == 0 {
				// Stop if both heaps are empty
				break
			}
			// Discard stale transactions if found during cleanup
			tx := l.floating.Pop()
			if l.all.GetRemote(tx.Hash()) == nil { // Removed or migrated
				l.stales--
				continue
			}
			// Non stale transaction found, discard it
			drop = append(drop, tx)
			slots -= numSlots(tx)
		}
	}
	// If we still can't make enough room for the new transaction
	if slots > 0 && !force {
		for _, tx := range drop {
			l.urgent.Push(tx)
		}
		return nil, false
	}
	return drop, true
}

// Reheap forcibly rebuilds the heap based on the current remote transaction set.
func (l *txPricedList) Reheap() {
	start := time.Now()
	l.stales = 0
	l.urgent.Clear()
	l.all.Range(func(hash common.Hash, tx *types.Transaction, local bool) bool {
		l.urgent.Add(tx)
		return true
	}, false, true) // Only iterate remotes
	l.urgent.Init()

	// balance out the two heaps by moving the worse half of transactions into the
	// floating heap
	// Note: Discard would also do this before the first eviction but Reheap can do
	// is more efficiently. Also, Underpriced would work suboptimally the first time
	// if the floating queue was empty.
	floatingCount := l.urgent.Len() * floatingRatio / (urgentRatio + floatingRatio)
	l.floating.Clear()
	for i := 0; i < floatingCount; i++ {
		l.floating.Add(l.urgent.Pop())
	}
	l.floating.Init()
	reheapTimer.Update(time.Since(start))
}

// SetBaseFee updates the base fee and triggers a re-heap. Note that Removed is not
// necessary to call right before SetBaseFee when processing a new block.
func (l *txPricedList) SetBaseFee(txCtx *txPoolContext) {
	l.urgent.SetBaseFee(txCtx)
	l.Reheap()
}
