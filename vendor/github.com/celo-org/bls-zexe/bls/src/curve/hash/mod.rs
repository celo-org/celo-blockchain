pub mod try_and_increment;

use algebra::curves::models::bls12::{Bls12Parameters, G2Projective};
use std::{
    fmt::{self, Display},
    error::Error,
};

#[derive(Debug)]
pub enum HashToCurveError {
    CannotFindPoint,
}

impl Display for HashToCurveError {
    fn fmt(&self, f: &mut fmt::Formatter) -> fmt::Result {
        write!(f, "cannot find point")
    }
}

impl Error for HashToCurveError {
    fn source(&self) -> Option<&(dyn Error + 'static)> {
        None
    }
}

pub trait HashToG2 {
    fn hash<P: Bls12Parameters>(&self, key: &[u8], domain: &[u8], message: &[u8], extra_data: &[u8]) -> Result<G2Projective<P>, Box<dyn Error>>;
}
