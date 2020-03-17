pub mod try_and_increment;

use algebra::curves::models::bls12::{Bls12Parameters, G1Projective, G2Projective};
use std::{
    error::Error,
    fmt::{self, Display},
};

#[derive(Debug)]
pub enum HashToCurveError {
    CannotFindPoint,
    SmallOrderPoint,
}

impl Display for HashToCurveError {
    fn fmt(&self, f: &mut fmt::Formatter) -> fmt::Result {
        match self {
            HashToCurveError::CannotFindPoint => write!(f, "cannot find point"),
            HashToCurveError::SmallOrderPoint => write!(f, "got small order point"),
        }
    }
}

impl Error for HashToCurveError {
    fn source(&self) -> Option<&(dyn Error + 'static)> {
        None
    }
}

pub trait HashToG2 {
    fn hash<P: Bls12Parameters>(
        &self,
        domain: &[u8],
        message: &[u8],
        extra_data: &[u8],
    ) -> Result<G2Projective<P>, Box<dyn Error>>;
}

pub trait HashToG1 {
    fn hash<P: Bls12Parameters>(
        &self,
        domain: &[u8],
        message: &[u8],
        extra_data: &[u8],
    ) -> Result<G1Projective<P>, Box<dyn Error>>;
}
