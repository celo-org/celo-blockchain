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
	registrySmartContractAddress = common.HexToAddress("0x000000000000000000000000000000000000ce10")
	registeredContractIds        = []string{
		params.AttestationsRegistryId,
		params.BondedDepositsRegistryId,
		params.GasCurrencyWhitelistRegistryId,
		params.GasPriceMinimumRegistryId,
		params.GoldTokenRegistryId,
		params.GovernanceRegistryId,
		params.RandomRegistryId,
		params.ReserveRegistryId,
		params.SortedOraclesRegistryId,
		params.ValidatorsRegistryId,
	}
	getAddressForFuncABI, _ = abi.JSON(strings.NewReader(getAddressForABI))

	zeroCaller   = vm.AccountRef(common.HexToAddress("0x0"))
	emptyMessage = types.NewMessage(common.HexToAddress("0x0"), nil, 0, common.Big0, 0, common.Big0, nil, nil, []byte{}, false)

	// ErrSmartContractNotDeployed is returned when the RegisteredAddresses mapping does not contain the specified contract
	var ErrSmartContractNotDeployed = errors.New("registered contract not deployed")
)

type regAddrCacheEntry struct {
        address common.Address
	registryStorageHash common.Hash
	registryCodeHash common.Hash
}

// An EVM handler to make calls to smart contracts from within geth
type InternalEVMHandler struct {
	chain  ChainContext
	chainConfig *params.ChainConfig
	vmConfig *vm.Config
	regAddrCache map[string]*regAddrCacheEntry
        regAddrCacheMu sync.RWMutex
}

func NewInternalEVMHandler(chain ChainContext, chainConfig *params.ChainConfig, vmConfig *vm.Config) *InternalEVMHandler {
	iEvmH := InternalEVMHandler{
		chain: chain,
		chainConfig: chaingConfig,
		vmConfig: vmConfig,
		regAddrCache: make(map[string]*regAddrCacheEntry),
	}
	return &iEvmH
}

func (iEvmH *InternalEVMHandler) createEVM(header *types.Header, state *state.stateDB, getRegAddrsForPrecompiles bool) (*vm.EVM, error) {
	// Normally, when making an evm call, we should use the current block's state.  However,
	// there are times (e.g. internal evm call within consensus.engine.Finalize) that we need
	// to call the evm using the currently mined block.  In that case, the header and state params
	// should be non nil.
	if header == nil {
		header = iEvmH.chain.CurrentHeader()
	}

	if state == nil {
		var err error
		state, err = iEvmH.chain.State()
		if err != nil {
			log.Error("Error in retrieving the state from the blockchain")
			return nil, err
		}
	}

	var regAddrsForPrecomipiles map[string]common.Address = nil
	if getRegAddrsForPrecompiles {
	    regAddrForPrecompiles = make(map[string]common.Address)
	    for registryId := range vm.RegisteredContractsForPrecompiledContracts {
	    	if scAddress, err := iEvmH.getRegistryAddress(registerId, header, state); err != nil {
		   return nil, err
		}

		regAddrsForPrecompiles[registryId] = scAddress
	    }
	}

	// The EVM Context requires a msg, but the actual field values don't really matter for this case.
	// Putting in zero values.
	context := NewEVMContext(emptyMessage, header, iEvmH.chain, nil, regAddrsForPrecompiles)
	evm := vm.NewEVM(context, state, iEvmH.chain.Config(), *iEvmH.chain.GetVMConfig())

	return evm, nil
}

func (iEvmH *InternalEVMHandler) getRegistryAddress (registryId string, header *types.Header, state *state.stateDB) (common.Address, err) {
        registryStorageHash := state.StorageTrie(registrySmartContractAddress).Hash()
	registryCodeHash := state.GetCodeHash(registrySmartContractAddress)

	iEvmH.regAddrCacheMu.RLock()
	if regAddrCacheEntry := iEvmH.regAddrCache[registryId]; regAddrCacheEntry != nil && entry[registerId].registryStorageHash == registryStorageHash && entry[registerId].registryCodeHash == registryCodeHash {
	    iEvmH.regAddrCacheMu.RUnLock()
	    return regAddrCacheEntry.address
	}
	iEvmH.regAddrCacheMu.RUnLock()

	// Fetch the address and update the cache
	var contractAddress common.Address
	iEvmH.registeredAddressesCacheMu.Lock()
	defer iEvmH.registeredAddressesCacheMu.UnLock()

	// Check to see if the cache just got inserted from another thread.  
	if regAddrCacheEntry := iEvmH.regAddrCache[registryId]; regAddrCacheEntry != nil && entry[registerId].registryStorageHash == registryStorageHash && entry[registerId].registryCodeHash == registryCodeHash {
	    return regAddrCacheEntry.address
	}	

	// Setting the registeredAddressMap to nil.  That means that none of the precompiled contracts can be called.
	evm, err := iEvmH.createEVM(header, state, false)

	if _, err := evm.ABIStaticCall(zeroCaller, registrySmartContractAddress, getAddressForFuncABI, 'getAddressFor', []inteface{}{registerId}, &contractAddress, 20000); err != nil {
	   return common.ZeroAddress, err
	}

	if _, ok := iEvmh.regAddrCache[registryId]; !ok {
	    iEvmh.regAddrCache[registryId] = new(regAddrCacheEntry)
	}
	
	iEvmh.regAddrCache[registryId].address = contractAddress
	iEvmh.regAddrCache[registryId].registryStorageHash = registryStorageHash
	iEvmh.regAddrCache[registryId].registryCodeHash = registryCodeHash

	// The contract has not been registered yet
	if contractAddress == common.ZeroAddress {
	    return common.ZeroAddress, ErrSmartContractNotDeployed
	}

	return contractAddress, nil
}

func (iEvmH *InternalEVMHandler) MakeStaticCall(registerId string, abi abi.ABI, funcName string, args []interface{}, returnObj interface{}, gas uint64, header *types.Header, state *state.StateDB) (uint64, error) {
        // Retrieve the address of the provided registerID
	scAddress, err := iEvmH.getRegistryAddress(registerId, header, state)
	if err != nil {
	   return 0, err
	}

	evm, err := iEvmH.createEVM(header, state, true)
	if err != nil {
	   return 0, err
	}

	return evm.ABIStaticCall(zeroCaller, scAddress, abi, funcName, args, returnObj, gas)
}


func (iEvmH *InternalEVMHandler) MakeCall(scAddress common.Address, abi abi.ABI, funcName string, args []interface{}, returnObj interface{}, gas uint64, value *big.Int, header *types.Header, state *state.StateDB) (uint64, error) {
        // Retrieve the address of the provided registerID
	scAddress, err := iEvmH.getRegistryAddress(registerId, header, state)
	if err != nil {
	   return 0, err
	}

	evm, err := iEvmH.createEVM(header, state, true)
	if err != nil {
	   return 0, err
	}

	return evm.ABICall(zeroCaller, scAddress, abi, funcName, args, returnObj, gas, value)
}
