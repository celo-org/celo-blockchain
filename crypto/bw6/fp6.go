package bw6

import (
	"errors"
	"math/big"
)

type fp6Temp struct {
	t3 [4]*fe3
	t1 [11]*fe
}

type fp6 struct {
	fp3 *fp3
	fp6Temp
}

func newFp6Temp() fp6Temp {
	t3 := [4]*fe3{}
	t1 := [11]*fe{}
	for i := 0; i < len(t3); i++ {
		t3[i] = &fe3{}
	}
	for i := 0; i < len(t1); i++ {
		t1[i] = &fe{}
	}
	return fp6Temp{t3, t1}
}

func newFp6(f *fp3) *fp6 {
	t := newFp6Temp()
	if f == nil {
		return &fp6{newFp3(), t}
	}
	return &fp6{f, t}
}

func (e *fp6) fromBytes(in []byte) (*fe6, error) {
	if len(in) != 6*fpByteSize {
		return nil, errors.New("input string length must be equal to 576 bytes")
	}
	fp3 := e.fp3
	c0, err := fp3.fromBytes(in[:3*fpByteSize])
	if err != nil {
		return nil, err
	}
	c1, err := fp3.fromBytes(in[3*fpByteSize:])
	if err != nil {
		return nil, err
	}
	return &fe6{*c0, *c1}, nil
}

func (e *fp6) toBytes(a *fe6) []byte {
	out := make([]byte, 6*fpByteSize)
	fp3 := e.fp3
	copy(out[:3*fpByteSize], fp3.toBytes(&a[0]))
	copy(out[3*fpByteSize:], fp3.toBytes(&a[1]))
	return out
}

func (e *fp6) new() *fe6 {
	return new(fe6).zero()
}

func (e *fp6) zero() *fe6 {
	return new(fe6).zero()
}

func (e *fp6) one() *fe6 {
	return new(fe6).one()
}

func (e *fp6) add(c, a, b *fe6) {
	// c0 = a0 + b0
	// c1 = a1 + b1
	fp3 := e.fp3
	fp3.add(&c[0], &a[0], &b[0])
	fp3.add(&c[1], &a[1], &b[1])
}

func (e *fp6) ladd(c, a, b *fe6) {
	// c0 = a0 + b0
	// c1 = a1 + b1
	fp3 := e.fp3
	fp3.ladd(&c[0], &a[0], &b[0])
	fp3.ladd(&c[1], &a[1], &b[1])
}

func (e *fp6) double(c, a *fe6) {
	// c0 = 2a0
	// c1 = 2a1
	fp3 := e.fp3
	fp3.double(&c[0], &a[0])
	fp3.double(&c[1], &a[1])
}

func (e *fp6) ldouble(c, a *fe6) {
	// c0 = 2a0
	// c1 = 2a1
	fp3 := e.fp3
	fp3.ldouble(&c[0], &a[0])
	fp3.ldouble(&c[1], &a[1])
}

func (e *fp6) sub(c, a, b *fe6) {
	// c0 = a0 - b0
	// c1 = a1 - b1
	fp3 := e.fp3
	fp3.sub(&c[0], &a[0], &b[0])
	fp3.sub(&c[1], &a[1], &b[1])
}

func (e *fp6) neg(c, a *fe6) {
	// c0 = -a0
	// c1 = -a1
	fp3 := e.fp3
	fp3.neg(&c[0], &a[0])
	fp3.neg(&c[1], &a[1])
}

func (e *fp6) conjugate(c, a *fe6) {
	// c0 = a0
	// c1 = -a1
	fp3 := e.fp3
	c[0].set(&a[0])
	fp3.neg(&c[1], &a[1])
}

func (e *fp6) mul(c, a, b *fe6) {
	// Multiplication and Squaring on Pairing-Friendly Fields
	// Karatsuba multiplication algorithm
	// https://eprint.iacr.org/2006/471

	fp3, t := e.fp3, e.t3

	fp3.mul(t[1], &a[0], &b[0]) // v0 = a0b0
	fp3.mul(t[2], &a[1], &b[1]) // v1 = a1b1

	fp3.ladd(t[0], &a[0], &a[1]) // a0 + a1
	fp3.ladd(t[3], &b[0], &b[1]) // b0 + b1
	fp3.mul(t[0], t[0], t[3])    // (a0 + a1)(b0 + b1)
	fp3.sub(t[0], t[0], t[1])    // (a0 + a1)(b0 + b1) - v0
	fp3.sub(&c[1], t[0], t[2])   // c1 = (a0 + a1)(b0 + b1) - v0 - v1

	fp3.mulByNonResidue(t[2], t[2])
	fp3.add(&c[0], t[1], t[2]) // c0 = v0 - ßv1
}

func (e *fp6) mulBy014Assign(a *fe6, c0, c1, c4 *fe) {

	t := e.t1

	t[6].set(&a[0][0])
	t[7].set(&a[0][1])
	t[8].set(&a[0][2])
	t[9].set(&a[1][0])
	t[10].set(&a[1][1])

	double(t[4], c1)
	doubleAssign(t[4])
	neg(t[4], t[4])

	mul(t[1], t[4], t[8])

	double(t[5], c4)
	doubleAssign(t[5])
	neg(t[5], t[5])
	mul(t[2], t[5], t[10])

	mul(t[0], c0, t[6])

	add(&a[0][0], t[0], t[1])
	addAssign(&a[0][0], t[2])

	mul(t[0], c0, t[7])

	mul(t[1], c1, t[6])
	mul(t[2], t[5], &a[1][2])
	add(&a[0][1], t[0], t[1])
	addAssign(&a[0][1], t[2])

	mul(t[0], c0, t[8])
	mul(t[1], c1, t[7])
	mul(t[2], c4, t[9])
	add(&a[0][2], t[0], t[1])
	addAssign(&a[0][2], t[2])

	mul(t[0], c0, t[9])
	mul(t[1], t[4], &a[1][2])
	mul(t[2], t[5], t[8])
	add(&a[1][0], t[0], t[1])
	addAssign(&a[1][0], t[2])

	mul(t[0], c0, t[10])
	mul(t[1], c1, t[9])
	mul(t[2], c4, t[6])
	add(&a[1][1], t[0], t[1])
	addAssign(&a[1][1], t[2])

	mul(t[0], c0, &a[1][2])
	mul(t[1], c1, t[10])
	mul(t[2], c4, t[7])
	add(&a[1][2], t[0], t[1])
	addAssign(&a[1][2], t[2])
}

func (e *fp6) fp2Square(c0, c1, a0, a1 *fe) {
	// Guide to Pairing Based Cryptography
	// Algorithm 5.17

	t := e.t1 // use 7,8

	sub(c0, a0, a1) // v0 = a0 - a1

	ldouble(t[7], a1)
	ldoubleAssign(t[7]) // -ßa1

	laddAssign(t[7], a0) // v3 = (a0 - ßa1)
	mul(c0, c0, t[7])    // v0 * v3
	mul(t[8], a0, a1)    // v2 = a0a1
	addAssign(c0, t[8])  // v0 = (v0 * v3) + v2
	double(c1, t[8])     // c1 = 2v0

	double(t[7], t[8])
	doubleAssign(t[7])  // -αv2
	subAssign(c0, t[7]) // v0 - αv2
}

func (e *fp6) square(c, a *fe6) {
	e.squareComplex(c, a)
}

func (e *fp6) squareKaratsuba(c, a *fe6) {
	// Multiplication and Squaring on Pairing-Friendly Fields
	// Karatsuba squaring algorithm
	// https://eprint.iacr.org/2006/471
	//
	// v0 = a0^2
	// v1 = a1^2
	// c0 = v0 + αv1 = v0 - ßv1
	// c1 = (a0 + a1)^2 - v0 - v1

	fp3, t := e.fp3, e.t3
	fp3.square(t[0], &a[0]) // v0 = a0^2
	fp3.square(t[1], &a[1]) // v1 = a1^2

	fp3.mulByNonResidue(t[2], t[2])
	fp3.sub(t[3], t[0], t[2]) // c0 = v0 - ßv1

	fp3.ladd(t[2], &a[0], &a[1]) // a0 + a1
	fp3.square(t[2], t[2])       // (a0 + a1)^2

	fp3.sub(t[2], t[2], t[0])  // (a0 + a1)^2 - v0
	fp3.sub(&c[1], t[2], t[1]) // c1 = (a0 + a1)^2 - v0 - v1

	c[0].set(t[3])

}

func (e *fp6) squareComplex(c, a *fe6) {
	// Multiplication and Squaring on Pairing-Friendly Fields
	// Complex squaring algorithm
	// https://eprint.iacr.org/2006/471
	//
	// v0 = a0a1
	// c0 = (a0 + a1)(a0 + ßa1) - v0 - ßv0
	// c1 = 2v0
	fp3, t := e.fp3, e.t3
	fp3.mulByNonResidue(t[0], &a[1]) // ßa1
	fp3.mul(t[1], &a[0], &a[1])      // v0 = a0a1
	fp3.mulByNonResidue(t[2], t[1])  // ßv0

	fp3.ladd(t[0], t[0], &a[0]) // a0 + ßa1
	fp3.add(t[2], t[2], t[1])   // v0 + ßv0

	fp3.ladd(t[3], &a[0], &a[1]) // a0 + a1
	fp3.mul(t[0], t[0], t[3])    // (a0 + a1)(a0 + ßa1)

	fp3.sub(&c[0], t[0], t[2]) // c0 = (a0 + a1)(a0 + ßa1) - v0 - ßv0
	fp3.double(&c[1], t[1])    // c1 = 2v0
}

func (e *fp6) cyclotomicSquaring(c, a *fe6) {
	t := e.t1
	// Guide to Pairing Based Cryptography
	// 5.5.4 Airthmetic in Cyclotomic Groups

	e.fp2Square(t[3], t[4], &a[0][0], &a[1][1])

	sub(t[2], t[3], &a[0][0])
	double(t[2], t[2])
	add(&c[0][0], t[2], t[3])
	add(t[2], t[4], &a[1][1])
	double(t[2], t[2])
	add(&c[1][1], t[2], t[4])

	e.fp2Square(t[3], t[4], &a[1][0], &a[0][2])
	e.fp2Square(t[5], t[6], &a[0][1], &a[1][2])

	sub(t[2], t[3], &a[0][1])
	double(t[2], t[2])
	add(&c[0][1], t[2], t[3])
	add(t[2], t[4], &a[1][2])
	double(t[2], t[2])
	add(&c[1][2], t[2], t[4])

	double(t[3], t[6])
	double(t[3], t[3])
	neg(t[3], t[3])

	add(t[2], t[3], &a[1][0])
	double(t[2], t[2])
	add(&c[1][0], t[2], t[3])
	sub(t[2], t[5], &a[0][2])
	double(t[2], t[2])
	add(&c[0][2], t[2], t[5])
}

func (e *fp6) inverse(c, a *fe6) {
	// Guide to Pairing Based Cryptography
	// Algorithm 5.19

	fp3, t := e.fp3, e.t3

	fp3.square(t[0], &a[0]) // a0^2
	fp3.square(t[1], &a[1]) // a1^2

	fp3.mulByNonResidue(t[2], t[1])
	fp3.sub(t[0], t[0], t[2]) // v = a0^2 + ßa1^2
	fp3.inverse(t[1], t[0])   // v = v^-1

	fp3.mul(&c[0], t[1], &a[0]) // a0v
	fp3.mul(t[1], t[1], &a[1])  // a1v
	fp3.neg(&c[1], t[1])
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

func (e *fp6) frobeniusMap(c, a *fe6, power int) {
	fp3 := e.fp3
	fp3.frobeniusMap(&c[0], &a[0], power)
	fp3.frobeniusMap(&c[1], &a[1], power)
	fp3.mul0(&c[1], &c[1], &frobeniusCoeffs6[power%6])
}
