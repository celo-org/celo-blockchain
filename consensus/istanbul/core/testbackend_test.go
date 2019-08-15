// Copyright 2017 The go-ethereum Authors
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
	"crypto/ecdsa"
	"math/big"
	"time"

	"github.com/celo-org/bls-zexe/go"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/istanbul"
	"github.com/ethereum/go-ethereum/consensus/istanbul/validator"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/crypto/bls"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/event"
	elog "github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/p2p/enode"
)

var testLogger = elog.New()

type testSystemBackend struct {
	id  uint64
	sys *testSystem

	engine Engine
	peers  istanbul.ValidatorSet
	events *event.TypeMux

	committedMsgs []testCommittedMsgs
	sentMsgs      [][]byte // store the message when Send is called by core

	key     []byte
	address common.Address
	db      ethdb.Database
}

type testCommittedMsgs struct {
	commitProposal istanbul.Proposal
	bitmap         *big.Int
	committedSeals []byte
}

// ==============================================
//
// define the functions that needs to be provided for Istanbul.

func (self *testSystemBackend) Authorize(address common.Address, _ istanbul.SignerFn, _ istanbul.SignerFn, _ istanbul.MessageSignerFn) {
	self.address = address
	self.engine.SetAddress(address)
}

func (self *testSystemBackend) Address() common.Address {
	return self.address
}

// Peers returns all connected peers
func (self *testSystemBackend) Validators(proposal istanbul.Proposal) istanbul.ValidatorSet {
	return self.peers
}

func (self *testSystemBackend) EventMux() *event.TypeMux {
	return self.events
}

func (self *testSystemBackend) Send(message []byte, target common.Address) error {
	testLogger.Info("enqueuing a message...", "address", self.Address())
	self.sentMsgs = append(self.sentMsgs, message)
	self.sys.queuedMessage <- istanbul.MessageEvent{
		Payload: message,
	}
	return nil
}

func (self *testSystemBackend) Broadcast(valSet istanbul.ValidatorSet, message []byte) error {
	testLogger.Info("enqueuing a message...", "address", self.Address())
	self.sentMsgs = append(self.sentMsgs, message)
	self.sys.queuedMessage <- istanbul.MessageEvent{
		Payload: message,
	}
	return nil
}
func (self *testSystemBackend) Gossip(valSet istanbul.ValidatorSet, message []byte, msgCode uint64, ignoreCache bool) error {
	testLogger.Warn("not sign any data")
	return nil
}

func (self *testSystemBackend) SignBlockHeader(data []byte) ([]byte, error) {
	privateKey, _ := bls.DeserializePrivateKey(self.key)
	defer privateKey.Destroy()

	signature, _ := privateKey.SignMessage(data, []byte{}, false)
	defer signature.Destroy()
	signatureBytes, _ := signature.Serialize()

	return signatureBytes, nil
}

func (self *testSystemBackend) Commit(proposal istanbul.Proposal, bitmap *big.Int, seals []byte) error {
	testLogger.Info("commit message", "address", self.Address())
	self.committedMsgs = append(self.committedMsgs, testCommittedMsgs{
		commitProposal: proposal,
		bitmap:         bitmap,
		committedSeals: seals,
	})

	// fake new head events
	go self.events.Post(istanbul.FinalCommittedEvent{})
	return nil
}

func (self *testSystemBackend) Verify(proposal istanbul.Proposal, src istanbul.Validator) (time.Duration, error) {
	return 0, nil
}

func (self *testSystemBackend) Sign(data []byte) ([]byte, error) {
	testLogger.Warn("not sign any data")
	return data, nil
}

func (self *testSystemBackend) CheckSignature([]byte, common.Address, []byte) error {
	return nil
}

func (self *testSystemBackend) CheckValidatorSignature(data []byte, sig []byte) (common.Address, error) {
	return common.Address{}, nil
}

func (self *testSystemBackend) Hash(b interface{}) common.Hash {
	return common.BytesToHash([]byte("Test"))
}

func (self *testSystemBackend) NewRequest(request istanbul.Proposal) {
	go self.events.Post(istanbul.RequestEvent{
		Proposal: request,
	})
}

func (self *testSystemBackend) HasBadProposal(hash common.Hash) bool {
	return false
}

func (self *testSystemBackend) LastProposal() (istanbul.Proposal, common.Address) {
	l := len(self.committedMsgs)
	if l > 0 {
		return self.committedMsgs[l-1].commitProposal, common.Address{}
	}
	return makeBlock(0), common.Address{}
}

// Only block height 5 will return true
func (self *testSystemBackend) HasProposal(hash common.Hash, number *big.Int) bool {
	return number.Cmp(big.NewInt(5)) == 0
}

func (self *testSystemBackend) GetProposer(number uint64) common.Address {
	return common.Address{}
}

func (self *testSystemBackend) ParentValidators(proposal istanbul.Proposal) istanbul.ValidatorSet {
	return self.peers
}

func (self *testSystemBackend) AddValidatorPeer(enodeURL string) {}

func (self *testSystemBackend) RemoveValidatorPeer(enodeURL string) {}

func (self *testSystemBackend) GetValidatorPeers() []string {
	return nil
}

func (self *testSystemBackend) Enode() *enode.Node {
	return nil
}

func (self *testSystemBackend) RefreshValPeers(valSet istanbul.ValidatorSet) {}

// ==============================================
//
// define the struct that need to be provided for integration tests.

type testSystem struct {
	backends       []*testSystemBackend
	validatorsKeys [][]byte

	queuedMessage chan istanbul.MessageEvent
	quit          chan struct{}
}

func newTestSystem(n uint64, keys [][]byte) *testSystem {
	testLogger.SetHandler(elog.StdoutHandler)
	return &testSystem{
		backends:       make([]*testSystemBackend, n),
		validatorsKeys: keys,

		queuedMessage: make(chan istanbul.MessageEvent),
		quit:          make(chan struct{}),
	}
}

func generateValidators(n int) ([]istanbul.ValidatorData, [][]byte) {
	vals := make([]istanbul.ValidatorData, 0)
	keys := make([][]byte, 0)
	for i := 0; i < n; i++ {
		privateKey, _ := crypto.GenerateKey()
		blsPrivateKey, _ := blscrypto.ECDSAToBLS(privateKey)
		blsPublicKey, _ := blscrypto.PrivateToPublic(blsPrivateKey)
		vals = append(vals, istanbul.ValidatorData{
			crypto.PubkeyToAddress(privateKey.PublicKey),
			blsPublicKey,
		})
		keys = append(keys, blsPrivateKey)
	}
	return vals, keys
}

func newTestValidatorSet(n int) istanbul.ValidatorSet {
	validators, _ := generateValidators(n)
	return validator.NewSet(validators, istanbul.RoundRobin)
}

// FIXME: int64 is needed for N and F
func NewTestSystemWithBackend(n, f uint64) *testSystem {
	testLogger.SetHandler(elog.StdoutHandler)

	validators, keys := generateValidators(int(n))
	sys := newTestSystem(n, keys)
	config := istanbul.DefaultConfig

	for i := uint64(0); i < n; i++ {
		vset := validator.NewSet(validators, istanbul.RoundRobin)
		backend := sys.NewBackend(i)
		backend.peers = vset
		backend.address = vset.GetByIndex(i).Address()
		backend.key = keys[i]

		core := New(backend, config).(*core)
		core.state = StateAcceptRequest
		core.current = newRoundState(&istanbul.View{
			Round:    big.NewInt(0),
			Sequence: big.NewInt(1),
		}, vset, common.Hash{}, nil, nil, func(hash common.Hash) bool {
			return false
		})
		core.valSet = vset
		core.logger = testLogger
		core.validateFn = backend.CheckValidatorSignature

		backend.engine = core
	}

	return sys
}

// listen will consume messages from queue and deliver a message to core
func (t *testSystem) listen() {
	for {
		select {
		case <-t.quit:
			return
		case queuedMessage := <-t.queuedMessage:
			testLogger.Info("consuming a queue message...")
			for _, backend := range t.backends {
				go backend.EventMux().Post(queuedMessage)
			}
		}
	}
}

// Run will start system components based on given flag, and returns a closer
// function that caller can control lifecycle
//
// Given a true for core if you want to initialize core engine.
func (t *testSystem) Run(core bool) func() {
	for _, b := range t.backends {
		if core {
			b.engine.Start() // start Istanbul core
		}
	}

	go t.listen()
	closer := func() { t.stop(core) }
	return closer
}

func (t *testSystem) stop(core bool) {
	close(t.quit)

	for _, b := range t.backends {
		if core {
			b.engine.Stop()
		}
	}
}

func (t *testSystem) NewBackend(id uint64) *testSystemBackend {
	// assume always success
	ethDB := ethdb.NewMemDatabase()
	backend := &testSystemBackend{
		id:     id,
		sys:    t,
		events: new(event.TypeMux),
		db:     ethDB,
	}

	t.backends[id] = backend
	return backend
}

// ==============================================
//
// helper functions.

func getPublicKeyAddress(privateKey *ecdsa.PrivateKey) common.Address {
	return crypto.PubkeyToAddress(privateKey.PublicKey)
}
