pub mod composite;
pub mod direct;
use std::error::Error;

pub trait PRF {
    fn crh(&self, message: &[u8]) -> Result<Vec<u8>, Box<dyn Error>>;
    fn prf(&self, key: &[u8], domain: &[u8], hashed_message: &[u8], output_size_in_bits: usize) -> Result<Vec<u8>, Box<dyn Error>>;
    fn hash(&self, key: &[u8], domain: &[u8], message: &[u8], output_size_in_bits: usize) -> Result<Vec<u8>, Box<dyn Error>>;
}
