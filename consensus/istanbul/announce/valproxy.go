package announce

import (
	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/p2p/enode"
)

// ValEnodeAssigmentProvider is responsible for, given a list of validator addresses,
// returning a mapping <validator address, assigned eNode>, where each validator will
// be assigned to an external eNode this node.
type ValEnodeAssigmentProvider interface {
	// GetValEnodeAssignments returns the remote validator -> external node assignments.
	// If this is a standalone validator, it will set the external node to itself.
	GetValEnodeAssignments(valAddresses []common.Address) (map[common.Address]*enode.Node, error)
}

type selfVEAP struct {
	selfNodeFn func() *enode.Node
}

func NewSelfValEnodeAssigmentProvider(selfNodeFn func() *enode.Node) ValEnodeAssigmentProvider {
	return &selfVEAP{
		selfNodeFn: selfNodeFn,
	}
}

func (p *selfVEAP) GetValEnodeAssignments(valAddresses []common.Address) (map[common.Address]*enode.Node, error) {
	var valEnodeAssignments map[common.Address]*enode.Node = make(map[common.Address]*enode.Node)
	var selfEnode *enode.Node = p.selfNodeFn()

	for _, valAddress := range valAddresses {
		valEnodeAssignments[valAddress] = selfEnode
	}

	return valEnodeAssignments, nil
}
