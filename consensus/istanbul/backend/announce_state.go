package backend

import (
	"github.com/celo-org/celo-blockchain/consensus/istanbul/backend/announce"
	"github.com/celo-org/celo-blockchain/consensus/istanbul/backend/internal/enodes"
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
