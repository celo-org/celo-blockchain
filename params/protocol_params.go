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
	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/common/hexutil"
	"github.com/celo-org/celo-blockchain/common/math"
	"github.com/celo-org/celo-blockchain/crypto"
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

	SstoreSetGas    uint64 = 20000 // Once per SSTORE operation.
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

	SstoreSentryGasEIP2200            uint64 = 2300  // Minimum gas required to be present for an SSTORE call, not consumed
	SstoreSetGasEIP2200               uint64 = 20000 // Once per SSTORE operation from clean zero to non-zero
	SstoreResetGasEIP2200             uint64 = 5000  // Once per SSTORE operation from clean non-zero to something else
	SstoreClearsScheduleRefundEIP2200 uint64 = 15000 // Once per SSTORE operation for clearing an originally existing storage slot

	// ColdAccountAccessCostEIP2929 (2600 -> 900), ColdSloadCostEIP2929 (2100 -> 800) are modified according to CIP-0048
	// Links: https://github.com/celo-org/celo-proposals/blob/master/CIPs/cip-0048.md
	ColdAccountAccessCostEIP2929 = uint64(900) // COLD_ACCOUNT_ACCESS_COST
	ColdSloadCostEIP2929         = uint64(800) // COLD_SLOAD_COST
	WarmStorageReadCostEIP2929   = uint64(100) // WARM_STORAGE_READ_COST

	// In EIP-2200: SstoreResetGas was 5000.
	// In EIP-2929: SstoreResetGas was changed to '5000 - COLD_SLOAD_COST'.
	// In EIP-3529: SSTORE_CLEARS_SCHEDULE is defined as SSTORE_RESET_GAS + ACCESS_LIST_STORAGE_KEY_COST
	// Which becomes: 5000 - 2100 + 1900 = 4800
	SstoreClearsScheduleRefundEIP3529 uint64 = SstoreResetGasEIP2200 - ColdSloadCostEIP2929 + TxAccessListStorageKeyGas

	JumpdestGas   uint64 = 1     // Once per JUMPDEST operation.
	EpochDuration uint64 = 30000 // Duration between proof-of-work epochs.

	CreateDataGas         uint64 = 200   //
	CallCreateDepth       uint64 = 1024  // Maximum depth of call/create stack.
	ExpGas                uint64 = 10    // Once per EXP instruction
	LogGas                uint64 = 375   // Per LOG* operation.
	CopyGas               uint64 = 3     //
	StackLimit            uint64 = 1024  // Maximum size of VM stack allowed.
	TierStepGas           uint64 = 0     // Once per operation, for a selection of them.
	LogTopicGas           uint64 = 375   // Multiplied by the * of the LOG*, per LOG transaction. e.g. LOG0 incurs 0 * c_txLogTopicGas, LOG4 incurs 4 * c_txLogTopicGas.
	CreateGas             uint64 = 32000 // Once per CREATE operation & contract-creation transaction.
	Create2Gas            uint64 = 32000 // Once per CREATE2 operation
	SelfdestructRefundGas uint64 = 24000 // Refunded following a selfdestruct operation.
	MemoryGas             uint64 = 3     // Times the address of the (highest referenced byte in memory + 1). NOTE: referencing happens on read, write and in instructions such as RETURN and CALL.

	TxDataNonZeroGasFrontier  uint64 = 68   // Per byte of data attached to a transaction that is not equal to zero. NOTE: Not payable on data of calls between transactions.
	TxDataNonZeroGasEIP2028   uint64 = 16   // Per byte of non zero data attached to a transaction after EIP 2028 (part in Istanbul)
	TxAccessListAddressGas    uint64 = 2400 // Per address specified in EIP 2930 access list
	TxAccessListStorageKeyGas uint64 = 1900 // Per storage key specified in EIP 2930 access list

	// These have been changed during the course of the chain
	CallGasFrontier              uint64 = 40  // Once per CALL operation & message call transaction.
	CallGasEIP150                uint64 = 700 // Static portion of gas for CALL-derivates after EIP 150 (Tangerine)
	BalanceGasFrontier           uint64 = 20  // The cost of a BALANCE operation
	BalanceGasEIP150             uint64 = 400 // The cost of a BALANCE operation after Tangerine
	BalanceGasEIP1884            uint64 = 700 // The cost of a BALANCE operation after EIP 1884 (part of Istanbul)
	ExtcodeSizeGasFrontier       uint64 = 20  // Cost of EXTCODESIZE before EIP 150 (Tangerine)
	ExtcodeSizeGasEIP150         uint64 = 700 // Cost of EXTCODESIZE after EIP 150 (Tangerine)
	SloadGasFrontier             uint64 = 50
	SloadGasEIP150               uint64 = 200
	SloadGasEIP1884              uint64 = 800  // Cost of SLOAD after EIP 1884 (part of Istanbul)
	SloadGasEIP2200              uint64 = 800  // Cost of SLOAD after EIP 2200 (part of Istanbul)
	ExtcodeHashGasConstantinople uint64 = 400  // Cost of EXTCODEHASH (introduced in Constantinople)
	ExtcodeHashGasEIP1884        uint64 = 700  // Cost of EXTCODEHASH after EIP 1884 (part in Istanbul)
	SelfdestructGasEIP150        uint64 = 5000 // Cost of SELFDESTRUCT post EIP 150 (Tangerine)

	// EXP has a dynamic portion depending on the size of the exponent
	ExpByteFrontier uint64 = 10 // was set to 10 in Frontier
	ExpByteEIP158   uint64 = 50 // was raised to 50 during Eip158 (Spurious Dragon)

	// Extcodecopy has a dynamic AND a static cost. This represents only the
	// static portion of the gas. It was changed during EIP 150 (Tangerine)
	ExtcodeCopyBaseFrontier uint64 = 20
	ExtcodeCopyBaseEIP150   uint64 = 700

	// CreateBySelfdestructGas is used when the refunded account is one that does
	// not exist. This logic is similar to call.
	// Introduced in Tangerine Whistle (Eip 150)
	CreateBySelfdestructGas uint64 = 25000

	MaxCodeSize = 65536 // Maximum bytecode to permit for a contract (2^16)

	// Precompiled contract gas prices

	EcrecoverGas        uint64 = 3000 // Elliptic curve sender recovery gas price
	Sha256BaseGas       uint64 = 60   // Base price for a SHA256 operation
	Sha256PerWordGas    uint64 = 12   // Per-word price for a SHA256 operation
	Ripemd160BaseGas    uint64 = 600  // Base price for a RIPEMD160 operation
	Ripemd160PerWordGas uint64 = 120  // Per-word price for a RIPEMD160 operation
	IdentityBaseGas     uint64 = 15   // Base price for a data copy operation
	IdentityPerWordGas  uint64 = 3    // Per-work price for a data copy operation

	Bn256AddGasByzantium             uint64 = 500    // Byzantium gas needed for an elliptic curve addition
	Bn256AddGasIstanbul              uint64 = 150    // Gas needed for an elliptic curve addition
	Bn256ScalarMulGasByzantium       uint64 = 40000  // Byzantium gas needed for an elliptic curve scalar multiplication
	Bn256ScalarMulGasIstanbul        uint64 = 6000   // Gas needed for an elliptic curve scalar multiplication
	Bn256PairingBaseGasByzantium     uint64 = 100000 // Byzantium base price for an elliptic curve pairing check
	Bn256PairingBaseGasIstanbul      uint64 = 45000  // Base price for an elliptic curve pairing check
	Bn256PairingPerPointGasByzantium uint64 = 80000  // Byzantium per-point price for an elliptic curve pairing check
	Bn256PairingPerPointGasIstanbul  uint64 = 34000  // Per-point price for an elliptic curve pairing check

	// Celo precompiled contracts
	FractionMulExpGas           uint64 = 50     // Cost of performing multiplication and exponentiation of fractions to an exponent of up to 10^3.
	ProofOfPossessionGas        uint64 = 350000 // Cost of verifying a BLS proof of possession.
	GetValidatorGas             uint64 = 1000   // Cost of reading a validator's address.
	GetValidatorBLSGas          uint64 = 1000   // Cost of reading a validator's BLS public key.
	GetEpochSizeGas             uint64 = 10     // Cost of querying the number of blocks in an epoch.
	GetBlockNumberFromHeaderGas uint64 = 10     // Cost of decoding a block header.
	HashHeaderGas               uint64 = 10     // Cost of hashing a block header.
	GetParentSealBitmapGas      uint64 = 100    // Cost of reading the parent seal bitmap from the chain.
	// May take a bit more time with 100 validators, need to bench that
	GetVerifiedSealBitmapGas uint64 = 350000           // Cost of verifying the seal on a given RLP encoded header.
	Ed25519VerifyGas         uint64 = 1500             // Gas needed for and Ed25519 signature verification
	Sha2_512BaseGas          uint64 = Sha256BaseGas    // Base price for a Sha2-512 operation
	Sha2_512PerWordGas       uint64 = Sha256PerWordGas // Per-word price for a Sha2-512 operation

	Sha3_256BaseGas     uint64 = Sha3Gas     // Base price for a Sha3-256 operation
	Sha3_256PerWordGas  uint64 = Sha3WordGas // Per-word price for a sha3-256 operation
	Sha3_512BaseGas     uint64 = Sha3Gas     // Base price for a Sha3-512 operation
	Sha3_512PerWordGas  uint64 = Sha3WordGas // Per-word price for a Sha3-512 operation
	Keccak512BaseGas    uint64 = Sha3Gas     // Per-word price for a Keccak512 operation
	Keccak512PerWordGas uint64 = Sha3WordGas // Base price for a Keccak512 operation

	Blake2sBaseGas    uint64 = Sha256BaseGas    // Per-word price for a Blake2s operation
	Blake2sPerWordGas uint64 = Sha256PerWordGas // Base price for a Blake2s
	InvalidCip20Gas   uint64 = 200              // Price of attempting to access an unsupported CIP20 hash function

	Bls12377G1AddGas          uint64 = 600   // Price for BLS12-377 elliptic curve G1 point addition
	Bls12377G1MulGas          uint64 = 12000 // Price for BLS12-377 elliptic curve G1 point scalar multiplication
	Bls12377G2AddGas          uint64 = 4500  // Price for BLS12-377 elliptic curve G2 point addition
	Bls12377G2MulGas          uint64 = 55000 // Price for BLS12-377 elliptic curve G2 point scalar multiplication
	Bls12377PairingBaseGas    uint64 = 65000 // Base gas price for BLS12-377 elliptic curve pairing check
	Bls12377PairingPerPairGas uint64 = 55000 // Per-point pair gas price for BLS12-377 elliptic curve pairing check

	Bls12381G1AddGas          uint64 = 600   // Price for BLS12-381 elliptic curve G1 point addition
	Bls12381G1MulGas          uint64 = 12000 // Price for BLS12-381 elliptic curve G1 point scalar multiplication
	Bls12381G2AddGas          uint64 = 800   // Price for BLS12-381 elliptic curve G2 point addition
	Bls12381G2MulGas          uint64 = 45000 // Price for BLS12-381 elliptic curve G2 point scalar multiplication
	Bls12381PairingBaseGas    uint64 = 65000 // Base gas price for BLS12-381 elliptic curve pairing check
	Bls12381PairingPerPairGas uint64 = 43000 // Per-point pair gas price for BLS12-381 elliptic curve pairing check
	Bls12381MapG1Gas          uint64 = 5500  // Gas price for BLS12-381 mapping field element to G1 operation
	Bls12381MapG2Gas          uint64 = 75000 // Gas price for BLS12-381 mapping field element to G2 operation

	// The Refund Quotient is the cap on how much of the used gas can be refunded. Before EIP-3529,
	// up to half the consumed gas could be refunded. Redefined as 1/5th in EIP-3529
	RefundQuotient        uint64 = 2
	RefundQuotientEIP3529 uint64 = 5
)

// Gas discount table for BLS12-377 G1 and G2 multi exponentiation operations
var Bls12377MultiExpDiscountTable = [128]uint64{1200, 888, 764, 641, 594, 547, 500, 453, 438, 423, 408, 394, 379, 364, 349, 334, 330, 326, 322, 318, 314, 310, 306, 302, 298, 294, 289, 285, 281, 277, 273, 269, 268, 266, 265, 263, 262, 260, 259, 257, 256, 254, 253, 251, 250, 248, 247, 245, 244, 242, 241, 239, 238, 236, 235, 233, 232, 231, 229, 228, 226, 225, 223, 222, 221, 220, 219, 219, 218, 217, 216, 216, 215, 214, 213, 213, 212, 211, 211, 210, 209, 208, 208, 207, 206, 205, 205, 204, 203, 202, 202, 201, 200, 199, 199, 198, 197, 196, 196, 195, 194, 193, 193, 192, 191, 191, 190, 189, 188, 188, 187, 186, 185, 185, 184, 183, 182, 182, 181, 180, 179, 179, 178, 177, 176, 176, 175, 174}

// Gas discount table for BLS12-381 G1 and G2 multi exponentiation operations
var Bls12381MultiExpDiscountTable = [128]uint64{1200, 888, 764, 641, 594, 547, 500, 453, 438, 423, 408, 394, 379, 364, 349, 334, 330, 326, 322, 318, 314, 310, 306, 302, 298, 294, 289, 285, 281, 277, 273, 269, 268, 266, 265, 263, 262, 260, 259, 257, 256, 254, 253, 251, 250, 248, 247, 245, 244, 242, 241, 239, 238, 236, 235, 233, 232, 231, 229, 228, 226, 225, 223, 222, 221, 220, 219, 219, 218, 217, 216, 216, 215, 214, 213, 213, 212, 211, 211, 210, 209, 208, 208, 207, 206, 205, 205, 204, 203, 202, 202, 201, 200, 199, 199, 198, 197, 196, 196, 195, 194, 193, 193, 192, 191, 191, 190, 189, 188, 188, 187, 186, 185, 185, 184, 183, 182, 182, 181, 180, 179, 179, 178, 177, 176, 176, 175, 174}

var (
	RegistrySmartContractAddress = common.HexToAddress("0x000000000000000000000000000000000000ce10")

	// Celo registered contract IDs.
	// The names are taken from celo-monorepo/packages/protocol/lib/registry-utils.ts
	AttestationsRegistryId         = makeRegistryId("Attestations")
	BlockchainParametersRegistryId = makeRegistryId("BlockchainParameters")
	ElectionRegistryId             = makeRegistryId("Election")
	EpochRewardsRegistryId         = makeRegistryId("EpochRewards")
	FeeCurrencyWhitelistRegistryId = makeRegistryId("FeeCurrencyWhitelist")
	FreezerRegistryId              = makeRegistryId("Freezer")
	GasPriceMinimumRegistryId      = makeRegistryId("GasPriceMinimum")
	GoldTokenRegistryId            = makeRegistryId("GoldToken")
	GovernanceRegistryId           = makeRegistryId("Governance")
	LockedGoldRegistryId           = makeRegistryId("LockedGold")
	RandomRegistryId               = makeRegistryId("Random")
	ReserveRegistryId              = makeRegistryId("Reserve")
	SortedOraclesRegistryId        = makeRegistryId("SortedOracles")
	StableTokenRegistryId          = makeRegistryId("StableToken")
	TransferWhitelistRegistryId    = makeRegistryId("TransferWhitelist")
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
	thousand = 1000
	million  = 1000 * 1000

	// Default intrinsic gas cost of transactions paying for gas in alternative currencies.
	// Calculated to estimate 1 balance read, 1 debit, and 4 credit transactions.
	IntrinsicGasForAlternativeFeeCurrency uint64 = 50 * thousand

	// Contract communication gas limits
	MaxGasForCalculateTargetEpochPaymentAndRewards uint64 = 2 * million
	MaxGasForCommitments                           uint64 = 2 * million
	MaxGasForComputeCommitment                     uint64 = 2 * million
	MaxGasForBlockRandomness                       uint64 = 2 * million
	MaxGasForDebitGasFeesTransactions              uint64 = 1 * million
	MaxGasForCreditGasFeesTransactions             uint64 = 1 * million
	MaxGasForDistributeEpochPayment                uint64 = 1 * million
	MaxGasForDistributeEpochRewards                uint64 = 1 * million
	MaxGasForElectValidators                       uint64 = 50 * million
	MaxGasForElectNValidatorSigners                uint64 = 50 * million
	MaxGasForGetAddressFor                         uint64 = 100 * thousand
	MaxGasForGetElectableValidators                uint64 = 100 * thousand
	MaxGasForGetEligibleValidatorGroupsVoteTotals  uint64 = 1 * million
	MaxGasForGetGasPriceMinimum                    uint64 = 2 * million
	MaxGasForGetGroupEpochRewards                  uint64 = 500 * thousand
	MaxGasForGetMembershipInLastEpoch              uint64 = 1 * million
	MaxGasForGetOrComputeTobinTax                  uint64 = 1 * million
	MaxGasForGetRegisteredValidators               uint64 = 2 * million
	MaxGasForGetValidator                          uint64 = 100 * thousand
	MaxGasForGetWhiteList                          uint64 = 200 * thousand
	MaxGasForGetTransferWhitelist                  uint64 = 2 * million
	MaxGasForIncreaseSupply                        uint64 = 50 * thousand
	MaxGasForIsFrozen                              uint64 = 20 * thousand
	MaxGasForMedianRate                            uint64 = 100 * thousand
	MaxGasForReadBlockchainParameter               uint64 = 40 * thousand // ad-hoc measurement is ~26k
	MaxGasForRevealAndCommit                       uint64 = 2 * million
	MaxGasForUpdateGasPriceMinimum                 uint64 = 2 * million
	MaxGasForUpdateTargetVotingYield               uint64 = 2 * million
	MaxGasForUpdateValidatorScore                  uint64 = 1 * million
	MaxGasForTotalSupply                           uint64 = 50 * thousand
	MaxGasForMintGas                               uint64 = 5 * million
	MaxGasToReadErc20Balance                       uint64 = 100 * thousand
	MaxGasForIsReserveLow                          uint64 = 1 * million
	MaxGasForGetCarbonOffsettingPartner            uint64 = 20 * thousand
)
