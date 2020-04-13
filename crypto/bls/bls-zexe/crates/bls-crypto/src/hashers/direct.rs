use crate::{hashers::XOF, BLSError};
use blake2s_simd::Params;
use byteorder::{LittleEndian, WriteBytesExt};

pub struct DirectHasher;

fn xof_digest_length_to_node_offset(
    node_offset: u64,
    xof_digest_length: usize,
) -> Result<u64, BLSError> {
    let mut xof_digest_length_bytes: [u8; 2] = [0; 2];
    (&mut xof_digest_length_bytes[..]).write_u16::<LittleEndian>(xof_digest_length as u16)?;
    let offset = node_offset as u64
        | ((xof_digest_length_bytes[0] as u64) << 32)
        | ((xof_digest_length_bytes[1] as u64) << 40);
    Ok(offset)
}

impl XOF for DirectHasher {
    type Error = BLSError;

    fn crh(
        &self,
        domain: &[u8],
        message: &[u8],
        xof_digest_length: usize,
    ) -> Result<Vec<u8>, Self::Error> {
        let hash_result = Params::new()
            .hash_length(32)
            .node_offset(xof_digest_length_to_node_offset(0, xof_digest_length)?)
            .personal(domain)
            .to_state()
            .update(message)
            .finalize()
            .as_ref()
            .to_vec();
        Ok(hash_result.to_vec())
    }

    fn xof(
        &self,
        domain: &[u8],
        hashed_message: &[u8],
        xof_digest_length: usize,
    ) -> Result<Vec<u8>, Self::Error> {
        if domain.len() > 8 {
            return Err(BLSError::DomainTooLarge(domain.len()));
        }
        let num_hashes = (xof_digest_length + 32 - 1) / 32;

        let mut result = vec![];
        for i in 0..num_hashes {
            let hash_length = if i == num_hashes - 1 && (xof_digest_length % 32 != 0) {
                xof_digest_length % 32
            } else {
                32
            };
            let mut hash_result = Params::new()
                .hash_length(hash_length)
                .max_leaf_length(32)
                .inner_hash_length(32)
                .fanout(0)
                .max_depth(0)
                .personal(domain)
                .node_offset(xof_digest_length_to_node_offset(
                    i as u64,
                    xof_digest_length,
                )?)
                .to_state()
                .update(hashed_message)
                .finalize()
                .as_ref()
                .to_vec();
            result.append(&mut hash_result);
        }

        Ok(result)
    }
}

#[cfg(test)]
mod test {
    use super::*;
    use rand::{Rng, SeedableRng};
    use rand_xorshift::XorShiftRng;

    #[test]
    fn test_crh_empty() {
        let msg: Vec<u8> = vec![];
        let result = DirectHasher.crh(&[], &msg, 96).unwrap();
        assert_eq!(
            hex::encode(result),
            "7a746244ad211d351f57a218255888174e719b54e683651e9314f55402eed414"
        )
    }

    #[test]
    fn test_crh_random() {
        let mut rng = XorShiftRng::from_seed([
            0x5d, 0xbe, 0x62, 0x59, 0x8d, 0x31, 0x3d, 0x76, 0x32, 0x37, 0xdb, 0x17, 0xe5, 0xbc,
            0x06, 0x54,
        ]);
        let mut msg: Vec<u8> = vec![0; 32];
        for i in msg.iter_mut() {
            *i = rng.gen();
        }
        let result = DirectHasher.crh(&[], &msg, 96).unwrap();
        assert_eq!(
            hex::encode(result),
            "b5a31242cffbefda914dc6d655fd200ee72e0297f951c345409936d45b5f080b"
        )
    }

    #[test]
    fn test_xof_random_96() {
        let hasher = DirectHasher;
        let mut rng = XorShiftRng::from_seed([
            0x2d, 0xbe, 0x62, 0x59, 0x8d, 0x31, 0x3d, 0x76, 0x32, 0x37, 0xdb, 0x17, 0xe5, 0xbc,
            0x06, 0x54,
        ]);
        let mut msg: Vec<u8> = vec![0; 32];
        for i in msg.iter_mut() {
            *i = rng.gen();
        }
        let result = hasher.crh(&[], &msg, 96).unwrap();
        let xof_result = hasher.xof(b"ULforxof", &result, 96).unwrap();
        assert_eq!(
            hex::encode(xof_result),
            "5801c1a4b06a9329109326c0fbccb028c5d7f0fb03ff5345f681f65f8b81dbb1c8c48d4cd4f5a4f1698dfc53a87db8865895a484f9c5d0d120709333418e6d2ac4787d996b564bbf5d6d506f1e280e4695599e42cd9e668c0ed9444a7b58a781");
    }

    #[test]
    fn test_hash_random() {
        let hasher = DirectHasher;
        let mut rng = XorShiftRng::from_seed([
            0x2d, 0xbe, 0x62, 0x59, 0x8d, 0x31, 0x3d, 0x76, 0x32, 0x37, 0xdb, 0x17, 0xe5, 0xbc,
            0x06, 0x54,
        ]);
        let mut msg: Vec<u8> = vec![0; 9820 * 4 / 8];
        for i in msg.iter_mut() {
            *i = rng.gen();
        }
        let result = hasher.hash(b"ULforxof", &msg, 96).unwrap();
        assert_eq!(hex::encode(result), "8ed2c28681f8be94c08c6ff066bf7ab514e1d68b5b71e0e9097e6e2834f8c3eba7c4a41efc9c34e839a8a2577c08ed2273fc6ec7611b5fa62446e7b6f01827ba7860c49174afdf6d26e5cef44d7f8530ca8ccdd3febe55a1401ac83d63e00eba")
    }

    #[test]
    fn test_blake2s_test_vectors() {
        let hasher = DirectHasher;
        let test_vectors = [(
            "7f8a56d8b5fb1f038ffbfce79f185f4aad9d603094edb85457d6c84d6bc02a82644ee42da51e9c3bb18395f450092d39721c32e7f05ec4c1f22a8685fcb89721738335b57e4ee88a3b32df3762503aa98e4a9bd916ed385d265021391745f08b27c37dc7bc6cb603cc27e19baf47bf00a2ab2c32250c98d79d5e1170dee4068d9389d146786c2a0d1e08ade5"   ,
            "87009aa74342449e10a3fd369e736fcb9ad1e7bd70ef007e6e2394b46c094074c86adf6c980be077fa6c4dc4af1ca0450a4f00cdd1a87e0c4f059f512832c2d92a1cde5de26d693ccd246a1530c0d6926185f9330d3524710b369f6d2976a44d",
        ), (
            "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f202122232425262728292a2b2c2d2e2f303132333435363738393a3b3c3d3e3f404142434445464748494a4b4c4d4e4f505152535455565758595a5b5c5d5e5f606162636465666768696a6b6c6d6e6f707172737475767778797a7b7c7d7e7f808182838485868788898a8b8c8d8e8f909192939495969798999a9b9c9d9e9fa0a1a2a3a4a5a6a7a8a9aaabacadaeafb0b1b2b3b4b5b6b7b8b9babbbcbdbebfc0c1c2c3c4c5c6c7c8c9cacbcccdcecfd0d1d2d3d4d5d6d7d8d9dadbdcdddedfe0e1e2e3e4e5e6e7e8e9eaebecedeeeff0f1f2f3f4f5f6f7f8f9fafbfcfdfeff",
            "57d5",
        ), (
            "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f202122232425262728292a2b2c2d2e2f303132333435363738393a3b3c3d3e3f404142434445464748494a4b4c4d4e4f505152535455565758595a5b5c5d5e5f606162636465666768696a6b6c6d6e6f707172737475767778797a7b7c7d7e7f808182838485868788898a8b8c8d8e8f909192939495969798999a9b9c9d9e9fa0a1a2a3a4a5a6a7a8a9aaabacadaeafb0b1b2b3b4b5b6b7b8b9babbbcbdbebfc0c1c2c3c4c5c6c7c8c9cacbcccdcecfd0d1d2d3d4d5d6d7d8d9dadbdcdddedfe0e1e2e3e4e5e6e7e8e9eaebecedeeeff0f1f2f3f4f5f6f7f8f9fafbfcfdfeff",
            "bfec8b58ee2e2e32008eb9d7d304914ea756ecb31879eb2318e066c182b0e77e6a518e366f345692e29f497515f799895983200f0d7dafa65c83a7506c03e8e5eee387cffdb27a0e6f5f3e9cb0ccbcfba827984586f608769f08f6b1a84872",
        )];
        for test_vector in &test_vectors {
            let bytes = hasher
                .hash(
                    b"",
                    &hex::decode(test_vector.0).unwrap(),
                    test_vector.1.len() / 2,
                )
                .unwrap();
            assert_eq!(hex::encode(&bytes), test_vector.1);
        }
    }
}
