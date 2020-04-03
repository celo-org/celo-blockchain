#![allow(clippy::op_ref)] // clippy throws a false positive around field ops

use algebra::{curves::bls12::Bls12Parameters, One, PrimeField};
use r1cs_core::SynthesisError;
use r1cs_std::{
    alloc::AllocGadget,
    bits::ToBitsGadget,
    boolean::Boolean,
    fields::{fp::FpGadget, FieldGadget},
    groups::curves::short_weierstrass::bls12::{G1Gadget, G2Gadget},
    Assignment,
};
use std::{marker::PhantomData, ops::Neg};

/// Enforces point compression on the provided element
pub struct YToBitGadget<P: Bls12Parameters> {
    parameters_type: PhantomData<P>,
}

impl<P: Bls12Parameters> YToBitGadget<P> {
    pub fn y_to_bit_g1<CS: r1cs_core::ConstraintSystem<P::Fp>>(
        mut cs: CS,
        pk: &G1Gadget<P>,
    ) -> Result<Boolean, SynthesisError> {
        let half_plus_one_neg =
            (P::Fp::from_repr(P::Fp::modulus_minus_one_div_two()) + &P::Fp::one()).neg();
        let y_bit = Boolean::alloc(cs.ns(|| "alloc y bit"), || {
            if pk.y.get_value().is_some() {
                let half = P::Fp::modulus_minus_one_div_two();
                Ok(pk.y.get_value().get()?.into_repr() > half)
            } else {
                Err(SynthesisError::AssignmentMissing)
            }
        })?;
        let y_adjusted = FpGadget::alloc(cs.ns(|| "alloc y"), || {
            if pk.y.get_value().is_some() {
                let half = P::Fp::modulus_minus_one_div_two();
                let y_value = pk.y.get_value().get()?;
                if y_value.into_repr() > half {
                    Ok(y_value - &(P::Fp::from_repr(half) + &P::Fp::one()))
                } else {
                    Ok(y_value)
                }
            } else {
                Err(SynthesisError::AssignmentMissing)
            }
        })?;
        let y_bit_lc = y_bit.lc(CS::one(), half_plus_one_neg);
        cs.enforce(
            || "check y bit",
            |lc| lc + (P::Fp::one(), CS::one()),
            |lc| pk.y.get_variable() + y_bit_lc + lc,
            |lc| y_adjusted.get_variable() + lc,
        );
        let y_adjusted_bits = &y_adjusted.to_bits(cs.ns(|| "y adjusted to bits"))?;
        Boolean::enforce_smaller_or_equal_than::<_, _, P::Fp, _>(
            cs.ns(|| "enforce smaller than modulus minus one div two"),
            y_adjusted_bits,
            P::Fp::modulus_minus_one_div_two(),
        )?;
        Ok(y_bit)
    }

    pub fn y_to_bit_g2<CS: r1cs_core::ConstraintSystem<P::Fp>>(
        mut cs: CS,
        pk: &G2Gadget<P>,
    ) -> Result<Boolean, SynthesisError> {
        let half_plus_one_neg =
            (P::Fp::from_repr(P::Fp::modulus_minus_one_div_two()) + &P::Fp::one()).neg();
        let y_c1_bit = Boolean::alloc(cs.ns(|| "alloc y c1 bit"), || {
            if pk.y.c1.get_value().is_some() {
                let half = P::Fp::modulus_minus_one_div_two();
                Ok(pk.y.c1.get_value().get()?.into_repr() > half)
            } else {
                Err(SynthesisError::AssignmentMissing)
            }
        })?;
        let y_c0_bit = Boolean::alloc(cs.ns(|| "alloc y c0 bit"), || {
            if pk.y.c0.get_value().is_some() {
                let half = P::Fp::modulus_minus_one_div_two();
                Ok(pk.y.c0.get_value().get()?.into_repr() > half)
            } else {
                Err(SynthesisError::AssignmentMissing)
            }
        })?;
        let y_eq_bit = Boolean::alloc(cs.ns(|| "alloc y eq bit"), || {
            if pk.y.c0.get_value().is_some() {
                let half = P::Fp::modulus_minus_one_div_two();
                Ok(pk.y.c0.get_value().get()?.into_repr() == half)
            } else {
                Err(SynthesisError::AssignmentMissing)
            }
        })?;
        let y_bit = Boolean::alloc(cs.ns(|| "alloc y bit"), || {
            if pk.y.c1.get_value().is_some() {
                let half = P::Fp::modulus_minus_one_div_two();
                let y_c1 = pk.y.c1.get_value().get()?.into_repr();
                let y_c0 = pk.y.c1.get_value().get()?.into_repr();
                Ok(y_c1 > half || (y_c1 == half && y_c0 > half))
            } else {
                Err(SynthesisError::AssignmentMissing)
            }
        })?;
        let y_c1_adjusted = FpGadget::alloc(cs.ns(|| "alloc y c1"), || {
            if pk.y.get_value().is_some() {
                let half = P::Fp::modulus_minus_one_div_two();
                let y_value = pk.y.c1.get_value().get()?;
                if y_value.into_repr() > half {
                    Ok(y_value - &(P::Fp::from_repr(half) + &P::Fp::one()))
                } else {
                    Ok(y_value)
                }
            } else {
                Err(SynthesisError::AssignmentMissing)
            }
        })?;
        let y_c0_adjusted = FpGadget::alloc(cs.ns(|| "alloc y c0"), || {
            if pk.y.get_value().is_some() {
                let half = P::Fp::modulus_minus_one_div_two();
                let y_value = pk.y.c0.get_value().get()?;
                if y_value.into_repr() > half {
                    Ok(y_value - &(P::Fp::from_repr(half) + &P::Fp::one()))
                } else {
                    Ok(y_value)
                }
            } else {
                Err(SynthesisError::AssignmentMissing)
            }
        })?;
        let y_c1_bit_lc = y_c1_bit.lc(CS::one(), half_plus_one_neg);
        cs.enforce(
            || "check y bit c1",
            |lc| lc + (P::Fp::one(), CS::one()),
            |lc| pk.y.c1.get_variable() + y_c1_bit_lc + lc,
            |lc| y_c1_adjusted.get_variable() + lc,
        );
        let y_c1_adjusted_bits = &y_c1_adjusted.to_bits(cs.ns(|| "y c1 adjusted to bits"))?;
        Boolean::enforce_smaller_or_equal_than::<_, _, P::Fp, _>(
            cs.ns(|| "enforce y c1 smaller than modulus minus one div two"),
            y_c1_adjusted_bits,
            P::Fp::modulus_minus_one_div_two(),
        )?;
        let y_c0_bit_lc = y_c0_bit.lc(CS::one(), half_plus_one_neg);
        cs.enforce(
            || "check y bit c0",
            |lc| lc + (P::Fp::one(), CS::one()),
            |lc| pk.y.c0.get_variable() + y_c0_bit_lc + lc,
            |lc| y_c0_adjusted.get_variable() + lc,
        );
        let y_c0_adjusted_bits = &y_c0_adjusted.to_bits(cs.ns(|| "y c0 adjusted to bits"))?;
        Boolean::enforce_smaller_or_equal_than::<_, _, P::Fp, _>(
            cs.ns(|| "enforce y c0 smaller than modulus minus one div two"),
            y_c0_adjusted_bits,
            P::Fp::modulus_minus_one_div_two(),
        )?;

        // (1-a)*(b*c) == o - a
        // a is c1
        // b is y_eq
        // c is c0

        let bc = Boolean::and(cs.ns(|| "and bc"), &y_eq_bit, &y_c0_bit)?;

        cs.enforce(
            || "enforce y bit derived correctly",
            |lc| lc + (P::Fp::one(), CS::one()) + y_c1_bit.lc(CS::one(), P::Fp::one().neg()),
            |_| bc.lc(CS::one(), P::Fp::one()),
            |lc| {
                lc + y_bit.lc(CS::one(), P::Fp::one()) + y_c1_bit.lc(CS::one(), P::Fp::one().neg())
            },
        );

        Ok(y_bit)
    }
}

#[cfg(test)]
mod test {
    use rand::SeedableRng;
    use rand_xorshift::XorShiftRng;

    use algebra::{
        bls12_377::{
            Fr as Bls12_377Fr, G1Projective as Bls12_377G1Projective,
            G2Projective as Bls12_377G2Projective, Parameters as Bls12_377Parameters,
        },
        curves::bls12::Bls12Parameters,
        sw6::Fr as SW6Fr,
        PrimeField, ProjectiveCurve, UniformRand,
    };
    use r1cs_core::ConstraintSystem;
    use r1cs_std::{
        alloc::AllocGadget,
        fields::FieldGadget,
        groups::curves::short_weierstrass::bls12::{G1Gadget, G2Gadget},
        test_constraint_system::TestConstraintSystem,
        Assignment,
    };

    use super::YToBitGadget;

    #[test]
    fn test_y_to_bit_g1() {
        let rng = &mut XorShiftRng::from_seed([
            0x5d, 0xbe, 0x62, 0x59, 0x8d, 0x31, 0x3d, 0x76, 0x32, 0x37, 0xdb, 0x17, 0xe5, 0xbc,
            0x06, 0x54,
        ]);
        for i in 0..10 {
            let secret_key = Bls12_377Fr::rand(rng);

            let generator = Bls12_377G1Projective::prime_subgroup_generator();
            let pub_key = generator.clone().mul(secret_key);

            let half = <Bls12_377Parameters as Bls12Parameters>::Fp::modulus_minus_one_div_two();

            {
                let mut cs = TestConstraintSystem::<SW6Fr>::new();

                let pk =
                    G1Gadget::<Bls12_377Parameters>::alloc(&mut cs.ns(|| "alloc"), || Ok(pub_key))
                        .unwrap();

                let y_bit =
                    YToBitGadget::<Bls12_377Parameters>::y_to_bit_g1(cs.ns(|| "y to bit"), &pk)
                        .unwrap();

                assert_eq!(
                    pk.y.get_value().get().unwrap().into_repr() > half,
                    y_bit.get_value().get().unwrap()
                );

                if i == 0 {
                    println!("number of constraints: {}", cs.num_constraints());
                }

                assert!(cs.is_satisfied());
            }
        }
    }

    #[test]
    fn test_y_to_bit_g2() {
        let rng = &mut XorShiftRng::from_seed([
            0x5d, 0xbe, 0x62, 0x59, 0x8d, 0x31, 0x3d, 0x76, 0x32, 0x37, 0xdb, 0x17, 0xe5, 0xbc,
            0x06, 0x54,
        ]);
        for i in 0..10 {
            let secret_key = Bls12_377Fr::rand(rng);

            let generator = Bls12_377G2Projective::prime_subgroup_generator();
            let pub_key = generator.clone().mul(secret_key);

            let half = <Bls12_377Parameters as Bls12Parameters>::Fp::modulus_minus_one_div_two();

            {
                let mut cs = TestConstraintSystem::<SW6Fr>::new();

                let pk =
                    G2Gadget::<Bls12_377Parameters>::alloc(&mut cs.ns(|| "alloc"), || Ok(pub_key))
                        .unwrap();

                let y_bit =
                    YToBitGadget::<Bls12_377Parameters>::y_to_bit_g2(cs.ns(|| "y to bit"), &pk)
                        .unwrap();

                if pk.y.c1.get_value().get().unwrap().into_repr() > half
                    || (pk.y.c1.get_value().get().unwrap().into_repr() == half
                        && pk.y.c0.get_value().get().unwrap().into_repr() > half)
                {
                    assert_eq!(true, y_bit.get_value().get().unwrap());
                } else {
                    assert_eq!(false, y_bit.get_value().get().unwrap());
                }

                if i == 0 {
                    println!("number of constraints: {}", cs.num_constraints());
                }

                if !cs.is_satisfied() {
                    println!("{}", cs.which_is_unsatisfied().unwrap());
                }
                assert!(cs.is_satisfied());
            }
        }
    }
}
