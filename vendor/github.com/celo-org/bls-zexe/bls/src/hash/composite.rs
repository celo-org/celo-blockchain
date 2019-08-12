extern crate hex;

use crate::hash::PRF;
use super::direct::DirectHasher;

use algebra::{bytes::ToBytes, curves::edwards_sw6::EdwardsAffine as Edwards};
use blake2s_simd::Params;
use dpc::crypto_primitives::crh::{
    pedersen::{PedersenCRH, PedersenParameters, PedersenWindow},
    FixedLengthCRH,
};
use rand::{chacha::ChaChaRng, Rng, SeedableRng};

use byteorder::{LittleEndian, ReadBytesExt};
use std::error::Error;

type CRH = PedersenCRH<Edwards, Window>;
type CRHParameters = PedersenParameters<Edwards>;

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
        let mut seed = vec![];
        for i in 0..hash_result.len() / 4 {
            let mut buf = &hash_result[i..i + 4];
            let num = buf.read_u32::<LittleEndian>()?;
            seed.push(num);
        }
        Ok(ChaChaRng::from_seed(&seed))
    }

    fn setup_crh() -> Result<CRHParameters, Box<dyn Error>> {
        let mut rng = CompositeHasher::prng()?;
        CRH::setup::<_>(&mut rng)
    }
}

impl PRF for CompositeHasher {
    fn crh(&self, message: &[u8]) -> Result<Vec<u8>, Box<dyn Error>> {
        let h = CRH::evaluate(&self.parameters, message)?;
        let mut res = vec![];
        h.x.write(&mut res)?;

        Ok(res)

    }

    fn prf(&self, key: &[u8], domain: &[u8], hashed_message: &[u8], output_size_in_bits: usize) -> Result<Vec<u8>, Box<dyn Error>> {
        self.direct_hasher.prf(key, domain, hashed_message, output_size_in_bits)
    }

    fn hash(&self, key: &[u8], domain: &[u8], message: &[u8], output_size_in_bits: usize) -> Result<Vec<u8>, Box<dyn Error>> {
        let prepared_message = self.crh(message)?;
        self.prf(key, domain, &prepared_message, output_size_in_bits)
    }
}

#[cfg(test)]
mod test {
    use super::CompositeHasher as Hasher;
    use crate::hash::composite::CompositeHasher;
    use crate::hash::PRF;
    use algebra::bytes::ToBytes;
    use rand::{Rng, SeedableRng, XorShiftRng};
    use std::fs::File;
    use std::path::Path;

    #[test]
    fn test_crh_empty() {
        let msg: Vec<u8> = vec![];
        let hasher = Hasher::new().unwrap();
        let _result = hasher.crh(&msg).unwrap();
    }

    #[test]
    fn test_crh_random() {
        let hasher = Hasher::new().unwrap();
        let mut rng = XorShiftRng::from_seed([0x5dbe6259, 0x8d313d76, 0x3237db17, 0xe5bc0654]);
        let mut msg: Vec<u8> = vec![0; 32];
        for i in msg.iter_mut() {
            *i = rng.gen();
        }
        let _result = hasher.crh(&msg).unwrap();
    }

    #[test]
    fn test_prf_random_768() {
        let hasher = Hasher::new().unwrap();
        let mut rng = XorShiftRng::from_seed([0x2dbe6259, 0x8d313d76, 0x3237db17, 0xe5bc0654]);
        let mut msg: Vec<u8> = vec![0; 32];
        for i in msg.iter_mut() {
            *i = rng.gen();
        }
        let result = hasher.crh(&msg).unwrap();
        let _prf_result = hasher.prf(b"096b36a5804bfacef1691e173c366a47ff5ba84a44f26ddd7e8d9f79d5b42df0",b"ULforprf", &result, 768).unwrap();
    }

    #[test]
    fn test_prf_random_769() {
        let hasher = Hasher::new().unwrap();
        let mut rng = XorShiftRng::from_seed([0x0dbe6259, 0x8d313d76, 0x3237db17, 0xe5bc0654]);
        let mut msg: Vec<u8> = vec![0; 32];
        for i in msg.iter_mut() {
            *i = rng.gen();
        }
        let result = hasher.crh(&msg).unwrap();
        let _prf_result = hasher.prf(b"096b36a5804bfacef1691e173c366a47ff5ba84a44f26ddd7e8d9f79d5b42df0",b"ULforprf", &result, 769).unwrap();
    }

    #[test]
    fn test_prf_random_760() {
        let hasher = Hasher::new().unwrap();
        let mut rng = XorShiftRng::from_seed([0x2dbe6259, 0x8d313d76, 0x3237db17, 0xe5bc0654]);
        let mut msg: Vec<u8> = vec![0; 32];
        for i in msg.iter_mut() {
            *i = rng.gen();
        }
        let result = hasher.crh(&msg).unwrap();
        let _prf_result = hasher.prf(b"096b36a5804bfacef1691e173c366a47ff5ba84a44f26ddd7e8d9f79d5b42df0",b"ULforprf", &result, 760).unwrap();
    }

    #[test]
    fn test_hash_random() {
        let hasher = Hasher::new().unwrap();
        let mut rng = XorShiftRng::from_seed([0x2dbe6259, 0x8d313d76, 0x3237db17, 0xe5bc0654]);
        let mut msg: Vec<u8> = vec![0; 9820 * 4 / 8];
        for i in msg.iter_mut() {
            *i = rng.gen();
        }
        let _result = hasher.hash(b"096b36a5804bfacef1691e173c366a47ff5ba84a44f26ddd7e8d9f79d5b42df0",b"ULforprf", &msg, 760).unwrap();
    }

    #[test]
    #[should_panic]
    fn test_invalid_message() {
        let hasher = Hasher::new().unwrap();
        let mut rng = XorShiftRng::from_seed([0x2dbe6259, 0x8d313d76, 0x3237db17, 0xe5bc0654]);
        let mut msg: Vec<u8> = vec![0; 9820 * 4 / 8 + 1];
        for i in msg.iter_mut() {
            *i = rng.gen();
        }
        let _result = hasher.hash(b"096b36a5804bfacef1691e173c366a47ff5ba84a44f26ddd7e8d9f79d5b42df0",b"ULforprf", &msg, 760).unwrap();
    }

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
            y_co.push(hex::encode(&res))
        }
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
            let mut test_hash = hasher.crh(&msg).unwrap();
            test_hash.reverse();
            let msg_hash = hex::encode(&test_hash);

            if msg_hash.to_string() != actual_hash {
                println!("msg: {}", row);
            }
            assert_eq!( msg_hash.to_string() , actual_hash );
        }
    }

    #[test]
    fn test_crh_print() {
        let hasher = CompositeHasher::new().unwrap();

        let hex_msg = hex::encode(&[0b10]);
        println!("{}",  hex_msg.to_string());
        let to_hash = hex::decode(&hex_msg).unwrap();
        println!("{:?}", to_hash);
        let mut test_hash = hasher.crh(&to_hash).unwrap();
        test_hash.reverse();
        let hex_hash = hex::encode(&test_hash);
        println!("{}", hex_hash.to_string() );
    }
}
