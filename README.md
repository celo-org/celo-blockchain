## Celo Blockchain

Official golang implementation of the Celo blockchain, based off of the [official golang implementation of the Ethereum protocol](https://github.com/ethereum/go-ethereum).

[![Discord](https://img.shields.io/badge/discord-join%20chat-blue.svg)](https://chat.celo.org)

Prebuilt [Docker](https://en.wikipedia.org/wiki/Docker_\(software\)) images are available for immediate use: [us.gcr.io/celo-testnet/celo-node](https://us.gcr.io/celo-testnet/celo-node). See [docs.celo.org/getting-started](https://docs.celo.org/getting-started) for a guide to the Celo networks and how to get started.

Documentation for Celo more generally can be found at [docs.celo.org](https://docs.celo.org/)

Most functionality of this client is similar to `go-ethereum`, also known as `geth`, from which it was forked. If you do not find your question answered by Celo-specific documentation, try searching the [geth wiki](https://github.com/ethereum/go-ethereum/wiki).

## Building the source

Building `geth` requires both a Go (version 1.16) and a C compiler.
You can install them using your favourite package manager. Once the dependencies are installed, run

```shell
make geth
```

or, to build the full suite of utilities:

```shell
make all
```

### Mobile Clients

There are two different commands in the `Makefile` to build the `ios` and the `android` clients.

```shell
make ios
```

and

```shell
make android
```

Note: The `android` command it applies a git patch (`patches/mobileLibsForBuild.patch`) required to swap some libs from the `go.mod` for the client to work, installs those libs, builds the client, and then reverts the patch.

## Executables

The Celo blockchain client comes with several wrappers/executables found in the `cmd` directory.

| Command    | Description |
|:----------:|-------------|
| **`geth`** | The main Celo Blockchain client. It is the entry point into the Celo network, capable of running as a full node (default), archive node (retaining all historical state), light node (retrieving data live), or lightest node (retrieving minimum number of block headers to verify existing validator set). It can be used by other processes as a gateway into the Celo network via JSON RPC endpoints exposed on top of HTTP, WebSocket and/or IPC transports. `geth --help` and the [Ethereum CLI Wiki page](https://github.com/ethereum/go-ethereum/wiki/Command-Line-Options) for command line options. |
| `abigen` | Source code generator to convert Celo contract definitions into easy to use, compile-time type-safe Go packages. It operates on plain [Ethereum contract ABIs](https://github.com/ethereum/wiki/wiki/Ethereum-Contract-ABI) with expanded functionality if the contract bytecode is also available. However it also accepts Solidity source files, making development much more streamlined. Please see [Ethereum's Native DApps](https://github.com/ethereum/go-ethereum/wiki/Native-DApps:-Go-bindings-to-Ethereum-contracts) wiki page for details. |
| `bootnode` | Stripped down version of the Celo client implementation that only takes part in the network node discovery protocol, but does not run any of the higher level application protocols. It can be used as a lightweight bootstrap node to aid in finding peers in private networks. |
| `evm` | Developer utility version of the EVM (Ethereum Virtual Machine) that is capable of running bytecode snippets within a configurable environment and execution mode. Its purpose is to allow isolated, fine-grained debugging of EVM opcodes (e.g. `evm --code 60ff60ff --debug run`). |
| `gethrpctest` | Developer utility tool to support the [ethereum/rpc-test](https://github.com/ethereum/rpc-tests) test suite which validates baseline conformity to the [Ethereum JSON RPC](https://github.com/ethereum/wiki/wiki/JSON-RPC) specs. Please see the [ethereum test suite's readme](https://github.com/ethereum/rpc-tests/blob/master/README.md) for details. |
| `rlpdump` | Developer utility tool to convert binary RLP ([Recursive Length Prefix](https://github.com/ethereum/wiki/wiki/RLP)) dumps (data encoding used by the Celo protocol both network as well as consensus wise) to user friendlier hierarchical representation (e.g. `rlpdump --hex CE0183FFFFFFC4C304050583616263`). |

## Running tests

Prior to running tests you will need to run `make prepare-system-contracts`.
This will checkout the celo-monorepo and compile the system contracts for use in
full network tests.

Without first running this certain tests will fail with errors such as:

```
panic: Can't read bytecode for monorepo/packages/protocol/build/contracts/FixidityLib.json: open
```

## Running Celo

Please see the [docs.celo.org/getting-started](https://docs.celo.org/getting-started) for instructions on how to run a node connected the Celo network using the prebuilt Docker image.

Going through all the possible command line flags is out of scope here, please consult `geth --help` for more complete information.
We've enumerated a few common parameter combos to get you up to speed quickly on how you can run your own Celo blockchain client instance.

### Full node on the main Celo network

By default, the Celo client will connect to the Mainnet.
Running the following command will create a full node that will sync with the Celo network and allow access to all of its functionality.

```shell
$ geth console
```

This command will:
 * Start `geth` in full sync mode which will download and execute all historical block information.
 * Start up `geth`'s built-in interactive [JavaScript console](https://github.com/ethereum/go-ethereum/wiki/JavaScript-Console),
   (via the trailing `console` subcommand) through which you can invoke all official [`web3` methods](https://github.com/ethereum/wiki/wiki/JavaScript-API)
   as well as `geth`'s own [management APIs](https://github.com/ethereum/go-ethereum/wiki/Management-APIs).
   This tool is optional and if you leave it out you can always attach to an already running
   `geth` instance with `geth attach`.

### A Full node on the Alfajores test network

Smart contract developers will be most interested in the Alfajores testnet.
On Alfajores, you can receive testnet Celo Gold through the [Alfajores faucet](https://celo.org/developers/faucet) and deploy smart contracts in an environment very similar to Mainnet.
More information about the Alfajores testnet can be found on [docs.celo.org](https://docs.celo.org/getting-started/alfajores-testnet).

```shell
$ geth --alfajores console
```

*Note: Although there are some internal protective measures to prevent transactions from
crossing over between the main network and test network, you should make sure to always
use separate accounts for testnet-tokens and real-tokens. Unless you manually move
accounts, `geth` will by default correctly separate the two networks and will not make any
accounts available between them.*

### Full node on the Baklava test network

Validators and full node operators will be most interested in the Baklava testnet.
On Baklava, you can receive a distribution of testnet Celo Gold to become a validator on the network and test out running a validator for the first time, or try out new infrastructure.
More information about the Baklava testnet can be found on [docs.celo.org](https://docs.celo.org/getting-started/baklava-testnet).
A full guide to getting started as a validator on Baklava can be found in the [Getting Started guides](https://docs.celo.org/getting-started/baklava-testnet/running-a-validator-in-baklava)

```shell
$ geth --baklava console
```

### Configuration

As an alternative to passing the numerous flags to the `Celo` binary, you can also pass a configuration file via:

```shell
$ geth --config /path/to/your_config.toml
```

To get an idea how the file should look like you can use the `dumpconfig` subcommand to
export your existing configuration:

```shell
$ geth --your-favourite-flags dumpconfig
```

### Programmatically interfacing `geth` nodes

As a developer, sooner rather than later you'll want to start interacting with `geth` and the
Celo network via your own programs and not manually through the console. To aid
this, `geth` has built-in support for a JSON-RPC based APIs ([standard APIs](https://github.com/ethereum/wiki/wiki/JSON-RPC)
and [`geth` specific APIs](https://github.com/ethereum/go-ethereum/wiki/Management-APIs)).
These can be exposed via HTTP, WebSockets and IPC (UNIX sockets on UNIX based
platforms, and named pipes on Windows).

The IPC interface is enabled by default and exposes all the APIs supported by `geth`,
whereas the HTTP and WS interfaces need to manually be enabled and only expose a
subset of APIs due to security reasons. These can be turned on/off and configured as
you'd expect.

HTTP based JSON-RPC API options:

  * `--rpc` Enable the HTTP-RPC server
  * `--rpcaddr` HTTP-RPC server listening interface (default: `localhost`)
  * `--rpcport` HTTP-RPC server listening port (default: `8545`)
  * `--rpcapi` API's offered over the HTTP-RPC interface (default: `eth,net,web3`)
  * `--rpccorsdomain` Comma separated list of domains from which to accept cross origin requests (browser enforced)
  * `--ws` Enable the WS-RPC server
  * `--wsaddr` WS-RPC server listening interface (default: `localhost`)
  * `--wsport` WS-RPC server listening port (default: `8546`)
  * `--wsapi` API's offered over the WS-RPC interface (default: `eth,net,web3`)
  * `--wsorigins` Origins from which to accept websockets requests
  * `--ipcdisable` Disable the IPC-RPC server
  * `--ipcapi` API's offered over the IPC-RPC interface (default: `admin,debug,eth,miner,net,personal,shh,txpool,web3`)
  * `--ipcpath` Filename for IPC socket/pipe within the datadir (explicit paths escape it)

You'll need to use your own programming environments' capabilities (libraries, tools, etc) to
connect via HTTP, WS or IPC to a `geth` node configured with the above flags and you'll
need to speak [JSON-RPC](https://www.jsonrpc.org/specification) on all transports. You
can reuse the same connection for multiple requests!

**Note: Please understand the security implications of opening up an HTTP/WS based
transport before doing so! Hackers on the internet are actively trying to subvert
Celo nodes with exposed APIs! Further, all browser tabs can access locally
running web servers, so malicious web pages could try to subvert locally available
APIs!**


## Contribution

Thank you for considering to help out with the source code! We welcome contributions
from anyone on the internet, and are grateful for even the smallest of fixes!

If you'd like to contribute to celo-blockchain, please fork, fix, commit and send a pull request
for the maintainers to review and merge into the main code base. If you wish to submit more
complex changes though, please check up with the core devs first on [the official Celo forum](https://forum.celo.org/c/protocol)
to ensure those changes are in line with the general philosophy of the project and/or get some
early feedback which can make both your efforts much lighter as well as our review and merge
procedures quick and simple.

Please make sure your contributions adhere to our coding guidelines:

 * Code must adhere to the official Go [formatting](https://golang.org/doc/effective_go.html#formatting)
   guidelines (i.e. uses [gofmt](https://golang.org/cmd/gofmt/)).
 * Code must be documented adhering to the official Go [commentary](https://golang.org/doc/effective_go.html#commentary)
   guidelines.
 * Pull requests need to be based on and opened against the `master` branch.
 * Commit messages should be prefixed with the package(s) they modify.
   * E.g. "eth, rpc: make trace configs optional"

### Submitting an issue

If you come across a bug, pleas open a [GitHub issue](https://github.com/celo-org/celo-blockchain/issues/new) with information about your build and what happened.

### CI Testing and automerge

We run a circle CI test suite on each PR. The following tests are required to
merge a PR.
  * Unit tests: `make test` or `./build/env.sh go run build/ci.go test`
  * Lint: `make lint` (Fix go format errors with `gofmt -s`)
  * Build: `make`
  * End to end sync and transfer tests
  * Check imports: `./scripts/check_imports.sh`
 
 `celo-blockchain` is based on `go-ethereum`, but the import path has been renamed from `github.com/ethereum/go-ethereum` to `github.com/celo-org/celo-blockchain`.
 Developers are encouraged to run `./scripts/setup_git_hooks.sh` to enable checking that import path has been changed to `celo-org` on `git merge` and `git commit`.
 Imports can automatically be renamed with `./scripts/rename_imports.sh`.


Individual package tests can be run with
`./build/env.sh go test github.com/celo-org/celo-blockchain/$(PATH_TO_GO_PACKAGE)`
if you don't have `GOPATH` set-up.


Once a PR is approved, adding on the `automerge` label will keep it up to date
and do a squash merge once all the required tests have passed.

### Benchmarking

Golang has built in support for running benchmarks with go tool
`go test -run=ThisIsNotATestName -bench=. ./$PACKAGE_NAME` will run all benchmarks in a package.

One note around running benchmarks is that `BenchmarkHandlePreprepare` is quite takes a while to run, particularly when testing with a larger number of validators.
Substituting `-bench=REGEX` for `-bench=.` will specify which tests to run. Adding `-cpuprofile=cpu.out` which can be visualized with `go tool pprof -html:8080 cpu.out` if `graphviz` is installed.

See the [go testing flags](https://golang.org/cmd/go/#hdr-Testing_flags) and [go docs](https://golang.org/pkg/testing/#hdr-Benchmarks) for more information on benchmarking.


## License

The celo-blockchain library (i.e. all code outside of the `cmd` directory) is licensed under the
[GNU Lesser General Public License v3.0](https://www.gnu.org/licenses/lgpl-3.0.en.html), also
included in our repository in the `COPYING.LESSER` file.

The celo-blockchain binaries (i.e. all code inside of the `cmd` directory) is licensed under the
[GNU General Public License v3.0](https://www.gnu.org/licenses/gpl-3.0.en.html), also included
in our repository in the `COPYING` file.
