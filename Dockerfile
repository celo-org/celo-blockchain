# How to test changes to this file
# docker build -f Dockerfile -t gcr.io/celo-testnet/geth:$USER .
# To locally run that image
# docker run gcr.io/celo-testnet/geth:$USER
# Then connect to it by first getting its container ID via docker ps
# and then docker exec -t -i <containerID>  /bin/sh
# Once you are satisfied, push the image to the cloud
# docker push gcr.io/celo-testnet/geth:$USER
# To use this image for testing, modify GETH_NODE_DOCKER_IMAGE_TAG in celo-monorepo/.env file

# Build Geth in a stock Go builder container
FROM golang:1.11-alpine as builder

RUN apk add --no-cache make gcc musl-dev linux-headers

ADD . /go-ethereum
RUN cd /go-ethereum && make geth

# Pull Geth into a second stage deploy alpine container
FROM alpine:latest

RUN apk add --no-cache ca-certificates
COPY --from=builder /go-ethereum/build/bin/geth /usr/local/bin/

# Uncomment this if you want to edit a file inside this docker image via vim
# RUN apk add vim

# For google-cloud-sdk
RUN apk add bash
RUN apk add curl
RUN apk add python
RUN curl -sSL https://sdk.cloud.google.com | bash

EXPOSE 8545 8546 30303 30303/udp
ENTRYPOINT ["geth"]
