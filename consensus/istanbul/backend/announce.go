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
	"encoding/hex"
	"fmt"
	"io"
	mrand "math/rand"
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

type AnnounceGossipFrequencyState int

const (
	// In this state, send out an announce message every 1 minute until the first peer is established
	HighFreqBeforeFirstPeerState AnnounceGossipFrequencyState = iota

	// In this state, send out an announce message every 1 minute for the first 10 announce messages after the first peer is established.
	// This is on the assumption that when this node first establishes a peer, the p2p network that this node is in may
	// be partitioned with the broader p2p network. We want to give that p2p network some time to connect to the broader p2p network.
	HighFreqAfterFirstPeerState

	// In this state, send out an announce message every 10 minutes
	LowFreqState
)

func (sb *Backend) shouldGenerateAndProcessAnnounce() (bool, error) {

	// Check if this node is in the registered/elected validator set
	regAndActiveVals, err := sb.retrieveActiveAndRegisteredValidators()
	if err != nil {
		return false, err
	}

	return sb.coreStarted && regAndActiveVals[sb.Address()], nil
}

// The announceThread thread function
// It will generate and gossip it's announce message periodically.
// It will also check with it's peers for it's announce message versions, and request any updated ones if necessary.
//
// The announce thread does 3 things
// 1) It will poll to see if this node should send an announce once a minute
// 2) If it should announce, then it will periodically gossip an announce message
// 3) Regardless of whether it should announce, it will periodically ask it's peers for their announceVersions set, and update it's own announce cache accordingly
func (sb *Backend) announceThread() {
	logger := sb.logger.New("func", "announceThread")

	sb.announceThreadWg.Add(1)
	defer sb.announceThreadWg.Done()

	// Create a ticker to poll if istanbul core is running and check if this is a registered/elected validator.
	// If both conditions are true, then this node should announce.
	checkIfShouldAnnounceTicker := time.NewTicker(1 * time.Minute)

	// Create a ticker to check peers' announce versions one every 10 minutes
	announceVersionsCheckTicker := time.NewTicker(10 * time.Minute)

	// Create all the variables needed for the periodic gossip
	var announceGossipTicker *time.Ticker
	var announceGossipTickerCh <-chan time.Time
	var announceGossipFrequencyState AnnounceGossipFrequencyState
	var currentAnnounceGossipTickerDuration time.Duration
	var numGossipedMsgsInHighFreqAfterFirstPeerState int

	for {
		select {
		case <-checkIfShouldAnnounceTicker.C:
			logger.Trace("Checking if this node should announce it's enode")

			shouldAnnounce, err := sb.shouldGenerateAndProcessAnnounce()
			if err != nil {
				logger.Warn("Error in checking if should announce", err)
				break
			}

			if shouldAnnounce && announceGossipTickerCh == nil {
				// Immediately gossip an announce
				go sb.gossipAnnounce()

				if sb.config.AnnounceAggressiveGossipOnEnablement {
					announceGossipFrequencyState = HighFreqBeforeFirstPeerState

					// Send an announce message once a minute
					currentAnnounceGossipTickerDuration = 1 * time.Minute

					numGossipedMsgsInHighFreqAfterFirstPeerState = 0
				} else {
					announceGossipFrequencyState = LowFreqState
					currentAnnounceGossipTickerDuration = time.Duration(sb.config.AnnounceGossipPeriod) * time.Second
				}

				// Enable periodic gossiping by setting announceGossipTickerCh to non nil value
				announceGossipTicker = time.NewTicker(currentAnnounceGossipTickerDuration)
				announceGossipTickerCh = announceGossipTicker.C
				logger.Trace("Enabled periodic gossiping of announce message")
			} else if !shouldAnnounce && announceGossipTickerCh != nil {
				// Disable periodic gossiping by setting announceGossipTickerCh to nil
				announceGossipTicker.Stop()
				announceGossipTickerCh = nil
				logger.Trace("Disabled periodic gossiping of announce message")
			}

		case <-announceGossipTickerCh: // If this is nil (when shouldAnnounce was most recently false), this channel will never receive an event
			logger.Trace("Going to gossip an announce message", "announceGossipFrequencyState", announceGossipFrequencyState, "numGossipedMsgsInHighFreqAfterFirstPeerState", numGossipedMsgsInHighFreqAfterFirstPeerState)
			switch announceGossipFrequencyState {
			case HighFreqBeforeFirstPeerState:
				if len(sb.broadcaster.FindPeers(nil, p2p.AnyPurpose)) > 0 {
					announceGossipFrequencyState = HighFreqAfterFirstPeerState
				}

			case HighFreqAfterFirstPeerState:
				if numGossipedMsgsInHighFreqAfterFirstPeerState >= 10 {
					announceGossipFrequencyState = LowFreqState
				}
				numGossipedMsgsInHighFreqAfterFirstPeerState += 1

			case LowFreqState:
				if currentAnnounceGossipTickerDuration != time.Duration(sb.config.AnnounceGossipPeriod)*time.Second {
					// Reset the ticker
					currentAnnounceGossipTickerDuration = time.Duration(sb.config.AnnounceGossipPeriod) * time.Second
					announceGossipTicker.Stop()
					announceGossipTicker = time.NewTicker(currentAnnounceGossipTickerDuration)
					announceGossipTickerCh = announceGossipTicker.C
				}
			}

			go sb.gossipAnnounce()

		case <-announceVersionsCheckTicker.C:
			logger.Trace("Going to check peers' announce version set")
			go sb.checkPeersAnnounceVersions()

		case <-sb.announceThreadQuit:
			announceVersionsCheckTicker.Stop()
			if announceGossipTicker != nil {
				announceGossipTicker.Stop()
			}
			return
		}
	}
}

// ===============================================================
//
// define the IstanbulAnnounce message format, the AnnounceMsgCache entries, the announce send function (both the gossip version and the "retrieve from cache" version), and the announce get function

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
type validatorEncryptedEnodes struct {
	ValAddress      common.Address
	EncryptedEnodes []*encryptedEnode
	EnodeURLHash    common.Hash
	Version         uint64 // This fields first version will have a value of the ValAddress's local timestamp at the time it sent it's announce message
}

func (vee *validatorEncryptedEnodes) String() string {
	return fmt.Sprintf("{ValAddress: %s, Version: %v, EnodeURLHash: %v, AnnounceRecords: %v}", vee.ValAddress.String(), vee.Version, vee.EnodeURLHash.Hex(), vee.EncryptedEnodes)
}

// EncodeRLP serializes vee into the Ethereum RLP format.
func (vee *validatorEncryptedEnodes) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, []interface{}{vee.ValAddress, vee.EncryptedEnodes, vee.EnodeURLHash, vee.Version})
}

// DecodeRLP implements rlp.Decoder, and load the vee fields from a RLP stream.
func (vee *validatorEncryptedEnodes) DecodeRLP(s *rlp.Stream) error {
	var msg struct {
		ValAddress      common.Address
		EncryptedEnodes []*encryptedEnode
		EnodeURLHash    common.Hash
		Version         uint64
	}

	if err := s.Decode(&msg); err != nil {
		return err
	}
	vee.ValAddress, vee.EncryptedEnodes, vee.EnodeURLHash, vee.Version = msg.ValAddress, msg.EncryptedEnodes, msg.EnodeURLHash, msg.Version
	return nil
}

// Define the announce msg cache entry.
// The latest version (dictated by the announce's timestamp field) will always be cached
// by this node.  Note that it will only cache the announce msgs from registered or elected
// validators.
type announceMsgCachedEntry struct {
	MsgVersion uint64
	MsgPayload []byte
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
func (sb *Backend) handleGetAnnouncesMsg(peer consensus.Peer, payload []byte) error {
	logger := sb.logger.New("func", "handleGetAnnouncesMsg")
	var valAddresses []common.Address

	if err := rlp.DecodeBytes(payload, &valAddresses); err != nil {
		logger.Error("Error in decoding valAddresses", "err", err)
		return err
	}

	sb.cachedAnnounceMsgsMu.RLock()
	defer sb.cachedAnnounceMsgsMu.RUnlock()
	// TODO:  Add support for the AnnounceMsg to contain multiple announce messages within it's payload
	for _, valAddress := range valAddresses {
		if cachedAnnounceMsgEntry, ok := sb.cachedAnnounceMsgs[valAddress]; ok {
			logger.Trace("Sending announce msg", "peer", peer, "valAddress", valAddress)
			go peer.Send(istanbulAnnounceMsg, cachedAnnounceMsgEntry.MsgPayload)
		}
	}

	return nil
}

// This function will generate the lastest announce msg from this node (as opposed to retrieving from the announceMsgCache) and then broadcast it to it's peers,
// which should then gossip the announce msg message throughout the p2p network, since this announce msg's timestamp should be the latest among all of this
// validator's previous announce msgs.
func (sb *Backend) gossipAnnounce() error {
	logger := sb.logger.New("func", "gossipAnnounce")
	logger.Trace("gossipAnnounce called")
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

	// Add the generated announce message to this node's cache
	var announcePayload validatorEncryptedEnodes
	rlp.DecodeBytes(istMsg.Msg, &announcePayload)
	sb.cachedAnnounceMsgs[announcePayload.ValAddress] = &announceMsgCachedEntry{MsgVersion: announcePayload.Version,
		MsgPayload: payload}

	sb.Multicast(nil, payload, istanbulAnnounceMsg)

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

	// Retrieve the set of remote validators' public key to encrypt the enodeUrl
	regAndActiveVals, err := sb.retrieveActiveAndRegisteredValidators()
	if err != nil {
		return nil, err
	}

	encryptedEnodes := make([]*encryptedEnode, 0, len(regAndActiveVals))
	for addr := range regAndActiveVals {
		// TODO - Need to encrypt using the remote validator's validator key
		encryptedEnodes = append(encryptedEnodes, &encryptedEnode{DecrypterAddress: addr, EncryptedEnodeURL: []byte(enodeUrl)})
	}

	announcePayload := &validatorEncryptedEnodes{
		ValAddress:      sb.Address(),
		EncryptedEnodes: encryptedEnodes,
		EnodeURLHash:    istanbul.RLPHash(enodeUrl),
		Version:         uint64(time.Now().Unix()),
	}

	announceBytes, err := rlp.EncodeToBytes(announcePayload)
	if err != nil {
		logger.Error("Error encoding announce payload for an Announce message", "AnnouncePayload", announcePayload.String(), "err", err)
		return nil, err
	}

	// This message should be signed before it's sent out, or else the other nodes will reject it.
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
func (sb *Backend) handleAnnounceMsg(peer consensus.Peer, payload []byte) error {
	logger := sb.logger.New("func", "handleAnnounceMsg")

	msg := new(istanbul.Message)

	// Decode message
	err := msg.FromPayload(payload, istanbul.GetSignatureAddress)
	if err != nil {
		logger.Error("Error in decoding received Istanbul Announce message", "err", err, "payload", hex.EncodeToString(payload))
		return err
	}
	logger.Trace("Handling an IstanbulAnnounce message", "from", msg.Address)

	// Check if the sender is within the registered/elected valset
	regAndActiveVals, err := sb.retrieveActiveAndRegisteredValidators()
	if err != nil {
		return err
	}

	if !regAndActiveVals[msg.Address] {
		logger.Debug("Received a message from a non registered/elected validator. Ignoring it.", "sender", msg.Address)
		return errUnauthorizedAnnounceMessage
	}

	var announcePayload validatorEncryptedEnodes
	err = rlp.DecodeBytes(msg.Msg, &announcePayload)
	if err != nil {
		logger.Warn("Error in decoding received Istanbul Announce message content", "err", err, "IstanbulMsg", msg.String())
		return err
	}

	logger = logger.New("msgAddress", msg.Address, "msgVersion", announcePayload.Version)

	// Ignore the message if it's older than the one that this node currently has persisted and return from this function
	sb.cachedAnnounceMsgsMu.Lock()
	defer sb.cachedAnnounceMsgsMu.Unlock()
	if cachedAnnounceMsgEntry, ok := sb.cachedAnnounceMsgs[msg.Address]; ok && cachedAnnounceMsgEntry.MsgVersion >= announcePayload.Version {
		logger.Trace("Received announce message that has older or same version than the one cached", "cached_timestamp", cachedAnnounceMsgEntry.MsgVersion)
		return errOldAnnounceMessage
	}

	// If this is a registered or elected validator, then process the announce message
	var destAddresses = make([]string, 0, len(announcePayload.EncryptedEnodes))
	var msgHasDupsOrIrrelevantEntries bool = false
	shouldProcessAnnounce, err := sb.shouldGenerateAndProcessAnnounce()
	if err != nil {
		logger.Warn("Error in checking if should process announce", err)
		return err
	}

	if shouldProcessAnnounce {
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
			if err := sb.valEnodeTable.Upsert(map[common.Address]*vet.AddressEntry{msg.Address: {Node: node, Version: announcePayload.Version}}); err != nil {
				logger.Warn("Error in upserting a valenode entry", "AnnouncePayload", announcePayload.String(), "error", err)
				return err
			}
		}
	}

	if !msgHasDupsOrIrrelevantEntries {
		// Cache this announce message
		sb.cachedAnnounceMsgs[msg.Address] = &announceMsgCachedEntry{MsgVersion: announcePayload.Version,
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
func (sb *Backend) regossipAnnounce(msg *istanbul.Message, payload []byte, announcePayload validatorEncryptedEnodes, regAndActiveVals map[common.Address]bool, destAddresses []string) error {
	logger := sb.logger.New("func", "regossipAnnounce", "msgAddress", msg.Address, "msg_timestamp", announcePayload.Version)
	// If we gossiped an announce from this address within the last 5 minutes, then don't regossip.
	// This is to prevent a malicious registered/elected validator from DOS the network with very frequent announce messages.
	// Note that even if the registered/elected validator is not malicious, but changing their enode very frequently, the
	// other validators will eventually get that validator's latest enode, since all nodes will periodically check it's neighbors
	// for updated announce messages.

	sb.lastAnnounceGossipedMu.RLock()
	if lastGossipTs, ok := sb.lastAnnounceGossiped[msg.Address]; ok {
		if time.Since(lastGossipTs) < 5*time.Minute {
			logger.Trace("Already regossiped the msg within the last 5 minutes, so not regossiping.", "IstanbulMsg", msg.String(), "AnnouncePayload", announcePayload.String())
			sb.lastAnnounceGossipedMu.RUnlock()
			return nil
		}
	}
	sb.lastAnnounceGossipedMu.RUnlock()

	logger.Trace("Regossiping the istanbul announce message", "IstanbulMsg", msg.String(), "AnnouncePayload", announcePayload.String())
	sb.Multicast(nil, payload, istanbulAnnounceMsg)

	sb.lastAnnounceGossipedMu.Lock()
	defer sb.lastAnnounceGossipedMu.Unlock()
	sb.lastAnnounceGossiped[msg.Address] = time.Now()

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
	ValAddress         common.Address
	AnnounceMsgVersion uint64
}

// EncodeRLP serializes announceVersion into the Ethereum RLP format.
func (av *announceVersion) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, []interface{}{av.ValAddress, av.AnnounceMsgVersion})
}

// DecodeRLP implements rlp.Decoder, and load the announceVerion fields from a RLP stream.
func (av *announceVersion) DecodeRLP(s *rlp.Stream) error {
	var msg struct {
		ValAddress         common.Address
		AnnounceMsgVersion uint64
	}

	if err := s.Decode(&msg); err != nil {
		return err
	}
	av.ValAddress, av.AnnounceMsgVersion = msg.ValAddress, msg.AnnounceMsgVersion
	return nil
}

// String returns the string representation of announceVersion.
func (av *announceVersion) String() string {
	return fmt.Sprintf("{ValAddress: %s, AnnounceMsgTimstamp: %d}", av.ValAddress.String(), av.AnnounceMsgVersion)
}

// sendGetAnnounceVersions will send a GetAnnounceVersions message to a specific peer to request it's announceVersion set
func (sb *Backend) sendGetAnnounceVersions(peer consensus.Peer) {
	sb.logger.Trace("Sending a GetAnnounceVersions message", "func", "sendGetAnnounceVersions", "peer", peer)
	go peer.Send(istanbulGetAnnounceVersionsMsg, []byte{})
}

// handleGetAnnounceVersionsMsg will handle a GetAnnounceVersions message.  Specifically, it will return to the peer
// all of this node's cached announce msgs' version.
func (sb *Backend) handleGetAnnounceVersionsMsg(peer consensus.Peer, payload []byte) error {
	logger := sb.logger.New("func", "handleGetAnnounceVersionsMsg", "peer", peer)

	logger.Trace("Handling a GetAnnounceVersions message")

	sb.cachedAnnounceMsgsMu.RLock()
	defer sb.cachedAnnounceMsgsMu.RUnlock()

	announceVersions := make([]*announceVersion, 0, len(sb.cachedAnnounceMsgs))

	for valAddress, cachedAnnounceEntry := range sb.cachedAnnounceMsgs {
		announceVersions = append(announceVersions, &announceVersion{ValAddress: valAddress, AnnounceMsgVersion: cachedAnnounceEntry.MsgVersion})
	}

	announceVersionsBytes, err := rlp.EncodeToBytes(announceVersions)
	if err != nil {
		logger.Error("Error encoding announce versions array", "AnnounceVersions", announceVersions, "err", err)
		return err
	}

	logger.Trace("Sending an AnnounceVersions message", "announceVersions", announceVersions, "peer", peer)
	go peer.Send(istanbulAnnounceVersionsMsg, announceVersionsBytes)

	return nil
}

// handleAnnounceVersionsMsg will handle a received AnnounceVersions message.
// Specifically, this node will compare the received version set with it's own,
// and request announce messages that have a higher version that it's own.
func (sb *Backend) handleAnnounceVersionsMsg(peer consensus.Peer, payload []byte) error {
	logger := sb.logger.New("func", "handleAnnounceVersionsMsg", "peer", peer)
	var announceVersions []announceVersion

	if err := rlp.DecodeBytes(payload, &announceVersions); err != nil {
		logger.Error("Error in decoding announce versions array", "err", err)
	}

	logger.Trace("Handling an AnnounceVersions message", "announceVersions", fmt.Sprintf("%v", announceVersions))

	// If the announce's valAddress is not within the registered validator set, then ignore it
	regAndActiveVals, err := sb.retrieveActiveAndRegisteredValidators()
	if err != nil {
		return err
	}

	announcesToRequest := make([]common.Address, 0)
	for _, announceVersion := range announceVersions {
		// Ignore this announceVersion entry if it's val address is not in the current registered or elected valset.
		if !regAndActiveVals[announceVersion.ValAddress] {
			logger.Trace("Ignoring announceVersion since it's not in active/elected valset", "valAddress", announceVersion.ValAddress)
			continue
		}

		if cachedEntry, ok := sb.cachedAnnounceMsgs[announceVersion.ValAddress]; !ok || cachedEntry.MsgVersion < announceVersion.AnnounceMsgVersion {
			announcesToRequest = append(announcesToRequest, announceVersion.ValAddress)
		}
	}

	if len(announcesToRequest) > 0 {
		announcesToRequestBytes, err := rlp.EncodeToBytes(announcesToRequest)
		if err != nil {
			logger.Error("Error encoding announce to request array", "announcesToRequest", common.ConvertToStringSlice(announcesToRequest), "err", err)
			return err
		}

		logger.Trace("Going to send a GetAnnounces", "announcesToRequest", common.ConvertToStringSlice(announcesToRequest), "peer", peer)

		go peer.Send(istanbulGetAnnouncesMsg, announcesToRequestBytes)
	}

	return nil
}

func (sb *Backend) checkPeersAnnounceVersions() {
	peers := sb.broadcaster.FindPeers(nil, p2p.AnyPurpose)

	for _, peer := range peers {
		if peer.Version() == Istanbul65 {
			sb.sendGetAnnounceVersions(peer)
		}
	}
}
