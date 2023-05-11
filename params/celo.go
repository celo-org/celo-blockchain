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
)

// Gas discount table for BLS12-377 G1 and G2 multi exponentiation operations
var Bls12377MultiExpDiscountTable = [128]uint64{1200, 888, 764, 641, 594, 547, 500, 453, 438, 423, 408, 394, 379, 364, 349, 334, 330, 326, 322, 318, 314, 310, 306, 302, 298, 294, 289, 285, 281, 277, 273, 269, 268, 266, 265, 263, 262, 260, 259, 257, 256, 254, 253, 251, 250, 248, 247, 245, 244, 242, 241, 239, 238, 236, 235, 233, 232, 231, 229, 228, 226, 225, 223, 222, 221, 220, 219, 219, 218, 217, 216, 216, 215, 214, 213, 213, 212, 211, 211, 210, 209, 208, 208, 207, 206, 205, 205, 204, 203, 202, 202, 201, 200, 199, 199, 198, 197, 196, 196, 195, 194, 193, 193, 192, 191, 191, 190, 189, 188, 188, 187, 186, 185, 185, 184, 183, 182, 182, 181, 180, 179, 179, 178, 177, 176, 176, 175, 174}
