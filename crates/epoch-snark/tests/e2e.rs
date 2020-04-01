use algebra::CanonicalSerialize;
use epoch_snark::api::{ffi, prover, setup, verifier};

mod fixtures;
use fixtures::generate_test_data;

#[test]
#[ignore] // This test makes CI run out of memory and takes too long. It works though!
fn prover_verifier_groth16() {
    let rng = &mut rand::thread_rng();
    let num_transitions = 2;
    let faults = 1;
    let num_validators = 3 * faults + 1;

    let hashes_in_bls12_377 = true;

    // Trusted setup
    let params = setup::trusted_setup(
        num_validators,
        num_transitions,
        faults,
        rng,
        hashes_in_bls12_377,
    )
    .unwrap();

    // Create the state to be proven (first epoch + `num_transitions` transitions.
    // Note: This is all data which should be fetched via the Celo blockchain
    let (first_epoch, transitions, last_epoch) =
        generate_test_data(num_validators, faults, num_transitions);

    // Prover generates the proof given the params
    let proof = prover::prove(&params, num_validators as u32, &first_epoch, &transitions).unwrap();

    // Verifier checks the proof
    let res = verifier::verify(&params.epochs.vk, &first_epoch, &last_epoch, &proof);
    assert!(res.is_ok());

    // Serialize the proof / vk
    let mut serialized_vk = vec![];
    params.epochs.vk.serialize(&mut serialized_vk).unwrap();
    let mut serialized_proof = vec![];
    proof.serialize(&mut serialized_proof).unwrap();
    dbg!(hex::encode(&serialized_vk));
    dbg!(hex::encode(&serialized_proof));

    // Get the corresponding pointers
    let proof_ptr = &serialized_proof[0] as *const u8;
    let vk_ptr = &serialized_vk[0] as *const u8;

    let serialized_pubkeys = ffi::utils::serialize_pubkeys(&first_epoch.new_public_keys).unwrap();
    dbg!(hex::encode(&serialized_pubkeys));
    let first_epoch = ffi::utils::EpochBlockFFI {
        index: first_epoch.index,
        maximum_non_signers: first_epoch.maximum_non_signers,
        pubkeys_num: first_epoch.new_public_keys.len(),
        pubkeys: &serialized_pubkeys[0] as *const u8,
    };

    let serialized_pubkeys = ffi::utils::serialize_pubkeys(&last_epoch.new_public_keys).unwrap();
    dbg!(hex::encode(&serialized_pubkeys));
    let last_epoch = ffi::utils::EpochBlockFFI {
        index: last_epoch.index,
        maximum_non_signers: last_epoch.maximum_non_signers,
        pubkeys_num: last_epoch.new_public_keys.len(),
        pubkeys: &serialized_pubkeys[0] as *const u8,
    };

    // Make the verification
    let res = unsafe {
        ffi::verify(
            vk_ptr,
            serialized_vk.len() as u32,
            proof_ptr,
            serialized_proof.len() as u32,
            first_epoch,
            last_epoch,
        )
    };
    assert!(res);
}
