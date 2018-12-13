#!/usr/bin/env bash
set -euo pipefail

# A script to backup Geth node's blockchain data to GCS bucket.
# Currently, there is no way to configure the location of geth or geth data directory.
# The script assumes that geth binary is in the path and that
# Geth data directory is in its default location, that is, which as of now maps to ~/.ethereum directory.
# Usage: backup.sh <etherum network name> <node-name> <bucket-name>
# Sample invocation: /usr/local/bin/backup.sh ashishbtestnet3 gethminer1 blockchain_backup
export NETWORK="$1"  # {{ template "ethereum.fullname" . }}
export NODE_NAME="$2"  # {{ .Node.name}}
export GCS_BUCKET="$3"  # blockchain_backup

geth attach --exec 'eth.blockNumber' && export LATEST_BLOCK=$(geth attach --exec 'eth.blockNumber') || LATEST_BLOCK=999999999999999999
export BACKUP_NAME="geth-backup-$NODE_NAME-timestamp-$(date +%s)-blocks-0-to-$LATEST_BLOCK.db"
# If LATEST_BLOCK is initialized to 999999999999999999 then this will fail beyond the last valid block but the overall backup is still fine
echo "Exporting Geth database to $BACKUP_NAME"
# We need to copy before backup since if the sync is going on then "geth export" command fails with
# "Fatal: Could not open database: resource temporarily unavailable"
cp -r  ~/.ethereum ~/.ethereum_backup
geth --syncmode=full --datadir=~/.ethereum_backup --verbosity 5 export $BACKUP_NAME 0 $LATEST_BLOCK || true
rm -r ~/.ethereum_backup
echo "Geth database exported to $BACKUP_NAME"
ls -al $BACKUP_NAME
~/google-cloud-sdk/bin/gcloud config list
echo "gsutil cp $BACKUP_NAME gs://$GCS_BUCKET/$NETWORK"
# Irrespective of whether backup succeeded or failed, delete the file from the local disk
~/google-cloud-sdk/bin/gsutil cp $BACKUP_NAME gs://$GCS_BUCKET/$NETWORK || true
rm $BACKUP_NAME
