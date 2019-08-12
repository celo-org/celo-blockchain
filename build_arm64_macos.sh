docker rmi -f celoarm64
docker build -f Dockerfile.arm64 -t celoarm64 .
docker rm -f celoarm64build
docker run --name celoarm64build celoarm64 bash -c 'cd vendor/github.com/celo-org/bls-zexe/bls && cargo build --target aarch64-unknown-linux-gnu --release'
TARGET_DIR=vendor/github.com/celo-org/bls-zexe/bls/target/aarch64-unknown-linux-gnu/release
mkdir -p $TARGET_DIR
docker cp celoarm64build:/go-ethereum/vendor/github.com/celo-org/bls-zexe/bls/target/aarch64-unknown-linux-gnu/release/libbls_zexe.a $TARGET_DIR
make geth-linux-arm64
