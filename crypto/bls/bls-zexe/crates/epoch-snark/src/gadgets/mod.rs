mod epoch_data;
pub use epoch_data::EpochData;

mod hash_to_bits;
pub use hash_to_bits::HashToBits;

pub mod single_update;

mod pack;
pub use pack::MultipackGadget;

mod proof_of_compression;
pub use proof_of_compression::ProofOfCompression;

mod epochs;
pub use epochs::ValidatorSetUpdate;

// some helpers
use algebra::{bls12_377::Parameters, sw6::Fr, BigInteger, Field, FpParameters, PrimeField};
use r1cs_std::prelude::*;
use r1cs_std::{bls12_377::G2Gadget, fields::fp::FpGadget, Assignment};

type FrGadget = FpGadget<Fr>;
use bls_gadgets::YToBitGadget;

use r1cs_core::{ConstraintSystem, SynthesisError};

#[cfg(test)]
pub mod test_helpers {
    use super::*;
    use crate::epoch_block::EpochBlock;
    use algebra::{bls12_377::G1Projective, Bls12_377};
    use bls_crypto::{bls::keys::SIG_DOMAIN, CompositeHasher, PublicKey, TryAndIncrement};

    pub fn to_option_iter<T: Copy>(it: &[T]) -> Vec<Option<T>> {
        it.iter().map(|t| Some(*t)).collect()
    }

    pub fn hash_epoch(epoch: &EpochData<Bls12_377>) -> G1Projective {
        let mut pubkeys = Vec::new();
        for pk in &epoch.public_keys {
            pubkeys.push(PublicKey::from_pk(pk.unwrap()));
        }

        // Calculate the hash from our to_bytes function
        let epoch_bytes = EpochBlock::new(epoch.index.unwrap(), epoch.maximum_non_signers, pubkeys)
            .encode_to_bytes()
            .unwrap();
        let composite_hasher = CompositeHasher::new().unwrap();
        let try_and_increment = TryAndIncrement::new(&composite_hasher);
        let (hash, _) = try_and_increment
            .hash_with_attempt::<Parameters>(SIG_DOMAIN, &epoch_bytes, &[])
            .unwrap();

        hash
    }
}

pub fn pack<F: PrimeField, P: FpParameters>(values: &[bool]) -> Vec<F> {
    values
        .chunks(P::CAPACITY as usize)
        .map(|c| {
            let b = F::BigInt::from_bits(c);
            F::from_repr(b)
        })
        .collect::<Vec<_>>()
}

pub fn to_fr<T: Into<u64>, CS: ConstraintSystem<Fr>>(
    cs: &mut CS,
    num: Option<T>,
) -> Result<FrGadget, SynthesisError> {
    FrGadget::alloc(cs, || Ok(Fr::from(num.get()?.into())))
}

pub fn fr_to_bits<CS: ConstraintSystem<Fr>>(
    cs: &mut CS,
    input: &FrGadget,
    length: usize,
) -> Result<Vec<Boolean>, SynthesisError> {
    let mut input = input.to_bits(cs.ns(|| "input to bits"))?;
    input.reverse();
    Ok(input[0..length].to_vec())
}

pub fn g2_to_bits<CS: ConstraintSystem<Fr>>(
    cs: &mut CS,
    input: &G2Gadget,
) -> Result<Vec<Boolean>, SynthesisError> {
    let x_0 = input.x.c0.to_bits(cs.ns(|| "aggregated pub key c0 bits"))?;
    let x_1 = input.x.c1.to_bits(cs.ns(|| "aggregated pub key c1 bits"))?;
    let y_bit =
        YToBitGadget::<Parameters>::y_to_bit_g2(cs.ns(|| "aggregated pub key y bit"), &input)?;
    let mut output = Vec::new();
    output.extend_from_slice(&x_0);
    output.extend_from_slice(&x_1);
    output.push(y_bit);
    Ok(output)
}

pub fn constrain_bool<F: Field, CS: ConstraintSystem<F>>(
    cs: &mut CS,
    input: &[Option<bool>],
) -> Result<Vec<Boolean>, SynthesisError> {
    input
        .iter()
        .enumerate()
        .map(|(j, b)| Boolean::alloc(cs.ns(|| format!("{}", j)), || b.get()))
        .collect::<Result<Vec<_>, _>>()
}
