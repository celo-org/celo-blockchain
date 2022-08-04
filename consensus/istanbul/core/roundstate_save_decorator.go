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

package core

import (
	"math/big"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/consensus/istanbul"
)

// createOrRestoreRoundState will obtain the last saved RoundState and use it if it's newer than the given Sequence,
// if not it will create a new one with the information given.
func withSavingDecorator(db RoundStateDB, rs RoundState) RoundState {
	return &rsSaveDecorator{
		db: db,
		rs: rs,
	}
}

type rsSaveDecorator struct {
	rs RoundState
	db RoundStateDB
}

func (rsp *rsSaveDecorator) persistOnNoError(err error) error {
	if err != nil {
		return err
	}

	return rsp.db.UpdateLastRoundState(rsp.rs)
}

// mutation functions
func (rsp *rsSaveDecorator) StartNewRound(nextRound *big.Int, validatorSet istanbul.ValidatorSet, nextProposer istanbul.Validator) error {
	return rsp.persistOnNoError(rsp.rs.StartNewRound(nextRound, validatorSet, nextProposer))
}
func (rsp *rsSaveDecorator) StartNewSequence(nextSequence *big.Int, validatorSet istanbul.ValidatorSet, nextProposer istanbul.Validator, parentCommits MessageSet) error {
	return rsp.persistOnNoError(rsp.rs.StartNewSequence(nextSequence, validatorSet, nextProposer, parentCommits))
}
func (rsp *rsSaveDecorator) TransitionToPreprepared(preprepare *istanbul.Preprepare) error {
	return rsp.persistOnNoError(rsp.rs.TransitionToPreprepared(preprepare))
}
func (rsp *rsSaveDecorator) TransitionToWaitingForNewRound(r *big.Int, nextProposer istanbul.Validator) error {
	return rsp.persistOnNoError(rsp.rs.TransitionToWaitingForNewRound(r, nextProposer))
}
func (rsp *rsSaveDecorator) TransitionToCommitted() error {
	return rsp.persistOnNoError(rsp.rs.TransitionToCommitted())
}
func (rsp *rsSaveDecorator) TransitionToPrepared(quorumSize int) error {
	return rsp.persistOnNoError(rsp.rs.TransitionToPrepared(quorumSize))
}
func (rsp *rsSaveDecorator) AddCommit(msg *istanbul.Message) error {
	return rsp.persistOnNoError(rsp.rs.AddCommit(msg))
}
func (rsp *rsSaveDecorator) AddPrepare(msg *istanbul.Message) error {
	return rsp.persistOnNoError(rsp.rs.AddPrepare(msg))
}
func (rsp *rsSaveDecorator) AddParentCommit(msg *istanbul.Message) error {
	return rsp.persistOnNoError(rsp.rs.AddParentCommit(msg))
}
func (rsp *rsSaveDecorator) SetPendingRequest(pendingRequest *istanbul.Request) error {
	return rsp.persistOnNoError(rsp.rs.SetPendingRequest(pendingRequest))
}
func (rsp *rsSaveDecorator) SetProposalVerificationStatus(proposalHash common.Hash, verificationStatus error) {
	// Don't persist on proposal verification status change, since it's just a cache
	rsp.rs.SetProposalVerificationStatus(proposalHash, verificationStatus)
}
func (rsp *rsSaveDecorator) SetStateProcessResult(proposalHash common.Hash, result *StateProcessResult) {
	rsp.rs.SetStateProcessResult(proposalHash, result)
}

// DesiredRound implements RoundState.DesiredRound
func (rsp *rsSaveDecorator) DesiredRound() *big.Int { return rsp.rs.DesiredRound() }

// State implements RoundState.State
func (rsp *rsSaveDecorator) State() State { return rsp.rs.State() }

// Proposer implements RoundState.Proposer
func (rsp *rsSaveDecorator) Proposer() istanbul.Validator { return rsp.rs.Proposer() }

// Subject implements RoundState.Subject
func (rsp *rsSaveDecorator) Subject() *istanbul.Subject { return rsp.rs.Subject() }

// Proposal implements RoundState.Proposal
func (rsp *rsSaveDecorator) Proposal() istanbul.Proposal { return rsp.rs.Proposal() }

// Round implements RoundState.Round
func (rsp *rsSaveDecorator) Round() *big.Int { return rsp.rs.Round() }

// Commits implements RoundState.Commits
func (rsp *rsSaveDecorator) Commits() MessageSet { return rsp.rs.Commits() }

// Prepares implements RoundState.Prepares
func (rsp *rsSaveDecorator) Prepares() MessageSet { return rsp.rs.Prepares() }

// ParentCommits implements RoundState.ParentCommits
func (rsp *rsSaveDecorator) ParentCommits() MessageSet { return rsp.rs.ParentCommits() }

// Sequence implements RoundState.Sequence
func (rsp *rsSaveDecorator) Sequence() *big.Int { return rsp.rs.Sequence() }

// View implements RoundState.View
func (rsp *rsSaveDecorator) View() *istanbul.View { return rsp.rs.View() }

// Preprepare implements RoundState.Preprepare
func (rsp *rsSaveDecorator) Preprepare() *istanbul.Preprepare { return rsp.rs.Preprepare() }

// PendingRequest implements RoundState.PendingRequest
func (rsp *rsSaveDecorator) PendingRequest() *istanbul.Request { return rsp.rs.PendingRequest() }

// ValidatorSet implements RoundState.ValidatorSet
func (rsp *rsSaveDecorator) ValidatorSet() istanbul.ValidatorSet { return rsp.rs.ValidatorSet() }

// GetValidatorByAddress implements RoundState.GetValidatorByAddress
func (rsp *rsSaveDecorator) GetValidatorByAddress(address common.Address) istanbul.Validator {
	return rsp.rs.GetValidatorByAddress(address)
}

// GetProposalVerificationStatus implements RoundState.GetProposalVerificationStatus
func (rsp *rsSaveDecorator) GetProposalVerificationStatus(proposalHash common.Hash) (verificationStatus error, isChecked bool) {
	return rsp.rs.GetProposalVerificationStatus(proposalHash)
}

// GetStateProcessResult implements RoundState.GetStateProcessResult
func (rsp *rsSaveDecorator) GetStateProcessResult(proposalHash common.Hash) (result *StateProcessResult) {
	return rsp.rs.GetStateProcessResult(proposalHash)
}

// IsProposer implements RoundState.IsProposer
func (rsp *rsSaveDecorator) IsProposer(address common.Address) bool {
	return rsp.rs.IsProposer(address)
}

// PreparedCertificate implements RoundState.PreparedCertificate
func (rsp *rsSaveDecorator) PreparedCertificate() istanbul.PreparedCertificate {
	return rsp.rs.PreparedCertificate()
}

// Summary implements RoundState.Summary
func (rsp *rsSaveDecorator) Summary() *RoundStateSummary { return rsp.rs.Summary() }
