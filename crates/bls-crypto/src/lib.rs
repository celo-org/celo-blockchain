#![allow(clippy::all)]
pub mod bls;
pub mod curve;
pub mod hash;

// Clean public API
pub use bls::keys::{PrivateKey, PublicKey, Signature};
pub use curve::hash::try_and_increment::TryAndIncrement;
pub use hash::{composite::CompositeHasher, direct::DirectHasher};

use lazy_static::lazy_static;
use log::error;

use crate::{
    bls::keys::{PublicKeyCache, POP_DOMAIN, SIG_DOMAIN},
    curve::hash::HashToG1,
};
use algebra::{
    bls12_377::{Fq, Fq2, G1Affine, G2Affine, Parameters as Bls12_377Parameters},
    AffineCurve, FromBytes, ProjectiveCurve, ToBytes,
};
use rand::thread_rng;
use std::os::raw::c_int;
use std::slice;
use std::{error::Error, fmt::Display};

lazy_static! {
    static ref COMPOSITE_HASHER: CompositeHasher = { CompositeHasher::new().unwrap() };
    static ref DIRECT_HASHER: DirectHasher = { DirectHasher::new().unwrap() };
    static ref COMPOSITE_HASH_TO_G1: TryAndIncrement<'static, CompositeHasher> =
        { TryAndIncrement::new(&*COMPOSITE_HASHER) };
    static ref DIRECT_HASH_TO_G1: TryAndIncrement<'static, DirectHasher> =
        { TryAndIncrement::new(&*DIRECT_HASHER) };
}

fn convert_result_to_bool<T, E: Display, F: Fn() -> Result<T, E>>(f: F) -> bool {
    match f() {
        Err(e) => {
            error!("BLS library error: {}", e.to_string());
            false
        }
        _ => true,
    }
}

#[no_mangle]
/// Initializes the lazily evaluated hashers. Should
pub extern "C" fn init() {
    &*COMPOSITE_HASH_TO_G1;
    &*DIRECT_HASH_TO_G1;
}

#[no_mangle]
pub extern "C" fn generate_private_key(out_private_key: *mut *mut PrivateKey) -> bool {
    let mut rng = thread_rng();
    let key = PrivateKey::generate(&mut rng);
    unsafe {
        *out_private_key = Box::into_raw(Box::new(key));
    }

    true
}

fn deserialize<T: FromBytes>(in_bytes: *const u8, in_bytes_len: c_int, out: *mut *mut T) -> bool {
    convert_result_to_bool::<_, std::io::Error, _>(|| {
        let bytes = unsafe { slice::from_raw_parts(in_bytes, in_bytes_len as usize) };
        let key = T::read(bytes)?;
        unsafe {
            *out = Box::into_raw(Box::new(key));
        }

        Ok(())
    })
}

fn serialize<T: ToBytes>(in_obj: *const T, out_bytes: *mut *mut u8, out_len: *mut c_int) -> bool {
    convert_result_to_bool::<_, std::io::Error, _>(|| {
        let obj = unsafe { &*in_obj };
        let mut obj_bytes = vec![];
        obj.write(&mut obj_bytes)?;
        obj_bytes.shrink_to_fit();
        unsafe {
            *out_bytes = obj_bytes.as_mut_ptr();
            *out_len = obj_bytes.len() as c_int;
        }
        std::mem::forget(obj_bytes);

        Ok(())
    })
}

#[no_mangle]
pub extern "C" fn deserialize_private_key(
    in_private_key_bytes: *const u8,
    in_private_key_bytes_len: c_int,
    out_private_key: *mut *mut PrivateKey,
) -> bool {
    deserialize(
        in_private_key_bytes,
        in_private_key_bytes_len,
        out_private_key,
    )
}

#[no_mangle]
pub extern "C" fn serialize_private_key(
    in_private_key: *const PrivateKey,
    out_bytes: *mut *mut u8,
    out_len: *mut c_int,
) -> bool {
    serialize(in_private_key, out_bytes, out_len)
}

#[no_mangle]
pub extern "C" fn private_key_to_public_key(
    in_private_key: *const PrivateKey,
    out_public_key: *mut *mut PublicKey,
) -> bool {
    convert_result_to_bool::<_, std::io::Error, _>(|| {
        let private_key = unsafe { &*in_private_key };
        let public_key = private_key.to_public();
        unsafe {
            *out_public_key = Box::into_raw(Box::new(public_key));
        }

        Ok(())
    })
}

#[no_mangle]
pub extern "C" fn sign_message(
    in_private_key: *const PrivateKey,
    in_message: *const u8,
    in_message_len: c_int,
    in_extra_data: *const u8,
    in_extra_data_len: c_int,
    should_use_composite: bool,
    out_signature: *mut *mut Signature,
) -> bool {
    convert_result_to_bool::<_, Box<dyn Error>, _>(|| {
        let private_key = unsafe { &*in_private_key };
        let message = unsafe { slice::from_raw_parts(in_message, in_message_len as usize) };
        let extra_data =
            unsafe { slice::from_raw_parts(in_extra_data, in_extra_data_len as usize) };
        let signature = if should_use_composite {
            private_key.sign(message, extra_data, &*COMPOSITE_HASH_TO_G1)?
        } else {
            private_key.sign(message, extra_data, &*DIRECT_HASH_TO_G1)?
        };
        unsafe {
            *out_signature = Box::into_raw(Box::new(signature));
        }

        Ok(())
    })
}

#[no_mangle]
pub extern "C" fn sign_pop(
    in_private_key: *const PrivateKey,
    in_message: *const u8,
    in_message_len: c_int,
    out_signature: *mut *mut Signature,
) -> bool {
    convert_result_to_bool::<_, Box<dyn Error>, _>(|| {
        let private_key = unsafe { &*in_private_key };
        let message = unsafe { slice::from_raw_parts(in_message, in_message_len as usize) };
        let signature = private_key.sign_pop(message, &*DIRECT_HASH_TO_G1)?;
        unsafe {
            *out_signature = Box::into_raw(Box::new(signature));
        }

        Ok(())
    })
}

#[no_mangle]
pub extern "C" fn hash_direct(
    in_message: *const u8,
    in_message_len: c_int,
    out_hash: *mut *mut u8,
    out_len: *mut c_int,
    use_pop: bool,
) -> bool {
    convert_result_to_bool::<_, Box<dyn Error>, _>(|| {
        let message = unsafe { slice::from_raw_parts(in_message, in_message_len as usize) };
        let hash = if use_pop {
            DIRECT_HASH_TO_G1.hash::<Bls12_377Parameters>(POP_DOMAIN, message, &[])?
        } else {
            DIRECT_HASH_TO_G1.hash::<Bls12_377Parameters>(SIG_DOMAIN, message, &[])?
        };
        let mut obj_bytes = vec![];
        hash.into_affine().write(&mut obj_bytes)?;
        obj_bytes.shrink_to_fit();
        unsafe {
            *out_hash = obj_bytes.as_mut_ptr();
            *out_len = obj_bytes.len() as c_int;
        }
        std::mem::forget(obj_bytes);
        Ok(())
    })
}

#[no_mangle]
pub extern "C" fn hash_composite(
    in_message: *const u8,
    in_message_len: c_int,
    in_extra_data: *const u8,
    in_extra_data_len: c_int,
    out_hash: *mut *mut u8,
    out_len: *mut c_int,
) -> bool {
    convert_result_to_bool::<_, Box<dyn Error>, _>(|| {
        let message = unsafe { slice::from_raw_parts(in_message, in_message_len as usize) };
        let extra_data =
            unsafe { slice::from_raw_parts(in_extra_data, in_extra_data_len as usize) };
        let hash =
            COMPOSITE_HASH_TO_G1.hash::<Bls12_377Parameters>(SIG_DOMAIN, message, extra_data)?;
        let mut obj_bytes = vec![];
        hash.write(&mut obj_bytes)?;
        obj_bytes.shrink_to_fit();
        unsafe {
            *out_hash = obj_bytes.as_mut_ptr();
            *out_len = obj_bytes.len() as c_int;
        }
        std::mem::forget(obj_bytes);
        Ok(())
    })
}

#[no_mangle]
pub extern "C" fn compress_signature(
    in_signature: *const u8,
    in_signature_len: c_int,
    out_signature: *mut *mut u8,
    out_len: *mut c_int,
) -> bool {
    convert_result_to_bool::<_, Box<dyn Error>, _>(|| {
        let signature = unsafe { slice::from_raw_parts(in_signature, in_signature_len as usize) };
        let x = Fq::read(&signature[0..48]).unwrap();
        let y = Fq::read(&signature[48..96]).unwrap();
        let affine = G1Affine::new(x, y, false);
        let sig = Signature::from_sig(&affine.into_projective());
        let mut obj_bytes = vec![];
        sig.write(&mut obj_bytes)?;
        obj_bytes.shrink_to_fit();
        unsafe {
            *out_signature = obj_bytes.as_mut_ptr();
            *out_len = obj_bytes.len() as c_int;
        }
        std::mem::forget(obj_bytes);
        Ok(())
    })
}

#[no_mangle]
pub extern "C" fn compress_pubkey(
    in_pubkey: *const u8,
    in_pubkey_len: c_int,
    out_pubkey: *mut *mut u8,
    out_len: *mut c_int,
) -> bool {
    convert_result_to_bool::<_, Box<dyn Error>, _>(|| {
        let pubkey = unsafe { slice::from_raw_parts(in_pubkey, in_pubkey_len as usize) };
        let x = Fq2::read(&pubkey[0..96]).unwrap();
        let y = Fq2::read(&pubkey[96..192]).unwrap();
        let affine = G2Affine::new(x, y, false);
        let pk = PublicKey::from_pk(&affine.into_projective());
        let mut obj_bytes = vec![];
        pk.write(&mut obj_bytes)?;
        obj_bytes.shrink_to_fit();
        unsafe {
            *out_pubkey = obj_bytes.as_mut_ptr();
            *out_len = obj_bytes.len() as c_int;
        }
        std::mem::forget(obj_bytes);
        Ok(())
    })
}

#[no_mangle]
pub extern "C" fn destroy_private_key(private_key: *mut PrivateKey) {
    drop(unsafe {
        Box::from_raw(private_key);
    })
}

#[no_mangle]
pub extern "C" fn free_vec(bytes: *mut u8, len: c_int) {
    drop(unsafe { Vec::from_raw_parts(bytes, len as usize, len as usize) });
}

#[no_mangle]
pub extern "C" fn destroy_public_key(public_key: *mut PublicKey) {
    drop(unsafe {
        Box::from_raw(public_key);
    })
}

#[no_mangle]
pub extern "C" fn destroy_signature(signature: *mut Signature) {
    drop(unsafe {
        Box::from_raw(signature);
    })
}

#[no_mangle]
pub extern "C" fn deserialize_public_key(
    in_public_key_bytes: *const u8,
    in_public_key_bytes_len: c_int,
    out_public_key: *mut *mut PublicKey,
) -> bool {
    deserialize(in_public_key_bytes, in_public_key_bytes_len, out_public_key)
}

#[no_mangle]
pub extern "C" fn serialize_public_key(
    in_public_key: *const PublicKey,
    out_bytes: *mut *mut u8,
    out_len: *mut c_int,
) -> bool {
    serialize(in_public_key, out_bytes, out_len)
}

#[no_mangle]
pub extern "C" fn deserialize_signature(
    in_signature_bytes: *const u8,
    in_signature_bytes_len: c_int,
    out_signature: *mut *mut Signature,
) -> bool {
    deserialize(in_signature_bytes, in_signature_bytes_len, out_signature)
}

#[no_mangle]
pub extern "C" fn serialize_signature(
    in_signature: *const Signature,
    out_bytes: *mut *mut u8,
    out_len: *mut c_int,
) -> bool {
    serialize(in_signature, out_bytes, out_len)
}

#[no_mangle]
pub extern "C" fn verify_signature(
    in_public_key: *const PublicKey,
    in_message: *const u8,
    in_message_len: c_int,
    in_extra_data: *const u8,
    in_extra_data_len: c_int,
    in_signature: *const Signature,
    should_use_composite: bool,
    out_verified: *mut bool,
) -> bool {
    convert_result_to_bool::<_, std::io::Error, _>(|| {
        let public_key = unsafe { &*in_public_key };
        let message = unsafe { slice::from_raw_parts(in_message, in_message_len as usize) };
        let extra_data =
            unsafe { slice::from_raw_parts(in_extra_data, in_extra_data_len as usize) };
        let signature = unsafe { &*in_signature };
        let verified = if should_use_composite {
            public_key
                .verify(message, extra_data, signature, &*COMPOSITE_HASH_TO_G1)
                .is_ok()
        } else {
            public_key
                .verify(message, extra_data, signature, &*DIRECT_HASH_TO_G1)
                .is_ok()
        };
        unsafe { *out_verified = verified };

        Ok(())
    })
}

#[no_mangle]
pub extern "C" fn verify_pop(
    in_public_key: *const PublicKey,
    in_message: *const u8,
    in_message_len: c_int,
    in_signature: *const Signature,
    out_verified: *mut bool,
) -> bool {
    convert_result_to_bool::<_, std::io::Error, _>(|| {
        let public_key = unsafe { &*in_public_key };
        let message = unsafe { slice::from_raw_parts(in_message, in_message_len as usize) };
        let signature = unsafe { &*in_signature };
        let verified = public_key
            .verify_pop(message, signature, &*DIRECT_HASH_TO_G1)
            .is_ok();
        unsafe { *out_verified = verified };

        Ok(())
    })
}

#[no_mangle]
pub extern "C" fn aggregate_public_keys(
    in_public_keys: *const *const PublicKey,
    in_public_keys_len: c_int,
    out_public_key: *mut *mut PublicKey,
) -> bool {
    convert_result_to_bool::<_, std::io::Error, _>(|| {
        let public_keys_ptrs =
            unsafe { slice::from_raw_parts(in_public_keys, in_public_keys_len as usize) };
        let public_keys = public_keys_ptrs
            .to_vec()
            .into_iter()
            .map(|pk| unsafe { &*pk })
            .collect::<Vec<&PublicKey>>();
        let aggregated_public_key = PublicKeyCache::aggregate(&public_keys[..]);
        unsafe {
            *out_public_key = Box::into_raw(Box::new(aggregated_public_key));
        }

        Ok(())
    })
}

#[no_mangle]
pub extern "C" fn aggregate_public_keys_subtract(
    in_aggregated_public_key: *const PublicKey,
    in_public_keys: *const *const PublicKey,
    in_public_keys_len: c_int,
    out_public_key: *mut *mut PublicKey,
) -> bool {
    convert_result_to_bool::<_, std::io::Error, _>(|| {
        let aggregated_public_key = unsafe { &*in_aggregated_public_key };
        let public_keys_ptrs =
            unsafe { slice::from_raw_parts(in_public_keys, in_public_keys_len as usize) };
        let public_keys = public_keys_ptrs
            .to_vec()
            .into_iter()
            .map(|pk| unsafe { &*pk })
            .collect::<Vec<&PublicKey>>();
        let aggregated_public_key_to_subtract = PublicKeyCache::aggregate(&public_keys[..]);
        let prepared_aggregated_public_key = PublicKey::from_pk(
            &(aggregated_public_key.get_pk() - aggregated_public_key_to_subtract.get_pk()),
        );
        unsafe {
            *out_public_key = Box::into_raw(Box::new(prepared_aggregated_public_key));
        }

        Ok(())
    })
}

#[no_mangle]
pub extern "C" fn aggregate_signatures(
    in_signatures: *const *const Signature,
    in_signatures_len: c_int,
    out_signature: *mut *mut Signature,
) -> bool {
    convert_result_to_bool::<_, std::io::Error, _>(|| {
        let signatures_ptrs =
            unsafe { slice::from_raw_parts(in_signatures, in_signatures_len as usize) };
        let signatures = signatures_ptrs
            .to_vec()
            .into_iter()
            .map(|sig| unsafe { &*sig })
            .collect::<Vec<&Signature>>();
        let aggregated_signature = Signature::aggregate(&signatures[..]);
        unsafe {
            *out_signature = Box::into_raw(Box::new(aggregated_signature));
        }

        Ok(())
    })
}
