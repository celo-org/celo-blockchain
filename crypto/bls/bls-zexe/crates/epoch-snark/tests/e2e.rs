use epoch_snark::api::{prover, setup, verifier};

mod fixtures;
use fixtures::generate_test_data;

#[test]
#[ignore] // This test makes CI run out of memory and takes too long. It works though!
fn prover_verifier_groth16() {
    let rng = &mut rand::thread_rng();
    let num_epochs = 2;
    let faults = 1;
    let num_validators = 3 * faults + 1;

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
