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

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/consensus/istanbul"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/event"
	"github.com/ethereum/go-ethereum/p2p"
	lru "github.com/hashicorp/golang-lru"
)

const (
	istanbulConsensusMsg      = 0x11
	istanbulAnnounceMsg       = 0x12
	istanbulValEnodesShareMsg = 0x13
	istanbulFwdMsg            = 0x14
    istanbulDelegateSign     = 0x15
)

var (
	// errDecodeFailed is returned when decode message fails
	errDecodeFailed = errors.New("fail to decode istanbul message")
)

// Protocol implements consensus.Engine.Protocol
func (sb *Backend) Protocol() consensus.Protocol {
	return consensus.Protocol{
		Name:     "istanbul",
		Versions: []uint{64},
		Lengths:  []uint64{21},
		Primary:  true,
	}
}

func (sb *Backend) isIstanbulMsg(msg p2p.Msg) bool {
	return (msg.Code == istanbulConsensusMsg) || (msg.Code == istanbulAnnounceMsg) || (msg.Code == istanbulValEnodesShareMsg) || (msg.Code == istanbulFwdMsg)
}

// HandleMsg implements consensus.Handler.HandleMsg
func (sb *Backend) HandleMsg(addr common.Address, msg p2p.Msg, peer consensus.Peer) (bool, error) {
	sb.coreMu.Lock()
	defer sb.coreMu.Unlock()

	sb.logger.Trace("HandleMsg called", "address", addr, "msg", msg, "peer.Node()", peer.Node())

	if sb.isIstanbulMsg(msg) {
		if (!sb.coreStarted && !sb.config.Proxy) && (msg.Code == istanbulConsensusMsg) {
			return true, istanbul.ErrStoppedEngine
		}

		var data []byte
		if err := msg.Decode(&data); err != nil {
			sb.logger.Error("Failed to decode message payload", "msg", msg)
			return true, errDecodeFailed
		}

		if msg.Code == istanbulDelegateSign {
			if sb.config.Proxy {
				// got a signed message from the validator
				sb.logger.Warn("woohoo this is the proxy, got a signed message")
				go sb.delegateSignFeed.Send(istanbul.MessageEvent{Payload: data})
			} else {
				// got a message to sign from the proxy
				sb.logger.Warn("woohoo this is the proxied validator, will send a signed message")
				go sb.delegateSignFeed.Send(istanbul.MessageEvent{Payload: data})
				// @trevor - seems I need to do this to get proxyNode.peer.Send working inside of ethstats... weird
				// sb.proxyNode.peer.Send(istanbulDelegateSign, "")
			}

			return true, nil
		}

		hash := istanbul.RLPHash(data)

		// Mark peer's message
		ms, ok := sb.recentMessages.Get(addr)
		var m *lru.ARCCache
		if ok {
			m, _ = ms.(*lru.ARCCache)
		} else {
			m, _ = lru.NewARC(inmemoryMessages)
			sb.recentMessages.Add(addr, m)
		}
		m.Add(hash, true)

		// Mark self known message
		if _, ok := sb.knownMessages.Get(hash); ok {
			return true, nil
		}
		sb.knownMessages.Add(hash, true)

		if msg.Code == istanbulConsensusMsg {
			err := sb.handleConsensusMsg(peer, data)
			return true, err
		} else if msg.Code == istanbulFwdMsg {
			err := sb.handleFwdMsg(peer, data)
			return true, err
		} else if msg.Code == istanbulAnnounceMsg {
			go sb.handleIstAnnounce(data)
		} else if msg.Code == istanbulValEnodesShareMsg {
			go sb.handleValEnodesShareMsg(data)
		}

		return true, nil
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
		sb.logger.Debug("Forwarding consensus message to proxied validator")
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

    if msg.Code == istanbulDelegateSign {
        sb.logger.Warn("woohoo! got istanbulDelegateSign message", "msg", msg, "data string", string(data))

        if sb.config.Proxy {
            // got a signed message from the validator
            sb.logger.Warn("woohoo this is the proxy, got a signed message")
            go sb.delegateSignFeed.Send(istanbul.MessageEvent{ Payload: data })
        } else {
            // assumes it's the proxied validator
            sb.logger.Warn("woohoo this is the proxied validator, will send a signed message")
            sb.proxyNode.peer.Send(istanbulDelegateSign, "signed woohoooo")
        }

        return true, nil
    }
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
	go sb.Gossip(fwdMsg.DestAddresses, fwdMsg.Msg, istanbulConsensusMsg, false)
	return nil
}

// SubscribeNewDelegateSignEvent subscribes a channel to any new delegate sign messages
func (sb *Backend) SubscribeNewDelegateSignEvent(ch chan<- istanbul.MessageEvent) event.Subscription {
	return sb.delegateSignFeed.Subscribe(ch)
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
			if _, val := valset.GetByAddress(sb.ValidatorAddress()); val != nil {
				sb.logger.Info("Validators Election Results: Node IN ValidatorSet")
			} else {
				sb.logger.Info("Validators Election Results: Node OUT ValidatorSet")
			}

			sb.newEpochCh <- struct{}{}
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
	if sb.config.Proxy && isProxiedPeer {
		sb.proxiedPeer = peer
	} else if sb.config.Proxied {
		if sb.proxyNode != nil && peer.Node().ID() == sb.proxyNode.node.ID() {
			sb.proxyNode.peer = peer
		} else {
			sb.logger.Error("Unauthorized connected peer to the proxied validator", "peer node", peer.Node().String())
		}
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
