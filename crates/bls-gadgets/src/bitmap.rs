use crate::{is_setup, smaller_than::SmallerThanGadget};
use algebra::PrimeField;
use r1cs_core::{ConstraintSystem, LinearCombination, SynthesisError};
use r1cs_std::{
    fields::{fp::FpGadget, FieldGadget},
    prelude::*,
    Assignment,
};

/// Enforces that there are no more than `max_occurrences` of `value` (0 or 1)
/// present in the provided bitmap
pub fn enforce_maximum_occurrences_in_bitmap<F: PrimeField, CS: ConstraintSystem<F>>(
    cs: &mut CS,
    bitmap: &[Boolean],
    max_occurrences: &FpGadget<F>,
    value: bool,
) -> Result<(), SynthesisError> {
    let mut value_fp = F::one();
    if !value {
        // using the opposite value if we are counting 0s
        value_fp = value_fp.neg();
    }
    // If we're in setup mode, we skip the bit counting part since the bitmap
    // will be empty
    let is_setup = is_setup(&bitmap);

    let mut occurrences = 0;
    let mut occurrences_lc = LinearCombination::zero();
    // For each bit, increment the number of occurences if the bit matched `value`
    // We calculate both the number of occurrences
    // and a linear combination over it, in order to do 2 things:
    // 1. enforce that occurrences < maximum_occurences
    // 2. enforce that occurrences was calculated correctly from the bitmap
    for bit in bitmap {
        // Update the constraints
        if !value {
            // add 1 here only for zeros
            occurrences_lc += (F::one(), CS::one());
        }
        occurrences_lc = occurrences_lc + bit.lc(CS::one(), value_fp);

        // Update our count
        if !is_setup {
            let got_value = bit.get_value().get()?;
            occurrences += (got_value == value) as u8;
        }
    }
    // Rebind `occurrences` to a constraint
    let occurrences = FpGadget::alloc(&mut cs.ns(|| "num occurrences"), || {
        Ok(F::from(occurrences))
    })?;

    SmallerThanGadget::<F>::enforce_smaller_than_or_equal_to_strict(
        &mut cs.ns(|| "enforce maximum number of occurrences"),
        &occurrences,
        &max_occurrences,
    )?;

    // Enforce that we have correctly counted the number of occurrences
    cs.enforce(
        || "enforce num occurrences lc equal to num",
        |_| occurrences_lc,
        |lc| lc + (F::one(), CS::one()),
        |lc| occurrences.get_variable() + lc,
    );

    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;
    use algebra::{
        bls12_377::{Fq, Fr},
        Bls12_377,
    };
    use groth16::{
        create_random_proof, generate_random_parameters, prepare_verifying_key, verify_proof,
    };
    use r1cs_core::ConstraintSynthesizer;
    use r1cs_std::test_constraint_system::TestConstraintSystem;

    #[test]
    // "I know of a bitmap that has at most 2 zeros"
    fn groth16_ok() {
        let rng = &mut rand::thread_rng();

        #[derive(Clone)]
        struct BitmapGadget {
            bitmap: Vec<Option<bool>>,
            max_occurrences: u64,
            value: bool,
        }

        impl ConstraintSynthesizer<Fr> for BitmapGadget {
            fn generate_constraints<CS: ConstraintSystem<Fr>>(
                self,
                cs: &mut CS,
            ) -> Result<(), SynthesisError> {
                let bitmap = self
                    .bitmap
                    .iter()
                    .enumerate()
                    .map(|(i, b)| {
                        Boolean::alloc(cs.ns(|| i.to_string()), || Ok(b.unwrap())).unwrap()
                    })
                    .collect::<Vec<_>>();
                let max_occurrences = FpGadget::<Fr>::alloc(cs.ns(|| "max occurences"), || {
                    Ok(Fr::from(self.max_occurrences))
                })
                .unwrap();
                enforce_maximum_occurrences_in_bitmap(cs, &bitmap, &max_occurrences, self.value)
            }
        }

        let params = {
            let empty = BitmapGadget {
                bitmap: vec![None; 10],
                max_occurrences: 2,
                value: false,
            };
            generate_random_parameters::<Bls12_377, _, _>(empty, rng).unwrap()
        };

        // all true bitmap, max occurences of 2 zeros allowed
        let bitmap = vec![Some(true); 10];
        let circuit = BitmapGadget {
            bitmap,
            max_occurrences: 2,
            value: false,
        };

        // since our Test constraint system is satisfied, the groth16 proof
        // should also work
        let mut cs = TestConstraintSystem::<Fr>::new();
        circuit.clone().generate_constraints(&mut cs).unwrap();
        assert!(cs.is_satisfied());
        let proof = create_random_proof(circuit, &params, rng).unwrap();

        let pvk = prepare_verifying_key(&params.vk);
        assert!(verify_proof(&pvk, &proof, &[]).unwrap());
    }

    fn cs_enforce_value(
        bitmap: &[bool],
        max_number: u64,
        is_one: bool,
    ) -> TestConstraintSystem<Fq> {
        let mut cs = TestConstraintSystem::<Fq>::new();
        let bitmap = bitmap
            .iter()
            .map(|b| Boolean::constant(*b))
            .collect::<Vec<_>>();
        let max_occurrences =
            FpGadget::<Fq>::alloc(cs.ns(|| "max occurences"), || Ok(Fq::from(max_number))).unwrap();
        enforce_maximum_occurrences_in_bitmap(&mut cs, &bitmap, &max_occurrences, is_one).unwrap();
        cs
    }

    mod zeros {
        use super::*;

        #[test]
        fn one_zero_allowed() {
            assert!(cs_enforce_value(&[false], 1, false).is_satisfied());
        }

        #[test]
        fn no_zeros_allowed() {
            assert!(!cs_enforce_value(&[false], 0, false).is_satisfied());
        }

        #[test]
        fn three_zeros_allowed() {
            assert!(cs_enforce_value(&[false, true, true, false, false], 3, false).is_satisfied());
        }

        #[test]
        fn four_zeros_not_allowed() {
            assert!(
                !cs_enforce_value(&[false, false, true, false, false], 3, false).is_satisfied()
            );
        }
    }

    mod ones {
        use super::*;

        #[test]
        fn one_one_allowed() {
            assert!(cs_enforce_value(&[true], 1, true).is_satisfied());
        }

        #[test]
        fn no_ones_allowed() {
            assert!(!cs_enforce_value(&[true], 0, true).is_satisfied());
        }

        #[test]
        fn three_ones_allowed() {
            assert!(cs_enforce_value(&[false, true, true, true, false], 3, true).is_satisfied());
        }

        #[test]
        fn four_ones_not_allowed() {
            assert!(!cs_enforce_value(&[true, true, true, true, false], 3, true).is_satisfied());
        }
    }
}
