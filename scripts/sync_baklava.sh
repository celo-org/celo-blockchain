#!/bin/bash

DATADIR=`pwd`/syncdatadir
NETWORK_ID=200110
echo "Using datadir $DATADIR"

rm -rf $DATADIR
cd ..
make geth
cd scripts
mkdir $DATADIR
cd $DATADIR
curl https://www.googleapis.com/storage/v1/b/static_nodes/o/baklava\?alt\=media > static-nodes.json
curl https://www.googleapis.com/storage/v1/b/genesis_blocks/o/baklava\?alt\=media > genesis.json
../../build/bin/geth --datadir $DATADIR init genesis.json
../../build/bin/geth --datadir $DATADIR --verbosity 4 --networkid $NETWORK_ID --syncmode lightest --maxpeers 1100 --nodiscover
