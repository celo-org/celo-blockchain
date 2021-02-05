#!/usr/bin/env bash
grep --files-with-matches "github.com/ethereum/go-ethereum" --recursive . --include="*.go"
if [ "$?" -gt "0" ]; then
    exit 0
else
    echo The above files reference "github.com/ethereum/go-ethereum" instead of "github.com/celo-org/celo-blockchain"
    echo Run ./scripts/rename_imports.sh to fix.
    echo NOTE: You can still merge with the check failing at this time, but it will fail in the future.
    exit 0
fi
