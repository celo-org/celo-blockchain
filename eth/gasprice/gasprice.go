// Copyright 2015 The go-ethereum Authors
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

package gasprice

import (
	"context"
	"math/big"
	"sort"
	"sync"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/internal/ethapi"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/rpc"
)

var maxPrice = big.NewInt(500 * params.GWei)

type Config struct {
	Blocks     int
	Percentile int
	Default    *big.Int `toml:",omitempty"`
	AlwaysZero bool
}

// Oracle recommends gas prices based on the content of recent
// blocks. Suitable for both light and full clients.
type Oracle struct {
	backend   ethapi.Backend
	lastHead  common.Hash
	lastPrice *big.Int
	cacheLock sync.RWMutex
	fetchLock sync.Mutex

	checkBlocks, maxEmpty, maxBlocks int
	percentile                       int
	defaultPrice                     *big.Int
	alwaysZero                       bool
	pc                               *core.PriceComparator
}

// NewOracle returns a new oracle.
func NewOracle(backend ethapi.Backend, params Config, pc *core.PriceComparator) *Oracle {
	blocks := params.Blocks
	if blocks < 1 {
		blocks = 1
	}
	percent := params.Percentile
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}
	return &Oracle{
		backend:      backend,
		lastPrice:    params.Default,
		checkBlocks:  blocks,
		maxEmpty:     blocks / 2,
		maxBlocks:    blocks * 5,
		percentile:   percent,
		defaultPrice: params.Default,
		alwaysZero:   params.AlwaysZero,
		pc:           pc,
	}
}

// SuggestPrice returns the recommended gas price.
func (gpo *Oracle) SuggestPrice(ctx context.Context) (*big.Int, error) {

	if gpo.alwaysZero {
		return big.NewInt(0), nil
	}

	price := getCachedPrice(gpo, ctx)
	if price != nil {
		return price, nil
	}
	return calculateGasSuggestion(gpo, ctx)
}

func calculateGasSuggestion(gpo *Oracle, ctx context.Context) (*big.Int, error) {
	gpo.fetchLock.Lock()
	defer gpo.fetchLock.Unlock()

	// check if the cache was refreshed while we waited for the lock
	price := getCachedPrice(gpo, ctx)
	if price != nil {
		return price, nil
	}

	head, _ := gpo.backend.HeaderByNumber(ctx, rpc.LatestBlockNumber)
	headHash := head.Hash()
	lastPrice := gpo.lastPrice

	blockNum := head.Number.Uint64()
	ch := make(chan getBlockPricesResult, gpo.checkBlocks)
	sent := 0
	exp := 0
	var blockPrices []*big.Int
	for sent < gpo.checkBlocks && blockNum > 0 {
		go gpo.getBlockPrices(ctx, types.MakeSigner(gpo.backend.ChainConfig(), big.NewInt(int64(blockNum))), blockNum, ch)
		sent++
		exp++
		blockNum--
	}
	maxEmpty := gpo.maxEmpty
	for exp > 0 {
		res := <-ch
		if res.err != nil {
			return lastPrice, res.err
		}
		exp--
		if res.price != nil {
			blockPrices = append(blockPrices, res.price)
			continue
		}
		if maxEmpty > 0 {
			maxEmpty--
			continue
		}
		if blockNum > 0 && sent < gpo.maxBlocks {
			go gpo.getBlockPrices(ctx, types.MakeSigner(gpo.backend.ChainConfig(), big.NewInt(int64(blockNum))), blockNum, ch)
			sent++
			exp++
			blockNum--
		}
	}
	price = gpo.defaultPrice
	if len(blockPrices) > 0 {
		sort.Sort(bigIntArray(blockPrices))
		price = blockPrices[(len(blockPrices)-1)*gpo.percentile/100]
	}
	if price.Cmp(maxPrice) > 0 {
		price = new(big.Int).Set(maxPrice)
	}

	gpo.cacheLock.Lock()
	gpo.lastHead = headHash
	gpo.lastPrice = price
	gpo.cacheLock.Unlock()
	return price, nil
}

// getCachedPrice is a private function which checks if the last gasPrice
// suggestion was pulled from the current block and, if so, returns it
func getCachedPrice(gpo *Oracle, ctx context.Context) *big.Int {
	gpo.cacheLock.RLock()
	lastHead := gpo.lastHead
	lastPrice := gpo.lastPrice
	gpo.cacheLock.RUnlock()

	head, _ := gpo.backend.HeaderByNumber(ctx, rpc.LatestBlockNumber)
	headHash := head.Hash()
	if headHash == lastHead {
		return lastPrice
	}
	return nil
}

type getBlockPricesResult struct {
	price *big.Int
	err   error
}

// getBlockPrices calculates the lowest transaction gas price in a given block
// and sends it to the result channel. If the block is empty, price is nil.
func (gpo *Oracle) getBlockPrices(ctx context.Context, signer types.Signer, blockNum uint64, ch chan getBlockPricesResult) {
	block, err := gpo.backend.BlockByNumber(ctx, rpc.BlockNumber(blockNum))
	if block == nil {
		ch <- getBlockPricesResult{nil, err}
		return
	}

	blockTxs := block.Transactions()
	prices := make([]*big.Int, len(blockTxs))

	for _, tx := range blockTxs {
		sender, err := types.Sender(signer, tx)
		if err == nil && sender != block.Coinbase() {
			gpInGold, err := gpo.pc.ConvertToGold(tx.GasPrice(), tx.GasCurrency())
			if err != nil {
				log.Error("Error converting gas to gold", "error", err)
			} else {
				prices = append(prices, gpInGold)
			}
		}
	}
	if len(prices) == 0 {
		ch <- getBlockPricesResult{nil, nil}
	} else {
		sort.Sort(bigIntArray(prices))
		ch <- getBlockPricesResult{prices[0], nil}
	}
}

type bigIntArray []*big.Int

func (s bigIntArray) Len() int           { return len(s) }
func (s bigIntArray) Less(i, j int) bool { return s[i].Cmp(s[j]) < 0 }
func (s bigIntArray) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
