package bls12377

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"testing"
)

func (g *G2) one() *PointG2 {
	one := g.New()
	one.Set(&g2One)
	return one
}

func (g *G2) rand() *PointG2 {
	k, err := rand.Int(rand.Reader, q)
	if err != nil {
		panic(err)
	}
	return g.MulScalar(&PointG2{}, g.one(), k)
}

func (g *G2) new() *PointG2 {
	return g.Zero()
}

func TestG2Serialization(t *testing.T) {
	var err error
	g2 := NewG2()
	zero := g2.Zero()
	b0 := g2.ToBytes(zero)
	p0, err := g2.FromBytes(b0)
	if err != nil {
		t.Fatal(err)
	}
	if !g2.IsZero(p0) {
		t.Fatal("bad infinity serialization 3")
	}
	for i := 0; i < fuz; i++ {
		a := g2.rand()
		uncompressed := g2.ToBytes(a)
		b, err := g2.FromBytes(uncompressed)
		if err != nil {
			t.Fatal(err)
		}
		if !g2.Equal(a, b) {
			t.Fatal("bad serialization 3")
		}
	}
	for i := 0; i < fuz; i++ {
		a := g2.rand()
		encoded := g2.EncodePoint(a)
		b, err := g2.DecodePoint(encoded)
		if err != nil {
			t.Fatal(err)
		}
		if !g2.Equal(a, b) {
			t.Fatal("bad serialization encode/decode")
		}
	}
}

func TestG2IsOnCurve(t *testing.T) {
	g := NewG2()
	zero := g.Zero()
	if !g.IsOnCurve(zero) {
		t.Fatal("zero must be on curve")
	}
	one := new(fe2).one()
	p := &PointG2{*one, *one, *one}
	if g.IsOnCurve(p) {
		t.Fatal("(1, 1) is not on curve")
	}
}

func TestG2BatchAffine(t *testing.T) {
	n := 20
	g := NewG2()
	points0 := make([]*PointG2, n)
	points1 := make([]*PointG2, n)
	for i := 0; i < n; i++ {
		points0[i] = g.rand()
		points1[i] = g.New().Set(points0[i])
		if g.IsAffine(points0[i]) {
			t.Fatal("expect non affine point")
		}
	}
	g.AffineBatch(points0)
	for i := 0; i < n; i++ {
		if !g.Equal(points0[i], points1[i]) {
			t.Fatal("batch affine failed")
		}
	}
}

func TestG2AdditiveProperties(t *testing.T) {
	g := NewG2()
	t0, t1 := g.New(), g.New()
	zero := g.Zero()
	for i := 0; i < fuz; i++ {
		a, b := g.rand(), g.rand()
		_, _, _ = b, t1, zero
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
			t.Fatal("a - b == - (b - a)")
		}
		c := g.rand()
		g.Add(t0, a, b)
		g.Add(t0, t0, c)
		g.Add(t1, a, c)
		g.Add(t1, t1, b)
		if !g.Equal(t0, t1) {
			t.Fatal("(a + b) + c == (a + c) + b")
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

func TestG2MixedAdd(t *testing.T) {
	g := NewG2()

	t0, a := g.New(), g.rand()
	zero := g.Zero()

	g.addMixed(t0, a, zero)
	if !g.Equal(t0, a) {
		t.Fatal("a + 0 == a")
	}
	g.addMixed(a, t0, zero)
	if !g.Equal(t0, a) {
		t.Fatal("a + 0 == a")
	}
	g.Add(t0, zero, zero)
	if !g.Equal(t0, zero) {
		t.Fatal("0 + 0 == 0")
	}

	for i := 0; i < fuz; i++ {
		a, b := g.rand(), g.rand()
		if g.IsAffine(a) || g.IsAffine(b) {
			t.Fatal("expect non affine points")
		}
		bAffine := g.New().Set(b)
		g.Affine(bAffine)
		r0, r1 := g.New(), g.New()
		g.Add(r0, a, b)
		g.addMixed(r1, a, bAffine)
		if !g.Equal(r0, r1) {
			t.Fatal("mixed addition failed")
		}
		aAffine := g.New().Set(a)
		g.Affine(aAffine)
		g.addMixed(r0, a, aAffine)
		g.Double(r1, a)
		if !g.Equal(r0, r1) {
			t.Fatal("mixed addition must double where points are equal")
		}
	}
}

func TestG2MultiplicativeProperties(t *testing.T) {
	g := NewG2()
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

func TestWNAFMulAgainstNaive(t *testing.T) {
	g2 := NewG2()
	for i := 0; i < fuz; i++ {
		a := g2.rand()
		c0, c1 := g2.new(), g2.new()
		e := randScalar(g2.Q())
		g2.MulScalar(c0, a, e)
		g2.wnafMul(c1, a, e)
		if !g2.Equal(c0, c1) {
			t.Fatal("wnaf against naive failed")
		}
	}
}

func TestG2MultiplicativePropertiesWNAF(t *testing.T) {
	g := NewG2()
	t0, t1 := g.new(), g.new()
	zero := g.Zero()
	for i := 0; i < fuz; i++ {
		a := g.rand()
		s1, s2, s3 := randScalar(q), randScalar(q), randScalar(q)
		sone := big.NewInt(1)
		g.wnafMul(t0, zero, s1)
		if !g.Equal(t0, zero) {
			t.Fatalf(" 0 ^ s == 0")
		}
		g.wnafMul(t0, a, sone)
		if !g.Equal(t0, a) {
			t.Fatalf(" a ^ 1 == a")
		}
		g.wnafMul(t0, zero, s1)
		if !g.Equal(t0, zero) {
			t.Fatalf(" 0 ^ s == a")
		}
		g.wnafMul(t0, a, s1)
		g.wnafMul(t0, t0, s2)
		s3.Mul(s1, s2)
		g.wnafMul(t1, a, s3)
		if !g.Equal(t0, t1) {
			t.Errorf(" (a ^ s1) ^ s2 == a ^ (s1 * s2)")
		}
		g.wnafMul(t0, a, s1)
		g.wnafMul(t1, a, s2)
		g.Add(t0, t0, t1)
		s3.Add(s1, s2)
		g.wnafMul(t1, a, s3)
		if !g.Equal(t0, t1) {
			t.Errorf(" (a ^ s1) + (a ^ s2) == a ^ (s1 + s2)")
		}
	}
}

func TestG2MultiExpExpected(t *testing.T) {
	g := NewG2()
	one := g.one()
	var scalars [2]*big.Int
	var bases [2]*PointG2
	scalars[0] = big.NewInt(2)
	scalars[1] = big.NewInt(3)
	bases[0], bases[1] = new(PointG2).Set(one), new(PointG2).Set(one)
	expected, result := g.New(), g.New()
	g.MulScalar(expected, one, big.NewInt(5))
	_, _ = g.MultiExp(result, bases[:], scalars[:])
	if !g.Equal(expected, result) {
		t.Fatal("bad multi-exponentiation")
	}
}

func TestG2MultiExp(t *testing.T) {
	g := NewG2()
	for n := 1; n < 1024+1; n = n * 2 {
		bases := make([]*PointG2, n)
		scalars := make([]*big.Int, n)
		var err error
		for i := 0; i < n; i++ {
			scalars[i], err = rand.Int(rand.Reader, q)
			if err != nil {
				t.Fatal(err)
			}
			bases[i] = g.rand()
		}
		expected, tmp := g.New(), g.New()
		for i := 0; i < n; i++ {
			g.MulScalar(tmp, bases[i], scalars[i])
			g.Add(expected, expected, tmp)
		}
		result := g.New()
		_, _ = g.MultiExp(result, bases, scalars)
		if !g.Equal(expected, result) {
			t.Fatal("multi-exponentiation failed")
		}
	}
}
func TestClearCofactor(t *testing.T) {
	g2 := NewG2()
	for i := 0; i < fuz; i++ {
		a := g2.rand()
		g2.ClearCofactor(a)
		if !g2.InCorrectSubgroup(a) {
			t.Fatal("clear cofactor failed")
		}
	}
}

func BenchmarkG2Add(t *testing.B) {
	g2 := NewG2()
	a, b, c := g2.rand(), g2.rand(), PointG2{}
	t.ResetTimer()
	for i := 0; i < t.N; i++ {
		g2.Add(&c, a, b)
	}
}

func BenchmarkG2AddMixed(t *testing.B) {
	g2 := NewG2()
	a, b, c := g2.rand(), g2.rand(), PointG2{}
	t.ResetTimer()
	for i := 0; i < t.N; i++ {
		g2.addMixed(&c, a, b)
	}
}

func BenchmarkG2Mul(t *testing.B) {
	g2 := NewG2()
	a, e, c := g2.rand(), q, PointG2{}
	t.ResetTimer()
	for i := 0; i < t.N; i++ {
		g2.MulScalar(&c, a, e)
	}
}

func BenchmarkG2MultiExp(t *testing.B) {
	g := NewG2()
	v := func(n int) ([]*PointG2, []*big.Int) {
		bases := make([]*PointG2, n)
		scalars := make([]*big.Int, n)
		var err error
		for i := 0; i < n; i++ {
			scalars[i], err = rand.Int(rand.Reader, q)
			if err != nil {
				t.Fatal(err)
			}
			bases[i] = g.rand()
		}
		return bases, scalars
	}
	for _, i := range []int{2, 10, 100, 1000} {
		t.Run(fmt.Sprint(i), func(t *testing.B) {
			bases, scalars := v(i)
			result := g.New()
			t.ResetTimer()
			for i := 0; i < t.N; i++ {
				_, _ = g.MultiExp(result, bases, scalars)
			}
		})
	}
}

func BenchmarkG2ClearCofactor(t *testing.B) {
	g2 := NewG2()
	a := g2.rand()
	t.ResetTimer()
	for i := 0; i < t.N; i++ {
		g2.ClearCofactor(a)
	}
}
