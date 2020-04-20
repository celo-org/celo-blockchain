//! # Gadgets

mod bls;
pub use bls::BlsVerifyGadget;

mod bitmap;
pub use bitmap::enforce_maximum_occurrences_in_bitmap;

mod y_to_bit;
pub use y_to_bit::YToBitGadget;

mod hash_to_group;
pub use hash_to_group::{hash_to_bits, HashToGroupGadget};

mod smaller_than;
pub use smaller_than::SmallerThanGadget;

use algebra::Field;
use r1cs_core::{ConstraintSystem, SynthesisError};
use r1cs_std::{alloc::AllocGadget, boolean::Boolean};

/// Helper which _must_ be used before trying to the inner values
/// of a boolean vector
pub fn is_setup(message: &[Boolean]) -> bool {
    message.iter().any(|m| m.get_value().is_none())
}

pub fn constrain_bool<F: Field, CS: ConstraintSystem<F>>(
    cs: &mut CS,
    input: &[bool],
) -> Result<Vec<Boolean>, SynthesisError> {
    input
        .iter()
        .enumerate()
        .map(|(j, b)| Boolean::alloc(cs.ns(|| format!("{}", j)), || Ok(b)))
        .collect::<Result<Vec<_>, _>>()
}

pub fn bits_to_bytes(bits: &[bool]) -> Vec<u8> {
    let mut bytes = vec![];
    let reversed_bits = {
        let mut tmp = bits.to_owned();
        tmp.reverse();
        tmp
    };
    for chunk in reversed_bits.chunks(8) {
        let mut byte = 0;
        let mut twoi: u64 = 1;
        for c in chunk {
            byte += (twoi * (*c as u64)) as u8;
            twoi *= 2;
        }
        bytes.push(byte);
    }

    bytes
}

/// If bytes is a little endian representation of a number, this would return the bits of the
/// number in descending order
pub fn bytes_to_bits(bytes: &[u8], bits_to_take: usize) -> Vec<bool> {
    let mut bits = vec![];
    for b in bytes {
        let mut byte = *b;
        for _ in 0..8 {
            bits.push((byte & 1) == 1);
            byte >>= 1;
        }
    }

    bits.into_iter()
        .take(bits_to_take)
        .collect::<Vec<bool>>()
        .into_iter()
        .rev()
        .collect()
}

#[cfg(any(test, feature = "test-helpers"))]
pub mod test_helpers {
    use algebra::{Field, Group};
    use r1cs_core::ConstraintSystem;
    use r1cs_std::groups::GroupGadget;

    /// Allocates an array of group elements to a group gadget
    pub fn alloc_vec<F: Field, G: Group, GG: GroupGadget<G, F>, CS: ConstraintSystem<F>>(
        cs: &mut CS,
        elements: &[G],
    ) -> Vec<GG> {
        elements
            .iter()
            .enumerate()
            .map(|(i, element)| GG::alloc(&mut cs.ns(|| format!("{}", i)), || Ok(element)).unwrap())
            .collect::<Vec<_>>()
    }
}
