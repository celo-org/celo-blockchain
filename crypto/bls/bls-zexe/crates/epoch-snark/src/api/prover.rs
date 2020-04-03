use super::{BLSCurve, CPCurve, Parameters};
use crate::{
    api::CPField,
    epoch_block::{EpochBlock, EpochTransition},
    gadgets::{
        epochs::{HashToBitsHelper, ValidatorSetUpdate},
        single_update::SingleUpdate,
        EpochData, HashToBits,
    },
};
use bls_crypto::{
    bls::keys::SIG_DOMAIN, curve::hash::try_and_increment::TryAndIncrement, hash::composite::CRH,
    hash::XOF, CompositeHasher, PublicKey, Signature,
};
use bls_gadgets::bytes_to_bits;

use algebra::{bls12_377::G1Projective, Zero};
use groth16::{create_proof_no_zk, Parameters as Groth16Parameters, Proof as Groth16Proof};
use r1cs_core::{ConstraintSynthesizer, SynthesisError};

use tracing::{debug, error, info, span, warn, Level};

pub fn prove(
    parameters: &Parameters,
    num_validators: u32,
    initial_epoch: &EpochBlock,
    transitions: &[EpochTransition],
) -> Result<Groth16Proof<CPCurve>, SynthesisError> {
    info!(
        "Generating proof for {} epochs (first epoch: {}, {} validators per epoch)",
        transitions.len(),
        initial_epoch.index,
        num_validators,
    );

    let span = span!(Level::TRACE, "prove");
    let _enter = span.enter();

    let epochs = transitions
        .iter()
        .map(|transition| to_update(transition))
        .collect::<Vec<_>>();

    // Generate a helping proof if a Proving Key for the HashToBits
    // circuit was provided
    let hash_helper = if let Some(ref params) = parameters.hash_to_bits {
        Some(generate_hash_helper(&params, transitions)?)
    } else {
        None
    };

    // Generate the BLS proof
    let asig = transitions.iter().fold(G1Projective::zero(), |acc, epoch| {
        acc + epoch.aggregate_signature.get_sig()
    });

    let circuit = ValidatorSetUpdate::<BLSCurve> {
        initial_epoch: to_epoch_data(initial_epoch),
        epochs,
        aggregated_signature: Some(asig),
        num_validators,
        hash_helper,
    };
    info!("BLS");
    let bls_proof = create_proof_no_zk(circuit, &parameters.epochs)?;

    Ok(bls_proof)
}

/// Helper which creates the hashproof inside BLS12-377
pub fn generate_hash_helper(
    params: &Groth16Parameters<BLSCurve>,
    transitions: &[EpochTransition],
) -> Result<HashToBitsHelper<BLSCurve>, SynthesisError> {
    let composite_hasher = CompositeHasher::new().unwrap();
    let try_and_increment = TryAndIncrement::new(&composite_hasher);

    // Generate the CRH per epoch
    let message_bits = transitions
        .iter()
        .map(|transition| {
            let block = &transition.block;
            let epoch_bytes = block.encode_to_bytes().unwrap();

            // We need to find the counter so that the CRH hash we use will eventually result on an element on the curve
            let (_, counter) = try_and_increment
                .hash_with_attempt::<algebra::bls12_377::Parameters>(SIG_DOMAIN, &epoch_bytes, &[])
                .unwrap();
            let crh_bytes = composite_hasher
                .crh(&[], &[&[counter as u8][..], &epoch_bytes].concat(), 0)
                .unwrap();
            // The verifier should run both the crh and the xof here to generate a
            // valid statement for the verify
            bytes_to_bits(&crh_bytes, 384)
                .iter()
                .map(|b| Some(*b))
                .collect()
        })
        .collect::<Vec<_>>();

    // Generate proof of correct calculation of the CRH->Blake hashes
    // to make Hash to G1 cheaper
    let circuit = HashToBits { message_bits };
    info!("CRH->XOF");
    let hash_proof = create_proof_no_zk(circuit, params)?;

    Ok(HashToBitsHelper {
        proof: hash_proof,
        verifying_key: params.vk.clone(),
    })
}

pub fn to_epoch_data(block: &EpochBlock) -> EpochData<BLSCurve> {
    EpochData {
        index: Some(block.index),
        maximum_non_signers: block.maximum_non_signers,
        public_keys: block
            .new_public_keys
            .iter()
            .map(|pubkey| Some(pubkey.get_pk()))
            .collect(),
    }
}

pub fn to_update(transition: &EpochTransition) -> SingleUpdate<BLSCurve> {
    SingleUpdate {
        epoch_data: to_epoch_data(&transition.block),
        signed_bitmap: transition
            .bitmap
            .iter()
            .map(|b| Some(*b))
            .collect::<Vec<_>>(),
    }
}
