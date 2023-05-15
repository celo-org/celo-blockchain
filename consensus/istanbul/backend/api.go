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
	"fmt"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/consensus"
	"github.com/celo-org/celo-blockchain/consensus/istanbul"
	"github.com/celo-org/celo-blockchain/consensus/istanbul/announce"
	"github.com/celo-org/celo-blockchain/consensus/istanbul/core"
	"github.com/celo-org/celo-blockchain/consensus/istanbul/validator"
	"github.com/celo-org/celo-blockchain/core/types"
	blscrypto "github.com/celo-org/celo-blockchain/crypto/bls"
	"github.com/celo-org/celo-blockchain/rpc"
)

// API is a user facing RPC API to dump Istanbul state
type API struct {
	chain    consensus.ChainHeaderReader
	istanbul *Backend
}

// getHeaderByNumber retrieves the header requested block or current if unspecified.
func (api *API) getHeaderByNumber(number *rpc.BlockNumber) (*types.Header, error) {
	var header *types.Header
	if number == nil || *number == rpc.LatestBlockNumber {
		header = api.chain.CurrentHeader()
	} else if *number == rpc.PendingBlockNumber {
		return nil, fmt.Errorf("can't use pending block within istanbul")
	} else if *number == rpc.EarliestBlockNumber {
		header = api.chain.GetHeaderByNumber(0)
	} else {
		header = api.chain.GetHeaderByNumber(uint64(*number))
	}

	if header == nil {
		return nil, errUnknownBlock
	}
	return header, nil
}

// getParentHeaderByNumber retrieves the parent header requested block or current if unspecified.
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
	return istanbul.MapValidatorsToAddresses(validators), nil
}

// GetValidatorsBLSPublicKeys retrieves the list of validators BLS public keys that must sign a given block.
func (api *API) GetValidatorsBLSPublicKeys(number *rpc.BlockNumber) ([]blscrypto.SerializedPublicKey, error) {
	header, err := api.getParentHeaderByNumber(number)
	if err != nil {
		return nil, err
	}
	validators := api.istanbul.GetValidators(header.Number, header.Hash())
	return istanbul.MapValidatorsToPublicKeys(validators), nil
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

// Retrieve the Validator Enode Table
func (api *API) GetValEnodeTable() (map[string]*announce.ValEnodeEntryInfo, error) {
	return api.istanbul.valEnodeTable.ValEnodeTableInfo()
}

func (api *API) GetVersionCertificateTableInfo() (map[string]*announce.VersionCertificateEntryInfo, error) {
	return api.istanbul.announceManager.GetVersionCertificateTableInfo()
}

// GetCurrentRoundState retrieves the current IBFT RoundState
func (api *API) GetCurrentRoundState() (*core.RoundStateSummary, error) {
	api.istanbul.coreMu.RLock()
	defer api.istanbul.coreMu.RUnlock()

	if !api.istanbul.isCoreStarted() {
		return nil, istanbul.ErrStoppedEngine
	}
	return api.istanbul.core.CurrentRoundState().Summary(), nil
}

// GetCurrentRoundChangeSet retrieves the current round change set
func (api *API) GetCurrentRoundChangeSet() (*core.RoundChangeSetSummary, error) {
	api.istanbul.coreMu.RLock()
	defer api.istanbul.coreMu.RUnlock()

	if !api.istanbul.isCoreStarted() {
		return nil, istanbul.ErrStoppedEngine
	}
	return api.istanbul.core.CurrentRoundChangeSet(), nil
}

func (api *API) ForceRoundChange() (bool, error) {
	api.istanbul.coreMu.RLock()
	defer api.istanbul.coreMu.RUnlock()

	if !api.istanbul.isCoreStarted() {
		return false, istanbul.ErrStoppedEngine
	}
	api.istanbul.core.ForceRoundChange()
	return true, nil
}

// ResendPreprepare sends again the preprepare message
func (api *API) ResendPreprepare() error {
	return api.istanbul.core.ResendPreprepare()
}

// GossipPrepares gossips the prepare messages
func (api *API) GossipPrepares() error {
	return api.istanbul.core.GossipPrepares()
}

// GossipCommits gossips the commit messages
func (api *API) GossipCommits() error {
	return api.istanbul.core.GossipCommits()
}
