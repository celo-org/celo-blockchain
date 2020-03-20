/// Prover Verifier Generator
///
/// Setup: Trusted setup over Groth16 for the Hash To Bits and the Epoch Transition circuits
use std::marker::PhantomData;

use crate::gadgets::{HashToBits, ValidatorSetUpdate};

use algebra::PairingEngine;
use r1cs_core::ConstraintSynthesizer;
use r1cs_core::SynthesisError;
use rand::Rng;

use super::{BLSCurve, CPCurve, CPFrParams};

use groth16::{generate_random_parameters, Parameters as Groth16Parameters, VerifyingKey};

type Result<T> = std::result::Result<T, SynthesisError>;

pub struct Parameters<CP: PairingEngine, BLS: PairingEngine> {
    pub hash_to_bits: Groth16Parameters<BLS>,
    pub epochs: Groth16Parameters<CP>,
}

impl<CP: PairingEngine, BLS: PairingEngine> Parameters<CP, BLS> {
    pub fn vk(&self) -> (&VerifyingKey<CP>, &VerifyingKey<BLS>) {
        (&self.epochs.vk, &self.hash_to_bits.vk)
    }
}

/// Initializes the Hash To Bits and Validator Set Update circuits with random parameters
/// seeded by the provided RNG over BLS12-377 and SW6.
pub fn trusted_setup<R: Rng>(
    num_validators: usize,
    num_epochs: usize,
    maximum_non_signers: usize,
    rng: &mut R,
) -> Result<Parameters<CPCurve, BLSCurve>> {
    setup(
        num_validators,
        num_epochs,
        maximum_non_signers,
        rng,
        |c, rng| generate_random_parameters(c, rng),
        |c, rng| generate_random_parameters(c, rng),
    )
}

#[cfg(test)]
mod tests {
    use super::*;
    #[test]
    fn runs_setup() {
        let rng = &mut rand::thread_rng();
        assert!(trusted_setup(3, 2, 1, rng).is_ok())
    }
}

/// Performs a Groth16 setup over the 2 provided Pairing-friendly curves for the Hash to Bits and Validator set update circuits
/// The consumer may provide the setup function, which can be one which performs a private-trusted setup, or one which outputs
/// parameters which were computed via an [MPC](https://eprint.iacr.org/2017/1050)
///
/// If you do not know what this means, use the `trusted_setup` function
fn setup<CP, BLS, F, G, R>(
    num_validators: usize,
    num_epochs: usize,
    maximum_non_signers: usize,
    rng: &mut R,
    hash_to_bits_setup: F,
    validator_setup_fn: G,
) -> Result<Parameters<CP, BLS>>
where
    CP: PairingEngine,
    BLS: PairingEngine,
    R: Rng,
    F: FnOnce(HashToBits, &mut R) -> Result<Groth16Parameters<BLS>>,
    G: FnOnce(ValidatorSetUpdate<BLS>, &mut R) -> Result<Groth16Parameters<CP>>,
{
    let empty_hash_to_bits = HashToBits::empty::<CPFrParams>(num_epochs);
    let hash_to_bits = hash_to_bits_setup(empty_hash_to_bits, rng)?;

    let empty_epochs = ValidatorSetUpdate::empty(
        num_validators,
        num_epochs,
        maximum_non_signers,
        hash_to_bits.vk.clone(),
    );
    let epochs = validator_setup_fn(empty_epochs, rng)?;

    Ok(Parameters {
        hash_to_bits,
        epochs,
    })
}
