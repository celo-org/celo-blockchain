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
	"time"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/consensus"
	"github.com/celo-org/celo-blockchain/consensus/istanbul"
	"github.com/celo-org/celo-blockchain/consensus/istanbul/validator"
	"github.com/celo-org/celo-blockchain/core/types"
	"github.com/celo-org/celo-blockchain/p2p"
	"github.com/celo-org/celo-blockchain/p2p/enode"
	"github.com/celo-org/celo-blockchain/rlp"
)

var (
	// errDecodeFailed is returned when decode message fails
	errDecodeFailed = errors.New("fail to decode istanbul message")
)

const (
	handshakeTimeout = 5 * time.Second
)

// HandleMsg implements consensus.Handler.HandleMsg
func (sb *Backend) HandleMsg(addr common.Address, msg p2p.Msg, peer consensus.Peer) (bool, error) {
	logger := sb.logger.New("func", "HandleMsg", "msgCode", msg.Code)

	if !istanbul.IsIstanbulMsg(msg) {
		return false, nil
	}

	var data []byte
	if err := msg.Decode(&data); err != nil {
		logger.Error("Failed to decode message payload", "err", err, "from", addr)
		return true, errDecodeFailed
	}

	if sb.IsValidating() {
		// Handle messages as primary validator
		switch msg.Code {
		case istanbul.ConsensusMsg:
			go sb.istanbulEventMux.Post(istanbul.MessageEvent{
				Payload: data,
			})
			return true, nil
		case istanbul.EnodeCertificateMsg:
			go sb.announceManager.HandleEnodeCertificateMsg(peer, data)
			return true, nil
		case istanbul.QueryEnodeMsg:
			go sb.announceManager.HandleQueryEnodeMsg(addr, peer, data)
			return true, nil
		case istanbul.VersionCertificatesMsg:
			go sb.announceManager.HandleVersionCertificatesMsg(addr, peer, data)
			return true, nil
		case istanbul.ValidatorHandshakeMsg:
			logger.Warn("Received unexpected Istanbul validator handshake message")
			return true, nil
		default:
			logger.Error("Unhandled istanbul message as primary", "address", addr, "peer's enodeURL", peer.Node().String(), "ethMsgCode", msg.Code)
			return false, nil
		}
	} else if !sb.IsValidating() {
		// Handle messages as replica validator
		switch msg.Code {
		case istanbul.ConsensusMsg:
			// Ignore consensus messages
			return true, nil
		case istanbul.EnodeCertificateMsg:
			go sb.announceManager.HandleEnodeCertificateMsg(peer, data)
			return true, nil
		case istanbul.QueryEnodeMsg:
			go sb.announceManager.HandleQueryEnodeMsg(addr, peer, data)
			return true, nil
		case istanbul.VersionCertificatesMsg:
			go sb.announceManager.HandleVersionCertificatesMsg(addr, peer, data)
			return true, nil
		case istanbul.ValidatorHandshakeMsg:
			logger.Warn("Received unexpected Istanbul validator handshake message")
			return true, nil
		default:
			logger.Error("Unhandled istanbul message as replica", "address", addr, "peer's enodeURL", peer.Node().String(), "ethMsgCode", msg.Code)
			return false, nil
		}
	}

	// If we got here, then that means that there is an istanbul message type that either there
	// is an istanbul message that is not handled.
	logger.Error("Unhandled istanbul message", "address", addr, "peer's enodeURL", peer.Node().String(), "ethMsgCode", msg.Code)
	return false, nil
}

// SetBroadcaster implements consensus.Handler.SetBroadcaster
func (sb *Backend) SetBroadcaster(broadcaster consensus.Broadcaster) {
	sb.broadcaster = broadcaster
}

// SetP2PServer implements consensus.Handler.SetP2PServer
func (sb *Backend) SetP2PServer(p2pserver consensus.P2PServer) {
	sb.p2pserver = p2pserver
}

// NewWork is called by miner/worker.go whenever it's mainLoop gets a newWork event.
func (sb *Backend) NewWork() error {
	sb.logger.Debug("NewWork called, acquiring core lock", "func", "NewWork")

	sb.coreMu.RLock()
	defer sb.coreMu.RUnlock()
	if !sb.isCoreStarted() {
		return istanbul.ErrStoppedEngine
	}

	sb.logger.Debug("Posting FinalCommittedEvent", "func", "NewWork")

	go sb.istanbulEventMux.Post(istanbul.FinalCommittedEvent{})
	return nil
}

// UpdateMetricsForParentOfBlock maintains metrics around the *parent* of the supplied block.
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
	gpValSetIndex, _ := gpValSet.GetByAddress(sb.Address())

	// Now check if this validator signer is in the "parent seal" on the child block.
	// The parent seal is used for downtime calculations.
	childExtra, err := child.Header().IstanbulExtra()
	if err != nil {
		return
	}

	// valSetSize is the total number of possible signers.
	valSetSize := gpValSet.Size()
	sb.blocksValSetSizeGauge.Update(int64(valSetSize))

	// countInParentSeal is the number of signers represented in the parent seal of the child block.
	countInParentSeal := 0
	for i := 0; i < gpValSet.Size(); i++ {
		countInParentSeal += int(childExtra.ParentAggregatedSeal.Bitmap.Bit(i))
	}
	sb.blocksTotalSigsGauge.Update(int64(countInParentSeal))

	// Cumulative count of rounds missed (i.e sequences not agreed on round=0)
	missedRounds := childExtra.ParentAggregatedSeal.Round.Int64()
	if missedRounds > 0 {
		sb.blocksTotalMissedRoundsMeter.Mark(missedRounds)
	}

	// Is this validator signer elected?
	elected := gpValSetIndex >= 0
	if !elected {
		sb.blocksElectedButNotSignedGauge.Update(0)
		return
	}
	sb.blocksElectedMeter.Mark(1)

	// The following metrics are only tracked if the validator is elected.

	// Did this validator propose the block that was finalized?
	if parentHeader.Coinbase == sb.Address() {
		sb.blocksElectedAndProposedMeter.Mark(1)
	} else {
		// could have proposed a block that was not finalized?
		// This is a could, not did because the consensus algo may have forced the proposer
		// to re-propose an existing block, thus not placing it's own signature on it.
		gpAuthor := sb.AuthorForBlock(number - 2)
		for i := int64(0); i < missedRounds; i++ {
			proposer := validator.GetProposerSelector(sb.config.ProposerPolicy)(gpValSet, gpAuthor, uint64(i))
			if sb.Address() == proposer.Address() {
				sb.blocksMissedRoundsAsProposerMeter.Mark(1)
				break
			}
		}
	}

	// signed, or missed?
	inParentSeal := childExtra.ParentAggregatedSeal.Bitmap.Bit(gpValSetIndex) != 0
	if inParentSeal {
		sb.blocksElectedAndSignedMeter.Mark(1)
		sb.blocksElectedButNotSignedGauge.Update(0)
	} else {
		sb.blocksElectedButNotSignedMeter.Mark(1)
		sb.blocksElectedButNotSignedGauge.Inc(1)
		if sb.blocksElectedButNotSignedGauge.Value() != 0 {
			sb.logger.Warn("Elected but didn't sign block", "number", number-1, "address", sb.ValidatorAddress(), "missed in a row", sb.blocksElectedButNotSignedGauge.Value())
		} else {
			sb.logger.Warn("Elected but didn't sign block", "number", number-1, "address", sb.ValidatorAddress())
		}
	}

	// Clear downtime counter on end of epoch.
	if istanbul.IsLastBlockOfEpoch(number-1, sb.config.Epoch) {
		sb.blocksElectedButNotSignedGauge.Update(0)
	}
}

// Actions triggered by a new block being added to the chain.
func (sb *Backend) NewChainHead(newBlock *types.Block) {

	sb.logger.Trace("Start NewChainHead", "number", newBlock.Number().Uint64())

	// Update metrics for whether we were elected and signed the parent of this block.
	sb.UpdateMetricsForParentOfBlock(newBlock)

	// If this is the last block of the epoch:
	// * Print an easy to find log message giving our address and whether we're elected in next epoch.
	// * If this is a node maintaining validator connections (e.g. a proxy or a standalone validator), refresh the validator enode table.
	// * If this is a proxied validator, notify the proxied validator engine of a new epoch.
	if istanbul.IsLastBlockOfEpoch(newBlock.Number().Uint64(), sb.config.Epoch) {

		sb.coreMu.RLock()
		defer sb.coreMu.RUnlock()

		valSet := sb.getValidators(newBlock.Number().Uint64(), newBlock.Hash())
		valSetIndex, _ := valSet.GetByAddress(sb.ValidatorAddress())

		sb.logger.Info("Validator Election Results", "address", sb.ValidatorAddress(), "elected", valSetIndex >= 0, "number", newBlock.Number().Uint64())

		// We lock here (inside IsAnnounceRunning) to protect access to announceRunning because
		// announceRunning is also accessed in StartAnnouncing and
		// StopAnnouncing.
		if sb.announceManager.IsAnnounceRunning() {
			sb.logger.Trace("At end of epoch and going to refresh validator peers", "new_block_number", newBlock.Number().Uint64())
			if err := sb.RefreshValPeers(); err != nil {
				sb.logger.Warn("Error refreshing validator peers", "err", err)
			}
		}
	}

	sb.blocksFinalizedTransactionsGauge.Update(int64(len(newBlock.Transactions())))
	sb.blocksFinalizedGasUsedGauge.Update(int64(newBlock.GasUsed()))
	sb.logger.Trace("End NewChainHead", "number", newBlock.Number().Uint64())
}

func (sb *Backend) RegisterPeer(peer consensus.Peer) error {
	// TODO: For added security, we may want verify that all newly connected proxied peer has the
	// correct validator key
	logger := sb.logger.New("func", "RegisterPeer")

	logger.Trace("RegisterPeer called", "peer", peer)

	if err := sb.announceManager.SendVersionCertificateTable(peer); err != nil {
		logger.Debug("Error sending all version certificates", "err", err)
	}

	return nil
}

func (sb *Backend) UnregisterPeer(peer consensus.Peer) {}

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
			enodeCertMsg := sb.RetrieveEnodeCertificateMsgMap()[sb.SelfNode().ID()]
			if enodeCertMsg != nil {
				msg = enodeCertMsg.Msg
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
		// No need to use sb.AsyncSendCeloMsg, since this is already
		// being called within a goroutine.
		err = peer.Send(istanbul.ValidatorHandshakeMsg, msgBytes)
		if err != nil {
			errCh <- err
			return
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
	if peerMsg.Code != istanbul.ValidatorHandshakeMsg {
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

	var enodeCertificate istanbul.EnodeCertificate
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

	// By this point, this node and the peer are both validators and we update
	// our val enode table accordingly. Upsert will only use this entry if the version is new
	err = sb.valEnodeTable.UpsertVersionAndEnode([]*istanbul.AddressEntry{{Address: msg.Address, Node: node, Version: enodeCertificate.Version}})
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
