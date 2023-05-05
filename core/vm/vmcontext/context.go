package vmcontext

import (
	"math/big"

	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/consensus"
	"github.com/celo-org/celo-blockchain/consensus/istanbul"
	"github.com/celo-org/celo-blockchain/contracts"
	"github.com/celo-org/celo-blockchain/contracts/config"
	"github.com/celo-org/celo-blockchain/contracts/reserve"
	"github.com/celo-org/celo-blockchain/core/types"
	"github.com/celo-org/celo-blockchain/core/vm"
	"github.com/celo-org/celo-blockchain/log"
	"github.com/celo-org/celo-blockchain/params"
)

// chainContext defines methods required to build a context
// a copy of this exist on core.ChainContext (needed to break dependency)
type chainContext interface {
	// Engine retrieves the blockchain's consensus engine.
	Engine() consensus.Engine

	// GetHeader returns the hash corresponding to the given hash and number.
	GetHeader(common.Hash, uint64) *types.Header

	// GetHeaderByNumber returns the hash corresponding number.
	// in the correct fork.
	GetHeaderByNumber(uint64) *types.Header

	// Config returns the blockchain's chain configuration
	Config() *params.ChainConfig
}

// New creates a new context for use in the EVM.
func NewBlockContext(header *types.Header, chain chainContext, txFeeRecipient *common.Address) vm.BlockContext {
	// If we don't have an explicit txFeeRecipient (i.e. not mining), extract from the header
	// The only call that fills the txFeeRecipient, is the ApplyTransaction from the state processor
	// All the other calls, assume that will be retrieved from the header
	var beneficiary common.Address
	if txFeeRecipient == nil {
		beneficiary = header.Coinbase
	} else {
		beneficiary = *txFeeRecipient
	}

	ctx := vm.BlockContext{
		CanTransfer: CanTransfer,
		Transfer:    TobinTransfer,
		GetHash:     GetHashFn(header, chain),
		VerifySeal:  VerifySealFn(header, chain),
		Coinbase:    beneficiary,
		GasLimit:    header.GasLimit,
		BlockNumber: new(big.Int).Set(header.Number),
		Time:        new(big.Int).SetUint64(header.Time),

		IsGoldTokenAddress: IsGoldTokenAddress,
	}

	if chain != nil {
		ctx.EpochSize = chain.Engine().EpochSize()
		ctx.GetValidators = chain.Engine().GetValidators
		ctx.GetHeaderByNumber = chain.GetHeaderByNumber
	} else {
		ctx.GetValidators = func(blockNumber *big.Int, headerHash common.Hash) []istanbul.Validator { return nil }
		ctx.GetHeaderByNumber = func(uint64) *types.Header { panic("evm context without blockchain context") }
	}
	return ctx
}

func IsGoldTokenAddress(evm *vm.EVM, addr common.Address) (bool, error) {
	caller := &SharedEVMRunner{evm}
	goldTokenAddr, err := contracts.GetRegisteredAddress(caller, config.GoldTokenRegistryId)
	if err != nil {
		return false, err
	}
	return goldTokenAddr == addr, nil
}

// GetHashFn returns a GetHashFunc which retrieves header hashes by number
func GetHashFn(ref *types.Header, chain chainContext) func(uint64) common.Hash {
	// Cache will initially contain [refHash.parent],
	// Then fill up with [refHash.p, refHash.pp, refHash.ppp, ...]
	var cache []common.Hash

	return func(n uint64) common.Hash {
		// If there's no hash cache yet, make one
		if len(cache) == 0 {
			cache = append(cache, ref.ParentHash)
		}
		if idx := ref.Number.Uint64() - n - 1; idx < uint64(len(cache)) {
			return cache[idx]
		}
		// No luck in the cache, but we can start iterating from the last element we already know
		lastKnownHash := cache[len(cache)-1]
		lastKnownNumber := ref.Number.Uint64() - uint64(len(cache))

		for {
			header := chain.GetHeader(lastKnownHash, lastKnownNumber)
			if header == nil {
				break
			}
			cache = append(cache, header.ParentHash)
			lastKnownHash = header.ParentHash
			lastKnownNumber = header.Number.Uint64() - 1
			if n == lastKnownNumber {
				return lastKnownHash
			}
		}
		return common.Hash{}
	}
}

// CanTransfer checks whether there are enough funds in the address' account to make a transfer.
// This does not take the necessary gas into account to make the transfer valid.
func CanTransfer(db vm.StateDB, addr common.Address, amount *big.Int) bool {
	return db.GetBalance(addr).Cmp(amount) >= 0
}

// Transfer subtracts amount from sender and adds amount to recipient using the given Db
func Transfer(db vm.StateDB, sender, recipient common.Address, amount *big.Int) {
	db.SubBalance(sender, amount)
	db.AddBalance(recipient, amount)
}

// VerifySealFn returns a function which returns true when the given header has a verifiable seal.
func VerifySealFn(ref *types.Header, chain chainContext) func(*types.Header) bool {
	return func(header *types.Header) bool {
		// If the block is later than the unsealed reference block, return false.
		if header.Number.Cmp(ref.Number) > 0 {
			return false
		}

		// FIXME: Implementation currently relies on the Istanbul engine's internal view of the
		// chain, so return false if this is not an Istanbul chain. As a consequence of this the
		// seal is always verified against the canonical chain, which makes behavior undefined if
		// this function is evaluated on a chain which does not have the highest total difficulty.
		if chain.Config().Istanbul == nil {
			return false
		}

		// Submit the header to the engine's seal verification function.
		return chain.Engine().VerifySeal(header) == nil
	}
}

// TobinTransfer performs a transfer that may take a tax from the sent amount and give it to the reserve.
// If the calculation or transfer of the tax amount fails for any reason, the regular transfer goes ahead.
// NB: Gas is not charged or accounted for this calculation.
func TobinTransfer(evm *vm.EVM, sender, recipient common.Address, amount *big.Int) {
	// Run only primary evm.Call() with tracer
	if evm.GetDebug() {
		evm.SetDebug(false)
		defer func() { evm.SetDebug(true) }()
	}

	// Only deduct tobin tax before the g hardfork
	if evm.ChainConfig().IsGFork(evm.Context.BlockNumber) {
		Transfer(evm.StateDB, sender, recipient, amount)
	} else {
		if amount.Cmp(big.NewInt(0)) != 0 {
			caller := &SharedEVMRunner{evm}
			tax, taxRecipient, err := reserve.ComputeTobinTax(caller, sender, amount)
			if err == nil {
				Transfer(evm.StateDB, sender, recipient, new(big.Int).Sub(amount, tax))
				Transfer(evm.StateDB, sender, taxRecipient, tax)
				return
			} else {
				log.Error("Failed to get tobin tax", "error", err)
			}
		}

		// Complete a normal transfer if the amount is 0 or the tobin tax value is unable to be fetched and parsed.
		// We transfer even when the amount is 0 because state trie clearing [EIP161] is necessary at the end of a transaction
		Transfer(evm.StateDB, sender, recipient, amount)
	}
}
