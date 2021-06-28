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
	"bytes"
	"crypto/ecdsa"
	"encoding/binary"
	"fmt"
	"math"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/consensus/istanbul"
	"github.com/ethereum/go-ethereum/consensus/istanbul/validator"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	blscrypto "github.com/ethereum/go-ethereum/crypto/bls"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/event"
	elog "github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/params"
	"github.com/celo-org/celo-bls-go/bls"
)

// ErrorReporter is the intersection of the testing.B and testing.T interfaces.
// This enables setup functions to be used by both benchmarks and tests.
type ErrorReporter interface {
	Errorf(format string, args ...interface{})
}

var testLogger = elog.New()

type testSystemBackend struct {
	id  uint64
	sys *testSystem

	engine Engine
	peers  istanbul.ValidatorSet
	events *event.TypeMux

	committedMsgs []testCommittedMsgs
	sentMsgs      [][]byte // store the message when Send is called by core

	key     ecdsa.PrivateKey
	blsKey  []byte
	address common.Address
	db      ethdb.Database

	// Function pointer to a verify function, so that the test core_test.go/TestVerifyProposal
	// can inject in different proposal verification statuses.
	verifyImpl func(proposal istanbul.Proposal) (*StateProcessResult, time.Duration, error)

	donutBlock *big.Int
}

type testCommittedMsgs struct {
	commitProposal                  istanbul.Proposal
	aggregatedSeal                  types.IstanbulAggregatedSeal
	aggregatedEpochValidatorSetSeal types.IstanbulEpochValidatorSetSeal
	stateProcessResult              *StateProcessResult
}

// ==============================================
//
// define the functions that needs to be provided for Istanbul.

func (self *testSystemBackend) Authorize(address, _ common.Address, _ *ecdsa.PublicKey, _ istanbul.DecryptFn, _ istanbul.SignerFn, _ istanbul.BLSSignerFn) {
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

func (self *testSystemBackend) IsValidating() bool {
	return true
}

func (self *testSystemBackend) ChainConfig() *params.ChainConfig {
	return &params.ChainConfig{
		DonutBlock: self.donutBlock,
	}
}

func (self *testSystemBackend) HashForBlock(number uint64) common.Hash {
	buffer := new(bytes.Buffer)
	_ = binary.Write(buffer, binary.LittleEndian, number)
	hash := common.Hash{}
	copy(hash[:], buffer.Bytes())
	return hash
}

func (self *testSystemBackend) IsPrimary() bool {
	return true
}

func (self *testSystemBackend) IsPrimaryForSeq(seq *big.Int) bool {
	return true
}

func (self *testSystemBackend) NextBlockValidators(proposal istanbul.Proposal) (istanbul.ValidatorSet, error) {
	//This doesn't really return the next block validators
	return self.peers, nil
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

func (self *testSystemBackend) Multicast(validators []common.Address, message []byte, msgCode uint64, sendToSelf bool) error {
	testLogger.Info("enqueuing a message...", "address", self.Address())
	self.sentMsgs = append(self.sentMsgs, message)
	send := func() {
		self.sys.queuedMessage <- istanbul.MessageEvent{
			Payload: message,
		}
	}
	go send()
	return nil
}

func (self *testSystemBackend) Gossip(payload []byte, ethMsgCode uint64) error {
	return nil
}

func (self *testSystemBackend) SignBLS(data []byte, extra []byte, useComposite, cip22 bool) (blscrypto.SerializedSignature, error) {
	privateKey, _ := bls.DeserializePrivateKey(self.blsKey)
	defer privateKey.Destroy()

	signature, _ := privateKey.SignMessage(data, extra, useComposite, cip22)
	defer signature.Destroy()
	signatureBytes, _ := signature.Serialize()

	return blscrypto.SerializedSignatureFromBytes(signatureBytes)
}

func (self *testSystemBackend) Commit(proposal istanbul.Proposal, aggregatedSeal types.IstanbulAggregatedSeal, aggregatedEpochValidatorSetSeal types.IstanbulEpochValidatorSetSeal, stateProcessResult *StateProcessResult) error {
	testLogger.Info("commit message", "address", self.Address())
	self.committedMsgs = append(self.committedMsgs, testCommittedMsgs{
		commitProposal:                  proposal,
		aggregatedSeal:                  aggregatedSeal,
		aggregatedEpochValidatorSetSeal: aggregatedEpochValidatorSetSeal,
		stateProcessResult:              stateProcessResult,
	})

	// fake new head events
	go self.events.Post(istanbul.FinalCommittedEvent{})
	return nil
}

func (self *testSystemBackend) Verify(proposal istanbul.Proposal) (*StateProcessResult, time.Duration, error) {
	if self.verifyImpl == nil {
		return self.verifyWithSuccess(proposal)
	} else {
		return self.verifyImpl(proposal)
	}
}

func (self *testSystemBackend) verifyWithSuccess(proposal istanbul.Proposal) (*StateProcessResult, time.Duration, error) {
	return nil, 0, nil
}

func (self *testSystemBackend) verifyWithFailure(proposal istanbul.Proposal) (*StateProcessResult, time.Duration, error) {
	return nil, 0, InvalidProposalError
}

func (self *testSystemBackend) verifyWithFutureProposal(proposal istanbul.Proposal) (*StateProcessResult, time.Duration, error) {
	return nil, 5, consensus.ErrFutureBlock
}

func (self *testSystemBackend) Sign(data []byte) ([]byte, error) {
	hashData := crypto.Keccak256(data)
	return crypto.Sign(hashData, &self.key)
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

func (self *testSystemBackend) GetCurrentHeadBlock() istanbul.Proposal {
	l := len(self.committedMsgs)
	if l > 0 {
		testLogger.Info("have proposal for block", "num", l)
		return self.committedMsgs[l-1].commitProposal
	}
	return makeBlock(0)
}

func (self *testSystemBackend) GetCurrentHeadBlockAndAuthor() (istanbul.Proposal, common.Address) {
	l := len(self.committedMsgs)
	if l > 0 {
		testLogger.Info("have proposal for block", "num", l)
		return self.committedMsgs[l-1].commitProposal, common.Address{}
	}
	return makeBlock(0), common.Address{}
}

func (self *testSystemBackend) LastSubject() (istanbul.Subject, error) {
	lastProposal := self.GetCurrentHeadBlock()
	lastView := &istanbul.View{Sequence: lastProposal.Number(), Round: big.NewInt(1)}
	return istanbul.Subject{View: lastView, Digest: lastProposal.Hash()}, nil
}

// Only block height 5 will return true
func (self *testSystemBackend) HasBlock(hash common.Hash, number *big.Int) bool {
	return number.Cmp(big.NewInt(5)) == 0
}

func (self *testSystemBackend) AuthorForBlock(number uint64) common.Address {
	return common.Address{}
}

func (self *testSystemBackend) ParentBlockValidators(proposal istanbul.Proposal) istanbul.ValidatorSet {
	return self.peers
}

func (self *testSystemBackend) UpdateReplicaState(seq *big.Int) { /* pass */ }

func (self *testSystemBackend) finalizeAndReturnMessage(msg *istanbul.Message) (istanbul.Message, error) {
	message := new(istanbul.Message)
	data, err := self.engine.(*core).finalizeMessage(msg)
	if err != nil {
		return *message, err
	}
	err = message.FromPayload(data, self.engine.(*core).validateFn)
	return *message, err
}

func (self *testSystemBackend) getPreprepareMessage(view istanbul.View, roundChangeCertificate istanbul.RoundChangeCertificate, proposal istanbul.Proposal) (istanbul.Message, error) {
	preprepare := &istanbul.Preprepare{
		View:                   &view,
		RoundChangeCertificate: roundChangeCertificate,
		Proposal:               proposal,
	}

	payload, err := Encode(preprepare)
	if err != nil {
		return istanbul.Message{}, err
	}

	msg := &istanbul.Message{
		Code: istanbul.MsgPreprepare,
		Msg:  payload,
	}

	return self.finalizeAndReturnMessage(msg)
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

	msg := &istanbul.Message{
		Code: istanbul.MsgPrepare,
		Msg:  payload,
	}

	return self.finalizeAndReturnMessage(msg)
}

func (self *testSystemBackend) getCommitMessage(view istanbul.View, proposal istanbul.Proposal) (istanbul.Message, error) {
	subject := &istanbul.Subject{
		View:   &view,
		Digest: proposal.Hash(),
	}

	committedSeal, err := self.engine.(*core).generateCommittedSeal(subject)
	if err != nil {
		return istanbul.Message{}, err
	}

	committedSubject := &istanbul.CommittedSubject{
		Subject:       subject,
		CommittedSeal: committedSeal[:],
	}

	payload, err := Encode(committedSubject)
	if err != nil {
		return istanbul.Message{}, err
	}

	msg := &istanbul.Message{
		Code: istanbul.MsgCommit,
		Msg:  payload,
	}

	// // We swap in the provided proposal so that the message is finalized for the provided proposal
	// // and not for the current preprepare.
	// cachePreprepare := self.engine.(*core).current.Preprepare()
	// fmt.Println("5")
	// self.engine.(*core).current.TransitionToPreprepared(&istanbul.Preprepare{
	// 	View:     &view,
	// 	Proposal: proposal,
	// })
	message, err := self.finalizeAndReturnMessage(msg)
	// self.engine.(*core).current.TransitionToPreprepared(cachePreprepare)
	return message, err
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

	msg := &istanbul.Message{
		Code: istanbul.MsgRoundChange,
		Msg:  payload,
	}

	return self.finalizeAndReturnMessage(msg)
}

func (self *testSystemBackend) Enode() *enode.Node {
	return nil
}

func (self *testSystemBackend) RefreshValPeers() error {
	return nil
}

func (self *testSystemBackend) setVerifyImpl(verifyImpl func(proposal istanbul.Proposal) (*StateProcessResult, time.Duration, error)) {
	self.verifyImpl = verifyImpl
}

// ==============================================
//
// define the struct that need to be provided for integration tests.

type testSystem struct {
	backends       []*testSystemBackend
	f              uint64
	n              uint64
	validatorsKeys [][]byte

	queuedMessage chan istanbul.MessageEvent
	quit          chan struct{}
}

func newTestSystem(n uint64, f uint64, keys [][]byte) *testSystem {
	return &testSystem{
		backends:       make([]*testSystemBackend, n),
		validatorsKeys: keys,
		f:              f,
		n:              n,

		queuedMessage: make(chan istanbul.MessageEvent),
		quit:          make(chan struct{}),
	}
}

func generateValidators(n int) ([]istanbul.ValidatorData, [][]byte, []*ecdsa.PrivateKey) {
	vals := make([]istanbul.ValidatorData, 0)
	blsKeys := make([][]byte, 0)
	keys := make([]*ecdsa.PrivateKey, 0)
	for i := 0; i < n; i++ {
		privateKey, _ := crypto.GenerateKey()
		blsPrivateKey, _ := blscrypto.ECDSAToBLS(privateKey)
		blsPublicKey, _ := blscrypto.PrivateToPublic(blsPrivateKey)
		vals = append(vals, istanbul.ValidatorData{
			Address:      crypto.PubkeyToAddress(privateKey.PublicKey),
			BLSPublicKey: blsPublicKey,
		})
		keys = append(keys, privateKey)
		blsKeys = append(blsKeys, blsPrivateKey)
	}
	return vals, blsKeys, keys
}

func newTestValidatorSet(n int) istanbul.ValidatorSet {
	validators, _, _ := generateValidators(n)
	return validator.NewSet(validators)
}

func newTestSystemWithBackend(n, f uint64) *testSystem {

	validators, blsKeys, keys := generateValidators(int(n))
	sys := newTestSystem(n, f, blsKeys)
	config := *istanbul.DefaultConfig
	config.ProposerPolicy = istanbul.RoundRobin
	config.RoundStateDBPath = ""
	config.RequestTimeout = 300
	config.TimeoutBackoffFactor = 100
	config.MinResendRoundChangeTimeout = 1000
	config.MaxResendRoundChangeTimeout = 10000

	for i := uint64(0); i < n; i++ {
		vset := validator.NewSet(validators)
		backend := sys.NewBackend(i, nil)
		backend.peers = vset
		backend.address = vset.GetByIndex(i).Address()
		backend.key = *keys[i]
		backend.blsKey = blsKeys[i]

		core := New(backend, &config).(*core)
		core.logger = testLogger
		core.validateFn = backend.CheckValidatorSignature

		backend.engine = core
	}

	return sys
}

// FIXME: int64 is needed for N and F
func NewTestSystemWithBackendDonut(n, f, epoch uint64, donutBlock int64) *testSystem {
	testLogger.SetHandler(elog.StdoutHandler)

	validators, blsKeys, keys := generateValidators(int(n))
	sys := newTestSystem(n, f, blsKeys)
	config := *istanbul.DefaultConfig
	config.ProposerPolicy = istanbul.RoundRobin
	config.RoundStateDBPath = ""
	config.RequestTimeout = 300
	config.TimeoutBackoffFactor = 100
	config.MinResendRoundChangeTimeout = 1000
	config.MaxResendRoundChangeTimeout = 10000
	config.Epoch = epoch

	for i := uint64(0); i < n; i++ {
		vset := validator.NewSet(validators)
		backend := sys.NewBackend(i, big.NewInt(donutBlock))
		backend.peers = vset
		backend.address = vset.GetByIndex(i).Address()
		backend.key = *keys[i]
		backend.blsKey = blsKeys[i]

		core := New(backend, &config).(*core)
		core.logger = testLogger
		core.validateFn = backend.CheckValidatorSignature

		backend.engine = core
	}

	return sys
}

// FIXME: int64 is needed for N and F
func NewTestSystemWithBackend(n, f uint64) *testSystem {
	testLogger.SetHandler(elog.StdoutHandler)
	return newTestSystemWithBackend(n, f)
}

// FIXME: int64 is needed for N and F
func NewMutedTestSystemWithBackend(n, f uint64) *testSystem {
	testLogger.SetHandler(elog.DiscardHandler())
	return newTestSystemWithBackend(n, f)

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
			err := b.engine.Start() // start Istanbul core
			if err != nil {
				fmt.Printf("Error Starting istanbul engine: %s", err)
				panic("Error Starting istanbul engine")
			}
		}
	}

	go t.listen()
	closer := func() { t.Stop(core) }
	return closer
}

func (t *testSystem) Stop(core bool) {
	close(t.quit)

	for _, b := range t.backends {
		if core {
			err := b.engine.Stop()
			if err != nil {
				fmt.Printf("Error Stopping istanbul engine: %s", err)
				panic("Error Stopping istanbul engine")
			}
		}
	}
}

func (t *testSystem) NewBackend(id uint64, donutBlock *big.Int) *testSystemBackend {
	// assume always success
	backend := &testSystemBackend{
		id:         id,
		sys:        t,
		events:     new(event.TypeMux),
		db:         rawdb.NewMemoryDatabase(),
		donutBlock: donutBlock,
	}

	t.backends[id] = backend
	return backend
}

func (t *testSystem) F() uint64 {
	return t.f
}

func (t *testSystem) MinQuorumSize() uint64 {
	return uint64(math.Ceil(float64(2*t.n) / 3))
}

func (sys *testSystem) getPreparedCertificate(t ErrorReporter, views []istanbul.View, proposal istanbul.Proposal) istanbul.PreparedCertificate {
	preparedCertificate := istanbul.PreparedCertificate{
		Proposal:                proposal,
		PrepareOrCommitMessages: []istanbul.Message{},
	}
	for i, backend := range sys.backends {
		if uint64(i) == sys.MinQuorumSize() {
			break
		}
		var err error
		var msg istanbul.Message
		if i%2 == 0 {
			msg, err = backend.getPrepareMessage(views[i%len(views)], proposal.Hash())
		} else {
			msg, err = backend.getCommitMessage(views[i%len(views)], proposal)
		}
		if err != nil {
			t.Errorf("Failed to create message %v: %v", i, err)
		}
		preparedCertificate.PrepareOrCommitMessages = append(preparedCertificate.PrepareOrCommitMessages, msg)
	}
	return preparedCertificate
}

func (sys *testSystem) getRoundChangeCertificate(t ErrorReporter, views []istanbul.View, preparedCertificate istanbul.PreparedCertificate) istanbul.RoundChangeCertificate {
	var roundChangeCertificate istanbul.RoundChangeCertificate
	for i, backend := range sys.backends {
		if uint64(i) == sys.MinQuorumSize() {
			break
		}
		msg, err := backend.getRoundChangeMessage(views[i%len(views)], preparedCertificate)
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
