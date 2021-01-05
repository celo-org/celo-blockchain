package bw6

import "math/big"

var glvQ2 = bigFromHex("0x072030ba8ee9c0643042fb73b015bd4eeda5ba6bfab7176f0a")

var glvB1 = bigFromHex("0x0bf9b117dd04a4002e16ba886000000058b0800000000001")

var glvQ1 = bigFromHex("0x072030ba8ee9c0643042fb73b015bd4e9e7ccf39ddb5613b6f")

var glvB2 = bigFromHex("0x0bf9b117dd04a4002e16ba885fffffffd3a7bfffffffffff")

// glvLambda
var glvLambda = bigFromHex("0x9b3af05dd14f6ec619aaf7d34594aabc5ed1347970dec00452217cc900000008508c00000000001")

// glvPhi2 ^ 3 = 1
var glvPhi1 = &fe{7467050525960156664, 11327349735975181567, 4886471689715601876, 825788856423438757, 532349992164519008, 5190235139112556877, 10134108925459365126, 2188880696701890397, 14832254987849135908, 2933451070611009188, 11385631952165834796, 64130670718986244}

// glvPhi1 ^ 3 = 1
var glvPhi2 = &fe{9193734820520314185, 15390913228415833887, 5309822015742495676, 5431732283202763350, 17252325881282386417, 298854800984767943, 15252629665615712253, 11476276919959978448, 6617989123466214626, 293279592164056124, 3271178847573361778, 76563709148138387}

// halfR = 2**384 / 2
var halfR = bigFromHex("0x800000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000")

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
	if v.k2.Cmp(zero) < 0 {
		naf2.neg()
	}
	return naf1, naf2
}

func (v *glvVector) new(k *big.Int) *glvVector {

	// Implementation is adapted from
	// https://github.com/celo-org/zexe/blob/master/algebra-core/src/curves/glv.rs

	c1 := new(big.Int)
	c1.Mul(k, glvQ1)
	c1.Add(c1, halfR)
	c1.Rsh(c1, sixWordBitSize)

	c2 := new(big.Int)
	c2.Mul(k, glvQ2)
	c2.Add(c2, halfR)
	c2.Rsh(c2, sixWordBitSize)

	d1 := new(big.Int).Mul(c1, glvB1)
	d1.Mod(d1, q)
	if d1.Cmp(q) == 1 {
		d1.Sub(d1, q)
	}

	d2 := new(big.Int).Mul(c2, glvB2)
	d2.Mod(d2, q)
	if d2.Cmp(q) == 1 {
		d2.Sub(d2, q)
	}

	k2 := new(big.Int).Sub(d1, d2)
	k1 := new(big.Int)
	k1.Mul(k2, glvLambda).Mod(k1, q)
	k1.Sub(k, k1)

	v.k1 = new(big.Int).Set(k1)
	v.k2 = new(big.Int).Set(k2)
	return v
}

func (g *G) glvEndomorphismG1(r, p *Point) {
	t := g.Affine(p)
	if g.IsZero(p) {
		r.Zero()
		return
	}
	r[1].set(&t[1])
	mul(&r[0], &t[0], glvPhi1)
	r[2].one()
}

func (g *G) glvEndomorphismG2(r, p *Point) {
	t := g.Affine(p)
	if g.IsZero(p) {
		r.Zero()
		return
	}
	r[1].set(&t[1])
	mul(&r[0], &t[0], glvPhi2)
	r[2].one()
}
