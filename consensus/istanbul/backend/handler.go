// Copyright 2017 The go-ethereum Authors
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

package backend

import (
	"errors"
	"reflect"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/consensus/istanbul"
	vet "github.com/ethereum/go-ethereum/consensus/istanbul/backend/internal/enodes"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/event"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/rlp"
	lru "github.com/hashicorp/golang-lru"
)

var (
	// errDecodeFailed is returned when decode message fails
	errDecodeFailed = errors.New("fail to decode istanbul message")
	// errNonValidatorMessage is returned when `handleConsensusMsg` receives
	// a message with a signature from a non validator
	errNonValidatorMessage = errors.New("proxy received consensus message of a non validator")
)

// If you want to add a code, you need to increment the Lengths Array size!
const (
	istanbulConsensusMsg = 0x11
	// TODO:  Support sending multiple announce messages withone one message
	istanbulQueryEnodeMsg          = 0x12
	istanbulValEnodesShareMsg      = 0x13
	istanbulFwdMsg                 = 0x14
	istanbulDelegateSign           = 0x15
	istanbulVersionCertificatesMsg = 0x16
	istanbulEnodeCertificateMsg    = 0x17
	istanbulValidatorHandshakeMsg  = 0x18

	handshakeTimeout = 5 * time.Second
)

func (sb *Backend) isIstanbulMsg(msg p2p.Msg) bool {
	return msg.Code >= istanbulConsensusMsg && msg.Code <= istanbulValidatorHandshakeMsg
}

func (sb *Backend) isGossipedMsgCode(msgCode uint64) bool {
	return msgCode == istanbulQueryEnodeMsg || msgCode == istanbulVersionCertificatesMsg
}

type announceMsgHandler func(consensus.Peer, []byte) error

// HandleMsg implements consensus.Handler.HandleMsg
func (sb *Backend) HandleMsg(addr common.Address, msg p2p.Msg, peer consensus.Peer) (bool, error) {
	logger := sb.logger.New("func", "HandleMsg")
	sb.coreMu.Lock()
	defer sb.coreMu.Unlock()

	if sb.isIstanbulMsg(msg) {
		if (!sb.coreStarted && !sb.config.Proxy) && (msg.Code == istanbulConsensusMsg) {
			return true, istanbul.ErrStoppedEngine
		}

		var data []byte
		if err := msg.Decode(&data); err != nil {
			logger.Error("Failed to decode message payload", "err", err, "from", addr)
			return true, errDecodeFailed
		}

		if msg.Code == istanbulDelegateSign {
			if sb.shouldHandleDelegateSign() {
				go sb.delegateSignFeed.Send(istanbul.MessageEvent{Payload: data})
				return true, nil
			}

			return true, errors.New("No proxy or proxied validator found")
		}

		// Only use the recent messages and known messages cache for messages
		// that are gossiped
		if sb.isGossipedMsgCode(msg.Code) {
			hash := istanbul.RLPHash(data)

			// Mark peer's message
			ms, ok := sb.peerRecentMessages.Get(addr)
			var m *lru.ARCCache
			if ok {
				m, _ = ms.(*lru.ARCCache)
			} else {
				m, _ = lru.NewARC(inmemoryMessages)
				sb.peerRecentMessages.Add(addr, m)
			}
			m.Add(hash, true)

			// Mark self known message
			if _, ok := sb.selfRecentMessages.Get(hash); ok {
				return true, nil
			}
			sb.selfRecentMessages.Add(hash, true)
		}

		if msg.Code == istanbulConsensusMsg {
			err := sb.handleConsensusMsg(peer, data)
			return true, err
		} else if msg.Code == istanbulFwdMsg {
			err := sb.handleFwdMsg(peer, data)
			return true, err
		} else if announceHandlerFunc, ok := sb.istanbulAnnounceMsgHandlers[msg.Code]; ok { // Note that the valEnodeShare message is handled here as well
			go announceHandlerFunc(peer, data)
			return true, nil
		} else if msg.Code == istanbulValidatorHandshakeMsg {
			logger.Warn("Received unexpected Istanbul validator handshake message")
			return true, nil
		}

		// If we got here, then that means that there is an istanbul message type that we
		// don't handle, and hence a bug in the code.
		logger.Crit("Unhandled istanbul message type")
		return false, nil
	}
	return false, nil
}

// Handle an incoming consensus msg
func (sb *Backend) handleConsensusMsg(peer consensus.Peer, payload []byte) error {
	if sb.config.Proxy {
		// Verify that this message is not from the proxied peer
		if reflect.DeepEqual(peer, sb.proxiedPeer) {
			sb.logger.Warn("Got a consensus message from the proxied validator. Ignoring it", "from", peer.Node().ID())
			return nil
		}

		msg := new(istanbul.Message)

		// Verify that this message is created by a legitimate validator before forwarding.
		checkValidatorSignature := func(data []byte, sig []byte) (common.Address, error) {
			block := sb.currentBlock()
			valSet := sb.getValidators(block.Number().Uint64(), block.Hash())
			return istanbul.CheckValidatorSignature(valSet, data, sig)
		}
		if err := msg.FromPayload(payload, checkValidatorSignature); err != nil {
			sb.logger.Error("Got a consensus message signed by a non validator.", "err", err)
			return errNonValidatorMessage
		}

		// Need to forward the message to the proxied validator
		sb.logger.Trace("Forwarding consensus message to proxied validator", "from", peer.Node().ID())
		if sb.proxiedPeer != nil {
			go sb.proxiedPeer.Send(istanbulConsensusMsg, payload)
		}
	} else { // The case when this node is a validator
		go sb.istanbulEventMux.Post(istanbul.MessageEvent{
			Payload: payload,
		})
	}

	return nil
}

// Handle an incoming forward msg
func (sb *Backend) handleFwdMsg(peer consensus.Peer, payload []byte) error {
	// Ignore the message if this node it not a proxy
	if !sb.config.Proxy {
		sb.logger.Warn("Got a forward consensus message and this node is not a proxy. Ignoring it", "from", peer.Node().ID())
		return nil
	}

	// Verify that it's coming from the proxied peer
	if !reflect.DeepEqual(peer, sb.proxiedPeer) {
		sb.logger.Warn("Got a forward consensus message from a non proxied validator. Ignoring it", "from", peer.Node().ID())
		return nil
	}

	istMsg := new(istanbul.Message)

	// An Istanbul FwdMsg doesn't have a signature since it's coming from a trusted peer and
	// the wrapped message is already signed by the proxied validator.
	if err := istMsg.FromPayload(payload, nil); err != nil {
		sb.logger.Error("Failed to decode message from payload", "from", peer.Node().ID(), "err", err)
		return err
	}

	var fwdMsg *istanbul.ForwardMessage
	err := istMsg.Decode(&fwdMsg)
	if err != nil {
		sb.logger.Error("Failed to decode a ForwardMessage", "from", peer.Node().ID(), "err", err)
		return err
	}

	sb.logger.Trace("Forwarding a message", "msg code", fwdMsg.Code)
	go sb.Multicast(fwdMsg.DestAddresses, fwdMsg.Msg, fwdMsg.Code)
	return nil
}

func (sb *Backend) shouldHandleDelegateSign() bool {
	return sb.IsProxy() || sb.IsProxiedValidator()
}

// SubscribeNewDelegateSignEvent subscribes a channel to any new delegate sign messages
func (sb *Backend) SubscribeNewDelegateSignEvent(ch chan<- istanbul.MessageEvent) event.Subscription {
	return sb.delegateSignScope.Track(sb.delegateSignFeed.Subscribe(ch))
}

// SetBroadcaster implements consensus.Handler.SetBroadcaster
func (sb *Backend) SetBroadcaster(broadcaster consensus.Broadcaster) {
	sb.broadcaster = broadcaster
}

// SetP2PServer implements consensus.Handler.SetP2PServer
func (sb *Backend) SetP2PServer(p2pserver consensus.P2PServer) {
	sb.p2pserver = p2pserver
}

// This function is called by miner/worker.go whenever it's mainLoop gets a newWork event.
func (sb *Backend) NewWork() error {
	sb.logger.Debug("NewWork called, acquiring core lock", "func", "NewWork")

	sb.coreMu.RLock()
	defer sb.coreMu.RUnlock()
	if !sb.coreStarted {
		return istanbul.ErrStoppedEngine
	}

	sb.logger.Debug("Posting FinalCommittedEvent", "func", "NewWork")

	go sb.istanbulEventMux.Post(istanbul.FinalCommittedEvent{})
	return nil
}

// Maintain metrics around the *parent* of the supplied block.
// To figure out if this validator signed the parent block:
// * First check the grandparent's validator set. If not elected, it didn't.
// * Then, check the parent seal on the supplied (child) block.
// We cannot determine any specific info from the validators in the seal of
// the parent block, because different nodes circulate different versions.
// The bitmap of signed validators only becomes canonical when the child block is proposed.
func (sb *Backend) UpdateMetricsForParentOfBlock(child *types.Block) {
	sb.coreMu.RLock()
	defer sb.coreMu.RUnlock()

	// Check the parent is not the genesis block.
	number := child.Number().Uint64()
	if number <= 1 {
		return
	}

	childHeader := child.Header()
	parentHeader := sb.chain.GetHeader(childHeader.ParentHash, number-1)

	// Check validator in grandparent valset.
	gpValSet := sb.getValidators(number-2, parentHeader.ParentHash)
	gpValSetIndex, _ := gpValSet.GetByAddress(sb.ValidatorAddress())

	// Now check if in the "parent seal" (used for downtime calcs, on the child block)
	childExtra, err := types.ExtractIstanbulExtra(child.Header())
	if err != nil {
		return
	}

	// total possible signatures
	valSetSize := gpValSet.Size()
	sb.blocksValSetSizeGauge.Update(int64(valSetSize))

	// signatures present
	countInParentSeal := 0
	for i := 0; i < gpValSet.Size(); i++ {
		countInParentSeal += int(childExtra.ParentAggregatedSeal.Bitmap.Bit(i))
	}
	sb.blocksTotalSigsGauge.Update(int64(countInParentSeal))

	// cumulative count of rounds missed (i.e sequences not agreed on round=0)
	missedRounds := childExtra.ParentAggregatedSeal.Round.Int64()
	if missedRounds > 0 {
		sb.blocksTotalMissedRoundsMeter.Mark(missedRounds)
	}

	// elected?
	elected := gpValSetIndex >= 0
	if !elected {
		return
	}
	sb.blocksElectedMeter.Mark(1)

	// The following metrics are only tracked if the validator is elected.

	// proposed?
	if parentHeader.Coinbase == sb.ValidatorAddress() {
		sb.blocksElectedAndProposedMeter.Mark(1)
	}

	// signed, or missed?
	inParentSeal := childExtra.ParentAggregatedSeal.Bitmap.Bit(gpValSetIndex) != 0
	if inParentSeal {
		sb.blocksElectedAndSignedMeter.Mark(1)
	} else {
		sb.blocksElectedButNotSignedMeter.Mark(1)
		sb.logger.Warn("Elected but didn't sign block", "number", number-1, "address", sb.ValidatorAddress())
	}
}

// Actions triggered by a new block being added to the chain.
func (sb *Backend) NewChainHead(newBlock *types.Block) {

	sb.logger.Trace("Start NewChainHead", "number", newBlock.Number().Uint64())

	// Update metrics for whether we were elected and signed the parent of this block.
	sb.UpdateMetricsForParentOfBlock(newBlock)

	// If this is the last block of the epoch:
	// * Print an easy to find log message giving our address and whether we're elected in next epoch.
	// * if this is a proxy or a non proxied validator, refresh the validator enode table.
	if istanbul.IsLastBlockOfEpoch(newBlock.Number().Uint64(), sb.config.Epoch) {

		sb.coreMu.RLock()
		defer sb.coreMu.RUnlock()

		valSet := sb.getValidators(newBlock.Number().Uint64(), newBlock.Hash())
		valSetIndex, _ := valSet.GetByAddress(sb.ValidatorAddress())

		sb.logger.Info("Validator Election Results", "address", sb.ValidatorAddress(), "elected", valSetIndex >= 0, "number", newBlock.Number().Uint64())

		if sb.announceRunning {
			sb.logger.Trace("At end of epoch and going to refresh validator peers", "new_block_number", newBlock.Number().Uint64())
			if err := sb.RefreshValPeers(); err != nil {
				sb.logger.Warn("Error refreshing validator peers", "err", err)
			}
		}
	}

	sb.logger.Trace("End NewChainHead", "number", newBlock.Number().Uint64())
}

func (sb *Backend) RegisterPeer(peer consensus.Peer, isProxiedPeer bool) {
	// TODO - For added security, we may want the node keys of the proxied validators to be
	//        registered with the proxy, and verify that all newly connected proxied peer has
	//        the correct node key
	logger := sb.logger.New("func", "RegisterPeer")

	logger.Trace("RegisterPeer called", "peer", peer, "isProxiedPeer", isProxiedPeer)

	// Check to see if this connecting peer if a proxied validator
	if sb.config.Proxy && isProxiedPeer {
		sb.proxiedPeer = peer
	} else if sb.config.Proxied {
		if sb.proxyNode != nil && peer.Node().ID() == sb.proxyNode.node.ID() {
			sb.proxyNode.peer = peer
			// Share this node's enodeCertificate for the proxy to use for handshakes
			enodeCertificateMsg, err := sb.retrieveEnodeCertificateMsg()
			if err != nil {
				logger.Warn("Error getting enode certificate message", "err", err)
			} else if enodeCertificateMsg != nil {
				if err := sb.sendEnodeCertificateMsg(peer, enodeCertificateMsg); err != nil {
					logger.Warn("Error sending enode certificate message to proxy peer", "err", err)
				}
			}
			// Share the whole val enode table
			if err := sb.sendValEnodesShareMsg(); err != nil {
				logger.Warn("Error sending val enodes share message to proxy peer", "err", err)
			}
		} else {
			logger.Error("Unauthorized connected peer to the proxied validator", "peer", peer.Node().ID())
		}
	}

	if err := sb.sendVersionCertificateTable(peer); err != nil {
		logger.Info("Error sending all version certificates", "err", err)
	}
}

func (sb *Backend) UnregisterPeer(peer consensus.Peer, isProxiedPeer bool) {
	if sb.config.Proxy && isProxiedPeer && reflect.DeepEqual(sb.proxiedPeer, peer) {
		sb.proxiedPeer = nil
	} else if sb.config.Proxied {
		if sb.proxyNode != nil && peer.Node().ID() == sb.proxyNode.node.ID() {
			sb.proxyNode.peer = nil
		}
	}
}

// Handshake allows the initiating peer to identify itself as a validator
func (sb *Backend) Handshake(peer consensus.Peer) (bool, error) {
	// Only written to if there was a non-nil error when sending or receiving
	errCh := make(chan error)
	isValidatorCh := make(chan bool)

	sendHandshake := func() {
		var msg *istanbul.Message
		var err error
		peerIsValidator := peer.PurposeIsSet(p2p.ValidatorPurpose)
		if peerIsValidator {
			// msg may be nil
			msg, err = sb.retrieveEnodeCertificateMsg()
			if err != nil {
				errCh <- err
				return
			}
		}
		// Even if we decide not to identify ourselves,
		// send an empty message to complete the handshake
		if msg == nil {
			msg = &istanbul.Message{}
		}
		msgBytes, err := msg.Payload()
		if err != nil {
			errCh <- err
			return
		}
		err = peer.Send(istanbulValidatorHandshakeMsg, msgBytes)
		if err != nil {
			errCh <- err
		}
		isValidatorCh <- peerIsValidator
	}
	readHandshake := func() {
		isValidator, err := sb.readValidatorHandshakeMessage(peer)
		if err != nil {
			errCh <- err
			return
		}
		isValidatorCh <- isValidator
	}

	// Only the initating peer sends the message
	if peer.Inbound() {
		go readHandshake()
	} else {
		go sendHandshake()
	}

	timeout := time.NewTimer(handshakeTimeout)
	defer timeout.Stop()
	select {
	case err := <-errCh:
		return false, err
	case <-timeout.C:
		return false, p2p.DiscReadTimeout
	case isValidator := <-isValidatorCh:
		return isValidator, nil
	}
}

// readValidatorHandshakeMessage reads a validator handshake message.
// Returns if the peer is a validator or if an error occurred.
func (sb *Backend) readValidatorHandshakeMessage(peer consensus.Peer) (bool, error) {
	logger := sb.logger.New("func", "readValidatorHandshakeMessage")
	peerMsg, err := peer.ReadMsg()
	if err != nil {
		return false, err
	}
	if peerMsg.Code != istanbulValidatorHandshakeMsg {
		logger.Warn("Read incorrect message code", "code", peerMsg.Code)
		return false, errors.New("Incorrect message code")
	}

	var payload []byte
	if err := peerMsg.Decode(&payload); err != nil {
		return false, err
	}

	var msg istanbul.Message
	err = msg.FromPayload(payload, sb.verifyValidatorHandshakeMessage)
	if err != nil {
		return false, err
	}
	// If the Signature is empty, the peer has decided not to reveal its info
	if len(msg.Signature) == 0 {
		return false, nil
	}

	var enodeCertificate enodeCertificate
	err = rlp.DecodeBytes(msg.Msg, &enodeCertificate)
	if err != nil {
		return false, err
	}

	node, err := enode.ParseV4(enodeCertificate.EnodeURL)
	if err != nil {
		return false, err
	}

	// Ensure the node in the enodeCertificate matches the peer node
	if node.ID() != peer.Node().ID() {
		logger.Warn("Peer provided incorrect node ID in enodeCertificate", "enodeCertificate enode url", enodeCertificate.EnodeURL, "peer enode url", peer.Node().URLv4())
		return false, errors.New("Incorrect node in enodeCertificate")
	}

	// Check if the peer is within the validator conn set.
	validatorConnSet := sb.retrieveCachedValidatorConnSet()
	// If no set has ever been cached, update it and try again. This is an expensive
	// operation and risks the handshake timing out, but will happen at most once
	// and is unlikely to occur.
	if validatorConnSet == nil {
		if err := sb.updateCachedValidatorConnSet(); err != nil {
			logger.Trace("Error updating cached validator conn set")
			return false, err
		}
		validatorConnSet = sb.retrieveCachedValidatorConnSet()
	}
	if !validatorConnSet[sb.ValidatorAddress()] {
		logger.Trace("This validator is not in the validator conn set")
		return false, nil
	}
	if !validatorConnSet[msg.Address] {
		logger.Debug("Received a validator handshake message from peer not in the validator conn set", "msg.Address", msg.Address)
		return false, nil
	}

	// If the enodeCertificate message is too old, we don't count the msg as valid
	// An error is given if the entry doesn't exist, so we ignore the error.
	knownVersion, err := sb.valEnodeTable.GetVersionFromAddress(msg.Address)
	if err == nil && enodeCertificate.Version < knownVersion {
		logger.Debug("Received a validator handshake message with an old version", "received version", enodeCertificate.Version, "known version", knownVersion)
		return false, nil
	}

	// Forward this message to the proxied validator if this is a proxy.
	// We leave the validator to determine if the proxy should add this node
	// to its val enode table, which occurs if the proxied validator sends back
	// the enode certificate to this proxy.
	if sb.config.Proxy {
		if sb.proxiedPeer != nil {
			go sb.sendEnodeCertificateMsg(sb.proxiedPeer, &msg)
		}
		return false, nil
	}

	// By this point, this node and the peer are both validators and we update
	// our val enode table accordingly. Upsert will only use this entry if the version is new
	err = sb.valEnodeTable.UpsertVersionAndEnode([]*vet.AddressEntry{{Address: msg.Address, Node: node, Version: enodeCertificate.Version}})
	if err != nil {
		return false, err
	}
	return true, nil
}

// verifyValidatorHandshakeMessage allows messages that are not signed to still be
// decoded in case the peer has decided not to identify itself with the validator handshake
// message
func (sb *Backend) verifyValidatorHandshakeMessage(data []byte, sig []byte) (common.Address, error) {
	// If the message was not signed, allow it to still be decoded.
	// A later check will verify if the signature was empty or not.
	if len(sig) == 0 {
		return common.ZeroAddress, nil
	}
	return istanbul.GetSignatureAddress(data, sig)
}
