use bench_utils::{end_timer, start_timer};
use byteorder::WriteBytesExt;
use hex;
use log::trace;
use std::marker::PhantomData;

use super::HashToCurve;
use crate::hashers::{
    composite::{CompositeHasher, COMPOSITE_HASHER, CRH},
    DirectHasher, XOF,
};
use crate::BLSError;

use algebra::{
    bls12_377::Parameters,
    curves::models::short_weierstrass_jacobian::{GroupAffine, GroupProjective},
    curves::models::{bls12::Bls12Parameters, SWModelParameters},
    AffineCurve, Zero,
};

use algebra::ConstantSerializedSize;

use once_cell::sync::Lazy;

const NUM_TRIES: u8 = 255;

/// Composite Try-and-Increment hasher for BLS 12-377.
pub static COMPOSITE_HASH_TO_G1: Lazy<
    TryAndIncrement<CompositeHasher<CRH>, <Parameters as Bls12Parameters>::G1Parameters>,
> = Lazy::new(|| TryAndIncrement::new(&*COMPOSITE_HASHER));

pub static DIRECT_HASH_TO_G1: Lazy<
    TryAndIncrement<DirectHasher, <Parameters as Bls12Parameters>::G1Parameters>,
> = Lazy::new(|| TryAndIncrement::new(&DirectHasher));

/// A try-and-increment method for hashing to G1 and G2. See page 521 in
/// https://link.springer.com/content/pdf/10.1007/3-540-45682-1_30.pdf.
#[derive(Clone)]
pub struct TryAndIncrement<'a, H, P> {
    hasher: &'a H,
    curve_params: PhantomData<P>,
}

impl<'a, H, P> TryAndIncrement<'a, H, P>
where
    H: XOF<Error = BLSError>,
    P: SWModelParameters,
{
    /// Instantiates a new Try-and-increment hasher with the provided hashing method
    /// and curve parameters based on the type
    pub fn new(h: &'a H) -> Self {
        TryAndIncrement {
            hasher: h,
            curve_params: PhantomData,
        }
    }
}

impl<'a, H, P> HashToCurve for TryAndIncrement<'a, H, P>
where
    H: XOF<Error = BLSError>,
    P: SWModelParameters,
{
    type Output = GroupProjective<P>;

    fn hash(
        &self,
        domain: &[u8],
        message: &[u8],
        extra_data: &[u8],
    ) -> Result<Self::Output, BLSError> {
        self.hash_with_attempt(domain, message, extra_data)
            .map(|res| res.0)
    }
}

impl<'a, H, P> TryAndIncrement<'a, H, P>
where
    H: XOF<Error = BLSError>,
    P: SWModelParameters,
{
    /// Hash with attempt takes the input, appends a counter
    pub fn hash_with_attempt(
        &self,
        domain: &[u8],
        message: &[u8],
        extra_data: &[u8],
    ) -> Result<(GroupProjective<P>, usize), BLSError> {
        let num_bytes = GroupAffine::<P>::SERIALIZED_SIZE;
        let hash_loop_time = start_timer!(|| "try_and_increment::hash_loop");
        let hash_bytes = hash_length(num_bytes);

        let mut counter = [0; 1];
        for c in 0..NUM_TRIES {
            (&mut counter[..]).write_u8(c as u8)?;

            // concatenate the message with the counter
            let msg = &[&counter, extra_data, &message].concat();

            // produce a hash with sufficient length
            let candidate_hash = self.hasher.hash(domain, msg, hash_bytes)?;

            if let Some(p) = GroupAffine::<P>::from_random_bytes(&candidate_hash[..num_bytes]) {
                trace!(
                    "succeeded hashing \"{}\" to curve in {} tries",
                    hex::encode(message),
                    c
                );
                end_timer!(hash_loop_time);

                let scaled = p.scale_by_cofactor();
                if scaled.is_zero() {
                    continue;
                }

                return Ok((scaled, c as usize));
            }
        }
        Err(BLSError::HashToCurveError)
    }
}

/// Given `n` bytes, it returns the value rounded to the nearest multiple of 256 bits (in bytes)
/// e.g. 1. given 48 = 384 bits, it will return 64 bytes (= 512 bits)
///      2. given 96 = 768 bits, it will return 96 bytes (no rounding needed since 768 is already a
///         multiple of 256)
fn hash_length(n: usize) -> usize {
    let bits = (n * 8) as f64 / 256.0;
    let rounded_bits = bits.ceil() * 256.0;
    rounded_bits as usize / 8
}

#[cfg(test)]
mod test {
    use super::*;
    use crate::hash_to_curve::try_and_increment::COMPOSITE_HASH_TO_G1;
    use algebra::{bls12_377::Parameters, curves::ProjectiveCurve, CanonicalSerialize};
    use rand::RngCore;
    use rand::{Rng, SeedableRng};
    use rand_xorshift::XorShiftRng;

    #[test]
    fn test_hash_length() {
        assert_eq!(hash_length(48), 64);
        assert_eq!(hash_length(96), 96);
    }

    fn generate_test_data<R: Rng>(rng: &mut R) -> (Vec<u8>, Vec<u8>, Vec<u8>) {
        let msg_size: u8 = rng.gen();
        let mut msg: Vec<u8> = vec![0; msg_size as usize];
        for i in msg.iter_mut() {
            *i = rng.gen();
        }

        let mut domain = vec![0u8; 8];
        for i in domain.iter_mut() {
            *i = rng.gen();
        }

        let extra_data_size: u8 = rng.gen();
        let mut extra_data: Vec<u8> = vec![0; extra_data_size as usize];
        for i in extra_data.iter_mut() {
            *i = rng.gen();
        }

        (domain, msg, extra_data)
    }

    #[test]
    fn hash_to_curve_direct_g1() {
        let h = DirectHasher;
        hash_to_curve_test::<<Parameters as Bls12Parameters>::G1Parameters, _>(h)
    }

    #[test]
    fn hash_to_curve_composite_g1() {
        let h = CompositeHasher::<CRH>::new().unwrap();
        hash_to_curve_test::<<Parameters as Bls12Parameters>::G1Parameters, _>(h)
    }

    #[test]
    fn hash_to_curve_direct_g2() {
        let h = DirectHasher;
        hash_to_curve_test::<<Parameters as Bls12Parameters>::G2Parameters, _>(h)
    }

    #[test]
    fn hash_to_curve_composite_g2() {
        let h = CompositeHasher::<CRH>::new().unwrap();
        hash_to_curve_test::<<Parameters as Bls12Parameters>::G2Parameters, _>(h)
    }

    fn hash_to_curve_test<P: SWModelParameters, X: XOF<Error = BLSError>>(h: X) {
        let hasher = TryAndIncrement::<X, P>::new(&h);
        let mut rng = rand::thread_rng();
        for length in &[10, 25, 50, 100, 200, 300] {
            let mut input = vec![0; *length];
            rng.fill_bytes(&mut input);
            hasher.hash(&b"domain"[..], &input, &b"extra"[..]).unwrap();
        }
    }

    #[test]
    fn test_hash_to_curve_g1() {
        let mut rng = XorShiftRng::from_seed([
            0x5d, 0xbe, 0x62, 0x59, 0x8d, 0x31, 0x3d, 0x76, 0x32, 0x37, 0xdb, 0x17, 0xe5, 0xbc,
            0x06, 0x54,
        ]);
        let expected_hashes = vec![
            "a7e17c99126acf78536e64fffe88e1032d834b483584fe5757b1deafa493c97a132572c7825ca4f617f6bcef93b93980",
            "21e328cfedb263f8c815131cc42f0357ab0ba903d855a11de6e7bcd7e61375a818d1b093bcf9fce224536714efad5c80",
            "fcc8bc80a528b32762ad3b3f72d40b069083b833ad4b6e135040414e2634657e1cf1ec070235ba1425f350df8c585d81",
            "9b99c3cee5f7c486f962b1391b4108cd464b05bc24b2e488e9aa04f848467315ed70d83d3abfa63150564ad0c549c480",
            "9df1b6ba0e8d2a42866d78a90b5fdf56cea80b2ec588774ceb7cc4f414d7b49ca55f81169535a4c3a4c7c39148af3e81",
            "f365f54ba587b863d5d5ecef6a2932f4eb225c0cd2c4e727c3fa5b1a30fbcfa8e2a2e0d7a68476ee10d90b3b8846b400",
            "1cb6008bca08b85df6f9a87ca141533145ed88abb0bbace96f4b1ca42d15ba888d4948c21548207a0abd22d5c234d180",
            "1c529f631ddaffde7cbe62bbb8d48cc8dbe59b8548dc69b156d0568c7aae898d8051a3ef31ad17c60a85ad82203a9b81",
            "de54da7a8813a30c267d662d428e28520a159b51a9e226ceb663d460d9065b66a9586cb8b3a9ba0ef0e27c626f20dc00",
            "b68e1db4b648801676a79ac199eaf003757bf2a96cdbb804bfefe0484afdc0cc299d50d660221d1de374e92c44291200",
        ].into_iter().map(|x| hex::decode(&x).unwrap()).collect::<Vec<_>>();

        test_hash_to_group(&*COMPOSITE_HASH_TO_G1, &mut rng, expected_hashes)
    }

    #[test]
    fn test_hash_to_curve_g2() {
        let mut rng = XorShiftRng::from_seed([
            0x5d, 0xbe, 0x62, 0x59, 0x8d, 0x31, 0x3d, 0x76, 0x32, 0x37, 0xdb, 0x17, 0xe5, 0xbc,
            0x06, 0x54,
        ]);

        let expected_hashes = vec![
            "9c76f364d39ce5747f475088f459a11cb32d39033245c039104dfe88a71047ea078d6f15ed9fc64539410167ffe1800020ec8138f9f8b03c675f4ff33d621c76f41784bf994aa8cf53b2e11961f4c77caaab6681dc29bb2f90e14ecd05a5f500",
            "ffb0b3275d2188bee71e0f626b2bc422ee4ce23692e6d329e085ec74413410cedd354d9571e9de149a286dc48ba83d012ad171f4280acbc3c3d946086fe2a0c9f56d271f0c9bb13e78774cb6244b2e84c24116d8ff76311cf2f76db741ab7200",
            "59af04e977ac914d077d1488639b90dfb5b723bf8516157b9ebc8b584a0f507f20c3b758284fe3c91bc93df86244a9017e06d3f930163642a3c85965aac19ea8a18b0bd08d7bd44e99e343acfe24f98ff6f2401432187a07dd97320f73fa7300",
            "5a1610b23a5a5be0ee255fcc766d0f6d384b3d51b4364d5587102e8905b7233fd5b274973451cb56ca69a945832c1000d0b2744278ffdf5cd33f11bcc4ecc5759b0d5b90f54d454909d73f49c1226e428acfb25995d83ba44826adb8158f1281",
            "d82143317b1a5b90e633a4a208129edd526f9137b9c47221c827aa6317be94cb1bc006ba8afce455be5bf51ee6f184011c535bee7ab3e954731a6a96edb3ea9a6c1d02916817147355a2406757023e27fb2f58fec61f37ddb6125c797bfa5780",
            "48bfa38e3c4a6a7de2a5c4b8c57671c7b1bfb2c225d89786cbcd065b2b7844b910b5cbfc334eff1956bc7245127d970154c38985b770d11994c20072a053f0f720028615753c9c42372580782dd49653b4c0fee2a8e88de1697678a505ffc980",
            "ddc0e29af05439bcb5157802afd9a112394fb190e0dda7b5c7852693da3b3403c911751c24b28af1d05e76326d1117007f14cc765d5c3e73adbbcf7a1d59cf58186d7b576d3e58ccafd2ea527bf31651f4b0d0ba44ee5b54ec6c86c2e1bf1b01",
            "ddc865ffe876a3e19c1401f784eaf88b50c4f04cfaadf7690173a33385cb5af899189478cdbc1abbe8d8a89768e411003a5000c7866f3a5648d7944e97bcbff87f89cd26045dc15494036ce4ce799de532438576bfe32389269a6e3a4ce98201",
            "8de37ce0a7105c14880d9201f2ac1c724e031904f9c88614fa414ad57f00c89e596fadb4f5151c84f4ea04d576931c008fc43faec79d0e300d2192a8e376b25f920f14f467f050e4f2869012fce196e9af5f2041889031e2bbe81c6b3d344480",
            "10341299c41179084a0bfee8b65bac0f48af827daad4f01d3e9925a3b0335736c5d13f44765fecec45941781da5a1000d0bb26a4faa4dc8060b0b2dd0cb6acce7dd10bd081dac7f263b97aec89d6434a55b31a65b3e25f59c40ea92887b03180",
        ].into_iter().map(|x| hex::decode(&x).unwrap()).collect::<Vec<_>>();
        let hasher_g2 = TryAndIncrement::<_, <Parameters as Bls12Parameters>::G2Parameters>::new(
            &*COMPOSITE_HASHER,
        );
        test_hash_to_group(&hasher_g2, &mut rng, expected_hashes)
    }

    fn test_hash_to_group<P: SWModelParameters, H: HashToCurve<Output = GroupProjective<P>>>(
        hasher: &H,
        rng: &mut impl Rng,
        expected_hashes: Vec<Vec<u8>>,
    ) {
        for i in 0..10 {
            let (domain, msg, extra_data) = generate_test_data(rng);
            let g = hasher.hash(&domain, &msg, &extra_data).unwrap();
            let mut bytes = vec![];
            g.into_affine().serialize(&mut bytes).unwrap();
            assert_eq!(expected_hashes[i], bytes);
        }
    }
}
