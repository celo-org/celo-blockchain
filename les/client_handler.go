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
	"math"
	"math/big"
	"sync"
	"sync/atomic"
	"time"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/common/mclock"
	"github.com/celo-org/celo-blockchain/consensus/istanbul"
	"github.com/celo-org/celo-blockchain/core/types"
	"github.com/celo-org/celo-blockchain/eth/downloader"
	"github.com/celo-org/celo-blockchain/light"
	"github.com/celo-org/celo-blockchain/log"
	"github.com/celo-org/celo-blockchain/p2p"
	"github.com/celo-org/celo-blockchain/params"
)

// clientHandler is responsible for receiving and processing all incoming server
// responses.
type clientHandler struct {
	ulc        *ulc
	checkpoint *params.TrustedCheckpoint
	fetcher    *lightFetcher
	downloader *downloader.Downloader
	backend    *LightEthereum
	syncMode   downloader.SyncMode

	// TODO(nategraf) Remove this field once gateway fees can be retreived.
	gatewayFee *big.Int

	closeCh  chan struct{}
	wg       sync.WaitGroup // WaitGroup used to track all connected peers.
	syncDone func()         // Test hooks when syncing is done.

	gatewayFeeCache *gatewayFeeCache
}

type GatewayFeeInformation struct {
	GatewayFee *big.Int
	Etherbase  common.Address
}

type gatewayFeeCache struct {
	mutex         *sync.RWMutex
	gatewayFeeMap map[string]*GatewayFeeInformation
}

func newGatewayFeeCache() *gatewayFeeCache {
	cache := &gatewayFeeCache{
		mutex:         new(sync.RWMutex),
		gatewayFeeMap: make(map[string]*GatewayFeeInformation),
	}
	return cache
}

func (c *gatewayFeeCache) getMap() map[string]*GatewayFeeInformation {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	mapCopy := make(map[string]*GatewayFeeInformation)
	for k, v := range c.gatewayFeeMap {
		mapCopy[k] = v
	}

	return mapCopy
}

func (c *gatewayFeeCache) update(nodeID string, val *GatewayFeeInformation) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if val.Etherbase == common.ZeroAddress || val.GatewayFee.Cmp(big.NewInt(0)) < 0 {
		return errors.New("invalid gatewayFeeInformation object")
	}
	c.gatewayFeeMap[nodeID] = val

	return nil
}

func (c *gatewayFeeCache) MinPeerGatewayFee() (*GatewayFeeInformation, error) {
	gatewayFeeMap := c.getMap()

	if len(gatewayFeeMap) == 0 {
		return nil, nil
	}

	minGwFee := big.NewInt(math.MaxInt64)
	minEtherbase := common.ZeroAddress
	for _, gwFeeInformation := range gatewayFeeMap {
		if gwFeeInformation.GatewayFee.Cmp(minGwFee) < 0 {
			minGwFee = gwFeeInformation.GatewayFee
			minEtherbase = gwFeeInformation.Etherbase
		}
	}

	minGatewayFeeInformation := &GatewayFeeInformation{minGwFee, minEtherbase}
	return minGatewayFeeInformation, nil
}

func newClientHandler(syncMode downloader.SyncMode, ulcServers []string, ulcFraction int, checkpoint *params.TrustedCheckpoint, backend *LightEthereum, gatewayFee *big.Int) *clientHandler {
	handler := &clientHandler{
		checkpoint: checkpoint,
		backend:    backend,
		closeCh:    make(chan struct{}),
		syncMode:   syncMode,
		gatewayFee: gatewayFee,
	}
	if ulcServers != nil {
		ulc, err := newULC(ulcServers, ulcFraction)
		if err != nil {
			log.Error("Failed to initialize ultra light client")
		}
		handler.ulc = ulc
		log.Info("Enable ultra light client mode")
	}
	var height uint64
	if checkpoint != nil {
		height = (checkpoint.SectionIndex+1)*params.CHTFrequency - 1
	}
	handler.fetcher = newLightFetcher(handler, backend.serverPool.getTimeout)
	// TODO mcortesi lightest boolean
	handler.downloader = downloader.New(height, backend.chainDb, nil, backend.eventMux, nil, backend.blockchain, handler.removePeer)
	handler.backend.peers.subscribe((*downloaderPeerNotify)(handler))

	handler.gatewayFeeCache = newGatewayFeeCache()
	return handler
}

func (h *clientHandler) stop() {
	close(h.closeCh)
	h.downloader.Terminate()
	h.fetcher.close()
	h.wg.Wait()
}

// runPeer is the p2p protocol run function for the given version.
func (h *clientHandler) runPeer(version uint, p *p2p.Peer, rw p2p.MsgReadWriter) error {
	trusted := false
	if h.ulc != nil {
		trusted = h.ulc.trusted(p.ID())
	}
	peer := newServerPeer(int(version), h.backend.config.NetworkId, trusted, p, newMeteredMsgWriter(rw, int(version)))
	defer peer.close()
	h.wg.Add(1)
	defer h.wg.Done()
	err := h.handle(peer)
	return err
}

func (h *clientHandler) handle(p *serverPeer) error {
	// KJUE - Remove the server not nil check after restoring peer check in server.go
	if p.Peer.Server != nil {
		if err := p.Peer.Server.CheckPeerCounts(p.Peer); err != nil {
			return err
		}
	}
	if h.backend.peers.len() >= h.backend.config.LightPeers && !p.Peer.Info().Network.Trusted {
		return p2p.DiscTooManyPeers
	}
	p.Log().Debug("Light Ethereum peer connected", "name", p.Name())

	// Execute the LES handshake
	var (
		head   = h.backend.blockchain.CurrentHeader()
		hash   = head.Hash()
		number = head.Number.Uint64()
		td     = h.backend.blockchain.GetTd(hash, number)
	)
	if err := p.Handshake(td, hash, number, h.backend.blockchain.Genesis().Hash(), nil); err != nil {
		p.Log().Debug("Light Ethereum handshake failed", "err", err)
		return err
	}

	// TODO(nategraf) The local gateway fee is temporarily being used as the peer gateway fee.
	p.SetGatewayFee(h.gatewayFee)

	// Register the peer locally
	if err := h.backend.peers.register(p); err != nil {
		p.Log().Error("Light Ethereum peer registration failed", "err", err)
		return err
	}
	serverConnectionGauge.Update(int64(h.backend.peers.len()))

	connectedAt := mclock.Now()
	defer func() {
		h.backend.peers.unregister(p.id)
		connectionTimer.Update(time.Duration(mclock.Now() - connectedAt))
		serverConnectionGauge.Update(int64(h.backend.peers.len()))
	}()

	h.fetcher.announce(p, &announceData{Hash: p.headInfo.Hash, Number: p.headInfo.Number, Td: p.headInfo.Td})

	// Loop until we receive a RequestEtherbase response or timeout.
	go func() {
		maxRequests := 10
		for requests := 1; requests <= maxRequests; requests++ {
			p.Log().Trace("Requesting etherbase from new peer")
			reqID := genReqID()
			cost := p.getRequestCost(GetEtherbaseMsg, int(1))
			err := p.RequestEtherbase(reqID, cost)

			if err != nil {
				p.Log().Warn("Unable to request etherbase from peer", "err", err)
			}

			time.Sleep(time.Duration(math.Pow(2, float64(requests))/2) * time.Second)
			if _, ok := p.Etherbase(); ok {
				return
			}
		}
	}()

	// Mark the peer starts to be served.
	atomic.StoreUint32(&p.serving, 1)
	defer atomic.StoreUint32(&p.serving, 0)

	// Spawn a main loop to handle all incoming messages.
	for {
		if err := h.handleMsg(p); err != nil {
			p.Log().Debug("Light Ethereum message handling failed", "err", err)
			p.fcServer.DumpLogs()
			return err
		}
	}
}

// handleMsg is invoked whenever an inbound message is received from a remote
// peer. The remote connection is torn down upon returning any error.
func (h *clientHandler) handleMsg(p *serverPeer) error {
	// Read the next message from the remote peer, and ensure it's fully consumed
	msg, err := p.rw.ReadMsg()
	if err != nil {
		return err
	}
	p.Log().Trace("Light Ethereum message arrived", "code", msg.Code, "bytes", msg.Size)

	if msg.Size > ProtocolMaxMsgSize {
		return errResp(ErrMsgTooLarge, "%v > %v", msg.Size, ProtocolMaxMsgSize)
	}
	defer msg.Discard()

	var deliverMsg *Msg

	// Handle the message depending on its contents
	switch msg.Code {
	case AnnounceMsg:
		p.Log().Trace("Received announce message")
		var req announceData
		if err := msg.Decode(&req); err != nil {
			return errResp(ErrDecode, "%v: %v", msg, err)
		}
		if err := req.sanityCheck(); err != nil {
			return err
		}
		update, size := req.Update.decode()
		if p.rejectUpdate(size) {
			return errResp(ErrRequestRejected, "")
		}
		p.updateFlowControl(update)
		p.updateVtParams()

		if req.Hash != (common.Hash{}) {
			if p.announceType == announceTypeNone {
				return errResp(ErrUnexpectedResponse, "")
			}
			if p.announceType == announceTypeSigned {
				if err := req.checkSignature(p.ID(), update); err != nil {
					p.Log().Trace("Invalid announcement signature", "err", err)
					return err
				}
				p.Log().Trace("Valid announcement signature")
			}
			p.Log().Trace("Announce message content", "number", req.Number, "hash", req.Hash, "td", req.Td, "reorg", req.ReorgDepth)
			h.fetcher.announce(p, &req)
		}
	case BlockHeadersMsg:
		var resp struct {
			ReqID, BV uint64
			Headers   []*types.Header
		}
		if err := msg.Decode(&resp); err != nil {
			return errResp(ErrDecode, "msg %v: %v", msg, err)
		}
		p.Log().Error("Received block header response message", "headers", resp.Headers)
		p.fcServer.ReceivedReply(resp.ReqID, resp.BV)
		p.answeredRequest(resp.ReqID)
		if h.fetcher.requestedID(resp.ReqID) {
			h.fetcher.deliverHeaders(p, resp.ReqID, resp.Headers)
		} else { // ODR
			h.backend.retriever.lock.RLock()
			headerRequested := h.backend.retriever.sentReqs[resp.ReqID]
			h.backend.retriever.lock.RUnlock()
			if headerRequested != nil {
				contiguousHeaders := h.syncMode != downloader.LightestSync
				log.Error("Inserting header chain")
				if _, err := h.fetcher.chain.InsertHeaderChain(resp.Headers, 1, contiguousHeaders); err != nil {
					return err
				}
				deliverMsg = &Msg{
					MsgType: MsgBlockHeaders,
					ReqID:   resp.ReqID,
					Obj:     resp.Headers,
				}
			} else {
				if err := h.downloader.DeliverHeaders(p.id, resp.Headers); err != nil {
					log.Error("Failed to deliver headers", "err", err)
				}
			}
		}
	case BlockBodiesMsg:
		p.Log().Trace("Received block bodies response")
		var resp struct {
			ReqID, BV uint64
			Data      []*types.Body
		}
		if err := msg.Decode(&resp); err != nil {
			return errResp(ErrDecode, "msg %v: %v", msg, err)
		}
		p.fcServer.ReceivedReply(resp.ReqID, resp.BV)
		p.answeredRequest(resp.ReqID)
		deliverMsg = &Msg{
			MsgType: MsgBlockBodies,
			ReqID:   resp.ReqID,
			Obj:     resp.Data,
		}
	case CodeMsg:
		p.Log().Trace("Received code response")
		var resp struct {
			ReqID, BV uint64
			Data      [][]byte
		}
		if err := msg.Decode(&resp); err != nil {
			return errResp(ErrDecode, "msg %v: %v", msg, err)
		}
		p.fcServer.ReceivedReply(resp.ReqID, resp.BV)
		p.answeredRequest(resp.ReqID)
		deliverMsg = &Msg{
			MsgType: MsgCode,
			ReqID:   resp.ReqID,
			Obj:     resp.Data,
		}
	case ReceiptsMsg:
		p.Log().Trace("Received receipts response")
		var resp struct {
			ReqID, BV uint64
			Receipts  []types.Receipts
		}
		if err := msg.Decode(&resp); err != nil {
			return errResp(ErrDecode, "msg %v: %v", msg, err)
		}
		p.fcServer.ReceivedReply(resp.ReqID, resp.BV)
		p.answeredRequest(resp.ReqID)
		deliverMsg = &Msg{
			MsgType: MsgReceipts,
			ReqID:   resp.ReqID,
			Obj:     resp.Receipts,
		}
	case ProofsV2Msg:
		p.Log().Trace("Received les/2 proofs response")
		var resp struct {
			ReqID, BV uint64
			Data      light.NodeList
		}
		if err := msg.Decode(&resp); err != nil {
			return errResp(ErrDecode, "msg %v: %v", msg, err)
		}
		p.fcServer.ReceivedReply(resp.ReqID, resp.BV)
		p.answeredRequest(resp.ReqID)
		deliverMsg = &Msg{
			MsgType: MsgProofsV2,
			ReqID:   resp.ReqID,
			Obj:     resp.Data,
		}
	case HelperTrieProofsMsg:
		p.Log().Trace("Received helper trie proof response")
		var resp struct {
			ReqID, BV uint64
			Data      HelperTrieResps
		}
		if err := msg.Decode(&resp); err != nil {
			return errResp(ErrDecode, "msg %v: %v", msg, err)
		}
		p.fcServer.ReceivedReply(resp.ReqID, resp.BV)
		p.answeredRequest(resp.ReqID)
		deliverMsg = &Msg{
			MsgType: MsgHelperTrieProofs,
			ReqID:   resp.ReqID,
			Obj:     resp.Data,
		}
	case TxStatusMsg:
		p.Log().Trace("Received tx status response")
		var resp struct {
			ReqID, BV uint64
			Status    []light.TxStatus
		}
		if err := msg.Decode(&resp); err != nil {
			return errResp(ErrDecode, "msg %v: %v", msg, err)
		}
		p.fcServer.ReceivedReply(resp.ReqID, resp.BV)
		p.answeredRequest(resp.ReqID)
		p.Log().Trace("Transaction status update", "status", resp.Status, "req", resp.ReqID)
		deliverMsg = &Msg{
			MsgType: MsgTxStatus,
			ReqID:   resp.ReqID,
			Obj:     resp.Status,
		}
	case StopMsg:
		p.freeze()
		h.backend.retriever.frozen(p)
		p.Log().Debug("Service stopped")
	case ResumeMsg:
		var bv uint64
		if err := msg.Decode(&bv); err != nil {
			return errResp(ErrDecode, "msg %v: %v", msg, err)
		}
		p.fcServer.ResumeFreeze(bv)
		p.unfreeze()
		p.Log().Debug("Service resumed")
	case EtherbaseMsg:
		p.Log().Trace("Received etherbase response")
		// TODO(asa): do we need to do anything with flow control here?
		var resp struct {
			ReqID, BV uint64
			Etherbase common.Address
		}
		if err := msg.Decode(&resp); err != nil {
			return errResp(ErrDecode, "msg %v: %v", msg, err)
		}
		p.fcServer.ReceivedReply(resp.ReqID, resp.BV)
		p.Log().Trace("Setting peer etherbase", "etherbase", resp.Etherbase, "Peer ID", p.ID)
		p.SetEtherbase(resp.Etherbase)

	case GatewayFeeMsg:
		var resp struct {
			ReqID, BV uint64
			Data      GatewayFeeInformation
		}

		if err := msg.Decode(&resp); err != nil {
			return errResp(ErrDecode, "msg %v: %v", msg, err)
		}

		p.fcServer.ReceivedReply(resp.ReqID, resp.BV)
		h.gatewayFeeCache.update(p.id, &resp.Data)

	case PlumoProofInventoryMsg:
		p.Log().Error("Received PlumoProofInventory response")
		var resp struct {
			// TODO would be nice to have detail on what BV exactly is? Looks like a buffer limit of sorts
			ReqID, BV       uint64
			ProofsInventory []types.PlumoProofMetadata
		}
		if err := msg.Decode(&resp); err != nil {
			return errResp(ErrDecode, "msg %v: %v", msg, err)
		}

		p.fcServer.ReceivedReply(resp.ReqID, resp.BV)
		p.Log().Error("Received proof inventory", "inventory", resp.ProofsInventory)
		h.fetcher.importKnownPlumoProofs(p, resp.ProofsInventory)

	case PlumoProofsMsg:
		p.Log().Error("Recieved PlumoProofsMsg response")
		var resp struct {
			ReqID, BV   uint64
			LightProofs []istanbul.LightPlumoProof
		}
		if err := msg.Decode(&resp); err != nil {
			p.Log().Error("Error decoding")
			return errResp(ErrDecode, "msg %v: %v", msg, err)
		}

		p.fcServer.ReceivedReply(resp.ReqID, resp.BV)
		p.Log().Error("Received requested proofs", "light_proofs", resp.LightProofs)
		if err := h.downloader.DeliverPlumoProofs(p.id, resp.LightProofs); err != nil {
			log.Error("Failed to deliver proofs", "err", err)
		}

	default:
		p.Log().Trace("Received invalid message", "code", msg.Code)
		return errResp(ErrInvalidMsgCode, "%v", msg.Code)
	}
	// Deliver the received response to retriever.
	if deliverMsg != nil {
		if err := h.backend.retriever.deliver(p, deliverMsg); err != nil {
			p.errCount++
			if p.errCount > maxResponseErrors {
				return err
			}
		}
	}
	return nil
}

func (h *clientHandler) removePeer(id string) {
	h.backend.peers.unregister(id)
}

type peerConnection struct {
	handler *clientHandler
	peer    *serverPeer
}

func (pc *peerConnection) Head() (common.Hash, *big.Int) {
	return pc.peer.HeadAndTd()
}

func (pc *peerConnection) RequestHeadersByHash(origin common.Hash, amount int, skip int, reverse bool) error {
	rq := &distReq{
		getCost: func(dp distPeer) uint64 {
			peer := dp.(*serverPeer)
			return peer.getRequestCost(GetBlockHeadersMsg, amount)
		},
		canSend: func(dp distPeer) bool {
			return dp.(*serverPeer) == pc.peer
		},
		request: func(dp distPeer) func() {
			reqID := genReqID()
			peer := dp.(*serverPeer)
			cost := peer.getRequestCost(GetBlockHeadersMsg, amount)
			peer.fcServer.QueuedRequest(reqID, cost)
			return func() { peer.requestHeadersByHash(reqID, origin, amount, skip, reverse) }
		},
	}
	_, ok := <-pc.handler.backend.reqDist.queue(rq)
	if !ok {
		log.Error("Returning no peers from request headers by hash")
		return light.ErrNoPeers
	}
	return nil
}

func (pc *peerConnection) RequestHeadersByNumber(origin uint64, amount int, skip int, reverse bool) error {
	rq := &distReq{
		getCost: func(dp distPeer) uint64 {
			peer := dp.(*serverPeer)
			return peer.getRequestCost(GetBlockHeadersMsg, amount)
		},
		canSend: func(dp distPeer) bool {
			return dp.(*serverPeer) == pc.peer
		},
		request: func(dp distPeer) func() {
			reqID := genReqID()
			peer := dp.(*serverPeer)
			cost := peer.getRequestCost(GetBlockHeadersMsg, amount)
			peer.fcServer.QueuedRequest(reqID, cost)
			return func() { peer.requestHeadersByNumber(reqID, origin, amount, skip, reverse) }
		},
	}
	_, ok := <-pc.handler.backend.reqDist.queue(rq)
	if !ok {
		log.Error("Returning no peers from request headers by number")
		return light.ErrNoPeers
	}
	return nil
}

func (pc *peerConnection) RequestPlumoProofInventory() error {
	rq := &distReq{
		getCost: func(dp distPeer) uint64 {
			peer := dp.(*serverPeer)
			// Would 0 work here?
			return peer.getRequestCost(GetPlumoProofInventoryMsg, 0)
		},
		canSend: func(dp distPeer) bool {
			return dp.(*serverPeer) == pc.peer
		},
		request: func(dp distPeer) func() {
			reqID := genReqID()
			peer := dp.(*serverPeer)
			cost := peer.getRequestCost(GetPlumoProofInventoryMsg, 0)
			peer.fcServer.QueuedRequest(reqID, cost)
			return func() { peer.RequestPlumoProofInventory(reqID, cost) }
		},
	}
	_, ok := <-pc.handler.backend.reqDist.queue(rq)
	if !ok {
		log.Error("Returning no peers from request proof inventory")
		return light.ErrNoPeers
	}
	return nil
}

func (pc *peerConnection) RequestPlumoProofsAndHeaders(from uint64, epoch uint64, skip int, maxPlumoProofFetch int, maxEpochHeaderFetch int) error {
	// Greedy alg for grabbing proofs
	// TODO version number
	// TODO limit based on max fetch
	var proofsToRequest []types.PlumoProofMetadata
	type headerGap struct {
		FirstEpoch uint
		Amount     int
	}
	var headerGaps []headerGap
	knownPlumoProofs := pc.peer.knownPlumoProofs
	var currFrom uint = uint(from)
	var currEpoch = uint(istanbul.GetEpochNumber(uint64(currFrom), epoch)) - 1
	// Outer loop finding the path
	for {
		// Inner loop adding the next proof
		var earliestMatch uint = math.MaxUint32
		var maxRange uint = 0
		var chosenProofMetadata types.PlumoProofMetadata
		for _, proofMetadata := range knownPlumoProofs {
			log.Error("iterating proofs", "currFrom", currFrom, "currEpoch", currEpoch, "firstEpoch", proofMetadata.FirstEpoch, "lastEpoch", proofMetadata.LastEpoch)
			if proofMetadata.FirstEpoch >= currEpoch {
				proofRange := proofMetadata.LastEpoch - proofMetadata.FirstEpoch
				if proofMetadata.FirstEpoch <= earliestMatch && proofRange > maxRange {
					log.Error("Updating match", "prev", earliestMatch, "prevRange", maxRange, "to", proofMetadata.FirstEpoch, "toAmount", proofRange)
					earliestMatch = uint(proofMetadata.FirstEpoch)
					maxRange = proofRange
					chosenProofMetadata = proofMetadata
				}
			}
		}
		// No more proofs to add, break
		if maxRange == 0 && currEpoch < earliestMatch {
			// TODO check height
			log.Error("No more proofs", "currFrom", currFrom, "original from", from)
			amount := int(earliestMatch - currEpoch)
			if amount > maxEpochHeaderFetch {
				amount = maxEpochHeaderFetch
			}
			gap := headerGap{
				FirstEpoch: currEpoch,
				Amount:     amount,
			}
			headerGaps = append(headerGaps, gap)
			break
		}
		if currEpoch < earliestMatch {
			log.Error("Need to add header gap", "currEpoch", currEpoch, "earliestMatch", earliestMatch)
			gap := headerGap{
				FirstEpoch: currEpoch + 1,
				Amount:     int(earliestMatch - currEpoch),
			}
			headerGaps = append(headerGaps, gap)
		}
		proofsToRequest = append(proofsToRequest, chosenProofMetadata)
		if len(proofsToRequest) >= maxPlumoProofFetch {
			break
		}
		currEpoch = chosenProofMetadata.LastEpoch
	}

	if len(proofsToRequest) > 0 {
		log.Error("Requesting proofs", "numProofs", len(proofsToRequest))
		proofReq := &distReq{
			getCost: func(dp distPeer) uint64 {
				peer := dp.(*serverPeer)
				return peer.getRequestCost(GetPlumoProofsMsg, len(proofsToRequest))
			},
			canSend: func(dp distPeer) bool {
				return dp.(*serverPeer) == pc.peer
			},
			request: func(dp distPeer) func() {
				reqID := genReqID()
				peer := dp.(*serverPeer)
				cost := peer.getRequestCost(GetPlumoProofInventoryMsg, len(proofsToRequest))
				peer.fcServer.QueuedRequest(reqID, cost)
				return func() { peer.RequestPlumoProofs(reqID, cost, proofsToRequest) }
			},
		}
		_, ok := <-pc.handler.backend.reqDist.queue(proofReq)
		if !ok {
			log.Error("Returning no peers from request proofs and headers")
			return light.ErrNoPeers
		}
	}
	// This does seem to work in some ways, testing proofs now
	for i := len(headerGaps) - 1; i >= 0; i-- {
		headerGap := headerGaps[i]
		// for _, headerGap := range headerGaps {
		log.Error("Requesting headergap", "firstEpoch", headerGap.FirstEpoch, "amount", headerGap.Amount, "greater?", int(headerGap.FirstEpoch) > 52)

		// if int(headerGap.FirstEpoch)+headerGap.Amount >= 52 {
		// 	if int(headerGap.FirstEpoch) == 52 {
		// 		headerGap.Amount = 1
		// 	} else {
		// 		headerGap.Amount = 52 - int(headerGap.FirstEpoch)
		// 	}
		// }
		headerReq := &distReq{
			getCost: func(dp distPeer) uint64 {
				peer := dp.(*serverPeer)
				return peer.getRequestCost(GetBlockHeadersMsg, headerGap.Amount)
			},
			canSend: func(dp distPeer) bool {
				return dp.(*serverPeer) == pc.peer
			},
			request: func(dp distPeer) func() {
				reqID := genReqID()
				peer := dp.(*serverPeer)
				cost := peer.getRequestCost(GetBlockHeadersMsg, headerGap.Amount)
				peer.fcServer.QueuedRequest(reqID, cost)
				return func() {
					blockNumber := uint64(headerGap.FirstEpoch) * epoch
					log.Error("Requesting headers by number", "blockNumber", blockNumber, "amount", headerGap.Amount)
					peer.requestHeadersByNumber(reqID, blockNumber, headerGap.Amount, skip, false)
				}
			},
		}
		_, ok := <-pc.handler.backend.reqDist.queue(headerReq)
		if !ok {
			log.Error("Returning no peers from request proofs and headers 2")
			return light.ErrNoPeers
		}
	}
	return nil
}

// downloaderPeerNotify implements peerSetNotify
type downloaderPeerNotify clientHandler

func (d *downloaderPeerNotify) registerPeer(p *serverPeer) {
	h := (*clientHandler)(d)
	pc := &peerConnection{
		handler: h,
		peer:    p,
	}
	h.downloader.RegisterLightPeer(p.id, ethVersion, pc)
}

func (d *downloaderPeerNotify) unregisterPeer(p *serverPeer) {
	h := (*clientHandler)(d)
	h.downloader.UnregisterPeer(p.id)
}
