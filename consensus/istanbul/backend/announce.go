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
	"crypto/ecdsa"
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
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/crypto/ecies"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/rlp"
)

// ==============================================
//
// define the constants and function for the sendAnnounce thread


const (
	announceGossipCooldownDuration = 5 * time.Minute
	// Schedule retries to be strictly later than the cooldown duration
	// that other nodes will impose for regossiping announces from this node.
	announceRetryDuration = announceGossipCooldownDuration + (5 * time.Second)

	signedAnnounceVersionGossipCooldownDuration = 5 * time.Minute
)

// The announceThread will:
// 1) Periodically poll to see if this node should be announcing
// 2) Periodically share the entire signed announce version table with all peers
// 3) Periodically prune announce-related data structures
// 4) Gossip announce messages when requested
// 5) Retry sending announce messages if they go unanswered
// 6) Update announce version when requested
func (sb *Backend) announceThread() {
	logger := sb.logger.New("func", "announceThread")

	sb.announceThreadWg.Add(1)
	defer sb.announceThreadWg.Done()

	// Create a ticker to poll if istanbul core is running and if this node is in
	// the validator conn set. If both conditions are true, then this node should announce.
	checkIfShouldAnnounceTicker := time.NewTicker(5 * time.Second)
	// Occasionally share the entire signed announce version table with all peers
	shareSignedAnnounceVersionTicker := time.NewTicker(5 * time.Minute)
	pruneAnnounceDataStructuresTicker := time.NewTicker(10 * time.Minute)

	var announceRetryTimer *time.Timer
	var announceRetryTimerCh <-chan time.Time
	var announceVersion uint
	var announcing bool
	var shouldAnnounce bool
	var err error

	updateAnnounceVersionFunc := func() {
		version := newAnnounceVersion()
		if version <= announceVersion {
			logger.Debug("Announce version is not newer than the existing version", "existing version", announceVersion, "attempted new version", version)
			return
		}
		if err := sb.setAndShareUpdatedAnnounceVersion(version); err != nil {
			logger.Warn("Error updating announce version", "err", err)
			return
		}
		announceVersion = version
	}

	for {
		select {
		case <-checkIfShouldAnnounceTicker.C:
			logger.Trace("Checking if this node should announce it's enode")

			shouldAnnounce, err = sb.shouldGenerateAndProcessAnnounce()
			if err != nil {
				logger.Warn("Error in checking if should announce", err)
				break
			}

			if shouldAnnounce && !announcing {
				// Gossip the announce after a minute.
				// The delay allows for all receivers of the announce message to
				// have a more up-to-date cached registered/elected valset, and
				// hence more likely that they will be aware that this node is
				// within that set.
				time.AfterFunc(1*time.Minute, func() {
					sb.startGossipAnnounceTask()
				})

				announcing = true
				logger.Trace("Enabled periodic gossiping of announce message")
			} else if !shouldAnnounce && announcing {
				if announceRetryTimer != nil {
					announceRetryTimer.Stop()
					announceRetryTimer = nil
					announceRetryTimerCh = nil
				}
				announcing = false
				logger.Trace("Disabled periodic gossiping of announce message")
			}

		case <-pruneAnnounceDataStructuresTicker.C:
			if err := sb.pruneAnnounceDataStructures(); err != nil {
				logger.Warn("Error in pruning announce data structures", "err", err)
			}

		case <-shareSignedAnnounceVersionTicker.C:
			// Send all signed announce versions to every peer. Only the entries
			// that are new to a node will end up being regossiped throughout the
			// network.
			allSignedAnnounceVersions, err := sb.getAllSignedAnnounceVersions()
			if err != nil {
				logger.Warn("Error getting all signed announce versions", "err", err)
				break
			}
			if err := sb.gossipSignedAnnounceVersionsMsg(allSignedAnnounceVersions); err != nil {
				logger.Warn("Error gossiping all signed announce versions")
			}

		case <-announceRetryTimerCh: // If this is nil, this channel will never receive an event
			announceRetryTimer = nil
			announceRetryTimerCh = nil
			sb.startGossipAnnounceTask()

		case <-sb.generateAndGossipAnnounceCh:
			if shouldAnnounce {
				// This node may have recently sent out an announce message within
				// the gossip cooldown period imposed by other nodes.
				// Regardless, send the announce so that it will at least be
				// processed by this node's peers. This is especially helpful when a network
				// is first starting up.
				success, err := sb.generateAndGossipAnnounce(announceVersion)
				if err != nil {
					logger.Warn("Error in generating and gossiping announce", "err", err)
				}
				// If a retry hasn't been scheduled already by a previous announce,
				// schedule one.
				if success && announceRetryTimer != nil {
					announceRetryTimer = time.NewTimer(announceRetryDuration)
					announceRetryTimerCh = announceRetryTimer.C
				}
			}

		case <-sb.updateAnnounceVersionCh:
			updateAnnounceVersionFunc()
			// Show that the announce update has been completed so we can rely on
			// it synchronously
			sb.updateAnnounceVersionCompleteCh <- struct{}{}

		case <-sb.announceThreadQuit:
			checkIfShouldAnnounceTicker.Stop()
			pruneAnnounceDataStructuresTicker.Stop()
			if announceRetryTimer != nil {
				announceRetryTimer.Stop()
				announceRetryTimer = nil
				announceRetryTimerCh = nil
			}
			return
		}
	}
}

func (sb *Backend) shouldGenerateAndProcessAnnounce() (bool, error) {

	// Check if this node is in the validator connection set
	validatorConnSet, err := sb.retrieveValidatorConnSet()
	if err != nil {
		return false, err
	}

	return sb.coreStarted && validatorConnSet[sb.Address()], nil
}

// pruneAnnounceDataStructures will remove entries that are not in the validator connection set from all announce related data structures.
// The data structures that it prunes are:
// 1)  lastAnnounceGossiped
// 2)  valEnodeTable
// 3)  lastSignedAnnounceVersionsGossiped
// 4)  signedAnnounceVersionTable
func (sb *Backend) pruneAnnounceDataStructures() error {
	logger := sb.logger.New("func", "pruneAnnounceDataStructures")

	// retrieve the validator connection set
	validatorConnSet, err := sb.retrieveValidatorConnSet()
	if err != nil {
		return err
	}

	sb.lastAnnounceGossipedMu.Lock()
	for remoteAddress := range sb.lastAnnounceGossiped {
		if !validatorConnSet[remoteAddress] && time.Since(sb.lastAnnounceGossiped[remoteAddress].Time) >= announceGossipCooldownDuration {
			logger.Trace("Deleting entry from lastAnnounceGossiped", "address", remoteAddress, "gossip timestamp", sb.lastAnnounceGossiped[remoteAddress])
			delete(sb.lastAnnounceGossiped, remoteAddress)
		}
	}
	sb.lastAnnounceGossipedMu.Unlock()

	if err := sb.valEnodeTable.PruneEntries(validatorConnSet); err != nil {
		logger.Trace("Error in pruning valEnodeTable", "err", err)
		return err
	}

	sb.lastSignedAnnounceVersionsGossipedMu.Lock()
	for remoteAddress := range sb.lastSignedAnnounceVersionsGossiped {
		if !validatorConnSet[remoteAddress] && time.Since(sb.lastSignedAnnounceVersionsGossiped[remoteAddress]) >= signedAnnounceVersionGossipCooldownDuration {
			logger.Trace("Deleting entry from lastSignedAnnounceVersionsGossiped", "address", remoteAddress, "gossip timestamp", sb.lastSignedAnnounceVersionsGossiped[remoteAddress])
			delete(sb.lastSignedAnnounceVersionsGossiped, remoteAddress)
		}
	}
	sb.lastSignedAnnounceVersionsGossipedMu.Unlock()

	if err := sb.signedAnnounceVersionTable.Prune(validatorConnSet); err != nil {
		logger.Trace("Error in pruning signedAnnounceVersionTable", "err", err)
		return err
	}

	return nil
}

// ===============================================================
//
// define the IstanbulAnnounce message format, the AnnounceMsgCache entries, the announce send function (both the gossip version and the "retrieve from cache" version), and the announce get function

type versionedGossipTime struct {
	Time    time.Time
	Version uint
}

type scheduledRegossip struct {
	Timer   *time.Timer
	Version uint
}

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
	// sb.generateAndGossipAnnounceCh has a buffer of 1. If there is a value
	// already sent to the channel that has not been read from, don't block.
	select {
	case sb.generateAndGossipAnnounceCh <- struct{}{}:
	default:
	}
}

// generateAndGossipAnnounce will generate the lastest announce msg from this node
// and then broadcast it to it's peers, which should then gossip the announce msg
// message throughout the p2p network if there has not been a message sent from
// this node within the last announceGossipCooldownDuration.
// Returns if an announce message was multicasted and if there was an error.
func (sb *Backend) generateAndGossipAnnounce(version uint) (bool, error) {
	logger := sb.logger.New("func", "generateAndGossipAnnounce")
	logger.Trace("generateAndGossipAnnounce called")
	istMsg, err := sb.generateAnnounce(version)
	if err != nil {
		return false, err
	}

	if istMsg == nil {
		return false, nil
	}

	// Convert to payload
	payload, err := istMsg.Payload()
	if err != nil {
		logger.Error("Error in converting Istanbul Announce Message to payload", "AnnounceMsg", istMsg.String(), "err", err)
		return false, err
	}

	if err := sb.Multicast(nil, payload, istanbulAnnounceMsg); err != nil {
		return false, err
	}
	return true, nil
}

// generateAnnounce returns a announce message from this node with a given version.
func (sb *Backend) generateAnnounce(version uint) (*istanbul.Message, error) {
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
	announceData := &announceData{
		AnnounceRecords: announceRecords,
		Version:         version,
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

// generateAnnounceRecords returns the announce records intended for validators
// whose entries in the val enode table do not exist or are outdated when compared
// to the signed announce version table.
func (sb *Backend) generateAnnounceRecords(enodeURL string) ([]*announceRecord, error) {
	allSignedAnnVersionEntries, err := sb.signedAnnounceVersionTable.GetAll()
	if err != nil {
		return nil, err
	}
	allValEnodes, err := sb.valEnodeTable.GetAllValEnodes()
	if err != nil {
		return nil, err
	}
	var announceRecords []*announceRecord
	for _, signedAnnVersionEntry := range allSignedAnnVersionEntries {
		// Don't generate an announce record for ourselves
		if signedAnnVersionEntry.Address == sb.Address() {
			continue
		}

		valEnode := allValEnodes[signedAnnVersionEntry.Address]
		// If the version in the val enode table is up to date with the corresponding
		// version in our signed announce version table, don't send the validator an announce
		// message.
		// It's also possible the version in the val enode table is newer than the version
		// in the signed announce version table in the case that the remote validator
		// sent us an enodeCertificate and we haven't received a signed announce version
		// update yet.
		if valEnode != nil && valEnode.Version >= signedAnnVersionEntry.Version {
			continue
		}

		signedAnnounceVersion := newSignedAnnounceVersionFromEntry(signedAnnVersionEntry)
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

	// Check if the sender is within the validator connection set
	validatorConnSet, err := sb.retrieveValidatorConnSet()
	if err != nil {
		logger.Trace("Error in retrieving validator connection set", "err", err)
		return err
	}

	if !validatorConnSet[msg.Address] {
		logger.Debug("Received a message from a validator not within the validator connection set. Ignoring it.", "sender", msg.Address)
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
	if isValid, err := sb.validateAnnounce(msg.Address, &announceData); !isValid || err != nil {
		logger.Warn("Validation of announce message failed", "isValid", isValid, "err", err)
		return err
	}

	// If this is an elected or nearly elected validator, then process the announce message
	shouldProcessAnnounce, err := sb.shouldGenerateAndProcessAnnounce()
	if err != nil {
		logger.Warn("Error in checking if should process announce", err)
	}

	if shouldProcessAnnounce {
		logger.Trace("Processing an announce message", "announce records", announceData.AnnounceRecords)
		for _, announceRecord := range announceData.AnnounceRecords {
			// Only process an announceRecord intended for this node
			if announceRecord.DestAddress != sb.Address() {
				continue
			}
			enodeBytes, err := sb.decryptFn(accounts.Account{Address: sb.Address()}, announceRecord.EncryptedEnodeURL, nil, nil)
			if err != nil {
				sb.logger.Warn("Error decrypting endpoint", "err", err, "announceRecord.EncryptedEnodeURL", announceRecord.EncryptedEnodeURL)
				return err
			}
			enodeURL := string(enodeBytes)
			node, err := enode.ParseV4(enodeURL)
			if err != nil {
				logger.Warn("Error parsing enodeURL", "enodeUrl", enodeURL)
				return err
			}
			if err := sb.answerAnnounceMsg(msg.Address, node, announceData.Version); err != nil {
				logger.Warn("Error answering an announce msg", "target node", node.URLv4(), "error", err)
				return err
			}
			break
		}
	}

	// Regossip this announce message
	return sb.regossipAnnounce(msg, announceData.Version, payload, false)
}

// answerAnnounceMsg will answer a received announce message from an origin
// node. If the node is already a peer of any kind, an enodeCertificate will be sent.
// Regardless, the origin node will be upserted into the val enode table
// to ensure this node designates the origin node as a ValidatorPurpose peer.
func (sb *Backend) answerAnnounceMsg(address common.Address, node *enode.Node, version uint) error {
	targetIDs := map[enode.ID]bool{
		node.ID(): true,
	}
	// The target could be an existing peer of any purpose.
	matches := sb.broadcaster.FindPeers(targetIDs, p2p.AnyPurpose)
	if matches[node.ID()] != nil {
		enodeCertificateMsg, err := sb.retrieveEnodeCertificateMsg()
		if err != nil {
			return err
		}
		if err := sb.sendEnodeCertificateMsg(matches[node.ID()], enodeCertificateMsg); err != nil {
			return err
		}
	}
	// Upsert regardless to account for the case that the target is a non-ValidatorPurpose
	// peer but should be.
	// If the target is not a peer and should be a ValidatorPurpose peer, this
	// will designate the target as a ValidatorPurpose peer and send an enodeCertificate
	// during the istanbul handshake.
	if err := sb.valEnodeTable.Upsert(map[common.Address]*vet.AddressEntry{address: {Node: node, Version: version}}); err != nil {
		return err
	}
	return nil
}

// validateAnnounce will do some validation to check the contents of the announce
// message. This is to force all validators that send an announce message to
// create as succint message as possible, and prevent any possible network DOS attacks
// via extremely large announce message.
func (sb *Backend) validateAnnounce(msgAddress common.Address, announceData *announceData) (bool, error) {
	logger := sb.logger.New("func", "validateAnnounce", "msg address", msgAddress)

	// Ensure the version in the announceData is not older than the signed
	// announce version if it is known by this node
	knownVersion, err := sb.signedAnnounceVersionTable.GetVersion(msgAddress)
	if err == nil && announceData.Version < knownVersion {
		logger.Debug("Announce message has version older than known version from signed announce table", "msg version", announceData.Version, "known version", knownVersion)
		return false, nil
	}

	// Check if there are any duplicates in the announce message
	var encounteredAddresses = make(map[common.Address]bool)
	for _, announceRecord := range announceData.AnnounceRecords {
		if encounteredAddresses[announceRecord.DestAddress] {
			logger.Info("Announce message has duplicate entries", "address", announceRecord.DestAddress)
			return false, nil
		}

		encounteredAddresses[announceRecord.DestAddress] = true
	}

	// Check if the number of rows in the announcePayload is at most 2 times the size of the current validator connection set.
	// Note that this is a heuristic of the actual size of validator connection set at the time the validator constructed the announce message.
	validatorConnSet, err := sb.retrieveValidatorConnSet()
	if err != nil {
		return false, err
	}

	if len(announceData.AnnounceRecords) > 2*len(validatorConnSet) {
		logger.Info("Number of announce message encrypted enodes is more than two times the size of the current validator connection set", "num announce enodes", len(announceData.AnnounceRecords), "reg/elected val set size", len(validatorConnSet))
		return false, err
	}

	return true, nil
}

// scheduleAnnounceRegossip schedules a regossip for an announce message after
// its cooldown period. If one is already scheduled, it's overwritten.
func (sb *Backend) scheduleAnnounceRegossip(msg *istanbul.Message, announceVersion uint, payload []byte) error {
	logger := sb.logger.New("func", "scheduleAnnounceRegossip", "msg address", msg.Address)
	sb.scheduledAnnounceRegossipsMu.Lock()
	defer sb.scheduledAnnounceRegossipsMu.Unlock()

	// If another announce version regossip has been scheduled for the message
	// address already, cancel it
	if scheduledRegossip := sb.scheduledAnnounceRegossips[msg.Address]; scheduledRegossip != nil {
		scheduledRegossip.Timer.Stop()
	}

	sb.lastAnnounceGossipedMu.RLock()
	versionedGossipTime := sb.lastAnnounceGossiped[msg.Address]
	if versionedGossipTime == nil {
		logger.Debug("Last announce gossiped not found")
		sb.lastAnnounceGossipedMu.RUnlock()
		return nil
	}
	scheduledTime := versionedGossipTime.Time.Add(announceGossipCooldownDuration)
	sb.lastAnnounceGossipedMu.RUnlock()

	duration := time.Until(scheduledTime)
	// If by this time the cooldown period has ended, just regossip
	if duration < 0 {
		return sb.regossipAnnounce(msg, announceVersion, payload, true)
	}
	regossipFunc := func() {
		if err := sb.regossipAnnounce(msg, announceVersion, payload, true); err != nil {
			logger.Debug("Error in scheduled announce regossip", "err", err)
		}
		sb.scheduledAnnounceRegossipsMu.Lock()
		delete(sb.scheduledAnnounceRegossips, msg.Address)
		sb.scheduledAnnounceRegossipsMu.Unlock()
	}
	sb.scheduledAnnounceRegossips[msg.Address] = &scheduledRegossip{
		Timer: time.AfterFunc(duration, regossipFunc),
		Version: announceVersion,
	}
	return nil
}

// regossipAnnounce will regossip a received announce message.
// If this node regossiped an announce from the same source address within the last
// 5 minutes, then it won't regossip. This is to prevent a malicious validator from
// DOS'ing the network with very frequent announce messages.
// This opens an attack vector where any malicious node could continue to gossip
// a previously gossiped announce message, causing other nodes to regossip and
// enforce the cooldown period for future messages originating from the origin validator.
// This could result in new legitimate announce messages from the origin validator
// not being regossiped due to the cooldown period enforced by the other nodes in
// the network.
// To ensure that a new legitimate message will eventually be gossiped to the rest
// of the network, we schedule an announce message with a new version that is
// received during the cooldown period to be sent after the cooldown period ends,
// and refuse to regossip old messages from the validator during that time.
// Providing `force` as true will skip any of the DOS checks.
func (sb *Backend) regossipAnnounce(msg *istanbul.Message, announceVersion uint, payload []byte, force bool) error {
	logger := sb.logger.New("func", "regossipAnnounce", "announceSourceAddress", msg.Address, "announceVersion", announceVersion)

	if !force {
		sb.scheduledAnnounceRegossipsMu.RLock()
		scheduledRegossip := sb.scheduledAnnounceRegossips[msg.Address]
		// if a regossip for this msg address has been scheduled already, override
		// the scheduling if this version is newer. Otherwise, don't regossip this message
		// and let the scheduled regossip occur.
		if scheduledRegossip != nil {
			if announceVersion > scheduledRegossip.Version {
				sb.scheduledAnnounceRegossipsMu.RUnlock()
				return sb.scheduleAnnounceRegossip(msg, announceVersion, payload)
			}
			sb.scheduledAnnounceRegossipsMu.RUnlock()
			logger.Debug("Announce message is not greater than scheduled regossip version, not regossiping")
			return nil
		}
		sb.scheduledAnnounceRegossipsMu.RUnlock()

		sb.lastAnnounceGossipedMu.RLock()
		if lastGossiped, ok := sb.lastAnnounceGossiped[msg.Address]; ok {
			if time.Since(lastGossiped.Time) < announceGossipCooldownDuration {
				// If this version is newer than the previously regossiped one,
				// schedule it to be regossiped once the cooldown period is over
				if lastGossiped.Version < announceVersion {
					sb.lastAnnounceGossipedMu.RUnlock()
					logger.Trace("Already regossiped msg from this source address with an older announce version within the cooldown period, scheduling regossip for after the cooldown")
					return sb.scheduleAnnounceRegossip(msg, announceVersion, payload)
				}
				sb.lastAnnounceGossipedMu.RUnlock()
				logger.Trace("Already regossiped msg from this source address within the cooldown period, not regossiping.")
				return nil
			}
		}
		sb.lastAnnounceGossipedMu.RUnlock()
	}

	logger.Trace("Regossiping the istanbul announce message", "IstanbulMsg", msg.String())
	if err := sb.Multicast(nil, payload, istanbulAnnounceMsg); err != nil {
		return err
	}

	sb.lastAnnounceGossipedMu.Lock()
	defer sb.lastAnnounceGossipedMu.Unlock()
	sb.lastAnnounceGossiped[msg.Address] = &versionedGossipTime{
		Time: time.Now(),
		Version: announceVersion,
	}

	return nil
}

// signedAnnounceVersion is a signed message from a validator indicating the most
// recent version of its enode.
type signedAnnounceVersion struct {
	Address   common.Address
	Version   uint
	Signature []byte
}

func newSignedAnnounceVersionFromEntry(entry *vet.SignedAnnounceVersionEntry) *signedAnnounceVersion {
	return &signedAnnounceVersion{
		Address: entry.Address,
		Version: entry.Version,
		Signature: entry.Signature,
	}
}

func (sav *signedAnnounceVersion) Sign(signingFn func(data []byte) ([]byte, error)) error {
	payloadNoSig, err := sav.payloadNoSig()
	if err != nil {
		return err
	}
	sav.Signature, err = signingFn(payloadNoSig)
	if err != nil {
		return err
	}
	return nil
}

func (sav *signedAnnounceVersion) ECDSAPublicKey() (*ecdsa.PublicKey, error) {
	payloadNoSig, err := sav.payloadNoSig()
	if err != nil {
		return nil, err
	}
	payloadHash := crypto.Keccak256(payloadNoSig)
	pubkey, err := crypto.SigToPub(payloadHash, sav.Signature)
	if err != nil {
		return nil, err
	}
	return pubkey, nil
}

// ValidateSignature will return an error if a SignedAnnounceVersion's signature
// is invalid.
func (sav *signedAnnounceVersion) ValidateSignature() error {
	payloadNoSig, err := sav.payloadNoSig()
	if err != nil {
		return err
	}
	address, err := istanbul.GetSignatureAddress(payloadNoSig, sav.Signature)
	if err != nil {
		return err
	}
	if address != sav.Address {
		return errors.New("Signature does not match address")
	}
	return nil
}

// EncodeRLP serializes signedAnnounceVersion into the Ethereum RLP format.
func (sav *signedAnnounceVersion) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, []interface{}{sav.Address, sav.Version, sav.Signature})
}

// DecodeRLP implements rlp.Decoder, and load the signedAnnounceVersion fields from a RLP stream.
func (sav *signedAnnounceVersion) DecodeRLP(s *rlp.Stream) error {
	var msg struct {
		Address   common.Address
		Version   uint
		Signature []byte
	}

	if err := s.Decode(&msg); err != nil {
		return err
	}
	sav.Address, sav.Version, sav.Signature = msg.Address, msg.Version, msg.Signature
	return nil
}

func (sav *signedAnnounceVersion) Entry() *vet.SignedAnnounceVersionEntry {
	return &vet.SignedAnnounceVersionEntry{
		Address: sav.Address,
		Version: sav.Version,
		Signature: sav.Signature,
	}
}

func (sav *signedAnnounceVersion) payloadNoSig() ([]byte, error) {
	savNoSig := &signedAnnounceVersion{
		Address: sav.Address,
		Version: sav.Version,
	}
	payloadNoSig, err := rlp.EncodeToBytes(savNoSig)
	if err != nil {
		return nil, err
	}
	return payloadNoSig, nil
}

func (sb *Backend) generateSignedAnnounceVersion(version uint) (*signedAnnounceVersion, error) {
	sav := &signedAnnounceVersion{
		Address: sb.Address(),
		Version: version,
	}
	err := sav.Sign(sb.Sign)
	if err != nil {
		return nil, err
	}
	return sav, nil
}

func (sb *Backend) gossipSignedAnnounceVersionsMsg(signedAnnVersions []*signedAnnounceVersion) error {
	logger := sb.logger.New("func", "gossipSignedAnnounceVersionsMsg")

	payload, err := rlp.EncodeToBytes(signedAnnVersions)
	if err != nil {
		logger.Warn("Error encoding entries", "err", err)
		return err
	}
	return sb.Multicast(nil, payload, istanbulSignedAnnounceVersionsMsg)
}

func (sb *Backend) getAllSignedAnnounceVersions() ([]*signedAnnounceVersion, error) {
	allEntries, err := sb.signedAnnounceVersionTable.GetAll()
	if err != nil {
		return nil, err
	}
	allSignedAnnounceVersions := make([]*signedAnnounceVersion, len(allEntries))
	for i, entry := range allEntries {
		allSignedAnnounceVersions[i] = newSignedAnnounceVersionFromEntry(entry)
	}
	return allSignedAnnounceVersions, nil
}

// sendAnnounceVersionTable sends all SignedAnnounceVersions this node
// has to a peer
func (sb *Backend) sendAnnounceVersionTable(peer consensus.Peer) error {
	logger := sb.logger.New("func", "sendAnnounceVersionTable")
	allSignedAnnounceVersions, err := sb.getAllSignedAnnounceVersions()
	if err != nil {
		logger.Warn("Error getting all signed announce versions", "err", err)
		return err
	}
	payload, err := rlp.EncodeToBytes(allSignedAnnounceVersions)
	if err != nil {
		logger.Warn("Error encoding entries", "err", err)
		return err
	}
	return peer.Send(istanbulSignedAnnounceVersionsMsg, payload)
}

func (sb *Backend) handleSignedAnnounceVersionsMsg(peer consensus.Peer, payload []byte) error {
	logger := sb.logger.New("func", "handleSignedAnnounceVersionsMsg")
	logger.Trace("Handling signed announce version msg")
	var signedAnnVersions []*signedAnnounceVersion

	err := rlp.DecodeBytes(payload, &signedAnnVersions)
	if err != nil {
		logger.Warn("Error in decoding received Signed Announce Versions msg", "err", err)
		return err
	}

	// If the announce's valAddress is not within the validator connection set, then ignore it
	validatorConnSet, err := sb.retrieveValidatorConnSet()
	if err != nil {
		logger.Trace("Error in retrieving validator conn set", "err", err)
		return err
	}

	var validEntries []*vet.SignedAnnounceVersionEntry
	validAddresses := make(map[common.Address]bool)
	// Verify all entries are valid and remove duplicates
	for _, signedAnnVersion := range signedAnnVersions {
		err := signedAnnVersion.ValidateSignature()
		if err != nil {
			logger.Debug("Error validating signed announce version signature", "address", signedAnnVersion.Address, "err", err)
			continue
		}
		if !validatorConnSet[signedAnnVersion.Address] {
			logger.Debug("Found signed announce version from an address not in the validator conn set", "address", signedAnnVersion.Address)
			continue
		}
		if _, ok := validAddresses[signedAnnVersion.Address]; ok {
			logger.Debug("Found duplicate signed announce version in message", "address", signedAnnVersion.Address)
			continue
		}
		validAddresses[signedAnnVersion.Address] = true
		validEntries = append(validEntries, signedAnnVersion.Entry())
	}
	return sb.upsertSignedAnnounceVersionEntries(validEntries)
}

func (sb *Backend) upsertSignedAnnounceVersionEntries(entries []*vet.SignedAnnounceVersionEntry) error {
	logger := sb.logger.New("func", "upsertSignedAnnounceVersions")
	newEntries, err := sb.signedAnnounceVersionTable.Upsert(entries)
	if err != nil {
		logger.Warn("Error in upserting entries", "err", err)
	}

	// If this node is a validator (checked later as a result of this call) and it receives a signed announce
	// version from a remote validator that is newer than the remote validator's
	// version in the val enode table, this node did not receive a direct announce
	// and needs to announce its own enode to the remote validator.
	sb.startGossipAnnounceTask()

	// Only regossip entries that do not originate from an address that we have
	// gossiped a signed announce version for within the last 5 minutes, excluding
	// our own address.
	var signedAnnVersionsToRegossip []*signedAnnounceVersion
	sb.lastSignedAnnounceVersionsGossipedMu.Lock()
	for _, entry := range newEntries {
		lastGossipTime, ok := sb.lastSignedAnnounceVersionsGossiped[entry.Address]
		if ok && time.Since(lastGossipTime) >= signedAnnounceVersionGossipCooldownDuration && entry.Address != sb.ValidatorAddress() {
			continue
		}
		signedAnnVersionsToRegossip = append(signedAnnVersionsToRegossip, &signedAnnounceVersion{
			Address: entry.Address,
			Version: entry.Version,
			Signature: entry.Signature,
		})
		sb.lastSignedAnnounceVersionsGossiped[entry.Address] = time.Now()
	}
	sb.lastSignedAnnounceVersionsGossipedMu.Unlock()
	if len(signedAnnVersionsToRegossip) > 0 {
		return sb.gossipSignedAnnounceVersionsMsg(signedAnnVersionsToRegossip)
	}
	return nil
}

// updateAnnounceVersion will synchronously update the announce version.
// Must be called in a separate goroutine from the announceThread to avoid
// a deadlock.
func (sb *Backend) updateAnnounceVersion() {
	sb.updateAnnounceVersionCh <- struct{}{}
	<-sb.updateAnnounceVersionCompleteCh
}

// setAndShareUpdatedAnnounceVersion generates announce data structures and
// and shares them with relevant nodes.
// It will:
//  1) Generate a new enode certificate
//  2) Send the new enode certificate to this node's proxy if one exists
//  3) Send the new enode certificate to all peers in the validator conn set
//  4) Generate a new signed announce version
//  5) Gossip the new signed announce version to all peers
func (sb *Backend) setAndShareUpdatedAnnounceVersion(version uint) error {
	logger := sb.logger.New("func", "setAndShareUpdatedAnnounceVersion")
	// Send new versioned enode msg to all other registered or elected validators
	validatorConnSet, err := sb.retrieveValidatorConnSet()
	if err != nil {
		return err
	}
	// Don't send any messages if this node is not registered or elected
	if !validatorConnSet[sb.Address()] {
		logger.Trace("Not registered or elected, not updating announce version")
		return nil
	}
	enodeCertificateMsg, err := sb.generateEnodeCertificateMsg(version)
	if err != nil {
		return err
	}
	sb.setEnodeCertificateMsg(enodeCertificateMsg)
	// Send the new versioned enode msg to the proxy peer
	if sb.config.Proxied && sb.proxyNode != nil && sb.proxyNode.peer != nil {
		err := sb.sendEnodeCertificateMsg(sb.proxyNode.peer, enodeCertificateMsg)
		if err != nil {
			logger.Error("Error in sending versioned enode msg to proxy", "err", err)
			return err
		}
	}
	payload, err := enodeCertificateMsg.Payload()
	if err != nil {
		return err
	}
	destAddresses := make([]common.Address, len(validatorConnSet))
	i := 0
	for address := range validatorConnSet {
		destAddresses[i] = address
		i++
	}
	err = sb.Multicast(destAddresses, payload, istanbulEnodeCertificateMsg)
	if err != nil {
		return err
	}

	// Generate and gossip a new signed announce version
	newSignedAnnVersion, err := sb.generateSignedAnnounceVersion(version)
	if err != nil {
		return err
	}
	return sb.upsertSignedAnnounceVersionEntries([]*vet.SignedAnnounceVersionEntry{
		newSignedAnnVersion.Entry(),
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

func newAnnounceVersion() uint {
	// Unix() returns a int64, but we need a uint for the golang rlp encoding implmentation. Warning: This timestamp value will be truncated in 2106.
	return uint(time.Now().Unix())
}

type enodeCertificate struct {
	EnodeURL string
	Version  uint
}

// ==============================================
//
// define the functions that needs to be provided for rlp Encoder/Decoder.

// EncodeRLP serializes ec into the Ethereum RLP format.
func (ec *enodeCertificate) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, []interface{}{ec.EnodeURL, ec.Version})
}

// DecodeRLP implements rlp.Decoder, and load the ec fields from a RLP stream.
func (ec *enodeCertificate) DecodeRLP(s *rlp.Stream) error {
	var msg struct {
		EnodeURL string
		Version  uint
	}

	if err := s.Decode(&msg); err != nil {
		return err
	}
	ec.EnodeURL, ec.Version = msg.EnodeURL, msg.Version
	return nil
}

// retrieveEnodeCertificateMsg gets the most recent enode certificate message.
// May be nil if no message was generated as a result of the core not being
// started, or if a proxy has not received a message from its proxied validator
func (sb *Backend) retrieveEnodeCertificateMsg() (*istanbul.Message, error) {
	sb.enodeCertificateMsgMu.Lock()
	defer sb.enodeCertificateMsgMu.Unlock()
	if sb.enodeCertificateMsg == nil {
		return nil, nil
	}
	return sb.enodeCertificateMsg.Copy(), nil
}

// generateEnodeCertificateMsg generates an enode certificate message with the enode
// this node is publicly accessible at. If this node is proxied, the proxy's
// public enode is used.
func (sb *Backend) generateEnodeCertificateMsg(version uint) (*istanbul.Message, error) {
	logger := sb.logger.New("func", "generateEnodeCertificateMsg")

	var enodeURL string
	if sb.config.Proxied {
		if sb.proxyNode != nil {
			enodeURL = sb.proxyNode.externalNode.URLv4()
		} else {
			return nil, errNoProxyConnection
		}
	} else {
		enodeURL = sb.p2pserver.Self().URLv4()
	}

	enodeCertificate := &enodeCertificate{
		EnodeURL: enodeURL,
		Version:  version,
	}
	enodeCertificateBytes, err := rlp.EncodeToBytes(enodeCertificate)
	if err != nil {
		return nil, err
	}
	msg := &istanbul.Message{
		Code:    istanbulEnodeCertificateMsg,
		Address: sb.Address(),
		Msg:     enodeCertificateBytes,
	}
	// Sign the message
	if err := msg.Sign(sb.Sign); err != nil {
		return nil, err
	}
	logger.Trace("Generated Istanbul Enode Certificate message", "enodeCertificate", enodeCertificate, "address", msg.Address)
	return msg, nil
}

// handleEnodeCertificateMsg handles an enode certificate message.
// At the moment, this message is only supported if it's sent from a proxied
// validator to its proxy or vice versa. If the message is sent by the proxy
// to the proxied validator, the proxy is forwarding an enodeCertificate to the
// proxied validator.
// If a message is sent by a proxied validator to its proxy, it is either
// sending its own enodeCertificate that can be used during handshakes, or sending
// an enodeCertificate to a proxy that was forwarded by a proxy before.
func (sb *Backend) handleEnodeCertificateMsg(peer consensus.Peer, payload []byte) error {
	logger := sb.logger.New("func", "handleEnodeCertificateMsg")

	var msg istanbul.Message
	// Decode payload into msg
	err := msg.FromPayload(payload, istanbul.GetSignatureAddress)
	if err != nil {
		logger.Error("Error in decoding received Istanbul Enode Certificate message", "err", err, "payload", hex.EncodeToString(payload))
		return err
	}
	logger = logger.New("msg address", msg.Address)

	var enodeCertificate enodeCertificate
	if err := rlp.DecodeBytes(msg.Msg, &enodeCertificate); err != nil {
		logger.Warn("Error in decoding received Istanbul Enode Certificate message content", "err", err, "IstanbulMsg", msg.String())
		return err
	}
	logger.Trace("Received Istanbul Enode Certificate message", "enodeCertificate", enodeCertificate)

	parsedNode, err := enode.ParseV4(enodeCertificate.EnodeURL)
	if err != nil {
		logger.Warn("Malformed v4 node in received Istanbul Enode Certificate message", "enodeCertificate", enodeCertificate, "err", err)
		return err
	}

	isFromProxiedPeer := sb.config.Proxy && sb.proxiedPeer != nil && sb.proxiedPeer.Node().ID() == peer.Node().ID()

	// Handle the special case where this node is a proxy and the proxied validator sent the msg
	if isFromProxiedPeer && msg.Address == sb.config.ProxiedValidatorAddress {
		existingVersion := sb.getEnodeCertificateMsgVersion()
		if enodeCertificate.Version < existingVersion {
			logger.Warn("Enode certificate from proxied peer contains version lower than existing enode msg", "msg version", enodeCertificate.Version, "existing", existingVersion)
			return errors.New("Version too low")
		}
		// There may be a difference in the URLv4 string because of `discport`,
		// so instead compare the ID
		selfNode := sb.p2pserver.Self()
		if parsedNode.ID() != selfNode.ID() {
			logger.Warn("Received Istanbul Enode Certificate message with an incorrect enode url", "message enode url", enodeCertificate.EnodeURL, "self enode url", sb.p2pserver.Self().URLv4())
			return errors.New("Incorrect enode url")
		}
		if err := sb.setEnodeCertificateMsg(&msg); err != nil {
			logger.Warn("Error setting enode certificate msg", "err", err)
			return err
		}
		return nil
	}

	validatorConnSet, err := sb.retrieveValidatorConnSet()
	if err != nil {
		logger.Debug("Error in retrieving registered/elected valset", "err", err)
		return err
	}

	if !validatorConnSet[msg.Address] {
		logger.Debug("Received Istanbul Enode Certificate message originating from a node not in the validator conn set")
		return errUnauthorizedAnnounceMessage
	}

	if err := sb.valEnodeTable.Upsert(map[common.Address]*vet.AddressEntry{msg.Address: {Node: parsedNode, Version: enodeCertificate.Version}}); err != nil {
		logger.Warn("Error in upserting a val enode table entry", "error", err)
		return err
	}
	return nil
}

func (sb *Backend) sendEnodeCertificateMsg(peer consensus.Peer, msg *istanbul.Message) error {
	logger := sb.logger.New("func", "sendEnodeCertificateMsg")
	payload, err := msg.Payload()
	if err != nil {
		logger.Error("Error getting payload of enode certificate message", "err", err)
		return err
	}
	return peer.Send(istanbulEnodeCertificateMsg, payload)
}

func (sb *Backend) setEnodeCertificateMsg(msg *istanbul.Message) error {
	sb.enodeCertificateMsgMu.Lock()
	var enodeCertificate enodeCertificate
	if err := rlp.DecodeBytes(msg.Msg, &enodeCertificate); err != nil {
		return err
	}
	sb.enodeCertificateMsg = msg
	sb.enodeCertificateMsgVersion = enodeCertificate.Version
	sb.enodeCertificateMsgMu.Unlock()
	return nil
}

func (sb *Backend) getEnodeCertificateMsgVersion() uint {
	sb.enodeCertificateMsgMu.RLock()
	defer sb.enodeCertificateMsgMu.RUnlock()
	return sb.enodeCertificateMsgVersion
}
