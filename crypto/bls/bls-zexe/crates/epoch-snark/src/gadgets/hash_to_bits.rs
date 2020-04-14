use algebra::{
    bls12_377::{Fr, FrParameters},
    FpParameters,
};
use r1cs_core::{ConstraintSynthesizer, ConstraintSystem, SynthesisError};

use super::{constrain_bool, MultipackGadget};
use bls_crypto::bls::keys::SIG_DOMAIN;
use bls_gadgets::hash_to_bits;
use tracing::{debug, info, span, trace, Level};

#[derive(Clone)]
/// Gadget which
/// 1. Converts its inputs to Boolean constraints
/// 1. Applies blake2x to them
/// 1. Packs both the boolean constraints and the blake2x constraints in Fr elements
///
/// Utilizes the `hash_to_bits` call under the hood
///
/// This is used as a helper gadget to enforce that the provided XOF bits inputs
/// are correctly calculated from the CRH in the SNARK
pub struct HashToBits {
    pub message_bits: Vec<Vec<Option<bool>>>,
}

impl HashToBits {
    /// To be used when generating the trusted setup parameters
    pub fn empty<P: FpParameters>(num_epochs: usize) -> Self {
        let modulus_bit_rounded = (((P::MODULUS_BITS + 7) / 8) * 8) as usize;
        HashToBits {
            message_bits: vec![vec![None; modulus_bit_rounded]; num_epochs],
        }
    }
}

impl ConstraintSynthesizer<Fr> for HashToBits {
    #[allow(clippy::cognitive_complexity)] // false positive triggered by the info!("generating constraints") log
    fn generate_constraints<CS: ConstraintSystem<Fr>>(
        self,
        cs: &mut CS,
    ) -> Result<(), SynthesisError> {
        let span = span!(Level::TRACE, "HashToBits");
        info!("generating constraints");
        let _enter = span.enter();
        let mut personalization = [0; 8];
        personalization.copy_from_slice(SIG_DOMAIN);

        let mut all_bits = vec![];
        let mut xof_bits = vec![];
        for (i, message_bits) in self.message_bits.iter().enumerate() {
            trace!(epoch = i, "hashing to bits");
            let bits = constrain_bool(&mut cs.ns(|| i.to_string()), &message_bits)?;
            let hash = hash_to_bits(
                cs.ns(|| format!("{}: hash to bits", i)),
                &bits,
                512,
                personalization,
                true,
            )?;
            all_bits.extend_from_slice(&bits);
            xof_bits.extend_from_slice(&hash);
        }

        // Pack them as public inputs
        debug!(capacity = FrParameters::CAPACITY, "packing CRH bits");
        MultipackGadget::pack(
            cs.ns(|| "pack messages"),
            &all_bits,
            FrParameters::CAPACITY as usize,
            true,
        )?;
        debug!(capacity = FrParameters::CAPACITY, "packing XOF bits");
        MultipackGadget::pack(
            cs.ns(|| "pack xof bits"),
            &xof_bits,
            FrParameters::CAPACITY as usize,
            true,
        )?;

        info!("constraints generated");
        Ok(())
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::gadgets::pack;
    use algebra::{sw6::FrParameters as SW6FrParameters, Bls12_377};
    use bls_crypto::hash::XOF;
    use bls_gadgets::{bits_to_bytes, bytes_to_bits};
    use groth16::{
        create_random_proof, generate_random_parameters, prepare_verifying_key, verify_proof,
    };
    use rand::RngCore;

    // applies the XOF to the input
    fn hash_to_bits_fn(message: &[bool]) -> Vec<bool> {
        let mut personalization = [0; 8];
        personalization.copy_from_slice(SIG_DOMAIN);
        let message = bits_to_bytes(&message);
        let hasher = bls_crypto::DirectHasher::new().unwrap();
        let hash_result = hasher.xof(&personalization, &message, 64).unwrap();
        let mut bits = bytes_to_bits(&hash_result, 512);
        bits.reverse();
        bits
    }

    #[test]
    fn test_verify_crh_to_xof() {
        let rng = &mut rand::thread_rng();
        // generate an empty circuit for 3 epochs
        let num_epochs = 3;
        // Trusted Setup -- USES THE SW6FrParameters!
        let params = {
            let empty = HashToBits::empty::<SW6FrParameters>(num_epochs);
            generate_random_parameters::<Bls12_377, _, _>(empty, rng).unwrap()
        };

        // Prover generates the input and the proof
        // Each message must be 384 bits.
        let (proof, input) = {
            let mut message_bits = Vec::new();
            for _ in 0..num_epochs {
                // say we have some input
                let mut input = vec![0; 64];
                rng.fill_bytes(&mut input);
                let bits = bytes_to_bits(&input, 384)
                    .iter()
                    .map(|b| Some(*b))
                    .collect::<Vec<_>>();
                message_bits.push(bits);
            }

            // generate the proof
            let circuit = HashToBits {
                message_bits: message_bits.clone(),
            };
            let proof = create_random_proof(circuit, &params, rng).unwrap();

            (proof, message_bits)
        };

        // verifier takes the input, hashes it and packs it
        // (both the crh and the xof bits are public inputs!)
        let public_inputs = {
            let mut message_bits = Vec::new();
            let mut xof_bits = Vec::new();
            for message in &input {
                let bits = message.iter().map(|m| m.unwrap()).collect::<Vec<_>>();
                xof_bits.extend_from_slice(&hash_to_bits_fn(&bits));
                message_bits.extend_from_slice(&bits);
            }
            // The public inputs are the CRH and XOF bits split in `Fr::CAPACITY` chunks
            // encoded in LE
            let packed_crh_bits = pack::<Fr, FrParameters>(&message_bits);
            let packed_xof_bits = pack::<Fr, FrParameters>(&xof_bits);
            [packed_crh_bits, packed_xof_bits].concat()
        };

        let pvk = prepare_verifying_key(&params.vk);
        assert!(verify_proof(&pvk, &proof, &public_inputs).unwrap());
    }
}
