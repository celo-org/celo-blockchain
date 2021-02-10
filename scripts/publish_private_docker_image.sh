#!/usr/bin/env bash
COMMIT_SHA=$(git rev-list  HEAD | head -n 1)

docker build --build-arg COMMIT_SHA="$COMMIT_SHA" -t gcr.io/celo-testnet/geth:"$COMMIT_SHA" .
docker build --build-arg COMMIT_SHA="$COMMIT_SHA" -t gcr.io/celo-testnet/geth-all:"$COMMIT_SHA"  -f Dockerfile.alltools .
docker push gcr.io/celo-testnet/geth:"$COMMIT_SHA"
docker push gcr.io/celo-testnet/geth-all:"$COMMIT_SHA"
