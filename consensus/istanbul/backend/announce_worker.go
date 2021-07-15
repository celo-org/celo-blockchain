package backend

import (
	"time"

	"github.com/celo-org/celo-blockchain/consensus/istanbul"
	"github.com/celo-org/celo-blockchain/log"
	"github.com/celo-org/celo-blockchain/p2p"
)

type AnnounceWorker interface {
	Run()
	UpdateVersion()
	Stop()
}

type worker struct {
	logger            log.Logger
	initialWaitPeriod time.Duration
	checker           ValidatorChecker
	state             *AnnounceState
	pruner            AnnounceStatePruner
	vcGossiper        VersionCertificateGossiper
	config            *istanbul.AnnounceConfig
	countPeers        PeerCounterFn

	updateAnnounceVersion       func()
	generateAndGossipQueryEnode func(bool) (*istanbul.Message, error)

	updateAnnounceVersionCh chan struct{}
	announceThreadQuit      chan struct{}
}

func NewAnnounceWorker(initialWaitPeriod time.Duration,
	state *AnnounceState,
	checker ValidatorChecker,
	pruner AnnounceStatePruner,
	vcGossiper VersionCertificateGossiper,
	config *istanbul.AnnounceConfig,
	countPeersFn PeerCounterFn,
	updateAnnounceVersion func(),
	generateAndGossipQueryEnode func(bool) (*istanbul.Message, error)) AnnounceWorker {
	return &worker{
		logger:            log.New("module", "announceWorker"),
		initialWaitPeriod: initialWaitPeriod,
		checker:           checker,
		state:             state,
		pruner:            pruner,
		vcGossiper:        vcGossiper,
		config:            config,
		countPeers:        countPeersFn,

		updateAnnounceVersion:       updateAnnounceVersion,
		generateAndGossipQueryEnode: generateAndGossipQueryEnode,
		updateAnnounceVersionCh:     make(chan struct{}, 1),
	}
}

func (w *worker) Stop() {
	close(w.announceThreadQuit)
}

func (w *worker) UpdateVersion() {
	// Send to the channel iff it does not already have a message.
	select {
	case w.updateAnnounceVersionCh <- struct{}{}:
	default:
	}
}

func (w *worker) Run() {
	w.announceThreadQuit = make(chan struct{})
	shouldQueryAndAnnounce := func() (bool, bool) {
		var err error
		shouldQuery, err := w.checker.IsElectedOrNearValidator()
		if err != nil {
			w.logger.Warn("Error in checking if should announce", err)
			return false, false
		}
		shouldAnnounce := shouldQuery && w.checker.IsValidating()
		return shouldQuery, shouldAnnounce
	}

	st := NewAnnounceTaskState(w.config)
	for {
		select {
		case <-st.checkIfShouldAnnounceTicker.C:
			w.logger.Trace("Checking if this node should announce it's enode")
			st.shouldQuery, st.shouldAnnounce = shouldQueryAndAnnounce()
			st.updateAnnounceThreadStatus(w.logger, w.initialWaitPeriod, w.updateAnnounceVersion)

		case <-st.shareVersionCertificatesTicker.C:
			if err := w.vcGossiper.GossipAllFrom(w.state.versionCertificateTable); err != nil {
				w.logger.Warn("Error gossiping all version certificates")
			}

		case <-st.updateAnnounceVersionTickerCh:
			if st.shouldAnnounce {
				w.updateAnnounceVersion()
			}

		case <-st.queryEnodeTickerCh:
			st.startGossipQueryEnodeTask()

		case <-st.generateAndGossipQueryEnodeCh:
			if st.shouldQuery {
				peers := w.countPeers(p2p.AnyPurpose)
				st.UpdateFrequencyOnGenerate(peers)
				// This node may have recently sent out an announce message within
				// the gossip cooldown period imposed by other nodes.
				// Regardless, send the queryEnode so that it will at least be
				// processed by this node's peers. This is especially helpful when a network
				// is first starting up.
				if _, err := w.generateAndGossipQueryEnode(st.queryEnodeFrequencyState == LowFreqState); err != nil {
					w.logger.Warn("Error in generating and gossiping queryEnode", "err", err)
				}
			}

		case <-w.updateAnnounceVersionCh:
			if st.shouldAnnounce {
				w.updateAnnounceVersion()
			}

		case <-st.pruneAnnounceDataStructuresTicker.C:
			if err := w.pruner.Prune(w.state); err != nil {
				w.logger.Warn("Error in pruning announce data structures", "err", err)
			}

		case <-w.announceThreadQuit:
			st.OnAnnounceThreadQuitting()
			return
		}
	}
}
