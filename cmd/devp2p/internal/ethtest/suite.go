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

package ethtest

import (
	"time"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/eth/protocols/eth"
	"github.com/celo-org/celo-blockchain/internal/utesting"
	"github.com/celo-org/celo-blockchain/p2p/enode"
)

// Suite represents a structure used to test a node's conformance
// to the eth protocol.
type Suite struct {
	Dest *enode.Node

	chain     *Chain
	fullChain *Chain
}

// NewSuite creates and returns a new eth-test suite that can
// be used to test the given node against the given blockchain
// data.
func NewSuite(dest *enode.Node, chainfile string, genesisfile string) (*Suite, error) {
	chain, err := loadChain(chainfile, genesisfile)
	if err != nil {
		return nil, err
	}
	return &Suite{
		Dest:      dest,
		chain:     chain.Shorten(1000),
		fullChain: chain,
	}, nil
}

func (s *Suite) AllEthTests() []utesting.Test {
	return []utesting.Test{
		// status
		{Name: "TestStatus", Fn: s.TestStatus},
		{Name: "TestStatus67", Fn: s.TestStatus67},
		// get block headers
		{Name: "TestGetBlockHeaders", Fn: s.TestGetBlockHeaders},
		{Name: "TestGetBlockHeaders67", Fn: s.TestGetBlockHeaders67},
		{Name: "TestSimultaneousRequests67", Fn: s.TestSimultaneousRequests67},
		{Name: "TestSameRequestID67", Fn: s.TestSameRequestID67},
		{Name: "TestZeroRequestID67", Fn: s.TestZeroRequestID67},
		// get block bodies
		{Name: "TestGetBlockBodies", Fn: s.TestGetBlockBodies},
		{Name: "TestGetBlockBodies67", Fn: s.TestGetBlockBodies67},
		// broadcast
		{Name: "TestBroadcast", Fn: s.TestBroadcast},
		{Name: "TestBroadcast67", Fn: s.TestBroadcast67},
		{Name: "TestLargeAnnounce", Fn: s.TestLargeAnnounce},
		{Name: "TestLargeAnnounce67", Fn: s.TestLargeAnnounce67},
		{Name: "TestOldAnnounce", Fn: s.TestOldAnnounce},
		{Name: "TestOldAnnounce67", Fn: s.TestOldAnnounce67},
		{Name: "TestBlockHashAnnounce", Fn: s.TestBlockHashAnnounce},
		{Name: "TestBlockHashAnnounce67", Fn: s.TestBlockHashAnnounce67},
		// malicious handshakes + status
		{Name: "TestMaliciousHandshake", Fn: s.TestMaliciousHandshake},
		{Name: "TestMaliciousStatus", Fn: s.TestMaliciousStatus},
		{Name: "TestMaliciousHandshake67", Fn: s.TestMaliciousHandshake67},
		{Name: "TestMaliciousStatus67", Fn: s.TestMaliciousStatus67},
		// test transactions
		{Name: "TestTransaction", Fn: s.TestTransaction},
		{Name: "TestTransaction67", Fn: s.TestTransaction67},
		{Name: "TestMaliciousTx", Fn: s.TestMaliciousTx},
		{Name: "TestMaliciousTx67", Fn: s.TestMaliciousTx67},
		{Name: "TestLargeTxRequest67", Fn: s.TestLargeTxRequest67},
		{Name: "TestNewPooledTxs67", Fn: s.TestNewPooledTxs67},
	}
}

func (s *Suite) EthTests() []utesting.Test {
	return []utesting.Test{
		{Name: "TestStatus", Fn: s.TestStatus},
		{Name: "TestGetBlockHeaders", Fn: s.TestGetBlockHeaders},
		{Name: "TestGetBlockBodies", Fn: s.TestGetBlockBodies},
		{Name: "TestBroadcast", Fn: s.TestBroadcast},
		{Name: "TestLargeAnnounce", Fn: s.TestLargeAnnounce},
		{Name: "TestOldAnnounce", Fn: s.TestOldAnnounce},
		{Name: "TestBlockHashAnnounce", Fn: s.TestBlockHashAnnounce},
		{Name: "TestMaliciousHandshake", Fn: s.TestMaliciousHandshake},
		{Name: "TestMaliciousStatus", Fn: s.TestMaliciousStatus},
		{Name: "TestTransaction", Fn: s.TestTransaction},
		{Name: "TestMaliciousTx", Fn: s.TestMaliciousTx},
	}
}

func (s *Suite) Eth67Tests() []utesting.Test {
	return []utesting.Test{
		// only proceed with celo67 (eth66) test suite if node supports celo 67 protocol
		{Name: "TestStatus67", Fn: s.TestStatus67},
		{Name: "TestGetBlockHeaders67", Fn: s.TestGetBlockHeaders67},
		{Name: "TestSimultaneousRequests67", Fn: s.TestSimultaneousRequests67},
		{Name: "TestSameRequestID67", Fn: s.TestSameRequestID67},
		{Name: "TestZeroRequestID67", Fn: s.TestZeroRequestID67},
		{Name: "TestGetBlockBodies67", Fn: s.TestGetBlockBodies67},
		{Name: "TestBroadcast67", Fn: s.TestBroadcast67},
		{Name: "TestLargeAnnounce67", Fn: s.TestLargeAnnounce67},
		{Name: "TestOldAnnounce67", Fn: s.TestOldAnnounce67},
		{Name: "TestBlockHashAnnounce67", Fn: s.TestBlockHashAnnounce67},
		{Name: "TestMaliciousHandshake67", Fn: s.TestMaliciousHandshake67},
		{Name: "TestMaliciousStatus67", Fn: s.TestMaliciousStatus67},
		{Name: "TestTransaction67", Fn: s.TestTransaction67},
		{Name: "TestMaliciousTx67", Fn: s.TestMaliciousTx67},
		{Name: "TestLargeTxRequest67", Fn: s.TestLargeTxRequest67},
		{Name: "TestNewPooledTxs67", Fn: s.TestNewPooledTxs67},
	}
}

var (
	celo67 = true  // indicates whether suite should negotiate celo67 (eth66) connection
	celo66 = false // indicates whether suite should negotiate celo66 (eth65) connection or below.
)

// TestStatus attempts to connect to the given node and exchange
// a status message with it.
func (s *Suite) TestStatus(t *utesting.T) {
	conn, err := s.dial()
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	defer conn.Close()
	if err := conn.peer(s.chain, nil); err != nil {
		t.Fatalf("peering failed: %v", err)
	}
}

// TestStatus67 attempts to connect to the given node and exchange
// a status message with it on the celo67 (eth66) protocol.
func (s *Suite) TestStatus67(t *utesting.T) {
	conn, err := s.dial67()
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	defer conn.Close()
	if err := conn.peer(s.chain, nil); err != nil {
		t.Fatalf("peering failed: %v", err)
	}
}

// TestGetBlockHeaders tests whether the given node can respond to
// a `GetBlockHeaders` request accurately.
func (s *Suite) TestGetBlockHeaders(t *utesting.T) {
	conn, err := s.dial()
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	defer conn.Close()
	if err := conn.peer(s.chain, nil); err != nil {
		t.Fatalf("handshake(s) failed: %v", err)
	}
	// write request
	req := &GetBlockHeaders{
		Origin: eth.HashOrNumber{
			Hash: s.chain.blocks[1].Hash(),
		},
		Amount:  2,
		Skip:    1,
		Reverse: false,
	}
	headers, err := conn.headersRequest(req, s.chain, celo66, 0)
	if err != nil {
		t.Fatalf("GetBlockHeaders request failed: %v", err)
	}
	// check for correct headers
	expected, err := s.chain.GetHeaders(*req)
	if err != nil {
		t.Fatalf("failed to get headers for given request: %v", err)
	}
	if !headersMatch(expected, headers) {
		t.Fatalf("header mismatch: \nexpected %v \ngot %v", expected, headers)
	}
}

// TestGetBlockHeaders67 tests whether the given node can respond to
// an celo67 (eth66) `GetBlockHeaders` request and that the response is accurate.
func (s *Suite) TestGetBlockHeaders67(t *utesting.T) {
	conn, err := s.dial67()
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	defer conn.Close()
	if err = conn.peer(s.chain, nil); err != nil {
		t.Fatalf("peering failed: %v", err)
	}
	// write request
	req := &GetBlockHeaders{
		Origin: eth.HashOrNumber{
			Hash: s.chain.blocks[1].Hash(),
		},
		Amount:  2,
		Skip:    1,
		Reverse: false,
	}
	headers, err := conn.headersRequest(req, s.chain, celo67, 33)
	if err != nil {
		t.Fatalf("could not get block headers: %v", err)
	}
	// check for correct headers
	expected, err := s.chain.GetHeaders(*req)
	if err != nil {
		t.Fatalf("failed to get headers for given request: %v", err)
	}
	if !headersMatch(expected, headers) {
		t.Fatalf("header mismatch: \nexpected %v \ngot %v", expected, headers)
	}
}

// TestSimultaneousRequests67 sends two simultaneous `GetBlockHeader` requests from
// the same connection with different request IDs and checks to make sure the node
// responds with the correct headers per request.
func (s *Suite) TestSimultaneousRequests67(t *utesting.T) {
	// create a connection
	conn, err := s.dial67()
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	defer conn.Close()
	if err := conn.peer(s.chain, nil); err != nil {
		t.Fatalf("peering failed: %v", err)
	}
	// create two requests
	req1 := &eth.GetBlockHeadersPacket67{
		RequestId: uint64(111),
		GetBlockHeadersPacket: &eth.GetBlockHeadersPacket{
			Origin: eth.HashOrNumber{
				Hash: s.chain.blocks[1].Hash(),
			},
			Amount:  2,
			Skip:    1,
			Reverse: false,
		},
	}
	req2 := &eth.GetBlockHeadersPacket67{
		RequestId: uint64(222),
		GetBlockHeadersPacket: &eth.GetBlockHeadersPacket{
			Origin: eth.HashOrNumber{
				Hash: s.chain.blocks[1].Hash(),
			},
			Amount:  4,
			Skip:    1,
			Reverse: false,
		},
	}
	// write the first request
	if err := conn.Write67(req1, GetBlockHeaders{}.Code()); err != nil {
		t.Fatalf("failed to write to connection: %v", err)
	}
	// write the second request
	if err := conn.Write67(req2, GetBlockHeaders{}.Code()); err != nil {
		t.Fatalf("failed to write to connection: %v", err)
	}
	// wait for responses
	msg := conn.waitForResponse(s.chain, timeout, req1.RequestId)
	headers1, ok := msg.(BlockHeaders)
	if !ok {
		t.Fatalf("unexpected %s", pretty.Sdump(msg))
	}
	msg = conn.waitForResponse(s.chain, timeout, req2.RequestId)
	headers2, ok := msg.(BlockHeaders)
	if !ok {
		t.Fatalf("unexpected %s", pretty.Sdump(msg))
	}
	// check received headers for accuracy
	expected1, err := s.chain.GetHeaders(GetBlockHeaders(*req1.GetBlockHeadersPacket))
	if err != nil {
		t.Fatalf("failed to get expected headers for request 1: %v", err)
	}
	expected2, err := s.chain.GetHeaders(GetBlockHeaders(*req2.GetBlockHeadersPacket))
	if err != nil {
		t.Fatalf("failed to get expected headers for request 2: %v", err)
	}
	if !headersMatch(expected1, headers1) {
		t.Fatalf("header mismatch: \nexpected %v \ngot %v", expected1, headers1)
	}
	if !headersMatch(expected2, headers2) {
		t.Fatalf("header mismatch: \nexpected %v \ngot %v", expected2, headers2)
	}
}

// TestSameRequestID_67 sends two requests with the same request ID to a
// single node.
func (s *Suite) TestSameRequestID67(t *utesting.T) {
	conn, err := s.dial67()
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	defer conn.Close()
	if err := conn.peer(s.chain, nil); err != nil {
		t.Fatalf("peering failed: %v", err)
	}
	// create requests
	reqID := uint64(1234)
	request1 := &eth.GetBlockHeadersPacket67{
		RequestId: reqID,
		GetBlockHeadersPacket: &eth.GetBlockHeadersPacket{
			Origin: eth.HashOrNumber{
				Number: 1,
			},
			Amount: 2,
		},
	}
	request2 := &eth.GetBlockHeadersPacket67{
		RequestId: reqID,
		GetBlockHeadersPacket: &eth.GetBlockHeadersPacket{
			Origin: eth.HashOrNumber{
				Number: 33,
			},
			Amount: 2,
		},
	}
	// write the requests
	if err = conn.Write67(request1, GetBlockHeaders{}.Code()); err != nil {
		t.Fatalf("failed to write to connection: %v", err)
	}
	if err = conn.Write67(request2, GetBlockHeaders{}.Code()); err != nil {
		t.Fatalf("failed to write to connection: %v", err)
	}
	// wait for responses
	msg := conn.waitForResponse(s.chain, timeout, reqID)
	headers1, ok := msg.(BlockHeaders)
	if !ok {
		t.Fatalf("unexpected %s", pretty.Sdump(msg))
	}
	msg = conn.waitForResponse(s.chain, timeout, reqID)
	headers2, ok := msg.(BlockHeaders)
	if !ok {
		t.Fatalf("unexpected %s", pretty.Sdump(msg))
	}
	// check if headers match
	expected1, err := s.chain.GetHeaders(GetBlockHeaders(*request1.GetBlockHeadersPacket))
	if err != nil {
		t.Fatalf("failed to get expected block headers: %v", err)
	}
	expected2, err := s.chain.GetHeaders(GetBlockHeaders(*request2.GetBlockHeadersPacket))
	if err != nil {
		t.Fatalf("failed to get expected block headers: %v", err)
	}
	if !headersMatch(expected1, headers1) {
		t.Fatalf("header mismatch: \nexpected %v \ngot %v", expected1, headers1)
	}
	if !headersMatch(expected2, headers2) {
		t.Fatalf("header mismatch: \nexpected %v \ngot %v", expected2, headers2)
	}
}

// TestZeroRequestID_67 checks that a message with a request ID of zero is still handled
// by the node.
func (s *Suite) TestZeroRequestID67(t *utesting.T) {
	conn, err := s.dial67()
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	defer conn.Close()
	if err := conn.peer(s.chain, nil); err != nil {
		t.Fatalf("peering failed: %v", err)
	}
	req := &GetBlockHeaders{
		Origin: eth.HashOrNumber{
			Number: 0,
		},
		Amount: 2,
	}
	headers, err := conn.headersRequest(req, s.chain, celo67, 0)
	if err != nil {
		t.Fatalf("failed to get block headers: %v", err)
	}
	expected, err := s.chain.GetHeaders(*req)
	if err != nil {
		t.Fatalf("failed to get expected block headers: %v", err)
	}
	if !headersMatch(expected, headers) {
		t.Fatalf("header mismatch: \nexpected %v \ngot %v", expected, headers)
	}
}

// TestGetBlockBodies tests whether the given node can respond to
// a `GetBlockBodies` request and that the response is accurate.
func (s *Suite) TestGetBlockBodies(t *utesting.T) {
	conn, err := s.dial()
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	defer conn.Close()
	if err := conn.peer(s.chain, nil); err != nil {
		t.Fatalf("peering failed: %v", err)
	}
	// create block bodies request
	req := &GetBlockBodies{
		s.chain.blocks[54].Hash(),
		s.chain.blocks[75].Hash(),
	}
	if err := conn.Write(req); err != nil {
		t.Fatalf("could not write to connection: %v", err)
	}
	// wait for response
	switch msg := conn.readAndServe(s.chain, timeout).(type) {
	case *BlockBodies:
		t.Logf("received %d block bodies", len(*msg))
		if len(*msg) != len(*req) {
			t.Fatalf("wrong bodies in response: expected %d bodies, "+
				"got %d", len(*req), len(*msg))
		}
	default:
		t.Fatalf("unexpected: %s", pretty.Sdump(msg))
	}
}

// TestGetBlockBodies67 tests whether the given node can respond to
// a `GetBlockBodies` request and that the response is accurate over
// the celo67 (eth66) protocol.
func (s *Suite) TestGetBlockBodies67(t *utesting.T) {
	conn, err := s.dial67()
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	defer conn.Close()
	if err := conn.peer(s.chain, nil); err != nil {
		t.Fatalf("peering failed: %v", err)
	}
	// create block bodies request
	req := &eth.GetBlockBodiesPacket67{
		RequestId: uint64(55),
		GetBlockBodiesPacket: eth.GetBlockBodiesPacket{
			s.chain.blocks[54].Hash(),
			s.chain.blocks[75].Hash(),
		},
	}
	if err := conn.Write67(req, GetBlockBodies{}.Code()); err != nil {
		t.Fatalf("could not write to connection: %v", err)
	}
	// wait for block bodies response
	msg := conn.waitForResponse(s.chain, timeout, req.RequestId)
	blockBodies, ok := msg.(BlockBodies)
	if !ok {
		t.Fatalf("unexpected: %s", pretty.Sdump(msg))
	}
	t.Logf("received %d block bodies", len(blockBodies))
	if len(blockBodies) != len(req.GetBlockBodiesPacket) {
		t.Fatalf("wrong bodies in response: expected %d bodies, "+
			"got %d", len(req.GetBlockBodiesPacket), len(blockBodies))
	}
}

// TestBroadcast tests whether a block announcement is correctly
// propagated to the given node's peer(s).
func (s *Suite) TestBroadcast(t *utesting.T) {
	if err := s.sendNextBlock(celo66); err != nil {
		t.Fatalf("block broadcast failed: %v", err)
	}
}

// TestBroadcast67 tests whether a block announcement is correctly
// propagated to the given node's peer(s) on the celo67 (eth66) protocol.
func (s *Suite) TestBroadcast67(t *utesting.T) {
	if err := s.sendNextBlock(celo67); err != nil {
		t.Fatalf("block broadcast failed: %v", err)
	}
}

// TestLargeAnnounce tests the announcement mechanism with a large block.
func (s *Suite) TestLargeAnnounce(t *utesting.T) {
	nextBlock := len(s.chain.blocks)
	blocks := []*NewBlock{
		{
			Block: largeBlock(),
			TD:    s.fullChain.TotalDifficultyAt(nextBlock),
		},
		{
			Block: s.fullChain.blocks[nextBlock],
			TD:    largeNumber(2),
		},
		{
			Block: largeBlock(),
			TD:    largeNumber(2),
		},
	}

	for i, blockAnnouncement := range blocks {
		t.Logf("Testing malicious announcement: %v\n", i)
		conn, err := s.dial()
		if err != nil {
			t.Fatalf("dial failed: %v", err)
		}
		if err = conn.peer(s.chain, nil); err != nil {
			t.Fatalf("peering failed: %v", err)
		}
		if err = conn.Write(blockAnnouncement); err != nil {
			t.Fatalf("could not write to connection: %v", err)
		}
		// Invalid announcement, check that peer disconnected
		switch msg := conn.readAndServe(s.chain, time.Second*8).(type) {
		case *Disconnect:
		case *Error:
			break
		default:
			t.Fatalf("unexpected: %s wanted disconnect", pretty.Sdump(msg))
		}
		conn.Close()
	}
	// Test the last block as a valid block
	if err := s.sendNextBlock(celo66); err != nil {
		t.Fatalf("failed to broadcast next block: %v", err)
	}
}

// TestLargeAnnounce67 tests the announcement mechanism with a large
// block over the celo67 (eth66) protocol.
func (s *Suite) TestLargeAnnounce67(t *utesting.T) {
	nextBlock := len(s.chain.blocks)
	blocks := []*NewBlock{
		{
			Block: largeBlock(),
			TD:    s.fullChain.TotalDifficultyAt(nextBlock),
		},
		{
			Block: s.fullChain.blocks[nextBlock],
			TD:    largeNumber(2),
		},
		{
			Block: largeBlock(),
			TD:    largeNumber(2),
		},
	}

	for i, blockAnnouncement := range blocks[0:3] {
		t.Logf("Testing malicious announcement: %v\n", i)
		conn, err := s.dial67()
		if err != nil {
			t.Fatalf("dial failed: %v", err)
		}
		if err := conn.peer(s.chain, nil); err != nil {
			t.Fatalf("peering failed: %v", err)
		}
		if err := conn.Write(blockAnnouncement); err != nil {
			t.Fatalf("could not write to connection: %v", err)
		}
		// Invalid announcement, check that peer disconnected
		switch msg := conn.readAndServe(s.chain, time.Second*8).(type) {
		case *Disconnect:
		case *Error:
			break
		default:
			t.Fatalf("unexpected: %s wanted disconnect", pretty.Sdump(msg))
		}
		conn.Close()
	}
	// Test the last block as a valid block
	if err := s.sendNextBlock(celo67); err != nil {
		t.Fatalf("failed to broadcast next block: %v", err)
	}
}

// TestOldAnnounce tests the announcement mechanism with an old block.
func (s *Suite) TestOldAnnounce(t *utesting.T) {
	if err := s.oldAnnounce(celo66); err != nil {
		t.Fatal(err)
	}
}

// TestOldAnnounce67 tests the announcement mechanism with an old block,
// over the celo67 (eth66) protocol.
func (s *Suite) TestOldAnnounce67(t *utesting.T) {
	if err := s.oldAnnounce(celo67); err != nil {
		t.Fatal(err)
	}
}

// TestBlockHashAnnounce sends a new block hash announcement and expects
// the node to perform a `GetBlockHeaders` request.
func (s *Suite) TestBlockHashAnnounce(t *utesting.T) {
	if err := s.hashAnnounce(celo66); err != nil {
		t.Fatalf("block hash announcement failed: %v", err)
	}
}

// TestBlockHashAnnounce67 sends a new block hash announcement and expects
// the node to perform a `GetBlockHeaders` request.
func (s *Suite) TestBlockHashAnnounce67(t *utesting.T) {
	if err := s.hashAnnounce(celo67); err != nil {
		t.Fatalf("block hash announcement failed: %v", err)
	}
}

// TestMaliciousHandshake tries to send malicious data during the handshake.
func (s *Suite) TestMaliciousHandshake(t *utesting.T) {
	if err := s.maliciousHandshakes(t, celo66); err != nil {
		t.Fatal(err)
	}
}

// TestMaliciousHandshake67 tries to send malicious data during the handshake.
func (s *Suite) TestMaliciousHandshake67(t *utesting.T) {
	if err := s.maliciousHandshakes(t, celo67); err != nil {
		t.Fatal(err)
	}
}

// TestMaliciousStatus sends a status package with a large total difficulty.
func (s *Suite) TestMaliciousStatus(t *utesting.T) {
	conn, err := s.dial()
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	defer conn.Close()

	if err := s.maliciousStatus(conn); err != nil {
		t.Fatal(err)
	}
}

// TestMaliciousStatus67 sends a status package with a large total
// difficulty over the celo67 (eth66) protocol.
func (s *Suite) TestMaliciousStatus67(t *utesting.T) {
	conn, err := s.dial67()
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	defer conn.Close()

	if err := s.maliciousStatus(conn); err != nil {
		t.Fatal(err)
	}
}

// TestTransaction sends a valid transaction to the node and
// checks if the transaction gets propagated.
func (s *Suite) TestTransaction(t *utesting.T) {
	if err := s.sendSuccessfulTxs(t, celo66); err != nil {
		t.Fatal(err)
	}
}

// TestTransaction67 sends a valid transaction to the node and
// checks if the transaction gets propagated.
func (s *Suite) TestTransaction67(t *utesting.T) {
	if err := s.sendSuccessfulTxs(t, celo67); err != nil {
		t.Fatal(err)
	}
}

// TestMaliciousTx sends several invalid transactions and tests whether
// the node will propagate them.
func (s *Suite) TestMaliciousTx(t *utesting.T) {
	if err := s.sendMaliciousTxs(t, celo66); err != nil {
		t.Fatal(err)
	}
}

// TestMaliciousTx67 sends several invalid transactions and tests whether
// the node will propagate them.
func (s *Suite) TestMaliciousTx67(t *utesting.T) {
	if err := s.sendMaliciousTxs(t, celo67); err != nil {
		t.Fatal(err)
	}
}

// TestLargeTxRequest67 tests whether a node can fulfill a large GetPooledTransactions
// request.
func (s *Suite) TestLargeTxRequest67(t *utesting.T) {
	// send the next block to ensure the node is no longer syncing and
	// is able to accept txs
	if err := s.sendNextBlock(celo67); err != nil {
		t.Fatalf("failed to send next block: %v", err)
	}
	// send 2000 transactions to the node
	hashMap, txs, err := generateTxs(s, 2000)
	if err != nil {
		t.Fatalf("failed to generate transactions: %v", err)
	}
	if err = sendMultipleSuccessfulTxs(t, s, txs); err != nil {
		t.Fatalf("failed to send multiple txs: %v", err)
	}
	// set up connection to receive to ensure node is peered with the receiving connection
	// before tx request is sent
	conn, err := s.dial67()
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	defer conn.Close()
	if err = conn.peer(s.chain, nil); err != nil {
		t.Fatalf("peering failed: %v", err)
	}
	// create and send pooled tx request
	hashes := make([]common.Hash, 0)
	for _, hash := range hashMap {
		hashes = append(hashes, hash)
	}
	getTxReq := &eth.GetPooledTransactionsPacket67{
		RequestId:                   1234,
		GetPooledTransactionsPacket: hashes,
	}
	if err = conn.Write67(getTxReq, GetPooledTransactions{}.Code()); err != nil {
		t.Fatalf("could not write to conn: %v", err)
	}
	// check that all received transactions match those that were sent to node
	switch msg := conn.waitForResponse(s.chain, timeout, getTxReq.RequestId).(type) {
	case PooledTransactions:
		for _, gotTx := range msg {
			if _, exists := hashMap[gotTx.Hash()]; !exists {
				t.Fatalf("unexpected tx received: %v", gotTx.Hash())
			}
		}
	default:
		t.Fatalf("unexpected %s", pretty.Sdump(msg))
	}
}

// TestNewPooledTxs67 tests whether a node will do a GetPooledTransactions
// request upon receiving a NewPooledTransactionHashes announcement.
func (s *Suite) TestNewPooledTxs67(t *utesting.T) {
	// send the next block to ensure the node is no longer syncing and
	// is able to accept txs
	if err := s.sendNextBlock(celo67); err != nil {
		t.Fatalf("failed to send next block: %v", err)
	}

	// generate 50 txs
	hashMap, _, err := generateTxs(s, 50)
	if err != nil {
		t.Fatalf("failed to generate transactions: %v", err)
	}

	// create new pooled tx hashes announcement
	hashes := make([]common.Hash, 0)
	for _, hash := range hashMap {
		hashes = append(hashes, hash)
	}
	announce := NewPooledTransactionHashes(hashes)

	// send announcement
	conn, err := s.dial67()
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	defer conn.Close()
	if err = conn.peer(s.chain, nil); err != nil {
		t.Fatalf("peering failed: %v", err)
	}
	if err = conn.Write(announce); err != nil {
		t.Fatalf("failed to write to connection: %v", err)
	}

	// wait for GetPooledTxs request
	for {
		_, msg := conn.readAndServe67(s.chain, timeout)
		switch msg := msg.(type) {
		case GetPooledTransactions:
			if len(msg) != len(hashes) {
				t.Fatalf("unexpected number of txs requested: wanted %d, got %d", len(hashes), len(msg))
			}
			return
		// ignore propagated txs from previous tests
		case *NewPooledTransactionHashes:
			continue
		// ignore block announcements from previous tests
		case *NewBlockHashes:
			continue
		case *NewBlock:
			continue
		default:
			t.Fatalf("unexpected %s", pretty.Sdump(msg))
		}
	}
}
