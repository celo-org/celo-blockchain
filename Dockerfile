# How to test changes to this file
# docker build -f Dockerfile -t gcr.io/celo-testnet/geth:$USER .
# To locally run that image
# docker rm geth_container ; docker run --name geth_container gcr.io/celo-testnet/geth:$USER
# and connect to it with
# docker exec -t -i geth_container /bin/sh
#
# Once you are satisfied, build the image using
# export COMMIT_SHA=$(git rev-parse HEAD)
# docker build -f Dockerfile --build-arg COMMIT_SHA=$COMMIT_SHA -t gcr.io/celo-testnet/geth:$COMMIT_SHA .
#
# push the image to the cloud
# docker push gcr.io/celo-testnet/geth:$COMMIT_SHA
#
# To use this image for testing, modify GETH_NODE_DOCKER_IMAGE_TAG in celo-monorepo/.env file

FROM ubuntu:16.04 as rustbuilder
RUN apt update && apt install -y curl musl-tools
RUN curl https://sh.rustup.rs -sSf | sh -s -- -y
ENV PATH=$PATH:~/.cargo/bin
RUN $HOME/.cargo/bin/rustup install 1.37.0 && $HOME/.cargo/bin/rustup default 1.37.0 && $HOME/.cargo/bin/rustup target add x86_64-unknown-linux-musl
ADD ./vendor /go-ethereum/vendor
RUN cd /go-ethereum/vendor/github.com/celo-org/bls-zexe/bls && $HOME/.cargo/bin/cargo build --target x86_64-unknown-linux-musl --release

# Build Geth in a stock Go builder container
FROM golang:1.11-alpine as builder

RUN apk add --no-cache make gcc musl-dev linux-headers
ADD . /go-ethereum
RUN mkdir -p /go-ethereum/vendor/github.com/celo-org/bls-zexe/bls/target/release
COPY --from=rustbuilder /go-ethereum/vendor/github.com/celo-org/bls-zexe/bls/target/x86_64-unknown-linux-musl/release/libbls_zexe.a /go-ethereum/vendor/github.com/celo-org/bls-zexe/bls/target/release
COPY --from=rustbuilder /go-ethereum/vendor/github.com/celo-org/bls-zexe/bls/target/x86_64-unknown-linux-musl/release/libbls_snark.a /go-ethereum/vendor/github.com/celo-org/bls-zexe/bls/target/release
RUN cd /go-ethereum && make geth

# Pull Geth into a second stage deploy alpine container
FROM alpine:latest
ARG COMMIT_SHA

RUN apk add --no-cache ca-certificates
COPY --from=builder /go-ethereum/build/bin/geth /usr/local/bin/
RUN echo $COMMIT_SHA > /version.txt
ADD scripts/run_geth_in_docker.sh /

EXPOSE 8545 8546 30303 30303/udp
ENTRYPOINT ["sh", "/run_geth_in_docker.sh"]
