use crate::{hashers::DirectHasher, BLSError, XOF};

use algebra::{bytes::ToBytes, edwards_sw6::EdwardsProjective as Edwards, ProjectiveCurve};

use blake2s_simd::Params;
use crypto_primitives::crh::{
    bowe_hopwood::{BoweHopwoodPedersenCRH, BoweHopwoodPedersenParameters},
    pedersen::PedersenWindow,
    FixedLengthCRH,
};
use once_cell::sync::Lazy;
use rand::{Rng, SeedableRng};
use rand_chacha::ChaChaRng;

/// The window which will be used with the Fixed Length CRH
#[derive(Clone)]
pub struct Window;

impl PedersenWindow for Window {
    const WINDOW_SIZE: usize = 93;
    const NUM_WINDOWS: usize = 560;
}

pub type CRH = BoweHopwoodPedersenCRH<Edwards, Window>;
pub type CRHParameters = BoweHopwoodPedersenParameters<Edwards>;

/// Lazily evaluated composite hasher instantiated over the
/// Bowe-Hopwood-Pedersen CRH.
pub static COMPOSITE_HASHER: Lazy<CompositeHasher<CRH>> =
    Lazy::new(|| CompositeHasher::<CRH>::new().unwrap());

/// A composite hasher which uses the provided CRH independently of domain/message length
/// provided
#[derive(Clone, Debug)]
pub struct CompositeHasher<H: FixedLengthCRH> {
    parameters: H::Parameters,
}

impl<H: FixedLengthCRH> CompositeHasher<H> {
    /// Initializes the CRH and returns a new hasher
    pub fn new() -> Result<CompositeHasher<H>, BLSError> {
        Ok(CompositeHasher {
            parameters: Self::setup_crh()?,
        })
    }

    fn prng() -> impl Rng {
        let hash_result = Params::new()
            .hash_length(32)
            .personal(b"UL_prngs") // personalization
            .to_state()
            .update(b"ULTRALIGHT PRNG SEED") // message
            .finalize()
            .as_ref()
            .to_vec();
        let mut seed = [0; 32];
        seed.copy_from_slice(&hash_result[..32]);
        ChaChaRng::from_seed(seed)
    }

    /// Instantiates the CRH's parameters
    pub fn setup_crh() -> Result<H::Parameters, BLSError> {
        let mut rng = Self::prng();
        Ok(H::setup::<_>(&mut rng)?)
    }
}

impl<H: FixedLengthCRH<Output = Edwards>> XOF for CompositeHasher<H> {
    type Error = BLSError;

    fn crh(&self, _: &[u8], message: &[u8], _: usize) -> Result<Vec<u8>, Self::Error> {
        let h = H::evaluate(&self.parameters, message)?.into_affine();
        let mut res = vec![];
        h.x.write(&mut res)?;

        Ok(res)
    }

    fn xof(
        &self,
        domain: &[u8],
        hashed_message: &[u8],
        xof_digest_length: usize,
    ) -> Result<Vec<u8>, Self::Error> {
        DirectHasher.xof(domain, hashed_message, xof_digest_length)
    }
}

#[cfg(test)]
mod test {
    use super::*;
    use crate::hashers::XOF;
    use rand::{Rng, SeedableRng};
    use rand_xorshift::XorShiftRng;

    #[test]
    fn test_crh_empty() {
        let msg: Vec<u8> = vec![];
        let hasher = &*COMPOSITE_HASHER;
        let result = hasher.crh(&[], &msg, 96).unwrap();
        assert_eq!(hex::encode(result), "000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000")
    }

    #[test]
    fn test_crh_random() {
        let hasher = &*COMPOSITE_HASHER;
        let mut rng = XorShiftRng::from_seed([
            0x5d, 0xbe, 0x62, 0x59, 0x8d, 0x31, 0x3d, 0x76, 0x32, 0x37, 0xdb, 0x17, 0xe5, 0xbc,
            0x06, 0x54,
        ]);
        let mut msg: Vec<u8> = vec![0; 32];
        for i in msg.iter_mut() {
            *i = rng.gen();
        }
        let result = hasher.crh(&[], &msg, 96).unwrap();
        assert_eq!(hex::encode(result), "066e4894d9e5074a8aaf37d342703e48f83aa967952b79bf99cb9db98270907c1d92043890256cf7b19a0cb5b8155300")
    }

    #[test]
    fn test_xof_random_768() {
        let hasher = &*COMPOSITE_HASHER;
        let mut rng = XorShiftRng::from_seed([
            0x2d, 0xbe, 0x62, 0x59, 0x8d, 0x31, 0x3d, 0x76, 0x32, 0x37, 0xdb, 0x17, 0xe5, 0xbc,
            0x06, 0x54,
        ]);
        let mut msg: Vec<u8> = vec![0; 32];
        for i in msg.iter_mut() {
            *i = rng.gen();
        }
        let result = hasher.crh(&[], &msg, 96).unwrap();
        let xof_result = hasher.xof(b"ULforxof", &result, 768).unwrap();
        assert_eq!(hex::encode(xof_result), "b11ad468878c6b02da76f9916a03612f0e076bd54c52fb8f40d528e537a6489df15ba807b296f13b6f85ca3bf4115b92cfb132ce5941fc2919f7841220c2b9a640cbbdeb365b2f50048dba1fd245aed889d435c08000cd09a3464080f54e98455eefd87f43ca866fa4381f618748ad6a7087f3a7a96e02869a31ef09cead5f2b205f61160eb0e12f6df78387ad7784d0e64c2d98282994c17589b5e5cf3de37afd83eebb0ef22a1a8b079ff0b672b43b45be8eac4bd8be0c245ccb4d2c159bf7aa7c7e869dbed5b0d9b69918f178e5e3570d8486b274b6363b367fd420d0c44fb08307ef34375e809b8ac5a4699f4c89efe1cbd993b119cb98a2b81881eb1435ae380f9d89343bb2a876fb0f9b65fae1126519a5960ed0d43a63e613e71f41c8773d7c7f7b0ce28ede4a302c502be13325877c401e8316dc9c307164dc6500cf6b43895dd805585bab64a0f715c7ecd0de08da21b72b7880401b03b2b1801665f7425c0150d009a4c37403d9ce588b3dcfca4660081d59adcc1a32998a809fef7d9f359a89bee996e9252e618d5caed2dacd0003ca4dc37b0809361516f5261bbadc2a13c9e2b68cac6ef4d00d8b854d16721b23797d1063dfe26332ae0d8494bc7c637a4d1d6238c901707aa0e09f1b6607fbf172fe9b75808301d69df0ef137d6427ff909a818dba52767ae257afa6ac4ef738bf8adbc445e282cf1559689e28edefc948d34559e3c6a03884bc01b415064894f1f4c1a899426570d5c284a1195811b992fab25a621f021078219ec770f47aabe600009a62cf50b521fe4e2a4eed8429e0f29163954231309a91d39beffe6abc96f3eddf7d413864a027c18f1ad3ea55bbbb6e03ebbdcf1e187f291b8c96e1f5f71c2d67bfd1c0c3105619172e95833147e2b90cfbea571653cf7de057d99b311db73aee3518f1a5bd52b32e96c0be564abfb8a4046a626747a31dde1f067a8e4657d152f2169e1c2e86223fca550c034ff3a3d2e2d65e751061d704776a675f5082ecb8887de1866f56d0f5670c1a8a56fb16b3300d05ae255dd2ac6704cb05ba525db0e5cc10365dfc8bcd");
    }

    #[test]
    fn test_xof_random_769() {
        let hasher = &*COMPOSITE_HASHER;
        let mut rng = XorShiftRng::from_seed([
            0x0d, 0xbe, 0x62, 0x59, 0x8d, 0x31, 0x3d, 0x76, 0x32, 0x37, 0xdb, 0x17, 0xe5, 0xbc,
            0x06, 0x54,
        ]);
        let mut msg: Vec<u8> = vec![0; 32];
        for i in msg.iter_mut() {
            *i = rng.gen();
        }
        let result = hasher.crh(&[], &msg, 96).unwrap();
        let xof_result = hasher.xof(b"ULforxof", &result, 769).unwrap();
        assert_eq!(hex::encode(xof_result), "99b212c07e386d7afa3d1da421a360d88b2e8fa45abf2eaf2c9dd5c85287d4c240d4c8eb39945a8fccc233ad529d1679700e2850758bfe442a31ba48708d2e86b28ca60c683f31815ad0b8cca4d062f524df17271f91d03c33c3e985e50794fc4466c89187f89d319276423a46eb7a75c18c575bcc0530a0c0003c207f9874eda2218eb0da88400d7a9beb523e9b93f10db280f033cbe245586cd85410ffe645400ac828d146d962810d4f219cf2efd89e8ebc2b40eaa1b816d5d25a8777972094607900880ccafcda106cb7978c88a1caa1362c21d4e412d83dbbb15c6148496bc65287500aadf69b2f609a0839d02834c09c735c4f522acea2d91ed5e2029d48b9f8adfaa7594e1d2f175d0089af3c42ed9b54bd6248c31d5f2ef28f38cb095266a4469c98ce4ecc6c01a1a82668f72e99574d9016af99736ce30f6b31532cadc12e482944db567c290570a15d8ee2d711843995b6788bc337ef954e165dc830b3af5c98809a9d0b616f34f802f561ad43d40784855d664a3aae2c2d4b5fb6b0e27ba65bddbd4dc3c2dc15ccac917e539fc0b7c194f49e7e22677109563a77e8f7b7e51b703fb57e965e1f4e09b4854b8fc010dcae6a9488db006cd0aa8ab1aebc2d52c87d6a551c2e55cb3605199986721d2648b152fcbbbb1872920b518f6a0a89c804b7e4b9d235f5c55aefe732073565bc32a684c18f65676b88b182f57f7f0868575dbc365d9fa96f925937a173d9a0639a6096d623a134da9c52a0c87ac118be66c00743a921134dc49b90e05957a5a1c75ba2e3124d46c36b2024a2ec0098f02ff59b5c8f31ad1829adf51db20b0e26b4f98c68d17548da0f9e97bbde4edbe59d705c6f97e9eaf228c2c38a9f52d952049b2f5d64cab9f8f6ff6abd98a3679700586cfaf2e45dc4d159bf0a3d24f9815a35da4011f83b39845114d9b10f5d8093cc3ff1956598989cb6ec88a2c817f8a9218bf88864b6c77be1f55b69639b5b3e570c259ea20bc2f029c4350e6c9dde56870ef8438f145789e7450724816c7a5f014a9217177896cbb41affdd7274b22193844ccc4f4bdca6677121cf")
    }

    #[test]
    fn test_xof_random_96() {
        let hasher = &*COMPOSITE_HASHER;
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
        assert_eq!(hex::encode(xof_result), "12b0fa43ad6823768667daa148174d65a43c457ad2358830fbddf8e3f00bd9a76014753b12ecb355d1deda25038969754bd9ef5045f59460b527ef11a8084c71983139dfe7c3fda876358ff7591e6dbd24e07ba961b7c0cb634eae5d07a172ee")
    }

    #[test]
    fn test_hash_random() {
        let hasher = &*COMPOSITE_HASHER;
        let mut rng = XorShiftRng::from_seed([
            0x2d, 0xbe, 0x62, 0x59, 0x8d, 0x31, 0x3d, 0x76, 0x32, 0x37, 0xdb, 0x17, 0xe5, 0xbc,
            0x06, 0x54,
        ]);
        let mut msg: Vec<u8> = vec![0; 9820 * 4 / 8];
        for i in msg.iter_mut() {
            *i = rng.gen();
        }
        let result = hasher.hash(b"ULforxof", &msg, 96).unwrap();
        assert_eq!(hex::encode(result), "9108330a206e984c63b034fa59ee6f774628a881c38f2ef3e1f02d135b41958a124fabc66e547a2030f5c8142d610b1d272a67577f2c75addfd54cc96d08cff7f014fbd3147a58a8ecc2a892a04426adee811f2b7f056d58557cd7a42751dde8")
    }

    #[test]
    #[should_panic]
    fn test_invalid_message() {
        let hasher = &*COMPOSITE_HASHER;
        let mut rng = XorShiftRng::from_seed([
            0x2d, 0xbe, 0x62, 0x59, 0x8d, 0x31, 0x3d, 0x76, 0x32, 0x37, 0xdb, 0x17, 0xe5, 0xbc,
            0x06, 0x54,
        ]);
        let mut msg: Vec<u8> = vec![0; 100_000_0];
        for i in msg.iter_mut() {
            *i = rng.gen();
        }
        let _result = hasher.hash(b"ULforxof", &msg, 96).unwrap();
    }
}
