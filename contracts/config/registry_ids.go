package config

import (
	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/crypto"
)

var (
	RegistrySmartContractAddress = common.HexToAddress("0x000000000000000000000000000000000000ce10")

	// Celo registered contract IDs.
	// The names are taken from celo-monorepo/packages/protocol/lib/registry-utils.ts
	AttestationsRegistryId         = makeRegistryId("Attestations")
	BlockchainParametersRegistryId = makeRegistryId("BlockchainParameters")
	ElectionRegistryId             = makeRegistryId("Election")
	FeeCurrencyWhitelistRegistryId = makeRegistryId("FeeCurrencyWhitelist")
	GasPriceMinimumRegistryId      = makeRegistryId("GasPriceMinimum")
	GoldTokenRegistryId            = makeRegistryId("GoldToken")
	GovernanceRegistryId           = makeRegistryId("Governance")
	RandomRegistryId               = makeRegistryId("Random")
	SortedOraclesRegistryId        = makeRegistryId("SortedOracles")
	StableTokenRegistryId          = makeRegistryId("StableToken")
	TransferWhitelistRegistryId    = makeRegistryId("TransferWhitelist")
	ValidatorsRegistryId           = makeRegistryId("Validators")
)

func makeRegistryId(contractName string) [32]byte {
	hash := crypto.Keccak256([]byte(contractName))
	var id [32]byte
	copy(id[:], hash)

	return id
}
