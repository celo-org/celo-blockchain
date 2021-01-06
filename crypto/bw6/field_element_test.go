package bw6

import (
	"bytes"
	"crypto/rand"
	"math/big"
	"testing"
)

func TestFieldElementValidation(t *testing.T) {
	// fe
	zero := new(fe).zero()
	if !zero.isValid() {
		t.Fatal("zero must be valid")
	}
	one := new(fe).one()
	if !one.isValid() {
		t.Fatal("one must be valid")
	}
	if modulus.isValid() {
		t.Fatal("modulus must be invalid")
	}
	n := modulus.big()
	n.Add(n, big.NewInt(1))
	if new(fe).setBig(n).isValid() {
		t.Fatal("number greater than modulus must be invalid")
	}
}

func TestFieldElementEquality(t *testing.T) {
	// fe
	zero := new(fe).zero()
	if !zero.equal(zero) {
		t.Fatal("0 == 0")
	}
	one := new(fe).one()
	if !one.equal(one) {
		t.Fatal("1 == 1")
	}
	a, _ := new(fe).rand(rand.Reader)
	if !a.equal(a) {
		t.Fatal("a == a")
	}
	b := new(fe)
	add(b, a, one)
	if a.equal(b) {
		t.Fatal("a != a + 1")
	}
	// fe3
	zero3 := new(fe3).zero()
	if !zero3.equal(zero3) {
		t.Fatal("0 == 0")
	}
	one3 := new(fe3).one()
	if !one3.equal(one3) {
		t.Fatal("1 == 1")
	}
	a3, _ := new(fe3).rand(rand.Reader)
	if !a3.equal(a3) {
		t.Fatal("a == a")
	}
	b3 := new(fe3)
	fp3 := newFp3()
	fp3.add(b3, a3, one3)
	if a3.equal(b3) {
		t.Fatal("a != a + 1")
	}
	// fe6
	zero6 := new(fe6).zero()
	if !zero6.equal(zero6) {
		t.Fatal("0 == 0")
	}
	one6 := new(fe6).one()
	if !one6.equal(one6) {
		t.Fatal("1 == 1")
	}
	a6, _ := new(fe6).rand(rand.Reader)
	if !a6.equal(a6) {
		t.Fatal("a == a")
	}
	b6 := new(fe6)
	fp6 := newFp6(fp3)
	fp6.add(b6, a6, one6)
	if a6.equal(b6) {
		t.Fatal("a != a + 1")
	}
}

func TestFieldElementHelpers(t *testing.T) {
	// fe
	zero := new(fe).zero()
	if !zero.isZero() {
		t.Fatal("'zero' is not zero")
	}
	one := new(fe).one()
	if !one.isOne() {
		t.Fatal("'one' is not one")
	}
	odd := new(fe).setBig(big.NewInt(1))
	if !odd.isOdd() {
		t.Fatal("1 must be odd")
	}
	if odd.isEven() {
		t.Fatal("1 must not be even")
	}
	even := new(fe).setBig(big.NewInt(2))
	if !even.isEven() {
		t.Fatal("2 must be even")
	}
	if even.isOdd() {
		t.Fatal("2 must not be odd")
	}
	// fe2
	zero3 := new(fe3).zero()
	if !zero3.isZero() {
		t.Fatal("'zero' is not zero, 3")
	}
	one3 := new(fe3).one()
	if !one3.isOne() {
		t.Fatal("'one' is not one, 3")
	}
	// fe6
	zero6 := new(fe6).zero()
	if !zero6.isZero() {
		t.Fatal("'zero' is not zero, 6")
	}
	one6 := new(fe6).one()
	if !one6.isOne() {
		t.Fatal("'one' is not one, 6")
	}
}

func TestFieldElementSerialization(t *testing.T) {
	t.Run("zero", func(t *testing.T) {
		in := make([]byte, fpByteSize)
		fe := new(fe).setBytes(in)
		if !fe.isZero() {
			t.Fatal("serialization failed")
		}
		if !bytes.Equal(in, fe.bytes()) {
			t.Fatal("serialization failed")
		}
	})
	t.Run("bytes", func(t *testing.T) {
		for i := 0; i < fuz; i++ {
			a, _ := new(fe).rand(rand.Reader)
			b := new(fe).setBytes(a.bytes())
			if !a.equal(b) {
				t.Fatal("serialization failed")
			}
		}
	})
	t.Run("big", func(t *testing.T) {
		for i := 0; i < fuz; i++ {
			a, _ := new(fe).rand(rand.Reader)
			b := new(fe).setBig(a.big())
			if !a.equal(b) {
				t.Fatal("encoding or decoding failed")
			}
		}
	})
	t.Run("string", func(t *testing.T) {
		for i := 0; i < fuz; i++ {
			a, _ := new(fe).rand(rand.Reader)
			b, err := new(fe).setString(a.string())
			if err != nil {
				t.Fatal(err)
			}
			if !a.equal(b) {
				t.Fatal("encoding or decoding failed")
			}
		}
	})
}

func TestFieldElementByteInputs(t *testing.T) {
	zero := new(fe).zero()
	in := make([]byte, 0)
	a := new(fe).setBytes(in)
	if !a.equal(zero) {
		t.Fatal("serialization failed")
	}
	in = make([]byte, fpByteSize)
	a = new(fe).setBytes(in)
	if !a.equal(zero) {
		t.Fatal("serialization failed")
	}
	in = make([]byte, fpByteSize+200)
	a = new(fe).setBytes(in)
	if !a.equal(zero) {
		t.Fatal("serialization failed")
	}
	in = make([]byte, fpByteSize+1)
	in[fpByteSize-1] = 1
	normalOne := &fe{1}
	a = new(fe).setBytes(in)
	if !a.equal(normalOne) {
		t.Fatal("serialization failed")
	}
}

func TestFieldElementCopy(t *testing.T) {
	a, _ := new(fe).rand(rand.Reader)
	b := new(fe).set(a)
	if !a.equal(b) {
		t.Fatal("copy failed")
	}
	a3, _ := new(fe3).rand(rand.Reader)
	b3 := new(fe3).set(a3)
	if !a3.equal(b3) {
		t.Fatal("copy failed")
	}
	a6, _ := new(fe6).rand(rand.Reader)
	b6 := new(fe6).set(a6)
	if !a6.equal(b6) {
		t.Fatal("copy failed")
	}
}
