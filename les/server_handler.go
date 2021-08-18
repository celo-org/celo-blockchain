// Copyright 2019 The go-ethereum Authors
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
	"errors"
	"fmt"
	"math/big"
	"sync"
	"sync/atomic"
	"time"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/common/mclock"
	"github.com/celo-org/celo-blockchain/core"
	"github.com/celo-org/celo-blockchain/core/forkid"
	"github.com/celo-org/celo-blockchain/core/rawdb"
	"github.com/celo-org/celo-blockchain/core/state"
	"github.com/celo-org/celo-blockchain/core/types"
	"github.com/celo-org/celo-blockchain/ethdb"
	"github.com/celo-org/celo-blockchain/light"
	"github.com/celo-org/celo-blockchain/log"
	"github.com/celo-org/celo-blockchain/metrics"
	"github.com/celo-org/celo-blockchain/p2p"
	"github.com/celo-org/celo-blockchain/p2p/enode"
	"github.com/celo-org/celo-blockchain/p2p/nodestate"
	"github.com/celo-org/celo-blockchain/rlp"
	"github.com/celo-org/celo-blockchain/trie"
	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/common/mclock"
	"github.com/celo-org/celo-blockchain/core"
	"github.com/celo-org/celo-blockchain/core/forkid"
	"github.com/celo-org/celo-blockchain/core/rawdb"
	"github.com/celo-org/celo-blockchain/core/state"
	"github.com/celo-org/celo-blockchain/core/types"
	"github.com/celo-org/celo-blockchain/ethdb"
	"github.com/celo-org/celo-blockchain/light"
	"github.com/celo-org/celo-blockchain/log"
	"github.com/celo-org/celo-blockchain/metrics"
	"github.com/celo-org/celo-blockchain/p2p"
	"github.com/celo-org/celo-blockchain/p2p/enode"
	"github.com/celo-org/celo-blockchain/p2p/nodestate"
	"github.com/celo-org/celo-blockchain/rlp"
	"github.com/celo-org/celo-blockchain/trie"
	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/common/mclock"
	"github.com/celo-org/celo-blockchain/core"
	"github.com/celo-org/celo-blockchain/core/forkid"
	"github.com/celo-org/celo-blockchain/core/rawdb"
	"github.com/celo-org/celo-blockchain/core/state"
	"github.com/celo-org/celo-blockchain/core/types"
	"github.com/celo-org/celo-blockchain/ethdb"
	"github.com/celo-org/celo-blockchain/les/flowcontrol"
	"github.com/celo-org/celo-blockchain/light"
	"github.com/celo-org/celo-blockchain/log"
	"github.com/celo-org/celo-blockchain/metrics"
	"github.com/celo-org/celo-blockchain/p2p"
	"github.com/celo-org/celo-blockchain/rlp"
	"github.com/celo-org/celo-blockchain/trie"
)

const (
	softResponseLimit = 2 * 1024 * 1024 // Target maximum size of returned blocks, headers or node data.
	estHeaderRlpSize  = 500             // Approximate size of an RLP encoded block header
<<<<<<< HEAD
	ethVersion        = 64              // equivalent eth version for the downloader
||||||| e78727290
	ethVersion        = 63              // equivalent eth version for the downloader
=======
>>>>>>> v1.10.7

	MaxHeaderFetch           = 192 // Amount of block headers to be fetched per retrieval request
	MaxBodyFetch             = 32  // Amount of block bodies to be fetched per retrieval request
	MaxReceiptFetch          = 128 // Amount of transaction receipts to allow fetching per request
	MaxCodeFetch             = 64  // Amount of contract codes to allow fetching per request
	MaxProofsFetch           = 64  // Amount of merkle proofs to be fetched per retrieval request
	MaxHelperTrieProofsFetch = 64  // Amount of helper tries to be fetched per retrieval request
	MaxTxSend                = 64  // Amount of transactions to be send per request
	MaxTxStatus              = 256 // Amount of transactions to queried per request
	MaxEtherbase             = 1
	MaxGatewayFee            = 1
)

var (
	errTooManyInvalidRequest = errors.New("too many invalid requests made")
)

// serverHandler is responsible for serving light client and process
// all incoming light requests.
type serverHandler struct {
	forkFilter forkid.Filter
	blockchain *core.BlockChain
	chainDb    ethdb.Database
	txpool     *core.TxPool
	server     *LesServer

	closeCh chan struct{}  // Channel used to exit all background routines of handler.
	wg      sync.WaitGroup // WaitGroup used to track all background routines of handler.
	synced  func() bool    // Callback function used to determine whether local node is synced.

	// Celo Specific
	etherbase  common.Address
	gatewayFee *big.Int

	// Testing fields
	addTxsSync bool
}

func newServerHandler(server *LesServer, blockchain *core.BlockChain, chainDb ethdb.Database, txpool *core.TxPool, synced func() bool, etherbase common.Address, gatewayFee *big.Int) *serverHandler {
	handler := &serverHandler{
		forkFilter: forkid.NewFilter(blockchain),
		server:     server,
		blockchain: blockchain,
		chainDb:    chainDb,
		txpool:     txpool,
		closeCh:    make(chan struct{}),
		synced:     synced,
		etherbase:  etherbase,
		gatewayFee: gatewayFee,
	}
	return handler
}

// start starts the server handler.
func (h *serverHandler) start() {
	h.wg.Add(1)
	go h.broadcastLoop()
}

// stop stops the server handler.
func (h *serverHandler) stop() {
	close(h.closeCh)
	h.wg.Wait()
}

// runPeer is the p2p protocol run function for the given version.
func (h *serverHandler) runPeer(version uint, p *p2p.Peer, rw p2p.MsgReadWriter) error {
	peer := newClientPeer(int(version), h.server.config.NetworkId, p, newMeteredMsgWriter(rw, int(version)))
	defer peer.close()
	h.wg.Add(1)
	defer h.wg.Done()
	return h.handle(peer)
}

func (h *serverHandler) handle(p *clientPeer) error {
	p.Log().Debug("Light Ethereum peer connected", "name", p.Name())

	// Execute the LES handshake
	var (
		head   = h.blockchain.CurrentHeader()
		hash   = head.Hash()
		number = head.Number.Uint64()
		td     = h.blockchain.GetTd(hash, number)
		forkID = forkid.NewID(h.blockchain.Config(), h.blockchain.Genesis().Hash(), h.blockchain.CurrentBlock().NumberU64())
	)
	if err := p.Handshake(td, hash, number, h.blockchain.Genesis().Hash(), forkID, h.forkFilter, h.server); err != nil {
		p.Log().Debug("Light Ethereum handshake failed", "err", err)
		return err
	}
	// Connected to another server, no messages expected, just wait for disconnection
	if p.server {
		if err := h.server.serverset.register(p); err != nil {
			return err
		}
		_, err := p.rw.ReadMsg()
		h.server.serverset.unregister(p)
		return err
	}
	// Setup flow control mechanism for the peer
	p.fcClient = flowcontrol.NewClientNode(h.server.fcManager, p.fcParams)
	defer p.fcClient.Disconnect()

	// Reject light clients if server is not synced. Put this checking here, so
	// that "non-synced" les-server peers are still allowed to keep the connection.
	if !h.synced() {
		p.Log().Debug("Light server not synced, rejecting peer")
		return p2p.DiscRequested
	}

	// Register the peer into the peerset and clientpool
	if err := h.server.peers.register(p); err != nil {
		return err
	}
	if p.balance = h.server.clientPool.Register(p); p.balance == nil {
		h.server.peers.unregister(p.ID())
		p.Log().Debug("Client pool already closed")
		return p2p.DiscRequested
	}
	p.connectedAt = mclock.Now()

	var wg sync.WaitGroup // Wait group used to track all in-flight task routines.
	defer func() {
		wg.Wait() // Ensure all background task routines have exited.
		h.server.clientPool.Unregister(p)
		h.server.peers.unregister(p.ID())
		p.balance = nil
		connectionTimer.Update(time.Duration(mclock.Now() - p.connectedAt))
	}()

	// Mark the peer as being served.
	atomic.StoreUint32(&p.serving, 1)
	defer atomic.StoreUint32(&p.serving, 0)

	// Spawn a main loop to handle all incoming messages.
	for {
		select {
		case err := <-p.errCh:
			p.Log().Debug("Failed to send light ethereum response", "err", err)
			return err
		default:
		}
		if err := h.handleMsg(p, &wg); err != nil {
			p.Log().Debug("Light Ethereum message handling failed", "err", err)
			return err
		}
	}
}

// beforeHandle will do a series of prechecks before handling message.
func (h *serverHandler) beforeHandle(p *clientPeer, reqID, responseCount uint64, msg p2p.Msg, reqCnt uint64, maxCount uint64) (*servingTask, uint64) {
	// Ensure that the request sent by client peer is valid
	inSizeCost := h.server.costTracker.realCost(0, msg.Size, 0)
	if reqCnt == 0 || reqCnt > maxCount {
		p.fcClient.OneTimeCost(inSizeCost)
		return nil, 0
	}
	// Ensure that the client peer complies with the flow control
	// rules agreed by both sides.
	if p.isFrozen() {
		p.fcClient.OneTimeCost(inSizeCost)
		return nil, 0
	}
	maxCost := p.fcCosts.getMaxCost(msg.Code, reqCnt)
	accepted, bufShort, priority := p.fcClient.AcceptRequest(reqID, responseCount, maxCost)
	if !accepted {
		p.freeze()
		p.Log().Error("Request came too early", "remaining", common.PrettyDuration(time.Duration(bufShort*1000000/p.fcParams.MinRecharge)))
		p.fcClient.OneTimeCost(inSizeCost)
		return nil, 0
	}
	// Create a multi-stage task, estimate the time it takes for the task to
	// execute, and cache it in the request service queue.
	factor := h.server.costTracker.globalFactor()
	if factor < 0.001 {
		factor = 1
		p.Log().Error("Invalid global cost factor", "factor", factor)
	}
	maxTime := uint64(float64(maxCost) / factor)
	task := h.server.servingQueue.newTask(p, maxTime, priority)
	if !task.start() {
		p.fcClient.RequestProcessed(reqID, responseCount, maxCost, inSizeCost)
		return nil, 0
	}
	return task, maxCost
}

// Afterhandle will perform a series of operations after message handling,
// such as updating flow control data, sending reply, etc.
func (h *serverHandler) afterHandle(p *clientPeer, reqID, responseCount uint64, msg p2p.Msg, maxCost uint64, reqCnt uint64, task *servingTask, reply *reply) {
	if reply != nil {
		task.done()
	}
	p.responseLock.Lock()
	defer p.responseLock.Unlock()

	// Short circuit if the client is already frozen.
	if p.isFrozen() {
		realCost := h.server.costTracker.realCost(task.servingTime, msg.Size, 0)
		p.fcClient.RequestProcessed(reqID, responseCount, maxCost, realCost)
		return
	}
	// Positive correction buffer value with real cost.
	var replySize uint32
	if reply != nil {
		replySize = reply.size()
	}
	var realCost uint64
	if h.server.costTracker.testing {
		realCost = maxCost // Assign a fake cost for testing purpose
	} else {
		realCost = h.server.costTracker.realCost(task.servingTime, msg.Size, replySize)
		if realCost > maxCost {
			realCost = maxCost
		}
	}
	bv := p.fcClient.RequestProcessed(reqID, responseCount, maxCost, realCost)
	if reply != nil {
		// Feed cost tracker request serving statistic.
		h.server.costTracker.updateStats(msg.Code, reqCnt, task.servingTime, realCost)
		// Reduce priority "balance" for the specific peer.
		p.balance.RequestServed(realCost)
		p.queueSend(func() {
			if err := reply.send(bv); err != nil {
				select {
				case p.errCh <- err:
				default:
				}
			}
		})
	}
}

// handleMsg is invoked whenever an inbound message is received from a remote
// peer. The remote connection is torn down upon returning any error.
func (h *serverHandler) handleMsg(p *clientPeer, wg *sync.WaitGroup) error {
	// Read the next message from the remote peer, and ensure it's fully consumed
	msg, err := p.rw.ReadMsg()
	if err != nil {
		return err
	}
	p.Log().Trace("Light Ethereum message arrived", "code", msg.Code, "bytes", msg.Size)

	// Discard large message which exceeds the limitation.
	if msg.Size > ProtocolMaxMsgSize {
		clientErrorMeter.Mark(1)
		return errResp(ErrMsgTooLarge, "%v > %v", msg.Size, ProtocolMaxMsgSize)
	}
	defer msg.Discard()

<<<<<<< HEAD
	var (
		maxCost uint64
		task    *servingTask
	)
	p.responseCount++
	responseCount := p.responseCount
	// accept returns an indicator whether the request can be served.
	// If so, deduct the max cost from the flow control buffer.
	accept := func(reqID, reqCnt, maxCnt uint64) bool {
		// Short circuit if the peer is already frozen or the request is invalid.
		inSizeCost := h.server.costTracker.realCost(0, msg.Size, 0)
		if p.isFrozen() || reqCnt == 0 || reqCnt > maxCnt {
			p.fcClient.OneTimeCost(inSizeCost)
			return false
		}
		// Prepaid max cost units before request been serving.
		maxCost = p.fcCosts.getMaxCost(msg.Code, reqCnt)
		accepted, bufShort, priority := p.fcClient.AcceptRequest(reqID, responseCount, maxCost)
		if !accepted {
			p.freeze()
			p.Log().Error("Request came too early", "remaining", common.PrettyDuration(time.Duration(bufShort*1000000/p.fcParams.MinRecharge)))
			p.fcClient.OneTimeCost(inSizeCost)
			return false
		}
		// Create a multi-stage task, estimate the time it takes for the task to
		// execute, and cache it in the request service queue.
		factor := h.server.costTracker.globalFactor()
		if factor < 0.001 {
			factor = 1
			p.Log().Error("Invalid global cost factor", "factor", factor)
		}
		maxTime := uint64(float64(maxCost) / factor)
		task = h.server.servingQueue.newTask(p, maxTime, priority)
		if task.start() {
			return true
		}
		p.fcClient.RequestProcessed(reqID, responseCount, maxCost, inSizeCost)
		return false
	}
	// sendResponse sends back the response and updates the flow control statistic.
	sendResponse := func(reqID, amount uint64, reply *reply, servingTime uint64) {
		p.responseLock.Lock()
		defer p.responseLock.Unlock()
		// Short circuit if the client is already frozen.
		if p.isFrozen() {
			realCost := h.server.costTracker.realCost(servingTime, msg.Size, 0)
			p.fcClient.RequestProcessed(reqID, responseCount, maxCost, realCost)
			return
		}
		// Positive correction buffer value with real cost.
		var replySize uint32
		if reply != nil {
			replySize = reply.size()
		}
		var realCost uint64
		if h.server.costTracker.testing {
			realCost = maxCost // Assign a fake cost for testing purpose
		} else {
			realCost = h.server.costTracker.realCost(servingTime, msg.Size, replySize)
			if realCost > maxCost {
				realCost = maxCost
			}
		}
		bv := p.fcClient.RequestProcessed(reqID, responseCount, maxCost, realCost)
		if amount != 0 {
			// Feed cost tracker request serving statistic.
			h.server.costTracker.updateStats(msg.Code, amount, servingTime, realCost)
			// Reduce priority "balance" for the specific peer.
			p.balance.RequestServed(realCost)
		}
		if reply != nil {
			p.queueSend(func() {
				if err := reply.send(bv); err != nil {
					select {
					case p.errCh <- err:
					default:
					}
				}
			})
		}
	}
	switch msg.Code {
	case GetBlockHeadersMsg:
		p.Log().Trace("Received block header request")
		if metrics.EnabledExpensive {
			miscInHeaderPacketsMeter.Mark(1)
			miscInHeaderTrafficMeter.Mark(int64(msg.Size))
		}
		var req struct {
			ReqID uint64
			Query getBlockHeadersData
		}
		if err := msg.Decode(&req); err != nil {
			clientErrorMeter.Mark(1)
			return errResp(ErrDecode, "%v: %v", msg, err)
		}
		query := req.Query
		if accept(req.ReqID, query.Amount, MaxHeaderFetch) {
			wg.Add(1)
			go func() {
				defer wg.Done()
				hashMode := query.Origin.Hash != (common.Hash{})
				first := true
				maxNonCanonical := uint64(100)

				// Gather headers until the fetch or network limits is reached
				var (
					bytes   common.StorageSize
					headers []*types.Header
					unknown bool
				)
				for !unknown && len(headers) < int(query.Amount) && bytes < softResponseLimit {
					if !first && !task.waitOrStop() {
						sendResponse(req.ReqID, 0, nil, task.servingTime)
						return
					}
					// Retrieve the next header satisfying the query
					var origin *types.Header
					if hashMode {
						if first {
							origin = h.blockchain.GetHeaderByHash(query.Origin.Hash)
							if origin != nil {
								query.Origin.Number = origin.Number.Uint64()
							}
						} else {
							origin = h.blockchain.GetHeader(query.Origin.Hash, query.Origin.Number)
						}
					} else {
						origin = h.blockchain.GetHeaderByNumber(query.Origin.Number)
					}
					if origin == nil {
						break
					}
					headers = append(headers, origin)
					bytes += estHeaderRlpSize

					// Advance to the next header of the query
					switch {
					case hashMode && query.Reverse:
						// Hash based traversal towards the genesis block
						ancestor := query.Skip + 1
						if ancestor == 0 {
							unknown = true
						} else {
							query.Origin.Hash, query.Origin.Number = h.blockchain.GetAncestor(query.Origin.Hash, query.Origin.Number, ancestor, &maxNonCanonical)
							unknown = query.Origin.Hash == common.Hash{}
						}
					case hashMode && !query.Reverse:
						// Hash based traversal towards the leaf block
						var (
							current = origin.Number.Uint64()
							next    = current + query.Skip + 1
						)
						if next <= current {
							infos, _ := json.MarshalIndent(p.Peer.Info(), "", "  ")
							p.Log().Warn("GetBlockHeaders skip overflow attack", "current", current, "skip", query.Skip, "next", next, "attacker", infos)
							unknown = true
						} else {
							if header := h.blockchain.GetHeaderByNumber(next); header != nil {
								nextHash := header.Hash()
								expOldHash, _ := h.blockchain.GetAncestor(nextHash, next, query.Skip+1, &maxNonCanonical)
								if expOldHash == query.Origin.Hash {
									query.Origin.Hash, query.Origin.Number = nextHash, next
								} else {
									unknown = true
								}
							} else {
								unknown = true
							}
						}
					case query.Reverse:
						// Number based traversal towards the genesis block
						if query.Origin.Number >= query.Skip+1 {
							query.Origin.Number -= query.Skip + 1
						} else {
							unknown = true
						}

					case !query.Reverse:
						// Number based traversal towards the leaf block
						query.Origin.Number += query.Skip + 1
					}
					first = false
				}
				reply := p.replyBlockHeaders(req.ReqID, headers)
				sendResponse(req.ReqID, query.Amount, reply, task.done())
				if metrics.EnabledExpensive {
					miscOutHeaderPacketsMeter.Mark(1)
					miscOutHeaderTrafficMeter.Mark(int64(reply.size()))
					miscServingTimeHeaderTimer.Update(time.Duration(task.servingTime))
				}
			}()
		}

	case GetBlockBodiesMsg:
		p.Log().Trace("Received block bodies request")
		if metrics.EnabledExpensive {
			miscInBodyPacketsMeter.Mark(1)
			miscInBodyTrafficMeter.Mark(int64(msg.Size))
		}
		var req struct {
			ReqID  uint64
			Hashes []common.Hash
		}
		if err := msg.Decode(&req); err != nil {
			clientErrorMeter.Mark(1)
			return errResp(ErrDecode, "msg %v: %v", msg, err)
		}
		var (
			bytes  int
			bodies []rlp.RawValue
		)
		reqCnt := len(req.Hashes)
		if accept(req.ReqID, uint64(reqCnt), MaxBodyFetch) {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for i, hash := range req.Hashes {
					if i != 0 && !task.waitOrStop() {
						sendResponse(req.ReqID, 0, nil, task.servingTime)
						return
					}
					if bytes >= softResponseLimit {
						break
					}
					body := h.blockchain.GetBodyRLP(hash)
					if body == nil {
						p.bumpInvalid()
						continue
					}
					bodies = append(bodies, body)
					bytes += len(body)
				}
				reply := p.replyBlockBodiesRLP(req.ReqID, bodies)
				sendResponse(req.ReqID, uint64(reqCnt), reply, task.done())
				if metrics.EnabledExpensive {
					miscOutBodyPacketsMeter.Mark(1)
					miscOutBodyTrafficMeter.Mark(int64(reply.size()))
					miscServingTimeBodyTimer.Update(time.Duration(task.servingTime))
				}
			}()
		}

	case GetCodeMsg:
		p.Log().Trace("Received code request")
		if metrics.EnabledExpensive {
			miscInCodePacketsMeter.Mark(1)
			miscInCodeTrafficMeter.Mark(int64(msg.Size))
		}
		var req struct {
			ReqID uint64
			Reqs  []CodeReq
		}
		if err := msg.Decode(&req); err != nil {
			clientErrorMeter.Mark(1)
			return errResp(ErrDecode, "msg %v: %v", msg, err)
		}
		var (
			bytes int
			data  [][]byte
		)
		reqCnt := len(req.Reqs)
		if accept(req.ReqID, uint64(reqCnt), MaxCodeFetch) {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for i, request := range req.Reqs {
					if i != 0 && !task.waitOrStop() {
						sendResponse(req.ReqID, 0, nil, task.servingTime)
						return
					}
					// Look up the root hash belonging to the request
					header := h.blockchain.GetHeaderByHash(request.BHash)
					if header == nil {
						p.Log().Warn("Failed to retrieve associate header for code", "hash", request.BHash)
						p.bumpInvalid()
						continue
					}
					// Refuse to search stale state data in the database since looking for
					// a non-exist key is kind of expensive.
					local := h.blockchain.CurrentHeader().Number.Uint64()
					if !h.server.archiveMode && header.Number.Uint64()+core.TriesInMemory <= local {
						p.Log().Debug("Reject stale code request", "number", header.Number.Uint64(), "head", local)
						p.bumpInvalid()
						continue
					}
					triedb := h.blockchain.StateCache().TrieDB()

					account, err := h.getAccount(triedb, header.Root, common.BytesToHash(request.AccKey))
					if err != nil {
						p.Log().Warn("Failed to retrieve account for code", "block", header.Number, "hash", header.Hash(), "account", common.BytesToHash(request.AccKey), "err", err)
						p.bumpInvalid()
						continue
					}
					code, err := h.blockchain.StateCache().ContractCode(common.BytesToHash(request.AccKey), common.BytesToHash(account.CodeHash))
					if err != nil {
						p.Log().Warn("Failed to retrieve account code", "block", header.Number, "hash", header.Hash(), "account", common.BytesToHash(request.AccKey), "codehash", common.BytesToHash(account.CodeHash), "err", err)
						continue
					}
					// Accumulate the code and abort if enough data was retrieved
					data = append(data, code)
					if bytes += len(code); bytes >= softResponseLimit {
						break
					}
				}
				reply := p.replyCode(req.ReqID, data)
				sendResponse(req.ReqID, uint64(reqCnt), reply, task.done())
				if metrics.EnabledExpensive {
					miscOutCodePacketsMeter.Mark(1)
					miscOutCodeTrafficMeter.Mark(int64(reply.size()))
					miscServingTimeCodeTimer.Update(time.Duration(task.servingTime))
				}
			}()
		}

	case GetReceiptsMsg:
		p.Log().Trace("Received receipts request")
		if metrics.EnabledExpensive {
			miscInReceiptPacketsMeter.Mark(1)
			miscInReceiptTrafficMeter.Mark(int64(msg.Size))
		}
		var req struct {
			ReqID  uint64
			Hashes []common.Hash
		}
		if err := msg.Decode(&req); err != nil {
			clientErrorMeter.Mark(1)
			return errResp(ErrDecode, "msg %v: %v", msg, err)
		}
		var (
			bytes    int
			receipts []rlp.RawValue
		)
		reqCnt := len(req.Hashes)
		if accept(req.ReqID, uint64(reqCnt), MaxReceiptFetch) {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for i, hash := range req.Hashes {
					if i != 0 && !task.waitOrStop() {
						sendResponse(req.ReqID, 0, nil, task.servingTime)
						return
					}
					if bytes >= softResponseLimit {
						break
					}
					// Retrieve the requested block's receipts, skipping if unknown to us
					results := h.blockchain.GetReceiptsByHash(hash)
					if results == nil {
						if header := h.blockchain.GetHeaderByHash(hash); header == nil || header.ReceiptHash != types.EmptyRootHash {
							p.bumpInvalid()
							continue
						}
					}
					// If known, encode and queue for response packet
					if encoded, err := rlp.EncodeToBytes(results); err != nil {
						log.Error("Failed to encode receipt", "err", err)
					} else {
						receipts = append(receipts, encoded)
						bytes += len(encoded)
					}
				}
				reply := p.replyReceiptsRLP(req.ReqID, receipts)
				sendResponse(req.ReqID, uint64(reqCnt), reply, task.done())
				if metrics.EnabledExpensive {
					miscOutReceiptPacketsMeter.Mark(1)
					miscOutReceiptTrafficMeter.Mark(int64(reply.size()))
					miscServingTimeReceiptTimer.Update(time.Duration(task.servingTime))
				}
			}()
		}

	case GetProofsV2Msg:
		p.Log().Trace("Received les/2 proofs request")
		if metrics.EnabledExpensive {
			miscInTrieProofPacketsMeter.Mark(1)
			miscInTrieProofTrafficMeter.Mark(int64(msg.Size))
		}
		var req struct {
			ReqID uint64
			Reqs  []ProofReq
		}
		if err := msg.Decode(&req); err != nil {
			clientErrorMeter.Mark(1)
			return errResp(ErrDecode, "msg %v: %v", msg, err)
		}
		// Gather state data until the fetch or network limits is reached
		var (
			lastBHash common.Hash
			root      common.Hash
			header    *types.Header
		)
		reqCnt := len(req.Reqs)
		if accept(req.ReqID, uint64(reqCnt), MaxProofsFetch) {
			wg.Add(1)
			go func() {
				defer wg.Done()
				nodes := light.NewNodeSet()

				for i, request := range req.Reqs {
					if i != 0 && !task.waitOrStop() {
						sendResponse(req.ReqID, 0, nil, task.servingTime)
						return
					}
					// Look up the root hash belonging to the request
					if request.BHash != lastBHash {
						root, lastBHash = common.Hash{}, request.BHash

						if header = h.blockchain.GetHeaderByHash(request.BHash); header == nil {
							p.Log().Warn("Failed to retrieve header for proof", "hash", request.BHash)
							p.bumpInvalid()
							continue
						}
						// Refuse to search stale state data in the database since looking for
						// a non-exist key is kind of expensive.
						local := h.blockchain.CurrentHeader().Number.Uint64()
						if !h.server.archiveMode && header.Number.Uint64()+core.TriesInMemory <= local {
							p.Log().Debug("Reject stale trie request", "number", header.Number.Uint64(), "head", local)
							p.bumpInvalid()
							continue
						}
						root = header.Root
					}
					// If a header lookup failed (non existent), ignore subsequent requests for the same header
					if root == (common.Hash{}) {
						p.bumpInvalid()
						continue
					}
					// Open the account or storage trie for the request
					statedb := h.blockchain.StateCache()

					var trie state.Trie
					switch len(request.AccKey) {
					case 0:
						// No account key specified, open an account trie
						trie, err = statedb.OpenTrie(root)
						if trie == nil || err != nil {
							p.Log().Warn("Failed to open storage trie for proof", "block", header.Number, "hash", header.Hash(), "root", root, "err", err)
							continue
						}
					default:
						// Account key specified, open a storage trie
						account, err := h.getAccount(statedb.TrieDB(), root, common.BytesToHash(request.AccKey))
						if err != nil {
							p.Log().Warn("Failed to retrieve account for proof", "block", header.Number, "hash", header.Hash(), "account", common.BytesToHash(request.AccKey), "err", err)
							p.bumpInvalid()
							continue
						}
						trie, err = statedb.OpenStorageTrie(common.BytesToHash(request.AccKey), account.Root)
						if trie == nil || err != nil {
							p.Log().Warn("Failed to open storage trie for proof", "block", header.Number, "hash", header.Hash(), "account", common.BytesToHash(request.AccKey), "root", account.Root, "err", err)
							continue
						}
					}
					// Prove the user's request from the account or stroage trie
					if err := trie.Prove(request.Key, request.FromLevel, nodes); err != nil {
						p.Log().Warn("Failed to prove state request", "block", header.Number, "hash", header.Hash(), "err", err)
						continue
					}
					if nodes.DataSize() >= softResponseLimit {
						break
					}
				}
				reply := p.replyProofsV2(req.ReqID, nodes.NodeList())
				sendResponse(req.ReqID, uint64(reqCnt), reply, task.done())
				if metrics.EnabledExpensive {
					miscOutTrieProofPacketsMeter.Mark(1)
					miscOutTrieProofTrafficMeter.Mark(int64(reply.size()))
					miscServingTimeTrieProofTimer.Update(time.Duration(task.servingTime))
				}
			}()
		}

	case GetHelperTrieProofsMsg:
		p.Log().Trace("Received helper trie proof request")
		if metrics.EnabledExpensive {
			miscInHelperTriePacketsMeter.Mark(1)
			miscInHelperTrieTrafficMeter.Mark(int64(msg.Size))
		}
		var req struct {
			ReqID uint64
			Reqs  []HelperTrieReq
		}
		if err := msg.Decode(&req); err != nil {
			clientErrorMeter.Mark(1)
			return errResp(ErrDecode, "msg %v: %v", msg, err)
		}
		// Gather state data until the fetch or network limits is reached
		var (
			auxBytes int
			auxData  [][]byte
		)
		reqCnt := len(req.Reqs)
		if accept(req.ReqID, uint64(reqCnt), MaxHelperTrieProofsFetch) {
			wg.Add(1)
			go func() {
				defer wg.Done()
				var (
					lastIdx  uint64
					lastType uint
					root     common.Hash
					auxTrie  *trie.Trie
				)
				nodes := light.NewNodeSet()
				for i, request := range req.Reqs {
					if i != 0 && !task.waitOrStop() {
						sendResponse(req.ReqID, 0, nil, task.servingTime)
						return
					}
					if auxTrie == nil || request.Type != lastType || request.TrieIdx != lastIdx {
						auxTrie, lastType, lastIdx = nil, request.Type, request.TrieIdx

						var prefix string
						if root, prefix = h.getHelperTrie(request.Type, request.TrieIdx); root != (common.Hash{}) {
							auxTrie, _ = trie.New(root, trie.NewDatabase(rawdb.NewTable(h.chainDb, prefix)))
						}
					}
					if request.AuxReq == auxRoot {
						var data []byte
						if root != (common.Hash{}) {
							data = root[:]
						}
						auxData = append(auxData, data)
						auxBytes += len(data)
					} else {
						if auxTrie != nil {
							auxTrie.Prove(request.Key, request.FromLevel, nodes)
						}
						if request.AuxReq != 0 {
							data := h.getAuxiliaryHeaders(request)
							auxData = append(auxData, data)
							auxBytes += len(data)
						}
					}
					if nodes.DataSize()+auxBytes >= softResponseLimit {
						break
					}
				}
				reply := p.replyHelperTrieProofs(req.ReqID, HelperTrieResps{Proofs: nodes.NodeList(), AuxData: auxData})
				sendResponse(req.ReqID, uint64(reqCnt), reply, task.done())
				if metrics.EnabledExpensive {
					miscOutHelperTriePacketsMeter.Mark(1)
					miscOutHelperTrieTrafficMeter.Mark(int64(reply.size()))
					miscServingTimeHelperTrieTimer.Update(time.Duration(task.servingTime))
				}
			}()
		}

	case SendTxV2Msg:
		p.Log().Trace("Received new transactions")
		if metrics.EnabledExpensive {
			miscInTxsPacketsMeter.Mark(1)
			miscInTxsTrafficMeter.Mark(int64(msg.Size))
		}
		var req struct {
			ReqID uint64
			Txs   []*types.Transaction
		}
		if err := msg.Decode(&req); err != nil {
			clientErrorMeter.Mark(1)
			return errResp(ErrDecode, "msg %v: %v", msg, err)
		}
		reqCnt := len(req.Txs)
		if accept(req.ReqID, uint64(reqCnt), MaxTxSend) {
			wg.Add(1)
			go func() {
				defer wg.Done()

				stats := make([]light.TxStatus, len(req.Txs))
				for i, tx := range req.Txs {
					if i != 0 && !task.waitOrStop() {
						return
					}
					hash := tx.Hash()
					stats[i] = h.txStatus(hash)
					if stats[i].Status == core.TxStatusUnknown {
						// Only include transactions that have a valid gateway fee recipient & fee
						if err := h.verifyGatewayFee(tx.GatewayFeeRecipient(), tx.GatewayFee()); err != nil {
							p.Log().Trace("Rejected transaction from light peer for invalid gateway fee", "hash", hash.String(), "err", err)
							stats[i].Error = err.Error()
							continue
						}

						addFn := h.txpool.AddRemotes
						// Add transactions synchronously for testing purpose
						if h.addTxsSync {
							addFn = h.txpool.AddRemotesSync
						}
						if errs := addFn([]*types.Transaction{tx}); errs[0] != nil {
							stats[i].Error = errs[0].Error()
							continue
						}
						stats[i] = h.txStatus(hash)
						p.Log().Trace("Added transaction from light peer to pool", "hash", hash.String(), "tx", tx)
					}
				}
				reply := p.replyTxStatus(req.ReqID, stats)
				sendResponse(req.ReqID, uint64(reqCnt), reply, task.done())
				if metrics.EnabledExpensive {
					miscOutTxsPacketsMeter.Mark(1)
					miscOutTxsTrafficMeter.Mark(int64(reply.size()))
					miscServingTimeTxTimer.Update(time.Duration(task.servingTime))
				}
			}()
		}

	case GetTxStatusMsg:
		p.Log().Trace("Received transaction status query request")
		if metrics.EnabledExpensive {
			miscInTxStatusPacketsMeter.Mark(1)
			miscInTxStatusTrafficMeter.Mark(int64(msg.Size))
		}
		var req struct {
			ReqID  uint64
			Hashes []common.Hash
		}
		if err := msg.Decode(&req); err != nil {
			clientErrorMeter.Mark(1)
			return errResp(ErrDecode, "msg %v: %v", msg, err)
		}
		reqCnt := len(req.Hashes)
		if accept(req.ReqID, uint64(reqCnt), MaxTxStatus) {
			wg.Add(1)
			go func() {
				defer wg.Done()
				stats := make([]light.TxStatus, len(req.Hashes))
				for i, hash := range req.Hashes {
					if i != 0 && !task.waitOrStop() {
						sendResponse(req.ReqID, 0, nil, task.servingTime)
						return
					}
					stats[i] = h.txStatus(hash)
				}
				reply := p.replyTxStatus(req.ReqID, stats)
				sendResponse(req.ReqID, uint64(reqCnt), reply, task.done())
				if metrics.EnabledExpensive {
					miscOutTxStatusPacketsMeter.Mark(1)
					miscOutTxStatusTrafficMeter.Mark(int64(reply.size()))
					miscServingTimeTxStatusTimer.Update(time.Duration(task.servingTime))
				}
			}()
		}

	case GetEtherbaseMsg:
		// Celo: Handle Etherbase Request
		p.Log().Trace("Received etherbase request")
		if metrics.EnabledExpensive {
			miscInEtherbasePacketsMeter.Mark(1)
			miscInEtherbaseTrafficMeter.Mark(int64(msg.Size))
			defer func(start time.Time) { miscServingTimeEtherbaseTimer.UpdateSince(start) }(time.Now())
		}

		var req struct {
			ReqID uint64
		}
		if err := msg.Decode(&req); err != nil {
			return errResp(ErrDecode, "msg %v: %v", msg, err)
		}

		if accept(req.ReqID, 1, MaxEtherbase) {
			wg.Add(1)
			go func() {
				defer wg.Done()
				reply := p.SendEtherbaseRLP(req.ReqID, h.etherbase)
				sendResponse(req.ReqID, 1, reply, task.done())
				if metrics.EnabledExpensive {
					miscOutEtherbasePacketsMeter.Mark(1)
					miscOutEtherbaseTrafficMeter.Mark(int64(reply.size()))
				}
			}()
		}
	case GetGatewayFeeMsg:
		p.Log().Trace("Received gatewayFee request")
		var req struct {
			ReqID uint64
		}
		if err := msg.Decode(&req); err != nil {
			return errResp(ErrDecode, "msg %v: %v", msg, err)
		}

		if accept(req.ReqID, 1, MaxGatewayFee) {
			wg.Add(1)
			go func() {
				defer wg.Done()
				reply := p.ReplyGatewayFee(req.ReqID, GatewayFeeInformation{GatewayFee: h.gatewayFee, Etherbase: h.etherbase})
				sendResponse(req.ReqID, 1, reply, task.done())
			}()
		}

	default:
||||||| e78727290
	var (
		maxCost uint64
		task    *servingTask
	)
	p.responseCount++
	responseCount := p.responseCount
	// accept returns an indicator whether the request can be served.
	// If so, deduct the max cost from the flow control buffer.
	accept := func(reqID, reqCnt, maxCnt uint64) bool {
		// Short circuit if the peer is already frozen or the request is invalid.
		inSizeCost := h.server.costTracker.realCost(0, msg.Size, 0)
		if p.isFrozen() || reqCnt == 0 || reqCnt > maxCnt {
			p.fcClient.OneTimeCost(inSizeCost)
			return false
		}
		// Prepaid max cost units before request been serving.
		maxCost = p.fcCosts.getMaxCost(msg.Code, reqCnt)
		accepted, bufShort, priority := p.fcClient.AcceptRequest(reqID, responseCount, maxCost)
		if !accepted {
			p.freeze()
			p.Log().Error("Request came too early", "remaining", common.PrettyDuration(time.Duration(bufShort*1000000/p.fcParams.MinRecharge)))
			p.fcClient.OneTimeCost(inSizeCost)
			return false
		}
		// Create a multi-stage task, estimate the time it takes for the task to
		// execute, and cache it in the request service queue.
		factor := h.server.costTracker.globalFactor()
		if factor < 0.001 {
			factor = 1
			p.Log().Error("Invalid global cost factor", "factor", factor)
		}
		maxTime := uint64(float64(maxCost) / factor)
		task = h.server.servingQueue.newTask(p, maxTime, priority)
		if task.start() {
			return true
		}
		p.fcClient.RequestProcessed(reqID, responseCount, maxCost, inSizeCost)
		return false
	}
	// sendResponse sends back the response and updates the flow control statistic.
	sendResponse := func(reqID, amount uint64, reply *reply, servingTime uint64) {
		p.responseLock.Lock()
		defer p.responseLock.Unlock()

		// Short circuit if the client is already frozen.
		if p.isFrozen() {
			realCost := h.server.costTracker.realCost(servingTime, msg.Size, 0)
			p.fcClient.RequestProcessed(reqID, responseCount, maxCost, realCost)
			return
		}
		// Positive correction buffer value with real cost.
		var replySize uint32
		if reply != nil {
			replySize = reply.size()
		}
		var realCost uint64
		if h.server.costTracker.testing {
			realCost = maxCost // Assign a fake cost for testing purpose
		} else {
			realCost = h.server.costTracker.realCost(servingTime, msg.Size, replySize)
			if realCost > maxCost {
				realCost = maxCost
			}
		}
		bv := p.fcClient.RequestProcessed(reqID, responseCount, maxCost, realCost)
		if amount != 0 {
			// Feed cost tracker request serving statistic.
			h.server.costTracker.updateStats(msg.Code, amount, servingTime, realCost)
			// Reduce priority "balance" for the specific peer.
			p.balance.RequestServed(realCost)
		}
		if reply != nil {
			p.queueSend(func() {
				if err := reply.send(bv); err != nil {
					select {
					case p.errCh <- err:
					default:
					}
				}
			})
		}
	}
	switch msg.Code {
	case GetBlockHeadersMsg:
		p.Log().Trace("Received block header request")
		if metrics.EnabledExpensive {
			miscInHeaderPacketsMeter.Mark(1)
			miscInHeaderTrafficMeter.Mark(int64(msg.Size))
		}
		var req struct {
			ReqID uint64
			Query getBlockHeadersData
		}
		if err := msg.Decode(&req); err != nil {
			clientErrorMeter.Mark(1)
			return errResp(ErrDecode, "%v: %v", msg, err)
		}
		query := req.Query
		if accept(req.ReqID, query.Amount, MaxHeaderFetch) {
			wg.Add(1)
			go func() {
				defer wg.Done()
				hashMode := query.Origin.Hash != (common.Hash{})
				first := true
				maxNonCanonical := uint64(100)

				// Gather headers until the fetch or network limits is reached
				var (
					bytes   common.StorageSize
					headers []*types.Header
					unknown bool
				)
				for !unknown && len(headers) < int(query.Amount) && bytes < softResponseLimit {
					if !first && !task.waitOrStop() {
						sendResponse(req.ReqID, 0, nil, task.servingTime)
						return
					}
					// Retrieve the next header satisfying the query
					var origin *types.Header
					if hashMode {
						if first {
							origin = h.blockchain.GetHeaderByHash(query.Origin.Hash)
							if origin != nil {
								query.Origin.Number = origin.Number.Uint64()
							}
						} else {
							origin = h.blockchain.GetHeader(query.Origin.Hash, query.Origin.Number)
						}
					} else {
						origin = h.blockchain.GetHeaderByNumber(query.Origin.Number)
					}
					if origin == nil {
						break
					}
					headers = append(headers, origin)
					bytes += estHeaderRlpSize

					// Advance to the next header of the query
					switch {
					case hashMode && query.Reverse:
						// Hash based traversal towards the genesis block
						ancestor := query.Skip + 1
						if ancestor == 0 {
							unknown = true
						} else {
							query.Origin.Hash, query.Origin.Number = h.blockchain.GetAncestor(query.Origin.Hash, query.Origin.Number, ancestor, &maxNonCanonical)
							unknown = query.Origin.Hash == common.Hash{}
						}
					case hashMode && !query.Reverse:
						// Hash based traversal towards the leaf block
						var (
							current = origin.Number.Uint64()
							next    = current + query.Skip + 1
						)
						if next <= current {
							infos, _ := json.MarshalIndent(p.Peer.Info(), "", "  ")
							p.Log().Warn("GetBlockHeaders skip overflow attack", "current", current, "skip", query.Skip, "next", next, "attacker", infos)
							unknown = true
						} else {
							if header := h.blockchain.GetHeaderByNumber(next); header != nil {
								nextHash := header.Hash()
								expOldHash, _ := h.blockchain.GetAncestor(nextHash, next, query.Skip+1, &maxNonCanonical)
								if expOldHash == query.Origin.Hash {
									query.Origin.Hash, query.Origin.Number = nextHash, next
								} else {
									unknown = true
								}
							} else {
								unknown = true
							}
						}
					case query.Reverse:
						// Number based traversal towards the genesis block
						if query.Origin.Number >= query.Skip+1 {
							query.Origin.Number -= query.Skip + 1
						} else {
							unknown = true
						}

					case !query.Reverse:
						// Number based traversal towards the leaf block
						query.Origin.Number += query.Skip + 1
					}
					first = false
				}
				reply := p.replyBlockHeaders(req.ReqID, headers)
				sendResponse(req.ReqID, query.Amount, reply, task.done())
				if metrics.EnabledExpensive {
					miscOutHeaderPacketsMeter.Mark(1)
					miscOutHeaderTrafficMeter.Mark(int64(reply.size()))
					miscServingTimeHeaderTimer.Update(time.Duration(task.servingTime))
				}
			}()
		}

	case GetBlockBodiesMsg:
		p.Log().Trace("Received block bodies request")
		if metrics.EnabledExpensive {
			miscInBodyPacketsMeter.Mark(1)
			miscInBodyTrafficMeter.Mark(int64(msg.Size))
		}
		var req struct {
			ReqID  uint64
			Hashes []common.Hash
		}
		if err := msg.Decode(&req); err != nil {
			clientErrorMeter.Mark(1)
			return errResp(ErrDecode, "msg %v: %v", msg, err)
		}
		var (
			bytes  int
			bodies []rlp.RawValue
		)
		reqCnt := len(req.Hashes)
		if accept(req.ReqID, uint64(reqCnt), MaxBodyFetch) {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for i, hash := range req.Hashes {
					if i != 0 && !task.waitOrStop() {
						sendResponse(req.ReqID, 0, nil, task.servingTime)
						return
					}
					if bytes >= softResponseLimit {
						break
					}
					body := h.blockchain.GetBodyRLP(hash)
					if body == nil {
						p.bumpInvalid()
						continue
					}
					bodies = append(bodies, body)
					bytes += len(body)
				}
				reply := p.replyBlockBodiesRLP(req.ReqID, bodies)
				sendResponse(req.ReqID, uint64(reqCnt), reply, task.done())
				if metrics.EnabledExpensive {
					miscOutBodyPacketsMeter.Mark(1)
					miscOutBodyTrafficMeter.Mark(int64(reply.size()))
					miscServingTimeBodyTimer.Update(time.Duration(task.servingTime))
				}
			}()
		}

	case GetCodeMsg:
		p.Log().Trace("Received code request")
		if metrics.EnabledExpensive {
			miscInCodePacketsMeter.Mark(1)
			miscInCodeTrafficMeter.Mark(int64(msg.Size))
		}
		var req struct {
			ReqID uint64
			Reqs  []CodeReq
		}
		if err := msg.Decode(&req); err != nil {
			clientErrorMeter.Mark(1)
			return errResp(ErrDecode, "msg %v: %v", msg, err)
		}
		var (
			bytes int
			data  [][]byte
		)
		reqCnt := len(req.Reqs)
		if accept(req.ReqID, uint64(reqCnt), MaxCodeFetch) {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for i, request := range req.Reqs {
					if i != 0 && !task.waitOrStop() {
						sendResponse(req.ReqID, 0, nil, task.servingTime)
						return
					}
					// Look up the root hash belonging to the request
					header := h.blockchain.GetHeaderByHash(request.BHash)
					if header == nil {
						p.Log().Warn("Failed to retrieve associate header for code", "hash", request.BHash)
						p.bumpInvalid()
						continue
					}
					// Refuse to search stale state data in the database since looking for
					// a non-exist key is kind of expensive.
					local := h.blockchain.CurrentHeader().Number.Uint64()
					if !h.server.archiveMode && header.Number.Uint64()+core.TriesInMemory <= local {
						p.Log().Debug("Reject stale code request", "number", header.Number.Uint64(), "head", local)
						p.bumpInvalid()
						continue
					}
					triedb := h.blockchain.StateCache().TrieDB()

					account, err := h.getAccount(triedb, header.Root, common.BytesToHash(request.AccKey))
					if err != nil {
						p.Log().Warn("Failed to retrieve account for code", "block", header.Number, "hash", header.Hash(), "account", common.BytesToHash(request.AccKey), "err", err)
						p.bumpInvalid()
						continue
					}
					code, err := h.blockchain.StateCache().ContractCode(common.BytesToHash(request.AccKey), common.BytesToHash(account.CodeHash))
					if err != nil {
						p.Log().Warn("Failed to retrieve account code", "block", header.Number, "hash", header.Hash(), "account", common.BytesToHash(request.AccKey), "codehash", common.BytesToHash(account.CodeHash), "err", err)
						continue
					}
					// Accumulate the code and abort if enough data was retrieved
					data = append(data, code)
					if bytes += len(code); bytes >= softResponseLimit {
						break
					}
				}
				reply := p.replyCode(req.ReqID, data)
				sendResponse(req.ReqID, uint64(reqCnt), reply, task.done())
				if metrics.EnabledExpensive {
					miscOutCodePacketsMeter.Mark(1)
					miscOutCodeTrafficMeter.Mark(int64(reply.size()))
					miscServingTimeCodeTimer.Update(time.Duration(task.servingTime))
				}
			}()
		}

	case GetReceiptsMsg:
		p.Log().Trace("Received receipts request")
		if metrics.EnabledExpensive {
			miscInReceiptPacketsMeter.Mark(1)
			miscInReceiptTrafficMeter.Mark(int64(msg.Size))
		}
		var req struct {
			ReqID  uint64
			Hashes []common.Hash
		}
		if err := msg.Decode(&req); err != nil {
			clientErrorMeter.Mark(1)
			return errResp(ErrDecode, "msg %v: %v", msg, err)
		}
		var (
			bytes    int
			receipts []rlp.RawValue
		)
		reqCnt := len(req.Hashes)
		if accept(req.ReqID, uint64(reqCnt), MaxReceiptFetch) {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for i, hash := range req.Hashes {
					if i != 0 && !task.waitOrStop() {
						sendResponse(req.ReqID, 0, nil, task.servingTime)
						return
					}
					if bytes >= softResponseLimit {
						break
					}
					// Retrieve the requested block's receipts, skipping if unknown to us
					results := h.blockchain.GetReceiptsByHash(hash)
					if results == nil {
						if header := h.blockchain.GetHeaderByHash(hash); header == nil || header.ReceiptHash != types.EmptyRootHash {
							p.bumpInvalid()
							continue
						}
					}
					// If known, encode and queue for response packet
					if encoded, err := rlp.EncodeToBytes(results); err != nil {
						log.Error("Failed to encode receipt", "err", err)
					} else {
						receipts = append(receipts, encoded)
						bytes += len(encoded)
					}
				}
				reply := p.replyReceiptsRLP(req.ReqID, receipts)
				sendResponse(req.ReqID, uint64(reqCnt), reply, task.done())
				if metrics.EnabledExpensive {
					miscOutReceiptPacketsMeter.Mark(1)
					miscOutReceiptTrafficMeter.Mark(int64(reply.size()))
					miscServingTimeReceiptTimer.Update(time.Duration(task.servingTime))
				}
			}()
		}

	case GetProofsV2Msg:
		p.Log().Trace("Received les/2 proofs request")
		if metrics.EnabledExpensive {
			miscInTrieProofPacketsMeter.Mark(1)
			miscInTrieProofTrafficMeter.Mark(int64(msg.Size))
		}
		var req struct {
			ReqID uint64
			Reqs  []ProofReq
		}
		if err := msg.Decode(&req); err != nil {
			clientErrorMeter.Mark(1)
			return errResp(ErrDecode, "msg %v: %v", msg, err)
		}
		// Gather state data until the fetch or network limits is reached
		var (
			lastBHash common.Hash
			root      common.Hash
			header    *types.Header
		)
		reqCnt := len(req.Reqs)
		if accept(req.ReqID, uint64(reqCnt), MaxProofsFetch) {
			wg.Add(1)
			go func() {
				defer wg.Done()
				nodes := light.NewNodeSet()

				for i, request := range req.Reqs {
					if i != 0 && !task.waitOrStop() {
						sendResponse(req.ReqID, 0, nil, task.servingTime)
						return
					}
					// Look up the root hash belonging to the request
					if request.BHash != lastBHash {
						root, lastBHash = common.Hash{}, request.BHash

						if header = h.blockchain.GetHeaderByHash(request.BHash); header == nil {
							p.Log().Warn("Failed to retrieve header for proof", "hash", request.BHash)
							p.bumpInvalid()
							continue
						}
						// Refuse to search stale state data in the database since looking for
						// a non-exist key is kind of expensive.
						local := h.blockchain.CurrentHeader().Number.Uint64()
						if !h.server.archiveMode && header.Number.Uint64()+core.TriesInMemory <= local {
							p.Log().Debug("Reject stale trie request", "number", header.Number.Uint64(), "head", local)
							p.bumpInvalid()
							continue
						}
						root = header.Root
					}
					// If a header lookup failed (non existent), ignore subsequent requests for the same header
					if root == (common.Hash{}) {
						p.bumpInvalid()
						continue
					}
					// Open the account or storage trie for the request
					statedb := h.blockchain.StateCache()

					var trie state.Trie
					switch len(request.AccKey) {
					case 0:
						// No account key specified, open an account trie
						trie, err = statedb.OpenTrie(root)
						if trie == nil || err != nil {
							p.Log().Warn("Failed to open storage trie for proof", "block", header.Number, "hash", header.Hash(), "root", root, "err", err)
							continue
						}
					default:
						// Account key specified, open a storage trie
						account, err := h.getAccount(statedb.TrieDB(), root, common.BytesToHash(request.AccKey))
						if err != nil {
							p.Log().Warn("Failed to retrieve account for proof", "block", header.Number, "hash", header.Hash(), "account", common.BytesToHash(request.AccKey), "err", err)
							p.bumpInvalid()
							continue
						}
						trie, err = statedb.OpenStorageTrie(common.BytesToHash(request.AccKey), account.Root)
						if trie == nil || err != nil {
							p.Log().Warn("Failed to open storage trie for proof", "block", header.Number, "hash", header.Hash(), "account", common.BytesToHash(request.AccKey), "root", account.Root, "err", err)
							continue
						}
					}
					// Prove the user's request from the account or stroage trie
					if err := trie.Prove(request.Key, request.FromLevel, nodes); err != nil {
						p.Log().Warn("Failed to prove state request", "block", header.Number, "hash", header.Hash(), "err", err)
						continue
					}
					if nodes.DataSize() >= softResponseLimit {
						break
					}
				}
				reply := p.replyProofsV2(req.ReqID, nodes.NodeList())
				sendResponse(req.ReqID, uint64(reqCnt), reply, task.done())
				if metrics.EnabledExpensive {
					miscOutTrieProofPacketsMeter.Mark(1)
					miscOutTrieProofTrafficMeter.Mark(int64(reply.size()))
					miscServingTimeTrieProofTimer.Update(time.Duration(task.servingTime))
				}
			}()
		}

	case GetHelperTrieProofsMsg:
		p.Log().Trace("Received helper trie proof request")
		if metrics.EnabledExpensive {
			miscInHelperTriePacketsMeter.Mark(1)
			miscInHelperTrieTrafficMeter.Mark(int64(msg.Size))
		}
		var req struct {
			ReqID uint64
			Reqs  []HelperTrieReq
		}
		if err := msg.Decode(&req); err != nil {
			clientErrorMeter.Mark(1)
			return errResp(ErrDecode, "msg %v: %v", msg, err)
		}
		// Gather state data until the fetch or network limits is reached
		var (
			auxBytes int
			auxData  [][]byte
		)
		reqCnt := len(req.Reqs)
		if accept(req.ReqID, uint64(reqCnt), MaxHelperTrieProofsFetch) {
			wg.Add(1)
			go func() {
				defer wg.Done()
				var (
					lastIdx  uint64
					lastType uint
					root     common.Hash
					auxTrie  *trie.Trie
				)
				nodes := light.NewNodeSet()
				for i, request := range req.Reqs {
					if i != 0 && !task.waitOrStop() {
						sendResponse(req.ReqID, 0, nil, task.servingTime)
						return
					}
					if auxTrie == nil || request.Type != lastType || request.TrieIdx != lastIdx {
						auxTrie, lastType, lastIdx = nil, request.Type, request.TrieIdx

						var prefix string
						if root, prefix = h.getHelperTrie(request.Type, request.TrieIdx); root != (common.Hash{}) {
							auxTrie, _ = trie.New(root, trie.NewDatabase(rawdb.NewTable(h.chainDb, prefix)))
						}
					}
					if request.AuxReq == auxRoot {
						var data []byte
						if root != (common.Hash{}) {
							data = root[:]
						}
						auxData = append(auxData, data)
						auxBytes += len(data)
					} else {
						if auxTrie != nil {
							auxTrie.Prove(request.Key, request.FromLevel, nodes)
						}
						if request.AuxReq != 0 {
							data := h.getAuxiliaryHeaders(request)
							auxData = append(auxData, data)
							auxBytes += len(data)
						}
					}
					if nodes.DataSize()+auxBytes >= softResponseLimit {
						break
					}
				}
				reply := p.replyHelperTrieProofs(req.ReqID, HelperTrieResps{Proofs: nodes.NodeList(), AuxData: auxData})
				sendResponse(req.ReqID, uint64(reqCnt), reply, task.done())
				if metrics.EnabledExpensive {
					miscOutHelperTriePacketsMeter.Mark(1)
					miscOutHelperTrieTrafficMeter.Mark(int64(reply.size()))
					miscServingTimeHelperTrieTimer.Update(time.Duration(task.servingTime))
				}
			}()
		}

	case SendTxV2Msg:
		p.Log().Trace("Received new transactions")
		if metrics.EnabledExpensive {
			miscInTxsPacketsMeter.Mark(1)
			miscInTxsTrafficMeter.Mark(int64(msg.Size))
		}
		var req struct {
			ReqID uint64
			Txs   []*types.Transaction
		}
		if err := msg.Decode(&req); err != nil {
			clientErrorMeter.Mark(1)
			return errResp(ErrDecode, "msg %v: %v", msg, err)
		}
		reqCnt := len(req.Txs)
		if accept(req.ReqID, uint64(reqCnt), MaxTxSend) {
			wg.Add(1)
			go func() {
				defer wg.Done()
				stats := make([]light.TxStatus, len(req.Txs))
				for i, tx := range req.Txs {
					if i != 0 && !task.waitOrStop() {
						return
					}
					hash := tx.Hash()
					stats[i] = h.txStatus(hash)
					if stats[i].Status == core.TxStatusUnknown {
						addFn := h.txpool.AddRemotes
						// Add txs synchronously for testing purpose
						if h.addTxsSync {
							addFn = h.txpool.AddRemotesSync
						}
						if errs := addFn([]*types.Transaction{tx}); errs[0] != nil {
							stats[i].Error = errs[0].Error()
							continue
						}
						stats[i] = h.txStatus(hash)
					}
				}
				reply := p.replyTxStatus(req.ReqID, stats)
				sendResponse(req.ReqID, uint64(reqCnt), reply, task.done())
				if metrics.EnabledExpensive {
					miscOutTxsPacketsMeter.Mark(1)
					miscOutTxsTrafficMeter.Mark(int64(reply.size()))
					miscServingTimeTxTimer.Update(time.Duration(task.servingTime))
				}
			}()
		}

	case GetTxStatusMsg:
		p.Log().Trace("Received transaction status query request")
		if metrics.EnabledExpensive {
			miscInTxStatusPacketsMeter.Mark(1)
			miscInTxStatusTrafficMeter.Mark(int64(msg.Size))
		}
		var req struct {
			ReqID  uint64
			Hashes []common.Hash
		}
		if err := msg.Decode(&req); err != nil {
			clientErrorMeter.Mark(1)
			return errResp(ErrDecode, "msg %v: %v", msg, err)
		}
		reqCnt := len(req.Hashes)
		if accept(req.ReqID, uint64(reqCnt), MaxTxStatus) {
			wg.Add(1)
			go func() {
				defer wg.Done()
				stats := make([]light.TxStatus, len(req.Hashes))
				for i, hash := range req.Hashes {
					if i != 0 && !task.waitOrStop() {
						sendResponse(req.ReqID, 0, nil, task.servingTime)
						return
					}
					stats[i] = h.txStatus(hash)
				}
				reply := p.replyTxStatus(req.ReqID, stats)
				sendResponse(req.ReqID, uint64(reqCnt), reply, task.done())
				if metrics.EnabledExpensive {
					miscOutTxStatusPacketsMeter.Mark(1)
					miscOutTxStatusTrafficMeter.Mark(int64(reply.size()))
					miscServingTimeTxStatusTimer.Update(time.Duration(task.servingTime))
				}
			}()
		}

	default:
=======
	// Lookup the request handler table, ensure it's supported
	// message type by the protocol.
	req, ok := Les3[msg.Code]
	if !ok {
>>>>>>> v1.10.7
		p.Log().Trace("Received invalid message", "code", msg.Code)
		clientErrorMeter.Mark(1)
		return errResp(ErrInvalidMsgCode, "%v", msg.Code)
	}
	p.Log().Trace("Received " + req.Name)

	// Decode the p2p message, resolve the concrete handler for it.
	serve, reqID, reqCnt, err := req.Handle(msg)
	if err != nil {
		clientErrorMeter.Mark(1)
		return errResp(ErrDecode, "%v: %v", msg, err)
	}
	if metrics.EnabledExpensive {
		req.InPacketsMeter.Mark(1)
		req.InTrafficMeter.Mark(int64(msg.Size))
	}
	p.responseCount++
	responseCount := p.responseCount

	// First check this client message complies all rules before
	// handling it and return a processor if all checks are passed.
	task, maxCost := h.beforeHandle(p, reqID, responseCount, msg, reqCnt, req.MaxCount)
	if task == nil {
		return nil
	}
	wg.Add(1)
	go func() {
		defer wg.Done()

		reply := serve(h, p, task.waitOrStop)
		h.afterHandle(p, reqID, responseCount, msg, maxCost, reqCnt, task, reply)

		if metrics.EnabledExpensive {
			size := uint32(0)
			if reply != nil {
				size = reply.size()
			}
			req.OutPacketsMeter.Mark(1)
			req.OutTrafficMeter.Mark(int64(size))
			req.ServingTimeMeter.Update(time.Duration(task.servingTime))
		}
	}()
	// If the client has made too much invalid request(e.g. request a non-existent data),
	// reject them to prevent SPAM attack.
	if p.getInvalid() > maxRequestErrors {
		clientErrorMeter.Mark(1)
		return errTooManyInvalidRequest
	}
	return nil
}

// BlockChain implements serverBackend
func (h *serverHandler) BlockChain() *core.BlockChain {
	return h.blockchain
}

// TxPool implements serverBackend
func (h *serverHandler) TxPool() *core.TxPool {
	return h.txpool
}

// ArchiveMode implements serverBackend
func (h *serverHandler) ArchiveMode() bool {
	return h.server.archiveMode
}

// AddTxsSync implements serverBackend
func (h *serverHandler) AddTxsSync() bool {
	return h.addTxsSync
}

// getAccount retrieves an account from the state based on root.
func getAccount(triedb *trie.Database, root, hash common.Hash) (state.Account, error) {
	trie, err := trie.New(root, triedb)
	if err != nil {
		return state.Account{}, err
	}
	blob, err := trie.TryGet(hash[:])
	if err != nil {
		return state.Account{}, err
	}
	var account state.Account
	if err = rlp.DecodeBytes(blob, &account); err != nil {
		return state.Account{}, err
	}
	return account, nil
}

// getHelperTrie returns the post-processed trie root for the given trie ID and section index
func (h *serverHandler) GetHelperTrie(typ uint, index uint64) *trie.Trie {
	var (
		root   common.Hash
		prefix string
	)
	switch typ {
	case htCanonical:
		sectionHead := rawdb.ReadCanonicalHash(h.chainDb, (index+1)*h.server.iConfig.ChtSize-1)
		root, prefix = light.GetChtRoot(h.chainDb, index, sectionHead), light.ChtTablePrefix
	case htBloomBits:
		sectionHead := rawdb.ReadCanonicalHash(h.chainDb, (index+1)*h.server.iConfig.BloomTrieSize-1)
		root, prefix = light.GetBloomTrieRoot(h.chainDb, index, sectionHead), light.BloomTrieTablePrefix
	}
	if root == (common.Hash{}) {
		return nil
	}
	trie, _ := trie.New(root, trie.NewDatabase(rawdb.NewTable(h.chainDb, prefix)))
	return trie
}

// broadcastLoop broadcasts new block information to all connected light
// clients. According to the agreement between client and server, server should
// only broadcast new announcement if the total difficulty is higher than the
// last one. Besides server will add the signature if client requires.
func (h *serverHandler) broadcastLoop() {
	defer h.wg.Done()

	headCh := make(chan core.ChainHeadEvent, 10)
	headSub := h.blockchain.SubscribeChainHeadEvent(headCh)
	defer headSub.Unsubscribe()

	var (
		lastHead *types.Header
		lastTd   = common.Big0
	)
	for {
		select {
		case ev := <-headCh:
			header := ev.Block.Header()
			hash, number := header.Hash(), header.Number.Uint64()
			td := h.blockchain.GetTd(hash, number)
			if td == nil || td.Cmp(lastTd) <= 0 {
				continue
			}
			var reorg uint64
			if lastHead != nil {
				reorg = lastHead.Number.Uint64() - rawdb.FindCommonAncestor(h.chainDb, header, lastHead).Number.Uint64()
			}
			lastHead, lastTd = header, td
			log.Debug("Announcing block to peers", "number", number, "hash", hash, "td", td, "reorg", reorg)
			h.server.peers.broadcast(announceData{Hash: hash, Number: number, Td: td, ReorgDepth: reorg})
		case <-h.closeCh:
			return
		}
	}
}
<<<<<<< HEAD

func (h *serverHandler) verifyGatewayFee(gatewayFeeRecipient *common.Address, gatewayFee *big.Int) error {

	// If this node does not specify an etherbase, accept any GatewayFeeRecipient.
	if h.etherbase == common.ZeroAddress {
		return nil
	}

	// If this node does not specify a non-zero gateway fee accept any value.
	if h.gatewayFee == nil || h.gatewayFee.Cmp(common.Big0) <= 0 {
		return nil
	}

	// Otherwise, reject transactions that don't pay gas fees to this node.
	if gatewayFeeRecipient == nil {
		return fmt.Errorf("gateway fee recipient must be %s, got <nil>", h.etherbase.String())
	}
	if *gatewayFeeRecipient != h.etherbase {
		return fmt.Errorf("gateway fee recipient must be %s, got %s", h.etherbase.String(), (*gatewayFeeRecipient).String())
	}

	// Check that the value of the supplied gateway fee is at least the minimum.
	if gatewayFee == nil || gatewayFee.Cmp(h.gatewayFee) < 0 {
		return fmt.Errorf("gateway fee value must be at least %s, got %s", h.gatewayFee, gatewayFee)
	}
	return nil
}

// broadcaster sends new header announcements to active client peers
type broadcaster struct {
	ns                           *nodestate.NodeStateMachine
	privateKey                   *ecdsa.PrivateKey
	lastAnnounce, signedAnnounce announceData
}

// newBroadcaster creates a new broadcaster
func newBroadcaster(ns *nodestate.NodeStateMachine) *broadcaster {
	b := &broadcaster{ns: ns}
	ns.SubscribeState(priorityPoolSetup.ActiveFlag, func(node *enode.Node, oldState, newState nodestate.Flags) {
		if newState.Equals(priorityPoolSetup.ActiveFlag) {
			// send last announcement to activated peers
			b.sendTo(node)
		}
	})
	return b
}

// setSignerKey sets the signer key for signed announcements. Should be called before
// starting the protocol handler.
func (b *broadcaster) setSignerKey(privateKey *ecdsa.PrivateKey) {
	b.privateKey = privateKey
}

// broadcast sends the given announcements to all active peers
func (b *broadcaster) broadcast(announce announceData) {
	b.ns.Operation(func() {
		// iterate in an Operation to ensure that the active set does not change while iterating
		b.lastAnnounce = announce
		b.ns.ForEach(priorityPoolSetup.ActiveFlag, nodestate.Flags{}, func(node *enode.Node, state nodestate.Flags) {
			b.sendTo(node)
		})
	})
}

// sendTo sends the most recent announcement to the given node unless the same or higher Td
// announcement has already been sent.
func (b *broadcaster) sendTo(node *enode.Node) {
	if b.lastAnnounce.Td == nil {
		return
	}
	if p, _ := b.ns.GetField(node, clientPeerField).(*clientPeer); p != nil {
		if p.headInfo.Td == nil || b.lastAnnounce.Td.Cmp(p.headInfo.Td) > 0 {
			announce := b.lastAnnounce
			switch p.announceType {
			case announceTypeSimple:
				if !p.queueSend(func() { p.sendAnnounce(announce) }) {
					log.Debug("Drop announcement because queue is full", "number", announce.Number, "hash", announce.Hash)
				} else {
					log.Debug("Sent announcement", "number", announce.Number, "hash", announce.Hash)
				}
			case announceTypeSigned:
				if b.signedAnnounce.Hash != b.lastAnnounce.Hash {
					b.signedAnnounce = b.lastAnnounce
					b.signedAnnounce.sign(b.privateKey)
				}
				announce := b.signedAnnounce
				if !p.queueSend(func() { p.sendAnnounce(announce) }) {
					log.Debug("Drop announcement because queue is full", "number", announce.Number, "hash", announce.Hash)
				} else {
					log.Debug("Sent announcement", "number", announce.Number, "hash", announce.Hash)
				}
			}
			p.headInfo = blockInfo{b.lastAnnounce.Hash, b.lastAnnounce.Number, b.lastAnnounce.Td}
		}
	}
}
||||||| e78727290

// broadcaster sends new header announcements to active client peers
type broadcaster struct {
	ns                           *nodestate.NodeStateMachine
	privateKey                   *ecdsa.PrivateKey
	lastAnnounce, signedAnnounce announceData
}

// newBroadcaster creates a new broadcaster
func newBroadcaster(ns *nodestate.NodeStateMachine) *broadcaster {
	b := &broadcaster{ns: ns}
	ns.SubscribeState(priorityPoolSetup.ActiveFlag, func(node *enode.Node, oldState, newState nodestate.Flags) {
		if newState.Equals(priorityPoolSetup.ActiveFlag) {
			// send last announcement to activated peers
			b.sendTo(node)
		}
	})
	return b
}

// setSignerKey sets the signer key for signed announcements. Should be called before
// starting the protocol handler.
func (b *broadcaster) setSignerKey(privateKey *ecdsa.PrivateKey) {
	b.privateKey = privateKey
}

// broadcast sends the given announcements to all active peers
func (b *broadcaster) broadcast(announce announceData) {
	b.ns.Operation(func() {
		// iterate in an Operation to ensure that the active set does not change while iterating
		b.lastAnnounce = announce
		b.ns.ForEach(priorityPoolSetup.ActiveFlag, nodestate.Flags{}, func(node *enode.Node, state nodestate.Flags) {
			b.sendTo(node)
		})
	})
}

// sendTo sends the most recent announcement to the given node unless the same or higher Td
// announcement has already been sent.
func (b *broadcaster) sendTo(node *enode.Node) {
	if b.lastAnnounce.Td == nil {
		return
	}
	if p, _ := b.ns.GetField(node, clientPeerField).(*clientPeer); p != nil {
		if p.headInfo.Td == nil || b.lastAnnounce.Td.Cmp(p.headInfo.Td) > 0 {
			announce := b.lastAnnounce
			switch p.announceType {
			case announceTypeSimple:
				if !p.queueSend(func() { p.sendAnnounce(announce) }) {
					log.Debug("Drop announcement because queue is full", "number", announce.Number, "hash", announce.Hash)
				} else {
					log.Debug("Sent announcement", "number", announce.Number, "hash", announce.Hash)
				}
			case announceTypeSigned:
				if b.signedAnnounce.Hash != b.lastAnnounce.Hash {
					b.signedAnnounce = b.lastAnnounce
					b.signedAnnounce.sign(b.privateKey)
				}
				announce := b.signedAnnounce
				if !p.queueSend(func() { p.sendAnnounce(announce) }) {
					log.Debug("Drop announcement because queue is full", "number", announce.Number, "hash", announce.Hash)
				} else {
					log.Debug("Sent announcement", "number", announce.Number, "hash", announce.Hash)
				}
			}
			p.headInfo = blockInfo{b.lastAnnounce.Hash, b.lastAnnounce.Number, b.lastAnnounce.Td}
		}
	}
}
=======
>>>>>>> v1.10.7
