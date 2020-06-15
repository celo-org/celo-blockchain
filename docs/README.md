### Docs

Celo Blockchain docs are a superset of Geth docs with Celo protocol specific additions.

Links:
- [celo client docs](https://client-docs.celo.org)
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
