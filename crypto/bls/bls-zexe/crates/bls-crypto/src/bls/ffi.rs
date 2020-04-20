/// FFI Utilities
///
/// Utilities for working with variable length data structures.
use crate::{PublicKey, Signature};
use std::slice;

/// A per-epoch block witness to be used with the batch sig verification
#[derive(Clone, Debug, PartialEq)]
pub struct Message<'a> {
    /// The data which was signed
    pub data: &'a [u8],
    /// Extra data which was signed alongside the `data`
    pub extra: &'a [u8],
    /// The aggregate public key of the epoch which signed the data/extra pair
    pub public_key: &'a PublicKey,
    /// The aggregate signature corresponding the aggregate public key
    pub sig: &'a Signature,
}

#[repr(C)]
#[derive(Clone, Debug, PartialEq)]
/// Pointers to the necessary data for signature verification of an epoch
pub struct MessageFFI {
    /// Pointer to the data which was signed
    pub data: Buffer,
    /// Pointer to the extra data which was signed alongside the `data`
    pub extra: Buffer,
    /// Pointer to the aggregate public key of the epoch which signed the data/extra pair
    pub public_key: *const PublicKey,
    /// Pointer to the aggregate signature corresponding the aggregate public key
    pub sig: *const Signature,
}

impl<'a> From<&'a MessageFFI> for Message<'a> {
    fn from(src: &'a MessageFFI) -> Message<'a> {
        let data = <&[u8]>::from(&src.data);
        let extra = <&[u8]>::from(&src.extra);
        Message {
            data,
            extra,
            public_key: unsafe { &*src.public_key },
            sig: unsafe { &*src.sig },
        }
    }
}

impl From<&Message<'_>> for MessageFFI {
    fn from(src: &Message) -> MessageFFI {
        MessageFFI {
            data: Buffer::from(src.data.as_ref()),
            extra: Buffer::from(src.extra.as_ref()),
            public_key: src.public_key as *const PublicKey,
            sig: src.sig as *const Signature,
        }
    }
}

/// Data structure which is used to store buffers of varying length
#[repr(C)]
#[derive(Clone, Debug, PartialEq)]
pub struct Buffer {
    /// Pointer to the message
    pub ptr: *const u8,
    /// The length of the buffer
    pub len: usize,
}

impl From<&[u8]> for Buffer {
    fn from(src: &[u8]) -> Self {
        Self {
            ptr: &src[0] as *const u8,
            len: src.len(),
        }
    }
}

impl<'a> From<&Buffer> for &'a [u8] {
    fn from(src: &Buffer) -> &'a [u8] {
        unsafe { slice::from_raw_parts(src.ptr, src.len) }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use algebra::{
        bls12_377::{G1Projective, G2Projective},
        UniformRand,
    };

    #[test]
    fn buffer_convert_ok() {
        let buf = vec![1u8, 2, 3, 4];
        let buffer = Buffer::from(buf.as_ref());
        assert_eq!(buffer.len, 4);
        let de: &[u8] = <&[u8]>::from(&buffer);
        assert_eq!(buf.as_ref() as &[u8], de);
    }

    #[test]
    fn msg_convert_ok() {
        let rng = &mut rand::thread_rng();
        let public_key = G2Projective::rand(rng);
        let public_key = PublicKey::from_pk(public_key);

        let sig = G1Projective::rand(rng);
        let sig = Signature::from_sig(sig);
        let msg = Message {
            data: &[1, 2, 3, 4],
            extra: &[5, 6, 7, 8],
            public_key: &public_key,
            sig: &sig,
        };
        let m = msg.clone();
        let msg_ffi = MessageFFI::from(&m);
        let original = Message::from(&msg_ffi);
        assert_eq!(msg, original);
    }
}
