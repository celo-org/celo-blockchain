package backend

import (
	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/consensus/istanbul/proxy"
	"github.com/celo-org/celo-blockchain/p2p/enode"
)

type ExternalFacingEnodeGetter interface {
	GetEnodeCertNodesAndDestAddresses() ([]*enode.Node, map[enode.ID][]common.Address, error)
}

func NewSelfExternalFacingEnodeGetter(selfNodeFn func() *enode.Node) ExternalFacingEnodeGetter {
	return &selfEFEG{selfNode: selfNodeFn}
}

func NewProxiedExternalFacingEnodeGetter(getProxiesAndValAssignmentsFn func() ([]*proxy.Proxy, map[enode.ID][]common.Address, error)) ExternalFacingEnodeGetter {
	return &proxiedEFEG{getProxiesAndValAssignments: getProxiesAndValAssignmentsFn}
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

type proxiedEFEG struct {
	getProxiesAndValAssignments func() ([]*proxy.Proxy, map[enode.ID][]common.Address, error)
}

func (p *proxiedEFEG) GetEnodeCertNodesAndDestAddresses() ([]*enode.Node, map[enode.ID][]common.Address, error) {
	proxies, valDestinations, err := p.getProxiesAndValAssignments()
	if err != nil {
		return nil, nil, err
	}

	externalEnodes := make([]*enode.Node, len(proxies))
	for i, proxy := range proxies {
		externalEnodes[i] = proxy.ExternalNode()
	}
	return externalEnodes, valDestinations, nil
}
