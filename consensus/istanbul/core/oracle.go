package core

import (
	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/consensus/istanbul/algorithm"
)

type RoundStateOracle struct {
	rs RoundState
}

func NewRoundStateOracle(rs RoundState) *RoundStateOracle {
	return &RoundStateOracle{
		rs: rs,
	}
}

func (o *RoundStateOracle) CurrentState() algorithm.State {
	return algorithm.State(o.rs.State())
}

func (o *RoundStateOracle) checkHeightRoundAndValue(height, round uint64, value algorithm.Value) bool {
	if height != o.rs.Sequence().Uint64() {
		return false
	}
	if round != o.rs.Round().Uint64() {
		return false
	}
	if o.rs.Proposal() == nil {
		return false
	}
	if o.rs.Proposal().Hash() != common.Hash(value) {
		return false
	}
	return true
}

func (o *RoundStateOracle) QuorumCommit(height, round uint64, value algorithm.Value) bool {
	if !o.checkHeightRoundAndValue(height, round, value) {
		return false
	}
	return o.rs.Commits().Size() >= o.rs.ValidatorSet().MinQuorumSize()
}

func (o *RoundStateOracle) QuorumPrepare(height, round uint64, value algorithm.Value) bool {
	if !o.checkHeightRoundAndValue(height, round, value) {
		return false
	}
	return o.rs.GetPrepareOrCommitSize() >= o.rs.ValidatorSet().MinQuorumSize()
}
