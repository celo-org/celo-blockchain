use crate::curve::hash::HashToG1;

use lazy_static::lazy_static;
use lru::LruCache;
use std::{
    collections::HashSet,
    hash::{Hash, Hasher},
    sync::Mutex,
};

use algebra::{
    bls12_377::{
        g1::Parameters as Bls12_377G1Parameters, g2::Parameters as Bls12_377G2Parameters,
        Bls12_377, Fq, Fq12, Fq2, Fr, G1Affine, G1Projective, G2Affine, G2Projective,
        Parameters as Bls12_377Parameters,
    },
    bytes::{FromBytes, ToBytes},
    curves::SWModelParameters,
    AffineCurve, Field, One, PairingEngine, PrimeField, ProjectiveCurve, SquareRootField,
    UniformRand, Zero,
};
use rand::Rng;

use std::{
    error::Error,
    fmt::{self, Display},
};

pub static SIG_DOMAIN: &[u8] = b"ULforxof";
pub static POP_DOMAIN: &[u8] = b"ULforpop";

/// Implements BLS signatures as specified in https://crypto.stanford.edu/~dabo/pubs/papers/BLSmultisig.html.
use std::{
    io::{self, Read, Result as IoResult, Write},
    ops::Neg,
};

#[derive(Clone)]
pub struct PrivateKey {
    sk: Fr,
}

impl PrivateKey {
    pub fn generate<R: Rng>(rng: &mut R) -> PrivateKey {
        PrivateKey { sk: Fr::rand(rng) }
    }

    pub fn from_sk(sk: &Fr) -> PrivateKey {
        PrivateKey { sk: sk.clone() }
    }

    pub fn get_sk(&self) -> Fr {
        self.sk.clone()
    }

    pub fn sign<H: HashToG1>(
        &self,
        message: &[u8],
        extra_data: &[u8],
        hash_to_g1: &H,
    ) -> Result<Signature, Box<dyn Error>> {
        self.sign_message(SIG_DOMAIN, message, extra_data, hash_to_g1)
    }

    pub fn sign_pop<H: HashToG1>(
        &self,
        message: &[u8],
        hash_to_g1: &H,
    ) -> Result<Signature, Box<dyn Error>> {
        self.sign_message(POP_DOMAIN, &message, &[], hash_to_g1)
    }

    fn sign_message<H: HashToG1>(
        &self,
        domain: &[u8],
        message: &[u8],
        extra_data: &[u8],
        hash_to_g1: &H,
    ) -> Result<Signature, Box<dyn Error>> {
        Ok(Signature::from_sig(
            &hash_to_g1
                .hash::<Bls12_377Parameters>(domain, message, extra_data)?
                .mul(self.sk),
        ))
    }

    pub fn to_public(&self) -> PublicKey {
        PublicKey::from_pk(&G2Projective::prime_subgroup_generator().mul(self.sk))
    }
}

impl ToBytes for PrivateKey {
    #[inline]
    fn write<W: Write>(&self, mut writer: W) -> IoResult<()> {
        self.sk.write(&mut writer)
    }
}

impl FromBytes for PrivateKey {
    #[inline]
    fn read<R: Read>(mut reader: R) -> IoResult<Self> {
        let sk = Fr::read(&mut reader)?;
        Ok(PrivateKey::from_sk(&sk))
    }
}

#[derive(Debug)]
pub enum BLSError {
    VerificationFailed,
}

impl Display for BLSError {
    fn fmt(&self, f: &mut fmt::Formatter) -> fmt::Result {
        match self {
            BLSError::VerificationFailed => write!(f, "signature verification failed"),
        }
    }
}

impl Error for BLSError {
    fn source(&self) -> Option<&(dyn Error + 'static)> {
        None
    }
}

#[derive(Clone, Eq)]
pub struct PublicKey {
    pk: G2Projective,
}

impl PublicKey {
    pub fn from_pk(pk: &G2Projective) -> PublicKey {
        PublicKey { pk: pk.clone() }
    }

    pub fn get_pk(&self) -> G2Projective {
        self.pk.clone()
    }

    pub fn clone(&self) -> PublicKey {
        PublicKey::from_pk(&self.pk)
    }

    pub fn aggregate(public_keys: &[&PublicKey]) -> PublicKey {
        let mut apk = G2Projective::zero();
        for i in public_keys.iter() {
            apk = apk + &(*i).pk;
        }
        PublicKey { pk: apk }
    }

    pub fn from_vec(data: &Vec<u8>) -> IoResult<PublicKey> {
        let mut x_bytes_with_y: Vec<u8> = data.to_owned();
        let x_bytes_with_y_len = x_bytes_with_y.len();
        let y_over_half = (x_bytes_with_y[x_bytes_with_y_len - 1] & 0x80) == 0x80;
        x_bytes_with_y[x_bytes_with_y_len - 1] &= 0xFF - 0x80;
        let x = Fq2::read(x_bytes_with_y.as_slice())?;
        let x3b = <Bls12_377G2Parameters as SWModelParameters>::add_b(
            &((x.square() * &x) + &<Bls12_377G2Parameters as SWModelParameters>::mul_by_a(&x)),
        );
        let y = x3b.sqrt().ok_or(io::Error::new(
            io::ErrorKind::NotFound,
            "couldn't find square root for x",
        ))?;

        let y_c0_big = y.c0.into_repr();
        let y_c1_big = y.c1.into_repr();

        let negy = -y;

        let (bigger, smaller) = {
            let half = Fq::modulus_minus_one_div_two();
            if y_c1_big > half {
                (y, negy)
            } else if y_c1_big == half && y_c0_big > half {
                (y, negy)
            } else {
                (negy, y)
            }
        };

        let chosen_y = if y_over_half { bigger } else { smaller };
        let pk = G2Affine::new(x, chosen_y, false);
        Ok(PublicKey::from_pk(&pk.into_projective()))
    }

    pub fn verify<H: HashToG1>(
        &self,
        message: &[u8],
        extra_data: &[u8],
        signature: &Signature,
        hash_to_g1: &H,
    ) -> Result<(), Box<dyn Error>> {
        self.verify_sig(SIG_DOMAIN, message, extra_data, signature, hash_to_g1)
    }

    pub fn verify_pop<H: HashToG1>(
        &self,
        message: &[u8],
        signature: &Signature,
        hash_to_g1: &H,
    ) -> Result<(), Box<dyn Error>> {
        self.verify_sig(POP_DOMAIN, &message, &[], signature, hash_to_g1)
    }

    fn verify_sig<H: HashToG1>(
        &self,
        domain: &[u8],
        message: &[u8],
        extra_data: &[u8],
        signature: &Signature,
        hash_to_g1: &H,
    ) -> Result<(), Box<dyn Error>> {
        let pairing = Bls12_377::product_of_pairings(&vec![
            (
                signature.get_sig().into_affine().into(),
                G2Affine::prime_subgroup_generator().neg().into(),
            ),
            (
                hash_to_g1
                    .hash::<Bls12_377Parameters>(domain, message, extra_data)?
                    .into_affine()
                    .into(),
                self.pk.into_affine().into(),
            ),
        ]);
        if pairing == Fq12::one() {
            Ok(())
        } else {
            Err(BLSError::VerificationFailed)?
        }
    }
}

impl PartialEq for PublicKey {
    fn eq(&self, other: &Self) -> bool {
        // This byte-level equality operator differs from the (much slower) semantic
        // equality operator in G2Projective.  We require byte-level equality here
        // for HashSet to work correctly.  HashSet requires that item equality
        // implies hash equality.
        self.pk.x == other.pk.x && self.pk.y == other.pk.y && self.pk.z == other.pk.z
    }
}

impl Hash for PublicKey {
    fn hash<H: Hasher>(&self, state: &mut H) {
        // Only hash based on `y` for slight speed improvement
        self.pk.y.hash(state);
        // self.pk.x.hash(state);
        // self.pk.z.hash(state);
    }
}

impl ToBytes for PublicKey {
    #[inline]
    fn write<W: Write>(&self, mut writer: W) -> IoResult<()> {
        let affine = self.pk.into_affine();
        let mut x_bytes: Vec<u8> = vec![];
        let y_c0_big = affine.y.c0.into_repr();
        let y_c1_big = affine.y.c1.into_repr();
        let half = Fq::modulus_minus_one_div_two();
        affine.x.write(&mut x_bytes)?;
        let num_x_bytes = x_bytes.len();
        if y_c1_big > half {
            x_bytes[num_x_bytes - 1] |= 0x80;
        } else if y_c1_big == half && y_c0_big > half {
            x_bytes[num_x_bytes - 1] |= 0x80;
        }
        writer.write(&x_bytes)?;

        Ok(())
    }
}

impl FromBytes for PublicKey {
    #[inline]
    fn read<R: Read>(mut reader: R) -> IoResult<Self> {
        let mut x_bytes_with_y: Vec<u8> = vec![];
        reader.read_to_end(&mut x_bytes_with_y)?;
        PublicKeyCache::from_vec(&x_bytes_with_y)
    }
}

#[derive(Clone)]
pub struct Signature {
    sig: G1Projective,
}

impl Signature {
    pub fn from_sig(sig: &G1Projective) -> Signature {
        Signature { sig: sig.clone() }
    }

    pub fn get_sig(&self) -> G1Projective {
        self.sig.clone()
    }

    pub fn aggregate(signatures: &[&Signature]) -> Signature {
        let mut asig = G1Projective::zero();
        for i in signatures.iter() {
            asig = asig + &(*i).sig;
        }

        Signature { sig: asig }
    }
}

impl ToBytes for Signature {
    #[inline]
    fn write<W: Write>(&self, mut writer: W) -> IoResult<()> {
        let affine = self.sig.into_affine();
        let mut x_bytes: Vec<u8> = vec![];
        let y_big = affine.y.into_repr();
        let half = Fq::modulus_minus_one_div_two();
        affine.x.write(&mut x_bytes)?;
        if y_big > half {
            let num_x_bytes = x_bytes.len();
            x_bytes[num_x_bytes - 1] |= 0x80;
        }
        writer.write(&x_bytes)?;
        Ok(())
    }
}

impl FromBytes for Signature {
    #[inline]
    fn read<R: Read>(mut reader: R) -> IoResult<Self> {
        let mut x_bytes_with_y: Vec<u8> = vec![];
        reader.read_to_end(&mut x_bytes_with_y)?;
        let x_bytes_with_y_len = x_bytes_with_y.len();
        let y_over_half = (x_bytes_with_y[x_bytes_with_y_len - 1] & 0x80) == 0x80;
        x_bytes_with_y[x_bytes_with_y_len - 1] &= 0xFF - 0x80;
        let x = Fq::read(x_bytes_with_y.as_slice())?;
        let x3b = <Bls12_377G1Parameters as SWModelParameters>::add_b(
            &((x.square() * &x) + &<Bls12_377G1Parameters as SWModelParameters>::mul_by_a(&x)),
        );
        let y = x3b.sqrt().ok_or(io::Error::new(
            io::ErrorKind::NotFound,
            "couldn't find square root for x",
        ))?;
        let negy = -y;
        let chosen_y = if (y <= negy) ^ y_over_half { y } else { negy };
        let sig = G1Affine::new(x, chosen_y, false);
        Ok(Signature::from_sig(&sig.into_projective()))
    }
}

struct AggregateCacheState {
    keys: HashSet<PublicKey>,
    combined: G2Projective,
}

lazy_static! {
    static ref FROM_VEC_CACHE: Mutex<LruCache<Vec<u8>, PublicKey>> = Mutex::new(LruCache::new(128));
    static ref AGGREGATE_CACHE: Mutex<AggregateCacheState> = Mutex::new(AggregateCacheState {
        keys: HashSet::new(),
        combined: G2Projective::zero().clone()
    });
}

pub struct PublicKeyCache {}
impl PublicKeyCache {
    pub fn clear_cache() {
        FROM_VEC_CACHE.lock().unwrap().clear();
        let mut cache = AGGREGATE_CACHE.lock().unwrap();
        cache.keys = HashSet::new();
        cache.combined = G2Projective::zero().clone();
    }

    pub fn resize(cap: usize) {
        FROM_VEC_CACHE.lock().unwrap().resize(cap);
    }

    pub fn from_vec(data: &Vec<u8>) -> IoResult<PublicKey> {
        let cached_result = PublicKeyCache::from_vec_cached(data);
        if cached_result.is_none() {
            // cache miss
            let generated_result = PublicKey::from_vec(data)?;
            FROM_VEC_CACHE
                .lock()
                .unwrap()
                .put(data.to_owned(), generated_result.clone());
            Ok(generated_result)
        } else {
            // cache hit
            Ok(cached_result.unwrap())
        }
    }

    pub fn from_vec_cached(data: &Vec<u8>) -> Option<PublicKey> {
        let mut cache = FROM_VEC_CACHE.lock().unwrap();
        Some(cache.get(data)?.clone())
    }

    pub fn aggregate(public_keys: &[&PublicKey]) -> PublicKey {
        // The set of validators changes slowly, so for speed we will compute the
        // difference from the last call and do an incremental update
        let mut keys: HashSet<PublicKey> = HashSet::with_capacity(public_keys.len());
        for key in public_keys.iter() {
            keys.insert((*key).clone());
        }
        let mut cache = AGGREGATE_CACHE.lock().unwrap();
        let mut combined = cache.combined;

        for key in cache.keys.difference(&keys) {
            combined = combined - &(*key).pk;
        }

        for key in keys.difference(&cache.keys) {
            combined = combined + &(*key).pk;
        }

        cache.keys = keys;
        cache.combined = combined;
        PublicKey::from_pk(&combined)
    }
}

#[cfg(test)]
mod test {
    use super::*;
    use crate::{
        curve::hash::try_and_increment::TryAndIncrement,
        hash::{composite::CompositeHasher, direct::DirectHasher},
    };
    use rand::thread_rng;

    fn init() {
        let _ = env_logger::builder().is_test(true).try_init();
    }

    #[test]
    fn test_simple_sig() {
        init();

        let rng = &mut thread_rng();
        let composite_hasher = CompositeHasher::new().unwrap();
        let try_and_increment = TryAndIncrement::new(&composite_hasher);

        for _ in 0..10 {
            let mut message: Vec<u8> = vec![];
            for _ in 0..32 {
                message.push(rng.gen());
            }
            let sk = PrivateKey::generate(rng);

            let sig = sk.sign(&message[..], &[], &try_and_increment).unwrap();
            let pk = sk.to_public();
            pk.verify(&message[..], &[], &sig, &try_and_increment)
                .unwrap();
            let message2 = b"goodbye";
            pk.verify(&message2[..], &[], &sig, &try_and_increment)
                .unwrap_err();
        }
    }

    #[test]
    fn test_simple_sig_non_composite() {
        init();

        let rng = &mut thread_rng();
        let direct_hasher = DirectHasher::new().unwrap();
        let try_and_increment = TryAndIncrement::new(&direct_hasher);

        for _ in 0..10 {
            let mut message: Vec<u8> = vec![];
            for _ in 0..32 {
                message.push(rng.gen());
            }
            let sk = PrivateKey::generate(rng);

            let sig = sk.sign(&message[..], &[], &try_and_increment).unwrap();
            let pk = sk.to_public();
            pk.verify(&message[..], &[], &sig, &try_and_increment)
                .unwrap();
            let message2 = b"goodbye";
            pk.verify(&message2[..], &[], &sig, &try_and_increment)
                .unwrap_err();
        }
    }

    #[test]
    fn test_pop() {
        init();

        let rng = &mut thread_rng();
        let direct_hasher = DirectHasher::new().unwrap();
        let try_and_increment = TryAndIncrement::new(&direct_hasher);

        let sk = PrivateKey::generate(rng);
        let sk2 = PrivateKey::generate(rng);

        let pk = sk.to_public();
        let mut pk_bytes = vec![];
        pk.write(&mut pk_bytes).unwrap();

        let sig = sk.sign_pop(&pk_bytes, &try_and_increment).unwrap();

        let pk2 = sk2.to_public();
        pk.verify_pop(&pk_bytes, &sig, &try_and_increment).unwrap();
        pk2.verify_pop(&pk_bytes, &sig, &try_and_increment)
            .unwrap_err();
    }

    #[test]
    fn test_aggregated_sig() {
        let message = b"hello";
        let rng = &mut thread_rng();

        let composite_hasher = CompositeHasher::new().unwrap();
        let try_and_increment = TryAndIncrement::new(&composite_hasher);
        let sk1 = PrivateKey::generate(rng);
        let sk2 = PrivateKey::generate(rng);

        let sig1 = sk1.sign(&message[..], &[], &try_and_increment).unwrap();
        let sig2 = sk2.sign(&message[..], &[], &try_and_increment).unwrap();

        let apk = PublicKeyCache::aggregate(&[&sk1.to_public(), &sk2.to_public()]);
        let asig = Signature::aggregate(&[&sig1, &sig2]);
        apk.verify(&message[..], &[], &asig, &try_and_increment)
            .unwrap();
        apk.verify(&message[..], &[], &sig1, &try_and_increment)
            .unwrap_err();
        sk1.to_public()
            .verify(&message[..], &[], &asig, &try_and_increment)
            .unwrap_err();
        let message2 = b"goodbye";
        apk.verify(&message2[..], &[], &asig, &try_and_increment)
            .unwrap_err();

        let apk2 = PublicKeyCache::aggregate(&[&sk1.to_public()]);
        apk2.verify(&message[..], &[], &asig, &try_and_increment)
            .unwrap_err();
        apk2.verify(&message[..], &[], &sig1, &try_and_increment)
            .unwrap();

        let apk3 = PublicKeyCache::aggregate(&[&sk2.to_public(), &sk1.to_public()]);
        apk3.verify(&message[..], &[], &asig, &try_and_increment)
            .unwrap();
        apk3.verify(&message[..], &[], &sig1, &try_and_increment)
            .unwrap_err();

        let apk4 = PublicKey::aggregate(&[&sk1.to_public(), &sk2.to_public()]);
        apk4.verify(&message[..], &[], &asig, &try_and_increment)
            .unwrap();
        apk4.verify(&message[..], &[], &sig1, &try_and_increment)
            .unwrap_err();
    }

    #[test]
    fn test_public_key_serialization() {
        PublicKeyCache::resize(256);
        PublicKeyCache::clear_cache();
        let rng = &mut thread_rng();
        for _i in 0..100 {
            let sk = PrivateKey::generate(rng);
            let pk = sk.to_public();
            let mut pk_bytes = vec![];
            pk.write(&mut pk_bytes).unwrap();
            let pk2 = PublicKey::read(pk_bytes.as_slice()).unwrap();
            assert_eq!(pk.get_pk().into_affine().x, pk2.get_pk().into_affine().x);
            assert_eq!(pk.get_pk().into_affine().y, pk2.get_pk().into_affine().y);
            assert_eq!(pk2.eq(&PublicKey::read(pk_bytes.as_slice()).unwrap()), true);
        }
    }

    #[test]
    fn test_pop_single() {
        init();

        let direct_hasher = DirectHasher::new().unwrap();
        let try_and_increment = TryAndIncrement::new(&direct_hasher);

        let sk_bytes = Fr::read(
            hex::decode("e3990a59d80a91429406be0000677a7eea8b96c5b429c70c71dabc3b7cf80d0a")
                .unwrap()
                .as_slice(),
        )
        .unwrap();
        let sk = PrivateKey::from_sk(&sk_bytes);

        let pk = sk.to_public();
        let mut pk_bytes = vec![];
        pk.write(&mut pk_bytes).unwrap();
        println!("pk: {}", hex::encode(&pk_bytes));

        let sig = sk
            .sign_pop(
                &hex::decode("a0Af2E71cECc248f4a7fD606F203467B500Dd53B").unwrap(),
                &try_and_increment,
            )
            .unwrap();
        let mut sig_bytes = vec![];
        sig.write(&mut sig_bytes).unwrap();

        println!("sig: {}", hex::encode(&sig_bytes));
    }
}
