### Docs

The Celo blockchain client documentation is forked from the [go-ethereum](https://github.com/ethereum/go-ethereum) documentation, modified as needed to match changes made to the client.
These are work in progress and might not always be accurate. Please [let us know](https://github.com/celo-org/celo-blockchain/issues/new) if there are any issues.

Links:
- [geth docs](https://geth.ethereum.org/)

See also:
- [general celo docs](https://docs.celo.org)

### Building the docs locally

#### With Docker

If you can take advantage of Docker, run:
```
$ docker-compose up
```

Navigate to [localhost:4000](http://localhost:4000) to browse the docs.

#### With Ruby

In order to build the docs you need to have `ruby` installed. We recommend using [asdf](https://asdf-vm.com/) or [rvm](https://rvm.io/).
After that you need to install dependencies:

```
$ bundle install
```

And run jekyll:

```
$ bundle exec jekyll server
```


Navigate to [localhost:4000](http://localhost:4000) to browse the docs.
