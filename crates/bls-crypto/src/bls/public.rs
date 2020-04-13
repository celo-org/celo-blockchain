use crate::{BLSError, HashToCurve, PrivateKey, PublicKeyCache, Signature, POP_DOMAIN, SIG_DOMAIN};

use algebra::{
    bls12_377::{
        g2::Parameters as Bls12_377G2Parameters, Bls12_377, Fq, Fq12, Fq2, G1Projective, G2Affine,
        G2Projective,
    },
    bytes::{FromBytes, ToBytes},
    curves::SWModelParameters,
    AffineCurve, CanonicalDeserialize, CanonicalSerialize, Field, One, PairingEngine, PrimeField,
    ProjectiveCurve, SerializationError, SquareRootField, Zero,
};
use std::hash::{Hash, Hasher};

use crate::BlsResult;

use std::{
    io::{self, Read, Result as IoResult, Write},
    ops::Neg,
};

/// A BLS public key on G2
#[derive(Clone, Eq, Debug)]
pub struct PublicKey(G2Projective);

impl From<G2Projective> for PublicKey {
    fn from(pk: G2Projective) -> PublicKey {
        PublicKey(pk)
    }
}

impl From<&PrivateKey> for PublicKey {
    fn from(pk: &PrivateKey) -> PublicKey {
        PublicKey::from(G2Projective::prime_subgroup_generator().mul(*pk.as_ref()))
    }
}

impl AsRef<G2Projective> for PublicKey {
    fn as_ref(&self) -> &G2Projective {
        &self.0
    }
}

impl PublicKey {
    pub fn aggregate(public_keys: &[PublicKey]) -> PublicKey {
        let mut apk = G2Projective::zero();
        for pk in public_keys.iter() {
            apk = apk + pk.as_ref();
        }
        apk.into()
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
        Ok(PublicKey::from(pk.into_projective()))
    }

    pub fn verify<H: HashToCurve<Output = G1Projective>>(
        &self,
        message: &[u8],
        extra_data: &[u8],
        signature: &Signature,
        hash_to_g1: &H,
    ) -> BlsResult<()> {
        self.verify_sig(SIG_DOMAIN, message, extra_data, signature, hash_to_g1)
    }

    pub fn verify_pop<H: HashToCurve<Output = G1Projective>>(
        &self,
        message: &[u8],
        signature: &Signature,
        hash_to_g1: &H,
    ) -> BlsResult<()> {
        self.verify_sig(POP_DOMAIN, &message, &[], signature, hash_to_g1)
    }

    fn verify_sig<H: HashToCurve<Output = G1Projective>>(
        &self,
        domain: &[u8],
        message: &[u8],
        extra_data: &[u8],
        signature: &Signature,
        hash_to_g1: &H,
    ) -> BlsResult<()> {
        let pairing = Bls12_377::product_of_pairings(&vec![
            (
                signature.as_ref().into_affine().into(),
                G2Affine::prime_subgroup_generator().neg().into(),
            ),
            (
                hash_to_g1
                    .hash(domain, message, extra_data)?
                    .into_affine()
                    .into(),
                self.0.into_affine().into(),
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
        let a = self.as_ref();
        let b = other.as_ref();
        a.x == b.x && a.y == b.y && a.z == b.z
    }
}

impl Hash for PublicKey {
    fn hash<H: Hasher>(&self, state: &mut H) {
        // Only hash based on `y` for slight speed improvement
        self.0.y.hash(state);
        // self.pk.x.hash(state);
        // self.pk.z.hash(state);
    }
}

impl ToBytes for PublicKey {
    #[inline]
    fn write<W: Write>(&self, mut writer: W) -> IoResult<()> {
        let affine = self.0.into_affine();
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

impl CanonicalSerialize for PublicKey {
    fn serialize<W: Write>(&self, writer: &mut W) -> Result<(), SerializationError> {
        self.0.into_affine().serialize(writer)
    }

    fn serialize_uncompressed<W: Write>(&self, writer: &mut W) -> Result<(), SerializationError> {
        self.0.into_affine().serialize_uncompressed(writer)
    }

    fn serialized_size(&self) -> usize {
        self.0.into_affine().serialized_size()
    }
}

impl CanonicalDeserialize for PublicKey {
    fn deserialize<R: Read>(reader: &mut R) -> Result<Self, SerializationError> {
        Ok(PublicKey::from(
            G2Affine::deserialize(reader)?.into_projective(),
        ))
    }

    fn deserialize_uncompressed<R: Read>(reader: &mut R) -> Result<Self, SerializationError> {
        Ok(PublicKey::from(
            G2Affine::deserialize_uncompressed(reader)?.into_projective(),
        ))
    }
}

#[cfg(test)]
mod test {
    use super::*;
    use crate::bls::PrivateKey;
    use rand::thread_rng;

    #[test]
    fn test_public_key_serialization() {
        PublicKeyCache::resize(256);
        PublicKeyCache::clear_cache();

        let rng = &mut thread_rng();
        for _ in 0..100 {
            let sk = PrivateKey::generate(rng);
            let pk = sk.to_public();

            let mut pk_bytes = vec![];
            pk.write(&mut pk_bytes).unwrap();

            let mut pk_bytes2 = vec![];
            pk.serialize(&mut pk_bytes2).unwrap();

            assert_eq!(pk_bytes, pk_bytes2);

            let de_pk = PublicKey::read(&pk_bytes[..]).unwrap();
            let de_pk2 = PublicKey::deserialize(&mut &pk_bytes[..]).unwrap();

            assert_eq!(de_pk, de_pk2);

            // check that the points match (the PartialEq does only bytes equality)
            assert_eq!(de_pk.as_ref().x, de_pk2.as_ref().x);
            assert_eq!(de_pk.as_ref().y, de_pk2.as_ref().y);
        }
    }
}
