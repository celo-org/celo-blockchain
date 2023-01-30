package announce

import (
	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/consensus/istanbul"
)

// Network manages the communication needed for the announce protocol to work.
type Network interface {
	// Gossip gossips protocol messages
	Gossip(payload []byte, ethMsgCode uint64) error
	// RetrieveValidatorConnSet returns the validator connection set
	RetrieveValidatorConnSet() (map[common.Address]bool, error)
	// Multicast will send the eth message (with the message's payload and msgCode field set to the params
	// payload and ethMsgCode respectively) to the nodes with the signing address in the destAddresses param.
	Multicast(destAddresses []common.Address, msg *istanbul.Message, ethMsgCode uint64, sendToSelf bool) error
}
