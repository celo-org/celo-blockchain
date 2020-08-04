// Copyright 2015 The go-ethereum Authors
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

package fetcher

import (
	"encoding/hex"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/core/types"
)

var (
	proof, _     = hex.DecodeString("dummyProof0")
	testMetadata = types.PlumoProofMetadata{
		FirstEpoch:    0,
		LastEpoch:     1,
		VersionNumber: 1,
	}
	testProof = types.PlumoProof{
		Proof:    proof,
		Metadata: testMetadata,
	}
)

// makeProofs creates a list of n proofs, iteratively increasing the proof range by `step`
func makeProofs(n int, step int) ([]types.PlumoProofMetadata, map[types.PlumoProofMetadata]*types.PlumoProof) {
	var proofsMetadata []types.PlumoProofMetadata
	proofs := make(map[types.PlumoProofMetadata]*types.PlumoProof, n)
	for i := 0; i < n; i++ {
		metadata := types.PlumoProofMetadata{uint(step * i), uint(step*i + step), 0}
		proofsMetadata = append(proofsMetadata, metadata)
		dummyProofString, _ := hex.DecodeString(fmt.Sprintf("%s%d", "dummyProof", i))
		dummyProof := &types.PlumoProof{dummyProofString, metadata}
		proofs[metadata] = dummyProof
	}
	return proofsMetadata, proofs
}

// proofFetcherTester is a test simulator for mocking out local proof gathering.
type proofFetcherTester struct {
	proofFetcher *ProofFetcher

	proofsMetadata []types.PlumoProofMetadata                     // Proof metadata belonging to the tester
	proofs         map[types.PlumoProofMetadata]*types.PlumoProof // Proofs belonging to the tester
	drops          map[string]bool                                // Map of peers dropped by the proof fetcher

	lock sync.RWMutex
}

// newProofTester creates a new proof fetcher test mocker.
func newProofTester() *proofFetcherTester {
	var proofsMetadata []types.PlumoProofMetadata
	proofsMetadata = append(proofsMetadata, testMetadata)
	tester := &proofFetcherTester{
		proofsMetadata: nil,
		proofs:         make(map[types.PlumoProofMetadata]*types.PlumoProof),
		drops:          make(map[string]bool),
	}
	tester.proofFetcher = NewProofFetcher(tester.getProof, tester.verifyProof, tester.broadcastProof, tester.insertProofs, tester.dropPeer)
	tester.proofFetcher.Start()

	return tester
}

// getProof retrieves a proof from the tester's storage.
func (pf *proofFetcherTester) getProof(metadata types.PlumoProofMetadata) *types.PlumoProof {
	pf.lock.RLock()
	defer pf.lock.RUnlock()

	return pf.proofs[metadata]
}

// verifyProof is a nop placeholder for the proof verification.
func (pf *proofFetcherTester) verifyProof(proof *types.PlumoProof) error {
	return nil
}

// broadcastProof is a nop placeholder for proof broadcasting.
func (pf *proofFetcherTester) broadcastProof(proof *types.PlumoProof, propagate bool) {
}

// insertProofs injects a new proof into the db.
func (pf *proofFetcherTester) insertProofs(proofs types.PlumoProofs) (int, error) {
	pf.lock.Lock()
	defer pf.lock.Unlock()

	for _, proof := range proofs {
		pf.proofsMetadata = append(pf.proofsMetadata, proof.Metadata)
		pf.proofs[proof.Metadata] = proof
	}
	return 0, nil
}

// dropPeer is an emulator for the peer removal, simply accumulating the various
// peers dropped by the fetcher.
func (pf *proofFetcherTester) dropPeer(peer string) {
	pf.lock.Lock()
	defer pf.lock.Unlock()

	pf.drops[peer] = true
}

// makeProofFetcher retrieves a proof fetcher associated with simulated peer.
func (pf *proofFetcherTester) makeProofFetcher(t *testing.T, peer string, proofs map[types.PlumoProofMetadata]*types.PlumoProof, drift time.Duration) proofRequesterFn {
	closure := make(map[types.PlumoProofMetadata]*types.PlumoProof)
	for metadata, proof := range proofs {
		closure[metadata] = proof
	}
	// Create a function that returns proofs from the closure
	return func(proofsMetadata []types.PlumoProofMetadata) error {
		// Gather the proofs to return
		proofs := make(types.PlumoProofs, 0, 1)
		for _, metadata := range proofsMetadata {
			if proof, ok := closure[metadata]; ok {
				proofs = append(proofs, proof)
			}
		}
		// Return on a new thread
		go pf.proofFetcher.FilterProofs(peer, proofs, time.Now().Add(drift))

		return nil
	}
}

// verifyProofImportEvent verifies that one single event arrive on an import channel.
func verifyProofImportEvent(t *testing.T, imported chan *types.PlumoProof, arrive bool) {
	if arrive {
		select {
		case <-imported:
		case <-time.After(2 * time.Second):
			t.Fatalf("import timeout")
		}
	} else {
		select {
		case <-imported:
			t.Fatalf("import invoked")
		case <-time.After(20 * time.Millisecond):
		}
	}
}

// verifyProofImportCount verifies that exactly count number of events arrive on an
// import hook channel.
func verifyProofImportCount(t *testing.T, imported chan *types.PlumoProof, count int) {
	for i := 0; i < count; i++ {
		select {
		case <-imported:
		case <-time.After(time.Second):
			t.Fatalf("proof %d: import timeout", i+1)
		}
	}
	verifyProofImportDone(t, imported)
}

// verifyProofImportDone verifies that no more events are arriving on an import channel.
func verifyProofImportDone(t *testing.T, imported chan *types.PlumoProof) {
	select {
	case <-imported:
		t.Fatalf("extra proof imported")
	case <-time.After(50 * time.Millisecond):
	}
}

// Tests that a fetcher accepts proof announcements and initiates retrievals for
// them, successfully importing into the local storage.
func TestSequentialProofAnnouncements(t *testing.T) { testSequentialProofAnnouncements(t) }

func testSequentialProofAnnouncements(t *testing.T) {
	// Create a chain of proofs to import
	targetProofs := 4 * proofLimit
	proofsMetadata, proofs := makeProofs(targetProofs, 1)

	tester := newProofTester()
	proofFetcher := tester.makeProofFetcher(t, "valid", proofs, -gatherSlack)

	// Iteratively announce proofs until all are imported
	imported := make(chan *types.PlumoProof)
	tester.proofFetcher.importedHook = func(proof *types.PlumoProof) { imported <- proof }

	for i := 0; i < len(proofsMetadata); i++ {
		tester.proofFetcher.Notify("valid", proofsMetadata[i], time.Now().Add(-arriveTimeout), proofFetcher)
		verifyProofImportEvent(t, imported, true)
	}
	verifyProofImportDone(t, imported)
}

// Tests that if proofs are announced by multiple peers (or even the same buggy
// peer), they will only get downloaded at most once.
func TestConcurrentProofAnnouncements(t *testing.T) { testConcurrentProofAnnouncements(t) }

func testConcurrentProofAnnouncements(t *testing.T) {
	// Create a chain of proofs to import
	targetProofs := 4 * proofLimit
	proofsMetadata, proofs := makeProofs(targetProofs, 1)

	// Assemble a tester with a built in counter for the requests
	tester := newProofTester()
	firstProofFetcher := tester.makeProofFetcher(t, "first", proofs, -gatherSlack)
	secondProofFetcher := tester.makeProofFetcher(t, "second", proofs, -gatherSlack)

	counter := uint32(0)
	firstProofWrapper := func(metadata []types.PlumoProofMetadata) error {
		atomic.AddUint32(&counter, 1)
		return firstProofFetcher(metadata)
	}
	secondProofWrapper := func(metadata []types.PlumoProofMetadata) error {
		atomic.AddUint32(&counter, 1)
		return secondProofFetcher(metadata)
	}
	// Iteratively announce proofs until all are imported
	imported := make(chan *types.PlumoProof)
	tester.proofFetcher.importedHook = func(proof *types.PlumoProof) { imported <- proof }

	for i := 0; i < len(proofsMetadata); i++ {
		tester.proofFetcher.Notify("first", proofsMetadata[i], time.Now().Add(-arriveTimeout), firstProofWrapper)
		tester.proofFetcher.Notify("second", proofsMetadata[i], time.Now().Add(-arriveTimeout+time.Millisecond), secondProofWrapper)
		tester.proofFetcher.Notify("second", proofsMetadata[i], time.Now().Add(-arriveTimeout-time.Millisecond), secondProofWrapper)
		verifyProofImportEvent(t, imported, true)
	}
	verifyProofImportDone(t, imported)

	// Make sure no proofs were retrieved twice
	if int(counter) != targetProofs {
		t.Fatalf("retrieval count mismatch: have %v, want %v", counter, targetProofs)
	}
}
