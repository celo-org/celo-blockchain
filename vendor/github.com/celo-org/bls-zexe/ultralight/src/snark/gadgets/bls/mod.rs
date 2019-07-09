use algebra::{PairingEngine, ProjectiveCurve};
use snark::{ConstraintSystem, SynthesisError};
use snark_gadgets::{
    fields::FieldGadget,
    groups::GroupGadget,
    pairing::PairingGadget,
    utils::{AllocGadget, EqGadget},
};
pub struct BlsVerifyGadget<
    PairingE: PairingEngine,
    ConstraintE: PairingEngine,
    P: PairingGadget<PairingE, ConstraintE>,
> {
    pub pub_keys: Vec<P::G1Gadget>,
    pub message_hash: P::G2Gadget,
    pub signature: P::G2Gadget,
}

impl<
        PairingE: PairingEngine,
        ConstraintE: PairingEngine,
        P: PairingGadget<PairingE, ConstraintE>,
    > BlsVerifyGadget<PairingE, ConstraintE, P>
{
    pub fn alloc<CS: ConstraintSystem<ConstraintE>>(
        &self,
        mut cs: CS,
    ) -> Result<(), SynthesisError> {
        let mut aggregated_pk = P::G1Gadget::zero(cs.ns(|| "init pk"))?;
        for (i, pk) in self.pub_keys.iter().enumerate() {
            aggregated_pk = aggregated_pk.add(cs.ns(|| format!("add pk {}", i)), pk)?;
        }
        let prepared_aggregated_pk =
            P::prepare_g1(cs.ns(|| "prepared aggregaed pk"), &aggregated_pk)?;
        let prepared_message_hash =
            P::prepare_g2(cs.ns(|| "prepared message hash"), &self.message_hash)?;
        let prepared_signature = P::prepare_g2(cs.ns(|| "prepared signature"), &self.signature)?;
        let g1_neg_generator = P::G1Gadget::alloc(cs.ns(|| "G1 generator"), || {
            Ok(PairingE::G1Projective::prime_subgroup_generator())
        })?
        .negate(cs.ns(|| "negate g1 generator"))?;
        let prepared_g1_neg_generator =
            P::prepare_g1(cs.ns(|| "prepared g1 neg generator"), &g1_neg_generator)?;
        let bls_equation = P::product_of_pairings(
            cs.ns(|| "verify BLS signature"),
            &[prepared_g1_neg_generator, prepared_aggregated_pk],
            &[prepared_signature, prepared_message_hash],
        )?;
        let gt_one = &P::GTGadget::one(&mut cs.ns(|| "GT one"))?;
        bls_equation.enforce_equal(&mut cs.ns(|| "BLS equation is one"), gt_one)?;
        Ok(())
    }
}

#[cfg(test)]
mod test {
    use rand;

    use algebra::{
        curves::{
            bls12_377::{
                Bls12_377, G1Projective as Bls12_377G1Projective,
                G2Projective as Bls12_377G2Projective,
            },
            sw6::SW6,
            ProjectiveCurve,
        },
        fields::bls12_377::Fr as Bls12_377Fr,
    };
    use snark::ConstraintSystem;
    use snark_gadgets::{
        groups::bls12::bls12_377::{G1Gadget as Bls12_377G1Gadget, G2Gadget as Bls12_377G2Gadget},
        pairing::bls12_377::PairingGadget as Bls12_377PairingGadget,
        test_constraint_system::TestConstraintSystem,
        utils::AllocGadget,
    };

    use super::BlsVerifyGadget;

    #[test]
    fn test_signature() {
        let message_hash: Bls12_377G2Projective = rand::random();
        let secret_key: Bls12_377Fr = rand::random();

        let generator = Bls12_377G1Projective::prime_subgroup_generator();
        let pub_key = generator * &secret_key;
        let signature = message_hash * &secret_key;
        let fake_signature: Bls12_377G2Projective = rand::random();

        {
            let mut cs = TestConstraintSystem::<SW6>::new();
            let message_hash_var =
                Bls12_377G2Gadget::alloc(cs.ns(|| "message_hash"), || Ok(message_hash)).unwrap();
            let pub_key_var =
                Bls12_377G1Gadget::alloc(cs.ns(|| "pub_key"), || Ok(pub_key)).unwrap();
            let signature_var =
                Bls12_377G2Gadget::alloc(cs.ns(|| "signature"), || Ok(signature)).unwrap();

            let g = BlsVerifyGadget::<Bls12_377, SW6, Bls12_377PairingGadget> {
                pub_keys: [pub_key_var].to_vec(),
                message_hash: message_hash_var,
                signature: signature_var,
            };

            g.alloc(cs.ns(|| "verify sig")).unwrap();

            assert!(cs.is_satisfied());
        }
        {
            let mut cs = TestConstraintSystem::<SW6>::new();
            let message_hash_var =
                Bls12_377G2Gadget::alloc(cs.ns(|| "message_hash"), || Ok(message_hash)).unwrap();
            let pub_key_var =
                Bls12_377G1Gadget::alloc(cs.ns(|| "pub_key"), || Ok(pub_key)).unwrap();
            let signature_var =
                Bls12_377G2Gadget::alloc(cs.ns(|| "signature"), || Ok(fake_signature)).unwrap();

            let g = BlsVerifyGadget::<Bls12_377, SW6, Bls12_377PairingGadget> {
                pub_keys: [pub_key_var].to_vec(),
                message_hash: message_hash_var,
                signature: signature_var,
            };

            g.alloc(cs.ns(|| "verify sig")).unwrap();

            assert!(!cs.is_satisfied());
        }
    }
}
