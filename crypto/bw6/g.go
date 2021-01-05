package bw6

import (
	"errors"
	"math"
	"math/big"
)

// Point is type for point in G and used for both affine and Jacobian representation.
// A point is accounted as in affine form if z is equal to one.
type Point [3]fe

var wnafMulWindow uint = 6
var glvMulWindow uint = 4

// Set sets the point p2 to p
func (p *Point) Set(p2 *Point) *Point {
	p[0].set(&p2[0])
	p[1].set(&p2[1])
	p[2].set(&p2[2])
	return p
}

// Zero sets point p as point at infinity
func (p *Point) Zero() *Point {
	p[0].zero()
	p[1].one()
	p[2].zero()
	return p
}

// IsAffine checks a G point whether it is in affine form.
func (p *Point) IsAffine() bool {
	return p[2].isOne()
}

type tempG struct {
	t [9]*fe
}

// G is struct for group.
type G struct {
	tempG
}

// NewG constructs a new G instance.
func NewG() *G {
	t := newTempG()
	return &G{t}
}

func newTempG() tempG {
	t := [9]*fe{}
	for i := 0; i < 9; i++ {
		t[i] = &fe{}
	}
	return tempG{t}
}

// Q returns group order in big.Int.
func (g *G) Q() *big.Int {
	return new(big.Int).Set(q)
}

// G2FromBytes constructs a new point given uncompressed byte input.
// Input string is expected to be equal to 192 bytes and concatenation of x and y cooridanates.
// (0, 0) is considered as infinity.
func (g *G) G1FromBytes(in []byte) (*Point, error) {
	p, err := g.fromBytes(in)
	if err != nil {
		return nil, err
	}
	if !g.IsOnG1Curve(p) {
		return nil, errors.New("point is not on curve")
	}
	return p, nil
}

// G2FromBytes constructs a new point given uncompressed byte input.
// Input string is expected to be equal to 192 bytes and concatenation of x and y cooridanates.
// (0, 0) is considered as infinity.
func (g *G) G2FromBytes(in []byte) (*Point, error) {
	p, err := g.fromBytes(in)
	if err != nil {
		return nil, err
	}
	if !g.IsOnG2Curve(p) {
		return nil, errors.New("point is not on curve")
	}
	return p, nil
}

func (g *G) fromBytes(in []byte) (*Point, error) {
	if len(in) != 2*fpByteSize {
		return nil, errors.New("input string length must be equal to 192 bytes")
	}
	x, err := fromBytes(in[:fpByteSize])
	if err != nil {
		return nil, err
	}
	y, err := fromBytes(in[fpByteSize:])
	if err != nil {
		return nil, err
	}
	// check if given input points to infinity
	if x.isZero() && y.isZero() {
		return g.Zero(), nil
	}
	z := new(fe).one()
	return &Point{*x, *y, *z}, nil
}

// ToBytes serializes a point into bytes in uncompressed form.
// It returns (0, 0) if point is infinity.
func (g *G) ToBytes(p *Point) []byte {
	out := make([]byte, 2*fpByteSize)
	if g.IsZero(p) {
		return out
	}
	g.Affine(p)
	copy(out[:fpByteSize], toBytes(&p[0]))
	copy(out[fpByteSize:], toBytes(&p[1]))
	return out
}

// New creates a new G Point which is equal to zero in other words point at infinity.
func (g *G) New() *Point {
	return g.Zero()
}

// Zero returns a new G Point which is equal to point at infinity.
func (g *G) Zero() *Point {
	return new(Point).Zero()
}

// G1One returns a new G1 generator.
func (g *G) G1One() *Point {
	p := &Point{}
	return p.Set(&g1One)
}

// G1One returns a new G1 generator.
func (g *G) G2One() *Point {
	p := &Point{}
	return p.Set(&g2One)
}

// IsZero returns true if given point is equal to zero.
func (g *G) IsZero(p *Point) bool {
	return p[2].isZero()
}

// Equal checks if given two G point is equal in their affine form.
func (g *G) Equal(p1, p2 *Point) bool {
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

// IsOnG1Curve checks if G1 point is on curve.
func (g *G) IsOnG1Curve(p *Point) bool {
	if g.IsZero(p) {
		return true
	}
	t := g.t
	square(t[0], &p[1])    // y^2
	square(t[1], &p[0])    // x^2
	mul(t[1], t[1], &p[0]) // x^3
	if p.IsAffine() {
		addAssign(t[1], b)      // x^2 + b
		return t[0].equal(t[1]) // y^2 ?= x^3 + b
	}
	square(t[2], &p[2])     // z^2
	square(t[3], t[2])      // z^4
	mul(t[2], t[2], t[3])   // -b*z^6
	subAssign(t[1], t[2])   // x^3 + b * z^6
	return t[0].equal(t[1]) // y^2 ?= x^3 + b * z^6
}

// IsOnG1Curve checks if G1 point is on curve.
func (g *G) IsOnG2Curve(p *Point) bool {
	if g.IsZero(p) {
		return true
	}
	t := g.t
	square(t[0], &p[1])    // y^2
	square(t[1], &p[0])    // x^2
	mul(t[1], t[1], &p[0]) // x^3
	if p.IsAffine() {
		addAssign(t[1], b2)     // x^2 + b
		return t[0].equal(t[1]) // y^2 ?= x^3 + b
	}
	square(t[2], &p[2])   // z^2
	square(t[3], t[2])    // z^4
	mul(t[2], t[2], t[3]) // z^6

	doubleAssign(t[2])
	doubleAssign(t[2]) // b * z^6

	addAssign(t[1], t[2])   // x^3 + b * z^6
	return t[0].equal(t[1]) // y^2 ?= x^3 + b * z^6
}

// IsAffine checks a G point whether it is in affine form.
func (g *G) IsAffine(p *Point) bool {
	return p[2].isOne()
}

// Affine returns the affine representation of the given point
func (g *G) Affine(p *Point) *Point {
	return g.affine(p, p)
}

func (g *G) affine(r, p *Point) *Point {
	if g.IsZero(p) {
		return r.Zero()
	}
	if !g.IsAffine(p) {
		t := g.t
		inverse(t[0], &p[2])    // z^-1
		square(t[1], t[0])      // z^-2
		mul(&r[0], &p[0], t[1]) // x = x * z^-2
		mul(t[0], t[0], t[1])   // z^-3
		mul(&r[1], &p[1], t[0]) // y = y * z^-3
		r[2].one()              // z = 1
	} else {
		r.Set(p)
	}
	return r
}

// AffineBatch given multiple of points returns affine representations
func (g *G) AffineBatch(p []*Point) {
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

// Add adds two G points p1, p2 and assigns the result to point at first argument.
func (g *G) Add(r, p1, p2 *Point) *Point {
	// http://www.hyperelliptic.org/EFD/gp/auto-shortw-jacobian-0.html#addition-add-2007-bl
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
	square(t[8], &p2[2])    // z2z2
	mul(t[3], &p1[0], t[8]) // u1 = x1 * z2z2
	mul(t[4], &p2[2], t[8]) // z2z2 * z2
	mul(t[2], &p1[1], t[4]) // s1 = y1 * z2z2 * z2
	if t[1].equal(t[3]) {
		if t[0].equal(t[2]) {
			return g.Double(r, p1)
		} else {
			return r.Zero()
		}
	}
	subAssign(t[1], t[3])      // h = u2 - u1
	ldouble(t[4], t[1])        // 2h
	square(t[4], t[4])         // i = 2h^2
	mul(t[5], t[1], t[4])      // j = h*i
	subAssign(t[0], t[2])      // s2 - s1
	ldoubleAssign(t[0])        // r = 2*(s2 - s1)
	square(t[6], t[0])         // r^2
	subAssign(t[6], t[5])      // r^2 - j
	mul(t[3], t[3], t[4])      // v = u1 * i
	double(t[4], t[3])         // 2*v
	sub(&r[0], t[6], t[4])     // x3 = r^2 - j - 2*v
	sub(t[4], t[3], &r[0])     // v - x3
	mul(t[6], t[2], t[5])      // s1 * j
	doubleAssign(t[6])         // 2 * s1 * j
	mul(t[0], t[0], t[4])      // r * (v - x3)
	sub(&r[1], t[0], t[6])     // y3 = r * (v - x3) - (2 * s1 * j)
	ladd(t[0], &p1[2], &p2[2]) // z1 + z2
	square(t[0], t[0])         // (z1 + z2)^2
	subAssign(t[0], t[7])      // (z1 + z2)^2 - z1z1
	subAssign(t[0], t[8])      // (z1 + z2)^2 - z1z1 - z2z2
	mul(&r[2], t[0], t[1])     // z3 = ((z1 + z2)^2 - z1z1 - z2z2) * h
	return r
}

// Add adds two G points p1, p2 and assigns the result to point at first argument.
// Expects the second point p2 in affine form.
func (g *G) AddMixed(r, p1, p2 *Point) *Point {
	// http://www.hyperelliptic.org/EFD/Gp/auto-shortw-jacobian-0.html#addition-madd-2007-bl
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

// Double doubles a G point p and assigns the result to the point at first argument.
func (g *G) Double(r, p *Point) *Point {
	// http://www.hyperelliptic.org/EFD/gp/auto-shortw-jacobian-0.html#doubling-dbl-2009-l
	if g.IsZero(p) {
		return r.Set(p)
	}
	t := g.t
	square(t[0], &p[0])     // a = x^2
	square(t[1], &p[1])     // b = y^2
	square(t[2], t[1])      // c = b^2
	laddAssign(t[1], &p[0]) // b + x1
	square(t[1], t[1])      // (b + x1)^2
	subAssign(t[1], t[0])   // (b + x1)^2 - a
	subAssign(t[1], t[2])   // (b + x1)^2 - a - c
	doubleAssign(t[1])      // d = 2((b+x1)^2 - a - c)
	ldouble(t[3], t[0])     // 2a
	laddAssign(t[0], t[3])  // e = 3a
	square(t[4], t[0])      // f = e^2
	double(t[3], t[1])      // 2d
	sub(&r[0], t[4], t[3])  // x3 = f - 2d
	sub(t[1], t[1], &r[0])  // d-x3
	doubleAssign(t[2])      //
	doubleAssign(t[2])      //
	doubleAssign(t[2])      // 8c
	mul(t[0], t[0], t[1])   // e * (d - x3)
	sub(t[1], t[0], t[2])   // x3 = e * (d - x3) - 8c
	mul(t[0], &p[1], &p[2]) // y1 * z1
	r[1].set(t[1])          //
	double(&r[2], t[0])     // z3 = 2(y1 * z1)
	return r
}

// Sub subtracts two G points p1, p2 and assigns the result to point at first argument.
func (g *G) Sub(c, a, b *Point) *Point {
	d := &Point{}
	g.Neg(d, b)
	g.Add(c, a, d)
	return c
}

// Neg negates a G point p and assigns the result to the point at first argument.
func (g *G) Neg(r, p *Point) *Point {
	r[0].set(&p[0])
	r[2].set(&p[2])
	neg(&r[1], &p[1])
	return r
}

// MulScalar multiplies a G1 point by given scalar value in big.Int and assigns the result to point at first argument.
func (g *G) MulScalarG1(r, p *Point, e *big.Int) *Point {
	return g.glvMulG1(r, p, e)
}

// MulScalarG2 multiplies a G2 point by given scalar value in big.Int and assigns the result to point at first argument.
func (g *G) MulScalarG2(r, p *Point, e *big.Int) *Point {
	return g.glvMulG2(r, p, e)
}

func (g *G) mulScalar(r, p *Point, e *big.Int) *Point {
	q, n := &Point{}, &Point{}
	n.Set(p)
	l := e.BitLen()
	for i := 0; i < l; i++ {
		if e.Bit(i) == 1 {
			g.Add(q, q, n)
		}
		g.Double(n, n)
	}
	return r.Set(q)
}

func (g *G) wnafMul(r, p *Point, e *big.Int) *Point {
	wnaf := bigToWNAF(e, wnafMulWindow)
	return g._wnafMul(r, p, wnaf)
}

func (g *G) _wnafMul(r, p *Point, wnaf nafNumber) *Point {

	l := (1 << (wnafMulWindow - 1))

	twoP, acc := g.New(), new(Point).Set(p)
	g.Double(twoP, p)
	g.Affine(twoP)

	// table = {p, 3p, 5p, ..., -p, -3p, -5p}
	table := make([]*Point, l*2)
	table[0], table[l] = g.New(), g.New()
	table[0].Set(p)
	g.Neg(table[l], table[0])

	for i := 1; i < l; i++ {
		g.AddMixed(acc, acc, twoP)
		table[i], table[i+l] = g.New(), g.New()
		table[i].Set(acc)
		g.Neg(table[i+l], table[i])
	}

	q := g.Zero()
	for i := len(wnaf) - 1; i >= 0; i-- {
		if wnaf[i] > 0 {
			g.Add(q, q, table[wnaf[i]>>1])
		} else if wnaf[i] < 0 {
			g.Add(q, q, table[((-wnaf[i])>>1)+l])
		}
		if i != 0 {
			g.Double(q, q)
		}
	}
	return r.Set(q)
}

func (g *G) glvMulG1(r, p *Point, e *big.Int) *Point {
	return g.glvMul(r, p, new(glvVector).new(e), g.glvEndomorphismG1)
}

func (g *G) glvMulG2(r, p *Point, e *big.Int) *Point {
	return g.glvMul(r, p, new(glvVector).new(e), g.glvEndomorphismG2)
}

func (g *G) glvMul(r, p0 *Point, v *glvVector, endoFn func(r, p *Point)) *Point {

	w := glvMulWindow
	l := 1 << (w - 1)

	// prepare tables
	// tableK1 = {P, 3P, 5P, ...}
	// tableK2 = {λP, 3λP, 5λP, ...}
	tableK1, tableK2 := make([]*Point, l), make([]*Point, l)
	double := g.New()
	g.Double(double, p0)
	g.affine(double, double)
	tableK1[0] = new(Point)
	tableK1[0].Set(p0)
	for i := 1; i < l; i++ {
		tableK1[i] = new(Point)
		g.AddMixed(tableK1[i], tableK1[i-1], double)
	}
	g.AffineBatch(tableK1)
	for i := 0; i < l; i++ {
		tableK2[i] = new(Point)
		endoFn(tableK2[i], tableK1[i])
	}

	// recode small scalars
	naf1, naf2 := v.wnaf(w)
	lenNAF1, lenNAF2 := len(naf1), len(naf2)
	lenNAF := lenNAF1
	if lenNAF2 > lenNAF {
		lenNAF = lenNAF2
	}

	acc, p1 := g.New(), g.New()

	// function for naf addition
	add := func(table []*Point, naf int) {
		if naf != 0 {
			nafAbs := naf
			if nafAbs < 0 {
				nafAbs = -nafAbs
			}
			p1.Set(table[nafAbs>>1])
			if naf < 0 {
				g.Neg(p1, p1)
			}
			g.AddMixed(acc, acc, p1)
		}
	}

	// sliding
	for i := lenNAF - 1; i >= 0; i-- {
		if i < lenNAF1 {
			add(tableK1, naf1[i])
		}
		if i < lenNAF2 {
			add(tableK2, naf2[i])
		}
		if i != 0 {
			g.Double(acc, acc)
		}
	}
	return r.Set(acc)
}

// MultiExp calculates multi exponentiation. Given pairs of G point and scalar values
// (P_0, e_0), (P_1, e_1), ... (P_n, e_n) calculates r = e_0 * P_0 + e_1 * P_1 + ... + e_n * P_n
// Length of points and scalars are expected to be equal, otherwise an error is returned.
// Result is assigned to point at first argument.
func (g *G) MultiExp(r *Point, points []*Point, scalars []*big.Int) (*Point, error) {
	if len(points) != len(scalars) {
		return nil, errors.New("point and scalar vectors should be in same length")
	}

	c := 3
	if len(scalars) >= 32 {
		c = int(math.Ceil(math.Log(float64(len(scalars)))))
	}

	bucketSize := (1 << c) - 1
	windows := make([]Point, frBitSize/c+1)
	bucket := make([]Point, bucketSize)
	for j := 0; j < len(windows); j++ {

		for i := 0; i < bucketSize; i++ {
			bucket[i].Zero()
		}

		for i := 0; i < len(scalars); i++ {
			index := bucketSize & int(new(big.Int).Rsh(scalars[i], uint(c*j)).Int64())
			if index != 0 {
				g.Add(&bucket[index-1], &bucket[index-1], points[i])
			}
		}

		acc, sum := g.New(), g.New()
		for i := bucketSize - 1; i >= 0; i-- {
			g.Add(sum, sum, &bucket[i])
			g.Add(acc, acc, sum)
		}
		windows[j].Set(acc)
	}

	acc := g.New()
	for i := len(windows) - 1; i >= 0; i-- {
		for j := 0; j < c; j++ {
			g.Double(acc, acc)
		}
		g.Add(acc, acc, &windows[i])
	}
	return r.Set(acc), nil
}

// ClearG1Cofactor maps given a G1 point to correct subgroup
func (g *G) ClearG1Cofactor(p *Point) *Point {
	return g.wnafMul(p, p, cofactorG1)
}

// ClearG2Cofactor maps given a G1 point to correct subgroup
func (g *G) ClearG2Cofactor(p *Point) *Point {
	return g.wnafMul(p, p, cofactorG2)
}

// InCorrectSubgroup checks whether given point is in correct subgroup.
func (g *G) InCorrectSubgroup(p *Point) bool {
	tmp := &Point{}
	g.wnafMul(tmp, p, q)
	return g.IsZero(tmp)
}
