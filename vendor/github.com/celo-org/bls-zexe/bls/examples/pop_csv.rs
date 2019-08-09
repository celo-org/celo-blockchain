use bls_zexe::{
    bls::keys::PrivateKey,
    curve::hash::try_and_increment::TryAndIncrement,
    hash::direct::DirectHasher,
};

use algebra::bytes::{ToBytes};

use clap::{App, Arg};
use rand::thread_rng;
use std::{
    io::Write,
    fs::File
};

fn main() {
    let matches = App::new("BLS Proof of Possession test vectors")
        .about("Generates many proof of posession for random keys")
        .arg(
            Arg::with_name("num")
                .short("n")
                .value_name("NUM")
                .help("Sets the number of test vectors")
                .required(true),
        )
        .arg(
            Arg::with_name("out")
                .short("o")
                .value_name("OUT")
                .help("Sets the output file path")
                .required(true),
        )
        .get_matches();

    let num: i32 = matches.value_of("num").unwrap().parse().unwrap();
    let out = matches.value_of("out").unwrap();

    let direct_hasher = DirectHasher::new().unwrap();
    let try_and_increment = TryAndIncrement::new(&direct_hasher);
    let rng = &mut thread_rng();
    let mut file = File::create(out).unwrap();
    for _ in 0..num {
        let sk = PrivateKey::generate(rng);
        let pop = sk.sign_pop(&try_and_increment).unwrap();
        let mut pop_bytes = vec![];
        pop.write(&mut pop_bytes).unwrap();

        let mut sk_bytes = vec![];
        sk.write(&mut sk_bytes).unwrap();
        let pk = sk.to_public();
        let mut pk_bytes = vec![];
        pk.write(&mut pk_bytes).unwrap();

        pk.verify_pop(&pop, &try_and_increment).unwrap();

        file.write_all(format!("{},{},{}\n", hex::encode(sk_bytes), hex::encode(pk_bytes), hex::encode(pop_bytes)).as_bytes()).unwrap();
    }
}
