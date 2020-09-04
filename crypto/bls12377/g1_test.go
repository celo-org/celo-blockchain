package bls12377

import (
	"crypto/rand"
	"math/big"
	"testing"
)

func (g *G1) one() *PointG1 {
	one := g.New()
	one.Set(&g1One)
	return one
}

func (g *G1) rand() *PointG1 {
	k, err := rand.Int(rand.Reader, q)
	if err != nil {
		panic(err)
	}
	return g.MulScalar(&PointG1{}, g.one(), k)
}

func (g *G1) rand2() *PointG1 {
	for {
		x, _ := new(fe).rand(rand.Reader)
		y := new(fe)
		square(y, x)
		mul(y, y, x)
		add(y, y, b)
		if sqrt(y, y) {
			return &PointG1{*x, *y, *one}
		}
	}
}

func TestG1ClearCofactor(t *testing.T) {
	g1 := NewG1()
	for i := 0; i < fuz; i++ {
		a := g1.rand2()
		if g1.InCorrectSubgroup(a) {
			t.Fatal("near 0 probablity that this would occur")
		}
		g1.ClearCofactor(a)
		if !g1.InCorrectSubgroup(a) {
			t.Fatal("cofactor is not cleared")
		}
	}

}

func TestG1Serialization(t *testing.T) {
	var err error
	g1 := NewG1()
	zero := g1.Zero()
	b0 := g1.ToBytes(zero)
	p0, err := g1.FromBytes(b0)
	if err != nil {
		t.Fatal(err)
	}
	if !g1.IsZero(p0) {
		t.Fatal("bad infinity serialization 3")
	}
	for i := 0; i < fuz; i++ {
		a := g1.rand()
		_ = a
		uncompressed := g1.ToBytes(a)
		b, err := g1.FromBytes(uncompressed)
		if err != nil {
			t.Fatal(err)
		}
		if !g1.Equal(a, b) {
			t.Fatal("bad serialization 3")
		}
	}
}

func TestG1IsOnCurve(t *testing.T) {
	g := NewG1()
	zero := g.Zero()
	if !g.IsOnCurve(zero) {
		t.Fatal("zero must be on curve")
	}
	one := new(fe).one()
	p := &PointG1{*one, *one, *one}
	if g.IsOnCurve(p) {
		t.Fatal("(1, 1) is not on curve")
	}
}

func TestG1AdditiveProperties(t *testing.T) {
	g := NewG1()
	t0, t1 := g.New(), g.New()
	zero := g.Zero()
	for i := 0; i < fuz; i++ {
		a, b := g.rand(), g.rand()
		g.Add(t0, a, zero)
		if !g.Equal(t0, a) {
			t.Fatal("a + 0 == a")
		}
		g.Add(t0, zero, zero)
		if !g.Equal(t0, zero) {
			t.Fatal("0 + 0 == 0")
		}
		g.Sub(t0, a, zero)
		if !g.Equal(t0, a) {
			t.Fatal("a - 0 == a")
		}
		g.Sub(t0, zero, zero)
		if !g.Equal(t0, zero) {
			t.Fatal("0 - 0 == 0")
		}
		g.Neg(t0, zero)
		if !g.Equal(t0, zero) {
			t.Fatal("- 0 == 0")
		}
		g.Sub(t0, zero, a)
		g.Neg(t0, t0)
		if !g.Equal(t0, a) {
			t.Fatal(" - (0 - a) == a")
		}
		g.Double(t0, zero)
		if !g.Equal(t0, zero) {
			t.Fatal("2 * 0 == 0")
		}
		g.Double(t0, a)
		g.Sub(t0, t0, a)
		if !g.Equal(t0, a) || !g.IsOnCurve(t0) {
			t.Fatal(" (2 * a) - a == a")
		}
		g.Add(t0, a, b)
		g.Add(t1, b, a)
		if !g.Equal(t0, t1) {
			t.Fatal("a + b == b + a")
		}
		g.Sub(t0, a, b)
		g.Sub(t1, b, a)
		g.Neg(t1, t1)
		if !g.Equal(t0, t1) {
			t.Fatal("a - b == - ( b - a )")
		}
		c := g.rand()
		g.Add(t0, a, b)
		g.Add(t0, t0, c)
		g.Add(t1, a, c)
		g.Add(t1, t1, b)
		if !g.Equal(t0, t1) {
			t.Fatal("(a + b) + c == (a + c ) + b")
		}
		g.Sub(t0, a, b)
		g.Sub(t0, t0, c)
		g.Sub(t1, a, c)
		g.Sub(t1, t1, b)
		if !g.Equal(t0, t1) {
			t.Fatal("(a - b) - c == (a - c) -b")
		}
	}
}

func TestG1MultiplicativeProperties(t *testing.T) {
	g := NewG1()
	t0, t1 := g.New(), g.New()
	zero := g.Zero()
	for i := 0; i < fuz; i++ {
		a := g.rand()
		s1, s2, s3 := randScalar(q), randScalar(q), randScalar(q)
		sone := big.NewInt(1)
		g.MulScalar(t0, zero, s1)
		if !g.Equal(t0, zero) {
			t.Fatal(" 0 ^ s == 0")
		}
		g.MulScalar(t0, a, sone)
		if !g.Equal(t0, a) {
			t.Fatal(" a ^ 1 == a")
		}
		g.MulScalar(t0, zero, s1)
		if !g.Equal(t0, zero) {
			t.Fatal(" 0 ^ s == a")
		}
		g.MulScalar(t0, a, s1)
		g.MulScalar(t0, t0, s2)
		s3.Mul(s1, s2)
		g.MulScalar(t1, a, s3)
		if !g.Equal(t0, t1) {
			t.Errorf(" (a ^ s1) ^ s2 == a ^ (s1 * s2)")
		}
		g.MulScalar(t0, a, s1)
		g.MulScalar(t1, a, s2)
		g.Add(t0, t0, t1)
		s3.Add(s1, s2)
		g.MulScalar(t1, a, s3)
		if !g.Equal(t0, t1) {
			t.Errorf(" (a ^ s1) + (a ^ s2) == a ^ (s1 + s2)")
		}
	}
}

func TestG1MultiExpExpected(t *testing.T) {
	g := NewG1()
	one := g.one()
	var scalars [2]*big.Int
	var bases [2]*PointG1
	scalars[0] = big.NewInt(2)
	scalars[1] = big.NewInt(3)
	bases[0], bases[1] = new(PointG1).Set(one), new(PointG1).Set(one)
	expected, result := g.New(), g.New()
	g.MulScalar(expected, one, big.NewInt(5))
	_, _ = g.MultiExp(result, bases[:], scalars[:])
	if !g.Equal(expected, result) {
		t.Fatal("bad multi-exponentiation")
	}
}

func TestG1MultiExpBatch(t *testing.T) {
	g := NewG1()
	one := g.one()
	n := 1000
	bases := make([]*PointG1, n)
	scalars := make([]*big.Int, n)
	// scalars: [s0,s1 ... s(n-1)]
	// bases: [P0,P1,..P(n-1)] = [s(n-1)*G, s(n-2)*G ... s0*G]
	for i, j := 0, n-1; i < n; i, j = i+1, j-1 {
		scalars[j], _ = rand.Int(rand.Reader, big.NewInt(100000))
		bases[i] = g.New()
		g.MulScalar(bases[i], one, scalars[j])
	}
	// expected: s(n-1)*P0 + s(n-2)*P1 + s0*P(n-1)
	expected, tmp := g.New(), g.New()
	for i := 0; i < n; i++ {
		g.MulScalar(tmp, bases[i], scalars[i])
		g.Add(expected, expected, tmp)
	}
	result := g.New()
	_, _ = g.MultiExp(result, bases, scalars)
	if !g.Equal(expected, result) {
		t.Fatal("bad multi-exponentiation")
	}
}

func BenchmarkG1Add(t *testing.B) {
	g1 := NewG1()
	a, b, c := g1.rand(), g1.rand(), PointG1{}
	t.ResetTimer()
	for i := 0; i < t.N; i++ {
		g1.Add(&c, a, b)
	}
}

func BenchmarkG1Mul(t *testing.B) {
	g1 := NewG1()
	a, e, c := g1.rand(), q, PointG1{}
	t.ResetTimer()
	for i := 0; i < t.N; i++ {
		g1.MulScalar(&c, a, e)
	}
}
