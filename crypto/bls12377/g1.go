package bls12377

import (
	"errors"
	"math"
	"math/big"
)

// PointG1 is type for point in G1.
// PointG1 is both used for Affine and Jacobian point representation.
// If z is equal to one the point is accounted as in affine form.
type PointG1 [3]fe

func (p *PointG1) Set(p2 *PointG1) *PointG1 {
	p[0].set(&p2[0])
	p[1].set(&p2[1])
	p[2].set(&p2[2])
	return p
}

func (p *PointG1) Zero() *PointG1 {
	p[0].zero()
	p[1].one()
	p[2].zero()
	return p
}

type tempG1 struct {
	t [9]*fe
}

// G1 is struct for G1 group.
type G1 struct {
	tempG1
}

// NewG1 constructs a new G1 instance.
func NewG1() *G1 {
	t := newTempG1()
	return &G1{t}
}

func newTempG1() tempG1 {
	t := [9]*fe{}
	for i := 0; i < 9; i++ {
		t[i] = &fe{}
	}
	return tempG1{t}
}

// Q returns group order in big.Int.
func (g *G1) Q() *big.Int {
	return new(big.Int).Set(q)
}

// FromBytes constructs a new point given uncompressed byte input.
// Input string is expected to be equal to 96 bytes and concatenation of x and y cooridanates.
// (0, 0) is considered as infinity.
func (g *G1) FromBytes(in []byte) (*PointG1, error) {
	if len(in) != 2*FE_BYTE_SIZE {
		return nil, errors.New("input string should be equal or larger than 96")
	}
	p0, err := fromBytes(in[:FE_BYTE_SIZE])
	if err != nil {
		return nil, err
	}
	p1, err := fromBytes(in[FE_BYTE_SIZE:])
	if err != nil {
		return nil, err
	}
	// check if given input points to infinity
	if p0.isZero() && p1.isZero() {
		return g.Zero(), nil
	}
	p2 := new(fe).one()
	p := &PointG1{*p0, *p1, *p2}
	if !g.IsOnCurve(p) {
		return nil, errors.New("point is not on curve")
	}
	return p, nil
}

// DecodePoint given encoded (x, y) coordinates in 128 bytes returns a valid G1 Point.
func (g *G1) DecodePoint(in []byte) (*PointG1, error) {
	if len(in) != 2*ENCODED_FIELD_ELEMENT_SIZE {
		return nil, errors.New("invalid g1 point length")
	}
	pointBytes := make([]byte, 2*FE_BYTE_SIZE)
	// decode x
	xBytes, err := decodeFieldElement(in[:ENCODED_FIELD_ELEMENT_SIZE])
	if err != nil {
		return nil, err
	}
	// decode y
	yBytes, err := decodeFieldElement(in[ENCODED_FIELD_ELEMENT_SIZE:])
	if err != nil {
		return nil, err
	}
	copy(pointBytes[:FE_BYTE_SIZE], xBytes)
	copy(pointBytes[FE_BYTE_SIZE:], yBytes)
	return g.FromBytes(pointBytes)
}

// ToBytes serializes a point into bytes in uncompressed form. Returns (0, 0) if point is infinity.
func (g *G1) ToBytes(p *PointG1) []byte {
	out := make([]byte, 2*FE_BYTE_SIZE)
	if g.IsZero(p) {
		return out
	}
	g.Affine(p)
	copy(out[:FE_BYTE_SIZE], toBytes(&p[0]))
	copy(out[FE_BYTE_SIZE:], toBytes(&p[1]))
	return out
}

// EncodePoint encodes a point into 128 bytes.
func (g *G1) EncodePoint(p *PointG1) []byte {
	outRaw := g.ToBytes(p)
	out := make([]byte, 2*ENCODED_FIELD_ELEMENT_SIZE)
	// encode x

	copy(out[ENCODED_FIELD_ELEMENT_SIZE-FE_BYTE_SIZE:], outRaw[:FE_BYTE_SIZE])
	// encode y
	copy(out[2*ENCODED_FIELD_ELEMENT_SIZE-FE_BYTE_SIZE:], outRaw[FE_BYTE_SIZE:])
	return out
}

// New creates a new G1 Point which is equal to zero in other words point at infinity.
func (g *G1) New() *PointG1 {
	return g.Zero()
}

// Zero returns a new G1 Point which is equal to point at infinity.
func (g *G1) Zero() *PointG1 {
	return new(PointG1).Zero()
}

// One returns a new G1 Point which is equal to generator point.
func (g *G1) One() *PointG1 {
	p := &PointG1{}
	return p.Set(&g1One)
}

// IsZero returns true if given point is equal to zero.
func (g *G1) IsZero(p *PointG1) bool {
	return p[2].isZero()
}

// Equal checks if given two G1 point is equal in their affine form.
func (g *G1) Equal(p1, p2 *PointG1) bool {
	if g.IsZero(p1) {
		return g.IsZero(p2)
	}
	if g.IsZero(p2) {
		return g.IsZero(p1)
	}
	t := g.t
	square(t[0], &p1[2])
	square(t[1], &p2[2])
	mul(t[2], t[0], &p2[0])
	mul(t[3], t[1], &p1[0])
	mul(t[0], t[0], &p1[2])
	mul(t[1], t[1], &p2[2])
	mul(t[1], t[1], &p1[1])
	mul(t[0], t[0], &p2[1])
	return t[0].equal(t[1]) && t[2].equal(t[3])
}

// InCorrectSubgroup checks whether given point is in correct subgroup.
func (g *G1) InCorrectSubgroup(p *PointG1) bool {
	tmp := &PointG1{}
	g.MulScalar(tmp, p, q)
	return g.IsZero(tmp)
}

// IsOnCurve checks a G1 point is on curve.
func (g *G1) IsOnCurve(p *PointG1) bool {
	if g.IsZero(p) {
		return true
	}
	t := g.t
	square(t[0], &p[1])
	square(t[1], &p[0])
	mul(t[1], t[1], &p[0])
	square(t[2], &p[2])
	square(t[3], t[2])
	mul(t[2], t[2], t[3])
	mul(t[2], b, t[2])
	add(t[1], t[1], t[2])
	return t[0].equal(t[1])
}

// IsAffine checks a G1 point whether it is in affine form.
func (g *G1) IsAffine(p *PointG1) bool {
	return p[2].isOne()
}

// Affine returns the affine representation of the given point
func (g *G1) Affine(p *PointG1) *PointG1 {
	if g.IsZero(p) {
		return p
	}
	if !g.IsAffine(p) {
		t := g.t
		inverse(t[0], &p[2])
		square(t[1], t[0])
		mul(&p[0], &p[0], t[1])
		mul(t[0], t[0], t[1])
		mul(&p[1], &p[1], t[0])
		p[2].one()
	}
	return p
}

// AffineBatch given multiple of points returns affine representations
func (g *G1) AffineBatch(p []*PointG1) {
	inverses := make([]fe, len(p))
	for i := 0; i < len(p); i++ {
		inverses[i].set(&p[i][2])
	}
	inverseBatch(inverses)
	t := g.t
	for i := 0; i < len(p); i++ {
		if !g.IsAffine(p[i]) && !g.IsZero(p[i]) {
			square(t[1], &inverses[i])
			mul(&p[i][0], &p[i][0], t[1])
			mul(t[0], &inverses[i], t[1])
			mul(&p[i][1], &p[i][1], t[0])
			p[i][2].one()
		}
	}
}

// Add adds two G1 points p1, p2 and assigns the result to point at first argument.
func (g *G1) Add(r, p1, p2 *PointG1) *PointG1 {
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
	square(t[7], &p1[2])
	mul(t[1], &p2[0], t[7])
	mul(t[2], &p1[2], t[7])
	mul(t[0], &p2[1], t[2])
	square(t[8], &p2[2])
	mul(t[3], &p1[0], t[8])
	mul(t[4], &p2[2], t[8])
	mul(t[2], &p1[1], t[4])
	if t[1].equal(t[3]) {
		if t[0].equal(t[2]) {
			return g.Double(r, p1)
		} else {
			return r.Zero()
		}
	}
	sub(t[1], t[1], t[3])
	double(t[4], t[1])
	square(t[4], t[4])
	mul(t[5], t[1], t[4])
	sub(t[0], t[0], t[2])
	double(t[0], t[0])
	square(t[6], t[0])
	sub(t[6], t[6], t[5])
	mul(t[3], t[3], t[4])
	double(t[4], t[3])
	sub(&r[0], t[6], t[4])
	sub(t[4], t[3], &r[0])
	mul(t[6], t[2], t[5])
	double(t[6], t[6])
	mul(t[0], t[0], t[4])
	sub(&r[1], t[0], t[6])
	add(t[0], &p1[2], &p2[2])
	square(t[0], t[0])
	sub(t[0], t[0], t[7])
	sub(t[0], t[0], t[8])
	mul(&r[2], t[0], t[1])
	return r
}

// Add adds two G1 points p1, p2 and assigns the result to point at first argument.
// Expects point p2 in affine form.
func (g *G1) addMixed(r, p1, p2 *PointG1) *PointG1 {
	// http://www.hyperelliptic.org/EFD/g1p/auto-shortw-jacobian-0.html#addition-madd-2007-bl
	if g.IsZero(p1) {
		return r.Set(p2)
	}
	if g.IsZero(p2) {
		return r.Set(p1)
	}
	t := g.t
	square(t[7], &p1[2])    // z1z1
	mul(t[1], &p2[0], t[7]) // u2 = x2 * z1z1
	mul(t[2], &p1[2], t[7]) // z1z1 * z1
	mul(t[0], &p2[1], t[2]) // s2 = y2 * z1z1 * z1

	if p1[0].equal(t[1]) && p1[1].equal(t[0]) {
		return g.Double(r, p1)
	}

	sub(t[1], t[1], &p1[0]) // h = u2 - x1
	square(t[2], t[1])      // hh
	double(t[4], t[2])
	doubleAssign(t[4])      // 4hh
	mul(t[5], t[1], t[4])   // j = h*i
	subAssign(t[0], &p1[1]) // s2 - y1
	doubleAssign(t[0])      // r = 2*(s2 - y1)
	square(t[6], t[0])      // r^2
	subAssign(t[6], t[5])   // r^2 - j
	mul(t[3], &p1[0], t[4]) // v = x1 * i
	double(t[4], t[3])      // 2*v
	sub(&r[0], t[6], t[4])  // x3 = r^2 - j - 2*v
	sub(t[4], t[3], &r[0])  // v - x3
	mul(t[6], &p1[1], t[5]) // y1 * j
	doubleAssign(t[6])      // 2 * y1 * j
	mul(t[0], t[0], t[4])   // r * (v - x3)
	sub(&r[1], t[0], t[6])  // y3 = r * (v - x3) - (2 * y1 * j)
	add(t[0], &p1[2], t[1]) // z1 + h
	square(t[0], t[0])      // (z1 + h)^2
	subAssign(t[0], t[7])   // (z1 + h)^2 - z1z1
	sub(&r[2], t[0], t[2])  // z3 = (z1 + z2)^2 - z1z1 - hh
	return r
}

// Double doubles a G1 point p and assigns the result to the point at first argument.
func (g *G1) Double(r, p *PointG1) *PointG1 {
	// http://www.hyperelliptic.org/EFD/gp/auto-shortw-jacobian-0.html#doubling-dbl-2009-l
	if g.IsZero(p) {
		return r.Set(p)
	}
	t := g.t
	square(t[0], &p[0])
	square(t[1], &p[1])
	square(t[2], t[1])
	add(t[1], &p[0], t[1])
	square(t[1], t[1])
	sub(t[1], t[1], t[0])
	sub(t[1], t[1], t[2])
	double(t[1], t[1])
	double(t[3], t[0])
	add(t[0], t[3], t[0])
	square(t[4], t[0])
	double(t[3], t[1])
	sub(&r[0], t[4], t[3])
	sub(t[1], t[1], &r[0])
	double(t[2], t[2])
	double(t[2], t[2])
	double(t[2], t[2])
	mul(t[0], t[0], t[1])
	sub(t[1], t[0], t[2])
	mul(t[0], &p[1], &p[2])
	r[1].set(t[1])
	double(&r[2], t[0])
	return r
}

// Neg negates a G1 point p and assigns the result to the point at first argument.
func (g *G1) Neg(r, p *PointG1) *PointG1 {
	r[0].set(&p[0])
	r[2].set(&p[2])
	neg(&r[1], &p[1])
	return r
}

// Sub subtracts two G1 points p1, p2 and assigns the result to point at first argument.
func (g *G1) Sub(c, a, b *PointG1) *PointG1 {
	d := &PointG1{}
	g.Neg(d, b)
	g.Add(c, a, d)
	return c
}

// MulScalar multiplies a point by given scalar value in big.Int and assigns the result to point at first argument.
func (g *G1) MulScalar(c, p *PointG1, e *big.Int) *PointG1 {
	q, n := &PointG1{}, &PointG1{}
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

// ClearCofactor maps given a G1 point to correct subgroup
func (g *G1) ClearCofactor(p *PointG1) {
	g.MulScalar(p, p, cofactorG1)
}

// MultiExp calculates multi exponentiation. Given pairs of G1 point and scalar values
// (P_0, e_0), (P_1, e_1), ... (P_n, e_n) calculates r = e_0 * P_0 + e_1 * P_1 + ... + e_n * P_n
// Length of points and scalars are expected to be equal, otherwise an error is returned.
// Result is assigned to point at first argument.
func (g *G1) MultiExp(r *PointG1, points []*PointG1, scalars []*big.Int) (*PointG1, error) {
	if len(points) != len(scalars) {
		return nil, errors.New("point and scalar vectors should be in same length")
	}

	g.AffineBatch(points)

	c := 3
	if len(scalars) >= 32 {
		c = int(math.Ceil(math.Log(float64(len(scalars)))))
	}

	bucketSize := (1 << c) - 1
	windows := make([]*PointG1, SCALAR_FIELD_BIT_SIZE/c+1)
	bucket := make([]PointG1, bucketSize)

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
