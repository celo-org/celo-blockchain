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
	checkIfShouldAnnounceTicker := time.NewTicker(5 * time.Second)

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
				// Gossip the announce after a minute.
				// The delay allows for all receivers of the announce message to
				// have a more up-to-date cached registered/elected valset, and
				// hence more likely that they will be aware that this node is
				// within that set.
				time.AfterFunc(1*time.Minute, func() {
					if err := sb.generateAndGossipAnnounce(); err != nil {
						logger.Error("Error in gossiping announce", "err", err)
					}
				})

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

			go func() {
				if err := sb.generateAndGossipAnnounce(); err != nil {
					logger.Error("Failed to generateAndGossipAnnounce", "err", err)
				}
			}()

			// Use this timer to also prune all announce related data structures.
			if err := sb.pruneAnnounceDataStructures(); err != nil {
				logger.Warn("Error in pruning announce data structures", "err", err)
			}

		case <-announceVersionsCheckTicker.C:
			logger.Trace("Going to check peers' announce version set")
			go sb.checkPeersAnnounceVersions()

		case <-sb.announceThreadQuit:
			checkIfShouldAnnounceTicker.Stop()
			announceVersionsCheckTicker.Stop()
			if announceGossipTicker != nil {
				announceGossipTicker.Stop()
			}
			return
		}
	}
}

func (sb *Backend) shouldGenerateAndProcessAnnounce() (bool, error) {

	// Check if this node is in the registered/elected validator set
	regAndElectedVals, err := sb.retrieveRegisteredAndElectedValidators()
	if err != nil {
		return false, err
	}

	return sb.coreStarted && regAndElectedVals[sb.Address()], nil
}

// pruneAnnounceDataStructures will remove entries that are not in the registered/elected validator set from all announce related data structures.
// The data structures that it prunes are:
// 1)  cachedAnnounceMsgs
// 2)  lastAnnounceGossiped
// 3)  valEnodeTable
func (sb *Backend) pruneAnnounceDataStructures() error {
	logger := sb.logger.New("func", "pruneAnnounceDataStructures")

	// retrieve the registered/elected validator set
	regAndElectedVals, err := sb.retrieveRegisteredAndElectedValidators()
	if err != nil {
		return err
	}

	// Prune the announce cache for entries that are not in the current registered/elected validator set
	sb.cachedAnnounceMsgsMu.Lock()
	for cachedValAddress := range sb.cachedAnnounceMsgs {
		if !regAndElectedVals[cachedValAddress] {
			logger.Trace("Deleting entry from cachedAnnounceMsgs", "cachedValAddress", cachedValAddress, "cachedAnnounceVersion", sb.cachedAnnounceMsgs[cachedValAddress].MsgVersion)
			delete(sb.cachedAnnounceMsgs, cachedValAddress)
		}
	}
	sb.cachedAnnounceMsgsMu.Unlock()

	sb.lastAnnounceGossipedMu.Lock()
	for remoteAddress := range sb.lastAnnounceGossiped {
		if !regAndElectedVals[remoteAddress] {
			logger.Trace("Deleting entry from lastAnnounceGossiped", "address", remoteAddress, "gossip timestamp", sb.lastAnnounceGossiped[remoteAddress])
			delete(sb.lastAnnounceGossiped, remoteAddress)
		}
	}
	sb.lastAnnounceGossipedMu.Unlock()

	if err := sb.valEnodeTable.PruneEntries(regAndElectedVals); err != nil {
		logger.Trace("Error in pruning valEnodeTable", "err", err)
		return err
	}

	return nil
}

// ===============================================================
//
// define the IstanbulAnnounce message format, the AnnounceMsgCache entries, the announce send function (both the gossip version and the "retrieve from cache" version), and the announce get function

type announceRecord struct {
	DestAddress       common.Address
	EncryptedEnodeURL []byte
}

func (ar *announceRecord) String() string {
	return fmt.Sprintf("{DestAddress: %s, EncryptedEnodeURL length: %d}", ar.DestAddress.String(), len(ar.EncryptedEnodeURL))
}

type announceData struct {
	AnnounceRecords []*announceRecord
	EnodeURLHash    common.Hash
	Version         uint
}

func (ad *announceData) String() string {
	return fmt.Sprintf("{Version: %v, EnodeURLHash: %v, AnnounceRecords: %v}", ad.Version, ad.EnodeURLHash.Hex(), ad.AnnounceRecords)
}

// ==============================================
//
// define the functions that needs to be provided for rlp Encoder/Decoder.

// EncodeRLP serializes ar into the Ethereum RLP format.
func (ar *announceRecord) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, []interface{}{ar.DestAddress, ar.EncryptedEnodeURL})
}

// DecodeRLP implements rlp.Decoder, and load the ar fields from a RLP stream.
func (ar *announceRecord) DecodeRLP(s *rlp.Stream) error {
	var msg struct {
		DestAddress       common.Address
		EncryptedEnodeURL []byte
	}

	if err := s.Decode(&msg); err != nil {
		return err
	}
	ar.DestAddress, ar.EncryptedEnodeURL = msg.DestAddress, msg.EncryptedEnodeURL
	return nil
}

// EncodeRLP serializes ad into the Ethereum RLP format.
func (ad *announceData) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, []interface{}{ad.AnnounceRecords, ad.EnodeURLHash, ad.Version})
}

// DecodeRLP implements rlp.Decoder, and load the ad fields from a RLP stream.
func (ad *announceData) DecodeRLP(s *rlp.Stream) error {
	var msg struct {
		AnnounceRecords []*announceRecord
		EnodeURLHash    common.Hash
		Version         uint
	}

	if err := s.Decode(&msg); err != nil {
		return err
	}
	ad.AnnounceRecords, ad.EnodeURLHash, ad.Version = msg.AnnounceRecords, msg.EnodeURLHash, msg.Version
	return nil
}

// Define the announce msg cache entry.
// The latest version (dictated by the announce's version field) will always be cached
// by this node.  Note that it will only cache the announce msgs from registered or elected
// validators.
type announceMsgCachedEntry struct {
	MsgVersion uint
	MsgPayload []byte
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
			go func() {
				if err := peer.Send(istanbulAnnounceMsg, cachedAnnounceMsgEntry.MsgPayload); err != nil {
					logger.Error("Failed to send announce message", "err", err)
				}
			}()
		}
	}

	return nil
}

// generateAndGossipAnnounce will generate the lastest announce msg from this node (as opposed to retrieving from the announceMsgCache) and then broadcast it to it's peers,
// which should then gossip the announce msg message throughout the p2p network, since this announce msg's timestamp should be the latest among all of this
// validator's previous announce msgs.
func (sb *Backend) generateAndGossipAnnounce() error {
	logger := sb.logger.New("func", "generateAndGossipAnnounce")
	logger.Trace("generateAndGossipAnnounce called")
	istMsg, announceVersion, announceAddress, err := sb.generateAnnounce()
	if err != nil {
		return err
	}

	if istMsg == nil {
		return nil
	}

	// Convert to payload
	payload, err := istMsg.Payload()
	if err != nil {
		logger.Error("Error in converting Istanbul Announce Message to payload", "AnnounceMsg", istMsg.String(), "err", err)
		return err
	}

	// Add the generated announce message to this node's cache
	sb.cachedAnnounceMsgsMu.Lock()
	defer sb.cachedAnnounceMsgsMu.Unlock()
	sb.cachedAnnounceMsgs[announceAddress] = &announceMsgCachedEntry{MsgVersion: announceVersion, MsgPayload: payload}

	return sb.Multicast(nil, payload, istanbulAnnounceMsg)
}

// This function is a helper function for generateAndGossipAnnounce.  It will create the latest announce msg for this node.
func (sb *Backend) generateAnnounce() (*istanbul.Message, uint, common.Address, error) {
	logger := sb.logger.New("func", "generateAnnounce")
	var enodeUrl string
	if sb.config.Proxied {
		if sb.proxyNode != nil {
			enodeUrl = sb.proxyNode.externalNode.URLv4()
		} else {
			logger.Error("Proxied node is not connected to a proxy")
			return nil, 0, common.Address{}, errNoProxyConnection
		}
	} else {
		enodeUrl = sb.p2pserver.Self().URLv4()
	}

	// Retrieve the set of remote validators' public key to encrypt the enodeUrl
	regAndActiveVals, err := sb.retrieveRegisteredAndElectedValidators()
	if err != nil {
		return nil, 0, common.Address{}, err
	}

	announceRecords := make([]*announceRecord, 0, len(regAndActiveVals))
	for addr := range regAndActiveVals {
		// TODO - Need to encrypt using the remote validator's validator key
		announceRecords = append(announceRecords, &announceRecord{DestAddress: addr, EncryptedEnodeURL: []byte(enodeUrl)})
	}

	announceData := &announceData{
		AnnounceRecords: announceRecords,
		EnodeURLHash:    istanbul.RLPHash(enodeUrl),
		// Unix() returns a int64, but we need a uint for the golang rlp encoding implmentation. Warning: This timestamp value will be truncated in 2106.
		Version: uint(time.Now().Unix()),
	}

	announceBytes, err := rlp.EncodeToBytes(announceData)
	if err != nil {
		logger.Error("Error encoding announce content", "AnnounceData", announceData.String(), "err", err)
		return nil, 0, common.Address{}, err
	}

	msg := &istanbul.Message{
		Code:      istanbulAnnounceMsg,
		Msg:       announceBytes,
		Address:   sb.Address(),
		Signature: []byte{},
	}

	// Sign the announce message
	if err := msg.Sign(sb.Sign); err != nil {
		logger.Error("Error in signing an Announce Message", "AnnounceMsg", msg.String(), "err", err)
		return nil, 0, common.Address{}, err
	}

	logger.Debug("Generated an announce message", "IstanbulMsg", msg.String(), "AnnounceData", announceData.String())

	return msg, announceData.Version, msg.Address, nil
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
	regAndActiveVals, err := sb.retrieveRegisteredAndElectedValidators()
	if err != nil {
		logger.Trace("Error in retrieving registered/elected valset", "err", err)
		return err
	}

	if !regAndActiveVals[msg.Address] {
		logger.Debug("Received a message from a non registered/elected validator. Ignoring it.", "sender", msg.Address)
		return errUnauthorizedAnnounceMessage
	}

	var announceData announceData
	err = rlp.DecodeBytes(msg.Msg, &announceData)
	if err != nil {
		logger.Warn("Error in decoding received Istanbul Announce message content", "err", err, "IstanbulMsg", msg.String())
		return err
	}

	logger = logger.New("msgAddress", msg.Address, "msgVersion", announceData.Version)

	// Ignore the message if it's older than the one that this node currently has persisted and return from this function
	sb.cachedAnnounceMsgsMu.Lock()
	if cachedAnnounceMsgEntry, ok := sb.cachedAnnounceMsgs[msg.Address]; ok && cachedAnnounceMsgEntry.MsgVersion >= announceData.Version {
		sb.cachedAnnounceMsgsMu.Unlock()
		logger.Trace("Received announce message that has older or same version than the one cached", "cached_timestamp", cachedAnnounceMsgEntry.MsgVersion)
		return errOldAnnounceMessage
	}
	sb.cachedAnnounceMsgsMu.Unlock()

	// Do some validation checks on the announceData
	if isValid, err := sb.validateAnnounce(&announceData); !isValid || err != nil {
		logger.Warn("Validation of announce message failed", "isValid", isValid, "err", err)
		return err
	}

	// If this is a registered or elected validator, then process the announce message
	shouldProcessAnnounce, err := sb.shouldGenerateAndProcessAnnounce()
	if err != nil {
		logger.Warn("Error in checking if should process announce", err)
		return err
	}

	if shouldProcessAnnounce {
		logger.Trace("Going to process an announce msg", "announce records", announceData.AnnounceRecords)
		for _, announceRecord := range announceData.AnnounceRecords {
			if announceRecord.DestAddress == sb.Address() {
				// TODO: Decrypt the enodeURL using this validator's validator key after making changes to encrypt it
				enodeUrl := string(announceRecord.EncryptedEnodeURL)
				node, err := enode.ParseV4(enodeUrl)
				if err != nil {
					logger.Error("Error in parsing enodeURL", "enodeUrl", enodeUrl)
					return err
				}

				if err := sb.valEnodeTable.Upsert(map[common.Address]*vet.AddressEntry{msg.Address: {Node: node, Version: announceData.Version}}); err != nil {
					logger.Warn("Error in upserting a valenode entry", "AnnounceData", announceData.String(), "error", err)
					return err
				}

				break
			}
		}
	}

	sb.cachedAnnounceMsgsMu.Lock()
	sb.cachedAnnounceMsgs[msg.Address] = &announceMsgCachedEntry{MsgVersion: announceData.Version, MsgPayload: payload}
	sb.cachedAnnounceMsgsMu.Unlock()

	// Regossip this announce message
	return sb.regossipAnnounce(msg, payload)
}

// validateAnnounce will do some validation to check the contents of the announce
// message. This is to force all validators that send an announce message to
// create as succint message as possible, and prevent any possible network DOS attacks
// via extremely large announce message.
func (sb *Backend) validateAnnounce(announceData *announceData) (bool, error) {
	logger := sb.logger.New("func", "validateAnnounce")

	// Check if there are any duplicates in the announce message
	var encounteredAddresses = make(map[common.Address]bool)
	for _, announceRecord := range announceData.AnnounceRecords {
		if encounteredAddresses[announceRecord.DestAddress] {
			logger.Info("Announce message has duplicate entries", "address", announceRecord.DestAddress)
			return false, nil
		}

		encounteredAddresses[announceRecord.DestAddress] = true
	}

	// Check if the number of rows in the announcePayload is at most 2 times the size of the current registered/elected validator set.
	// Note that this is a heuristic of the actual size of registered/elected at the time the validator constructed the announce message.
	// Ideally, this should be changed so that as part of the generate announce message, the block number is included, and this node will
	// then verify that all of the registered/elected validators of that block number is included in the announce message.
	regAndActiveVals, err := sb.retrieveRegisteredAndElectedValidators()
	if err != nil {
		return false, err
	}

	if len(announceData.AnnounceRecords) > 2*len(regAndActiveVals) {
		logger.Info("Number of announce message encrypted enodes is more than two times the size of the current reg/elected validator set", "num announce enodes", len(announceData.AnnounceRecords), "reg/elected val set size", len(regAndActiveVals))
		return false, err
	}

	return true, nil
}

// regossipAnnounce will regossip a received announce message.
// If this node regossiped an announce from the same source address within the last 5 minutes, then it wouldn't regossip.
// This is to prevent a malicious registered/elected validator from DOS'ing the network with very frequent announce messages.
// Note that even if the registered/elected validator is not malicious, but changing their enode very frequently, the
// other validators will eventually get that validator's latest enode, since all nodes will periodically check it's neighbors
// for updated announce messages.
func (sb *Backend) regossipAnnounce(msg *istanbul.Message, payload []byte) error {
	logger := sb.logger.New("func", "regossipAnnounce", "announceSourceAddress", msg.Address)

	sb.lastAnnounceGossipedMu.RLock()
	if lastGossipTs, ok := sb.lastAnnounceGossiped[msg.Address]; ok {
		if time.Since(lastGossipTs) < 5*time.Minute {
			logger.Trace("Already regossiped msg from this source address within the last 5 minutes, so not regossiping.")
			sb.lastAnnounceGossipedMu.RUnlock()
			return nil
		}
	}
	sb.lastAnnounceGossipedMu.RUnlock()

	logger.Trace("Regossiping the istanbul announce message", "IstanbulMsg", msg.String())
	if err := sb.Multicast(nil, payload, istanbulAnnounceMsg); err != nil {
		return err
	}

	sb.lastAnnounceGossipedMu.Lock()
	defer sb.lastAnnounceGossipedMu.Unlock()
	sb.lastAnnounceGossiped[msg.Address] = time.Now()

	return nil
}

// ===============================================================
//
// define the AnnounceVersion messge format, the getAnnounceVersions send and handle function, and the announceVersions handle function

type announceVersion struct {
	ValAddress         common.Address
	AnnounceMsgVersion uint
}

// EncodeRLP serializes announceVersion into the Ethereum RLP format.
func (av *announceVersion) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, []interface{}{av.ValAddress, av.AnnounceMsgVersion})
}

// DecodeRLP implements rlp.Decoder, and load the announceVerion fields from a RLP stream.
func (av *announceVersion) DecodeRLP(s *rlp.Stream) error {
	var msg struct {
		ValAddress         common.Address
		AnnounceMsgVersion uint
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
	go func() {
		if err := peer.Send(istanbulGetAnnounceVersionsMsg, []byte{}); err != nil {
			sb.logger.Error("Failed to send announce message", "err", err)
		}
	}()
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
	go func() {
		if err := peer.Send(istanbulAnnounceVersionsMsg, announceVersionsBytes); err != nil {
			logger.Error("Failed to send announce message", "err", err)
		}
	}()

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
	regAndActiveVals, err := sb.retrieveRegisteredAndElectedValidators()
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

		sb.cachedAnnounceMsgsMu.RLock()
		if cachedEntry, ok := sb.cachedAnnounceMsgs[announceVersion.ValAddress]; !ok || cachedEntry.MsgVersion < announceVersion.AnnounceMsgVersion {
			announcesToRequest = append(announcesToRequest, announceVersion.ValAddress)
		}
		sb.cachedAnnounceMsgsMu.RUnlock()
	}

	if len(announcesToRequest) > 0 {
		announcesToRequestBytes, err := rlp.EncodeToBytes(announcesToRequest)
		if err != nil {
			logger.Error("Error encoding announce to request array", "announcesToRequest", common.ConvertToStringSlice(announcesToRequest), "err", err)
			return err
		}

		logger.Trace("Going to send a GetAnnounces", "announcesToRequest", common.ConvertToStringSlice(announcesToRequest), "peer", peer)

		go func() {
			if err := peer.Send(istanbulGetAnnouncesMsg, announcesToRequestBytes); err != nil {
				logger.Error("Failed to send announce message", "err", err)
			}
		}()

	}

	return nil
}

func (sb *Backend) checkPeersAnnounceVersions() {
	peers := sb.broadcaster.FindPeers(nil, p2p.AnyPurpose)

	for _, peer := range peers {
		if peer.Version() >= 65 {
			sb.sendGetAnnounceVersions(peer)
		}
	}
}
