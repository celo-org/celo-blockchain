package backend

import (
	"github.com/celo-org/celo-blockchain/consensus"
	"github.com/celo-org/celo-blockchain/consensus/istanbul"
	"github.com/celo-org/celo-blockchain/consensus/istanbul/backend/internal/enodes"
	vet "github.com/celo-org/celo-blockchain/consensus/istanbul/backend/internal/enodes"
	"github.com/celo-org/celo-blockchain/log"
)

type VersionCertificateGossiper interface {
	GossipAllFrom(*enodes.VersionCertificateDB) error
	SendAllFrom(*enodes.VersionCertificateDB, consensus.Peer) error
	Gossip(versionCertificates []*versionCertificate) error
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

// GossipAllFrom sends all version certificates to every peer. Only the entries
// that are new to a node will end up being regossiped throughout the
// network.
func (vg *vcGossiper) GossipAllFrom(vcDb *enodes.VersionCertificateDB) error {
	allVersionCertificates, err := getAllVersionCertificates(vcDb)
	if err != nil {
		vg.logger.Warn("Error getting all version certificates", "err", err)
		return err
	}
	return vg.Gossip(allVersionCertificates)
}

func (vg *vcGossiper) Gossip(versionCertificates []*versionCertificate) error {
	logger := vg.logger.New("func", "Gossip")

	payload, err := encodeVersionCertificatesMsg(versionCertificates)
	if err != nil {
		logger.Warn("Error encoding version certificate msg", "err", err)
		return err
	}
	return vg.gossip(payload)
}

// SendVersionCertificateTable sends all VersionCertificates this node
// has to a peer
func (vg *vcGossiper) SendAllFrom(vcDb *enodes.VersionCertificateDB, peer consensus.Peer) error {
	logger := vg.logger.New("func", "SendAllFrom")
	allVersionCertificates, err := getAllVersionCertificates(vcDb)
	if err != nil {
		logger.Warn("Error getting all version certificates", "err", err)
		return err
	}
	payload, err := encodeVersionCertificatesMsg(allVersionCertificates)
	if err != nil {
		logger.Warn("Error encoding version certificate msg", "err", err)
		return err
	}

	return peer.Send(istanbul.VersionCertificatesMsg, payload)
}

func getAllVersionCertificates(vcTable *vet.VersionCertificateDB) ([]*versionCertificate, error) {
	allEntries, err := vcTable.GetAll()
	if err != nil {
		return nil, err
	}
	return fromVCEntries(allEntries), nil
}
