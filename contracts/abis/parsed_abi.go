package abis

import (
	"fmt"
	"strings"

	"github.com/celo-org/celo-blockchain/accounts/abi"
)

var (
	Registry             *abi.ABI = mustParseAbi("Registry", RegistryStr)
	BlockchainParameters *abi.ABI = mustParseAbi("BlockchainParameters", BlockchainParametersStr)
	SortedOracles        *abi.ABI = mustParseAbi("SortedOracles", SortedOraclesStr)
	ERC20                *abi.ABI = mustParseAbi("ERC20", ERC20Str)
	FeeCurrency          *abi.ABI = mustParseAbi("FeeCurrency", FeeCurrencyStr)
	Elections            *abi.ABI = mustParseAbi("Elections", ElectionsStr)
	EpochRewards         *abi.ABI = mustParseAbi("EpochRewards", EpochRewardsStr)
	Freezer              *abi.ABI = mustParseAbi("Freezer", FreezerStr)
	GasPriceMinimum      *abi.ABI = mustParseAbi("GasPriceMinimum", GasPriceMinimumStr)
	GoldToken            *abi.ABI = mustParseAbi("GoldToken", GoldTokenStr)
	Random               *abi.ABI = mustParseAbi("Random", RandomStr)
	Validators           *abi.ABI = mustParseAbi("Validators", ValidatorsStr)
)

func mustParseAbi(name, abiStr string) *abi.ABI {
	parsedAbi, err := abi.JSON(strings.NewReader(abiStr))
	if err != nil {
		panic(fmt.Sprintf("Error reading ABI %s err=%s", name, err))
	}
	return &parsedAbi
}
