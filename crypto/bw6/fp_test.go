package bw6

import (
	"bytes"
	"crypto/rand"
	"math/big"
	"testing"
)

func TestFpSerialization(t *testing.T) {
	t.Run("zero", func(t *testing.T) {
		in := make([]byte, fpByteSize)
		fe, err := fromBytes(in)
		if err != nil {
			t.Fatal(err)
		}
		if !fe.isZero() {
			t.Fatal("serialization failed")
		}
		if !bytes.Equal(in, toBytes(fe)) {
			t.Fatal("serialization failed")
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
				t.Fatal("serialization failed")
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
				t.Fatal("encoding or decoding failed")
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
				t.Fatal("encoding or decoding failed")
			}
		}
	})
}

func TestFpAdditionCrossAgainstBigInt(t *testing.T) {
	for i := 0; i < fuz; i++ {
		a, _ := new(fe).rand(rand.Reader)
		b, _ := new(fe).rand(rand.Reader)
		c := new(fe)
		big_a := a.big()
		big_b := b.big()
		big_c := new(big.Int)
		add(c, a, b)
		out_1 := c.bytes()
		out_2 := padBytes(big_c.Add(big_a, big_b).Mod(big_c, modulus.big()).Bytes(), fpByteSize)
		if !bytes.Equal(out_1, out_2) {
			t.Fatal("cross test against big.Int is failed A")
		}
		double(c, a)
		out_1 = c.bytes()
		out_2 = padBytes(big_c.Add(big_a, big_a).Mod(big_c, modulus.big()).Bytes(), fpByteSize)
		if !bytes.Equal(out_1, out_2) {
			t.Fatal("cross test against big.Int is failed B")
		}
		sub(c, a, b)
		out_1 = c.bytes()
		out_2 = padBytes(big_c.Sub(big_a, big_b).Mod(big_c, modulus.big()).Bytes(), fpByteSize)
		if !bytes.Equal(out_1, out_2) {
			t.Fatal("cross test against big.Int is failed C")
		}
		neg(c, a)
		out_1 = c.bytes()
		out_2 = padBytes(big_c.Neg(big_a).Mod(big_c, modulus.big()).Bytes(), fpByteSize)
		if !bytes.Equal(out_1, out_2) {
			t.Fatal("cross test against big.Int is failed D")
		}
	}
}

func TestFpAdditionCrossAgainstBigIntAssigned(t *testing.T) {
	for i := 0; i < fuz; i++ {
		a, _ := new(fe).rand(rand.Reader)
		b, _ := new(fe).rand(rand.Reader)
		big_a, big_b := a.big(), b.big()
		addAssign(a, b)
		out_1 := a.bytes()
		out_2 := padBytes(big_a.Add(big_a, big_b).Mod(big_a, modulus.big()).Bytes(), fpByteSize)
		if !bytes.Equal(out_1, out_2) {
			t.Fatal("cross test against big.Int is failed A")
		}
		a, _ = new(fe).rand(rand.Reader)
		big_a = a.big()
		doubleAssign(a)
		out_1 = a.bytes()
		out_2 = padBytes(big_a.Add(big_a, big_a).Mod(big_a, modulus.big()).Bytes(), fpByteSize)
		if !bytes.Equal(out_1, out_2) {
			t.Fatal("cross test against big.Int is failed B")
		}
		a, _ = new(fe).rand(rand.Reader)
		b, _ = new(fe).rand(rand.Reader)
		big_a, big_b = a.big(), b.big()
		subAssign(a, b)
		out_1 = a.bytes()
		out_2 = padBytes(big_a.Sub(big_a, big_b).Mod(big_a, modulus.big()).Bytes(), fpByteSize)
		if !bytes.Equal(out_1, out_2) {
			t.Fatal("cross test against big.Int is failed A")
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
		c0, _ := new(fe).rand(rand.Reader)
		add(c1, a, b)
		add(c1, c1, c0)
		add(c2, a, c0)
		add(c2, c2, b)
		if !c1.equal(c2) {
			t.Fatal("(a + b) + c == (a + c ) + b")
		}
		sub(c1, a, b)
		sub(c1, c1, c0)
		sub(c2, a, c0)
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
		out_2 := padBytes(big_c.Mul(big_a, big_b).Mod(big_c, modulus.big()).Bytes(), fpByteSize)
		if !bytes.Equal(out_1, out_2) {
			t.Fatal("cross test against big.Int is failed")
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
		c0, _ := new(fe).rand(rand.Reader)
		mul(c1, a, b)
		mul(c1, c1, c0)
		mul(c2, c0, b)
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
		_, _ = a.rand(rand.Reader)
		_, _ = b.rand(rand.Reader)
		square(c0, a)
		square(c1, b)
		sub(c0, c0, c1)
		sub(c1, a, b)
		add(c2, a, b)
		mul(c1, c1, c2)
		if !c0.equal(c1) {
			t.Fatal("a^2 - b^2 == (a - b)(a + b)")
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
			t.Fatal("sqrt failed")
		}
		square(rr, r)
		if !rr.equal(aa) {
			t.Fatal("sqrt failed")
		}
	}
}

func TestFpNonResidue(t *testing.T) {
	if !isQuadraticNonResidue(nonResidue1) {
		t.Fatal("element is quadratic non residue, nonResidue1")
	}
	if !isQuadraticNonResidue(new(fe).zero()) {
		t.Fatal("should accept zero as quadratic non residue")
	}
	for i := 0; i < fuz; i++ {
		a, _ := new(fe).rand(rand.Reader)
		square(a, a)
		if isQuadraticNonResidue(a) {
			t.Fatal("element is not quadratic non residue, rand", i)
		}
	}
	for i := 0; i < fuz; i++ {
		a, _ := new(fe).rand(rand.Reader)
		if !sqrt(new(fe), a) {
			if !isQuadraticNonResidue(a) {
				t.Fatal("element is quadratic non residue, rand", i)
			}
		} else {
			i -= 1
		}
	}
}

func TestFp3Serialization(t *testing.T) {
	field := newFp3()
	for i := 0; i < fuz; i++ {
		a, _ := new(fe3).rand(rand.Reader)
		b, err := field.fromBytes(field.toBytes(a))
		if err != nil {
			t.Fatal(err)
		}
		if !a.equal(b) {
			t.Fatal("serialization failed")
		}
	}
}

func TestFp3AdditionProperties(t *testing.T) {
	field := newFp3()
	for i := 0; i < fuz; i++ {
		zero := field.zero()
		a, _ := new(fe3).rand(rand.Reader)
		b, _ := new(fe3).rand(rand.Reader)
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
		c0, _ := new(fe3).rand(rand.Reader)
		field.add(c1, a, b)
		field.add(c1, c1, c0)
		field.add(c2, a, c0)
		field.add(c2, c2, b)
		if !c1.equal(c2) {
			t.Fatal("(a + b) + c == (a + c ) + b")
		}
		field.sub(c1, a, b)
		field.sub(c1, c1, c0)
		field.sub(c2, a, c0)
		field.sub(c2, c2, b)
		if !c1.equal(c2) {
			t.Fatal("(a - b) - c == (a - c ) -b")
		}
	}
}

func TestFp3MultiplicationProperties(t *testing.T) {
	field := newFp3()
	for i := 0; i < fuz; i++ {
		a, _ := new(fe3).rand(rand.Reader)
		b, _ := new(fe3).rand(rand.Reader)
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
		c0, _ := new(fe3).rand(rand.Reader)
		field.mul(c1, a, b)
		field.mul(c1, c1, c0)
		field.mul(c2, c0, b)
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
		_, _ = a.rand(rand.Reader)
		_, _ = b.rand(rand.Reader)
		field.square(c0, a)
		field.square(c1, b)
		field.sub(c0, c0, c1)
		field.sub(c1, a, b)
		field.add(c2, a, b)
		field.mul(c1, c1, c2)
		if !c0.equal(c1) {
			t.Fatal("a^2 - b^2 == (a - b)(a + b)")
		}
	}
}

func TestFp3LazyOperations(t *testing.T) {
	field := newFp3()
	for i := 0; i < fuz; i++ {
		a, _ := new(fe3).rand(rand.Reader)
		b, _ := new(fe3).rand(rand.Reader)
		c, _ := new(fe3).rand(rand.Reader)
		c0 := new(fe3)
		c1 := new(fe3)
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
			t.Fatal("2 l* a = a l+ a", i)
		}
	}
}

func TestFp3Exponentiation(t *testing.T) {
	field := newFp3()
	for i := 0; i < fuz; i++ {
		a, _ := new(fe3).rand(rand.Reader)
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

func TestFp3Inversion(t *testing.T) {
	field := newFp3()
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
		a, _ := new(fe3).rand(rand.Reader)
		field.inverse(u, a)
		field.mul(u, u, a)
		if !u.equal(one) {
			t.Fatal("(r * a) * r * (a ^ -1) == r)")
		}
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
			t.Fatal("serialization failed")
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
		c0, _ := new(fe6).rand(rand.Reader)
		field.add(c1, a, b)
		field.add(c1, c1, c0)
		field.add(c2, a, c0)
		field.add(c2, c2, b)
		if !c1.equal(c2) {
			t.Fatal("(a + b) + c == (a + c ) + b")
		}
		field.sub(c1, a, b)
		field.sub(c1, c1, c0)
		field.sub(c2, a, c0)
		field.sub(c2, c2, b)
		if !c1.equal(c2) {
			t.Fatal("(a - b) - c == (a - c ) -b")
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
		c0, _ := new(fe6).rand(rand.Reader)
		field.mul(c1, a, b)
		field.mul(c1, c1, c0)
		field.mul(c2, c0, b)
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
		_, _ = a.rand(rand.Reader)
		_, _ = b.rand(rand.Reader)
		field.square(c0, a)
		field.square(c1, b)
		field.sub(c0, c0, c1)
		field.sub(c1, a, b)
		field.add(c2, a, b)
		field.mul(c1, c1, c2)
		if !c0.equal(c1) {
			t.Fatal("a^2 - b^2 == (a - b)(a + b)")
		}

	}
}

func TestFp6LazyOperations(t *testing.T) {
	field := newFp6(nil)
	for i := 0; i < fuz; i++ {
		a, _ := new(fe6).rand(rand.Reader)
		b, _ := new(fe6).rand(rand.Reader)
		c, _ := new(fe6).rand(rand.Reader)
		c0 := new(fe6)
		c1 := new(fe6)
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
			t.Fatal("2 l* a = a l+ a", i)
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
		a, _ := new(fe6).rand(rand.Reader)
		field.inverse(u, a)
		field.mul(u, u, a)
		if !u.equal(one) {
			t.Fatal("(r * a) * r * (a ^ -1) == r)")
		}
	}
}

func TestFrobeniusMapping3(t *testing.T) {

	{
		one, two, three := big.NewInt(1), big.NewInt(2), big.NewInt(3)
		for i := 0; i < 3; i++ {
			z := new(big.Int).Exp(modulus.big(), big.NewInt(int64(i)), nil)
			// (p ^ i - 1) / 3
			u1 := new(big.Int).Sub(z, one)
			u1.Div(u1, three)
			// (2 * p ^ i - 2) / 3
			u2 := new(big.Int).Mul(z, two)
			u2.Sub(u2, two).Div(u2, three)

			c1, c2 := new(fe), new(fe)
			exp(c1, nonResidue1, u1)
			exp(c2, nonResidue1, u2)

			if !c1.equal(&frobeniuCoeffs31[i]) {
				t.Fatal("bad Frobenius coefficient fp3 c1")
			}
			if !c2.equal(&frobeniuCoeffs32[i]) {
				t.Fatal("bad Frobenius coefficient fp3 c2")
			}
		}
	}
	f := newFp3()
	a, _ := new(fe3).rand(rand.Reader)
	for i := 0; i < 3; i++ {
		r0, r1 := new(fe3), new(fe3)
		z := new(big.Int).Exp(modulus.big(), big.NewInt(int64(i)), nil)
		f.exp(r0, a, z)
		f.frobeniusMap(r1, a, i)
		if !r1.equal(r0) {
			t.Fatal("frobenius map failed")
		}
	}
}

func TestFrobeniusMapping6(t *testing.T) {
	{
		f := newFp3()
		one, two := big.NewInt(1), big.NewInt(2)
		for i := 0; i < 6; i++ {
			z, r := modulus.big(), new(fe3)
			// p ^ i
			z.Exp(z, big.NewInt(int64(i)), nil)
			// (p ^ i - 1) / 2
			z.Sub(z, one).Div(z, two)
			// r = nr ^ (p ^ i - 1) / 2
			f.exp(r, new(fe3).set(nonResidue3), z)
			if !r[0].equal(&frobeniusCoeffs6[i]) {
				t.Fatal("bad frobenius fp6 coefficient")
			}
		}
	}
	{
		f := newFp6(nil)
		a, _ := new(fe6).rand(rand.Reader)
		for i := 0; i < 6; i++ {
			r0, r1 := new(fe6), new(fe6)

			z := new(big.Int).Exp(modulus.big(), big.NewInt(int64(i)), nil)
			f.exp(r0, a, z)

			f.frobeniusMap(r1, a, i)
			if !r1.equal(r0) {
				t.Fatal("frobenius map failed")
			}
		}
	}
}

func BenchmarkAdd(t *testing.B) {
	a, _ := new(fe).rand(rand.Reader)
	b, _ := new(fe).rand(rand.Reader)
	c := new(fe)
	t.ResetTimer()
	for i := 0; i < t.N; i++ {
		add(c, a, b)
	}
	_ = c
}

func BenchmarkDouble(t *testing.B) {
	a, _ := new(fe).rand(rand.Reader)
	c := new(fe)
	t.ResetTimer()
	for i := 0; i < t.N; i++ {
		double(c, a)
	}
	_ = c
}

func BenchmarkSub(t *testing.B) {
	a, _ := new(fe).rand(rand.Reader)
	b, _ := new(fe).rand(rand.Reader)
	c := new(fe)
	t.ResetTimer()
	for i := 0; i < t.N; i++ {
		sub(c, a, b)
	}
	_ = c
}

func BenchmarkMul(t *testing.B) {
	a, _ := new(fe).rand(rand.Reader)
	b, _ := new(fe).rand(rand.Reader)
	c := new(fe)
	t.ResetTimer()
	for i := 0; i < t.N; i++ {
		mul(c, a, b)
	}
	_ = c
}

func BenchmarkInv(t *testing.B) {
	a, _ := new(fe).rand(rand.Reader)
	c := new(fe)
	t.ResetTimer()
	for i := 0; i < t.N; i++ {
		inverse(c, a)
	}
	_ = c
}
