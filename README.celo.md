### How to build geth for Mac OS

1. `make -j geth` or,
2. `celotooljs geth build` ([celotooljs](https://github.com/celo-org/celo-monorepo/tree/master/packages/celotool))

### How to build geth for Android

`make -j android` will produce the binary in `build/bin/geth.aar`

You might have to provide the path to ANDROID_NDK
`ANDROID_NDK=/usr/local/Caskroom/android-ndk/18/android-ndk-r18/ make android -j`

then `cp build/bin/geth.aar <celo-monorepo>/node_modules/@celo/geth/build/bin/geth.aar` testing the new geth.aar binary. Note that running `yarn` in <celo-monorepo> will overwrite this.

### How to test geth

1. `make -j test` - this runs, primarily, the unittests which came  from go-ethereum open-source package
2. `packages/protocol $ ./ci_test.sh` - this contains a few basic Celo-specific tests like transferring Celo $ and Celo Gold. Customize the [Geth dir](https://github.com/celo-org/celo-monorepo/blob/master/packages/celotool/geth_tests/constants.sh#L13) to run these tests against your local Geth node.


### How to run/interact with geth

Use [Celotooljs](https://github.com/celo-org/celo-monorepo/tree/master/packages/celotool) for the most high-level Celo-specific commands related to Geth.


### List of major changes we have made to Geth

1. [Transfer Precompiled Contract](https://github.com/celo-org/geth/pull/75)
2. [CeloLatestSync Mode](https://github.com/celo-org/geth/pull/62)

(This section is incomplete)
