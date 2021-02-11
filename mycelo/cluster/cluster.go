package cluster

import (
	"context"
	"fmt"
	"log"

	"github.com/ethereum/go-ethereum/accounts/keystore"
	blscrypto "github.com/ethereum/go-ethereum/crypto/bls"
	"github.com/ethereum/go-ethereum/mycelo/console"
	"github.com/ethereum/go-ethereum/mycelo/env"
	"golang.org/x/sync/errgroup"
)

type Cluster struct {
	env      *env.Environment
	gethPath string

	nodes []*Node
}

var scryptN = keystore.LightScryptN
var scryptP = keystore.LightScryptP

func New(env *env.Environment, gethPath string) *Cluster {
	return &Cluster{
		env:      env,
		gethPath: gethPath,
	}
}

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
		validators := cl.env.ValidatorAccounts()
		cl.nodes = make([]*Node, len(validators))
		for i, validator := range validators {
			nodeConfig := &NodeConfig{
				GethPath: cl.gethPath,
				Number:   i,
				Account:  validator,
				Datadir:  cl.env.ValidatorDatadir(i),
				ChainID:  cl.env.Config.ChainID,
			}
			cl.nodes[i] = NewNode(nodeConfig)
		}
	}
	return cl.nodes
}

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

func toSerializedBlsPublicKey(bs []byte) blscrypto.SerializedPublicKey {
	if len(bs) != blscrypto.PUBLICKEYBYTES {
		log.Fatal("Invalid bls key size")
	}
	key := blscrypto.SerializedPublicKey{}
	for i, b := range bs {
		key[i] = b
	}
	return key
}
