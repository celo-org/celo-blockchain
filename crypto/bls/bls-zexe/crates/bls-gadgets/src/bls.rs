use crate::enforce_maximum_occurrences_in_bitmap;
use algebra::{PairingEngine, PrimeField, ProjectiveCurve};
use r1cs_core::{ConstraintSystem, SynthesisError};
use r1cs_std::{
    alloc::AllocGadget, boolean::Boolean, eq::EqGadget, fields::fp::FpGadget, fields::FieldGadget,
    groups::GroupGadget, pairing::PairingGadget, select::CondSelectGadget,
};
use std::marker::PhantomData;
use tracing::{debug, span, trace, Level};

/// BLS Signature Verification Pairing Gadget.
///
/// Implements BLS Verification as written in [BDN18](https://eprint.iacr.org/2018/483.pdf)
/// in a Pairing-based SNARK.
pub struct BlsVerifyGadget<E, F, P> {
    /// The curve being used
    pairing_engine_type: PhantomData<E>,
    /// The field we're operating on
    constraint_field_type: PhantomData<F>,
    /// The pairing gadget we use, which MUST match our pairing engine
    pairing_gadget_type: PhantomData<P>,
}

impl<E, F, P> BlsVerifyGadget<E, F, P>
where
    E: PairingEngine,
    F: PrimeField,
    P: PairingGadget<E, F>,
{
    /// Enforces verification of a BLS Signature against a list of public keys and a bitmap indicating
    /// which of these pubkeys signed.
    ///
    /// A maximum number of non_signers is also provided to
    /// indicate our threshold
    ///
    /// The verification equation can be found in pg.11 from
    /// https://eprint.iacr.org/2018/483.pdf: "Multi-Signature Verification"
    pub fn verify<CS: ConstraintSystem<F>>(
        mut cs: CS,
        pub_keys: &[P::G2Gadget],
        signed_bitmap: &[Boolean],
        message_hash: &P::G1Gadget,
        signature: &P::G1Gadget,
        maximum_non_signers: &FpGadget<F>,
    ) -> Result<(), SynthesisError> {
        let span = span!(Level::TRACE, "BlsVerifyGadget_verify");
        let _enter = span.enter();
        // Get the message hash and the aggregated public key based on the bitmap
        // and allowed number of non-signers
        let (prepared_message_hash, prepared_aggregated_pk) = Self::enforce_bitmap_and_prepare(
            cs.ns(|| "verify partial"),
            pub_keys,
            signed_bitmap,
            message_hash,
            maximum_non_signers,
        )?;

        // Prepare the signature and get the generator
        let (prepared_signature, prepared_g2_neg_generator) =
            Self::prepare_signature_neg_generator(&mut cs, &signature)?;

        // e(σ, g_2^-1) * e(H(m), apk) == 1_{G_T}
        Self::enforce_bls_equation(
            &mut cs,
            &[prepared_signature, prepared_message_hash],
            &[prepared_g2_neg_generator, prepared_aggregated_pk],
        )?;

        Ok(())
    }

    /// Enforces batch verification of a an aggregate BLS Signature against a
    /// list of (pubkey, message) tuples.
    ///
    /// The verification equation can be found in pg.11 from
    /// https://eprint.iacr.org/2018/483.pdf: "Batch verification"
    pub fn batch_verify<CS: ConstraintSystem<F>>(
        mut cs: CS,
        aggregated_pub_keys: &[P::G2Gadget],
        message_hashes: &[P::G1Gadget],
        aggregated_signature: &P::G1Gadget,
    ) -> Result<(), SynthesisError> {
        debug!("batch verifying BLS signature");
        let prepared_message_hashes = message_hashes
            .iter()
            .enumerate()
            .map(|(i, message_hash)| {
                P::prepare_g1(
                    cs.ns(|| format!("prepared message hash {}", i)),
                    &message_hash,
                )
            })
            .collect::<Result<Vec<_>, _>>()?;
        let prepared_aggregated_pub_keys = aggregated_pub_keys
            .iter()
            .enumerate()
            .map(|(i, pubkey)| P::prepare_g2(cs.ns(|| format!("prepared pubkey {}", i)), &pubkey))
            .collect::<Result<Vec<_>, _>>()?;

        Self::batch_verify_prepared(
            cs,
            &prepared_aggregated_pub_keys,
            &prepared_message_hashes,
            aggregated_signature,
        )
    }

    /// Batch verification against prepared messages
    pub fn batch_verify_prepared<CS: ConstraintSystem<F>>(
        mut cs: CS,
        prepared_aggregated_pub_keys: &[P::G2PreparedGadget],
        prepared_message_hashes: &[P::G1PreparedGadget],
        aggregated_signature: &P::G1Gadget,
    ) -> Result<(), SynthesisError> {
        // Prepare the signature and get the generator
        let (prepared_signature, prepared_g2_neg_generator) =
            Self::prepare_signature_neg_generator(&mut cs, aggregated_signature)?;

        // Create the vectors which we'll batch verify
        let mut prepared_g1s = vec![prepared_signature];
        let mut prepared_g2s = vec![prepared_g2_neg_generator];
        prepared_g1s.extend_from_slice(&prepared_message_hashes);
        prepared_g2s.extend_from_slice(&prepared_aggregated_pub_keys);

        // Enforce the BLS check
        // e(σ, g_2^-1) * e(H(m0), pk_0) * e(H(m1), pk_1) ...  * e(H(m_n), pk_n)) == 1_{G_T}
        Self::enforce_bls_equation(&mut cs, &prepared_g1s, &prepared_g2s)?;

        Ok(())
    }

    /// Returns a gadget which checks that an aggregate pubkey is correctly calculated
    /// by the sum of the pub keys which had a 1 in the bitmap
    ///
    /// # Panics
    /// If signed_bitmap length != pub_keys length
    pub fn enforce_aggregated_pubkeys<CS: ConstraintSystem<F>>(
        mut cs: CS,
        pub_keys: &[P::G2Gadget],
        signed_bitmap: &[Boolean],
    ) -> Result<P::G2PreparedGadget, SynthesisError> {
        // Bitmap and Pubkeys must be of the same length
        assert_eq!(signed_bitmap.len(), pub_keys.len());
        // Allocate the G2 Generator
        let g2_generator = P::G2Gadget::alloc(cs.ns(|| "G2 generator"), || {
            Ok(E::G2Projective::prime_subgroup_generator())
        })?;

        // We initialize the Aggregate Public Key as a generator point, in order to
        // calculate the sum of all keys which have signed according to the bitmap.
        // This is needed since we cannot add to a Zero.
        // After the sum is calculated, we must subtract the generator to get the
        // correct result
        let mut aggregated_pk = g2_generator.clone();
        for (i, (pk, bit)) in pub_keys.iter().zip(signed_bitmap).enumerate() {
            // Add the pubkey to the sum
            // if bit: aggregated_pk += pk
            let added = aggregated_pk.add(cs.ns(|| format!("add pk {}", i)), pk)?;
            aggregated_pk = P::G2Gadget::conditionally_select(
                &mut cs.ns(|| format!("cond_select {}", i)),
                &bit,
                &added,
                &aggregated_pk,
            )?;
        }
        // Subtract the generator to get the correct aggregate pubkey
        aggregated_pk = aggregated_pk.sub(cs.ns(|| "add neg generator"), &g2_generator)?;

        let prepared_aggregated_pk =
            P::prepare_g2(cs.ns(|| "prepared aggregated pk"), &aggregated_pk)?;
        Ok(prepared_aggregated_pk)
    }

    /// Returns a gadget which checks that an aggregate pubkey is correctly calculated
    /// by the sum of the pub keys
    pub fn enforce_aggregated_all_pubkeys<CS: ConstraintSystem<F>>(
        mut cs: CS,
        pub_keys: &[P::G2Gadget],
    ) -> Result<P::G2Gadget, SynthesisError> {
        // Allocate the G2 Generator
        let g2_generator = P::G2Gadget::alloc(cs.ns(|| "G2 generator"), || {
            Ok(E::G2Projective::prime_subgroup_generator())
        })?;

        // We initialize the Aggregate Public Key as a generator point, in order to
        // calculate the sum of all keys.
        // This is needed since we cannot add to a Zero.
        // After the sum is calculated, we must subtract the generator to get the
        // correct result
        let mut aggregated_pk = g2_generator.clone();
        for (i, pk) in pub_keys.iter().enumerate() {
            // Add the pubkey to the sum
            // aggregated_pk += pk
            aggregated_pk = aggregated_pk.add(&mut cs.ns(|| format!("add pk {}", i)), pk)?;
        }
        // Subtract the generator to get the correct aggregate pubkey
        aggregated_pk = aggregated_pk.sub(cs.ns(|| "add neg generator"), &g2_generator)?;

        Ok(aggregated_pk)
    }

    /// Enforces that the provided bitmap contains no more than `maximum_non_signers`
    /// 0s. Also returns a gadget of the prepared message hash and a gadget for the aggregate public key
    ///
    /// # Panics
    /// If signed_bitmap length != pub_keys length (due to internal call to `enforced_aggregated_pubkeys`)
    pub fn enforce_bitmap_and_prepare<CS: ConstraintSystem<F>>(
        mut cs: CS,
        pub_keys: &[P::G2Gadget],
        signed_bitmap: &[Boolean],
        message_hash: &P::G1Gadget,
        maximum_non_signers: &FpGadget<F>,
    ) -> Result<(P::G1PreparedGadget, P::G2PreparedGadget), SynthesisError> {
        trace!("enforcing bitmap");
        enforce_maximum_occurrences_in_bitmap(&mut cs, signed_bitmap, maximum_non_signers, false)?;

        trace!("preparing message hash and aggregated pubkey");
        let prepared_message_hash =
            P::prepare_g1(cs.ns(|| "prepared message hash"), &message_hash)?;
        let prepared_aggregated_pk =
            Self::enforce_aggregated_pubkeys(&mut cs, pub_keys, signed_bitmap)?;

        Ok((prepared_message_hash, prepared_aggregated_pk))
    }

    /// Verifying BLS signatures requires preparing a G1 Signature and
    /// preparing a negated G2 generator
    fn prepare_signature_neg_generator<CS: ConstraintSystem<F>>(
        cs: &mut CS,
        signature: &P::G1Gadget,
    ) -> Result<(P::G1PreparedGadget, P::G2PreparedGadget), SynthesisError> {
        // Ensure the signature is prepared
        let prepared_signature = P::prepare_g1(cs.ns(|| "prepared signature"), signature)?;

        // Allocate the generator on G2
        let g2_generator = P::G2Gadget::alloc(cs.ns(|| "G2 generator"), || {
            Ok(E::G2Projective::prime_subgroup_generator())
        })?;
        // and negate it for the purpose of verification
        let g2_neg_generator = g2_generator.negate(cs.ns(|| "negate g2 generator"))?;
        let prepared_g2_neg_generator =
            P::prepare_g2(cs.ns(|| "prepared g2 neg generator"), &g2_neg_generator)?;

        Ok((prepared_signature, prepared_g2_neg_generator))
    }

    /// Multiply the pairings together and check that their product == 1 in G_T, which indicates
    /// that the verification has passed.
    ///
    /// Each G1 element is paired with the corresponding G2 element.
    /// Fails if the 2 slices have different lengths.
    fn enforce_bls_equation<CS: ConstraintSystem<F>>(
        cs: &mut CS,
        g1: &[P::G1PreparedGadget],
        g2: &[P::G2PreparedGadget],
    ) -> Result<(), SynthesisError> {
        trace!("enforcing BLS equation");
        let bls_equation = P::product_of_pairings(cs.ns(|| "verify BLS signature"), g1, g2)?;
        let gt_one = &P::GTGadget::one(&mut cs.ns(|| "GT one"))?;
        bls_equation.enforce_equal(&mut cs.ns(|| "BLS equation is one"), gt_one)?;
        Ok(())
    }
}

#[cfg(test)]
mod verify_one_message {
    use super::*;
    use crate::test_helpers::*;

    use algebra::{
        bls12_377::{Bls12_377, Fr as Bls12_377Fr, G1Projective, G2Projective},
        sw6::Fr as SW6Fr,
        ProjectiveCurve, UniformRand, Zero,
    };
    use r1cs_core::ConstraintSystem;
    use r1cs_std::{
        alloc::AllocGadget,
        bls12_377::{G1Gadget, PairingGadget as Bls12_377PairingGadget},
        boolean::Boolean,
        test_constraint_system::TestConstraintSystem,
    };

    // converts the arguments to constraints and checks them against the `verify` function
    fn cs_verify<E: PairingEngine, F: PrimeField, P: PairingGadget<E, F>>(
        message_hash: E::G1Projective,
        pub_keys: &[E::G2Projective],
        signature: E::G1Projective,
        bitmap: &[bool],
        num_non_signers: u64,
    ) -> TestConstraintSystem<F> {
        let mut cs = TestConstraintSystem::<F>::new();

        let message_hash_var =
            P::G1Gadget::alloc(cs.ns(|| "message_hash"), || Ok(message_hash)).unwrap();
        let signature_var = P::G1Gadget::alloc(cs.ns(|| "signature"), || Ok(signature)).unwrap();

        let pub_keys = pub_keys
            .iter()
            .enumerate()
            .map(|(i, pub_key)| {
                P::G2Gadget::alloc(cs.ns(|| format!("pub_key_{}", i)), || Ok(pub_key)).unwrap()
            })
            .collect::<Vec<_>>();
        let bitmap = bitmap
            .iter()
            .map(|b| Boolean::constant(*b))
            .collect::<Vec<_>>();

        let max_occurrences =
            &FpGadget::<F>::alloc(cs.ns(|| "num non signers"), || Ok(F::from(num_non_signers)))
                .unwrap();
        BlsVerifyGadget::<E, F, P>::verify(
            cs.ns(|| "verify sig"),
            &pub_keys,
            &bitmap,
            &message_hash_var,
            &signature_var,
            &max_occurrences,
        )
        .unwrap();

        cs
    }

    #[test]
    fn batch_verify_ok() {
        // generate 5 (aggregate sigs, message hash pairs)
        // verify them all in 1 call
        let batch_size = 5;
        let num_keys = 7;
        let rng = &mut rand::thread_rng();

        // generate some random messages
        let messages = (0..batch_size)
            .map(|_| G1Projective::rand(rng))
            .collect::<Vec<_>>();
        // keygen for multiple rounds (7 keys per round)
        let (secret_keys, public_keys_batches) = keygen_batch::<Bls12_377>(batch_size, num_keys);
        // get the aggregate public key for each rounds
        let aggregate_pubkeys = public_keys_batches
            .iter()
            .map(|pks| sum(pks))
            .collect::<Vec<_>>();
        // the keys from each epoch sign the messages from the corresponding epoch
        let asigs = sign_batch::<Bls12_377>(&secret_keys, &messages);
        // get the complete aggregate signature
        let asig = sum(&asigs);

        // allocate the constraints
        let mut cs = TestConstraintSystem::<SW6Fr>::new();
        let messages = alloc_vec(&mut cs.ns(|| "messages"), &messages);
        let aggregate_pubkeys = alloc_vec(&mut cs.ns(|| "aggregate pubkeys"), &aggregate_pubkeys);
        let asig = G1Gadget::alloc(&mut cs.ns(|| "asig"), || Ok(asig)).unwrap();

        // check that verification is correct
        BlsVerifyGadget::<Bls12_377, SW6Fr, Bls12_377PairingGadget>::batch_verify(
            &mut cs,
            &aggregate_pubkeys,
            &messages,
            &asig,
        )
        .unwrap();
        assert!(cs.is_satisfied());
    }

    #[test]
    // Verifies signatures over BLS12_377 with Sw6 field (384 bits).
    fn one_signature_ok() {
        let (secret_key, pub_key) = keygen::<Bls12_377>();
        let rng = &mut rng();
        let message_hash = G1Projective::rand(rng);
        let signature = message_hash.mul(secret_key);
        let fake_signature = G1Projective::rand(rng);

        // good sig passes
        let cs = cs_verify::<Bls12_377, SW6Fr, Bls12_377PairingGadget>(
            message_hash,
            &[pub_key],
            signature,
            &[true],
            0,
        );
        assert!(cs.is_satisfied());
        assert_eq!(cs.num_constraints(), 21754);

        // random sig fails
        let cs = cs_verify::<Bls12_377, SW6Fr, Bls12_377PairingGadget>(
            message_hash,
            &[pub_key],
            fake_signature,
            &[true],
            0,
        );
        assert!(!cs.is_satisfied());
    }

    #[test]
    fn multiple_signatures_ok() {
        let rng = &mut rng();
        let message_hash = G1Projective::rand(rng);
        let (sk, pk) = keygen::<Bls12_377>();
        let (sk2, pk2) = keygen::<Bls12_377>();
        let (sigs, asig) = sign::<Bls12_377>(message_hash, &[sk, sk2]);

        // good aggregate sig passes
        let cs = cs_verify::<Bls12_377, SW6Fr, Bls12_377PairingGadget>(
            message_hash,
            &[pk, pk2],
            asig,
            &[true, true],
            1,
        );
        assert!(cs.is_satisfied());

        // using the single sig if second guy is OK as long as
        // we tolerate 1 non-signers
        let cs = cs_verify::<Bls12_377, SW6Fr, Bls12_377PairingGadget>(
            message_hash,
            &[pk, pk2],
            sigs[0],
            &[true, false],
            1,
        );
        assert!(cs.is_satisfied());

        // bitmap set to false on the second one fails since we don't tolerate
        // >0 failures
        let cs = cs_verify::<Bls12_377, SW6Fr, Bls12_377PairingGadget>(
            message_hash,
            &[pk, pk2],
            asig,
            &[true, false],
            0,
        );
        assert!(!cs.is_satisfied());
        let cs = cs_verify::<Bls12_377, SW6Fr, Bls12_377PairingGadget>(
            message_hash,
            &[pk, pk2],
            sigs[0],
            &[true, false],
            0,
        );
        assert!(!cs.is_satisfied());
    }

    #[test]
    fn zero_fails() {
        let rng = &mut rng();
        let message_hash = G1Projective::rand(rng);
        let generator = G2Projective::prime_subgroup_generator();

        // if the first key is a bad one, it should fail, since the pubkey
        // won't be on the curve
        let sk = Bls12_377Fr::zero();
        let pk = generator.clone().mul(sk);
        let (sk2, pk2) = keygen::<Bls12_377>();

        let (sigs, _) = sign::<Bls12_377>(message_hash, &[sk, sk2]);

        let cs = cs_verify::<Bls12_377, SW6Fr, Bls12_377PairingGadget>(
            message_hash,
            &[pk, pk2],
            sigs[1],
            &[false, true],
            3,
        );
        assert!(!cs.is_satisfied());
    }
}
