#!/usr/bin/env bash
find . \
     -type f \
     -name '*.go' \
     -exec sed -i "" "s|github.com/ethereum/go-ethereum|github.com/celo-org/celo-blockchain|" {} \;
