// Copyright 2020 The go-ethereum Authors
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

package les

import (
	"math/big"
	"testing"
	"time"

	mockEngine "github.com/celo-org/celo-blockchain/consensus/consensustest"
	"github.com/celo-org/celo-blockchain/core"
	"github.com/celo-org/celo-blockchain/core/rawdb"
	"github.com/celo-org/celo-blockchain/core/types"
	"github.com/celo-org/celo-blockchain/eth/downloader"
	"github.com/celo-org/celo-blockchain/p2p/enode"
)

// verifyImportEvent verifies that one single event arrive on an import channel.
func verifyImportEvent(t *testing.T, imported chan interface{}, arrive bool) {
	if arrive {
		select {
		case <-imported:
		case <-time.After(time.Second):
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

// verifyImportDone verifies that no more events are arriving on an import channel.
func verifyImportDone(t *testing.T, imported chan interface{}) {
	select {
	case <-imported:
		t.Fatalf("extra block imported")
	case <-time.After(50 * time.Millisecond):
	}
}

// verifyChainHeight verifies the chain height is as expected.
func verifyChainHeight(t *testing.T, fetcher *lightFetcher, height uint64) {
	local := fetcher.chain.CurrentHeader().Number.Uint64()
	if local != height {
		t.Fatalf("chain height mismatch, got %d, want %d", local, height)
	}
}

func TestSequentialAnnouncementsLes2(t *testing.T) {
	testSequentialAnnouncements(t, downloader.LightSync, lpv2)
}
func TestSequentialAnnouncementsUltraLes2(t *testing.T) {
	testSequentialAnnouncements(t, downloader.LightestSync, lpv2)
}
func TestSequentialAnnouncementsLes3(t *testing.T) {
	testSequentialAnnouncements(t, downloader.LightSync, lpv3)
}
func TestSequentialAnnouncementsUltraLes3(t *testing.T) {
	testSequentialAnnouncements(t, downloader.LightestSync, lpv3)
}
func TestSequentialAnnouncementsLes4(t *testing.T) {
	testSequentialAnnouncements(t, downloader.LightSync, lpv4)
}
func TestSequentialAnnouncementsUltraLes4(t *testing.T) {
	testSequentialAnnouncements(t, downloader.LightestSync, lpv4)
}
func TestSequentialAnnouncementsLes5(t *testing.T) {
	testSequentialAnnouncements(t, downloader.LightSync, lpv5)
}
func TestSequentialAnnouncementsUltraLes5(t *testing.T) {
	testSequentialAnnouncements(t, downloader.LightestSync, lpv5)
}

func testSequentialAnnouncements(t *testing.T, syncMode downloader.SyncMode, protocol int) {
	netconfig := testnetConfig{
		blocks:    4,
		syncMode:  syncMode,
		protocol:  protocol,
		nopruning: true,
	}
	s, c, teardown := newClientServerEnv(t, netconfig)
	defer teardown()

	// Create connected peer pair.
	c.handler.fetcher.noAnnounce = true // Ignore the first announce from peer which can trigger a resync.
	p1, _, err := newTestPeerPair("peer", protocol, s.handler, c.handler)
	if err != nil {
		t.Fatalf("Failed to create peer pair %v", err)
	}
	c.handler.fetcher.noAnnounce = false

	importCh := make(chan interface{})
	c.handler.fetcher.newHeadHook = func(header *types.Header) {
		importCh <- header
	}
	for i := uint64(1); i <= s.backend.Blockchain().CurrentHeader().Number.Uint64(); i++ {
		header := s.backend.Blockchain().GetHeaderByNumber(i)
		hash, number := header.Hash(), header.Number.Uint64()
		td := rawdb.ReadTd(s.db, hash, number)

		announce := announceData{hash, number, td, 0, nil}
		if p1.cpeer.announceType == announceTypeSigned {
			announce.sign(s.handler.server.privateKey)
		}
		p1.cpeer.sendAnnounce(announce)
		verifyImportEvent(t, importCh, true)
	}
	verifyImportDone(t, importCh)
	verifyChainHeight(t, c.handler.fetcher, 4)
}

func TestGappedAnnouncementsLes2(t *testing.T) {
	testGappedAnnouncements(t, downloader.LightSync, lpv2)
}
func TestGappedAnnouncementsUltraLes2(t *testing.T) {
	t.Skip("added in the les refactor, check if necessary for LightestSync")
	testGappedAnnouncements(t, downloader.LightestSync, 2)
}
func TestGappedAnnouncementsLes3(t *testing.T) {
	testGappedAnnouncements(t, downloader.LightSync, lpv3)
}
func TestGappedAnnouncementsUltraLes3(t *testing.T) {
	t.Skip("added in the les refactor, check if necessary for LightestSync")
	testGappedAnnouncements(t, downloader.LightestSync, lpv3)
}
func TestGappedAnnouncementsLes4(t *testing.T) {
	testGappedAnnouncements(t, downloader.LightSync, lpv4)
}
func TestGappedAnnouncementsUltraLes4(t *testing.T) {
	t.Skip("added in the les refactor, check if necessary for LightestSync")
	testGappedAnnouncements(t, downloader.LightestSync, lpv4)
}
func TestGappedAnnouncementsLes5(t *testing.T) {
	testGappedAnnouncements(t, downloader.LightSync, lpv5)
}
func TestGappedAnnouncementsUltraLes5(t *testing.T) {
	t.Skip("added in the les refactor, check if necessary for LightestSync")
	testGappedAnnouncements(t, downloader.LightestSync, lpv5)
}

func testGappedAnnouncements(t *testing.T, syncMode downloader.SyncMode, protocol int) {
	netconfig := testnetConfig{
		blocks:    4,
		syncMode:  syncMode,
		protocol:  protocol,
		nopruning: true,
	}
	s, c, teardown := newClientServerEnv(t, netconfig)
	defer teardown()

	// Create connected peer pair.
	c.handler.fetcher.noAnnounce = true // Ignore the first announce from peer which can trigger a resync.
	peer, _, err := newTestPeerPair("peer", protocol, s.handler, c.handler)
	if err != nil {
		t.Fatalf("Failed to create peer pair %v", err)
	}
	c.handler.fetcher.noAnnounce = false

	done := make(chan *types.Header, 1)
	c.handler.fetcher.newHeadHook = func(header *types.Header) { done <- header }

	// Prepare announcement by latest header.
	latest := s.backend.Blockchain().CurrentHeader()
	hash, number := latest.Hash(), latest.Number.Uint64()
	td := rawdb.ReadTd(s.db, hash, number)

	// Sign the announcement if necessary.
	announce := announceData{hash, number, td, 0, nil}
	if peer.cpeer.announceType == announceTypeSigned {
		announce.sign(s.handler.server.privateKey)
	}
	peer.cpeer.sendAnnounce(announce)

	<-done // Wait syncing
	verifyChainHeight(t, c.handler.fetcher, 4)

	// Send a reorged announcement
	var newAnno = make(chan struct{}, 1)
	c.handler.fetcher.noAnnounce = true
	c.handler.fetcher.newAnnounce = func(*serverPeer, *announceData) {
		newAnno <- struct{}{}
	}
	blocks, _ := core.GenerateChain(rawdb.ReadChainConfig(s.db, s.backend.Blockchain().Genesis().Hash()), s.backend.Blockchain().GetBlockByNumber(3),
		mockEngine.NewFaker(), s.db, 2, func(i int, gen *core.BlockGen) {
			gen.OffsetTime(-9) // higher block difficulty
		})
	s.backend.Blockchain().InsertChain(blocks)
	<-newAnno
	c.handler.fetcher.noAnnounce = false
	c.handler.fetcher.newAnnounce = nil

	latest = blocks[len(blocks)-1].Header()
	hash, number = latest.Hash(), latest.Number.Uint64()
	td = rawdb.ReadTd(s.db, hash, number)

	announce = announceData{hash, number, td, 1, nil}
	if peer.cpeer.announceType == announceTypeSigned {
		announce.sign(s.handler.server.privateKey)
	}
	peer.cpeer.sendAnnounce(announce)

	<-done // Wait syncing
	verifyChainHeight(t, c.handler.fetcher, 5)
}

func TestTrustedAnnouncementsLes2(t *testing.T) {
	testTrustedAnnouncement(t, downloader.LightSync, lpv2)
}

func TestTrustedAnnouncementsUltraLes2(t *testing.T) {
	t.Skip("added in the les refactor, check if necessary for LightestSync")
	testTrustedAnnouncement(t, downloader.LightestSync, lpv2)
}
func TestTrustedAnnouncementsLes3(t *testing.T) {
	testTrustedAnnouncement(t, downloader.LightSync, lpv3)
}

func TestTrustedAnnouncementsUltraLes3(t *testing.T) {
	t.Skip("added in the les refactor, check if necessary for LightestSync")
	testTrustedAnnouncement(t, downloader.LightestSync, lpv3)
}

func TestTrustedAnnouncementsLes4(t *testing.T) {
	testTrustedAnnouncement(t, downloader.LightSync, lpv4)
}

func TestTrustedAnnouncementsUltraLes4(t *testing.T) {
	t.Skip("added in the les refactor, check if necessary for LightestSync")
	testTrustedAnnouncement(t, downloader.LightestSync, lpv4)
}

func TestTrustedAnnouncementsLes5(t *testing.T) {
	testTrustedAnnouncement(t, downloader.LightSync, lpv5)
}

func TestTrustedAnnouncementsUltraLes5(t *testing.T) {
	t.Skip("added in the les refactor, check if necessary for LightestSync")
	testTrustedAnnouncement(t, downloader.LightestSync, lpv5)
}

func testTrustedAnnouncement(t *testing.T, syncMode downloader.SyncMode, protocol int) {
	var (
		servers   []*testServer
		teardowns []func()
		nodes     []*enode.Node
		ids       []string
		cpeers    []*clientPeer
		// speers    []*serverPeer
	)
	for i := 0; i < 10; i++ {
		s, n, teardown := newTestServerPeer(t, 10, protocol)

		servers = append(servers, s)
		nodes = append(nodes, n)
		teardowns = append(teardowns, teardown)

		// A half of them are trusted servers.
		if i < 5 {
			ids = append(ids, n.String())
		}
	}
	netconfig := testnetConfig{
		protocol:    protocol,
		syncMode:    syncMode,
		nopruning:   true,
		ulcServers:  ids,
		ulcFraction: 60,
	}
	_, c, teardown := newClientServerEnv(t, netconfig)
	defer teardown()
	defer func() {
		for i := 0; i < len(teardowns); i++ {
			teardowns[i]()
		}
	}()

	c.handler.fetcher.noAnnounce = true // Ignore the first announce from peer which can trigger a resync.

	// Connect all server instances.
	for i := 0; i < len(servers); i++ {
		_, cp, err := connect(servers[i].handler, nodes[i].ID(), c.handler, protocol)
		if err != nil {
			t.Fatalf("connect server and client failed, err %s", err)
		}
		cpeers = append(cpeers, cp)
		// speers = append(speers, sp)
	}
	c.handler.fetcher.noAnnounce = false

	newHead := make(chan *types.Header, 1)
	c.handler.fetcher.newHeadHook = func(header *types.Header) { newHead <- header }

	check := func(height []uint64, expected uint64, callback func()) {
		for i := 0; i < len(height); i++ {
			for j := 0; j < len(servers); j++ {
				h := servers[j].backend.Blockchain().GetHeaderByNumber(height[i])
				hash, number := h.Hash(), h.Number.Uint64()
				td := rawdb.ReadTd(servers[j].db, hash, number)

				// Sign the announcement if necessary.
				announce := announceData{hash, number, td, 0, nil}
				p := cpeers[j]
				if p.announceType == announceTypeSigned {
					announce.sign(servers[j].handler.server.privateKey)
				}
				p.sendAnnounce(announce)
			}
		}
		if callback != nil {
			callback()
		}
		verifyChainHeight(t, c.handler.fetcher, expected)
	}
	check([]uint64{1}, 1, func() { <-newHead })   // Sequential announcements
	check([]uint64{4}, 4, func() { <-newHead })   // ULC-style light syncing, rollback untrusted headers
	check([]uint64{10}, 10, func() { <-newHead }) // Sync the whole chain.
}

func TestInvalidAnnouncesLES2(t *testing.T) { testInvalidAnnounces(t, downloader.LightSync, lpv2) }
func TestInvalidAnnouncesLES3(t *testing.T) { testInvalidAnnounces(t, downloader.LightSync, lpv3) }
func TestInvalidAnnouncesLES4(t *testing.T) { testInvalidAnnounces(t, downloader.LightSync, lpv4) }
func TestInvalidAnnouncesLES5(t *testing.T) { testInvalidAnnounces(t, downloader.LightSync, lpv5) }

func testInvalidAnnounces(t *testing.T, syncMode downloader.SyncMode, protocol int) {
	netconfig := testnetConfig{
		blocks:    4,
		syncMode:  syncMode,
		protocol:  protocol,
		nopruning: true,
	}
	s, c, teardown := newClientServerEnv(t, netconfig)
	defer teardown()

	// Create connected peer pair.
	c.handler.fetcher.noAnnounce = true // Ignore the first announce from peer which can trigger a resync.
	peer, _, err := newTestPeerPair("peer", lpv3, s.handler, c.handler)
	if err != nil {
		t.Fatalf("Failed to create peer pair %v", err)
	}
	c.handler.fetcher.noAnnounce = false

	done := make(chan *types.Header, 1)
	c.handler.fetcher.newHeadHook = func(header *types.Header) { done <- header }

	// Prepare announcement by latest header.
	headerOne := s.backend.Blockchain().GetHeaderByNumber(1)
	hash, number := headerOne.Hash(), headerOne.Number.Uint64()
	td := big.NewInt(200) // bad td

	// Sign the announcement if necessary.
	announce := announceData{hash, number, td, 0, nil}
	if peer.cpeer.announceType == announceTypeSigned {
		announce.sign(s.handler.server.privateKey)
	}
	peer.cpeer.sendAnnounce(announce)
	<-done // Wait syncing

	// Ensure the bad peer is evicited
	if c.handler.backend.peers.len() != 0 {
		t.Fatalf("Failed to evict invalid peer")
	}
}
