pub mod composite;
pub mod direct;
use std::error::Error;

pub trait XOF {
    fn crh(&self, domain: &[u8], message: &[u8], xof_digest_length: usize) -> Result<Vec<u8>, Box<dyn Error>>;
    fn xof(&self, domain: &[u8], hashed_message: &[u8], output_size_in_bytes: usize) -> Result<Vec<u8>, Box<dyn Error>>;
    fn hash(&self, domain: &[u8], message: &[u8], output_size_in_bytes: usize) -> Result<Vec<u8>, Box<dyn Error>>;
}
