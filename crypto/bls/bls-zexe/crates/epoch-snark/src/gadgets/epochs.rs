//! # Validator Set Update Circuit
//!
//! Prove the validator state transition function for the BLS 12-377 curve.

use algebra::{bls12_377::Bls12_377, sw6::Fr};
use r1cs_std::prelude::*;
use r1cs_std::{
    bls12_377::{G1Gadget, G2Gadget, PairingGadget},
    bls12_377::{G1PreparedGadget, G2PreparedGadget},
    fields::fp::FpGadget,
    Assignment,
};
use tracing::{debug, span, Level};

use r1cs_core::{ConstraintSynthesizer, ConstraintSystem, SynthesisError};

use algebra::PairingEngine;
use groth16::{Proof, VerifyingKey};

use crate::gadgets::{g2_to_bits, single_update::SingleUpdate, EpochData, ProofOfCompression};

use bls_gadgets::BlsVerifyGadget;
type BlsGadget = BlsVerifyGadget<Bls12_377, Fr, PairingGadget>;
type FrGadget = FpGadget<Fr>;

#[derive(Clone)]
/// Multiple epoch block transitions
pub struct ValidatorSetUpdate<E: PairingEngine> {
    pub initial_epoch: EpochData<E>,
    /// The number of validators over all the epochs
    pub num_validators: u32,
    /// A list of all the updates for multiple epochs
    pub epochs: Vec<SingleUpdate<E>>,
    /// The aggregated signature of all the validators over all the epoch changes
    pub aggregated_signature: Option<E::G1Projective>,
    /// The optional hash to bits proof data. If provided, the circuit **will not**
    /// constrain the inner CRH->XOF hashes in SW6 and instead it will be verified
    /// via the helper's proof which is in BLS12-377.
    pub hash_helper: Option<HashToBitsHelper<E>>,
}

#[derive(Clone)]
pub struct HashToBitsHelper<E: PairingEngine> {
    /// The Groth16 proof satisfying the statement
    pub proof: Proof<E>,
    /// The VK produced by the trusted setup
    pub verifying_key: VerifyingKey<E>,
}

impl<E: PairingEngine> ValidatorSetUpdate<E> {
    pub fn empty(
        num_validators: usize,
        num_epochs: usize,
        maximum_non_signers: usize,
        vk: Option<VerifyingKey<E>>,
    ) -> Self {
        let empty_update = SingleUpdate::empty(num_validators, maximum_non_signers);
        let hash_helper = vk.map(|vk| HashToBitsHelper {
            proof: Proof::<E>::default(),
            verifying_key: vk,
        });

        ValidatorSetUpdate {
            initial_epoch: EpochData::empty(num_validators, maximum_non_signers),
            num_validators: num_validators as u32,
            epochs: vec![empty_update; num_epochs],
            aggregated_signature: None,
            hash_helper,
        }
    }
}

impl ConstraintSynthesizer<Fr> for ValidatorSetUpdate<Bls12_377> {
    // Enforce that the signatures over the epochs have been calculated
    // correctly, and then compress the public inputs
    fn generate_constraints<CS: ConstraintSystem<Fr>>(
        self,
        cs: &mut CS,
    ) -> Result<(), SynthesisError> {
        let span = span!(Level::TRACE, "ValidatorSetUpdate");
        let _enter = span.enter();
        info!("generating constraints");
        let proof_of_compression = self.enforce(&mut cs.ns(|| "check signature"))?;
        proof_of_compression
            .compress_public_inputs(&mut cs.ns(|| "compress public inputs"), self.hash_helper)?;

        info!("constraints generated");

        Ok(())
    }
}

impl ValidatorSetUpdate<Bls12_377> {
    fn enforce<CS: ConstraintSystem<Fr>>(
        &self,
        cs: &mut CS,
    ) -> Result<ProofOfCompression, SynthesisError> {
        let span = span!(Level::TRACE, "ValidatorSetUpdate_enforce");
        let _enter = span.enter();

        debug!("converting initial EpochData to_bits");
        // Constrain the initial epoch and get its bits
        let (first_epoch_bits, first_epoch_index, initial_maximum_non_signers, initial_pubkey_vars) =
            self.initial_epoch.to_bits(&mut cs.ns(|| "initial epoch"))?;

        // Constrain all intermediate epochs, and get the aggregate pubkey and epoch hash
        // from each one, to be used for the batch verification
        debug!("verifying intermediate epochs");
        let (
            last_epoch_bits,
            crh_bits,
            xof_bits,
            prepared_aggregated_public_keys,
            prepared_message_hashes,
        ) = self.verify_intermediate_epochs(
            &mut cs.ns(|| "verify epochs"),
            first_epoch_index,
            initial_pubkey_vars,
            initial_maximum_non_signers,
        )?;

        // Verify the aggregate BLS signature
        debug!("verifying bls signature");
        self.verify_signature(
            &mut cs.ns(|| "verify aggregated signature"),
            &prepared_aggregated_public_keys,
            &prepared_message_hashes,
        )?;

        Ok(ProofOfCompression {
            first_epoch_bits,
            last_epoch_bits,
            crh_bits,
            xof_bits,
        })
    }

    /// Ensure that all epochs's bitmaps have been correctly computed
    /// and generates the witness data necessary for the final BLS Sig
    /// verification and witness compression
    #[allow(clippy::type_complexity)]
    fn verify_intermediate_epochs<CS: ConstraintSystem<Fr>>(
        &self,
        cs: &mut CS,
        first_epoch_index: FrGadget,
        initial_pubkey_vars: Vec<G2Gadget>,
        initial_max_non_signers: FrGadget,
    ) -> Result<
        (
            Vec<Boolean>,
            Vec<Boolean>,
            Vec<Boolean>,
            Vec<G2PreparedGadget>,
            Vec<G1PreparedGadget>,
        ),
        SynthesisError,
    > {
        let span = span!(Level::TRACE, "verify_intermediate_epochs");
        let _enter = span.enter();

        let mut prepared_aggregated_public_keys = vec![];
        let mut prepared_message_hashes = vec![];
        let mut last_epoch_bits = vec![];
        let mut previous_epoch_index = first_epoch_index;
        let mut previous_pubkey_vars = initial_pubkey_vars;
        let mut previous_max_non_signers = initial_max_non_signers;
        let mut all_crh_bits = vec![];
        let mut all_xof_bits = vec![];
        for (i, epoch) in self.epochs.iter().enumerate() {
            let span = span!(Level::TRACE, "index", i);
            let _enter = span.enter();
            let constrained_epoch = epoch.constrain(
                &mut cs.ns(|| format!("epoch {}", i)),
                &previous_pubkey_vars,
                &previous_epoch_index,
                &previous_max_non_signers,
                self.num_validators,
                self.hash_helper.is_none(), // generate constraints in SW6 if no helper was provided
            )?;

            // Update the pubkeys for the next iteration
            previous_epoch_index = constrained_epoch.index;
            previous_pubkey_vars = constrained_epoch.new_pubkeys;
            previous_max_non_signers = constrained_epoch.new_max_non_signers;

            // Save the aggregated pubkey / message hash pair for the BLS batch verification
            prepared_aggregated_public_keys.push(constrained_epoch.aggregate_pk);
            prepared_message_hashes.push(constrained_epoch.message_hash);

            // Save the xof/crh and the last epoch's bits for compressing the public inputs
            all_crh_bits.extend_from_slice(&constrained_epoch.crh_bits);
            all_xof_bits.extend_from_slice(&constrained_epoch.xof_bits);
            if i == self.epochs.len() - 1 {
                let last_apk = BlsGadget::enforce_aggregated_all_pubkeys(
                    cs.ns(|| "last epoch aggregated pk"),
                    &previous_pubkey_vars, // These are now the last epoch new pubkeys
                )?;
                let last_apk_bits =
                    g2_to_bits(&mut cs.ns(|| "last epoch aggregated pk bits"), &last_apk)?;
                last_epoch_bits = constrained_epoch.bits;
                last_epoch_bits.extend_from_slice(&last_apk_bits);
            }
            debug!("epoch {} constrained", i);
        }

        debug!("intermediate epochs verified");

        Ok((
            last_epoch_bits,
            all_crh_bits,
            all_xof_bits,
            prepared_aggregated_public_keys,
            prepared_message_hashes,
        ))
    }

    // Verify the aggregate signature
    fn verify_signature<CS: ConstraintSystem<Fr>>(
        &self,
        cs: &mut CS,
        pubkeys: &[G2PreparedGadget],
        messages: &[G1PreparedGadget],
    ) -> Result<(), SynthesisError> {
        let aggregated_signature = G1Gadget::alloc(cs.ns(|| "aggregated signature"), || {
            self.aggregated_signature.get()
        })?;
        BlsGadget::batch_verify_prepared(
            cs.ns(|| "batch verify BLS"),
            &pubkeys,
            &messages,
            &aggregated_signature,
        )?;

        Ok(())
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    use crate::gadgets::single_update::test_helpers::generate_single_update;
    use algebra::bls12_377::G1Projective;
    use bls_gadgets::test_helpers::{keygen_batch, keygen_mul, sign_batch, sum};
    use r1cs_std::test_constraint_system::TestConstraintSystem;

    type Curve = Bls12_377;

    // let's run our tests with 7 validators and 2 faulty ones
    mod epoch_batch_verification {
        use super::*;

        #[test]
        fn test_multiple_epochs() {
            let faults: u32 = 2;
            let num_validators = 3 * faults + 1;
            let initial_validator_set = keygen_mul::<Curve>(num_validators as usize);
            let initial_epoch =
                generate_single_update::<Curve>(0, faults, &initial_validator_set.1, &[])
                    .epoch_data;

            let num_epochs = 4;
            // no more than `faults` 0s exist in the bitmap
            // (i.e. at most `faults` validators who do not sign on the next validator set)
            let bitmaps = &[
                &[true, true, false, true, true, true, true],
                &[true, true, false, true, true, true, true],
                &[true, true, true, true, false, false, true],
                &[true, true, true, true, true, true, true],
            ];
            // Generate validators for each of the epochs
            let validators = keygen_batch::<Curve>(num_epochs, num_validators as usize);
            // Generate `num_epochs` epochs
            let epochs = validators
                .1
                .iter()
                .enumerate()
                .map(|(epoch_index, epoch_validators)| {
                    generate_single_update::<Curve>(
                        epoch_index as u16 + 1,
                        faults,
                        epoch_validators,
                        bitmaps[epoch_index],
                    )
                })
                .collect::<Vec<_>>();

            // The i-th validator set, signs on the i+1th epoch's G1 hash
            let mut signers = vec![initial_validator_set.0];
            signers.extend_from_slice(&validators.0[..validators.1.len() - 1]);

            // Filter the private keys which had a 1 in the boolean per epoch
            let mut signers_filtered = Vec::new();
            for i in 0..signers.len() {
                let mut epoch_signers_filtered = Vec::new();
                let epoch_signers = &signers[i];
                let epoch_bitmap = bitmaps[i];
                for (j, epoch_signer) in epoch_signers.iter().enumerate() {
                    if epoch_bitmap[j] {
                        epoch_signers_filtered.push(*epoch_signer);
                    }
                }
                signers_filtered.push(epoch_signers_filtered);
            }

            use crate::gadgets::test_helpers::hash_epoch;
            let epoch_hashes = epochs
                .iter()
                .map(|update| hash_epoch(&update.epoch_data))
                .collect::<Vec<G1Projective>>();

            let asigs = sign_batch::<Bls12_377>(&signers_filtered, &epoch_hashes);
            let aggregated_signature = sum(&asigs);

            let valset = ValidatorSetUpdate::<Curve> {
                initial_epoch,
                epochs,
                num_validators,
                aggregated_signature: Some(aggregated_signature),
                hash_helper: None,
            };

            let mut cs = TestConstraintSystem::<Fr>::new();
            valset.enforce(&mut cs).unwrap();
            assert!(cs.is_satisfied());
        }
    }
}
