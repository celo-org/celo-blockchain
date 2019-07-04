package params

import (
	"errors"
	"strings"
	"sync"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/log"
)

const (
	// This is taken from celo-monorepo/packages/protocol/build/<env>/contracts/Registry.json
	getAddressForABI = `[{"constant": true,
                              "inputs": [
                                   {
                                       "name": "identifier",
                                       "type": "string"
                                   }
                              ],
                              "name": "getAddressFor",
                              "outputs": [
                                   {
                                       "name": "",
                                       "type": "address"
                                   }
                              ],
                              "payable": false,
                              "stateMutability": "view",
                              "type": "function"
                             }]`
)

var (
	getAddressForFuncABI, _ = abi.JSON(strings.NewReader(getAddressForABI))

	// ErrSmartContractNotDeployed is returned when the RegisteredAddresses mapping does not contain the specified contract
	ErrSmartContractNotDeployed    = errors.New("registered contract not deployed")
	ErrRegistryContractNotDeployed = errors.New("contract registry not deployed")

	regAddrCache   = make(map[string]*regAddrCacheEntry)
	regAddrCacheMu sync.RWMutex
)

type StateDB interface {
	GetCodeHash(addr common.Address) common.Hash
	GetCodeSize(addr common.Address) int
	GetStorageRoot(addr common.Address) common.Hash
}

type EVM interface {
	StaticCallFromSystem(contractAddress common.Address, abi abi.ABI, funcName string, args []interface{}, returnObj interface{}, gas uint64) (uint64, error)
	GetStateDB() StateDB
}

type regAddrCacheEntry struct {
	address             *common.Address
	registryStorageHash common.Hash
	registryCodeHash    common.Hash
}

func GetRegisteredAddress(registryId string, evm EVM) (*common.Address, error) {
	if evm.GetStateDB().GetCodeSize(registrySmartContractAddress) == 0 {
		return nil, ErrSmartContractNotDeployed
	}

  var contractAddress common.Address
	_, err := evm.StaticCallFromSystem(registrySmartContractAddress, getAddressForFuncABI, "getAddressFor", []interface{}{registryId}, &contractAddress, 20000)

	if err == abi.ErrEmptyOutput {
		return nil, ErrRegistryContractNotDeployed
	} else if err != nil {
		return nil, err
	}

	if contractAddress == common.ZeroAddress {
		return nil, ErrSmartContractNotDeployed
	}

	return &contractAddress, nil
}
