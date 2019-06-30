use crate::hash::PRF;

use algebra::{bytes::ToBytes, curves::edwards_sw6::EdwardsAffine as Edwards};
use blake2s_simd::Params;
use dpc::crypto_primitives::crh::{
    pedersen::{PedersenCRH, PedersenParameters, PedersenWindow},
    FixedLengthCRH,
};
use failure::Error;
use rand::{chacha::ChaChaRng, Rng, SeedableRng};

use byteorder::{LittleEndian, ReadBytesExt, WriteBytesExt};

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
}

impl CompositeHasher {
    pub fn new() -> Result<CompositeHasher, Error> {
        Ok(CompositeHasher {
            parameters: CompositeHasher::setup_crh()?,
        })
    }

    fn prng() -> Result<impl Rng, Error> {
        let hash_result = Params::new()
            .hash_length(32)
            .personal(b"UL_prngs")
            .to_state()
            .update(b"ULTRALIGHT PRNG SEED")
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

    fn setup_crh() -> Result<CRHParameters, Error> {
        let mut rng = CompositeHasher::prng()?;
        CRH::setup::<_>(&mut rng)
    }
}

impl PRF for CompositeHasher {
    fn crh(&self, message: &[u8]) -> Result<Vec<u8>, Error> {
        let h = CRH::evaluate(&self.parameters, message)?;
        let mut res = vec![];
        h.x.write(&mut res)?;

        Ok(res)
    }

    fn prf(&self, hashed_message: &[u8], output_size_in_bits: usize) -> Result<Vec<u8>, Error> {
        let num_hashes = (output_size_in_bits + 256 - 1) / 256;
        let last_bits_to_keep = match output_size_in_bits % 256 {
            0 => 256,
            x => x,
        };
        let last_byte_position = last_bits_to_keep / 8;
        let last_byte_mask = (1 << (last_bits_to_keep % 8)) - 1;
        let mut counter: [u8; 4] = [0; 4];

        let mut result = vec![];
        for i in 0..num_hashes {
            (&mut counter[..]).write_u32::<LittleEndian>(i as u32)?;

            let mut hash_result = Params::new()
                .hash_length(32)
                .personal(b"ULforprf")
                .to_state()
                .update(&counter)
                .update(hashed_message)
                .finalize()
                .as_ref()
                .to_vec();
            if i == num_hashes - 1 {
                let mut current_index = 0;
                for j in hash_result.iter_mut() {
                    if current_index == last_byte_position {
                        *j = *j & last_byte_mask;
                    } else if current_index > last_byte_position {
                        *j = 0;
                    }
                    current_index += 1;
                }
            }
            result.append(&mut hash_result);
        }

        Ok(result)
    }
    fn hash(&self, message: &[u8], output_size_in_bits: usize) -> Result<Vec<u8>, Error> {
        let hashed_message = self.crh(message)?;
        self.prf(&hashed_message, output_size_in_bits)
    }
}

#[cfg(test)]
mod test {
    use super::CompositeHasher as Hasher;
    use crate::hash::PRF;
    use rand::{Rng, SeedableRng, XorShiftRng};

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
        let _prf_result = hasher.prf(&result, 768).unwrap();
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
        let _prf_result = hasher.prf(&result, 769).unwrap();
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
        let _prf_result = hasher.prf(&result, 760).unwrap();
    }

    #[test]
    fn test_hash_random() {
        let hasher = Hasher::new().unwrap();
        let mut rng = XorShiftRng::from_seed([0x2dbe6259, 0x8d313d76, 0x3237db17, 0xe5bc0654]);
        let mut msg: Vec<u8> = vec![0; 9820 * 4 / 8];
        for i in msg.iter_mut() {
            *i = rng.gen();
        }
        let _result = hasher.hash(&msg, 760).unwrap();
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
        let _result = hasher.hash(&msg, 760).unwrap();
    }
}
