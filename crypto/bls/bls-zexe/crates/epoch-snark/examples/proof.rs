use epoch_snark::api::{prover, setup, verifier};
use std::env;

#[path = "../tests/fixtures.rs"]
mod fixtures;
use fixtures::generate_test_data;

use tracing_subscriber::{
    filter::EnvFilter,
    fmt::{time::ChronoUtc, Subscriber},
};

use bench_utils::{end_timer, start_timer};

fn main() {
    Subscriber::builder()
        .with_timer(ChronoUtc::rfc3339())
        .with_env_filter(EnvFilter::from_default_env())
        .init();

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

    // Trusted setup
    let time = start_timer!(|| "Trusted setup");
    let params =
        setup::trusted_setup(num_validators, num_epochs, faults, rng, hashes_in_bls12_377).unwrap();
    end_timer!(time);

    // Create the state to be proven (first - last and in between)
    // Note: This is all data which should be fetched via the Celo blockchain
    let (first_epoch, transitions, last_epoch) =
        generate_test_data(num_validators, faults, num_epochs);

    // Prover generates the proof given the params
    let time = start_timer!(|| "Generate proof");
    let proof = prover::prove(&params, num_validators as u32, &first_epoch, &transitions).unwrap();
    end_timer!(time);

    // Verifier checks the proof
    let time = start_timer!(|| "Verify proof");
    let res = verifier::verify(&params.epochs.vk, &first_epoch, &last_epoch, &proof);
    end_timer!(time);
    assert!(res.is_ok());
}
