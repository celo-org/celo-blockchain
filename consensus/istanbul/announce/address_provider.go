package announce

import (
	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/p2p/enode"
)

// AddressProvider provides the different addresses the announce manager needs
type AddressProvider interface {
	SelfNode() *enode.Node
	ValidatorAddress() common.Address
}
