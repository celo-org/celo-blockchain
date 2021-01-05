package bw6

import (
	"errors"
	"math/big"
)

type fp3Temp struct {
	t [6]*fe
}

type fp3 struct {
	fp3Temp
}

func newFp3Temp() fp3Temp {
	t := [6]*fe{}
	for i := 0; i < len(t); i++ {
		t[i] = &fe{}
	}
	return fp3Temp{t}
}

func newFp3() *fp3 {
	t := newFp3Temp()
	return &fp3{t}
}

func (e *fp3) fromBytes(in []byte) (*fe3, error) {
	if len(in) != 3*fpByteSize {
		return nil, errors.New("input string length must be equal to 288 bytes")
	}
	c0, err := fromBytes(in[:fpByteSize])
	if err != nil {
		return nil, err
	}
	c1, err := fromBytes(in[fpByteSize : 2*fpByteSize])
	if err != nil {
		return nil, err
	}
	c2, err := fromBytes(in[2*fpByteSize : 3*fpByteSize])
	if err != nil {
		return nil, err
	}
	return &fe3{*c0, *c1, *c2}, nil
}

func (e *fp3) toBytes(a *fe3) []byte {
	out := make([]byte, 3*fpByteSize)
	copy(out[:fpByteSize], toBytes(&a[0]))
	copy(out[fpByteSize:2*fpByteSize], toBytes(&a[1]))
	copy(out[2*fpByteSize:3*fpByteSize], toBytes(&a[2]))
	return out
}

func (e *fp3) new() *fe3 {
	return new(fe3).zero()
}

func (e *fp3) zero() *fe3 {
	return new(fe3).zero()
}

func (e *fp3) one() *fe3 {
	return new(fe3).one()
}

func (e *fp3) add(c, a, b *fe3) {
	// c0 = a0 + b0
	// c1 = a1 + b1
	// c2 = a2 + b2
	add(&c[0], &a[0], &b[0])
	add(&c[1], &a[1], &b[1])
	add(&c[2], &a[2], &b[2])
}

func (e *fp3) ladd(c, a, b *fe3) {
	// c0 = a0 + b0
	// c1 = a1 + b1
	// c2 = a2 + b2
	ladd(&c[0], &a[0], &b[0])
	ladd(&c[1], &a[1], &b[1])
	ladd(&c[2], &a[2], &b[2])
}

func (e *fp3) double(c, a *fe3) {
	// c0 = 2a0
	// c1 = 2a1
	// c2 = 2a2
	double(&c[0], &a[0])
	double(&c[1], &a[1])
	double(&c[2], &a[2])
}

func (e *fp3) ldouble(c, a *fe3) {
	// c0 = 2a0
	// c1 = 2a1
	// c2 = 2a2
	ldouble(&c[0], &a[0])
	ldouble(&c[1], &a[1])
	ldouble(&c[2], &a[2])
}

func (e *fp3) sub(c, a, b *fe3) {
	// c0 = a0 - b0
	// c1 = a1 - b1
	// c2 = a2 - b2
	sub(&c[0], &a[0], &b[0])
	sub(&c[1], &a[1], &b[1])
	sub(&c[2], &a[2], &b[2])
}

func (e *fp3) neg(c, a *fe3) {
	// c0 = -a0
	// c1 = -a1
	// c2 = -a2
	neg(&c[0], &a[0])
	neg(&c[1], &a[1])
	neg(&c[2], &a[2])
}

func (e *fp3) conjugate(c, a *fe3) {
	// c0 = a0
	// c1 = -a1
	// c2 = a2
	c.set(a)
	c[0].set(&a[0])
	neg(&c[1], &a[1])
	c[2].set(&a[2])
}

func (e *fp3) mul(c, a, b *fe3) {
	// Guide to Pairing Based Cryptography
	// Algorithm 5.21

	t := e.t
	mul(t[0], &a[0], &b[0])  // v0 = a0b0
	mul(t[1], &a[1], &b[1])  // v1 = a1b1
	mul(t[2], &a[2], &b[2])  // v2 = a2b2
	ladd(t[3], &a[1], &a[2]) // a1 + a2
	ladd(t[4], &b[1], &b[2]) // b1 + b2
	mul(t[3], t[3], t[4])    // (a1 + a2)(b1 + b2)
	add(t[4], t[1], t[2])    // v1 + v2
	subAssign(t[3], t[4])    // (a1 + a2)(b1 + b2) - v1 - v2

	doubleAssign(t[3])
	doubleAssign(t[3])    // -((a1 + a2)(b1 + b2) - v1 - v2)α
	sub(t[5], t[0], t[3]) // c0 = ((a1 + a2)(b1 + b2) - v1 - v2)α + v0

	ladd(t[3], &a[0], &a[1]) // a0 + a1
	ladd(t[4], &b[0], &b[1]) // b0 + b1
	mul(t[3], t[3], t[4])    // (a0 + a1)(b0 + b1)
	add(t[4], t[0], t[1])    // v0 + v1
	sub(t[3], t[3], t[4])    // (a0 + a1)(b0 + b1) - v0 - v1

	double(t[4], t[2])
	doubleAssign(t[4])     // -αv2
	sub(&c[1], t[3], t[4]) // c1 = (a0 + a1)(b0 + b1) - v0 - v1 + αv2

	ladd(t[3], &a[0], &a[2]) // a0 + a2
	ladd(t[4], &b[0], &b[2]) // b0 + b2
	mul(t[3], t[3], t[4])    // (a0 + a2)(b0 + b2)
	add(t[4], t[0], t[2])    // v0 + v2
	sub(t[3], t[3], t[4])    // (a0 + a2)(b0 + b2) - v0 - v2
	add(&c[2], t[1], t[3])   // c2 = (a0 + a2)(b0 + b2) - v0 - v2 + v1
	c[0].set(t[5])
}

func (e *fp3) square(c, a *fe3) {
	// Multiplication and Squaring on Pairing-Friendly Fields
	// Algorithm CH-SQR2
	// https://eprint.iacr.org/2006/471

	t := e.t
	square(t[0], &a[0])     // s0 = a0^2
	mul(t[1], &a[0], &a[1]) // a0a1
	doubleAssign(t[1])      // s1 = 2a0a1
	sub(t[2], &a[0], &a[1]) // a0 - a1
	laddAssign(t[2], &a[2]) // a0 - a1 + a2
	square(t[2], t[2])      // s2 = (a0 - a1 + a2)^2
	mul(t[3], &a[1], &a[2]) // a1a2
	doubleAssign(t[3])      // s3 = 2a1a2
	square(t[4], &a[2])     // s4 = a2^2

	double(t[5], t[3])
	doubleAssign(t[5]) // -αs3

	sub(&c[0], t[0], t[5]) // c0 = s0 + αs3

	double(t[5], t[4])
	doubleAssign(t[5]) // -αs4

	sub(&c[1], t[1], t[5]) // c1 = s1 + αs4

	addAssign(t[1], t[2])
	add(t[1], t[1], t[3])
	addAssign(t[0], t[4])
	sub(&c[2], t[1], t[0]) // c2 = s1 + s2 - s0 - s4
}

func (e *fp3) mul0(c, a *fe3, z *fe) {
	mul(&c[0], &a[0], z)
	mul(&c[1], &a[1], z)
	mul(&c[2], &a[2], z)
}

func (e *fp3) mulByNonResidue(c, a *fe3) {
	// c0 = -4 * a2
	// c1 = a0
	// c2 = a1

	t := e.t
	t[0].set(&a[2])
	c[2].set(&a[1])
	c[1].set(&a[0])
	doubleAssign(t[0])
	doubleAssign(t[0])
	neg(t[0], t[0])
	c[0].set(t[0])
}

func (e *fp3) exp(c, a *fe3, s *big.Int) {
	z := e.one()
	for i := s.BitLen() - 1; i >= 0; i-- {
		e.square(z, z)
		if s.Bit(i) == 1 {
			e.mul(z, z, a)
		}
	}
	c.set(z)
}

func (e *fp3) inverse(c, a *fe3) {
	// Guide to Pairing Based Cryptography
	// Algorithm 5.23

	t := e.t
	square(t[0], &a[0])     // v0 = a0^2
	mul(t[1], &a[1], &a[2]) // v5 = a1a2

	doubleAssign(t[1])
	doubleAssign(t[1])
	neg(t[1], t[1]) // αv5

	subAssign(t[0], t[1])   // A = v0 - αv5
	square(t[1], &a[1])     // v1 = a1^2
	mul(t[2], &a[0], &a[2]) // v4 = a0a2
	subAssign(t[1], t[2])   // C = v1 - v4
	square(t[2], &a[2])     // v2 = a2^2

	doubleAssign(t[2])
	doubleAssign(t[2])
	neg(t[2], t[2]) // αv2

	mul(t[3], &a[0], &a[1]) // v3 = a0a1
	subAssign(t[2], t[3])   // B = αv2 - v3
	mul(t[3], &a[2], t[2])  // B * a2
	mul(t[4], &a[1], t[1])  // C * a1
	addAssign(t[3], t[4])   // Ca1 + Ba2

	doubleAssign(t[3])
	doubleAssign(t[3]) // -α(Ca1 + Ba2)

	mul(t[4], &a[0], t[0]) // Aa0
	subAssign(t[4], t[3])  // v6 = Aa0 + α(Ca1 + Ba2)
	inverse(t[4], t[4])    // F = v6^-1
	mul(&c[0], t[0], t[4]) // c0 = AF
	mul(&c[1], t[2], t[4]) // c1 = BF
	mul(&c[2], t[1], t[4]) // c2 = CF
}

func (e *fp3) frobeniusMap(c, a *fe3, power int) {
	c[0].set(&a[0])
	mul(&c[1], &a[1], &frobeniuCoeffs31[power%3])
	mul(&c[2], &a[2], &frobeniuCoeffs32[power%3])
}

func (e *fp3) frobeniusMap1(c, a *fe3, power int) {
	c[0].set(&a[0])
	mul(&c[1], &a[1], &frobeniuCoeffs31[1])
	mul(&c[2], &a[2], &frobeniuCoeffs32[1])
}

func (e *fp3) frobeniusMap2(c, a *fe3, power int) {
	c[0].set(&a[0])
	mul(&c[1], &a[1], &frobeniuCoeffs31[2])
	mul(&c[2], &a[2], &frobeniuCoeffs32[2])
}
