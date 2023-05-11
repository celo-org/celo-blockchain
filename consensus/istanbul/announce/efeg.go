package announce

import (
	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/p2p/enode"
)

type ExternalFacingEnodeGetter interface {
	// GetEnodeCertNodesAndDestAddresses will retrieve all the external facing external nodes for this validator
	// (itself for standalone validators) for the purposes of generating enode certificates
	// for those enodes.  It will also return the destination validators for each enode certificate.  If the destAddress is a
	// `nil` value, then that means that the associated enode certificate should be sent to all of the connected validators.
	GetEnodeCertNodesAndDestAddresses() ([]*enode.Node, map[enode.ID][]common.Address, error)
}

func NewSelfExternalFacingEnodeGetter(selfNodeFn func() *enode.Node) ExternalFacingEnodeGetter {
	return &selfEFEG{selfNode: selfNodeFn}
}

type selfEFEG struct {
	selfNode func() *enode.Node
}

func (s *selfEFEG) GetEnodeCertNodesAndDestAddresses() ([]*enode.Node, map[enode.ID][]common.Address, error) {
	self := s.selfNode()
	valDestinations := make(map[enode.ID][]common.Address)
	valDestinations[self.ID()] = nil
	return []*enode.Node{self}, valDestinations, nil
}
