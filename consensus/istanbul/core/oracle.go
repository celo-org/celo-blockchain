package core

import (
	"crypto/rand"
	"math/big"
	"reflect"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/consensus/istanbul"
	"github.com/celo-org/celo-blockchain/consensus/istanbul/algorithm"
)

type RoundStateOracle struct {
	rs RoundState
	c  *core

	roundChangeCertId *algorithm.Value
	preprepare        *istanbul.Preprepare
}

func NewRoundStateOracle(rs RoundState, c *core) *RoundStateOracle {
	return &RoundStateOracle{
		rs: rs,
		c:  c,
	}
}

func (o *RoundStateOracle) CurrentState() algorithm.State {
	return algorithm.State(o.rs.State())
}

func (o *RoundStateOracle) DesiredRound() uint64 {
	return o.rs.DesiredRound().Uint64()
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

// SetRoundChangeCertificate maps a round change certificate to an id and
// returns the id. This allows the algorithm to reference the round change
// certificate without having a compile time dependency on
// istanbul.RoundChangeCertificate.
func (o *RoundStateOracle) SetRoundChangeCertificate(preprepare *istanbul.Preprepare) (*algorithm.Value, error) {
	// Unset old values
	o.roundChangeCertId = nil
	o.preprepare = nil

	// No round change for round 0
	if preprepare.View.Round.Uint64() == 0 {
		return nil, nil
	}

	var id algorithm.Value
	_, err := rand.Read(id[:])
	if err != nil {
		return nil, err
	}
	subject := istanbul.Subject{
		View:   preprepare.View,
		Digest: preprepare.Proposal.Hash(),
	}
	logger := o.c.newLogger().New("func", "SetRoundChangeCertificate", "tag", "handlePreprepare",
		"msg_hash", subject.Digest, "msg_seq", subject.View.Sequence, "msg_round", subject.View.Round)

	if preprepare.View.Round.Uint64() > 0 {
		if preprepare.RoundChangeCertificate.IsEmpty() {
			logger.Error("Preprepare for non-zero round did not contain a round change certificate.")
			return nil, errMissingRoundChangeCertificate
		}
		err = o.c.handleRoundChangeCertificate(o.c.roundChangeSet, o.rs, subject, preprepare.RoundChangeCertificate)
		if err != nil {
			logger.Warn("Invalid round change certificate with preprepare.", "err", err)
			return nil, err
		}
	} else if !preprepare.RoundChangeCertificate.IsEmpty() {
		logger.Error("Preprepare for round 0 has a round change certificate.")
		return nil, errInvalidProposal
	}
	o.roundChangeCertId = &id
	o.preprepare = preprepare
	return &id, nil
}

func (o *RoundStateOracle) ValidRoundChangeCert(height, round uint64, val algorithm.Value, rcc *algorithm.Value) bool {
	if rcc == nil {
		return false
	}
	if *rcc != *o.roundChangeCertId {
		return false
	}

	subject := istanbul.Subject{
		View: &istanbul.View{
			Sequence: new(big.Int).SetUint64(height),
			Round:    new(big.Int).SetUint64(round),
		},
		Digest: common.Hash(val),
	}
	preprepareSubject := istanbul.Subject{
		View:   o.preprepare.View,
		Digest: o.preprepare.Proposal.Hash(),
	}
	return reflect.DeepEqual(preprepareSubject, subject)
}
