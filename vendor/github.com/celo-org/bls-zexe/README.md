# BLS-ZEXE

Implements BLS signatures as described in [BDN18].

## Using the code

The BLS library code resides in the `bls` directory. The following commands assumed this is your current directory.

A Go package consuming the library exists in the `go` directory.

### Quick start

The `simple_signature` program shows how to generate keys, sign and aggregate signatures.

To run it with debug logging enabled, execute:

`RUST_LOG=debug cargo run --example simple_signature -- -m hello`

### Building

To build the project, you should use a recent stable Rust version. We test with 1.37.

`cargo build`

or

`cargo build --release`

### Running tests

Most of the modules have tests.

 You should run tests in release mode, as some of the cryptographic operations are slow in debug mode.

`cargo test`

## Construction

We work over the $E_{CP}$ curve from [BCGMMW18].

Secret keys are elements of the scalar field *Fr*.

We would like to minimize the public key size, since we expect many of them to be communicated. Therefore, public keys are in *G1* and signatures are in *G2*.

To hash a message to *G2*, we currently use the try-and-increment method coupled with a composite hash. The composite hash is composed of a Pedersen hash over $E_{Ed/CP}$ from [BCGMMW18] and Blake2s. First, the Pedersen hash is applied to the message, and then the try-and-increment methods attempts incrementing counters over the hashed message using Blake2s.

We implement fast cofactor multiplication, as the *G2* cofactor is large.

## License

BLS-ZEXE is licensed under either of the following licenses, at your discretion.

Apache License Version 2.0 (LICENSE-APACHE or http://www.apache.org/licenses/LICENSE-2.0)
MIT license (LICENSE-MIT or http://opensource.org/licenses/MIT)
Unless you explicitly state otherwise, any contribution submitted for inclusion in BLS-ZEXE by you shall be dual licensed as above (as defined in the Apache v2 License), without any additional terms or conditions.

## Third-party Libraries

BLS-ZEXE distributes the Zexe source code under [zexe](zexe). Zexe's authors are listed in its [AUTHORS](zexe/AUTHORS) file.

## Third-Party Software Licenses

[ZEXE](https://github.com/scipr-lab/zexe) is licensed under either MIT or Apache License Version 2.0. The notices are the same as [LICENSE-MIT](LICENSE-MIT) and [LICENSE-APACHE](LICENSE-APACHE).

## References

[BDN18] Boneh, D., Drijvers, M., & Neven, G. (2018, December). [Compact multi-signatures for smaller blockchains](https://eprint.iacr.org/2018/483.pdf). In International Conference on the Theory and Application of Cryptology and Information Security (pp. 435-464). Springer, Cham.

[BLS01] Boneh, D., Lynn, B., & Shacham, H. (2001, December). [Short signatures from the Weil pairing](https://link.springer.com/content/pdf/10.1007/3-540-45682-1_30.pdf). In International Conference on the Theory and Application of Cryptology and Information Security (pp. 514-532). Springer, Berlin, Heidelberg.

[BCGMMW18] Bowe, S., Chiesa, A., Green, M., Miers, I., Mishra, P., & Wu, H. (2018). [Zexe: Enabling decentralized private computation](https://eprint.iacr.org/2018/962.pdf). IACR ePrint, 962.

[pairings] Costello, C. . [Pairings for beginners](http://www.craigcostello.com.au/pairings/PairingsForBeginners.pdf).

[BP17] Budroni, A., & Pintore, F. (2017). [Efficient hash maps to G2 on BLS curves](https://eprint.iacr.org/2017/419.pdf). Cryptology ePrint Archive, Report 2017/419.

[RY07] Ristenpart, T., & Yilek, S. (2007, May). The power of proofs-of-possession: Securing multiparty signatures against rogue-key attacks. In Annual International Conference on the Theory and Applications of Cryptographic Techniques (pp. 228-245). Springer, Berlin, Heidelberg.
