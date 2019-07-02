package params

import (
	"errors"
	"strings"
	"sync"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
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
	ErrSmartContractNotDeployed = errors.New("registered contract not deployed")

	regAddrCache   = make(map[string]*regAddrCacheEntry)
	regAddrCacheMu sync.RWMutex
)

type StateDB interface {
	GetCodeHash(addr common.Address) common.Hash
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
	registryStorageHash := evm.GetStateDB().GetStorageRoot(registrySmartContractAddress)
	registryCodeHash := evm.GetStateDB().GetCodeHash(registrySmartContractAddress)

	regAddrCacheMu.RLock()
	if regAddrCacheEntry := regAddrCache[registryId]; regAddrCacheEntry != nil &&
		regAddrCacheEntry.registryStorageHash == registryStorageHash &&
		regAddrCacheEntry.registryCodeHash == registryCodeHash {
		regAddrCacheMu.RUnlock()
		return regAddrCacheEntry.address, nil
	}
	regAddrCacheMu.RUnlock()

	// Fetch the address and update the cache
	var contractAddress common.Address
	regAddrCacheMu.Lock()
	defer regAddrCacheMu.Unlock()

	// Check to see if the cache just got inserted from another thread.
	if regAddrCacheEntry := regAddrCache[registryId]; regAddrCacheEntry != nil &&
		regAddrCacheEntry.registryStorageHash == registryStorageHash &&
		regAddrCacheEntry.registryCodeHash == registryCodeHash {
		return regAddrCacheEntry.address, nil
	}

	if _, err := evm.StaticCallFromSystem(registrySmartContractAddress, getAddressForFuncABI, "getAddressFor", []interface{}{registryId}, &contractAddress, 20000); err != nil {
		return nil, err
	}

	if _, ok := regAddrCache[registryId]; !ok {
		regAddrCache[registryId] = new(regAddrCacheEntry)
	}

	// The contract has not been registered yet
	if contractAddress == common.ZeroAddress {
		return nil, ErrSmartContractNotDeployed
	}

	regAddrCache[registryId].address = &contractAddress
	regAddrCache[registryId].registryStorageHash = registryStorageHash
	regAddrCache[registryId].registryCodeHash = registryCodeHash

	return &contractAddress, nil
}
