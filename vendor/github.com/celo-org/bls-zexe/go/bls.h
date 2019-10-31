#include <stdbool.h>
#include <stdint.h>

struct PrivateKey;
typedef struct PrivateKey PrivateKey;

struct PublicKey;
typedef struct PublicKey PublicKey;

struct Signature;
typedef struct Signature Signature;

void init();
bool generate_private_key(PrivateKey**);
bool deserialize_private_key(const unsigned char*, int32_t, PrivateKey**);
bool serialize_private_key(const PrivateKey*, unsigned char**, int32_t*);
bool private_key_to_public_key(const PrivateKey*, PublicKey**);
bool sign_message(const PrivateKey*, const unsigned char*, int32_t, const unsigned char*, int32_t, bool, Signature**);
bool sign_pop(const PrivateKey*, const unsigned char*, int32_t, Signature**);
void destroy_private_key(PrivateKey*);
void free_vec(unsigned char*, int32_t);
bool deserialize_public_key(const unsigned char*, int32_t, PublicKey**);
bool serialize_public_key(const PublicKey*, unsigned char**, int32_t*);
void destroy_public_key(PublicKey*);
bool deserialize_signature(const unsigned char*, int32_t, Signature**);
bool serialize_signature(const Signature*, unsigned char**, int32_t*);
void destroy_signature(Signature*);
bool verify_signature(const PublicKey*, const unsigned char*, int32_t, const unsigned char*, int32_t, const Signature*, bool, bool*);
bool verify_pop(const PublicKey*, const unsigned char*, int32_t, const Signature*, bool*);
bool aggregate_public_keys(const PublicKey**, int32_t, PublicKey**);
bool aggregate_public_keys_subtract(const PublicKey*, const PublicKey**, int32_t, PublicKey**);
bool aggregate_signatures(const Signature**, int32_t, Signature**);
