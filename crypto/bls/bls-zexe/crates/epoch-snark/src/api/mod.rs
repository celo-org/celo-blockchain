#![allow(unused)]
pub mod prover;
/// API for setting up, generating and verifying proofs for the
/// SNARK instantiated over BLS12-377 / SW6 which proves language
/// ...
pub mod setup;
pub mod verifier;

pub mod ffi;

use algebra::{bls12_377, sw6};

// Instantiate certain types to avoid confusion
pub type Parameters = setup::Parameters<CPCurve, BLSCurve>;

pub type BLSField = bls12_377::Fr;
pub type BLSCurve = bls12_377::Bls12_377;
pub type BLSFrParams = bls12_377::FrParameters;

pub type CPField = sw6::Fr;
pub type CPCurve = sw6::SW6;
pub type CPFrParams = sw6::FrParameters;
