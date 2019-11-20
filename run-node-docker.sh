#!/usr/bin/env bash
set -euo pipefail

###
# Docker node runner
# Usage ./run-node.sh <network> <networkid> <docker image> <rpc port> <ws port> <port> <syncmode>
###

NETWORK=${1:-integration}
NETWORKID=${2:-1101}
IMAGE=${3:-us.gcr.io/celo-testnet/celo-node:integration}
RPCPORT=${4:-8545}
WSPORT=${5:-8546}
PORT=${6:-30303}
SYNCMODE=${7:-full}
WORKDIR="."
VALIDATOR="false"

# go to work dir
cd "$WORKDIR"

# create nodes dir
mkdir -p ./nodes/
cd ./nodes/ || exit

# create keys if not yet done
if [ ! -d "./keystore/" ]
then
  docker run -v "$(pwd)":/root/.celo -it "$IMAGE" account new
fi

# create network dir
mkdir -p "$NETWORK"

# copy password and keystore to network
cp -r ./keystore/ "$NETWORK"/keystore/
cp ./password.txt "$NETWORK"/

# get address of node key
ADDRESS="$(grep -ro '"address": *"[^"]*"' --include='UTC*' ./keystore/ | grep -o '"[^"]*"$' | tr -d '"')"

echo Using Celo address: "$ADDRESS"

# switch to network di
cd "$NETWORK" || exit

# create genesis file
docker run -v "$(pwd)":/root/.celo \
           -it \
           "$IMAGE" \
           init /celo/genesis.json

# copy static boot nodes
docker run -v "$(pwd)":/root/.celo \
           --entrypoint cp \
           -it \
           "$IMAGE" \
           /celo/static-nodes.json \
           /root/.celo/

# evaluate validator args in case the node should be started as an validator
if [ $VALIDATOR == "true" ]; then
  mine_args="--mine
             --password=/root/.celo/password.txt
             --unlock=$ADDRESS"
else
  mine_args=''
fi

# run the node
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
           --etherbase "$ADDRESS" \
           ${mine_args}
