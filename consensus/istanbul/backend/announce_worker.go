package backend

import (
	"sync/atomic"
	"time"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/consensus/istanbul"
	"github.com/celo-org/celo-blockchain/consensus/istanbul/backend/announce"
	"github.com/celo-org/celo-blockchain/log"
	"github.com/celo-org/celo-blockchain/p2p"
)

// AnnounceWorker is responsible for, while running, spawn all messages that this node
// should send: VersionCertificates sharing, and QueryEnode messages if the node
// is a NearlyElectedValidator.
//
// It automatically polls to check if it entered (or exited) NearlyElectedValidator status.
//
// It also periodically runs Prune in an AnnounceStatePruner.
type AnnounceWorker interface {
	Run()
	UpdateVersion()
	GetVersion() uint
	Stop()
}

type worker struct {
	logger            log.Logger
	aWallets          *atomic.Value
	version           announce.Version
	initialWaitPeriod time.Duration
	checker           ValidatorChecker
	state             *AnnounceState
	pruner            AnnounceStatePruner
	vcGossiper        VersionCertificateGossiper
	enodeGossiper     EnodeQueryGossiper
	config            *istanbul.Config
	countPeers        PeerCounterFn
	vpap              ValProxyAssigmnentProvider
	avs               AnnounceVersionSharer

	updateAnnounceVersionCh chan struct{}
	announceThreadQuit      chan struct{}
}

func NewAnnounceWorker(initialWaitPeriod time.Duration,
	aWallets *atomic.Value,
	version announce.Version,
	state *AnnounceState,
	checker ValidatorChecker,
	pruner AnnounceStatePruner,
	vcGossiper VersionCertificateGossiper,
	enodeGossiper EnodeQueryGossiper,
	config *istanbul.Config,
	countPeersFn PeerCounterFn,
	vpap ValProxyAssigmnentProvider,
	avs AnnounceVersionSharer) AnnounceWorker {
	return &worker{
		logger:                  log.New("module", "announceWorker"),
		aWallets:                aWallets,
		version:                 version,
		initialWaitPeriod:       initialWaitPeriod,
		checker:                 checker,
		state:                   state,
		pruner:                  pruner,
		vcGossiper:              vcGossiper,
		enodeGossiper:           enodeGossiper,
		config:                  config,
		countPeers:              countPeersFn,
		vpap:                    vpap,
		avs:                     avs,
		updateAnnounceVersionCh: make(chan struct{}, 1),
	}
}

func (m *worker) wallets() *Wallets {
	return m.aWallets.Load().(*Wallets)
}

func (w *worker) Stop() {
	close(w.announceThreadQuit)
}

func (w *worker) GetVersion() uint {
	return w.version.Get()
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
			w.logger.Warn("Error in checking if should announce", "err", err)
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

// generateAndGossipAnnounce will generate the lastest announce msg from this node
// and then broadcast it to it's peers, which should then gossip the announce msg
// message throughout the p2p network if there has not been a message sent from
// this node within the last announceGossipCooldownDuration.
// Note that this function must ONLY be called by the announceThread.
func (w *worker) generateAndGossipQueryEnode(enforceRetryBackoff bool) (*istanbul.Message, error) {
	logger := w.logger.New("func", "generateAndGossipQueryEnode")
	logger.Trace("generateAndGossipQueryEnode called")

	wts := w.wallets()
	// Retrieve the set valEnodeEntries (and their publicKeys)
	// for the queryEnode message
	qeep := NewQueryEnodeEntryProvider(w.state.valEnodeTable)
	valEnodeEntries, err := qeep.GetQueryEnodeValEnodeEntries(enforceRetryBackoff, wts.Ecdsa.Address)
	if err != nil {
		return nil, err
	}

	valAddresses := make([]common.Address, len(valEnodeEntries))
	for i, valEnodeEntry := range valEnodeEntries {
		valAddresses[i] = valEnodeEntry.Address
	}
	valProxyAssignments, err := w.vpap.GetValProxyAssignments(valAddresses)
	if err != nil {
		return nil, err
	}

	var enodeQueries []*enodeQuery
	for _, valEnodeEntry := range valEnodeEntries {
		if valEnodeEntry.PublicKey != nil {
			externalEnode := valProxyAssignments[valEnodeEntry.Address]
			if externalEnode == nil {
				continue
			}

			externalEnodeURL := externalEnode.URLv4()
			enodeQueries = append(enodeQueries, &enodeQuery{
				recipientAddress:   valEnodeEntry.Address,
				recipientPublicKey: valEnodeEntry.PublicKey,
				enodeURL:           externalEnodeURL,
			})
		}
	}

	var qeMsg *istanbul.Message
	if len(enodeQueries) > 0 {
		if qeMsg, err = w.enodeGossiper.GossipEnodeQueries(&wts.Ecdsa, enodeQueries); err != nil {
			return nil, err
		}
		if err = w.state.valEnodeTable.UpdateQueryEnodeStats(valEnodeEntries); err != nil {
			return nil, err
		}
	}

	return qeMsg, err
}

func (w *worker) updateAnnounceVersion() {
	version := getTimestamp()
	currVersion := w.version.Get()
	if version <= currVersion {
		w.logger.Debug("Announce version is not newer than the existing version", "existing version", currVersion, "attempted new version", version)
		return
	}
	if err := w.avs.ShareVersion(version); err != nil {
		w.logger.Warn("Error updating announce version", "err", err)
		return
	}
	w.logger.Debug("Updating announce version", "announceVersion", version)
	w.version.Set(version)
}
