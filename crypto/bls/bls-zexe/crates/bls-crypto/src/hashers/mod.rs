pub mod composite;
pub use composite::COMPOSITE_HASHER;

mod direct;
pub use direct::DirectHasher;

pub trait XOF {
    /// The returned error type from each hashing call
    type Error;

    /// Runs a collision resistant function over the input with the specified domain
    fn crh(
        &self,
        domain: &[u8],
        message: &[u8],
        xof_digest_length: usize,
    ) -> Result<Vec<u8>, Self::Error>;

    fn xof(
        &self,
        domain: &[u8],
        hashed_message: &[u8],
        output_size_in_bytes: usize,
    ) -> Result<Vec<u8>, Self::Error>;

    /// Runs the CRH over the domain on the input, and then runs it again over the XOF
    fn hash(
        &self,
        domain: &[u8],
        message: &[u8],
        output_size_in_bytes: usize,
    ) -> Result<Vec<u8>, Self::Error> {
        let prepared_message = self.crh(domain, message, output_size_in_bytes)?;
        self.xof(domain, &prepared_message, output_size_in_bytes)
    }
}
