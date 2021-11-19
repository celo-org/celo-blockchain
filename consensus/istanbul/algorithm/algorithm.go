package algorithm

type Value [32]byte

type Type uint8

func (t Type) In(types ...Type) bool {
	for _, x := range types {
		if t == x {
			return true
		}
	}
	return false
}

type State uint8

const (
	Propose Type = iota
	Prepare
	Commit
	RoundChange
)

// We define these in a separate const block to the message types to maintain
// the equivalence with the istanbul states. This is because the iota
// identifier represents Successive untyped integer values within the scope of
// a const block.
const (
	AcceptRequest State = iota
	Preprepared
	Prepared
	Committed
	WaitingForNewRound
)

type Msg struct {
	MsgType Type
	Height  uint64
	Round   uint64
	Val     Value
}

type Oracle interface {
	CurrentState() State
	QuorumCommit(height, round uint64, val Value) bool
	QuorumPrepare(height, round uint64, val Value) bool
}

type Algorithm struct {
	O Oracle
}

func NewAlgorithm(o Oracle) *Algorithm {
	return &Algorithm{
		O: o,
	}
}

// the bool represents whether we are committed or not
// Maybe we could return the state and say transitioned to state, but then that duplicates information in the message.
func (a *Algorithm) HandleMessage(m *Msg) (*Msg, bool) {
	h := m.Height
	r := m.Round
	t := m.MsgType
	v := m.Val
	oracle := a.O
	s := oracle.CurrentState()

	// We see a quorum of commits and we are not yet committed, then move to
	// committed state.
	if t == Commit && s < Committed && oracle.QuorumCommit(h, r, v) {
		return nil, true
	}

	// We are not yet prepared and see a quorum of prepares, send a commit.
	if s < Prepared && oracle.QuorumPrepare(h, r, v) {
		// We send a commit
		return &Msg{
			MsgType: Commit,
			Height:  h,
			Round:   r,
			Val:     v,
		}, false
	}
	return nil, false
}
