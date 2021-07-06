package backend

import (
	"time"

	"github.com/celo-org/celo-blockchain/consensus/istanbul"
	"github.com/celo-org/celo-blockchain/p2p"
)

// announceTaskState encapsulates the state needed to guide the behavior of the announce protocol
// thread
type announceTaskState struct {
	config *istanbul.AnnounceConfig
	// Create a ticker to poll if istanbul core is running and if this node is in
	// the validator conn set. If both conditions are true, then this node should announce.
	checkIfShouldAnnounceTicker *time.Ticker
	// Occasionally share the entire version certificate table with all peers
	shareVersionCertificatesTicker    *time.Ticker
	pruneAnnounceDataStructuresTicker *time.Ticker

	queryEnodeTicker                            *time.Ticker
	queryEnodeTickerCh                          <-chan time.Time
	queryEnodeFrequencyState                    QueryEnodeGossipFrequencyState
	currentQueryEnodeTickerDuration             time.Duration
	numQueryEnodesInHighFreqAfterFirstPeerState int
	// TODO: this can be removed once we have more faith in this protocol
	updateAnnounceVersionTicker   *time.Ticker
	updateAnnounceVersionTickerCh <-chan time.Time

	// Replica validators listen & query for enodes       (query true, announce false)
	// Primary validators annouce (updateAnnounceVersion) (query true, announce true)
	// Replicas need to query to populate their validator enode table, but don't want to
	// update the proxie's validator assignments at the same time as the primary.
	shouldQuery, shouldAnnounce bool
	querying, announcing        bool
}

func NewAnnounceTaskState(config *istanbul.AnnounceConfig) *announceTaskState {
	return &announceTaskState{
		config:                            config,
		checkIfShouldAnnounceTicker:       time.NewTicker(5 * time.Second),
		shareVersionCertificatesTicker:    time.NewTicker(5 * time.Minute),
		pruneAnnounceDataStructuresTicker: time.NewTicker(10 * time.Minute),
	}
}

func (st *announceTaskState) ShouldStartQuerying() bool {
	return st.shouldQuery && !st.querying
}

func (st *announceTaskState) OnStartQuerying() {
	if st.config.AggressiveQueryEnodeGossipOnEnablement {
		st.queryEnodeFrequencyState = HighFreqBeforeFirstPeerState
		// Send an query enode message once a minute
		st.currentQueryEnodeTickerDuration = 1 * time.Minute
		st.numQueryEnodesInHighFreqAfterFirstPeerState = 0
	} else {
		st.queryEnodeFrequencyState = LowFreqState
		st.currentQueryEnodeTickerDuration = time.Duration(st.config.QueryEnodeGossipPeriod) * time.Second
	}

	// Enable periodic gossiping by setting announceGossipTickerCh to non nil value
	st.queryEnodeTicker = time.NewTicker(st.currentQueryEnodeTickerDuration)
	st.queryEnodeTickerCh = st.queryEnodeTicker.C

	st.querying = true
}

func (st *announceTaskState) ShouldStopQuerying() bool {
	return !st.shouldQuery && st.querying
}

func (st *announceTaskState) OnStopQuerying() {
	// Disable periodic queryEnode msgs by setting queryEnodeTickerCh to nil
	st.queryEnodeTicker.Stop()
	st.queryEnodeTickerCh = nil
	st.querying = false
}

func (st *announceTaskState) ShouldStartAnnouncing() bool {
	return st.shouldAnnounce && !st.announcing
}

func (st *announceTaskState) OnStartAnnouncing() {
	st.updateAnnounceVersionTicker = time.NewTicker(5 * time.Minute)
	st.updateAnnounceVersionTickerCh = st.updateAnnounceVersionTicker.C

	st.announcing = true
}

func (st *announceTaskState) ShouldStopAnnouncing() bool {
	return !st.shouldAnnounce && st.announcing
}

func (st *announceTaskState) OnStopAnnouncing() {
	// Disable periodic updating of announce version
	st.updateAnnounceVersionTicker.Stop()
	st.updateAnnounceVersionTickerCh = nil

	st.announcing = false
}

func (st *announceTaskState) UpdateFrequencyOnGenerate(peerCounter PeerCounterFn) {
	switch st.queryEnodeFrequencyState {
	case HighFreqBeforeFirstPeerState:
		if peerCounter(p2p.AnyPurpose) > 0 {
			st.queryEnodeFrequencyState = HighFreqAfterFirstPeerState
		}

	case HighFreqAfterFirstPeerState:
		if st.numQueryEnodesInHighFreqAfterFirstPeerState >= 10 {
			st.queryEnodeFrequencyState = LowFreqState
		}
		st.numQueryEnodesInHighFreqAfterFirstPeerState++

	case LowFreqState:
		if st.currentQueryEnodeTickerDuration != time.Duration(st.config.QueryEnodeGossipPeriod)*time.Second {
			// Reset the ticker
			st.currentQueryEnodeTickerDuration = time.Duration(st.config.QueryEnodeGossipPeriod) * time.Second
			st.queryEnodeTicker.Stop()
			st.queryEnodeTicker = time.NewTicker(st.currentQueryEnodeTickerDuration)
			st.queryEnodeTickerCh = st.queryEnodeTicker.C
		}
	}
}

func (st *announceTaskState) OnAnnounceThreadQuitting() {
	st.checkIfShouldAnnounceTicker.Stop()
	st.pruneAnnounceDataStructuresTicker.Stop()
	st.shareVersionCertificatesTicker.Stop()
	if st.querying {
		st.queryEnodeTicker.Stop()
	}
	if st.announcing {
		st.updateAnnounceVersionTicker.Stop()
	}
}
