docker rmi -f celoarm64
docker build -f Dockerfile.arm64 -t celoarm64 .
docker rm -f celoarm64build
docker run --name celoarm64build celoarm64 bash -c 'cd crypto/bls/bls-zexe/bls && cargo build --target aarch64-unknown-linux-gnu --release'
TARGET_DIR=crypto/bls/bls-zexe/bls/target/aarch64-unknown-linux-gnu/release
mkdir -p $TARGET_DIR
docker cp celoarm64build:/go-ethereum/crypto/bls/bls-zexe/bls/target/aarch64-unknown-linux-gnu/release/libbls_zexe.a $TARGET_DIR
make geth-linux-arm64
