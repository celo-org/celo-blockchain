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
		librariesMapping[name] = env.MustLibraryAddressFor(name)
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

type truflleFile struct {
	bytecode         []byte
	deployedBytecode []byte
}

func readContractBuildFile(truffleJSONFile string, libraries map[string]common.Address) (*truflleFile, error) {
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

	return &truflleFile{
		bytecode:         bytecode,
		deployedBytecode: deployedBytecode,
	}, nil
}
