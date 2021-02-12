package contract

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"path"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/mycelo/env"
)

type TruffleReader interface {
	ReadBytecodeFor(name string) ([]byte, error)
	ReadDeployedBytecodeFor(name string) ([]byte, error)
	MustReadBytecodeFor(name string) []byte
	MustReadDeployedBytecodeFor(name string) []byte
}

type truffleReader struct {
	buildPath string
	libraries map[string]common.Address
}

func NewTruffleReader(buildPath string) TruffleReader {

	librariesMapping := make(map[string]common.Address, len(env.Libraries()))
	for _, name := range env.Libraries() {
		librariesMapping[name] = env.MustAddressFor(name)
	}

	return &truffleReader{
		buildPath: buildPath,
		libraries: librariesMapping,
	}

}

func (tr *truffleReader) jsonFileFor(name string) string {
	return path.Join(tr.buildPath, name+".json")
}

func (tr *truffleReader) ReadDeployedBytecodeFor(name string) ([]byte, error) {
	c, err := readContractBuildFile(tr.jsonFileFor(name), tr.libraries)
	if err != nil {
		return nil, err
	}
	return c.deployedBytecode, nil
}

func (tr *truffleReader) ReadBytecodeFor(name string) ([]byte, error) {
	c, err := readContractBuildFile(tr.jsonFileFor(name), tr.libraries)
	if err != nil {
		return nil, err
	}
	return c.bytecode, nil
}

func (tr *truffleReader) MustReadBytecodeFor(name string) []byte {
	ret, err := tr.ReadBytecodeFor(name)
	if err != nil {
		panic(err)
	}
	return ret
}

func (tr *truffleReader) MustReadDeployedBytecodeFor(name string) []byte {
	ret, err := tr.ReadDeployedBytecodeFor(name)
	if err != nil {
		panic(err)
	}
	return ret
}

func replaceLibrariesInBytecode(mappings map[string]common.Address, bytecode string) string {
	for name, addr := range mappings {
		pattern := "__" + name + strings.Repeat("_", 40-4-len(name)) + "__"
		bytecode = strings.ReplaceAll(bytecode, pattern, addr.Hex()[2:])
	}
	return bytecode
}

// func NewCoreContracts(buildPath string) *CoreContracts {
// 	return &CoreContracts{
// 		BuildPath: buildPath,

// 		Libraries: Libraries{
// 			"FixidityLib":                       common.HexToAddress("0xa001"),
// 			"Proposals":                         common.HexToAddress("0xa002"),
// 			"LinkedList":                        common.HexToAddress("0xa003"),
// 			"SortedLinkedList":                  common.HexToAddress("0xa004"),
// 			"SortedLinkedListWithMedian":        common.HexToAddress("0xa051"),
// 			"AddressLinkedList":                 common.HexToAddress("0xa006"),
// 			"AddressSortedLinkedList":           common.HexToAddress("0xa007"),
// 			"IntegerSortedLinkedList":           common.HexToAddress("0xa008"),
// 			"AddressSortedLinkedListWithMedian": common.HexToAddress("0xa009"),
// 			"Signatures":                        common.HexToAddress("0xa010"),
// 		},

// 		Proxies: ContractSet{
// 			"Registry":                   common.HexToAddress("0xce10"),
// 			"Freezer":                    common.HexToAddress("0xd001"),
// 			"FeeCurrencyWhitelist":       common.HexToAddress("0xd002"),
// 			"GoldToken":                  common.HexToAddress("0xd003"),
// 			"SortedOracles":              common.HexToAddress("0xd004"),
// 			"GasPriceMinimum":            common.HexToAddress("0xd005"),
// 			"ReserveSpenderMultiSig":     common.HexToAddress("0xd006"),
// 			"Reserve":                    common.HexToAddress("0xd007"),
// 			"StableToken":                common.HexToAddress("0xd008"),
// 			"Exchange":                   common.HexToAddress("0xd009"),
// 			"Accounts":                   common.HexToAddress("0xd010"),
// 			"LockedGold":                 common.HexToAddress("0xd011"),
// 			"Validators":                 common.HexToAddress("0xd012"),
// 			"Election":                   common.HexToAddress("0xd013"),
// 			"EpochRewards":               common.HexToAddress("0xd014"),
// 			"Random":                     common.HexToAddress("0xd015"),
// 			"Attestations":               common.HexToAddress("0xd016"),
// 			"Escrow":                     common.HexToAddress("0xd017"),
// 			"BlockchainParameters":       common.HexToAddress("0xd018"),
// 			"GovernanceSlasher":          common.HexToAddress("0xd019"),
// 			"DoubleSigningSlasher":       common.HexToAddress("0xd020"),
// 			"DowntimeSlasher":            common.HexToAddress("0xd021"),
// 			"GovernanceApproverMultiSig": common.HexToAddress("0xd022"),
// 			"Governance":                 common.HexToAddress("0xd023"),
// 		},

// 		Contracts: ContractSet{
// 			"Registry":                   common.HexToAddress("0xce11"),
// 			"Freezer":                    common.HexToAddress("0xf001"),
// 			"FeeCurrencyWhitelist":       common.HexToAddress("0xf002"),
// 			"GoldToken":                  common.HexToAddress("0xf003"),
// 			"SortedOracles":              common.HexToAddress("0xf004"),
// 			"GasPriceMinimum":            common.HexToAddress("0xf005"),
// 			"ReserveSpenderMultiSig":     common.HexToAddress("0xf006"),
// 			"Reserve":                    common.HexToAddress("0xf007"),
// 			"StableToken":                common.HexToAddress("0xf008"),
// 			"Exchange":                   common.HexToAddress("0xf009"),
// 			"Accounts":                   common.HexToAddress("0xf011"),
// 			"LockedGold":                 common.HexToAddress("0xf011"),
// 			"Validators":                 common.HexToAddress("0xf012"),
// 			"Election":                   common.HexToAddress("0xf013"),
// 			"EpochRewards":               common.HexToAddress("0xf014"),
// 			"Random":                     common.HexToAddress("0xf015"),
// 			"Attestations":               common.HexToAddress("0xf016"),
// 			"Escrow":                     common.HexToAddress("0xf017"),
// 			"BlockchainParameters":       common.HexToAddress("0xf018"),
// 			"GovernanceSlasher":          common.HexToAddress("0xf019"),
// 			"DoubleSigningSlasher":       common.HexToAddress("0xf020"),
// 			"DowntimeSlasher":            common.HexToAddress("0xf021"),
// 			"GovernanceApproverMultiSig": common.HexToAddress("0xf022"),
// 			"Governance":                 common.HexToAddress("0xf023"),

// 			// those were we don't know the address yet
// 			"TransferWhitelist": common.HexToAddress("0xffffffffff"),
// 		},
// 	}
// }

type contractBuild struct {
	bytecode         []byte
	deployedBytecode []byte
}

func readContractBuildFile(truffleJSONFile string, libraries map[string]common.Address) (*contractBuild, error) {
	jsonData, err := ioutil.ReadFile(truffleJSONFile)
	if err != nil {
		return nil, fmt.Errorf("Can't read bytecode for %s: %w", truffleJSONFile, err)
	}

	var data struct {
		Bytecode         string `json:"bytecode"`
		DeployedBytecode string `json:"deployedBytecode"`
	}

	err = json.Unmarshal(jsonData, &data)
	if err != nil {
		return nil, fmt.Errorf("Can't read bytecode for %s: %w", truffleJSONFile, err)
	}

	if libraries != nil {
		data.DeployedBytecode = replaceLibrariesInBytecode(libraries, data.DeployedBytecode)
		data.Bytecode = replaceLibrariesInBytecode(libraries, data.Bytecode)
	}

	deployedBytecode, err := hexutil.Decode(data.DeployedBytecode)
	if err != nil {
		fmt.Println(data.DeployedBytecode)
		return nil, fmt.Errorf("Can't read bytecode for %s: %w", truffleJSONFile, err)
	}

	bytecode, err := hexutil.Decode(data.Bytecode)
	if err != nil {
		fmt.Println(data.Bytecode)
		return nil, fmt.Errorf("Can't read bytecode for %s: %w", truffleJSONFile, err)
	}

	return &contractBuild{
		bytecode:         bytecode,
		deployedBytecode: deployedBytecode,
	}, nil
}
