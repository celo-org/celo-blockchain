pub mod try_and_increment;

use failure::Error;

use algebra::curves::models::bls12::{Bls12Parameters, G2Projective};

#[derive(Debug, Fail)]
pub enum HashToCurveError {
    #[fail(display = "cannot find point")]
    CannotFindPoint,
}

pub trait HashToG2 {
    fn hash<P: Bls12Parameters>(&self, message: &[u8]) -> Result<G2Projective<P>, Error>;
}
