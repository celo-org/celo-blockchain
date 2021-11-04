package core

import (
	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/consensus/istanbul"
)

type MessageSender interface {
	Send(payload []byte, addresses []common.Address) error
}

type DefaultMessageSender struct {
	backend CoreBackend
}

func NewDefaultMessageSender() *DefaultMessageSender {
	return &DefaultMessageSender{}
}

func (s *DefaultMessageSender) Send(payload []byte, addresses []common.Address) error {
	return s.backend.Multicast(addresses, payload, istanbul.ConsensusMsg, true)
}

func (s *DefaultMessageSender) setBackend(backend CoreBackend) {
	s.backend = backend
}
