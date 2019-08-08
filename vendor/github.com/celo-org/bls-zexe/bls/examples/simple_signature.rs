#[macro_use]
extern crate algebra;

use bls_zexe::{
    bls::keys::{PrivateKey, PublicKey, Signature},
    curve::hash::try_and_increment::TryAndIncrement,
    hash::composite::CompositeHasher,
};

use algebra::bytes::ToBytes;

use clap::{App, Arg};
use rand::thread_rng;

fn main() {
    let matches = App::new("SimpleAggregatedSignature")
        .about("Show an example of a simple signature with a random key")
        .arg(
            Arg::with_name("message")
                .short("m")
                .value_name("MESSAGE")
                .help("Sets the message to sign")
                .required(true),
        )
        .get_matches();

    let message = matches.value_of("message").unwrap();

    println!("matches: {}", message);

    let rng = &mut thread_rng();

    println!("rng");

    let composite_hasher = CompositeHasher::new().unwrap();
    println!("hasher");
    let try_and_increment = TryAndIncrement::new(&composite_hasher);
    println!("try_and_increment");
    let sk1 = PrivateKey::generate(rng);
    println!("sk1: {}", hex::encode(to_bytes!(sk1.get_sk()).unwrap()));
    let sk2 = PrivateKey::generate(rng);
    println!("sk2: {}", hex::encode(to_bytes!(sk2.get_sk()).unwrap()));
    let sk3 = PrivateKey::generate(rng);
    println!("sk3: {}", hex::encode(to_bytes!(sk3.get_sk()).unwrap()));

    println!("Starting!\n\n");

    let sig1 = sk1.sign(&message.as_bytes(), &[], &try_and_increment).unwrap();
    println!("sig1: {}", hex::encode(to_bytes!(sig1.get_sig()).unwrap()));
    let sig2 = sk2.sign(&message.as_bytes(), &[], &try_and_increment).unwrap();
    println!("sig2: {}", hex::encode(to_bytes!(sig2.get_sig()).unwrap()));
    let sig3 = sk3.sign(&message.as_bytes(), &[], &try_and_increment).unwrap();
    println!("sig3: {}", hex::encode(to_bytes!(sig3.get_sig()).unwrap()));

    let apk = PublicKey::aggregate(&[
        &sk1.to_public(),
        &sk2.to_public(),
        &sk3.to_public(),
        &sk3.to_public(),
    ]);
    println!("apk: {}", hex::encode(to_bytes!(apk.get_pk()).unwrap()));
    let asig1 = Signature::aggregate(&[&sig1, &sig3]);
    let asig2 = Signature::aggregate(&[&sig2, &sig3]);
    let asig = Signature::aggregate(&[&asig1, &asig2]);
    println!("asig: {}", hex::encode(to_bytes!(asig.get_sig()).unwrap()));
    apk.verify(&message.as_bytes(), &[], &asig, &try_and_increment)
        .unwrap();
    println!("aggregated signature verified successfully");
}
