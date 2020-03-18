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
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/params"
)

var (
	errFakeFail = errors.New("MockEngine Fake Fail")
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

func NewMockP2PServer() *MockP2PServer {
	mockNode := enode.NewV4(
		&ecdsa.PublicKey{
			Curve: crypto.S256(),
			X:     hexutil.MustDecodeBig("0x760c4460e5336ac9bbd87952a3c7ec4363fc0a97bd31c86430806e287b437fd1"),
			Y:     hexutil.MustDecodeBig("0xb01abc6e1db640cf3106b520344af1d58b00b57823db3e1407cbc433e1b6d04d")},
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

// MockEngine is adapted from consensus/ethash (which has been deleted) for the purpose of
// preserving legacy tests.
type Mode uint

// Config are the configuration parameters of the MockEngine.
type Config struct {
	Mode Mode
}

type MockEngine struct {
	consensus.Engine

	config Config

	fakeFail  uint64        // Block number which fails consensus even in fake mode
	fakeDelay time.Duration // Time delay to sleep for before returning from verify
}

const (
	ModeFake Mode = iota
	ModeFakeFail
	ModeDelayFake
	ModeFullFake
)

// NewFaker creates a MockEngine consensus engine that accepts
// all blocks' seal as valid, though they still have to conform to the Ethereum
// consensus rules.
func NewFaker() *MockEngine {
	return &MockEngine{
		config: Config{
			Mode: ModeFake,
		},
	}
}

// NewFakeFailer creates a MockEngine consensus engine that
// accepts all blocks as valid apart from the single one specified, though they
// still have to conform to the Ethereum consensus rules.
func NewFakeFailer(fail uint64) *MockEngine {
	return &MockEngine{
		config: Config{
			Mode: ModeFakeFail,
		},
		fakeFail: fail,
	}
}

// NewFakeDelayer creates a MockEngine consensus engine that
// accepts all blocks as valid, but delays verifications by some time, though
// they still have to conform to the Ethereum consensus rules.
func NewFakeDelayer(delay time.Duration) *MockEngine {
	return &MockEngine{
		config: Config{
			Mode: ModeDelayFake,
		},
		fakeDelay: delay,
	}
}

// NewFullFaker creates an MockEngine consensus engine with a full fake scheme that
// accepts all blocks as valid, without checking any consensus rules whatsoever.
func NewFullFaker() *MockEngine {
	return &MockEngine{
		config: Config{
			Mode: ModeFullFake,
		},
	}
}

func (e *MockEngine) accumulateRewards(config *params.ChainConfig, state *state.StateDB, header *types.Header) {
	// Simply touch miner coinbase account
	reward := big.NewInt(0)
	state.AddBalance(header.Coinbase, reward)
}

func (e *MockEngine) Finalize(chain consensus.ChainReader, header *types.Header, statedb *state.StateDB, txs []*types.Transaction) {
	e.accumulateRewards(chain.Config(), statedb, header)
	header.Root = statedb.IntermediateRoot(chain.Config().IsEIP158(header.Number))
}

func (e *MockEngine) FinalizeAndAssemble(chain consensus.ChainReader, header *types.Header, statedb *state.StateDB, txs []*types.Transaction, receipts []*types.Receipt, randomness *types.Randomness) (*types.Block, error) {
	e.accumulateRewards(chain.Config(), statedb, header)
	header.Root = statedb.IntermediateRoot(chain.Config().IsEIP158(header.Number))

	// Header seems complete, assemble into a block and return
	return types.NewBlock(header, txs, receipts, nil), nil
}

func (e *MockEngine) Author(header *types.Header) (common.Address, error) {
	return common.Address{}, nil
}

func (e *MockEngine) VerifyHeader(chain consensus.ChainReader, header *types.Header, seal bool) error {

	switch e.config.Mode {
	case ModeFake:
		return nil
	case ModeFullFake:
		return nil
	case ModeDelayFake:
		time.Sleep(e.fakeDelay)
		return nil
	case ModeFakeFail:
		if header.Number.Cmp(big.NewInt(int64(e.fakeFail))) == 0 {
			return errFakeFail
		}
		return nil
	default:
		return nil
	}
}

// VerifyHeaders is similar to VerifyHeader, but verifies a batch of headers
// concurrently. The method returns a quit channel to abort the operations and
// a results channel to retrieve the async verifications (the order is that of
// the input slice).
func (e *MockEngine) VerifyHeaders(chain consensus.ChainReader, headers []*types.Header, seals []bool) (chan<- struct{}, <-chan error) {
	abort := make(chan struct{})
	results := make(chan error, len(headers))
	go func() {
		for i, header := range headers {
			err := e.VerifyHeader(chain, header, seals[i])

			select {
			case <-abort:
				return
			case results <- err:
			}
		}
	}()
	return abort, results
}
