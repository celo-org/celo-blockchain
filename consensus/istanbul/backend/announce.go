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
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/consensus/istanbul"
	vet "github.com/ethereum/go-ethereum/consensus/istanbul/backend/internal/enodes"
	"github.com/ethereum/go-ethereum/crypto/ecies"
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

	// Create all the variables needed for the periodic gossip
	var announceGossipTicker *time.Ticker
	var announceGossipTickerCh <-chan time.Time
	var announceGossipFrequencyState AnnounceGossipFrequencyState
	var currentAnnounceGossipTickerDuration time.Duration
	var numGossipedMsgsInHighFreqAfterFirstPeerState int
	var shouldAnnounce bool
	var err error

	for {
		select {
		case <-checkIfShouldAnnounceTicker.C:
			logger.Trace("Checking if this node should announce it's enode")

			shouldAnnounce, err = sb.shouldGenerateAndProcessAnnounce()
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
					sb.startGossipAnnounceTask()
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

			// Use this timer to also prune all announce related data structures.
			if err := sb.pruneAnnounceDataStructures(); err != nil {
				logger.Warn("Error in pruning announce data structures", "err", err)
			}

			sb.startGossipAnnounceTask()

		case <-sb.generateAndGossipAnnounceCh:
			if shouldAnnounce {
				err := sb.generateAndGossipAnnounce()
				if err != nil {
					logger.Warn("Error in generating and gossiping announce", "err", err)
				}
			}
		case <-sb.announceThreadQuit:
			checkIfShouldAnnounceTicker.Stop()
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
// 1)  lastAnnounceGossiped
// 2)  valEnodeTable
// 3)  lastSignedAnnounceVersionsGossiped
// 4)  signedAnnounceVersionTable
func (sb *Backend) pruneAnnounceDataStructures() error {
	logger := sb.logger.New("func", "pruneAnnounceDataStructures")

	// retrieve the registered/elected validator set
	regAndElectedVals, err := sb.retrieveRegisteredAndElectedValidators()
	if err != nil {
		return err
	}

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

	sb.lastSignedAnnounceVersionsGossipedMu.Lock()
	for remoteAddress := range sb.lastSignedAnnounceVersionsGossiped {
		if !regAndElectedVals[remoteAddress] {
			logger.Trace("Deleting entry from lastSignedAnnounceVersionsGossiped", "address", remoteAddress, "gossip timestamp", sb.lastSignedAnnounceVersionsGossiped[remoteAddress])
			delete(sb.lastSignedAnnounceVersionsGossiped, remoteAddress)
		}
	}
	sb.lastSignedAnnounceVersionsGossipedMu.Unlock()

	if err := sb.signedAnnounceVersionTable.Prune(regAndElectedVals); err != nil {
		logger.Trace("Error in pruning signedAnnounceVersionTable", "err", err)
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
	Version         uint
}

func (ad *announceData) String() string {
	return fmt.Sprintf("{Version: %v, AnnounceRecords: %v}", ad.Version, ad.AnnounceRecords)
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
	return rlp.Encode(w, []interface{}{ad.AnnounceRecords, ad.Version})
}

// DecodeRLP implements rlp.Decoder, and load the ad fields from a RLP stream.
func (ad *announceData) DecodeRLP(s *rlp.Stream) error {
	var msg struct {
		AnnounceRecords []*announceRecord
		Version         uint
	}

	if err := s.Decode(&msg); err != nil {
		return err
	}
	ad.AnnounceRecords, ad.Version = msg.AnnounceRecords, msg.Version
	return nil
}

func (sb *Backend) startGossipAnnounceTask() {
	select {
	case sb.generateAndGossipAnnounceCh <- struct{}{}:
	default:
	}
}

// generateAndGossipAnnounce will generate the lastest announce msg from this node
// and then broadcast it to it's peers, which should then gossip the announce msg
// message throughout the p2p network, since this announce msg's timestamp should be
// the latest among all of this validator's previous announce msgs.
func (sb *Backend) generateAndGossipAnnounce() error {
	logger := sb.logger.New("func", "generateAndGossipAnnounce")
	logger.Trace("generateAndGossipAnnounce called")
	istMsg, err := sb.generateAnnounce()
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

	return sb.Multicast(nil, payload, istanbulAnnounceMsg)
}

// This function is a helper function for generateAndGossipAnnounce that will
// use the version for this node's address in the signed announce version db
func (sb *Backend) generateAnnounce() (*istanbul.Message, error) {
	logger := sb.logger.New("func", "generateAnnounce")

	enodeURL, err := sb.getEnodeURL()
	if err != nil {
		logger.Error("Error getting enode URL", "err", err)
		return nil, err
	}
	announceRecords, err := sb.generateAnnounceRecords(enodeURL)
	if err != nil {
		logger.Warn("Error generating announce records", "err", err)
		return nil, err
	}
	if len(announceRecords) == 0 {
		logger.Trace("No announce records were generated, will not generate announce")
		return nil, nil
	}
	announceVersion, err := sb.signedAnnounceVersionTable.GetVersion(sb.Address())
	if err != nil {
		return nil, err
	}

	announceData := &announceData{
		AnnounceRecords: announceRecords,
		Version:         announceVersion,
	}

	announceBytes, err := rlp.EncodeToBytes(announceData)
	if err != nil {
		logger.Error("Error encoding announce content", "AnnounceData", announceData.String(), "err", err)
		return nil, err
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
		return nil, err
	}

	logger.Debug("Generated an announce message", "IstanbulMsg", msg.String(), "AnnounceData", announceData.String())

	return msg, nil
}

// This is a helper function for generateAnnounce.
// Returns the announce records intended for validators whose entries in the
// val enode table do not exist or are outdated.
func (sb *Backend) generateAnnounceRecords(enodeURL string) ([]*announceRecord, error) {
	allSignedAnnounceVersions, err := sb.signedAnnounceVersionTable.GetAll()
	if err != nil {
		return nil, err
	}
	allValEnodes, err := sb.valEnodeTable.GetAllValEnodes()
	if err != nil {
		return nil, err
	}
	var announceRecords []*announceRecord
	for _, signedAnnounceVersion := range allSignedAnnounceVersions {
		// Don't generate an announce record for ourselves
		if signedAnnounceVersion.Address == sb.Address() {
			continue
		}

		valEnode := allValEnodes[signedAnnounceVersion.Address]
		sb.logger.Warn("in generateAnnounceRecords", "valEnode", valEnode, "signedAnnounceVersion", signedAnnounceVersion)
		// If the version in the val enode table is up to date with the version
		// we are aware of (or is newer in the case the remote validator sent us
		// a direct announce but the signed version is only now being received),
		// don't send them an announce message
		if valEnode != nil && valEnode.Version >= signedAnnounceVersion.Version {
			continue
		}

		ecdsaPubKey, err := signedAnnounceVersion.ECDSAPublicKey()
		if err != nil {
			return nil, err
		}
		publicKey := ecies.ImportECDSAPublic(ecdsaPubKey)
		encryptedEnodeURL, err := ecies.Encrypt(rand.Reader, publicKey, []byte(enodeURL), nil, nil)
		if err != nil {
			return nil, err
		}
		announceRecords = append(announceRecords, &announceRecord{
			DestAddress:       signedAnnounceVersion.Address,
			EncryptedEnodeURL: encryptedEnodeURL,
		})
	}
	return announceRecords, nil
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
				enodeBytes, err := sb.decryptFn(accounts.Account{Address: sb.Address()}, announceRecord.EncryptedEnodeURL, nil, nil)
				if err != nil {
					sb.logger.Warn("Error decrypting endpoint", "err", err, "announceRecord.EncryptedEnodeURL", announceRecord.EncryptedEnodeURL)
					return err
				}
				enodeURL := string(enodeBytes)
				node, err := enode.ParseV4(enodeURL)
				if err != nil {
					logger.Error("Error parsing enodeURL", "enodeUrl", enodeURL)
					return err
				}
				if err := sb.valEnodeTable.Upsert(map[common.Address]*vet.AddressEntry{msg.Address: {Node: node, Version: announceData.Version}}); err != nil {
					logger.Warn("Error upserting a val enode entry", "AnnounceData", announceData.String(), "error", err)
					return err
				}
				// If the announce was only intended for this node, do not regossip
				if len(announceData.AnnounceRecords) == 1 {
					return nil
				}
				break
			}
		}
	}

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

func (sb *Backend) generateSignedAnnounceVersion(version uint) (*vet.SignedAnnounceVersion, error) {
	sav := &vet.SignedAnnounceVersion{
		Address: sb.Address(),
		Version: version,
	}
	err := sav.Sign(sb.Sign)
	if err != nil {
		return nil, err
	}
	return sav, nil
}

func (sb *Backend) gossipSignedAnnounceVersionsMsg(entries []*vet.SignedAnnounceVersion) error {
	logger := sb.logger.New("func", "gossipSignedAnnounceVersionsMsg")

	payload, err := rlp.EncodeToBytes(entries)
	if err != nil {
		logger.Warn("Error encoding entries", "err", err)
		return err
	}
	return sb.Multicast(nil, payload, istanbulSignedAnnounceVersionsMsg)
}

// sendAllSignedAnnounceVersions sends all SignedAnnounceVersions this node
// has to a peer
func (sb *Backend) sendAllSignedAnnounceVersions(peer consensus.Peer) error {
	logger := sb.logger.New("func", "sendAllSignedAnnounceVersions")
	allEntries, err := sb.signedAnnounceVersionTable.GetAll()
	if err != nil {
		logger.Warn("Error getting all entries of signed announce version table", "err", err)
		return err
	}
	payload, err := rlp.EncodeToBytes(allEntries)
	if err != nil {
		logger.Warn("Error encoding entries", "err", err)
		return err
	}
	return peer.Send(istanbulSignedAnnounceVersionsMsg, payload)
}

func (sb *Backend) handleSignedAnnounceVersionsMsg(peer consensus.Peer, payload []byte) error {
	logger := sb.logger.New("func", "handleSignedAnnounceVersionsMsg")
	logger.Trace("Handling signed announce version msg")
	var signedAnnVersions []*vet.SignedAnnounceVersion

	err := rlp.DecodeBytes(payload, &signedAnnVersions)
	if err != nil {
		logger.Warn("Error in decoding received Signed Announce Versions msg", "err", err)
		return err
	}
	regAndActiveVals, err := sb.retrieveRegisteredAndElectedValidators()
	if err != nil {
		logger.Trace("Error in retrieving registered/elected valset", "err", err)
		return err
	}

	var validEntries []*vet.SignedAnnounceVersion
	validAddresses := make(map[common.Address]bool)
	// Verify all entries are valid and remove duplicates
	for _, signedAnnVersion := range signedAnnVersions {
		err := signedAnnVersion.ValidateSignature()
		if err != nil {
			logger.Debug("Error validating signed announce version signature", "address", signedAnnVersion.Address, "err", err)
			continue
		}
		if !regAndActiveVals[signedAnnVersion.Address] {
			logger.Debug("Found signed announce version from a non registered or active validator", "address", signedAnnVersion.Address)
			continue
		}
		if _, ok := validAddresses[signedAnnVersion.Address]; ok {
			logger.Debug("Found duplicate signed announce version in message", "address", signedAnnVersion.Address)
			continue
		}
		validAddresses[signedAnnVersion.Address] = true
		validEntries = append(validEntries, signedAnnVersion)
	}
	return sb.upsertSignedAnnounceVersions(signedAnnVersions)
}

func (sb *Backend) upsertSignedAnnounceVersions(signedAnnVersions []*vet.SignedAnnounceVersion) error {
	logger := sb.logger.New("func", "upsertSignedAnnounceVersions")
	newEntries, err := sb.signedAnnounceVersionTable.Upsert(signedAnnVersions)
	if err != nil {
		logger.Warn("Error in upserting entries", "err", err)
	}

	// If this node is a validator (checked later as a result of this call) and it receives a signed announce
	// version from a remote validator that is newer than the remote validator's
	// version in the val enode table, this node did not receive a direct announce
	// and needs to announce its own enode to the remote validator.
	sb.startGossipAnnounceTask()

	// Only regossip entries that are new to this node and do not originate
	// from an address that we have gossiped a signed announce version for
	// within the last 5 minutes
	var entriesToRegossip []*vet.SignedAnnounceVersion
	sb.lastSignedAnnounceVersionsGossipedMu.Lock()
	for _, entry := range newEntries {
		lastGossipTime, ok := sb.lastSignedAnnounceVersionsGossiped[entry.Address]
		if !ok || time.Since(lastGossipTime) >= 5*time.Minute {
			entriesToRegossip = append(entriesToRegossip, entry)
			sb.lastSignedAnnounceVersionsGossiped[entry.Address] = time.Now()
		}
	}
	sb.lastSignedAnnounceVersionsGossipedMu.Unlock()
	if len(entriesToRegossip) > 0 {
		return sb.gossipSignedAnnounceVersionsMsg(entriesToRegossip)
	}
	return nil
}

func (sb *Backend) updateAnnounceVersion() error {
	logger := sb.logger.New("func", "updateAnnounceVersion")
	// Send new versioned enode msg to all other registered or elected validators
	regAndActiveSet, err := sb.retrieveRegisteredAndElectedValidators()
	if err != nil {
		return err
	}
	// Don't send any messages if this node is not registered or elected
	if !regAndActiveSet[sb.Address()] {
		logger.Trace("Not registered or elected, not updating announce version")
		return nil
	}
	destAddresses := make([]common.Address, len(regAndActiveSet))
	i := 0
	for address := range regAndActiveSet {
		destAddresses[i] = address
		i++
	}
	version := newAnnounceVersion()
	versionedEnodeMsg, err := sb.generateVersionedEnodeMsg(version)
	if err != nil {
		return err
	}
	sb.setVersionedEnodeMsg(versionedEnodeMsg)
	// Send the new versioned enode msg to the proxy peer
	if sb.config.Proxied && sb.proxyNode != nil && sb.proxyNode.peer != nil {
		err := sb.sendVersionedEnodeMsg(sb.proxyNode.peer, versionedEnodeMsg)
		if err != nil {
			logger.Error("Error in sending versioned enode msg to proxy", "err", err)
			return err
		}
	}
	payload, err := versionedEnodeMsg.Payload()
	if err != nil {
		return err
	}
	err = sb.Multicast(destAddresses, payload, istanbulVersionedEnodeMsg)
	if err != nil {
		return err
	}

	// Generate and gossip a new signed announce version
	newSignedAnnVersion, err := sb.generateSignedAnnounceVersion(version)
	if err != nil {
		return err
	}
	return sb.upsertSignedAnnounceVersions([]*vet.SignedAnnounceVersion{
		newSignedAnnVersion,
	})
}

func (sb *Backend) getEnodeURL() (string, error) {
	if sb.config.Proxied {
		if sb.proxyNode != nil {
			return sb.proxyNode.externalNode.URLv4(), nil
		}
		return "", errNoProxyConnection
	}
	return sb.p2pserver.Self().URLv4(), nil
}

type versionedEnode struct {
	EnodeURL string
	Version  uint
}

// ==============================================
//
// define the functions that needs to be provided for rlp Encoder/Decoder.

// EncodeRLP serializes ve into the Ethereum RLP format.
func (ve *versionedEnode) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, []interface{}{ve.EnodeURL, ve.Version})
}

// DecodeRLP implements rlp.Decoder, and load the ve fields from a RLP stream.
func (ve *versionedEnode) DecodeRLP(s *rlp.Stream) error {
	var msg struct {
		EnodeURL string
		Version  uint
	}

	if err := s.Decode(&msg); err != nil {
		return err
	}
	ve.EnodeURL, ve.Version = msg.EnodeURL, msg.Version
	return nil
}

// retrieveVersionedEnodeMsg gets the most recent versioned enode message.
// May be nil if no message was generated as a result of the core not being
// started, or if a proxy has not received a message from its proxied validator
func (sb *Backend) retrieveVersionedEnodeMsg() (*istanbul.Message, error) {
	sb.versionedEnodeMsgMu.RLock()
	defer sb.versionedEnodeMsgMu.RUnlock()
	if sb.versionedEnodeMsg == nil {
		return nil, nil
	}
	return sb.versionedEnodeMsg.Copy(), nil
}

// generateVersionedEnodeMsg generates a versioned enode message with the enode
// this node is publicly accessible at. If this node is proxied, the proxy's
// public enode is used.
func (sb *Backend) generateVersionedEnodeMsg(version uint) (*istanbul.Message, error) {
	logger := sb.logger.New("func", "generateVersionedEnodeMsg")

	enodeURL, err := sb.getEnodeURL()
	if err != nil {
		return nil, err
	}
	versionedEnode := &versionedEnode{
		EnodeURL: enodeURL,
		Version:  version,
	}
	versionedEnodeBytes, err := rlp.EncodeToBytes(versionedEnode)
	if err != nil {
		return nil, err
	}
	msg := &istanbul.Message{
		Code:    istanbulVersionedEnodeMsg,
		Address: sb.Address(),
		Msg:     versionedEnodeBytes,
	}
	// Sign the message
	if err := msg.Sign(sb.Sign); err != nil {
		return nil, err
	}
	logger.Trace("Generated Istanbul Versioned Enode message", "versionedEnode", versionedEnode, "address", msg.Address)
	return msg, nil
}

// handleVersionedEnodeMsg handles a versioned enode message.
// At the moment, this message is only supported if it's sent from a proxied
// validator to its proxy or vice versa.
func (sb *Backend) handleVersionedEnodeMsg(peer consensus.Peer, payload []byte) error {
	logger := sb.logger.New("func", "handleVersionedEnodeMsg")

	var msg istanbul.Message
	// Decode payload into msg
	err := msg.FromPayload(payload, istanbul.GetSignatureAddress)
	if err != nil {
		logger.Error("Error in decoding received Istanbul Versioned Enode message", "err", err, "payload", hex.EncodeToString(payload))
		return err
	}
	logger = logger.New("msg address", msg.Address)
	logger.Trace("Handling an Istanbul Versioned Enode message")

	var versionedEnode versionedEnode
	if err := rlp.DecodeBytes(msg.Msg, &versionedEnode); err != nil {
		logger.Warn("Error in decoding received Istanbul Versioned Enode message content", "err", err, "IstanbulMsg", msg.String())
		return err
	}

	parsedNode, err := enode.ParseV4(versionedEnode.EnodeURL)
	if err != nil {
		logger.Warn("Malformed v4 node in received Istanbul Versioned Enode message", "versionedEnode", versionedEnode, "err", err)
		return err
	}

	// Handle the special case where this node is a proxy and the proxied validator
	// sent a versioned enode for the proxy to use in handshakes
	if sb.config.Proxy && msg.Address == sb.config.ProxiedValidatorAddress {
		// There may be a difference in the URLv4 string because of `discport`,
		// so instead compare the ID
		selfNode := sb.p2pserver.Self()
		if parsedNode.ID() != selfNode.ID() {
			logger.Warn("Received Istanbul Versioned Enode message with an incorrect enode url", "message enode url", versionedEnode.EnodeURL, "self enode url", sb.p2pserver.Self().URLv4())
			return errors.New("Incorrect enode url")
		}
		sb.setVersionedEnodeMsg(&msg)
		return nil
	}

	regAndActiveVals, err := sb.retrieveRegisteredAndElectedValidators()
	if err != nil {
		logger.Debug("Error in retrieving registered/elected valset", "err", err)
		return err
	}

	if !regAndActiveVals[msg.Address] {
		logger.Debug("Received Istanbul Versioned Enode message originating from a non registered or active validator")
		return errUnauthorizedAnnounceMessage
	}

	logger.Trace("Received Istanbul Versioned Enode message", "versionedEnode", versionedEnode)

	if err := sb.valEnodeTable.Upsert(map[common.Address]*vet.AddressEntry{msg.Address: {Node: parsedNode, Version: versionedEnode.Version}}); err != nil {
		logger.Warn("Error in upserting a val enode table entry", "error", err)
		return err
	}
	return nil
}

func (sb *Backend) sendVersionedEnodeMsg(peer consensus.Peer, msg *istanbul.Message) error {
	logger := sb.logger.New("func", "sendVersionedEnodeMsg")
	payload, err := msg.Payload()
	if err != nil {
		logger.Error("Error getting payload of versioned enode message", "err", err)
		return err
	}
	return peer.Send(istanbulVersionedEnodeMsg, payload)
}

func (sb *Backend) setVersionedEnodeMsg(msg *istanbul.Message) {
	sb.versionedEnodeMsgMu.Lock()
	sb.versionedEnodeMsg = msg
	sb.versionedEnodeMsgMu.Unlock()
}

func newAnnounceVersion() uint {
	// Unix() returns a int64, but we need a uint for the golang rlp encoding implmentation. Warning: This timestamp value will be truncated in 2106.
	return uint(time.Now().Unix())
}
