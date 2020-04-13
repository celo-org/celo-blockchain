use bls_crypto::{
    hash_to_curve::try_and_increment::COMPOSITE_HASH_TO_G1, PrivateKey, PublicKey, Signature,
};

use algebra::{to_bytes, ToBytes};

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

    let try_and_increment = &*COMPOSITE_HASH_TO_G1;
    let sk1 = PrivateKey::generate(rng);
    println!("sk1: {}", hex::encode(to_bytes!(sk1.as_ref()).unwrap()));
    let sk2 = PrivateKey::generate(rng);
    println!("sk2: {}", hex::encode(to_bytes!(sk2.as_ref()).unwrap()));
    let sk3 = PrivateKey::generate(rng);
    println!("sk3: {}", hex::encode(to_bytes!(sk3.as_ref()).unwrap()));

    println!("Starting!\n\n");

    let sig1 = sk1
        .sign(&message.as_bytes(), &[], try_and_increment)
        .unwrap();
    println!("sig1: {}", hex::encode(to_bytes!(sig1.as_ref()).unwrap()));
    let sig2 = sk2
        .sign(&message.as_bytes(), &[], try_and_increment)
        .unwrap();
    println!("sig2: {}", hex::encode(to_bytes!(sig2.as_ref()).unwrap()));
    let sig3 = sk3
        .sign(&message.as_bytes(), &[], try_and_increment)
        .unwrap();
    println!("sig3: {}", hex::encode(to_bytes!(sig3.as_ref()).unwrap()));

    let apk = PublicKey::aggregate(&[
        sk1.to_public(),
        sk2.to_public(),
        sk3.to_public(),
        sk3.to_public(),
    ]);
    println!("apk: {}", hex::encode(to_bytes!(apk.as_ref()).unwrap()));
    let asig1 = Signature::aggregate(&[sig1, sig3.clone()]);
    let asig2 = Signature::aggregate(&[sig2, sig3]);
    let asig = Signature::aggregate(&[asig1, asig2]);
    println!("asig: {}", hex::encode(to_bytes!(asig.as_ref()).unwrap()));
    apk.verify(&message.as_bytes(), &[], &asig, try_and_increment)
        .unwrap();
    println!("aggregated signature verified successfully");
}
