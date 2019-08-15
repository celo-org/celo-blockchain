use std::ops::{Div, Mul, Neg};

use algebra::{
    biginteger::BigInteger,
    curves::{
        models::{
            bls12::{Bls12Parameters, G2Affine, G2Projective},
            ModelParameters,
        },
        AffineCurve, ProjectiveCurve,
    },
    fields::{fp12_2over3over2::Fp12, fp6_3over2::Fp6, BitIterator, Field, Fp2, PrimeField},
};

/// The sextic twist is constructed the quadratic and cubic non-residue, called w in Fp12.
/// Fp12 is constructed as 2over3over2, with u = w^2 and v = w.
fn twist_omega<P: Bls12Parameters>() -> (Fp12<P::Fp12Params>, Fp12<P::Fp12Params>) {
    let omega2 = Fp12::<P::Fp12Params>::new(
        Fp6::<P::Fp6Params>::new(
            Fp2::<P::Fp2Params>::zero(),
            Fp2::<P::Fp2Params>::one(),
            Fp2::<P::Fp2Params>::zero(),
        ),
        Fp6::<P::Fp6Params>::zero(),
    ); //w^2 = u

    let omega3 = Fp12::<P::Fp12Params>::new(
        Fp6::<P::Fp6Params>::zero(),
        Fp6::<P::Fp6Params>::new(
            Fp2::<P::Fp2Params>::zero(),
            Fp2::<P::Fp2Params>::one(),
            Fp2::<P::Fp2Params>::zero(),
        ),
    ); //w^3 = u*v

    (omega2, omega3)
}

/// Move a point from the Fp2 twisted curve to the Fp12 curve. See page 63 in
/// http://www.craigcostello.com.au/pairings/PairingsForBeginners.pdf.
pub fn untwist<P: Bls12Parameters>(
    x: &Fp2<P::Fp2Params>,
    y: &Fp2<P::Fp2Params>,
) -> (Fp12<P::Fp12Params>, Fp12<P::Fp12Params>) {
    let (omega2, omega3) = twist_omega::<P>();
    let new_x = Fp12::<P::Fp12Params>::new(
        Fp6::<P::Fp6Params>::new(
            x.clone(),
            Fp2::<P::Fp2Params>::zero(),
            Fp2::<P::Fp2Params>::zero(),
        ),
        Fp6::<P::Fp6Params>::zero(),
    ) * &omega2;

    let new_y = Fp12::<P::Fp12Params>::new(
        Fp6::<P::Fp6Params>::new(
            y.clone(),
            Fp2::<P::Fp2Params>::zero(),
            Fp2::<P::Fp2Params>::zero(),
        ),
        Fp6::<P::Fp6Params>::zero(),
    ) * &omega3;

    (new_x, new_y)
}

/// Move a point from the Fp12 curve to the Fp2 twisted curve. See page 63 in
/// http://www.craigcostello.com.au/pairings/PairingsForBeginners.pdf.
pub fn twist<P: Bls12Parameters>(
    x: &Fp12<P::Fp12Params>,
    y: &Fp12<P::Fp12Params>,
) -> (Fp2<P::Fp2Params>, Fp2<P::Fp2Params>) {
    let (omega2, omega3) = twist_omega::<P>();

    let omega2x = x.div(&omega2);
    let omega3y = y.div(&omega3);
    (omega2x.c0.c0, omega3y.c0.c0)
}

/// An endormophism useful for fast cofactor multiplication. See page 4 in
/// https://eprint.iacr.org/2017/419.pdf.
pub fn psi<P: Bls12Parameters>(p: &G2Projective<P>, power: usize) -> G2Projective<P> {
    let p = p.into_affine();
    let (mut untwisted_x, mut untwisted_y) = untwist::<P>(&p.x, &p.y);
    untwisted_x.frobenius_map(power);
    untwisted_y.frobenius_map(power);
    let (twisted_x, twisted_y) = twist::<P>(&untwisted_x, &untwisted_y);
    G2Affine::<P>::new(twisted_x, twisted_y, false).into_projective()
}

fn curve_x<P: Bls12Parameters>() -> <P::G2Parameters as ModelParameters>::ScalarField {
    let x_bits: Vec<bool> = BitIterator::new(P::X).collect();
    let x = <<P::G2Parameters as ModelParameters>::ScalarField as PrimeField>::BigInt::from_bits(
        &x_bits,
    );

    <P::G2Parameters as ModelParameters>::ScalarField::from_repr(x)
}

/// Scott et al. method for fast cofactor multiplication. See page 7 in
/// https://eprint.iacr.org/2017/419.pdf.
pub fn scale_by_cofactor_scott<P: Bls12Parameters>(p: &G2Projective<P>) -> G2Projective<P> {
    let x = curve_x::<P>();

    let one = <P::G2Parameters as ModelParameters>::ScalarField::one();
    let p1 = p.mul(&x); //x
    let p15 = p1 - &p; //x-1
    let p2 = p1.mul(&(x - &one)); //x^2-x
    let p3 = p2.neg() + &p15; //-x^2+2x-1
    let p4 = (p.neg() + &p2).mul(&x); //x^3-x^2-x
    let p5 = p4.clone() + &p; //x^3-x^2-x+1
    let p6 = p4 + &p.double().double(); //x^3-x^2-x+4

    p6 + &psi::<P>(&p5, 1) + &psi::<P>(&p3, 2)
}

/// Fuentes et al. method for fast cofactor multiplication. See page 10 in
/// https://eprint.iacr.org/2017/419.pdf.
pub fn scale_by_cofactor_fuentes<P: Bls12Parameters>(p: &G2Projective<P>) -> G2Projective<P> {
    let x = curve_x::<P>();

    let one = <P::G2Parameters as ModelParameters>::ScalarField::one();
    let p1 = p.mul(&x); //x
    let p2 = p1 - &p; //x-1
    let p3 = p2.mul(&(x + &one)); //x^2-1
    let p4 = p3 - &p1; //x^2-x-1
    let p5 = p.double(); //2

    p4 + &psi::<P>(&p2, 1) + &psi::<P>(&p5, 2)
}

#[cfg(test)]
mod test {
    use rand::{Rng, SeedableRng, XorShiftRng};
    use std::{ops::Mul, str::FromStr};

    use super::{curve_x, psi, scale_by_cofactor_fuentes, scale_by_cofactor_scott};

    use algebra::{
        biginteger::BigInteger,
        curves::{
            bls12_377::{Bls12_377Parameters, G2Projective},
            models::bls12::Bls12Parameters,
            ModelParameters, ProjectiveCurve,
        },
        fields::{bls12_377::Fr, FpParameters, PrimeField},
        BitIterator,
    };

    fn curve_r_modulus<P: Bls12Parameters>() -> <P::G2Parameters as ModelParameters>::ScalarField {
        let x_bits: Vec<bool> = BitIterator::new(
            <<P::G2Parameters as ModelParameters>::ScalarField as PrimeField>::Params::MODULUS,
        )
        .collect();
        let x =
            <<P::G2Parameters as ModelParameters>::ScalarField as PrimeField>::BigInt::from_bits(
                &x_bits,
            );

        <P::G2Parameters as ModelParameters>::ScalarField::from_repr(x)
    }

    #[test]
    fn test_twist_untwist() {
        let mut rng = XorShiftRng::from_seed([0x5dbe6259, 0x8d313d76, 0x3237db17, 0xe5bc0654]);

        let p: G2Projective = rng.gen();
        assert_eq!(psi::<Bls12_377Parameters>(&p, 0), p);
    }

    #[test]
    fn test_scale_by_cofactor_scott() {
        let mut rng = XorShiftRng::from_seed([0x5dbe6259, 0x8d313d76, 0x3237db17, 0xe5bc0654]);

        for _i in 0..5 {
            let p: G2Projective = rng.gen();
            let scott_cofactor = scale_by_cofactor_scott::<Bls12_377Parameters>(&p);

            let three = Fr::from_str("3").unwrap();
            let naive_cofactor = p.into_affine().scale_by_cofactor() * &three;
            assert_eq!(naive_cofactor, scott_cofactor);
            let modulus = curve_r_modulus::<Bls12_377Parameters>();
            assert!(scott_cofactor.mul(&modulus).is_zero());
        }
    }

    #[test]
    fn test_scale_by_cofactor_fuentes() {
        let mut rng = XorShiftRng::from_seed([0x5dbe6259, 0x8d313d76, 0x3237db17, 0xe5bc0654]);
        let x = curve_x::<Bls12_377Parameters>();

        for _i in 0..5 {
            let p: G2Projective = rng.gen();
            let fuentes_cofactor = scale_by_cofactor_fuentes::<Bls12_377Parameters>(&p);

            let three = Fr::from_str("3").unwrap();
            let px2 = p.mul(&x).mul(&x);
            let p = px2 - &p;
            let naive_cofactor = p.into_affine().scale_by_cofactor() * &three;
            assert_eq!(naive_cofactor, fuentes_cofactor);
            let modulus = curve_r_modulus::<Bls12_377Parameters>();
            assert!(fuentes_cofactor.mul(&modulus).is_zero());
        }
    }

}
