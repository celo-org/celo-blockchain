// Copyright 2017 The go-ethereum Authors
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

package backend

import (
	"errors"
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/consensus/istanbul"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/rpc"
)

// API is a user facing RPC API to dump Istanbul state
type API struct {
	chain    consensus.ChainReader
	istanbul *Backend
}

// GetSnapshot retrieves the state snapshot at a given block.
func (api *API) GetSnapshot(number *rpc.BlockNumber) (*Snapshot, error) {
	// Retrieve the requested block number (or current if none requested)
	var header *types.Header
	if number == nil || *number == rpc.LatestBlockNumber {
		header = api.chain.CurrentHeader()
	} else {
		header = api.chain.GetHeaderByNumber(uint64(number.Int64()))
	}
	// Ensure we have an actually valid block and return its snapshot
	if header == nil {
		return nil, errUnknownBlock
	}
	return api.istanbul.snapshot(api.chain, header.Number.Uint64(), header.Hash(), nil)
}

// GetSnapshotAtHash retrieves the state snapshot at a given block.
func (api *API) GetSnapshotAtHash(hash common.Hash) (*Snapshot, error) {
	header := api.chain.GetHeaderByHash(hash)
	if header == nil {
		return nil, errUnknownBlock
	}
	return api.istanbul.snapshot(api.chain, header.Number.Uint64(), header.Hash(), nil)
}

// GetValidators retrieves the list of authorized validators at the specified block.
func (api *API) GetValidators(number *rpc.BlockNumber) ([]common.Address, error) {
	// Retrieve the requested block number (or current if none requested)
	var header *types.Header
	if number == nil || *number == rpc.LatestBlockNumber {
		header = api.chain.CurrentHeader()
	} else {
		header = api.chain.GetHeaderByNumber(uint64(number.Int64()))
	}
	// Ensure we have an actually valid block and return the validators from its snapshot
	if header == nil {
		return nil, errUnknownBlock
	}
	snap, err := api.istanbul.snapshot(api.chain, header.Number.Uint64(), header.Hash(), nil)
	if err != nil {
		return nil, err
	}
	validators := snap.validators()
	validatorsAddresses, _ := istanbul.SeparateValidatorDataIntoIstanbulExtra(validators)
	return validatorsAddresses, nil
}

// GetValidatorsAtHash retrieves the state snapshot at a given block.
func (api *API) GetValidatorsAtHash(hash common.Hash) ([]common.Address, error) {
	header := api.chain.GetHeaderByHash(hash)
	if header == nil {
		return nil, errUnknownBlock
	}
	snap, err := api.istanbul.snapshot(api.chain, header.Number.Uint64(), header.Hash(), nil)
	if err != nil {
		return nil, err
	}
	validators := snap.validators()
	validatorsAddresses, _ := istanbul.SeparateValidatorDataIntoIstanbulExtra(validators)
	return validatorsAddresses, nil
}

// AddProxy peers with a remote node that acts as a proxy, even if slots are full
func (api *API) AddProxy(url, externalUrl string) (bool, error) {
	if !api.istanbul.config.Proxied {
		api.istanbul.logger.Error("Add proxy node failed: this node is not configured to be proxied")
		return false, errors.New("Can't add proxy for node that is not configured to be proxied")
	}

	node, err := enode.ParseV4(url)
	if err != nil {
		return false, fmt.Errorf("invalid enode: %v", err)
	}

	externalNode, err := enode.ParseV4(externalUrl)
	if err != nil {
		return false, fmt.Errorf("invalid external enode: %v", err)
	}

	err = api.istanbul.addProxy(node, externalNode)
	return true, err
}

// RemoveProxy removes a node from acting as a proxy
func (api *API) RemoveProxy(url string) (bool, error) {
	// Try to remove the url as a proxy and return
	node, err := enode.ParseV4(url)
	if err != nil {
		return false, fmt.Errorf("invalid enode: %v", err)
	}
	api.istanbul.removeProxy(node)
	return true, nil
}

// TODO(kevjue) - implement this
// ProxyInfo retrieves all the information we know about each individual proxy node
/* func (api *PublicAdminAPI) ProxyInfo() ([]*p2p.PeerInfo, error) {
	server := api.node.Server()
	if server == nil {
		return nil, ErrNodeStopped
	}
	return server.ProxyInfo(), nil
} */
