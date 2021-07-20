package backend

import (
	"math"
	"time"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/consensus/istanbul"
	"github.com/celo-org/celo-blockchain/consensus/istanbul/backend/internal/enodes"
	"github.com/celo-org/celo-blockchain/log"
)

type QueryEnodeEntryProvider interface {
	GetQueryEnodeValEnodeEntries(enforceRetryBackoff bool, exceptAddress common.Address) ([]*istanbul.AddressEntry, error)
}

type qeep struct {
	logger        log.Logger
	valEnodeTable *enodes.ValidatorEnodeDB
}

func NewQueryEnodeEntryProvider(valEnodeTable *enodes.ValidatorEnodeDB) QueryEnodeEntryProvider {
	return &qeep{
		logger:        log.New("module", "announceQueryEnodeEntryProvider"),
		valEnodeTable: valEnodeTable,
	}
}

func (q *qeep) GetQueryEnodeValEnodeEntries(enforceRetryBackoff bool, exceptEcdsaAddress common.Address) ([]*istanbul.AddressEntry, error) {
	logger := q.logger.New("func", "getQueryEnodeValEnodeEntries")
	valEnodeEntries, err := q.valEnodeTable.GetValEnodes(nil)
	if err != nil {
		return nil, err
	}

	var queryEnodeValEnodeEntries []*istanbul.AddressEntry
	for address, valEnodeEntry := range valEnodeEntries {
		// Don't generate an announce record for ourselves
		if address == exceptEcdsaAddress {
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
