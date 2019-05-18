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
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/istanbul"
	"github.com/ethereum/go-ethereum/consensus/istanbul/validator"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/event"
	elog "github.com/ethereum/go-ethereum/log"
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

	address    common.Address
	privateKey ecdsa.PrivateKey
	db         ethdb.Database
}

type testCommittedMsgs struct {
	commitProposal istanbul.Proposal
	committedSeals [][]byte
}

// ==============================================
//
// define the functions that needs to be provided for Istanbul.

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

func (self *testSystemBackend) Gossip(valSet istanbul.ValidatorSet, message []byte) error {
	testLogger.Warn("not sign any data")
	return nil
}

func (self *testSystemBackend) Commit(proposal istanbul.Proposal, seals [][]byte) error {
	testLogger.Info("commit message", "address", self.Address())
	self.committedMsgs = append(self.committedMsgs, testCommittedMsgs{
		commitProposal: proposal,
		committedSeals: seals,
	})

	// fake new head events
	go self.events.Post(istanbul.FinalCommittedEvent{})
	return nil
}

func (self *testSystemBackend) Verify(proposal istanbul.Proposal) (time.Duration, error) {
	return 0, nil
}

func (self *testSystemBackend) Sign(data []byte) ([]byte, error) {
	hashData := crypto.Keccak256(data)
	return crypto.Sign(hashData, &self.privateKey)
}

func (self *testSystemBackend) CheckSignature([]byte, common.Address, []byte) error {
	return nil
}

func (self *testSystemBackend) CheckValidatorSignature(data []byte, sig []byte) (common.Address, error) {
	return istanbul.CheckValidatorSignature(self.peers, data, sig)
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

func (self *testSystemBackend) getPrepareMessage(view istanbul.View, digest common.Hash) (istanbul.Message, error) {
	prepare := &istanbul.Subject{
		View:   &view,
		Digest: digest,
	}

	payload, err := Encode(prepare)
	if err != nil {
		return istanbul.Message{}, err
	}

	msg := istanbul.Message{
		Code:          istanbul.MsgPrepare,
		Msg:           payload,
		Address:       self.address,
		CommittedSeal: []byte{},
	}

	data, err := msg.PayloadNoSig()
	if err != nil {
		return istanbul.Message{}, err
	}
	msg.Signature, err = self.Sign(data)
	return msg, err
}

func (self *testSystemBackend) getRoundChangeMessage(view istanbul.View, preparedCert istanbul.PreparedCertificate) (istanbul.Message, error) {
	rc := &istanbul.RoundChange{
		View:                &view,
		PreparedCertificate: preparedCert,
	}

	payload, err := Encode(rc)
	if err != nil {
		return istanbul.Message{}, err
	}

	msg := istanbul.Message{
		Code:          istanbul.MsgRoundChange,
		Msg:           payload,
		Address:       self.address,
		CommittedSeal: []byte{},
	}

	data, err := msg.PayloadNoSig()
	if err != nil {
		return istanbul.Message{}, err
	}
	msg.Signature, err = self.Sign(data)
	return msg, err
}

// ==============================================
//
// define the struct that need to be provided for integration tests.

type testSystem struct {
	backends []*testSystemBackend
	f        uint64

	queuedMessage chan istanbul.MessageEvent
	quit          chan struct{}
}

func newTestSystem(n, f uint64) *testSystem {
	testLogger.SetHandler(elog.StdoutHandler)
	return &testSystem{
		backends: make([]*testSystemBackend, n),
		f:        f,

		queuedMessage: make(chan istanbul.MessageEvent),
		quit:          make(chan struct{}),
	}
}

func generateValidators(n int) ([]common.Address, []ecdsa.PrivateKey) {
	vals := make([]common.Address, 0)
	privateKeys := make([]ecdsa.PrivateKey, 0)
	for i := 0; i < n; i++ {
		privateKey, _ := crypto.GenerateKey()
		vals = append(vals, crypto.PubkeyToAddress(privateKey.PublicKey))
		privateKeys = append(privateKeys, *privateKey)
	}
	return vals, privateKeys
}

func newTestValidatorSet(n int) istanbul.ValidatorSet {
	addresses, _ := generateValidators(n)
	return validator.NewSet(addresses, istanbul.RoundRobin)
}

func NewTestSystemWithBackend(n, f uint64) *testSystem {
	return NewTestSystemWithBackendAndCurrentRoundState(n, f, func(vset istanbul.ValidatorSet) *roundState {
		return newRoundState(&istanbul.View{
			Round:    big.NewInt(0),
			Sequence: big.NewInt(1),
		}, vset, nil, nil, func(hash common.Hash) bool {
			return false
		})
	})
}

// FIXME: int64 is needed for N and F
func NewTestSystemWithBackendAndCurrentRoundState(n, f uint64, getRoundState func(vset istanbul.ValidatorSet) *roundState) *testSystem {
	testLogger.SetHandler(elog.StdoutHandler)

	addrs, privateKeys := generateValidators(int(n))
	sys := newTestSystem(n, f)
	config := istanbul.DefaultConfig
	// Addresses are sorted in the validator set, we make a mapping here
	// so we can fetch the private key for each validator.
	privateKeyMap := make(map[common.Address]ecdsa.PrivateKey)
	for i := uint64(0); i < n; i++ {
		privateKeyMap[addrs[i]] = privateKeys[i]
	}

	for i := uint64(0); i < n; i++ {
		vset := validator.NewSet(addrs, istanbul.RoundRobin)
		backend := sys.NewBackend(i)
		backend.peers = vset
		backend.address = vset.GetByIndex(i).Address()
		backend.privateKey = privateKeyMap[backend.address]

		core := New(backend, config).(*core)
		core.state = StateAcceptRequest
		core.current = getRoundState(vset)
		core.roundChangeSet = newRoundChangeSet(vset)
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

func (t *testSystem) F() uint64 {
	return t.f
}

func (sys *testSystem) getPreparedCertificate(t *testing.T, view istanbul.View, proposal istanbul.Proposal) istanbul.PreparedCertificate {
	preparedCertificate := istanbul.PreparedCertificate{
		Proposal:        proposal,
		PrepareMessages: []istanbul.Message{},
	}
	for i, backend := range sys.backends {
		if uint64(i) == 2*sys.F()+1 {
			break
		}
		msg, err := backend.getPrepareMessage(view, proposal.Hash())
		if err != nil {
			t.Errorf("Failed to create PREPARE message: %v", err)
		}
		preparedCertificate.PrepareMessages = append(preparedCertificate.PrepareMessages, msg)
	}
	return preparedCertificate
}

func (sys *testSystem) getRoundChangeCertificate(t *testing.T, view istanbul.View, preparedCertificate istanbul.PreparedCertificate) istanbul.RoundChangeCertificate {
	var roundChangeCertificate istanbul.RoundChangeCertificate
	for i, backend := range sys.backends {
		if uint64(i) == 2*sys.F()+1 {
			break
		}
		msg, err := backend.getRoundChangeMessage(view, preparedCertificate)
		if err != nil {
			t.Errorf("Failed to create ROUND CHANGE message: %v", err)
		}
		roundChangeCertificate.RoundChangeMessages = append(roundChangeCertificate.RoundChangeMessages, msg)
	}
	return roundChangeCertificate
}

// ==============================================
//
// helper functions.

func getPublicKeyAddress(privateKey *ecdsa.PrivateKey) common.Address {
	return crypto.PubkeyToAddress(privateKey.PublicKey)
}
