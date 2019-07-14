use bls_zexe::{
    bls::keys::PrivateKey,
    curve::hash::try_and_increment::TryAndIncrement,
    hash::direct::DirectHasher,
};

use algebra::bytes::{FromBytes, ToBytes};

use clap::{App, Arg};

fn main() {
    let matches = App::new("BLS Proof of Possession")
        .about("Generates a proof of posession for the given private key")
        .arg(
            Arg::with_name("key")
                .short("k")
                .value_name("KEY")
                .help("Sets the BLS private key")
                .required(true),
        )
        .get_matches();

    let key = matches.value_of("key").unwrap();

    let key_bytes = hex::decode(key).unwrap();

    let direct_hasher = DirectHasher::new().unwrap();
    let try_and_increment = TryAndIncrement::new(&direct_hasher);
    let sk = PrivateKey::read(key_bytes.as_slice()).unwrap();
    let pop = sk.sign_pop(&try_and_increment).unwrap();
    let mut pop_bytes = vec![];
    pop.write(&mut pop_bytes).unwrap();

    let pk = sk.to_public();
    pk.verify_pop(&pop, &try_and_increment).unwrap();

    let pop_hex = hex::encode(&pop_bytes);
    println!("{}", pop_hex);
}
