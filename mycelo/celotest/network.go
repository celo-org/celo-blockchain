// Copyright 2021 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package celotest

import (
	"context"
	"fmt"
	"os"
	"path"

	"github.com/celo-org/celo-blockchain/ethclient"
	"github.com/celo-org/celo-blockchain/mycelo/cluster"
	"github.com/celo-org/celo-blockchain/mycelo/env"
	"github.com/celo-org/celo-blockchain/mycelo/genesis"
	"github.com/celo-org/celo-blockchain/mycelo/templates"
	"golang.org/x/sync/errgroup"
)

type network struct {
	env      *env.Environment
	nodes    *cluster.Cluster
	group    *errgroup.Group
	shutdown context.CancelFunc
}

func gethPath() string {
	geth := path.Join(os.Getenv("CELO_BLOCKCHAIN"), "build/bin/geth")
	return geth
}

func contractsPath() string {
	contracts := path.Join(os.Getenv("CELO_MONOREPO"), "packages/protocol/build/contracts")
	return contracts
}

func NewMyceloNetwork(tempDir string) (*network, error) {
	// 1. Create genesis & environment
	template := templates.TemplateFromString("loadtest")
	env, _ := template.CreateEnv(tempDir)
	genesisConfig, _ := template.CreateGenesisConfig(env)

	generatedGenesis, _ := genesis.GenerateGenesis(env.Accounts(), genesisConfig, contractsPath())

	env.SaveGenesis(generatedGenesis)
	env.Save()
	// 2. Init validator to temp datadir
	clusterCfg := cluster.Config{
		GethPath: gethPath(),
	}

	cluster := cluster.New(env, clusterCfg)

	if err := cluster.Init(); err != nil {
		return nil, fmt.Errorf("error running init: %w", err)
	}

	ret := &network{
		env:   env,
		nodes: cluster,
	}
	return ret, nil
}

func (n *network) Run() {
	ctx, shutdown := context.WithCancel(context.Background())
	group, runCtx := errgroup.WithContext(ctx)
	group.Go(func() error { return n.nodes.Run(runCtx) })

	// Save context and shutdown function for later
	n.group = group
	n.shutdown = shutdown
}

func (n *network) Shutdown() {
	n.shutdown()
}

func (n *network) Wait() error {
	return (*n.group).Wait()
}

func (n *network) Clients() []*ethclient.Client {
	var clients []*ethclient.Client
	for i := 0; i < n.env.Accounts().NumValidators; i++ {
		client, err := ethclient.Dial(n.env.ValidatorIPC(i))
		if err != nil {
			panic(fmt.Sprintf("Failed to dial the node: %v", err))
		}
		clients = append(clients, client)
	}
	return clients
}
