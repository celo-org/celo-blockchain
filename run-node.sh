#!/usr/bin/env bash
set -euo pipefail

# Usage: start_geth.sh [geth dir] [network name] [network id] [sync mode]
GETH_DIR=${1:-"."}
# Default to testing the integration network
NETWORK_NAME=${2:-"integration"}
NETWORK_ID=${3:-"1101"}
# Default to testing the full sync mode
SYNCMODE=${4:-"full"}

echo "This will start geth local node from ${GETH_DIR} in '${SYNCMODE}' sync mode which will connect to network '${NETWORK_NAME}'..."

echo "Setting constants..."
DATA_DIR="/tmp/tmp1"
#rm -rf ${DATA_DIR}
GENESIS_FILE_PATH="/tmp/genesis_ibft.json"
GETH_BINARY="${GETH_DIR}/build/bin/geth --datadir ${DATA_DIR}"

echo "Building geth..."
cd ${GETH_DIR} && make -j geth && cd -
echo "Initializing genesis file..."
rm -rf ${GENESIS_FILE_PATH}
curl "https://www.googleapis.com/storage/v1/b/genesis_blocks/o/${NETWORK_NAME}?alt=media" > ${GENESIS_FILE_PATH}

echo "Initializing data dir..."
#rm -rf ${DATA_DIR}
${GETH_BINARY} init ${GENESIS_FILE_PATH} 1>/dev/null
echo "Initializing static nodes..."
curl "https://www.googleapis.com/storage/v1/b/static_nodes/o/${NETWORK_NAME}?alt=media" > ${DATA_DIR}/static-nodes.json

echo "Running geth..."
# Run geth in the background
${GETH_BINARY} --syncmode ${SYNCMODE} \
   --rpc \
   --ws \
   --wsport=8546 \
   --wsorigins=* \
   --rpcapi=eth,net,web3,debug,admin,personal,txpool \
   --debug \
   --port=30303 \
   --nodiscover \
   --rpcport=8545 \
   --rpcvhosts=* \
   --networkid="$NETWORK_ID" \
   --verbosity=3 \
   --consoleoutput=stdout \
   --consoleformat=term \
   --ethstats=${HOSTNAME}:${WS_SECRET:-1234}@localhost:3000 \
   --rpccorsdomain='*' \
   --rpcvhosts='*' \
   --ws \
   --wsaddr 0.0.0.0 \
   --wsapi=eth,net,web3,debug
