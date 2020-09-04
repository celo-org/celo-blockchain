package bls12377

import (
	"math/big"
	"testing"
)

func TestPairingExpected(t *testing.T) {
	bls := NewEngine()
	G1, G2 := bls.G1, bls.G2
	GT := bls.GT()
	expected, err := GT.FromBytes(
		fromHex(
			48,
			"0x00b718ff624a95f189bfb44bcd6d6556226837c1f74d1afbf4bea573b71c17d3a243cae41d966e2164aad0991fd790cc",
			"0x0197261459eb50c526a28ebbdbd4b5b33d4c55b759d8c926289c96e4ea032783da4f1994ed09ee68fd791367c8b54d87",
			"0x00756970de5e545d91121e151ce96c26ad820ebe4ffbc9dee234351401925eaa4193e377135ced4d3845057c0c39ecd6",
			"0x00373f07857759dbec3d57af8bfdc79d28f44db5103e523e28ea69c688af7c831e726417cb5123530fadb5540ac05763",
			"0x00ec2d5430932820eb74bd698a2d919cf7086335f235019815501b97fd833d90f07eb111885af785beb343ea1db8d4e7",
			"0x0051ae2dce91bcd2251abbaf8dfb67c7e5cf6d864c61f81a09aaeac3dfdcf6ae0b3168929ccc7d91abb8b4e13974b7db",
			"0x0095fcebb2a29b10d2f5283a40b147a82ea62114c9bae68e0d745c1afc70c6eeaf1b1c5bf6352d82931b6bdcbff8da47",
			"0x001fdad7541653e8ac2d735c24f472716122bb24a3e675c20ab2c23d7380c7a349d49dd0db11f95c08861744e3b19a8e",
			"0x00b3530a66bf5754b3e0b7b2c070a35c072bb613698c32db836cef1fcb77086125efd02528d4235f7d7b87e554174d82",
			"0x004064943ac5c2fc0ef854d8168c67f56adb2a5a16d900dba15be3ecb0172a9ecd96ebf6375d0262f5d43d0709dc8c5f",
			"0x0066910d06a91685179f1b448b9b198d5ed2eabc44d21580005e5f708a3c7858eb9b921691e40ba25804aced41190d34",
			"0x0008f3e3e451ff584f864ca1d53fc34562f2ebf3baa7c610d8a3b51a7fa9e8dfaac34399e40540e3bc57a73d11924c03",
		),
	)
	if err != nil {
		t.Fatal(err)
	}
	r := bls.AddPair(G1.One(), G2.One()).Result()
	if !r.Equal(expected) {
		t.Fatal("bad pairing")
	}
	if !GT.IsValid(r) {
		t.Fatal("element is not in correct subgroup")
	}
}

func TestPairingNonDegeneracy(t *testing.T) {
	bls := NewEngine()
	G1, G2 := bls.G1, bls.G2
	g1Zero, g2Zero, g1One, g2One := G1.Zero(), G2.Zero(), G1.One(), G2.One()
	GT := bls.GT()
	// e(g1^a, g2^b) != 1
	bls.Reset()
	{
		bls.AddPair(g1One, g2One)
		e := bls.Result()
		if e.IsOne() {
			t.Fatal("pairing result is not expected to be one")
		}
		if !GT.IsValid(e) {
			t.Fatal("pairing result is not valid")
		}
	}
	// e(g1^a, 0) == 1
	bls.Reset()
	{
		bls.AddPair(g1One, g2Zero)
		e := bls.Result()
		if !e.IsOne() {
			t.Fatal("pairing result is expected to be one")
		}
	}
	// e(0, g2^b) == 1
	bls.Reset()
	{
		bls.AddPair(g1Zero, g2One)
		e := bls.Result()
		if !e.IsOne() {
			t.Fatal("pairing result is expected to be one")
		}
	}
	//
	bls.Reset()
	{
		bls.AddPair(g1Zero, g2One)
		bls.AddPair(g1One, g2Zero)
		bls.AddPair(g1Zero, g2Zero)
		e := bls.Result()
		if !e.IsOne() {
			t.Fatal("pairing result is expected to be one")
		}
	}

	bls.Reset()
	{
		expected, err := GT.FromBytes(
			fromHex(
				48,
				"0x00b718ff624a95f189bfb44bcd6d6556226837c1f74d1afbf4bea573b71c17d3a243cae41d966e2164aad0991fd790cc",
				"0x0197261459eb50c526a28ebbdbd4b5b33d4c55b759d8c926289c96e4ea032783da4f1994ed09ee68fd791367c8b54d87",
				"0x00756970de5e545d91121e151ce96c26ad820ebe4ffbc9dee234351401925eaa4193e377135ced4d3845057c0c39ecd6",
				"0x00373f07857759dbec3d57af8bfdc79d28f44db5103e523e28ea69c688af7c831e726417cb5123530fadb5540ac05763",
				"0x00ec2d5430932820eb74bd698a2d919cf7086335f235019815501b97fd833d90f07eb111885af785beb343ea1db8d4e7",
				"0x0051ae2dce91bcd2251abbaf8dfb67c7e5cf6d864c61f81a09aaeac3dfdcf6ae0b3168929ccc7d91abb8b4e13974b7db",
				"0x0095fcebb2a29b10d2f5283a40b147a82ea62114c9bae68e0d745c1afc70c6eeaf1b1c5bf6352d82931b6bdcbff8da47",
				"0x001fdad7541653e8ac2d735c24f472716122bb24a3e675c20ab2c23d7380c7a349d49dd0db11f95c08861744e3b19a8e",
				"0x00b3530a66bf5754b3e0b7b2c070a35c072bb613698c32db836cef1fcb77086125efd02528d4235f7d7b87e554174d82",
				"0x004064943ac5c2fc0ef854d8168c67f56adb2a5a16d900dba15be3ecb0172a9ecd96ebf6375d0262f5d43d0709dc8c5f",
				"0x0066910d06a91685179f1b448b9b198d5ed2eabc44d21580005e5f708a3c7858eb9b921691e40ba25804aced41190d34",
				"0x0008f3e3e451ff584f864ca1d53fc34562f2ebf3baa7c610d8a3b51a7fa9e8dfaac34399e40540e3bc57a73d11924c03",
			),
		)
		if err != nil {
			t.Fatal(err)
		}
		bls.AddPair(g1Zero, g2One)
		bls.AddPair(g1One, g2Zero)
		bls.AddPair(g1Zero, g2Zero)
		bls.AddPair(g1One, g2One)
		e := bls.Result()
		if !e.Equal(expected) {
			t.Fatal("bad pairing")
		}
	}
}

func TestPairingBilinearity(t *testing.T) {
	bls := NewEngine()
	g1, g2 := bls.G1, bls.G2
	gt := bls.GT()
	// e(a*G1, b*G2) = e(G1, G2)^c
	{
		a, b := big.NewInt(17), big.NewInt(117)
		c := new(big.Int).Mul(a, b)
		G1, G2 := g1.One(), g2.One()
		e0 := bls.AddPair(G1, G2).Result()
		P1, P2 := g1.New(), g2.New()
		g1.MulScalar(P1, G1, a)
		g2.MulScalar(P2, G2, b)
		e1 := bls.AddPair(P1, P2).Result()
		gt.Exp(e0, e0, c)
		if !e0.Equal(e1) {
			t.Fatal("bad pairing, 1")
		}
	}
	// e(a * G1, b * G2) = e((a + b) * G1, G2)
	{
		// scalars
		a, b := big.NewInt(17), big.NewInt(117)
		c := new(big.Int).Mul(a, b)
		// LHS
		G1, G2 := g1.One(), g2.One()
		g1.MulScalar(G1, G1, c)
		bls.AddPair(G1, G2)
		// RHS
		P1, P2 := g1.One(), g2.One()
		g1.MulScalar(P1, P1, a)
		g2.MulScalar(P2, P2, b)
		bls.AddPairInv(P1, P2)
		// should be one
		if !bls.Check() {
			t.Fatal("bad pairing, 2")
		}
	}
	// e(a * G1, b * G2) = e((a + b) * G1, G2)
	{
		// scalars
		a, b := big.NewInt(17), big.NewInt(117)
		c := new(big.Int).Mul(a, b)
		// LHS
		G1, G2 := g1.One(), g2.One()
		g2.MulScalar(G2, G2, c)
		bls.AddPair(G1, G2)
		// RHS
		H1, H2 := g1.One(), g2.One()
		g1.MulScalar(H1, H1, a)
		g2.MulScalar(H2, H2, b)
		bls.AddPairInv(H1, H2)
		// should be one
		if !bls.Check() {
			t.Fatal("bad pairing, 3")
		}
	}
}

func TestPairingMulti(t *testing.T) {
	// e(G1, G2) ^ t == e(a01 * G1, a02 * G2) * e(a11 * G1, a12 * G2) * ... * e(an1 * G1, an2 * G2)
	// where t = sum(ai1 * ai2)
	bls := NewEngine()
	g1, g2 := bls.G1, bls.G2
	numOfPair := 100
	targetExp := new(big.Int)
	// RHS
	for i := 0; i < numOfPair; i++ {
		// (ai1 * G1, ai2 * G2)
		a1, a2 := randScalar(q), randScalar(q)
		P1, P2 := g1.One(), g2.One()
		g1.MulScalar(P1, P1, a1)
		g2.MulScalar(P2, P2, a2)
		bls.AddPair(P1, P2)
		// accumulate targetExp
		// t += (ai1 * ai2)
		a1.Mul(a1, a2)
		targetExp.Add(targetExp, a1)
	}
	// LHS
	// e(t * G1, G2)
	T1, T2 := g1.One(), g2.One()
	g1.MulScalar(T1, T1, targetExp)
	bls.AddPairInv(T1, T2)
	if !bls.Check() {
		t.Fatal("fail multi pairing")
	}
}

func TestPairingEmpty(t *testing.T) {
	bls := NewEngine()
	if !bls.Check() {
		t.Fatal("empty check should be accepted")
	}
	if !bls.Result().IsOne() {
		t.Fatal("empty pairing result should be one")
	}
}

func BenchmarkPairing(t *testing.B) {
	bls := NewEngine()
	g1, g2, gt := bls.G1, bls.G2, bls.GT()
	bls.AddPair(g1.One(), g2.One())
	e := gt.New()
	t.ResetTimer()
	for i := 0; i < t.N; i++ {
		e = bls.calculate()
	}
	_ = e
}
