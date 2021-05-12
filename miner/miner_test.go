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

package miner

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/celo-org/celo-blockchain/core/types"
	"github.com/celo-org/celo-blockchain/ethclient"
	"github.com/celo-org/celo-blockchain/mycelo/celotest"
)

func waitForNBlocks(client *ethclient.Client, N int, perBlockTimeout, totalTimeout time.Duration) error {
	newChainHeadCh := make(chan *types.Header, 10)
	newChainHeadSub, err := client.SubscribeNewHead(context.TODO(), newChainHeadCh)
	if err != nil {
		return fmt.Errorf("Failed to subscribe to new chain head: %w", err)
	}
	defer newChainHeadSub.Unsubscribe()

	count := 0
	totalTimer := time.After(totalTimeout)
	for {
		select {
		case <-newChainHeadCh:
			count++
			if count == N {
				return nil
			}
		case err := <-newChainHeadSub.Err():
			return fmt.Errorf("New Chain Head subscription failed: %w", err)
		case <-time.After(perBlockTimeout):
			return errors.New("Timed out from the previous block while waiting for new blocks")
		case <-totalTimer:
			return errors.New("Time out on the total timeout while waiting for new blocks")
		}
	}
}

func TestProducesBlocksWithLoad(t *testing.T) {
	network, err := celotest.NewMyceloNetwork(t.TempDir())
	if err != nil {
		t.Fatalf("Failed to create a mycelo network: %v", err)
	}

	network.Run()
	defer network.Wait()
	defer network.Shutdown()

	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	clients, err := network.Clients(ctx)
	if err != nil {
		t.Fatalf("Failed to connect to the network: %v", err)
	}

	err = waitForNBlocks(clients[0], 2, 1500*time.Millisecond, 3*time.Second)
	if err != nil {
		t.Fatalf("Failed to mine blocks at the start of the network: %v", err)
	}

	ctx, cancel = context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err = network.SendAndWaitForNTransactions(ctx, 10, clients)
	if err != nil {
		t.Fatalf("Failed to mine transactions in the network: %v", err)
	}

	waitForNBlocks(clients[0], 2, 1500*time.Millisecond, 3*time.Second)
	if err != nil {
		t.Fatalf("Failed to mine blocks after including transactions in the network: %v", err)
	}

}
