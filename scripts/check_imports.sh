#!/usr/bin/env bash
grep --exclude-dir=compiled-system-contracts --files-with-matches "github.com/ethereum/go-ethereum" --recursive . --include="*.go"
if [ "$?" -gt "0" ]; then
    exit 0
else
    echo The above files reference "github.com/ethereum/go-ethereum" instead of "github.com/celo-org/celo-blockchain"
    echo Run ./scripts/rename_imports.sh to fix.
    exit 1
fi
