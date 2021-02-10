### BLS12-377 Implementation in Go

This library is adapted from [BLS12-381 implementation](https://github.com/kilic/bls12-381)

#### Pairing Instance

A Group instance or a pairing engine instance _is not_ suitable for concurrent processing since an instance has its own preallocated memory for temporary variables. A new instance must be created for each thread.

#### Base Field

x86 optimized base field is generated with [kilic/fp](https://github.com/kilic/fp) and for native go is generated with [goff](https://github.com/ConsenSys/goff). Generated codes are slightly edited in both for further requirements.

#### Scalar Field

Standart big.Int module is currently used for scalar field elements. x86 optimized faster field implementation is planned to be added.

#### Benchmarks

on _2.3 GHz i7_

```
BenchmarkPairing  1089696 ns/op
```

