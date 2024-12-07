
# Mycelo

`mycelo` is a developer utility to easily run celo blockchain testnets and related jobs around testnets.

Its main advantage over previous solutions is that it's able to create a `genesis.json` where all core conctracts are already deployed in it. Eventually it can be extended to support other cases, like e2e tests, load tests, and other operations.

## Using mycelo

There are 2 main use cases for mycelo:

 1. Run a local tesnet
 2. Create a `genesis.json` to be used in another testnet that will be run on a CloudProvider/Kubernetes

### Generating a genesis.json

Both cases share the need to create the `genesis.json`; to do so run:

```bash
mycelo genesis --buildpath path/to/protocol/build
```

Where `buildpath` is the path to truffle compile output folder. By default it will use `CELO_MONOREPO` environment variable as `$CELO_MONOREPO/packages/protocol/build/contracts`.

This will create a `genesis.json`.

If you want to run a local testnet, you'll want to create an environment, for that use:

```bash
mycelo genesis --newenv path/to/envfolder
```

This command will create folder `path/to/envfolder` and write there `genesis.json` and `env.json`

### Configuring Genesis

Genesis creation has many configuration options, for that `mycelo` use the concept of templates.

```bash
mycelo genesis --template=[local|loadtest|monorepo]
```

Additionally, you can override template options via command line, check `mycelo genesis --help` for options:

```bash
   --validators value    Number of Validators (default: 0)
   --dev.accounts value  Number of developer accounts (default: 0)
   --blockperiod value   Seconds between each block (default: 0)
   --epoch value         Epoch size (default: 0)
   --mnemonic value      Mnemonic to generate accounts
```

### Configuring Genesis (Advanced)

If that's not enough, you can ask mycelo to generate a `genesis-config` file that you can then customize and use to generate genesis

```bash
mycelo genesis-config path/to/env
```

This will create `path/to/env/env.json` & `path/to/env/genesis-config.json`.

Next step is to customize those files with your desired options, and then run:

```bash
mycelo genesis-from-config path/to/env
```

This command will read those files, and generate a `genesis.json` on the env folder


### Running a local testnet

Once you've created an environment, and the `genesis.json` you can now run your own local testnet.

First, you need to configure the nodes (this will run `geth init`, configure `static-nodes`, add validator accounts to the node, etc):

```bash
mycelo validator-init --geth path/to/geth/binary path/to/env
```

**NOTE**: If you don't specify geth binary path. It will attempt to use `$CELO_BLOCKCHAIN/build/bin/geth`


And then, run the nodes:

```bash
mycelo validator-run --geth path/to/geth/binary path/to/env
```

This command will run one geth node for each validator as subprocesses.


### Running a load bot (Experimental)

You can run a simple load bot with:

```bash
mycelo load-bot path/to/env
```

This will generate cUSD transfer on each of the developers account in the enviroment.

This feature is still experimental and needs more work, but it's already usable.


## What's missing?

You tell us!

Happy Coding!!!!!




