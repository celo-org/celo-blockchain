package backend

import (
	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/consensus"
	"github.com/celo-org/celo-blockchain/consensus/istanbul"
	"github.com/celo-org/celo-blockchain/consensus/istanbul/backend/internal/enodes"
	"github.com/celo-org/celo-blockchain/log"
)

type VersionCertificateGossiper interface {
	// GossipAllFrom gossips all version certificates to every peer. Only the entries
	// that are new to a node will end up being regossiped throughout the
	// network.
	GossipAllFrom(*enodes.VersionCertificateDB) error
	// SendAllFrom sends all VersionCertificates this node
	// has to a specific peer.
	SendAllFrom(*enodes.VersionCertificateDB, consensus.Peer) error
	// Gossip will send the given version certificates to all peers.
	Gossip(versionCertificates []*istanbul.VersionCertificate) error
}

func NewVcGossiper(gossipFunction func(payload []byte) error) VersionCertificateGossiper {
	return &vcGossiper{
		logger: log.New("module", "versionCertificateGossiper"),
		gossip: gossipFunction,
	}
}

type vcGossiper struct {
	logger log.Logger
	// gossip gossips protocol messages
	gossip func(payload []byte) error
}

func (vg *vcGossiper) GossipAllFrom(vcDb *enodes.VersionCertificateDB) error {
	allVersionCertificates, err := vcDb.GetAll()
	if err != nil {
		vg.logger.Warn("Error getting all version certificates", "err", err)
		return err
	}
	return vg.Gossip(allVersionCertificates)
}

func (vg *vcGossiper) Gossip(versionCertificates []*istanbul.VersionCertificate) error {
	logger := vg.logger.New("func", "Gossip")
	payload, err := istanbul.NewVersionCeritifcatesMessage(versionCertificates, common.Address{}).Payload()
	if err != nil {
		logger.Warn("Error encoding version certificate msg", "err", err)
		return err
	}
	return vg.gossip(payload)
}

func (vg *vcGossiper) SendAllFrom(vcDb *enodes.VersionCertificateDB, peer consensus.Peer) error {
	logger := vg.logger.New("func", "SendAllFrom")
	allVersionCertificates, err := vcDb.GetAll()
	if err != nil {
		logger.Warn("Error getting all version certificates", "err", err)
		return err
	}
	payload, err := istanbul.NewVersionCeritifcatesMessage(allVersionCertificates, common.Address{}).Payload()
	if err != nil {
		logger.Warn("Error encoding version certificate msg", "err", err)
		return err
	}

	return peer.Send(istanbul.VersionCertificatesMsg, payload)
}
