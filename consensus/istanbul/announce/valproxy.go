package announce

import (
	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/p2p/enode"
)

// ValProxyAssigmentProvider is responsible for, given a list of validator addresses,
// returning a mapping <validator address, assigned eNode>, where each validator will
// be assigned to an external eNode this node.
// E.g if this node has two proxies: A and B, it is possible to assign A to a set of
// validators, and B to another set, therefore splitting the load of validator to
// validator connections through these proxies instances.
// If this node has no proxy, then all values should be the self eNode.
type ValProxyAssigmnentProvider interface {
	// GetValProxyAssignments returns the remote validator -> external node assignments.
	// If this is a standalone validator, it will set the external node to itself.
	// If this is a proxied validator, it will set external node to the proxy's external node.
	GetValProxyAssignments(valAddresses []common.Address) (map[common.Address]*enode.Node, error)
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
