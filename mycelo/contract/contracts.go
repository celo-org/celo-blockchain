package contract

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"path"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

// CoreContracts describes the addresses for each contracts
type CoreContracts struct {
	BuildPath string

	Libraries Libraries
	Proxies   ContractSet
	Contracts ContractSet
}

type ContractSet map[string]common.Address

var CeloLibraries = Libraries{
	"FixidityLib":                       common.HexToAddress("0xa001"),
	"Proposals":                         common.HexToAddress("0xa002"),
	"LinkedList":                        common.HexToAddress("0xa003"),
	"SortedLinkedList":                  common.HexToAddress("0xa004"),
	"SortedLinkedListWithMedian":        common.HexToAddress("0xa051"),
	"AddressLinkedList":                 common.HexToAddress("0xa006"),
	"AddressSortedLinkedList":           common.HexToAddress("0xa007"),
	"IntegerSortedLinkedList":           common.HexToAddress("0xa008"),
	"AddressSortedLinkedListWithMedian": common.HexToAddress("0xa009"),
	"Signatures":                        common.HexToAddress("0xa010"),
}

func (cs ContractSet) DeployedBytecodeMappings(buildPath string, libs Libraries) (map[common.Address][]byte, error) {
	bytecodes := make(map[common.Address][]byte)
	for name, addr := range cs {
		bs, err := readContractBuildFile(path.Join(buildPath, name+".json"), libs)
		if err != nil {
			return nil, err
		}
		bytecodes[addr] = bs.deployedBytecode
	}
	return bytecodes, nil
}

func (cs ContractSet) DeployedBytecodeFor(buildPath string, libs Libraries, name string) ([]byte, error) {
	c, err := readContractBuildFile(path.Join(buildPath, name+".json"), libs)
	if err != nil {
		return nil, err
	}
	return c.deployedBytecode, nil
}

func (cs ContractSet) BytecodeFor(buildPath string, libs Libraries, name string) ([]byte, error) {
	c, err := readContractBuildFile(path.Join(buildPath, name+".json"), libs)
	if err != nil {
		return nil, err
	}
	return c.bytecode, nil
}

type Libraries ContractSet

func (libs Libraries) DeployedBytecodeMappings(buildPath string) (map[common.Address][]byte, error) {
	return ContractSet(libs).DeployedBytecodeMappings(buildPath, libs)
}

func (libs Libraries) InjectLibraries(bytecode string) string {
	for name, addr := range libs {
		pattern := "__" + name + strings.Repeat("_", 40-4-len(name)) + "__"
		bytecode = strings.ReplaceAll(bytecode, pattern, addr.Hex()[2:])
	}
	return bytecode
}

func NewCoreContracts(buildPath string) *CoreContracts {
	return &CoreContracts{
		BuildPath: buildPath,

		Libraries: Libraries{
			"FixidityLib":                       common.HexToAddress("0xa001"),
			"Proposals":                         common.HexToAddress("0xa002"),
			"LinkedList":                        common.HexToAddress("0xa003"),
			"SortedLinkedList":                  common.HexToAddress("0xa004"),
			"SortedLinkedListWithMedian":        common.HexToAddress("0xa051"),
			"AddressLinkedList":                 common.HexToAddress("0xa006"),
			"AddressSortedLinkedList":           common.HexToAddress("0xa007"),
			"IntegerSortedLinkedList":           common.HexToAddress("0xa008"),
			"AddressSortedLinkedListWithMedian": common.HexToAddress("0xa009"),
			"Signatures":                        common.HexToAddress("0xa010"),
		},

		Proxies: ContractSet{
			"Registry":                   common.HexToAddress("0xce10"),
			"Freezer":                    common.HexToAddress("0xd001"),
			"FeeCurrencyWhitelist":       common.HexToAddress("0xd002"),
			"GoldToken":                  common.HexToAddress("0xd003"),
			"SortedOracles":              common.HexToAddress("0xd004"),
			"GasPriceMinimum":            common.HexToAddress("0xd005"),
			"ReserveSpenderMultiSig":     common.HexToAddress("0xd006"),
			"Reserve":                    common.HexToAddress("0xd007"),
			"StableToken":                common.HexToAddress("0xd008"),
			"Exchange":                   common.HexToAddress("0xd009"),
			"Accounts":                   common.HexToAddress("0xd010"),
			"LockedGold":                 common.HexToAddress("0xd011"),
			"Validators":                 common.HexToAddress("0xd012"),
			"Election":                   common.HexToAddress("0xd013"),
			"EpochRewards":               common.HexToAddress("0xd014"),
			"Random":                     common.HexToAddress("0xd015"),
			"Attestations":               common.HexToAddress("0xd016"),
			"Escrow":                     common.HexToAddress("0xd017"),
			"BlockchainParameters":       common.HexToAddress("0xd018"),
			"GovernanceSlasher":          common.HexToAddress("0xd019"),
			"DoubleSigningSlasher":       common.HexToAddress("0xd020"),
			"DowntimeSlasher":            common.HexToAddress("0xd021"),
			"GovernanceApproverMultiSig": common.HexToAddress("0xd022"),
			"Governance":                 common.HexToAddress("0xd023"),
		},

		Contracts: ContractSet{
			"Registry":                   common.HexToAddress("0xce11"),
			"Freezer":                    common.HexToAddress("0xf001"),
			"FeeCurrencyWhitelist":       common.HexToAddress("0xf002"),
			"GoldToken":                  common.HexToAddress("0xf003"),
			"SortedOracles":              common.HexToAddress("0xf004"),
			"GasPriceMinimum":            common.HexToAddress("0xf005"),
			"ReserveSpenderMultiSig":     common.HexToAddress("0xf006"),
			"Reserve":                    common.HexToAddress("0xf007"),
			"StableToken":                common.HexToAddress("0xf008"),
			"Exchange":                   common.HexToAddress("0xf009"),
			"Accounts":                   common.HexToAddress("0xf011"),
			"LockedGold":                 common.HexToAddress("0xf011"),
			"Validators":                 common.HexToAddress("0xf012"),
			"Election":                   common.HexToAddress("0xf013"),
			"EpochRewards":               common.HexToAddress("0xf014"),
			"Random":                     common.HexToAddress("0xf015"),
			"Attestations":               common.HexToAddress("0xf016"),
			"Escrow":                     common.HexToAddress("0xf017"),
			"BlockchainParameters":       common.HexToAddress("0xf018"),
			"GovernanceSlasher":          common.HexToAddress("0xf019"),
			"DoubleSigningSlasher":       common.HexToAddress("0xf020"),
			"DowntimeSlasher":            common.HexToAddress("0xf021"),
			"GovernanceApproverMultiSig": common.HexToAddress("0xf022"),
			"Governance":                 common.HexToAddress("0xf023"),

			// those were we don't know the address yet
			"TransferWhitelist": common.HexToAddress("0xffffffffff"),
		},
	}
}

func (cc *CoreContracts) ProxyAddressFor(name string) common.Address {
	addr := cc.Proxies[name]
	if addr == common.ZeroAddress {
		panic(fmt.Sprintf("Can't find ProxyAddress for %s", name))
	}
	return addr
}

func (cc *CoreContracts) ImplAddressFor(name string) common.Address {
	addr := cc.Contracts[name]
	if addr == common.ZeroAddress {
		panic(fmt.Sprintf("Can't find Address for %s", name))
	}
	return addr
}

func (cc *CoreContracts) MustDeployedBytecodeFor(name string) []byte {
	ret, err := cc.DeployedBytecodeFor(name)
	if err != nil {
		panic(err)
	}
	return ret
}

func (cc *CoreContracts) MustBytecodeFor(name string) []byte {
	ret, err := cc.BytecodeFor(name)
	if err != nil {
		panic(err)
	}
	return ret
}

func (cc *CoreContracts) DeployedBytecodeFor(name string) ([]byte, error) {
	_, found := cc.Contracts[name]
	if !found {
		return nil, fmt.Errorf("Invalid contract name %s", name)
	}
	return cc.Contracts.DeployedBytecodeFor(cc.BuildPath, cc.Libraries, name)
}

func (cc *CoreContracts) BytecodeFor(name string) ([]byte, error) {
	_, found := cc.Contracts[name]
	if !found {
		return nil, fmt.Errorf("Invalid contract name %s", name)
	}
	return cc.Contracts.BytecodeFor(cc.BuildPath, cc.Libraries, name)
}

func (cc *CoreContracts) LibraryDeployedBytecodes() (map[common.Address][]byte, error) {
	return cc.Libraries.DeployedBytecodeMappings(cc.BuildPath)
}
func (cc *CoreContracts) ProxyAddresses() []common.Address {
	addresses := make([]common.Address, 0, len(cc.Proxies))
	for _, addr := range cc.Proxies {
		addresses = append(addresses, addr)
	}
	return addresses
}

func (cc *CoreContracts) DeployedBytecodes() (map[common.Address][]byte, error) {
	return cc.Contracts.DeployedBytecodeMappings(cc.BuildPath, cc.Libraries)
}

type contractBuild struct {
	bytecode         []byte
	deployedBytecode []byte
}

func readContractBuildFile(truffleJsonFile string, libraries Libraries) (*contractBuild, error) {
	jsonData, err := ioutil.ReadFile(truffleJsonFile)
	if err != nil {
		return nil, fmt.Errorf("Can't read bytecode for %s: %w", truffleJsonFile, err)
	}

	var data struct {
		Bytecode         string `json:"bytecode"`
		DeployedBytecode string `json:"deployedBytecode"`
	}

	err = json.Unmarshal(jsonData, &data)
	if err != nil {
		return nil, fmt.Errorf("Can't read bytecode for %s: %w", truffleJsonFile, err)
	}

	if libraries != nil {
		data.DeployedBytecode = libraries.InjectLibraries(data.DeployedBytecode)
		data.Bytecode = libraries.InjectLibraries(data.Bytecode)
	}

	deployedBytecode, err := hexutil.Decode(data.DeployedBytecode)
	if err != nil {
		fmt.Println(data.DeployedBytecode)
		return nil, fmt.Errorf("Can't read bytecode for %s: %w", truffleJsonFile, err)
	}

	bytecode, err := hexutil.Decode(data.Bytecode)
	if err != nil {
		fmt.Println(data.Bytecode)
		return nil, fmt.Errorf("Can't read bytecode for %s: %w", truffleJsonFile, err)
	}

	return &contractBuild{
		bytecode:         bytecode,
		deployedBytecode: deployedBytecode,
	}, nil
}
