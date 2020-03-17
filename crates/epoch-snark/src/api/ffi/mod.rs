pub mod utils;
use utils::{read_slice, EpochBlockFFI};

#[cfg(test)]
mod test_helpers;

use crate::api::{CPCurve, Parameters};
use crate::convert_result_to_bool;
use crate::epoch_block::{EpochBlock, EpochTransition};
use algebra::{bls12_377::G2Affine, AffineCurve, CanonicalDeserialize};
use std::convert::TryFrom;

#[no_mangle]
/// Verifies a Groth16 proof about the validity of the epoch transitions
/// between the provided `first_epoch` and `last_epoch` blocks.
///
/// All elements are assumed to be sent as serialized byte arrays
/// of **compressed elements**. There are no assumptions made about
/// the length of the verifying key or the proof, so that must be
/// provided by the caller.
///
/// # Safety
/// 1. VK and Proof must be valid pointers
/// 1. The vector of pubkeys inside EpochBlockFFI must point to valid memory
pub unsafe extern "C" fn verify(
    // Serialized verifying key
    vk: *const u8,
    // Length of serialized verifying key
    vk_len: u32,
    // Serialized proof
    proof: *const u8,
    // Length of serialized proof
    proof_len: u32,
    // First epoch data (pubkeys serialized)
    first_epoch: EpochBlockFFI,
    // Last epoch data (pubkeys serialized)
    last_epoch: EpochBlockFFI,
) -> bool {
    convert_result_to_bool(|| {
        let first_epoch = EpochBlock::try_from(&first_epoch)?;
        let last_epoch = EpochBlock::try_from(&last_epoch)?;
        let vk = read_slice(vk, vk_len as usize)?;
        let proof = read_slice(proof, proof_len as usize)?;

        super::verifier::verify(&vk, &first_epoch, &last_epoch, &proof)
    })
}
