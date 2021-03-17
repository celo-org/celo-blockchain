package bls12381

import (
	"math/big"
)

// Guide to Pairing Based Cryptography
// 6.3.2. Decompositions for the k = 12 BLS Family

// glvQ1 = x^2 * R / q
var glvQ1 = bigFromHex("0x017c6becf1e01faadd63f6e522f6cfee30")

// glvQ2 = R / q = 2
var glvQ2 = bigFromHex("0x02")

// glvB1 = x^2 - 1 = 0xac45a4010001a40200000000ffffffff
var glvB1 = bigFromHex("0xac45a4010001a40200000000ffffffff")

// glvB2 = x^2 = 0xac45a4010001a4020000000100000000
var glvB2 = bigFromHex("0xac45a4010001a4020000000100000000")

// glvLambdaA = x^2 - 1
var glvLambda = bigFromHex("0xac45a4010001a40200000000ffffffff")

// halfR = 2**256 / 2
var halfR = bigFromHex("0x8000000000000000000000000000000000000000000000000000000000000000")

// glvPhi1 ^ 3 = 1
var glvPhi1 = &fe{0xcd03c9e48671f071, 0x5dab22461fcda5d2, 0x587042afd3851b95, 0x8eb60ebe01bacb9e, 0x03f97d6e83d050d2, 0x18f0206554638741}

// glvPhi2 ^ 3 = 1
var glvPhi2 = &fe{0x30f1361b798a64e8, 0xf3b8ddab7ece5a2a, 0x16a8ca3ac61577f7, 0xc26a2ff874fd029b, 0x3636b76660701c6e, 0x051ba4ab241b6160}

var glvMulWindowG1 uint = 4
var glvMulWindowG2 uint = 4

type glvVector struct {
	k1 *big.Int
	k2 *big.Int
}

func (v *glvVector) wnaf(w uint) (nafNumber, nafNumber) {
	naf1, naf2 := bigToWNAF(v.k1, w), bigToWNAF(v.k2, w)
	zero := new(big.Int)
	if v.k1.Cmp(zero) < 0 {
		naf1.neg()
	}
	if v.k2.Cmp(zero) > 0 {
		naf2.neg()
	}
	return naf1, naf2
}

func (v *glvVector) new(m *big.Int) *glvVector {
	// Guide to Pairing Based Cryptography
	// 6.3.2. Decompositions for the k = 12 BLS Family

	// alpha1 = round(x^2 * m  / r)
	alpha1 := new(big.Int).Mul(m, glvQ1)
	alpha1.Add(alpha1, halfR)
	alpha1.Rsh(alpha1, fourWordBitSize)

	// alpha2 = round(m / r)
	alpha2 := new(big.Int).Mul(m, glvQ2)
	alpha2.Add(alpha2, halfR)
	alpha2.Rsh(alpha2, fourWordBitSize)

	z1, z2 := new(big.Int), new(big.Int)
	// z1 = (x^2 - 1) * round(x^2 * m  / r)
	z1.Mul(alpha1, glvB1).Mod(z1, q)
	// z2 = x^2 * round(m / r)
	z2.Mul(alpha2, glvB2).Mod(z2, q)

	k1, k2 := new(big.Int), new(big.Int)

	// k1 = m - z1 - alpha2
	k1.Sub(m, z1)
	k1.Sub(k1, alpha2)

	// k2 = z2 - alpha1
	k2.Sub(z2, alpha1)

	v.k1 = new(big.Int).Set(k1)
	v.k2 = new(big.Int).Set(k2)
	return v
}

func phi(a, b *fe) {
	mul(a, b, glvPhi1)
}

func (e *fp2) phi(a, b *fe2) {
	mul(&a[0], &b[0], glvPhi2)
	mul(&a[1], &b[1], glvPhi2)
}

func (g *G1) glvEndomorphism(r, p *PointG1) {
	t := g.Affine(p)
	if g.IsZero(p) {
		r.Zero()
		return
	}
	r[1].set(&t[1])
	phi(&r[0], &t[0])
	r[2].one()
}

func (g *G2) glvEndomorphism(r, p *PointG2) {
	t := g.Affine(p)
	if g.IsZero(p) {
		r.Zero()
		return
	}
	r[1].set(&t[1])
	g.f.phi(&r[0], &t[0])
	r[2].one()
}
