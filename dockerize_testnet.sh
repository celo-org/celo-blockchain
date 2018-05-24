#!/bin/bash
PROJECT_NAME=$1

docker build . -t testnet-geth
docker build . -f Dockerfile.kubeboot -t testnet-boot
docker tag testnet-geth:latest gcr.io/$PROJECT_NAME/testnet-geth
docker tag testnet-boot:latest gcr.io/$PROJECT_NAME/testnet-boot
docker push gcr.io/$PROJECT_NAME/testnet-geth
docker push gcr.io/$PROJECT_NAME/testnet-boot


