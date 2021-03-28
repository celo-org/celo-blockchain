package contracts

import (
	"math/big"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/core/vm"
	"github.com/celo-org/celo-blockchain/metrics"
	lru "github.com/hashicorp/golang-lru"
)

var (
	cacheHits    = metrics.NewRegisteredMeter("contract_comm/caller/cache_hits", nil)
	cacheMisses  = metrics.NewRegisteredMeter("contract_comm/caller/cache_misses", nil)
	cacheBadCast = metrics.NewRegisteredMeter("contract_comm/caller/cache_bad_cast", nil)
)

type cacheKey struct {
	stateRoot common.Hash
	caller    vm.ContractRef
	addr      common.Address
	input     []byte
	gas       uint64
}

type cacheResult struct {
	ret     []byte
	gasLeft uint64
}

type cacheBackend struct {
	source Backend
	cache  *lru.Cache
}

func WithCache(backend Backend, cache *lru.Cache) Backend {
	return &cacheBackend{
		source: backend,
		cache:  cache,
	}
}

func createGlobalCache() func(Backend) Backend {
	var globalCache, err = lru.New(100)
	if err != nil {
		panic("error creating global cache: " + err.Error())
	}
	return func(backend Backend) Backend {
		return WithCache(backend, globalCache)
	}
}

var WithGlobalCache = createGlobalCache()

// Call implements Backend.Call
func (cb *cacheBackend) Call(caller vm.ContractRef, addr common.Address, input []byte, gas uint64, value *big.Int) (ret []byte, leftOverGas uint64, err error) {
	return cb.source.Call(caller, addr, input, gas, value)
}

// StaticCall implements Backend.StaticCall
func (cb *cacheBackend) StaticCall(caller vm.ContractRef, addr common.Address, input []byte, gas uint64) (ret []byte, leftOverGas uint64, err error) {
	state := cb.source.GetStateDB()

	// there's a race condition between state.IsDirty(), state.StateRoot() and the actual call: source.StaticCall()
	// it is assumed that statedb is not being accessed outside the cache (and caller treats cache in a thread safe manner)
	if state.IsDirty() {
		cacheMisses.Mark(1)
		return cb.source.StaticCall(caller, addr, input, gas)
	}

	key := cacheKey{state.StateRoot(), caller, addr, input, gas}

	if result, ok := cb.cache.Get(key); ok {
		if cachedResult, castOk := result.(cacheResult); castOk {
			cacheHits.Mark(1)
			ret := make([]byte, len(cachedResult.ret))
			copy(ret, cachedResult.ret)
			return ret, cachedResult.gasLeft, nil
		} else {
			cacheBadCast.Mark(1)
			cb.cache.Remove(key)
		}
	}

	cacheMisses.Mark(1)

	ret, gasLeft, err := cb.source.StaticCall(caller, addr, input, gas)
	if err != nil {
		return ret, gasLeft, err
	}

	result := cacheResult{
		ret:     make([]byte, len(ret)),
		gasLeft: gasLeft,
	}
	copy(result.ret, ret)
	cb.cache.Add(key, result)
	return ret, gasLeft, nil
}

// StopGasMetering implements Backend.StopGasMetering
func (cb *cacheBackend) StopGasMetering() {
	cb.source.StopGasMetering()
}

// StartGasMetering implements Backend.StartGasMetering
func (cb *cacheBackend) StartGasMetering() {
	cb.source.StartGasMetering()
}

// GetStateDB implements Backend.GetStateDB
func (cb *cacheBackend) GetStateDB() vm.StateDB {
	return cb.source.GetStateDB()
}
