package backend

import (
	"time"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/consensus/istanbul/backend/announce"
	"github.com/celo-org/celo-blockchain/consensus/istanbul/backend/internal/enodes"
	"github.com/celo-org/celo-blockchain/log"
)

type AnnounceState struct {
	valEnodeTable           *enodes.ValidatorEnodeDB
	versionCertificateTable *enodes.VersionCertificateDB

	lastVersionCertificatesGossiped *announce.AddressTime
	lastQueryEnodeGossiped          *announce.AddressTime
}

func NewAnnounceState(valEnodeTable *enodes.ValidatorEnodeDB, versionCertificateTable *enodes.VersionCertificateDB) *AnnounceState {
	return &AnnounceState{
		valEnodeTable:                   valEnodeTable,
		versionCertificateTable:         versionCertificateTable,
		lastQueryEnodeGossiped:          announce.NewAddressTime(),
		lastVersionCertificatesGossiped: announce.NewAddressTime(),
	}
}

type AnnounceStatePruner interface {
	Prune(*AnnounceState) error
}

func NewAnnounceStatePruner(retrieveValidatorConnSetFn func() (map[common.Address]bool, error)) AnnounceStatePruner {
	return &pruner{
		logger:                   log.New("module", "announceStatePruner"),
		retrieveValidatorConnSet: retrieveValidatorConnSetFn,
	}
}

type pruner struct {
	logger                   log.Logger
	retrieveValidatorConnSet func() (map[common.Address]bool, error)
}

// Prune will remove entries that are not in the validator connection set from all announce related data structures.
// The data structures that it prunes are:
// 1)  lastQueryEnodeGossiped
// 2)  valEnodeTable
// 3)  lastVersionCertificatesGossiped
// 4)  versionCertificateTable
func (p *pruner) Prune(state *AnnounceState) error {
	// retrieve the validator connection set
	validatorConnSet, err := p.retrieveValidatorConnSet()
	if err != nil {
		p.logger.Warn("Error in pruning announce data structures", "err", err)
	}

	state.lastQueryEnodeGossiped.RemoveIf(func(remoteAddress common.Address, t time.Time) bool {
		return !validatorConnSet[remoteAddress] && time.Since(t) >= queryEnodeGossipCooldownDuration
	})

	if err := state.valEnodeTable.PruneEntries(validatorConnSet); err != nil {
		p.logger.Trace("Error in pruning valEnodeTable", "err", err)
		return err
	}

	state.lastVersionCertificatesGossiped.RemoveIf(func(remoteAddress common.Address, t time.Time) bool {
		return !validatorConnSet[remoteAddress] && time.Since(t) >= versionCertificateGossipCooldownDuration
	})

	if err := state.versionCertificateTable.Prune(validatorConnSet); err != nil {
		p.logger.Trace("Error in pruning versionCertificateTable", "err", err)
		return err
	}

	return nil
}
