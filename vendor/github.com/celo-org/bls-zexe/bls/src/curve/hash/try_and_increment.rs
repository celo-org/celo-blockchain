use crate::{
    curve::{
        cofactor,
        hash::{HashToCurveError, HashToG2},
    },
    hash::PRF,
};
use byteorder::{LittleEndian, WriteBytesExt};
use failure::Error;
use hex;

use algebra::{
    curves::{
        models::{
            bls12::{Bls12Parameters, G2Affine, G2Projective},
            ModelParameters, SWModelParameters,
        },
        AffineCurve,
    },
    fields::{Field, Fp2, FpParameters, PrimeField, SquareRootField},
};

fn bytes_to_fp<P: Bls12Parameters>(bytes: &[u8]) -> P::Fp {
    let two = {
        let tmp = P::Fp::one();
        tmp + &tmp
    };
    let mut current_power = P::Fp::one();
    let mut element = P::Fp::zero();
    for i in bytes.iter().rev() {
        let current_byte = *i;
        let mut current_bit = 128;
        for _ in 0..8 {
            match (current_byte & current_bit) == 1 {
                true => {
                    element += &current_power;
                }
                false => {}
            }
            current_power *= &two;
            //debug!("current power: {}, elemenet: {}", current_power, element);
            current_bit = current_bit / 2;
        }
    }

    element
}

/// A try-and-increment method for hashing to G2. See page 521 in
/// https://link.springer.com/content/pdf/10.1007/3-540-45682-1_30.pdf.
pub struct TryAndIncrement<'a, H: PRF> {
    hasher: &'a H,
}

impl<'a, H: PRF> TryAndIncrement<'a, H> {
    pub fn new(h: &'a H) -> Self {
        TryAndIncrement::<H> { hasher: h }
    }
}

fn get_point_from_x<P: Bls12Parameters>(
    x: <P::G2Parameters as ModelParameters>::BaseField,
    greatest: bool,
) -> Option<G2Affine<P>> {
    // Compute x^3 + ax + b
    let x3b = <P::G2Parameters as SWModelParameters>::add_b(
        &((x.square() * &x) + &<P::G2Parameters as SWModelParameters>::mul_by_a(&x)),
    );

    x3b.sqrt().map(|y| {
        let negy = -y;

        let y = if (y < negy) ^ greatest { y } else { negy };
        G2Affine::<P>::new(x, y, false)
    })
}
impl<'a, H: PRF> HashToG2 for TryAndIncrement<'a, H> {
    fn hash<P: Bls12Parameters>(&self, domain: &[u8], message: &[u8]) -> Result<G2Projective<P>, Error> {
        const NUM_TRIES: usize = 10000;
        const EXTRA_BITS: usize = 135;
        const EXPECTED_TOTAL_BITS: usize = 1024;

        let fp_bits = <P::Fp as PrimeField>::Params::MODULUS_BITS as usize;
        let fp_bits_with_extra = fp_bits + EXTRA_BITS;
        let num_bits = 2 * fp_bits_with_extra; //2*(Fq + EXTRA_BITS), generate 2 field elements with extra bits to reduce modulo bias
        assert_eq!(num_bits, EXPECTED_TOTAL_BITS);
        let message_hash = self.hasher.crh(message)?;
        let mut counter: [u8; 4] = [0; 4];
        for c in 1..NUM_TRIES {
            (&mut counter[..]).write_u32::<LittleEndian>(c as u32)?;
            let hash = self
                .hasher
                .prf(domain, &[&counter, message_hash.as_slice()].concat(), num_bits)?;
            let possible_x = {
                let possible_x_0 = bytes_to_fp::<P>(&hash[..hash.len() / 2]);
                let possible_x_1 = bytes_to_fp::<P>(&hash[hash.len() / 2..]);
                Fp2::<P::Fp2Params>::new(possible_x_0, possible_x_1)
            };
            match get_point_from_x::<P>(possible_x, true) {
                None => continue,
                Some(x) => {
                    debug!(
                        "succeeded hashing \"{}\" to G2 in {} tries",
                        hex::encode(message),
                        c
                    );
                    return Ok(cofactor::scale_by_cofactor_fuentes::<P>(
                        &x.into_projective(),
                    ));
                }
            }
        }
        Err(HashToCurveError::CannotFindPoint)?
    }
}

#[cfg(test)]
mod test {

    use crate::{
        curve::hash::{try_and_increment::TryAndIncrement, HashToG2},
        hash::composite::CompositeHasher,
    };

    use algebra::curves::bls12_377::{Bls12_377Parameters, G2Projective};

    #[test]
    fn test_hash_to_curve() {
        let composite_hasher = CompositeHasher::new().unwrap();
        let try_and_increment = TryAndIncrement::new(&composite_hasher);
        let _g: G2Projective = try_and_increment.hash::<Bls12_377Parameters>(&[], &[]).unwrap();
    }
}
