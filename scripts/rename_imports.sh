#!/usr/bin/env bash
find . \
     -type f \
     -name '*.go' \
     -not -name "rename.sh" \
     -exec sed -i "" "s|github.com/ethereum/go-ethereum|github.com/celo-org/celo-blockchain|" {} \;
