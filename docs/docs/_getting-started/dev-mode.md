---
title: Dev mode
sort_key: B
---

Geth has a development mode that sets up a single node Ethereum test network with options optimized for developing on local machines. You enable it with the `--dev` argument.

Starting geth in dev mode does the following:

-   Initializes the data directory with a testing genesis block
  -   Sets default block period to 1 second
-   Sets max peers to 0
-   Turns off discovery by other nodes
-   Sets the gas price to 0
-   Uses the Clique PoA consensus engine with which allows blocks to be mined as-needed without excessive CPU and memory consumption
-   Uses on-demand block generation, producing blocks when transactions are waiting to be mined

## Start Geth in Dev Mode

Note: this command currently does not work as intended in ephemeral mode and must be run with a data dictionary that contains a keystore file with the hard-coded developer account `0x47e172f6cfb6c7d01c1574fa3e2be7cc73269d95`. The easiest way of doing this is to:

1. Run the command with the desired `--datadir` (and `--dev.period` if a specific block period is desired): `geth --datadir test-chain-dir --dev.period 5`.
    This will create the required genesis file in `test-chain-dir` and import the hard-coded developer account `0x47e172f6cfb6c7d01c1574fa3e2be7cc73269d95`

2. Then run: `geth --dev --miner.validator 0x47e172f6cfb6c7d01c1574fa3e2be7cc73269d95 --datadir test-chain-dir`
    Note that this will use the block period configured in the first step.

3. Re-run with the same state by repeating step 2. To start the chain without previous state, delete the datadir and start from step 1.


### Example with Remix

For this guide, start geth in dev mode as described above, and enable [RPC](../_rpc/server.md) so you can connect other applications to geth. For this guide, we use Remix, the web-based Ethereum IDE, so also allow its domains to accept cross-origin requests.

```shell
geth --dev --datadir test-chain-dir
geth --dev --datadir test-chain-dir --miner.validator 0x47e172f6cfb6c7d01c1574fa3e2be7cc73269d95 --http --http.corsdomain "https://remix.ethereum.org,http://remix.ethereum.org"
```

Connect to the IPC console on the node from another terminal window:

```shell
geth attach <IPC_LOCATION>
```

Once geth is running in dev mode, you can interact with it in the same way as when geth is running in other ways.

For example, create a test account:

```shell
> personal.newAccount()
```

And transfer ether from the coinbase to the new account:

```shell
> eth.sendTransaction({from:eth.coinbase, to:eth.accounts[1], value: web3.toWei(0.05, "ether")})
```

And check the balance of the account:

```shell
> eth.getBalance(eth.accounts[1])
```

If you want to test your dapps with a realistic block time use the `--dev.period` option when you start dev mode with the `--dev.period 14` argument.

#### Connect Remix to Geth

With geth now running, open <https://remix.ethereum.org>. Compile the contract as normal, but when you deploy and run a contract, select _Web3 Provider_ from the _Environment_ dropdown, and add "http://127.0.0.1:8545" to the popup box. Click _Deploy_, and interact with the contract. You should see contract creation, mining, and transaction activity.