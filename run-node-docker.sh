#!/usr/bin/env bash
set -euo pipefail

###
# Docker node runner
# Usage ./run-node.sh <network> <networkid> <docker image> <rpc port> <es port> <port> <syncmode>
###

NETWORK=${1:-integration}
NETWORKID=${2:-1101}
IMAGE=${3:-us.gcr.io/celo-testnet/celo-node:integration}
RPCPORT=${4:-8545}
WSPORT=${5:-8546}
PORT=${6:-30303}
SYNCMODE="full"

mkdir -p ./nodes/

cd ./nodes/ || exit

if [ ! -d "./keystore/" ]
then
  docker run -v "$(pwd)":/root/.celo -it "$IMAGE" account new
fi

mkdir -p "$NETWORK"

cp -r ./keystore/ "$NETWORK"/keystore/

ADDRESS="$(grep -ro '"address": *"[^"]*"' --include='UTC*' ./keystore/ | grep -o '"[^"]*"$' | tr -d '"')"

echo Using Celo address: "$ADDRESS"

cd "$NETWORK" || exit

docker run -v "$(pwd)":/root/.celo \
           -it \
           "$IMAGE" \
           init /celo/genesis.json

docker run -v "$(pwd)":/root/.celo \
           --entrypoint cp \
           -it \
           "$IMAGE" \
           /celo/static-nodes.json \
           /root/.celo/

docker run -p 127.0.0.1:"$RPCPORT":8545 \
           -p 127.0.0.1:"$WSPORT":8546 \
           -p "$PORT":30303 \
           -p "$PORT":30303/udp \
           -v "$(pwd)":/root/.celo \
           -it \
           "$IMAGE" \
           --verbosity 3 \
           --networkid "$NETWORKID" \
           --syncmode "$SYNCMODE" \
           --rpc \
           --rpcaddr 0.0.0.0 \
           --rpcapi eth,net,web3,debug,admin,personal \
           --lightserv 90 \
           --lightpeers 1000 \
           --maxpeers 1100 \
           --etherbase "$ADDRESS"
