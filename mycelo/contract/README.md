
To generate gen_abis.go run

Using the monorepo_commit file:
```
go run ./mycelo/internal/scripts/generate -buildpath ./compiled-system-contracts
```

For a custom one:
```
go run ./mycelo/internal/scripts/generate -buildpath $CELO_MONOREPO/packages/protocol/build
```