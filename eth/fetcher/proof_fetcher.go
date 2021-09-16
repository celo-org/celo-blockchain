package fetcher

import (
	"math/rand"
	"time"

	"github.com/celo-org/celo-blockchain/common/prque"
	"github.com/celo-org/celo-blockchain/core/types"
	"github.com/celo-org/celo-blockchain/log"
	"github.com/celo-org/celo-blockchain/metrics"
)

const (
	metadataLimit = 256 // Maximum number of unique proofs a peer may have announced
	proofLimit    = 64  // Maximum number of unique proofs a peer may have delivered
)

var (
	proofAnnounceInMeter   = metrics.NewRegisteredMeter("eth/fetcher/proof/announces/in", nil)
	proofAnnounceOutTimer  = metrics.NewRegisteredTimer("eth/fetcher/proof/announces/out", nil)
	proofAnnounceDropMeter = metrics.NewRegisteredMeter("eth/fetcher/proof/announces/drop", nil)
	proofAnnounceDOSMeter  = metrics.NewRegisteredMeter("eth/fetcher/proof/announces/dos", nil)

	proofBroadcastInMeter   = metrics.NewRegisteredMeter("eth/fetcher/proof/broadcasts/in", nil)
	proofBroadcastOutTimer  = metrics.NewRegisteredTimer("eth/fetcher/proof/broadcasts/out", nil)
	proofBroadcastDropMeter = metrics.NewRegisteredMeter("eth/fetcher/proof/broadcasts/drop", nil)
	proofBroadcastDOSMeter  = metrics.NewRegisteredMeter("eth/fetcher/proof/broadcasts/dos", nil)

	proofFetchMeter = metrics.NewRegisteredMeter("eth/fetcher/fetch/proofs", nil)

	proofFilterInMeter  = metrics.NewRegisteredMeter("eth/fetcher/filter/proofs/in", nil)
	proofFilterOutMeter = metrics.NewRegisteredMeter("eth/fetcher/filter/proofs/out", nil)
)

// proofRetrievalFn is a callback type for retrieving a proof from the local db.
type proofRetrievalFn func(types.PlumoProofMetadata) *types.PlumoProof

// proofRequesterFn is a callback type for sending a proof retrieval request.
type proofRequesterFn func([]types.PlumoProofMetadata) error

// proofVerifierFn is a callback type to verify a proof.
type proofVerifierFn func(proof *types.PlumoProof) error

// proofBroadcasterFn is a callback type for broadcasting a proof to connected peers.
type proofBroadcasterFn func(proof *types.PlumoProof, propagate bool)

// proofInsertFn is a callback type to insert a batch of proofs into the local db.
type proofInsertFn func(types.PlumoProofs) error

// announce is the metadata notification of the availability of a new proof in the
// network.
type proofAnnounce struct {
	metadata types.PlumoProofMetadata // Metadata of the proof being anounced
	time     time.Time                // Tiemstamp of the announcement

	origin string // Identifier of the peer originating the notification

	fetchProofs proofRequesterFn // Fetcher function to retrieve the announced proofs
}

// proofFilterTask represents a batch of proofs needing fetcher filtering.
type proofFilterTask struct {
	peer   string              // The soruce peer of proofs
	proofs []*types.PlumoProof // Collection of proofs to filter
	time   time.Time           // Arrival time of the proofs
}

// inject represents a scheduled import operation.
type proofInject struct {
	origin string
	proof  *types.PlumoProof
}

// ProofFetcher is responsible for accumulating proof announcements from various peers
// and scheduling them for retrieval.
type ProofFetcher struct {
	// Various event channels
	notify chan *proofAnnounce
	inject chan *proofInject

	proofFilter chan chan *proofFilterTask

	done chan types.PlumoProofMetadata
	quit chan struct{}

	// Announce states
	announces map[string]int                                // Per peer announce counts to prevent memory exhaustion
	announced map[types.PlumoProofMetadata][]*proofAnnounce // Announced proofs, scheduled for fetching
	fetching  map[types.PlumoProofMetadata]*proofAnnounce   // Announced proofs, currently fetching

	// Proof cache
	queue  *prque.Prque                              // Queue containing the import operations (version number sorted) (TODO pqueue necessary?)
	queues map[string]int                            // Per peer proof counts to prevent memory exhaustion
	queued map[types.PlumoProofMetadata]*proofInject // Set of already queued proofs (to dedupe imports)

	// Callbacks
	getProof       proofRetrievalFn   // Retrieves a proof from the local db
	verifyProof    proofVerifierFn    // Checks if a proof is verified
	broadcastProof proofBroadcasterFn // Broadcasts a proof to connected peers
	insertProofs   proofInsertFn      // Injects a batch of proofs into the chain
	dropPeer       peerDropFn         // Drops a peer for misbehaving

	// Testing hooks
	announceChangeHook func(types.PlumoProofMetadata, bool) // Method to call upon adding or deleting a proof's metadata from the announce list
	queueChangeHook    func(types.PlumoProofMetadata, bool) // Method to call upon adding or deleting a proof's metadata from the import queue
	fetchingHook       func([]types.PlumoProofMetadata)     // Method to call upon starting a proof fetch
	importedHook       func(*types.PlumoProof)              // Method to call upon successful proof import
}

func NewProofFetcher(getProof proofRetrievalFn, verifyProof proofVerifierFn, broadcastProof proofBroadcasterFn, insertProofs proofInsertFn, dropPeer peerDropFn) *ProofFetcher {
	return &ProofFetcher{
		notify:         make(chan *proofAnnounce),
		inject:         make(chan *proofInject),
		proofFilter:    make(chan chan *proofFilterTask),
		done:           make(chan types.PlumoProofMetadata),
		quit:           make(chan struct{}),
		announces:      make(map[string]int),
		announced:      make(map[types.PlumoProofMetadata][]*proofAnnounce),
		fetching:       make(map[types.PlumoProofMetadata]*proofAnnounce),
		queue:          prque.New(nil),
		queues:         make(map[string]int),
		queued:         make(map[types.PlumoProofMetadata]*proofInject),
		getProof:       getProof,
		verifyProof:    verifyProof,
		broadcastProof: broadcastProof,
		insertProofs:   insertProofs,
		dropPeer:       dropPeer,
	}
}

// Start boots up the announcement based synchroniser, accepting and processing
// metadata notifications until termination requested.
func (pf *ProofFetcher) Start() {
	go pf.loop()
}

// Stop terminates the announcement based synchroniser, canceling all pending
// operations.
func (pf *ProofFetcher) Stop() {
	close(pf.quit)
}

// Notify announces the fetcher of the potential availability of a new proof in
// the network.
func (pf *ProofFetcher) Notify(peer string, metadata types.PlumoProofMetadata, time time.Time,
	proofFetcher proofRequesterFn) error {
	proof := &proofAnnounce{
		metadata:    metadata,
		time:        time,
		origin:      peer,
		fetchProofs: proofFetcher,
	}
	select {
	case pf.notify <- proof:
		return nil
	case <-pf.quit:
		return errTerminated
	}
}

// Enqueue tries to fill gaps the fetcher's future import queue.
func (pf *ProofFetcher) Enqueue(peer string, proof *types.PlumoProof) error {
	op := &proofInject{
		origin: peer,
		proof:  proof,
	}
	select {
	case pf.inject <- op:
		return nil
	case <-pf.quit:
		return errTerminated
	}
}

func (pf *ProofFetcher) FilterProofs(peer string, proofs []*types.PlumoProof, time time.Time) []*types.PlumoProof {
	log.Trace("Filtering proofs", "peer", peer, "proofs", len(proofs))

	// Send the filter channel to the fetcher
	filter := make(chan *proofFilterTask)

	select {
	case pf.proofFilter <- filter:
	case <-pf.quit:
		return nil
	}

	// Request the filtering of the proof list
	select {
	case filter <- &proofFilterTask{peer: peer, proofs: proofs, time: time}:
	case <-pf.quit:
		return nil
	}

	// Retrieve the proofs remaining after filtering
	select {
	case task := <-filter:
		return task.proofs
	case <-pf.quit:
		return nil
	}
}

// Loop is the main fetcher loop, checking and processing various notification
// events.
func (pf *ProofFetcher) loop() {
	// Iterate the block fetching until a quit is requested
	fetchTimer := time.NewTimer(0)

	for {
		// Clean up any expired proof fetches
		for metadata, announce := range pf.fetching {
			if time.Since(announce.time) > fetchTimeout {
				pf.forgetMetadata(metadata)
			}
		}
		// Import any queued proofs that could potentially fit
		for !pf.queue.Empty() {
			op := pf.queue.PopItem().(*proofInject)
			metadata := op.proof.Metadata
			if pf.queueChangeHook != nil {
				pf.queueChangeHook(metadata, false)
			}
			if pf.getProof(metadata) != nil {
				pf.forgetMetadata(metadata)
				continue
			}
			pf.insert(op.origin, op.proof)
		}
		// Wait for an outside event to occur
		select {
		case <-pf.quit:
			// Fetcher terminating, abort all operations
			return

		case notification := <-pf.notify:
			// A proof was announced, make sure the peer isn't DOSing us
			proofAnnounceInMeter.Mark(1)

			count := pf.announces[notification.origin] + 1
			if count > metadataLimit {
				log.Debug("Peer exceeded outstanding announces", "peer", notification.origin, "limit", metadataLimit)
				proofAnnounceDOSMeter.Mark(1)
				break
			}
			// If we have a valid metadata, check that it's potentially useful
			if notification.metadata != (types.PlumoProofMetadata{}) {
				// TODO is this ok? I think it simulates the expected behavior as best it can
				if pf.getProof(notification.metadata) != nil {
					log.Debug("Peer discarded announcement", "peer", notification.origin, "metadata", notification.metadata.String())
					proofAnnounceDropMeter.Mark(1)
					break
				}
			}
			// All is well, schedule the announce if proof's not yet downloading
			if _, ok := pf.fetching[notification.metadata]; ok {
				break
			}
			pf.announces[notification.origin] = count
			pf.announced[notification.metadata] = append(pf.announced[notification.metadata], notification)
			if pf.announceChangeHook != nil && len(pf.announced[notification.metadata]) == 1 {
				pf.announceChangeHook(notification.metadata, true)
			}
			if len(pf.announced) == 1 {
				pf.rescheduleFetch(fetchTimer)
			}

		case op := <-pf.inject:
			// A direct proof insertion was requested, try and fill any pending gaps
			proofBroadcastInMeter.Mark(1)
			pf.enqueue(op.origin, op.proof)

		case metadata := <-pf.done:
			// A pending import finished, remove all traces of the notification
			pf.forgetMetadata(metadata)

		case <-fetchTimer.C:
			// At least one proof's timer ran out, check for needing retrieval
			request := make(map[string][]types.PlumoProofMetadata)

			for metadata, announces := range pf.announced {
				if time.Since(announces[0].time) > arriveTimeout-gatherSlack {
					// Pick a random peer to retrieve from, reset all others
					announce := announces[rand.Intn(len(announces))]
					pf.forgetMetadata(metadata)

					// If the proof still didn't arrive, queue for fetching
					if pf.getProof(metadata) == nil {
						request[announce.origin] = append(request[announce.origin], metadata)
						pf.fetching[metadata] = announce
					}
				}
			}
			// Send out all proof requests
			for peer, proofsMetadata := range request {
				log.Trace("Fetching scheduled headers", "peer", peer, "list", proofsMetadata)

				// Create a closure of the fetch and schedule in on a new thread
				if pf.fetchingHook != nil {
					pf.fetchingHook(proofsMetadata)
				}
				proofFetchMeter.Mark(int64(len(proofsMetadata)))
				go pf.fetching[proofsMetadata[0]].fetchProofs(proofsMetadata)
			}
			// Schedule the next fetch if proofs are still pending
			pf.rescheduleFetch(fetchTimer)

		case filter := <-pf.proofFilter:
			// Proofs arrived from a remote peer. Extract those that were explicitly
			// requested by the fetcher, and return everything else so it's delivered
			// to other parts of the system.
			var task *proofFilterTask
			select {
			case task = <-filter:
			case <-pf.quit:
				return
			}
			proofFilterInMeter.Mark(int64(len(task.proofs)))

			proofs := []*types.PlumoProof{}
			for i := 0; i < len(task.proofs); i++ {
				// Match up a proof to any possible completion request
				matched := false

				for metadata, announce := range pf.fetching {
					if pf.queued[metadata] == nil {
						if task.proofs[i].Metadata == announce.metadata && announce.origin == task.peer {
							// Mark the proof matched
							matched = true

							if pf.getProof(metadata) == nil {
								proofs = append(proofs, task.proofs[i])
							} else {
								pf.forgetMetadata(metadata)
							}
						}
					}
				}
				if matched {
					if i+1 < len(task.proofs) {
						task.proofs = append(task.proofs[:i], task.proofs[i+1:]...)
						i--
						continue
					} else {
						task.proofs = task.proofs[:i]
					}
				}
			}
			proofFilterOutMeter.Mark(int64(len(task.proofs)))
			select {
			case filter <- task:
			case <-pf.quit:
				return
			}
			// Schedule the retrieved proofs for import
			for _, proof := range proofs {
				if announce := pf.fetching[proof.Metadata]; announce != nil {
					pf.enqueue(announce.origin, proof)
				}
			}
		}
	}
}

// rescheduleFetch resets the specified fetch timer to the next announce timeout.
func (pf *ProofFetcher) rescheduleFetch(fetch *time.Timer) {
	// Short circuit if no proofs are announced
	if len(pf.announced) == 0 {
		return
	}
	// Otherwise find the earliest expiring announcement
	earliest := time.Now()
	for _, announces := range pf.announced {
		if earliest.After(announces[0].time) {
			earliest = announces[0].time
		}
	}
	fetch.Reset(arriveTimeout - time.Since(earliest))
}

// enqueue schedules a new future import operation, if the proof to be imported
// has not yet been seen.
func (pf *ProofFetcher) enqueue(peer string, proof *types.PlumoProof) {
	metadata := proof.Metadata

	// Ensure the peer isn't DOSing us
	count := pf.queues[peer] + 1
	if count > proofLimit {
		log.Info("Discarded propagated proof, exceeded allowance", "peer", peer, "metadata", metadata.String(), "limit", proofLimit)
		proofBroadcastDOSMeter.Mark(1)
		pf.forgetMetadata(metadata)
		return
	}
	// Schedule the proof for future importing
	if _, ok := pf.queued[metadata]; !ok {
		op := &proofInject{
			origin: peer,
			proof:  proof,
		}
		pf.queues[peer] = count
		pf.queued[metadata] = op
		pf.queue.Push(op, -int64(proof.Metadata.VersionNumber))
		if pf.queueChangeHook != nil {
			pf.queueChangeHook(op.proof.Metadata, true)
		}
		log.Debug("Queued propagated proof", "peer", peer, "metadata", metadata.String(), "queued", pf.queue.Size())
	}
}

// insert spawns a new goroutine to run a proof insertion into the chain.
func (pf *ProofFetcher) insert(peer string, proof *types.PlumoProof) {
	metadata := proof.Metadata

	// Run the import on a new thread
	log.Debug("Importing propagated proof", "peer", peer, "metadata", metadata.String())
	go func() {
		defer func() { pf.done <- metadata }()

		// Quickly validate the proof and propagate the proof if it passes
		switch err := pf.verifyProof(proof); err {
		case nil:
			// All ok, quickly propagate to our peers
			// TODO
			// propBroadcastOutTimer.UpdateSince(block.ReceivedAt)
			go pf.broadcastProof(proof, true)

		default:
			// Something went very wrong, drop the peer
			log.Debug("Propagated proof verification failed", "peer", peer, "metadata", metadata.String(), "err", err)
			pf.dropPeer(peer)
			return
		}
		// Run the actual import and log any issues
		if err := pf.insertProofs(types.PlumoProofs{proof}); err != nil {
			log.Debug("Propagated proof import failed", "peer", peer, "metadata", metadata.String(), "err", err)
			return
		}
		// If import succeeded, broadcast the proof
		// TODO
		// propAnnounceOutTimer.UpdateSince(block.ReceivedAt)
		go pf.broadcastProof(proof, false)

		// Invoke the testing hook if needed
		if pf.importedHook != nil {
			pf.importedHook(proof)
		}
	}()
}

// forgetMetadata removes all traces of a queued proof from the fetcher's internal
// state.
func (pf *ProofFetcher) forgetMetadata(metadata types.PlumoProofMetadata) {
	// Remove all pending announces and decrement DOS counters
	for _, announce := range pf.announced[metadata] {
		pf.announces[announce.origin]--
		if pf.announces[announce.origin] <= 0 {
			delete(pf.announces, announce.origin)
		}
	}
	delete(pf.announced, metadata)
	if pf.announceChangeHook != nil {
		pf.announceChangeHook(metadata, false)
	}
	// Remove any pending fetches and decrement the DOS counters
	if announce := pf.fetching[metadata]; announce != nil {
		pf.announces[announce.origin]--
		if pf.announces[announce.origin] <= 0 {
			delete(pf.announces, announce.origin)
		}
		delete(pf.fetching, metadata)
	}
	if insert := pf.queued[metadata]; insert != nil {
		pf.queues[insert.origin]--
		if pf.queues[insert.origin] == 0 {
			delete(pf.queues, insert.origin)
		}
		delete(pf.queued, metadata)
	}
}
