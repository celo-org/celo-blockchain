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
	"github.com/ethereum/go-ethereum/p2p"
	lru "github.com/hashicorp/golang-lru"
)

const (
	istanbulMsg              = 0x11
	istanbulAnnounceMsg      = 0x12
	istanbulValEnodeShareMsg = 0x13
	istanbulFwdMsg           = 0x14
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

// HandleMsg implements consensus.Handler.HandleMsg
func (sb *Backend) HandleMsg(addr common.Address, msg p2p.Msg, peer consensus.Peer) (bool, error) {
	sb.coreMu.Lock()
	defer sb.coreMu.Unlock()

	sb.logger.Trace("HandleMsg called", "address", addr, "msg", msg, "peer.Node()", peer.Node())

	if (msg.Code == istanbulMsg) || (msg.Code == istanbulAnnounceMsg) || (msg.Code == istanbulValEnodeShareMsg) || (msg.Code == istanbulFwdMsg) {
		if (!sb.coreStarted && !sb.config.Proxy) && (msg.Code == istanbulMsg) {
			return true, istanbul.ErrStoppedEngine
		}

		var data []byte
		if err := msg.Decode(&data); err != nil {
			sb.logger.Error("Failed to decode message payload", "msg", msg)
			return true, errDecodeFailed
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

		if msg.Code == istanbulMsg {
			if sb.config.Proxy {
				// Verify that this message is not from the proxied peer
				if reflect.DeepEqual(peer, sb.proxiedPeer) {
					sb.logger.Debug("Got a consensus message from the proxied valiator.  Ignoring it")
					return true, nil
				}

				// Need to forward the message to the proxied validator
				sb.logger.Debug("Forwarding consensus message to proxied validator")
				if sb.proxiedPeer != nil {
					go sb.proxiedPeer.Send(msg.Code, data)
				}
			} else { // The case when this node is a validator
				go sb.istanbulEventMux.Post(istanbul.MessageEvent{
					Payload: data,
				})
			}
		} else if msg.Code == istanbulFwdMsg {
			// Ignore the message if this node it not a proxy
			if !sb.config.Proxy {
				sb.logger.Debug("Got a forward consensus message and this node is not a proxy.  Ignoring it")
				return true, nil
			}

			// Verify that it's coming from the proxied peer
			if !reflect.DeepEqual(peer, sb.proxiedPeer) {
				sb.logger.Debug("Got a forward consensus message from a non proxied valiator.  Ignoring it")
				return true, nil
			}

			istMsg := new(istanbul.Message)

			// An Istanbul FwdMsg doesn't have a signature since it's coming from a trusted peer and
			// the wrapped message is already signed by the proxied validator.
			if err := istMsg.FromPayload(data, nil); err != nil {
				sb.logger.Error("Failed to decode message from payload", "err", err)
				return true, err
			}

			var fwdMsg *istanbul.ForwardMessage
			err := istMsg.Decode(&fwdMsg)
			if err != nil {
				sb.logger.Error("Failed to decode a ForwardMessage", "err", err)
				return true, err
			}

			sb.logger.Debug("Forwarding a consensus message")
			go sb.Gossip(fwdMsg.DestAddresses, fwdMsg.Msg, istanbulMsg, false)
		} else if msg.Code == istanbulAnnounceMsg {
			go sb.handleIstAnnounce(data)
		} else if msg.Code == istanbulValEnodeShareMsg {
			go sb.handleValEnodeShareMsg(data)
		}

		return true, nil
	}
	return false, nil
}

// SetBroadcaster implements consensus.Handler.SetBroadcaster
func (sb *Backend) SetBroadcaster(broadcaster consensus.Broadcaster) {
	sb.broadcaster = broadcaster
}

// SetP2PServer implements consensus.Handler.SetP2PServer
func (sb *Backend) SetP2PServer(p2pserver consensus.P2PServer) {
	sb.p2pserver = p2pserver
	sb.valEnodeTable.p2pserver = p2pserver
}

// This function is called by worker.go whenever it gets a newWork event.
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

			sb.newEpoch <- struct{}{}
		}

		// If this is a proxy or a non proxied validator and a
		// new epoch just started, then refresh the validator enode table
		if sb.config.Proxy || (sb.coreStarted && !sb.config.Proxied) {
			sb.logger.Trace("At end of epoch and going to refresh validator peers if not proxied", "new block number", newBlock.Number().Uint64())
			sb.RefreshValPeers(valset)
		}
	}
}

func (sb *Backend) RegisterPeer(peer consensus.Peer, isProxiedPeer bool) {
	// TODO - For added security, we may want the node keys of the proxied validators to be
	//        registered with the proxy, and verify that all newly connected proxied peer has
	//        the correct node key
	if sb.config.Proxy && isProxiedPeer {
		sb.proxiedPeer = peer
	} else if sb.config.Proxied {
		if peer.Node().ID() == sb.proxyNode.node.ID() {
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
