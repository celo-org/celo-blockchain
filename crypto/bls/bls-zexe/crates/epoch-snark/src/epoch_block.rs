use super::encoding::{encode_public_key, encode_u16, encode_u32, EncodingError};
use algebra::{bls12_377::G1Projective, bls12_377::Parameters};
use blake2s_simd::Params;
use bls_crypto::{
    bls::keys::SIG_DOMAIN, curve::hash::HashToG1, CompositeHasher, PublicKey, Signature,
    TryAndIncrement,
};
use bls_gadgets::{bits_to_bytes, bytes_to_bits};

pub static OUT_DOMAIN: &[u8] = b"ULforout";

/// A header as parsed after being fetched from the Celo Blockchain
/// It contains information about the new epoch, as well as an aggregated
/// signature and bitmap from the validators from the previous block that
/// signed on it
#[derive(Clone)]
pub struct EpochTransition {
    /// The new epoch block which is being processed
    pub block: EpochBlock,
    /// The aggregate signature produced over the `EpochBlock`
    /// by the validators of the previous epoch
    pub aggregate_signature: Signature,
    /// The bitmap which determined the state transition
    pub bitmap: Vec<bool>,
}

#[derive(Clone)]
pub struct EpochBlock {
    pub index: u16,
    pub maximum_non_signers: u32,
    pub new_public_keys: Vec<PublicKey>,
}

impl EpochBlock {
    pub fn new(index: u16, maximum_non_signers: u32, new_public_keys: Vec<PublicKey>) -> Self {
        Self {
            index,
            maximum_non_signers,
            new_public_keys,
        }
    }

    pub fn hash_to_g1(&self) -> Result<G1Projective, EncodingError> {
        let input = self.encode_to_bytes()?;
        let composite_hasher = CompositeHasher::new().unwrap();
        let try_and_increment = TryAndIncrement::new(&composite_hasher);
        let expected_hash: G1Projective = try_and_increment
            .hash::<Parameters>(SIG_DOMAIN, &input, &[])
            .unwrap();
        Ok(expected_hash)
    }

    pub fn blake2(&self) -> Result<Vec<bool>, EncodingError> {
        Ok(hash_to_bits(&self.encode_to_bytes()?))
    }

    pub fn blake2_with_aggregated_pk(&self) -> Result<Vec<bool>, EncodingError> {
        Ok(hash_to_bits(&self.encode_to_bytes_with_aggregated_pk()?))
    }

    /// The goal of the validator diff encoding is to be a constant-size encoding so it would be
    /// more easily processable in SNARKs
    pub fn encode_to_bits(&self) -> Result<Vec<bool>, EncodingError> {
        let mut epoch_bits = vec![];
        epoch_bits.extend_from_slice(&encode_u16(self.index)?);
        epoch_bits.extend_from_slice(&encode_u32(self.maximum_non_signers)?);
        for added_public_key in &self.new_public_keys {
            epoch_bits.extend_from_slice(encode_public_key(&added_public_key)?.as_slice());
        }
        Ok(epoch_bits)
    }

    pub fn encode_to_bits_with_aggregated_pk(&self) -> Result<Vec<bool>, EncodingError> {
        let mut epoch_bits = self.encode_to_bits()?;
        let aggregated_pk = PublicKey::aggregate(&self.new_public_keys);
        epoch_bits.extend_from_slice(encode_public_key(&aggregated_pk)?.as_slice());
        Ok(epoch_bits)
    }

    pub fn encode_to_bytes(&self) -> Result<Vec<u8>, EncodingError> {
        Ok(bits_to_bytes(&self.encode_to_bits()?))
    }

    pub fn encode_to_bytes_with_aggregated_pk(&self) -> Result<Vec<u8>, EncodingError> {
        Ok(bits_to_bytes(&self.encode_to_bits_with_aggregated_pk()?))
    }
}

/// Serializes the first and last epoch to bytes, hashes them with Blake2 personalized to
/// `OUT_DOMAIN` and returns the LE bit representation
pub fn hash_first_last_epoch_block(
    first: &EpochBlock,
    last: &EpochBlock,
) -> Result<Vec<bool>, EncodingError> {
    let h1 = first.blake2()?;
    let h2 = last.blake2_with_aggregated_pk()?;
    Ok([h1, h2].concat())
}

/// Blake2 hash of the input personalized to `OUT_DOMAIN`
pub fn hash_to_bits(bytes: &[u8]) -> Vec<bool> {
    let hash = Params::new()
        .hash_length(32)
        .personal(OUT_DOMAIN)
        .to_state()
        .update(&bytes)
        .finalize()
        .as_ref()
        .to_vec();
    let mut bits = bytes_to_bits(&hash, 256);
    bits.reverse();
    bits
}
