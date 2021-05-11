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
	"math/big"
	"os"
	"path"
	"time"

	"github.com/celo-org/celo-blockchain/ethclient"
	"github.com/celo-org/celo-blockchain/mycelo/cluster"
	"github.com/celo-org/celo-blockchain/mycelo/env"
	"github.com/celo-org/celo-blockchain/mycelo/genesis"
	"github.com/celo-org/celo-blockchain/mycelo/loadbot"
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
	env.Config.Accounts.NumDeveloperAccounts = 100
	genesisConfig, _ := template.CreateGenesisConfig(env)
	genesisConfig.Istanbul.BlockPeriod = 1

	generatedGenesis, _ := genesis.GenerateGenesis(env.Accounts(), genesisConfig, contractsPath())

	env.SaveGenesis(generatedGenesis)
	env.Save()
	// 2. Init validator to temp datadir
	clusterCfg := cluster.Config{
		GethPath: gethPath(),
	}

	cluster := cluster.New(env, clusterCfg)
	fmt.Println("created new cluster")

	if err := cluster.Init(); err != nil {
		return nil, fmt.Errorf("error running init: %w", err)
	}
	fmt.Println("initialized cluster")

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

func (n *network) WaitForIPCToBeAvailable(ctx context.Context) error {
	ticker := time.NewTicker(50 * time.Millisecond)
	for {
		select {
		case <-ticker.C:
			done := true
			for i := 0; i < n.env.Accounts().NumValidators; i++ {
				path := n.env.ValidatorIPC(i)
				if _, err := os.Stat(path); err != nil {
					done = false
				}
				if done {
					return nil
				}
			}
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// Dials each client on ws. WARNING: hardcodes the port
// Note: will hang until it can connect or the context is cancelled.
func (n *network) Clients(ctx context.Context) ([]*ethclient.Client, error) {
	ticker := time.NewTicker(50 * time.Millisecond)
	var clients []*ethclient.Client
	for i := 0; i < n.env.Accounts().NumValidators; i++ {
	retryLoop:
		for {
			select {
			case <-ticker.C:
				client, err := ethclient.Dial(fmt.Sprintf("ws://localhost:%v", 8546+i))
				if err == nil {
					clients = append(clients, client)
					break retryLoop
				}
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
	}
	return clients, nil
}

func (n *network) SendAndWaitForNTransactions(ctx context.Context, N int, clients []*ethclient.Client) error {
	lg := &loadbot.LoadGenerator{}
	accounts := n.env.Accounts().DeveloperAccounts()
	// Offset the receiver from the sender so that they are different
	recvIdx := len(accounts) / 2
	sendIdx := 0
	clientIdx := 0
	nonces := make([]uint64, len(accounts))

	group, ctx := errgroup.WithContext(ctx)
	for i := 0; i < N; i++ {
		// We use round robin selectors that rollover
		recvIdx++
		recipient := accounts[recvIdx%len(accounts)].Address

		sendIdx++
		sender := accounts[sendIdx%len(accounts)]
		nonce := nonces[sendIdx%len(accounts)]
		nonces[sendIdx%len(accounts)]++

		clientIdx++
		client := clients[clientIdx%len(clients)]
		group.Go(func() error {
			txCfg := loadbot.TxConfig{
				Acc:               sender,
				Nonce:             nonce,
				Recipient:         recipient,
				Value:             big.NewInt(10000000),
				Verbose:           false,
				SkipGasEstimation: false,
				MixFeeCurrency:    true,
			}
			return loadbot.RunTransaction(ctx, client, lg, txCfg)
		})
	}

	return group.Wait()
}
