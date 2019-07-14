use crate::curve::hash::HashToG2;
use algebra::{
    bytes::{FromBytes, ToBytes},
    curves::{
        bls12_377::{
            Bls12_377, Bls12_377Parameters, G1Affine, G1Projective, G2Affine, G2Projective,
        },
        AffineCurve, PairingCurve, PairingEngine, ProjectiveCurve,
    },
    fields::{
        bls12_377::{Fq12, Fr},
        Field,
    },
};

use failure::Error;
use rand::Rng;

static SIG_DOMAIN: &'static [u8] = b"ULforprf";
static POP_DOMAIN: &'static [u8] = b"ULforpop";

/// Implements BLS signatures as specified in https://crypto.stanford.edu/~dabo/pubs/papers/BLSmultisig.html.
use std::{
    io::{Read, Result as IoResult, Write},
    ops::{Mul, Neg},
};

pub struct PrivateKey {
    sk: Fr,
}

impl PrivateKey {
    pub fn generate<R: Rng>(rng: &mut R) -> PrivateKey {
        PrivateKey { sk: rng.gen() }
    }

    pub fn from_sk(sk: &Fr) -> PrivateKey {
        PrivateKey { sk: sk.clone() }
    }

    pub fn get_sk(&self) -> Fr {
        self.sk.clone()
    }

    pub fn sign<H: HashToG2>(&self, message: &[u8], hash_to_g2: &H) -> Result<Signature, Error> {
        self.sign_message(SIG_DOMAIN, message, hash_to_g2)
    }

    pub fn sign_pop<H: HashToG2>(&self, hash_to_g2: &H) -> Result<Signature, Error> {
        let pubkey = self.to_public();
        let mut pubkey_bytes = vec![];
        pubkey.write(&mut pubkey_bytes)?;

        self.sign_message(POP_DOMAIN, &pubkey_bytes, hash_to_g2)
    }


    fn sign_message<H: HashToG2>(&self, domain: &[u8], message: &[u8], hash_to_g2: &H) -> Result<Signature, Error> {
        Ok(Signature::from_sig(
            &hash_to_g2
                .hash::<Bls12_377Parameters>(domain, message)?
                .mul(&self.sk),
        ))
    }

    pub fn to_public(&self) -> PublicKey {
        PublicKey::from_pk(&G1Projective::prime_subgroup_generator().mul(&self.sk))
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

#[derive(Debug, Fail)]
pub enum BLSError {
    #[fail(display = "signature verification failed")]
    VerificationFailed,
}

pub struct PublicKey {
    pk: G1Projective,
}

impl PublicKey {
    pub fn from_pk(pk: &G1Projective) -> PublicKey {
        PublicKey { pk: pk.clone() }
    }

    pub fn get_pk(&self) -> G1Projective {
        self.pk.clone()
    }

    pub fn aggregate(public_keys: &[&PublicKey]) -> PublicKey {
        let mut apk = G1Projective::zero();
        for i in public_keys.iter() {
            apk = apk + &(*i).pk;
        }

        PublicKey { pk: apk }
    }

    pub fn verify<H: HashToG2>(
        &self,
        message: &[u8],
        signature: &Signature,
        hash_to_g2: &H,
    ) -> Result<(), Error> {
        self.verify_sig(SIG_DOMAIN, message, signature, hash_to_g2)
    }

    pub fn verify_pop<H: HashToG2>(
        &self,
        signature: &Signature,
        hash_to_g2: &H,
    ) -> Result<(), Error> {
        let mut pubkey_bytes = vec![];
        self.write(&mut pubkey_bytes)?;
        self.verify_sig(POP_DOMAIN, &pubkey_bytes, signature, hash_to_g2)
    }


    fn verify_sig<H: HashToG2>(
        &self,
        domain: &[u8],
        message: &[u8],
        signature: &Signature,
        hash_to_g2: &H,
    ) -> Result<(), Error> {
        let pairing = Bls12_377::product_of_pairings(&vec![
            (
                &G1Affine::prime_subgroup_generator().neg().prepare(),
                &signature.get_sig().into_affine().prepare(),
            ),
            (
                &self.pk.into_affine().prepare(),
                &hash_to_g2
                    .hash::<Bls12_377Parameters>(domain, message)?
                    .into_affine()
                    .prepare(),
            ),
        ]);
        if pairing == Fq12::one() {
            Ok(())
        } else {
            Err(BLSError::VerificationFailed)?
        }
    }
}

impl ToBytes for PublicKey {
    #[inline]
    fn write<W: Write>(&self, mut writer: W) -> IoResult<()> {
        self.pk.into_affine().write(&mut writer)
    }
}

impl FromBytes for PublicKey {
    #[inline]
    fn read<R: Read>(mut reader: R) -> IoResult<Self> {
        let pk = G1Affine::read(&mut reader)?;
        Ok(PublicKey::from_pk(&pk.into_projective()))
    }
}

pub struct Signature {
    sig: G2Projective,
}

impl Signature {
    pub fn from_sig(sig: &G2Projective) -> Signature {
        Signature { sig: sig.clone() }
    }

    pub fn get_sig(&self) -> G2Projective {
        self.sig.clone()
    }

    pub fn aggregate(signatures: &[&Signature]) -> Signature {
        let mut asig = G2Projective::zero();
        for i in signatures.iter() {
            asig = asig + &(*i).sig;
        }

        Signature { sig: asig }
    }
}

impl ToBytes for Signature {
    #[inline]
    fn write<W: Write>(&self, mut writer: W) -> IoResult<()> {
        self.sig.into_affine().write(&mut writer)
    }
}

impl FromBytes for Signature {
    #[inline]
    fn read<R: Read>(mut reader: R) -> IoResult<Self> {
        let sig = G2Affine::read(&mut reader)?;
        Ok(Signature::from_sig(&sig.into_projective()))
    }
}

#[cfg(test)]
mod test {
    use super::*;
    use crate::{
        curve::hash::try_and_increment::TryAndIncrement, hash::{
            direct::DirectHasher,
            composite::CompositeHasher,
        },
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

            let sig = sk.sign(&message[..], &try_and_increment).unwrap();
            let pk = sk.to_public();
            pk.verify(&message[..], &sig, &try_and_increment).unwrap();
            let message2 = b"goodbye";
            pk.verify(&message2[..], &sig, &try_and_increment)
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

            let sig = sk.sign(&message[..], &try_and_increment).unwrap();
            let pk = sk.to_public();
            pk.verify(&message[..], &sig, &try_and_increment).unwrap();
            let message2 = b"goodbye";
            pk.verify(&message2[..], &sig, &try_and_increment)
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

        let sig = sk.sign_pop(&try_and_increment).unwrap();
        let pk = sk.to_public();
        let pk2 = sk2.to_public();
        pk.verify_pop(&sig, &try_and_increment).unwrap();
        pk2.verify_pop(&sig, &try_and_increment)
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

        let sig1 = sk1.sign(&message[..], &try_and_increment).unwrap();
        let sig2 = sk2.sign(&message[..], &try_and_increment).unwrap();

        let apk = PublicKey::aggregate(&[&sk1.to_public(), &sk2.to_public()]);
        let asig = Signature::aggregate(&[&sig1, &sig2]);
        apk.verify(&message[..], &asig, &try_and_increment).unwrap();
        apk.verify(&message[..], &sig1, &try_and_increment)
            .unwrap_err();
        sk1.to_public()
            .verify(&message[..], &asig, &try_and_increment)
            .unwrap_err();
        let message2 = b"goodbye";
        apk.verify(&message2[..], &asig, &try_and_increment)
            .unwrap_err();
    }
}
