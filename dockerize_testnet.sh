#!/bin/bash
PROJECT_ID=false
TESTNET_NAME=false

while getopts 'p:t:' flag; do
  case "${flag}" in
    p) PROJECT_ID="${OPTARG}" ;;
    t) TESTNET_NAME="${OPTARG}" ;;
    *) error "Unexpected option ${flag}" ;;
  esac
done

docker build . -t testnet-geth
docker build . -f Dockerfile.kubeboot -t testnet-boot
docker tag testnet-geth:latest gcr.io/$PROJECT_ID/testnet-geth:$TESTNET_NAME
docker tag testnet-boot:latest gcr.io/$PROJECT_ID/testnet-boot:$TESTNET_NAME
docker push gcr.io/$PROJECT_ID/testnet-geth:$TESTNET_NAME
docker push gcr.io/$PROJECT_ID/testnet-boot:$TESTNET_NAME


