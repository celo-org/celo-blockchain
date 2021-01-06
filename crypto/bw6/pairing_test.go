package bw6

import (
	"math/big"
	"testing"
)

func TestPairingExpected(t *testing.T) {
	bw6 := NewEngine()
	GT := bw6.GT()
	expected, err := GT.FromBytes(
		fromHex(
			fpByteSize,
			"0x00BE64FE0B5406B66F0A022E3580AC6D06CE4120E47DE81EFF70E9C0CF73CF2E4931D5DDA2805079C6383C7D696D6D8B3952B8F1E9EC995B7D6147FC1EE97641CCC27644CD905282B0A87F554A61F4457D29FD1163DD39E019E89F7A1B09D2AB",
			"0x0071056F4F5862343DE7A18942A1F6A4B24257BC60D3820437E33C374943153FFC29B1A9812B6F27A704826CEF5E9D8D6412A33443C490464CDB57319EB2A393592EB80140D9F39C68E20EF8138B1375A4EAEE503B1077E0BB7612FF3BA19FDF",
			"0x00A7DEBDD2D2B712D9EF7DBD6B8840B9B6ECCE5DE3C631FB14676C849F1D839BCE995A18AF25E0E154C0F5854D81B6ABDC7C18E5E4380111EE5F95A51B1CC08084C6BADF64B80431029911ED13D165A92C5D3A60C4B6B701DB09E5AD8713CF33",
			"0x009C94404EFD09DAC985C2B82ECDD08374E82B3BEFD767C997520C277EB6C56DCEDB059CB831C2B393374D95A0438D84FDE4259309349EF86BCD55EC422F3618BB539378E407B89779D3DBF7B7E412C5EFE04E220C10A2790F1A58263F699689",
			"0x009263C36801AC6C5626F28D85867F80AB684E3A3E5C5E0E8BA32B876728E8ADCFCB556BA7A2D661F849E985D4FB15909D29C80BE33760C872D6EE16117AE127F39DB7BEC503E3028ABBEC925AE37E40F988E2CCA427AF28C365B51479E83C90",
			"0x00477D78FFA08531DF538752849578C78F2C66458DB8A28C27CE7802B03456880B844A03A571DB64B8988BA8B50D6597D561EE93A71D771A529CC56AFDA5C0CDB3C756CD5279D53C3F08E2550F98EE122936E8B6597F9F81E839D01F39CAA971",
		),
	)
	if err != nil {
		t.Fatal(err)
	}
	r := bw6.AddPair(bw6.g.G1One(), bw6.g.G2One()).Result()
	if !r.Equal(expected) {
		t.Fatalf("expected pairing failed")
	}
	if !GT.IsValid(r) {
		t.Fatal("element is not in correct subgroup")
	}
}

func TestPairingNonDegeneracy(t *testing.T) {
	bw6 := NewEngine()
	g1Zero, g2Zero, g1One, g2One := bw6.g.Zero(), bw6.g.Zero(), bw6.g.G1One(), bw6.g.G2One()
	GT := bw6.GT()
	// e(g1^a, g2^b) != 1
	bw6.Reset()
	{
		bw6.AddPair(g1One, g2One)
		e := bw6.Result()
		if e.IsOne() {
			t.Fatal("pairing result is not expected to be one")
		}
		if !GT.IsValid(e) {
			t.Fatal("pairing result is not valid")
		}
	}
	// e(g1^a, 0) == 1
	bw6.Reset()
	{
		bw6.AddPair(g1One, g2Zero)
		e := bw6.Result()
		if !e.IsOne() {
			t.Fatal("pairing result is expected to be one")
		}
	}
	// e(0, g2^b) == 1
	bw6.Reset()
	{
		bw6.AddPair(g1Zero, g2One)
		e := bw6.Result()
		if !e.IsOne() {
			t.Fatal("pairing result is expected to be one")
		}
	}
	//
	bw6.Reset()
	{
		bw6.AddPair(g1Zero, g2One)
		bw6.AddPair(g1One, g2Zero)
		bw6.AddPair(g1Zero, g2Zero)
		e := bw6.Result()
		if !e.IsOne() {
			t.Fatal("pairing result is expected to be one")
		}
	}
}

func TestPairingBilinearity(t *testing.T) {
	bw6 := NewEngine()

	gt := bw6.GT()
	// e(a*G1, b*G2) = e(G1, G2)^c
	{
		a, b := big.NewInt(17), big.NewInt(117)
		c := new(big.Int).Mul(a, b)
		G1, G2 := bw6.g.G1One(), bw6.g.G2One()
		e0 := bw6.AddPair(G1, G2).Result()
		P1, P2 := bw6.g.new(), bw6.g.new()
		bw6.g.MulScalarG1(P1, G1, a)
		bw6.g.MulScalarG2(P2, G2, b)
		e1 := bw6.AddPair(P1, P2).Result()
		gt.Exp(e0, e0, c)
		if !e0.Equal(e1) {
			t.Fatal("pairing failed")
		}
	}
	// e(a * G1, b * G2) = e((a * b) * G1, G2)
	{
		// scalars
		a, b := big.NewInt(17), big.NewInt(117)
		c := new(big.Int).Mul(a, b)
		// LHS
		G1, G2 := bw6.g.G1One(), bw6.g.G2One()
		bw6.g.MulScalarG1(G1, G1, c)
		bw6.AddPair(G1, G2)
		// RHS
		P1, P2 := bw6.g.G1One(), bw6.g.G2One()
		bw6.g.MulScalarG1(P1, P1, a)
		bw6.g.MulScalarG2(P2, P2, b)
		bw6.AddPairInv(P1, P2)
		// should be one
		if !bw6.Check() {
			t.Fatal("pairing failed")
		}
	}
}

func TestPairingMulti(t *testing.T) {
	// e(G1, G2) ^ t == e(a01 * G1, a02 * G2) * e(a11 * G1, a12 * G2) * ... * e(an1 * G1, an2 * G2)
	// where t = sum(ai1 * ai2)
	bw6 := NewEngine()
	numOfPair := 100
	targetExp := new(big.Int)
	// RHS
	for i := 0; i < numOfPair; i++ {
		// (ai1 * G1, ai2 * G2)
		a1, a2 := randScalar(q), randScalar(q)
		P1, P2 := bw6.g.G1One(), bw6.g.G2One()
		bw6.g.MulScalarG1(P1, P1, a1)
		bw6.g.MulScalarG2(P2, P2, a2)
		bw6.AddPair(P1, P2)
		// accumulate targetExp
		// t += (ai1 * ai2)
		a1.Mul(a1, a2)
		targetExp.Add(targetExp, a1)
	}
	// LHS
	// e(t * G1, G2)
	T1, T2 := bw6.g.G1One(), bw6.g.G2One()
	bw6.g.MulScalarG1(T1, T1, targetExp)
	bw6.AddPairInv(T1, T2)
	if !bw6.Check() {
		t.Fatal("fail multi pairing")
	}
}

func TestPairingEmpty(t *testing.T) {
	bw6 := NewEngine()
	if !bw6.Check() {
		t.Fatal("empty check should be accepted")
	}
	if !bw6.Result().IsOne() {
		t.Fatal("empty pairing result should be one")
	}
}

func BenchmarkPairing(t *testing.B) {
	t.ResetTimer()
	for i := 0; i < t.N; i++ {
		bw6 := NewEngine()
		bw6.AddPair(bw6.g.G1One(), bw6.g.G2One())
		bw6.calculate()
	}
}
