use std::{borrow::Borrow, marker::PhantomData};

use algebra::{
    curves::{
        bls12::G1Projective, models::bls12::Bls12Parameters,
        short_weierstrass_jacobian::GroupProjective, SWModelParameters,
    },
    AffineCurve, BigInteger, BitIterator, One, PrimeField, ProjectiveCurve,
};
use crypto_primitives::prf::{
    blake2s::constraints::blake2s_gadget_with_parameters, Blake2sWithParameterBlock,
};
use r1cs_core::{ConstraintSystem, SynthesisError};
use r1cs_std::{
    alloc::AllocGadget, bits::ToBitsGadget, boolean::Boolean, groups::bls12::G1Gadget,
    groups::GroupGadget, uint8::UInt8, Assignment,
};

// Imported for the BLS12-377 API
use algebra::{
    bls12_377::{Fq as Bls12_377_Fq, Parameters as Bls12_377_Parameters},
    edwards_sw6::EdwardsProjective,
};
use bls_crypto::{
    bls::keys::SIG_DOMAIN,
    curve::hash::try_and_increment::get_point_from_x_g1,
    hash::composite::{CompositeHasher, CRH},
};
use crypto_primitives::{
    crh::bowe_hopwood::constraints::BoweHopwoodPedersenCRHGadget as BHHash, FixedLengthCRHGadget,
};
use r1cs_std::edwards_sw6::EdwardsSWGadget;

use crate::YToBitGadget;

/// Pedersen Gadget instantiated over the Edwards SW6 curve over BLS12-377 Fq (384 bits)
type BHHashSW6 = BHHash<EdwardsProjective, Bls12_377_Fq, EdwardsSWGadget>;

/// Hash to curve requires 378 bits (377 for field element, 1 for y)
///
/// Parameters for Blake2x as specified in: https://blake2.net/blake2x.pdf
/// • “Key length” is set to 0 (even if the root hash was keyed)
/// • “Fanout” is set to 0 (unlimited)
/// • “Maximal depth” is set to 0
/// • “Leaf maximal byte length” is set to 32 for BLAKE2Xs, and 64 for BLAKE2Xb
/// • “XOF digest length” is set to the length of the final output digest
/// • “Node depth” is set to 0 (leaves)
/// • “Inner hash byte length” is set to 32 for BLAKE2Xs and 64 for BLAKE2Xb
fn blake2xs_params(
    hash_length: u16,
    offset: u32,
    personalization: [u8; 8],
) -> Blake2sWithParameterBlock {
    Blake2sWithParameterBlock {
        digest_length: 32,
        key_length: 0,
        fan_out: 0,
        depth: 0,
        leaf_length: 32,
        node_offset: offset,
        xof_digest_length: hash_length / 8, // need to convert to bytes
        node_depth: 0,
        inner_length: 32,
        salt: [0; 8],
        personalization,
    }
}

/// Gadget for checking that hashes to group are computed correctly
pub struct HashToGroupGadget<P> {
    parameters_type: PhantomData<P>,
}

// If we're on Bls12-377, we can have a nice public API for the whole hash to group operation
// by taking the input, compressing it via an instantiation of Pedersen Hash with a CRH over Edwards SW6
// and then hashing it to bits and to group
impl HashToGroupGadget<Bls12_377_Parameters> {
    pub fn enforce_hash_to_group<CS: ConstraintSystem<Bls12_377_Fq>>(
        cs: &mut CS,
        counter: UInt8,
        message: &[UInt8],
    ) -> Result<G1Gadget<Bls12_377_Parameters>, SynthesisError> {
        // combine the counter with the message
        let mut input = vec![counter];
        input.extend_from_slice(message);
        // compress the input
        let crh_bits = Self::pedersen_hash(cs, &input)?;

        // Hash to bits
        let mut personalization = [0; 8];
        personalization.copy_from_slice(SIG_DOMAIN);
        // We want 378 random bits for hashing to curve, so we get 512 from the hash and will
        // discard any unneeded ones
        let xof_bits =
            Self::hash_to_bits(cs.ns(|| "hash to bits"), &crh_bits, 512, personalization)?;

        let hash = Self::hash_to_group(cs.ns(|| "hash to group"), &xof_bits)?;

        Ok(hash)
    }

    /// Compress the input by passing it through a Pedersen hash
    pub fn pedersen_hash<CS: ConstraintSystem<Bls12_377_Fq>>(
        cs: &mut CS,
        input: &[UInt8],
    ) -> Result<Vec<Boolean>, SynthesisError> {
        // We setup by getting the Parameters over the provided CRH
        let crh_params = <BHHashSW6 as FixedLengthCRHGadget<CRH, _>>::ParametersGadget::alloc(
            cs.ns(|| "pedersen parameters"),
            || CompositeHasher::setup_crh().ok().get(),
        )?;

        let pedersen_hash = <BHHashSW6 as FixedLengthCRHGadget<CRH, _>>::check_evaluation_gadget(
            &mut cs.ns(|| "pedersen evaluation"),
            &crh_params,
            &input,
        )?;

        let mut crh_bits = pedersen_hash.x.to_bits(cs.ns(|| "crh bits")).unwrap();
        // The hash must be front-padded to the nearest multiple of 8 for the LE encoding
        loop {
            if crh_bits.len() % 8 == 0 {
                break;
            }
            crh_bits.insert(0, Boolean::constant(false));
        }
        Ok(crh_bits)
    }
}

impl<P: Bls12Parameters> HashToGroupGadget<P> {
    /// Hashes the message to produce a `hash_length` hash with the provided personalization
    ///
    /// This uses Blake2s under the hood and is expensive for large messages.
    /// Consider reducing the input size by passing it through a Collision Resistant Hash function
    /// such as Pedersen.
    ///
    /// # Panics
    ///
    /// If the provided hash_length is not a multiple of 256.
    pub fn hash_to_bits<F: PrimeField, CS: ConstraintSystem<F>>(
        mut cs: CS,
        message: &[Boolean],
        hash_length: u16,
        personalization: [u8; 8],
    ) -> Result<Vec<Boolean>, SynthesisError> {
        // Blake2s outputs 256 bit hashes so the desired output hash length
        // must be a multiple of that.
        assert_eq!(hash_length % 256, 0, "invalid hash length size");
        let iterations = hash_length / 256;

        // Reverse the message to LE
        let mut message = message.to_vec();
        message.reverse();

        let mut xof_bits = Vec::new();
        // Run Blake on the message N times, each time offset by `i`
        // to get a `hash_length` hash. The hash is in LE.
        for i in 0..iterations {
            // calculate the hash
            let blake2s_parameters = blake2xs_params(hash_length, i.into(), personalization);
            let xof_result = blake2s_gadget_with_parameters(
                cs.ns(|| format!("xof result {}", i)),
                &message,
                &blake2s_parameters.parameters(),
            )?;
            // convert hash result to LE bits
            let xof_bits_i = xof_result
                .into_iter()
                .map(|n| n.to_bits_le())
                .flatten()
                .collect::<Vec<Boolean>>();
            xof_bits.extend_from_slice(&xof_bits_i);
        }

        Ok(xof_bits)
    }

    // Receives the output of `HashToBitsGadget::hash_to_bits` in Little Endian
    // decodes the G1 point and then multiplies it by the curve's cofactor to
    // get the hash
    pub fn hash_to_group<CS: ConstraintSystem<P::Fp>>(
        mut cs: CS,
        xof_bits: &[Boolean],
    ) -> Result<G1Gadget<P>, SynthesisError> {
        // if we're in setup mode, just return an error
        if xof_bits.iter().any(|x| x.get_value().is_none()) {
            return Err(SynthesisError::AssignmentMissing);
        }

        let x_bits = &xof_bits[..377];
        let greatest = xof_bits[377];

        let expected_point_before_cofactor =
            G1Gadget::<P>::alloc(cs.ns(|| "expected point before cofactor"), || {
                // get the bits from the Boolean constraints
                // we assume that these are already encoded as LE
                let mut bits = x_bits
                    .iter()
                    .map(|x| x.get_value().get())
                    .collect::<Result<Vec<bool>, _>>()?;

                // `BigInt::from_bits` takes BigEndian representations so we need to
                // reverse them since they are read in LE
                bits.reverse();
                let big = <P::Fp as PrimeField>::BigInt::from_bits(&bits);
                let x = P::Fp::from_repr(big);
                let greatest = greatest.get_value().get()?;

                // Converts the point read from the xof bits to a G1 element
                // with point decompression
                let p = get_point_from_x_g1::<P>(x, greatest)
                    .ok_or(SynthesisError::AssignmentMissing)?;

                Ok(p.into_projective())
            })?;

        // Point compression on the G1 Gadget
        let compressed_point: Vec<Boolean> = {
            // Convert x to LE
            let mut bits: Vec<Boolean> =
                expected_point_before_cofactor.x.to_bits(cs.ns(|| "bits"))?;
            bits.reverse();

            // Get a constraint about the y point's sign
            let greatest_bit = YToBitGadget::<P>::y_to_bit_g1(
                cs.ns(|| "y to bit"),
                &expected_point_before_cofactor,
            )?;

            // return the x point plus the greatest bit constraint
            bits.push(greatest_bit);

            bits
        };

        compressed_point
            .iter()
            .zip(xof_bits.iter())
            .enumerate()
            .for_each(|(i, (a, b))| {
                cs.enforce(
                    || format!("enforce bit {}", i),
                    |lc| lc + (P::Fp::one(), CS::one()),
                    |_| a.lc(CS::one(), P::Fp::one()),
                    |_| b.lc(CS::one(), P::Fp::one()),
                );
            });

        let scaled_point = Self::scale_by_cofactor_g1(
            cs.ns(|| "scale by cofactor"),
            &expected_point_before_cofactor,
        )?;

        Ok(scaled_point)
    }

    fn scale_by_cofactor_g1<CS: r1cs_core::ConstraintSystem<P::Fp>>(
        mut cs: CS,
        p: &G1Gadget<P>,
    ) -> Result<G1Gadget<P>, SynthesisError>
    where
        G1Projective<P>: Borrow<GroupProjective<P::G1Parameters>>,
    {
        // get the cofactor's bits
        let mut x_bits = BitIterator::new(P::G1Parameters::COFACTOR)
            .map(Boolean::constant)
            .collect::<Vec<Boolean>>();

        // Zexe's mul_bits requires that inputs _MUST_ be in LE form, so we have to reverse
        x_bits.reverse();

        // return p * cofactor - [g]_1
        let generator = G1Gadget::<P>::alloc(cs.ns(|| "generator"), || {
            Ok(G1Projective::<P>::prime_subgroup_generator())
        })?;
        let scaled = p
            .mul_bits(cs.ns(|| "scaled"), &generator, x_bits.iter())?
            .sub(cs.ns(|| "scaled finalize"), &generator)?;
        Ok(scaled)
    }
}

#[cfg(test)]
mod test {
    use super::*;

    use algebra::bls12_377;
    use r1cs_std::{groups::GroupGadget, test_constraint_system::TestConstraintSystem};

    use bls_crypto::{bls::keys::SIG_DOMAIN, curve::hash::try_and_increment::TryAndIncrement};
    use r1cs_std::bits::uint8::UInt8;
    use rand::{thread_rng, RngCore};

    #[test]
    fn test_hash_to_group() {
        let mut rng = thread_rng();
        // test for various input sizes
        for length in &[10, 25, 50, 100, 200, 300] {
            // fill a buffer with random elements
            let mut input = vec![0; *length];
            rng.fill_bytes(&mut input);
            // check that they get hashed properly
            hash_to_group(&input);
        }
    }

    fn hash_to_group(input: &[u8]) {
        let composite_hasher = CompositeHasher::new().unwrap();
        let try_and_increment = TryAndIncrement::new(&composite_hasher);
        let (expected_hash, attempt) = try_and_increment
            .hash_with_attempt::<bls12_377::Parameters>(SIG_DOMAIN, input, &[])
            .unwrap();

        let mut cs = TestConstraintSystem::<bls12_377::Fq>::new();

        let counter = UInt8::alloc(&mut cs.ns(|| "alloc counter"), || Ok(attempt as u8)).unwrap();
        let input = input
            .iter()
            .enumerate()
            .map(|(i, num)| {
                UInt8::alloc(&mut cs.ns(|| format!("input {}", i)), || Ok(num)).unwrap()
            })
            .collect::<Vec<_>>();

        let hash = HashToGroupGadget::<bls12_377::Parameters>::enforce_hash_to_group(
            &mut cs.ns(|| "hash to group"),
            counter,
            &input,
        )
        .unwrap();

        assert!(cs.is_satisfied());
        assert_eq!(expected_hash, hash.get_value().unwrap());
    }
}
