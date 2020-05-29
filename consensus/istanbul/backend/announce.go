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
	"math"
	"time"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/consensus/istanbul"
	vet "github.com/ethereum/go-ethereum/consensus/istanbul/backend/internal/enodes"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/crypto/ecies"
	"github.com/ethereum/go-ethereum/metrics"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/rlp"
)

// ==============================================
//
// define the constants and function for the sendAnnounce thread

const (
	queryEnodeGossipCooldownDuration         = 5 * time.Minute
	versionCertificateGossipCooldownDuration = 5 * time.Minute
)

// QueryEnodeGossipFrequencyState specifies how frequently to gossip query enode messages
type QueryEnodeGossipFrequencyState int

const (
	// HighFreqBeforeFirstPeerState will send out a query enode message every 1 minute until the first peer is established
	HighFreqBeforeFirstPeerState QueryEnodeGossipFrequencyState = iota

	// HighFreqAfterFirstPeerState will send out an query enode message every 1 minute for the first 10 query enode messages after the first peer is established.
	// This is on the assumption that when this node first establishes a peer, the p2p network that this node is in may
	// be partitioned with the broader p2p network. We want to give that p2p network some time to connect to the broader p2p network.
	HighFreqAfterFirstPeerState

	// LowFreqState will send out an query every config.AnnounceQueryEnodeGossipPeriod seconds
	LowFreqState
)

// The announceThread will:
// 1) Periodically poll to see if this node should be announcing
// 2) Periodically share the entire version certificate table with all peers
// 3) Periodically prune announce-related data structures
// 4) Gossip announce messages periodically when requested
// 5) Update announce version when requested
func (sb *Backend) announceThread() {
	logger := sb.logger.New("func", "announceThread")

	sb.announceThreadWg.Add(1)
	defer sb.announceThreadWg.Done()

	// Create a ticker to poll if istanbul core is running and if this node is in
	// the validator conn set. If both conditions are true, then this node should announce.
	checkIfShouldAnnounceTicker := time.NewTicker(5 * time.Second)
	// Occasionally share the entire version certificate table with all peers
	shareVersionCertificatesTicker := time.NewTicker(5 * time.Minute)
	pruneAnnounceDataStructuresTicker := time.NewTicker(10 * time.Minute)

	var queryEnodeTicker *time.Ticker
	var queryEnodeTickerCh <-chan time.Time
	var queryEnodeFrequencyState QueryEnodeGossipFrequencyState
	var currentQueryEnodeTickerDuration time.Duration
	var numQueryEnodesInHighFreqAfterFirstPeerState int
	// TODO: this can be removed once we have more faith in this protocol
	var updateAnnounceVersionTicker *time.Ticker
	var updateAnnounceVersionTickerCh <-chan time.Time

	var announceVersion uint
	var announcing bool
	var shouldAnnounce bool
	var err error

	updateAnnounceVersionFunc := func() {
		version := getTimestamp()
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

			shouldAnnounce, err = sb.shouldSaveAndPublishValEnodeURLs()
			if err != nil {
				logger.Warn("Error in checking if should announce", err)
				break
			}

			if shouldAnnounce && !announcing {
				updateAnnounceVersionFunc()
				// Gossip the announce after a minute.
				// The delay allows for all receivers of the announce message to
				// have a more up-to-date cached registered/elected valset, and
				// hence more likely that they will be aware that this node is
				// within that set.
				waitPeriod := 1 * time.Minute
				if sb.config.Epoch <= 10 {
					waitPeriod = 5 * time.Second
				}
				time.AfterFunc(waitPeriod, func() {
					sb.startGossipQueryEnodeTask()
				})

				if sb.config.AnnounceAggressiveQueryEnodeGossipOnEnablement {
					queryEnodeFrequencyState = HighFreqBeforeFirstPeerState
					// Send an query enode message once a minute
					currentQueryEnodeTickerDuration = 1 * time.Minute
					numQueryEnodesInHighFreqAfterFirstPeerState = 0
				} else {
					queryEnodeFrequencyState = LowFreqState
					currentQueryEnodeTickerDuration = time.Duration(sb.config.AnnounceQueryEnodeGossipPeriod) * time.Second
				}

				// Enable periodic gossiping by setting announceGossipTickerCh to non nil value
				queryEnodeTicker = time.NewTicker(currentQueryEnodeTickerDuration)
				queryEnodeTickerCh = queryEnodeTicker.C

				updateAnnounceVersionTicker = time.NewTicker(5 * time.Minute)
				updateAnnounceVersionTickerCh = updateAnnounceVersionTicker.C

				announcing = true
				logger.Trace("Enabled periodic gossiping of announce message")
			} else if !shouldAnnounce && announcing {
				// Disable periodic queryEnode msgs by setting queryEnodeTickerCh to nil
				queryEnodeTicker.Stop()
				queryEnodeTickerCh = nil
				// Disable periodic updating of announce version
				updateAnnounceVersionTicker.Stop()
				updateAnnounceVersionTickerCh = nil

				announcing = false
				logger.Trace("Disabled periodic gossiping of announce message")
			}

		case <-shareVersionCertificatesTicker.C:
			// Send all version certificates to every peer. Only the entries
			// that are new to a node will end up being regossiped throughout the
			// network.
			allVersionCertificates, err := sb.getAllVersionCertificates()
			if err != nil {
				logger.Warn("Error getting all version certificates", "err", err)
				break
			}
			if err := sb.gossipVersionCertificatesMsg(allVersionCertificates); err != nil {
				logger.Warn("Error gossiping all version certificates")
			}

		case <-updateAnnounceVersionTickerCh:
			updateAnnounceVersionFunc()

		case <-queryEnodeTickerCh:
			sb.startGossipQueryEnodeTask()

		case <-sb.generateAndGossipQueryEnodeCh:
			if shouldAnnounce {
				switch queryEnodeFrequencyState {
				case HighFreqBeforeFirstPeerState:
					if len(sb.broadcaster.FindPeers(nil, p2p.AnyPurpose)) > 0 {
						queryEnodeFrequencyState = HighFreqAfterFirstPeerState
					}

				case HighFreqAfterFirstPeerState:
					if numQueryEnodesInHighFreqAfterFirstPeerState >= 10 {
						queryEnodeFrequencyState = LowFreqState
					}
					numQueryEnodesInHighFreqAfterFirstPeerState++

				case LowFreqState:
					if currentQueryEnodeTickerDuration != time.Duration(sb.config.AnnounceQueryEnodeGossipPeriod)*time.Second {
						// Reset the ticker
						currentQueryEnodeTickerDuration = time.Duration(sb.config.AnnounceQueryEnodeGossipPeriod) * time.Second
						queryEnodeTicker.Stop()
						queryEnodeTicker = time.NewTicker(currentQueryEnodeTickerDuration)
						queryEnodeTickerCh = queryEnodeTicker.C
					}
				}
				// This node may have recently sent out an announce message within
				// the gossip cooldown period imposed by other nodes.
				// Regardless, send the queryEnode so that it will at least be
				// processed by this node's peers. This is especially helpful when a network
				// is first starting up.
				if err := sb.generateAndGossipQueryEnode(announceVersion, queryEnodeFrequencyState == LowFreqState); err != nil {
					logger.Warn("Error in generating and gossiping queryEnode", "err", err)
				}
			}

		case <-sb.updateAnnounceVersionCh:
			updateAnnounceVersionFunc()
			// Show that the announce update has been completed so we can rely on
			// it synchronously
			sb.updateAnnounceVersionCompleteCh <- struct{}{}

		case <-pruneAnnounceDataStructuresTicker.C:
			if err := sb.pruneAnnounceDataStructures(); err != nil {
				logger.Warn("Error in pruning announce data structures", "err", err)
			}

		case <-sb.announceThreadQuit:
			checkIfShouldAnnounceTicker.Stop()
			pruneAnnounceDataStructuresTicker.Stop()
			if announcing {
				queryEnodeTicker.Stop()
				updateAnnounceVersionTicker.Stop()
			}
			return
		}
	}
}

// startGossipQueryEnodeTask will schedule a task for the announceThread to
// generate and gossip a queryEnode message
func (sb *Backend) startGossipQueryEnodeTask() {
	// sb.generateAndGossipQueryEnodeCh has a buffer of 1. If there is a value
	// already sent to the channel that has not been read from, don't block.
	select {
	case sb.generateAndGossipQueryEnodeCh <- struct{}{}:
	default:
	}
}

func (sb *Backend) shouldSaveAndPublishValEnodeURLs() (bool, error) {

	// Check if this node is in the validator connection set
	validatorConnSet, err := sb.retrieveValidatorConnSet()
	if err != nil {
		return false, err
	}

	return sb.coreStarted && validatorConnSet[sb.Address()], nil
}

// pruneAnnounceDataStructures will remove entries that are not in the validator connection set from all announce related data structures.
// The data structures that it prunes are:
// 1)  lastQueryEnodeGossiped
// 2)  valEnodeTable
// 3)  lastVersionCertificatesGossiped
// 4)  versionCertificateTable
func (sb *Backend) pruneAnnounceDataStructures() error {
	logger := sb.logger.New("func", "pruneAnnounceDataStructures")

	// retrieve the validator connection set
	validatorConnSet, err := sb.retrieveValidatorConnSet()
	if err != nil {
		return err
	}

	sb.lastQueryEnodeGossipedMu.Lock()
	for remoteAddress := range sb.lastQueryEnodeGossiped {
		if !validatorConnSet[remoteAddress] && time.Since(sb.lastQueryEnodeGossiped[remoteAddress]) >= queryEnodeGossipCooldownDuration {
			logger.Trace("Deleting entry from lastQueryEnodeGossiped", "address", remoteAddress, "gossip timestamp", sb.lastQueryEnodeGossiped[remoteAddress])
			delete(sb.lastQueryEnodeGossiped, remoteAddress)
		}
	}
	sb.lastQueryEnodeGossipedMu.Unlock()

	if err := sb.valEnodeTable.PruneEntries(validatorConnSet); err != nil {
		logger.Trace("Error in pruning valEnodeTable", "err", err)
		return err
	}

	sb.lastVersionCertificatesGossipedMu.Lock()
	for remoteAddress := range sb.lastVersionCertificatesGossiped {
		if !validatorConnSet[remoteAddress] && time.Since(sb.lastVersionCertificatesGossiped[remoteAddress]) >= versionCertificateGossipCooldownDuration {
			logger.Trace("Deleting entry from lastVersionCertificatesGossiped", "address", remoteAddress, "gossip timestamp", sb.lastVersionCertificatesGossiped[remoteAddress])
			delete(sb.lastVersionCertificatesGossiped, remoteAddress)
		}
	}
	sb.lastVersionCertificatesGossipedMu.Unlock()

	if err := sb.versionCertificateTable.Prune(validatorConnSet); err != nil {
		logger.Trace("Error in pruning versionCertificateTable", "err", err)
		return err
	}

	return nil
}

// ===============================================================
//
// define the IstanbulQueryEnode message format, the QueryEnodeMsgCache entries, the queryEnode send function (both the gossip version and the "retrieve from cache" version), and the announce get function

type encryptedEnodeURL struct {
	DestAddress       common.Address
	EncryptedEnodeURL []byte
}

func (ee *encryptedEnodeURL) String() string {
	return fmt.Sprintf("{DestAddress: %s, EncryptedEnodeURL length: %d}", ee.DestAddress.String(), len(ee.EncryptedEnodeURL))
}

type queryEnodeData struct {
	EncryptedEnodeURLs []*encryptedEnodeURL
	Version            uint
	// The timestamp of the node when the message is generated.
	// This results in a new hash for a newly generated message so it gets regossiped by other nodes
	Timestamp uint
}

func (qed *queryEnodeData) String() string {
	return fmt.Sprintf("{Version: %v, Timestamp: %v, EncryptedEnodeURLs: %v}", qed.Version, qed.Timestamp, qed.EncryptedEnodeURLs)
}

// ==============================================
//
// define the functions that needs to be provided for rlp Encoder/Decoder.

// EncodeRLP serializes ar into the Ethereum RLP format.
func (ee *encryptedEnodeURL) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, []interface{}{ee.DestAddress, ee.EncryptedEnodeURL})
}

// DecodeRLP implements rlp.Decoder, and load the ar fields from a RLP stream.
func (ee *encryptedEnodeURL) DecodeRLP(s *rlp.Stream) error {
	var msg struct {
		DestAddress       common.Address
		EncryptedEnodeURL []byte
	}

	if err := s.Decode(&msg); err != nil {
		return err
	}
	ee.DestAddress, ee.EncryptedEnodeURL = msg.DestAddress, msg.EncryptedEnodeURL
	return nil
}

// EncodeRLP serializes ad into the Ethereum RLP format.
func (qed *queryEnodeData) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, []interface{}{qed.EncryptedEnodeURLs, qed.Version, qed.Timestamp})
}

// DecodeRLP implements rlp.Decoder, and load the ad fields from a RLP stream.
func (qed *queryEnodeData) DecodeRLP(s *rlp.Stream) error {
	var msg struct {
		EncryptedEnodeURLs []*encryptedEnodeURL
		Version            uint
		Timestamp          uint
	}

	if err := s.Decode(&msg); err != nil {
		return err
	}
	qed.EncryptedEnodeURLs, qed.Version, qed.Timestamp = msg.EncryptedEnodeURLs, msg.Version, msg.Timestamp
	return nil
}

// generateAndGossipAnnounce will generate the lastest announce msg from this node
// and then broadcast it to it's peers, which should then gossip the announce msg
// message throughout the p2p network if there has not been a message sent from
// this node within the last announceGossipCooldownDuration.
// Note that this function must ONLY be called by the announceThread.
func (sb *Backend) generateAndGossipQueryEnode(version uint, enforceRetryBackoff bool) error {
	logger := sb.logger.New("func", "generateAndGossipQueryEnode")
	logger.Trace("generateAndGossipQueryEnode called")

	// Retrieve the set valEnodeEntries (and their publicKeys)
	// for the queryEnode message
	valEnodeEntries, err := sb.getQueryEnodeValEnodeEntries(enforceRetryBackoff)
	if err != nil {
		return err
	}

	queryEnodeDestAddresses := make([]common.Address, 0)
	queryEnodePublicKeys := make([]*ecdsa.PublicKey, 0)
	for _, valEnodeEntry := range valEnodeEntries {
		if valEnodeEntry.PublicKey != nil {
			queryEnodeDestAddresses = append(queryEnodeDestAddresses, valEnodeEntry.Address)
			queryEnodePublicKeys = append(queryEnodePublicKeys, valEnodeEntry.PublicKey)
		}
	}

	if len(queryEnodeDestAddresses) > 0 {
		istMsg, err := sb.generateQueryEnodeMsg(version, queryEnodeDestAddresses, queryEnodePublicKeys)
		if err != nil {
			return err
		}

		if istMsg == nil {
			return nil
		}

		// Convert to payload
		payload, err := istMsg.Payload()
		if err != nil {
			logger.Error("Error in converting Istanbul QueryEnode Message to payload", "QueryEnodeMsg", istMsg.String(), "err", err)
			return err
		}

		if err := sb.Multicast(nil, payload, istanbulQueryEnodeMsg); err != nil {
			return err
		}

		// Add the amount of queryEnode messages to metrics
		// TODO: since Multicast does not return the amount of messages actually sent,
		// we estimate it by getting the same recipient list that multicast uses.
		// However, a better solution would be for Multicast to provide this information,
		// but being an implementation of the istanbul.Backend.Multicast method, the
		// signature cannot be changed without careful consideration on the implications.
		sb.announceMetrics.MarkSentQueryEnodeMsgs(len(sb.getPeersForMessage(nil)))

		if err := sb.valEnodeTable.UpdateQueryEnodeStats(valEnodeEntries); err != nil {
			return err
		}
	}

	return err
}

func (sb *Backend) getQueryEnodeValEnodeEntries(enforceRetryBackoff bool) ([]*vet.AddressEntry, error) {
	logger := sb.logger.New("func", "getQueryEnodeValEnodeEntries")
	valEnodeEntries, err := sb.valEnodeTable.GetAllValEnodes()
	if err != nil {
		return nil, err
	}

	var queryEnodeValEnodeEntries []*vet.AddressEntry
	for address, valEnodeEntry := range valEnodeEntries {
		// Don't generate an announce record for ourselves
		if address == sb.Address() {
			continue
		}

		if valEnodeEntry.Version == valEnodeEntry.HighestKnownVersion {
			continue
		}

		if valEnodeEntry.PublicKey == nil {
			logger.Warn("Cannot generate encrypted enode URL for a val enode entry without a PublicKey", "address", address)
			continue
		}

		if enforceRetryBackoff && valEnodeEntry.NumQueryAttemptsForHKVersion > 0 {
			timeoutFactorPow := math.Min(float64(valEnodeEntry.NumQueryAttemptsForHKVersion-1), 5)
			timeoutMinutes := int64(math.Pow(1.5, timeoutFactorPow) * 5)
			timeoutForQuery := time.Duration(timeoutMinutes) * time.Minute

			if time.Since(*valEnodeEntry.LastQueryTimestamp) < timeoutForQuery {
				continue
			}
		}

		queryEnodeValEnodeEntries = append(queryEnodeValEnodeEntries, valEnodeEntry)
	}

	return queryEnodeValEnodeEntries, nil
}

// generateQueryEnodeMsg returns a queryEnode message from this node with a given version.
func (sb *Backend) generateQueryEnodeMsg(version uint, queryEnodeDestAddresses []common.Address, queryEnodePublicKeys []*ecdsa.PublicKey) (*istanbul.Message, error) {
	logger := sb.logger.New("func", "generateQueryEnodeMsg")

	enodeURL, err := sb.getEnodeURL()
	if err != nil {
		logger.Error("Error getting enode URL", "err", err)
		return nil, err
	}
	encryptedEnodeURLs, err := sb.generateEncryptedEnodeURLs(enodeURL, queryEnodeDestAddresses, queryEnodePublicKeys)
	if err != nil {
		logger.Warn("Error generating encrypted enodeURLs", "err", err)
		return nil, err
	}
	if len(encryptedEnodeURLs) == 0 {
		logger.Trace("No encrypted enodeURLs were generated, will not generate encryptedEnodeMsg")
		return nil, nil
	}
	queryEnodeData := &queryEnodeData{
		EncryptedEnodeURLs: encryptedEnodeURLs,
		Version:            version,
		Timestamp:          getTimestamp(),
	}

	queryEnodeBytes, err := rlp.EncodeToBytes(queryEnodeData)
	if err != nil {
		logger.Error("Error encoding queryEnode content", "QueryEnodeData", queryEnodeData.String(), "err", err)
		return nil, err
	}

	msg := &istanbul.Message{
		Code:      istanbulQueryEnodeMsg,
		Msg:       queryEnodeBytes,
		Address:   sb.Address(),
		Signature: []byte{},
	}

	// Sign the announce message
	if err := msg.Sign(sb.Sign); err != nil {
		logger.Error("Error in signing a QueryEnode Message", "QueryEnodeMsg", msg.String(), "err", err)
		return nil, err
	}

	logger.Debug("Generated a queryEnode message", "IstanbulMsg", msg.String(), "QueryEnodeData", queryEnodeData.String())

	return msg, nil
}

// generateEncryptedEnodeURLs returns the encryptedEnodeURLs intended for validators
// whose entries in the val enode table do not exist or are outdated when compared
// to the version certificate table.
func (sb *Backend) generateEncryptedEnodeURLs(enodeURL string, queryEnodeDestAddresses []common.Address, queryEnodePublicKeys []*ecdsa.PublicKey) ([]*encryptedEnodeURL, error) {
	logger := sb.logger.New("func", "generateEncryptedEnodeURLs")

	var encryptedEnodeURLs []*encryptedEnodeURL
	for i, destAddress := range queryEnodeDestAddresses {
		logger.Info("encrypting enodeURL", "enodeURL", enodeURL, "publicKey", queryEnodePublicKeys[i])
		publicKey := ecies.ImportECDSAPublic(queryEnodePublicKeys[i])
		encEnodeURL, err := ecies.Encrypt(rand.Reader, publicKey, []byte(enodeURL), nil, nil)
		if err != nil {
			logger.Error("Error in encrypting enodeURL", "enodeURL", enodeURL, "publicKey", publicKey)
			return nil, err
		}

		encryptedEnodeURLs = append(encryptedEnodeURLs, &encryptedEnodeURL{
			DestAddress:       destAddress,
			EncryptedEnodeURL: encEnodeURL,
		})
	}

	return encryptedEnodeURLs, nil
}

// This function will handle a queryEnode message.
func (sb *Backend) handleQueryEnodeMsg(peer consensus.Peer, payload []byte) error {
	logger := sb.logger.New("func", "handleQueryEnodeMsg")

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

	var qeData queryEnodeData
	err = rlp.DecodeBytes(msg.Msg, &qeData)
	if err != nil {
		logger.Warn("Error in decoding received Istanbul QueryEnode message content", "err", err, "IstanbulMsg", msg.String())
		return err
	}

	logger = logger.New("msgAddress", msg.Address, "msgVersion", qeData.Version)

	// Do some validation checks on the queryEnodeData
	if isValid, err := sb.validateQueryEnode(msg.Address, &qeData); !isValid || err != nil {
		logger.Warn("Validation of queryEnode message failed", "isValid", isValid, "err", err)
		return err
	}

	// If this is an elected or nearly elected validator and core is started, then process the queryEnode message
	shouldProcess, err := sb.shouldSaveAndPublishValEnodeURLs()
	if err != nil {
		logger.Warn("Error in checking if should process queryEnode", err)
	}

	if shouldProcess {
		logger.Trace("Processing an queryEnode message", "queryEnode records", qeData.EncryptedEnodeURLs)
		for _, encEnodeURL := range qeData.EncryptedEnodeURLs {
			// Only process an encEnodURL intended for this node
			if encEnodeURL.DestAddress != sb.Address() {
				continue
			}
			enodeBytes, err := sb.decryptFn(accounts.Account{Address: sb.Address()}, encEnodeURL.EncryptedEnodeURL, nil, nil)
			if err != nil {
				sb.logger.Warn("Error decrypting endpoint", "err", err, "encEnodeURL.EncryptedEnodeURL", encEnodeURL.EncryptedEnodeURL)
				return err
			}
			enodeURL := string(enodeBytes)
			node, err := enode.ParseV4(enodeURL)
			if err != nil {
				logger.Warn("Error parsing enodeURL", "enodeUrl", enodeURL)
				return err
			}

			sb.announceMetrics.MarkReceivedQueryEnodeResponse()

			// queryEnode messages should only be processed once because selfRecentMessages
			// will cache seen queryEnode messages, so it's safe to answer without any throttling
			if err := sb.answerQueryEnodeMsg(msg.Address, node, qeData.Version); err != nil {
				logger.Warn("Error answering an announce msg", "target node", node.URLv4(), "error", err)
				return err
			}

			break
		}
	}

	// Regossip this queryEnode message
	return sb.regossipQueryEnode(msg, qeData.Version, payload)
}

// answerQueryEnodeMsg will answer a received queryEnode message from an origin
// node. If the origin node is already a peer of any kind, an enodeCertificate will be sent.
// Regardless, the origin node will be upserted into the val enode table
// to ensure this node designates the origin node as a ValidatorPurpose peer.
func (sb *Backend) answerQueryEnodeMsg(address common.Address, node *enode.Node, version uint) error {
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
	if err := sb.valEnodeTable.UpsertVersionAndEnode([]*vet.AddressEntry{{Address: address, Node: node, Version: version}}); err != nil {
		return err
	}
	return nil
}

// validateQueryEnode will do some validation to check the contents of the queryEnode
// message. This is to force all validators that send a queryEnode message to
// create as succint message as possible, and prevent any possible network DOS attacks
// via extremely large queryEnode message.
func (sb *Backend) validateQueryEnode(msgAddress common.Address, qeData *queryEnodeData) (bool, error) {
	logger := sb.logger.New("func", "validateQueryEnode", "msg address", msgAddress)

	// Check if there are any duplicates in the queryEnode message
	var encounteredAddresses = make(map[common.Address]bool)
	for _, encEnodeURL := range qeData.EncryptedEnodeURLs {
		if encounteredAddresses[encEnodeURL.DestAddress] {
			logger.Info("QueryEnode message has duplicate entries", "address", encEnodeURL.DestAddress)
			return false, nil
		}

		encounteredAddresses[encEnodeURL.DestAddress] = true
	}

	// Check if the number of rows in the queryEnodePayload is at most 2 times the size of the current validator connection set.
	// Note that this is a heuristic of the actual size of validator connection set at the time the validator constructed the announce message.
	validatorConnSet, err := sb.retrieveValidatorConnSet()
	if err != nil {
		return false, err
	}

	if len(qeData.EncryptedEnodeURLs) > 2*len(validatorConnSet) {
		logger.Info("Number of queryEnode message encrypted enodes is more than two times the size of the current validator connection set", "num queryEnode enodes", len(qeData.EncryptedEnodeURLs), "reg/elected val set size", len(validatorConnSet))
		return false, err
	}

	return true, nil
}

// regossipQueryEnode will regossip a received querEnode message.
// If this node regossiped a queryEnode from the same source address within the last
// 5 minutes, then it won't regossip. This is to prevent a malicious validator from
// DOS'ing the network with very frequent announce messages.
// This opens an attack vector where any malicious node could continue to gossip
// a previously gossiped announce message from any validator, causing other nodes to regossip and
// enforce the cooldown period for future messages originating from the origin validator.
// This is circumvented by caching the hashes of messages that are regossiped
// with sb.selfRecentMessages to prevent future regossips.
func (sb *Backend) regossipQueryEnode(msg *istanbul.Message, msgTimestamp uint, payload []byte) error {
	logger := sb.logger.New("func", "regossipQueryEnode", "queryEnodeSourceAddress", msg.Address, "msgTimestamp", msgTimestamp)
	sb.lastQueryEnodeGossipedMu.Lock()
	defer sb.lastQueryEnodeGossipedMu.Unlock()

	// Don't throttle messages from our own address so that proxies always regossip
	// query enode messages sent from the proxied validator
	if msg.Address != sb.ValidatorAddress() {
		if lastGossiped, ok := sb.lastQueryEnodeGossiped[msg.Address]; ok {
			if time.Since(lastGossiped) < queryEnodeGossipCooldownDuration {
				logger.Trace("Already regossiped msg from this source address within the cooldown period, not regossiping.")
				return nil
			}
		}
	}

	logger.Trace("Regossiping the istanbul queryEnode message", "IstanbulMsg", msg.String())
	if err := sb.Multicast(nil, payload, istanbulQueryEnodeMsg); err != nil {
		return err
	}

	sb.lastQueryEnodeGossiped[msg.Address] = time.Now()

	return nil
}

// Used as a salt when signing versionCertificate. This is to account for
// the unlikely case where a different signed struct with the same field types
// is used elsewhere and shared with other nodes. If that were to happen, a
// malicious node could try sending the other struct where this struct is used,
// or vice versa. This ensures that the signature is only valid for this struct.
var versionCertificateSalt = []byte("versionCertificate")

// versionCertificate is a signed message from a validator indicating the most
// recent version of its enode.
type versionCertificate vet.VersionCertificateEntry

func newVersionCertificateFromEntry(entry *vet.VersionCertificateEntry) *versionCertificate {
	return &versionCertificate{
		Address:   entry.Address,
		PublicKey: entry.PublicKey,
		Version:   entry.Version,
		Signature: entry.Signature,
	}
}

func (vc *versionCertificate) Sign(signingFn func(data []byte) ([]byte, error)) error {
	payloadToSign, err := vc.payloadToSign()
	if err != nil {
		return err
	}
	vc.Signature, err = signingFn(payloadToSign)
	if err != nil {
		return err
	}
	return nil
}

// RecoverPublicKeyAndAddress recovers the ECDSA public key and corresponding
// address from the Signature
func (vc *versionCertificate) RecoverPublicKeyAndAddress() error {
	payloadToSign, err := vc.payloadToSign()
	if err != nil {
		return err
	}
	payloadHash := crypto.Keccak256(payloadToSign)
	publicKey, err := crypto.SigToPub(payloadHash, vc.Signature)
	if err != nil {
		return err
	}
	address, err := crypto.PubkeyToAddress(*publicKey), nil
	if err != nil {
		return err
	}
	vc.PublicKey = publicKey
	vc.Address = address
	return nil
}

// EncodeRLP serializes versionCertificate into the Ethereum RLP format.
// Only the Version and Signature are encoded, as the public key and address
// can be recovered from the Signature using RecoverPublicKeyAndAddress
func (vc *versionCertificate) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, []interface{}{vc.Version, vc.Signature})
}

// DecodeRLP implements rlp.Decoder, and load the versionCertificate fields from a RLP stream.
// Only the Version and Signature are encoded/decoded, as the public key and address
// can be recovered from the Signature using RecoverPublicKeyAndAddress
func (vc *versionCertificate) DecodeRLP(s *rlp.Stream) error {
	var msg struct {
		Version   uint
		Signature []byte
	}

	if err := s.Decode(&msg); err != nil {
		return err
	}
	vc.Version, vc.Signature = msg.Version, msg.Signature
	return nil
}

func (vc *versionCertificate) Entry() *vet.VersionCertificateEntry {
	return &vet.VersionCertificateEntry{
		Address:   vc.Address,
		PublicKey: vc.PublicKey,
		Version:   vc.Version,
		Signature: vc.Signature,
	}
}

func (vc *versionCertificate) payloadToSign() ([]byte, error) {
	signedContent := []interface{}{versionCertificateSalt, vc.Version}
	payload, err := rlp.EncodeToBytes(signedContent)
	if err != nil {
		return nil, err
	}
	return payload, nil
}

func (sb *Backend) generateVersionCertificate(version uint) (*versionCertificate, error) {
	vc := &versionCertificate{
		Address:   sb.Address(),
		PublicKey: sb.publicKey,
		Version:   version,
	}
	err := vc.Sign(sb.Sign)
	if err != nil {
		return nil, err
	}
	return vc, nil
}

func (sb *Backend) encodeVersionCertificatesMsg(versionCertificates []*versionCertificate) ([]byte, error) {
	payload, err := rlp.EncodeToBytes(versionCertificates)
	if err != nil {
		return nil, err
	}
	msg := &istanbul.Message{
		Code: istanbulVersionCertificatesMsg,
		Msg:  payload,
	}
	msgPayload, err := msg.Payload()
	if err != nil {
		return nil, err
	}
	return msgPayload, nil
}

func (sb *Backend) gossipVersionCertificatesMsg(versionCertificates []*versionCertificate) error {
	logger := sb.logger.New("func", "gossipVersionCertificatesMsg")

	payload, err := sb.encodeVersionCertificatesMsg(versionCertificates)
	if err != nil {
		logger.Warn("Error encoding version certificate msg", "err", err)
		return err
	}
	return sb.Multicast(nil, payload, istanbulVersionCertificatesMsg)
}

func (sb *Backend) getAllVersionCertificates() ([]*versionCertificate, error) {
	allEntries, err := sb.versionCertificateTable.GetAll()
	if err != nil {
		return nil, err
	}
	allVersionCertificates := make([]*versionCertificate, len(allEntries))
	for i, entry := range allEntries {
		allVersionCertificates[i] = newVersionCertificateFromEntry(entry)
	}
	return allVersionCertificates, nil
}

// sendVersionCertificateTable sends all VersionCertificates this node
// has to a peer
func (sb *Backend) sendVersionCertificateTable(peer consensus.Peer) error {
	logger := sb.logger.New("func", "sendVersionCertificateTable")
	allVersionCertificates, err := sb.getAllVersionCertificates()
	if err != nil {
		logger.Warn("Error getting all version certificates", "err", err)
		return err
	}
	payload, err := sb.encodeVersionCertificatesMsg(allVersionCertificates)
	if err != nil {
		logger.Warn("Error encoding version certificate msg", "err", err)
		return err
	}
	return peer.Send(istanbulVersionCertificatesMsg, payload)
}

func (sb *Backend) handleVersionCertificatesMsg(peer consensus.Peer, payload []byte) error {
	logger := sb.logger.New("func", "handleVersionCertificatesMsg")
	logger.Trace("Handling version certificates msg")

	var msg istanbul.Message
	if err := msg.FromPayload(payload, nil); err != nil {
		logger.Error("Error in decoding version certificates message", "err", err, "payload", hex.EncodeToString(payload))
		return err
	}
	logger = logger.New("msg address", msg.Address)

	var versionCertificates []*versionCertificate
	if err := rlp.DecodeBytes(msg.Msg, &versionCertificates); err != nil {
		logger.Warn("Error in decoding received version certificates msg", "err", err)
		return err
	}

	// If the announce's valAddress is not within the validator connection set, then ignore it
	validatorConnSet, err := sb.retrieveValidatorConnSet()
	if err != nil {
		logger.Trace("Error in retrieving validator conn set", "err", err)
		return err
	}

	var validEntries []*vet.VersionCertificateEntry
	validAddresses := make(map[common.Address]bool)
	// Verify all entries are valid and remove duplicates
	for _, versionCertificate := range versionCertificates {
		// The public key and address are not RLP encoded/decoded and must be
		// explicitly recovered.
		if err := versionCertificate.RecoverPublicKeyAndAddress(); err != nil {
			logger.Warn("Error recovering version certificates public key and address from signature", "err", err)
			continue
		}
		if !validatorConnSet[versionCertificate.Address] {
			logger.Debug("Found version certificate from an address not in the validator conn set", "address", versionCertificate.Address)
			continue
		}
		if _, ok := validAddresses[versionCertificate.Address]; ok {
			logger.Debug("Found duplicate version certificate in message", "address", versionCertificate.Address)
			continue
		}
		validAddresses[versionCertificate.Address] = true
		validEntries = append(validEntries, versionCertificate.Entry())
	}
	if err := sb.upsertAndGossipVersionCertificateEntries(validEntries); err != nil {
		logger.Warn("Error upserting and gossiping entries", "err", err)
		return err
	}
	return nil
}

func (sb *Backend) upsertAndGossipVersionCertificateEntries(entries []*vet.VersionCertificateEntry) error {
	logger := sb.logger.New("func", "upsertAndGossipVersionCertificateEntries")

	shouldProcess, err := sb.shouldSaveAndPublishValEnodeURLs()
	if err != nil {
		logger.Warn("Error in checking if should process queryEnode", err)
	}
	if shouldProcess {
		// Update entries in val enode db
		var valEnodeEntries []*vet.AddressEntry
		for _, entry := range entries {
			// Don't add ourselves into the val enode table
			if entry.Address == sb.Address() {
				continue
			}
			// Update the HighestKnownVersion for this address. Upsert will
			// only update this entry if the HighestKnownVersion is greater
			// than the existing one.
			// Also store the PublicKey for future encryption in queryEnode msgs
			valEnodeEntries = append(valEnodeEntries, &vet.AddressEntry{
				Address:             entry.Address,
				PublicKey:           entry.PublicKey,
				HighestKnownVersion: entry.Version,
			})
		}
		if err := sb.valEnodeTable.UpsertHighestKnownVersion(valEnodeEntries); err != nil {
			logger.Warn("Error upserting val enode table entries", "err", err)
		}
	}

	newEntries, err := sb.versionCertificateTable.Upsert(entries)
	if err != nil {
		logger.Warn("Error upserting version certificate table entries", "err", err)
	}

	// Only regossip entries that do not originate from an address that we have
	// gossiped a version certificate for within the last 5 minutes, excluding
	// our own address.
	var versionCertificatesToRegossip []*versionCertificate
	sb.lastVersionCertificatesGossipedMu.Lock()
	for _, entry := range newEntries {
		lastGossipTime, ok := sb.lastVersionCertificatesGossiped[entry.Address]
		if ok && time.Since(lastGossipTime) >= versionCertificateGossipCooldownDuration && entry.Address != sb.ValidatorAddress() {
			continue
		}
		versionCertificatesToRegossip = append(versionCertificatesToRegossip, newVersionCertificateFromEntry(entry))
		sb.lastVersionCertificatesGossiped[entry.Address] = time.Now()
	}
	sb.lastVersionCertificatesGossipedMu.Unlock()
	if len(versionCertificatesToRegossip) > 0 {
		return sb.gossipVersionCertificatesMsg(versionCertificatesToRegossip)
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
//  4) Generate a new version certificate
//  5) Gossip the new version certificate to all peers
func (sb *Backend) setAndShareUpdatedAnnounceVersion(version uint) error {
	logger := sb.logger.New("func", "setAndShareUpdatedAnnounceVersion")
	// Send new versioned enode msg to all other registered or elected validators
	validatorConnSet, err := sb.retrieveValidatorConnSet()
	if err != nil {
		return err
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
	// Don't send any of the following messages if this node is not in the validator conn set
	if !validatorConnSet[sb.Address()] {
		logger.Trace("Not in the validator conn set, not updating announce version")
		return nil
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

	// Generate and gossip a new version certificate
	newVersionCertificate, err := sb.generateVersionCertificate(version)
	if err != nil {
		return err
	}
	return sb.upsertAndGossipVersionCertificateEntries([]*vet.VersionCertificateEntry{
		newVersionCertificate.Entry(),
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

func getTimestamp() uint {
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
// If this node is a proxy and the enode certificate is from a remote validator
// (ie not the proxied validator), this node will forward the enode certificate
// to its proxied validator. If the proxied validator decides this node should process
// the enode certificate and upsert it into its val enode table, the proxied validator
// will send it back to this node.
// If the proxied validator sends an enode certificate for itself to this node,
// this node will set the enode certificate as its own for handshaking.
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

	upsertVersionAndEnode := func() error {
		if err := sb.valEnodeTable.UpsertVersionAndEnode([]*vet.AddressEntry{{Address: msg.Address, Node: parsedNode, Version: enodeCertificate.Version}}); err != nil {
			logger.Warn("Error in upserting a val enode table entry", "error", err)
			return err
		}
		return nil
	}

	if sb.config.Proxy {
		if sb.proxiedPeer == nil {
			logger.Warn("No proxied peer, ignoring message")
			return nil
		}
		if sb.proxiedPeer.Node().ID() == peer.Node().ID() {
			// if this message is from the proxied peer and contains the proxied
			// validator's enodeCertificate, save it for handshake use
			if msg.Address == sb.config.ProxiedValidatorAddress {
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
			// Otherwise, this enode certificate originates from a remote validator
			// but was sent by the proxied validator to be upserted
			return upsertVersionAndEnode()
		}
		// If this message is not from the proxied validator, send it to the
		// proxied validator without upserting it in this node. If the validator
		// decides this proxy should upsert the enodeCertificate, then it
		// will send it back to this node.
		if err := sb.sendEnodeCertificateMsg(sb.proxiedPeer, &msg); err != nil {
			logger.Warn("Error forwarding enodeCertificate to proxied validator", "err", err)
			return err
		}
		return nil
	}
	// Ensure this node is a validator in the validator conn set
	shouldSave, err := sb.shouldSaveAndPublishValEnodeURLs()
	if err != nil {
		logger.Debug("Error checking if should save val enode url", "err", err)
		return err
	}
	if !shouldSave {
		logger.Debug("This node should not save val enode urls, ignoring enodeCertificate")
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

	// Send enode certificate to proxy
	if sb.config.Proxied && sb.proxyNode != nil && sb.proxyNode.peer != nil {
		if err := sb.sendEnodeCertificateMsg(sb.proxyNode.peer, &msg); err != nil {
			logger.Warn("Error sending enodeCertificate back to proxy peer", "err", err)
		}
	}

	return upsertVersionAndEnode()
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

// announceMetrics holds all the announce protocol metric handles
type announceMetrics struct {
	// valenodetable_size: current size of the val_enode_table
	valEnodeTableSizeGauge metrics.Gauge
	// valenodetable_num_stale_entries: number of entries in the val_enode_table where it's version number is less than the known highest version number
	valEnodeTableNumStaleEntriesGauge metrics.Gauge
	// intendedvalconnections_size: current number of intended validator connections
	intendedValConnectionsSizeGauge metrics.Gauge
	// valconnections_size: current number of validator connections
	valConnectionsSizeGauge metrics.Gauge
	// selfenodeurl_update_count: count of number of self enodeURL version updates
	selfEnodeURLUpdateCountCounter metrics.Counter
	// remoteenodeurl_update_count: count of number of remote query enodeURL updates
	remoteEnodeURLUpdateCountCounter metrics.Counter
	// unanswered_queryenoderequest_count: # of queryEnodeURL requests that have not been responded to within 5 minute
	unansweredQueryEnodeRequestCountMeter metrics.Meter
}

func newAnnounceMetrics(prefix string, r metrics.Registry) *announceMetrics {
	return &announceMetrics{
		valEnodeTableSizeGauge:                metrics.NewRegisteredGauge(prefix+"valenodetable_size", r),
		valEnodeTableNumStaleEntriesGauge:     metrics.NewRegisteredGauge(prefix+"valenodetable_num_stale_entries", r),
		intendedValConnectionsSizeGauge:       metrics.NewRegisteredGauge(prefix+"intendedvalconnections_size", r),
		valConnectionsSizeGauge:               metrics.NewRegisteredGauge(prefix+"valconnections_size", r),
		selfEnodeURLUpdateCountCounter:        metrics.NewRegisteredCounter(prefix+"selfenodeurl_update_count", r),
		remoteEnodeURLUpdateCountCounter:      metrics.NewRegisteredCounter(prefix+"remoteenodeurl_update_count", r),
		unansweredQueryEnodeRequestCountMeter: metrics.NewRegisteredMeter(prefix+"unanswered_queryenoderequest_count", r),
	}
}

// GetValidatorEnodeChangeListener creates and returns a new ValidatorEnodeChangeListener which updates this
// announceMetrics stats.
func (am *announceMetrics) GetValidatorEnodeChangeListener(selfAddressProvider func() common.Address) vet.ValidatorEnodeChangeListener {
	return &valEnodeAnnounceMetricsListener{
		announceMetrics:     am,
		selfAddressProvider: selfAddressProvider,
	}
}

// MarkReceivedQueryEnodeResponse registers a response received event in metrics.
func (am *announceMetrics) MarkReceivedQueryEnodeResponse() {
	am.unansweredQueryEnodeRequestCountMeter.Mark(-1)
}

// MarkSentQueryEnodeMsgs registers in metrics that amount query enode msgs were just sent.
func (am *announceMetrics) MarkSentQueryEnodeMsgs(amount int) {
	am.unansweredQueryEnodeRequestCountMeter.Mark(int64(amount))
}

// valEnodeAnnounceMetricsListener implements vet.VersionUpdateEvent.
type valEnodeAnnounceMetricsListener struct {
	announceMetrics     *announceMetrics
	selfAddressProvider func() common.Address
}

func (vem *valEnodeAnnounceMetricsListener) OnChange(vdb *vet.ValidatorEnodeDB) {
	all, _ := vdb.GetAllValEnodes()
	stale, _ := vdb.GetAllStaleValEnodes()
	vem.announceMetrics.valEnodeTableSizeGauge.Update(int64(len(all)))
	vem.announceMetrics.valEnodeTableNumStaleEntriesGauge.Update(int64(len(stale)))
}

func (vem *valEnodeAnnounceMetricsListener) OnVersionUpdated(ev *vet.VersionUpdateEvent) {
	if vem.selfAddressProvider() == ev.Address {
		vem.announceMetrics.selfEnodeURLUpdateCountCounter.Count()
	} else {
		vem.announceMetrics.remoteEnodeURLUpdateCountCounter.Count()
	}
}
