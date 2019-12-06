use crate::{
    curve::{
        cofactor,
        hash::{HashToCurveError, HashToG1, HashToG2},
    },
    hash::XOF,
};
use byteorder::WriteBytesExt;
use hex;

use algebra::{
    curves::{
        models::{
            bls12::{Bls12Parameters, G1Affine, G2Affine, G1Projective, G2Projective},
            ModelParameters, SWModelParameters,
        },
        AffineCurve,
    },
    fields::{Field, Fp2, FpParameters, PrimeField, SquareRootField},
    bytes::FromBytes,
};
use std::error::Error;

#[allow(dead_code)]
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

/// A try-and-increment method for hashing to G1 and G2. See page 521 in
/// https://link.springer.com/content/pdf/10.1007/3-540-45682-1_30.pdf.
pub struct TryAndIncrement<'a, H: XOF> {
    hasher: &'a H,
}

impl<'a, H: XOF> TryAndIncrement<'a, H> {
    pub fn new(h: &'a H) -> Self {
        TryAndIncrement::<H> { hasher: h }
    }
}

fn get_point_from_x_g1<P: Bls12Parameters>(
    x: <P::G1Parameters as ModelParameters>::BaseField,
    greatest: bool,
) -> Option<G1Affine<P>> {
    // Compute x^3 + ax + b
    let x3b = <P::G1Parameters as SWModelParameters>::add_b(
        &((x.square() * &x) + &<P::G1Parameters as SWModelParameters>::mul_by_a(&x)),
    );

    x3b.sqrt().map(|y| {
        let negy = -y;

        let y = if (y < negy) ^ greatest { y } else { negy };
        G1Affine::<P>::new(x, y, false)
    })
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
impl<'a, H: XOF> HashToG1 for TryAndIncrement<'a, H> {
    fn hash<P: Bls12Parameters>(&self, domain: &[u8], message: &[u8], extra_data: &[u8]) -> Result<G1Projective<P>, Box<dyn Error>> {
        const NUM_TRIES: usize = 256;
        const EXPECTED_TOTAL_BITS: usize = 384;
        const LAST_BYTE_MASK: u8 = 1;
        const GREATEST_MASK: u8 = 2;

        //round up to a multiple of 8
        let fp_bits = (((<P::Fp as PrimeField>::Params::MODULUS_BITS as f64)/8.0).ceil() as usize)*8;
        let num_bits = fp_bits;
        assert_eq!(num_bits, EXPECTED_TOTAL_BITS);
        let num_bytes = num_bits / 8;
        let mut counter: [u8; 1] = [0; 1];
        let hash_loop_time = start_timer!(|| "try_and_increment::hash_loop");
        for c in 0..NUM_TRIES {
            (&mut counter[..]).write_u8(c as u8)?;
            let hash = self
                .hasher
                .hash(domain, &[&counter, extra_data, &message].concat(), num_bytes)?;
            let (possible_x, greatest) = {
                //zero out the last byte except the first bit, to get to a total of 377 bits
                let mut possible_x_bytes = hash.to_vec();
                let possible_x_bytes_len = possible_x_bytes.len();
                possible_x_bytes[possible_x_bytes_len - 1] &= LAST_BYTE_MASK;
                let possible_x = P::Fp::read(possible_x_bytes.as_slice())?;
                if possible_x == P::Fp::zero() {
                    continue;
                }
                let greatest = (possible_x_bytes[possible_x_bytes_len - 1] & GREATEST_MASK) == GREATEST_MASK;

                (possible_x, greatest)
            };
            match get_point_from_x_g1::<P>(possible_x, greatest) {
                None => continue,
                Some(x) => {
                    debug!(
                        "succeeded hashing \"{}\" to G2 in {} tries",
                        hex::encode(message),
                        c
                    );
                    end_timer!(hash_loop_time);
                    return Ok(cofactor::scale_by_cofactor_g1::<P>(
                        &x.into_projective(),
                    ));
                }
            }
        }
        Err(HashToCurveError::CannotFindPoint)?
    }
}

impl<'a, H: XOF> HashToG2 for TryAndIncrement<'a, H> {
    fn hash<P: Bls12Parameters>(&self, domain: &[u8], message: &[u8], extra_data: &[u8]) -> Result<G2Projective<P>, Box<dyn Error>> {
        const NUM_TRIES: usize = 256;
        const EXPECTED_TOTAL_BITS: usize = 384*2;
        const LAST_BYTE_MASK: u8 = 1;
        const GREATEST_MASK: u8 = 2;

        //round up to a multiple of 8
        let fp_bits = (((<P::Fp as PrimeField>::Params::MODULUS_BITS as f64)/8.0).ceil() as usize)*8;
        let num_bits = 2 * fp_bits;
        assert_eq!(num_bits, EXPECTED_TOTAL_BITS);
        let num_bytes = num_bits / 8;
        let mut counter: [u8; 1] = [0; 1];
        let hash_loop_time = start_timer!(|| "try_and_increment::hash_loop");
        for c in 0..NUM_TRIES {
            (&mut counter[..]).write_u8(c as u8)?;
            let hash = self
                .hasher
                .hash(domain, &[&counter, extra_data, &message].concat(), num_bytes)?;
            let (possible_x, greatest) = {
                //zero out the last byte except the first bit, to get to a total of 377 bits
                let mut possible_x_0_bytes = (&hash[..hash.len()/2]).to_vec();
                let possible_x_0_bytes_len = possible_x_0_bytes.len();
                possible_x_0_bytes[possible_x_0_bytes_len - 1] &= LAST_BYTE_MASK;
                let possible_x_0 = P::Fp::read(possible_x_0_bytes.as_slice())?;
                if possible_x_0 == P::Fp::zero() {
                    continue;
                }
                let mut possible_x_1_bytes = (&hash[hash.len() / 2..]).to_vec();
                let possible_x_1_bytes_len = possible_x_1_bytes.len();
                let greatest = (possible_x_1_bytes[possible_x_1_bytes_len - 1] & GREATEST_MASK) == GREATEST_MASK;
                possible_x_1_bytes[possible_x_1_bytes_len - 1] &= LAST_BYTE_MASK;
                let possible_x_1 = P::Fp::read(possible_x_1_bytes.as_slice())?;
                if possible_x_1 == P::Fp::zero() {
                    continue;
                }
                (Fp2::<P::Fp2Params>::new(possible_x_0, possible_x_1), greatest)
            };
            match get_point_from_x::<P>(possible_x, greatest) {
                None => continue,
                Some(x) => {
                    debug!(
                        "succeeded hashing \"{}\" to G2 in {} tries",
                        hex::encode(message),
                        c
                    );
                    end_timer!(hash_loop_time);
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
        let _g: G2Projective = try_and_increment.hash::<Bls12_377Parameters>( &[], &[], &[]).unwrap();
    }
}
