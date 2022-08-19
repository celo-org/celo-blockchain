package env

import (
	"fmt"

	"github.com/celo-org/celo-blockchain/common"
)

var addr = common.HexToAddress

var libraryAddresses = map[string]common.Address{
	"FixidityLib":                       addr("0xa001"),
	"Proposals":                         addr("0xa002"),
	"LinkedList":                        addr("0xa003"),
	"SortedLinkedList":                  addr("0xa004"),
	"SortedLinkedListWithMedian":        addr("0xa051"),
	"AddressLinkedList":                 addr("0xa006"),
	"AddressSortedLinkedList":           addr("0xa007"),
	"IntegerSortedLinkedList":           addr("0xa008"),
	"AddressSortedLinkedListWithMedian": addr("0xa009"),
	"Signatures":                        addr("0xa010"),
}

var genesisAddresses = map[string]common.Address{
	// Contract implementations
	"Registry":                   addr("0xce11"),
	"Freezer":                    addr("0xf001"),
	"FeeCurrencyWhitelist":       addr("0xf002"),
	"GoldToken":                  addr("0xf003"),
	"SortedOracles":              addr("0xf004"),
	"GasPriceMinimum":            addr("0xf005"),
	"ReserveSpenderMultiSig":     addr("0xf006"),
	"Reserve":                    addr("0xf007"),
	"StableToken":                addr("0xf008"),
	"Exchange":                   addr("0xf009"),
	"Accounts":                   addr("0xf010"),
	"LockedGold":                 addr("0xf011"),
	"Validators":                 addr("0xf012"),
	"Election":                   addr("0xf013"),
	"EpochRewards":               addr("0xf014"),
	"Random":                     addr("0xf015"),
	"Attestations":               addr("0xf016"),
	"Escrow":                     addr("0xf017"),
	"BlockchainParameters":       addr("0xf018"),
	"GovernanceSlasher":          addr("0xf019"),
	"DoubleSigningSlasher":       addr("0xf020"),
	"DowntimeSlasher":            addr("0xf021"),
	"GovernanceApproverMultiSig": addr("0xf022"),
	"Governance":                 addr("0xf023"),
	"StableTokenEUR":             addr("0xf024"),
	"ExchangeEUR":                addr("0xf025"),
	"StableTokenBRL":             addr("0xf026"),
	"ExchangeBRL":                addr("0xf027"),

	// Contract Proxies
	"RegistryProxy":                   addr("0xce10"),
	"FreezerProxy":                    addr("0xd001"),
	"FeeCurrencyWhitelistProxy":       addr("0xd002"),
	"GoldTokenProxy":                  addr("0xd003"),
	"SortedOraclesProxy":              addr("0xd004"),
	"GasPriceMinimumProxy":            addr("0xd005"),
	"ReserveSpenderMultiSigProxy":     addr("0xd006"),
	"ReserveProxy":                    addr("0xd007"),
	"StableTokenProxy":                addr("0xd008"),
	"ExchangeProxy":                   addr("0xd009"),
	"AccountsProxy":                   addr("0xd010"),
	"LockedGoldProxy":                 addr("0xd011"),
	"ValidatorsProxy":                 addr("0xd012"),
	"ElectionProxy":                   addr("0xd013"),
	"EpochRewardsProxy":               addr("0xd014"),
	"RandomProxy":                     addr("0xd015"),
	"AttestationsProxy":               addr("0xd016"),
	"EscrowProxy":                     addr("0xd017"),
	"BlockchainParametersProxy":       addr("0xd018"),
	"GovernanceSlasherProxy":          addr("0xd019"),
	"DoubleSigningSlasherProxy":       addr("0xd020"),
	"DowntimeSlasherProxy":            addr("0xd021"),
	"GovernanceApproverMultiSigProxy": addr("0xd022"),
	"GovernanceProxy":                 addr("0xd023"),
	"StableTokenEURProxy":             addr("0xd024"),
	"ExchangeEURProxy":                addr("0xd025"),
	"StableTokenBRLProxy":             addr("0xd026"),
	"ExchangeBRLProxy":                addr("0xd027"),
}

var libraries = []string{
	"FixidityLib",
	"Proposals",
	"LinkedList",
	"SortedLinkedList",
	"SortedLinkedListWithMedian",
	"AddressLinkedList",
	"AddressSortedLinkedList",
	"IntegerSortedLinkedList",
	"AddressSortedLinkedListWithMedian",
	"Signatures",
}

// Libraries returns all celo-blockchain library names
func Libraries() []string { return libraries }

// LibraryAddressFor obtains the address for a core contract
func LibraryAddressFor(name string) (common.Address, error) {
	address, ok := libraryAddresses[name]
	if !ok {
		return common.ZeroAddress, fmt.Errorf("can't find genesis address for %s", name)
	}
	return address, nil
}

// ImplAddressFor obtains the address for a core contract
func ImplAddressFor(name string) (common.Address, error) {
	address, ok := genesisAddresses[name]
	if !ok {
		return common.ZeroAddress, fmt.Errorf("can't find genesis address for %s", name)
	}
	return address, nil
}

// ProxyAddressFor obtains the address for a core contract proxy
func ProxyAddressFor(name string) (common.Address, error) {
	address, ok := genesisAddresses[name+"Proxy"]
	if !ok {
		return common.ZeroAddress, fmt.Errorf("can't find genesis address for %sProxy", name)
	}
	return address, nil
}

// MustLibraryAddressFor obtains the address for a core contract
// this variant panics on error
func MustLibraryAddressFor(name string) common.Address {
	address, err := LibraryAddressFor(name)
	if err != nil {
		panic(err)
	}
	return address
}

// MustImplAddressFor obtains the address for a core contract
// this variant panics on error
func MustImplAddressFor(name string) common.Address {
	address, err := ImplAddressFor(name)
	if err != nil {
		panic(err)
	}
	return address
}

// MustProxyAddressFor obtains the address for a core contract proxy
// this variant panics on error
func MustProxyAddressFor(name string) common.Address {
	address, err := ProxyAddressFor(name)
	if err != nil {
		panic(err)
	}
	return address
}
