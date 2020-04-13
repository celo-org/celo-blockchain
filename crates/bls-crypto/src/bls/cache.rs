use lru::LruCache;
use once_cell::sync::Lazy;
use std::io::Result as IoResult;

use std::{collections::HashSet, sync::Mutex};

use algebra::{bls12_377::G2Projective, Zero};

use super::PublicKey;

struct AggregateCacheState {
    keys: HashSet<PublicKey>,
    combined: G2Projective,
}

static FROM_VEC_CACHE: Lazy<Mutex<LruCache<Vec<u8>, PublicKey>>> =
    Lazy::new(|| Mutex::new(LruCache::new(128)));
static AGGREGATE_CACHE: Lazy<Mutex<AggregateCacheState>> = Lazy::new(|| {
    Mutex::new(AggregateCacheState {
        keys: HashSet::new(),
        combined: G2Projective::zero().clone(),
    })
});

pub struct PublicKeyCache;

impl PublicKeyCache {
    pub fn clear_cache() {
        FROM_VEC_CACHE.lock().unwrap().clear();
        let mut cache = AGGREGATE_CACHE.lock().unwrap();
        cache.keys = HashSet::new();
        cache.combined = G2Projective::zero().clone();
    }

    pub fn resize(cap: usize) {
        FROM_VEC_CACHE.lock().unwrap().resize(cap);
    }

    pub fn from_vec(data: &Vec<u8>) -> IoResult<PublicKey> {
        let cached_result = PublicKeyCache::from_vec_cached(data);
        if cached_result.is_none() {
            // cache miss
            let generated_result = PublicKey::from_vec(data)?;
            FROM_VEC_CACHE
                .lock()
                .unwrap()
                .put(data.to_owned(), generated_result.clone());
            Ok(generated_result)
        } else {
            // cache hit
            Ok(cached_result.unwrap())
        }
    }

    pub fn from_vec_cached(data: &Vec<u8>) -> Option<PublicKey> {
        let mut cache = FROM_VEC_CACHE.lock().unwrap();
        Some(cache.get(data)?.clone())
    }

    pub fn aggregate(public_keys: &[PublicKey]) -> PublicKey {
        // The set of validators changes slowly, so for speed we will compute the
        // difference from the last call and do an incremental update
        let mut keys: HashSet<PublicKey> = HashSet::with_capacity(public_keys.len());
        for key in public_keys {
            keys.insert(key.clone());
        }
        let mut cache = AGGREGATE_CACHE.lock().unwrap();
        let mut combined = cache.combined;

        for key in cache.keys.difference(&keys) {
            combined = combined - key.as_ref();
        }

        for key in keys.difference(&cache.keys) {
            combined = combined + key.as_ref();
        }

        cache.keys = keys;
        cache.combined = combined;
        PublicKey::from(combined)
    }
}
