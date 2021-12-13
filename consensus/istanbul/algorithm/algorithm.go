// Package algorithm encapsulates logic pertaining the the celo ibft consensus algorithm.
package algorithm

import (
	"fmt"

	"github.com/celo-org/celo-blockchain/common/hexutil"
)

// Value represents the value to be agreed by consensus. It's limited size
// means that often the value here will be a hash or some similar reduction of
// the real value to be agreed.
type Value [32]byte

func (v Value) String() string {
	return hexutil.Encode(v[:])
}

// Type represents the different types of consensus messages.
type Type uint8

const (
	Propose Type = iota
	Prepare
	Commit
	RoundChange
)

// In returns true if the method receiver is one of the given types.
func (t Type) In(types ...Type) bool {
	for _, x := range types {
		if t == x {
			return true
		}
	}
	return false
}

func (t Type) String() string {
	switch t {
	case Propose:
		return "Propose"
	case Prepare:
		return "Prepare"
	case Commit:
		return "Commit"
	case RoundChange:
		return "RoundChange"
	default:
		return fmt.Sprintf("unrecognised type: %d", t)
	}
}

// State reprsents the discreet states that the algorithm can be in.
type State uint8

// We define these states in a separate const block to the message types to
// maintain the equivalence with the istanbul states (istanbul/core/types.go).
// This is because the iota identifier represents Successive untyped integer
// values within the scope of a const block.
const (
	AcceptRequest State = iota
	Preprepared
	Prepared
	Committed
	WaitingForNewRound
)

// Msg represents consensus messages.
type Msg struct {
	MsgType         Type
	Height          uint64
	Round           uint64
	Val             Value
	RoundChangeCert *Value
}

func (m *Msg) String() string {
	return fmt.Sprintf("T: %12v H: %12d R: %4d V: %v", m.MsgType, m.Height, m.Round, m.Val)
}

// Oracle provides access to external information. Allowing us to represent
// concepts that are relevant to the consensus algorithm without needing to
// have compile time dependencies on all the code required to manage an compute
// those concepts. Currently it is managing more than it should be, since
// certain logic that is really part of the consensus algorithm is abstracted
// away by the Oracle, for example managing the round and state of the
// algorithm and the logic for processing round change messages. In time we
// should be able to move this logic here and slim down the oracle's interface.
type Oracle interface {
	CurrentState() State
	QuorumCommit(height, round uint64, val Value) bool
	QuorumPrepare(height, round uint64, val Value) bool
	DesiredRound() uint64
	QuorumRoundChange() *uint64
	FPlus1RoundChange() *uint64
	ValidRoundChangeCert(height, round uint64, val Value, rcc *Value) bool
}

// The algorithm struct, for now it only holds an oracle.
type Algorithm struct {
	O Oracle
}

// NewAlgorithm creates a new algorithm instance with the given oracle.
func NewAlgorithm(o Oracle) *Algorithm {
	return &Algorithm{
		O: o,
	}
}

// HandleMessage handles the given message and returns at most one of 3
// results. A message indicates a state transition has occurred and also that
// the returned message needs to be broadcast. A round indicates that the
// instance should move to that round, if the round is 0 it indicates that we
// have committed. A desiredRound indicates that the instance should wait for
// they desired round.
func (a *Algorithm) HandleMessage(m *Msg) (msg *Msg, round, desiredRound *uint64) {
	h := m.Height
	r := m.Round
	t := m.MsgType
	v := m.Val
	rcc := m.RoundChangeCert
	oracle := a.O
	s := oracle.CurrentState()

	// We see a quorum of commits and we are not yet committed, then move to
	// committed state.
	if t == Commit && s < Committed && oracle.QuorumCommit(h, r, v) {
		var round uint64 = 0
		return nil, &round, nil
	}

	// We are not yet prepared and see a quorum of prepares (where a commit
	// also counts as a prepare), send a commit.
	if t.In(Prepare, Commit) && s < Prepared && oracle.QuorumPrepare(h, r, v) {
		// We send a commit
		return &Msg{
			MsgType: Commit,
			Height:  h,
			Round:   r,
			Val:     v,
		}, nil, nil
	}

	if t == Propose && s == AcceptRequest {
		if (r == 0 && rcc == nil) || (r > 0 && a.O.ValidRoundChangeCert(h, r, v, rcc)) {
			// We send a prepare
			return &Msg{
				MsgType: Prepare,
				Height:  h,
				Round:   r,
				Val:     v,
			}, nil, nil
		}
	}

	if t == RoundChange {
		qr := a.O.QuorumRoundChange()
		if qr != nil && *qr >= a.O.DesiredRound() {
			return nil, qr, nil
		}
		f1r := a.O.FPlus1RoundChange()
		if f1r != nil && *f1r > a.O.DesiredRound() {
			return nil, nil, f1r
		}
	}
	return nil, nil, nil
}
