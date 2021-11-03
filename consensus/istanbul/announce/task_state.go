package announce

import (
	"time"

	"github.com/celo-org/celo-blockchain/consensus/istanbul"
	"github.com/celo-org/celo-blockchain/log"
)

// QueryEnodeGossipFrequencyState specifies how frequently to gossip query enode messages
type QueryEnodeGossipFrequencyState int

const (
	// HighFreqBeforeFirstPeerState will send out a query enode message every 1 minute until the first peer is established
	HighFreqBeforeFirstPeerState QueryEnodeGossipFrequencyState = iota

	// HighFreqAfterFirstPeerState will send out an query enode message every 1 minute for the first 10 query enode messages after the first peer is established.
	// This is on the assumption that when this node first establishes a peer, the p2p network that this node is in may
	// be partitioned with the broader p2p network. We want to give that p2p network some time to connect to the broader p2p network.
	HighFreqAfterFirstPeerState

	// LowFreqState will send out an query every config.AnnounceQueryEnodeGossipPeriod seconds
	LowFreqState
)

// announceTaskState encapsulates the state needed to guide the behavior of the announce protocol
// thread. This type is designed to be used from A SINGLE THREAD only.
type announceTaskState struct {
	config *istanbul.Config
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

	generateAndGossipQueryEnodeCh chan struct{}

	// Replica validators listen & query for enodes       (query true, announce false)
	// Primary validators annouce (updateAnnounceVersion) (query true, announce true)
	// Replicas need to query to populate their validator enode table, but don't want to
	// update the proxie's validator assignments at the same time as the primary.
	shouldQuery, shouldAnnounce bool
	querying, announcing        bool
}

func NewAnnounceTaskState(config *istanbul.Config) *announceTaskState {
	return &announceTaskState{
		config:                            config,
		checkIfShouldAnnounceTicker:       time.NewTicker(5 * time.Second),
		shareVersionCertificatesTicker:    time.NewTicker(5 * time.Minute),
		pruneAnnounceDataStructuresTicker: time.NewTicker(10 * time.Minute),
		generateAndGossipQueryEnodeCh:     make(chan struct{}, 1),
	}
}

func (st *announceTaskState) ShouldStartQuerying() bool {
	return st.shouldQuery && !st.querying
}

func (st *announceTaskState) OnStartQuerying() {
	if st.config.AnnounceAggressiveQueryEnodeGossipOnEnablement {
		st.queryEnodeFrequencyState = HighFreqBeforeFirstPeerState
		// Send an query enode message once a minute
		st.currentQueryEnodeTickerDuration = 1 * time.Minute
		st.numQueryEnodesInHighFreqAfterFirstPeerState = 0
	} else {
		st.queryEnodeFrequencyState = LowFreqState
		st.currentQueryEnodeTickerDuration = time.Duration(st.config.AnnounceQueryEnodeGossipPeriod) * time.Second
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

func (st *announceTaskState) UpdateFrequencyOnGenerate(peers int) {
	switch st.queryEnodeFrequencyState {
	case HighFreqBeforeFirstPeerState:
		if peers > 0 {
			st.queryEnodeFrequencyState = HighFreqAfterFirstPeerState
		}

	case HighFreqAfterFirstPeerState:
		if st.numQueryEnodesInHighFreqAfterFirstPeerState >= 10 {
			st.queryEnodeFrequencyState = LowFreqState
		}
		st.numQueryEnodesInHighFreqAfterFirstPeerState++

	case LowFreqState:
		if st.currentQueryEnodeTickerDuration != time.Duration(st.config.AnnounceQueryEnodeGossipPeriod)*time.Second {
			// Reset the ticker
			st.currentQueryEnodeTickerDuration = time.Duration(st.config.AnnounceQueryEnodeGossipPeriod) * time.Second
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

// startGossipQueryEnodeTask will schedule a task for the announceThread to
// generate and gossip a queryEnode message
func (st *announceTaskState) startGossipQueryEnodeTask() {
	// sb.generateAndGossipQueryEnodeCh has a buffer of 1. If there is a value
	// already sent to the channel that has not been read from, don't block.
	select {
	case st.generateAndGossipQueryEnodeCh <- struct{}{}:
	default:
	}
}

func (st *announceTaskState) updateAnnounceThreadStatus(logger log.Logger, startWaitPeriod time.Duration, updateAnnounceVersion func()) {
	if st.ShouldStartQuerying() {
		logger.Info("Starting to query")

		time.AfterFunc(startWaitPeriod, func() {
			st.startGossipQueryEnodeTask()
		})

		st.OnStartQuerying()

		logger.Trace("Enabled periodic gossiping of announce message (query mode)")

	} else if st.ShouldStopQuerying() {
		logger.Info("Stopping querying")

		st.OnStopQuerying()
		logger.Trace("Disabled periodic gossiping of announce message (query mode)")
	}

	if st.ShouldStartAnnouncing() {
		logger.Info("Starting to announce")

		updateAnnounceVersion()

		st.OnStartAnnouncing()
		logger.Trace("Enabled periodic gossiping of announce message")
	} else if st.ShouldStopAnnouncing() {
		logger.Info("Stopping announcing")

		st.OnStopAnnouncing()
		logger.Trace("Disabled periodic gossiping of announce message")
	}
}
