// Copyright 2017 The Celo Authors
// This file is part of the celo library.
//
// The celo library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The celo library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the celo library. If not, see <http://www.gnu.org/licenses/>.

package consensustest

import (
	"crypto/ecdsa"
	"errors"
	"math/big"
	"net"
	"runtime"
	"time"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/common/hexutil"
	"github.com/celo-org/celo-blockchain/consensus"
	"github.com/celo-org/celo-blockchain/core/state"
	"github.com/celo-org/celo-blockchain/core/types"
	"github.com/celo-org/celo-blockchain/crypto"
	"github.com/celo-org/celo-blockchain/p2p"
	"github.com/celo-org/celo-blockchain/p2p/enode"
	"github.com/celo-org/celo-blockchain/params"
	"github.com/celo-org/celo-blockchain/rpc"
	"github.com/celo-org/celo-blockchain/trie"
)

var (
	errFakeFail = errors.New("mockEngine fake fail")
)

type MockBroadcaster struct{}

func (b *MockBroadcaster) Enqueue(id string, block *types.Block) {
}

func (b *MockBroadcaster) FindPeers(targets map[enode.ID]bool, purpose p2p.PurposeFlag) map[enode.ID]consensus.Peer {
	return make(map[enode.ID]consensus.Peer)
}

type MockP2PServer struct {
	Node *enode.Node
}

var defaultPubKey *ecdsa.PublicKey = &ecdsa.PublicKey{
	Curve: crypto.S256(),
	X:     hexutil.MustDecodeBig("0x760c4460e5336ac9bbd87952a3c7ec4363fc0a97bd31c86430806e287b437fd1"),
	Y:     hexutil.MustDecodeBig("0xb01abc6e1db640cf3106b520344af1d58b00b57823db3e1407cbc433e1b6d04d")}

func NewMockP2PServer(pubKey *ecdsa.PublicKey) *MockP2PServer {
	if pubKey == nil {
		pubKey = defaultPubKey
	}

	mockNode := enode.NewV4(
		pubKey,
		net.IP{192, 168, 0, 1},
		30303,
		30303)

	return &MockP2PServer{Node: mockNode}
}

func (serv *MockP2PServer) Self() *enode.Node {
	return serv.Node
}

func (serv *MockP2PServer) AddPeer(node *enode.Node, purpose p2p.PurposeFlag) {}

func (serv *MockP2PServer) RemovePeer(node *enode.Node, purpose p2p.PurposeFlag) {}

func (serv *MockP2PServer) AddTrustedPeer(node *enode.Node, purpose p2p.PurposeFlag) {}

func (serv *MockP2PServer) RemoveTrustedPeer(node *enode.Node, purpose p2p.PurposeFlag) {}

type MockPeer struct {
	node     *enode.Node
	purposes p2p.PurposeFlag
}

func NewMockPeer(node *enode.Node, purposes p2p.PurposeFlag) *MockPeer {
	mockPeer := &MockPeer{node: node, purposes: purposes}

	return mockPeer
}

func (mp *MockPeer) Send(msgCode uint64, data interface{}) error {
	return nil
}

func (mp *MockPeer) Node() *enode.Node {
	return mp.node
}

func (mp *MockPeer) Version() uint {
	return 0
}

func (mp *MockPeer) ReadMsg() (p2p.Msg, error) {
	return p2p.Msg{}, nil
}

func (mp *MockPeer) Inbound() bool {
	return false
}

func (mp *MockPeer) PurposeIsSet(purpose p2p.PurposeFlag) bool {
	return true
}

type Mode uint

// MockEngine provides a minimal fake implementation of a consensus engine for use in blockchain tests.
type MockEngine struct {
	consensus.Engine

	mode Mode

	fakeFail  uint64        // Block number which fails consensus even in fake mode
	fakeDelay time.Duration // Time delay to sleep for before returning from verify

	processBlock        func(block *types.Block, statedb *state.StateDB) (types.Receipts, []*types.Log, uint64, error)
	validateState       func(block *types.Block, statedb *state.StateDB, receipts types.Receipts, usedGas uint64) error
	onNewConsensusBlock func(block *types.Block, receipts []*types.Receipt, logs []*types.Log, state *state.StateDB)
}

const (
	Fake Mode = iota
	FullFake
)

var (
	// Max time from current time allowed for blocks, before they're considered future blocks
	allowedFutureBlockTime = 15 * time.Second

	errZeroBlockTime = errors.New("timestamp equals parent's")
)

// NewFaker creates a MockEngine consensus engine that accepts
// all blocks' seal as valid, though they still have to conform to the Ethereum
// consensus rules.
func NewFaker() *MockEngine {
	return &MockEngine{
		mode: Fake,
	}
}

// NewFakeFailer creates a MockEngine consensus engine that
// accepts all blocks as valid apart from the single one specified, though they
// still have to conform to the Ethereum consensus rules.
func NewFakeFailer(blockNumber uint64) *MockEngine {
	return &MockEngine{
		mode:     Fake,
		fakeFail: blockNumber,
	}
}

// NewFakeDelayer creates a MockEngine consensus engine that
// accepts all blocks as valid, but delays verifications by some time, though
// they still have to conform to the Ethereum consensus rules.
func NewFakeDelayer(delay time.Duration) *MockEngine {
	return &MockEngine{
		mode:      Fake,
		fakeDelay: delay,
	}
}

// NewFullFaker creates an MockEngine consensus engine with a full fake scheme that
// accepts all blocks as valid, without checking any consensus rules whatsoever.
func NewFullFaker() *MockEngine {
	return &MockEngine{
		mode: FullFake,
	}
}

func (e *MockEngine) accumulateRewards(config *params.ChainConfig, state *state.StateDB, header *types.Header) {
	// Simply touch coinbase account
	reward := big.NewInt(1)
	state.AddBalance(header.Coinbase, reward)
}

func (e *MockEngine) Finalize(chain consensus.ChainHeaderReader, header *types.Header, statedb *state.StateDB, txs []*types.Transaction) {
	e.accumulateRewards(chain.Config(), statedb, header)
	header.Root = statedb.IntermediateRoot(chain.Config().IsEIP158(header.Number))
}

func (e *MockEngine) FinalizeAndAssemble(chain consensus.ChainHeaderReader, header *types.Header, statedb *state.StateDB, txs []*types.Transaction, receipts []*types.Receipt, randomness *types.Randomness) (*types.Block, error) {
	e.accumulateRewards(chain.Config(), statedb, header)
	header.Root = statedb.IntermediateRoot(chain.Config().IsEIP158(header.Number))

	// Header seems complete, assemble into a block and return
	return types.NewBlock(header, txs, receipts, randomness, new(trie.Trie)), nil
}

func (e *MockEngine) Author(header *types.Header) (common.Address, error) {
	return header.Coinbase, nil
}

func (e *MockEngine) VerifyHeader(chain consensus.ChainHeaderReader, header *types.Header, seal bool) error {
	if e.mode == FullFake {
		return nil
	}
	// Short circuit if the header is known, or if its parent is unknown
	number := header.Number.Uint64()
	if chain.GetHeader(header.Hash(), number) != nil {
		return nil
	}
	parent := chain.GetHeader(header.ParentHash, number-1)
	if parent == nil && chain.Config().FullHeaderChainAvailable {
		return consensus.ErrUnknownAncestor
	}
	// Sanity checks passed, do a proper verification
	return e.verifyHeader(chain, header, parent, seal)
}

// verifyHeader checks whether a header conforms to the consensus rules
func (e *MockEngine) verifyHeader(chain consensus.ChainHeaderReader, header, parent *types.Header, seal bool) error {
	// Ensure that the extra data format is satisfied
	if _, err := types.ExtractIstanbulExtra(header); err != nil {
		return errors.New("invalid extra data format")
	}
	// Verify the header's timestamp
	if header.Time > uint64(time.Now().Add(allowedFutureBlockTime).Unix()) {
		return consensus.ErrFutureBlock
	}
	if parent != nil {
		if header.Time <= parent.Time {
			return errZeroBlockTime
		}
		// Verify that the block number is parent's +1
		if diff := new(big.Int).Sub(header.Number, parent.Number); diff.Cmp(big.NewInt(1)) != 0 {
			return consensus.ErrInvalidNumber
		}
	}
	// Verify the engine specific seal securing the block
	if seal {
		if err := e.VerifySeal(header); err != nil {
			return err
		}
	}
	return nil
}

func (e *MockEngine) VerifySeal(header *types.Header) error {
	return e.verifySeal(header)
}

func (e *MockEngine) verifySeal(header *types.Header) error {
	time.Sleep(e.fakeDelay)
	if e.fakeFail == header.Number.Uint64() {
		return errFakeFail
	}
	return nil
}

// VerifyHeaders is similar to VerifyHeader, but verifies a batch of headers
// concurrently. The method returns a quit channel to abort the operations and
// a results channel to retrieve the async verifications.
func (e *MockEngine) VerifyHeaders(chain consensus.ChainHeaderReader, headers []*types.Header, seals []bool) (chan<- struct{}, <-chan error) {
	if e.mode == FullFake || len(headers) == 0 {
		abort, results := make(chan struct{}), make(chan error, len(headers))
		for i := 0; i < len(headers); i++ {
			results <- nil
		}
		return abort, results
	}

	// Spawn as many workers as allowed threads
	workers := runtime.GOMAXPROCS(0)
	if len(headers) < workers {
		workers = len(headers)
	}

	// Create a task channel and spawn the verifiers
	var (
		inputs = make(chan int)
		done   = make(chan int, workers)
		errors = make([]error, len(headers))
		abort  = make(chan struct{})
	)
	for i := 0; i < workers; i++ {
		go func() {
			for index := range inputs {
				errors[index] = e.verifyHeaderWorker(chain, headers, seals, index)
				done <- index
			}
		}()
	}

	errorsOut := make(chan error, len(headers))
	go func() {
		defer close(inputs)
		var (
			in, out = 0, 0
			checked = make([]bool, len(headers))
			inputs  = inputs
		)
		for {
			select {
			case inputs <- in:
				if in++; in == len(headers) {
					// Reached end of headers. Stop sending to workers.
					inputs = nil
				}
			case index := <-done:
				for checked[index] = true; checked[out]; out++ {
					errorsOut <- errors[out]
					if out == len(headers)-1 {
						return
					}
				}
			case <-abort:
				return
			}
		}
	}()
	return abort, errorsOut
}

func (e *MockEngine) verifyHeaderWorker(chain consensus.ChainHeaderReader, headers []*types.Header, seals []bool, index int) error {
	var parent *types.Header
	if index == 0 {
		parent = chain.GetHeader(headers[0].ParentHash, headers[0].Number.Uint64()-1)
	} else if headers[index-1].Hash() == headers[index].ParentHash {
		parent = headers[index-1]
	}
	if parent == nil && chain.Config().FullHeaderChainAvailable {
		return consensus.ErrUnknownAncestor
	}
	if chain.GetHeader(headers[index].Hash(), headers[index].Number.Uint64()) != nil {
		return nil // known block
	}
	return e.verifyHeader(chain, headers[index], parent, seals[index])
}

func (e *MockEngine) Prepare(chain consensus.ChainHeaderReader, header *types.Header) error {
	parent := chain.GetHeader(header.ParentHash, header.Number.Uint64()-1)
	if parent == nil {
		return consensus.ErrUnknownAncestor
	}

	// Matches delay in consensus/istanbul/backend/engine.go:386 in (*Backend).Prepare
	delay := time.Until(time.Unix(int64(header.Time), 0))
	if delay > 0 {
		time.Sleep(delay)
	}

	return nil
}

type fullChain interface {
	CurrentBlock() *types.Block
	StateAt(common.Hash) (*state.StateDB, error)
}

func (e *MockEngine) Seal(chain consensus.ChainHeaderReader, block *types.Block) error {
	header := block.Header()
	finalBlock := block.WithHeader(header)
	c := chain.(fullChain)

	parent := c.CurrentBlock()

	state, err := c.StateAt(parent.Root())
	if err != nil {
		return err
	}

	receipts, logs, _, err := e.processBlock(finalBlock, state)
	if err != nil {
		return err
	}
	e.onNewConsensusBlock(block, receipts, logs, state)

	return nil
}

// APIs implements consensus.Engine, returning the user facing RPC APIs.
func (e *MockEngine) APIs(chain consensus.ChainHeaderReader) []rpc.API {
	return []rpc.API{}
}

// Close closes the exit channel to notify all backend threads exiting.
func (e *MockEngine) Close() error {
	return nil
}

// EpochSize size of the epoch
func (e *MockEngine) EpochSize() uint64 {
	return 100
}

// SetCallBacks sets call back functions
func (e *MockEngine) SetCallBacks(hasBadBlock func(common.Hash) bool,
	processBlock func(*types.Block, *state.StateDB) (types.Receipts, []*types.Log, uint64, error),
	validateState func(*types.Block, *state.StateDB, types.Receipts, uint64) error,
	onNewConsensusBlock func(block *types.Block, receipts []*types.Receipt, logs []*types.Log, state *state.StateDB)) error {
	e.processBlock = processBlock
	e.validateState = validateState
	e.onNewConsensusBlock = onNewConsensusBlock

	return nil

}
