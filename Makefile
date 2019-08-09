# This Makefile is meant to be used by people that do not usually work
# with Go source code. If you know what GOPATH is then you probably
# don't need to bother with make.

.PHONY: geth android ios geth-cross swarm evm all test clean
.PHONY: geth-linux geth-linux-386 geth-linux-amd64 geth-linux-mips64 geth-linux-mips64le
.PHONY: geth-linux-arm geth-linux-arm-5 geth-linux-arm-6 geth-linux-arm-7 geth-linux-arm64
.PHONY: geth-darwin geth-darwin-386 geth-darwin-amd64
.PHONY: geth-windows geth-windows-386 geth-windows-amd64

GOBIN = $(shell pwd)/build/bin
GO ?= latest

CARGO_exists := $(shell command -v cargo 2> /dev/null)

# example NDK values
#NDK_VERSION=android-ndk-r19c
#ANDROID_NDK_HOME=$(PWD)/ndk_bundle/$(NDK_VERSION)

geth: bls-zexe
	build/env.sh go run build/ci.go install ./cmd/geth
	@echo "Done building."
	@echo "Run \"$(GOBIN)/geth\" to launch geth."

bls-zexe: vendor/github.com/celo-org/bls-zexe/bls/target/release/libbls_zexe.a

check_android_env:
	@test $${ANDROID_NDK_HOME?Please set environment variable ANDROID_NDK_HOME}
	@test $${NDK_VERSION?Please set environment variable NDK_VERSION}

ndk_bundle: check_android_env
	curl --silent --show-error --location --fail --retry 3 --output /tmp/$(NDK_VERSION).zip \
		https://dl.google.com/android/repository/$(NDK_VERSION)-linux-x86_64.zip && \
	mkdir $(ANDROID_NDK_HOME) && \
	unzip -q /tmp/$(NDK_VERSION).zip -d $(ANDROID_NDK_HOME) && \
	rm /tmp/$(NDK_VERSION).zip

bls-zexe-android: check_android_env
	PATH="$$PATH:$(ANDROID_NDK_HOME)/toolchains/llvm/prebuilt/linux-x86_64/bin:$(ANDROID_NDK_HOME)/toolchains/aarch64-linux-android-4.9/prebuilt/linux-x86_64/bin"; ln -s $(ANDROID_NDK_HOME)/toolchains/llvm/prebuilt/linux-x86_64/bin/aarch64-linux-android21-clang $(ANDROID_NDK_HOME)/toolchains/llvm/prebuilt/linux-x86_64/bin/aarch64-linux-android-clang; cd vendor/github.com/celo-org/bls-zexe/bls && cargo build --release --target=aarch64-linux-android --lib
	PATH="$$PATH:$(ANDROID_NDK_HOME)/toolchains/llvm/prebuilt/linux-x86_64/bin:$(ANDROID_NDK_HOME)/toolchains/arm-linux-androideabi-4.9/prebuilt/linux-x86_64/bin"; cd vendor/github.com/celo-org/bls-zexe/bls && cargo build --release --target=armv7-linux-androideabi --lib
	PATH="$$PATH:$(ANDROID_NDK_HOME)/toolchains/llvm/prebuilt/linux-x86_64/bin:$(ANDROID_NDK_HOME)/toolchains/aarch64-linux-android-4.9/prebuilt/linux-x86_64/bin"; ln -s $(ANDROID_NDK_HOME)/toolchains/llvm/prebuilt/linux-x86_64/bin/i686-linux-android16-clang $(ANDROID_NDK_HOME)/toolchains/llvm/prebuilt/linux-x86_64/bin/i686-linux-android-clang; cd vendor/github.com/celo-org/bls-zexe/bls && cargo build --release --target=i686-linux-android --lib
	PATH="$$PATH:$(ANDROID_NDK_HOME)/toolchains/llvm/prebuilt/linux-x86_64/bin:$(ANDROID_NDK_HOME)/toolchains/aarch64-linux-android-4.9/prebuilt/linux-x86_64/bin"; ln -s $(ANDROID_NDK_HOME)/toolchains/llvm/prebuilt/linux-x86_64/bin/x86_64-linux-android21-clang $(ANDROID_NDK_HOME)/toolchains/llvm/prebuilt/linux-x86_64/bin/x86_64-linux-android-clang; cd vendor/github.com/celo-org/bls-zexe/bls && cargo build --release --target=x86_64-linux-android --lib

vendor/github.com/celo-org/bls-zexe/bls/target/release/libbls_zexe.a:
ifeq ("$(CARGO_exists)","")
	$(error "No cargo in PATH, consult https://github.com/celo-org/celo-monorepo/blob/master/SETUP.md")
else
	cd vendor/github.com/celo-org/bls-zexe/bls && cargo build --release && cargo build --release --example pop
endif

swarm:
	build/env.sh go run build/ci.go install ./cmd/swarm
	@echo "Done building."
	@echo "Run \"$(GOBIN)/swarm\" to launch swarm."

all:
	build/env.sh go run build/ci.go install

android: bls-zexe-android
	build/env.sh go run build/ci.go aar --local
	@echo "Done building."
	@echo "Import \"$(GOBIN)/geth.aar\" to use the library."

ios:
	build/env.sh go run build/ci.go xcode --local
	@echo "Done building."
	@echo "Import \"$(GOBIN)/Geth.framework\" to use the library."

test: all
	build/env.sh go run build/ci.go test

lint: ## Run linters.
	build/env.sh go run build/ci.go lint

clean:
	./build/clean_go_build_cache.sh
	rm -fr build/_workspace/pkg/ $(GOBIN)/*

# The devtools target installs tools required for 'go generate'.
# You need to put $GOBIN (or $GOPATH/bin) in your PATH to use 'go generate'.

devtools:
	env GOBIN= go get -u golang.org/x/tools/cmd/stringer
	env GOBIN= go get -u github.com/kevinburke/go-bindata/go-bindata
	env GOBIN= go get -u github.com/fjl/gencodec
	env GOBIN= go get -u github.com/golang/protobuf/protoc-gen-go
	env GOBIN= go install ./cmd/abigen
	@type "npm" 2> /dev/null || echo 'Please install node.js and npm'
	@type "solc" 2> /dev/null || echo 'Please install solc'
	@type "protoc" 2> /dev/null || echo 'Please install protoc'

swarm-devtools:
	env GOBIN= go install ./cmd/swarm/mimegen

# Cross Compilation Targets (xgo)

geth-cross: geth-linux geth-darwin geth-windows geth-android geth-ios
	@echo "Full cross compilation done:"
	@ls -ld $(GOBIN)/geth-*

geth-linux: geth-linux-386 geth-linux-amd64 geth-linux-arm geth-linux-mips64 geth-linux-mips64le
	@echo "Linux cross compilation done:"
	@ls -ld $(GOBIN)/geth-linux-*

geth-linux-386:
	rustup target add i686-unknown-linux-gnu
	cd vendor/github.com/celo-org/bls-zexe/bls && cargo build --target=i686-unknown-linux-gnu --release
	build/env.sh go run build/ci.go xgo -- --go=$(GO) --targets=linux/386 -v ./cmd/geth
	@echo "Linux 386 cross compilation done:"
	@ls -ld $(GOBIN)/geth-linux-* | grep 386

geth-linux-amd64:
	rustup target add x86_64-unknown-linux-gnu
	cd vendor/github.com/celo-org/bls-zexe/bls && cargo build --target=x86_64-unknown-linux-gnu --release
	build/env.sh go run build/ci.go xgo -- --go=$(GO) --targets=linux/amd64 -v ./cmd/geth
	@echo "Linux amd64 cross compilation done:"
	@ls -ld $(GOBIN)/geth-linux-* | grep amd64

geth-linux-arm: geth-linux-arm-5 geth-linux-arm-6 geth-linux-arm-7 geth-linux-arm64
	@echo "Linux ARM cross compilation done:"
	@ls -ld $(GOBIN)/geth-linux-* | grep arm

geth-linux-arm-5:
	# requires an arm compiler, on Ubuntu: sudo apt-get install gcc-arm-linux-gnueabi g++-arm-linux-gnueabi
	rustup target add arm-unknown-linux-gnueabi
	cd vendor/github.com/celo-org/bls-zexe/bls && cargo build --target=arm-unknown-linux-gnueabi --release
	build/env.sh go run build/ci.go xgo -- --go=$(GO) --targets=linux/arm-5 -v ./cmd/geth
	@echo "Linux ARMv5 cross compilation done:"
	@ls -ld $(GOBIN)/geth-linux-* | grep arm-5

geth-linux-arm-6:
	rustup target add arm-unknown-linux-gnueabi
	cd vendor/github.com/celo-org/bls-zexe/bls && cargo build --target=arm-unknown-linux-gnueabi --release
	build/env.sh go run build/ci.go xgo -- --go=$(GO) --targets=linux/arm-6 -v ./cmd/geth
	@echo "Linux ARMv6 cross compilation done:"
	@ls -ld $(GOBIN)/geth-linux-* | grep arm-6

geth-linux-arm-7:
	# requires an arm compiler, on Ubuntu: sudo apt-get install gcc-arm-linux-gnueabihf g++-arm-linux-gnueabihf
	rustup target add arm-unknown-linux-gnueabihf
	cd vendor/github.com/celo-org/bls-zexe/bls && cargo build --target=arm-unknown-linux-gnueabihf --release
	build/env.sh go run build/ci.go xgo -- --go=$(GO) --targets=linux/arm-7 -v  --tags arm7 ./cmd/geth
	@echo "Linux ARMv7 cross compilation done:"
	@ls -ld $(GOBIN)/geth-linux-* | grep arm-7

geth-linux-arm64:
	# requires an arm64 compiler, on Ubuntu: sudo apt-get install gcc-aarch64-linux-gnu	g++-aarch64-linux-gnu
	# seems to fail with a problem in the cloudflare library
	rustup target add aarch64-unknown-linux-gnu
	cd vendor/github.com/celo-org/bls-zexe/bls && cargo build --target=aarch64-unknown-linux-gnu --release
	build/env.sh go run build/ci.go xgo -- --go=$(GO) --targets=linux/arm64 -v ./cmd/geth
	@echo "Linux ARM64 cross compilation done:"
	@ls -ld $(GOBIN)/geth-linux-* | grep arm64

geth-linux-mips:
	# requires a mips compiler, on Ubuntu: sudo apt-get install gcc-mips-linux-gnu
	rustup target add mips-unknown-linux-gnu
	cd vendor/github.com/celo-org/bls-zexe/bls && cargo build --target=mips-unknown-linux-gnu --release
	build/env.sh go run build/ci.go xgo -- --go=$(GO) --targets=linux/mips --ldflags '-extldflags "-static"' -v ./cmd/geth
	@echo "Linux MIPS cross compilation done:"
	@ls -ld $(GOBIN)/geth-linux-* | grep mips

geth-linux-mipsle:
	# requires a mips compiler, on Ubuntu: sudo apt-get install gcc-mipsel-linux-gnu
	rustup target add mipsel-unknown-linux-gnu
	cd vendor/github.com/celo-org/bls-zexe/bls && cargo build --target=mipsel-unknown-linux-gnu --release
	build/env.sh go run build/ci.go xgo -- --go=$(GO) --targets=linux/mipsle --ldflags '-extldflags "-static"' -v ./cmd/geth
	@echo "Linux MIPSle cross compilation done:"
	@ls -ld $(GOBIN)/geth-linux-* | grep mipsle

geth-linux-mips64:
	# requires a mips compiler, on Ubuntu: sudo apt-get install gcc-mips64-linux-gnuabi64
	rustup target add mips64-unknown-linux-gnuabi64
	cd vendor/github.com/celo-org/bls-zexe/bls && cargo build --target=mips64-unknown-linux-gnuabi64 --release
	build/env.sh go run build/ci.go xgo -- --go=$(GO) --targets=linux/mips64 --ldflags '-extldflags "-static"' -v ./cmd/geth
	@echo "Linux MIPS64 cross compilation done:"
	@ls -ld $(GOBIN)/geth-linux-* | grep mips64

geth-linux-mips64le:
	# requires a mips compiler, on Ubuntu: sudo apt-get install gcc-mips64el-linux-gnuabi64
	rustup target add mips64el-unknown-linux-gnuabi64
	cd vendor/github.com/celo-org/bls-zexe/bls && cargo build --target=mips64el-unknown-linux-gnuabi64 --release
	build/env.sh go run build/ci.go xgo -- --go=$(GO) --targets=linux/mips64le --ldflags '-extldflags "-static"' -v ./cmd/geth
	@echo "Linux MIPS64le cross compilation done:"
	@ls -ld $(GOBIN)/geth-linux-* | grep mips64le

geth-darwin: geth-darwin-386 geth-darwin-amd64
	@echo "Darwin cross compilation done:"
	@ls -ld $(GOBIN)/geth-darwin-*

geth-darwin-386:
	# needs include files for asm errno, on Ubuntu: sudo apt-get install linux-libc-dev:i386
	# currently doesn't compile on Ubuntu
	rustup target add i686-apple-darwin
	cd vendor/github.com/celo-org/bls-zexe/bls && cargo build --target=i686-apple-darwin --release
	build/env.sh go run build/ci.go xgo -- --go=$(GO) --targets=darwin/386 -v ./cmd/geth
	@echo "Darwin 386 cross compilation done:"
	@ls -ld $(GOBIN)/geth-darwin-* | grep 386

geth-darwin-amd64:
	# needs include files for asm errno, on Ubuntu: sudo apt-get install linux-libc-dev
	# currently doesn't compile on Ubuntu
	rustup target add x86_64-apple-darwin
	cd vendor/github.com/celo-org/bls-zexe/bls && cargo build --target=x86_64-apple-darwin --release
	build/env.sh go run build/ci.go xgo -- --go=$(GO) --targets=darwin/amd64 -v ./cmd/geth
	@echo "Darwin amd64 cross compilation done:"
	@ls -ld $(GOBIN)/geth-darwin-* | grep amd64

geth-windows: geth-windows-386 geth-windows-amd64
	@echo "Windows cross compilation done:"
	@ls -ld $(GOBIN)/geth-windows-*

geth-windows-386:
	# currently doesn't compile on Ubuntu, missing libunwind in xgo
	rustup target add i686-pc-windows-msvc
	cd vendor/github.com/celo-org/bls-zexe/bls && cargo build --target=i686-pc-windows-msvc --release
	build/env.sh go run build/ci.go xgo -- --go=$(GO) --targets=windows/386 -v ./cmd/geth
	@echo "Windows 386 cross compilation done:"
	@ls -ld $(GOBIN)/geth-windows-* | grep 386

geth-windows-amd64:
	# currently doesn't compile on Ubuntu, missing libunwind in xgo
	rustup target add x86_64-pc-windows-gnu
	cd vendor/github.com/celo-org/bls-zexe/bls && cargo build --target=x86_64-pc-windows-gnu --release
	build/env.sh go run build/ci.go xgo -- --go=$(GO) --targets=windows/amd64 -v ./cmd/geth
	@echo "Windows amd64 cross compilation done:"
	@ls -ld $(GOBIN)/geth-windows-* | grep amd64
