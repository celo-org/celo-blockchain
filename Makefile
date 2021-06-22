# This Makefile is meant to be used by people that do not usually work
# with Go source code. If you know what GOPATH is then you probably
# don't need to bother with make.

.PHONY: geth android ios geth-cross evm all test clean
.PHONY: geth-linux geth-linux-386 geth-linux-amd64 geth-linux-mips64 geth-linux-mips64le
.PHONY: geth-linux-arm geth-linux-arm-5 geth-linux-arm-6 geth-linux-arm-7 geth-linux-arm64
.PHONY: geth-darwin geth-darwin-amd64
.PHONY: geth-windows geth-windows-386 geth-windows-amd64
.PHONY: prepare-system-contracts

GOBIN = ./build/bin
GO ?= latest
GORUN = env GO111MODULE=on go run

LSB_exists := $(shell command -v lsb_release 2> /dev/null)

OS :=
ifeq ("$(LSB_exists)","")
	OS = darwin
else
	OS = linux
endif

MONOREPO_COMMIT=celo-core-contracts-v3.rc0

# We checkout the monorepo as a sibling to the celo-blockchain dir because the
# huge amount of files in the monorepo interferes with tooling such as gopls,
# which becomes very slow.
MONOREPO_PATH=../monorepo

# example NDK values
export NDK_VERSION ?= android-ndk-r19c
export ANDROID_NDK ?= $(PWD)/ndk_bundle/$(NDK_VERSION)

geth:
	$(GORUN) build/ci.go install ./cmd/geth
	@echo "Done building."
	@echo "Run \"$(GOBIN)/geth\" to launch geth."

# This rule checks out celo-monorepo under MONOREPO_PATH at commit
# MONOREPO_COMMIT and compiles the system solidty contracts. A softlink is
# created at ./monorepo so that code in this repo can always access the system
# contracts with a consistent path.
prepare-system-contracts: ./monorepo ./monorepo/packages/protocol/build

# Clone the monorepo and setup softlink.
./monorepo: 
	@echo "Cloning monorepo at $(MONOREPO_COMMIT)"
	@git clone --depth 1 --branch $(MONOREPO_COMMIT) https://github.com/celo-org/celo-monorepo.git $(MONOREPO_PATH)
	@ln -s $(MONOREPO_PATH) ./monorepo

# If any of the source files found by the find command are more recent than the
# build dir then we remove the build dir, yarn install and rebuild the contracts.
./monorepo/packages/protocol/build: $(shell 2>/dev/null find ./monorepo/packages/protocol -not -path "*/node_modules*" -not -path "./monorepo/packages/protocol/test*" -not -path "./monorepo/packages/protocol/build*" -not -path "./monorepo/packages/protocol/types*")
	@node --version | grep "^v10" || (echo "node v10 is required to build the monorepo (nvm use 10)" && exit 1)
	@echo Running yarn install and compiling contracts
	@cd ./monorepo && rm -rf packages/protocol/build && yarn && cd packages/protocol && yarn run build:sol


geth-musl:
	$(GORUN) build/ci.go install -musl ./cmd/geth
	@echo "Done building with musl."
	@echo "Run \"$(GOBIN)/geth\" to launch geth."

check_android_env:
	@test $${ANDROID_NDK?Please set environment variable ANDROID_NDK}
	@test $${ANDROID_HOME?Please set environment variable ANDROID_HOME}

ndk_bundle: check_android_env
ifeq ("$(wildcard $(ANDROID_NDK))","")
	@test $${NDK_VERSION?Please set environment variable NDK_VERSION}
	curl --silent --show-error --location --fail --retry 3 --output /tmp/$(NDK_VERSION).zip \
		https://dl.google.com/android/repository/$(NDK_VERSION)-$(OS)-x86_64.zip && \
		rm -rf $(ANDROID_NDK) && \
		mkdir -p $(ANDROID_NDK) && \
		unzip -q /tmp/$(NDK_VERSION).zip -d $(ANDROID_NDK)/.. && \
		rm /tmp/$(NDK_VERSION).zip
else
ifeq ("$(wildcard $(ANDROID_NDK)/toolchains/llvm/prebuilt/$(OS)-x86_64)","")
	$(error "Android NDK is installed but doesn't contain an llvm cross-compilation toolchain. Delete your current NDK or modify the ANDROID_NDK environment variable to an empty directory download it automatically.")
endif
endif

swarm:
	$(GORUN) build/ci.go install ./cmd/swarm
	@echo "Done building."
	@echo "Run \"$(GOBIN)/swarm\" to launch swarm."

all:
	$(GORUN) build/ci.go install

all-musl:
	$(GORUN) build/ci.go install -musl

android:
	@echo "Applying patch for mobile libs..."
	git apply patches/mobileLibsForBuild.patch
	ANDROID_NDK_HOME=$(ANDROID_NDK) $(GORUN) build/ci.go aar --local --metrics-default
	@echo "Done building."
	@echo "Import \"$(GOBIN)/geth.aar\" to use the library."
	@echo "Remove patch for mobile libs..."
	git apply -R patches/mobileLibsForBuild.patch

ios:
	DISABLE_BITCODE=true $(GORUN) build/ci.go xcode --local --metrics-default
	pushd "$(GOBIN)"; rm -rf Geth.framework.tgz; tar -czvf Geth.framework.tgz Geth.framework; popd
	# Geth.framework is a static framework, so we have to also keep the other static libs it depends on
	# in order to link it to the final app
	# One day gomobile will probably support xcframework which would solve this ;-)
	cp -f "$$(go list -m -f "{{ .Dir }}" github.com/celo-org/celo-bls-go)/libs/universal/libbls_snark_sys.a" .
	@echo "Done building."
	@echo "Import \"$(GOBIN)/Geth.framework\" to use the library."

test: all
	$(GORUN) build/ci.go test $(TEST_FLAGS)

lint: ## Run linters.
	$(GORUN) build/ci.go lint

clean-geth:
	env GO111MODULE=on go clean -cache
	rm -fr build/_workspace/pkg/ $(GOBIN)/*

clean: clean-geth

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

# Cross Compilation Targets (xgo)

geth-cross: geth-linux geth-darwin geth-windows geth-android geth-ios
	@echo "Full cross compilation done:"
	@ls -ld $(GOBIN)/geth-*

geth-linux: geth-linux-386 geth-linux-amd64 geth-linux-arm geth-linux-mips64 geth-linux-mips64le
	@echo "Linux cross compilation done:"
	@ls -ld $(GOBIN)/geth-linux-*

geth-linux-386:
	$(GORUN) build/ci.go xgo -- --go=$(GO) --targets=linux/386 -v ./cmd/geth
	@echo "Linux 386 cross compilation done:"
	@ls -ld $(GOBIN)/geth-linux-* | grep 386

geth-linux-amd64:
	$(GORUN) build/ci.go xgo -- --go=$(GO) --targets=linux/amd64 -v ./cmd/geth
	@echo "Linux amd64 cross compilation done:"
	@ls -ld $(GOBIN)/geth-linux-* | grep amd64

geth-linux-arm: geth-linux-arm-5 geth-linux-arm-6 geth-linux-arm-7 geth-linux-arm64
	@echo "Linux ARM cross compilation done:"
	@ls -ld $(GOBIN)/geth-linux-* | grep arm

geth-linux-arm-5:
	# requires an arm compiler, on Ubuntu: sudo apt-get install gcc-arm-linux-gnueabi g++-arm-linux-gnueabi
	$(GORUN) build/ci.go xgo -- --go=$(GO) --targets=linux/arm-5 -v ./cmd/geth
	@echo "Linux ARMv5 cross compilation done:"
	@ls -ld $(GOBIN)/geth-linux-* | grep arm-5

geth-linux-arm-6:
	$(GORUN) build/ci.go xgo -- --go=$(GO) --targets=linux/arm-6 -v ./cmd/geth
	@echo "Linux ARMv6 cross compilation done:"
	@ls -ld $(GOBIN)/geth-linux-* | grep arm-6

geth-linux-arm-7:
	# requires an arm compiler, on Ubuntu: sudo apt-get install gcc-arm-linux-gnueabihf g++-arm-linux-gnueabihf
	$(GORUN) build/ci.go xgo -- --go=$(GO) --targets=linux/arm-7 -v  --tags arm7 ./cmd/geth
	@echo "Linux ARMv7 cross compilation done:"
	@ls -ld $(GOBIN)/geth-linux-* | grep arm-7

geth-linux-arm64:
	# requires an arm64 compiler, on Ubuntu: sudo apt-get install gcc-aarch64-linux-gnu	g++-aarch64-linux-gnu
	$(GORUN) build/ci.go xgo -- --go=$(GO) --targets=linux/arm64 -v ./cmd/geth
	@echo "Linux ARM64 cross compilation done:"
	@ls -ld $(GOBIN)/geth-linux-* | grep arm64

geth-linux-mips:
	# requires a mips compiler, on Ubuntu: sudo apt-get install gcc-mips-linux-gnu
	$(GORUN) build/ci.go xgo -- --go=$(GO) --targets=linux/mips --ldflags '-extldflags "-static"' -v ./cmd/geth
	@echo "Linux MIPS cross compilation done:"
	@ls -ld $(GOBIN)/geth-linux-* | grep mips

geth-linux-mipsle:
	# requires a mips compiler, on Ubuntu: sudo apt-get install gcc-mipsel-linux-gnu
	$(GORUN) build/ci.go xgo -- --go=$(GO) --targets=linux/mipsle --ldflags '-extldflags "-static"' -v ./cmd/geth
	@echo "Linux MIPSle cross compilation done:"
	@ls -ld $(GOBIN)/geth-linux-* | grep mipsle

geth-linux-mips64:
	# requires a mips compiler, on Ubuntu: sudo apt-get install gcc-mips64-linux-gnuabi64
	$(GORUN) build/ci.go xgo -- --go=$(GO) --targets=linux/mips64 --ldflags '-extldflags "-static"' -v ./cmd/geth
	@echo "Linux MIPS64 cross compilation done:"
	@ls -ld $(GOBIN)/geth-linux-* | grep mips64

geth-linux-mips64le:
	# requires a mips compiler, on Ubuntu: sudo apt-get install gcc-mips64el-linux-gnuabi64
	$(GORUN) build/ci.go xgo -- --go=$(GO) --targets=linux/mips64le --ldflags '-extldflags "-static"' -v ./cmd/geth
	@echo "Linux MIPS64le cross compilation done:"
	@ls -ld $(GOBIN)/geth-linux-* | grep mips64le

geth-darwin: geth-darwin-amd64
	@echo "Darwin cross compilation done:"
	@ls -ld $(GOBIN)/geth-darwin-*

geth-darwin-amd64:
	# needs include files for asm errno, on Ubuntu: sudo apt-get install linux-libc-dev
	# currently doesn't compile on Ubuntu
	$(GORUN) build/ci.go xgo -- --go=$(GO) --targets=darwin/amd64 -v ./cmd/geth
	@echo "Darwin amd64 cross compilation done:"
	@ls -ld $(GOBIN)/geth-darwin-* | grep amd64

geth-windows: geth-windows-386 geth-windows-amd64
	@echo "Windows cross compilation done:"
	@ls -ld $(GOBIN)/geth-windows-*

geth-windows-386:
	# currently doesn't compile on Ubuntu, missing libunwind in xgo
	$(GORUN) build/ci.go xgo -- --go=$(GO) --targets=windows/386 -v ./cmd/geth
	@echo "Windows 386 cross compilation done:"
	@ls -ld $(GOBIN)/geth-windows-* | grep 386

geth-windows-amd64:
	# currently doesn't compile on Ubuntu, missing libunwind in xgo
	$(GORUN) build/ci.go xgo -- --go=$(GO) --targets=windows/amd64 -v ./cmd/geth
	@echo "Windows amd64 cross compilation done:"
	@ls -ld $(GOBIN)/geth-windows-* | grep amd64
