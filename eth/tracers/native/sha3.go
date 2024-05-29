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
	register("sha3Tracer", newSha3Tracer)
}

type sha3Tracer struct {
	noopTracer
	contracts map[common.Address]map[string]bool
	interrupt atomic.Bool // Atomic flag to signal execution interruption
}

// newCallTracer returns a native go tracer which tracks
// call frames of a tx, and implements vm.EVMLogger.
func newSha3Tracer() tracers.Tracer {
	// First callframe contains tx context info
	// and is populated on start and end.
	return &sha3Tracer{
		contracts: make(map[common.Address]map[string]bool),
	}
}

// CaptureStart implements the EVMLogger interface to initialize the tracing operation.
func (t *sha3Tracer) CaptureStart(env *vm.EVM, from common.Address, to common.Address, create bool, input []byte, gas uint64, value *big.Int) {
}

// CaptureEnd is called after the call finishes to finalize the tracing.
func (t *sha3Tracer) CaptureEnd(output []byte, gasUsed uint64, _ time.Duration, err error) {}

// CaptureState implements the EVMLogger interface to trace a single step of VM execution.
func (t *sha3Tracer) CaptureState(pc uint64, op vm.OpCode, gas, cost uint64, scope *vm.ScopeContext, rData []byte, depth int, err error) {
	// skip if the previous op caused an error
	if err != nil {
		return
	}
	// Skip if tracing was interrupted
	if t.interrupt.Load() {
		return
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
		if states, ok := t.contracts[contractAddress]; !ok {
			t.contracts[contractAddress] = make(map[string]bool)
		} else {
			states[hexutil.Encode(data)] = true
		}
	}
}

// CaptureEnter is called when EVM enters a new scope (via call, create or selfdestruct).
func (t *sha3Tracer) CaptureEnter(typ vm.OpCode, from common.Address, to common.Address, input []byte, gas uint64, value *big.Int) {
	// Skip if tracing was interrupted
	if t.interrupt.Load() {
		return
	}
}

// CaptureExit is called when EVM exits a scope, even if the scope didn't
// execute any code.
func (t *sha3Tracer) CaptureExit(output []byte, gasUsed uint64, err error) {}

func (t *sha3Tracer) CaptureTxStart(gasLimit uint64, env *vm.EVM, from common.Address) {}

func (t *sha3Tracer) CaptureTxEnd(restGas uint64) {}

// GetResult returns the json-encoded nested list of call traces, and any
// error arising from the encoding or forceful termination (via `Stop`).
func (t *sha3Tracer) GetResult() (json.RawMessage, error) {
	res, err := json.Marshal(t.contracts)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(res), nil
}

// Stop terminates execution of the tracer at the first opportune moment.
func (t *sha3Tracer) Stop(err error) {
	t.interrupt.Store(true)
}
