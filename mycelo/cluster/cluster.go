package cluster

import (
	"context"
	"fmt"
	"log"

	"github.com/celo-org/celo-blockchain/mycelo/env"
	"github.com/celo-org/celo-blockchain/mycelo/internal/console"
	"golang.org/x/sync/errgroup"
)

// Cluster represent a set of nodes (validators)
// that are managed together
type Cluster struct {
	env    *env.Environment
	config Config

	nodes []*Node
}

type Config struct {
	GethPath   string
	ExtraFlags string
}

// New creates a new cluster instance
func New(env *env.Environment, cfg Config) *Cluster {
	return &Cluster{
		env:    env,
		config: cfg,
	}
}

// Init will initialize the nodes
// This implies running `geth init` but also
// configuring static nodes and node accounts
func (cl *Cluster) Init() error {
	var err error

	nodes := cl.ensureNodes()
	enodeUrls := make([]string, len(nodes))
	console.Info("Initializing validator nodes")
	for i, node := range nodes {
		console.Infof("validator-%d> geth init", i)
		if err := node.Init(cl.env.GenesisPath()); err != nil {
			return err
		}

		enodeUrls[i], err = node.EnodeURL()
		if err != nil {
			return err
		}
	}

	// Connect each validator to each other
	for i, node := range nodes {
		var urls []string
		urls = append(urls, enodeUrls[:i]...)
		urls = append(urls, enodeUrls[i+1:]...)
		err = node.SetStaticNodes(urls...)
		if err != nil {
			return err
		}
	}

	return nil
}

func (cl *Cluster) ensureNodes() []*Node {

	if cl.nodes == nil {
		validators := cl.env.Accounts().ValidatorAccounts()
		txFeeRecipients := cl.env.Accounts().TxFeeRecipientAccounts()
		cl.nodes = make([]*Node, len(validators))
		for i, validator := range validators {
			nodeConfig := &NodeConfig{
				GethPath:              cl.config.GethPath,
				ExtraFlags:            cl.config.ExtraFlags,
				Number:                i,
				Account:               validator,
				TxFeeRecipientAccount: txFeeRecipients[i],
				Datadir:               cl.env.ValidatorDatadir(i),
				ChainID:               cl.env.Config.ChainID,
			}
			cl.nodes[i] = NewNode(nodeConfig)
		}
	}
	return cl.nodes
}

// PrintNodeInfo prints debug information about nodes
func (cl *Cluster) PrintNodeInfo() error {
	for i, node := range cl.ensureNodes() {
		endoreURL, err := node.EnodeURL()
		if err != nil {
			return err
		}
		fmt.Printf("validator-%d: %s\n", i, endoreURL)
	}
	return nil
}

// Run will run all the cluster nodes
func (cl *Cluster) Run(ctx context.Context) error {
	group, ctx := errgroup.WithContext(ctx)
	log.Printf("Starting cluster")
	for i, node := range cl.ensureNodes() {
		node := node
		i := i
		log.Printf("Starting validator%02d...", i)
		group.Go(func() error { return node.Run(ctx) })
	}
	return group.Wait()
}
