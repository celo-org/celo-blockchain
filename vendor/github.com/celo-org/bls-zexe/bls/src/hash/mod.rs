pub mod composite;
pub mod direct;

use failure::Error;

pub trait PRF {
    fn crh(&self, message: &[u8]) -> Result<Vec<u8>, Error>;
    fn prf(&self, domain: &[u8], hashed_message: &[u8], output_size_in_bits: usize) -> Result<Vec<u8>, Error>;
    fn hash(&self, domain: &[u8], message: &[u8], output_size_in_bits: usize) -> Result<Vec<u8>, Error>;
}
