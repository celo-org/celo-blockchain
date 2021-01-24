package bls12377

import (
	"errors"
	"math"
	"math/big"
)

// PointG2 is type for point in G2.
// PointG2 is both used for Affine and Jacobian point representation.
// If z is equal to one the point is accounted as in affine form.
type PointG2 [3]fe2

// Set copies valeus of one point to another.
func (p *PointG2) Set(p2 *PointG2) *PointG2 {
	p[0].set(&p2[0])
	p[1].set(&p2[1])
	p[2].set(&p2[2])
	return p
}

func (p *PointG2) Zero() *PointG2 {
	p[0].zero()
	p[1].one()
	p[2].zero()
	return p

}

type tempG2 struct {
	t [9]*fe2
}

// G2 is struct for G2 group.
type G2 struct {
	f *fp2
	tempG2
}

// NewG2 constructs a new G2 instance.
func NewG2() *G2 {
	return newG2(nil)
}

func newG2(f *fp2) *G2 {
	if f == nil {
		f = newFp2()
	}
	t := newTempG2()
	return &G2{f, t}
}

func newTempG2() tempG2 {
	t := [9]*fe2{}
	for i := 0; i < 9; i++ {
		t[i] = &fe2{}
	}
	return tempG2{t}
}

// Q returns group order in big.Int.
func (g *G2) Q() *big.Int {
	return new(big.Int).Set(q)
}

// FromBytes constructs a new point given uncompressed byte input.
// Input string expected to be 192 bytes and concatenation of x and y values
// Point (0, 0) is considered as infinity.
func (g *G2) FromBytes(in []byte) (*PointG2, error) {
	if len(in) != 4*FE_BYTE_SIZE {
		return nil, errors.New("input string should be 192 bytes")
	}
	x, err := g.f.fromBytes(in[:2*FE_BYTE_SIZE])
	if err != nil {
		return nil, err
	}
	y, err := g.f.fromBytes(in[2*FE_BYTE_SIZE:])
	if err != nil {
		return nil, err
	}
	// check if given input points to infinity
	if x.isZero() && y.isZero() {
		return g.Zero(), nil
	}
	z := new(fe2).one()
	p := &PointG2{*x, *y, *z}
	if !g.IsOnCurve(p) {
		return nil, errors.New("point is not on curve")
	}
	return p, nil
}

// DecodePoint given encoded (x, y) coordinates in 256 bytes returns a valid G1 Point.
func (g *G2) DecodePoint(in []byte) (*PointG2, error) {
	if len(in) != 4*ENCODED_FIELD_ELEMENT_SIZE {
		return nil, errors.New("invalid g2 point length")
	}
	pointBytes := make([]byte, 4*FE_BYTE_SIZE)
	x0Bytes, err := decodeFieldElement(in[:ENCODED_FIELD_ELEMENT_SIZE])
	if err != nil {
		return nil, err
	}
	x1Bytes, err := decodeFieldElement(in[ENCODED_FIELD_ELEMENT_SIZE : 2*ENCODED_FIELD_ELEMENT_SIZE])
	if err != nil {
		return nil, err
	}
	y0Bytes, err := decodeFieldElement(in[2*ENCODED_FIELD_ELEMENT_SIZE : 3*ENCODED_FIELD_ELEMENT_SIZE])
	if err != nil {
		return nil, err
	}
	y1Bytes, err := decodeFieldElement(in[3*ENCODED_FIELD_ELEMENT_SIZE:])
	if err != nil {
		return nil, err
	}
	copy(pointBytes[:FE_BYTE_SIZE], x0Bytes)
	copy(pointBytes[FE_BYTE_SIZE:2*FE_BYTE_SIZE], x1Bytes)
	copy(pointBytes[2*FE_BYTE_SIZE:3*FE_BYTE_SIZE], y0Bytes)
	copy(pointBytes[3*FE_BYTE_SIZE:], y1Bytes)
	return g.FromBytes(pointBytes)
}

// ToBytes serializes a point into bytes in uncompressed form,
// returns (0, 0) if point is at infinity.
func (g *G2) ToBytes(p *PointG2) []byte {
	out := make([]byte, 4*FE_BYTE_SIZE)
	if g.IsZero(p) {
		return out
	}
	g.Affine(p)
	copy(out[:2*FE_BYTE_SIZE], g.f.toBytes(&p[0]))
	copy(out[2*FE_BYTE_SIZE:], g.f.toBytes(&p[1]))
	return out
}

// EncodePoint encodes a point into 256 bytes.
func (g *G2) EncodePoint(p *PointG2) []byte {
	// outRaw is 96 bytes
	outRaw := g.ToBytes(p)
	out := make([]byte, 4*ENCODED_FIELD_ELEMENT_SIZE)
	// encode x
	copy(out[ENCODED_FIELD_ELEMENT_SIZE-FE_BYTE_SIZE:ENCODED_FIELD_ELEMENT_SIZE], outRaw[:FE_BYTE_SIZE])
	copy(out[2*ENCODED_FIELD_ELEMENT_SIZE-FE_BYTE_SIZE:2*ENCODED_FIELD_ELEMENT_SIZE], outRaw[FE_BYTE_SIZE:2*FE_BYTE_SIZE])
	// encode y
	copy(out[3*ENCODED_FIELD_ELEMENT_SIZE-FE_BYTE_SIZE:3*ENCODED_FIELD_ELEMENT_SIZE], outRaw[2*FE_BYTE_SIZE:3*FE_BYTE_SIZE])
	copy(out[4*ENCODED_FIELD_ELEMENT_SIZE-FE_BYTE_SIZE:], outRaw[3*FE_BYTE_SIZE:])
	return out
}

// New creates a new G2 Point which is equal to zero in other words point at infinity.
func (g *G2) New() *PointG2 {
	return new(PointG2).Zero()
}

// Zero returns a new G2 Point which is equal to point at infinity.
func (g *G2) Zero() *PointG2 {
	return new(PointG2).Zero()
}

// One returns a new G2 Point which is equal to generator point.
func (g *G2) One() *PointG2 {
	p := &PointG2{}
	return p.Set(&g2One)
}

// IsZero returns true if given point is equal to zero.
func (g *G2) IsZero(p *PointG2) bool {
	return p[2].isZero()
}

// Equal checks if given two G2 point is equal in their affine form.
func (g *G2) Equal(p1, p2 *PointG2) bool {
	if g.IsZero(p1) {
		return g.IsZero(p2)
	}
	if g.IsZero(p2) {
		return g.IsZero(p1)
	}
	t := g.t
	g.f.square(t[0], &p1[2])
	g.f.square(t[1], &p2[2])
	g.f.mul(t[2], t[0], &p2[0])
	g.f.mul(t[3], t[1], &p1[0])
	g.f.mul(t[0], t[0], &p1[2])
	g.f.mul(t[1], t[1], &p2[2])
	g.f.mul(t[1], t[1], &p1[1])
	g.f.mul(t[0], t[0], &p2[1])
	return t[0].equal(t[1]) && t[2].equal(t[3])
}

// InCorrectSubgroup checks whether given point is in correct subgroup.
func (g *G2) InCorrectSubgroup(p *PointG2) bool {
	tmp := &PointG2{}
	g.MulScalar(tmp, p, q)
	return g.IsZero(tmp)
}

// IsOnCurve checks a G2 point is on curve.
func (g *G2) IsOnCurve(p *PointG2) bool {
	if g.IsZero(p) {
		return true
	}
	t := g.t
	g.f.square(t[0], &p[1])
	g.f.square(t[1], &p[0])
	g.f.mul(t[1], t[1], &p[0])
	g.f.square(t[2], &p[2])
	g.f.square(t[3], t[2])
	g.f.mul(t[2], t[2], t[3])
	g.f.mul(t[2], b2, t[2])
	g.f.add(t[1], t[1], t[2])
	return t[0].equal(t[1])
}

// IsAffine checks a G2 point whether it is in affine form.
func (g *G2) IsAffine(p *PointG2) bool {
	return p[2].isOne()
}

// Affine calculates affine form of given G2 point.
func (g *G2) Affine(p *PointG2) *PointG2 {
	if g.IsZero(p) {
		return p
	}
	if !g.IsAffine(p) {
		t := g.t
		g.f.inverse(t[0], &p[2])
		g.f.square(t[1], t[0])
		g.f.mul(&p[0], &p[0], t[1])
		g.f.mul(t[0], t[0], t[1])
		g.f.mul(&p[1], &p[1], t[0])
		p[2].one()
	}
	return p
}

// AffineBatch given multiple of points returns affine representations
func (g *G2) AffineBatch(p []*PointG2) {
	inverses := make([]fe2, len(p))
	for i := 0; i < len(p); i++ {
		inverses[i].set(&p[i][2])
	}
	g.f.inverseBatch(inverses)
	t := g.t
	for i := 0; i < len(p); i++ {
		if !g.IsAffine(p[i]) && !g.IsZero(p[i]) {
			g.f.square(t[1], &inverses[i])
			g.f.mul(&p[i][0], &p[i][0], t[1])
			g.f.mul(t[0], &inverses[i], t[1])
			g.f.mul(&p[i][1], &p[i][1], t[0])
			p[i][2].one()
		}
	}
}

// Add adds two G2 points p1, p2 and assigns the result to point at first argument.
func (g *G2) Add(r, p1, p2 *PointG2) *PointG2 {
	// http://www.hyperelliptic.org/EFD/gp/auto-shortw-jacobian-0.html#addition-add-2007-bl
	if g.IsAffine(p2) {
		return g.addMixed(r, p1, p2)
	}
	if g.IsZero(p1) {
		return r.Set(p2)
	}
	if g.IsZero(p2) {
		return r.Set(p1)
	}
	t := g.t
	g.f.square(t[7], &p1[2])
	g.f.mul(t[1], &p2[0], t[7])
	g.f.mul(t[2], &p1[2], t[7])
	g.f.mul(t[0], &p2[1], t[2])
	g.f.square(t[8], &p2[2])
	g.f.mul(t[3], &p1[0], t[8])
	g.f.mul(t[4], &p2[2], t[8])
	g.f.mul(t[2], &p1[1], t[4])
	if t[1].equal(t[3]) {
		if t[0].equal(t[2]) {
			return g.Double(r, p1)
		} else {
			return r.Zero()
		}
	}
	g.f.sub(t[1], t[1], t[3])
	g.f.double(t[4], t[1])
	g.f.square(t[4], t[4])
	g.f.mul(t[5], t[1], t[4])
	g.f.sub(t[0], t[0], t[2])
	g.f.double(t[0], t[0])
	g.f.square(t[6], t[0])
	g.f.sub(t[6], t[6], t[5])
	g.f.mul(t[3], t[3], t[4])
	g.f.double(t[4], t[3])
	g.f.sub(&r[0], t[6], t[4])
	g.f.sub(t[4], t[3], &r[0])
	g.f.mul(t[6], t[2], t[5])
	g.f.double(t[6], t[6])
	g.f.mul(t[0], t[0], t[4])
	g.f.sub(&r[1], t[0], t[6])
	g.f.add(t[0], &p1[2], &p2[2])
	g.f.square(t[0], t[0])
	g.f.sub(t[0], t[0], t[7])
	g.f.sub(t[0], t[0], t[8])
	g.f.mul(&r[2], t[0], t[1])
	return r
}

// Add adds two G1 points p1, p2 and assigns the result to point at first argument.
// Expects point p2 in affine form.
func (g *G2) addMixed(r, p1, p2 *PointG2) *PointG2 {
	// http://www.hyperelliptic.org/EFD/g1p/auto-shortw-jacobian-0.html#addition-madd-2007-bl
	if g.IsZero(p1) {
		return r.Set(p2)
	}
	if g.IsZero(p2) {
		return r.Set(p1)
	}
	t := g.t
	g.f.square(t[7], &p1[2])    // z1z1
	g.f.mul(t[1], &p2[0], t[7]) // u2 = x2 * z1z1
	g.f.mul(t[2], &p1[2], t[7]) // z1z1 * z1
	g.f.mul(t[0], &p2[1], t[2]) // s2 = y2 * z1z1 * z1

	if p1[0].equal(t[1]) && p1[1].equal(t[0]) {
		return g.Double(r, p1)
	}

	g.f.sub(t[1], t[1], &p1[0]) // h = u2 - x1
	g.f.square(t[2], t[1])      // hh
	g.f.double(t[4], t[2])
	g.f.double(t[4], t[4])      // 4hh
	g.f.mul(t[5], t[1], t[4])   // j = h*i
	g.f.sub(t[0], t[0], &p1[1]) // s2 - y1
	g.f.double(t[0], t[0])      // r = 2*(s2 - y1)
	g.f.square(t[6], t[0])      // r^2
	g.f.sub(t[6], t[6], t[5])   // r^2 - j
	g.f.mul(t[3], &p1[0], t[4]) // v = x1 * i
	g.f.double(t[4], t[3])      // 2*v
	g.f.sub(&r[0], t[6], t[4])  // x3 = r^2 - j - 2*v
	g.f.sub(t[4], t[3], &r[0])  // v - x3
	g.f.mul(t[6], &p1[1], t[5]) // y1 * j
	g.f.double(t[6], t[6])      // 2 * y1 * j
	g.f.mul(t[0], t[0], t[4])   // r * (v - x3)
	g.f.sub(&r[1], t[0], t[6])  // y3 = r * (v - x3) - (2 * y1 * j)
	g.f.add(t[0], &p1[2], t[1]) // z1 + h
	g.f.square(t[0], t[0])      // (z1 + h)^2
	g.f.sub(t[0], t[0], t[7])   // (z1 + h)^2 - z1z1
	g.f.sub(&r[2], t[0], t[2])  // z3 = (z1 + z2)^2 - z1z1 - hh
	return r
}

// Double doubles a G2 point p and assigns the result to the point at first argument.
func (g *G2) Double(r, p *PointG2) *PointG2 {
	// http://www.hyperelliptic.org/EFD/gp/auto-shortw-jacobian-0.html#doubling-dbl-2009-l
	if g.IsZero(p) {
		return r.Set(p)
	}
	t := g.t
	g.f.square(t[0], &p[0])
	g.f.square(t[1], &p[1])
	g.f.square(t[2], t[1])
	g.f.add(t[1], &p[0], t[1])
	g.f.square(t[1], t[1])
	g.f.sub(t[1], t[1], t[0])
	g.f.sub(t[1], t[1], t[2])
	g.f.double(t[1], t[1])
	g.f.double(t[3], t[0])
	g.f.add(t[0], t[3], t[0])
	g.f.square(t[4], t[0])
	g.f.double(t[3], t[1])
	g.f.sub(&r[0], t[4], t[3])
	g.f.sub(t[1], t[1], &r[0])
	g.f.double(t[2], t[2])
	g.f.double(t[2], t[2])
	g.f.double(t[2], t[2])
	g.f.mul(t[0], t[0], t[1])
	g.f.sub(t[1], t[0], t[2])
	g.f.mul(t[0], &p[1], &p[2])
	r[1].set(t[1])
	g.f.double(&r[2], t[0])
	return r
}

// Neg negates a G2 point p and assigns the result to the point at first argument.
func (g *G2) Neg(r, p *PointG2) *PointG2 {
	r[0].set(&p[0])
	g.f.neg(&r[1], &p[1])
	r[2].set(&p[2])
	return r
}

// Sub subtracts two G2 points p1, p2 and assigns the result to point at first argument.
func (g *G2) Sub(c, a, b *PointG2) *PointG2 {
	d := &PointG2{}
	g.Neg(d, b)
	g.Add(c, a, d)
	return c
}

// MulScalar multiplies a point by given scalar value in big.Int and assigns the result to point at first argument.
func (g *G2) MulScalar(c, p *PointG2, e *big.Int) *PointG2 {
	q, n := &PointG2{}, &PointG2{}
	n.Set(p)
	l := e.BitLen()
	for i := 0; i < l; i++ {
		if e.Bit(i) == 1 {
			g.Add(q, q, n)
		}
		g.Double(n, n)
	}
	return c.Set(q)
}

// ClearCofactor maps given a G2 point to correct subgroup
func (g *G2) ClearCofactor(p *PointG2) *PointG2 {
	return g.wnafMul(p, p, cofactorG2)
}

// MultiExp calculates multi exponentiation. Given pairs of G1 point and scalar values
// (P_0, e_0), (P_1, e_1), ... (P_n, e_n) calculates r = e_0 * P_0 + e_1 * P_1 + ... + e_n * P_n
// Length of points and scalars are expected to be equal, otherwise an error is returned.
// Result is assigned to point at first argument.
func (g *G2) MultiExp(r *PointG2, points []*PointG2, scalars []*big.Int) (*PointG2, error) {
	if len(points) != len(scalars) {
		return nil, errors.New("point and scalar vectors should be in same length")
	}

	g.AffineBatch(points)

	c := 3
	if len(scalars) >= 32 {
		c = int(math.Ceil(math.Log(float64(len(scalars)))))
	}

	bucketSize := (1 << c) - 1
	windows := make([]*PointG2, SCALAR_FIELD_BIT_SIZE/c+1)
	bucket := make([]PointG2, bucketSize)

	for j := 0; j < len(windows); j++ {

		for i := 0; i < bucketSize; i++ {
			bucket[i].Zero()
		}

		for i := 0; i < len(scalars); i++ {
			index := bucketSize & int(new(big.Int).Rsh(scalars[i], uint(c*j)).Int64())
			if index != 0 {
				g.addMixed(&bucket[index-1], &bucket[index-1], points[i])
			}
		}

		acc, sum := g.New(), g.New()
		for i := bucketSize - 1; i >= 0; i-- {
			g.Add(sum, sum, &bucket[i])
			g.Add(acc, acc, sum)
		}
		windows[j] = g.New().Set(acc)
	}

	g.AffineBatch(windows)

	acc := g.New()
	for i := len(windows) - 1; i >= 0; i-- {
		for j := 0; j < c; j++ {
			g.Double(acc, acc)
		}
		g.addMixed(acc, acc, windows[i])
	}
	return r.Set(acc), nil
}

func (g *G2) wnafMul(c, p *PointG2, e *big.Int) *PointG2 {
	windowSize := 6

	l := (1 << (windowSize - 1))
	tablePositive := make([]PointG2, l)
	tableNegative := make([]PointG2, l)

	twoP, acc := g.New(), new(PointG2).Set(p)
	g.Double(twoP, p)

	// p
	tablePositive[0].Set(acc)
	// -p
	g.Neg(&tableNegative[0], acc)

	for i := 1; i < l; i++ {
		g.Add(acc, acc, twoP)
		// 3p, 5p, 7p ...
		tablePositive[i].Set(acc)
		// -3p, -5p, -7p ...
		g.Neg(&tableNegative[i], acc)
	}

	wnaf := toWNAF(e, windowSize)

	q := g.Zero()

	for i := len(wnaf) - 1; i >= 0; i-- {

		if wnaf[i] > 0 {

			g.Add(q, q, &tablePositive[wnaf[i]>>1])
		} else if wnaf[i] < 0 {

			g.Add(q, q, &tableNegative[(-wnaf[i])>>1])
		}

		if i != 0 {
			g.Double(q, q)
		}

	}
	return c.Set(q)
}
