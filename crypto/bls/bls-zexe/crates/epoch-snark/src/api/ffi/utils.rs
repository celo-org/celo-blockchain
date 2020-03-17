use super::{CPCurve, Parameters};
use crate::convert_result_to_bool;
use crate::epoch_block::{EpochBlock, EpochTransition};
use crate::EncodingError;
use algebra::{
    bls12_377::G2Affine, AffineCurve, CanonicalDeserialize, CanonicalSerialize, ProjectiveCurve,
};
use bls_crypto::PublicKey;
use groth16::{Proof, VerifyingKey};
use std::convert::TryFrom;
use std::slice;

/// Each pubkey is a BLS G2Projective element
const PUBKEY_BYTES: usize = 96;

/// Data structure received from consumers of the FFI interface describing
/// an epoch block.
#[repr(C)]
pub struct EpochBlockFFI {
    /// The epoch's index
    pub index: u16,
    /// Pointer to the public keys array
    pub pubkeys: *const u8,
    /// The number of public keys to be read from the pointer
    pub pubkeys_num: usize,
    /// Maximum number of non signers for that epoch
    pub maximum_non_signers: u32,
}

impl TryFrom<&EpochBlockFFI> for EpochBlock {
    type Error = EncodingError;

    fn try_from(src: &EpochBlockFFI) -> Result<EpochBlock, Self::Error> {
        let pubkeys = unsafe { read_pubkeys(src.pubkeys, src.pubkeys_num as usize)? };
        Ok(EpochBlock {
            index: src.index,
            maximum_non_signers: src.maximum_non_signers,
            new_public_keys: pubkeys,
        })
    }
}

/// Reads `len` bytes starting from the pointer's location
///
/// # Safety
///
/// This WILL read invalid data if you give it a larger `num` argument
/// than expected. Use with caution.
pub(super) fn read_slice<C: CanonicalDeserialize>(
    ptr: *const u8,
    len: usize,
) -> Result<C, EncodingError> {
    let mut data = unsafe { slice::from_raw_parts(ptr, len) };
    Ok(C::deserialize(&mut data)?)
}

/// Reads `num` * `PUBKEY_BYTES` bytes starting from the pointer's location
///
/// # Safety
///
/// This WILL read invalid data if you give it a larger `num` argument
/// than expected. Use with caution.
unsafe fn read_serialized_pubkeys<'a>(ptr: *const u8, num: usize) -> &'a [u8] {
    slice::from_raw_parts(ptr, num * PUBKEY_BYTES)
}

/// Serializes the inner G2 elements of the pubkeys to a vector
pub fn serialize_pubkeys(pubkeys: &[PublicKey]) -> Result<Vec<u8>, EncodingError> {
    let mut v = Vec::new();
    for p in pubkeys {
        p.get_pk().into_affine().serialize(&mut v)?
    }
    Ok(v)
}

/// Reads `num` PublicKey elements starting from the memory that the pointer points to.
///
/// # Safety
/// This WILL NOT fail if the `num` variable is larger than the expected elements, and will
/// simply return an array of `PublicKeys` whose internals will be whatever data was in the memory.
/// Use with caution.
unsafe fn read_pubkeys(ptr: *const u8, num: usize) -> Result<Vec<PublicKey>, EncodingError> {
    let mut data = unsafe { read_serialized_pubkeys(ptr, num) };
    let mut pubkeys = Vec::new();
    for _ in 0..num {
        let key = G2Affine::deserialize(&mut data)?;
        let key = key.into_projective();
        pubkeys.push(PublicKey::from_pk(key))
    }
    Ok(pubkeys)
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::api::ffi::test_helpers::TestCircuit;
    use algebra::{
        bls12_377::{Fr, G2Projective},
        serialize::CanonicalSerialize,
        Bls12_377, ProjectiveCurve, UniformRand,
    };
    use groth16::{create_proof_no_zk, generate_random_parameters};

    #[test]
    fn ffi_block_conversion() {
        let num_keys = 10;
        let pubkeys = rand_pubkeys(num_keys);
        let block = EpochBlock {
            index: 1,
            maximum_non_signers: 19,
            new_public_keys: pubkeys,
        };
        let src = block;
        let serialized_pubkeys = serialize_pubkeys(&src.new_public_keys).unwrap();
        let ffi_block = EpochBlockFFI {
            index: src.index,
            maximum_non_signers: src.maximum_non_signers,
            pubkeys_num: src.new_public_keys.len(),
            pubkeys: &serialized_pubkeys[0] as *const u8,
        };
        let block_from_ffi = EpochBlock::try_from(&ffi_block).unwrap();
        assert_eq!(block_from_ffi, src);
    }

    #[test]
    fn groth_verifying_key_from_pointer() {
        let rng = &mut rand::thread_rng();
        let c = TestCircuit::<Bls12_377>(None);
        let params = generate_random_parameters(c, rng).unwrap();
        let vk = params.vk;
        let mut serialized = vec![];
        vk.serialize(&mut serialized).unwrap();
        let ptr = &serialized[0] as *const u8;
        let deserialized: VerifyingKey<Bls12_377> =
            unsafe { read_slice(ptr, serialized.len()).unwrap() };
        assert_eq!(deserialized, vk);

        // reading a bigger slice is fine
        let deserialized: VerifyingKey<Bls12_377> =
            unsafe { read_slice(ptr, 2 * serialized.len()).unwrap() };
        assert_eq!(deserialized, vk);

        // reading a smaller slice is not
        unsafe { read_slice::<VerifyingKey<Bls12_377>>(ptr, serialized.len() - 1).unwrap_err() };
    }

    #[test]
    fn groth_proof_from_pointer() {
        let rng = &mut rand::thread_rng();
        let c = TestCircuit::<Bls12_377>(None);
        let params = generate_random_parameters(c, rng).unwrap();
        let c = TestCircuit::<Bls12_377>(Some(Fr::rand(rng)));
        let proof = create_proof_no_zk(c, &params).unwrap();
        let mut serialized = vec![];
        proof.serialize(&mut serialized).unwrap();
        let ptr = &serialized[0] as *const u8;
        let deserialized: Proof<Bls12_377> = unsafe { read_slice(ptr, serialized.len()).unwrap() };
        assert_eq!(deserialized, proof);

        // reading a bigger slice is fine (although still mis-use of the code)
        let deserialized: Proof<Bls12_377> =
            unsafe { read_slice(ptr, 2 * serialized.len()).unwrap() };
        assert_eq!(deserialized, proof);

        // reading a smaller slice is not
        unsafe { read_slice::<Proof<Bls12_377>>(ptr, serialized.len() - 1).unwrap_err() };
    }

    #[test]
    fn pubkeys_from_pointer() {
        let num_keys = 10;
        let pubkeys = rand_pubkeys(num_keys);
        let serialized = serialize_pubkeys(&pubkeys).unwrap();
        let ptr = &serialized[0] as *const u8;
        let deserialized_from_ptr = unsafe { read_pubkeys(ptr, num_keys).unwrap() };
        assert_eq!(deserialized_from_ptr, pubkeys);
    }

    #[test]
    fn invalid_pubkey_len_panic() {
        let num_keys = 10;
        let pubkeys = rand_pubkeys(num_keys);
        let serialized = serialize_pubkeys(&pubkeys).unwrap();
        let ptr = &serialized[0] as *const u8;
        // We read a bunch of junk data, hence why this MUST
        // be unsafe :)
        unsafe { read_pubkeys(ptr, 99).unwrap_err() };
    }

    fn rand_pubkeys(num_keys: usize) -> Vec<PublicKey> {
        let rng = &mut rand::thread_rng();
        let mut points = (0..num_keys)
            .map(|_| G2Projective::rand(rng))
            .collect::<Vec<_>>();
        // for the purposes of the test, we'll normalize these points to compare them with affine ones
        // which are already normalized due to the `into_affine()` method
        G2Projective::batch_normalization(&mut points);
        points
            .iter()
            .map(|p| PublicKey::from_pk(*p))
            .collect::<Vec<_>>()
    }
}
