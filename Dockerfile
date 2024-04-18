# How to test changes to this file
# docker build -f Dockerfile -t gcr.io/celo-testnet/geth:$USER .
# To locally run that image
# docker rm geth_container ; docker run --name geth_container gcr.io/celo-testnet/geth:$USER
# and connect to it with
# docker exec -t -i geth_container /bin/sh
#
# Once you are satisfied, build the image using
# export COMMIT_SHA=$(git rev-parse HEAD)
# docker buildx build --platform linux/amd64,linux/arm64 -f Dockerfile --build-arg COMMIT_SHA=$COMMIT_SHA -t gcr.io/celo-testnet/geth:$COMMIT_SHA .
#
# push the image to the cloud
# docker push gcr.io/celo-testnet/geth:$COMMIT_SHA
#
# To use this image for testing, modify GETH_NODE_DOCKER_IMAGE_TAG in celo-monorepo/.env file

# Build Geth in a stock Go builder container
FROM golang:1.19-bookworm as builder

ADD . /go-ethereum

RUN apt update && \
    cd /go-ethereum && \
    platform="$(dpkg --print-architecture)" && \
    case "$platform" in \
      "amd64") apt install -y build-essential git linux-headers-$platform && make geth ;; \
      "arm64") apt install -y build-essential git linux-headers-$platform musl-dev && make geth-musl ;; \
      *) echo "Unsupported platform: $platform" && exit 1 ;; \
    esac

# Using debian:bookworm-slim as the base image for the final
FROM debian:bookworm-slim
ARG COMMIT_SHA

RUN apt update &&\
    apt install -y ca-certificates wget curl &&\
    rm -rf /var/cache/apt &&\
    rm -rf /var/lib/apt/lists/* &&\
    ln -sf /bin/bash /bin/sh

COPY --from=builder /go-ethereum/build/bin/geth /usr/local/bin/
RUN echo $COMMIT_SHA > /version.txt
ADD scripts/run_geth_in_docker.sh /

EXPOSE 8545 8546 30303 30303/udp
ENTRYPOINT ["sh", "/run_geth_in_docker.sh"]

# Add some metadata labels to help programatic image consumption
ARG COMMIT=""
ARG VERSION=""
ARG BUILDNUM=""

LABEL commit="$COMMIT" version="$VERSION" buildnum="$BUILDNUM"

