package bw6

type pair struct {
	g1 *Point
	g2 *Point
}

func newPair(g1 *Point, g2 *Point) pair {
	return pair{g1, g2}
}

type Engine struct {
	g   *G
	fp6 *fp6
	fp3 *fp3
	pairingEngineTemp
	pairs []pair
}

// NewEngine creates new pairing engine insteace.
func NewEngine() *Engine {
	fp3 := newFp3()
	fp6 := newFp6(fp3)
	return &Engine{
		fp6:               fp6,
		fp3:               fp3,
		g:                 NewG(),
		pairingEngineTemp: newEngineTemp(),
	}
}

type pairingEngineTemp struct {
	t  [10]*fe
	t6 [23]fe6
}

func newEngineTemp() pairingEngineTemp {
	t := [10]*fe{}
	for i := 0; i < 10; i++ {
		t[i] = &fe{}
	}
	t6 := [23]fe6{}
	return pairingEngineTemp{t, t6}
}

func (e *Engine) doublingStep(coeff *[3]fe, r *Point) {

	t := e.t

	mul(t[0], &r[0], &r[1])
	square(t[1], &r[1])
	double(t[2], t[1])
	doubleAssign(t[2])
	square(t[3], &r[2])
	double(t[4], t[3])
	addAssign(t[4], t[3])
	mul(t[5], b2, t[4])
	sub(&coeff[0], t[5], t[1])
	double(t[6], t[5])
	addAssign(t[6], t[5])
	add(t[7], t[1], t[6])
	add(t[8], &r[1], &r[2])
	square(t[8], t[8])
	addAssign(t[3], t[1])
	subAssign(t[1], t[6])
	subAssign(t[8], t[3])
	square(t[4], &r[0])
	doubleAssign(t[5])
	square(t[5], t[5])
	double(&r[0], t[0])
	mul(&r[0], &r[0], t[1])
	square(&r[1], t[7])
	double(t[1], t[5])
	addAssign(t[1], t[5])
	sub(&r[1], &r[1], t[1])
	mul(&r[2], t[2], t[8])
	double(t[1], t[4])
	add(&coeff[1], t[1], t[4])
	neg(&coeff[2], t[8])
}

func (e *Engine) additionStep(coeff *[3]fe, r, q *Point) {

	t := e.t

	mul(t[0], &q[1], &r[2])
	sub(t[1], &r[1], t[0])
	mul(t[0], &q[0], &r[2])
	sub(t[2], &r[0], t[0])
	square(t[3], t[1])
	square(t[4], t[2])
	mul(t[5], t[2], t[4])
	mul(t[3], &r[2], t[3])
	mul(t[4], &r[0], t[4])
	double(t[0], t[4])
	addAssign(t[3], t[5])
	subAssign(t[3], t[0])
	mul(&r[0], t[2], t[3])
	mul(t[0], t[5], &r[1])
	sub(&r[1], t[4], t[3])
	mul(&r[1], &r[1], t[1])
	subAssign(&r[1], t[0])
	mul(&r[2], &r[2], t[5])
	mul(t[0], t[2], &q[1])
	mul(t[3], t[1], &q[0])
	sub(&coeff[0], t[3], t[0])
	neg(&coeff[1], t[1])
	coeff[2].set(t[2])
}

func (e *Engine) ell(f *fe6, coeffs *[3]fe, p *Point) {
	c0, c1, c2 := new(fe).set(&coeffs[0]), new(fe), new(fe)
	mul(c1, &coeffs[1], &p[0])
	mul(c2, &coeffs[2], &p[1])
	e.fp6.mulBy014Assign(f, c0, c1, c2)
}

func (e *Engine) preCompute(ellCoeffs *[288][3]fe, twistPoint *Point) {
	if e.g.IsZero(twistPoint) {
		return
	}
	r := new(Point).Set(twistPoint)
	j := 0

	for i := ateLoop1.BitLen() - 2; i >= 0; i-- {
		e.doublingStep(&ellCoeffs[j], r)
		j++
		if ateLoop1.Bit(i) != 0 {
			ellCoeffs[j] = fe3{}
			e.additionStep(&ellCoeffs[j], r, twistPoint)
			j++
		}
	}

	r.Set(twistPoint)
	negTwist := e.g.Neg(e.g.New(), twistPoint)
	for i := 188; i >= 0; i-- {
		e.doublingStep(&ellCoeffs[j], r)
		j++
		switch ateLoop2NAF[i] {
		case 1:
			e.additionStep(&ellCoeffs[j], r, twistPoint)
			j++
		case -1:
			e.additionStep(&ellCoeffs[j], r, negTwist)
			j++
		}
	}

}

func (e *Engine) millerLoop(f *fe6) {

	ellCoeffs := make([][288][3]fe, len(e.pairs))
	for i := 0; i < len(e.pairs); i++ {
		e.preCompute(&ellCoeffs[i], e.pairs[i].g2)
	}

	f1, f2 := e.fp6.one(), e.fp6.one()

	j := 0
	for i := ateLoop1.BitLen() - 2; i >= 0; i-- {
		e.fp6.square(f1, f1)
		for k := 0; k < len(e.pairs); k++ {
			e.ell(f1, &ellCoeffs[k][j], e.pairs[k].g1)
		}
		j++
		if ateLoop1.Bit(i) != 0 {
			for k := 0; k < len(e.pairs); k++ {
				e.ell(f1, &ellCoeffs[k][j], e.pairs[k].g1)
			}
			j++
		}
	}

	for i := 188; i >= 0; i-- {
		if i != 188 {
			e.fp6.square(f2, f2)
		}
		for k := 0; k < len(e.pairs); k++ {
			e.ell(f2, &ellCoeffs[k][j], e.pairs[k].g1)
		}
		j++
		if ateLoop2NAF[i] != 0 {
			for k := 0; k < len(e.pairs); k++ {
				e.ell(f2, &ellCoeffs[k][j], e.pairs[k].g1)
			}
			j++
		}
	}

	e.fp6.frobeniusMap(f2, f2, 1)
	e.fp6.mul(f, f1, f2)
}

func (e *Engine) exp(c, a *fe6) {
	fp6 := e.fp6
	t0, t1, t2 := new(fe6).set(a), new(fe6), new(fe6)
	fp6.cyclotomicSquaring(c, t0)
	fp6.mul(t1, t0, c)
	for i := 0; i < 4; i++ {
		fp6.cyclotomicSquaring(c, c)
	}
	fp6.mul(t2, c, t0)
	fp6.cyclotomicSquaring(c, t2)
	for i := 0; i < 6; i++ {
		fp6.cyclotomicSquaring(c, c)
	}
	fp6.mul(c, c, t2)
	for i := 0; i < 5; i++ {
		fp6.cyclotomicSquaring(c, c)
	}
	fp6.mul(c, c, t1)
	for i := 0; i < 46; i++ {
		fp6.cyclotomicSquaring(c, c)
	}
	fp6.mul(c, c, t0)
}

// (q^k-1)/r where k = 6
func (e *Engine) finalExp(f *fe6) {
	// (q^6-1)/r
	fp6, t := e.fp6, e.t6
	fp6.inverse(&t[0], f)

	// easy part f^(q^3-1)*(q+1)
	//  f1 = f^(q^3)*f^(-1)
	//  f2 = f^q * f1
	fp6.conjugate(&t[1], f)
	fp6.mul(&t[1], &t[1], &t[0])
	fp6.frobeniusMap(&t[2], &t[1], 1)
	fp6.mul(&t[2], &t[2], &t[1])

	// hard part (q^2-q+1)/r
	// R_0(x) * q*R_1(x)
	// where
	// R_0(x) = (-103*x^7 + 70*x^6 + 269*x^5 - 197*x^4 - 314*x^3 - 73*x^2 - 263*x - 220)
	// R_1(x) = (103*x^9 - 276*x^8 + 77*x^7 + 492*x^6 - 445*x^5 - 65*x^4 + 452*x^3 - 181*x^2 + 34*x + 229)
	fp6.frobeniusMap(&t[5], &t[2], 1)

	e.exp(&t[7], &t[2])
	fp6.frobeniusMap(&t[6], &t[7], 1)

	e.exp(&t[9], &t[7])
	fp6.frobeniusMap(&t[8], &t[9], 1)

	e.exp(&t[11], &t[9])
	fp6.frobeniusMap(&t[10], &t[11], 1)

	e.exp(&t[13], &t[11])
	fp6.frobeniusMap(&t[12], &t[13], 1)

	e.exp(&t[14], &t[13])
	fp6.frobeniusMap(&t[15], &t[14], 1)

	e.exp(&t[17], &t[14])
	fp6.frobeniusMap(&t[16], &t[17], 1)

	e.exp(&t[19], &t[17])
	fp6.frobeniusMap(&t[18], &t[19], 1)

	e.exp(&t[20], &t[18])
	e.exp(&t[21], &t[20])

	fp6.conjugate(&t[22], &t[15])
	fp6.mul(&t[3], &t[10], &t[16])
	fp6.mul(&t[3], &t[3], &t[22])

	fp6.mul(&t[13], &t[13], &t[8])
	fp6.mul(&t[1], &t[2], &t[7])
	fp6.mul(&t[1], &t[1], &t[11])
	fp6.mul(&t[1], &t[1], &t[13])
	fp6.mul(&t[1], &t[1], &t[20])
	fp6.conjugate(&t[1], &t[1])
	fp6.square(&t[4], &t[3])
	fp6.mul(&t[4], &t[4], &t[14])
	fp6.mul(&t[4], &t[4], &t[5])
	fp6.mul(&t[4], &t[4], &t[1])

	fp6.conjugate(&t[1], &t[19])
	fp6.square(&t[3], &t[4])
	fp6.mul(&t[3], &t[3], &t[21])
	fp6.mul(&t[3], &t[3], &t[1])

	fp6.mul(&t[12], &t[9], &t[12])
	fp6.mul(&t[22], &t[13], &t[15])
	fp6.mul(&t[1], &t[12], &t[11])
	fp6.mul(&t[1], &t[1], &t[10])
	fp6.conjugate(&t[1], &t[1])
	fp6.square(&t[4], &t[3])
	fp6.mul(&t[4], &t[4], &t[22])
	fp6.mul(&t[4], &t[4], &t[17])
	fp6.mul(&t[4], &t[4], &t[18])
	fp6.mul(&t[4], &t[4], &t[1])

	fp6.mul(&t[1], &t[5], &t[21])
	fp6.conjugate(&t[1], &t[1])
	fp6.square(&t[3], &t[4])
	fp6.mul(&t[3], &t[3], &t[2])
	fp6.mul(&t[3], &t[3], &t[19])
	fp6.mul(&t[3], &t[3], &t[6])
	fp6.mul(&t[3], &t[3], &t[1])

	fp6.mul(&t[16], &t[16], &t[20])
	fp6.square(&t[4], &t[3])
	fp6.mul(&t[4], &t[4], &t[8])
	fp6.mul(&t[8], &t[14], &t[18])
	fp6.conjugate(&t[1], &t[16])
	fp6.mul(&t[4], &t[4], &t[8])
	fp6.mul(&t[4], &t[4], &t[1])

	fp6.mul(&t[11], &t[11], &t[17])
	fp6.mul(&t[17], &t[7], &t[19])
	fp6.mul(&t[1], &t[17], &t[9])
	fp6.conjugate(&t[1], &t[1])
	fp6.square(&t[3], &t[4])
	fp6.mul(&t[3], &t[3], &t[11])
	fp6.mul(&t[3], &t[3], &t[21])
	fp6.mul(&t[3], &t[3], &t[1])

	fp6.mul(&t[1], &t[13], &t[8])
	fp6.mul(&t[1], &t[1], &t[16])
	fp6.conjugate(&t[1], &t[1])
	fp6.square(&t[4], &t[3])
	fp6.mul(&t[4], &t[4], &t[2])
	fp6.mul(&t[4], &t[4], &t[5])
	fp6.mul(&t[4], &t[4], &t[10])
	fp6.mul(&t[4], &t[4], &t[15])
	fp6.mul(&t[4], &t[4], &t[1])

	fp6.conjugate(&t[1], &t[11])
	fp6.square(&t[3], &t[4])
	fp6.mul(&t[3], &t[3], &t[6])
	fp6.mul(&t[3], &t[3], &t[1])

	fp6.mul(&t[1], &t[12], &t[22])
	fp6.mul(&t[1], &t[1], &t[21])
	fp6.conjugate(&t[1], &t[1])
	fp6.square(&t[4], &t[3])
	fp6.mul(&t[4], &t[4], &t[17])
	fp6.mul(&t[4], &t[4], &t[8])
	fp6.mul(&t[4], &t[4], &t[5])
	fp6.mul(f, &t[4], &t[1])

}

func (e *Engine) affine(p pair) {
	e.g.Affine(p.g1)
	e.g.Affine(p.g2)
}

func (e *Engine) calculate() *fe6 {
	f := e.fp6.one()
	if len(e.pairs) == 0 {
		return f
	}
	e.millerLoop(f)
	e.finalExp(f)
	return f
}

// AddPair adds a g1, g2 point pair to pairing engine
func (e *Engine) AddPair(g1 *Point, g2 *Point) *Engine {
	p := newPair(g1, g2)
	if !e.isZero(p) {
		e.affine(p)
		e.pairs = append(e.pairs, p)
	}
	return e
}

// AddPairInv adds a G1, G2 point pair to pairing engine. G1 point is negated.
func (e *Engine) AddPairInv(g1 *Point, g2 *Point) *Engine {
	ng1 := e.g.New().Set(g1)
	e.g.Neg(ng1, g1)
	e.AddPair(ng1, g2)
	return e
}

// Reset deletes added pairs.
func (e *Engine) Reset() *Engine {
	e.pairs = []pair{}
	return e
}

func (e *Engine) isZero(p pair) bool {
	return e.g.IsZero(p.g1) || e.g.IsZero(p.g2)
}

// Result computes pairing and returns target group element as result.
func (e *Engine) Result() *E {
	r := e.calculate()
	e.Reset()
	return r
}

// GT returns target group instance.
func (e *Engine) GT() *GT {
	return NewGT()
}

// Check computes pairing and checks if result is equal to one
func (e *Engine) Check() bool {
	return e.calculate().isOne()
}
