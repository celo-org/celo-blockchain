package bls12377

import (
	"errors"
	"math/big"
)

type fp2Temp struct {
	t [4]*fe
}

type fp2 struct {
	fp2Temp
}

func newFp2Temp() fp2Temp {
	t := [4]*fe{}
	for i := 0; i < len(t); i++ {
		t[i] = &fe{}
	}
	return fp2Temp{t}
}

func newFp2() *fp2 {
	t := newFp2Temp()
	return &fp2{t}
}

func (e *fp2) fromBytes(in []byte) (*fe2, error) {
	if len(in) != 2*FE_BYTE_SIZE {
		return nil, errors.New("input string should be larger than 96 bytes")
	}
	c0, err := fromBytes(in[:FE_BYTE_SIZE])
	if err != nil {
		return nil, err
	}
	c1, err := fromBytes(in[FE_BYTE_SIZE:])
	if err != nil {
		return nil, err
	}
	return &fe2{*c0, *c1}, nil
}

func (e *fp2) toBytes(a *fe2) []byte {
	out := make([]byte, 2*FE_BYTE_SIZE)
	copy(out[:FE_BYTE_SIZE], toBytes(&a[0]))
	copy(out[FE_BYTE_SIZE:], toBytes(&a[1]))
	return out
}

func (e *fp2) new() *fe2 {
	return new(fe2).zero()
}

func (e *fp2) zero() *fe2 {
	return new(fe2).zero()
}

func (e *fp2) one() *fe2 {
	return new(fe2).one()
}

func (e *fp2) fromMont(c, a *fe2) {
	// c0 = a0 / r
	// c1 = a1 / r
	fromMont(&c[0], &a[0])
	fromMont(&c[1], &a[1])
}

func (e *fp2) add(c, a, b *fe2) {
	// c0 = a0 * b0
	// c1 = a1 * b1
	add(&c[0], &a[0], &b[0])
	add(&c[1], &a[1], &b[1])
}

func (e *fp2) ladd(c, a, b *fe2) {
	// c0 = a0 * b0
	// c1 = a1 * b1
	ladd(&c[0], &a[0], &b[0])
	ladd(&c[1], &a[1], &b[1])
}

func (e *fp2) double(c, a *fe2) {
	// c0 = a0 * 2
	// c1 = a1 * 2
	double(&c[0], &a[0])
	double(&c[1], &a[1])
}

func (e *fp2) ldouble(c, a *fe2) {
	// c0 = a0 * 2
	// c1 = a1 * 2
	ldouble(&c[0], &a[0])
	ldouble(&c[1], &a[1])
}

func (e *fp2) sub(c, a, b *fe2) {
	// c0 = a0 - b0
	// c1 = a1 - b1
	sub(&c[0], &a[0], &b[0])
	sub(&c[1], &a[1], &b[1])
}

func (e *fp2) neg(c, a *fe2) {
	// c0 = -a0
	// c1 = -a1
	neg(&c[0], &a[0])
	neg(&c[1], &a[1])
}

func (e *fp2) conjugate(c, a *fe2) {
	// c0 = a0
	// c1 = -a1
	c[0].set(&a[0])
	neg(&c[1], &a[1])
}

func (e *fp2) mul(c, a, b *fe2) {
	t := e.t
	// Guide to Pairing Based Cryptography
	// 5. Arithmetic of Finite Fields
	// Algorithm 5.16
	//
	// c0 = a0b0 - 5a1b1
	// c1 = (a0 + a1)(b0 + b1) - (a0b0 + a1b1)
	mul(t[1], &a[0], &b[0]) // v0 = a0b0
	mul(t[2], &a[1], &b[1]) // v1 = a1b1
	add(t[0], t[1], t[2])   // v0 + v1
	double(t[3], t[2])      // 2a1b1
	doubleAssign(t[3])      // 4a1b1
	addAssign(t[2], t[3])   // 5(a1 + b1)
	sub(t[3], t[1], t[2])   // a0b0 - 5(a1 + b1)
	add(t[1], &a[0], &a[1]) // a0 + a1
	add(t[2], &b[0], &b[1]) // b0 + b1
	mul(t[1], t[1], t[2])   // (a0 + a1)(b0 + b1)
	c[0].set(t[3])
	sub(&c[1], t[1], t[0]) // (a0 + a1)(b0 + b1) - (a0b0 + a1b1)
}

func (e *fp2) mul0(c, a *fe2, b *fe) {
	mul(&c[0], &a[0], b)
	mul(&c[1], &a[1], b)
}

func (e *fp2) square(c, a *fe2) {
	t := e.t
	// Guide to Pairing Based Cryptography
	// 5. Arithmetic of Finite Fields
	// Algorithm 5.16
	//
	// c0 = (a0 - a1)(a0 + 5a1) - 4a1a0
	// c1 = 2a0a1
	sub(t[0], &a[0], &a[1]) // (a0 - a1)
	double(t[1], &a[1])     // 2a1
	mul(t[2], t[1], &a[0])  // 2a0a1
	c[1].set(t[2])
	double(t[3], t[1])     // 4a1
	addAssign(t[1], t[3])  // 6a1
	doubleAssign(t[2])     // 4a0a1
	addAssign(t[1], t[0])  // (a0 + 5a1)
	mul(t[0], t[0], t[1])  // (a0 - a1)(a0 - 5a1)
	sub(&c[0], t[0], t[2]) // (a0 - a1)(a0 - 5a1) - 4a1a0
}

func (e *fp2) mulByNonResidue(c, a *fe2) {
	t := e.t
	// a = a0 + a1 * v
	// c = -5a1 + a0 * v
	t[0].set(&a[0])
	neg(t[1], &a[1])
	doubleAssign(t[1])
	doubleAssign(t[1])
	sub(&c[0], t[1], &a[1])
	c[1].set(t[0])
}

func (e *fp2) inverse(c, a *fe2) {
	t := e.t
	// c0 = a0 * (a0^2 - β * a1^2)^-1
	// c1 = a1 * (a0^2 - β * a1^2)^-1
	square(t[0], &a[0])
	square(t[1], &a[1])
	double(t[2], t[1])
	doubleAssign(t[2])
	addAssign(t[1], t[2])
	addAssign(t[0], t[1])
	inverse(t[0], t[0])
	mul(&c[0], &a[0], t[0])
	mul(t[0], t[0], &a[1])
	neg(&c[1], t[0])
}

func (e *fp2) exp(c, a *fe2, s *big.Int) {
	z := e.one()
	for i := s.BitLen() - 1; i >= 0; i-- {
		e.square(z, z)
		if s.Bit(i) == 1 {
			e.mul(z, z, a)
		}
	}
	c.set(z)
}

func (e *fp2) frobeniusMap1(a *fe2) {
	e.conjugate(a, a)
}

func (e *fp2) frobeniusMap(a *fe2, power int) {
	mul(&a[1], &a[1], &frobeniusCoeffs2[power%2])
}

func (e *fp2) isQuadraticNonResidue(a *fe2) bool {
	c0, c1, c2 := new(fe), new(fe), new(fe)
	square(c0, &a[0])
	square(c1, &a[1])
	double(c2, c1)
	double(c2, c2)
	addAssign(c1, c2)
	add(c1, c1, c0)
	return isQuadraticNonResidue(c1)
}
