use algebra::{
    bls12_377::{Bls12_377, Fr as BlsFr, FrParameters as BlsFrParameters},
    sw6::{Fr, FrParameters},
    FpParameters,
};
use r1cs_std::bls12_377::PairingGadget;
use r1cs_std::prelude::*;

use r1cs_core::{ConstraintSystem, SynthesisError};

// Groth16 Specific imports
use crypto_primitives::{
    nizk::{
        constraints::NIZKVerifierGadget,
        groth16::{
            constraints::{Groth16VerifierGadget, ProofGadget, VerifyingKeyGadget},
            Groth16,
        },
    },
    prf::blake2s::{constraints::blake2s_gadget_with_parameters, Blake2sWithParameterBlock},
};

use groth16::{Proof, VerifyingKey};

pub static OUT_DOMAIN: &[u8] = b"ULforout";

use r1cs_std::fields::fp::FpGadget;
type FrGadget = FpGadget<Fr>;

use crate::gadgets::{HashToBits, MultipackGadget};

/// Parameters for compressing the public inputs of the R1CS system
pub struct ProofOfCompression {
    pub first_epoch_bits: Vec<Boolean>,
    pub last_epoch_bits: Vec<Boolean>,
    pub crh_bits: Vec<Boolean>,
    pub xof_bits: Vec<Boolean>,
}

impl ProofOfCompression {
    /// Compress the public inputs by verifying that the intermediate proofs are computed correctly
    /// and that the edges are also consistent
    pub fn compress_public_inputs<CS: ConstraintSystem<Fr>>(
        &self,
        cs: &mut CS,
        proof: &Proof<Bls12_377>,
        verifying_key: &VerifyingKey<Bls12_377>,
    ) -> Result<(), SynthesisError> {
        self.verify_proof(&mut cs.ns(|| "verify proof"), proof, verifying_key)?;
        self.verify_edges(&mut cs.ns(|| "verify edges"))?;
        Ok(())
    }

    fn verify_edges<CS: ConstraintSystem<Fr>>(
        &self,
        cs: &mut CS,
    ) -> Result<Vec<FrGadget>, SynthesisError> {
        // Verify the edges
        let mut xof_bits = vec![];
        let first_and_last_bits = [self.first_epoch_bits.clone(), self.last_epoch_bits.clone()];
        for (i, bits) in first_and_last_bits.iter().enumerate() {
            let mut message = bits.to_owned();
            message.reverse();
            let message_rounded_len = 8 * ((message.len() + 7) / 8);
            message.resize(message_rounded_len, Boolean::constant(false));

            let mut personalization = [0; 8];
            personalization.copy_from_slice(OUT_DOMAIN);

            let blake2s_parameters = Blake2sWithParameterBlock {
                digest_length: 32,
                key_length: 0,
                fan_out: 1,
                depth: 1,
                leaf_length: 0,
                node_offset: 0,
                xof_digest_length: 0,
                node_depth: 0,
                inner_length: 0,
                salt: [0; 8],
                personalization,
            };
            let xof_result = blake2s_gadget_with_parameters(
                cs.ns(|| format!("first and last xof result {}", i)),
                &message,
                &blake2s_parameters.parameters(),
            )?;
            let xof_bits_i = xof_result
                .into_iter()
                .map(|n| n.to_bits_le())
                .flatten()
                .collect::<Vec<Boolean>>();
            xof_bits.extend_from_slice(&xof_bits_i);
        }

        // Make the edges public inputs
        // packed over SW6 Fr.
        let packed = MultipackGadget::pack(
            cs.ns(|| "pack output hash"),
            &xof_bits,
            FrParameters::CAPACITY as usize,
            true,
        )?;

        Ok(packed)
    }

    /// Ensure that the intermediate BH and Blake2 hashes match
    fn verify_proof<CS: ConstraintSystem<Fr>>(
        &self,
        cs: &mut CS,
        proof: &Proof<Bls12_377>,
        verifying_key: &VerifyingKey<Bls12_377>,
    ) -> Result<(), SynthesisError> {
        // Verify the proof
        let proof =
            ProofGadget::<_, _, PairingGadget>::alloc(cs.ns(|| "alloc proof"), || Ok(proof))?;

        // Allocate the VK
        let verifying_key = VerifyingKeyGadget::<_, _, PairingGadget>::alloc(
            cs.ns(|| "allocate verifying key"),
            || Ok(verifying_key),
        )?;

        // The public inputs are the CRH and XOF bits split in `Fr::CAPACITY` chunks
        // encoded in LE
        let packed_crh_bits = le_chunks(&self.crh_bits, BlsFrParameters::CAPACITY);
        let packed_xof_bits = le_chunks(&self.xof_bits, BlsFrParameters::CAPACITY);

        let public_inputs: Vec<Vec<Boolean>> = [packed_crh_bits, packed_xof_bits].concat();

        <Groth16VerifierGadget<_, _, PairingGadget> as NIZKVerifierGadget<
            Groth16<Bls12_377, HashToBits, BlsFr>,
            Fr,
        >>::check_verify(
            cs.ns(|| "verify hash proof"),
            &verifying_key,
            public_inputs.iter(),
            &proof,
        )?;

        Ok(())
    }
}

fn le_chunks(iter: &[Boolean], chunk_size: u32) -> Vec<Vec<Boolean>> {
    iter.chunks(chunk_size as usize)
        .map(|b| {
            let mut b = b.to_vec();
            b.reverse();
            b
        })
        .collect::<Vec<_>>()
}

#[cfg(test)]
mod tests {
    use super::*;
    use bls_gadgets::bytes_to_bits;
    use rand::RngCore;

    use crate::epoch_block::hash_to_bits;
    use crate::gadgets::pack;
    use r1cs_std::test_constraint_system::TestConstraintSystem;

    fn to_bool(iter: &[bool]) -> Vec<Boolean> {
        iter.iter().map(|b| Boolean::constant(*b)).collect()
    }

    #[test]
    fn correct_blake2_hash() {
        let rng = &mut rand::thread_rng();
        let mut first_bytes = vec![0; 32];
        rng.fill_bytes(&mut first_bytes);
        let mut last_bytes = vec![0; 32];
        rng.fill_bytes(&mut last_bytes);

        let both_blake_bits = [first_bytes.clone(), last_bytes.clone()]
            .iter()
            .map(|b| hash_to_bits(b))
            .flatten()
            .collect::<Vec<bool>>();

        // encode each epoch's bytes to LE and pas them to the constraint system
        let first_epoch_bits = bytes_to_bits(&first_bytes, 256);
        let last_epoch_bits = bytes_to_bits(&last_bytes, 256);
        let poc = ProofOfCompression {
            crh_bits: vec![],
            xof_bits: vec![],
            first_epoch_bits: to_bool(&first_epoch_bits),
            last_epoch_bits: to_bool(&last_epoch_bits),
        };

        let mut cs = TestConstraintSystem::<Fr>::new();
        let packed = poc.verify_edges(&mut cs).unwrap();
        assert!(cs.is_satisfied());

        // get the inner packed value
        let inner = packed
            .into_iter()
            .map(|i| i.get_value().unwrap())
            .collect::<Vec<_>>();
        // pack our bits to Fr as well, and see if they match
        let public_inputs = pack::<Fr, FrParameters>(&both_blake_bits);
        assert_eq!(inner, public_inputs);
    }
}
