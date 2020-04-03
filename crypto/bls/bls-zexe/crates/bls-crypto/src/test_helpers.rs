use algebra::{PairingEngine, ProjectiveCurve, UniformRand, Zero};

// Same RNG for all tests
pub fn rng() -> rand::rngs::ThreadRng {
    rand::thread_rng()
}

/// generate a keypair
pub fn keygen<E: PairingEngine>() -> (E::Fr, E::G2Projective) {
    let rng = &mut rng();
    let generator = E::G2Projective::prime_subgroup_generator();

    let secret_key = E::Fr::rand(rng);
    let pubkey = generator.mul(secret_key);
    (secret_key, pubkey)
}

/// generate N keypairs
pub fn keygen_mul<E: PairingEngine>(num: usize) -> (Vec<E::Fr>, Vec<E::G2Projective>) {
    let mut secret_keys = Vec::new();
    let mut public_keys = Vec::new();
    for _ in 0..num {
        let (secret_key, public_key) = keygen::<E>();
        secret_keys.push(secret_key);
        public_keys.push(public_key);
    }
    (secret_keys, public_keys)
}

/// generate N * sets of keypair vectors, each of size 3-7
#[allow(clippy::type_complexity)]
pub fn keygen_batch<E: PairingEngine>(
    batch_size: usize,
    num_per_batch: usize,
) -> (Vec<Vec<E::Fr>>, Vec<Vec<E::G2Projective>>) {
    let mut secret_keys = Vec::new();
    let mut public_keys = Vec::new();
    (0..batch_size).for_each(|_| {
        let (secret_keys_i, public_keys_i) = keygen_mul::<E>(num_per_batch);
        secret_keys.push(secret_keys_i);
        public_keys.push(public_keys_i);
    });
    (secret_keys, public_keys)
}

/// sum the elements in the provided slice
pub fn sum<P: ProjectiveCurve>(elements: &[P]) -> P {
    elements.iter().fold(P::zero(), |acc, key| acc + key)
}

/// N messages get signed by N committees of varying sizes
/// N aggregate signatures are returned
pub fn sign_batch<E: PairingEngine>(
    secret_keys: &[Vec<E::Fr>],
    messages: &[E::G1Projective],
) -> Vec<E::G1Projective> {
    secret_keys
        .iter()
        .zip(messages)
        .map(|(secret_keys, message)| {
            let (_, asig) = sign::<E>(*message, &secret_keys);
            asig
        })
        .collect::<Vec<_>>()
}

// signs a message with a vector of secret keys and returns the list of sigs + the agg sig
pub fn sign<E: PairingEngine>(
    message_hash: E::G1Projective,
    secret_keys: &[E::Fr],
) -> (Vec<E::G1Projective>, E::G1Projective) {
    let sigs = secret_keys
        .iter()
        .map(|key| message_hash.mul(*key))
        .collect::<Vec<_>>();
    let asig = sigs
        .iter()
        .fold(E::G1Projective::zero(), |acc, sig| acc + sig);
    (sigs, asig)
}
