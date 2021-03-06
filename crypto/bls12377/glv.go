package bls12377

import (
	"math/big"
)

// Guide to Pairing Based Cryptography
// 6.3.2. Decompositions for the k = 12 BLS Family

// glvQ1 = x^2 * R / q

var glvQ1Big = bigFromHex("0x3b3f7aa969fd371607f72ed32af90182c")

// glvQ2 = R / q = 14
var glvQ2Big = bigFromHex("0x0e")

// glvB1 = x^2 - 1 = 0x452217cc900000010a11800000000000
var glvB1 = bigFromHex("0x452217cc900000010a11800000000000")

// glvB2 = x^2 = 0x452217cc900000010a11800000000001
var glvB2 = bigFromHex("0x452217cc900000010a11800000000001")

// glvLambda = x^2 - 1
var glvLambda = bigFromHex("0x452217cc900000010a11800000000000")

// glvPhi1 ^ 3 = 1
var glvPhi1 = &fe{0xdacd106da5847973, 0xd8fe2454bac2a79a, 0x1ada4fd6fd832edc, 0xfb9868449d150908, 0xd63eb8aeea32285e, 0x167d6a36f873fd0}

// glvPhi2 ^ 3 = 1
var glvPhi2 = &fe{0x2c766f925a7b8727, 0x03d7f6b0253d58b5, 0x838ec0deec122131, 0xbd5eb3e9f658bb10, 0x6942bd126ed3e52e, 0x01673786dd04ed6a}

var glvMulWindowG1 uint = 4
var glvMulWindowG2 uint = 4

// halfR = 2**256 / 2
var halfR = bigFromHex("0x8000000000000000000000000000000000000000000000000000000000000000")

type glvVector struct {
	k1 *big.Int
	k2 *big.Int
}

func (v *glvVector) wnaf(w uint) (nafNumber, nafNumber) {
	naf1, naf2 := toWNAF(v.k1, w), toWNAF(v.k2, w)
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
	alpha1 := new(big.Int).Mul(m, glvQ1Big)
	alpha1.Add(alpha1, halfR)
	alpha1.Rsh(alpha1, fourWordBitSize)

	// alpha2 = round(m / r)
	alpha2 := new(big.Int).Mul(m, glvQ2Big)
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
