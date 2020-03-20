use epoch_snark::api::{prover, setup, verifier};
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
    let faults = (num_validators - 1) / 3;

    // Trusted setup
    let params = setup::trusted_setup(num_validators, num_epochs, faults, rng).unwrap();

    // Create the state to be proven (first - last and in between)
    // Note: This is all data which should be fetched via the Celo blockchain
    let (first_epoch, transitions, last_epoch) =
        generate_test_data(num_validators, faults, num_epochs);

    // Prover generates the proof given the params
    let proof = prover::prove(
        &params,
        num_validators as u32,
        &first_epoch,
        &transitions,
        rng,
    )
    .unwrap();

    // Verifier checks the proof
    let res = verifier::verify(params.vk().0, &first_epoch, &last_epoch, proof);
    assert!(res.is_ok());
}
