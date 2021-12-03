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

// Add tries to insert a new transaction into the list, returning whether the
// transaction was accepted, and if yes, any previous transaction it replaced.
//
// If the new transaction is accepted into the list, the lists' cost, gas and
// gasPriceMinimum thresholds are also potentially updated.
func (l *txList) Add(tx *types.Transaction, priceBump uint64) (bool, *types.Transaction) {
	// If there's an older better transaction, abort
	old := l.txs.Get(tx.Nonce())
	if old != nil {
		var oldPrice, newPrice *big.Int
		// Short circuit conversion if both are the same currency
		if old.FeeCurrency() == tx.FeeCurrency() {
			oldPrice = old.GasPrice()
			newPrice = tx.GasPrice()
		} else {
			ctx := l.ctx.Load().(txPoolContext)
			if fc := old.FeeCurrency(); fc != nil {
				currency, err := ctx.GetCurrency(fc)
				if err != nil {
					return false, nil
				}
				oldPrice = currency.ToCELO(old.GasPrice())
			} else {
				oldPrice = old.GasPrice()
			}
			if fc := tx.FeeCurrency(); fc != nil {
				currency, err := ctx.GetCurrency(fc)
				if err != nil {
					return false, nil
				}
				newPrice = currency.ToCELO(tx.GasPrice())
			} else {
				newPrice = tx.GasPrice()
			}
		}
		// threshold = oldGP * (100 + priceBump) / 100
		a := big.NewInt(100 + int64(priceBump))
		a = a.Mul(a, oldPrice)
		b := big.NewInt(100)
		threshold := a.Div(a, b)
		// Have to ensure that the new gas price is higher than the old gas
		// price as well as checking the percentage threshold to ensure that
		// this is accurate for low (Wei-level) gas price replacements
		if oldPrice.Cmp(newPrice) >= 0 || threshold.Cmp(newPrice) > 0 {
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
// price-sorted transactions to discard when the pool fills up.
type priceHeap []*types.Transaction

func (h priceHeap) Len() int      { return len(h) }
func (h priceHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }

func (h priceHeap) Less(i, j int) bool {
	// Sort primarily by price, returning the cheaper one
	switch h[i].GasPriceCmp(h[j]) {
	case -1:
		return true
	case 1:
		return false
	}
	// If the prices match, stabilize via nonces (high nonce is worse)
	return h[i].Nonce() > h[j].Nonce()
}

func (h *priceHeap) Push(x interface{}) {
	*h = append(*h, x.(*types.Transaction))
}

func (h *priceHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}

// txPricedList is a price-sorted heap to allow operating on transactions pool
// contents in a price-incrementing way.
type txPricedList struct {
	ctx                 *atomic.Value
	all                 *txLookup                     // Pointer to the map of all transactions
	nonNilCurrencyHeaps map[common.Address]*priceHeap // Heap of prices of all the stored non-nil currency transactions
	nilCurrencyHeap     *priceHeap                    // Heap of prices of all the stored nil currency transactions
	stales              int                           // Number of stale price points to (re-heap trigger)
}

// newTxPricedList creates a new price-sorted transaction heap.
func newTxPricedList(all *txLookup, ctx *atomic.Value) *txPricedList {
	return &txPricedList{
		ctx:                 ctx,
		all:                 all,
		nonNilCurrencyHeaps: make(map[common.Address]*priceHeap),
		nilCurrencyHeap:     new(priceHeap),
	}
}

// Gets the price heap for the given currency
func (l *txPricedList) getPriceHeap(tx *types.Transaction) *priceHeap {
	feeCurrency := tx.FeeCurrency()
	if feeCurrency == nil {
		return l.nilCurrencyHeap
	} else {
		if _, ok := l.nonNilCurrencyHeaps[*feeCurrency]; !ok {
			l.nonNilCurrencyHeaps[*feeCurrency] = new(priceHeap)
		}
		return l.nonNilCurrencyHeaps[*feeCurrency]
	}
}

// Put inserts a new transaction into the heap.
func (l *txPricedList) Put(tx *types.Transaction) {
	pHeap := l.getPriceHeap(tx)
	heap.Push(pHeap, tx)
}

// Removed notifies the prices transaction list that an old transaction dropped
// from the pool. The list will just keep a counter of stale objects and update
// the heap if a large enough ratio of transactions go stale.
func (l *txPricedList) Removed(count int) {
	// Bump the stale counter, but exit if still too low (< 25%)
	l.stales += count
	if l.stales <= l.Len()/4 {
		return
	}
	// Seems we've reached a critical number of stale transactions, reheap
	reheapNilCurrencyHeap := make(priceHeap, 0, l.all.nilCurrencyTxCurrCount)

	reheapNonNilCurrencyMap := make(map[common.Address]*priceHeap)
	for feeCurrency, count := range l.all.nonNilCurrencyTxCurrCount {
		reheapNonNilCurrencyHeap := make(priceHeap, 0, count)
		reheapNonNilCurrencyMap[feeCurrency] = &reheapNonNilCurrencyHeap
	}

	l.stales, l.nonNilCurrencyHeaps, l.nilCurrencyHeap = 0, reheapNonNilCurrencyMap, &reheapNilCurrencyHeap
	l.all.Range(func(hash common.Hash, tx *types.Transaction) bool {
		pHeap := l.getPriceHeap(tx)
		*pHeap = append(*pHeap, tx)
		return true
	})

	for _, h := range l.nonNilCurrencyHeaps {
		heap.Init(h)
	}

	heap.Init(l.nilCurrencyHeap)
}

// Cap finds all the transactions below the given celo gold price threshold, drops them
// from the priced list and returns them for further removal from the entire pool.
func (l *txPricedList) Cap(cgThreshold *big.Int, local *accountSet) types.Transactions {
	drop := make(types.Transactions, 0, 128) // Remote underpriced transactions to drop
	save := make(types.Transactions, 0, 64)  // Local underpriced transactions to keep

	for l.Len() > 0 {
		// Discard stale transactions if found during cleanup
		tx := l.pop()
		if l.all.Get(tx.Hash()) == nil {
			l.stales--
			continue
		}

		if ctx := l.ctx.Load().(txPoolContext); ctx.CmpValues(tx.GasPrice(), tx.FeeCurrency(), cgThreshold, nil) >= 0 {
			save = append(save, tx)
			break
		}

		// Non stale transaction found, discard unless local
		if local.containsTx(tx) {
			save = append(save, tx)
		} else {
			drop = append(drop, tx)
		}
	}
	for _, tx := range save {
		l.Put(tx)
	}
	return drop
}

// Underpriced checks whether a transaction is cheaper than (or as cheap as) the
// lowest priced transaction currently being tracked.
func (l *txPricedList) Underpriced(tx *types.Transaction, local *accountSet) bool {
	// Local transactions cannot be underpriced
	if local.containsTx(tx) {
		return false
	}
	// Discard stale price points if found at the heap start
	for l.Len() > 0 {
		head := l.getMinPricedTx()
		if l.all.Get(head.Hash()) == nil {
			l.stales--
			l.pop()
			continue
		}
		break
	}
	// Check if the transaction is underpriced or not
	if l.Len() == 0 {
		log.Error("Pricing query for empty pool") // This cannot happen, print to catch programming errors
		return false
	}

	cheapest := l.getMinPricedTx()
	ctx := l.ctx.Load().(txPoolContext)
	return ctx.CmpValues(cheapest.GasPrice(), cheapest.FeeCurrency(), tx.GasPrice(), tx.FeeCurrency()) >= 0
}

// getAllPriceHeaps returns a slice of all the price heaps for each currency
// plus the nil currency heap
func (l *txPricedList) getAllPriceHeaps() []*priceHeap {
	heaps := make([]*priceHeap, 0, len(l.nonNilCurrencyHeaps)+1)
	for _, h := range l.nonNilCurrencyHeaps {
		heaps = append(heaps, h)
	}
	heaps = append(heaps, l.nilCurrencyHeap)
	return heaps
}

// phsLen returns the sum of all of the heaps sizes
func phsLen(heaps []*priceHeap) int {
	len := 0
	for _, h := range heaps {
		len += h.Len()
	}
	return len
}

// Discard finds a number of most underpriced transactions, removes them from the
// priced list and returns them for further removal from the entire pool.
func (l *txPricedList) Discard(slots int, local *accountSet) types.Transactions {
	heaps := l.getAllPriceHeaps()
	totalItems := phsLen(heaps)
	// If we have some local accountset, those will not be discarded
	if !local.empty() {
		// In case the list is filled to the brim with 'local' txs, we do this
		// little check to avoid unpacking / repacking the heap later on, which
		// is very expensive
		discardable := 0
		for _, items := range heaps {
			for _, tx := range *items {
				if !local.containsTx(tx) {
					discardable++
				}
				if discardable >= slots {
					break
				}
			}
		}
		if slots > discardable {
			slots = discardable
		}
	}
	if slots == 0 {
		return nil
	}
	drop := make(types.Transactions, 0, slots)            // Remote underpriced transactions to drop
	save := make(types.Transactions, 0, totalItems-slots) // Local underpriced transactions to keep

	for l.Len() > 0 && slots > 0 {
		// Discard stale transactions if found during cleanup
		tx := l.pop()
		if l.all.Get(tx.Hash()) == nil {
			l.stales--
			continue
		}
		// Non stale transaction found, discard unless local
		if local.containsTx(tx) {
			save = append(save, tx)
		} else {
			drop = append(drop, tx)
			slots -= numSlots(tx)
		}
	}
	for _, tx := range save {
		l.Put(tx)
	}
	return drop
}

// Retrieves the heap with the lowest normalized price at it's head
func (l *txPricedList) getHeapWithMinHead() (*priceHeap, *types.Transaction) {
	// Initialize it to the nilCurrencyHeap
	var cheapestHeap *priceHeap
	var cheapestTxn *types.Transaction

	if len(*l.nilCurrencyHeap) > 0 {
		cheapestHeap = l.nilCurrencyHeap
		cheapestTxn = []*types.Transaction(*l.nilCurrencyHeap)[0]
	}

	ctx := l.ctx.Load().(txPoolContext)
	for _, priceHeap := range l.nonNilCurrencyHeaps {
		if len(*priceHeap) > 0 {
			if cheapestHeap == nil {
				cheapestHeap = priceHeap
				cheapestTxn = []*types.Transaction(*cheapestHeap)[0]
			} else {
				txn := []*types.Transaction(*priceHeap)[0]
				if ctx.CmpValues(txn.GasPrice(), txn.FeeCurrency(), cheapestTxn.GasPrice(), cheapestTxn.FeeCurrency()) < 0 {
					cheapestHeap = priceHeap
				}
			}
		}
	}

	return cheapestHeap, cheapestTxn
}

// Retrieves the tx with the lowest normalized price among all the heaps
func (l *txPricedList) getMinPricedTx() *types.Transaction {
	_, minTx := l.getHeapWithMinHead()

	return minTx
}

// Retrieves the total number of txns within the priced list
func (l *txPricedList) Len() int {
	totalLen := len(*l.nilCurrencyHeap)
	for _, h := range l.nonNilCurrencyHeaps {
		totalLen += len(*h)
	}

	return totalLen
}

// Pops the tx with the lowest normalized price.
func (l *txPricedList) pop() *types.Transaction {
	cheapestHeap, _ := l.getHeapWithMinHead()

	if cheapestHeap != nil {
		return heap.Pop(cheapestHeap).(*types.Transaction)
	} else {
		return nil
	}
}
