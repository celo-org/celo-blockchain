pub mod composite;

use failure::Error;

pub trait PRF {
    fn crh(&self, message: &[u8]) -> Result<Vec<u8>, Error>;
    fn prf(&self, hashed_message: &[u8], output_size_in_bits: usize) -> Result<Vec<u8>, Error>;
    fn hash(&self, message: &[u8], output_size_in_bits: usize) -> Result<Vec<u8>, Error>;
}
