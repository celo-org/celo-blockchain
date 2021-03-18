package contract_comm

import (
	"math/big"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/contracts"
	"github.com/celo-org/celo-blockchain/core/types"
	"github.com/celo-org/celo-blockchain/core/vm"
)

type contractsContext struct {
}

func (cc *contractsContext) GetAddressFromRegistry(evm *vm.EVM, registryId common.Hash) (common.Address, error) {
	return contracts.GetRegisteredAddress(evm, registryId)
}

func (cc *contractsContext) ComputeTobinTax(evm *vm.EVM, sender common.Address, transferAmount *big.Int) (tax *big.Int, taxRecipient common.Address, err error) {
	return contracts.ComputeTobinTax(evm, sender, transferAmount)
}

// NewEVMContext creates a new context for use in the EVM.
func NewEVMContext(msg vm.Message, header *types.Header, chain vm.ChainContext, txFeeRecipient *common.Address) vm.Context {
	contractsContext := &contractsContext{}

	// The EVM Context requires a msg, but the actual field values don't really matter for this case.
	// Putting in zero values.
	return vm.NewEVMContext(emptyMessage, header, internalEvmHandlerSingleton.chain, contractsContext, nil)
}
