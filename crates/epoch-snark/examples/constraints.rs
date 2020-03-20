use algebra::{bls12_377::G1Projective, Zero};
use epoch_snark::{
    api::{prover, BLSCurve, CPFrParams},
    gadgets::{HashToBits, ValidatorSetUpdate},
};
use groth16::generate_random_parameters;
use r1cs_core::ConstraintSynthesizer;
use r1cs_std::test_constraint_system::TestConstraintSystem;
use std::env;

#[path = "../tests/fixtures.rs"]
mod fixtures;
use fixtures::generate_test_data;

fn main() {
    let rng = &mut rand::thread_rng();
    let mut args = env::args();
    args.next().unwrap(); // discard the program name
    let num_validators = args
        .next()
        .expect("num validators was expected")
        .parse()
        .expect("NaN");
    let num_epochs = args
        .next()
        .expect("num epochs was expected")
        .parse()
        .expect("NaN");
    let hashes_in_bls12_377: bool = args
        .next()
        .expect("expected flag for generating or not constraints inside BLS12_377")
        .parse()
        .expect("not a bool");
    let faults = (num_validators - 1) / 3;

    // Make a random initialization of the circuit
    let (first_epoch, transitions, _) = generate_test_data(num_validators, faults, num_epochs);

    // Trusted setup for the HashToBits circuit if required
    let hash_helper = if hashes_in_bls12_377 {
        let empty_hash_to_bits = HashToBits::empty::<CPFrParams>(num_epochs);
        let params = generate_random_parameters(empty_hash_to_bits, rng).unwrap();
        let helper = prover::generate_hash_helper(&params, &transitions).unwrap();
        Some(helper)
    } else {
        None
    };

    let asig = transitions.iter().fold(G1Projective::zero(), |acc, epoch| {
        acc + epoch.aggregate_signature.get_sig()
    });
    let epochs = transitions
        .iter()
        .map(|transition| prover::to_update(transition))
        .collect::<Vec<_>>();
    let circuit = ValidatorSetUpdate::<BLSCurve> {
        initial_epoch: prover::to_epoch_data(&first_epoch),
        epochs,
        aggregated_signature: Some(asig),
        num_validators: num_validators as u32,
        hash_helper,
    };

    // generate test constraints and log them
    let mut cs = TestConstraintSystem::new();
    circuit.generate_constraints(&mut cs).unwrap();

    println!(
        "Number of constraints for {} epochs ({} validators, hashes in BLS12-377 {}): {}",
        num_epochs,
        num_validators,
        hashes_in_bls12_377,
        cs.num_constraints()
    )
}
