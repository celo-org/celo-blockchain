extern crate hex;

use crate::hash::PRF;

use blake2s_simd::Params;
use failure::Error;

use byteorder::{LittleEndian, WriteBytesExt};

pub struct DirectHasher {
}

impl DirectHasher {
    pub fn new() -> Result<DirectHasher, Error> {
        Ok(DirectHasher {})
    }

}

impl PRF for DirectHasher {
    fn crh(&self, message: &[u8]) -> Result<Vec<u8>, Error> {
        let hash_result = Params::new()
            .hash_length(32)
            .to_state()
            .update(message)
            .finalize()
            .as_ref()
            .to_vec();
        return Ok(hash_result.to_vec());
    }

    fn prf(&self, domain: &[u8], hashed_message: &[u8], output_size_in_bits: usize) -> Result<Vec<u8>, Error> {
        if domain.len() > 8 {
            return Err(format_err!("domain length is too large: {}", domain.len()));
        }
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
                .personal(domain)
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

    fn hash(&self, domain: &[u8], message: &[u8], output_size_in_bits: usize) -> Result<Vec<u8>, Error> {
        let prepared_message = self.crh(message)?;
        self.prf(domain, &prepared_message, output_size_in_bits)
    }
}

#[cfg(test)]
mod test {
    use super::DirectHasher as Hasher;
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
        let _prf_result = hasher.prf(b"ULforprf", &result, 768).unwrap();
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
        let _prf_result = hasher.prf(b"ULforprf", &result, 769).unwrap();
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
        let _prf_result = hasher.prf(b"ULforprf", &result, 760).unwrap();
    }

    #[test]
    fn test_hash_random() {
        let hasher = Hasher::new().unwrap();
        let mut rng = XorShiftRng::from_seed([0x2dbe6259, 0x8d313d76, 0x3237db17, 0xe5bc0654]);
        let mut msg: Vec<u8> = vec![0; 9820 * 4 / 8];
        for i in msg.iter_mut() {
            *i = rng.gen();
        }
        let _result = hasher.hash(b"ULforprf", &msg, 760).unwrap();
    }
}
