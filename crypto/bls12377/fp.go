package bls12377

import (
	"errors"
	"math/big"
)

func fromBytes(in []byte) (*fe, error) {
	fe := &fe{}
	if len(in) != FE_BYTE_SIZE {
		return nil, errors.New("input string should be equal 48 bytes")
	}
	fe.setBytes(in)
	if !fe.isValid() {
		return nil, errors.New("must be less than modulus")
	}
	toMont(fe, fe)
	return fe, nil
}

func fromBig(in *big.Int) (*fe, error) {
	fe := new(fe).setBig(in)
	if !fe.isValid() {
		return nil, errors.New("invalid input string")
	}
	toMont(fe, fe)
	return fe, nil
}

func fromString(in string) (*fe, error) {
	fe, err := new(fe).setString(in)
	if err != nil {
		return nil, err
	}
	if !fe.isValid() {
		return nil, errors.New("invalid input string")
	}
	toMont(fe, fe)
	return fe, nil
}

func toBytes(e *fe) []byte {
	e2 := new(fe)
	fromMont(e2, e)
	return e2.bytes()
}

func toBig(e *fe) *big.Int {
	e2 := new(fe)
	fromMont(e2, e)
	return e2.big()
}

func toString(e *fe) (s string) {
	e2 := new(fe)
	fromMont(e2, e)
	return e2.string()
}

func toMont(c, a *fe) {
	mul(c, a, r2)
}

func fromMont(c, a *fe) {
	mul(c, a, &fe{1})
}

func exp(c, a *fe, e *big.Int) {
	z := new(fe).set(r1)
	for i := e.BitLen(); i >= 0; i-- {
		mul(z, z, z)
		if e.Bit(i) == 1 {
			mul(z, z, a)
		}
	}
	c.set(z)
}

func inverse(inv, e *fe) {
	if e.isZero() {
		inv.zero()
		return
	}
	u := new(fe).set(&modulus)
	v := new(fe).set(e)
	s := &fe{1}
	r := &fe{0}
	var k int
	var z uint64
	var found = false
	// Phase 1
	for i := 0; i < SIX_WORD_BIT_SIZE*2; i++ {
		if v.isZero() {
			found = true
			break
		}
		if u.isEven() {
			u.div2(0)
			s.mul2()
		} else if v.isEven() {
			v.div2(0)
			z += r.mul2()
		} else if u.cmp(v) == 1 {
			lsubAssign(u, v)
			u.div2(0)
			laddAssign(r, s)
			s.mul2()
		} else {
			lsubAssign(v, u)
			v.div2(0)
			laddAssign(s, r)
			z += r.mul2()
		}
		k += 1
	}

	if !found {
		inv.zero()
		return
	}

	if k < FE_BIT_SIZE || k > FE_BIT_SIZE+SIX_WORD_BIT_SIZE {
		inv.zero()
		return
	}

	if r.cmp(&modulus) != -1 || z > 0 {
		lsubAssign(r, &modulus)
	}
	u.set(&modulus)
	lsubAssign(u, r)

	// Phase 2
	for i := k; i < SIX_WORD_BIT_SIZE*2; i++ {
		double(u, u)
	}
	inv.set(u)
}

func isQuadraticNonResidue(elem *fe) bool {
	result := new(fe)
	exp(result, elem, pMinus1Over2)
	return !result.isOne()
}

func sqrt(c, a *fe) bool {
	// Adapted from gurvy library
	// https://github.com/ConsenSys/gurvy/blob/54ab476362d81c5017e853b0c68321544d5acfa8/bls377/fp/element.go#L417
	w, y, b, t, g := new(fe), new(fe), new(fe), new(fe), new(fe)
	exp(w, a, sSqrt)
	mul(y, a, w)
	mul(b, w, y)
	g.set(sNonResidue)
	r := uint64(46)
	t.set(b)
	for i := uint64(0); i < r-1; i++ {
		square(t, t)
	}
	if t.isZero() {
		c.set(zero)
		return true
	}
	if !t.isOne() {
		return false
	}
	for {
		var m uint64
		t.set(b)

		for !t.isOne() {
			square(t, t)
			m += 1
		}

		if m == 0 {
			c.set(y)
			return true
		}

		ge := int(r - m - 1)
		t.set(g)
		for ge > 0 {
			square(t, t)
			ge--
		}
		square(g, t)
		mul(y, y, t)
		mul(b, b, g)
		r = m
	}
}
