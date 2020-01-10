// Copyright 2015 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package params

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/crypto"
)

const (
	DefaultGasLimit uint64 = 20000000 // Gas limit of the blocks before BlockchainParams contract is loaded.

	MaximumExtraDataSize  uint64 = 32    // Maximum size extra data may be after Genesis.
	ExpByteGas            uint64 = 10    // Times ceil(log256(exponent)) for the EXP instruction.
	SloadGas              uint64 = 50    // Multiplied by the number of 32-byte words that are copied (round up) for any *COPY operation and added.
	CallValueTransferGas  uint64 = 9000  // Paid for CALL when the value transfer is non-zero.
	CallNewAccountGas     uint64 = 25000 // Paid for CALL when the destination address didn't exist prior.
	TxGas                 uint64 = 21000 // Per transaction not creating a contract. NOTE: Not payable on data of calls between transactions.
	TxGasContractCreation uint64 = 53000 // Per transaction that creates a contract. NOTE: Not payable on data of calls between transactions.
	TxDataZeroGas         uint64 = 4     // Per byte of data attached to a transaction that equals zero. NOTE: Not payable on data of calls between transactions.
	QuadCoeffDiv          uint64 = 512   // Divisor for the quadratic particle of the memory cost equation.
	LogDataGas            uint64 = 8     // Per byte in a LOG* operation's data.
	CallStipend           uint64 = 2300  // Free gas given at beginning of call.

	Sha3Gas     uint64 = 30 // Once per SHA3 operation.
	Sha3WordGas uint64 = 6  // Once per word of the SHA3 operation's data.

	SstoreSetGas    uint64 = 20000 // Once per SLOAD operation.
	SstoreResetGas  uint64 = 5000  // Once per SSTORE operation if the zeroness changes from zero.
	SstoreClearGas  uint64 = 5000  // Once per SSTORE operation if the zeroness doesn't change.
	SstoreRefundGas uint64 = 15000 // Once per SSTORE operation if the zeroness changes to zero.

	NetSstoreNoopGas  uint64 = 200   // Once per SSTORE operation if the value doesn't change.
	NetSstoreInitGas  uint64 = 20000 // Once per SSTORE operation from clean zero.
	NetSstoreCleanGas uint64 = 5000  // Once per SSTORE operation from clean non-zero.
	NetSstoreDirtyGas uint64 = 200   // Once per SSTORE operation from dirty.

	NetSstoreClearRefund      uint64 = 15000 // Once per SSTORE operation for clearing an originally existing storage slot
	NetSstoreResetRefund      uint64 = 4800  // Once per SSTORE operation for resetting to the original non-zero value
	NetSstoreResetClearRefund uint64 = 19800 // Once per SSTORE operation for resetting to the original zero value

	JumpdestGas      uint64 = 1     // Refunded gas, once per SSTORE operation if the zeroness changes to zero.
	EpochDuration    uint64 = 30000 // Duration between proof-of-work epochs.
	CallGas          uint64 = 40    // Once per CALL operation & message call transaction.
	CreateDataGas    uint64 = 200   //
	CallCreateDepth  uint64 = 1024  // Maximum depth of call/create stack.
	ExpGas           uint64 = 10    // Once per EXP instruction
	LogGas           uint64 = 375   // Per LOG* operation.
	CopyGas          uint64 = 3     //
	StackLimit       uint64 = 1024  // Maximum size of VM stack allowed.
	TierStepGas      uint64 = 0     // Once per operation, for a selection of them.
	LogTopicGas      uint64 = 375   // Multiplied by the * of the LOG*, per LOG transaction. e.g. LOG0 incurs 0 * c_txLogTopicGas, LOG4 incurs 4 * c_txLogTopicGas.
	CreateGas        uint64 = 32000 // Once per CREATE operation & contract-creation transaction.
	Create2Gas       uint64 = 32000 // Once per CREATE2 operation
	SuicideRefundGas uint64 = 24000 // Refunded following a suicide operation.
	MemoryGas        uint64 = 3     // Times the address of the (highest referenced byte in memory + 1). NOTE: referencing happens on read, write and in instructions such as RETURN and CALL.
	TxDataNonZeroGas uint64 = 68    // Per byte of data attached to a transaction that is not equal to zero. NOTE: Not payable on data of calls between transactions.

	MaxCodeSize = 65536 // Maximum bytecode to permit for a contract (2^16)

	// Precompiled contract gas prices

	EcrecoverGas            uint64 = 3000   // Elliptic curve sender recovery gas price
	Sha256BaseGas           uint64 = 60     // Base price for a SHA256 operation
	Sha256PerWordGas        uint64 = 12     // Per-word price for a SHA256 operation
	Ripemd160BaseGas        uint64 = 600    // Base price for a RIPEMD160 operation
	Ripemd160PerWordGas     uint64 = 120    // Per-word price for a RIPEMD160 operation
	IdentityBaseGas         uint64 = 15     // Base price for a data copy operation
	IdentityPerWordGas      uint64 = 3      // Per-work price for a data copy operation
	ModExpQuadCoeffDiv      uint64 = 20     // Divisor for the quadratic particle of the big int modular exponentiation
	Bn256AddGas             uint64 = 500    // Gas needed for an elliptic curve addition
	Bn256ScalarMulGas       uint64 = 40000  // Gas needed for an elliptic curve scalar multiplication
	Bn256PairingBaseGas     uint64 = 100000 // Base price for an elliptic curve pairing check
	Bn256PairingPerPointGas uint64 = 80000  // Per-point price for an elliptic curve pairing check

	// Celo precompiled contracts
	// TODO: make this cost variable- https://github.com/celo-org/geth/issues/250
	FractionMulExpGas uint64 = 1050 // Cost of performing multiplication and exponentiation of fractions to an exponent of up to 10^3.
	// TODO(kobigurk):  Figure out what the actual gas cost of this contract should be.
	ProofOfPossessionGas        uint64 = 50000 // Cost of verifying a BLS proof of possession.
	GetValidatorGas             uint64 = 5000  // Cost of reading a validator's address.
	GetEpochSizeGas             uint64 = 1000  // Cost of querying the number of blocks in an epoch.
	GetBlockNumberFromHeaderGas uint64 = 10000 // Cost of decoding a block header.
	HashHeaderGas               uint64 = 20000 // Cost of hashing a block header.
	GetParentSealBitmapGas      uint64 = 500   // Cost of reading the parent seal bitmap from the chain.
	GetVerifiedSealBitmapGas    uint64 = 55000 // Cost of verifying the seal on a given RLP encoded header.
)

var (
	DifficultyBoundDivisor = big.NewInt(2048)   // The bound divisor of the difficulty, used in the update calculations.
	GenesisDifficulty      = big.NewInt(131072) // Difficulty of the Genesis block.
	MinimumDifficulty      = big.NewInt(131072) // The minimum that the difficulty may ever be.
	DurationLimit          = big.NewInt(13)     // The decision boundary on the blocktime duration used to determine whether difficulty should go up or not.

	RegistrySmartContractAddress = common.HexToAddress("0x000000000000000000000000000000000000ce10")

	// Celo registered contract IDs.
	// The names are taken from celo-monorepo/packages/protocol/lib/registry-utils.ts
	AttestationsRegistryId         = makeRegistryId("Attestations")
	BlockchainParametersRegistryId = makeRegistryId("BlockchainParameters")
	ElectionRegistryId             = makeRegistryId("Election")
	EpochRewardsRegistryId         = makeRegistryId("EpochRewards")
	FeeCurrencyWhitelistRegistryId = makeRegistryId("FeeCurrencyWhitelist")
	GasPriceMinimumRegistryId      = makeRegistryId("GasPriceMinimum")
	GoldTokenRegistryId            = makeRegistryId("GoldToken")
	GovernanceRegistryId           = makeRegistryId("Governance")
	LockedGoldRegistryId           = makeRegistryId("LockedGold")
	RandomRegistryId               = makeRegistryId("Random")
	ReserveRegistryId              = makeRegistryId("Reserve")
	SortedOraclesRegistryId        = makeRegistryId("SortedOracles")
	StableTokenRegistryId          = makeRegistryId("StableToken")
	ValidatorsRegistryId           = makeRegistryId("Validators")

	// Function is "getOrComputeTobinTax()"
	// selector is first 4 bytes of keccak256 of "getOrComputeTobinTax()"
	// Source:
	// pip3 install pyethereum
	// python3 -c 'from ethereum.utils import sha3; print(sha3("getOrComputeTobinTax()")[0:4].hex())'
	TobinTaxFunctionSelector = hexutil.MustDecode("0x17f9a6f7")

	// Scale factor for the solidity fixidity library
	Fixidity1 = math.BigPow(10, 24)
)

func makeRegistryId(contractName string) [32]byte {
	hash := crypto.Keccak256([]byte(contractName))
	var id [32]byte
	copy(id[:], hash)

	return id
}

const (
	// Default intrinsic gas cost of transactions paying for gas in alternative currencies.
	// Calculated to estimate 1 balance read, 1 debit, and 4 credit transactions.
	IntrinsicGasForAlternativeFeeCurrency uint64 = 50000

	// Contract communication gas limits
	MaxGasForCalculateTargetEpochPaymentAndRewards uint64 = 2000000
	MaxGasForCommitments                           uint64 = 2000000
	MaxGasForComputeCommitment                     uint64 = 2000000
	MaxGasForCreditToTransactions                  uint64 = 100000
	MaxGasForDebitFromTransactions                 uint64 = 100000
	MaxGasForDistributeEpochPayment                uint64 = 1 * 1000000
	MaxGasForDistributeEpochRewards                uint64 = 1 * 1000000
	MaxGasForElectValidators                       uint64 = 50 * 1000000
	MaxGasForGetAddressFor                         uint64 = 1 * 100000
	MaxGasForGetEligibleValidatorGroupsVoteTotals  uint64 = 1 * 1000000
	MaxGasForGetGasPriceMinimum                    uint64 = 2000000
	MaxGasForGetGroupEpochRewards                  uint64 = 500 * 1000
	MaxGasForGetMembershipInLastEpoch              uint64 = 1 * 1000000
	MaxGasForGetOrComputeTobinTax                  uint64 = 1000000
	MaxGasForGetRegisteredValidators               uint64 = 2000000
	MaxGasForGetValidator                          uint64 = 100 * 1000
	MaxGasForGetWhiteList                          uint64 = 20000
	MaxGasForIncreaseSupply                        uint64 = 50 * 1000
	MaxGasForMedianRate                            uint64 = 20000
	MaxGasForReadBlockchainParameter               uint64 = 20000
	MaxGasForRevealAndCommit                       uint64 = 2000000
	MaxGasForUpdateGasPriceMinimum                 uint64 = 2000000
	MaxGasForUpdateTargetVotingYield               uint64 = 2000000
	MaxGasForUpdateValidatorScore                  uint64 = 1 * 1000000
	MaxGasForTotalSupply                           uint64 = 50 * 1000
	MaxGasToReadErc20Balance                       uint64 = 100000
)
