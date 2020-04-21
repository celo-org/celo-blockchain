use crate::curve::hash::HashToG1;
use lazy_static::lazy_static;
use lru::LruCache;
use std::borrow::Borrow;
use std::{
    collections::HashSet,
    hash::{Hash, Hasher},
    sync::Mutex,
};

use algebra::{
    bls12_377::{
        Bls12_377, Fq12, Fr, G1Affine, G1Projective, G2Affine, G2Projective,
        Parameters as Bls12_377Parameters,
    },
    bytes::{FromBytes, ToBytes},
    serialize::{CanonicalDeserialize, CanonicalSerialize},
    AffineCurve, One, PairingEngine, ProjectiveCurve, UniformRand, Zero,
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

#[derive(Clone, Debug)]
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
            hash_to_g1
                .hash::<Bls12_377Parameters>(domain, message, extra_data)?
                .mul(self.sk),
        ))
    }

    pub fn to_public(&self) -> PublicKey {
        PublicKey::from_pk(G2Projective::prime_subgroup_generator().mul(self.sk))
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
    HashToCurveFailed(Vec<u8>, Vec<u8>),
}

impl Display for BLSError {
    fn fmt(&self, f: &mut fmt::Formatter) -> fmt::Result {
        match self {
            BLSError::VerificationFailed => write!(f, "signature verification failed"),
            BLSError::HashToCurveFailed(msg, data) => write!(
                f,
                "could not hash to curve (msg: {}, extra data: {})",
                hex::encode(msg),
                hex::encode(data)
            ),
        }
    }
}

impl Error for BLSError {
    fn source(&self) -> Option<&(dyn Error + 'static)> {
        None
    }
}

#[derive(Clone, Eq, Debug)]
pub struct PublicKey {
    pk: G2Projective,
}

impl PublicKey {
    pub fn from_pk(pk: G2Projective) -> PublicKey {
        PublicKey { pk }
    }

    pub fn get_pk(&self) -> G2Projective {
        self.pk.clone()
    }

    pub fn clone(&self) -> PublicKey {
        PublicKey::from_pk(self.pk)
    }

    pub fn aggregate(public_keys: &[PublicKey]) -> PublicKey {
        let mut apk = G2Projective::zero();
        for i in public_keys.iter() {
            apk = apk + &(*i).pk;
        }
        PublicKey { pk: apk }
    }

    pub fn from_vec(data: &Vec<u8>) -> IoResult<PublicKey> {
        let p = G2Affine::deserialize(&mut &data[..])
            .map_err(|_| io::ErrorKind::InvalidInput)?
            .into_projective();
        Ok(PublicKey::from_pk(p))
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
        affine
            .serialize(&mut writer)
            .map_err(|_| io::ErrorKind::InvalidInput.into())
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

#[derive(Clone, Debug, PartialEq)]
pub struct Signature {
    sig: G1Projective,
}

impl Signature {
    pub fn from_sig(sig: G1Projective) -> Signature {
        Signature { sig }
    }

    pub fn get_sig(&self) -> G1Projective {
        self.sig
    }

    /// Sums the provided signatures to produce the aggregate signature.
    pub fn aggregate<S: Borrow<Signature>>(signatures: &[S]) -> Signature {
        let mut asig = G1Projective::zero();
        for i in signatures.iter() {
            asig = asig + &(*i).borrow().sig;
        }

        Signature { sig: asig }
    }

    /// Verifies the signature against a vector of pubkey & message tuples, for the provided
    /// messages domain.
    ///
    /// For each message, an optional extra_data field can be provided (empty otherwise).
    ///
    /// The provided hash_to_g1 implementation will be used to hash each message-extra_data pair
    /// to G1.
    ///
    /// The verification equation can be found in pg.11 from
    /// https://eprint.iacr.org/2018/483.pdf: "Batch verification"
    pub fn batch_verify<H: HashToG1, P: Borrow<PublicKey>>(
        &self,
        pubkeys: &[P],
        domain: &[u8],
        messages: &[(&[u8], &[u8])],
        hash_to_g1: &H,
    ) -> Result<(), BLSError> {
        let message_hashes = messages
            .iter()
            .map(|(message, extra_data)| {
                hash_to_g1
                    .hash::<Bls12_377Parameters>(domain, message, extra_data)
                    .map_err(|_| BLSError::HashToCurveFailed(message.to_vec(), extra_data.to_vec()))
            })
            .collect::<Result<Vec<G1Projective>, _>>()?;

        self.batch_verify_hashes(pubkeys, &message_hashes)
    }

    /// Verifies the signature against a vector of pubkey & message hash tuples
    /// This is a lower level method, if you prefer hashing to be done internally,
    /// consider using the `batch_verify` method.
    ///
    /// The verification equation can be found in pg.11 from
    /// https://eprint.iacr.org/2018/483.pdf: "Batch verification"
    pub fn batch_verify_hashes<P: Borrow<PublicKey>>(
        &self,
        pubkeys: &[P],
        message_hashes: &[G1Projective],
    ) -> Result<(), BLSError> {
        // `.into()` is needed to prepared the points
        let mut els = vec![(
            self.get_sig().into_affine().into(),
            G2Affine::prime_subgroup_generator().neg().into(),
        )];
        message_hashes
            .iter()
            .zip(pubkeys)
            .for_each(|(hash, pubkey)| {
                els.push((
                    hash.into_affine().into(),
                    pubkey.borrow().get_pk().into_affine().into(),
                ));
            });

        let pairing = Bls12_377::product_of_pairings(&els);
        if pairing == Fq12::one() {
            Ok(())
        } else {
            Err(BLSError::VerificationFailed)?
        }
    }
}

impl ToBytes for Signature {
    #[inline]
    fn write<W: Write>(&self, mut writer: W) -> IoResult<()> {
        let affine = self.sig.into_affine();
        affine
            .serialize(&mut writer)
            .map_err(|_| io::ErrorKind::InvalidInput.into())
    }
}

impl FromBytes for Signature {
    #[inline]
    fn read<R: Read>(mut reader: R) -> IoResult<Self> {
        let p = G1Affine::deserialize(&mut reader)
            .map_err(|_| io::ErrorKind::InvalidInput)?
            .into_projective();
        Ok(Signature::from_sig(p))
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

    pub fn aggregate(public_keys: &[PublicKey]) -> PublicKey {
        // The set of validators changes slowly, so for speed we will compute the
        // difference from the last call and do an incremental update
        let mut keys: HashSet<PublicKey> = HashSet::with_capacity(public_keys.len());
        for key in public_keys {
            keys.insert(key.clone());
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
        PublicKey::from_pk(combined)
    }
}

#[cfg(test)]
mod test {
    use super::*;
    use crate::test_helpers::{keygen_batch, sign_batch, sum};
    use crate::{
        bls::ffi::{Message, MessageFFI},
        curve::hash::try_and_increment::TryAndIncrement,
        hash::{composite::CompositeHasher, direct::DirectHasher, XOF},
    };
    use algebra::{
        bls12_377::{
            g1::Parameters as Bls12_377G1Parameters, g2::Parameters as Bls12_377G2Parameters, Fq,
            Fq2,
        },
        curves::{ProjectiveCurve, SWModelParameters},
        fields::{Field, PrimeField, SquareRootField},
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
    fn test_batch_verify() {
        init();

        let direct_hasher = DirectHasher::new().unwrap();
        let composite_hasher = CompositeHasher::new().unwrap();

        test_batch_verify_with_hasher(direct_hasher, false);
        test_batch_verify_with_hasher(composite_hasher, true);
    }

    fn test_batch_verify_with_hasher<X: XOF>(hasher: X, is_composite: bool) {
        let rng = &mut thread_rng();
        let try_and_increment = TryAndIncrement::new(&hasher);
        let num_epochs = 10;
        let num_validators = 7;

        // generate some msgs and extra data
        let mut msgs = Vec::new();
        for _ in 0..num_epochs {
            let message: Vec<u8> = (0..32).map(|_| rng.gen()).collect::<Vec<u8>>();
            let extra_data: Vec<u8> = (0..32).map(|_| rng.gen()).collect::<Vec<u8>>();
            msgs.push((message, extra_data));
        }
        let msgs = msgs
            .iter()
            .map(|(m, d)| (m.as_ref(), d.as_ref()))
            .collect::<Vec<_>>();

        // get each signed by a committee _on the same domain_ and get the agg sigs of the commitee
        let mut asig = G1Projective::zero();
        let mut pubkeys = Vec::new();
        let mut sigs = Vec::new();
        for i in 0..num_epochs {
            let mut epoch_pubkey = G2Projective::zero();
            let mut epoch_sig = G1Projective::zero();
            for _ in 0..num_validators {
                let sk = PrivateKey::generate(rng);
                let s = sk.sign(&msgs[i].0, &msgs[i].1, &try_and_increment).unwrap();

                epoch_sig += s.sig;
                epoch_pubkey += sk.to_public().pk;
            }

            pubkeys.push(PublicKey::from_pk(epoch_pubkey));
            sigs.push(Signature::from_sig(epoch_sig));

            asig += epoch_sig;
        }

        let asig = Signature::from_sig(asig);

        let res = asig.batch_verify(&pubkeys, SIG_DOMAIN, &msgs, &try_and_increment);

        assert!(res.is_ok());

        let mut messages = Vec::new();
        for i in 0..num_epochs {
            messages.push(Message {
                data: msgs[i].0,
                extra: msgs[i].1,
                public_key: &pubkeys[i],
                sig: &sigs[i],
            });
        }

        let msgs_ffi = messages
            .iter()
            .map(|m| MessageFFI::from(m))
            .collect::<Vec<_>>();

        let mut verified: bool = false;

        let success = crate::batch_verify_signature(
            &msgs_ffi[0] as *const MessageFFI,
            msgs_ffi.len(),
            is_composite,
            &mut verified as *mut bool,
        );
        assert!(success);
        assert!(verified);
    }

    #[test]
    fn batch_verify_hashes() {
        // generate 5 (aggregate sigs, message hash pairs)
        // verify them all in 1 call
        let batch_size = 5;
        let num_keys = 7;
        let rng = &mut rand::thread_rng();

        // generate some random messages
        let messages = (0..batch_size)
            .map(|_| G1Projective::rand(rng))
            .collect::<Vec<_>>();
        //
        // keygen for multiple rounds (7 keys per round)
        let (secret_keys, public_keys_batches) = keygen_batch::<Bls12_377>(batch_size, num_keys);

        // get the aggregate public key for each rounds
        let aggregate_pubkeys = public_keys_batches
            .iter()
            .map(|pks| sum(pks))
            .map(PublicKey::from_pk)
            .collect::<Vec<_>>();

        // the keys from each epoch sign the messages from the corresponding epoch
        let asigs = sign_batch::<Bls12_377>(&secret_keys, &messages);

        // get the complete aggregate signature
        let asig = sum(&asigs);
        let asig = Signature::from_sig(asig);

        let res = asig.batch_verify_hashes(&aggregate_pubkeys, &messages);

        assert!(res.is_ok());
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
        let sigs = &[sig1, sig2];

        let apk = PublicKeyCache::aggregate(&[sk1.to_public(), sk2.to_public()]);
        let asig = Signature::aggregate(sigs);
        apk.verify(&message[..], &[], &asig, &try_and_increment)
            .unwrap();
        apk.verify(&message[..], &[], &sigs[0], &try_and_increment)
            .unwrap_err();
        sk1.to_public()
            .verify(&message[..], &[], &asig, &try_and_increment)
            .unwrap_err();
        let message2 = b"goodbye";
        apk.verify(&message2[..], &[], &asig, &try_and_increment)
            .unwrap_err();

        let apk2 = PublicKeyCache::aggregate(&[sk1.to_public()]);
        apk2.verify(&message[..], &[], &asig, &try_and_increment)
            .unwrap_err();
        apk2.verify(&message[..], &[], &sigs[0], &try_and_increment)
            .unwrap();

        let apk3 = PublicKeyCache::aggregate(&[sk2.to_public(), sk1.to_public()]);
        apk3.verify(&message[..], &[], &asig, &try_and_increment)
            .unwrap();
        apk3.verify(&message[..], &[], &sigs[0], &try_and_increment)
            .unwrap_err();

        let apk4 = PublicKey::aggregate(&[sk1.to_public(), sk2.to_public()]);
        apk4.verify(&message[..], &[], &asig, &try_and_increment)
            .unwrap();
        apk4.verify(&message[..], &[], &sigs[0], &try_and_increment)
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

    // this has the bug mentioned in https://github.com/celo-org/bls-zexe/issues/149.
    // c1 should have been checked to be equal to -c1 or equivalently 0.
    fn old_serialization_logic(pk: PublicKey, writer: &mut Vec<u8>) -> IoResult<()> {
        let affine = pk.get_pk().into_affine();
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

    fn old_deserialization_logic(data: &[u8]) -> IoResult<PublicKey> {
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
        Ok(PublicKey::from_pk(pk.into_projective()))
    }

    #[test]
    fn old_serialization_logic_matches_new() {
        let rng = &mut thread_rng();
        // Check cases where c1 != 0, which are the normal case.
        for _ in 0..1000 {
            let sk = PrivateKey::generate(rng);
            let pk = sk.to_public();
            if pk.get_pk().into_affine().y.c1 == Fq::zero() {
                // If it happens, we want to know about it.
                panic!(format!(
                    "point had c1 = 0! point was: {}",
                    pk.get_pk().into_affine()
                ));
            }

            let mut pk_bytes1 = vec![];
            let mut pk_bytes2 = vec![];
            pk.write(&mut pk_bytes1).unwrap();
            old_serialization_logic(pk.clone(), &mut pk_bytes2).unwrap();
            assert_eq!(pk_bytes1, pk_bytes2);

            let de_pk1 = PublicKey::read(&pk_bytes1[..]).unwrap().get_pk();
            let de_pk2 = old_deserialization_logic(&pk_bytes2[..]).unwrap().get_pk();
            assert_eq!(de_pk1, de_pk2);
            assert_eq!(pk.get_pk(), de_pk1);
        }
    }

    #[test]
    fn issue_149() {
        let rng = &mut thread_rng();
        // Check cases where c1 = 0. These don't occur normally and in fact the manually patched
        // points are not on the curve.
        for _ in 0..1000 {
            let sk = PrivateKey::generate(rng);

            // construct a publickey with y.c1 == 0
            let pk = sk.to_public();
            let mut pk_affine = pk.get_pk().into_affine();
            pk_affine.y.c1 = Fq::zero();
            let pk = PublicKey::from_pk(pk_affine.into_projective());

            let mut pk_bytes1 = vec![];
            let mut pk_bytes2 = vec![];
            pk.write(&mut pk_bytes1).unwrap();
            old_serialization_logic(pk.clone(), &mut pk_bytes2).unwrap();
            // check for the bug mentioned in https://github.com/celo-org/bls-zexe/issues/149.
            // If c1 == 0, and c0 > half (or equivalently c0 > -c0), we get the wrong y bit, and
            // serialization will produce the negative of the point.
            if pk_affine.y > pk_affine.y.neg() {
                assert_ne!(pk_bytes1, pk_bytes2);
            } else {
                assert_eq!(pk_bytes1, pk_bytes2);
            }

            let de_pk1 = PublicKey::read(&pk_bytes1[..]).unwrap().get_pk();
            let de_pk2 = old_deserialization_logic(&pk_bytes2[..]).unwrap().get_pk();

            // if they had the same serialization, they have the same deserialization
            if pk_affine.y > pk_affine.y.neg() {
                assert_ne!(de_pk1, de_pk2);
            } else {
                assert_eq!(de_pk1, de_pk2);
            }

            // there's no way to make them equal to the original pk
            assert_ne!(de_pk1, pk.get_pk());
            assert_ne!(de_pk2, pk.get_pk());
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

    #[test]
    fn test_signature_serialization() {
        let hasher = CompositeHasher::new().unwrap();
        let try_and_increment = TryAndIncrement::new(&hasher);

        let rng = &mut thread_rng();

        let old_serialization_logic = |sig: Signature, writer: &mut Vec<u8>| -> IoResult<_> {
            let affine = sig.get_sig().into_affine();
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
        };

        let old_deserialization_logic = |data: &[u8]| -> IoResult<_> {
            let mut x_bytes_with_y: Vec<u8> = data.to_owned();
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
            Ok(Signature::from_sig(sig.into_projective()))
        };

        for _ in 0..100 {
            let message = b"hello";
            let sk = PrivateKey::generate(rng);
            let sig = sk.sign(&message[..], &[], &try_and_increment).unwrap();

            let mut sig_bytes1 = vec![];
            sig.write(&mut sig_bytes1).unwrap();

            let mut sig_bytes2 = vec![];
            old_serialization_logic(sig.clone(), &mut sig_bytes2).unwrap();

            // both methods have the same result
            assert_eq!(sig_bytes1, sig_bytes2);

            let de_sig1 = Signature::read(&sig_bytes1[..]).unwrap();
            let de_sig2 = old_deserialization_logic(&sig_bytes2[..]).unwrap();

            // both deserialization methods have the same result
            assert_eq!(de_sig1, sig);
            assert_eq!(de_sig2, sig);
        }
    }
}
