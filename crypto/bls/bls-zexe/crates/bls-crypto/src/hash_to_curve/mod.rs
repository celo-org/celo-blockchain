pub mod cofactor;
pub mod try_and_increment;

use crate::BLSError;

pub trait HashToCurve {
    type Output;

    fn hash(
        &self,
        domain: &[u8],
        message: &[u8],
        extra_data: &[u8],
    ) -> Result<Self::Output, BLSError>;
}
