#include <stdarg.h>
#include <stdbool.h>
#include <stdint.h>
#include <stdlib.h>

typedef struct PrivateKey PrivateKey;

typedef struct PublicKey PublicKey;

typedef struct Signature Signature;

/**
 * Data structure which is used to store buffers of varying length
 */
typedef struct {
  /**
   * Pointer to the message
   */
  const uint8_t *ptr;
  /**
   * The length of the buffer
   */
  int len;
} Buffer;

/**
 * Pointers to the necessary data for signature verification of an epoch
 */
typedef struct {
  /**
   * Pointer to the data which was signed
   */
  Buffer data;
  /**
   * Pointer to the extra data which was signed alongside the `data`
   */
  Buffer extra;
  /**
   * Pointer to the aggregate public key of the epoch which signed the data/extra pair
   */
  const PublicKey *public_key;
  /**
   * Pointer to the aggregate signature corresponding the aggregate public key
   */
  const Signature *sig;
} MessageFFI;

bool aggregate_public_keys(const PublicKey *const *in_public_keys,
                           int in_public_keys_len,
                           PublicKey **out_public_key);

bool aggregate_public_keys_subtract(const PublicKey *in_aggregated_public_key,
                                    const PublicKey *const *in_public_keys,
                                    int in_public_keys_len,
                                    PublicKey **out_public_key);

bool aggregate_signatures(const Signature *const *in_signatures,
                          int in_signatures_len,
                          Signature **out_signature);

/**
 * Receives a list of messages composed of:
 * 1. the data
 * 1. the public keys which signed on the data
 * 1. the signature produced by the public keys
 *
 * It will create the aggregate signature from all messages and execute batch
 * verification against each (data, publickey) pair. Internally calls `Signature::batch_verify`
 *
 * The verification equation can be found in pg.11 from
 * https://eprint.iacr.org/2018/483.pdf: "Batch verification"
 */
bool batch_verify_signature(const MessageFFI *messages_ptr,
                            int messages_len,
                            bool should_use_composite,
                            bool *verified);

bool compress_pubkey(const uint8_t *in_pubkey,
                     int in_pubkey_len,
                     uint8_t **out_pubkey,
                     int *out_len);

bool compress_signature(const uint8_t *in_signature,
                        int in_signature_len,
                        uint8_t **out_signature,
                        int *out_len);

bool deserialize_private_key(const uint8_t *in_private_key_bytes,
                             int in_private_key_bytes_len,
                             PrivateKey **out_private_key);

bool deserialize_public_key(const uint8_t *in_public_key_bytes,
                            int in_public_key_bytes_len,
                            PublicKey **out_public_key);

bool deserialize_signature(const uint8_t *in_signature_bytes,
                           int in_signature_bytes_len,
                           Signature **out_signature);

void destroy_private_key(PrivateKey *private_key);

void destroy_public_key(PublicKey *public_key);

void destroy_signature(Signature *signature);

bool encode_epoch_block_to_bytes(unsigned short in_epoch_index,
                                 unsigned int in_maximum_non_signers,
                                 const PublicKey *const *in_added_public_keys,
                                 int in_added_public_keys_len,
                                 bool in_should_encode_aggregated_pk,
                                 uint8_t **out_bytes,
                                 int *out_len);


void free_vec(uint8_t *bytes, int len);

bool generate_private_key(PrivateKey **out_private_key);

bool hash_composite(const uint8_t *in_message,
                    int in_message_len,
                    const uint8_t *in_extra_data,
                    int in_extra_data_len,
                    uint8_t **out_hash,
                    int *out_len);

bool hash_direct(const uint8_t *in_message,
                 int in_message_len,
                 uint8_t **out_hash,
                 int *out_len,
                 bool use_pop);

/**
 * Initializes the lazily evaluated hashers.
 */
void init(void);

bool private_key_to_public_key(const PrivateKey *in_private_key, PublicKey **out_public_key);

bool serialize_private_key(const PrivateKey *in_private_key, uint8_t **out_bytes, int *out_len);

bool serialize_public_key(const PublicKey *in_public_key, uint8_t **out_bytes, int *out_len);

bool serialize_signature(const Signature *in_signature, uint8_t **out_bytes, int *out_len);

bool sign_message(const PrivateKey *in_private_key,
                  const uint8_t *in_message,
                  int in_message_len,
                  const uint8_t *in_extra_data,
                  int in_extra_data_len,
                  bool should_use_composite,
                  Signature **out_signature);

bool sign_pop(const PrivateKey *in_private_key,
              const uint8_t *in_message,
              int in_message_len,
              Signature **out_signature);

bool verify_pop(const PublicKey *in_public_key,
                const uint8_t *in_message,
                int in_message_len,
                const Signature *in_signature,
                bool *out_verified);

bool verify_signature(const PublicKey *in_public_key,
                      const uint8_t *in_message,
                      int in_message_len,
                      const uint8_t *in_extra_data,
                      int in_extra_data_len,
                      const Signature *in_signature,
                      bool should_use_composite,
                      bool *out_verified);
