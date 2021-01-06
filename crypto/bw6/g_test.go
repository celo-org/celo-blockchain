package bw6

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"testing"
)

func (g *G) _rand(b *fe) *Point {
	p := &Point{}
	z, _ := new(fe).rand(rand.Reader)
	z6, bz6 := new(fe), new(fe)
	square(z6, z)
	square(z6, z6)
	mul(z6, z6, z)
	mul(z6, z6, z)
	mul(bz6, z6, b)
	for {
		x, _ := new(fe).rand(rand.Reader)
		y := new(fe)
		square(y, x)
		mul(y, y, x)
		add(y, y, bz6)
		if sqrt(y, y) {
			p.Set(&Point{*x, *y, *z})
			break
		}
	}
	if g.InCorrectSubgroup(p) {
		panic("rand point must be out of correct subgroup")
	}
	return p
}

func (g *G) randG1() *Point {
	p := g._rand(b)

	if !g.IsOnG1Curve(p) {
		panic("rand point must be on curve")
	}

	return p
}

func (g *G) randG2() *Point {
	p := g._rand(b2)
	if !g.IsOnG2Curve(p) {
		panic("rand point must be on curve")
	}
	return p
}

func (g *G) randG1Correct() *Point {
	p := g.ClearG1Cofactor(g.randG1())
	if !g.InCorrectSubgroup(p) {
		panic("must be in correct subgroup")
	}
	return p
}

func (g *G) randG2Correct() *Point {
	p := g.ClearG2Cofactor(g.randG2())
	if !g.InCorrectSubgroup(p) {
		panic("must be in correct subgroup")
	}
	return p
}

func (g *G) randG1Affine() *Point {
	return g.Affine(g.randG1Correct())
}

func (g *G) randG2Affine() *Point {
	return g.Affine(g.randG2Correct())
}

func (g *G) new() *Point {
	return g.Zero()
}

func TestGroupSerialization(t *testing.T) {
	var err error
	g := NewG()
	zero := g.Zero()
	b0 := g.ToBytes(zero)
	p0, err := g.G1FromBytes(b0)
	if err != nil {
		t.Fatal(err)
	}
	if !g.IsZero(p0) {
		t.Fatal("infinity serialization failed")
	}
	p0, err = g.G2FromBytes(b0)
	if err != nil {
		t.Fatal(err)
	}
	if !g.IsZero(p0) {
		t.Fatal("infinity serialization failed")
	}
	for i := 0; i < fuz; i++ {
		a1 := g.randG1()
		b0 := g.ToBytes(a1)
		b1, err := g.G1FromBytes(b0)
		if err != nil {
			t.Fatal(err)
		}
		if !g.Equal(a1, b1) {
			t.Fatal("serialization failed")
		}
	}
	for i := 0; i < fuz; i++ {
		a1 := g.randG2()
		b0 := g.ToBytes(a1)
		b1, err := g.G2FromBytes(b0)
		if err != nil {
			t.Fatal(err)
		}
		if !g.Equal(a1, b1) {
			t.Fatal("serialization failed")
		}
	}
}

func TestGroupIsOnCurve(t *testing.T) {
	g := NewG()
	zero := g.Zero()
	if !g.IsOnG1Curve(zero) {
		t.Fatal("zero must be on g1 curve")
	}
	if !g.IsOnG2Curve(zero) {
		t.Fatal("zero must be on g2 curve")
	}
	one := new(fe).one()
	p := &Point{*one, *one, *one}
	if g.IsOnG1Curve(p) {
		t.Fatal("(1, 1) is not on curve")
	}
	if g.IsOnG2Curve(p) {
		t.Fatal("(1, 1) is not on curve")
	}
}

func TestGroupBatchAffine(t *testing.T) {
	n := 20
	g := NewG()
	points0 := make([]*Point, n)
	points1 := make([]*Point, n)
	for i := 0; i < n; i++ {
		points0[i] = g.randG1()
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

func TestGroupAdditiveProperties(t *testing.T) {
	g := NewG()
	t0, t1 := g.New(), g.New()
	zero := g.Zero()
	for i := 0; i < fuz; i++ {
		a, b := g.randG1(), g.randG1()
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
		if !g.Equal(t0, a) {
			t.Fatal("(2 * a) - a == a")
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
		c := g.randG1()
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

func TestG1MixedAdd(t *testing.T) {
	g := NewG()

	t0, a := g.New(), g.randG1()
	zero := g.Zero()

	g.AddMixed(t0, a, zero)
	if !g.Equal(t0, a) {
		t.Fatal("a + 0 == a")
	}
	g.AddMixed(a, t0, zero)
	if !g.Equal(t0, a) {
		t.Fatal("a + 0 == a")
	}
	g.Add(t0, zero, zero)
	if !g.Equal(t0, zero) {
		t.Fatal("0 + 0 == 0")
	}

	for i := 0; i < fuz; i++ {
		a, b := g.randG1(), g.randG1()
		if g.IsAffine(a) || g.IsAffine(b) {
			t.Fatal("expect non affine points")
		}
		bAffine := g.New().Set(b)
		g.Affine(bAffine)
		r0, r1 := g.New(), g.New()
		g.Add(r0, a, b)
		g.AddMixed(r1, a, bAffine)
		if !g.Equal(r0, r1) {
			t.Fatal("mixed addition failed")
		}
		aAffine := g.New().Set(a)
		g.Affine(aAffine)
		g.AddMixed(r0, a, aAffine)
		g.Double(r1, a)
		if !g.Equal(r0, r1) {
			t.Fatal("mixed addition must double where points are equal")
		}
	}
}

func TestG1MultiplicativeProperties(t *testing.T) {
	g := NewG()
	t0, t1 := g.New(), g.New()
	zero := g.Zero()
	for i := 0; i < fuz; i++ {
		a := g.randG1Affine()
		s1, s2, s3 := randScalar(q), randScalar(q), randScalar(q)
		sone := big.NewInt(1)
		g.MulScalarG1(t0, zero, s1)
		if !g.Equal(t0, zero) {
			t.Fatal("0 ^ s == 0")
		}
		g.MulScalarG1(t0, a, sone)
		if !g.Equal(t0, a) {
			t.Fatal("a ^ 1 == a")
		}
		g.MulScalarG1(t0, zero, s1)
		if !g.Equal(t0, zero) {
			t.Fatal("0 ^ s == a")
		}
		g.MulScalarG1(t0, a, s1)
		g.MulScalarG1(t0, t0, s2)
		s3.Mul(s1, s2)
		g.MulScalarG1(t1, a, s3)
		if !g.Equal(t0, t1) {
			t.Fatal("(a ^ s1) ^ s2 == a ^ (s1 * s2)")
		}
		g.MulScalarG1(t0, a, s1)
		g.MulScalarG1(t1, a, s2)
		g.Add(t0, t0, t1)
		s3.Add(s1, s2)
		g.MulScalarG1(t1, a, s3)
		if !g.Equal(t0, t1) {
			t.Fatal("(a ^ s1) + (a ^ s2) == a ^ (s1 + s2)")
		}
	}
}

func TestG2MultiplicativeProperties(t *testing.T) {
	g := NewG()
	t0, t1 := g.New(), g.New()
	zero := g.Zero()
	for i := 0; i < fuz; i++ {
		a := g.randG2Affine()
		s1, s2, s3 := randScalar(q), randScalar(q), randScalar(q)
		sone := big.NewInt(1)
		g.MulScalarG2(t0, zero, s1)
		if !g.Equal(t0, zero) {
			t.Fatal("0 ^ s == 0")
		}
		g.MulScalarG2(t0, a, sone)
		if !g.Equal(t0, a) {
			t.Fatal("a ^ 1 == a")
		}
		g.MulScalarG2(t0, zero, s1)
		if !g.Equal(t0, zero) {
			t.Fatal("0 ^ s == a")
		}
		g.MulScalarG2(t0, a, s1)
		g.MulScalarG2(t0, t0, s2)
		s3.Mul(s1, s2)
		g.MulScalarG2(t1, a, s3)
		if !g.Equal(t0, t1) {
			t.Fatal("(a ^ s1) ^ s2 == a ^ (s1 * s2)")
		}
		g.MulScalarG2(t0, a, s1)
		g.MulScalarG2(t1, a, s2)
		g.Add(t0, t0, t1)
		s3.Add(s1, s2)
		g.MulScalarG2(t1, a, s3)
		if !g.Equal(t0, t1) {
			t.Fatal("(a ^ s1) + (a ^ s2) == a ^ (s1 + s2)")
		}
	}
}

func TestGroupMultiplicationCross(t *testing.T) {
	g := NewG()
	for i := 0; i < fuz; i++ {
		a := g.randG1Correct()
		s := randScalar(q)
		res0, res1, res2, res3 := g.New(), g.New(), g.New(), g.New()

		g.mulScalar(res0, a, s)
		g.wnafMul(res1, a, s)
		g.glvMulG1(res2, a, s)
		_, _ = g.MultiExp(res3, []*Point{a}, []*big.Int{s})

		if !g.Equal(res0, res1) {
			t.Fatal("cross multiplication failed (wnaf)", i)
		}
		if !g.Equal(res0, res2) {
			t.Fatal("cross multiplication failed (glv)", i)
		}
		if !g.Equal(res0, res3) {
			t.Fatal("cross multiplication failed (multiexp)", i)
		}
	}
	for i := 0; i < fuz; i++ {
		a := g.randG2Correct()
		s := randScalar(q)
		res0, res1, res2, res3 := g.New(), g.New(), g.New(), g.New()

		g.mulScalar(res0, a, s)
		g.wnafMul(res1, a, s)
		g.glvMulG2(res2, a, s)
		_, _ = g.MultiExp(res3, []*Point{a}, []*big.Int{s})

		if !g.Equal(res0, res1) {
			t.Fatal("cross multiplication failed (wnaf)", i)
		}
		if !g.Equal(res0, res2) {
			t.Fatal("cross multiplication failed (glv)", i)
		}
		if !g.Equal(res0, res3) {
			t.Fatal("cross multiplication failed (multiexp)", i)
		}
	}
}

func TestGroupMultiExpExpected(t *testing.T) {
	g := NewG()
	one := g.New().Set(&g1One)
	var scalars [2]*big.Int
	var bases [2]*Point
	scalars[0] = big.NewInt(2)
	scalars[1] = big.NewInt(3)
	bases[0], bases[1] = new(Point).Set(one), new(Point).Set(one)
	expected, result := g.New(), g.New()
	g.mulScalar(expected, one, big.NewInt(5))
	_, _ = g.MultiExp(result, bases[:], scalars[:])
	if !g.Equal(expected, result) {
		t.Fatal("multi-exponentiation failed")
	}
}

func TestGroupMultiExp(t *testing.T) {
	g := NewG()
	for n := 1; n < 1024+1; n = n * 2 {
		bases := make([]*Point, n)
		scalars := make([]*big.Int, n)
		var err error
		for i := 0; i < n; i++ {
			scalars[i], err = rand.Int(rand.Reader, q)
			if err != nil {
				t.Fatal(err)
			}
			bases[i] = g.randG1Affine()
		}
		expected, tmp := g.New(), g.New()
		for i := 0; i < n; i++ {
			g.mulScalar(tmp, bases[i], scalars[i])
			g.Add(expected, expected, tmp)
		}
		result := g.New()
		_, _ = g.MultiExp(result, bases, scalars)
		if !g.Equal(expected, result) {
			t.Fatal("multi-exponentiation failed")
		}
	}
}

func TestGroupClearCofactor(t *testing.T) {
	g := NewG()
	for i := 0; i < fuz; i++ {
		a := g.randG1()
		if g.InCorrectSubgroup(a) {
			t.Fatal("near 0 probablity that this would occur")
		}
		g.ClearG1Cofactor(a)
		if !g.InCorrectSubgroup(a) {
			t.Fatal("cofactor is not cleared")
		}
	}
	for i := 0; i < fuz; i++ {
		a := g.randG2()
		if g.InCorrectSubgroup(a) {
			t.Fatal("near 0 probablity that this would occur")
		}
		g.ClearG2Cofactor(a)
		if !g.InCorrectSubgroup(a) {
			t.Fatal("cofactor is not cleared")
		}
	}
}

func BenchmarkGroupAdd(t *testing.B) {
	g := NewG()
	a, b, c := g.randG1(), g.randG1(), Point{}
	t.ResetTimer()
	for i := 0; i < t.N; i++ {
		g.Add(&c, a, b)
	}
}

func BenchmarkGroupMulWNAF(t *testing.B) {
	g := NewG()
	p := new(Point).Set(&g1One)
	s := randScalar(q)
	res := new(Point)
	t.Run("Naive", func(t *testing.B) {
		t.ResetTimer()
		for i := 0; i < t.N; i++ {
			g.mulScalar(res, p, s)
		}
	})
	for i := 1; i < 8; i++ {
		wnafMulWindow = uint(i)
		t.Run(fmt.Sprintf("window: %d", i), func(t *testing.B) {
			t.ResetTimer()
			for i := 0; i < t.N; i++ {
				g.wnafMul(res, p, s)
			}
		})
	}
}

func BenchmarkG1MulGLV(t *testing.B) {
	g := NewG()
	p := new(Point).Set(&g1One)
	s := randScalar(q)
	res := new(Point)
	t.Run("Naive", func(t *testing.B) {
		t.ResetTimer()
		for i := 0; i < t.N; i++ {
			g.mulScalar(res, p, s)
		}
	})
	for i := 1; i < 8; i++ {
		glvMulWindow = uint(i)
		t.Run(fmt.Sprintf("window: %d", i), func(t *testing.B) {
			t.ResetTimer()
			for i := 0; i < t.N; i++ {
				g.glvMulG1(res, p, s)
			}
		})
	}
}

func BenchmarkG1MultiExp(t *testing.B) {
	g := NewG()
	v := func(n int) ([]*Point, []*big.Int) {
		bases := make([]*Point, n)
		scalars := make([]*big.Int, n)
		var err error
		for i := 0; i < n; i++ {
			scalars[i] = randScalar(q)
			if err != nil {
				t.Fatal(err)
			}
			bases[i] = g.randG1Affine()
		}
		return bases, scalars
	}
	for _, i := range []int{1, 2, 10, 100, 1000} {
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

func BenchmarkGroupClearCofactor(t *testing.B) {
	g := NewG()
	t.Run("G1", func(t *testing.B) {
		a := g.randG1()
		t.ResetTimer()
		for i := 0; i < t.N; i++ {
			g.ClearG1Cofactor(a)
		}
	})
	t.Run("G2", func(t *testing.B) {
		a := g.randG2()
		t.ResetTimer()
		for i := 0; i < t.N; i++ {
			g.ClearG2Cofactor(a)
		}
	})
}

func BenchmarkGroupSubgroupCheck(t *testing.B) {
	g := NewG()
	a := g.randG1()
	t.ResetTimer()
	for i := 0; i < t.N; i++ {
		g.InCorrectSubgroup(a)
	}
}
