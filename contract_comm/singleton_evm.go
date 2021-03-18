package contract_comm

import (
	"reflect"

	"github.com/celo-org/celo-blockchain/common"

	"github.com/celo-org/celo-blockchain/core/types"
	"github.com/celo-org/celo-blockchain/core/vm"
	"github.com/celo-org/celo-blockchain/log"
)

var (
	emptyMessage                = types.NewMessage(common.HexToAddress("0x0"), nil, 0, common.Big0, 0, common.Big0, nil, nil, common.Big0, []byte{}, false)
	internalEvmHandlerSingleton *InternalEVMHandler
)

type InternalEVMHandler struct {
	chain vm.ChainContext
}

func createEVM(header *types.Header, state vm.StateDB) (*vm.EVM, error) {
	// Normally, when making an evm call, we should use the current block's state.  However,
	// there are times (e.g. retrieving the set of validators when an epoch ends) that we need
	// to call the evm using the currently mined block.  In that case, the header and state params
	// will be non nil.
	if internalEvmHandlerSingleton == nil {
		panic("contract_comm.SetInternalEVMHandler() must be called on init.")
	}

	if header == nil {
		header = internalEvmHandlerSingleton.chain.CurrentHeader()
	}

	if state == nil || reflect.ValueOf(state).IsNil() {
		var err error
		state, err = internalEvmHandlerSingleton.chain.State()
		if err != nil {
			log.Error("Error in retrieving the state from the blockchain", "err", err)
			return nil, err
		}
	}

	// The EVM Context requires a msg, but the actual field values don't really matter for this case.
	// Putting in zero values.
	context := NewEVMContext(emptyMessage, header, internalEvmHandlerSingleton.chain, nil)
	evm := vm.NewEVM(context, state, internalEvmHandlerSingleton.chain.Config(), *internalEvmHandlerSingleton.chain.GetVMConfig())

	return evm, nil
}

func SetInternalEVMHandler(chain vm.ChainContext) {
	if internalEvmHandlerSingleton == nil {
		log.Trace("Setting the InternalEVMHandler Singleton")
		internalEvmHandler := InternalEVMHandler{
			chain: chain,
		}
		internalEvmHandlerSingleton = &internalEvmHandler
	}
}
