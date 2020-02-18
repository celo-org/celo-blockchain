extern crate hex;

use crate::hash::XOF;
use super::direct::DirectHasher;

use algebra::{bytes::ToBytes, curves::edwards_bls12::EdwardsAffine as Edwards};
use blake2s_simd::Params;
use crypto_primitives::crh::{
    pedersen::PedersenWindow,
    bowe_hopwood::{BoweHopwoodPedersenCRH, BoweHopwoodPedersenParameters},
    FixedLengthCRH,
};
use rand::{Rng, SeedableRng};
use rand_chacha::ChaChaRng;

use std::error::Error;

type CRH = BoweHopwoodPedersenCRH<Edwards, Window>;
type CRHParameters = BoweHopwoodPedersenParameters<Edwards>;

#[derive(Clone)]
struct Window;

impl PedersenWindow for Window {
    const WINDOW_SIZE: usize = 4;
    const NUM_WINDOWS: usize = 9820; //(100*385+384*2+1)/4 ~ 9820 ~ 100*(Fq + sign bit) + Fq2 + sign bit
}

pub struct CompositeHasher {
    parameters: CRHParameters,
    direct_hasher: DirectHasher,
}

impl CompositeHasher {
    pub fn new() -> Result<CompositeHasher, Box<dyn Error>> {
        Ok(CompositeHasher {
            parameters: CompositeHasher::setup_crh()?,
            direct_hasher: DirectHasher{},
        })
    }
    fn prng() -> Result<impl Rng, Box<dyn Error>> {
        let hash_result = Params::new()
            .hash_length(32)
            .personal(b"UL_prngs") // personalization
            .to_state()
            .update(b"ULTRALIGHT PRNG SEED") // message
            .finalize()
            .as_ref()
            .to_vec();
        let mut seed = [0;32];
        seed.copy_from_slice(&hash_result[..32]);
        Ok(ChaChaRng::from_seed(seed))
    }

    fn setup_crh() -> Result<CRHParameters, Box<dyn Error>> {
        let mut rng = CompositeHasher::prng()?;
        CRH::setup::<_>(&mut rng)
    }
}

impl XOF for CompositeHasher {
    fn crh(&self, _: &[u8], message: &[u8], _: usize) -> Result<Vec<u8>, Box<dyn Error>> {
        let h = CRH::evaluate(&self.parameters, message)?;
        let mut res = vec![];
        h.x.write(&mut res)?;

        Ok(res)

    }

    fn xof(&self, domain: &[u8], hashed_message: &[u8], xof_digest_length: usize) -> Result<Vec<u8>, Box<dyn Error>> {
        self.direct_hasher.xof(domain, hashed_message, xof_digest_length)
    }

    fn hash(&self, domain: &[u8], message: &[u8], xof_digest_length: usize) -> Result<Vec<u8>, Box<dyn Error>> {
        let prepared_message = self.crh(domain, message, xof_digest_length)?;
        self.xof(domain, &prepared_message, xof_digest_length)
    }
}

#[cfg(test)]
mod test {
    use super::CompositeHasher as Hasher;
    use crate::hash::composite::CompositeHasher;
    use crate::hash::XOF;
    use rand::{Rng, SeedableRng};
    use rand_xorshift::XorShiftRng;

    #[test]
    fn test_crh_empty() {
        let msg: Vec<u8> = vec![];
        let hasher = Hasher::new().unwrap();
        let _result = hasher.crh(&[], &msg, 96).unwrap();
    }

    #[test]
    fn test_crh_random() {
        let hasher = Hasher::new().unwrap();
        let mut rng = XorShiftRng::from_seed([0x5d, 0xbe, 0x62, 0x59, 0x8d, 0x31, 0x3d, 0x76, 0x32, 0x37, 0xdb, 0x17, 0xe5, 0xbc, 0x06, 0x54]);
        let mut msg: Vec<u8> = vec![0; 32];
        for i in msg.iter_mut() {
            *i = rng.gen();
        }
        let _result = hasher.crh(&[],  &msg, 96).unwrap();
    }

    #[test]
    fn test_xof_random_768() {
        let hasher = Hasher::new().unwrap();
        let mut rng = XorShiftRng::from_seed([0x2d, 0xbe, 0x62, 0x59, 0x8d, 0x31, 0x3d, 0x76, 0x32, 0x37, 0xdb, 0x17, 0xe5, 0xbc, 0x06, 0x54]);
        let mut msg: Vec<u8> = vec![0; 32];
        for i in msg.iter_mut() {
            *i = rng.gen();
        }
        let result = hasher.crh(&[], &msg, 96).unwrap();
        let _xof_result = hasher.xof(b"ULforxof", &result, 768).unwrap();
    }

    #[test]
    fn test_xof_random_769() {
        let hasher = Hasher::new().unwrap();
        let mut rng = XorShiftRng::from_seed([0x0d, 0xbe, 0x62, 0x59, 0x8d, 0x31, 0x3d, 0x76, 0x32, 0x37, 0xdb, 0x17, 0xe5, 0xbc, 0x06, 0x54]);
        let mut msg: Vec<u8> = vec![0; 32];
        for i in msg.iter_mut() {
            *i = rng.gen();
        }
        let result = hasher.crh(&[], &msg, 96).unwrap();
        let _xof_result = hasher.xof(b"ULforxof", &result, 769).unwrap();
    }

    #[test]
    fn test_xof_random_96() {
        let hasher = Hasher::new().unwrap();
        let mut rng = XorShiftRng::from_seed([0x2d, 0xbe, 0x62, 0x59, 0x8d, 0x31, 0x3d, 0x76, 0x32, 0x37, 0xdb, 0x17, 0xe5, 0xbc, 0x06, 0x54]);
        let mut msg: Vec<u8> = vec![0; 32];
        for i in msg.iter_mut() {
            *i = rng.gen();
        }
        let result = hasher.crh(&[], &msg, 96).unwrap();
        let _xof_result = hasher.xof(b"ULforxof", &result, 96).unwrap();
    }

    #[test]
    fn test_hash_random() {
        let hasher = Hasher::new().unwrap();
        let mut rng = XorShiftRng::from_seed([0x2d, 0xbe, 0x62, 0x59, 0x8d, 0x31, 0x3d, 0x76, 0x32, 0x37, 0xdb, 0x17, 0xe5, 0xbc, 0x06, 0x54]);
        let mut msg: Vec<u8> = vec![0; 9820 * 4 / 8];
        for i in msg.iter_mut() {
            *i = rng.gen();
        }
        let _result = hasher.hash(b"ULforxof", &msg, 96).unwrap();
    }

    #[test]
    #[should_panic]
    fn test_invalid_message() {
        let hasher = Hasher::new().unwrap();
        let mut rng = XorShiftRng::from_seed([0x2d, 0xbe, 0x62, 0x59, 0x8d, 0x31, 0x3d, 0x76, 0x32, 0x37, 0xdb, 0x17, 0xe5, 0xbc, 0x06, 0x54]);
        let mut msg: Vec<u8> = vec![0; 100000];
        for i in msg.iter_mut() {
            *i = rng.gen();
        }
        let _result = hasher.hash(b"ULforxof", &msg, 96).unwrap();
    }

    /*
    #[test]
    fn print_pedersen_bases() {
        let hasher = CompositeHasher::new().unwrap();
        let mut x_co = Vec::new();
        let mut y_co = Vec::new();


        for i in 1..9000 {
            let mut res = vec![];
            hasher.parameters.generators[i-1][0]
                .x
                .write(&mut res)
                .unwrap();
            res.reverse();

            x_co.push(hex::encode(&res));

            let mut res = vec![];
            hasher.parameters.generators[i-1][0]
                .y
                .write(&mut res)
                .unwrap();
            res.reverse();
            y_co.push(hex::encode(&res));

        }
        println!("x_co: {:?}", x_co);
        println!("y_co: {:?}", y_co);
    }

    #[test]
    fn test_pedersen_test_vectors() {
        let hasher = CompositeHasher::new().unwrap();
        let path = Path::new("test_utils/test_vec.csv");
        let file = File::open(path).unwrap();
        let mut rdr =csv::ReaderBuilder::new().has_headers(false).from_reader(file);
        for record in rdr.records() {
            let r = &record.unwrap();
            let row: String = r[0].to_string();
            let actual_hash: String = r[1].to_string();

            let msg = hex::decode(&row).unwrap();
            let mut test_hash = hasher.crh(&[], &msg, 96).unwrap();
            test_hash.reverse();
            let msg_hash = hex::encode(&test_hash);

            if msg_hash.to_string() != actual_hash {
                println!("msg: {}", row);
            }
            assert_eq!( msg_hash.to_string() , actual_hash );
        }
    }
    */

    #[test]
    fn test_crh_print() {
        let hasher = CompositeHasher::new().unwrap();

        let hex_msg = hex::encode(&[0b10]);
        println!("{}",  hex_msg.to_string());
        let to_hash = hex::decode(&hex_msg).unwrap();
        println!("{:?}", to_hash);
        let mut test_hash = hasher.crh(&[], &to_hash, 96).unwrap();
        test_hash.reverse();
        let hex_hash = hex::encode(&test_hash);
        println!("{}", hex_hash.to_string() );
    }
}
