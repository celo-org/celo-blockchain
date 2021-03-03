package bls12377

const N_LIMBS = 6
const FE_BYTE_SIZE = 48
const FE_BIT_SIZE = 377
const SIX_WORD_BIT_SIZE = 384
const SCALAR_FIELD_BIT_SIZE = 253
const ENCODED_FIELD_ELEMENT_SIZE = 64
const fourWordBitSize = 256

/*
 Field
 p = 0x01ae3a4617c510eac63b05c06ca1493b1a22d9f300f5138f1ef3622fba094800170b5d44300000008508c00000000001
 r = 2^384
*/

// modulus = p
var modulus = fe{0x8508c00000000001, 0x170b5d4430000000, 0x1ef3622fba094800, 0x1a22d9f300f5138f, 0xc63b05c06ca1493b, 0x01ae3a4617c510ea}

var (
	// -p^(-1) mod 2^64
	inp uint64 = 0x8508bfffffffffff
	// Suppress linter warning: This variable is used in assembly code
	_ = inp
)

// r1 = r mod p
var r1 = &fe{0x02cdffffffffff68, 0x51409f837fffffb1, 0x9f7db3a98a7d3ff2, 0x7b4e97b76e7c6305, 0x4cf495bf803c84e8, 0x008d6661e2fdf49a}

// zer0 = 0
var zero = &fe{}

// r^2 mod p
var r2 = &fe{
	0xb786686c9400cd22, 0x0329fcaab00431b1, 0x22a5f11162d6b46d, 0xbfdf7d03827dc3ac, 0x837e92f041790bf9, 0x006dfccb1e914b88,
}

// twoInv = 2 ^ (-1)
var twoInv = &fe{0x8166ffffffffffb4, 0x28a04fc1bfffffd8, 0xcfbed9d4c53e9ff9, 0x3da74bdbb73e3182, 0x267a4adfc01e4274, 0x0046b330f17efa4d}

// pMinus1Over2 = (p - 1) / 2
var pMinus1Over2 = bigFromHex("0x00d71d230be28875631d82e03650a49d8d116cf9807a89c78f79b117dd04a4000b85aea2180000004284600000000000")

// pMinus1Over4 = (p - 1) / 4
var pMinus1Over4 = bigFromHex("0x006b8e9185f1443ab18ec1701b28524ec688b67cc03d44e3c7bcd88bee82520005c2d7510c0000002142300000000000")

// sSqrt = ((p >> 46) - 1) / 2
var sSqrt = bigFromHex("0x035c748c2f8a21d58c760b80d94292763445b3e601ea271e3de6c45f741290002e16ba88600000010a11")

// nonResidue1 = -5
var nonResidue1 = &fe{0xfc0b8000000002fa, 0x97d39cf6e000018b, 0x2072420fbfa05044, 0xcbbcbd50d97c3802, 0x0baf1ec35813f9eb, 0x009974a2c0945ad2}

// nonResidue2 = (0 + 1 * u)
var nonResidue2 = &fe2{
	fe{0, 0, 0, 0, 0, 0},
	fe{0x02cdffffffffff68, 0x51409f837fffffb1, 0x9f7db3a98a7d3ff2, 0x7b4e97b76e7c6305, 0x4cf495bf803c84e8, 0x008d6661e2fdf49a},
}

// sNonResidue = nonResidue ^ s
var sNonResidue = &fe{0x1c104955744e6e0f, 0xf1bd15c3898dd1af, 0x76da78169a7f3950, 0xee086c1fe367c337, 0xf95564f4cbc1b61f, 0x00f3c1414ef58c54}

/*
	Curve
*/

// Group order
var q = bigFromHex("0x12ab655e9a2ca55660b44d1e5c37b00159aa76fed00000010a11800000000001")

// b coefficient for G1
// b = 1
var b = &fe{0x02cdffffffffff68, 0x51409f837fffffb1, 0x9f7db3a98a7d3ff2, 0x7b4e97b76e7c6305, 0x4cf495bf803c84e8, 0x008d6661e2fdf49a}

// b coefficient for G2
// b2 = (0, 0x10222f6db0fd6f343bd03737460c589dc7b4f91cd5fd889129207b63c6bf8000dd39e5c1ccccccd1c9ed9999999999a)
var b2 = &fe2{
	fe{0, 0, 0, 0, 0, 0},
	fe{0x8072266666666685, 0x8df55926899999a9, 0x7fe4561ad64f34cf, 0xb95da6d8b6e4f01b, 0x4b747cccfc142743, 0x0039c3fa70f49f43},
}

// G1 cofactor
var cofactorG1 = bigFromHex("0x170b5d44300000000000000000000000")

// G1 generator
// x = 0x008848defe740a67c8fc6225bf87ff5485951e2caa9d41bb188282c8bd37cb5cd5481512ffcd394eeab9b16eb21be9ef
// y = 0x1914a69c5102eff1f674f5d30afeec4bd7fb348ca3e52d96d182ad44fb82305c2fe3d3634a9591afd82de55559c8ea6
var g1One = PointG1{
	fe{0x260f33b9772451f4, 0xc54dd773169d5658, 0x5c1551c469a510dd, 0x761662e4425e1698, 0xc97d78cc6f065272, 0x00a41206b361fd4d},
	fe{0x8193961fb8cb81f3, 0x00638d4c5f44adb8, 0xfafaf3dad4daf54a, 0xc27849e2d655cd18, 0x2ec3ddb401d52814, 0x007da93326303c71},
	fe{0x02cdffffffffff68, 0x51409f837fffffb1, 0x9f7db3a98a7d3ff2, 0x7b4e97b76e7c6305, 0x4cf495bf803c84e8, 0x008d6661e2fdf49a},
}

var G1One = g1One

// G2 generator
// x0 = 0x018480be71c785fec89630a2a3841d01c565f071203e50317ea501f557db6b9b71889f52bb53540274e3e48f7c005196
// x1 = 0x00ea6040e700403170dc5a51b1b140d5532777ee6651cecbe7223ece0799c9de5cf89984bff76fe6b26bfefa6ea16afe
// y0 = 0x00690d665d446f7bd960736bcbb2efb4de03ed7274b49a58e458c282f832d204f2cf88886d8c7c2ef094094409fd4ddf
// y1 = 0x00f8169fd28355189e549da3151a70aa61ef11ac3d591bf12463b01acee304c24279b83f5e52270bd9a1cdd185eb8f93
var g2One = PointG2{
	fe2{
		fe{0x68904082f268725b, 0x668f2ea74f45328b, 0xebca7a65802be84f, 0x1e1850f4c1ada3e6, 0x830dc22d588ef1e9, 0x01862a81767c0982},
		fe{0x5f02a915c91c7f39, 0xf8c553ba388da2a7, 0xd51a416dbd198850, 0xe943c6f38ae3073a, 0xffe24aa8259a4981, 0x011853391e73dfdd},
	},
	fe2{
		fe{0xd5b19b897881430f, 0x05be9118a5b371ed, 0x6063f91f86c131ee, 0x3244a61be8f4ec19, 0xa02e425b9f9a3a12, 0x018af8c04f3360d2},
		fe{0x57601ac71a5b96f5, 0xe99acc1714f2440e, 0x2339612f10118ea9, 0x8321e68a3b1cd722, 0x2b543b050cc74917, 0x00590182b396c112},
	},
	fe2{
		fe{0x02cdffffffffff68, 0x51409f837fffffb1, 0x9f7db3a98a7d3ff2, 0x7b4e97b76e7c6305, 0x4cf495bf803c84e8, 0x008d6661e2fdf49a},
		fe{0x0000000000000000, 0x0000000000000000, 0x0000000000000000, 0x0000000000000000, 0x0000000000000000, 0x0000000000000000},
	},
}

var G2One = g2One

// Psi values untwist-frobenius-twist mapping
// z = u + 1

var psix = &fe2{
	// z ^ (( p ^ 1 - 1) / 3)
	fe{0x5892506da58478da, 0x133366940ac2a74b, 0x9b64a150cdf726cf, 0x5cc426090a9c587e, 0x5cf848adfdcd640c, 0x004702bf3ac02380},
	fe{0, 0, 0, 0, 0, 0},
}

var psiy = &fe2{
	// z ^ ((p ^ 3 - 1) / 6)
	fe{0x982c13d9d084771f, 0xfd49de0c6da34a32, 0x61a530d183ab0e53, 0xdf8fe44106dd9879, 0x40f29b58d88472bc, 0x0158723199046d5d},
	fe{0, 0, 0, 0, 0, 0},
}

/*
	Frobenius Coeffs
*/

// z = -1
var frobeniusCoeffs2 = [2]fe{
	// z ^ (( p ^ 0 - 1) / 2)
	{0x02cdffffffffff68, 0x51409f837fffffb1, 0x9f7db3a98a7d3ff2, 0x7b4e97b76e7c6305, 0x4cf495bf803c84e8, 0x008d6661e2fdf49a},
	// z ^ (( p ^ 1 - 1) / 2)
	{0x823ac00000000099, 0xc5cabdc0b000004f, 0x7f75ae862f8c080d, 0x9ed4423b9278b089, 0x79467000ec64c452, 0x0120d3e434c71c50},
}

// z = u + 1
var frobeniusCoeffs61 = [6]fe2{
	// z ^ (( p ^ 0 - 1) / 3)
	{
		fe{0x2cdffffffffff68, 0x51409f837fffffb1, 0x9f7db3a98a7d3ff2, 0x7b4e97b76e7c6305, 0x4cf495bf803c84e8, 0x008d6661e2fdf49a},
		fe{0, 0, 0, 0, 0, 0},
	},
	// z ^ (( p ^ 1 - 1) / 3)
	{
		fe{0x5892506da58478da, 0x133366940ac2a74b, 0x9b64a150cdf726cf, 0x5cc426090a9c587e, 0x5cf848adfdcd640c, 0x004702bf3ac02380},
		fe{0, 0, 0, 0, 0, 0},
	},
	// z ^ (( p ^ 2 - 1) / 3)
	{
		fe{0xdacd106da5847973, 0xd8fe2454bac2a79a, 0x1ada4fd6fd832edc, 0xfb9868449d150908, 0xd63eb8aeea32285e, 0x0167d6a36f873fd0},
		fe{0, 0, 0, 0, 0, 0},
	},
	// z ^ (( p ^ 3 - 1) / 3)
	{
		fe{0x823ac00000000099, 0xc5cabdc0b000004f, 0x7f75ae862f8c080d, 0x9ed4423b9278b089, 0x79467000ec64c452, 0x0120d3e434c71c50},
		fe{0, 0, 0, 0, 0, 0},
	},
	// z ^ (( p ^ 4 - 1) / 3)
	{
		fe{0x2c766f925a7b8727, 0x03d7f6b0253d58b5, 0x838ec0deec122131, 0xbd5eb3e9f658bb10, 0x6942bd126ed3e52e, 0x1673786dd04ed6a},
		fe{0, 0, 0, 0, 0, 0},
	},
	// z ^ (( p ^ 5 - 1) / 3)
	{
		fe{0xaa3baf925a7b868e, 0x3e0d38ef753d5865, 0x4191258bc861923, 0x1e8a71ae63e00a87, 0xeffc4d11826f20dc, 0x004663a2a83dd119},
		fe{0, 0, 0, 0, 0, 0},
	},
}

// z = u + 1
var frobeniusCoeffs62 = [6]fe2{
	// z ^ (( 2 * p ^ 0 - 2) / 3)
	{
		fe{0x02cdffffffffff68, 0x51409f837fffffb1, 0x9f7db3a98a7d3ff2, 0x7b4e97b76e7c6305, 0x4cf495bf803c84e8, 0x008d6661e2fdf49a},
		fe{0, 0, 0, 0, 0, 0},
	},
	// z ^ (( 2 * p ^ 1 - 2) / 3)
	{
		fe{0xdacd106da5847973, 0xd8fe2454bac2a79a, 0x1ada4fd6fd832edc, 0xfb9868449d150908, 0xd63eb8aeea32285e, 0x0167d6a36f873fd0},
		fe{0, 0, 0, 0, 0, 0},
	},
	// z ^ (( 2 * p ^ 2 - 2) / 3)
	{
		fe{0x2c766f925a7b8727, 0x03d7f6b0253d58b5, 0x838ec0deec122131, 0xbd5eb3e9f658bb10, 0x6942bd126ed3e52e, 0x01673786dd04ed6a},
		fe{0, 0, 0, 0, 0, 0},
	},
	// z ^ (( 2 * p ^ 3 - 2) / 3)
	{
		fe{0x2cdffffffffff68, 0x51409f837fffffb1, 0x9f7db3a98a7d3ff2, 0x7b4e97b76e7c6305, 0x4cf495bf803c84e8, 0x008d6661e2fdf49a},
		fe{0, 0, 0, 0, 0, 0},
	},
	// z ^ (( 2 * p ^ 4 - 2) / 3)
	{
		fe{0xdacd106da5847973, 0xd8fe2454bac2a79a, 0x1ada4fd6fd832edc, 0xfb9868449d150908, 0xd63eb8aeea32285e, 0x0167d6a36f873fd0},
		fe{0, 0, 0, 0, 0, 0},
	},
	// z ^ (( 2 * p ^ 5 - 2) / 3)
	{
		fe{0x2c766f925a7b8727, 0x03d7f6b0253d58b5, 0x838ec0deec122131, 0xbd5eb3e9f658bb10, 0x6942bd126ed3e52e, 0x01673786dd04ed6a},
		fe{0, 0, 0, 0, 0, 0},
	},
}

var frobeniusCoeffs12 = [12]fe2{
	// z = u + 1
	// z ^ ((p ^ 0 - 1) / 6)
	{
		fe{0x02cdffffffffff68, 0x51409f837fffffb1, 0x9f7db3a98a7d3ff2, 0x7b4e97b76e7c6305, 0x4cf495bf803c84e8, 0x008d6661e2fdf49a},
		fe{0, 0, 0, 0, 0, 0},
	},
	// z ^ ((p ^ 1 - 1) / 6)
	{
		fe{0x6ec47a04a3f7ca9e, 0xa42e0cb968c1fa44, 0x578d5187fbd2bd23, 0x930eeb0ac79dd4bd, 0xa24883de1e09a9ee, 0x00daa7058067d46f},
		fe{0, 0, 0, 0, 0, 0},
	},
	// z ^ ((p ^ 2 - 1) / 6)
	{
		fe{0x5892506da58478da, 0x133366940ac2a74b, 0x9b64a150cdf726cf, 0x5cc426090a9c587e, 0x5cf848adfdcd640c, 0x004702bf3ac02380},
		fe{0, 0, 0, 0, 0, 0},
	},
	// z ^ ((p ^ 3 - 1) / 6)
	{
		fe{0x982c13d9d084771f, 0xfd49de0c6da34a32, 0x61a530d183ab0e53, 0xdf8fe44106dd9879, 0x40f29b58d88472bc, 0x0158723199046d5d},
		fe{0, 0, 0, 0, 0, 0},
	},
	// z ^ ((p ^ 4 - 1) / 6)
	{
		fe{0xdacd106da5847973, 0xd8fe2454bac2a79a, 0x1ada4fd6fd832edc, 0xfb9868449d150908, 0xd63eb8aeea32285e, 0x167d6a36f873fd0},
		fe{0, 0, 0, 0, 0, 0},
	},
	// z ^ ((p ^ 5 - 1) / 6)
	{
		fe{0x296799d52c8cac81, 0x591bd15304e14fee, 0xa17df4987d85130, 0x4c80f9363f3fc3bc, 0x9eaa177aba7ac8ce, 0x7dcb2c189c98ed},
		fe{0, 0, 0, 0, 0, 0},
	},
	// z ^ ((p ^ 6 - 1) / 6)
	{
		fe{0x823ac00000000099, 0xc5cabdc0b000004f, 0x7f75ae862f8c080d, 0x9ed4423b9278b089, 0x79467000ec64c452, 0x120d3e434c71c50},
		fe{0, 0, 0, 0, 0, 0},
	},
	// z ^ ((p ^ 7 - 1) / 6)
	{
		fe{0x164445fb5c083563, 0x72dd508ac73e05bc, 0xc76610a7be368adc, 0x8713eee839573ed1, 0x23f281e24e979f4c, 0x0d39340975d3c7b},
		fe{0, 0, 0, 0, 0, 0},
	},
	// z ^ ((p ^ 8 - 1) / 6)
	{
		fe{0x2c766f925a7b8727, 0x3d7f6b0253d58b5, 0x838ec0deec122131, 0xbd5eb3e9f658bb10, 0x6942bd126ed3e52e, 0x01673786dd04ed6a},
		fe{0, 0, 0, 0, 0, 0},
	},
	// z ^ ((p ^ 9 - 1) / 6)
	{
		fe{0xecdcac262f7b88e2, 0x19c17f37c25cb5cd, 0xbd4e315e365e39ac, 0x3a92f5b1fa177b15, 0x85486a67941cd67e, 0x0055c8147ec0a38d},
		fe{0, 0, 0, 0, 0, 0},
	},
	// z ^ ((p ^ 10 - 1) / 6)
	{
		fe{0xaa3baf925a7b868e, 0x3e0d38ef753d5865, 0x4191258bc861923, 0x1e8a71ae63e00a87, 0xeffc4d11826f20dc, 0x004663a2a83dd119},
		fe{0, 0, 0, 0, 0, 0},
	},
	// z ^ ((p ^ 11 - 1) / 6)
	{
		fe{0x5ba1262ad3735380, 0xbdef8bf12b1eb012, 0x14db82e63230f6cf, 0xcda1e0bcc1b54fd3, 0x2790ee45b226806c, 0x01306f19ff2877fd},
		fe{0, 0, 0, 0, 0, 0},
	},
}

/*
	x
*/

var x = bigFromHex("0x8508c00000000001")
