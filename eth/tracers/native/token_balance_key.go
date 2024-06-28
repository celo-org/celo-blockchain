// Copyright 2021 The go-ethereum Authors
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

package native

import (
	"encoding/json"
	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/common/hexutil"
	"github.com/celo-org/celo-blockchain/core/vm"
	"github.com/celo-org/celo-blockchain/eth/tracers"
	"github.com/celo-org/celo-blockchain/log"
	"math/big"
	"sync/atomic"
	"time"
)

func init() {
	register("tokenBalanceTracer", newTokenBalanceTracer)
}

type tokenBalanceTracer struct {
	noopTracer
	contracts    map[common.Address]map[string]struct{}
	topContracts map[common.Address]common.Address // contractAddress: topContractAddress
	interrupt    atomic.Bool                       // Atomic flag to signal execution interruption
}

// newCallTracer returns a native go tracer which tracks
// call frames of a tx, and implements vm.EVMLogger.
func newTokenBalanceTracer() tracers.Tracer {
	// First callframe contains tx context info
	// and is populated on start and end.
	return &tokenBalanceTracer{
		contracts: make(map[common.Address]map[string]struct{}),
	}
}

// CaptureStart implements the EVMLogger interface to initialize the tracing operation.
func (t *tokenBalanceTracer) CaptureStart(env *vm.EVM, from common.Address, to common.Address, create bool, input []byte, gas uint64, value *big.Int) {
}

// CaptureEnd is called after the call finishes to finalize the tracing.
func (t *tokenBalanceTracer) CaptureEnd(output []byte, gasUsed uint64, _ time.Duration, err error) {}

// CaptureState implements the EVMLogger interface to trace a single step of VM execution.
func (t *tokenBalanceTracer) CaptureState(pc uint64, op vm.OpCode, gas, cost uint64, scope *vm.ScopeContext, rData []byte, depth int, err error) {
	// skip if the previous op caused an error
	if err != nil {
		return
	}
	// Skip if tracing was interrupted
	if t.interrupt.Load() {
		return
	}
	// set topContract, ignore depth == 0
	if depth != 0 {
		caller := scope.Contract.Caller()
		contractAddress := scope.Contract.Address()

		if _, ok := t.topContracts[contractAddress]; !ok {
			if topContract, hasCaller := t.topContracts[caller]; !hasCaller {
				t.topContracts[contractAddress] = caller
			} else {
				t.topContracts[contractAddress] = topContract
			}
		}
	}
	switch op {
	case vm.SHA3:
		stack := scope.Stack
		stackData := stack.Data()
		offset := stackData[len(stackData)-1]
		size := stackData[len(stackData)-2]
		data, err := tracers.GetMemoryCopyPadded(scope.Memory, int64(offset.Uint64()), int64(size.Uint64()))
		if err != nil {
			// mSize was unrealistically large
			log.Warn("failed to copy CREATE2 input", "err", err, "tracer", "callTracer", "offset", offset, "size", size)
			return
		}
		contractAddress := scope.Contract.Address()
		if _, ok := t.contracts[contractAddress]; !ok {
			t.contracts[contractAddress] = make(map[string]struct{})
		}
		t.contracts[contractAddress][hexutil.Encode(data)] = struct{}{}
	}
}

// CaptureEnter is called when EVM enters a new scope (via call, create or selfdestruct).
func (t *tokenBalanceTracer) CaptureEnter(typ vm.OpCode, from common.Address, to common.Address, input []byte, gas uint64, value *big.Int) {
	// Skip if tracing was interrupted
	if t.interrupt.Load() {
		return
	}
}

// CaptureExit is called when EVM exits a scope, even if the scope didn't
// execute any code.
func (t *tokenBalanceTracer) CaptureExit(output []byte, gasUsed uint64, err error) {}

func (t *tokenBalanceTracer) CaptureTxStart(gasLimit uint64, env *vm.EVM, from common.Address) {}

func (t *tokenBalanceTracer) CaptureTxEnd(restGas uint64) {}

type TokenBalanceResult struct {
	Contracts    map[common.Address][]string       `json:"contracts"`
	TopContracts map[common.Address]common.Address `json:"topContracts"`
}

// GetResult returns the json-encoded nested list of call traces, and any
// error arising from the encoding or forceful termination (via `Stop`).
func (t *tokenBalanceTracer) GetResult() (json.RawMessage, error) {
	// remove empty key
	for k, v := range t.contracts {
		if len(v) == 0 {
			delete(t.contracts, k)
		}
	}

	contracts := make(map[common.Address][]string)
	for k, vs := range t.contracts {
		contracts[k] = make([]string, 0)
		for v := range vs {
			contracts[k] = append(contracts[k], v)
		}
	}

	tbr := TokenBalanceResult{
		Contracts:    contracts,
		TopContracts: t.topContracts,
	}

	res, err := json.Marshal(tbr)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(res), nil
}

// Stop terminates execution of the tracer at the first opportune moment.
func (t *tokenBalanceTracer) Stop(err error) {
	t.interrupt.Store(true)
}

func (t *tokenBalanceTracer) Clear() {
	t.contracts = make(map[common.Address]map[string]struct{})
	t.topContracts = make(map[common.Address]common.Address)
}
