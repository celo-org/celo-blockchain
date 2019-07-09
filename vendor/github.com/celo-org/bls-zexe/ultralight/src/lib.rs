#[macro_use]
extern crate log;
#[macro_use]
extern crate failure;
#[macro_use]
extern crate lazy_static;

pub mod bls;
pub mod curve;
pub mod hash;
pub mod snark;

use crate::{
    bls::keys::{PrivateKey, PublicKey, Signature},
    curve::hash::try_and_increment::TryAndIncrement,
    hash::composite::CompositeHasher,
};
use algebra::{FromBytes, ToBytes};
use rand::thread_rng;
use std::fmt::Display;
use std::os::raw::c_int;
use std::slice;

lazy_static! {
    static ref COMPOSITE_HASHER: CompositeHasher = {
        let composite_hasher = CompositeHasher::new().unwrap();
        composite_hasher
    };
    static ref HASH_TO_G2: TryAndIncrement<'static, CompositeHasher> = {
        let try_and_increment = TryAndIncrement::new(&*COMPOSITE_HASHER);
        try_and_increment
    };
}

fn convert_result_to_bool<T, E: Display, F: Fn() -> Result<T, E>>(f: F) -> bool {
    match f() {
        Err(e) => {
            error!("ultralight library error: {}", e.to_string());
            false
        }
        _ => true,
    }
}

#[no_mangle]
pub extern "C" fn init() {
    &*HASH_TO_G2;
}

#[no_mangle]
pub extern "C" fn generate_private_key(out_private_key: *mut *mut PrivateKey) -> bool {
    let mut rng = thread_rng();
    let key = PrivateKey::generate(&mut rng);
    unsafe {
        *out_private_key = Box::into_raw(Box::new(key));
    }

    return true;
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
    out_signature: *mut *mut Signature,
) -> bool {
    convert_result_to_bool::<_, failure::Error, _>(|| {
        let private_key = unsafe { &*in_private_key };
        let message = unsafe { slice::from_raw_parts(in_message, in_message_len as usize) };
        let signature = private_key.sign(&message, &*HASH_TO_G2)?;
        unsafe {
            *out_signature = Box::into_raw(Box::new(signature));
        }

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
    in_signature: *const Signature,
    out_verified: *mut bool,
) -> bool {
    convert_result_to_bool::<_, std::io::Error, _>(|| {
        let public_key = unsafe { &*in_public_key };
        let message = unsafe { slice::from_raw_parts(in_message, in_message_len as usize) };
        let signature = unsafe { &*in_signature };
        unsafe { *out_verified = public_key.verify(message, signature, &*HASH_TO_G2).is_ok() };

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
        let aggregated_public_key = PublicKey::aggregate(&public_keys[..]);
        unsafe {
            *out_public_key = Box::into_raw(Box::new(aggregated_public_key));
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
