use algebra::{Group, PrimeField};
use bls_gadgets::enforce_maximum_occurrences_in_bitmap;
use r1cs_core::{ConstraintSystem, SynthesisError};
use r1cs_std::{boolean::Boolean, groups::GroupGadget};
use std::marker::PhantomData;

/// Gadget for checking that slice diffs are computed correctly
pub struct SliceUpdateGadget<G, F, GG> {
    group_type: PhantomData<G>,
    field_type: PhantomData<F>,
    gadget_type: PhantomData<GG>,
}

impl<G, F, GG> SliceUpdateGadget<G, F, GG>
where
    G: Group,
    F: PrimeField,
    GG: GroupGadget<G, F>,
{
    /// Enforces that the old slice's elements are replaced by the new slice's elements
    /// at the indexes where the bitmap is set to 1, and that no more than
    /// `maximum_removed` elements were changed.
    ///
    /// # Panics
    /// - If `old_elements.len()` != `bitmap.len()`
    /// - If `new_elements.len()` != `bitmap.len()`
    pub fn update<CS: ConstraintSystem<F>>(
        mut cs: CS,
        old_elements: &[GG],
        new_elements: &[GG],
        bitmap: &[Boolean],
        maximum_removed: u64,
    ) -> Result<Vec<GG>, SynthesisError> {
        assert_eq!(old_elements.len(), bitmap.len());
        assert_eq!(new_elements.len(), bitmap.len());
        // check that no more than `maximum_removed` 1s exist in
        // the provided bitmap
        enforce_maximum_occurrences_in_bitmap(&mut cs, bitmap, maximum_removed, true)?;

        // check that the new_elements are correctly computed from the
        // bitmap and the old slice
        Self::enforce_slice_update(&mut cs, old_elements, new_elements, bitmap)
    }

    /// Checks that if the i_th bit in the provided bitmap is set to 1:
    /// the i_th element in the old slice is replaced
    /// with the i_th element in the new slice
    fn enforce_slice_update<CS: ConstraintSystem<F>>(
        cs: &mut CS,
        old_elements: &[GG],
        new_elements: &[GG],
        bitmap: &[Boolean],
    ) -> Result<Vec<GG>, SynthesisError> {
        let mut updated_elements = Vec::with_capacity(old_elements.len());
        for (i, (pk, bit)) in old_elements.iter().zip(bitmap).enumerate() {
            // if the bit is 1, the element is replaced
            let new_element = GG::conditionally_select(
                cs.ns(|| format!("cond_select {}", i)),
                bit,
                &new_elements[i],
                pk,
            )?;
            updated_elements.push(new_element);
        }
        Ok(updated_elements)
    }
}

#[cfg(test)]
mod test {
    use super::*;
    use std::borrow::Borrow;

    use algebra::{
        bls12_377::{Bls12_377, Parameters as Bls12_377Parameters},
        curves::{bls12::Bls12Parameters, short_weierstrass_jacobian::GroupProjective},
        PairingEngine, ProjectiveCurve, UniformRand,
    };
    use r1cs_std::{
        alloc::AllocGadget, boolean::Boolean, groups::bls12::G2Gadget,
        test_constraint_system::TestConstraintSystem,
    };

    // We run a test where we assume the slice elements are validator pubkeys
    fn cs_update<E: PairingEngine, P: Bls12Parameters>(
        old_pubkeys: &[E::G2Projective],
        new_pubkeys: &[E::G2Projective],
        bitmap: &[bool],
        max_removed_validators: u64,
        satisfied: bool,
    ) -> Vec<GroupProjective<P::G2Parameters>>
    where
        // TODO: Is there a way to remove this awkward type bound?
        E::G2Projective: Borrow<GroupProjective<P::G2Parameters>>,
    {
        let mut cs = TestConstraintSystem::<P::Fp>::new();

        // convert the arguments to constraints
        let old_pub_keys = pubkeys_to_constraints::<P, E>(&mut cs, old_pubkeys, "old");
        let new_pub_keys = pubkeys_to_constraints::<P, E>(&mut cs, new_pubkeys, "new");
        let bitmap = bitmap
            .iter()
            .map(|b| Boolean::constant(*b))
            .collect::<Vec<_>>();

        // check the result and return the inner data
        let res = SliceUpdateGadget::update(
            cs.ns(|| "validator update"),
            &old_pub_keys,
            &new_pub_keys,
            &bitmap,
            max_removed_validators,
        )
        .unwrap();
        assert_eq!(cs.is_satisfied(), satisfied);
        res.into_iter()
            .map(|x| x.get_value().unwrap())
            .collect::<Vec<_>>()
    }

    #[test]
    fn all_validators_removed() {
        let (_, old_pubkeys) = keygen_mul::<Bls12_377>(5);
        let (_, new_pubkeys) = keygen_mul::<Bls12_377>(5);
        let result = cs_update::<Bls12_377, Bls12_377Parameters>(
            &old_pubkeys,
            &new_pubkeys,
            &[true; 5],
            5,
            true,
        );
        assert_eq!(new_pubkeys, result);
    }

    #[test]
    fn one_validator_removed() {
        let (_, mut old_pubkeys) = keygen_mul::<Bls12_377>(5);
        let (_, new_pubkeys) = keygen_mul::<Bls12_377>(5);
        let result = cs_update::<Bls12_377, Bls12_377Parameters>(
            &old_pubkeys,
            &new_pubkeys,
            &[true, false, false, false, false],
            3,
            true,
        );
        // the first pubkey was replaced with the 1st pubkey from the
        // new set
        old_pubkeys[0] = new_pubkeys[0];
        assert_eq!(old_pubkeys, result);
    }

    #[test]
    fn some_validators_removed() {
        let (_, old_pubkeys) = keygen_mul::<Bls12_377>(5);
        let (_, mut new_pubkeys) = keygen_mul::<Bls12_377>(5);
        let result = cs_update::<Bls12_377, Bls12_377Parameters>(
            &old_pubkeys,
            &new_pubkeys,
            &[false, true, true, false, true],
            3,
            true,
        );
        // all validators were replaced except the 1st and 4th
        new_pubkeys[0] = old_pubkeys[0];
        new_pubkeys[3] = old_pubkeys[3];
        assert_eq!(new_pubkeys, result);
    }

    #[test]
    fn cannot_remove_more_validators_than_allowed() {
        let (_, old_pubkeys) = keygen_mul::<Bls12_377>(5);
        let (_, new_pubkeys) = keygen_mul::<Bls12_377>(5);
        cs_update::<Bls12_377, Bls12_377Parameters>(
            &old_pubkeys,
            &new_pubkeys,
            &[true; 5], // tries to remove all 5, when only 4 are allowed
            4,
            false,
        );
    }

    #[test]
    #[should_panic]
    fn bad_bitmap_length() {
        let (_, old_pubkeys) = keygen_mul::<Bls12_377>(5);
        let (_, new_pubkeys) = keygen_mul::<Bls12_377>(5);
        cs_update::<Bls12_377, Bls12_377Parameters>(
            &old_pubkeys,
            &new_pubkeys,
            &[true; 3],
            5,
            false,
        );
    }

    #[test]
    #[should_panic]
    fn new_validator_set_wrong_size() {
        let (_, old_pubkeys) = keygen_mul::<Bls12_377>(5);
        let (_, new_pubkeys) = keygen_mul::<Bls12_377>(4);
        cs_update::<Bls12_377, Bls12_377Parameters>(
            &old_pubkeys,
            &new_pubkeys,
            &[true; 5],
            5,
            false,
        );
    }

    fn pubkeys_to_constraints<P: Bls12Parameters, E: PairingEngine>(
        cs: &mut TestConstraintSystem<P::Fp>,
        pubkeys: &[E::G2Projective],
        personalization: &str,
    ) -> Vec<G2Gadget<P>>
    where
        E::G2Projective: Borrow<GroupProjective<P::G2Parameters>>,
    {
        pubkeys
            .iter()
            .enumerate()
            .map(|(i, x)| {
                G2Gadget::<P>::alloc(
                    &mut cs.ns(|| format!("alloc {} {}", personalization, i)),
                    || Ok(*x),
                )
                .unwrap()
            })
            .collect()
    }

    fn keygen_mul<E: PairingEngine>(num: usize) -> (Vec<E::Fr>, Vec<E::G2Projective>) {
        let rng = &mut rand::thread_rng();
        let generator = E::G2Projective::prime_subgroup_generator();

        let mut secret_keys = Vec::new();
        let mut public_keys = Vec::new();
        for _ in 0..num {
            let secret_key = E::Fr::rand(rng);
            let public_key = generator.mul(secret_key);
            secret_keys.push(secret_key);
            public_keys.push(public_key);
        }
        (secret_keys, public_keys)
    }
}
