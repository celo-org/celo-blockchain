package istanbul_tests

import (
	"fmt"
	"testing"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/consensus/istanbul"
	"github.com/celo-org/celo-blockchain/test"
	"github.com/stretchr/testify/require"
)

// CapturingMessageSender is a sender implementation that cpatures messages
// sent on a channel.
type CapturingMessageSender struct {
	msgs chan msgAndDest
}

// msgAndDest simply wraps messages and the target destinations so that they
// can be sent as one down a channel.
type msgAndDest struct {
	msg  *istanbul.Message
	dest []common.Address
}

func NewCapturingMessageSender() *CapturingMessageSender {
	return &CapturingMessageSender{
		msgs: make(chan msgAndDest),
	}
}

func (s *CapturingMessageSender) Send(payload []byte, addresses []common.Address) error {
	msg := new(istanbul.Message)
	err := msg.FromPayload(payload, nil)
	if err != nil {
		panic(fmt.Sprintf("Failed to decode message after sending: %v", err))
	}
	mnd := msgAndDest{
		msg:  msg,
		dest: addresses,
	}
	s.msgs <- mnd
	return nil
}

func TestIstanbul(t *testing.T) {
	accounts := test.Accounts(3)
	gc, ec, err := test.BuildConfig(accounts)
	require.NoError(t, err)
	genesis, err := test.ConfigureGenesis(accounts, gc, ec)
	require.NoError(t, err)

	sender := NewCapturingMessageSender()
	n, err := test.NewNode(&accounts.ValidatorAccounts()[0], &accounts.DeveloperAccounts()[0], test.BaseNodeConfig, ec, genesis, sender)
	require.NoError(t, err)
	defer n.Close()

	m := <-sender.msgs
	println(m.msg.Code)
}
