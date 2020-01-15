// Copyright 2017 The Celo Authors
// This file is part of the celo library.
//
// The celo library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The celo library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the celo library. If not, see <http://www.gnu.org/licenses/>.

package backend

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"io"
	mrand "math/rand"
	"sort"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/consensus/istanbul"
	vet "github.com/ethereum/go-ethereum/consensus/istanbul/backend/internal/enodes"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/rlp"
)

// ==============================================
//
// define the constant, types, and function for the sendAnnounce thread

type AnnounceFrequencyState int

const (
	// In this state, send out an announce message every 1 minute until the first peer is established
	HighFreqBeforeFirstPeerState AnnounceFrequencyState = iota

	// In this state, send out an announce message every 1 minute for the first 10 announce messages after the first peer is established.
	// This is on the assumption that when this node first establishes a peer, the p2p network that this node is in may
	// be partitioned with the broader p2p network. We want to give that p2p network some time to connect to the broader p2p network.
	HighFreqAfterFirstPeerState

	// In this state, send out an announce message every 10 minutes
	LowFreqState
)

const (
	HighFreqTickerDuration = 1 * time.Minute
	LowFreqTickerDuration  = 10 * time.Minute
)

// The announceThread thread function
// It will generate and gossip it's announce message periodically.
// It will also check with it's peers for it's announce message versions, and request any updated ones if necessary.
func (sb *Backend) announceThread() {
	sb.announceWg.Add(1)
	defer sb.announceWg.Done()

	// Send out an announce message when this thread starts
	go sb.gossipAnnounce()

	// Set the initial states
	announceThreadState := HighFreqBeforeFirstPeerState
	currentTickerDuration := HighFreqTickerDuration
	ticker := time.NewTicker(currentTickerDuration)
	numSentMsgsInHighFreqAfterFirstPeerState := 0

	for {
		select {
		case <-sb.newEpochCh:
			go sb.gossipAnnounce()
			go sb.checkPeersAnnounceVersions()

		case <-ticker.C:
			switch announceThreadState {
			case HighFreqBeforeFirstPeerState:
				{
					if len(sb.broadcaster.FindPeers(nil, p2p.AnyPurpose)) > 0 {
						announceThreadState = HighFreqAfterFirstPeerState
					}
				}

			case HighFreqAfterFirstPeerState:
				{
					if numSentMsgsInHighFreqAfterFirstPeerState >= 10 {
						announceThreadState = LowFreqState
					}
					numSentMsgsInHighFreqAfterFirstPeerState += 1
				}

			case LowFreqState:
				{
					if currentTickerDuration != LowFreqTickerDuration {
						// Reset the ticker
						currentTickerDuration = LowFreqTickerDuration
						ticker.Stop()
						ticker = time.NewTicker(currentTickerDuration)
					}
				}
			}

			go sb.gossipAnnounce()
			go sb.checkPeersAnnounceVersions()

		case <-sb.announceQuit:
			ticker.Stop()
			return
		}
	}
}

// ===============================================================
//
// define the IstanbulAnnounce messge format, the AnnounceMsgCache entries, the announce send function (both the gossip version and the "retrieve from cache" version), and the announce get function

// EnodeURL ciphertext encrypted with the public key associated with the DecryptorAddress field
type encryptedEnode struct {
	DecrypterAddress  common.Address
	EncryptedEnodeURL []byte
}

// EncodeRLP serializes ee into the Ethereum RLP format.
func (ee *encryptedEnode) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, []interface{}{ee.DecrypterAddress, ee.EncryptedEnodeURL})
}

// DecodeRLP implements rlp.Decoder, and load the ee fields from a RLP stream.
func (ee *encryptedEnode) DecodeRLP(s *rlp.Stream) error {
	var msg struct {
		DecrypterAddress  common.Address
		EncryptedEnodeURL []byte
	}

	if err := s.Decode(&msg); err != nil {
		return err
	}
	ee.DecrypterAddress, ee.EncryptedEnodeURL = msg.DecrypterAddress, msg.EncryptedEnodeURL
	return nil
}

func (ee *encryptedEnode) String() string {
	return fmt.Sprintf("{DecrypterAddress: %s, EncryptedEnodeURL length: %d}", ee.DecrypterAddress.String(), len(ee.EncryptedEnodeURL))
}

// The set of encrypted enodes of ValAddress's enodeURL.  There is an encrypted enode for each registered validator.
// The RLP encoding of this struct is the payload for the IstanbulAnnounce message.
type valEncryptedEnodes struct {
	ValAddress      common.Address
	EncryptedEnodes []*encryptedEnode
	EnodeURLHash    common.Hash
	Timestamp       uint
}

func (vee *valEncryptedEnodes) String() string {
	return fmt.Sprintf("{ValAddress: %s, Timestamp: %v, EnodeURLHash: %v, AnnounceRecords: %v}", vee.ValAddress.String(), vee.Timestamp, vee.EnodeURLHash.Hex(), vee.EncryptedEnodes)
}

// EncodeRLP serializes ad into the Ethereum RLP format.
func (vee *valEncryptedEnodes) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, []interface{}{vee.ValAddress, vee.EncryptedEnodes, vee.EnodeURLHash, vee.Timestamp})
}

// DecodeRLP implements rlp.Decoder, and load the ad fields from a RLP stream.
func (vee *valEncryptedEnodes) DecodeRLP(s *rlp.Stream) error {
	var msg struct {
		ValAddress      common.Address
		EncryptedEnodes []*encryptedEnode
		EnodeURLHash    common.Hash
		Timestamp       uint
	}

	if err := s.Decode(&msg); err != nil {
		return err
	}
	vee.ValAddress, vee.EncryptedEnodes, vee.EnodeURLHash, vee.Timestamp = msg.ValAddress, msg.EncryptedEnodes, msg.EnodeURLHash, msg.Timestamp
	return nil
}

// Define the announce msg cache entry.
// The latest version (dictated by the announce's timestamp field) will always be cached
// by this node.  Note that it will only cache the announce msgs from registered or elected
// validators.
type announceMsgCachedEntry struct {
	MsgTimestamp uint
	MsgPayload   []byte
}

// This function will request announce messages from a set of validator addresses from a peer
func (sb *Backend) sendGetAnnounces(peer consensus.Peer, valAddresses []common.Address) error {
	logger := sb.logger.New("func", "sendGetAnnounces")
	valAddressesBytes, err := rlp.EncodeToBytes(valAddresses)
	if err != nil {
		logger.Error("Error encoding valAddresses", "valAddresses", common.ConvertToStringSlice(valAddresses), "err", err)
		return err
	}

	go peer.Send(istanbulGetAnnouncesMsg, valAddressesBytes)
	return nil
}

// This function will reply from a GetAnnounce message.  It will retrieve the requested announce messages
// from it's announceMsgCache.
func (sb *Backend) handleGetAnnounces(peer consensus.Peer, data []byte) error {
	logger := sb.logger.New("func", "handleGetAnnounces")
	var valAddresses []common.Address

	if err := rlp.DecodeBytes(data, &valAddresses); err != nil {
		logger.Error("Error in decoding valAddresses", "err", err)
		return err
	}

	sb.cachedAnnounceMsgsMu.RLock()
	defer sb.cachedAnnounceMsgsMu.RUnlock()
	// TODO:  Add support for the AnnounceMsg to contain multiple announce messages within it's payload
	for _, valAddress := range valAddresses {
		if payload, ok := sb.cachedAnnounceMsgs[valAddress]; ok {
			logger.Trace("Sending announce msg", "peer", peer, "valAddress", valAddress)
			go peer.Send(istanbulAnnounceMsg, payload)
		}
	}

	return nil
}

// This function will generate the lastest announce msg from this node (as opposed to retrieving from the announceMsgCache) and then broadcast it to it's peers,
// which should then gossip the announce msg message throughout the p2p network (since this announce msg's timestamp should be the latest among all of this
// validator's previous announce msgs).
func (sb *Backend) gossipAnnounce() error {
	logger := sb.logger.New("func", "gossipAnnounce")
	istMsg, err := sb.generateAnnounce()
	if err != nil {
		return err
	}

	if istMsg == nil {
		return nil
	}

	// Sign the announce message
	if err := istMsg.Sign(sb.Sign); err != nil {
		logger.Error("Error in signing an Istanbul Announce Message", "AnnounceMsg", istMsg.String(), "err", err)
		return err
	}

	// Convert to payload
	payload, err := istMsg.Payload()
	if err != nil {
		logger.Error("Error in converting Istanbul Announce Message to payload", "AnnounceMsg", istMsg.String(), "err", err)
		return err
	}

	sb.Gossip(nil, payload, istanbulAnnounceMsg, true)

	return nil
}

// This function is a helper function for gossipAnnounce.  It will create the latest announce msg for this node.
func (sb *Backend) generateAnnounce() (*istanbul.Message, error) {
	logger := sb.logger.New("func", "generateAnnounce")
	var enodeUrl string
	if sb.config.Proxied {
		if sb.proxyNode != nil {
			enodeUrl = sb.proxyNode.externalNode.String()
		} else {
			logger.Error("Proxied node is not connected to a proxy")
			return nil, errNoProxyConnection
		}
	} else {
		enodeUrl = sb.p2pserver.Self().String()
	}

	// If the message is not within the registered validator set, then ignore it
	regAndActiveVals, err := sb.retrieveActiveAndRegisteredValidators()
	if err != nil {
		return nil, err
	}

	encryptedEnodes := make([]*encryptedEnode, 0, len(regAndActiveVals))
	for addr := range regAndActiveVals {
		// TODO - Need to encrypt using the remote validator's validator key
		encryptedEnodes = append(encryptedEnodes, &encryptedEnode{DecrypterAddress: addr, EncryptedEnodeURL: []byte(enodeUrl)})
	}

	announcePayload := &valEncryptedEnodes{
		ValAddress:      sb.Address(),
		EncryptedEnodes: encryptedEnodes,
		EnodeURLHash:    istanbul.RLPHash(enodeUrl),
		// Unix() returns a int64, but we need a uint for the golang rlp encoding implmentation. Warning: This timestamp value will be truncated in 2106.
		Timestamp: uint(time.Now().Unix()),
	}

	announceBytes, err := rlp.EncodeToBytes(announcePayload)
	if err != nil {
		logger.Error("Error encoding announce payload for an Announce message", "AnnouncePayload", announcePayload.String(), "err", err)
		return nil, err
	}

	msg := &istanbul.Message{
		Code:      istanbulAnnounceMsg,
		Msg:       announceBytes,
		Address:   sb.Address(),
		Signature: []byte{},
	}

	logger.Debug("Generated an announce message", "IstanbulMsg", msg.String(), "AnnouncePayload", announcePayload.String())

	return msg, nil
}

// This function will handle an announce message.
func (sb *Backend) handleAnnounce(payload []byte) error {
	logger := sb.logger.New("func", "handleAnnounce")

	msg := new(istanbul.Message)

	// Decode message
	err := msg.FromPayload(payload, istanbul.GetSignatureAddress)
	if err != nil {
		logger.Error("Error in decoding received Istanbul Announce message", "err", err, "payload", hex.EncodeToString(payload))
		return err
	}
	logger.Trace("Handling an IstanbulAnnounce message", "from", msg.Address)

	// If the message is not within the registered validator set, then ignore it
	regAndActiveVals, err := sb.retrieveActiveAndRegisteredValidators()
	if err != nil {
		return err
	}

	if !regAndActiveVals[msg.Address] {
		logger.Debug("Received an IstanbulAnnounce message from a non registered validator. Ignoring it.", "AnnounceMsg", msg.String(), "err", err)
		return errUnauthorizedAnnounceMessage
	}

	var announcePayload valEncryptedEnodes
	err = rlp.DecodeBytes(msg.Msg, &announcePayload)
	if err != nil {
		logger.Warn("Error in decoding received Istanbul Announce message content", "err", err, "IstanbulMsg", msg.String())
		return err
	}

	logger = logger.New("msgAddress", msg.Address, "msg_timestamp", announcePayload.Timestamp)

	// Ignore the message if it's older than the one that this node currently has persisted and return from this function
	sb.cachedAnnounceMsgsMu.Lock()
	defer sb.cachedAnnounceMsgsMu.Unlock()
	if cachedAnnounceMsgEntry, ok := sb.cachedAnnounceMsgs[msg.Address]; ok && cachedAnnounceMsgEntry.MsgTimestamp >= announcePayload.Timestamp {
		logger.Trace("Received announce message that has older version than the one cached", "cached_timestamp", cachedAnnounceMsgEntry.MsgTimestamp)
		return errOldAnnounceMessage
	}

	// TODO: Figure out if all nodes should do some more basic sanity checking of the announce message

	// TODO: The valEnodeDB is going to be a subset of the announce msg cache data.  Figure out if they should be combined in any way.

	// If this is a registered or elected validator, then process the announce message
	var destAddresses = make([]string, 0, len(announcePayload.EncryptedEnodes))
	var msgHasDupsOrIrrelevantEntries bool = false
	if sb.coreStarted && regAndActiveVals[sb.Address()] {
		var node *enode.Node
		var processedAddresses = make(map[common.Address]bool)
		for _, encryptedEnode := range announcePayload.EncryptedEnodes {
			// Don't process duplicate entries or entries that are not in the regAndActive valset
			if !regAndActiveVals[encryptedEnode.DecrypterAddress] || processedAddresses[encryptedEnode.DecrypterAddress] {
				msgHasDupsOrIrrelevantEntries = true
				continue
			}

			if encryptedEnode.DecrypterAddress == sb.Address() {
				// TODO: Decrypt the enodeURL using this validator's validator key after making changes to encrypt it
				enodeUrl := string(encryptedEnode.EncryptedEnodeURL)
				node, err = enode.ParseV4(enodeUrl)
				if err != nil {
					logger.Error("Error in parsing enodeURL", "enodeUrl", enodeUrl)
					return err
				}
			}
			destAddresses = append(destAddresses, encryptedEnode.DecrypterAddress.String())
			processedAddresses[encryptedEnode.DecrypterAddress] = true
		}

		// Save in the valEnodeTable
		if node != nil {
			if err := sb.valEnodeTable.Upsert(map[common.Address]*vet.AddressEntry{msg.Address: {Node: node, Timestamp: announcePayload.Timestamp}}); err != nil {
				logger.Warn("Error in upserting a valenode entry", "AnnouncePayload", announcePayload.String(), "error", err)
				return err
			}
		}
	}

	if !msgHasDupsOrIrrelevantEntries {
		// Save this announce message
		sb.cachedAnnounceMsgs[msg.Address] = &announceMsgCachedEntry{MsgTimestamp: announcePayload.Timestamp,
			MsgPayload: payload}

		// Prune the announce cache for entries that are not in the current registered/elected validator set
		for cachedValAddress := range sb.cachedAnnounceMsgs {
			if !regAndActiveVals[cachedValAddress] {
				delete(sb.cachedAnnounceMsgs, cachedValAddress)
			}
		}

		if err = sb.regossipAnnounce(msg, payload, announcePayload, regAndActiveVals, destAddresses); err != nil {
			return err
		}
	}

	return nil
}

// This function will regossip a newly received announce message
func (sb *Backend) regossipAnnounce(msg *istanbul.Message, payload []byte, announcePayload valEncryptedEnodes, regAndActiveVals map[common.Address]bool, destAddresses []string) error {
	logger := sb.logger.New("func", "regossipAnnounce", "msgAddress", msg.Address, "msg_timestamp", announcePayload.Timestamp)
	// If we gossiped this address/enodeURL within the last 60 seconds and the enodeURLHash and destAddressHash didn't change, then don't regossip

	// Generate the destAddresses hash
	sort.Strings(destAddresses)
	destAddressesHash := istanbul.RLPHash(destAddresses)

	sb.lastAnnounceGossipedMu.RLock()
	if lastGossipTs, ok := sb.lastAnnounceGossiped[msg.Address]; ok {

		if lastGossipTs.enodeURLHash == announcePayload.EnodeURLHash && bytes.Equal(lastGossipTs.destAddressesHash.Bytes(), destAddressesHash.Bytes()) && time.Since(lastGossipTs.timestamp) < time.Minute {
			logger.Trace("Already regossiped the msg within the last minute, so not regossiping.", "IstanbulMsg", msg.String(), "AnnouncePayload", announcePayload.String())
			sb.lastAnnounceGossipedMu.RUnlock()
			return nil
		}
	}
	sb.lastAnnounceGossipedMu.RUnlock()

	logger.Trace("Regossiping the istanbul announce message", "IstanbulMsg", msg.String(), "AnnouncePayload", announcePayload.String())
	sb.Gossip(nil, payload, istanbulAnnounceMsg, true)

	sb.lastAnnounceGossipedMu.Lock()
	defer sb.lastAnnounceGossipedMu.Unlock()
	sb.lastAnnounceGossiped[msg.Address] = &AnnounceGossipTimestamp{enodeURLHash: announcePayload.EnodeURLHash, timestamp: time.Now(), destAddressesHash: destAddressesHash}

	// prune non registered validator entries in the valEnodeTable, reverseValEnodeTable, and lastAnnounceGossiped tables about 5% of the times that an announce msg is handled
	if (mrand.Int() % 100) <= 5 {
		for remoteAddress := range sb.lastAnnounceGossiped {
			if !regAndActiveVals[remoteAddress] {
				logger.Trace("Deleting entry from the lastAnnounceGossiped table", "address", remoteAddress, "gossip timestamp", sb.lastAnnounceGossiped[remoteAddress])
				delete(sb.lastAnnounceGossiped, remoteAddress)
			}
		}

		if err := sb.valEnodeTable.PruneEntries(regAndActiveVals); err != nil {
			return err
		}
	}

	return nil
}

// ===============================================================
//
// define the AnnounceVersion messge format, the getAnnounceVersions send and handle function, and the announceVersions handle function

type announceVersion struct {
	ValAddress           common.Address
	AnnounceMsgTimestamp uint
}

// EncodeRLP serializes av into the Ethereum RLP format.
func (av *announceVersion) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, []interface{}{av.ValAddress, av.AnnounceMsgTimestamp})
}

// DecodeRLP implements rlp.Decoder, and load the ee fields from a RLP stream.
func (av *announceVersion) DecodeRLP(s *rlp.Stream) error {
	var msg struct {
		ValAddress           common.Address
		AnnounceMsgTimestamp uint
	}

	if err := s.Decode(&msg); err != nil {
		return err
	}
	av.ValAddress, av.AnnounceMsgTimestamp = msg.ValAddress, msg.AnnounceMsgTimestamp
	return nil
}

// The stringer function for the announceVersion type
func (av *announceVersion) String() string {
	return fmt.Sprintf("{ValAddress: %s, AnnounceMsgTimstamp: %d}", av.ValAddress.String(), av.AnnounceMsgTimestamp)
}

// This function will send a GetAnnounceVersions message to a specific peer to request it's announceVersion set
func (sb *Backend) sendGetAnnounceVersions(peer consensus.Peer) {
	sb.logger.Trace("sending a GetAnnounceVersions message", "func", "sendGetAnnounceVersions", "peer", peer)
	go peer.Send(istanbulGetAnnounceVersionsMsg, []byte{})
}

// This function will handle a GetAnnounceVersions message.  Specifically, it will return to the peer
// all of this node's cached announce msgs' version.
func (sb *Backend) handleGetAnnounceVersions(peer consensus.Peer) error {
	logger := sb.logger.New("func", "handleGetAnnounceVersions")
	sb.cachedAnnounceMsgsMu.RLock()
	defer sb.cachedAnnounceMsgsMu.RUnlock()

	logger.Trace("Got a GetAnnounceVersions message", "peer", peer)

	announceVersions := make([]*announceVersion, len(sb.cachedAnnounceMsgs), 0)

	for valAddress, cachedAnnounceEntry := range sb.cachedAnnounceMsgs {
		announceVersions = append(announceVersions, &announceVersion{ValAddress: valAddress, AnnounceMsgTimestamp: cachedAnnounceEntry.MsgTimestamp})
	}

	announceVersionsBytes, err := rlp.EncodeToBytes(announceVersions)
	if err != nil {
		logger.Error("Error encoding announce versions array", "AnnounceVersions", announceVersions, "err", err)
		return err
	}

	logger.Trace("Going to send a AnnounceVersions message", "announceVersions", announceVersions)
	go peer.Send(istanbulAnnounceVersionsMsg, announceVersionsBytes)

	return nil
}

// This function will handle a received AnnounceVersions message.
// Specifically, this node will compare the received version set with it's own,
// and request announce messages that have a higher version that it's own.
func (sb *Backend) handleAnnounceVersions(peer consensus.Peer, data []byte) error {
	logger := sb.logger.New("func", "handleAnnounceVersions")
	var announceVersions []announceVersion

	if err := rlp.DecodeBytes(data, &announceVersions); err != nil {
		logger.Error("Error in decoding announce versions array", "err", err)
	}

	// If the message is not within the registered validator set, then ignore it
	regAndActiveVals, err := sb.retrieveActiveAndRegisteredValidators()
	if err != nil {
		return err
	}

	announcesToRequest := make([]common.Address, 0)
	for _, announceVersion := range announceVersions {
		// Ignore this announceVersion entry if it's val address is not in the current registered or elected valset.
		if !regAndActiveVals[announceVersion.ValAddress] {
			continue
		}

		if cachedEntry, ok := sb.cachedAnnounceMsgs[announceVersion.ValAddress]; !ok || cachedEntry.MsgTimestamp < announceVersion.AnnounceMsgTimestamp {
			announcesToRequest = append(announcesToRequest, announceVersion.ValAddress)
		}
	}

	announcesToRequestBytes, err := rlp.EncodeToBytes(announcesToRequest)
	if err != nil {
		logger.Error("Error encoding announce to request array", "announcesToRequest", common.ConvertToStringSlice(announcesToRequest), "err", err)
		return err
	}

	logger.Trace("Going to send a GetAnnounces", "announcesToRequest", common.ConvertToStringSlice(announcesToRequest))
	go peer.Send(istanbulGetAnnouncesMsg, announcesToRequestBytes)
	return nil
}

func (sb *Backend) checkPeersAnnounceVersions() {
	peers := sb.broadcaster.FindPeers(nil, p2p.AnyPurpose)

	for _, peer := range peers {
		sb.sendGetAnnounceVersions(peer)
	}
}
