use algebra::{Field, PairingEngine};
use r1cs_core::{ConstraintSynthesizer, ConstraintSystem, SynthesisError};

// circuit proving knowledge of a square root
// when generating the Setup, the element inside is None
#[derive(Clone, Debug)]
pub struct TestCircuit<E: PairingEngine>(pub Option<E::Fr>);
impl<E: PairingEngine> ConstraintSynthesizer<E::Fr> for TestCircuit<E> {
    fn generate_constraints<CS: ConstraintSystem<E::Fr>>(
        self,
        cs: &mut CS,
    ) -> std::result::Result<(), SynthesisError> {
        // allocate a private input `x`
        // this can be made public with `alloc_input`, which would then require
        // that the verifier provides it
        let x = cs
            .alloc(|| "x", || self.0.ok_or(SynthesisError::AssignmentMissing))
            .unwrap();
        // 1 input!
        let out = cs
            .alloc_input(
                || "square",
                || {
                    self.0
                        .map(|x| x.square())
                        .ok_or(SynthesisError::AssignmentMissing)
                },
            )
            .unwrap();
        // x * x = x^2
        cs.enforce(|| "x * x = x^2", |lc| lc + x, |lc| lc + x, |lc| lc + out);
        // add some dummy constraints to make the circuit a bit bigger
        // we do this so that we can write a failing test for our MPC
        // where the params are smaller than the circuit size
        // (7 in this case, since we allocated 3 constraints, plus 4 below)
        for _ in 0..4 {
            cs.alloc(
                || "dummy",
                || self.0.ok_or(SynthesisError::AssignmentMissing),
            )
            .unwrap();
        }
        Ok(())
    }
}
