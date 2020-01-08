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
	vet "github.com/ethereum/go-ethereum/consensus/istanbul/backend/internal/enodes"
	"github.com/ethereum/go-ethereum/consensus/istanbul/validator"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/rpc"
)

// API is a user facing RPC API to dump Istanbul state
type API struct {
	chain    consensus.ChainReader
	istanbul *Backend
}

// getHeaderByNumber retrieves the header requested block or current if unspecified.
func (api *API) getParentHeaderByNumber(number *rpc.BlockNumber) (*types.Header, error) {
	var parent uint64
	if number == nil || *number == rpc.LatestBlockNumber || *number == rpc.PendingBlockNumber {
		head := api.chain.CurrentHeader()
		if head == nil {
			return nil, errUnknownBlock
		}
		if number == nil || *number == rpc.LatestBlockNumber {
			parent = head.Number.Uint64() - 1
		} else {
			parent = head.Number.Uint64()
		}
	} else if *number == rpc.EarliestBlockNumber {
		return nil, errUnknownBlock
	} else {
		parent = uint64(*number - 1)
	}

	header := api.chain.GetHeaderByNumber(parent)
	if header == nil {
		return nil, errUnknownBlock
	}
	return header, nil
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

// GetValidators retrieves the list validators that must sign a given block.
func (api *API) GetValidators(number *rpc.BlockNumber) ([]common.Address, error) {
	header, err := api.getParentHeaderByNumber(number)
	if err != nil {
		return nil, err
	}
	validators := api.istanbul.GetValidators(header.Number, header.Hash())
	return istanbul.GetAddressesFromValidatorList(validators), nil
}

// GetProposer retrieves the proposer for a given block number (i.e. sequence) and round.
func (api *API) GetProposer(sequence *rpc.BlockNumber, round *uint64) (common.Address, error) {
	header, err := api.getParentHeaderByNumber(sequence)
	if err != nil {
		return common.Address{}, err
	}

	valSet := api.istanbul.getOrderedValidators(header.Number.Uint64(), header.Hash())
	if valSet == nil {
		return common.Address{}, err
	}
	previousProposer, err := api.istanbul.Author(header)
	if err != nil {
		return common.Address{}, err
	}
	if round == nil {
		round = new(uint64)
	}
	proposer := validator.GetProposerSelector(api.istanbul.config.ProposerPolicy)(valSet, previousProposer, *round)
	return proposer.Address(), nil
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

// Retrieve the Validator Enode Table
func (api *API) GetValEnodeTable() (map[string]*vet.ValEnodeEntryInfo, error) {
	return api.istanbul.valEnodeTable.ValEnodeTableInfo()
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
