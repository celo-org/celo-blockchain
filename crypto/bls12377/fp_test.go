package bls12377

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"math/big"
	"testing"
)

func TestFpSerialization(t *testing.T) {
	t.Run("zero", func(t *testing.T) {
		in := make([]byte, 48)
		fe, err := fromBytes(in)
		if err != nil {
			t.Fatal(err)
		}
		if !fe.isZero() {
			t.Fatal("bad serialization")
		}
		if !bytes.Equal(in, toBytes(fe)) {
			t.Fatal("bad serialization")
		}
	})
	t.Run("bytes", func(t *testing.T) {
		for i := 0; i < fuz; i++ {
			a, _ := new(fe).rand(rand.Reader)
			b, err := fromBytes(toBytes(a))
			if err != nil {
				t.Fatal(err)
			}
			if !a.equal(b) {
				t.Fatal("bad serialization")
			}
		}
	})
	t.Run("string", func(t *testing.T) {
		for i := 0; i < fuz; i++ {
			a, _ := new(fe).rand(rand.Reader)
			b, err := fromString(toString(a))
			if err != nil {
				t.Fatal(err)
			}
			if !a.equal(b) {
				t.Fatal("bad encoding or decoding")
			}
		}
	})
	t.Run("big", func(t *testing.T) {
		for i := 0; i < fuz; i++ {
			a, _ := new(fe).rand(rand.Reader)
			b, err := fromBig(toBig(a))
			if err != nil {
				t.Fatal(err)
			}
			if !a.equal(b) {
				t.Fatal("bad encoding or decoding")
			}
		}
	})
}

func TestFpAdditionCrossAgainstBigInt(t *testing.T) {
	for i := 0; i < fuz; i++ {
		a, _ := new(fe).rand(rand.Reader)
		b, _ := new(fe).rand(rand.Reader)
		c := new(fe)
		big_a := toBig(a)
		big_b := toBig(b)
		big_c := new(big.Int)
		add(c, a, b)
		out_1 := toBytes(c)
		out_2 := padBytes(big_c.Add(big_a, big_b).Mod(big_c, modulus.big()).Bytes(), 48)
		if !bytes.Equal(out_1, out_2) {
			t.Fatal("cross test against big.Int is not satisfied A")
		}
		double(c, a)
		out_1 = toBytes(c)
		out_2 = padBytes(big_c.Add(big_a, big_a).Mod(big_c, modulus.big()).Bytes(), 48)
		if !bytes.Equal(out_1, out_2) {
			t.Fatal("cross test against big.Int is not satisfied B")
		}
		sub(c, a, b)
		out_1 = toBytes(c)
		out_2 = padBytes(big_c.Sub(big_a, big_b).Mod(big_c, modulus.big()).Bytes(), 48)
		if !bytes.Equal(out_1, out_2) {
			t.Fatal("cross test against big.Int is not satisfied C")
		}
		neg(c, a)
		out_1 = toBytes(c)
		out_2 = padBytes(big_c.Neg(big_a).Mod(big_c, modulus.big()).Bytes(), 48)
		if !bytes.Equal(out_1, out_2) {
			t.Fatal("cross test against big.Int is not satisfied D")
		}
	}
}

func TestFpAdditionCrossAgainstBigIntAssigned(t *testing.T) {
	for i := 0; i < fuz; i++ {
		a, _ := new(fe).rand(rand.Reader)
		b, _ := new(fe).rand(rand.Reader)
		big_a, big_b := toBig(a), toBig(b)
		addAssign(a, b)
		out_1 := toBytes(a)
		out_2 := padBytes(big_a.Add(big_a, big_b).Mod(big_a, modulus.big()).Bytes(), 48)
		if !bytes.Equal(out_1, out_2) {
			t.Fatal("cross test against big.Int is not satisfied A")
		}
		a, _ = new(fe).rand(rand.Reader)
		big_a = toBig(a)
		doubleAssign(a)
		out_1 = toBytes(a)
		out_2 = padBytes(big_a.Add(big_a, big_a).Mod(big_a, modulus.big()).Bytes(), 48)
		if !bytes.Equal(out_1, out_2) {
			t.Fatal("cross test against big.Int is not satisfied B")
		}
		a, _ = new(fe).rand(rand.Reader)
		b, _ = new(fe).rand(rand.Reader)
		big_a, big_b = toBig(a), toBig(b)
		subAssign(a, b)
		out_1 = toBytes(a)
		out_2 = padBytes(big_a.Sub(big_a, big_b).Mod(big_a, modulus.big()).Bytes(), 48)
		if !bytes.Equal(out_1, out_2) {
			t.Fatal("cross test against big.Int is not satisfied A")
		}
	}
}

func TestFpAdditionProperties(t *testing.T) {
	for i := 0; i < fuz; i++ {

		zero := new(fe).zero()
		a, _ := new(fe).rand(rand.Reader)
		b, _ := new(fe).rand(rand.Reader)
		c1, c2 := new(fe), new(fe)
		add(c1, a, zero)
		if !c1.equal(a) {
			t.Fatal("a + 0 == a")
		}
		sub(c1, a, zero)
		if !c1.equal(a) {
			t.Fatal("a - 0 == a")
		}
		double(c1, zero)
		if !c1.equal(zero) {
			t.Fatal("2 * 0 == 0")
		}
		neg(c1, zero)
		if !c1.equal(zero) {
			t.Fatal("-0 == 0")
		}
		sub(c1, zero, a)
		neg(c2, a)
		if !c1.equal(c2) {
			t.Fatal("0-a == -a")
		}
		double(c1, a)
		add(c2, a, a)
		if !c1.equal(c2) {
			t.Fatal("2 * a == a + a")
		}
		add(c1, a, b)
		add(c2, b, a)
		if !c1.equal(c2) {
			t.Fatal("a + b = b + a")
		}
		sub(c1, a, b)
		sub(c2, b, a)
		neg(c2, c2)
		if !c1.equal(c2) {
			t.Fatal("a - b = - ( b - a )")
		}
		cx, _ := new(fe).rand(rand.Reader)
		add(c1, a, b)
		add(c1, c1, cx)
		add(c2, a, cx)
		add(c2, c2, b)
		if !c1.equal(c2) {
			t.Fatal("(a + b) + c == (a + c ) + b")
		}
		sub(c1, a, b)
		sub(c1, c1, cx)
		sub(c2, a, cx)
		sub(c2, c2, b)
		if !c1.equal(c2) {
			t.Fatal("(a - b) - c == (a - c ) -b")
		}
	}
}

func TestFpAdditionPropertiesAssigned(t *testing.T) {
	for i := 0; i < fuz; i++ {
		zero := new(fe).zero()
		a, b := new(fe), new(fe)
		_, _ = a.rand(rand.Reader)
		b.set(a)
		addAssign(a, zero)
		if !a.equal(b) {
			t.Fatal("a + 0 == a")
		}
		subAssign(a, zero)
		if !a.equal(b) {
			t.Fatal("a - 0 == a")
		}
		a.set(zero)
		doubleAssign(a)
		if !a.equal(zero) {
			t.Fatal("2 * 0 == 0")
		}
		a.set(zero)
		subAssign(a, b)
		neg(b, b)
		if !a.equal(b) {
			t.Fatal("0-a == -a")
		}
		_, _ = a.rand(rand.Reader)
		b.set(a)
		doubleAssign(a)
		addAssign(b, b)
		if !a.equal(b) {
			t.Fatal("2 * a == a + a")
		}
		_, _ = a.rand(rand.Reader)
		_, _ = b.rand(rand.Reader)
		c1, c2 := new(fe).set(a), new(fe).set(b)
		addAssign(c1, b)
		addAssign(c2, a)
		if !c1.equal(c2) {
			t.Fatal("a + b = b + a")
		}
		_, _ = a.rand(rand.Reader)
		_, _ = b.rand(rand.Reader)
		c1.set(a)
		c2.set(b)
		subAssign(c1, b)
		subAssign(c2, a)
		neg(c2, c2)
		if !c1.equal(c2) {
			t.Fatal("a - b = - ( b - a )")
		}
		_, _ = a.rand(rand.Reader)
		_, _ = b.rand(rand.Reader)
		c, _ := new(fe).rand(rand.Reader)
		a0 := new(fe).set(a)
		addAssign(a, b)
		addAssign(a, c)
		addAssign(b, c)
		addAssign(b, a0)
		if !a.equal(b) {
			t.Fatal("(a + b) + c == (b + c) + a")
		}
		_, _ = a.rand(rand.Reader)
		_, _ = b.rand(rand.Reader)
		_, _ = c.rand(rand.Reader)
		a0.set(a)
		subAssign(a, b)
		subAssign(a, c)
		subAssign(a0, c)
		subAssign(a0, b)
		if !a.equal(a0) {
			t.Fatal("(a - b) - c == (a - c) -b")
		}
	}
}

func TestFpLazyOperations(t *testing.T) {
	for i := 0; i < fuz; i++ {
		a, _ := new(fe).rand(rand.Reader)
		b, _ := new(fe).rand(rand.Reader)
		c, _ := new(fe).rand(rand.Reader)
		c0 := new(fe)
		c1 := new(fe)
		ladd(c0, a, b)
		add(c1, a, b)
		mul(c0, c0, c)
		mul(c1, c1, c)
		if !c0.equal(c1) {
			// l+ operator stands for lazy addition
			t.Fatal("(a + b) * c == (a l+ b) * c")
		}
		_, _ = a.rand(rand.Reader)
		b.set(a)
		ldouble(a, a)
		ladd(b, b, b)
		if !a.equal(b) {
			t.Fatal("2 l* a = a l+ a")
		}
		_, _ = a.rand(rand.Reader)
		_, _ = b.rand(rand.Reader)
		_, _ = c.rand(rand.Reader)
		a0 := new(fe).set(a)
		lsubAssign(a, b)
		laddAssign(a, &modulus)
		mul(a, a, c)
		subAssign(a0, b)
		mul(a0, a0, c)
		if !a.equal(a0) {
			t.Fatal("((a l- b) + p) * c = (a-b) * c")
		}
	}
}

func TestFpMultiplicationCrossAgainstBigInt(t *testing.T) {
	for i := 0; i < fuz; i++ {
		a, _ := new(fe).rand(rand.Reader)
		b, _ := new(fe).rand(rand.Reader)
		c := new(fe)
		big_a := toBig(a)
		big_b := toBig(b)
		big_c := new(big.Int)
		mul(c, a, b)
		out_1 := toBytes(c)
		out_2 := padBytes(big_c.Mul(big_a, big_b).Mod(big_c, modulus.big()).Bytes(), 48)
		if !bytes.Equal(out_1, out_2) {
			t.Fatal("cross test against big.Int is not satisfied")
		}
	}
}

func TestFpMultiplicationProperties(t *testing.T) {
	for i := 0; i < fuz; i++ {
		a, _ := new(fe).rand(rand.Reader)
		b, _ := new(fe).rand(rand.Reader)
		zero, one := new(fe).zero(), new(fe).one()
		c1, c2 := new(fe), new(fe)
		mul(c1, a, zero)
		if !c1.equal(zero) {
			t.Fatal("a * 0 == 0")
		}
		mul(c1, a, one)
		if !c1.equal(a) {
			t.Fatal("a * 1 == a")
		}
		mul(c1, a, b)
		mul(c2, b, a)
		if !c1.equal(c2) {
			t.Fatal("a * b == b * a")
		}
		cx, _ := new(fe).rand(rand.Reader)
		mul(c1, a, b)
		mul(c1, c1, cx)
		mul(c2, cx, b)
		mul(c2, c2, a)
		if !c1.equal(c2) {
			t.Fatal("(a * b) * c == (a * c) * b")
		}
		square(a, zero)
		if !a.equal(zero) {
			t.Fatal("0^2 == 0")
		}
		square(a, one)
		if !a.equal(one) {
			t.Fatal("1^2 == 1")
		}
		_, _ = a.rand(rand.Reader)
		square(c1, a)
		mul(c2, a, a)
		if !c1.equal(c1) {
			t.Fatal("a^2 == a*a")
		}
	}
}

func TestFpExponentiation(t *testing.T) {
	for i := 0; i < fuz; i++ {
		a, _ := new(fe).rand(rand.Reader)
		u := new(fe)
		exp(u, a, big.NewInt(0))
		if !u.isOne() {
			t.Fatal("a^0 == 1")
		}
		exp(u, a, big.NewInt(1))
		if !u.equal(a) {
			t.Fatal("a^1 == a")
		}
		v := new(fe)
		mul(u, a, a)
		mul(u, u, u)
		mul(u, u, u)
		exp(v, a, big.NewInt(8))
		if !u.equal(v) {
			t.Fatal("((a^2)^2)^2 == a^8")
		}
		p := modulus.big()
		exp(u, a, p)
		if !u.equal(a) {
			t.Fatal("a^p == a")
		}
		exp(u, a, p.Sub(p, big.NewInt(1)))
		if !u.isOne() {
			t.Fatal("a^(p-1) == 1")
		}
	}
}

func TestFpInversion(t *testing.T) {
	for i := 0; i < fuz; i++ {
		u := new(fe)
		zero, one := new(fe).zero(), new(fe).one()
		inverse(u, zero)
		if !u.equal(zero) {
			t.Fatal("(0^-1) == 0)")
		}
		inverse(u, one)
		if !u.equal(one) {
			t.Fatal("(1^-1) == 1)")
		}
		a, _ := new(fe).rand(rand.Reader)
		inverse(u, a)
		mul(u, u, a)
		if !u.equal(one) {
			t.Fatal("(r*a) * r*(a^-1) == r)")
		}
		v := new(fe)
		p := modulus.big()
		exp(u, a, p.Sub(p, big.NewInt(2)))
		inverse(v, a)
		if !v.equal(u) {
			t.Fatal("a^(p-2) == a^-1")
		}
	}
}

func TestFpSquareRoot(t *testing.T) {
	r := new(fe)
	if sqrt(r, nonResidue1) {
		t.Fatal("non residue cannot have a sqrt")
	}
	for i := 0; i < fuz; i++ {
		a, _ := new(fe).rand(rand.Reader)
		aa, rr, r := &fe{}, &fe{}, &fe{}
		square(aa, a)
		if !sqrt(r, aa) {
			t.Fatal("bad sqrt 1")
		}
		square(rr, r)
		if !rr.equal(aa) {
			t.Fatal("bad sqrt 2")
		}
	}
}

func TestFpNonResidue(t *testing.T) {

	if !isQuadraticNonResidue(nonResidue1) {
		t.Fatal("element is quadratic non residue, 1")
	}
	if isQuadraticNonResidue(new(fe).one()) {
		t.Fatal("one is not quadratic non residue")
	}
	if !isQuadraticNonResidue(new(fe).zero()) {
		t.Fatal("should accept zero as quadratic non residue")
	}
	for i := 0; i < fuz; i++ {
		a, _ := new(fe).rand(rand.Reader)
		square(a, a)
		if isQuadraticNonResidue(a) {
			t.Fatal("element is not quadratic non residue")
		}
	}
	for i := 0; i < fuz; i++ {
		a, _ := new(fe).rand(rand.Reader)
		if !sqrt(new(fe), a) {
			if !isQuadraticNonResidue(a) {
				t.Fatal("element is quadratic non residue, 2", i)
			}
		} else {
			i -= 1
		}
	}
}

func TestFp2Serialization(t *testing.T) {
	field := newFp2()
	for i := 0; i < fuz; i++ {
		a, _ := new(fe2).rand(rand.Reader)
		b, err := field.fromBytes(field.toBytes(a))
		if err != nil {
			t.Fatal(err)
		}
		if !a.equal(b) {
			t.Fatal("bad serialization")
		}
	}
}

func TestFp2AdditionProperties(t *testing.T) {
	field := newFp2()
	for i := 0; i < fuz; i++ {
		zero := field.zero()
		a, _ := new(fe2).rand(rand.Reader)
		b, _ := new(fe2).rand(rand.Reader)
		c1 := field.new()
		c2 := field.new()
		field.add(c1, a, zero)
		if !c1.equal(a) {
			t.Fatal("a + 0 == a")
		}
		field.sub(c1, a, zero)
		if !c1.equal(a) {
			t.Fatal("a - 0 == a")
		}
		field.double(c1, zero)
		if !c1.equal(zero) {
			t.Fatal("2 * 0 == 0")
		}
		field.neg(c1, zero)
		if !c1.equal(zero) {
			t.Fatal("-0 == 0")
		}
		field.sub(c1, zero, a)
		field.neg(c2, a)
		if !c1.equal(c2) {
			t.Fatal("0-a == -a")
		}
		field.double(c1, a)
		field.add(c2, a, a)
		if !c1.equal(c2) {
			t.Fatal("2 * a == a + a")
		}
		field.add(c1, a, b)
		field.add(c2, b, a)
		if !c1.equal(c2) {
			t.Fatal("a + b = b + a")
		}
		field.sub(c1, a, b)
		field.sub(c2, b, a)
		field.neg(c2, c2)
		if !c1.equal(c2) {
			t.Fatal("a - b = - ( b - a )")
		}
		cx, _ := new(fe2).rand(rand.Reader)
		field.add(c1, a, b)
		field.add(c1, c1, cx)
		field.add(c2, a, cx)
		field.add(c2, c2, b)
		if !c1.equal(c2) {
			t.Fatal("(a + b) + c == (a + c ) + b")
		}
		field.sub(c1, a, b)
		field.sub(c1, c1, cx)
		field.sub(c2, a, cx)
		field.sub(c2, c2, b)
		if !c1.equal(c2) {
			t.Fatal("(a - b) - c == (a - c ) -b")
		}
	}
}

func TestFp2LazyOperations(t *testing.T) {
	field := newFp2()
	for i := 0; i < fuz; i++ {
		a, _ := new(fe2).rand(rand.Reader)
		b, _ := new(fe2).rand(rand.Reader)
		c, _ := new(fe2).rand(rand.Reader)
		c0 := new(fe2)
		c1 := new(fe2)
		field.ladd(c0, a, b)
		field.add(c1, a, b)
		field.mul(c0, c0, c)
		field.mul(c1, c1, c)
		if !c0.equal(c1) {
			// l+ operator stands for lazy addition
			t.Fatal("(a + b) * c == (a l+ b) * c")
		}
		_, _ = a.rand(rand.Reader)
		b.set(a)
		field.ldouble(a, a)
		field.ladd(b, b, b)
		if !a.equal(b) {
			t.Fatal("2 l* a = a l+ a")
		}
	}
}

func TestFp2MultiplicationProperties(t *testing.T) {
	field := newFp2()
	for i := 0; i < fuz; i++ {
		a, _ := new(fe2).rand(rand.Reader)
		b, _ := new(fe2).rand(rand.Reader)
		zero := field.zero()
		one := field.one()
		c1, c2 := field.new(), field.new()
		field.mul(c1, a, zero)
		if !c1.equal(zero) {
			t.Fatal("a * 0 == 0")
		}
		field.mul(c1, a, one)
		if !c1.equal(a) {
			t.Fatal("a * 1 == a")
		}
		field.mul(c1, a, b)
		field.mul(c2, b, a)
		if !c1.equal(c2) {
			t.Fatal("a * b == b * a")
		}
		cx, _ := new(fe2).rand(rand.Reader)
		field.mul(c1, a, b)
		field.mul(c1, c1, cx)
		field.mul(c2, cx, b)
		field.mul(c2, c2, a)
		if !c1.equal(c2) {
			t.Fatal("(a * b) * c == (a * c) * b")
		}
		field.square(a, zero)
		if !a.equal(zero) {
			t.Fatal("0^2 == 0")
		}
		field.square(a, one)
		if !a.equal(one) {
			t.Fatal("1^2 == 1")
		}
		_, _ = a.rand(rand.Reader)
		field.square(c1, a)
		field.mul(c2, a, a)
		if !c2.equal(c1) {
			t.Fatal("a^2 == a*a")
		}
	}
}

func TestFp2MultiplicationPropertiesAssigned(t *testing.T) {
	field := newFp2()
	for i := 0; i < fuz; i++ {
		a, _ := new(fe2).rand(rand.Reader)
		zero, one := new(fe2).zero(), new(fe2).one()
		field.mul(a, a, zero)
		if !a.equal(zero) {
			t.Fatal("a * 0 == 0")
		}
		_, _ = a.rand(rand.Reader)
		a0 := new(fe2).set(a)
		field.mul(a, a, one)
		if !a.equal(a0) {
			t.Fatal("a * 1 == a")
		}
		_, _ = a.rand(rand.Reader)
		b, _ := new(fe2).rand(rand.Reader)
		a0.set(a)
		field.mul(a, a, b)
		field.mul(b, b, a0)
		if !a.equal(b) {
			t.Fatal("a * b == b * a")
		}
		c, _ := new(fe2).rand(rand.Reader)
		a0.set(a)
		field.mul(a, a, b)
		field.mul(a, a, c)
		field.mul(a0, a0, c)
		field.mul(a0, a0, b)
		if !a.equal(a0) {
			t.Fatal("(a * b) * c == (a * c) * b")
		}
		a0.set(a)
		field.square(a, a)
		field.mul(a0, a0, a0)
		if !a.equal(a0) {
			t.Fatal("a^2 == a*a")
		}
	}
}

func TestFp2Exponentiation(t *testing.T) {
	field := newFp2()
	for i := 0; i < fuz; i++ {
		a, _ := new(fe2).rand(rand.Reader)
		u := field.new()
		field.exp(u, a, big.NewInt(0))
		if !u.equal(field.one()) {
			t.Fatal("a^0 == 1")
		}
		field.exp(u, a, big.NewInt(1))
		if !u.equal(a) {
			t.Fatal("a^1 == a")
		}
		v := field.new()
		field.mul(u, a, a)
		field.mul(u, u, u)
		field.mul(u, u, u)
		field.exp(v, a, big.NewInt(8))
		if !u.equal(v) {
			t.Fatal("((a^2)^2)^2 == a^8")
		}
	}
}

func TestFp2Inversion(t *testing.T) {
	field := newFp2()
	u := field.new()
	zero := field.zero()
	one := field.one()
	field.inverse(u, zero)
	if !u.equal(zero) {
		t.Fatal("(0 ^ -1) == 0)")
	}
	field.inverse(u, one)
	if !u.equal(one) {
		t.Fatal("(1 ^ -1) == 1)")
	}
	for i := 0; i < fuz; i++ {
		a, _ := new(fe2).rand(rand.Reader)
		field.inverse(u, a)
		field.mul(u, u, a)
		if !u.equal(one) {
			t.Fatal("(r * a) * r * (a ^ -1) == r)")
		}
	}
}

func TestFp2NonResidue(t *testing.T) {
	field := newFp2()
	if !field.isQuadraticNonResidue(nonResidue2) {
		t.Fatal("quadratic non residue is not quadratic non residue")
	}
	if field.isQuadraticNonResidue(new(fe2).one()) {
		t.Fatal("one is not quadratic non residue")
	}
	if !field.isQuadraticNonResidue(new(fe2).zero()) {
		t.Fatal("should accept zero as quadratic non residue")
	}
	for i := 0; i < fuz; i++ {
		a, _ := new(fe2).rand(rand.Reader)
		field.square(a, a)
		if field.isQuadraticNonResidue(new(fe2).one()) {
			t.Fatal("element is not quadratic non residue")
		}
	}
	// for i := 0; i < fuz; i++ {
	// 	a, _ := new(fe2).rand(rand.Reader)
	// 	if !field.sqrt(new(fe2), a) {
	// 		if !field.isQuadraticNonResidue(a) {
	// 			t.Fatal("element is quadratic non residue, 2", i)
	// 		}
	// 	} else {
	// 		i -= 1
	// 	}
	// }
}

func TestFp2MulByNonResidue(t *testing.T) {
	field := newFp2()
	a, _ := new(fe2).rand(rand.Reader)
	r0 := new(fe2)
	r1 := new(fe2)
	field.mulByNonResidue(r0, a)
	field.mul(r1, nonResidue2, a)
	if !r0.equal(r1) {
		t.Fatal("mul by non residue failed")
	}
}

func TestFp6Serialization(t *testing.T) {
	field := newFp6(nil)
	for i := 0; i < fuz; i++ {
		a, _ := new(fe6).rand(rand.Reader)
		b, err := field.fromBytes(field.toBytes(a))
		if err != nil {
			t.Fatal(err)
		}
		if !a.equal(b) {
			t.Fatal("bad serialization")
		}
	}
}

func TestFp6AdditionProperties(t *testing.T) {
	field := newFp6(nil)
	for i := 0; i < fuz; i++ {
		zero := field.zero()
		a, _ := new(fe6).rand(rand.Reader)
		b, _ := new(fe6).rand(rand.Reader)
		c1 := field.new()
		c2 := field.new()
		field.add(c1, a, zero)
		if !c1.equal(a) {
			t.Fatal("a + 0 == a")
		}
		field.sub(c1, a, zero)
		if !c1.equal(a) {
			t.Fatal("a - 0 == a")
		}
		field.double(c1, zero)
		if !c1.equal(zero) {
			t.Fatal("2 * 0 == 0")
		}
		field.neg(c1, zero)
		if !c1.equal(zero) {
			t.Fatal("-0 == 0")
		}
		field.sub(c1, zero, a)
		field.neg(c2, a)
		if !c1.equal(c2) {
			t.Fatal("0-a == -a")
		}
		field.double(c1, a)
		field.add(c2, a, a)
		if !c1.equal(c2) {
			t.Fatal("2 * a == a + a")
		}
		field.add(c1, a, b)
		field.add(c2, b, a)
		if !c1.equal(c2) {
			t.Fatal("a + b = b + a")
		}
		field.sub(c1, a, b)
		field.sub(c2, b, a)
		field.neg(c2, c2)
		if !c1.equal(c2) {
			t.Fatal("a - b = - ( b - a )")
		}
		cx, _ := new(fe6).rand(rand.Reader)
		field.add(c1, a, b)
		field.add(c1, c1, cx)
		field.add(c2, a, cx)
		field.add(c2, c2, b)
		if !c1.equal(c2) {
			t.Fatal("(a + b) + c == (a + c ) + b")
		}
		field.sub(c1, a, b)
		field.sub(c1, c1, cx)
		field.sub(c2, a, cx)
		field.sub(c2, c2, b)
		if !c1.equal(c2) {
			t.Fatal("(a - b) - c == (a - c ) -b")
		}
	}
}

func TestFp6SparseMultiplication(t *testing.T) {
	fp6 := newFp6(nil)
	var a, b, u *fe6
	for j := 0; j < fuz; j++ {
		a, _ = new(fe6).rand(rand.Reader)
		b, _ = new(fe6).rand(rand.Reader)
		u, _ = new(fe6).rand(rand.Reader)
		b[2].zero()
		fp6.mul(u, a, b)
		fp6.mul01(a, a, &b[0], &b[1])
		if !a.equal(u) {
			t.Fatal("bad mul by 01")
		}
	}
	for j := 0; j < fuz; j++ {
		a, _ = new(fe6).rand(rand.Reader)
		b, _ = new(fe6).rand(rand.Reader)
		u, _ = new(fe6).rand(rand.Reader)
		b[2].zero()
		b[0].zero()
		fp6.mul(u, a, b)
		fp6.mul1(a, a, &b[1])
		if !a.equal(u) {
			t.Fatal("bad mul by 1")
		}
	}
}

func TestFp6MultiplicationProperties(t *testing.T) {
	field := newFp6(nil)
	for i := 0; i < fuz; i++ {
		a, _ := new(fe6).rand(rand.Reader)
		b, _ := new(fe6).rand(rand.Reader)
		zero := field.zero()
		one := field.one()
		c1, c2 := field.new(), field.new()
		field.mul(c1, a, zero)
		if !c1.equal(zero) {
			t.Fatal("a * 0 == 0")
		}
		field.mul(c1, a, one)
		if !c1.equal(a) {
			t.Fatal("a * 1 == a")
		}
		field.mul(c1, a, b)
		field.mul(c2, b, a)
		if !c1.equal(c2) {
			t.Fatal("a * b == b * a")
		}
		cx, _ := new(fe6).rand(rand.Reader)
		field.mul(c1, a, b)
		field.mul(c1, c1, cx)
		field.mul(c2, cx, b)
		field.mul(c2, c2, a)
		if !c1.equal(c2) {
			fmt.Println(i)
			t.Fatal("(a * b) * c == (a * c) * b")
		}
		field.square(a, zero)
		if !a.equal(zero) {
			t.Fatal("0^2 == 0")
		}
		field.square(a, one)
		if !a.equal(one) {
			t.Fatal("1^2 == 1")
		}
		_, _ = a.rand(rand.Reader)
		field.square(c1, a)
		field.mul(c2, a, a)
		if !c2.equal(c1) {
			t.Fatal("a^2 == a*a")
		}
	}
}

func TestFp6MultiplicationPropertiesAssigned(t *testing.T) {
	field := newFp6(nil)
	for i := 0; i < fuz; i++ {
		a, _ := new(fe6).rand(rand.Reader)
		zero, one := new(fe6).zero(), new(fe6).one()
		field.mul(a, a, zero)
		if !a.equal(zero) {
			t.Fatal("a * 0 == 0")
		}
		_, _ = a.rand(rand.Reader)
		a0 := new(fe6).set(a)
		field.mul(a, a, one)
		if !a.equal(a0) {
			t.Fatal("a * 1 == a")
		}
		_, _ = a.rand(rand.Reader)
		b, _ := new(fe6).rand(rand.Reader)
		a0.set(a)
		field.mul(a, a, b)
		field.mul(b, b, a0)
		if !a.equal(b) {
			t.Fatal("a * b == b * a")
		}
		c, _ := new(fe6).rand(rand.Reader)
		a0.set(a)
		field.mul(a, a, b)
		field.mul(a, a, c)
		field.mul(a0, a0, c)
		field.mul(a0, a0, b)
		if !a.equal(a0) {
			t.Fatal("(a * b) * c == (a * c) * b")
		}
	}
}

func TestFp6Exponentiation(t *testing.T) {
	field := newFp6(nil)
	for i := 0; i < fuz; i++ {
		a, _ := new(fe6).rand(rand.Reader)
		u := field.new()
		field.exp(u, a, big.NewInt(0))
		if !u.equal(field.one()) {
			t.Fatal("a^0 == 1")
		}
		field.exp(u, a, big.NewInt(1))
		if !u.equal(a) {
			t.Fatal("a^1 == a")
		}
		v := field.new()
		field.mul(u, a, a)
		field.mul(u, u, u)
		field.mul(u, u, u)
		field.exp(v, a, big.NewInt(8))
		if !u.equal(v) {
			t.Fatal("((a^2)^2)^2 == a^8")
		}
	}
}

func TestFp6Inversion(t *testing.T) {
	field := newFp6(nil)
	for i := 0; i < fuz; i++ {
		u := field.new()
		zero := field.zero()
		one := field.one()
		field.inverse(u, zero)
		if !u.equal(zero) {
			t.Fatal("(0^-1) == 0)")
		}
		field.inverse(u, one)
		if !u.equal(one) {
			t.Fatal("(1^-1) == 1)")
		}
		a, _ := new(fe6).rand(rand.Reader)
		field.inverse(u, a)
		field.mul(u, u, a)
		if !u.equal(one) {
			t.Fatal("(r*a) * r*(a^-1) == r)")
		}
	}
}

func TestFp12Serialization(t *testing.T) {
	field := newFp12(nil)
	for i := 0; i < fuz; i++ {
		a, _ := new(fe12).rand(rand.Reader)
		b, err := field.fromBytes(field.toBytes(a))
		if err != nil {
			t.Fatal(err)
		}
		if !a.equal(b) {
			t.Fatal("bad serialization")
		}
	}
}

func TestFp12AdditionProperties(t *testing.T) {
	field := newFp12(nil)
	for i := 0; i < fuz; i++ {
		zero := field.zero()
		a, _ := new(fe12).rand(rand.Reader)
		b, _ := new(fe12).rand(rand.Reader)
		c1 := field.new()
		c2 := field.new()
		field.add(c1, a, zero)
		if !c1.equal(a) {
			t.Fatal("a + 0 == a")
		}
		field.sub(c1, a, zero)
		if !c1.equal(a) {
			t.Fatal("a - 0 == a")
		}
		field.double(c1, zero)
		if !c1.equal(zero) {
			t.Fatal("2 * 0 == 0")
		}
		field.neg(c1, zero)
		if !c1.equal(zero) {
			t.Fatal("-0 == 0")
		}
		field.sub(c1, zero, a)
		field.neg(c2, a)
		if !c1.equal(c2) {
			t.Fatal("0-a == -a")
		}
		field.double(c1, a)
		field.add(c2, a, a)
		if !c1.equal(c2) {
			t.Fatal("2 * a == a + a")
		}
		field.add(c1, a, b)
		field.add(c2, b, a)
		if !c1.equal(c2) {
			t.Fatal("a + b = b + a")
		}
		field.sub(c1, a, b)
		field.sub(c2, b, a)
		field.neg(c2, c2)
		if !c1.equal(c2) {
			t.Fatal("a - b = - ( b - a )")
		}
		cx, _ := new(fe12).rand(rand.Reader)
		field.add(c1, a, b)
		field.add(c1, c1, cx)
		field.add(c2, a, cx)
		field.add(c2, c2, b)
		if !c1.equal(c2) {
			t.Fatal("(a + b) + c == (a + c ) + b")
		}
		field.sub(c1, a, b)
		field.sub(c1, c1, cx)
		field.sub(c2, a, cx)
		field.sub(c2, c2, b)
		if !c1.equal(c2) {
			t.Fatal("(a - b) - c == (a - c ) -b")
		}
	}
}

func TestFp12MultiplicationProperties(t *testing.T) {
	field := newFp12(nil)
	for i := 0; i < fuz; i++ {
		a, _ := new(fe12).rand(rand.Reader)
		b, _ := new(fe12).rand(rand.Reader)
		zero := field.zero()
		one := field.one()
		c1, c2 := field.new(), field.new()
		field.mul(c1, a, zero)
		if !c1.equal(zero) {
			t.Fatal("a * 0 == 0")
		}
		field.mul(c1, a, one)
		if !c1.equal(a) {
			t.Fatal("a * 1 == a")
		}
		field.mul(c1, a, b)
		field.mul(c2, b, a)
		if !c1.equal(c2) {
			t.Fatal("a * b == b * a")
		}
		cx, _ := new(fe12).rand(rand.Reader)
		field.mul(c1, a, b)
		field.mul(c1, c1, cx)
		field.mul(c2, cx, b)
		field.mul(c2, c2, a)
		if !c1.equal(c2) {
			t.Fatal("(a * b) * c == (a * c) * b")
		}
		field.square(a, zero)
		if !a.equal(zero) {
			t.Fatal("0^2 == 0")
		}
		field.square(a, one)
		if !a.equal(one) {
			t.Fatal("1^2 == 1")
		}
		_, _ = a.rand(rand.Reader)
		field.square(c1, a)
		field.mul(c2, a, a)
		if !c2.equal(c1) {
			t.Fatal("a^2 == a*a")
		}
	}
}

func TestFp12MultiplicationPropertiesAssigned(t *testing.T) {
	field := newFp12(nil)
	for i := 0; i < fuz; i++ {
		a, _ := new(fe12).rand(rand.Reader)
		zero, one := new(fe12).zero(), new(fe12).one()
		field.mul(a, a, zero)
		if !a.equal(zero) {
			t.Fatal("a * 0 == 0")
		}
		_, _ = a.rand(rand.Reader)
		a0 := new(fe12).set(a)
		field.mul(a, a, one)
		if !a.equal(a0) {
			t.Fatal("a * 1 == a")
		}
		_, _ = a.rand(rand.Reader)
		b, _ := new(fe12).rand(rand.Reader)
		a0.set(a)
		field.mul(a, a, b)
		field.mul(b, b, a0)
		if !a.equal(b) {
			t.Fatal("a * b == b * a")
		}
		c, _ := new(fe12).rand(rand.Reader)
		a0.set(a)
		field.mul(a, a, b)
		field.mul(a, a, c)
		field.mul(a0, a0, c)
		field.mul(a0, a0, b)
		if !a.equal(a0) {
			t.Fatal("(a * b) * c == (a * c) * b")
		}
	}
}

func TestFp12SparseMultiplication(t *testing.T) {
	fp12 := newFp12(nil)
	var a, b, u *fe12
	for j := 0; j < fuz; j++ {
		a, _ = new(fe12).rand(rand.Reader)
		b, _ = new(fe12).rand(rand.Reader)
		u, _ = new(fe12).rand(rand.Reader)
		b[0][2].zero()
		b[0][1].zero()
		b[1][2].zero()
		fp12.mul(u, a, b)
		fp12.mul034(a, &b[0][0], &b[1][0], &b[1][1])
		if !a.equal(u) {
			t.Fatal("bad mul by 034")
		}
	}
}

func TestFp12Exponentiation(t *testing.T) {
	field := newFp12(nil)
	for i := 0; i < fuz; i++ {
		a, _ := new(fe12).rand(rand.Reader)
		u := field.new()
		field.exp(u, a, big.NewInt(0))
		if !u.equal(field.one()) {
			t.Fatal("a^0 == 1")
		}
		field.exp(u, a, big.NewInt(1))
		if !u.equal(a) {
			t.Fatal("a^1 == a")
		}
		v := field.new()
		field.mul(u, a, a)
		field.mul(u, u, u)
		field.mul(u, u, u)
		field.exp(v, a, big.NewInt(8))
		if !u.equal(v) {
			t.Fatal("((a^2)^2)^2 == a^8")
		}
	}
}

func TestFp12Inversion(t *testing.T) {
	field := newFp12(nil)
	for i := 0; i < fuz; i++ {
		u := field.new()
		zero := field.zero()
		one := field.one()
		field.inverse(u, zero)
		if !u.equal(zero) {
			t.Fatal("(0^-1) == 0)")
		}
		field.inverse(u, one)
		if !u.equal(one) {
			t.Fatal("(1^-1) == 1)")
		}
		a, _ := new(fe12).rand(rand.Reader)
		field.inverse(u, a)
		field.mul(u, u, a)
		if !u.equal(one) {
			t.Fatal("(r*a) * r*(a^-1) == r)")
		}
	}
}

func TestFrobeniusMapping2(t *testing.T) {
	f := newFp2()
	a, _ := new(fe2).rand(rand.Reader)
	b0, b1, b2, b3 := new(fe2), new(fe2), new(fe2), new(fe2)
	f.exp(b0, a, modulus.big())
	f.conjugate(b1, a)
	b2.set(a)
	f.frobeniusMap1(b2)
	b3.set(a)
	f.frobeniusMap(b3, 1)
	if !b0.equal(b3) {
		t.Fatal("frobenius map failed a")
	}
	if !b1.equal(b3) {
		t.Fatal("frobenius map failed b")
	}
	if !b2.equal(b3) {
		t.Fatal("frobenius map failed c")
	}
	_ = b1
}

func TestFrobeniusMapping6(t *testing.T) {
	{
		f := newFp2()
		z := nonResidue2
		for i := 0; i < 6; i++ {
			p, r, e := modulus.big(), new(fe2), big.NewInt(0)
			// p ^ i
			p.Exp(p, big.NewInt(int64(i)), nil)
			// (p ^ i - 1) / 3
			e.Sub(p, big.NewInt(1)).Div(e, big.NewInt(3))
			// r = z ^ (p ^ i - 1) / 3
			f.exp(r, z, e)
			if !r.equal(&frobeniusCoeffs61[i]) {
				t.Fatalf("bad frobenius fp6 1q coefficient")
			}
		}
		for i := 0; i < 6; i++ {
			p, r, e := modulus.big(), new(fe2), big.NewInt(0)
			// p ^ i
			p.Exp(p, big.NewInt(int64(i)), nil).Mul(p, big.NewInt(2))
			// (2 * p ^ i - 2) / 3
			e.Sub(p, big.NewInt(2)).Div(e, big.NewInt(3))
			// r = z ^ (2 * p ^ i - 2) / 3
			f.exp(r, z, e)
			if !r.equal(&frobeniusCoeffs62[i]) {
				t.Fatalf("bad frobenius fp6 2q coefficient")
			}
		}
	}
	f := newFp6(nil)
	r0, r1 := f.new(), f.new()
	e, _ := new(fe6).rand(rand.Reader)
	r0.set(e)
	r1.set(e)
	f.frobeniusMap(r1, 1)
	f.frobeniusMap1(r0)
	if !r0.equal(r1) {
		t.Fatalf("frobenius mapping by 1 failed")
	}
	r0.set(e)
	r1.set(e)
	f.frobeniusMap(r1, 2)
	f.frobeniusMap2(r0)
	if !r0.equal(r1) {
		t.Fatalf("frobenius mapping by 2 failed")
	}
	r0.set(e)
	r1.set(e)
	f.frobeniusMap(r1, 3)
	f.frobeniusMap3(r0)
	if !r0.equal(r1) {
		t.Fatalf("frobenius mapping by 3 failed")
	}
}

func TestFrobeniusMapping12(t *testing.T) {
	{
		f := newFp2()
		z := nonResidue2
		for i := 0; i < 12; i++ {
			p, r, e := modulus.big(), new(fe2), big.NewInt(0)
			// p ^ i
			p.Exp(p, big.NewInt(int64(i)), nil)
			// (p ^ i - 1) / 6
			e.Sub(p, big.NewInt(1)).Div(e, big.NewInt(6))
			// r = z ^ (p ^ i - 1) / 6
			f.exp(r, z, e)
			if !r.equal(&frobeniusCoeffs12[i]) {
				t.Fatalf("bad frobenius fp12 coefficient")
			}
		}
	}
	f := newFp12(nil)
	r0, r1 := f.new(), f.new()
	e, _ := new(fe12).rand(rand.Reader)
	p := modulus.big()
	f.exp(r0, e, p)
	r1.set(e)
	f.frobeniusMap1(r1)
	if !r0.equal(r1) {
		t.Fatalf("frobenius mapping by 1 failed")
	}
	p.Mul(p, modulus.big())
	f.exp(r0, e, p)
	r1.set(e)
	f.frobeniusMap2(r1)
	if !r0.equal(r1) {
		t.Fatalf("frobenius mapping by 2 failed")
	}
	p.Mul(p, modulus.big())
	f.exp(r0, e, p)
	r1.set(e)
	f.frobeniusMap3(r1)
	if !r0.equal(r1) {
		t.Fatalf("frobenius mapping by 2 failed")
	}
}

func BenchmarkMultiplication(t *testing.B) {
	a, _ := new(fe).rand(rand.Reader)
	b, _ := new(fe).rand(rand.Reader)
	c, _ := new(fe).rand(rand.Reader)
	t.ResetTimer()
	for i := 0; i < t.N; i++ {
		mul(c, a, b)
	}
}

func padBytes(in []byte, size int) []byte {
	out := make([]byte, size)
	if len(in) > size {
		panic("bad input for padding")
	}
	copy(out[size-len(in):], in)
	return out
}
