package backend

import (
	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/consensus/istanbul/proxy"
	"github.com/celo-org/celo-blockchain/p2p/enode"
)

type ValProxyAssigmnentProvider interface {
	// GetValProxyAssignments returns the remote validator -> external node assignments.
	// If this is a standalone validator, it will set the external node to itself.
	// If this is a proxied validator, it will set external node to the proxy's external node.
	GetValProxyAssignments(valAddresses []common.Address) (map[common.Address]*enode.Node, error)
}

type proxyVPAP struct {
	proxyAssigmentsFn func([]common.Address) (map[common.Address]*proxy.Proxy, error)
}

func NewProxiedValProxyAssigmentProvider(proxyAssigmentsFn func([]common.Address) (map[common.Address]*proxy.Proxy, error)) ValProxyAssigmnentProvider {
	return &proxyVPAP{
		proxyAssigmentsFn: proxyAssigmentsFn,
	}
}

func (p *proxyVPAP) GetValProxyAssignments(valAddresses []common.Address) (map[common.Address]*enode.Node, error) {
	var valProxyAssignments map[common.Address]*enode.Node = make(map[common.Address]*enode.Node)
	var proxies map[common.Address]*proxy.Proxy // This var is only used if this is a proxied validator

	for _, valAddress := range valAddresses {
		var externalNode *enode.Node

		if proxies == nil {
			var err error
			proxies, err = p.proxyAssigmentsFn(nil)
			if err != nil {
				return nil, err
			}
		}
		proxyObj := proxies[valAddress]
		if proxyObj == nil {
			continue
		}

		externalNode = proxyObj.ExternalNode()

		valProxyAssignments[valAddress] = externalNode
	}

	return valProxyAssignments, nil
}

type selfVPAP struct {
	selfNodeFn func() *enode.Node
}

func NewSelfValProxyAssigmentProvider(selfNodeFn func() *enode.Node) ValProxyAssigmnentProvider {
	return &selfVPAP{
		selfNodeFn: selfNodeFn,
	}
}

func (p *selfVPAP) GetValProxyAssignments(valAddresses []common.Address) (map[common.Address]*enode.Node, error) {
	var valProxyAssignments map[common.Address]*enode.Node = make(map[common.Address]*enode.Node)
	var selfEnode *enode.Node = p.selfNodeFn()

	for _, valAddress := range valAddresses {
		valProxyAssignments[valAddress] = selfEnode
	}

	return valProxyAssignments, nil
}
