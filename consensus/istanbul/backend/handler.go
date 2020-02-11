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
	"io"
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
)

// If you want to add a code, you need to increment the Lengths Array size!
const (
	istanbulConsensusMsg = 0x11
	// TODO:  Support sending multiple announce messages withone one message
	istanbulAnnounceMsg            = 0x12
	istanbulValEnodesShareMsg      = 0x13
	istanbulFwdMsg                 = 0x14
	istanbulDelegateSign           = 0x15
	istanbulGetAnnouncesMsg        = 0x16
	istanbulGetAnnounceVersionsMsg = 0x17
	istanbulAnnounceVersionsMsg    = 0x18
	istanbulDirectAnnounceMsg      = 0x19
	istanbulValidatorProofMsg      = 0x1a
)

func (sb *Backend) isIstanbulMsg(msg p2p.Msg) bool {
	return msg.Code >= istanbulConsensusMsg && msg.Code <= istanbulValidatorProofMsg
}

type announceMsgHandler func(consensus.Peer, []byte) error

// HandleMsg implements consensus.Handler.HandleMsg
func (sb *Backend) HandleMsg(addr common.Address, msg p2p.Msg, peer consensus.Peer) (bool, error) {
	sb.coreMu.Lock()
	defer sb.coreMu.Unlock()

	logger := sb.logger.New("func", "HandleMsg", "m", msg, "peer", peer)

	logger.Trace("Handling a message")

	if sb.isIstanbulMsg(msg) {
		if (!sb.coreStarted && !sb.config.Proxy) && (msg.Code == istanbulConsensusMsg) {
			return true, istanbul.ErrStoppedEngine
		}

		var data []byte
		if err := msg.Decode(&data); err != nil {
			logger.Error("Failed to decode message payload", "err", err)
			return true, errDecodeFailed
		}

		if msg.Code == istanbulDelegateSign {
			if sb.shouldHandleDelegateSign() {
				go sb.delegateSignFeed.Send(istanbul.MessageEvent{Payload: data})
				return true, nil
			}

			return true, errors.New("No proxy or proxied validator found")
		}

		// Only use the recent messages and known messages cache for the
		// Announce message.  That is the only message that is gossiped.
		if msg.Code == istanbulAnnounceMsg {
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
			sb.logger.Warn("Got a consensus message from the proxied validator.  Ignoring it")
			return nil
		}

		// Need to forward the message to the proxied validator
		sb.logger.Trace("Forwarding consensus message to proxied validator")
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
		sb.logger.Warn("Got a forward consensus message and this node is not a proxy.  Ignoring it")
		return nil
	}

	// Verify that it's coming from the proxied peer
	if !reflect.DeepEqual(peer, sb.proxiedPeer) {
		sb.logger.Warn("Got a forward consensus message from a non proxied valiator.  Ignoring it")
		return nil
	}

	istMsg := new(istanbul.Message)

	// An Istanbul FwdMsg doesn't have a signature since it's coming from a trusted peer and
	// the wrapped message is already signed by the proxied validator.
	if err := istMsg.FromPayload(payload, nil); err != nil {
		sb.logger.Error("Failed to decode message from payload", "err", err)
		return err
	}

	var fwdMsg *istanbul.ForwardMessage
	err := istMsg.Decode(&fwdMsg)
	if err != nil {
		sb.logger.Error("Failed to decode a ForwardMessage", "err", err)
		return err
	}

	sb.logger.Debug("Forwarding a consensus message")
	go sb.Multicast(fwdMsg.DestAddresses, fwdMsg.Msg, istanbulConsensusMsg)
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
	sb.coreMu.RLock()
	defer sb.coreMu.RUnlock()
	if !sb.coreStarted {
		return istanbul.ErrStoppedEngine
	}

	go sb.istanbulEventMux.Post(istanbul.FinalCommittedEvent{})
	return nil
}

// This function is called by all nodes.
// At the end of each epoch, this function will
//    1)  Output if it is or isn't an elected validator if it has mining turned on.
//    2)  Refresh the validator connections if it's a proxy or non proxied validator
func (sb *Backend) NewChainHead(newBlock *types.Block) {
	if istanbul.IsLastBlockOfEpoch(newBlock.Number().Uint64(), sb.config.Epoch) {
		sb.coreMu.RLock()
		defer sb.coreMu.RUnlock()

		valset := sb.getValidators(newBlock.Number().Uint64(), newBlock.Hash())

		// Output whether this validator was or wasn't elected for the
		// new epoch's validator set
		if sb.coreStarted {
			_, val := valset.GetByAddress(sb.ValidatorAddress())
			sb.logger.Info("Validator Election Results", "address", sb.ValidatorAddress(), "elected", (val != nil), "number", newBlock.Number().Uint64())
		}

		// If this is a proxy or a non proxied validator and a
		// new epoch just started, then refresh the validator enode table
		sb.logger.Trace("At end of epoch and going to refresh validator peers", "new block number", newBlock.Number().Uint64())
		sb.RefreshValPeers(valset)
	}
}

func (sb *Backend) RegisterPeer(peer consensus.Peer, isProxiedPeer bool) {
	// TODO - For added security, we may want the node keys of the proxied validators to be
	//        registered with the proxy, and verify that all newly connected proxied peer has
	//        the correct node key

	sb.logger.Trace("RegisterPeer called", "peer", peer, "isProxiedPeer", isProxiedPeer)

	// Check to see if this connecting peer if a proxied validator
	if sb.config.Proxy && isProxiedPeer {
		sb.proxiedPeer = peer
	} else if sb.config.Proxied {
		if sb.proxyNode != nil && peer.Node().ID() == sb.proxyNode.node.ID() {
			sb.proxyNode.peer = peer
			go sb.sendDirectAnnounce(peer)
		} else {
			sb.logger.Error("Unauthorized connected peer to the proxied validator", "peer node", peer.Node().String())
		}
	}

	if peer.Version() >= 65 {
		sb.sendGetAnnounceVersions(peer)
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

type directAnnounce struct {
	Node string
	Version uint
}

// ==============================================
//
// define the functions that needs to be provided for rlp Encoder/Decoder.

// EncodeRLP serializes ar into the Ethereum RLP format.
func (da *directAnnounce) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, []interface{}{da.Node, da.Version})
}

// DecodeRLP implements rlp.Decoder, and load the ar fields from a RLP stream.
func (da *directAnnounce) DecodeRLP(s *rlp.Stream) error {
	var msg struct {
		Node string
		Version uint
	}

	if err := s.Decode(&msg); err != nil {
		return err
	}
	da.Node, da.Version = msg.Node, msg.Version
	return nil
}

func (sb *Backend) getValidatorProofMessage(peer consensus.Peer) (*istanbul.Message, error) {
	// If the peer is not a known validator, we do not want to give up extra
	// information about the current node. If there is an error retrieving the
	// peer from the val enode table, we do not know the peer to be another validator.
	if _, err := sb.valEnodeTable.GetAddressFromNodeID(peer.Node().ID()); err != nil {
		return nil, nil
	}

	if sb.config.Proxy {
		// if this proxy has been disconnected from its validator for any reason,
		// don't send a proof message
		if sb.proxyDirectAnnounceMsg != nil && sb.proxiedPeer != nil {
			// Make a copy of the proxyDirectAnnounceMsg for thread safety
			sb.proxyDirectAnnounceMsgMu.RLock()
			defer sb.proxyDirectAnnounceMsgMu.RUnlock()
			return sb.proxyDirectAnnounceMsg.Copy(), nil
		}
	} else {
		return sb.generateDirectAnnounce()
	}
	return nil, nil
}

// Handshake allows this node to identify itself to the peer as a validator and vice versa
func (sb *Backend) Handshake(peer consensus.Peer) (bool, error) {
	sb.logger.Warn("woo in sb handshake!")

	errCh := make(chan error, 4)
	isValidatorCh := make(chan bool, 1)

	go func() {
		validatorProofMessage, err := sb.getValidatorProofMessage(peer)
		errCh <- err
		if err != nil {
			return
		}
		// An empty validator proof message will not result in a special case when peering
		if validatorProofMessage == nil {
			validatorProofMessage = &istanbul.Message{}
		}
		sb.logger.Warn("our val proof", "Address", validatorProofMessage.Address, "Signature", validatorProofMessage.Signature)
		msgBytes, err := validatorProofMessage.Payload()
		errCh <- err
		if err != nil {
			return
		}
		sb.logger.Warn("sending istanbulValidatorProofMsg", "msgBytes", msgBytes)
		errCh <- peer.Send(istanbulValidatorProofMsg, msgBytes)
	}()
	go func() {
		isValidator, err := sb.readValidatorProofMessage(peer)
		errCh <- err
		isValidatorCh <- isValidator
	}()

	timeout := time.NewTimer(time.Minute / 10.0)
	defer timeout.Stop()
	// We write to errCh four times unless if the timeout is triggered
	for i := 0; i < 4; i++ {
		select {
		case err := <-errCh:
			if err != nil {
				sb.logger.Warn("Err :(", "err", err)
				return false, err
			}
		case <-timeout.C:
			sb.logger.Warn("In istanbul handshake DiscReadTimeout")
			return false, p2p.DiscReadTimeout
		}
	}
	return <-isValidatorCh, nil
}

func (sb *Backend) verifyValidatorProofMessage(data []byte, sig []byte) (common.Address, error) {
	// If the message was not signed, allow it to still be decoded.
	// A later check will verify if the address is in the validator set, which
	// will fail.
	if len(sig) == 0 {
		return common.ZeroAddress, nil
	}
	sb.logger.Warn("verifyValidatorProofMessage", "data", data, "sig", sig)
	return istanbul.GetSignatureAddress(data, sig)
}

// readValidatorProofMessage reads a validator proof message as a part of the
// istanbul handshake. Returns if the peer is a validator.
func (sb *Backend) readValidatorProofMessage(peer consensus.Peer) (bool, error) {
	msg, err := peer.ReadMsg()
	if err != nil {
		return false, err
	}
	sb.logger.Warn("After ReadMsg()", "msg", msg)
	var payload []byte
	if err := msg.Decode(&payload); err != nil {
		return false, err
	}
	sb.logger.Warn("After decode", "payload", payload)
	var validatorProofMessage istanbul.Message
	err = validatorProofMessage.FromPayload(payload, sb.verifyValidatorProofMessage)
	if err != nil {
		return false, err
	}
	sb.logger.Warn("Before validatorProofMessage.Msg len check", "len(validatorProofMessage.Msg)", len(validatorProofMessage.Msg))
	if len(validatorProofMessage.Msg) == 0 {
		return false, nil
	}
	sb.logger.Warn("After FromPayload", "validatorProofMessage", validatorProofMessage)
	var directAnnounce directAnnounce
	err = rlp.DecodeBytes(validatorProofMessage.Msg, &directAnnounce)
	if err != nil {
		return false, err
	}

	sb.logger.Warn("validatorProofMessage decoded", "address", validatorProofMessage.Address.String(), "signature", string(validatorProofMessage.Signature), "node", directAnnounce.Node, "version", directAnnounce.Version)
	// do validator proof check

	// Check if the sender is within the registered/elected valset
	regAndActiveVals, err := sb.retrieveRegisteredAndElectedValidators()
	if err != nil {
		sb.logger.Trace("Error in retrieving registered/elected valset", "err", err)
		return false, err
	}

	if !regAndActiveVals[validatorProofMessage.Address] {
		// not a validator
		return false, nil
	}

	node, err := enode.ParseV4(directAnnounce.Node)
	if err != nil {
		return false, err
	}

	err = sb.valEnodeTable.Upsert(map[common.Address]*vet.AddressEntry{validatorProofMessage.Address: {Node: node, Version: directAnnounce.Version}})
	if err != nil {
		return false, err
	}

	return true, nil
}
