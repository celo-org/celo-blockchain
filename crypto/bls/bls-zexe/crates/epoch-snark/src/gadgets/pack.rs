use algebra::{BigInteger, FpParameters, PrimeField};
use bls_gadgets::is_setup;
use r1cs_core::SynthesisError;
use r1cs_std::{fields::fp::FpGadget, prelude::*, Assignment};
use tracing::{span, trace, Level};

pub struct MultipackGadget;

impl MultipackGadget {
    pub fn pack<F: PrimeField, CS: r1cs_core::ConstraintSystem<F>>(
        mut cs: CS,
        bits: &[Boolean],
        target_capacity: usize,
        should_alloc_input: bool,
    ) -> Result<Vec<FpGadget<F>>, SynthesisError> {
        let span = span!(Level::TRACE, "multipack_gadget");
        let _enter = span.enter();
        let mut packed = vec![];
        let fp_chunks = bits.chunks(target_capacity);
        for (i, chunk) in fp_chunks.enumerate() {
            trace!(iteration = i);
            let alloc = if should_alloc_input {
                FpGadget::<F>::alloc_input
            } else {
                FpGadget::<F>::alloc
            };
            let fp = alloc(cs.ns(|| format!("chunk {}", i)), || {
                if is_setup(&chunk) {
                    return Err(SynthesisError::AssignmentMissing);
                }
                let fp_val = F::BigInt::from_bits(
                    &chunk
                        .iter()
                        .map(|x| x.get_value().get())
                        .collect::<Result<Vec<bool>, _>>()?,
                );
                Ok(F::from_repr(fp_val))
            })?;
            let fp_bits = fp.to_bits(cs.ns(|| format!("chunk bits {}", i)))?;
            let chunk_len = chunk.len();
            for j in 0..chunk_len {
                fp_bits[F::Params::MODULUS_BITS as usize - chunk_len + j]
                    .enforce_equal(cs.ns(|| format!("fp bit {} for chunk {}", j, i)), &chunk[j])?;
            }

            packed.push(fp);
        }
        Ok(packed)
    }

    pub fn unpack<F: PrimeField, CS: r1cs_core::ConstraintSystem<F>>(
        mut cs: CS,
        packed: &[FpGadget<F>],
        target_bits: usize,
        source_capacity: usize,
    ) -> Result<Vec<Boolean>, SynthesisError> {
        let bits_vecs = packed
            .iter()
            .enumerate()
            .map(|(i, x)| x.to_bits(cs.ns(|| format!("elem {} bits", i))))
            .collect::<Result<Vec<_>, _>>()?;
        let mut bits = vec![];
        let mut chunk = 0;
        let mut current_index = 0;
        while current_index < target_bits {
            let diff = if (target_bits - current_index) < source_capacity as usize {
                target_bits - current_index
            } else {
                source_capacity as usize
            };
            bits.extend_from_slice(
                &bits_vecs[chunk][<F::Params as FpParameters>::MODULUS_BITS as usize - diff..],
            );
            current_index += diff;
            chunk += 1;
        }
        Ok(bits)
    }
}
