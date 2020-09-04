package bls12377

import (
	"errors"
	"math/big"
)

type fp6Temp struct {
	t [6]*fe2
}

type fp6 struct {
	fp2 *fp2
	fp6Temp
}

func newFp6Temp() fp6Temp {
	t := [6]*fe2{}
	for i := 0; i < len(t); i++ {
		t[i] = &fe2{}
	}
	return fp6Temp{t}
}

func newFp6(f *fp2) *fp6 {
	t := newFp6Temp()
	if f == nil {
		return &fp6{newFp2(), t}
	}
	return &fp6{f, t}
}

func (e *fp6) fromBytes(b []byte) (*fe6, error) {
	if len(b) != 288 {
		return nil, errors.New("input string should be larger than 288 bytes")
	}
	fp2 := e.fp2
	u0, err := fp2.fromBytes(b[:96])
	if err != nil {
		return nil, err
	}
	u1, err := fp2.fromBytes(b[96:192])
	if err != nil {
		return nil, err
	}
	u2, err := fp2.fromBytes(b[192:])
	if err != nil {
		return nil, err
	}
	return &fe6{*u0, *u1, *u2}, nil
}

func (e *fp6) toBytes(a *fe6) []byte {
	fp2 := e.fp2
	out := make([]byte, 288)
	copy(out[:96], fp2.toBytes(&a[0]))
	copy(out[96:192], fp2.toBytes(&a[1]))
	copy(out[192:], fp2.toBytes(&a[2]))
	return out
}

func (e *fp6) new() *fe6 {
	return new(fe6)
}

func (e *fp6) zero() *fe6 {
	return new(fe6)
}

func (e *fp6) one() *fe6 {
	return new(fe6).one()
}

func (e *fp6) add(c, a, b *fe6) {
	fp2 := e.fp2
	fp2.add(&c[0], &a[0], &b[0])
	fp2.add(&c[1], &a[1], &b[1])
	fp2.add(&c[2], &a[2], &b[2])
}

func (e *fp6) double(c, a *fe6) {
	fp2 := e.fp2
	fp2.double(&c[0], &a[0])
	fp2.double(&c[1], &a[1])
	fp2.double(&c[2], &a[2])
}

func (e *fp6) sub(c, a, b *fe6) {
	fp2 := e.fp2
	fp2.sub(&c[0], &a[0], &b[0])
	fp2.sub(&c[1], &a[1], &b[1])
	fp2.sub(&c[2], &a[2], &b[2])
}

func (e *fp6) neg(c, a *fe6) {
	fp2 := e.fp2
	fp2.neg(&c[0], &a[0])
	fp2.neg(&c[1], &a[1])
	fp2.neg(&c[2], &a[2])
}

func (e *fp6) conjugate(c, a *fe6) {
	fp2 := e.fp2
	c[0].set(&a[0])
	fp2.neg(&c[1], &a[1])
	c[0].set(&a[2])
}

func (e *fp6) mul1(c, a *fe6, b1 *fe2) {
	fp2, t := e.fp2, e.t
	// c0 = αa2b1
	// c1 = a0b1
	// c2 = a1b1
	fp2.mul(t[0], &a[2], b1)
	fp2.mul(&c[2], &a[1], b1)
	fp2.mul(&c[1], &a[0], b1)
	fp2.mulByNonResidue(&c[0], t[0])
}

func (e *fp6) mul0(c, a *fe6, b0 *fe2) {
	// c0 = a0b0
	// c1 = a1b1
	// c2 = a2b2
	fp2 := e.fp2
	fp2.mul(&c[0], &a[0], b0)
	fp2.mul(&c[1], &a[1], b0)
	fp2.mul(&c[2], &a[2], b0)
}

func (e *fp6) mul(c, a, b *fe6) {
	fp2, t := e.fp2, e.t
	// v0 = a0b0
	// v1 = a1b1
	// v2 = a2b2
	// c0 = ((a1 + a2)(b1 + b2) - v1 - v2)α + v0
	// c1 = (a0 + a1)(b0 + b1) - v0 - v1 + αv2
	// c2 = (a0 + a2)(b0 + b2) - v0 - v2 + v1

	fp2.mul(t[0], &a[0], &b[0])     // v0 = a0b0
	fp2.mul(t[1], &a[1], &b[1])     // v1 = a1b1
	fp2.mul(t[2], &a[2], &b[2])     // v2 = b2*b2
	fp2.add(t[3], &a[1], &a[2])     // a1 + a2
	fp2.add(t[4], &b[1], &b[2])     // b1 + b2
	fp2.mul(t[3], t[3], t[4])       // (a1 + a2)(b1 + b2)
	fp2.add(t[4], t[1], t[2])       // v1 + v2
	fp2.sub(t[3], t[3], t[4])       // (a1 + a2)(b1 + b2) - v1 - v2
	fp2.mulByNonResidue(t[3], t[3]) // ((a1 + a2)(b1 + b2) - v1 - v2)α
	fp2.add(t[5], t[0], t[3])       // c0 = ((a1 + a2)(b1 + b2) - v1 - v2)α + v0

	fp2.add(t[3], &a[0], &a[1])     // a0 + a1
	fp2.add(t[4], &b[0], &b[1])     // b0 + b1
	fp2.mul(t[3], t[3], t[4])       // (a0 + a1)(b0 + b1)
	fp2.add(t[4], t[0], t[1])       // v0 + v1
	fp2.sub(t[3], t[3], t[4])       // (a0 + a1)(b0 + b1) - v0 - v1
	fp2.mulByNonResidue(t[4], t[2]) // αv2
	fp2.add(&c[1], t[3], t[4])      // c1 = (a0 + a1)(b0 + b1) - v0 - v1 + αv2

	fp2.add(t[3], &a[0], &a[2]) // a0 + a2
	fp2.add(t[4], &b[0], &b[2]) // b0 + b2
	fp2.mul(t[3], t[3], t[4])   // (a0 + a2)(b0 + b2)
	fp2.add(t[4], t[0], t[2])   // v0 + v2
	fp2.sub(t[3], t[3], t[4])   // (a0 + a2)(b0 + b2) - v0 - v2
	fp2.add(&c[2], t[1], t[3])  // c2 = (a0 + a2)(b0 + b2) - v0 - v2 + v1
	c[0].set(t[5])
}

func (e *fp6) mul01(c, a *fe6, b0, b1 *fe2) {
	// v0 = a0b0
	// v1 = a1b1
	// c0 = (b1(a1 + a2) - v1)α + v0
	// c1 = (a0 + a1)(b0 + b1) - v0 - v1
	// c2 = b0(a0 + a2) - v0 + v1

	fp2, t := e.fp2, e.t
	fp2.mul(t[0], &a[0], b0)        // v0 = b0a0
	fp2.mul(t[1], &a[1], b1)        // v1 = a1b1
	fp2.add(t[5], &a[1], &a[2])     // a1 + a2
	fp2.mul(t[2], b1, t[5])         // b1(a1 + a2)
	fp2.sub(t[2], t[2], t[1])       // b1(a1 + a2) - v1
	fp2.mulByNonResidue(t[2], t[2]) // (b1(a1 + a2) - v1)α
	fp2.add(t[5], &a[0], &a[2])     // a0 + a2
	fp2.mul(t[3], b0, t[5])         // b0(a0 + a2)
	fp2.sub(t[3], t[3], t[0])       // b0(a0 + a2) - v0
	fp2.add(&c[2], t[3], t[1])      // b0(a0 + a2) - v0 + v1
	fp2.add(t[4], b0, b1)           // (b0 + b1)
	fp2.add(t[5], &a[0], &a[1])     // (a0 + a1)
	fp2.mul(t[4], t[4], t[5])       // (a0 + a1)(b0 + b1)
	fp2.sub(t[4], t[4], t[0])       // (a0 + a1)(b0 + b1) - v0
	fp2.sub(&c[1], t[4], t[1])      // (a0 + a1)(b0 + b1) - v0 - v1
	fp2.add(&c[0], t[2], t[0])      //  (b1(a1 + a2) - v1)α + v0
}

func (e *fp6) square(c, a *fe6) {
	fp2, t := e.fp2, e.t
	fp2.square(t[0], &a[0])
	fp2.mul(t[1], &a[0], &a[1])
	fp2.double(t[1], t[1])
	fp2.sub(t[2], &a[0], &a[1])
	fp2.add(t[2], t[2], &a[2])
	fp2.square(t[2], t[2])
	fp2.mul(t[3], &a[1], &a[2])
	fp2.double(t[3], t[3])
	fp2.square(t[4], &a[2])
	fp2.mulByNonResidue(t[5], t[3])
	fp2.add(&c[0], t[0], t[5])
	fp2.mulByNonResidue(t[5], t[4])
	fp2.add(&c[1], t[1], t[5])
	fp2.add(t[1], t[1], t[2])
	fp2.add(t[1], t[1], t[3])
	fp2.add(t[0], t[0], t[4])
	fp2.sub(&c[2], t[1], t[0])
}

func (e *fp6) mulByNonResidue(c, a *fe6) {
	fp2, t := e.fp2, e.t
	t[0].set(&a[0])
	fp2.mulByNonResidue(&c[0], &a[2])
	c[2].set(&a[1])
	c[1].set(t[0])
}

func (e *fp6) exp(c, a *fe6, s *big.Int) {
	z := e.one()
	for i := s.BitLen() - 1; i >= 0; i-- {
		e.square(z, z)
		if s.Bit(i) == 1 {
			e.mul(z, z, a)
		}
	}
	c.set(z)
}

func (e *fp6) inverse(c, a *fe6) {
	fp2, t := e.fp2, e.t
	fp2.square(t[0], &a[0])
	fp2.mul(t[1], &a[1], &a[2])
	fp2.mulByNonResidue(t[1], t[1])
	fp2.sub(t[0], t[0], t[1])
	fp2.square(t[1], &a[1])
	fp2.mul(t[2], &a[0], &a[2])
	fp2.sub(t[1], t[1], t[2])
	fp2.square(t[2], &a[2])
	fp2.mulByNonResidue(t[2], t[2])
	fp2.mul(t[3], &a[0], &a[1])
	fp2.sub(t[2], t[2], t[3])
	fp2.mul(t[3], &a[2], t[2])
	fp2.mul(t[4], &a[1], t[1])
	fp2.add(t[3], t[3], t[4])
	fp2.mulByNonResidue(t[3], t[3])
	fp2.mul(t[4], &a[0], t[0])
	fp2.add(t[3], t[3], t[4])
	fp2.inverse(t[3], t[3])
	fp2.mul(&c[0], t[0], t[3])
	fp2.mul(&c[1], t[2], t[3])
	fp2.mul(&c[2], t[1], t[3])
}

func (e *fp6) frobeniusMap(a *fe6, power int) {
	fp2 := e.fp2
	fp2.frobeniusMap(&a[0], power)
	fp2.frobeniusMap(&a[1], power)
	fp2.frobeniusMap(&a[2], power)
	fp2.mul(&a[1], &a[1], &frobeniusCoeffs61[power%6])
	fp2.mul(&a[2], &a[2], &frobeniusCoeffs62[power%6])
}

func (e *fp6) frobeniusMap1(a *fe6) {
	fp2 := e.fp2
	fp2.frobeniusMap1(&a[0])
	fp2.frobeniusMap1(&a[1])
	fp2.frobeniusMap1(&a[2])
	e.fp2.mul(&a[1], &a[1], &frobeniusCoeffs61[1])
	e.fp2.mul(&a[2], &a[2], &frobeniusCoeffs62[1])
}

func (e *fp6) frobeniusMap2(a *fe6) {
	e.fp2.mul(&a[1], &a[1], &frobeniusCoeffs61[2])
	e.fp2.mul(&a[2], &a[2], &frobeniusCoeffs62[2])
}

func (e *fp6) frobeniusMap3(a *fe6) {
	fp2 := e.fp2
	fp2.frobeniusMap1(&a[0])
	fp2.frobeniusMap1(&a[1])
	fp2.frobeniusMap1(&a[2])
	fp2.neg(&a[1], &a[1])
}
