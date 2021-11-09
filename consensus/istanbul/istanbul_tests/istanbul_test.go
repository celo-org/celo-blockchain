package istanbul_tests

import (
	"fmt"
	"testing"
	"time"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/consensus/istanbul"
	"github.com/celo-org/celo-blockchain/consensus/istanbul/core"
	"github.com/celo-org/celo-blockchain/crypto"
	"github.com/celo-org/celo-blockchain/mycelo/env"
	"github.com/celo-org/celo-blockchain/test"
	"github.com/stretchr/testify/require"
	"gotest.tools/assert"
)

// CapturingMessageSender is a sender implementation that cpatures messages
// sent on a channel.
type CapturingMessageSender struct {
	Msgs chan MsgAndDest
}

// MsgAndDest simply wraps messages and the target destinations so that they
// can be sent as one down a channel.
type MsgAndDest struct {
	Msg  *istanbul.Message
	Dest []common.Address
}

func NewCapturingMessageSender() *CapturingMessageSender {
	return &CapturingMessageSender{
		Msgs: make(chan MsgAndDest),
	}
}

func (s *CapturingMessageSender) Send(payload []byte, addresses []common.Address) error {
	msg := new(istanbul.Message)
	err := msg.FromPayload(payload, nil)
	if err != nil {
		panic(fmt.Sprintf("Failed to decode message after sending: %v", err))
	}
	mnd := MsgAndDest{
		Msg:  msg,
		Dest: addresses,
	}
	s.Msgs <- mnd
	return nil
}

type Timer interface {
	AfterFunc(time.Duration, func())
	Stop()
}

func NewTestTimer() *TestTimer {
	return &TestTimer{}
}

type TestTimer struct {
	F func()
}

func (t *TestTimer) AfterFunc(_ time.Duration, f func()) {
	t.F = f
}

func (t *TestTimer) Stop() {
	t.F = nil
}

func NewTestTimers() *core.Timers {
	return &core.Timers{
		RoundChange:       NewTestTimer(),
		ResendRoundChange: NewTestTimer(),
		FuturePreprepare:  NewTestTimer(),
	}
}

func TestCommit(t *testing.T) {
	ac := test.AccountConfig(3, 0)
	gc, ec, err := test.BuildConfig(ac)
	require.NoError(t, err)
	genesis, err := test.ConfigureGenesis(ac, gc, ec)
	require.NoError(t, err)

	sender := NewCapturingMessageSender()
	n, err := test.NewNode(&ac.ValidatorAccounts()[0], test.BaseNodeConfig, ec, genesis, sender, NewTestTimers())
	require.NoError(t, err)
	defer n.Close()

	if n.Core.CurrentRoundState().IsProposer(ac.ValidatorAccounts()[0].Address) {
		// send our proposeal back to us
		mnd := <-sender.Msgs
		require.Equal(t, istanbul.MsgPreprepare, mnd.Msg.Code)
		err := n.HandleConsensusMessage(mnd.Msg)
		require.NoError(t, err)

		m := <-sender.Msgs
		println(m.Msg.Code)
		// if mnd.Msg.Code
		//Make prepare messages
		// s := n.Core.CurrentRoundState().Subject()
		// m1 := istanbul.NewPrepareMessage(s, accounts.ValidatorAccounts()[1].Address)
		// m2 := istanbul.NewPrepareMessage(s, accounts.ValidatorAccounts()[2].Address)

	} else {
		// Send a proposal
		r := n.Core.CurrentRoundState().PendingRequest()
		pp := &istanbul.Preprepare{
			View:                   n.Core.CurrentView(),
			Proposal:               r.Proposal,
			RoundChangeCertificate: istanbul.RoundChangeCertificate{},
		}
		proposerAdress := n.Core.CurrentRoundState().Proposer().Address()
		m := istanbul.NewPreprepareMessage(pp, proposerAdress)
		accountMap := make(map[common.Address]env.Account)
		for _, a := range ac.ValidatorAccounts() {
			accountMap[a.Address] = a
		}
		sf := func(data []byte) ([]byte, error) {
			hashData := crypto.Keccak256(data)
			return crypto.Sign(hashData, accountMap[proposerAdress].PrivateKey)
		}
		err = m.Sign(sf)
		require.NoError(t, err)
		// Send preprepare
		err := n.HandleConsensusMessage(m)
		require.NoError(t, err)
		prepare := <-sender.Msgs
		require.Equal(t, istanbul.MsgPrepare, prepare.Msg.Code)
		// Handle own prepare
		err = n.HandleConsensusMessage(prepare.Msg)
		require.NoError(t, err)

		// Handle other prepare
		otherPrepare := istanbul.NewPrepareMessage(prepare.Msg.Prepare(), proposerAdress)
		err = otherPrepare.Sign(sf)
		require.NoError(t, err)
		err = n.HandleConsensusMessage(otherPrepare)
		require.NoError(t, err)

	}

	m := <-sender.Msgs
	assert.Equal(t, istanbul.MsgCommit, m.Msg.Code)
}
