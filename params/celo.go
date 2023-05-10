package params

import "github.com/celo-org/celo-blockchain/common/math"

// Scale factor for the solidity fixidity library
var Fixidity1 = math.BigPow(10, 24)

const (
	DefaultGasLimit uint64 = 20000000 // Gas limit of the blocks before BlockchainParams contract is loaded.
)

// Celo precompiled contracts
const (
	FractionMulExpGas           uint64 = 50     // Cost of performing multiplication and exponentiation of fractions to an exponent of up to 10^3.
	ProofOfPossessionGas        uint64 = 350000 // Cost of verifying a BLS proof of possession.
	GetValidatorGas             uint64 = 1000   // Cost of reading a validator's address.
	GetValidatorBLSGas          uint64 = 1000   // Cost of reading a validator's BLS public key.
	GetEpochSizeGas             uint64 = 10     // Cost of querying the number of blocks in an epoch.
	GetBlockNumberFromHeaderGas uint64 = 10     // Cost of decoding a block header.
	HashHeaderGas               uint64 = 10     // Cost of hashing a block header.
	GetParentSealBitmapGas      uint64 = 100    // Cost of reading the parent seal bitmap from the chain.
	// May take a bit more time with 100 validators, need to bench that
	GetVerifiedSealBitmapGas uint64 = 350000 // Cost of verifying the seal on a given RLP encoded header.
)
