###
# Node runner
# Usage ./run-node.sh <network> <networkid> <docker image> <port 1> <port 2> <port 3> <syncmode>
###

NETWORK=${1:-integration}
NETWORKID=${2:-1101}
IMAGE=${3:-us.gcr.io/celo-testnet/celo-node:integration}
PORT1=${4:-8545}
PORT2=${5:-8546}
PORT3=${6:-30303}
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

docker run -p 127.0.0.1:"$PORT1":8545 \
           -p 127.0.0.1:"$PORT2":8546 \
           -p "$PORT3":30303 \
           -p "$PORT3":30303/udp \
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
