# BLS-ZEXE

Implements SNARK-friendly BLS signatures over BLS12-377 and SW6.

## Using the code

### Rust Crates

All Rust crates live under the `crates/` directory. You can import them in your code via git paths, until they get published on `crates.io`.

### Go and FFI

A Go package consuming the library exists in the `go` directory, using `cgo`.

## Quick start

The following commands assume your current directory is the root of this repository.

The `simple_signature` program shows how to generate keys, sign and aggregate signatures.

To run it with debug logging enabled, execute:

`RUST_LOG=debug cargo run --example simple_signature -- -m hello`

### Building

To build the project, you should use a recent stable Rust version. We test with 1.36.

```bash
# Build
cargo build (--release)
# Test. 
# Consider running tests in release mode, as some of 
# the cryptographic operations are slow in debug mode.
cargo test (--release)
```

## Construction

We work over the BLS12-377 curve from [BCGMMW18].

Secret keys are elements of the scalar field *Fr*.

We would like to minimize the computation required for signing, since we would also like to achieve hardware wallet compatibility. Therefore, public keys are in *G2* and signatures are in *G1*.

For most signatures - to hash a message to *G1*, we use the try-and-increment method coupled with Blake2Xs. 

For signatures that we would like to verify in SNARKs - to hash a message to *G1*, we use the try-and-increment method coupled with a composite hash. The composite hash is composed of a Bowe-Hopwood hash over $E_{Ed/CP}$ from [BCGMMW18] and Blake2s.

We perform cofactor muliplication in *G1* directly.

## License

BLS-ZEXE is licensed under either of the following licenses, at your discretion.

Apache License Version 2.0 (LICENSE-APACHE or http://www.apache.org/licenses/LICENSE-2.0)
MIT license (LICENSE-MIT or http://opensource.org/licenses/MIT)
Unless you explicitly state otherwise, any contribution submitted for inclusion in BLS-ZEXE by you shall be dual licensed as above (as defined in the Apache v2 License), without any additional terms or conditions.

## References

[BDN18] Boneh, D., Drijvers, M., & Neven, G. (2018, December). [Compact multi-signatures for smaller blockchains](https://eprint.iacr.org/2018/483.pdf). In International Conference on the Theory and Application of Cryptology and Information Security (pp. 435-464). Springer, Cham.

[BLS01] Boneh, D., Lynn, B., & Shacham, H. (2001, December). [Short signatures from the Weil pairing](https://link.springer.com/content/pdf/10.1007/3-540-45682-1_30.pdf). In International Conference on the Theory and Application of Cryptology and Information Security (pp. 514-532). Springer, Berlin, Heidelberg.

[BCGMMW18] Bowe, S., Chiesa, A., Green, M., Miers, I., Mishra, P., & Wu, H. (2018). [Zexe: Enabling decentralized private computation](https://eprint.iacr.org/2018/962.pdf). IACR ePrint, 962.

[pairings] Costello, C. . [Pairings for beginners](http://www.craigcostello.com.au/pairings/PairingsForBeginners.pdf).

[BP17] Budroni, A., & Pintore, F. (2017). [Efficient hash maps to G2 on BLS curves](https://eprint.iacr.org/2017/419.pdf). Cryptology ePrint Archive, Report 2017/419.

[RY07] Ristenpart, T., & Yilek, S. (2007, May). The power of proofs-of-possession: Securing multiparty signatures against rogue-key attacks. In Annual International Conference on the Theory and Applications of Cryptographic Techniques (pp. 228-245). Springer, Berlin, Heidelberg.