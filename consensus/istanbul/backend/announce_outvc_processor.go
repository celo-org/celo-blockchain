package backend

import (
	"time"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/consensus/istanbul"
	"github.com/celo-org/celo-blockchain/log"
)

type OutboundVersionCertificateProcessor interface {
	Process(*AnnounceState, []*istanbul.VersionCertificate, common.Address) error
}

func NewOutboundVCProcessor(checker ValidatorChecker,
	addrProvider AddressProvider,
	vcGossiper VersionCertificateGossiper) OutboundVersionCertificateProcessor {
	return &ovcp{
		logger:       log.New("module", "OutboundVersionCertificateProcessor"),
		checker:      checker,
		addrProvider: addrProvider,
		vcGossiper:   vcGossiper,
	}
}

type ovcp struct {
	logger       log.Logger
	checker      ValidatorChecker
	addrProvider AddressProvider
	vcGossiper   VersionCertificateGossiper
}

func (o *ovcp) Process(state *AnnounceState, versionCertificates []*istanbul.VersionCertificate, selfEcdsaAddress common.Address) error {
	shouldProcess, err := o.checker.IsElectedOrNearValidator()
	if err != nil {
		o.logger.Warn("Error in checking if should process queryEnode", err)
	}

	if shouldProcess {
		// Update entries in val enode db
		var valEnodeEntries []*istanbul.AddressEntry
		for _, entry := range versionCertificates {
			// Don't add ourselves into the val enode table
			if entry.Address() == selfEcdsaAddress {
				continue
			}
			// Update the HighestKnownVersion for this address. Upsert will
			// only update this entry if the HighestKnownVersion is greater
			// than the existing one.
			// Also store the PublicKey for future encryption in queryEnode msgs
			valEnodeEntries = append(valEnodeEntries, &istanbul.AddressEntry{
				Address:             entry.Address(),
				PublicKey:           entry.PublicKey(),
				HighestKnownVersion: entry.Version,
			})
		}
		if err := state.valEnodeTable.UpsertHighestKnownVersion(valEnodeEntries); err != nil {
			o.logger.Warn("Error upserting val enode table entries", "err", err)
		}
	}

	newVCs, err := state.versionCertificateTable.Upsert(versionCertificates)
	if err != nil {
		o.logger.Warn("Error upserting version certificate table entries", "err", err)
	}

	// Only regossip entries that do not originate from an address that we have
	// gossiped a version certificate for within the last 5 minutes, excluding
	// our own address.
	var versionCertificatesToRegossip []*istanbul.VersionCertificate

	for _, entry := range newVCs {
		lastGossipTime, ok := state.lastVersionCertificatesGossiped.Get(entry.Address())
		if ok && time.Since(lastGossipTime) >= versionCertificateGossipCooldownDuration && entry.Address() != o.addrProvider.ValidatorAddress() {
			continue
		}
		versionCertificatesToRegossip = append(versionCertificatesToRegossip, entry)
		state.lastVersionCertificatesGossiped.Set(entry.Address(), time.Now())
	}

	if len(versionCertificatesToRegossip) > 0 {
		return o.vcGossiper.Gossip(versionCertificatesToRegossip)
	}
	return nil
}
