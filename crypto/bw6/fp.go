package bw6

import (
	"errors"
	"math/big"
)

func fromBytes(in []byte) (*fe, error) {
	fe := &fe{}
	if len(in) != fpByteSize {
		return nil, errors.New("input string length must be equal to 96 bytes")
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
	for i := 0; i < twelveWordBitSize*2; i++ {
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

	if k < fpBitSize || k > fpBitSize+twelveWordBitSize {
		inv.zero()
		return
	}

	if r.cmp(&modulus) != -1 || z > 0 {
		lsubAssign(r, &modulus)
	}
	u.set(&modulus)
	lsubAssign(u, r)

	// Phase 2
	for i := k; i < twelveWordBitSize*2; i++ {
		double(u, u)
	}
	inv.set(u)
}

func inverseBatch(in []fe) {

	n, N, setFirst := 0, len(in), false

	for i := 0; i < len(in); i++ {
		if !in[i].isZero() {
			n++
		}
	}
	if n == 0 {
		return
	}

	tA := make([]fe, n)
	tB := make([]fe, n)

	for i, j := 0, 0; i < N; i++ {
		if !in[i].isZero() {
			if !setFirst {
				setFirst = true
				tA[j].set(&in[i])
			} else {
				mul(&tA[j], &in[i], &tA[j-1])
			}
			j = j + 1
		}
	}

	inverse(&tB[n-1], &tA[n-1])
	for i, j := N-1, n-1; j != 0; i-- {
		if !in[i].isZero() {
			mul(&tB[j-1], &tB[j], &in[i])
			j = j - 1
		}
	}

	for i, j := 0, 0; i < N; i++ {
		if !in[i].isZero() {
			if setFirst {
				setFirst = false
				in[i].set(&tB[j])
			} else {
				mul(&in[i], &tA[j-1], &tB[j])
			}
			j = j + 1
		}
	}
}

func sqrt(c, a *fe) bool {
	u, v := new(fe).set(a), new(fe)
	exp(c, a, pPlus1Over4)
	square(v, c)
	return u.equal(v)
}

func isQuadraticNonResidue(a *fe) bool {
	result := new(fe)
	exp(result, a, pMinus1Over2)
	return !result.isOne()
}
